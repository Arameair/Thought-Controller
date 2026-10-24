package main

// Source is Thought Controller's connection to a running Thought engine
// (Arameair/Thought). Every read goes to the engine; nothing is cached as
// truth. A response is only trusted after it is checked to be a compatible
// Thought engine — an unrelated program answering on the port is refused.
import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Thought engine versions this dashboard understands.
const compatiblePrefix = "thought-spark-trace-v0."

const (
	StateNotChecked   = "NOT_CHECKED"
	StateNotReachable = "NOT_REACHABLE"
	StateNotThought   = "NOT_THOUGHT"
	StateIncompatible = "INCOMPATIBLE"
	StateConnected    = "CONNECTED"
)

// EngineInfo is what the dashboard knows about the engine it is reading.
type EngineInfo struct {
	Version          string `json:"version"`
	Branch           string `json:"branch"`
	Commit           string `json:"commit"`
	Mode             string `json:"mode"`
	Profile          string `json:"profile"`
	InstallationID   string `json:"installation_id"`
	ThoughtServiceID string `json:"thought_service_id"`
	RuleText         string `json:"rule_text"`
	PID              int    `json:"pid"`
	Identities       int    `json:"identities"`
	Enabled          int    `json:"identities_enabled"`
	Problems         int    `json:"problem_files"`
}

type SourceCounters struct {
	Reads           int `json:"reads"`
	ReadErrors      int `json:"read_errors"`
	ActionsSent     int `json:"actions_sent"`
	ActionsAccepted int `json:"actions_accepted"`
	ActionsRefused  int `json:"actions_refused"`
	SourceChanges   int `json:"source_changes"`
}

type SourceState struct {
	Address     string         `json:"address"`
	State       string         `json:"state"`
	Label       string         `json:"label"`
	Detail      string         `json:"detail"`
	CheckedAt   *time.Time     `json:"checked_at,omitempty"`
	LastOKAt    *time.Time     `json:"last_ok_at,omitempty"`
	LatencyMS   int64          `json:"latency_ms"`
	Engine      *EngineInfo    `json:"engine,omitempty"`
	Counters    SourceCounters `json:"counters"`
	LastError   string         `json:"last_error,omitempty"`
	LastErrorAt *time.Time     `json:"last_error_at,omitempty"`
}

type Source struct {
	mu     sync.Mutex
	st     SourceState
	events *EventLog
	client *http.Client
}

