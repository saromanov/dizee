package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/saromanov/dizee"
)

func TestFormatStats(t *testing.T) {
	res := dizee.Result{Stats: dizee.Stats{
		Kind:       dizee.KindLog,
		Compressed: true,
		Before:     45210,
		After:      8112,
		Tokens:     2028,
	}}
	want := "dizee: log: 45210 -> 8112 bytes (~2028 tokens, -82.1%)\n"
	if got := formatStats(res); got != want {
		t.Errorf("formatStats() = %q, want %q", got, want)
	}
}

func TestFormatStatsKeptOriginal(t *testing.T) {
	res := dizee.Result{Stats: dizee.Stats{
		Kind:   dizee.KindText,
		Before: 9,
		After:  9,
		Tokens: 3,
	}}
	want := "dizee: text: kept original (9 bytes, ~3 tokens, nothing to save)\n"
	if got := formatStats(res); got != want {
		t.Errorf("formatStats() = %q, want %q", got, want)
	}
}

type fakeRecoveryStore struct {
	content []byte
	err     error
	calls   int
}

func (s *fakeRecoveryStore) Put(content []byte) (string, error) {
	s.calls++
	if s.err != nil {
		return "", s.err
	}
	s.content = append([]byte(nil), content...)
	return "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", nil
}

func TestShrinkWithRecoveryEmitsHandle(t *testing.T) {
	input := []byte(strings.Repeat("2026-01-02 03:04:05 INFO retrying request\n", 4))
	recovery := &fakeRecoveryStore{}
	outcome := shrinkWithRecovery(input, dizee.KindLog, 1, recovery)
	if outcome.err != nil {
		t.Fatalf("shrinkWithRecovery: %v", outcome.err)
	}
	if outcome.warning != nil {
		t.Fatalf("unexpected warning: %v", outcome.warning)
	}
	if !outcome.result.Stats.Compressed {
		t.Fatal("expected compressed output")
	}
	if !bytes.HasPrefix(outcome.output, []byte("[dizee:")) {
		t.Errorf("output has no recovery handle: %q", outcome.output)
	}
	if !bytes.Equal(recovery.content, input) {
		t.Errorf("stored content = %q, want exact original %q", recovery.content, input)
	}
	if outcome.result.Stats.After != len(outcome.output) {
		t.Errorf("Stats.After = %d, output length = %d", outcome.result.Stats.After, len(outcome.output))
	}
	if want := (len(outcome.output) + 3) / 4; outcome.result.Stats.Tokens != want {
		t.Errorf("Stats.Tokens = %d, want %d", outcome.result.Stats.Tokens, want)
	}
}

func TestShrinkWithRecoveryUnavailableFailsOpen(t *testing.T) {
	input := []byte(strings.Repeat("INFO retrying\n", 4))
	outcome := shrinkWithRecovery(input, dizee.KindLog, 1, nil)
	if outcome.err != nil {
		t.Fatalf("unexpected fatal error: %v", outcome.err)
	}
	if outcome.warning == nil {
		t.Fatal("expected recovery warning")
	}
	if outcome.result.Stats.Compressed {
		t.Fatal("lossy transform ran without a store")
	}
	if !bytes.Equal(outcome.output, input) {
		t.Errorf("output = %q, want original %q", outcome.output, input)
	}
	if want := (len(input) + 3) / 4; outcome.result.Stats.Tokens != want {
		t.Errorf("Stats.Tokens = %d, want %d", outcome.result.Stats.Tokens, want)
	}
}

func TestShrinkWithRecoveryFullFailsOpen(t *testing.T) {
	input := []byte(strings.Repeat("INFO retrying a request with useful diagnostic context\n", 4))
	recovery := &fakeRecoveryStore{err: errors.New("store full")}
	outcome := shrinkWithRecovery(input, dizee.KindLog, 1, recovery)
	if outcome.err != nil {
		t.Fatalf("unexpected fatal error: %v", outcome.err)
	}
	if outcome.warning == nil {
		t.Fatal("expected recovery warning")
	}
	if outcome.result.Stats.Compressed {
		t.Fatal("lossy output used after a store error")
	}
	if !bytes.Equal(outcome.output, input) {
		t.Errorf("output = %q, want original %q", outcome.output, input)
	}
}

func TestShrinkWithRecoveryNonLossyNeedsNoStore(t *testing.T) {
	input := []byte("{\n  \"answer\": 42\n}\n")
	outcome := shrinkWithRecovery(input, "", 1, nil)
	if outcome.err != nil || outcome.warning != nil {
		t.Fatalf("unexpected errors: %v, %v", outcome.err, outcome.warning)
	}
	if !outcome.result.Stats.Compressed || string(outcome.output) != "{\"answer\":42}" {
		t.Errorf("unexpected JSON outcome: %+v, %q", outcome.result, outcome.output)
	}
}

func TestShrinkWithRecoveryMarkerNeverEnlargesOutput(t *testing.T) {
	input := []byte("INFO x\nINFO x\n")
	recovery := &fakeRecoveryStore{}
	outcome := shrinkWithRecovery(input, dizee.KindLog, 1, recovery)
	if outcome.err != nil {
		t.Fatalf("shrinkWithRecovery: %v", outcome.err)
	}
	if outcome.result.Stats.Compressed || !bytes.Equal(outcome.output, input) {
		t.Errorf("marker made output larger: %+v, %q", outcome.result, outcome.output)
	}
	if recovery.calls != 0 {
		t.Errorf("store Put calls = %d, want 0", recovery.calls)
	}
}

func TestShrinkWithRecoveryDiffContext(t *testing.T) {
	input := []byte("@@ -1,18 +1,18 @@\n" + strings.Repeat(" a long common context line with useful surrounding source\n", 16) + "-old\n+new\n")
	outcome := shrinkWithRecovery(input, dizee.KindDiff, 2, &fakeRecoveryStore{})
	if outcome.err != nil {
		t.Fatalf("shrinkWithRecovery: %v", outcome.err)
	}
	if !strings.Contains(string(outcome.output), "[+12 context lines]") {
		t.Errorf("custom diff context was not used: %s", outcome.output)
	}
	if !bytes.HasPrefix(outcome.output, []byte("[dizee:")) {
		t.Errorf("diff output has no recovery handle: %q", outcome.output)
	}
}

func TestLossyKinds(t *testing.T) {
	for _, kind := range []dizee.Kind{dizee.KindLog, dizee.KindDiff, dizee.KindCode, dizee.KindTest, dizee.KindTerminal} {
		if !isLossy(kind) {
			t.Errorf("isLossy(%q) = false, want true", kind)
		}
	}
	for _, kind := range []dizee.Kind{dizee.KindJSON, dizee.KindText} {
		if isLossy(kind) {
			t.Errorf("isLossy(%q) = true, want false", kind)
		}
	}
}
