package main

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"thought-controller/internal/chatjimmy"
)

// Worker answers one prompt. The controller never runs two at once.
type Worker interface {
	Name() string
	Simulated() bool
	// Ready checks availability without sending a prompt.
	Ready(ctx context.Context) (bool, string)
	Ask(ctx context.Context, prompt string, fresh bool) (WorkerReply, error)
}

type WorkerReply struct {
	Text      string
	Took      time.Duration
	FirstText time.Duration
	RequestID string
	SiteTPS   float64
}

// Error categories recorded in the trace.
const (
	errBusy      = "busy"
	errBlocked   = "blocked"
	errUncertain = "uncertain"
	errNotReady  = "unavailable"
	errOther     = "error"
)

func errCategory(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, chatjimmy.ErrBusy):
		return errBusy
	case errors.Is(err, chatjimmy.ErrBlocked):
		return errBlocked
	case errors.Is(err, chatjimmy.ErrUncertain):
		return errUncertain
	case errors.Is(err, chatjimmy.ErrNotRunning):
		return errNotReady
	}
	return errOther
}

// ---------- Chat-Jimmy (an already-running worker; never started or stopped here) ----------

type jimmyWorker struct {
	mu  sync.Mutex
	cli *chatjimmy.Client
}

func (j *jimmyWorker) Name() string    { return "chat-jimmy" }
func (j *jimmyWorker) Simulated() bool { return false }

func (j *jimmyWorker) client() (*chatjimmy.Client, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.cli != nil {
		return j.cli, nil
	}
	c, err := chatjimmy.Find()
	if err != nil {
		return nil, err
	}
	c.Source = "thought-controller"
	j.cli = c
	return c, nil
}

func (j *jimmyWorker) Ready(ctx context.Context) (bool, string) {
	c, err := j.client()
	if err != nil {
		return false, "Chat-Jimmy not found — start it in Project-controller (looks for ~/.chat-jimmy/worker.json or $CHAT_JIMMY_URL)"
	}
	ok, err := c.Heartbeat(ctx)
	if err != nil {
		j.mu.Lock()
		j.cli = nil // the worker may have moved to another port; look again next time
		j.mu.Unlock()
		return false, "Chat-Jimmy not answering at " + c.URL
	}
	if !ok {
		return false, "Chat-Jimmy is running but its page isn't ready"
	}
	return true, "Chat-Jimmy ready at " + c.URL
}

func (j *jimmyWorker) Ask(ctx context.Context, prompt string, fresh bool) (WorkerReply, error) {
	c, err := j.client()
	if err != nil {
		return WorkerReply{}, err
	}
	ask := c.Ask
	if fresh {
		ask = c.AskFresh
	}
	r, err := ask(ctx, prompt)
	return WorkerReply{Text: r.Text, Took: r.Took, FirstText: r.FirstText, RequestID: r.RequestID, SiteTPS: r.SiteTPS}, err
}

// ---------- Mock: deterministic, local, always marked SIMULATED ----------

type mockWorker struct{ delay time.Duration }

func (m *mockWorker) Name() string    { return "mock" }
func (m *mockWorker) Simulated() bool { return true }
func (m *mockWorker) Ready(context.Context) (bool, string) {
	return true, "Mock worker (SIMULATED — no model involved)"
}

var (
	mockThoughtRe = regexp.MustCompile(`(?m)^- t-(\d+) \((\w+)\): (.*?)(?: — latest note: .*)?$`)
	mockFocusRe   = regexp.MustCompile(`(?m)^The thought \(t-(\d+)\): (.*)$`)
	mockNotesRe   = regexp.MustCompile(`(?m)^Notes so far:\n((?:- .*\n)+)`)
	mockEventsRe  = regexp.MustCompile(`(?s)What just happened \(oldest first\):\n(.*?)\n\n`)
)

// Ask follows simple rules so controller behaviour is predictable in tests:
// heartbeat -> wake a sleeping thought that shares a word with an event,
// otherwise one NEW question per event (max 2); work -> two NOTEs, then
// RESOLVE; the first step of a top-level thought also asks one follow-up.
func (m *mockWorker) Ask(ctx context.Context, prompt string, fresh bool) (WorkerReply, error) {
	t0 := time.Now()
	select {
	case <-ctx.Done():
		return WorkerReply{}, ctx.Err()
	case <-time.After(m.delay):
	}
	var lines []string
	if f := mockFocusRe.FindStringSubmatch(prompt); f != nil {
		notes := 0
		if n := mockNotesRe.FindStringSubmatch(prompt); n != nil {
			notes = strings.Count(n[1], "\n")
		}
		q := strings.TrimSpace(f[2])
		switch {
		case notes >= 2:
			lines = append(lines, "RESOLVE: (mock) settled: "+clip(q, 60))
		default:
			lines = append(lines, fmt.Sprintf("NOTE: (mock) step %d on: %s", notes+1, clip(q, 60)))
			if notes == 0 && !strings.HasPrefix(q, "Follow-up") {
				lines = append(lines, "NEW: Follow-up to t-"+f[1]+": what would change this?")
			}
		}
	} else if e := mockEventsRe.FindStringSubmatch(prompt); e != nil {
		events := strings.Split(strings.TrimSpace(e[1]), "\n")
		woke := map[string]bool{}
		for _, t := range mockThoughtRe.FindAllStringSubmatch(prompt, -1) {
			if t[2] != "dormant" {
				continue
			}
			for _, ev := range events {
				if sharesWord(ev, t[3]) && !woke[t[1]] {
					lines = append(lines, "WAKE t-"+t[1]+": (mock) related to: "+clip(strings.TrimPrefix(ev, "- "), 50))
					woke[t[1]] = true
				}
			}
		}
		if len(woke) == 0 {
			for i, ev := range events {
				if i == 2 {
					break
				}
				lines = append(lines, "NEW: What does this mean: "+clip(strings.TrimPrefix(ev, "- "), 80)+"?")
			}
		}
	}
	if len(lines) == 0 {
		lines = []string{"NOTHING"}
	}
	return WorkerReply{Text: strings.Join(lines, "\n"), Took: time.Since(t0), RequestID: fmt.Sprintf("mock-%d", time.Now().UnixNano())}, nil
}

var stop = map[string]bool{"the": true, "a": true, "an": true, "and": true, "or": true, "to": true, "of": true, "in": true,
	"is": true, "it": true, "i": true, "my": true, "what": true, "why": true, "does": true, "this": true, "that": true,
	"for": true, "on": true, "at": true, "mean": true, "keep": true, "again": true, "be": true, "do": true, "you": true}

func words(s string) []string {
	var out []string
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	}) {
		if len(w) > 2 && !stop[w] {
			out = append(out, w)
		}
	}
	return out
}

func sharesWord(a, b string) bool {
	set := map[string]bool{}
	for _, w := range words(a) {
		set[w] = true
	}
	for _, w := range words(b) {
		if set[w] {
			return true
		}
	}
	return false
}
