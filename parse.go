package main

// The worker answers in short lines, one change per line. A fixed line
// format is far more reliable than JSON for a small fast model, and every
// line is checked before anything changes.
//
//	NEW: <question worth thinking about>
//	WAKE t-3: <why it matters again>
//	NOTE t-3: <short new insight>        (or NOTE: ... when working one thought)
//	RESOLVE t-3: <conclusion>
//	SLEEP t-3
//	SAY: <something worth telling the owner>
//	NOTHING
import (
	"regexp"
	"strconv"
	"strings"
)

type Proposal struct {
	Kind   string `json:"kind"` // new wake note resolve sleep say
	Target int    `json:"target,omitempty"`
	Text   string `json:"text,omitempty"`
	Line   string `json:"line"`
}

type Parsed struct {
	Proposals []Proposal `json:"proposals"`
	Nothing   bool       `json:"nothing"`
	Ignored   []string   `json:"ignored,omitempty"` // lines that matched no form
}

// Malformed: the reply contained no usable line at all.
func (p Parsed) Malformed() bool { return !p.Nothing && len(p.Proposals) == 0 }

var (
	// a command that appears after some chatter on the same line: needs a colon
	inlineRe = regexp.MustCompile(`(?i)\b(new|wake|note|resolve|say)\b\s*(?:(?:t|#|thought)\s*-?\s*\d+)?\s*:`)
	lineRe   = regexp.MustCompile(`(?i)^(new|wake|note|resolve|sleep|say|nothing)\b\s*(?:(?:t|#|thought)\s*-?\s*(\d+))?\s*[:\-–—]?\s*(.*)$`)
	bulletRe = regexp.MustCompile(`^\s*(?:[-*•>]+|\d+[.)])\s*`)
)

// parseReply reads a worker reply. focus is the thought being worked on (0
// for a heartbeat); NOTE/RESOLVE/SLEEP without an id apply to it.
func parseReply(reply string, focus int) Parsed {
	var out Parsed
	for _, raw := range strings.Split(strings.ReplaceAll(reply, "\r", ""), "\n") {
		l := strings.TrimSpace(raw)
		if l == "" || strings.HasPrefix(l, "```") {
			continue
		}
		l = bulletRe.ReplaceAllString(l, "")
		l = strings.NewReplacer("**", "", "__", "", "`", "").Replace(l)
		segs := segments(strings.TrimSpace(l))
		if len(segs) == 0 {
			out.Ignored = append(out.Ignored, raw)
			continue
		}
		for _, seg := range segs {
			parseSegment(seg, raw, focus, &out)
		}
	}
	return out
}

// segments splits a line at each command it contains, dropping chatter
// before the first one. "Let me think. NOTE: x RESOLVE: y" -> two segments.
func segments(l string) []string {
	if lineRe.MatchString(l) && !inlineRe.MatchString(l[min(len(l), 1):]) {
		return []string{l}
	}
	idx := inlineRe.FindAllStringIndex(l, -1)
	if len(idx) == 0 {
		if lineRe.MatchString(l) {
			return []string{l}
		}
		return nil
	}
	var out []string
	if idx[0][0] > 0 && lineRe.MatchString(l) {
		out = append(out, strings.TrimSpace(l[:idx[0][0]])) // e.g. "SLEEP t-2 NOTE: ..."
	}
	for i, x := range idx {
		end := len(l)
		if i+1 < len(idx) {
			end = idx[i+1][0]
		}
		out = append(out, strings.TrimSpace(l[x[0]:end]))
	}
	return out
}

func parseSegment(l, raw string, focus int, out *Parsed) {
	{
		m := lineRe.FindStringSubmatch(l)
		if m == nil {
			out.Ignored = append(out.Ignored, raw)
			return
		}
		kind := strings.ToLower(m[1])
		text := strings.TrimSpace(strings.Trim(strings.TrimSpace(m[3]), `"`))
		id := 0
		if m[2] != "" {
			id, _ = strconv.Atoi(m[2])
		}
		if kind == "nothing" {
			out.Nothing = true
			return
		}
		if id == 0 && kind != "new" && kind != "say" {
			id = focus
		}
		p := Proposal{Kind: kind, Target: id, Text: clip(text, 280), Line: l}
		switch kind {
		case "new", "say":
			if text == "" || strings.EqualFold(text, "nothing") || strings.EqualFold(text, "none") {
				return // "SAY: nothing" means stay quiet
			}
			p.Target = 0
		case "wake", "note", "resolve", "sleep":
			if id == 0 || (kind != "sleep" && kind != "wake" && text == "") {
				out.Ignored = append(out.Ignored, raw)
				return
			}
		}
		out.Proposals = append(out.Proposals, p)
	}
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
