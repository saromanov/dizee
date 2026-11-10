package dizee

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestShrinkJSON(t *testing.T) {
	in := "{\n  \"user\" : \"ada\" ,\n  \"roles\": [\"admin\", \"dev\"]\n}\n"
	res, err := Shrink([]byte(in), "")
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	if !res.Stats.Compressed {
		t.Fatalf("expected compression, got stats %+v", res.Stats)
	}
	if res.Stats.Kind != KindJSON {
		t.Errorf("kind = %q, want json", res.Stats.Kind)
	}
	var v map[string]any
	if err := json.Unmarshal(res.Output, &v); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if res.Saved() != res.Stats.Before-res.Stats.After {
		t.Errorf("Saved() = %d, want %d", res.Saved(), res.Stats.Before-res.Stats.After)
	}
	if want := (len(res.Output) + 3) / 4; res.Stats.Tokens != want {
		t.Errorf("Tokens = %d, want %d", res.Stats.Tokens, want)
	}
}

func TestShrinkNDJSON(t *testing.T) {
	in := "{\"a\": 1, \"b\": [1, 2]}\n{\"a\": 2, \"b\": []}\n"
	res, err := Shrink([]byte(in), "")
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	if !res.Stats.Compressed {
		t.Fatalf("expected compression, got stats %+v", res.Stats)
	}
	want := "{\"a\":1,\"b\":[1,2]}\n{\"a\":2,\"b\":[]}\n"
	if string(res.Output) != want {
		t.Errorf("output = %q, want %q", res.Output, want)
	}
}

func TestShrinkLogDedupe(t *testing.T) {
	in := strings.Repeat("2026-01-02 03:04:05 INFO retrying request\n", 3) +
		"2026-01-02 03:04:06 INFO gave up\n"
	res, err := Shrink([]byte(in), "")
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	if !res.Stats.Compressed {
		t.Fatalf("expected compression, got stats %+v", res.Stats)
	}
	want := "2026-01-02 03:04:05 INFO retrying request [x3]\n2026-01-02 03:04:06 INFO gave up\n"
	if string(res.Output) != want {
		t.Errorf("output = %q, want %q", res.Output, want)
	}
}

func TestShrinkLogUniqueLinesKept(t *testing.T) {
	in := "2026-01-02 03:04:05 INFO one\n2026-01-02 03:04:06 INFO two\n"
	res, err := Shrink([]byte(in), "")
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	if res.Stats.Compressed {
		t.Errorf("unexpected compression, output = %q", res.Output)
	}
	if res.Stats.Before != res.Stats.After {
		t.Errorf("Before = %d, After = %d, want equal", res.Stats.Before, res.Stats.After)
	}
}

func TestShrinkText(t *testing.T) {
	in := "first   \n\n\n\nsecond\t\nthird\n"
	res, err := Shrink([]byte(in), "")
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	if !res.Stats.Compressed {
		t.Fatalf("expected compression, got stats %+v", res.Stats)
	}
	if want := "first\n\nsecond\nthird\n"; string(res.Output) != want {
		t.Errorf("output = %q, want %q", res.Output, want)
	}
}

func TestShrinkTextKeepsNoTrailingNewline(t *testing.T) {
	in := "a\n\n\n\nb"
	res, err := Shrink([]byte(in), KindText)
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	if !res.Stats.Compressed {
		t.Fatalf("expected compression, got stats %+v", res.Stats)
	}
	if want := "a\n\nb"; string(res.Output) != want {
		t.Errorf("output = %q, want %q", res.Output, want)
	}
}

func TestShrinkOutputNeverLarger(t *testing.T) {
	tests := []string{
		"",
		"\n\n\n",
		"   \n  \n",
		"a\nb\nc\n",
		"short",
	}
	for _, in := range tests {
		res, err := Shrink([]byte(in), "")
		if err != nil {
			t.Fatalf("Shrink(%q): %v", in, err)
		}
		if res.Stats.After > res.Stats.Before {
			t.Errorf("Shrink(%q): After = %d > Before = %d", in, res.Stats.After, res.Stats.Before)
		}
		if res.Stats.Compressed && len(res.Output) != res.Stats.After {
			t.Errorf("Shrink(%q): len(Output) = %d, Stats.After = %d", in, len(res.Output), res.Stats.After)
		}
	}
}

func TestShrinkForceKind(t *testing.T) {
	res, err := Shrink([]byte("a\n\n\nb"), KindText)
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	if res.Stats.Kind != KindText {
		t.Errorf("kind = %q, want text", res.Stats.Kind)
	}
}

func TestShrinkForceInvalidJSON(t *testing.T) {
	_, err := Shrink([]byte("not json"), KindJSON)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestShrinkUnknownKind(t *testing.T) {
	_, err := Shrink([]byte("anything"), Kind("yaml"))
	if err == nil {
		t.Fatal("expected error for unknown kind")
	}
}
