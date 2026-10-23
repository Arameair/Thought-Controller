package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testCommit = "1111111111111111111111111111111111111111"

func newTestHost(t *testing.T, dir string) *Host {
	t.Helper()
	ev := openEventLog(filepath.Join(dir, "host-events.jsonl"), false)
	t.Cleanup(ev.Close)
	h, err := openHost(Config{DataDir: dir, Commit: testCommit, Branch: "test", Port: "0"}, ev)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.Close)
	return h
}

func mustUUID(t *testing.T) string {
	id, err := newUUID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

type install struct{ iid, sid, aid, tass, shop string }

func newInstall(t *testing.T) install {
	return install{mustUUID(t), mustUUID(t), mustUUID(t), mustUUID(t), mustUUID(t)}
}

func (in install) req(rid string) StartRequest {
	return StartRequest{RequestID: rid, InstallationID: in.iid, ServiceID: in.sid, AdminID: in.aid,
		ApprovedCommit: testCommit, Launcher: "test", Entities: []EntityRef{
			{EntityID: in.tass, EntityType: "ai_companion", DisplayName: "TASS"},
			{EntityID: in.shop, EntityType: "business", DisplayName: "Example Workshop"},
		}}
}

func TestStartValidation(t *testing.T) {
	h := newTestHost(t, t.TempDir())
	in := newInstall(t)
	bad := []StartRequest{
		{},
		func() StartRequest { r := in.req("r"); r.InstallationID = "nope"; return r }(),
		func() StartRequest { r := in.req("r"); r.ServiceID = r.InstallationID; return r }(),
		func() StartRequest { r := in.req("r"); r.Entities[0].EntityType = "administrator"; return r }(),
		func() StartRequest { r := in.req("r"); r.Entities[0].EntityID = r.ServiceID; return r }(),
		func() StartRequest { r := in.req("r"); r.Entities[1].EntityID = r.Entities[0].EntityID; return r }(),
	}
	for i, r := range bad {
		if _, err := h.Start(r); err == nil {
			t.Errorf("case %d: expected validation error", i)
		}
	}
	if ws := h.Workspace(); ws.Started {
		t.Fatal("invalid requests must not start the service")
	}
}

func TestStartDedupReuseAndRefusals(t *testing.T) {
	dir := t.TempDir()
	h := newTestHost(t, dir)
	a := newInstall(t)
	res, _ := h.Start(a.req("req-1"))
	if res.Result != "started" || !res.Simulated {
		t.Fatalf("first start: %+v", res)
	}
	if res.Registrations[a.tass] != "registered" || res.Registrations[a.shop] != "registered" {
		t.Fatalf("registrations: %+v", res.Registrations)
	}
	if again, _ := h.Start(a.req("req-1")); again.Result != "duplicate_request" {
		t.Fatalf("same request id should be a duplicate, got %s", again.Result)
	}
	reuse, _ := h.Start(a.req("req-2"))
	if reuse.Result != "already_running" || reuse.Registrations[a.tass] != "already_registered" {
		t.Fatalf("second request should reuse: %+v", reuse)
	}
	b := newInstall(t)
	if busy, _ := h.Start(b.req("req-b")); busy.Result != "refused" || !strings.Contains(busy.Reason, "DIFFERENT installation") {
		t.Fatalf("other installation should be refused while A runs: %+v", busy)
	}
	wrong := a.req("req-3")
	wrong.ServiceID = mustUUID(t)
	if r, _ := h.Start(wrong); r.Result != "refused" {
		t.Fatalf("different service ID for the running installation must be refused: %+v", r)
	}
	c := h.Counters()
	if c.ServicesStarted != 1 || c.AlreadyRunning != 1 || c.DuplicateRequests != 1 || c.Refused != 2 || c.ModelCalls != 0 {
		t.Fatalf("counters: %+v", c)
	}
	// Version pin: a request approving another commit is refused.
	h.StopService("test")
	v := a.req("req-4")
	v.ApprovedCommit = "2222222222222222222222222222222222222222"
	if r, _ := h.Start(v); r.Result != "refused" || !strings.Contains(r.Reason, "version mismatch") {
		t.Fatalf("version mismatch should be refused: %+v", r)
	}
	// Re-binding the installation to a different service ID is refused even after stop.
	wrong.RequestID = "req-5"
	if r, _ := h.Start(wrong); r.Result != "refused" || !strings.Contains(r.Reason, "saved binding") {
		t.Fatalf("rebind should be refused: %+v", r)
	}
}

func TestThoughtsAreScopedToTheirEntity(t *testing.T) {
	h := newTestHost(t, t.TempDir())
	a := newInstall(t)
	h.Start(a.req("r1"))
	if l, _ := h.ListThoughts(a.shop); len(l) != 0 {
		t.Fatal("business should start empty")
	}
	th, _, err := h.CreateThought(a.tass, "TASS test thought", "active", "c1", "test")
	if err != nil {
		t.Fatal(err)
	}
	if !th.Simulated || th.WorkerCalls != 0 {
		t.Fatal("test thoughts must be SIMULATED with zero worker calls")
	}
	if l, _ := h.ListThoughts(a.shop); len(l) != 0 {
		t.Fatal("TASS thought leaked into the business")
	}
	if _, err := h.GetThought(a.shop, th.ThoughtID); err == nil {
		t.Fatal("reading TASS's thought through the business must fail")
	}
	if _, err := h.UpdateThought(a.shop, th.ThoughtID, "resolved", ""); err == nil {
		t.Fatal("changing TASS's thought through the business must fail")
	}
	if _, err := h.ListThoughts(mustUUID(t)); err == nil {
		t.Fatal("unregistered entity must be rejected, not defaulted")
	}
	same, repeat, _ := h.CreateThought(a.tass, "TASS test thought", "active", "c1", "test")
	if !repeat || same.ThoughtID != th.ThoughtID {
		t.Fatal("repeated create request must return the same thought")
	}
	bt, _, _ := h.CreateThought(a.shop, "Shop thought", "sleeping", "", "test")
	up, err := h.UpdateThought(a.tass, th.ThoughtID, "resolved", "done")
	if err != nil || up.State != "resolved" || len(up.Notes) != 1 || len(up.History) != 3 {
		t.Fatalf("update: %+v %v", up, err)
	}
	lt, _ := h.ListThoughts(a.tass)
	lb, _ := h.ListThoughts(a.shop)
	if len(lt) != 1 || len(lb) != 1 || lb[0].ThoughtID != bt.ThoughtID {
		t.Fatal("each entity must see only its own thought")
	}
}

func TestRestartRestoresAndSecondInstallationIsIsolated(t *testing.T) {
	dir := t.TempDir()
	a := newInstall(t)
	h1 := newTestHost(t, dir)
	h1.Start(a.req("r1"))
	th, _, _ := h1.CreateThought(a.tass, "Persisted", "sleeping", "", "test")
	h1.StopService("restart test")
	h1.Close()

	h2 := newTestHost(t, dir) // "Play" again on the same data folder
	if h2.Workspace().Started {
		t.Fatal("after a host restart the service must wait for a launcher request")
	}
	res, _ := h2.Start(a.req("r2"))
	if res.Result != "started" || res.Registrations[a.tass] != "already_registered" {
		t.Fatalf("restart: %+v", res)
	}
	got, err := h2.GetThought(a.tass, th.ThoughtID)
	if err != nil || got.Title != "Persisted" || got.State != "sleeping" || !got.CreatedAt.Equal(th.CreatedAt) {
		t.Fatalf("restored thought: %+v %v", got, err)
	}
	if ws := h2.Workspace(); ws.Binding.ServiceID != a.sid || ws.Binding.AdminID != a.aid {
		t.Fatal("binding changed across restart")
	}

	// Second, independent installation on the same host.
	h2.StopService("switch")
	b := newInstall(t)
	if r, _ := h2.Start(b.req("rb")); r.Result != "started" {
		t.Fatalf("B start: %+v", r)
	}
	ws := h2.Workspace()
	if ws.Binding.InstallationID != b.iid || len(ws.Entities) != 2 {
		t.Fatalf("B workspace: %+v", ws.Binding)
	}
	for _, e := range ws.Entities {
		if e.EntityID == a.tass || e.EntityID == a.shop || e.Total != 0 {
			t.Fatal("installation B must not see A's identities or thoughts")
		}
	}
	if _, err := h2.ListThoughts(a.tass); err == nil {
		t.Fatal("A's entity must not be reachable while B runs")
	}
	if _, err := os.Stat(filepath.Join(dir, "installations", a.iid, "entities", a.tass, "thoughts", th.ThoughtID+".json")); err != nil {
		t.Fatal("A's data must stay on disk untouched")
	}
}

func TestBrokenFilesAreReportedNotDeleted(t *testing.T) {
	dir := t.TempDir()
	a := newInstall(t)
	h := newTestHost(t, dir)
	h.Start(a.req("r1"))
	h.StopService("x")
	bad := filepath.Join(dir, "installations", a.iid, "entities", a.tass, "thoughts", "broken.json")
	os.WriteFile(bad, []byte("{not json"), 0o600)
	h.Start(a.req("r2"))
	if ws := h.Workspace(); len(ws.Problems) != 1 {
		t.Fatalf("expected 1 problem, got %+v", ws.Problems)
	}
	if _, err := os.Stat(bad); err != nil {
		t.Fatal("broken file must be left in place")
	}
}

func TestSampleSetCoversEveryState(t *testing.T) {
	for _, typ := range []string{"ai_companion", "business"} {
		seen := map[string]bool{}
		for _, s := range sampleSet(typ) {
			seen[s[0]] = true
		}
		for _, st := range thoughtStates {
			if !seen[st.Key] {
				t.Fatalf("%s sample set misses %s", typ, st.Key)
			}
		}
	}
}

func TestDataFolderRules(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	git := func(a ...string) (string, error) {
		switch a[len(a)-1] {
		case "--show-toplevel":
			return "/src/tc", nil
		case "HEAD":
			if a[0] == "rev-parse" && len(a) == 2 {
				return testCommit, nil
			}
			return "HEAD", nil
		}
		return "", nil
	}
	c, err := resolveConfig("", env(map[string]string{"PC_DATA": "/pc/x.data", "PC_BRANCH": "experiment/a", "PORT": "8123"}), git, "/home/u")
	if err != nil || c.DataDir != "/pc/x.data/thought-controller-first-user" || c.Branch != "experiment/a" || !c.Detached || c.Port != "8123" {
		t.Fatalf("PC config: %+v %v", c, err)
	}
	c, _ = resolveConfig("", env(nil), git, "/home/u")
	if !strings.HasPrefix(c.DataDir, "/home/u/.thought-controller-first-user/") {
		t.Fatalf("standalone default: %s", c.DataDir)
	}
	if strings.Contains(c.DataDir, "/.thought-controller/") {
		t.Fatal("must never default into the repaired V1 experiment's folder")
	}
	if _, err := resolveConfig("/src/tc/data", env(nil), git, "/home/u"); err == nil {
		t.Fatal("data inside the checkout must be refused")
	}
}
