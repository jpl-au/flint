package flint

import (
	"go/ast"
	"go/token"
)

// checkRawText reports calls to RawText() and RawTextf() where the
// first argument is not a string literal. RawText skips HTML escaping
// entirely, so passing dynamic content opens an XSS hole - this is
// the security counterpart to checkStatic, which is concerned with
// JIT pre-rendering rather than safety.
func (l *Linter) checkRawText(fset *token.FileSet, file *ast.File) []Diagnostic {
	return l.checkLiteralArgs(fset, file, literalArgCheck{
		name:     "raw-text",
		names:    []string{"RawText", "RawTextf"},
		nargs:    -1,
		severity: Warning,
		message:  "%s() first argument must be a string literal; got %s",
		fix:      "RawText() bypasses HTML escaping and must use a string literal; use fluent-security's HTML(input) to sanitise untrusted HTML, or replace RawText with Text or Textf for plain-text content",
	})
}
