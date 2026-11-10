package dizee

import (
	"strings"
	"testing"
)

const pytestFixture = `============================= test session starts ==============================
platform linux -- Python 3.13.0, pytest-8.0.0
rootdir: /work/project
collected 3 items

test_example.py ..F                                                        [100%]

=================================== FAILURES ===================================
_______________________________ test_answer __________________________________

    def test_answer():
>       assert answer() == 42
E       AssertionError: assert 41 == 42

test_example.py:9: AssertionError
=========================== short test summary info ============================
FAILED test_example.py::test_answer - AssertionError: assert 41 == 42
=========================== 1 failed, 2 passed in 0.12s ============================
`

const jestFixture = `PASS tests/example.test.js
  ✓ returns the default answer with a very descriptive test name (5 ms)
  ✓ handles another successful case with a very descriptive test name

FAIL tests/broken.test.js
  ● returns the answer

    expect(received).toBe(expected)

    Expected: 42
    Received: 41
      at Object.<anonymous> (tests/broken.test.js:8:23)

Test Suites: 1 failed, 1 passed, 2 total
Tests:       1 failed, 2 passed, 3 total
Snapshots:   0 total
Time:        1.234 s
Ran all test suites.
`

func TestDetectTestReports(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"pytest failures", pytestFixture},
		{"pytest summary", "=== 2 passed in 0.01s ===\n"},
		{"jest pass", "PASS tests/example.test.js\n"},
		{"jest fail", "FAIL tests/example.test.js\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Detect([]byte(tt.in)); got != KindTest {
				t.Errorf("Detect() = %q, want test", got)
			}
		})
	}
}

func TestShrinkPytest(t *testing.T) {
	res, err := Shrink([]byte(pytestFixture), "")
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	if !res.Stats.Compressed {
		t.Fatalf("expected compression, got stats %+v", res.Stats)
	}
	out := string(res.Output)
	for _, want := range []string{
		"[x2 passed]",
		"=================================== FAILURES ===================================",
		"E       AssertionError: assert 41 == 42",
		"test_example.py:9: AssertionError",
		"1 failed, 2 passed in 0.12s",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "test_example.py ..F") {
		t.Errorf("pytest progress line was not collapsed:\n%s", out)
	}
}

func TestShrinkPytestVerbosePasses(t *testing.T) {
	in := "test_example.py::test_one PASSED\n" +
		"test_example.py::test_two PASSED\n" +
		"test_example.py::test_three PASSED\n" +
		"======================== 3 passed in 0.01s ========================\n"
	res, err := Shrink([]byte(in), KindTest)
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	if want := "[x3 passed]"; !strings.Contains(string(res.Output), want) {
		t.Errorf("output does not contain %q: %s", want, res.Output)
	}
}

func TestShrinkJest(t *testing.T) {
	res, err := Shrink([]byte(jestFixture), "")
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	if !res.Stats.Compressed {
		t.Fatalf("expected compression, got stats %+v", res.Stats)
	}
	out := string(res.Output)
	for _, want := range []string{
		"PASS tests/example.test.js\n[x2 passed]",
		"FAIL tests/broken.test.js",
		"Expected: 42",
		"at Object.<anonymous> (tests/broken.test.js:8:23)",
		"Tests:       1 failed, 2 passed, 3 total",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	}
}

func TestShrinkTestRejectsUnknownInput(t *testing.T) {
	if _, err := Shrink([]byte("not a test report"), KindTest); err == nil {
		t.Fatal("expected unknown test report to return an error")
	}
}
