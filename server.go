package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

//go:embed web/index.html
var indexHTML []byte

type server struct{ host *Host }

func (s *server) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", local(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(indexHTML)
	}))
	mux.HandleFunc("GET /api/health", local(s.handleHealth))
	mux.HandleFunc("GET /api/workspace", local(s.handleWorkspace))
	mux.HandleFunc("GET /api/diagnostics", local(s.handleDiagnostics))

	// SIMULATED lifecycle + handshake used by Identity's simulated launcher.
	mux.HandleFunc("POST /api/sim/lifecycle/start", write(s.handleStart))
	mux.HandleFunc("POST /api/sim/lifecycle/stop", write(s.handleStop))
	mux.HandleFunc("POST /api/sim/register", write(s.handleRegister))

	// Thoughts: every route names its entity.
	mux.HandleFunc("GET /api/entities/{eid}/thoughts", local(func(w http.ResponseWriter, r *http.Request) {
		list, err := s.host.ListThoughts(r.PathValue("eid"))
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"entity_id": r.PathValue("eid"), "thoughts": list, "states": thoughtStates})
	}))
	mux.HandleFunc("GET /api/entities/{eid}/thoughts/{tid}", local(func(w http.ResponseWriter, r *http.Request) {
		t, err := s.host.GetThought(r.PathValue("eid"), r.PathValue("tid"))
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"thought": t})
	}))
	mux.HandleFunc("GET /api/entities/{eid}/thoughts/{tid}/diagnostics", local(s.handleThoughtDiagnostics))
	mux.HandleFunc("POST /api/entities/{eid}/thoughts", write(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Title     string `json:"title"`
			State     string `json:"state"`
			RequestID string `json:"request_id"`
		}
		if !decode(w, r, &in) {
			return
		}
		t, repeat, err := s.host.CreateThought(r.PathValue("eid"), in.Title, in.State, in.RequestID, "owner test button (SIMULATED — no model was involved)")
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 201, map[string]any{"thought": t, "repeat": repeat})
	}))
	mux.HandleFunc("POST /api/entities/{eid}/samples", write(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			RequestID string `json:"request_id"`
		}
		if !decode(w, r, &in) {
			return
		}
		eid := r.PathValue("eid")
		ws := s.host.Workspace()
		etype := ""
		for _, e := range ws.Entities {
			if e.EntityID == eid {
				etype = e.EntityType
			}
		}
		var made []Thought
		for i, st := range sampleSet(etype) {
			rid := ""
			if in.RequestID != "" {
				rid = fmt.Sprintf("%s-%d", in.RequestID, i)
			}
			t, _, err := s.host.CreateThought(eid, st[1], st[0], rid, "sample set button (SIMULATED sample — no model was involved)")
			if err != nil {
				writeErr(w, err)
				return
			}
			made = append(made, t)
		}
		writeJSON(w, 201, map[string]any{"thoughts": made})
	}))
	mux.HandleFunc("POST /api/entities/{eid}/thoughts/{tid}", write(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			State string `json:"state"`
			Note  string `json:"note"`
		}
		if !decode(w, r, &in) {
			return
		}
		t, err := s.host.UpdateThought(r.PathValue("eid"), r.PathValue("tid"), in.State, in.Note)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"thought": t})
	}))
	return mux
}

// Health is a read-only handshake. It never sends an AI prompt.
func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	h := s.host
	ws := h.Workspace()
	svc := map[string]any{"state": "not_started", "readiness": "NOT STARTED"}
	if ws.Started {
		svc = map[string]any{
			"state": "ready", "readiness": "READY (simulated)",
			"service_id": ws.Binding.ServiceID, "installation_id": ws.Binding.InstallationID,
			"admin_id": ws.Binding.AdminID, "started_at": ws.StartedAt, "start_request_id": ws.RequestID,
			"entities": len(ws.Entities),
		}
	}
	writeJSON(w, 200, map[string]any{
		"app":            "thought-controller",
		"protocol":       protocol,
		"simulated":      true,
		"authentication": "NONE — simulated handshake; any local program can call this API",
		"mode":           "SIMULATED HOST (no process launching, no AI worker, no GitHub)",
		"build": map[string]any{
			"reference_repo": refRepo, "branch": h.cfg.Branch, "branch_source": h.cfg.BranchFrom,
			"commit": h.cfg.Commit, "detached": h.cfg.Detached, "version": appVersion,
			"provenance": "TEST / LOCAL CHECKOUT — not a published approved release (none exists)",
		},
		"host":     map[string]any{"pid": os.Getpid(), "port": h.cfg.Port, "data_dir": h.cfg.DataDir, "data_source": h.cfg.DataSource, "started_at": h.started.UTC()},
		"service":  svc,
		"worker":   map[string]any{"state": "NOT READY", "reason": "no AI worker exists in this prototype"},
		"counters": h.Counters(),
		"features": []string{"sim-lifecycle-start", "sim-lifecycle-stop", "sim-register", "entity-scoped-thoughts"},
	})
}

