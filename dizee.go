// Package dizee shrinks noisy text (JSON, logs, plain text) before it is fed
// to an AI agent, so less of the context window is spent on formatting noise.
//
// Compression is deterministic and cheap: JSON is compacted without changing
// its meaning, log files collapse runs of identical lines, plain text drops
// trailing whitespace and repeated blank lines. If a transform does not make
// the input smaller, the original bytes are returned untouched.
package dizee

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Kind identifies the detected or forced content type.
type Kind string

// Supported content kinds.
const (
	KindJSON Kind = "json"
	KindLog  Kind = "log"
	KindText Kind = "text"
)

// Stats describes one shrink operation.
type Stats struct {
	Kind       Kind
	Compressed bool // false when the original bytes were kept
	Before     int
	After      int
}

// Result is the outcome of a Shrink call. Output is set only when
// Stats.Compressed is true; otherwise the caller should use the original bytes.
type Result struct {
	Stats  Stats
	Output []byte
}

// Saved returns the number of bytes removed. Zero when the original was kept.
func (r Result) Saved() int {
	return r.Stats.Before - r.Stats.After
}

// Detect classifies input as JSON, log or plain text. It never fails:
// unrecognized content falls back to text.
func Detect(b []byte) Kind {
	trimmed := bytes.TrimLeft(b, " \t\r\n")
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') && json.Valid(b) {
		return KindJSON
	}
	if isNDJSON(b) {
		return KindJSON
	}
	if isLog(b) {
		return KindLog
	}
	return KindText
}

// Shrink applies the transform for the detected (or forced) kind. When the
// transform does not produce smaller bytes, the original input is returned
// with Stats.Compressed set to false, so the output is never larger than
// the input.
func Shrink(b []byte, force Kind) (Result, error) {
	kind := force
	if kind == "" {
		kind = Detect(b)
	}
	var out []byte
	switch kind {
	case KindJSON:
		var err error
		out, err = shrinkJSON(b)
		if err != nil {
			return Result{}, fmt.Errorf("shrink json: %w", err)
		}
	case KindLog:
		out = shrinkLog(b)
	case KindText:
		out = shrinkText(b)
	default:
		return Result{}, fmt.Errorf("unknown kind %q (want json, log or text)", kind)
	}
	if len(out) >= len(b) {
		return Result{Stats: Stats{Kind: kind, Before: len(b), After: len(b)}}, nil
	}
	return Result{
		Stats:  Stats{Kind: kind, Compressed: true, Before: len(b), After: len(out)},
		Output: out,
	}, nil
}

var logLineRe = regexp.MustCompile(`(?m)^(?:` +
	`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}` + // ISO 8601-ish timestamps
	`|\d{4}/\d{2}/\d{2}[ T]\d{2}:\d{2}:\d{2}` + // slashed timestamps
	`|[A-Z][a-z]{2} +\d{1,2} \d{2}:\d{2}:\d{2}` + // syslog
	`|\[\d{2}:\d{2}:\d{2}(?:[.,]\d+)?\]` + // [hh:mm:ss(.fff)]
	`|\[\d{4}-\d{2}-\d{2}` + // [2026-01-02 ...
	`|\[(?i:info|warn|warning|error|err|debug|fatal|trace|notice|crit)\]` + // [LEVEL]
	`|(?i:info|warn|warning|error|err|debug|fatal|trace|notice|crit)\b` + // bare LEVEL
	`)`)

func isLog(b []byte) bool {
	lines := bytes.Split(b, []byte("\n"))
	if len(lines) < 3 {
		return false
	}
	nonEmpty, matches := 0, 0
	for _, l := range lines {
		if len(bytes.TrimSpace(l)) == 0 {
			continue
		}
		nonEmpty++
		if logLineRe.Match(l) {
			matches++
		}
	}
	return matches >= 3 && matches*2 >= nonEmpty
}

func isNDJSON(b []byte) bool {
	lines := bytes.Split(b, []byte("\n"))
	nonEmpty := 0
	for _, l := range lines {
		if len(bytes.TrimSpace(l)) == 0 {
			continue
		}
		if !json.Valid(l) {
			return false
		}
		nonEmpty++
	}
	return nonEmpty >= 2
}

// shrinkJSON compacts a single JSON document, or compacts NDJSON line by line.
func shrinkJSON(b []byte) ([]byte, error) {
	if json.Valid(b) {
		var buf bytes.Buffer
		if err := json.Compact(&buf, b); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}
	s := strings.TrimSuffix(string(b), "\n")
	hadTrailing := len(s) < len(b)
	lines := strings.Split(s, "\n")
	var buf bytes.Buffer
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			buf.WriteByte('\n')
			continue
		}
		if err := json.Compact(&buf, []byte(line)); err != nil {
			return nil, err
		}
		buf.WriteByte('\n')
	}
	out := buf.Bytes()
	if !hadTrailing && len(out) > 0 {
		out = bytes.TrimSuffix(out, []byte("\n"))
	}
	return out, nil
}

// shrinkLog collapses runs of consecutive identical lines into one line with
// an [xN] marker, and collapses runs of blank lines to a single blank line.
func shrinkLog(b []byte) []byte {
	s := strings.TrimSuffix(string(b), "\n")
	hadTrailing := len(s) < len(b)
	lines := strings.Split(s, "\n")
	var w strings.Builder
	prevBlank := false
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if line == "" {
			if !prevBlank {
				w.WriteByte('\n')
				prevBlank = true
			}
			continue
		}
		prevBlank = false
		n := 1
		for i+1 < len(lines) && lines[i+1] == line {
			i++
			n++
		}
		w.WriteString(line)
		if n > 1 {
			fmt.Fprintf(&w, " [x%d]", n)
		}
		w.WriteByte('\n')
	}
	out := w.String()
	if out != "" && !hadTrailing {
		out = strings.TrimSuffix(out, "\n")
	}
	return []byte(out)
}

// shrinkText trims trailing whitespace on every line and collapses runs of
// blank lines to a single blank line.
func shrinkText(b []byte) []byte {
	s := strings.TrimSuffix(string(b), "\n")
	hadTrailing := len(s) < len(b)
	lines := strings.Split(s, "\n")
	var w strings.Builder
	pendingBlank := false
	for _, line := range lines {
		line = strings.TrimRight(line, " \t\r")
		if line == "" {
			pendingBlank = true
			continue
		}
		if pendingBlank {
			w.WriteByte('\n')
			pendingBlank = false
		}
		w.WriteString(line)
		w.WriteByte('\n')
	}
	out := w.String()
	if out == "" {
		return []byte(out)
	}
	if !hadTrailing {
		out = strings.TrimSuffix(out, "\n")
	}
	return []byte(out)
}
