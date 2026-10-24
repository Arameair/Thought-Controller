package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

func call(t *testing.T, method, url string, body any, hdr map[string]string) (int, map[string]any, string) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, url, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		if k == "Host" {
			req.Host = v
		} else {
			req.Header.Set(k, v)
		}
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	var m map[string]any
	json.Unmarshal(raw, &m)
	return res.StatusCode, m, string(raw)
}

func hostPort(u string) string { return strings.TrimPrefix(u, "http://") }

func newTestApp(t *testing.T, dir, addr string) *httptest.Server {
	t.Helper()
	ev := openEventLog(filepath.Join(dir, "events.jsonl"), false)
	t.Cleanup(ev.Close)
	a, err := openApp(Config{DataDir: dir, Commit: "test", Branch: "test", Port: "0"}, ev, addr)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer((&server{app: a}).routes())
	t.Cleanup(ts.Close)
	return ts
}

const goodStatus = `{"config":{"version":"thought-spark-trace-v0.1","branch":"b","commit":"c"},"pid":1,"mode":"TEST","profile":"p",
"rule_text":"3 → 1","installation":{"installation_id":"11111111-1111-4111-8111-111111111111","thought_service_id":"22222222-2222-4222-8222-222222222222"},
"entities":[{"card":{"entity_id":"33333333-3333-4333-8333-333333333333","display_name":"TASS","entity_type":"ai_companion"},"grant":{"enabled":true}}],"problems":[]}`

func TestClassifyStatus(t *testing.T) {
	cases := []struct {
		code int
		body string
		want string
	}{
		{200, goodStatus, StateConnected},
		{200, `{"app":"identity"}`, StateNotThought},
		{404, `nope`, StateNotThought},
		{200, `{"config":{"version":"identity-first-admin-v0.1"}}`, StateNotThought},
		{200, strings.Replace(goodStatus, "thought-spark-trace-v0.1", "thought-spark-trace-v1.0", 1), StateIncompatible},
		{200, strings.Replace(goodStatus, "22222222-2222-4222-8222-222222222222", "bad", 1), StateIncompatible},
	}
	for i, c := range cases {
		got, _, _, info := classifyStatus(c.code, []byte(c.body))
		if got != c.want {
			t.Errorf("case %d: got %s want %s", i, got, c.want)
		}
		if got == StateConnected && (info.Identities != 1 || info.Enabled != 1 || info.ThoughtServiceID == "") {
			t.Errorf("engine info not filled: %+v", info)
		}
	}
}

