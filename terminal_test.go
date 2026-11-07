package dizee

import (
	"strings"
	"testing"
)

func TestShrinkTerminalStripsSGR(t *testing.T) {
	in := "\x1b[1;32mbuild ok\x1b[0m\n"
	res, err := Shrink([]byte(in), KindTerminal)
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	if !res.Stats.Compressed {
		t.Fatalf("expected compression, got stats %+v", res.Stats)
	}
	if res.Stats.Kind != KindTerminal {
		t.Errorf("kind = %q, want terminal", res.Stats.Kind)
	}
	if want := "build ok\n"; string(res.Output) != want {
		t.Errorf("output = %q, want %q", res.Output, want)
	}
}

func TestShrinkTerminalStripsCursorAndOSC(t *testing.T) {
	in := "\x1b]0;my title\x07\x1b[2J\x1b[H\x1b[?25lready\x1b[?25h\n"
	res, err := Shrink([]byte(in), KindTerminal)
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	if want := "ready\n"; string(res.Output) != want {
		t.Errorf("output = %q, want %q", res.Output, want)
	}
}

func TestShrinkTerminalStripsMalformedSequences(t *testing.T) {
	in := "\x1btext\x1b[31unterminated\x1b]0;no terminator\nplain\n"
	res, err := Shrink([]byte(in), KindTerminal)
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	out := string(res.Output)
	if strings.Contains(out, "\x1b") {
		t.Errorf("output still contains ESC: %q", out)
	}
	if !strings.Contains(out, "plain") {
		t.Errorf("text after malformed sequences lost: %q", out)
	}
}

func TestShrinkTerminalCollapsesProgressRedraws(t *testing.T) {
	in := "Downloading 10%\rDownloading 20%\rDownloading 100%\ndone\n"
	res, err := Shrink([]byte(in), KindTerminal)
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	if want := "Downloading 100%\ndone\n"; string(res.Output) != want {
		t.Errorf("output = %q, want %q", res.Output, want)
	}
}

func TestShrinkTerminalKeepsCRLF(t *testing.T) {
	in := "one\r\ntwo\r\n"
	res, err := Shrink([]byte(in), KindTerminal)
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	out := string(res.Output)
	if !strings.Contains(out, "one") || !strings.Contains(out, "two") {
		t.Errorf("CRLF lines lost: %q", out)
	}
	if strings.Contains(out, "\r") {
		t.Errorf("CRLF left as carriage return: %q", out)
	}
}

func TestShrinkTerminalReDetectsLog(t *testing.T) {
	in := "\x1b[36m2026-01-02 03:04:05 INFO retrying\x1b[0m\n" +
		"\x1b[36m2026-01-02 03:04:06 INFO retrying\x1b[0m\n" +
		"\x1b[36m2026-01-02 03:04:07 INFO gave up\x1b[0m\n"
	res, err := Shrink([]byte(in), "")
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	want := "2026-01-02 03:04:05 INFO retrying\n2026-01-02 03:04:06 INFO retrying\n2026-01-02 03:04:07 INFO gave up\n"
	if string(res.Output) != want {
		t.Errorf("output = %q, want %q", res.Output, want)
	}
}

func TestShrinkTerminalReDetectsLogAndDedupes(t *testing.T) {
	in := "\x1b[33m[warn] cache miss\x1b[0m\n\x1b[33m[warn] cache miss\x1b[0m\n\x1b[33m[warn] cache miss\x1b[0m\n"
	res, err := Shrink([]byte(in), "")
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	if want := "[warn] cache miss [x3]\n"; string(res.Output) != want {
		t.Errorf("output = %q, want %q", res.Output, want)
	}
}

func TestShrinkTerminalReDetectsJSON(t *testing.T) {
	in := "\x1b[1m{\x1b[0m\n\x1b[1m  \"a\": 1 \x1b[0m\n\x1b[1m}\x1b[0m\n"
	res, err := Shrink([]byte(in), "")
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	if want := `{"a":1}`; string(res.Output) != want {
		t.Errorf("output = %q, want %q", res.Output, want)
	}
}

func TestShrinkTerminalCleanInputKept(t *testing.T) {
	in := "hello\n"
	res, err := Shrink([]byte(in), KindTerminal)
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

func TestStripANSIKeepsTextByteForByte(t *testing.T) {
	in := []byte("no escapes here\n")
	if got := stripANSI(in); string(got) != string(in) {
		t.Errorf("stripANSI = %q, want input unchanged", got)
	}
}
