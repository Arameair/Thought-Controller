package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed web/index.html
var indexHTML []byte

type server struct {
	mind    *Mind
	run     runner
	mu      sync.Mutex
	worker  Worker
	workers map[string]Worker
	started time.Time
	port    string
	dataDir string
	branch  string
	commit  string
}

func (s *server) currentWorker() Worker {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.worker
}

// busy: a beat, a baseline call or a run is in progress.
func (s *server) busy() bool { return s.mind.Busy() || s.run.snapshot().Running }

func (s *server) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.local(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexHTML) // the page itself holds no data
	}))
	mux.HandleFunc("GET /api/heartbeat", s.local(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"ok": true, "service": "thought-controller", "uptime_seconds": int(time.Since(s.started).Seconds())})
	}))
	mux.HandleFunc("GET /api/state", s.read(s.handleState))
	mux.HandleFunc("GET /api/worker", s.read(func(w http.ResponseWriter, r *http.Request) {
		wk := s.currentWorker()
		ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
		defer cancel()
		rd := wk.Ready(ctx) // never sends a prompt
		writeJSON(w, 200, map[string]any{"worker": wk.Name(), "ready": rd.OK, "detail": rd.Detail, "source": rd.Source,
			"simulated": rd.Simulated, "live": rd.Live})
	}))
	mux.HandleFunc("GET /api/trace", s.read(s.handleTrace))
	mux.HandleFunc("GET /api/trace.jsonl", s.read(s.handleTraceJSONL))
	mux.HandleFunc("GET /api/diagnostic", s.read(s.handleDiagnostic))

	mux.HandleFunc("POST /api/event", s.write(func(w http.ResponseWriter, r *http.Request) {
		// Allowed during a beat on purpose: it makes the in-flight reply stale.
		var in struct{ Text, Source string }
		if !decode(w, r, &in) {
			return
		}
		e, err := s.mind.AddEvent(in.Text, firstSet(in.Source, "owner"))
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, e)
	}))
	mux.HandleFunc("POST /api/thought", s.write(func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Question string }
		if !decode(w, r, &in) {
			return
		}
		if s.busy() {
			writeJSON(w, 409, map[string]string{"error": ErrBusy.Error()})
			return
		}
		t, err := s.mind.AddThought(in.Question)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, t)
	}))
	mux.HandleFunc("POST /api/thought/state", s.write(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID    int    `json:"id"`
			State string `json:"state"`
			Note  string `json:"note"`
		}
		if !decode(w, r, &in) {
			return
		}
		if s.run.snapshot().Running {
			writeJSON(w, 409, map[string]string{"error": ErrBusy.Error()})
			return
		}
		if err := s.mind.SetState(in.ID, in.State, in.Note); err != nil {
			writeJSON(w, statusFor(err), map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]string{"status": "ok"})
	}))
	mux.HandleFunc("POST /api/thought/baseline", s.write(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID int `json:"id"`
		}
		if !decode(w, r, &in) {
			return
		}
		if s.run.snapshot().Running {
			writeJSON(w, 409, map[string]string{"error": ErrBusy.Error()})
			return
		}
		rec, err := s.mind.Baseline(r.Context(), s.currentWorker(), in.ID)
		if errors.Is(err, ErrBusy) || (err != nil && rec.Kind == "") {
			writeJSON(w, statusFor(err), map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, rec)
	}))
	mux.HandleFunc("POST /api/step", s.write(func(w http.ResponseWriter, r *http.Request) {
		if s.run.snapshot().Running {
			writeJSON(w, 409, map[string]string{"error": "a run is in progress"})
			return
		}
		rec, err := s.mind.Step(r.Context(), s.currentWorker())
		switch {
		case errors.Is(err, ErrBusy):
			writeJSON(w, 409, map[string]string{"error": err.Error()})
		case errors.Is(err, ErrNothing):
			writeJSON(w, 200, map[string]any{"nothing": true, "detail": err.Error()})
		default:
			writeJSON(w, 200, rec) // includes categorized worker errors
		}
	}))
	mux.HandleFunc("POST /api/run", s.write(func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Beats int }
		if !decode(w, r, &in) {
			return
		}
		if s.mind.Busy() {
			writeJSON(w, 409, map[string]string{"error": ErrBusy.Error()})
			return
		}
		if err := s.run.start(s.mind, s.currentWorker(), in.Beats); err != nil {
			writeJSON(w, 409, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 202, s.run.snapshot())
	}))
	mux.HandleFunc("POST /api/stop", s.write(func(w http.ResponseWriter, r *http.Request) {
		s.run.stop()
		s.mind.CancelBeat() // also stops a single Think once; a late reply is discarded
		writeJSON(w, 200, map[string]string{"status": "stopping; anything still in flight will be discarded"})
	}))
	mux.HandleFunc("POST /api/worker", s.write(func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Worker string }
		if !decode(w, r, &in) {
			return
		}
		wk, ok := s.workers[in.Worker]
		if !ok {
			writeJSON(w, 400, map[string]string{"error": "unknown worker"})
			return
		}
		if s.busy() {
			writeJSON(w, 409, map[string]string{"error": ErrBusy.Error()})
			return
		}
		s.mu.Lock()
		s.worker = wk
		s.mu.Unlock()
		s.mind.mu.Lock()
		s.mind.record(&BeatRecord{Beat: s.mind.Beat, Kind: "owner", Why: "owner switched worker to " + wk.Name()})
		s.mind.mu.Unlock()
		writeJSON(w, 200, map[string]string{"worker": wk.Name()})
	}))
	mux.HandleFunc("POST /api/settings", s.write(func(w http.ResponseWriter, r *http.Request) {
		var in Settings
		if !decode(w, r, &in) {
			return
		}
		if s.run.snapshot().Running {
			writeJSON(w, 409, map[string]string{"error": ErrBusy.Error()})
			return
		}
		if err := s.mind.UpdateSettings(in); err != nil {
			writeJSON(w, statusFor(err), map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, in)
	}))
	mux.HandleFunc("POST /api/reset", s.write(func(w http.ResponseWriter, r *http.Request) {
		if s.run.snapshot().Running {
			writeJSON(w, 409, map[string]string{"error": "stop the run first"})
			return
		}
		if err := s.mind.Reset(); err != nil {
			writeJSON(w, statusFor(err), map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]string{"status": "reset"})
	}))
	return mux
}

