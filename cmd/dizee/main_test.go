package main

import (
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
