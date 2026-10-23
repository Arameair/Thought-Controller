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
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	home, _ := os.UserHomeDir()
	dir := flag.String("data", filepath.Join(home, ".thought-controller"), "where state.json and trace.jsonl live")
	workerName := flag.String("worker", "mock", "starting worker: mock or chat-jimmy (switchable on the page)")
	flag.Parse()

	st, err := openStore(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "can't open data folder:", err)
		os.Exit(1)
	}
	workers := map[string]Worker{"mock": &mockWorker{delay: 40 * time.Millisecond}, "chat-jimmy": &jimmyWorker{}}
	wk, ok := workers[*workerName]
	if !ok {
		fmt.Fprintln(os.Stderr, "-worker must be mock or chat-jimmy")
		os.Exit(2)
	}
	port := firstSet(os.Getenv("PORT"), "7792")
	ln, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		fmt.Fprintln(os.Stderr, "can't listen:", err)
		os.Exit(1)
	}
	s := &server{mind: newMind(st), worker: wk, workers: workers, started: time.Now(), port: port}
	srv := &http.Server{Handler: s.routes(), ReadHeaderTimeout: 10 * time.Second}
	log("Thought Controller on http://127.0.0.1:%s/  (pid %d, worker %s, data %s, %d thoughts loaded)",
		port, os.Getpid(), wk.Name(), *dir, len(s.mind.Thoughts))

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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	st.close()
	log("stopped")
}

func log(f string, a ...any) {
	fmt.Printf("%s  %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(f, a...))
}
