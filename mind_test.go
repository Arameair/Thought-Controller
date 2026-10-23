package main

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestParseReply(t *testing.T) {
	reply := "Sure! Here are my thoughts:\n" +
		"1. NEW: Why does Goldshire keep mattering?\n" +
		"- **WAKE t-3**: the owner is back in that zone\n" +
		"NOTE T3: kobolds drop candles\n" +
		"* resolve #4 - it was the quest\n" +
		"SLEEP t-5\n" +
		"SAY: nothing\n" +
		"SAY: You have 4 candles left to find.\n" +
		"NOTE: (no id, heartbeat → ignored)\n" +
		"```"
	p := parseReply(reply, 0)
	kinds := []string{}
	for _, x := range p.Proposals {
		kinds = append(kinds, x.Kind+":"+itoa(x.Target))
	}
	want := "new:0 wake:3 note:3 resolve:4 sleep:5 say:0"
	if strings.Join(kinds, " ") != want {
		t.Fatalf("got %v want %s", kinds, want)
	}
	if p.Proposals[3].Text != "it was the quest" || p.Proposals[1].Text != "the owner is back in that zone" {
		t.Fatalf("texts: %+v", p.Proposals)
	}
	if len(p.Ignored) != 2 { // the chatty first line and the id-less NOTE
		t.Fatalf("ignored %q", p.Ignored)
	}
	// focus fills in the id when working one thought
	w := parseReply("NOTE: candles are in the mine\nRESOLVE: go to the mine", 7)
	if len(w.Proposals) != 2 || w.Proposals[0].Target != 7 || w.Proposals[1].Target != 7 {
		t.Fatalf("focus: %+v", w.Proposals)
	}
	if n := parseReply("NOTHING", 0); !n.Nothing || n.Malformed() {
		t.Fatal("NOTHING")
	}
	in := parseReply("Let me think about this. NOTE: the owner has been here before. RESOLVE: it's the quest hub", 4)
	if len(in.Proposals) != 2 || in.Proposals[0].Kind != "note" || in.Proposals[0].Text != "the owner has been here before." || in.Proposals[1].Target != 4 {
		t.Fatalf("inline: %+v", in.Proposals)
	}
	if x := parseReply("I have nothing new to say about notes today.", 0); !x.Malformed() {
		t.Fatalf("prose with keywords but no colon must not parse: %+v", x)
	}
	if g := parseReply("I think the owner is happy today.", 0); !g.Malformed() {
		t.Fatal("prose should be malformed")
	}
}

func itoa(i int) string { return strconv.Itoa(i) }

// scripted worker: returns canned replies in order, records prompts.
type scripted struct {
	mu       sync.Mutex
	replies  []string
	prompts  []string
	hold     chan struct{} // if set, Ask waits for it (or for cancel)
	err      error
	notReady bool
}