func newSource(addr string, events *EventLog) *Source {
	return &Source{events: events, st: SourceState{Address: addr, State: StateNotChecked, Label: "Not checked yet"},
		client: &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (s *Source) SetAddress(addr string) {
	s.mu.Lock()
	s.st = SourceState{Address: addr, State: StateNotChecked, Label: "Not checked yet", Counters: s.st.Counters}
	s.mu.Unlock()
}

func (s *Source) State() SourceState {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.st
	if st.Engine != nil {
		e := *st.Engine
		st.Engine = &e
	}
	return st
}

// statusShape is the part of the engine's /api/status this dashboard checks.
type statusShape struct {
	Config struct {
		Version string `json:"version"`
		Branch  string `json:"branch"`
		Commit  string `json:"commit"`
	} `json:"config"`
	PID          int    `json:"pid"`
	Mode         string `json:"mode"`
	Profile      string `json:"profile"`
	RuleText     string `json:"rule_text"`
	Installation struct {
		InstallationID   string `json:"installation_id"`
		ThoughtServiceID string `json:"thought_service_id"`
	} `json:"installation"`
	Entities []struct {
		Card struct {
			EntityID    string `json:"entity_id"`
			DisplayName string `json:"display_name"`
			EntityType  string `json:"entity_type"`
		} `json:"card"`
		Grant struct {
			Enabled bool `json:"enabled"`
		} `json:"grant"`
	} `json:"entities"`
	Problems []json.RawMessage `json:"problems"`
}

func classifyStatus(code int, body []byte) (state, label, detail string, info *EngineInfo) {
	var sh statusShape
	if code != 200 || json.Unmarshal(body, &sh) != nil || sh.Config.Version == "" {
		return StateNotThought, "NOT A THOUGHT ENGINE", fmt.Sprintf("Something answered (HTTP %d), but it is not a Thought engine. It is not trusted just because it answered.", code), nil
	}
	if !strings.HasPrefix(sh.Config.Version, "thought-") {
		return StateNotThought, "NOT A THOUGHT ENGINE", "This program reports version “" + sh.Config.Version + "”, which is not a Thought engine.", nil
	}
	if !strings.HasPrefix(sh.Config.Version, compatiblePrefix) {
		return StateIncompatible, "INCOMPATIBLE THOUGHT ENGINE", "This Thought engine is version “" + sh.Config.Version + "”; this dashboard understands " + compatiblePrefix + "x.", nil
	}
	if !uuidRe.MatchString(sh.Installation.ThoughtServiceID) || !uuidRe.MatchString(sh.Installation.InstallationID) {
		return StateIncompatible, "INCOMPATIBLE THOUGHT ENGINE", "The engine's reply has no valid installation or Thought service ID.", nil
	}
	info = &EngineInfo{Version: sh.Config.Version, Branch: sh.Config.Branch, Commit: sh.Config.Commit, Mode: sh.Mode, Profile: sh.Profile,
		InstallationID: sh.Installation.InstallationID, ThoughtServiceID: sh.Installation.ThoughtServiceID, RuleText: sh.RuleText,
		PID: sh.PID, Identities: len(sh.Entities), Problems: len(sh.Problems)}
	for _, e := range sh.Entities {
		if e.Grant.Enabled {
			info.Enabled++
		}
	}
	return StateConnected, "CONNECTED", "Reading live data from the Thought engine.", info
}

func (s *Source) get(path string) (int, []byte, time.Duration, error) {
	s.mu.Lock()
	addr := s.st.Address
	s.mu.Unlock()
	t0 := time.Now()
	res, err := s.client.Get("http://" + addr + path)
	if err != nil {
		return 0, nil, time.Since(t0), err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	return res.StatusCode, b, time.Since(t0), err
}

// Status reads the engine's /api/status, re-checks what answered, updates
// the connection state and returns the raw reply when it is trusted.
func (s *Source) Status() (json.RawMessage, SourceState) {
	code, body, d, err := s.get("/api/status")
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	st := &s.st
	prevState := st.State
	var prev *EngineInfo
	if st.Engine != nil {
		e := *st.Engine
		prev = &e
	}
	st.CheckedAt, st.LatencyMS = &now, d.Milliseconds()
	if err != nil {
		st.Counters.ReadErrors++
		st.State, st.Label, st.Engine = StateNotReachable, "NOT REACHABLE", nil
		st.Detail = "Nothing answered at " + st.Address + ". Is the Thought engine running? (" + shortErr(err) + ")"
		st.LastError, st.LastErrorAt = st.Detail, &now
	} else {
		state, label, detail, info := classifyStatus(code, body)
		st.State, st.Label, st.Detail, st.Engine = state, label, detail, info
		if state == StateConnected {
			st.Counters.Reads++
			st.LastOKAt = &now
		} else {
			st.Counters.ReadErrors++
			st.LastError, st.LastErrorAt = detail, &now
		}
	}
	if st.State != prevState && prevState != StateNotChecked {
		s.events.Add("source", fmt.Sprintf("Thought engine at %s: %s → %s (%s)", st.Address, prevState, st.State, st.Detail))
	} else if prevState == StateNotChecked {
		s.events.Add("source", fmt.Sprintf("Thought engine at %s: %s (%s)", st.Address, st.State, st.Detail))
	}
	if prev != nil && st.Engine != nil && (prev.ThoughtServiceID != st.Engine.ThoughtServiceID || prev.InstallationID != st.Engine.InstallationID || prev.Commit != st.Engine.Commit) {
		st.Counters.SourceChanges++
		s.events.Add("source", fmt.Sprintf("the Thought engine behind %s changed: installation %s → %s, profile %s → %s, commit %s → %s",
			st.Address, prev.InstallationID, st.Engine.InstallationID, prev.Profile, st.Engine.Profile, short(prev.Commit), short(st.Engine.Commit)))
	}
	if st.State != StateConnected {
		return nil, *st
	}
	return json.RawMessage(body), *st
}

func (s *Source) Probe(trigger string) SourceState {
	_, st := s.Status()
	return st
}

// Read fetches one entity-scoped engine path. The caller has already
// validated the IDs. The engine's own status code and error are returned.
func (s *Source) Read(path string) (int, json.RawMessage, error) {
	if st := s.State(); st.State != StateConnected {
		if _, st2 := s.Status(); st2.State != StateConnected {
			return 0, nil, fmt.Errorf("the Thought engine is %s: %s", st2.Label, st2.Detail)
		}
	}
	code, body, _, err := s.get(path)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.st.Counters.ReadErrors++
		now := time.Now().UTC()
		s.st.LastError, s.st.LastErrorAt = "read "+path+" failed: "+shortErr(err), &now
		return 0, nil, fmt.Errorf("the Thought engine stopped answering: %s", shortErr(err))
	}
	if !json.Valid(body) {
		s.st.Counters.ReadErrors++
		return 0, nil, fmt.Errorf("the Thought engine sent a reply that isn't JSON (HTTP %d)", code)
	}
	s.st.Counters.Reads++
	return code, json.RawMessage(body), nil
}

// Action forwards an owner's Sleep / Wake / Resolve / Reopen / Think to the engine.
// The engine decides: it checks the identity's permission and the Thought's
// state. Thought Controller never changes thought data itself.
func (s *Source) Action(eid, tid, action string) (int, json.RawMessage, error) {
	if st := s.State(); st.State != StateConnected {
		return 0, nil, fmt.Errorf("the Thought engine is %s — nothing was sent", st.Label)
	}
	s.mu.Lock()
	addr := s.st.Address
	s.st.Counters.ActionsSent++
	s.mu.Unlock()
	b, _ := json.Marshal(map[string]string{"action": action})
	// Think waits for the engine's AI worker, so actions get a longer timeout.
	slow := &http.Client{Timeout: 2 * time.Minute, CheckRedirect: s.client.CheckRedirect}
	res, err := slow.Post("http://"+addr+"/api/entities/"+eid+"/thoughts/"+tid, "application/json", bytes.NewReader(b))
	if err != nil {
		s.events.Add("error", fmt.Sprintf("%s request for Thought %s could not reach the engine: %s", action, tid, shortErr(err)))
		return 0, nil, fmt.Errorf("couldn't reach the Thought engine: %s — nothing was changed", shortErr(err))
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	s.mu.Lock()
	if res.StatusCode == 200 {
		s.st.Counters.ActionsAccepted++
	} else {
		s.st.Counters.ActionsRefused++
	}
	s.mu.Unlock()
	if res.StatusCode == 200 {
		s.events.Add("action", fmt.Sprintf("owner asked the Thought engine to %s Thought %s (identity %s): accepted", action, tid, eid))
	} else {
		var e struct {
			Error string `json:"error"`
		}
		json.Unmarshal(body, &e)
		// Only the engine's error category goes in the log: its full message can
		// contain display names, which diagnostics leave out.
		cat, _, _ := strings.Cut(e.Error, ":")
		s.events.Add("action", fmt.Sprintf("owner asked the Thought engine to %s Thought %s (identity %s): refused by the engine — HTTP %d %s", action, tid, eid, res.StatusCode, trimTo(cat, 40)))
	}
	if !json.Valid(body) {
		body = []byte(`{"error":"the Thought engine sent a reply that isn't JSON"}`)
	}
	return res.StatusCode, json.RawMessage(body), nil
}

type ScanHit struct {
	Address string `json:"address"`
	Version string `json:"version"`
	Branch  string `json:"branch"`
	Commit  string `json:"commit"`
	Profile string `json:"profile"`
	OK      bool   `json:"compatible"`
}

// Scan looks for Thought engines on this computer's usual Project Controller
// and standalone ports. Read-only GET /api/status only.
func Scan(skipPort string) []ScanHit {
	var mu sync.Mutex
	var hits []ScanHit
	var wg sync.WaitGroup
	sem := make(chan struct{}, 32)
	quick := &http.Client{Timeout: 500 * time.Millisecond}
	for p := 7778; p <= 7899; p++ {
		port := strconv.Itoa(p)
		if port == skipPort {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			res, err := quick.Get("http://127.0.0.1:" + port + "/api/status")
			if err != nil {
				return
			}
			defer res.Body.Close()
			b, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
			state, _, _, _ := classifyStatus(res.StatusCode, b)
			if state == StateNotThought {
				return
			}
			h := ScanHit{Address: "127.0.0.1:" + port, OK: state == StateConnected}
			var sh statusShape
			json.Unmarshal(b, &sh)
			h.Version, h.Branch, h.Commit, h.Profile = sh.Config.Version, sh.Config.Branch, sh.Config.Commit, sh.Profile
			mu.Lock()
			hits = append(hits, h)
			mu.Unlock()
		}()
	}
	wg.Wait()
	return hits
}

func trimTo(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n]
	}
	return s
}

func shortErr(err error) string {
	m := err.Error()
	if i := strings.LastIndex(m, ": "); i >= 0 && strings.Contains(m, "dial tcp") {
		return strings.TrimSpace(m[i+2:])
	}
	if len(m) > 160 {
		return m[:160]
	}
	return m
}
