package dizee

import (
	"strings"
	"testing"
)

func TestDetectDiff(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"git header", "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n"},
		{"hunk header", "--- a/a.go\n+++ b/a.go\n@@ -10,2 +10,2 @@\n old\n"},
		{"empty hunk counts", "@@ -0,0 +1 @@\n+new\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Detect([]byte(tt.in)); got != KindDiff {
				t.Errorf("Detect() = %q, want diff", got)
			}
		})
	}
}

func TestShrinkDiff(t *testing.T) {
	in := "diff --git a/a.go b/a.go\n" +
		"--- a/a.go\n+++ b/a.go\n" +
		"@@ -1,9 +1,9 @@\n" +
		" one\n two\n three\n four\n five\n six\n seven\n-old\n+new\n nine\n"
	res, err := Shrink([]byte(in), "")
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	if !res.Stats.Compressed {
		t.Fatalf("expected compression, got stats %+v", res.Stats)
	}
	want := "diff --git a/a.go b/a.go\n" +
		"--- a/a.go\n+++ b/a.go\n" +
		"@@ -1,9 +1,9 @@\n" +
		" one\n[+5 context lines]\n seven\n-old\n+new\n nine\n"
	if string(res.Output) != want {
		t.Errorf("output =\n%s\nwant =\n%s", res.Output, want)
	}
}

func TestShrinkDiffContextTwo(t *testing.T) {
	in := "@@ -1,10 +1,10 @@\n one\n two\n three\n four\n five\n six\n seven\n eight\n-old\n+new\n"
	res, err := ShrinkDiff([]byte(in), 2)
	if err != nil {
		t.Fatalf("ShrinkDiff: %v", err)
	}
	want := "@@ -1,10 +1,10 @@\n one\n two\n[+4 context lines]\n seven\n eight\n-old\n+new\n"
	if string(res.Output) != want {
		t.Errorf("output =\n%s\nwant =\n%s", res.Output, want)
	}
}

func TestShrinkDiffSeparateContextRuns(t *testing.T) {
	in := "@@ -1,16 +1,16 @@\n one\n two\n three\n four\n five\n six\n-old\n+new\n seven\n eight\n nine\n ten\n eleven\n twelve\n-thirteen\n+THIRTEEN\n"
	res, err := Shrink([]byte(in), KindDiff)
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	if got := strings.Count(string(res.Output), "context lines"); got != 2 {
		t.Errorf("context markers = %d, want 2; output =\n%s", got, res.Output)
	}
	for _, kept := range []string{"-old", "+new", "-thirteen", "+THIRTEEN"} {
		if !strings.Contains(string(res.Output), kept) {
			t.Errorf("changed line %q missing from output", kept)
		}
	}
}

func TestShrinkDiffKeepsShortContext(t *testing.T) {
	in := "@@ -1,2 +1,2 @@\n one\n-two\n+two\n"
	res, err := Shrink([]byte(in), KindDiff)
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	if res.Stats.Compressed {
		t.Errorf("unexpected compression: %q", res.Output)
	}
}

func TestShrinkDiffNoTrailingNewline(t *testing.T) {
	in := "@@ -1,9 +1,9 @@\n one\n two\n three\n four\n five\n six\n seven\n eight\n-nine"
	res, err := Shrink([]byte(in), KindDiff)
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	if !res.Stats.Compressed {
		t.Fatalf("expected compression, got stats %+v", res.Stats)
	}
	if strings.HasSuffix(string(res.Output), "\n") {
		t.Errorf("output gained a trailing newline: %q", res.Output)
	}
}

func TestShrinkDiffRejectsInvalidContext(t *testing.T) {
	for _, context := range []int{-1, 0, 3} {
		if _, err := ShrinkDiff([]byte("diff --git a/a b/a\n"), context); err == nil {
			t.Errorf("ShrinkDiff(context=%d) succeeded, want error", context)
		}
	}
}
