// Package dizee shrinks noisy text (JSON, logs, Go code, terminal output, plain text)
// before it is fed to an AI agent, so less of the context window is spent on
// formatting noise.
//
// Compression is deterministic and cheap: JSON is compacted without changing
// its meaning, log files collapse runs of identical lines, diffs drop the
// middle of long context runs, terminal output is stripped of ANSI escape
// sequences and redraw noise, plain text drops trailing whitespace and
// repeated blank lines. If a transform does not make the input smaller, the
// original bytes are returned untouched.
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
	KindJSON     Kind = "json"
	KindLog      Kind = "log"
	KindDiff     Kind = "diff"
	KindCode     Kind = "code"
	KindTerminal Kind = "terminal"
	KindText     Kind = "text"
)

// Stats describes one shrink operation.
type Stats struct {
	Kind       Kind
	Compressed bool // false when the original bytes were kept
	Before     int
	After      int
	Tokens     int // approximate tokens in the output, using bytes/4
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

// Detect classifies input as JSON, log, diff, terminal output or plain text.
// It never fails: unrecognized content falls back to text.
func Detect(b []byte) Kind {
	trimmed := bytes.TrimLeft(b, " \t\r\n")
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') && json.Valid(b) {
		return KindJSON
	}
	if isNDJSON(b) {
		return KindJSON
	}
	if isTerminal(b) {
		return KindTerminal
	}
	if isDiff(b) {
		return KindDiff
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
	case KindDiff:
		out = shrinkDiff(b, 1)
	case KindCode:
		var err error
		out, err = shrinkCode(b)
		if err != nil {
			return Result{}, fmt.Errorf("shrink code: %w", err)
		}
	case KindTerminal:
		out = shrinkTerminal(b)
	case KindText:
		out = shrinkText(b)
	default:
		return Result{}, fmt.Errorf("unknown kind %q (want json, log, diff, code, terminal or text)", kind)
	}
	return newResult(b, out, kind), nil
}

func newResult(b, out []byte, kind Kind) Result {
	if len(out) >= len(b) {
		return Result{Stats: Stats{
			Kind: kind, Before: len(b), After: len(b), Tokens: estimateTokens(len(b)),
		}}
	}
	return Result{
		Stats: Stats{
			Kind: kind, Compressed: true, Before: len(b), After: len(out), Tokens: estimateTokens(len(out)),
		},
		Output: out,
	}
}

func estimateTokens(n int) int {
	return (n + 3) / 4
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

// isTerminal reports whether b looks like a dump of terminal output. A few
// stray escape bytes are treated as noise in an ordinary text file, so at
// least three are required before committing to the terminal transform.
func isTerminal(b []byte) bool {
	return bytes.Count(b, []byte{0x1b}) >= 3
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

// shrinkTerminal strips ANSI escape sequences and carriage-return redraws
// from a terminal dump, then re-detects the cleaned content: stripped
// terminal output often turns out to be a log or a JSON document, which gets
// the corresponding transform applied on top.
func shrinkTerminal(b []byte) []byte {
	out := stripANSI(b)
	out = collapseCarriageReturns(out)
	switch {
	case json.Valid(out) || isNDJSON(out):
		if c, err := shrinkJSON(out); err == nil {
			out = c
		} else {
			out = shrinkText(out)
		}
	case isLog(out):
		out = shrinkLog(out)
	default:
		out = shrinkText(out)
	}
	return out
}

// stripANSI removes ANSI escape sequences: CSI sequences (SGR colors, cursor
// movement, erasing), OSC sequences (window titles, hyperlinks) and the
// remaining two- and three-byte escapes. Text content is kept byte for byte.
func stripANSI(b []byte) []byte {
	if bytes.IndexByte(b, 0x1b) < 0 {
		return b
	}
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); {
		if c := b[i]; c != 0x1b {
			out = append(out, c)
			i++
			continue
		}
		i = skipEscape(b, i)
	}
	return out
}

// skipEscape returns the index just past the escape sequence starting at i
// (where b[i] == ESC). Malformed sequences are skipped up to the first byte
// that breaks the grammar, so scanning always makes progress.
func skipEscape(b []byte, i int) int {
	if i+1 >= len(b) {
		return len(b) // trailing ESC at end of input
	}
	switch b[i+1] {
	case '[': // CSI: ESC [ params(0x30-0x3F) intermediates(0x20-0x2F) final(0x40-0x7E)
		j := i + 2
		for j < len(b) && b[j] >= 0x20 && b[j] <= 0x3f {
			j++
		}
		if j < len(b) && b[j] >= 0x40 && b[j] <= 0x7e {
			return j + 1
		}
		return j
	case ']': // OSC: ESC ] ... (BEL | ESC \). An OSC payload never contains
		// \n, so a line break means the sequence is truncated — stop there
		// instead of swallowing the rest of the input. The cap guards the
		// unterminated case.
		const oscLimit = 4096
		j := i + 2
		for j < len(b) && j-i <= oscLimit {
			switch {
			case b[j] == 0x07:
				return j + 1
			case b[j] == 0x1b && j+1 < len(b) && b[j+1] == '\\':
				return j + 2
			case b[j] == '\n':
				return j
			}
			j++
		}
		return j
	default: // two- or three-byte escape: ESC intermediates(0x20-0x2F) final(0x30-0x7E)
		j := i + 1
		for j < len(b) && b[j] >= 0x20 && b[j] <= 0x2f {
			j++
		}
		if j < len(b) && b[j] >= 0x30 && b[j] <= 0x7e {
			return j + 1
		}
		return j
	}
}

// collapseCarriageReturns keeps only the text after the last carriage return
// within each line, which turns progress redraws ("10%\r20%\r100%") into the
// final visible line. A \r right before \n is a CRLF line break, not a redraw.
func collapseCarriageReturns(b []byte) []byte {
	if bytes.IndexByte(b, '\r') < 0 {
		return b
	}
	lines := bytes.Split(b, []byte("\n"))
	for i, line := range lines {
		line = bytes.TrimSuffix(line, []byte("\r"))
		if j := bytes.LastIndexByte(line, '\r'); j >= 0 {
			line = line[j+1:]
		}
		lines[i] = line
	}
	return bytes.Join(lines, []byte("\n"))
}
