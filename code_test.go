package dizee

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

const codeFixture = `package sample

import "fmt"

type Counter struct {
	N int
}

func (c *Counter) Add(n int) int {
	c.N += n
	fmt.Println(c.N)
	return c.N
}

func Identity[T any](value T) T {
	return value
}
`

func TestShrinkCode(t *testing.T) {
	res, err := Shrink([]byte(codeFixture), KindCode)
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	if !res.Stats.Compressed {
		t.Fatalf("expected compression, got stats %+v", res.Stats)
	}
	out := string(res.Output)
	for _, want := range []string{
		`import "fmt"`,
		"type Counter struct",
		"func (c *Counter) Add(n int) int",
		"func Identity[T any](value T) T",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	}
	for _, dropped := range []string{"fmt.Println", "return c.N", "return value"} {
		if strings.Contains(out, dropped) {
			t.Errorf("output still contains function body statement %q", dropped)
		}
	}
}

func TestShrinkCodeOutputParseableAndDeterministic(t *testing.T) {
	first, err := shrinkCode([]byte(codeFixture))
	if err != nil {
		t.Fatalf("shrinkCode: %v", err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), "", first, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("output is not parseable Go: %v\n%s", err, first)
	}
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
			t.Errorf("function %s still has a body", fn.Name.Name)
		}
	}
	second, err := shrinkCode(first)
	if err != nil {
		t.Fatalf("second shrinkCode: %v", err)
	}
	if string(second) != string(first) {
		t.Errorf("output is not deterministic:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestShrinkCodeIsExplicitOnly(t *testing.T) {
	if got := Detect([]byte(codeFixture)); got != KindText {
		t.Errorf("Detect(Go code) = %q, want text", got)
	}
}

func TestShrinkCodeInvalidGo(t *testing.T) {
	if _, err := Shrink([]byte("package main\nfunc broken {"), KindCode); err == nil {
		t.Fatal("expected invalid Go to return an error")
	}
}