func statusFor(err error) int {
	switch {
	case errors.Is(err, ErrBusy):
		return 409
	case errors.Is(err, ErrNotReady):
		return 503
	}
	return 400
}

func (s *server) handleState(w http.ResponseWriter, r *http.Request) {
	m := s.mind
	m.mu.Lock()
	thoughts := make([]Thought, 0, len(m.Thoughts))
	for _, t := range m.Thoughts {
		thoughts = append(thoughts, *t)
	}
	out := map[string]any{
		"beat": m.Beat, "rev": m.Rev, "last_seen_rev": m.LastSeenRev, "busy": m.busy,
		"events": lastN(m.Events, 30), "thoughts": thoughts, "says": lastN(m.Says, 20),
		"settings": m.Settings, "metrics": m.metrics(), "save_error": m.saveErr, "trace_text": m.traceText,
	}
	m.mu.Unlock()
	wk := s.currentWorker()
	out["worker"], out["run"] = wk.Name(), s.run.snapshot()
	out["data_dir"], out["branch"], out["commit"] = s.dataDir, s.branch, s.commit
	writeJSON(w, 200, out)
}

func (s *server) handleTrace(w http.ResponseWriter, r *http.Request) {
	n, _ := strconv.Atoi(r.URL.Query().Get("n"))
	if n <= 0 || n > 500 {
		n = 60
	}
	s.mind.mu.Lock()
	tr := lastN(s.mind.trace, n)
	s.mind.mu.Unlock()
	rev := make([]BeatRecord, len(tr))
	for i := range tr {
		rev[len(tr)-1-i] = tr[i]
	}
	writeJSON(w, 200, map[string]any{"trace": rev})
}

func (s *server) handleTraceJSONL(w http.ResponseWriter, r *http.Request) {
	s.mind.mu.Lock()
	tr := append([]BeatRecord(nil), s.mind.trace...)
	s.mind.mu.Unlock()
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Content-Disposition", "attachment; filename=thought-trace.jsonl")
	enc := json.NewEncoder(w)
	for _, t := range tr {
		enc.Encode(t)
	}
}

