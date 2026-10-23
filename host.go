package main

// The simulated host keeps every installation's data apart on disk:
//
//	<data>/installations/<installation_id>/binding.json
//	<data>/installations/<installation_id>/events.jsonl
//	<data>/installations/<installation_id>/entities/<entity_id>/entity.json
//	<data>/installations/<installation_id>/entities/<entity_id>/thoughts/<thought_id>.json
//
// At most ONE controller service runs at a time, bound to exactly one
// installation. The host only ever loads the active installation's folder,
// so a second test installation cannot see the first one's identities or
// thoughts. Every thought read and write names its entity and is looked up
// only inside that entity's own folder.
//
// None of this is authenticated: any local program can call these routes.
// Scoping is enforced server-side, but it is not authorization.
import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

const schemaVersion = 1

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

var (
	ErrInvalid    = errors.New("invalid request")
	ErrNotStarted = errors.New("the controller service is not started — open Identity and let its simulated launcher start it")
	ErrNotFound   = errors.New("not found")
	ErrConflict   = errors.New("conflict")
)

// Managed entity types a controller can hold thoughts for. Administrator and
// system-service identities are part of the binding, not thought profiles.
var entityTypes = map[string]string{
	"ai_companion": "AI companion", "business": "Business", "person": "Person",
	"project": "Project", "other": "Other",
}

// Thought states, in the order the dashboard rows show them.
var thoughtStates = []struct{ Key, Row string }{
	{"continuing", "Continue Thinking"},
	{"active", "Active"},
	{"sleeping", "Sleeping / Waiting"},
	{"resolved", "Resolved / History"},
}

func validState(s string) bool {
	for _, t := range thoughtStates {
		if t.Key == s {
			return true
		}
	}
	return false
}

type Binding struct {
	SchemaVersion  int       `json:"schema_version"`
	InstallationID string    `json:"installation_id"`
	ServiceID      string    `json:"service_id"`
	AdminID        string    `json:"admin_id"`
	BoundAt        time.Time `json:"bound_at"`
	Note           string    `json:"note"`
}

type Entity struct {
	SchemaVersion  int       `json:"schema_version"`
	EntityID       string    `json:"entity_id"`
	EntityType     string    `json:"entity_type"`
	DisplayName    string    `json:"display_name"`
	InstallationID string    `json:"installation_id"`
	RegisteredAt   time.Time `json:"registered_at"`
}

type Note struct {
	At   time.Time `json:"at"`
	Text string    `json:"text"`
}

type HistoryItem struct {
	At     time.Time `json:"at"`
	Kind   string    `json:"kind"` // created | state | note
	From   string    `json:"from,omitempty"`
	To     string    `json:"to,omitempty"`
	Detail string    `json:"detail"`
}

