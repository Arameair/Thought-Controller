// Thought Controller — the developer dashboard for Thought activity.
//
// Thought Controller does not think and does not invent data. It reads the
// real Sparks, Traces and Thoughts that a running Thought engine
// (Arameair/Thought) has produced, organizes them by identity, and passes
// the owner's Sleep / Wake / Resolve / Reopen requests back to that engine,
// which enforces its own per-identity permission. It never launches
// programs, never calls an AI model and never contacts GitHub. Its only
// outbound requests go to the Thought engine address on this computer that
// the owner configures (loopback only).
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
	appVersion = "tc-dashboard-v0.2"
	refRepo    = "Arameair/Thought-Controller"
)

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

// resolveConfig: -data flag, TC_DATA, Project Controller's PC_DATA, then a
// per-branch folder outside the checkout. The names differ from the repaired
// V1 experiment (THOUGHT_CONTROLLER_DATA, ~/.thought-controller/...) so its
// state is never opened.
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
	case env("TC_DATA") != "":
		c.DataDir, c.DataSource = env("TC_DATA"), "TC_DATA"
	case env("PC_DATA") != "":
		c.DataDir, c.DataSource = filepath.Join(env("PC_DATA"), "thought-controller"), "PC_DATA"
	default:
		key := c.Branch
		if key == "unknown" {
			key = "commit-" + short(c.Commit)
		}
		c.DataDir, c.DataSource = filepath.Join(home, ".thought-controller-dashboard", safeName(key)), "standalone default (per branch)"
	}
	abs, err := filepath.Abs(c.DataDir)
	if err != nil {
		return c, err
	}
	c.DataDir = abs
	if c.CheckoutDir != "" && within(abs, c.CheckoutDir) {
		return c, fmt.Errorf("data folder %s is inside the code checkout %s — data must live outside the repository; set -data, TC_DATA or PC_DATA", abs, c.CheckoutDir)
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

// ---------- activity log ----------

type Event struct {
	At     time.Time `json:"at"`
	Kind   string    `json:"kind"`
	Detail string    `json:"detail"`
}

// EventLog keeps recent lines in memory (including the tail of earlier runs)
// and appends every line to events.jsonl and to stdout (the Project
// Controller log pane). Write errors are kept and shown.
type EventLog struct {
	mu   sync.Mutex
	list []Event
	f    *os.File
	err  string
	echo bool
}

func openEventLog(path string, echo bool) *EventLog {
	l := &EventLog{echo: echo}
	if b, err := os.ReadFile(path); err == nil {
		lines := strings.Split(strings.TrimSpace(string(b)), "\n")
		if len(lines) > 200 {
			lines = lines[len(lines)-200:]
		}
		for _, ln := range lines {
			var e Event
			if json.Unmarshal([]byte(ln), &e) == nil && !e.At.IsZero() {
				l.list = append(l.list, e)
			}
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		l.err = "activity log not writable: " + err.Error()
	} else {
		l.f = f
	}
	return l
}

func (l *EventLog) Add(kind, detail string) {
	e := Event{At: time.Now().UTC(), Kind: kind, Detail: detail}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.list = append(l.list, e)
	if len(l.list) > 200 {
		l.list = l.list[len(l.list)-200:]
	}
	if l.f != nil {
		b, _ := json.Marshal(e)
		if _, err := l.f.Write(append(b, '\n')); err != nil {
			l.err = "activity log write failed: " + err.Error()
		}
	}
	if l.echo {
		fmt.Printf("%s  %-9s %s\n", time.Now().Format("15:04:05"), kind, detail)
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
	dataFlag := flag.String("data", "", "data folder (overrides TC_DATA and PC_DATA)")
	thoughtAddr := flag.String("thought", "", "Thought engine address, e.g. 127.0.0.1:7798 (saved; overrides THOUGHT_ADDR)")
	flag.Parse()
	home, _ := os.UserHomeDir()
	cfg, err := resolveConfig(*dataFlag, os.Getenv, gitCmd, home)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if err := run(cfg, firstSet(*thoughtAddr, os.Getenv("THOUGHT_ADDR")), nil, nil); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// run serves until SIGINT/SIGTERM (or until stop is closed, in tests).
func run(cfg Config, thoughtAddr string, stop <-chan struct{}, ready chan<- string) error {
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return fmt.Errorf("can't create data folder %s: %w", cfg.DataDir, err)
	}
	lock, err := acquireLock(cfg.DataDir, cfg.Port)
	if err != nil {
		return err
	}
	defer lock.release()
	ln, err := net.Listen("tcp", "127.0.0.1:"+cfg.Port)
	if err != nil {
		return fmt.Errorf("can't listen on 127.0.0.1:%s: %w", cfg.Port, err)
	}
	cfg.Port = fmt.Sprint(ln.Addr().(*net.TCPAddr).Port)
	events := openEventLog(filepath.Join(cfg.DataDir, "events.jsonl"), true)
	defer events.Close()
	app, err := openApp(cfg, events, thoughtAddr)
	if err != nil {
		ln.Close()
		return err
	}
	events.Add("start", fmt.Sprintf("Thought Controller %s on http://127.0.0.1:%s/ (pid %d) · instance %s · branch %s (%s%s) · commit %s · data %s (%s) · Thought engine address %s",
		appVersion, cfg.Port, os.Getpid(), app.inst.InstanceID, cfg.Branch, cfg.BranchFrom, map[bool]string{true: ", detached checkout", false: ""}[cfg.Detached],
		cfg.Commit, cfg.DataDir, cfg.DataSource, firstSet(app.Settings().ThoughtAddress, "(not set)")))
	go app.src.Probe("startup")

	srv := &http.Server{Handler: (&server{app: app}).routes(), ReadHeaderTimeout: 10 * time.Second}
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
	events.Add("stop", "stopped cleanly ("+reason+"); no child processes exist; thought data lives in the Thought engine, not here")
	return nil
}

func firstSet(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
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
