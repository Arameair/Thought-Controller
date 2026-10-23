package main

// State lives in ~/.thought-controller/: state.json (the current mind,
// rewritten after each change) and trace.jsonl (every beat and owner
// action, append-only). Both survive restarts.
import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type Store struct {
	dir string
	mu  sync.Mutex
	tf  *os.File
}

func openStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "trace.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return &Store{dir: dir, tf: f}, nil
}

func (s *Store) load(m *Mind) {
	if b, err := os.ReadFile(filepath.Join(s.dir, "state.json")); err == nil {
		var saved Mind
		if json.Unmarshal(b, &saved) == nil {
			m.Events, m.Thoughts, m.Says = saved.Events, saved.Thoughts, saved.Says
			m.Beat, m.Rev, m.LastSeenRev = saved.Beat, saved.Rev, saved.LastSeenRev
			m.NextThought, m.NextEvent = max(saved.NextThought, 1), max(saved.NextEvent, 1)
			if saved.Settings.MaxActive > 0 {
				m.Settings = saved.Settings
			}
		}
	}
	f, err := os.Open(filepath.Join(s.dir, "trace.jsonl"))
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 8<<20)
	for sc.Scan() {
		var r BeatRecord
		if json.Unmarshal(sc.Bytes(), &r) == nil {
			m.trace = append(m.trace, r)
			if len(m.trace) > 2000 {
				m.trace = m.trace[1:]
			}
		}
	}
}

func (s *Store) save(m *Mind) {
	b, err := json.MarshalIndent(m, "", " ")
	if err != nil {
		return
	}
	p := filepath.Join(s.dir, "state.json")
	if os.WriteFile(p+".tmp", b, 0o600) == nil {
		_ = os.Rename(p+".tmp", p)
	}
}

func (s *Store) append(r BeatRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b, err := json.Marshal(r); err == nil {
		_, _ = s.tf.Write(append(b, '\n'))
	}
}

// reset starts a new trace file; the old one is kept with a timestamp.
func (s *Store) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := filepath.Join(s.dir, "trace.jsonl")
	s.tf.Close()
	_ = os.Rename(p, filepath.Join(s.dir, "trace-before-reset-"+nowStamp()+".jsonl"))
	s.tf, _ = os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
}

func (s *Store) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tf != nil {
		s.tf.Close()
	}
}
