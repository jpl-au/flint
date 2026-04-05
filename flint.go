// Package flint provides AST-based validation of Go source code that
// uses the fluent HTML framework. It catches common misuse patterns
// that defeat JIT optimisation or introduce security vulnerabilities.
//
// The linter operates on source code strings using go/parser and
// go/ast. It has no dependency on fluent itself.
package flint

import (
	"bytes"
	"go/parser"
	"go/token"
	"sort"
)

// Diagnostic reports a single problem found in the source code.
type Diagnostic struct {
	Pos     token.Position
	End     token.Position
	Message string
	Fix     string
}

// Linter validates Go source code that uses the fluent HTML framework.
// Create one with New and reuse it across files.
type Linter struct {
	registry    *Registry
	attrMethods map[string]string
}

// New creates a Linter with the given registry. Pass FluentRegistry()
// for full validation, or nil to run only Static and RawText checks.
func New(r *Registry) *Linter {
	l := &Linter{registry: r}
	if r != nil {
		l.attrMethods = mergeAttrMethods(r)
	}
	return l
}

// Source analyses Go source code and returns all diagnostics found.
// The filename is used only for position information in diagnostics.
//
// An error is returned only if the source cannot be parsed. Lint
// diagnostics are returned in the slice, not as errors.
func (l *Linter) Source(filename string, src []byte) ([]Diagnostic, error) {
	if bytes.Contains(src, []byte("// Code generated")) && bytes.Contains(src, []byte("DO NOT EDIT")) {
		return nil, nil
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.AllErrors)
	if err != nil {
		return nil, err
	}

	var diags []Diagnostic
	diags = append(diags, l.checkStatic(fset, file)...)
	diags = append(diags, l.checkRawText(fset, file)...)
	diags = append(diags, l.checkImports(fset, file)...)
	diags = append(diags, l.checkSetAttrChain(fset, file)...)
	diags = append(diags, l.checkSetAttrKey(fset, file)...)
	diags = append(diags, l.checkTypedParams(fset, file)...)
	diags = append(diags, l.checkConstructors(fset, file)...)
	diags = append(diags, l.checkSymbols(fset, file)...)
	diags = append(diags, l.checkArity(fset, file)...)

	sort.Slice(diags, func(i, j int) bool {
		if diags[i].Pos.Line != diags[j].Pos.Line {
			return diags[i].Pos.Line < diags[j].Pos.Line
		}
		return diags[i].Pos.Column < diags[j].Pos.Column
	})

	return diags, nil
}
