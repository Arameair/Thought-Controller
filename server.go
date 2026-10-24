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

const protocol = "tc-dashboard/0.2"

type server struct{ app *App }

var identityStatus = map[string]string{
	"state":  "NOT CONNECTED",
	"detail": "No authenticated Identity handshake exists yet. Identities and their Thought permissions shown here come from the Thought engine, which holds them itself until Identity is connected.",
}

func (s *server) routes() *http.ServeMux {
	a := s.app
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", local(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(indexHTML)
	}))

	// Thought Controller's own health. Read-only; never contacts anything else.
	mux.HandleFunc("GET /api/health", local(func(w http.ResponseWriter, r *http.Request) {
		st := a.src.State()
		writeJSON(w, 200, map[string]any{
			"app": "thought-controller", "protocol": protocol, "version": appVersion, "instance_id": a.inst.InstanceID,
			"build":    map[string]any{"reference_repo": refRepo, "branch": a.cfg.Branch, "branch_source": a.cfg.BranchFrom, "commit": a.cfg.Commit, "detached": a.cfg.Detached},
			"source":   map[string]any{"kind": "thought-engine", "address": st.Address, "state": st.State},
			"identity": identityStatus,
		})
	}))

	// Everything the page needs at the top level, read live from the engine.
	mux.HandleFunc("GET /api/overview", local(func(w http.ResponseWriter, r *http.Request) {
		raw, st := a.src.Status()
		var status any
		if raw != nil {
			status = raw
		}
		a.mu.Lock()
		probs := append([]string{}, a.problems...)
		a.mu.Unlock()
		writeJSON(w, 200, map[string]any{
			"tc": map[string]any{"instance": a.inst, "config": a.cfg, "pid": os.Getpid(), "started_at": a.started.UTC(), "protocol": protocol,
				"events": a.events.Recent(40), "events_error": a.events.Err(), "problems": probs},
			"source": st, "status": status, "identity": identityStatus, "server_time": time.Now().UTC(),
		})
	}))

	mux.HandleFunc("GET /api/entities/{eid}", local(func(w http.ResponseWriter, r *http.Request) {
		eid := r.PathValue("eid")
		if !uuidRe.MatchString(eid) {
			writeErr(w, fmt.Errorf("%w: identity ID is not valid", ErrInvalid))
			return
		}
		s.forward(w, "/api/entities/"+eid)
	}))
	mux.HandleFunc("GET /api/entities/{eid}/thoughts/{tid}", local(func(w http.ResponseWriter, r *http.Request) {
		eid, tid := r.PathValue("eid"), r.PathValue("tid")
		if !uuidRe.MatchString(eid) || !uuidRe.MatchString(tid) {
			writeErr(w, fmt.Errorf("%w: identity or Thought ID is not valid", ErrInvalid))
			return
		}
		s.forward(w, "/api/entities/"+eid+"/thoughts/"+tid)
	}))
	mux.HandleFunc("POST /api/entities/{eid}/thoughts/{tid}", write(func(w http.ResponseWriter, r *http.Request) {
		eid, tid := r.PathValue("eid"), r.PathValue("tid")
		var in struct {
			Action string `json:"action"`
		}
		if !decode(w, r, &in) {
			return
		}
		switch {
		case !uuidRe.MatchString(eid) || !uuidRe.MatchString(tid):
			writeErr(w, fmt.Errorf("%w: identity or Thought ID is not valid", ErrInvalid))
			return
		case in.Action != "sleep" && in.Action != "wake" && in.Action != "resolve" && in.Action != "reopen" && in.Action != "think":
			writeErr(w, fmt.Errorf("%w: action must be sleep, wake, resolve, reopen or think", ErrInvalid))
			return
		}
		code, body, err := a.src.Action(eid, tid, in.Action)
		if err != nil {
			writeJSON(w, 502, map[string]string{"error": err.Error()})
			return
		}
		writeRaw(w, code, body)
	}))

	mux.HandleFunc("POST /api/source", write(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Address string `json:"address"`
		}
		if !decode(w, r, &in) {
			return
		}
		if err := a.SetThoughtAddress(in.Address); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"source": a.src.State()})
	}))
	mux.HandleFunc("POST /api/source/scan", write(func(w http.ResponseWriter, r *http.Request) {
		hits := Scan(a.cfg.Port)
		a.events.Add("source", fmt.Sprintf("searched 127.0.0.1 ports 7778–7899 for Thought engines (read-only): %d found", len(hits)))
		writeJSON(w, 200, map[string]any{"hits": hits})
	}))
	mux.HandleFunc("GET /api/diagnostics", local(s.handleDiagnostics))
	return mux
}

