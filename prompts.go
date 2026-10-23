package main

import (
	"fmt"
	"strings"
)

// Prompts are short and self-contained: the worker may start a fresh chat
// at any time, so everything it needs is in the message.

const formsHeartbeat = `Reply with 1 to 4 lines, each in exactly one of these forms, and nothing else:
NEW: <a short question worth thinking about>
WAKE t-<number>: <why that sleeping thought matters again>
NOTE t-<number>: <a short new insight for that thought>
RESOLVE t-<number>: <its conclusion>
SLEEP t-<number>
SAY: <something worth telling the owner now, only if important>
If nothing deserves attention, reply with the single word: NOTHING`

func heartbeatPrompt(events []Event, thoughts []*Thought, identity string) string {
	var b strings.Builder
	b.WriteString("You are the heartbeat of a companion's mind. Read what just happened and the companion's current thoughts, then propose small changes. Keep every line short.\n")
	if identity != "" {
		fmt.Fprintf(&b, "\nAbout the companion: %s\n", identity)
	}
	b.WriteString("\nWhat just happened (oldest first):\n")
	for _, e := range events {
		fmt.Fprintf(&b, "- %s\n", oneLine(e.Text))
	}
	writeThoughts(&b, thoughts)
	b.WriteString("\n" + formsHeartbeat)
	return b.String()
}

const formsWork = `Reply with 1 to 3 lines, each in exactly one of these forms, and nothing else:
NOTE: <one short new insight or next step>
RESOLVE: <the conclusion, if you can reach one now>
SLEEP   (if this needs information that isn't available yet)
NEW: <a short follow-up question, only if truly needed>
SAY: <something worth telling the owner now, only if important>`

func workPrompt(t *Thought, events []Event, others []*Thought, identity string) string {
	var b strings.Builder
	b.WriteString("You are developing one thought for a companion. Move it one small step forward. Keep every line short.\n")
	if identity != "" {
		fmt.Fprintf(&b, "\nAbout the companion: %s\n", identity)
	}
	fmt.Fprintf(&b, "\nThe thought (t-%d): %s\n", t.ID, oneLine(t.Question))
	if len(t.Notes) > 0 {
		b.WriteString("Notes so far:\n")
		for _, n := range lastNotes(t.Notes, 4) {
			fmt.Fprintf(&b, "- %s\n", oneLine(n.Text))
		}
	}
	if len(events) > 0 {
		b.WriteString("\nRecent events (oldest first):\n")
		for _, e := range events {
			fmt.Fprintf(&b, "- %s\n", oneLine(e.Text))
		}
	}
	if len(others) > 0 {
		b.WriteString("\nOther thoughts (for context only):\n")
		for _, o := range others {
			fmt.Fprintf(&b, "- t-%d (%s): %s\n", o.ID, o.State, oneLine(o.Question))
		}
	}
	b.WriteString("\n" + formsWork)
	return b.String()
}

func writeThoughts(b *strings.Builder, thoughts []*Thought) {
	if len(thoughts) == 0 {
		b.WriteString("\nCurrent thoughts: none yet.\n")
		return
	}
	b.WriteString("\nCurrent thoughts:\n")
	for _, t := range thoughts {
		fmt.Fprintf(b, "- t-%d (%s): %s", t.ID, t.State, oneLine(t.Question))
		if n := lastNotes(t.Notes, 1); len(n) == 1 {
			fmt.Fprintf(b, " — latest note: %s", oneLine(n[0].Text))
		}
		b.WriteString("\n")
	}
}

func lastNotes(n []Note, k int) []Note {
	if len(n) > k {
		return n[len(n)-k:]
	}
	return n
}

func oneLine(s string) string { return clip(strings.Join(strings.Fields(s), " "), 240) }
