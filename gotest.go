package dizee

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	goTestRunRe    = regexp.MustCompile(`^\s*=== (?:RUN|PAUSE|CONT)\s+\S`)
	goTestResultRe = regexp.MustCompile(`^\s*--- (PASS|FAIL|SKIP):\s+(.+?)(?:\s+\([\d.]+s\))?\s*$`)
	goTestPkgRe    = regexp.MustCompile(`^(?:ok|FAIL|\?)\s+\S`)
)

// isGoTest reports whether b looks like `go test` output: verbose
// `=== RUN`/`--- PASS|FAIL|SKIP` lines or failing test results, plus at least
// one package summary line (`ok pkg 0.5s`, `FAIL pkg 0.1s`, `? pkg [no test
// files]`). The thresholds are deliberately conservative so that plain text
// mentioning a test or two is not rewritten.
func isGoTest(b []byte) bool {
	s := string(b)
	runs, results, pkgs := 0, 0, 0
	for _, line := range strings.Split(s, "\n") {
		switch {
		case goTestRunRe.MatchString(line):
			runs++
		case goTestResultRe.MatchString(line):
			results++
		case goTestPkgRe.MatchString(line):
			pkgs++
		}
	}
	return pkgs >= 1 && (runs >= 1 || results >= 1)
}

// shrinkGoTest collapses verbose passing Go test output. Package summary lines
// are always kept. Failing and skipped tests keep every line of their output,
// including panics and stack traces, together with the === RUN lines of their
// open ancestors for context. Passed tests collapse into a single
// [xN passed] marker together with their === RUN/PAUSE/CONT lines and any log
// lines emitted while they ran.
func shrinkGoTest(b []byte) []byte {
	s := strings.TrimSuffix(string(b), "\n")
	hadTrailing := len(s) < len(b)
	lines := strings.Split(s, "\n")
	var w strings.Builder
	var pending []string
	runPos := make(map[string]int)
	passed := 0
	keep := false // after a failure everything is kept verbatim until the next package line
	flushPassed := func() {
		if passed > 0 {
			fmt.Fprintf(&w, "[x%d passed]\n", passed)
			passed = 0
		}
	}
	flushPending := func() {
		for _, l := range pending {
			w.WriteString(l)
			w.WriteByte('\n')
		}
		pending = nil
		for name := range runPos {
			delete(runPos, name)
		}
	}
	for _, line := range lines {
		if goTestRunRe.MatchString(line) {
			if name, ok := goTestRunName(line); ok {
				runPos[name] = len(pending)
			}
			pending = append(pending, line)
			continue
		}
		if m := goTestResultRe.FindStringSubmatch(line); m != nil {
			if m[1] == "PASS" {
				passed++
				if p, ok := runPos[m[2]]; ok && p <= len(pending) {
					pending = pending[:p]
				}
				continue
			}
			flushPassed()
			flushPending()
			keep = true
			w.WriteString(line)
			w.WriteByte('\n')
			continue
		}
		switch {
		case goTestPkgRe.MatchString(line):
			flushPassed()
			flushPending()
			keep = false
			w.WriteString(line)
			w.WriteByte('\n')
		case line == "PASS":
			// bare verbose pass marker; the ok package line carries the verdict
		case line == "FAIL":
			flushPassed()
			flushPending()
			keep = true
			w.WriteString(line)
			w.WriteByte('\n')
		case keep:
			w.WriteString(line)
			w.WriteByte('\n')
		default:
			pending = append(pending, line)
		}
	}
	flushPassed()
	flushPending()
	return []byte(finishTestLines(w.String(), hadTrailing))
}

// goTestRunName extracts the test name from a `=== RUN|PAUSE|CONT name` line.
func goTestRunName(line string) (string, bool) {
	trimmed := strings.TrimLeft(line, " \t")
	if !strings.HasPrefix(trimmed, "=== ") {
		return "", false
	}
	rest := strings.TrimPrefix(trimmed, "=== ")
	for _, verb := range []string{"RUN", "PAUSE", "CONT"} {
		if strings.HasPrefix(rest, verb) {
			name := strings.TrimLeft(strings.TrimPrefix(rest, verb), " \t")
			return name, name != ""
		}
	}
	return "", false
}
