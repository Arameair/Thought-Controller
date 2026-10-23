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

// fakeJimmy is a stand-in Chat-Jimmy worker with a settable status.
type fakeJimmy struct {
	*httptest.Server
	ready     bool
	mode, url string
	simulated bool
	reply     string
	asks      int
	lastAsk   map[string]any
}

func newFakeJimmy(t *testing.T) *fakeJimmy {
	f := &fakeJimmy{ready: true, mode: "live", url: "http://127.0.0.1:8768/", reply: "NEW: Why does Goldshire keep mattering?"}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/heartbeat":
			json.NewEncoder(w).Encode(map[string]any{"ok": f.ready, "state": map[bool]string{true: "ready", false: "starting"}[f.ready]})
		case "/api/status":
			json.NewEncoder(w).Encode(map[string]any{"provider_ready": f.ready, "state": map[bool]string{true: "ready", false: "starting"}[f.ready],
				"mode": f.mode, "url": f.url, "last_error": nil})
		case "/api/ask":
			f.asks++
			json.NewDecoder(r.Body).Decode(&f.lastAsk)
			json.NewEncoder(w).Encode(map[string]any{"status": "success", "response": f.reply, "simulated": f.simulated,
				"metrics": map[string]any{"wall_ms": 190, "first_text_ms": 140}})
		}
	}))
	t.Cleanup(f.Close)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CHAT_JIMMY_URL", "")
	os.MkdirAll(filepath.Join(home, ".chat-jimmy"), 0o700)
	os.WriteFile(filepath.Join(home, ".chat-jimmy", "worker.json"), []byte(`{"url":"`+f.URL+`"}`), 0o600)
	return f
}

func workerInfo(t *testing.T, ts *httptest.Server) map[string]any {
	res, err := http.Get(ts.URL + "/api/worker")
	if err != nil {
		t.Fatal(err)
	}
	var ws map[string]any
	json.NewDecoder(res.Body).Decode(&ws)
	return ws
}

// Found through worker.json; readiness never sends; one ask per beat; source labeled.
func TestChatJimmyWorker(t *testing.T) {
	f := newFakeJimmy(t)
	s, ts := newTestServer(t, &mockWorker{})
	post(t, ts.URL+"/api/worker", map[string]any{"worker": "chat-jimmy"})
	ws := workerInfo(t, ts)
	if ws["ready"] != true || ws["source"] != "test site http://127.0.0.1:8768/" || ws["simulated"] != true || f.asks != 0 {
		t.Fatalf("readiness: %v asks %d", ws, f.asks)
	}
	post(t, ts.URL+"/api/event", map[string]any{"text": "I entered Goldshire"})
	_, out := post(t, ts.URL+"/api/step", map[string]any{})
	if f.asks != 1 || out["source"] != "test site http://127.0.0.1:8768/" || out["took_ms"].(float64) != 190 || len(s.mind.Thoughts) != 1 {
		t.Fatalf("step: asks %d out %v", f.asks, out)
	}
	if f.lastAsk["source"] != "thought-controller" || f.lastAsk["new_chat"] != true {
		t.Fatalf("request %v", f.lastAsk)
	}
	if out["simulated"] != true {
		t.Fatal("a test site is never real-model evidence")
	}
}

func TestChatJimmyProvenance(t *testing.T) {
	f := newFakeJimmy(t)
	_, ts := newTestServer(t, &jimmyWorker{})
	f.url = "https://chatjimmy.ai/"
	if ws := workerInfo(t, ts); ws["source"] != "chatjimmy.ai live site" || ws["live"] != true || ws["simulated"] != false {
		t.Fatalf("live: %v", ws)
	}
	f.mode = "mock"
	if ws := workerInfo(t, ts); ws["source"] != "chat-jimmy mock" || ws["simulated"] != true {
		t.Fatalf("mock: %v", ws)
	}
	// a worker that says live but returns a simulated reply is still marked simulated
	f.mode, f.simulated = "live", true
	post(t, ts.URL+"/api/event", map[string]any{"text": "hello"})
	_, out := post(t, ts.URL+"/api/step", map[string]any{})
	if out["simulated"] != true {
		t.Fatalf("simulated reply flag lost: %v", out)
	}
}

func TestChatJimmyNotReadyNeverSends(t *testing.T) {
	f := newFakeJimmy(t)
	f.ready = false
	s, ts := newTestServer(t, &jimmyWorker{})
	post(t, ts.URL+"/api/event", map[string]any{"text": "hello"})
	_, out := post(t, ts.URL+"/api/step", map[string]any{})
	st := out["stages"].(map[string]any)
	if f.asks != 0 || out["outcome"] != "not ready" || st["sent"] != "no" || s.mind.LastSeenRev != 0 {
		t.Fatalf("asks %d out %v", f.asks, out)
	}
	if ws := workerInfo(t, ts); ws["ready"] != false {
		t.Fatalf("%v", ws)
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
	if out["error_cat"] != "unavailable" || out["outcome"] != "not ready" || s.mind.byID(1).State != stActive {
		t.Fatalf("%v", out)
	}
}