func (s *server) handleWorkspace(w http.ResponseWriter, r *http.Request) {
	h := s.host
	writeJSON(w, 200, map[string]any{
		"workspace":         h.Workspace(),
		"states":            thoughtStates,
		"config":            h.cfg,
		"counters":          h.Counters(),
		"host_events":       h.events.Recent(20),
		"host_events_error": h.events.Err(),
		"protocol":          protocol,
		"pid":               os.Getpid(),
	})
}

func (s *server) handleStart(w http.ResponseWriter, r *http.Request) {
	var in StartRequest
	if !decode(w, r, &in) {
		return
	}
	res, err := s.host.Start(in)
	if err != nil {
		writeErr(w, err)
		return
	}
	code := 200
	if res.Result == "refused" {
		code = 409
	}
	writeJSON(w, code, res)
}

func (s *server) handleStop(w http.ResponseWriter, r *http.Request) {
	var in struct {
		InstallationID string `json:"installation_id"`
		Reason         string `json:"reason"`
	}
	if !decode(w, r, &in) {
		return
	}
	reason := firstSet(strings.TrimSpace(in.Reason), "stop requested")
	if len(reason) > 200 {
		reason = reason[:200]
	}
	var err error
	if in.InstallationID != "" {
		err = s.host.StopService(reason, in.InstallationID)
	} else {
		err = s.host.StopService(reason)
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"result": "stopped", "note": "SIMULATED stop: the host process keeps running; the controller service is no longer started"})
}

func (s *server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var in struct {
		InstallationID string      `json:"installation_id"`
		ServiceID      string      `json:"service_id"`
		Entities       []EntityRef `json:"entities"`
	}
	if !decode(w, r, &in) {
		return
	}
	res, err := s.host.Register(in.InstallationID, in.ServiceID, in.Entities)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"registrations": res, "simulated": true})
}

// handleDiagnostics: safe to paste. Thought titles, notes and names are left
// out; IDs, states, counts and provenance are kept. No credentials exist here.
func (s *server) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	h := s.host
	c := h.cfg
	ws := h.Workspace()
	cnt := h.Counters()
	var b strings.Builder
	fmt.Fprintf(&b, "## Thought Controller diagnostics (%s)\n\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "- Mode: **SIMULATED HOST** · protocol %s · authentication NONE · %s · Go %s · %s/%s\n", protocol, c.Version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&b, "- Reference: %s · branch %s (from %s)%s · commit %s\n", refRepo, c.Branch, c.BranchFrom, map[bool]string{true: " · detached checkout", false: ""}[c.Detached], c.Commit)
	fmt.Fprintf(&b, "- Provenance: TEST / LOCAL CHECKOUT (no published approved release exists)\n")
	fmt.Fprintf(&b, "- Host: pid %d · port %s · up %s · data %s (from %s)\n", os.Getpid(), c.Port, time.Since(h.started).Round(time.Second), c.DataDir, c.DataSource)
	fmt.Fprintf(&b, "- Lifecycle counters: start requests %d · services started %d · already running (reused) %d · duplicate requests %d · refused %d · stops %d\n",
		cnt.StartRequests, cnt.ServicesStarted, cnt.AlreadyRunning, cnt.DuplicateRequests, cnt.Refused, cnt.Stops)
	fmt.Fprintf(&b, "- AI worker: NOT READY (none exists) · model calls: %d · GitHub calls: 0 · processes launched: 0\n", cnt.ModelCalls)
	if !ws.Started {
		b.WriteString("- Controller service: NOT STARTED (waiting for Identity's simulated launcher)\n")
	} else {
		fmt.Fprintf(&b, "- Controller service: READY (simulated) since %s · start request %s · launcher %q · consent %s\n", ws.StartedAt.Format(time.RFC3339), ws.RequestID, ws.Launcher, firstSet(ws.ConsentID, "(none sent)"))
		fmt.Fprintf(&b, "  - installation_id %s\n  - controller service_id %s\n  - administrator_id %s\n  - bound at %s\n  - installation data %s\n",
			ws.Binding.InstallationID, ws.Binding.ServiceID, ws.Binding.AdminID, ws.Binding.BoundAt.Format(time.RFC3339), ws.DataDir)
		fmt.Fprintf(&b, "- Managed entities: %d\n", len(ws.Entities))
		for _, e := range ws.Entities {
			fmt.Fprintf(&b, "  - %s · %s · thoughts %d (continuing %d, active %d, sleeping %d, resolved %d)\n", e.EntityID, e.TypeLabel, e.Total,
				e.Counts["continuing"], e.Counts["active"], e.Counts["sleeping"], e.Counts["resolved"])
		}
		fmt.Fprintf(&b, "- Problem files: %d\n", len(ws.Problems))
		for _, p := range ws.Problems {
			fmt.Fprintf(&b, "  - %s: %s\n", p.File, p.Reason)
		}
		if ws.EventsErr != "" {
			fmt.Fprintf(&b, "- Installation event log: %s\n", ws.EventsErr)
		}
	}
	if e := h.events.Err(); e != "" {
		fmt.Fprintf(&b, "- Host event log: %s\n", e)
	}
	b.WriteString("\n### Recent host events\n\n")
	for _, e := range h.events.Recent(15) {
		fmt.Fprintf(&b, "- %s %s: %s%s\n", e.At.Format("15:04:05"), e.Kind, e.Detail, idSuffix(e.ID))
	}
	if ws.Started {
		b.WriteString("\n### Recent installation events\n\n")
		for _, e := range ws.Events {
			if len(ws.Events) > 15 && e.At.Before(ws.Events[len(ws.Events)-15].At) {
				continue
			}
			fmt.Fprintf(&b, "- %s %s: %s%s\n", e.At.Format("15:04:05"), e.Kind, e.Detail, idSuffix(e.ID))
		}
	}
	b.WriteString("\n_Names, thought titles and notes are left out. No credentials exist in this app._\n")
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write([]byte(b.String()))
}

