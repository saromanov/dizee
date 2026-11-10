package dizee

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
)

var diffHunkRe = regexp.MustCompile(`(?m)^@@ -\d+(?:,\d+)? \+\d+(?:,\d+)? @@`)

func isDiff(b []byte) bool {
	return bytes.Contains(b, []byte("\ndiff --git ")) || bytes.HasPrefix(b, []byte("diff --git ")) || diffHunkRe.Match(b)
}

// ShrinkDiff collapses context runs in a unified diff, keeping context lines
// at both boundaries of each run. Only one or two boundary lines are supported.
func ShrinkDiff(b []byte, context int) (Result, error) {
	if context < 1 || context > 2 {
		return Result{}, fmt.Errorf("diff context must be 1 or 2, got %d", context)
	}
	return newResult(b, shrinkDiff(b, context), KindDiff), nil
}

func shrinkDiff(b []byte, context int) []byte {
	s := strings.TrimSuffix(string(b), "\n")
	hadTrailing := len(s) < len(b)
	lines := strings.Split(s, "\n")
	var w strings.Builder
	inHunk := false
	for i := 0; i < len(lines); {
		line := lines[i]
		if diffHunkRe.MatchString(line) {
			inHunk = true
			w.WriteString(line)
			w.WriteByte('\n')
			i++
			continue
		}
		if !inHunk || !strings.HasPrefix(line, " ") {
			if strings.HasPrefix(line, "diff --git ") || strings.HasPrefix(line, "--- ") {
				inHunk = false
			}
			w.WriteString(line)
			w.WriteByte('\n')
			i++
			continue
		}

		j := i
		for j < len(lines) && strings.HasPrefix(lines[j], " ") {
			j++
		}
		n := j - i
		for k := 0; k < n; k++ {
			if k < context || k >= n-context || n <= 2*context {
				w.WriteString(lines[i+k])
				w.WriteByte('\n')
			} else if k == context {
				fmt.Fprintf(&w, "[+%d context lines]\n", n-2*context)
			}
		}
		i = j
	}
	out := w.String()
	if out != "" && !hadTrailing {
		out = strings.TrimSuffix(out, "\n")
	}
	return []byte(out)
}