// handleDiagnostic: a plain-text summary to paste into an issue. Event
// texts, prompts and replies are left out unless ?text=1.
func (s *server) handleDiagnostic(w http.ResponseWriter, r *http.Request) {
	withText := r.URL.Query().Get("text") == "1"
	m := s.mind
	wk := s.currentWorker()
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	rd := wk.Ready(ctx)
	cancel()
	m.mu.Lock()
	defer m.mu.Unlock()
	var b strings.Builder
	mt := m.metrics()
	fmt.Fprintf(&b, "## Thought Controller diagnostic\n\n")
	fmt.Fprintf(&b, "- Build: branch `%s`, commit `%s`\n- Data folder: `%s`\n", s.branch, s.commit, s.dataDir)
	fmt.Fprintf(&b, "- Worker: %s — %s (ready=%v, simulated=%v)\n", wk.Name(), rd.Source, rd.OK, rd.Simulated)
	fmt.Fprintf(&b, "- Beat %d, revision %d (seen %d)\n", m.Beat, m.Rev, m.LastSeenRev)
	if m.saveErr != "" {
		fmt.Fprintf(&b, "- ⚠ Last save error: %s\n", m.saveErr)
	}
	fmt.Fprintf(&b, "- Calls %d (simulated %d, baseline %d): applied %d, nothing %d, all-rejected %d, unreadable %d, stale %d, cancelled %d, not ready %d, errors %v\n",
		mt.Calls, mt.Simulated, mt.Baselines, mt.Outcomes["applied"], mt.Outcomes["nothing"], mt.Outcomes["all rejected"],
		mt.Outcomes["unreadable"], mt.Outcomes["stale"], mt.Outcomes["cancelled"], mt.Outcomes["not ready"], mt.Errors)
	fmt.Fprintf(&b, "- Call time: median %s, 95th %s\n", fmtMS(mt.MedianMS), fmtMS(mt.P95MS))
	fmt.Fprintf(&b, "- Thoughts: %v · rejections: %v · would-say: %d\n", mt.Thoughts, mt.RejectReasons, mt.Says)
	st, _ := json.Marshal(m.Settings)
	if !withText {
		var x Settings
		json.Unmarshal(st, &x)
		if x.Identity != "" {
			x.Identity = fmt.Sprintf("[%d characters]", len(x.Identity))
		}
		st, _ = json.Marshal(x)
	}
	fmt.Fprintf(&b, "- Settings: `%s`\n\n### Last beats\n\n| Beat | Kind | Source | Ready | Sent | Replied | Commands | Accepted | Outcome | ms |\n|---|---|---|---|---|---|---|---|---|---|\n", st)
	for _, rec := range lastN(m.trace, 60) {
		if rec.Stages == nil {
			continue
		}
		g := rec.Stages
		fmt.Fprintf(&b, "| %d | %s%s | %s | %v | %s | %v | %d | %d | %s%s | %d |\n", rec.Beat, rec.Kind, focusStr(rec.Focus), rec.Source,
			g.Ready, g.Sent, g.Replied, g.Commands, g.Accepted, rec.Outcome, catStr(rec.ErrorCat), rec.TookMS)
	}
	fmt.Fprintf(&b, "\n### Thoughts\n\n")
	for _, t := range m.Thoughts {
		q := fmt.Sprintf("[%d characters]", len([]rune(t.Question)))
		if withText {
			q = oneLine(t.Question)
		}
		fmt.Fprintf(&b, "- t-%d %s (parent %d, depth %d, worked %d, woken %d, notes %d): %s\n", t.ID, t.State, t.Parent, t.Depth, t.TotalWork, t.Activations, len(t.Notes), q)
	}
	if !withText {
		b.WriteString("\n_Event texts, questions, prompts and replies left out. Add ?text=1 to include them._\n")
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Write([]byte(b.String()))
}

func focusStr(f int) string {
	if f == 0 {
		return ""
	}
	return fmt.Sprintf(" t-%d", f)
}
func catStr(c string) string {
	if c == "" {
		return ""
	}
	return " (" + c + ")"
}
func fmtMS(v *int64) string {
	if v == nil {
		return "—"
	}
	return fmt.Sprintf("%d ms", *v)
}

func lastN[T any](v []T, n int) []T {
	if len(v) > n {
		v = v[len(v)-n:]
	}
	return append([]T{}, v...)
}

// ---------- access ----------

// local: only this machine's own address (stops DNS-rebinding pages).
func (s *server) local(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		if host != "127.0.0.1" && host != "localhost" && host != "::1" {
			writeJSON(w, 403, map[string]string{"error": "host not allowed"})
			return
		}
		h(w, r)
	}
}

// read: local + the token, if one is set. State and trace hold event text,
// prompts and replies, so reads are protected like writes.
func (s *server) read(h http.HandlerFunc) http.HandlerFunc {
	return s.local(func(w http.ResponseWriter, r *http.Request) {
		if tok := os.Getenv("THOUGHT_CONTROLLER_TOKEN"); tok != "" && r.Header.Get("X-Thought-Token") != tok {
			writeJSON(w, 401, map[string]string{"error": "missing or wrong X-Thought-Token"})
			return
		}
		h(w, r)
	})
}

// write: read + same-origin + JSON body (blocks cross-site form posts).
func (s *server) write(h http.HandlerFunc) http.HandlerFunc {
	return s.read(func(w http.ResponseWriter, r *http.Request) {
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
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(v); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad JSON: " + err.Error()})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}

func firstSet(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