type Thought struct {
	SchemaVersion  int           `json:"schema_version"`
	ThoughtID      string        `json:"thought_id"`
	EntityID       string        `json:"entity_id"`
	InstallationID string        `json:"installation_id"`
	Title          string        `json:"title"`
	State          string        `json:"state"`
	Simulated      bool          `json:"simulated"`
	Origin         string        `json:"origin"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
	Notes          []Note        `json:"notes"`
	History        []HistoryItem `json:"history"`
	WorkerCalls    int           `json:"worker_calls"`
	RequestID      string        `json:"request_id,omitempty"`
}

type Problem struct {
	File   string `json:"file"`
	Reason string `json:"reason"`
}

// activeService is the one running controller service.
type activeService struct {
	binding   Binding
	dir       string
	entities  map[string]Entity
	thoughts  map[string]map[string]Thought // entity -> thought id -> thought
	problems  []Problem
	events    *EventLog
	startedAt time.Time
	requestID string
	launcher  string
	consentID string
	requests  map[string]string // create-thought request id -> thought id
}

type StartResult struct {
	Result        string            `json:"result"` // started | already_running | duplicate_request | refused
	Reason        string            `json:"reason,omitempty"`
	RequestID     string            `json:"request_id"`
	Simulated     bool              `json:"simulated"`
	Registrations map[string]string `json:"registrations,omitempty"` // entity id -> registered | already_registered | conflict
	At            time.Time         `json:"at"`
}

type Counters struct {
	StartRequests     int `json:"start_requests"`
	ServicesStarted   int `json:"services_started"`
	AlreadyRunning    int `json:"already_running"`
	DuplicateRequests int `json:"duplicate_requests"`
	Refused           int `json:"refused"`
	Stops             int `json:"stops"`
	ModelCalls        int `json:"model_calls"` // always 0: there is no AI worker in this prototype
}

type Host struct {
	mu       sync.Mutex
	cfg      Config
	events   *EventLog
	svc      *activeService
	seen     map[string]StartResult
	counters Counters
	started  time.Time
	now      func() time.Time
}

func openHost(cfg Config, events *EventLog) (*Host, error) {
	if err := os.MkdirAll(filepath.Join(cfg.DataDir, "installations"), 0o700); err != nil {
		return nil, fmt.Errorf("can't create installations folder: %w", err)
	}
	return &Host{cfg: cfg, events: events, seen: map[string]StartResult{}, started: time.Now(), now: time.Now}, nil
}

func (h *Host) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.svc != nil {
		h.svc.events.Close()
	}
}

// ---------- lifecycle (SIMULATED launcher target) ----------

type StartRequest struct {
	RequestID      string      `json:"request_id"`
	InstallationID string      `json:"installation_id"`
	ServiceID      string      `json:"service_id"`
	AdminID        string      `json:"admin_id"`
	ApprovedCommit string      `json:"approved_commit"`
	Launcher       string      `json:"launcher"`
	ConsentID      string      `json:"consent_id"`
	Entities       []EntityRef `json:"entities"`
}

type EntityRef struct {
	EntityID    string `json:"entity_id"`
	EntityType  string `json:"entity_type"`
	DisplayName string `json:"display_name"`
}

func (r StartRequest) validate() error {
	switch {
	case r.RequestID == "" || len(r.RequestID) > 100:
		return fmt.Errorf("%w: request_id is required (max 100 chars)", ErrInvalid)
	case !uuidRe.MatchString(r.InstallationID):
		return fmt.Errorf("%w: installation_id must be an Identity-issued UUID", ErrInvalid)
	case !uuidRe.MatchString(r.ServiceID):
		return fmt.Errorf("%w: service_id must be an Identity-issued UUID", ErrInvalid)
	case !uuidRe.MatchString(r.AdminID):
		return fmt.Errorf("%w: admin_id must be an Identity-issued UUID", ErrInvalid)
	case r.InstallationID == r.ServiceID || r.InstallationID == r.AdminID || r.ServiceID == r.AdminID:
		return fmt.Errorf("%w: installation, administrator and service IDs must all differ", ErrInvalid)
	case len(r.Entities) > 50:
		return fmt.Errorf("%w: too many entities", ErrInvalid)
	}
	return validateRefs(r.Entities, r)
}

func validateRefs(refs []EntityRef, r StartRequest) error {
	seen := map[string]bool{}
	for _, e := range refs {
		if !uuidRe.MatchString(e.EntityID) {
			return fmt.Errorf("%w: entity_id %q is not a UUID", ErrInvalid, e.EntityID)
		}
		if e.EntityID == r.InstallationID || e.EntityID == r.ServiceID || e.EntityID == r.AdminID {
			return fmt.Errorf("%w: a managed entity can't reuse the installation, administrator or service ID", ErrInvalid)
		}
		if entityTypes[e.EntityType] == "" {
			return fmt.Errorf("%w: unsupported entity_type %q", ErrInvalid, e.EntityType)
		}
		if _, err := cleanText(e.DisplayName, 80, "display name"); err != nil {
			return err
		}
		if seen[e.EntityID] {
			return fmt.Errorf("%w: entity %s listed twice", ErrInvalid, e.EntityID)
		}
		seen[e.EntityID] = true
	}
	return nil
}

// Start is the simulated lifecycle adapter's only "start" action. It starts
// no process: it binds and opens the requesting installation's controller
// service inside this already-running host. One result per request_id.
func (h *Host) Start(r StartRequest) (StartResult, error) {
	if err := r.validate(); err != nil {
		return StartResult{}, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.counters.StartRequests++
	if prev, ok := h.seen[r.RequestID]; ok {
		h.counters.DuplicateRequests++
		h.events.Add("duplicate", "start request "+r.RequestID+" was already handled ("+prev.Result+") — nothing done again", r.InstallationID)
		prev.Result, prev.Reason = "duplicate_request", "this request_id was already handled with result: "+prev.Result
		return prev, nil
	}
	res := StartResult{RequestID: r.RequestID, Simulated: true, At: h.now().UTC()}
	finish := func(result, reason string) (StartResult, error) {
		res.Result, res.Reason = result, reason
		h.seen[r.RequestID] = res
		if result == "refused" {
			h.counters.Refused++
		}
		h.events.Add("lifecycle", fmt.Sprintf("start request %s from %q: %s %s", r.RequestID, firstSet(r.Launcher, "unnamed launcher"), result, reason), r.InstallationID)
		return res, nil
	}
	if r.ApprovedCommit != h.cfg.Commit {
		return finish("refused", fmt.Sprintf("version mismatch: this host is running commit %s but the request approved %s", h.cfg.Commit, firstSet(r.ApprovedCommit, "(none)")))
	}
	if h.svc != nil {
		b := h.svc.binding
		if b.InstallationID != r.InstallationID {
			return finish("refused", "busy: this simulated host is already running the controller service for a DIFFERENT installation; stop that one first (from its own Identity profile or this host's page)")
		}
		if b.ServiceID != r.ServiceID || b.AdminID != r.AdminID {
			return finish("refused", "identity mismatch: the running service for this installation has a different service or administrator ID")
		}
		res.Registrations = h.registerLocked(r.Entities)
		h.counters.AlreadyRunning++
		return finish("already_running", "the controller service for this installation is already running — reused, nothing started")
	}
	dir := filepath.Join(h.cfg.DataDir, "installations", r.InstallationID)
	if err := os.MkdirAll(filepath.Join(dir, "entities"), 0o700); err != nil {
		return finish("refused", "can't create the installation folder: "+err.Error())
	}
	bpath := filepath.Join(dir, "binding.json")
	var b Binding
	if err := readJSONStrict(bpath, &b); err == nil {
		if b.InstallationID != r.InstallationID || b.ServiceID != r.ServiceID || b.AdminID != r.AdminID {
			return finish("refused", "this installation's saved binding names a different service or administrator ID — refusing to re-bind (data left untouched)")
		}
	} else if errors.Is(err, os.ErrNotExist) {
		b = Binding{SchemaVersion: schemaVersion, InstallationID: r.InstallationID, ServiceID: r.ServiceID, AdminID: r.AdminID,
			BoundAt: h.now().UTC(), Note: "SIMULATED unauthenticated binding written by tc-sim-handshake/0.1"}
		if err := writeJSONAtomic(bpath, b); err != nil {
			return finish("refused", "binding NOT saved: "+err.Error())
		}
	} else {
		return finish("refused", "can't read this installation's binding.json: "+err.Error()+" (file left untouched)")
	}
	svc := &activeService{binding: b, dir: dir, entities: map[string]Entity{}, thoughts: map[string]map[string]Thought{},
		events: openEventLog(filepath.Join(dir, "events.jsonl"), false), startedAt: h.now().UTC(),
		requestID: r.RequestID, launcher: r.Launcher, consentID: r.ConsentID, requests: map[string]string{}}
	svc.load()
	h.svc = svc
	res.Registrations = h.registerLocked(r.Entities)
	h.counters.ServicesStarted++
	svc.events.Add("start", fmt.Sprintf("controller service started (SIMULATED) by request %s; %d entities, %d thoughts loaded", r.RequestID, len(svc.entities), svc.thoughtCount()), b.ServiceID)
	for _, p := range svc.problems {
		svc.events.Add("problem", "could not load "+p.File+": "+p.Reason+" (file left untouched)", "")
	}
	return finish("started", fmt.Sprintf("controller service started for this installation (SIMULATED — no process was launched); %d entities, %d thoughts restored from disk", len(svc.entities), svc.thoughtCount()))
}

// registerLocked registers entities with the active service. Repeats return
// already_registered; a changed type is a conflict and is not applied.
func (h *Host) registerLocked(refs []EntityRef) map[string]string {
	out := map[string]string{}
	svc := h.svc
	for _, ref := range refs {
		name, _ := cleanText(ref.DisplayName, 80, "display name")
		if e, ok := svc.entities[ref.EntityID]; ok {
			if e.EntityType != ref.EntityType {
				out[ref.EntityID] = "conflict"
				svc.events.Add("register", "registration conflict: entity is already registered with type "+e.EntityType, ref.EntityID)
				continue
			}
			if e.DisplayName != name { // display names may change; the ID never does
				e.DisplayName = name
				if err := writeJSONAtomic(filepath.Join(svc.dir, "entities", e.EntityID, "entity.json"), e); err == nil {
					svc.entities[e.EntityID] = e
				}
			}
			out[ref.EntityID] = "already_registered"
			continue
		}
		e := Entity{SchemaVersion: schemaVersion, EntityID: ref.EntityID, EntityType: ref.EntityType, DisplayName: name,
			InstallationID: svc.binding.InstallationID, RegisteredAt: h.now().UTC()}
		edir := filepath.Join(svc.dir, "entities", e.EntityID)
		if err := os.MkdirAll(filepath.Join(edir, "thoughts"), 0o700); err != nil {
			out[ref.EntityID] = "error: " + err.Error()
			continue
		}
		if err := writeJSONAtomic(filepath.Join(edir, "entity.json"), e); err != nil {
			out[ref.EntityID] = "error: NOT saved: " + err.Error()
			svc.events.Add("error", "entity registration NOT saved: "+err.Error(), e.EntityID)
			continue
		}
		svc.entities[e.EntityID] = e
		svc.thoughts[e.EntityID] = map[string]Thought{}
		out[ref.EntityID] = "registered"
		svc.events.Add("register", "registered "+entityTypes[e.EntityType]+" (SIMULATED registration, no authentication)", e.EntityID)
	}
	return out
}

// Register adds entities to the running service of the named installation.
func (h *Host) Register(installationID, serviceID string, refs []EntityRef) (map[string]string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.svc == nil {
		return nil, ErrNotStarted
	}
	b := h.svc.binding
	if b.InstallationID != installationID || b.ServiceID != serviceID {
		return nil, fmt.Errorf("%w: the running service belongs to a different installation or service ID — registration denied", ErrConflict)
	}
	if err := validateRefs(refs, StartRequest{InstallationID: b.InstallationID, ServiceID: b.ServiceID, AdminID: b.AdminID}); err != nil {
		return nil, err
	}
	return h.registerLocked(refs), nil
}

// StopService stops the running controller service (no process ends: this
// is the simulated lifecycle). If installationID is set it must match.
func (h *Host) StopService(reason string, installationID ...string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.svc == nil {
		return ErrNotStarted
	}
	if len(installationID) > 0 && installationID[0] != h.svc.binding.InstallationID {
		return fmt.Errorf("%w: the running service belongs to a different installation", ErrConflict)
	}
	h.svc.events.Add("stop", "controller service stopped: "+reason, h.svc.binding.ServiceID)
	h.svc.events.Close()
	h.events.Add("lifecycle", "controller service stopped: "+reason, h.svc.binding.InstallationID)
	h.svc = nil
	h.counters.Stops++
	return nil
}

func (s *activeService) load() {
	entries, err := os.ReadDir(filepath.Join(s.dir, "entities"))
	if err != nil {
		s.problems = append(s.problems, Problem{File: "entities/", Reason: err.Error()})
		return
	}
	for _, d := range entries {
		if !d.IsDir() {
			continue
		}
		edir := filepath.Join(s.dir, "entities", d.Name())
		var e Entity
		if err := readJSONStrict(filepath.Join(edir, "entity.json"), &e); err != nil {
			s.problems = append(s.problems, Problem{File: "entities/" + d.Name() + "/entity.json", Reason: err.Error()})
			continue
		}
		if e.EntityID != d.Name() || e.InstallationID != s.binding.InstallationID {
			s.problems = append(s.problems, Problem{File: "entities/" + d.Name() + "/entity.json", Reason: "entity file does not belong to this folder or installation"})
			continue
		}
		s.entities[e.EntityID] = e
		s.thoughts[e.EntityID] = map[string]Thought{}
		tdir := filepath.Join(edir, "thoughts")
		files, _ := os.ReadDir(tdir)
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
				continue
			}
			var t Thought
			rel := "entities/" + d.Name() + "/thoughts/" + f.Name()
			if err := readJSONStrict(filepath.Join(tdir, f.Name()), &t); err != nil {
				s.problems = append(s.problems, Problem{File: rel, Reason: err.Error()})
				continue
			}
			if t.ThoughtID+".json" != f.Name() || t.EntityID != e.EntityID || t.InstallationID != e.InstallationID || !validState(t.State) {
				s.problems = append(s.problems, Problem{File: rel, Reason: "thought file does not belong to this entity/installation or has an unknown state"})
				continue
			}
			s.thoughts[e.EntityID][t.ThoughtID] = t
		}
	}
}

func (s *activeService) thoughtCount() int {
	n := 0
	for _, m := range s.thoughts {
		n += len(m)
	}
	return n
}

// ---------- thoughts (always scoped to one entity) ----------

func (h *Host) scoped(entityID string) (*activeService, error) {
	if h.svc == nil {
		return nil, ErrNotStarted
	}
	if !uuidRe.MatchString(entityID) {
		return nil, fmt.Errorf("%w: entity ID is not valid", ErrInvalid)
	}
	if _, ok := h.svc.entities[entityID]; !ok {
		return nil, fmt.Errorf("%w: entity is not registered in this installation", ErrNotFound)
	}
	return h.svc, nil
}

func (h *Host) ListThoughts(entityID string) ([]Thought, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	svc, err := h.scoped(entityID)
	if err != nil {
		return nil, err
	}
	out := make([]Thought, 0, len(svc.thoughts[entityID]))
	for _, t := range svc.thoughts[entityID] {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

func (h *Host) GetThought(entityID, thoughtID string) (Thought, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	svc, err := h.scoped(entityID)
	if err != nil {
		return Thought{}, err
	}
	t, ok := svc.thoughts[entityID][thoughtID] // only this entity's own collection is searched
	if !ok {
		return Thought{}, fmt.Errorf("%w: no such thought for this entity", ErrNotFound)
	}
	return t, nil
}

func (h *Host) CreateThought(entityID, title, state, requestID, origin string) (Thought, bool, error) {
	title, err := cleanText(title, 140, "title")
	if err != nil {
		return Thought{}, false, err
	}
	if !validState(state) {
		return Thought{}, false, fmt.Errorf("%w: state must be continuing, active, sleeping or resolved", ErrInvalid)
	}
	if len(requestID) > 100 {
		return Thought{}, false, fmt.Errorf("%w: request_id too long", ErrInvalid)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	svc, err := h.scoped(entityID)
	if err != nil {
		return Thought{}, false, err
	}
	if requestID != "" {
		if prev, ok := svc.requests[requestID]; ok {
			for _, m := range svc.thoughts {
				if t, ok := m[prev]; ok {
					return t, true, nil
				}
			}
		}
	}
	id, err := newUUID()
	if err != nil {
		return Thought{}, false, err
	}
	now := h.now().UTC()
	t := Thought{SchemaVersion: schemaVersion, ThoughtID: id, EntityID: entityID, InstallationID: svc.binding.InstallationID,
		Title: title, State: state, Simulated: true, Origin: origin, CreatedAt: now, UpdatedAt: now, Notes: []Note{},
		History: []HistoryItem{{At: now, Kind: "created", To: state, Detail: "created by " + origin}}, RequestID: requestID}
	if err := writeJSONAtomic(filepath.Join(svc.dir, "entities", entityID, "thoughts", id+".json"), t); err != nil {
		svc.events.Add("error", "thought NOT saved: "+err.Error(), entityID)
		return Thought{}, false, fmt.Errorf("thought NOT saved: %w", err)
	}
	svc.thoughts[entityID][id] = t
	if requestID != "" {
		svc.requests[requestID] = id
	}
	svc.events.Add("thought", "SIMULATED thought created ("+state+") for entity "+entityID, id)
	return t, false, nil
}

// Update changes a thought's state and/or adds a note. The change is saved
// before memory is updated, so a failed save changes nothing.
func (h *Host) UpdateThought(entityID, thoughtID, newState, note string) (Thought, error) {
	if newState != "" && !validState(newState) {
		return Thought{}, fmt.Errorf("%w: unknown state", ErrInvalid)
	}
	if note != "" {
		var err error
		if note, err = cleanText(note, 1000, "note"); err != nil {
			return Thought{}, err
		}
	}
	if newState == "" && note == "" {
		return Thought{}, fmt.Errorf("%w: nothing to change", ErrInvalid)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	svc, err := h.scoped(entityID)
	if err != nil {
		return Thought{}, err
	}
	t, ok := svc.thoughts[entityID][thoughtID]
	if !ok {
		return Thought{}, fmt.Errorf("%w: no such thought for this entity", ErrNotFound)
	}
	now := h.now().UTC()
	t.Notes = append([]Note(nil), t.Notes...)
	t.History = append([]HistoryItem(nil), t.History...)
	if newState != "" && newState != t.State {
		t.History = append(t.History, HistoryItem{At: now, Kind: "state", From: t.State, To: newState, Detail: "owner changed state (test control)"})
		t.State = newState
	}
	if note != "" {
		t.Notes = append(t.Notes, Note{At: now, Text: note})
		t.History = append(t.History, HistoryItem{At: now, Kind: "note", Detail: "owner added a note"})
	}
	t.UpdatedAt = now
	if err := writeJSONAtomic(filepath.Join(svc.dir, "entities", entityID, "thoughts", thoughtID+".json"), t); err != nil {
		svc.events.Add("error", "thought change NOT saved: "+err.Error(), thoughtID)
		return Thought{}, fmt.Errorf("change NOT saved: %w", err)
	}
	svc.thoughts[entityID][thoughtID] = t
	svc.events.Add("thought", "thought updated (state "+t.State+")", thoughtID)
	return t, nil
}

// sampleSet returns one simulated thought per state, worded for the
// entity's type. These are test fixtures, not anything a model produced.
func sampleSet(entityType string) [][2]string {
	if entityType == "business" {
		return [][2]string{
			{"continuing", "Draft the weekly repair-queue summary"},
			{"active", "Compare prices for refurbished replacement parts"},
			{"sleeping", "Waiting on a customer reply about pickup time"},
			{"resolved", "Chose Saturday shop hours"},
		}
	}
	return [][2]string{
		{"continuing", "How to greet the owner after a long route"},
		{"active", "Notice the chat bridge has been quiet today"},
		{"sleeping", "Wait for a screen capture before describing the game"},
		{"resolved", "Picked a name for the morning check-in"},
	}
}

// ---------- snapshots for the API ----------

type EntitySummary struct {
	Entity
	TypeLabel string         `json:"type_label"`
	Counts    map[string]int `json:"counts"`
	Total     int            `json:"total"`
}

type Workspace struct {
	Started   bool            `json:"started"`
	Binding   *Binding        `json:"binding,omitempty"`
	StartedAt *time.Time      `json:"started_at,omitempty"`
	RequestID string          `json:"start_request_id,omitempty"`
	Launcher  string          `json:"launcher,omitempty"`
	ConsentID string          `json:"consent_id,omitempty"`
	Entities  []EntitySummary `json:"entities"`
	Problems  []Problem       `json:"problems"`
	Events    []Event         `json:"events"`
	EventsErr string          `json:"events_error,omitempty"`
	DataDir   string          `json:"installation_data_dir,omitempty"`
}

func (h *Host) Workspace() Workspace {
	h.mu.Lock()
	defer h.mu.Unlock()
	w := Workspace{Entities: []EntitySummary{}, Problems: []Problem{}, Events: []Event{}}
	if h.svc == nil {
		return w
	}
	s := h.svc
	b := s.binding
	w.Started, w.Binding, w.StartedAt, w.RequestID, w.Launcher, w.ConsentID = true, &b, &s.startedAt, s.requestID, s.launcher, s.consentID
	w.DataDir = s.dir
	for id, e := range s.entities {
		sum := EntitySummary{Entity: e, TypeLabel: entityTypes[e.EntityType], Counts: map[string]int{}}
		for _, st := range thoughtStates {
			sum.Counts[st.Key] = 0
		}
		for _, t := range s.thoughts[id] {
			sum.Counts[t.State]++
			sum.Total++
		}
		w.Entities = append(w.Entities, sum)
	}
	sort.Slice(w.Entities, func(i, j int) bool {
		if !w.Entities[i].RegisteredAt.Equal(w.Entities[j].RegisteredAt) {
			return w.Entities[i].RegisteredAt.Before(w.Entities[j].RegisteredAt)
		}
		return w.Entities[i].EntityID < w.Entities[j].EntityID
	})
	w.Problems = append(w.Problems, s.problems...)
	w.Events = s.events.Recent(30)
	w.EventsErr = s.events.Err()
	return w
}

func (h *Host) Counters() Counters {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.counters
}

// ---------- small helpers ----------

func cleanText(s string, max int, what string) (string, error) {
	s = strings.TrimSpace(s)
	switch {
	case s == "":
		return "", fmt.Errorf("%w: %s is empty", ErrInvalid, what)
	case !utf8.ValidString(s):
		return "", fmt.Errorf("%w: %s is not valid text", ErrInvalid, what)
	case utf8.RuneCountInString(s) > max:
		return "", fmt.Errorf("%w: %s is longer than %d characters", ErrInvalid, what, max)
	}
	for _, r := range s {
		if unicode.IsControl(r) && r != '\n' {
			return "", fmt.Errorf("%w: %s contains control characters", ErrInvalid, what)
		}
	}
	return s, nil
}

func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// writeJSONAtomic: temp file, flush, rename, then read back and compare.
// Only a record that is really on disk counts as saved.
func writeJSONAtomic(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	f, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	ok := false
	defer func() {
		if !ok {
			os.Remove(tmp)
		}
	}()
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	ok = true
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, b) {
		return fmt.Errorf("could not verify the save after writing (%v)", err)
	}
	return nil
}

func readJSONStrict(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("not a valid record: %v", err)
	}
	return nil
}
