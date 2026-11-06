package dizee

import "testing"

func TestDetect(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Kind
	}{
		{"pretty json object", "{\n  \"a\": 1\n}\n", KindJSON},
		{"json array", "[1, 2, 3]", KindJSON},
		{"ndjson", "{\"a\":1}\n{\"a\":2}\n", KindJSON},
		{"timestamped logs", "2026-01-02 03:04:05 INFO Server started\n2026-01-02 03:04:06 INFO Request handled\n2026-01-02 03:04:07 ERROR Connection lost\n", KindLog},
		{"syslog", "Jan  2 03:04:05 host sshd[123]: Accepted\nJan  2 03:04:06 host sshd[124]: Closed\nJan  2 03:04:07 host sshd[125]: Accepted\n", KindLog},
		{"bracket time", "[03:04:05] boot\n[03:04:06] ready\n[03:04:07] serve\n", KindLog},
		{"bare levels", "INFO one\nINFO two\nERROR three\n", KindLog},
		{"plain text", "The quick brown fox\njumps over the lazy dog.\n", KindText},
		{"markdown", "# Title\n\nSome *text* here.\n", KindText},
		{"code", "package main\n\nfunc main() {}\n", KindText},
		{"empty", "", KindText},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Detect([]byte(tt.in)); got != tt.want {
				t.Errorf("Detect(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestDetectLogNeedsMajority(t *testing.T) {
	// Two matching lines out of five is not a log.
	in := "INFO one\nINFO two\njust text\nmore text\nfinal text\n"
	if got := Detect([]byte(in)); got != KindText {
		t.Errorf("Detect = %q, want %q", got, KindText)
	}
}