func (s *server) handleThoughtDiagnostics(w http.ResponseWriter, r *http.Request) {
	t, err := s.host.GetThought(r.PathValue("eid"), r.PathValue("tid"))
	if err != nil {
		writeErr(w, err)
		return
	}
	c := s.host.cfg
	var b strings.Builder
	fmt.Fprintf(&b, "## Thought diagnostics (%s)\n\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "- thought_id %s\n- entity_id %s\n- installation_id %s\n", t.ThoughtID, t.EntityID, t.InstallationID)
	fmt.Fprintf(&b, "- state %s · SIMULATED %v · origin: %s\n", t.State, t.Simulated, t.Origin)
	fmt.Fprintf(&b, "- created %s · updated %s · notes %d · history entries %d\n", t.CreatedAt.Format(time.RFC3339), t.UpdatedAt.Format(time.RFC3339), len(t.Notes), len(t.History))
	fmt.Fprintf(&b, "- worker/model calls for this thought: %d (no AI worker exists in this prototype)\n", t.WorkerCalls)
	fmt.Fprintf(&b, "- stored at installations/%s/entities/%s/thoughts/%s.json\n", t.InstallationID, t.EntityID, t.ThoughtID)
	fmt.Fprintf(&b, "- controller build: %s @ %s (%s) · protocol %s\n", c.Branch, c.Commit, c.Version, protocol)
	b.WriteString("\n### Lifecycle\n\n")
	for _, hi := range t.History {
		arrow := ""
		if hi.From != "" || hi.To != "" {
			arrow = fmt.Sprintf(" %s→%s", firstSet(hi.From, "∅"), hi.To)
		}
		fmt.Fprintf(&b, "- %s %s%s: %s\n", hi.At.Format(time.RFC3339), hi.Kind, arrow, hi.Detail)
	}
	b.WriteString("\n_Title and note text are left out._\n")
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write([]byte(b.String()))
}

func idSuffix(id string) string {
	if id == "" {
		return ""
	}
	return " " + id
}

// ---------- safety (same guards as Identity V1's server.go) ----------

// local: only requests addressed to this machine's loopback name (blocks
// DNS-rebinding pages). The server also only listens on 127.0.0.1.
func local(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		if host != "127.0.0.1" && host != "localhost" && host != "::1" && host != "[::1]" {
			writeJSON(w, 403, map[string]string{"error": "host not allowed"})
			return
		}
		h(w, r)
	}
}

// write: local + same-origin + JSON body (stops other web pages posting).
// Server-to-server calls from Identity send no Origin header.
func write(h http.HandlerFunc) http.HandlerFunc {
	return local(func(w http.ResponseWriter, r *http.Request) {
		if o := r.Header.Get("Origin"); o != "" && o != "http://"+r.Host {
			writeJSON(w, 403, map[string]string{"error": "origin not allowed"})
			return
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			writeJSON(w, 415, map[string]string{"error": "send Content-Type: application/json"})
			return
		}
		h(w, r)
	})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request: " + err.Error()})
		return false
	}
	return true
}

func writeErr(w http.ResponseWriter, err error) {
	code := 500
	switch {
	case errors.Is(err, ErrInvalid):
		code = 400
	case errors.Is(err, ErrNotFound):
		code = 404
	case errors.Is(err, ErrNotStarted), errors.Is(err, ErrConflict):
		code = 409
	}
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}
