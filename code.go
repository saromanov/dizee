package dizee

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
)

func shrinkCode(b []byte) ([]byte, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", b, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			fn.Body = nil
		}
	}
	var buf bytes.Buffer
	cfg := printer.Config{Mode: printer.TabIndent | printer.UseSpaces, Tabwidth: 8}
	if err := cfg.Fprint(&buf, fset, file); err != nil {
		return nil, err
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}