// forward returns the engine's reply for an entity-scoped read, with the
// engine's own status code (e.g. 404 for a Thought another identity owns).
func (s *server) forward(w http.ResponseWriter, path string) {
	code, body, err := s.app.src.Read(path)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	writeRaw(w, code, body)
}

func (s *server) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	a := s.app
	raw, st := a.src.Status()
	c := a.cfg
	var b strings.Builder
	fmt.Fprintf(&b, "## Thought Controller diagnostics (%s)\n\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "- Thought Controller %s · protocol %s · Go %s · %s/%s\n", c.Version, protocol, runtime.Version(), runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&b, "- Branch: %s (from %s)%s\n- Commit: %s\n", c.Branch, c.BranchFrom, map[bool]string{true: " · detached checkout", false: ""}[c.Detached], c.Commit)
	fmt.Fprintf(&b, "- Instance: %s (created %s) · pid %d · port %s · up %s\n", a.inst.InstanceID, a.inst.CreatedAt.Format(time.RFC3339), os.Getpid(), c.Port, time.Since(a.started).Round(time.Second))
	fmt.Fprintf(&b, "- Data folder: %s (from %s) — holds only instance, settings and log; thought data stays in the Thought engine\n", c.DataDir, c.DataSource)
	fmt.Fprintf(&b, "- Identity: %s\n", identityStatus["state"])
	fmt.Fprintf(&b, "- External calls: AI model 0 · GitHub 0 · Identity 0 · processes launched 0\n")
	fmt.Fprintf(&b, "\n### Thought engine\n\n- Address: %s · state **%s** — %s\n", st.Address, st.State, st.Detail)
	if st.CheckedAt != nil {
		fmt.Fprintf(&b, "- Last check: %s · %d ms", st.CheckedAt.Format(time.RFC3339), st.LatencyMS)
		if st.LastOKAt != nil {
			fmt.Fprintf(&b, " · last good read %s", st.LastOKAt.Format(time.RFC3339))
		}
		b.WriteString("\n")
	}
	k := st.Counters
	fmt.Fprintf(&b, "- Reads %d · read errors %d · owner actions sent %d (accepted %d, refused by engine %d) · engine changes seen %d\n",
		k.Reads, k.ReadErrors, k.ActionsSent, k.ActionsAccepted, k.ActionsRefused, k.SourceChanges)
	if st.LastError != "" {
		fmt.Fprintf(&b, "- Last error (%s): %s\n", st.LastErrorAt.Format(time.RFC3339), st.LastError)
	}
	if e := st.Engine; e != nil {
		fmt.Fprintf(&b, "- Engine: %s · branch %s · commit %s · pid %d\n- Engine mode (as the engine reports it): %s\n- Engine profile %s · installation %s · Thought service %s\n- Engine rule: %s\n- Engine problem files: %d\n",
			e.Version, e.Branch, e.Commit, e.PID, e.Mode, e.Profile, e.InstallationID, e.ThoughtServiceID, e.RuleText, e.Problems)
	}
	if raw != nil {
		var sh struct {
			Entities []struct {
				Card struct {
					EntityID   string `json:"entity_id"`
					EntityType string `json:"entity_type"`
				} `json:"card"`
				Grant struct {
					Enabled bool   `json:"enabled"`
					Kind    string `json:"kind"`
				} `json:"grant"`
				Counts map[string]int `json:"counts"`
			} `json:"entities"`
		}
		if json.Unmarshal(raw, &sh) == nil {
			fmt.Fprintf(&b, "\n### Identities (as the engine reports them)\n\n")
			for _, e := range sh.Entities {
				fmt.Fprintf(&b, "- %s · %s · Thought %s · events %d · Traces %d · Thoughts active %d / sleeping %d / resolved %d\n", e.Card.EntityID, e.Card.EntityType,
					map[bool]string{true: "ENABLED", false: "DISABLED"}[e.Grant.Enabled], e.Counts["events"], e.Counts["traces"], e.Counts["thoughts_active"], e.Counts["thoughts_sleeping"], e.Counts["thoughts_resolved"])
			}
		}
	}
	a.mu.Lock()
	for _, p := range a.problems {
		fmt.Fprintf(&b, "- Problem: %s\n", p)
	}
	a.mu.Unlock()
	if e := a.events.Err(); e != "" {
		fmt.Fprintf(&b, "- Activity log: %s\n", e)
	}
	b.WriteString("\n### Recent activity\n\n")
	for _, e := range a.events.Recent(15) {
		fmt.Fprintf(&b, "- %s %s: %s\n", e.At.Format("15:04:05"), e.Kind, e.Detail)
	}
	b.WriteString("\n_Names, thought questions and event text are left out. No credentials exist in this app._\n")
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write([]byte(b.String()))
}

// ---------- safety ----------

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
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
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
	}
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func writeRaw(w http.ResponseWriter, code int, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	w.Write(body)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}
