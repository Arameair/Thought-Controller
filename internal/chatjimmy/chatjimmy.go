// Package chatjimmy talks to a running Chat-Jimmy worker (Arameair/chat-jimmy).
//
// It has no dependencies outside the standard library: copy this one file
// into any Go project (e.g. internal/chatjimmy/chatjimmy.go).
//
//	jim, err := chatjimmy.Find()            // reads ~/.chat-jimmy/worker.json, or $CHAT_JIMMY_URL
//	if err != nil { ... }                   // worker not running
//	r, err := jim.Ask(ctx, "Explain DHCP in one sentence.")
//	fmt.Println(r.Text, r.Took)
//
// Rules the worker enforces: one question at a time (ErrBusy if another is
// running — nothing was sent), a repeated request ID returns the first answer
// without sending again, and an uncertain send is never retried for you.
package chatjimmy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

var (
	ErrNotRunning = errors.New("chat-jimmy worker is not running")
	ErrBusy       = errors.New("chat-jimmy is answering another question (nothing was sent)")
	ErrBlocked    = errors.New("chatjimmy.ai is showing a challenge or limit page")
	ErrUncertain  = errors.New("question may or may not have been sent; not retried")
)

// Client is a connection to one worker.
type Client struct {
	URL    string // e.g. http://127.0.0.1:7791
	Token  string // only if the worker was started with CHAT_JIMMY_TOKEN
	Source string // shows up in the worker's metrics, e.g. "tass"
	HTTP   *http.Client
}

// Reply is one answer.
type Reply struct {
	Text      string        // the answer
	Took      time.Duration // total time, request in -> answer out
	FirstText time.Duration // send -> first answer text seen (0 if unknown)
	SiteTPS   float64       // ChatJimmy's own tokens/sec figure (0 if not shown)
	RequestID string
	Simulated bool // true when the worker runs in -mock mode
}

// Find locates the worker from $CHAT_JIMMY_URL or ~/.chat-jimmy/worker.json
// and checks it answers. Source defaults to the program name.
func Find() (*Client, error) {
	c := &Client{URL: strings.TrimRight(os.Getenv("CHAT_JIMMY_URL"), "/"), Token: os.Getenv("CHAT_JIMMY_TOKEN"),
		Source: filepath.Base(os.Args[0]), HTTP: &http.Client{Timeout: 2 * time.Minute}}
	if c.URL == "" {
		home, _ := os.UserHomeDir()
		b, err := os.ReadFile(filepath.Join(home, ".chat-jimmy", "worker.json"))
		if err != nil {
			return nil, ErrNotRunning
		}
		var w struct {
			URL string `json:"url"`
		}
		if json.Unmarshal(b, &w) != nil || w.URL == "" {
			return nil, ErrNotRunning
		}
		c.URL = w.URL
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := c.Heartbeat(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

// Heartbeat reports whether the worker is up and its chat page is ready.
// It never sends anything to ChatJimmy.
func (c *Client) Heartbeat(ctx context.Context) (ready bool, err error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", c.URL+"/api/heartbeat", nil)
	res, err := c.http().Do(req)
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrNotRunning, err)
	}
	defer res.Body.Close()
	var hb struct {
		OK    bool   `json:"ok"`
		State string `json:"state"`
	}
	if err := json.NewDecoder(res.Body).Decode(&hb); err != nil {
		return false, fmt.Errorf("%w: unexpected heartbeat reply", ErrNotRunning)
	}
	return hb.OK, nil
}

var seq atomic.Int64

// Ask sends one question in the current conversation.
func (c *Client) Ask(ctx context.Context, message string) (Reply, error) {
	return c.ask(ctx, message, false)
}

// AskFresh starts a new conversation first (no earlier context). The page
// reload adds roughly a second.
func (c *Client) AskFresh(ctx context.Context, message string) (Reply, error) {
	return c.ask(ctx, message, true)
}

func (c *Client) ask(ctx context.Context, message string, fresh bool) (Reply, error) {
	id := fmt.Sprintf("%s-%d-%d", c.source(), time.Now().UnixNano(), seq.Add(1))
	body, _ := json.Marshal(map[string]any{"schema": "chat-jimmy.request.v0", "request_id": id,
		"source": c.source(), "message": message, "new_chat": fresh})
	req, err := http.NewRequestWithContext(ctx, "POST", c.URL+"/api/ask", bytes.NewReader(body))
	if err != nil {
		return Reply{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("X-Chat-Jimmy-Token", c.Token)
	}
	res, err := c.http().Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return Reply{RequestID: id}, fmt.Errorf("%w: %v", ErrUncertain, err)
		}
		return Reply{RequestID: id}, fmt.Errorf("%w: %v", ErrNotRunning, err)
	}
	defer res.Body.Close()
	var out struct {
		Status    string  `json:"status"`
		Response  string  `json:"response"`
		Simulated bool    `json:"simulated"`
		Error     *string `json:"error_message"`
		Metrics   struct {
			WallMS      int64    `json:"wall_ms"`
			FirstTextMS *int64   `json:"first_text_ms"`
			TPS         *float64 `json:"tokens_per_sec"`
		} `json:"metrics"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return Reply{RequestID: id}, fmt.Errorf("unreadable reply from worker (HTTP %d): %w", res.StatusCode, err)
	}
	r := Reply{Text: out.Response, Took: time.Duration(out.Metrics.WallMS) * time.Millisecond, RequestID: id, Simulated: out.Simulated}
	if out.Metrics.FirstTextMS != nil {
		r.FirstText = time.Duration(*out.Metrics.FirstTextMS) * time.Millisecond
	}
	if out.Metrics.TPS != nil {
		r.SiteTPS = *out.Metrics.TPS
	}
	msg := out.Status
	if out.Error != nil {
		msg = *out.Error
	}
	switch out.Status {
	case "success":
		return r, nil
	case "busy":
		return r, ErrBusy
	case "blocked":
		return r, fmt.Errorf("%w: %s", ErrBlocked, msg)
	case "uncertain", "timeout":
		return r, fmt.Errorf("%w: %s", ErrUncertain, msg)
	case "unavailable":
		return r, fmt.Errorf("%w: %s", ErrNotRunning, msg)
	}
	return r, fmt.Errorf("chat-jimmy: %s", msg)
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func (c *Client) source() string {
	if c.Source != "" {
		return c.Source
	}
	return "client"
}
