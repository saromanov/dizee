package dizee

import (
	"strings"
	"testing"
)

const goTestVerbosePassFixture = `=== RUN   TestAlpha
--- PASS: TestAlpha (0.00s)
=== RUN   TestBeta
--- PASS: TestBeta (0.00s)
PASS
ok  	example.com/proj	0.012s
`

const goTestVerboseFailFixture = `=== RUN   TestAnswer
--- FAIL: TestAnswer (0.00s)
    main_test.go:12: got 41, want 42
=== RUN   TestWarm
--- PASS: TestWarm (0.00s)
FAIL
exit status 1
FAIL	example.com/proj	0.005s
`

const goTestNestedFixture = `=== RUN   TestFoo
=== RUN   TestFoo/sub
    foo_test.go:10: doing sub work
    --- PASS: TestFoo/sub (0.00s)
--- PASS: TestFoo (0.00s)
PASS
ok  	example.com/proj	0.012s
`

const goTestPanicFixture = `=== RUN   TestPanic
--- FAIL: TestPanic (0.00s)
panic: boom [recovered]
	panic: boom

goroutine 7 [running]:
testing.tRunner.func1.2({0x5a39e0, 0x6a0780})
	/usr/local/go/src/testing/testing.go:1548 +0x374
created by testing.tRunner.func1 in goroutine 6
	/usr/local/go/src/testing/testing.go:1559 +0x44
exit status 2
FAIL	example.com/proj	0.005s
`

const goTestMultiPackageFixture = `--- FAIL: TestAnswer (0.00s)
    a_test.go:7: nope
FAIL
exit status 1
FAIL	example.com/a	0.100s
ok  	example.com/b	0.200s
ok  	example.com/c	0.500s (cached)
?   	example.com/d	[no test files]
`

const goTestSkipFixture = `=== RUN   TestSkip
    skip_test.go:9: skipping: -short set
--- SKIP: TestSkip (0.00s)
=== RUN   TestGo
--- PASS: TestGo (0.00s)
PASS
ok  	example.com/proj	0.010s
`

func TestDetectGoTest(t *testing.T) {
	for _, tt := range []struct {
		name string
		in   string
	}{
		{"verbose pass", goTestVerbosePassFixture},
		{"verbose fail", goTestVerboseFailFixture},
		{"nested subtests", goTestNestedFixture},
		{"panic", goTestPanicFixture},
		{"multi package", goTestMultiPackageFixture},
		{"skip", goTestSkipFixture},
		{"non verbose fail", "--- FAIL: TestAnswer (0.00s)\n    main_test.go:12: nope\nFAIL\nexit status 1\nFAIL\texample.com/proj\t0.005s\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := Detect([]byte(tt.in)); got != KindTest {
				t.Errorf("Detect() = %q, want test", got)
			}
		})
	}
}

func TestDetectGoTestRejectsLookalikes(t *testing.T) {
	for _, tt := range []struct {
		name string
		in   string
	}{
		{"package line only", "ok  \texample.com/proj\t0.012s\n"},
		{"single run line", "=== RUN   TestAlpha\n"},
		{"run lines without package", "=== RUN   TestAlpha\n--- PASS: TestAlpha (0.00s)\n"},
		{"plain text", "the TestAlpha run passed ok\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := Detect([]byte(tt.in)); got == KindTest {
				t.Errorf("Detect() = test, want something else for %q", tt.in)
			}
		})
	}
}

func TestShrinkGoTestVerbosePasses(t *testing.T) {
	res, err := Shrink([]byte(goTestVerbosePassFixture), "")
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	if !res.Stats.Compressed {
		t.Fatalf("expected compression, got stats %+v", res.Stats)
	}
	out := string(res.Output)
	for _, want := range []string{
		"[x2 passed]",
		"ok  \texample.com/proj\t0.012s",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"=== RUN", "--- PASS", "\nPASS\n"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("output still contains %q:\n%s", unwanted, out)
		}
	}
}