func (s *scripted) Name() string    { return "scripted" }
func (s *scripted) Simulated() bool { return true }
func (s *scripted) Ready(context.Context) Readiness {
	return Readiness{OK: !s.notReady, Source: "scripted", Simulated: true, Detail: "scripted"}
}
func (s *scripted) Ask(ctx context.Context, p string, fresh bool) (WorkerReply, error) {
	if s.hold != nil {
		select {
		case <-s.hold:
		case <-ctx.Done():
			// like a real worker that was already asked: the reply still comes back
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompts = append(s.prompts, p)
	if s.err != nil {
		return WorkerReply{}, s.err
	}
	r := "NOTHING"
	if len(s.replies) > 0 {
		r, s.replies = s.replies[0], s.replies[1:]
	}
	return WorkerReply{Text: r, Took: 5 * time.Millisecond, RequestID: "req", Simulated: true}, ctx.Err()
}

func states(m *Mind) string {
	var b []string
	for _, t := range m.Thoughts {
		b = append(b, "t"+itoa(t.ID)+"="+t.State)
	}
	return strings.Join(b, " ")
}

func TestHeartbeatThenWorkThenResolve(t *testing.T) {
	m := newMind(nil)
	w := &scripted{replies: []string{
		"NEW: Why does Goldshire keep mattering?",
		"NOTE: the owner returns there often\nNEW: What quest is active there?",
		"RESOLVE: candles are in the mine",
		"RESOLVE: it is the anchor zone for questing",
	}}
	if _, err := m.Step(context.Background(), w); !errors.Is(err, ErrNothing) {
		t.Fatalf("empty mind must not call the worker: %v", err)
	}
	m.AddEvent("I entered Goldshire again.", "owner")
	r, err := m.Step(context.Background(), w)
	if err != nil || r.Kind != "heartbeat" || len(r.Applied) != 1 || states(m) != "t1=active" {
		t.Fatalf("heartbeat: %+v %v %s", r, err, states(m))
	}
	if !strings.Contains(w.prompts[0], "I entered Goldshire again.") {
		t.Fatal("event not in prompt")
	}
	r, _ = m.Step(context.Background(), w) // works t-1, which spawns child t-2
	if r.Kind != "work" || r.Focus != 1 || m.byID(2) == nil || m.byID(2).Parent != 1 || m.byID(2).Depth != 1 {
		t.Fatalf("work: %+v %+v", r, m.Thoughts)
	}
	r, _ = m.Step(context.Background(), w) // child t-2 has never been worked -> it goes first
	if r.Focus != 2 || m.byID(2).State != stResolved {
		t.Fatalf("expected t-2 worked and resolved: %+v %s", r, states(m))
	}
	m.Step(context.Background(), w)
	if states(m) != "t1=resolved t2=resolved" {
		t.Fatal(states(m))
	}
	if _, err := m.Step(context.Background(), w); !errors.Is(err, ErrNothing) {
		t.Fatal("all resolved: nothing to do")
	}
	if len(w.prompts) != 4 {
		t.Fatalf("one call per beat: %d", len(w.prompts))
	}
}

func TestBudgetsAndValidation(t *testing.T) {
	m := newMind(nil)
	m.Settings.MaxActive, m.Settings.MaxNewPerBeat = 3, 2
	m.AddEvent("lots happened", "owner")
	w := &scripted{replies: []string{"NEW: Alpha question about cats\nNEW: Beta question about dogs\nNEW: Gamma question about birds\nNOTE t-9: ghost\nWAKE t-1: hm"}}
	r, _ := m.Step(context.Background(), w)
	if len(r.Applied) != 2 || len(r.Rejected) != 3 {
		t.Fatalf("applied %v rejected %v", r.Applied, r.Rejected)
	}
	m.AddEvent("more", "owner")
	w.replies = []string{"NEW: alpha question about cats?\nNEW: Delta question about fish\nNEW: Epsilon question about cows"}
	r, _ = m.Step(context.Background(), w)
	joined := strings.Join(r.Rejected, " | ")
	if !strings.Contains(joined, "duplicate of t-1") || !strings.Contains(joined, "active thoughts (limit)") || m.count(stActive) != 3 {
		t.Fatalf("rejected %v, active %d", r.Rejected, m.count(stActive))
	}
}

func TestDepthLimitAndWorkBudget(t *testing.T) {
	m := newMind(nil)
	m.Settings.MaxDepth, m.Settings.MaxWorkPerThought = 1, 3
	m.AddThought("Top question")
	w := &scripted{replies: []string{
		"NOTE: a\nNEW: child question one",
		"NOTE: b\nNEW: grandchild question two", // working child t-2 -> depth 2 > 1
		"NOTE: c", "NOTE: d", "NOTE: e", "NOTE: f",
	}}
	m.Step(context.Background(), w)
	r, _ := m.Step(context.Background(), w)
	if r.Focus != 2 || !strings.Contains(strings.Join(r.Rejected, ""), "too deep") {
		t.Fatalf("%+v", r)
	}
	for i := 0; i < 4; i++ {
		m.Step(context.Background(), w)
	}
	if m.byID(1).State != stDormant || m.byID(2).State != stDormant {
		t.Fatalf("work budget should put both to sleep: %s", states(m))
	}
}

func TestStaleReplyIsNotApplied(t *testing.T) {
	m := newMind(nil)
	m.AddThought("Should we build the feature?")
	w := &scripted{replies: []string{"RESOLVE: yes, build it"}, hold: make(chan struct{})}
	done := make(chan BeatRecord)
	go func() { r, _ := m.Step(context.Background(), w); done <- r }()
	time.Sleep(50 * time.Millisecond)
	if _, err := m.Step(context.Background(), w); !errors.Is(err, ErrBusy) {
		t.Fatalf("second beat while one is running: %v", err)
	}
	m.AddEvent("I no longer want the feature.", "owner") // arrives while thinking
	close(w.hold)
	r := <-done
	if !r.Stale || len(r.Applied) != 0 || m.byID(1).State != stActive {
		t.Fatalf("stale reply applied: %+v state %s", r, m.byID(1).State)
	}
	// next beat is a heartbeat that includes the new event
	w.hold = nil
	w.replies = []string{"NOTE t-1: owner dropped the feature\nRESOLVE t-1: no, owner changed their mind"}
	r, _ = m.Step(context.Background(), w)
	if r.Kind != "heartbeat" || m.byID(1).State != stResolved || !strings.Contains(w.prompts[len(w.prompts)-1], "no longer want") {
		t.Fatalf("%+v %s", r, states(m))
	}
}

func TestIdleDormantExpireAndWake(t *testing.T) {
	m := newMind(nil)
	m.Settings.DormantAfter, m.Settings.ExpireAfter, m.Settings.MaxActive = 2, 5, 1
	m.AddThought("Where are the candles in Goldshire?")
	m.SetState(1, stDormant, "")
	m.AddThought("Unrelated question about lunch")
	w := &scripted{}
	for i := 0; i < 6; i++ {
		w.replies = append(w.replies, "NOTE: still thinking about lunch")
	}
	m.Step(context.Background(), w)
	if m.byID(1).State != stDormant {
		t.Fatal(states(m))
	}
	m.AddEvent("Back in Goldshire with candles on screen", "owner")
	w.replies = []string{"WAKE t-1: candles are visible"}
	r, _ := m.Step(context.Background(), w)
	if !strings.Contains(strings.Join(r.Rejected, ""), "limit") {
		t.Fatalf("max active 1 should block the wake: %+v", r)
	}
	m.SetState(2, stResolved, "sandwich")
	m.AddEvent("Candles again", "owner")
	w.replies = []string{"WAKE t-1: candles are visible"}
	m.Step(context.Background(), w)
	if m.byID(1).State != stActive || m.byID(1).Activations != 1 {
		t.Fatalf("wake: %s", states(m))
	}
	// idle aging: a heartbeat that does nothing, repeatedly
	for i := 0; i < 6; i++ {
		m.AddEvent("noise "+itoa(i), "owner")
		w.replies = []string{"NOTHING"}
		m.Step(context.Background(), w)
	}
	if m.byID(1).State != stExpired {
		t.Fatalf("expected expired after idling: %s", states(m))
	}
}

func TestWorkerErrorsAreRecordedNotRetried(t *testing.T) {
	m := newMind(nil)
	m.AddThought("q one")
	w := &scripted{err: errors.New("boom")}
	r, err := m.Step(context.Background(), w)
	if err == nil || r.ErrorCat != errOther || len(w.prompts) != 1 {
		t.Fatalf("%+v %v", r, err)
	}
	if mt := m.metrics(); mt.Calls != 1 || mt.Errors["error"] != 1 {
		t.Fatalf("%+v", mt)
	}
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	st, _ := openStore(dir)
	m := newMind(st)
	m.AddEvent("hello", "owner")
	m.Step(context.Background(), &scripted{replies: []string{"NEW: What does hello mean?"}})
	st.close()
	st2, _ := openStore(dir)
	m2 := newMind(st2)
	defer st2.close()
	if len(m2.Thoughts) != 1 || m2.Beat != 1 || m2.LastSeenRev != 1 || len(m2.trace) != 2 || m2.NextThought != 2 {
		t.Fatalf("reload: %+v trace %d", m2, len(m2.trace))
	}
	m2.Reset()
	if len(m2.Thoughts) != 0 || m2.Beat != 0 {
		t.Fatal("reset")
	}
}

func TestMockWorkerLoop(t *testing.T) {
	m := newMind(nil)
	w := &mockWorker{}
	m.AddEvent("I entered Goldshire", "owner")
	for i := 0; i < 20; i++ {
		if _, err := m.Step(context.Background(), w); err != nil {
			if errors.Is(err, ErrNothing) {
				break
			}
			t.Fatal(err)
		}
	}
	if m.count(stActive) != 0 || m.count(stResolved) < 2 {
		t.Fatalf("mock run should settle: %s", states(m))
	}
	m.SetState(1, stDormant, "")
	m.byID(1).State = stDormant
	m.AddEvent("Goldshire again", "owner")
	r, _ := m.Step(context.Background(), w)
	if !strings.Contains(strings.Join(r.Applied, ""), "woke t-1") {
		t.Fatalf("mock should wake the related thought: %+v", r)
	}
}
