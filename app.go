package main

// Thought Controller's own small, durable state:
//
//	<data>/instance.json   this installation's Thought Controller instance ID (made once)
//	<data>/settings.json   where the Thought engine is (loopback address)
//	<data>/events.jsonl    activity log
//
// Thought data (Sparks, Traces, Thoughts, grants) is NOT stored here. It
// belongs to the Thought engine, which is the single source of truth.
import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrInvalid  = errors.New("invalid request")
	ErrNotFound = errors.New("not found")
	uuidRe      = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
)

type Instance struct {
	SchemaVersion int       `json:"schema_version"`
	InstanceID    string    `json:"instance_id"`
	CreatedAt     time.Time `json:"created_at"`
	Note          string    `json:"note"`
}

type Settings struct {
	SchemaVersion  int    `json:"schema_version"`
	ThoughtAddress string `json:"thought_address"`
}

type App struct {
	mu       sync.Mutex
	cfg      Config
	events   *EventLog
	inst     Instance
	settings Settings
	problems []string
	src      *Source
	started  time.Time
}

func openApp(cfg Config, events *EventLog, thoughtAddr string) (*App, error) {
	a := &App{cfg: cfg, events: events, started: time.Now()}
	ipath := filepath.Join(cfg.DataDir, "instance.json")
	switch err := readJSONStrict(ipath, &a.inst); {
	case errors.Is(err, os.ErrNotExist):
		a.inst = Instance{SchemaVersion: 1, InstanceID: newUUID(), CreatedAt: time.Now().UTC(),
			Note: "This Thought Controller's own instance ID, made locally. Not yet registered with Identity (no authenticated Identity handshake exists)."}
		if err := writeJSONAtomic(ipath, a.inst); err != nil {
			return nil, fmt.Errorf("instance ID NOT saved: %w", err)
		}
		events.Add("instance", "created this Thought Controller's instance ID "+a.inst.InstanceID)
	case err != nil:
		return nil, fmt.Errorf("instance.json can't be read (%v) — left untouched; fix or move it, or use another data folder", err)
	case !uuidRe.MatchString(a.inst.InstanceID):
		return nil, fmt.Errorf("instance.json has an invalid instance_id — left untouched; use another data folder")
	}
	a.settings = Settings{SchemaVersion: 1, ThoughtAddress: "127.0.0.1:7798"}
	if err := readJSONStrict(filepath.Join(cfg.DataDir, "settings.json"), &a.settings); err != nil && !errors.Is(err, os.ErrNotExist) {
		a.problems = append(a.problems, "settings.json can't be read ("+err.Error()+"); using the default Thought address; file left untouched")
		events.Add("problem", a.problems[len(a.problems)-1])
	}
	if thoughtAddr != "" {
		addr, err := validAddress(thoughtAddr)
		if err != nil {
			return nil, fmt.Errorf("-thought / THOUGHT_ADDR: %w", err)
		}
		if addr != a.settings.ThoughtAddress {
			a.settings.ThoughtAddress = addr
			if err := writeJSONAtomic(filepath.Join(cfg.DataDir, "settings.json"), a.settings); err != nil {
				return nil, fmt.Errorf("Thought address NOT saved: %w", err)
			}
		}
	}
	a.src = newSource(a.settings.ThoughtAddress, events)
	return a, nil
}

func (a *App) Settings() Settings {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.settings
}

// SetThoughtAddress saves a new loopback address and re-checks the engine.
func (a *App) SetThoughtAddress(addr string) error {
	addr, err := validAddress(addr)
	if err != nil {
		return err
	}
	a.mu.Lock()
	next := a.settings
	next.ThoughtAddress = addr
	if err := writeJSONAtomic(filepath.Join(a.cfg.DataDir, "settings.json"), next); err != nil {
		a.mu.Unlock()
		return fmt.Errorf("NOT saved: %w", err)
	}
	a.settings = next
	a.mu.Unlock()
	a.events.Add("source", "Thought engine address set to "+addr)
	a.src.SetAddress(addr)
	a.src.Probe("address changed")
	return nil
}

// validAddress: loopback host:port only.
func validAddress(addr string) (string, error) {
	addr = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(addr, "http://"), "https://"))
	addr = strings.TrimSuffix(addr, "/")
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("%w: use host:port, e.g. 127.0.0.1:7798", ErrInvalid)
	}
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return "", fmt.Errorf("%w: only addresses on this computer (127.0.0.1 / localhost) are allowed", ErrInvalid)
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return "", fmt.Errorf("%w: port must be 1–65535", ErrInvalid)
	}
	return net.JoinHostPort(host, port), nil
}

// ---------- small helpers ----------

func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// writeJSONAtomic: temp file, flush, rename, then read back and compare.
func writeJSONAtomic(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	f, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	ok := false
	defer func() {
		if !ok {
			os.Remove(tmp)
		}
	}()
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	ok = true
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, b) {
		return fmt.Errorf("could not verify the save after writing (%v)", err)
	}
	return nil
}

func readJSONStrict(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("not a valid record: %v", err)
	}
	return nil
}
