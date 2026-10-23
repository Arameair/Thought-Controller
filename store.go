package main

// State lives in a data folder: state.json (the current mind, rewritten
// after each change) and trace.jsonl (every beat and owner action,
// append-only). Every write error is reported, never ignored.
import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// fileOps is the slice of the file system the store uses; tests replace it
// to simulate a full disk or a failed rename.
type fileOps interface {
	WriteFile(name string, b []byte) error
	Rename(from, to string) error
	OpenAppend(name string) (io.WriteCloser, error)
}

type osOps struct{}

func (osOps) WriteFile(n string, b []byte) error { return os.WriteFile(n, b, 0o600) }
func (osOps) Rename(a, b string) error           { return os.Rename(a, b) }
func (osOps) OpenAppend(n string) (io.WriteCloser, error) {
	return os.OpenFile(n, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
}

type Store struct {
	dir string
	ops fileOps
	mu  sync.Mutex
	tf  io.WriteCloser
}

func openStore(dir string) (*Store, error) { return openStoreWith(dir, osOps{}) }

func openStoreWith(dir string, ops fileOps) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	f, err := ops.OpenAppend(filepath.Join(dir, "trace.jsonl"))
	if err != nil {
		return nil, err
	}
	return &Store{dir: dir, ops: ops, tf: f}, nil
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

// save writes state.json via a temp file, so a failed write never
// damages the last good copy.
func (s *Store) save(m *Mind) error {
	b, err := json.MarshalIndent(m, "", " ")
	if err != nil {
		return err
	}
	p := filepath.Join(s.dir, "state.json")
	if err := s.ops.WriteFile(p+".tmp", b); err != nil {
		return err
	}
	return s.ops.Rename(p+".tmp", p)
}

func (s *Store) append(r BeatRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tf == nil { // e.g. a reset whose reopen failed: try again now
		f, err := s.ops.OpenAppend(filepath.Join(s.dir, "trace.jsonl"))
		if err != nil {
			return fmt.Errorf("trace file is not open: %w", err)
		}
		s.tf = f
	}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	_, err = s.tf.Write(append(b, '\n'))
	return err
}

// reset starts a new trace file and keeps the old one. If anything fails
// the old file stays in place and open.
func (s *Store) reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := filepath.Join(s.dir, "trace.jsonl")
	old := filepath.Join(s.dir, "trace-before-reset-"+nowStamp()+".jsonl")
	if s.tf != nil {
		s.tf.Close()
		s.tf = nil
	}
	if err := s.ops.Rename(p, old); err != nil && !errors.Is(err, os.ErrNotExist) {
		s.tf, _ = s.ops.OpenAppend(p)
		return err
	}
	f, err := s.ops.OpenAppend(p)
	if err != nil {
		_ = s.ops.Rename(old, p)
		s.tf, _ = s.ops.OpenAppend(p)
		return err
	}
	s.tf = f
	return nil
}

func (s *Store) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tf != nil {
		s.tf.Close()
		s.tf = nil
	}
}
