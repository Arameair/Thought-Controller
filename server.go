package main

import (
	"context"
	_ "embed"
	"encoding/json"
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
}

func (s *server) currentWorker() Worker {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.worker
}

func (s *server) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexHTML)
	})
	mux.HandleFunc("GET /api/state", s.handleState)
	mux.HandleFunc("GET /api/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"ok": true, "service": "thought-controller", "uptime_seconds": int(time.Since(s.started).Seconds())})
	})
	mux.HandleFunc("GET /api/worker", func(w http.ResponseWriter, r *http.Request) {
		wk := s.currentWorker()
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		ok, msg := wk.Ready(ctx) // never sends a prompt
		writeJSON(w, 200, map[string]any{"worker": wk.Name(), "simulated": wk.Simulated(), "ready": ok, "detail": msg})
	})
	mux.HandleFunc("GET /api/trace", s.handleTrace)
	mux.HandleFunc("GET /api/trace.jsonl", s.handleTraceJSONL)

	mux.HandleFunc("POST /api/event", s.guard(func(w http.ResponseWriter, r *http.Request) {
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
	mux.HandleFunc("POST /api/thought", s.guard(func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Question string }
		if !decode(w, r, &in) {
			return
		}
		t, err := s.mind.AddThought(in.Question)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, t)
	}))
	mux.HandleFunc("POST /api/thought/state", s.guard(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID    int    `json:"id"`
			State string `json:"state"`
			Note  string `json:"note"`
		}
		if !decode(w, r, &in) {
			return
		}
		if err := s.mind.SetState(in.ID, in.State, in.Note); err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]string{"status": "ok"})
	}))
	mux.HandleFunc("POST /api/step", s.guard(func(w http.ResponseWriter, r *http.Request) {
		if s.run.snapshot().Running {
			writeJSON(w, 409, map[string]string{"error": "a run is in progress"})
			return
		}
		rec, err := s.mind.Step(r.Context(), s.currentWorker())
		switch {
		case err == ErrBusy:
			writeJSON(w, 409, map[string]string{"error": err.Error()})
		case err == ErrNothing:
			writeJSON(w, 200, map[string]any{"nothing": true, "detail": err.Error()})
		default:
			writeJSON(w, 200, rec) // includes worker errors, categorized
		}
	}))
	mux.HandleFunc("POST /api/run", s.guard(func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Beats int }
		if !decode(w, r, &in) {
			return
		}
		if err := s.run.start(s.mind, s.currentWorker(), in.Beats); err != nil {
			writeJSON(w, 409, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 202, s.run.snapshot())
	}))
	mux.HandleFunc("POST /api/stop", s.guard(func(w http.ResponseWriter, r *http.Request) {
		s.run.stop()
		writeJSON(w, 200, map[string]string{"status": "stopping after the current beat"})
	}))
	mux.HandleFunc("POST /api/worker", s.guard(func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Worker string }
		if !decode(w, r, &in) {
			return
		}
		wk, ok := s.workers[in.Worker]
		if !ok {
			writeJSON(w, 400, map[string]string{"error": "worker must be mock or chat-jimmy"})
			return
		}
		if s.run.snapshot().Running {
			writeJSON(w, 409, map[string]string{"error": "stop the run first"})
			return
		}
		s.mu.Lock()
		s.worker = wk
		s.mu.Unlock()
		s.mind.mu.Lock()
		s.mind.record(BeatRecord{Beat: s.mind.Beat, Kind: "owner", Why: "owner switched worker to " + wk.Name()})
		s.mind.mu.Unlock()
		writeJSON(w, 200, map[string]string{"worker": wk.Name()})
	}))
	mux.HandleFunc("POST /api/settings", s.guard(func(w http.ResponseWriter, r *http.Request) {
		var in Settings
		if !decode(w, r, &in) {
			return
		}
		if err := s.mind.UpdateSettings(in); err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, in)
	}))
	mux.HandleFunc("POST /api/reset", s.guard(func(w http.ResponseWriter, r *http.Request) {
		if s.run.snapshot().Running {
			writeJSON(w, 409, map[string]string{"error": "stop the run first"})
			return
		}
		s.mind.Reset()
		writeJSON(w, 200, map[string]string{"status": "reset"})
	}))
	return mux
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
		"events":   lastN(m.Events, 30),
		"thoughts": thoughts,
		"says":     lastN(m.Says, 20),
		"settings": m.Settings,
		"metrics":  m.metrics(),
	}
	m.mu.Unlock()
	wk := s.currentWorker()
	out["worker"], out["simulated"] = wk.Name(), wk.Simulated()
	out["run"] = s.run.snapshot()
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

func lastN[T any](v []T, n int) []T {
	if len(v) > n {
		v = v[len(v)-n:]
	}
	return append([]T{}, v...)
}

// guard: same machine only, JSON bodies only, optional token.
func (s *server) guard(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		if host != "127.0.0.1" && host != "localhost" && host != "::1" {
			writeJSON(w, 403, map[string]string{"error": "host not allowed"})
			return
		}
		if o := r.Header.Get("Origin"); o != "" && o != "http://"+r.Host {
			writeJSON(w, 403, map[string]string{"error": "origin not allowed"})
			return
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			writeJSON(w, 415, map[string]string{"error": "send Content-Type: application/json"})
			return
		}
		if tok := os.Getenv("THOUGHT_CONTROLLER_TOKEN"); tok != "" && r.Header.Get("X-Thought-Token") != tok {
			writeJSON(w, 401, map[string]string{"error": "missing or wrong X-Thought-Token"})
			return
		}
		h(w, r)
	}
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