func TestShrinkGoTestNestedSubtests(t *testing.T) {
	res, err := Shrink([]byte(goTestNestedFixture), "")
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	if !res.Stats.Compressed {
		t.Fatalf("expected compression, got stats %+v", res.Stats)
	}
	out := string(res.Output)
	if want := "[x2 passed]"; !strings.Contains(out, want) {
		t.Errorf("output does not contain %q:\n%s", want, out)
	}
	for _, unwanted := range []string{"=== RUN", "--- PASS", "doing sub work"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("nested passing output still contains %q:\n%s", unwanted, out)
		}
	}
}

func TestShrinkGoTestKeepsCompleteFailure(t *testing.T) {
	res, err := Shrink([]byte(goTestVerboseFailFixture), "")
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	if !res.Stats.Compressed {
		t.Fatalf("expected compression, got stats %+v", res.Stats)
	}
	out := string(res.Output)
	for _, want := range []string{
		"[x1 passed]",
		"=== RUN   TestAnswer",
		"--- FAIL: TestAnswer (0.00s)",
		"    main_test.go:12: got 41, want 42",
		"exit status 1",
		"FAIL\texample.com/proj\t0.005s",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "--- PASS: TestWarm") || strings.Contains(out, "=== RUN   TestWarm") {
		t.Errorf("passing test was not collapsed:\n%s", out)
	}
}

func TestShrinkGoTestKeepsPanicAndStackTrace(t *testing.T) {
	res, err := Shrink([]byte(goTestPanicFixture), "")
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	if res.Stats.Compressed {
		t.Fatalf("panic output has nothing to collapse, got stats %+v", res.Stats)
	}
	out := string(shrinkGoTest([]byte(goTestPanicFixture)))
	for _, want := range []string{
		"=== RUN   TestPanic",
		"--- FAIL: TestPanic (0.00s)",
		"panic: boom [recovered]",
		"\tpanic: boom",
		"goroutine 7 [running]:",
		"testing.tRunner.func1.2({0x5a39e0, 0x6a0780})",
		"/usr/local/go/src/testing/testing.go:1548 +0x374",
		"created by testing.tRunner.func1 in goroutine 6",
		"exit status 2",
		"FAIL\texample.com/proj\t0.005s",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("panic output lost %q:\n%s", want, out)
		}
	}
}

func TestShrinkGoTestMultiPackage(t *testing.T) {
	res, err := Shrink([]byte(goTestMultiPackageFixture), "")
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	if res.Stats.Compressed {
		t.Fatalf("non-verbose failing output is already minimal, got stats %+v", res.Stats)
	}
	out := string(shrinkGoTest([]byte(goTestMultiPackageFixture)))
	if out != goTestMultiPackageFixture {
		t.Errorf("multi-package output changed:\ngot:\n%s\nwant:\n%s", out, goTestMultiPackageFixture)
	}
}

func TestShrinkGoTestPreservesSkipsSeparately(t *testing.T) {
	res, err := Shrink([]byte(goTestSkipFixture), "")
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	if !res.Stats.Compressed {
		t.Fatalf("expected compression, got stats %+v", res.Stats)
	}
	out := string(res.Output)
	for _, want := range []string{
		"[x1 passed]",
		"--- SKIP: TestSkip (0.00s)",
		"    skip_test.go:9: skipping: -short set",
		"ok  \texample.com/proj\t0.010s",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "[x2 passed]") || strings.Contains(out, "--- PASS: TestGo") {
		t.Errorf("skipped or passing test miscounted:\n%s", out)
	}
}

func TestShrinkGoTestCollapsesPassedNames(t *testing.T) {
	in := []byte(goTestVerbosePassFixture)
	res, err := Shrink(in, "")
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	if len(res.Output) >= len(in) {
		t.Fatalf("output %d bytes, want smaller than input %d", len(res.Output), len(in))
	}
	// Passed test names collapse into the marker entirely.
	if got := strings.Count(string(res.Output), "Test"); got != 0 {
		t.Errorf("collapsed output still mentions %d test names:\n%s", got, res.Output)
	}
}
