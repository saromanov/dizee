package dizee

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	pytestFailuresRe = regexp.MustCompile(`(?m)^=+\s+FAILURES\s+=+$`)
	pytestSummaryRe  = regexp.MustCompile(`(?m)^=+.*\b(?:passed|failed|errors?|skipped|xfailed|xpassed)\b.*=+$`)
	pytestProgressRe = regexp.MustCompile(`^\S+\.py\s+[.FfasxX]+\s+\[\s*\d+%\]$`)
	pytestPassedRe   = regexp.MustCompile(`^\S+::\S+\s+PASSED(?:\s+\[.*\])?$`)
	jestHeaderRe     = regexp.MustCompile(`(?m)^(?:PASS|FAIL) \S+`)
	jestPassedRe     = regexp.MustCompile(`^\s*[✓✔√]\s+`)
)

func isTestReport(b []byte) bool {
	return isPytest(b) || isJest(b) || isJUnit(b) || isGoTest(b)
}

func isPytest(b []byte) bool {
	s := string(b)
	return pytestFailuresRe.MatchString(s) || pytestSummaryRe.MatchString(s)
}

func isJest(b []byte) bool {
	return jestHeaderRe.MatchString(string(b))
}

func shrinkTest(b []byte) ([]byte, error) {
	switch {
	case isPytest(b):
		return shrinkPytest(b), nil
	case isJest(b):
		return shrinkJest(b), nil
	case isJUnit(b):
		return shrinkJUnit(b)
	case isGoTest(b):
		return shrinkGoTest(b), nil
	default:
		return nil, fmt.Errorf("unrecognized test report (want pytest, Jest, JUnit XML or go test output)")
	}
}

func shrinkPytest(b []byte) []byte {
	s := strings.TrimSuffix(string(b), "\n")
	hadTrailing := len(s) < len(b)
	lines := strings.Split(s, "\n")
	var w strings.Builder
	inFailures := false
	passed := 0
	flushPassed := func() {
		if passed > 0 {
			fmt.Fprintf(&w, "[x%d passed]\n", passed)
			passed = 0
		}
	}
	for _, line := range lines {
		if pytestFailuresRe.MatchString(line) {
			inFailures = true
		}
		if !inFailures {
			if n := pytestPassedCount(line); n > 0 {
				passed += n
				continue
			}
		}
		flushPassed()
		w.WriteString(line)
		w.WriteByte('\n')
	}
	flushPassed()
	return []byte(finishTestLines(w.String(), hadTrailing))
}

func pytestPassedCount(line string) int {
	if pytestPassedRe.MatchString(line) {
		return 1
	}
	if !pytestProgressRe.MatchString(line) {
		return 0
	}
	fields := strings.Fields(line)
	return strings.Count(fields[1], ".")
}

func shrinkJest(b []byte) []byte {
	s := strings.TrimSuffix(string(b), "\n")
	hadTrailing := len(s) < len(b)
	lines := strings.Split(s, "\n")
	var w strings.Builder
	inFailure := false
	passed := 0
	flushPassed := func() {
		if passed > 0 {
			fmt.Fprintf(&w, "[x%d passed]\n", passed)
			passed = 0
		}
	}
	for _, line := range lines {
		if jestHeaderRe.MatchString(line) {
			flushPassed()
			inFailure = strings.HasPrefix(line, "FAIL ")
			w.WriteString(line)
			w.WriteByte('\n')
			continue
		}
		if isJestSummary(line) {
			flushPassed()
			inFailure = false
			w.WriteString(line)
			w.WriteByte('\n')
			continue
		}
		if !inFailure && jestPassedRe.MatchString(line) {
			passed++
			continue
		}
		flushPassed()
		w.WriteString(line)
		w.WriteByte('\n')
	}
	flushPassed()
	return []byte(finishTestLines(w.String(), hadTrailing))
}

func isJestSummary(line string) bool {
	for _, prefix := range []string{"Test Suites:", "Tests:", "Snapshots:", "Time:", "Ran all test suites"} {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

func finishTestLines(s string, hadTrailing bool) string {
	if s != "" && !hadTrailing {
		s = strings.TrimSuffix(s, "\n")
	}
	return s
}
