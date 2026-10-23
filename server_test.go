package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestServer(t *testing.T, w Worker) (*server, *httptest.Server) {
	t.Helper()
	st, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.close)
	s := &server{mind: newMind(st), worker: w, workers: map[string]Worker{"mock": &mockWorker{}, "chat-jimmy": &jimmyWorker{}}, started: time.Now()}
	s.mind.Settings.GapMS = 5
	ts := httptest.NewServer(s.routes())
	t.Cleanup(ts.Close)
	return s, ts
}

func post(t *testing.T, url string, body any, hdr ...string) (int, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

func TestHTTPFlowWithMock(t *testing.T) {
	s, ts := newTestServer(t, &mockWorker{})
	if c, out := post(t, ts.URL+"/api/step", map[string]any{}); c != 200 || out["nothing"] != true {
		t.Fatalf("empty step: %d %v", c, out)
	}
	post(t, ts.URL+"/api/event", map[string]any{"text": "I entered Goldshire"})
	c, out := post(t, ts.URL+"/api/step", map[string]any{})
	if c != 200 || out["kind"] != "heartbeat" || out["simulated"] != true {
		t.Fatalf("%d %v", c, out)
	}
	if c, _ := post(t, ts.URL+"/api/run", map[string]any{"beats": 50}); c != 202 {
		t.Fatal("run")
	}
	deadline := time.Now().Add(10 * time.Second)
	for s.run.snapshot().Running && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	rs := s.run.snapshot()
	if rs.Planned != 20 || rs.Stopped != "nothing left to think about" {
		t.Fatalf("run capped at max_run_beats and stops when idle: %+v", rs)
	}
	res, _ := http.Get(ts.URL + "/api/state")
	var st struct {
		Thoughts []Thought `json:"thoughts"`
		Metrics  Metrics   `json:"metrics"`
	}
	json.NewDecoder(res.Body).Decode(&st)
	if len(st.Thoughts) < 2 || st.Metrics.Calls != st.Metrics.Simulated || st.Metrics.Calls < 3 {
		t.Fatalf("state %+v", st.Metrics)
	}
	for _, p := range []string{"/", "/api/trace", "/api/trace.jsonl", "/api/worker", "/api/heartbeat"} {
		if r, err := http.Get(ts.URL + p); err != nil || r.StatusCode != 200 {
			t.Fatalf("GET %s", p)
		}
	}
}

func TestStopRun(t *testing.T) {
	s, ts := newTestServer(t, &mockWorker{delay: 50 * time.Millisecond})
	s.mind.Settings.GapMS = 200
	for i := 0; i < 5; i++ {
		post(t, ts.URL+"/api/thought", map[string]any{"question": "Question number " + strings.Repeat("x", i+1) + " about topic " + string(rune('A'+i))})
	}
	post(t, ts.URL+"/api/run", map[string]any{"beats": 20})
	if c, _ := post(t, ts.URL+"/api/step", map[string]any{}); c != 409 {
		t.Fatal("step during run must be refused")
	}
	if c, _ := post(t, ts.URL+"/api/run", map[string]any{"beats": 5}); c != 409 {
		t.Fatal("second run must be refused")
	}
	time.Sleep(150 * time.Millisecond)
	post(t, ts.URL+"/api/stop", map[string]any{})
	deadline := time.Now().Add(5 * time.Second)
	for s.run.snapshot().Running && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	rs := s.run.snapshot()
	if rs.Running || rs.Stopped != "stopped by owner" || rs.Done >= 20 {
		t.Fatalf("%+v", rs)
	}
}

func TestGuardAndValidation(t *testing.T) {
	_, ts := newTestServer(t, &mockWorker{})
	if c, _ := post(t, ts.URL+"/api/event", map[string]any{"text": "x"}, "Origin", "https://evil.example"); c != 403 {
		t.Fatal("origin")
	}
	req, _ := http.NewRequest("POST", ts.URL+"/api/step", strings.NewReader("a=b"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if r, _ := http.DefaultClient.Do(req); r.StatusCode != 415 {
		t.Fatal("form post")
	}
	if c, _ := post(t, ts.URL+"/api/settings", map[string]any{"max_active": 0}); c != 400 {
		t.Fatal("bad settings accepted")
	}
	if c, _ := post(t, ts.URL+"/api/worker", map[string]any{"worker": "gpt"}); c != 400 {
		t.Fatal("bad worker accepted")
	}
	t.Setenv("THOUGHT_CONTROLLER_TOKEN", "k")
	if c, _ := post(t, ts.URL+"/api/event", map[string]any{"text": "x"}); c != 401 {
		t.Fatal("token")
	}
}

// Chat-Jimmy path: found through worker.json, readiness never sends, one ask per beat.
func TestChatJimmyWorker(t *testing.T) {
	asks := 0
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/heartbeat":
			json.NewEncoder(w).Encode(map[string]any{"ok": true, "state": "ready"})
		case "/api/ask":
			asks++
			var in map[string]any
			json.NewDecoder(r.Body).Decode(&in)
			if in["source"] != "thought-controller" || in["new_chat"] != true || !strings.Contains(in["message"].(string), "Goldshire") {
				t.Errorf("request %v", in)
			}
			json.NewEncoder(w).Encode(map[string]any{"status": "success", "response": "NEW: Why does Goldshire keep mattering?",
				"metrics": map[string]any{"wall_ms": 190, "first_text_ms": 140}})
		}
	}))
	defer fake.Close()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CHAT_JIMMY_URL", "")
	os.MkdirAll(filepath.Join(home, ".chat-jimmy"), 0o700)
	os.WriteFile(filepath.Join(home, ".chat-jimmy", "worker.json"), []byte(`{"url":"`+fake.URL+`"}`), 0o600)

	s, ts := newTestServer(t, &mockWorker{})
	post(t, ts.URL+"/api/worker", map[string]any{"worker": "chat-jimmy"})
	res, _ := http.Get(ts.URL + "/api/worker")
	var ws map[string]any
	json.NewDecoder(res.Body).Decode(&ws)
	if ws["ready"] != true || ws["simulated"] != false || asks != 0 {
		t.Fatalf("readiness: %v asks %d", ws, asks)
	}
	post(t, ts.URL+"/api/event", map[string]any{"text": "I entered Goldshire"})
	_, out := post(t, ts.URL+"/api/step", map[string]any{})
	if asks != 1 || out["simulated"] == true || out["took_ms"].(float64) != 190 || len(s.mind.Thoughts) != 1 {
		t.Fatalf("live step: asks %d out %v", asks, out)
	}
}

func TestChatJimmyNotRunning(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CHAT_JIMMY_URL", "")
	s, ts := newTestServer(t, &jimmyWorker{})
	post(t, ts.URL+"/api/thought", map[string]any{"question": "anything"})
	_, out := post(t, ts.URL+"/api/step", map[string]any{})
	if out["error_cat"] != "unavailable" || s.mind.byID(1).State != stActive {
		t.Fatalf("%v", out)
	}
}
