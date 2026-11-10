package dizee

import (
	"encoding/xml"
	"strings"
	"testing"
)

const junitFixture = `<?xml version="1.0" encoding="UTF-8"?>
<testsuites tests="4" failures="1" errors="0">
  <testsuite name="example" tests="4" failures="1" errors="0">
    <testcase classname="example.TestAnswer" name="test_one">
      <system-out>a verbose successful test produces unnecessary output</system-out>
    </testcase>
    <testcase classname="example.TestAnswer" name="test_two">
      <system-out>another verbose successful test produces unnecessary output</system-out>
    </testcase>
    <testcase classname="example.TestAnswer" name="test_three">
      <system-out>yet another verbose successful test produces unnecessary output</system-out>
    </testcase>
    <testcase classname="example.TestAnswer" name="test_answer">
      <failure message="AssertionError" type="AssertionError">Traceback (most recent call last):
  File "test_example.py", line 9, in test_answer
    assert answer() == 42
AssertionError: assert 41 == 42</failure>
    </testcase>
  </testsuite>
</testsuites>
`

func TestDetectJUnit(t *testing.T) {
	for _, in := range []string{
		junitFixture,
		"<testsuite tests=\"1\"><testcase name=\"ok\"/></testsuite>",
	} {
		if got := Detect([]byte(in)); got != KindTest {
			t.Errorf("Detect(JUnit) = %q, want test", got)
		}
	}
}

func TestShrinkJUnit(t *testing.T) {
	res, err := Shrink([]byte(junitFixture), "")
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	if !res.Stats.Compressed {
		t.Fatalf("expected compression, got stats %+v", res.Stats)
	}
	out := string(res.Output)
	for _, want := range []string{
		`tests="4" failures="1" errors="0"`,
		`name="example" tests="4" failures="1" errors="0"`,
		"<!--[x3 passed]-->",
		`name="test_answer"`,
		`message="AssertionError"`,
		"assert answer() == 42",
		"AssertionError: assert 41 == 42",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	}
	for _, dropped := range []string{`name="test_one"`, `name="test_two"`, `name="test_three"`, "verbose successful"} {
		if strings.Contains(out, dropped) {
			t.Errorf("output still contains passed test data %q", dropped)
		}
	}
	var doc junitDocument
	if err := xml.Unmarshal(res.Output, &doc); err != nil {
		t.Fatalf("output is not valid XML: %v\n%s", err, res.Output)
	}
}

func TestShrinkJUnitRootSuiteAndErrors(t *testing.T) {
	in := "<testsuite tests=\"2\" errors=\"1\">" +
		"<testcase name=\"ok\"/>" +
		"<testcase name=\"bad\"><error message=\"boom\">stack trace</error></testcase>" +
		"</testsuite>\n"
	res, err := Shrink([]byte(in), KindTest)
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	out := string(res.Output)
	for _, want := range []string{"<!--[x1 passed]-->", "<error message=\"boom\">stack trace</error>"} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not contain %q: %s", want, out)
		}
	}
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("trailing newline was not preserved: %q", out)
	}
}

func TestShrinkJUnitInvalidXML(t *testing.T) {
	if _, err := Shrink([]byte("<testsuite><testcase></testsuite>"), KindTest); err == nil {
		t.Fatal("expected malformed JUnit XML to return an error")
	}
}
