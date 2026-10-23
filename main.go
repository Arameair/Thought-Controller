// Thought Controller — first new-administrator prototype.
//
// This process is a SIMULATED HOST. It serves the identity-organized thought
// dashboard and a clearly labeled, unauthenticated simulated lifecycle /
// handshake that Identity's simulated launcher talks to. It never launches
// other programs, never calls an AI model, never contacts GitHub and never
// thinks on its own. Every thought it holds is created by an owner test
// button and marked SIMULATED.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	appVersion = "tc-first-user-v0.1"
	protocol   = "tc-sim-handshake/0.1"
	refRepo    = "Arameair/Thought-Controller"
)

// Config is everything the host knows about how it was started.
type Config struct {
	DataDir     string `json:"data_dir"`
	DataSource  string `json:"data_source"`
	Branch      string `json:"branch"`
	BranchFrom  string `json:"branch_source"`
	Detached    bool   `json:"detached"`
	Commit      string `json:"commit"`
	Port        string `json:"port"`
	Version     string `json:"version"`
	CheckoutDir string `json:"checkout_dir,omitempty"`
}

// resolveConfig: -data flag, TC_FIRST_USER_DATA, Project Controller's PC_DATA,
// then a per-branch test folder. The names deliberately differ from the
// repaired V1 experiment (THOUGHT_CONTROLLER_DATA, ~/.thought-controller) so
// this prototype can never open that experiment's state.
func resolveConfig(flagData string, env func(string) string, git func(...string) (string, error), home string) (Config, error) {
	c := Config{Version: appVersion, Port: firstSet(env("PORT"), "7797"), Commit: "unknown"}
	if top, err := git("rev-parse", "--show-toplevel"); err == nil {
		c.CheckoutDir = top
	}
	if sha, err := git("rev-parse", "HEAD"); err == nil {
		c.Commit = sha
	}
	head, _ := git("rev-parse", "--abbrev-ref", "HEAD")
	c.Detached = head == "HEAD"
	switch {
	case env("PC_BRANCH") != "":
		c.Branch, c.BranchFrom = env("PC_BRANCH"), "PC_BRANCH"
	case head != "" && head != "HEAD":
		c.Branch, c.BranchFrom = head, "git"
	default:
		c.Branch, c.BranchFrom = "unknown", "unknown"
	}
	switch {
	case flagData != "":
		c.DataDir, c.DataSource = flagData, "-data flag"
	case env("TC_FIRST_USER_DATA") != "":
		c.DataDir, c.DataSource = env("TC_FIRST_USER_DATA"), "TC_FIRST_USER_DATA"
	case env("PC_DATA") != "":
		c.DataDir, c.DataSource = filepath.Join(env("PC_DATA"), "thought-controller-first-user"), "PC_DATA"
	default:
		key := c.Branch
		if key == "unknown" {
			key = "commit-" + short(c.Commit)
		}
		c.DataDir, c.DataSource = filepath.Join(home, ".thought-controller-first-user", safeName(key)), "standalone default (per branch)"
	}
	abs, err := filepath.Abs(c.DataDir)
	if err != nil {
		return c, err
	}
	c.DataDir = abs
	if c.CheckoutDir != "" && within(abs, c.CheckoutDir) {
		return c, fmt.Errorf("data folder %s is inside the code checkout %s — test data must live outside the repository; set -data, TC_FIRST_USER_DATA or PC_DATA", abs, c.CheckoutDir)
	}
	return c, nil
}

func within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func gitCmd(args ...string) (string, error) {
	out, err := exec.Command("git", args...).Output()
	return strings.TrimSpace(string(out)), err
}

// ---------- events ----------

type Event struct {
	At     time.Time `json:"at"`
	Kind   string    `json:"kind"`
	Detail string    `json:"detail"`
	ID     string    `json:"id,omitempty"`
}

// EventLog keeps recent events in memory and appends every event to a file
// (host events.jsonl, or one per installation). Write errors are kept and
// shown, never ignored.
type EventLog struct {
	mu   sync.Mutex
	list []Event
	f    *os.File
	err  string
	echo bool
}