func TestEngineDownAndWrongProgramAreHonest(t *testing.T) {
	// Nothing listening.
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	dead := l.Addr().String()
	l.Close()
	ts := newTestApp(t, t.TempDir(), dead)
	_, o, _ := call(t, "GET", ts.URL+"/api/overview", nil, nil)
	if o["source"].(map[string]any)["state"] != StateNotReachable || o["status"] != nil {
		t.Fatalf("engine down must be NOT_REACHABLE with no data: %v", o["source"])
	}
	if c, _, _ := call(t, "GET", ts.URL+"/api/entities/33333333-3333-4333-8333-333333333333", nil, nil); c != 502 {
		t.Fatalf("entity read with engine down should be 502, got %d", c)
	}
	// Something that is not a Thought engine.
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"app":"identity","version":"identity-first-admin-v0.1"}`))
	}))
	defer other.Close()
	call(t, "POST", ts.URL+"/api/source", map[string]string{"address": hostPort(other.URL)}, nil)
	_, o, _ = call(t, "GET", ts.URL+"/api/overview", nil, nil)
	if o["source"].(map[string]any)["state"] != StateNotThought || o["status"] != nil {
		t.Fatalf("an unrelated program must not be trusted: %v", o["source"])
	}
	if c, _, _ := call(t, "POST", ts.URL+"/api/entities/33333333-3333-4333-8333-333333333333/thoughts/44444444-4444-4444-8444-444444444444", map[string]string{"action": "sleep"}, nil); c != 502 {
		t.Fatalf("no action may be sent to an untrusted program, got %d", c)
	}
}

func TestInputValidationAndGuards(t *testing.T) {
	var got []string
	eng := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/status" {
			w.Write([]byte(goodStatus))
			return
		}
		w.Write([]byte(`{}`))
	}))
	defer eng.Close()
	ts := newTestApp(t, t.TempDir(), hostPort(eng.URL))
	call(t, "GET", ts.URL+"/api/overview", nil, nil)
	for _, p := range []string{"/api/entities/..%2f..%2fapi%2fstatus", "/api/entities/not-a-uuid", "/api/entities/33333333-3333-4333-8333-333333333333/thoughts/x"} {
		if c, _, _ := call(t, "GET", ts.URL+p, nil, nil); c != 400 && c != 404 {
			t.Errorf("%s should be refused, got %d", p, c)
		}
	}
	if c, _, _ := call(t, "POST", ts.URL+"/api/entities/33333333-3333-4333-8333-333333333333/thoughts/44444444-4444-4444-8444-444444444444", map[string]string{"action": "delete"}, nil); c != 400 {
		t.Fatal("unknown action must be refused before reaching the engine")
	}
	for _, g := range got {
		if strings.Contains(g, "not-a-uuid") || strings.Contains(g, "delete") || strings.Contains(g, "..") {
			t.Fatalf("invalid input reached the engine: %s", g)
		}
	}
	if c, _, _ := call(t, "GET", ts.URL+"/api/overview", nil, map[string]string{"Host": "evil.example"}); c != 403 {
		t.Fatal("non-loopback Host must be refused")
	}
	if c, _, _ := call(t, "POST", ts.URL+"/api/source", map[string]string{"address": "127.0.0.1:1"}, map[string]string{"Origin": "http://evil.example"}); c != 403 {
		t.Fatal("cross-origin write must be refused")
	}
	for _, bad := range []string{"example.com:80", "10.0.0.1:7798", "127.0.0.1", "127.0.0.1:0"} {
		if c, _, _ := call(t, "POST", ts.URL+"/api/source", map[string]string{"address": bad}, nil); c != 400 {
			t.Errorf("address %s should be refused", bad)
		}
	}
	_, h, _ := call(t, "GET", ts.URL+"/api/health", nil, nil)
	if h["app"] != "thought-controller" || h["protocol"] != protocol || h["source"].(map[string]any)["state"] != StateConnected {
		t.Fatalf("health: %v", h)
	}
}

func TestDataFolderRules(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	git := func(a ...string) (string, error) {
		switch {
		case a[len(a)-1] == "--show-toplevel":
			return "/src/tc", nil
		case len(a) == 2:
			return "abcdef0123456789abcdef0123456789abcdef01", nil
		}
		return "HEAD", nil
	}
	c, err := resolveConfig("", env(map[string]string{"PC_DATA": "/pc/x.data", "PC_BRANCH": "experiment/a", "PORT": "8123"}), git, "/home/u")
	if err != nil || c.DataDir != "/pc/x.data/thought-controller" || c.Branch != "experiment/a" || !c.Detached || c.Port != "8123" {
		t.Fatalf("PC config: %+v %v", c, err)
	}
	c, _ = resolveConfig("", env(nil), git, "/home/u")
	if c.DataDir != "/home/u/.thought-controller-dashboard/commit-abcdef012345" {
		t.Fatalf("standalone detached default: %s", c.DataDir)
	}
	if _, err := resolveConfig("/src/tc/data", env(nil), git, "/home/u"); err == nil {
		t.Fatal("data inside the checkout must be refused")
	}
}

// ---------- real programs ----------

type proc struct {
	cmd *exec.Cmd
	out *bytes.Buffer
}

func freePort(t *testing.T) string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return strings.Split(l.Addr().String(), ":")[1]
}

func startProc(t *testing.T, dir, bin, port string, env ...string) *proc {
	t.Helper()
	cmd := exec.Command(bin)
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), "PORT="+port), env...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		if r, err := http.Get("http://127.0.0.1:" + port + "/"); err == nil {
			r.Body.Close()
			return &proc{cmd, &out}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("%s did not start:\n%s", bin, out.String())
	return nil
}

func (p *proc) stop() {
	p.cmd.Process.Signal(syscall.SIGTERM)
	p.cmd.Wait()
}

func build(t *testing.T, dir, out string) {
	t.Helper()
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = dir
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", dir, err, b)
	}
}

// TestRealProcessKeepsInstanceAndSettings: Thought Controller alone, the way
// Project Controller runs it (PORT, PC_BRANCH, PC_DATA), stopped and played again.
func TestRealProcessKeepsInstanceAndSettings(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses SIGTERM")
	}
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "tc")
	wd, _ := os.Getwd()
	build(t, wd, bin)
	pc, port := t.TempDir(), freePort(t)
	base := "http://127.0.0.1:" + port
	p := startProc(t, wd, bin, port, "PC_BRANCH=experiment/real", "PC_DATA="+pc)
	_, h, _ := call(t, "GET", base+"/api/health", nil, nil)
	id := h["instance_id"].(string)
	if h["build"].(map[string]any)["branch"] != "experiment/real" || h["source"].(map[string]any)["state"] == StateConnected {
		t.Fatalf("health: %v", h)
	}
	call(t, "POST", base+"/api/source", map[string]string{"address": "127.0.0.1:7999"}, nil)
	dup := exec.Command(bin)
	dup.Env = append(os.Environ(), "PORT=0", "PC_DATA="+pc)
	if b, err := dup.CombinedOutput(); err == nil || !strings.Contains(string(b), "already using data folder") {
		t.Fatalf("second copy on the same data must refuse: %v %s", err, b)
	}
	p.stop()
	if !strings.Contains(p.out.String(), "stopped cleanly") {
		t.Fatalf("no clean stop:\n%s", p.out)
	}
	p = startProc(t, wd, bin, port, "PC_BRANCH=experiment/real", "PC_DATA="+pc)
	defer p.stop()
	_, h, _ = call(t, "GET", base+"/api/health", nil, nil)
	if h["instance_id"] != id || h["source"].(map[string]any)["address"] != "127.0.0.1:7999" {
		t.Fatalf("instance ID and engine address must survive restart: %v", h)
	}
}

// TestWithRealThoughtEngine runs the REAL Thought engine and the REAL
// Thought Controller together. Set THOUGHT_SRC to an Arameair/Thought
// checkout (branch experiment/spark-trace-thought-v1-20261023):
//
//	THOUGHT_SRC=/path/to/Thought go test -run RealThought -v
func TestWithRealThoughtEngine(t *testing.T) {
	src := os.Getenv("THOUGHT_SRC")
	if src == "" {
		t.Skip("set THOUGHT_SRC to a Thought engine checkout to run the two-program test")
	}
	if runtime.GOOS == "windows" {
		t.Skip("uses SIGTERM")
	}
	tmp := t.TempDir()
	tcBin, thBin := filepath.Join(tmp, "tc"), filepath.Join(tmp, "thought")
	wd, _ := os.Getwd()
	build(t, wd, tcBin)
	build(t, src, thBin)
	thPort, tcPort := freePort(t), freePort(t)
	TH, TC := "http://127.0.0.1:"+thPort, "http://127.0.0.1:"+tcPort
	thData, tcData := filepath.Join(tmp, "th"), filepath.Join(tmp, "tc.data")

	tc := startProc(t, wd, tcBin, tcPort, "PC_DATA="+tcData, "THOUGHT_ADDR=127.0.0.1:"+thPort)
	defer func() { tc.stop() }()
	_, o, _ := call(t, "GET", TC+"/api/overview", nil, nil)
	if o["source"].(map[string]any)["state"] != StateNotReachable {
		t.Fatal("before the engine runs, the dashboard must say NOT_REACHABLE")
	}
	th := startProc(t, src, thBin, thPort, "PC_DATA="+thData)
	defer func() { th.stop() }()
	_, o, _ = call(t, "GET", TC+"/api/overview", nil, nil)
	if o["source"].(map[string]any)["state"] != StateConnected {
		t.Fatalf("engine running → CONNECTED: %v", o["source"])
	}
	var tass, shop string
	for _, e := range o["status"].(map[string]any)["entities"].([]any) {
		c := e.(map[string]any)["card"].(map[string]any)
		if c["display_name"] == "TASS" {
			tass = c["entity_id"].(string)
		} else {
			shop = c["entity_id"].(string)
		}
	}
	// Events are produced in the engine (its own buttons/API); the dashboard only reads them.
	call(t, "POST", TH+"/api/entities/"+tass+"/grant", map[string]bool{"enabled": true}, nil)
	for i := 0; i < 3; i++ {
		call(t, "POST", TH+"/api/entities/"+tass+"/simulate", map[string]string{"preset": "goldshire"}, nil)
	}
	_, ent, _ := call(t, "GET", TC+"/api/entities/"+tass, nil, nil)
	ths := ent["thoughts"].([]any)
	if len(ths) != 1 || len(ent["traces"].([]any)) != 3 {
		t.Fatalf("dashboard must show the engine's 1 Thought and 3 Traces: %v", ent["counts"])
	}
	tid := ths[0].(map[string]any)["thought_id"].(string)
	_, d, _ := call(t, "GET", TC+"/api/entities/"+tass+"/thoughts/"+tid, nil, nil)
	if len(d["evidence"].([]any)) != 3 {
		t.Fatal("thought detail must carry the engine's evidence")
	}
	if c, _, _ := call(t, "GET", TC+"/api/entities/"+shop+"/thoughts/"+tid, nil, nil); c != 404 {
		t.Fatalf("Business path must not reach TASS's Thought through the dashboard, got %d", c)
	}
	if c, m, _ := call(t, "POST", TC+"/api/entities/"+tass+"/thoughts/"+tid, map[string]string{"action": "sleep"}, nil); c != 200 || m["thought"].(map[string]any)["state"] != "sleeping" {
		t.Fatalf("sleep via dashboard: %d %v", c, m)
	}
	_, eng, _ := call(t, "GET", TH+"/api/entities/"+tass+"/thoughts/"+tid, nil, nil)
	if eng["thought"].(map[string]any)["state"] != "sleeping" {
		t.Fatal("the change must be made in the engine itself")
	}
	if c, _, _ := call(t, "POST", TC+"/api/entities/"+tass+"/thoughts/"+tid, map[string]string{"action": "sleep"}, nil); c != 400 {
		t.Fatalf("the engine's own refusal must be passed through (sleeping a sleeping Thought), got %d", c)
	}
	call(t, "POST", TH+"/api/entities/"+tass+"/grant", map[string]bool{"enabled": false}, nil)
	if c, m, _ := call(t, "POST", TC+"/api/entities/"+tass+"/thoughts/"+tid, map[string]string{"action": "wake"}, nil); c != 403 || !strings.Contains(m["error"].(string), "not enabled") {
		t.Fatalf("with permission off the engine must refuse, and the dashboard must say so: %d %v", c, m)
	}
	call(t, "POST", TH+"/api/entities/"+tass+"/grant", map[string]bool{"enabled": true}, nil)

	// Engine stops: honest NOT_REACHABLE, no stale data. Engine comes back: same Thought.
	th.stop()
	_, o, _ = call(t, "GET", TC+"/api/overview", nil, nil)
	if o["source"].(map[string]any)["state"] != StateNotReachable || o["status"] != nil {
		t.Fatal("engine stopped → NOT_REACHABLE, no data shown")
	}
	th = startProc(t, src, thBin, thPort, "PC_DATA="+thData)
	_, ent, _ = call(t, "GET", TC+"/api/entities/"+tass, nil, nil)
	if ent == nil || ent["thoughts"] == nil || ent["thoughts"].([]any)[0].(map[string]any)["thought_id"] != tid {
		t.Fatalf("after the engine restarts, the dashboard shows the same Thought: %v", ent)
	}
	// Dashboard restarts: instance and engine address kept, data still from the engine.
	tc.stop()
	tc = startProc(t, wd, tcBin, tcPort, "PC_DATA="+tcData)
	_, o, _ = call(t, "GET", TC+"/api/overview", nil, nil)
	if o["source"].(map[string]any)["state"] != StateConnected {
		t.Fatalf("dashboard restart must reconnect from saved settings: %v", o["source"])
	}
	_, _, diag := call(t, "GET", TC+"/api/diagnostics", nil, nil)
	for _, want := range []string{"state **CONNECTED**", "thought-spark-trace-v0.", tass, "Thought ENABLED · events 3 · Traces 3 · Thoughts active 0 / sleeping 1"} {
		if !strings.Contains(diag, want) {
			t.Fatalf("diagnostics missing %q:\n%s", want, diag)
		}
	}
	if strings.Contains(diag, "Goldshire") || strings.Contains(diag, "TASS") {
		t.Fatalf("diagnostics must leave names and thought text out:\n%s", diag)
	}
}
