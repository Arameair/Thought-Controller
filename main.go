// Thought Controller, first version: owner-triggered thoughts with a
// visible lifecycle, run through a mock worker or an already-running
// Chat-Jimmy. Nothing runs on its own; nothing is sent anywhere.
package main

import (
	"context"
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
	"syscall"
	"time"
)

func main() {
	home, _ := os.UserHomeDir()
	branch, commit := gitIdentity()
	dir := flag.String("data", "", "data folder (default: $THOUGHT_CONTROLLER_DATA, else ~/.thought-controller/branches/<git branch>)")
	workerName := flag.String("worker", "mock", "starting worker: mock, mock-slow or chat-jimmy (switchable on the page)")
	traceText := flag.Bool("trace-text", true, "keep full prompts and replies in the trace (false = sizes only)")
	flag.Parse()
	if *dir == "" {
		*dir = firstSet(os.Getenv("THOUGHT_CONTROLLER_DATA"), filepath.Join(home, ".thought-controller", "branches", safeName(branch)))
	}

	st, err := openStore(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "can't open data folder:", err)
		os.Exit(1)
	}
	workers := map[string]Worker{
		"mock":       &mockWorker{delay: 40 * time.Millisecond},
		"mock-slow":  &mockWorker{name: "mock-slow", delay: 4 * time.Second}, // for testing STALE and Stop by hand
		"chat-jimmy": &jimmyWorker{},
	}
	wk, ok := workers[*workerName]
	if !ok {
		fmt.Fprintln(os.Stderr, "-worker must be mock, mock-slow or chat-jimmy")
		os.Exit(2)
	}
	port := firstSet(os.Getenv("PORT"), "7792")
	ln, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		fmt.Fprintln(os.Stderr, "can't listen:", err)
		os.Exit(1)
	}
	s := &server{mind: newMind(st), worker: wk, workers: workers, started: time.Now(), port: port,
		dataDir: *dir, branch: branch, commit: commit}
	s.mind.traceText = *traceText
	srv := &http.Server{Handler: s.routes(), ReadHeaderTimeout: 10 * time.Second}
	log("Thought Controller on http://127.0.0.1:%s/  (pid %d, worker %s, branch %s @ %s, data %s, %d thoughts loaded)",
		port, os.Getpid(), wk.Name(), branch, short(commit), *dir, len(s.mind.Thoughts))

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	select {
	case sig := <-stop:
		log("stopping (%v)", sig)
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			log("server error: %v", err)
		}
	}
	s.run.stop()
	s.mind.CancelBeat()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	st.close()
	log("stopped")
}

func log(f string, a ...any) {
	fmt.Printf("%s  %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(f, a...))
}

// gitIdentity reads the checkout Project-controller runs us from.
func gitIdentity() (branch, commit string) {
	branch, commit = "default", "unknown"
	if v := os.Getenv("THOUGHT_CONTROLLER_BRANCH"); v != "" {
		branch = v
	} else if b, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil {
		if v := strings.TrimSpace(string(b)); v != "" && v != "HEAD" {
			branch = v
		}
	}
	if b, err := exec.Command("git", "rev-parse", "HEAD").Output(); err == nil {
		commit = strings.TrimSpace(string(b))
	}
	return branch, commit
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
	if b.Len() == 0 {
		return "default"
	}
	return b.String()
}

func short(c string) string {
	if len(c) > 8 {
		return c[:8]
	}
	return c
}
