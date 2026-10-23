package main

// The controller: events, thought records, lifecycle and budgets. It does
// no reasoning of its own. Each beat makes at most one worker call:
//
//   - new events since the last heartbeat -> HEARTBEAT: the worker reads the
//     events and the current thoughts and proposes changes;
//   - otherwise, an active thought -> WORK: the least recently worked active
//     thought is moved one step forward;
//   - otherwise -> nothing to do, and no call is made.
//
// Every proposal is validated before it changes anything. If an event
// arrives while a call is in flight, the reply is marked stale and nothing
// from it is applied.
import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	stActive   = "active"
	stDormant  = "dormant"
	stResolved = "resolved"
	stExpired  = "expired"
)

type Event struct {
	ID     int       `json:"id"`
	At     time.Time `json:"at"`
	Text   string    `json:"text"`
	Source string    `json:"source"`
	Rev    int       `json:"rev"`
}

type Note struct {
	Beat   int       `json:"beat"`
	At     time.Time `json:"at"`
	Text   string    `json:"text"`
	Source string    `json:"source"` // worker | owner | controller
}

type Thought struct {
	ID             int       `json:"id"`
	Parent         int       `json:"parent,omitempty"`
	Depth          int       `json:"depth"`
	Question       string    `json:"question"`
	Origin         string    `json:"origin"` // what started it
	State          string    `json:"state"`
	Notes          []Note    `json:"notes"`
	Conclusion     string    `json:"conclusion,omitempty"`
	CreatedBeat    int       `json:"created_beat"`
	LastBeat       int       `json:"last_beat"` // last beat anything happened to it
	LastWorkedBeat int       `json:"last_worked_beat"`
	WorkBeats      int       `json:"work_beats"` // since last woken (the work budget)
	TotalWork      int       `json:"total_work"`
	Activations    int       `json:"activations"` // times woken from dormant
	Rev            int       `json:"rev"`         // context revision when it last changed
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Say struct {
	Beat    int       `json:"beat"`
	At      time.Time `json:"at"`
	Text    string    `json:"text"`
	Thought int       `json:"thought,omitempty"`
}

// Settings are the experiment's variables. They are recorded with every beat
// so runs with different settings can be compared.
type Settings struct {
	MaxActive         int    `json:"max_active"`
	MaxNewPerBeat     int    `json:"max_new_per_beat"`
	MaxDepth          int    `json:"max_depth"`
	MaxWorkPerThought int    `json:"max_work_per_thought"`
	DormantAfter      int    `json:"dormant_after"` // beats with nothing happening
	ExpireAfter       int    `json:"expire_after"`
	MaxRunBeats       int    `json:"max_run_beats"`
	GapMS             int    `json:"gap_ms"`
	FreshChat         bool   `json:"fresh_chat"`
	Identity          string `json:"identity"`
}

func defaultSettings() Settings {
	return Settings{MaxActive: 5, MaxNewPerBeat: 2, MaxDepth: 2, MaxWorkPerThought: 6, DormantAfter: 8, ExpireAfter: 30,
		MaxRunBeats: 20, GapMS: 800, FreshChat: true}
}

// BeatRecord is one line of the trace.
type BeatRecord struct {
	Beat        int       `json:"beat"`
	At          time.Time `json:"at"`
	Kind        string    `json:"kind"` // heartbeat | work | owner | lifecycle
	Why         string    `json:"why"`
	Focus       int       `json:"focus,omitempty"`
	Worker      string    `json:"worker,omitempty"`
	Simulated   bool      `json:"simulated,omitempty"`
	RevBefore   int       `json:"rev_before"`
	RevAfter    int       `json:"rev_after"`
	Stale       bool      `json:"stale,omitempty"`
	RequestID   string    `json:"request_id,omitempty"`
	TookMS      int64     `json:"took_ms,omitempty"`
	FirstMS     int64     `json:"first_ms,omitempty"`
	PromptChars int       `json:"prompt_chars,omitempty"`
	Prompt      string    `json:"prompt,omitempty"`
	Reply       string    `json:"reply,omitempty"`
	Parsed      *Parsed   `json:"parsed,omitempty"`
	Malformed   bool      `json:"malformed,omitempty"`
	Applied     []string  `json:"applied,omitempty"`
	Rejected    []string  `json:"rejected,omitempty"`
	Changes     []string  `json:"changes,omitempty"` // lifecycle changes made by the controller
	ErrorCat    string    `json:"error_cat,omitempty"`
	Error       string    `json:"error,omitempty"`
	Settings    *Settings `json:"settings,omitempty"`
}

var (
	ErrBusy    = errors.New("a beat is already running")
	ErrNothing = errors.New("nothing to think about: no new events and no active thoughts")
)

type Mind struct {
	mu          sync.Mutex
	Events      []Event    `json:"events"`
	Thoughts    []*Thought `json:"thoughts"`
	Says        []Say      `json:"says"`
	Beat        int        `json:"beat"`
	Rev         int        `json:"rev"`
	LastSeenRev int        `json:"last_seen_rev"`
	NextThought int        `json:"next_thought"`
	NextEvent   int        `json:"next_event"`
	Settings    Settings   `json:"settings"`
	trace       []BeatRecord
	busy        bool
	store       *Store
	now         func() time.Time
}

func newMind(st *Store) *Mind {
	m := &Mind{Settings: defaultSettings(), NextThought: 1, NextEvent: 1, store: st, now: time.Now}
	if st != nil {
		st.load(m)
	}
	return m
}

func (m *Mind) byID(id int) *Thought {
	for _, t := range m.Thoughts {
		if t.ID == id {
			return t
		}
	}
	return nil
}

func (m *Mind) count(state string) int {
	n := 0
	for _, t := range m.Thoughts {
		if t.State == state {
			n++
		}
	}
	return n
}

// ---------- owner actions ----------

func (m *Mind) AddEvent(text, source string) (Event, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Event{}, errors.New("event text is empty")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Rev++
	e := Event{ID: m.NextEvent, At: m.now().UTC(), Text: clip(text, 1000), Source: source, Rev: m.Rev}
	m.NextEvent++
	m.Events = append(m.Events, e)
	if len(m.Events) > 500 {
		m.Events = m.Events[len(m.Events)-500:]
	}
	m.record(BeatRecord{Beat: m.Beat, Kind: "owner", Why: fmt.Sprintf("event e-%d added (revision %d): %s", e.ID, e.Rev, clip(text, 120)),
		RevBefore: m.Rev - 1, RevAfter: m.Rev})
	return e, nil
}

func (m *Mind) AddThought(question string) (*Thought, error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return nil, errors.New("question is empty")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	t, why := m.newThought(question, 0, "owner")
	if t == nil {
		return nil, errors.New(why)
	}
	m.record(BeatRecord{Beat: m.Beat, Kind: "owner", Why: fmt.Sprintf("owner started t-%d: %s", t.ID, clip(question, 120)), RevBefore: m.Rev, RevAfter: m.Rev})
	return t, nil
}

// SetState lets the owner sleep, wake or resolve a thought by hand.
func (m *Mind) SetState(id int, state, note string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.byID(id)
	if t == nil {
		return fmt.Errorf("no thought t-%d", id)
	}
	switch state {
	case stActive:
		if t.State == stActive {
			return nil
		}
		if m.count(stActive) >= m.Settings.MaxActive {
			return fmt.Errorf("already %d active thoughts (limit)", m.Settings.MaxActive)
		}
		t.Activations++
		t.WorkBeats = 0
	case stDormant, stResolved:
	default:
		return fmt.Errorf("can't set state %q", state)
	}
	prev := t.State
	t.State = state
	if state == stResolved && note != "" {
		t.Conclusion = note
	}
	m.touch(t)
	m.record(BeatRecord{Beat: m.Beat, Kind: "owner", Why: fmt.Sprintf("owner moved t-%d %s → %s", id, prev, state), RevBefore: m.Rev, RevAfter: m.Rev})
	return nil
}

func (m *Mind) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Events, m.Thoughts, m.Says, m.trace = nil, nil, nil, nil
	m.Beat, m.Rev, m.LastSeenRev, m.NextThought, m.NextEvent = 0, 0, 0, 1, 1
	if m.store != nil {
		m.store.reset()
	}
	m.record(BeatRecord{Kind: "owner", Why: "owner reset the mind (settings kept)"})
}

