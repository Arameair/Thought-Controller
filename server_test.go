package main

import (
	"bytes"
	"encoding/json"
	"io"
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

func newTestServer(t *testing.T) (*httptest.Server, *Host) {
	h := newTestHost(t, t.TempDir())
	ts := httptest.NewServer((&server{host: h}).routes())
	t.Cleanup(ts.Close)
	return ts, h
}

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

func TestHTTPHandshakeAndScoping(t *testing.T) {
	ts, _ := newTestServer(t)
	code, h, _ := call(t, "GET", ts.URL+"/api/health", nil, nil)
	if code != 200 || h["simulated"] != true || h["protocol"] != protocol || h["service"].(map[string]any)["state"] != "not_started" {
		t.Fatalf("health before start: %d %+v", code, h)
	}
	a := newInstall(t)
	if c, _, _ := call(t, "GET", ts.URL+"/api/entities/"+a.tass+"/thoughts", nil, nil); c != 409 {
		t.Fatalf("thought reads before start should be 409, got %d", c)
	}
	code, res, raw := call(t, "POST", ts.URL+"/api/sim/lifecycle/start", a.req("h1"), nil)
	if code != 200 || res["result"] != "started" {
		t.Fatalf("start: %d %s", code, raw)
	}
	_, h, _ = call(t, "GET", ts.URL+"/api/health", nil, nil)
	svc := h["service"].(map[string]any)
	if svc["service_id"] != a.sid || svc["installation_id"] != a.iid || svc["state"] != "ready" {
		t.Fatalf("health after start: %+v", svc)
	}
	b := newInstall(t)
	if c, r, _ := call(t, "POST", ts.URL+"/api/sim/lifecycle/start", b.req("h2"), nil); c != 409 || r["result"] != "refused" {
		t.Fatalf("other installation: %d %+v", c, r)
	}
	code, cr, _ := call(t, "POST", ts.URL+"/api/entities/"+a.tass+"/thoughts", map[string]string{"title": "Secret-ish title", "state": "active"}, nil)
	if code != 201 {
		t.Fatalf("create: %d", code)
	}
	tid := cr["thought"].(map[string]any)["thought_id"].(string)
	if c, _, _ := call(t, "GET", ts.URL+"/api/entities/"+a.shop+"/thoughts/"+tid, nil, nil); c != 404 {
		t.Fatalf("cross-entity read must be 404, got %d", c)
	}
	if c, _, _ := call(t, "POST", ts.URL+"/api/entities/"+a.shop+"/thoughts/"+tid, map[string]string{"state": "resolved"}, nil); c != 404 {
		t.Fatalf("cross-entity write must be 404, got %d", c)
	}
	if c, _, _ := call(t, "POST", ts.URL+"/api/entities/"+a.shop+"/samples", map[string]string{}, nil); c != 201 {
		t.Fatalf("samples: %d", c)
	}
	_, lt, _ := call(t, "GET", ts.URL+"/api/entities/"+a.tass+"/thoughts", nil, nil)
	_, lb, _ := call(t, "GET", ts.URL+"/api/entities/"+a.shop+"/thoughts", nil, nil)
	if len(lt["thoughts"].([]any)) != 1 || len(lb["thoughts"].([]any)) != 4 {
		t.Fatal("counts per entity wrong")
	}
	_, _, diag := call(t, "GET", ts.URL+"/api/diagnostics", nil, nil)
	if strings.Contains(diag, "Secret-ish") || strings.Contains(diag, "TASS") || !strings.Contains(diag, a.sid) || !strings.Contains(diag, "SIMULATED HOST") {
		t.Fatalf("diagnostics must carry IDs but no titles/names:\n%s", diag)
	}
	_, _, td := call(t, "GET", ts.URL+"/api/entities/"+a.tass+"/thoughts/"+tid+"/diagnostics", nil, nil)
	if strings.Contains(td, "Secret-ish") || !strings.Contains(td, "worker/model calls for this thought: 0") {
		t.Fatalf("thought diagnostics: %s", td)
	}
	// Register a later identity, then stop.
	extra := mustUUID(t)
	if c, r, _ := call(t, "POST", ts.URL+"/api/sim/register", map[string]any{"installation_id": a.iid, "service_id": a.sid,
		"entities": []EntityRef{{EntityID: extra, EntityType: "project", DisplayName: "Later"}}}, nil); c != 200 || r["registrations"].(map[string]any)[extra] != "registered" {
		t.Fatalf("register: %d %+v", c, r)
	}
	if c, _, _ := call(t, "POST", ts.URL+"/api/sim/register", map[string]any{"installation_id": b.iid, "service_id": b.sid, "entities": []EntityRef{}}, nil); c != 409 {
		t.Fatal("registration for another installation must be denied")
	}
	if c, _, _ := call(t, "POST", ts.URL+"/api/sim/lifecycle/stop", map[string]string{"installation_id": b.iid}, nil); c != 409 {
		t.Fatal("stop for another installation must be denied")
	}
	if c, _, _ := call(t, "POST", ts.URL+"/api/sim/lifecycle/stop", map[string]string{"installation_id": a.iid}, nil); c != 200 {
		t.Fatal("stop failed")
	}
}

func TestHTTPGuards(t *testing.T) {
	ts, _ := newTestServer(t)
	a := newInstall(t)
	if c, _, _ := call(t, "GET", ts.URL+"/api/health", nil, map[string]string{"Host": "evil.example:80"}); c != 403 {
		t.Fatal("non-loopback Host must be refused")
	}
	if c, _, _ := call(t, "POST", ts.URL+"/api/sim/lifecycle/start", a.req("g"), map[string]string{"Origin": "http://127.0.0.1:9999"}); c != 403 {
		t.Fatal("cross-origin write must be refused")
	}
	req, _ := http.NewRequest("POST", ts.URL+"/api/sim/lifecycle/start", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "text/plain")
	if r, _ := http.DefaultClient.Do(req); r.StatusCode != 415 {
		t.Fatal("non-JSON write must be refused")
	}
	body := map[string]any{"request_id": "x", "installation_id": a.iid, "service_id": a.sid, "admin_id": a.aid, "approved_commit": testCommit, "bogus": 1}
	if c, _, _ := call(t, "POST", ts.URL+"/api/sim/lifecycle/start", body, nil); c != 400 {
		t.Fatal("unknown fields must be refused")
	}
}

// TestRealProcess builds and runs the real binary the way Project Controller
// does (PORT, PC_BRANCH, PC_DATA), then stops it with SIGTERM.
func TestRealProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("signal test is Unix-only")
	}
	bin := filepath.Join(t.TempDir(), "tc")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	pc := t.TempDir()
	port := freePort(t)
	start := func() (*exec.Cmd, *bytes.Buffer) {
		cmd := exec.Command(bin)
		cmd.Env = append(os.Environ(), "PORT="+port, "PC_BRANCH=experiment/test", "PC_DATA="+pc)
		var out bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &out
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		waitUp(t, "http://127.0.0.1:"+port+"/api/health")
		return cmd, &out
	}
	cmd, out := start()
	_, h, _ := call(t, "GET", "http://127.0.0.1:"+port+"/api/health", nil, nil)
	build := h["build"].(map[string]any)
	commit := build["commit"].(string)
	if build["branch"] != "experiment/test" {
		t.Fatalf("branch: %+v", build)
	}
	a := newInstall(t)
	r := a.req("p1")
	r.ApprovedCommit = commit
	if c, res, raw := call(t, "POST", "http://127.0.0.1:"+port+"/api/sim/lifecycle/start", r, nil); c != 200 || res["result"] != "started" {
		t.Fatalf("start: %s", raw)
	}
	call(t, "POST", "http://127.0.0.1:"+port+"/api/entities/"+a.tass+"/thoughts", map[string]string{"title": "survives", "state": "active"}, nil)
	// A second host on the same data folder must refuse to start.
	dup := exec.Command(bin)
	dup.Env = append(os.Environ(), "PORT="+freePort(t), "PC_DATA="+pc)
	if b, err := dup.CombinedOutput(); err == nil || !strings.Contains(string(b), "already using data folder") {
		t.Fatalf("same-folder second copy should refuse: %v %s", err, b)
	}
	cmd.Process.Signal(syscall.SIGTERM)
	cmd.Wait()
	if !strings.Contains(out.String(), "host stopped cleanly") {
		t.Fatalf("no clean stop line:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(pc, "thought-controller-first-user", "thought-controller.lock")); !os.IsNotExist(err) {
		t.Fatal("lock not removed on stop")
	}
	cmd, _ = start()
	defer func() { cmd.Process.Signal(syscall.SIGTERM); cmd.Wait() }()
	_, h, _ = call(t, "GET", "http://127.0.0.1:"+port+"/api/health", nil, nil)
	if h["service"].(map[string]any)["state"] != "not_started" {
		t.Fatal("after restart the service must wait for the launcher")
	}
	r.RequestID = "p2"
	call(t, "POST", "http://127.0.0.1:"+port+"/api/sim/lifecycle/start", r, nil)
	_, lt, _ := call(t, "GET", "http://127.0.0.1:"+port+"/api/entities/"+a.tass+"/thoughts", nil, nil)
	if l := lt["thoughts"].([]any); len(l) != 1 || l[0].(map[string]any)["title"] != "survives" {
		t.Fatalf("thought not restored after real restart: %+v", lt)
	}
}

func freePort(t *testing.T) string {
	ts := httptest.NewServer(http.NotFoundHandler())
	p := ts.Listener.Addr().String()
	ts.Close()
	return p[strings.LastIndex(p, ":")+1:]
}

func waitUp(t *testing.T, url string) {
	for i := 0; i < 100; i++ {
		if r, err := http.Get(url); err == nil {
			r.Body.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("server did not come up: " + url)
}
