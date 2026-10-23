package main

// Run = up to N beats in a row, started by the owner, stopped by the owner,
// a budget, an error, or running out of things to think about.
import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type runState struct {
	Running  bool       `json:"running"`
	Started  *time.Time `json:"started,omitempty"`
	Finished *time.Time `json:"finished,omitempty"`
	Planned  int        `json:"planned"`
	Done     int        `json:"done"`
	Calls    int        `json:"calls"`
	Stopped  string     `json:"stopped,omitempty"`
}

type runner struct {
	mu     sync.Mutex
	st     runState
	cancel context.CancelFunc
}

func (r *runner) snapshot() runState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.st
}

func (r *runner) start(m *Mind, w Worker, n int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.st.Running {
		return errors.New("a run is already in progress")
	}
	m.mu.Lock()
	limit, gap := m.Settings.MaxRunBeats, time.Duration(m.Settings.GapMS)*time.Millisecond
	m.mu.Unlock()
	if n < 1 {
		n = 1
	}
	if n > limit {
		n = limit
	}
	ctx, cancel := context.WithCancel(context.Background())
	now := time.Now().UTC()
	r.st, r.cancel = runState{Running: true, Started: &now, Planned: n}, cancel
	go func() {
		stopped := fmt.Sprintf("finished %d beats", n)
		for i := 0; i < n; i++ {
			rec, err := m.Step(ctx, w)
			r.mu.Lock()
			if rec.Beat > 0 {
				r.st.Done++
				if rec.Stages != nil && rec.Stages.Sent != "no" {
					r.st.Calls++
				}
			}
			r.mu.Unlock()
			switch {
			case errors.Is(err, ErrNothing):
				stopped = "nothing left to think about"
			case errors.Is(err, ErrCancelled):
				stopped = "stopped by owner"
			case errors.Is(err, ErrUnreadable):
				stopped = "stopped: heartbeat reply was unreadable — events still pending; press Think once to retry"
			case err != nil:
				stopped = "stopped on error: " + err.Error()
			case ctx.Err() != nil:
				stopped = "stopped by owner"
			}
			if err != nil || ctx.Err() != nil {
				break
			}
			if i < n-1 {
				select {
				case <-ctx.Done():
				case <-time.After(gap):
				}
				if ctx.Err() != nil {
					stopped = "stopped by owner"
					break
				}
			}
		}
		r.mu.Lock()
		end := time.Now().UTC()
		r.st.Running, r.st.Finished, r.st.Stopped = false, &end, stopped
		r.mu.Unlock()
	}()
	return nil
}

func (r *runner) stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		r.cancel()
	}
}

// ---------- metrics ----------

type Metrics struct {
	Beats         int            `json:"beats"`
	Calls         int            `json:"calls"`
	OK            int            `json:"ok"`
	Errors        map[string]int `json:"errors"`
	Malformed     int            `json:"malformed"`
	Stale         int            `json:"stale"`
	Heartbeats    int            `json:"heartbeats"`
	WorkBeats     int            `json:"work_beats"`
	MedianMS      *int64         `json:"median_ms"`
	P95MS         *int64         `json:"p95_ms"`
	Applied       int            `json:"applied"`
	Rejected      int            `json:"rejected"`
	RejectReasons map[string]int `json:"reject_reasons"`
	Says          int            `json:"says"`
	Thoughts      map[string]int `json:"thoughts"`
	Created       int            `json:"created"`
	Simulated     int            `json:"simulated_calls"`
	Baselines     int            `json:"baselines"`
	Outcomes      map[string]int `json:"outcomes"`
	NotSent       int            `json:"not_sent"`
}

func (m *Mind) metrics() Metrics {
	mt := Metrics{Errors: map[string]int{}, RejectReasons: map[string]int{}, Thoughts: map[string]int{}, Outcomes: map[string]int{}}
	var ms []int64
	for _, r := range m.trace {
		if r.Kind != "heartbeat" && r.Kind != "work" && r.Kind != "baseline" {
			continue
		}
		if r.Outcome != "" {
			mt.Outcomes[r.Outcome]++
		}
		if r.Stages != nil && r.Stages.Sent == "no" {
			mt.NotSent++
			if r.ErrorCat != "" {
				mt.Errors[r.ErrorCat]++
			}
			continue // nothing reached the worker
		}
		mt.Calls++
		if r.Kind == "baseline" {
			mt.Baselines++
			if r.Simulated {
				mt.Simulated++
			}
			if r.ErrorCat == "" {
				ms = append(ms, r.TookMS)
			}
			continue
		}
		if r.Simulated {
			mt.Simulated++
		}
		if r.Kind == "heartbeat" {
			mt.Heartbeats++
		} else {
			mt.WorkBeats++
		}
		if r.ErrorCat != "" {
			mt.Errors[r.ErrorCat]++
			continue
		}
		mt.OK++
		ms = append(ms, r.TookMS)
		if r.Malformed {
			mt.Malformed++
		}
		if r.Stale {
			mt.Stale++
		}
		mt.Applied += len(r.Applied)
		for _, a := range r.Applied {
			if len(a) > 8 && a[:8] == "created " {
				mt.Created++
			}
		}
		if !r.Stale {
			mt.Rejected += len(r.Rejected)
			for _, x := range r.Rejected {
				mt.RejectReasons[rejectReason(x)]++
			}
		}
	}
	mt.Beats = m.Beat
	sort.Slice(ms, func(i, j int) bool { return ms[i] < ms[j] })
	if len(ms) > 0 {
		md := ms[len(ms)/2]
		p := ms[(len(ms)*95+99)/100-1]
		mt.MedianMS, mt.P95MS = &md, &p
	}
	mt.Says = len(m.Says)
	for _, t := range m.Thoughts {
		mt.Thoughts[t.State]++
	}
	return mt
}

func rejectReason(x string) string {
	for _, k := range []string{"duplicate", "limit", "too deep", "no thought", "not dormant", "only active", "already", "not active"} {
		if contains(x, k) {
			return k
		}
	}
	return "other"
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }

func nowStamp() string { return time.Now().UTC().Format("20060102-150405") }