func (m *Mind) UpdateSettings(s Settings) error {
	if s.MaxActive < 1 || s.MaxActive > 20 || s.MaxNewPerBeat < 0 || s.MaxNewPerBeat > 5 || s.MaxDepth < 0 || s.MaxDepth > 6 ||
		s.MaxWorkPerThought < 1 || s.MaxWorkPerThought > 50 || s.DormantAfter < 1 || s.ExpireAfter <= s.DormantAfter ||
		s.MaxRunBeats < 1 || s.MaxRunBeats > 100 || s.GapMS < 0 || s.GapMS > 60000 || len(s.Identity) > 500 {
		return errors.New("setting out of range")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Settings = s
	m.record(BeatRecord{Beat: m.Beat, Kind: "owner", Why: "owner changed settings", Settings: &s})
	return nil
}

// ---------- beats ----------

type plan struct {
	kind, why, prompt string
	focus             int
	beat, rev         int
}

func (m *Mind) planBeat() (plan, error) {
	if m.busy {
		return plan{}, ErrBusy
	}
	var unseen []Event
	for _, e := range m.Events {
		if e.Rev > m.LastSeenRev {
			unseen = append(unseen, e)
		}
	}
	live := m.liveThoughts(8)
	p := plan{rev: m.Rev}
	switch {
	case len(unseen) > 0:
		if len(unseen) > 6 {
			unseen = unseen[len(unseen)-6:]
		}
		p.kind = "heartbeat"
		p.why = fmt.Sprintf("%d new event(s) since the last heartbeat", len(unseen))
		p.prompt = heartbeatPrompt(unseen, live, m.Settings.Identity)
	default:
		var pick *Thought
		for _, t := range m.Thoughts {
			if t.State == stActive && (pick == nil || t.LastWorkedBeat < pick.LastWorkedBeat ||
				(t.LastWorkedBeat == pick.LastWorkedBeat && t.ID < pick.ID)) {
				pick = t
			}
		}
		if pick == nil {
			return plan{}, ErrNothing
		}
		var others []*Thought
		for _, t := range live {
			if t.ID != pick.ID {
				others = append(others, t)
			}
		}
		p.kind, p.focus = "work", pick.ID
		p.why = fmt.Sprintf("working t-%d (active thought waiting longest)", pick.ID)
		p.prompt = workPrompt(pick, m.recentEvents(4), others, m.Settings.Identity)
	}
	m.Beat++
	p.beat = m.Beat
	m.busy = true
	return p, nil
}

// Step runs one beat: at most one worker call.
func (m *Mind) Step(ctx context.Context, w Worker) (BeatRecord, error) {
	m.mu.Lock()
	p, err := m.planBeat()
	settings := m.Settings
	m.mu.Unlock()
	if err != nil {
		return BeatRecord{}, err
	}
	rec := BeatRecord{Beat: p.beat, Kind: p.kind, Why: p.why, Focus: p.focus, Worker: w.Name(), Simulated: w.Simulated(),
		RevBefore: p.rev, Prompt: p.prompt, PromptChars: len([]rune(p.prompt)), Settings: &settings}

	cctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	reply, err := w.Ask(cctx, p.prompt, settings.FreshChat)
	cancel()

	m.mu.Lock()
	defer m.mu.Unlock()
	m.busy = false
	rec.At = m.now().UTC()
	rec.RevAfter = m.Rev
	rec.RequestID, rec.TookMS, rec.FirstMS, rec.Reply = reply.RequestID, reply.Took.Milliseconds(), reply.FirstText.Milliseconds(), reply.Text
	if err != nil {
		rec.ErrorCat, rec.Error = errCategory(err), err.Error()
		if p.kind == "work" {
			// don't keep picking the same thought if the worker keeps failing on it
			if t := m.byID(p.focus); t != nil {
				t.LastWorkedBeat = p.beat
			}
		}
		m.record(rec)
		return rec, err
	}
	parsed := parseReply(reply.Text, p.focus)
	rec.Parsed = &parsed
	rec.Malformed = parsed.Malformed()
	rec.Stale = m.Rev != p.rev
	if rec.Stale {
		rec.Rejected = append(rec.Rejected, fmt.Sprintf("whole reply: stale — context changed from revision %d to %d while thinking; nothing applied", p.rev, m.Rev))
	} else {
		if p.kind == "heartbeat" {
			m.LastSeenRev = p.rev
		}
		m.apply(&rec, parsed, p)
	}
	if p.kind == "work" {
		if t := m.byID(p.focus); t != nil {
			t.LastWorkedBeat = p.beat
			if !rec.Stale {
				t.WorkBeats++
				t.TotalWork++
				t.LastBeat = p.beat
				if t.State == stActive && t.WorkBeats >= settings.MaxWorkPerThought {
					t.State = stDormant
					rec.Changes = append(rec.Changes, fmt.Sprintf("t-%d → dormant: reached %d work beats without resolving", t.ID, t.WorkBeats))
				}
			}
		}
	}
	rec.Changes = append(rec.Changes, m.age(p.beat)...)
	m.record(rec)
	return rec, nil
}

func (m *Mind) apply(rec *BeatRecord, parsed Parsed, p plan) {
	created := 0
	for _, pr := range parsed.Proposals {
		ok, why := m.applyOne(pr, p, &created)
		if ok {
			rec.Applied = append(rec.Applied, why)
		} else {
			rec.Rejected = append(rec.Rejected, fmt.Sprintf("%q: %s", clip(pr.Line, 90), why))
		}
	}
}

func (m *Mind) applyOne(pr Proposal, p plan, created *int) (bool, string) {
	s := m.Settings
	switch pr.Kind {
	case "new":
		if *created >= s.MaxNewPerBeat {
			return false, fmt.Sprintf("limit of %d new thoughts per beat", s.MaxNewPerBeat)
		}
		parent := 0
		if p.kind == "work" {
			parent = p.focus
		}
		t, why := m.newThought(pr.Text, parent, fmt.Sprintf("beat %d (%s)", p.beat, p.kind))
		if t == nil {
			return false, why
		}
		*created++
		return true, fmt.Sprintf("created t-%d: %s", t.ID, clip(t.Question, 100))
	case "say":
		m.Says = append(m.Says, Say{Beat: p.beat, At: m.now().UTC(), Text: pr.Text, Thought: p.focus})
		if len(m.Says) > 100 {
			m.Says = m.Says[len(m.Says)-100:]
		}
		return true, "would say (not sent): " + clip(pr.Text, 100)
	}
	t := m.byID(pr.Target)
	if t == nil {
		return false, fmt.Sprintf("no thought t-%d", pr.Target)
	}
	switch pr.Kind {
	case "wake":
		if t.State != stDormant {
			return false, fmt.Sprintf("t-%d is %s, not dormant", t.ID, t.State)
		}
		if m.count(stActive) >= s.MaxActive {
			return false, fmt.Sprintf("already %d active thoughts (limit)", s.MaxActive)
		}
		t.State, t.WorkBeats = stActive, 0
		t.Activations++
		if pr.Text != "" {
			m.addNote(t, p.beat, "woke: "+pr.Text, "worker")
		}
		m.touchAt(t, p.beat)
		return true, fmt.Sprintf("woke t-%d", t.ID)
	case "note":
		if t.State != stActive {
			return false, fmt.Sprintf("t-%d is %s; only active thoughts take notes", t.ID, t.State)
		}
		m.addNote(t, p.beat, pr.Text, "worker")
		m.touchAt(t, p.beat)
		return true, fmt.Sprintf("noted on t-%d: %s", t.ID, clip(pr.Text, 100))
	case "resolve":
		if t.State != stActive && t.State != stDormant {
			return false, fmt.Sprintf("t-%d is already %s", t.ID, t.State)
		}
		t.State, t.Conclusion = stResolved, pr.Text
		m.touchAt(t, p.beat)
		return true, fmt.Sprintf("resolved t-%d: %s", t.ID, clip(pr.Text, 100))
	case "sleep":
		if t.State != stActive {
			return false, fmt.Sprintf("t-%d is %s, not active", t.ID, t.State)
		}
		t.State = stDormant
		m.touchAt(t, p.beat)
		return true, fmt.Sprintf("t-%d went dormant", t.ID)
	}
	return false, "unknown proposal"
}

// newThought creates an active thought if the budgets and duplicate check allow.
func (m *Mind) newThought(q string, parent int, origin string) (*Thought, string) {
	s := m.Settings
	q = clip(strings.TrimSpace(q), 280)
	depth := 0
	if parent != 0 {
		if pt := m.byID(parent); pt != nil {
			depth = pt.Depth + 1
		}
	}
	if depth > s.MaxDepth {
		return nil, fmt.Sprintf("too deep: follow-up chains are limited to %d levels", s.MaxDepth)
	}
	if m.count(stActive) >= s.MaxActive {
		return nil, fmt.Sprintf("already %d active thoughts (limit)", s.MaxActive)
	}
	for _, t := range m.Thoughts {
		if t.State != stExpired && similar(t.Question, q) {
			return nil, fmt.Sprintf("duplicate of t-%d", t.ID)
		}
	}
	now := m.now().UTC()
	t := &Thought{ID: m.NextThought, Parent: parent, Depth: depth, Question: q, Origin: origin, State: stActive,
		Notes: []Note{}, CreatedBeat: m.Beat, LastBeat: m.Beat, Rev: m.Rev, CreatedAt: now, UpdatedAt: now}
	m.NextThought++
	m.Thoughts = append(m.Thoughts, t)
	return t, ""
}

func (m *Mind) addNote(t *Thought, beat int, text, src string) {
	t.Notes = append(t.Notes, Note{Beat: beat, At: m.now().UTC(), Text: clip(text, 400), Source: src})
	if len(t.Notes) > 30 {
		t.Notes = t.Notes[len(t.Notes)-30:]
	}
}

func (m *Mind) touch(t *Thought) { m.touchAt(t, m.Beat) }
func (m *Mind) touchAt(t *Thought, beat int) {
	t.LastBeat, t.Rev, t.UpdatedAt = beat, m.Rev, m.now().UTC()
}

// age applies the idle rules after each beat.
func (m *Mind) age(beat int) []string {
	var out []string
	for _, t := range m.Thoughts {
		idle := beat - t.LastBeat
		switch {
		case t.State == stActive && idle >= m.Settings.DormantAfter:
			t.State = stDormant
			out = append(out, fmt.Sprintf("t-%d → dormant: nothing happened for %d beats", t.ID, idle))
		case t.State == stDormant && idle >= m.Settings.ExpireAfter:
			t.State = stExpired
			out = append(out, fmt.Sprintf("t-%d → expired: dormant and untouched for %d beats", t.ID, idle))
		}
	}
	return out
}

// liveThoughts: active then dormant, most recently changed first, at most n.
func (m *Mind) liveThoughts(n int) []*Thought {
	var out []*Thought
	for _, t := range m.Thoughts {
		if t.State == stActive || t.State == stDormant {
			out = append(out, t)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if (out[i].State == stActive) != (out[j].State == stActive) {
			return out[i].State == stActive
		}
		return out[i].LastBeat > out[j].LastBeat
	})
	if len(out) > n {
		out = out[:n]
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (m *Mind) recentEvents(n int) []Event {
	if len(m.Events) > n {
		return m.Events[len(m.Events)-n:]
	}
	return m.Events
}

func (m *Mind) record(r BeatRecord) {
	if r.At.IsZero() {
		r.At = m.now().UTC()
	}
	m.trace = append(m.trace, r)
	if len(m.trace) > 2000 {
		m.trace = m.trace[len(m.trace)-2000:]
	}
	if m.store != nil {
		m.store.append(r)
		m.store.save(m)
	}
}

// similar: two questions share most of their meaningful words.
func similar(a, b string) bool {
	wa, wb := words(a), words(b)
	if len(wa) == 0 || len(wb) == 0 {
		return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
	}
	set := map[string]bool{}
	for _, w := range wa {
		set[w] = true
	}
	inter, uni := 0, len(set)
	seen := map[string]bool{}
	for _, w := range wb {
		if seen[w] {
			continue
		}
		seen[w] = true
		if set[w] {
			inter++
		} else {
			uni++
		}
	}
	return float64(inter)/float64(uni) >= 0.7
}