func openEventLog(path string, echo bool) *EventLog {
	l := &EventLog{echo: echo}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		l.err = "event log not writable: " + err.Error()
	} else {
		l.f = f
	}
	return l
}

func (l *EventLog) Add(kind, detail, id string) {
	e := Event{At: time.Now().UTC(), Kind: kind, Detail: detail, ID: id}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.list = append(l.list, e)
	if len(l.list) > 200 {
		l.list = l.list[len(l.list)-200:]
	}
	if l.f != nil {
		b, _ := json.Marshal(e)
		if _, err := l.f.Write(append(b, '\n')); err != nil {
			l.err = "event log write failed: " + err.Error()
		}
	}
	if l.echo {
		idPart := ""
		if id != "" {
			idPart = " id=" + id
		}
		fmt.Printf("%s  %-10s %s%s\n", time.Now().Format("15:04:05"), kind, detail, idPart)
	}
}

func (l *EventLog) Recent(n int) []Event {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.list) > n {
		return append([]Event{}, l.list[len(l.list)-n:]...)
	}
	return append([]Event{}, l.list...)
}

func (l *EventLog) Err() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.err
}

func (l *EventLog) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f != nil {
		l.f.Close()
		l.f = nil
	}
}

// ---------- main ----------

func main() {
	dataFlag := flag.String("data", "", "data folder (overrides TC_FIRST_USER_DATA and PC_DATA)")
	flag.Parse()
	home, _ := os.UserHomeDir()
	cfg, err := resolveConfig(*dataFlag, os.Getenv, gitCmd, home)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if err := run(cfg, nil, nil); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// run serves until SIGINT/SIGTERM (or until stop is closed, in tests).
// ready, if not nil, receives the listening address.
func run(cfg Config, stop <-chan struct{}, ready chan<- string) error {
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return fmt.Errorf("can't create data folder %s: %w", cfg.DataDir, err)
	}
	lock, err := acquireLock(cfg.DataDir, cfg.Port)
	if err != nil {
		return err
	}
	defer lock.release()
	events := openEventLog(filepath.Join(cfg.DataDir, "host-events.jsonl"), true)
	defer events.Close()
	host, err := openHost(cfg, events)
	if err != nil {
		return err
	}
	defer host.Close()

	ln, err := net.Listen("tcp", "127.0.0.1:"+cfg.Port)
	if err != nil {
		return fmt.Errorf("can't listen on 127.0.0.1:%s: %w", cfg.Port, err)
	}
	cfg.Port = fmt.Sprint(ln.Addr().(*net.TCPAddr).Port)
	host.cfg.Port = cfg.Port
	events.Add("start", fmt.Sprintf("Thought Controller %s SIMULATED HOST on http://127.0.0.1:%s/ (pid %d) · branch %s (%s%s) · commit %s · data %s (%s) · controller service NOT STARTED until Identity's simulated launcher asks",
		appVersion, cfg.Port, os.Getpid(), cfg.Branch, cfg.BranchFrom, map[bool]string{true: ", detached checkout", false: ""}[cfg.Detached],
		cfg.Commit, cfg.DataDir, cfg.DataSource), "")

	s := &server{host: host}
	srv := &http.Server{Handler: s.routes(), ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	if ready != nil {
		ready <- "127.0.0.1:" + cfg.Port
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)
	reason := ""
	select {
	case v := <-sig:
		reason = "signal " + v.String()
	case <-stop:
		reason = "stop requested"
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			reason = "server error: " + err.Error()
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	host.StopService("host stopping (" + reason + ")")
	events.Add("stop", "host stopped cleanly ("+reason+"); all saves were already on disk; no child processes exist", "")
	return nil
}

func firstSet(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func short(c string) string {
	if len(c) > 12 {
		return c[:12]
	}
	return c
}

func safeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '.' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 || b.String() == "." || b.String() == ".." {
		return "default"
	}
	return b.String()
}
