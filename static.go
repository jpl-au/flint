package flint

import (
	"fmt"
	"go/ast"
	"go/token"
)

// literalArgCheck describes a check that flags calls to named functions
// where the first argument is not a string literal.
type literalArgCheck struct {
	names   []string // function/method names to match
	nargs   int      // exact arg count to match, or -1 for any
	message string   // fmt pattern for the diagnostic
	fix     string
}

// checkStatic reports calls to Static() where the argument is not a
// string literal. Static content is marked for JIT pre-rendering and
// must not contain dynamic values.
func (l *Linter) checkStatic(fset *token.FileSet, file *ast.File) []Diagnostic {
	return l.checkLiteralArgs(fset, file, literalArgCheck{
		names:   []string{"Static"},
		nargs:   1,
		message: "Static() argument must be a string literal; got %s",
		fix:     "Static() is for string literals only (JIT pre-rendering); replace Static with Text or Textf for dynamic content",
	})
}

// checkRawText reports calls to RawText() and RawTextf() where the
// first argument is not a string literal. Raw text is not HTML-escaped,
// so passing dynamic content risks XSS vulnerabilities.
func (l *Linter) checkRawText(fset *token.FileSet, file *ast.File) []Diagnostic {
	return l.checkLiteralArgs(fset, file, literalArgCheck{
		names:   []string{"RawText", "RawTextf"},
		nargs:   -1,
		message: "%s() first argument must be a string literal; got %s",
		fix:     "RawText() bypasses HTML escaping and must use a string literal; replace RawText with Text or Textf for dynamic content",
	})
}

// checkLiteralArgs walks the AST and reports calls matching check where
// the first argument is not a string literal. Only flags calls on
// fluent elements (scoped via the registry).
func (l *Linter) checkLiteralArgs(fset *token.FileSet, file *ast.File, check literalArgCheck) []Diagnostic {
	var diags []Diagnostic

	names := make(map[string]bool, len(check.names))
	for _, n := range check.names {
		names[n] = true
	}

	// Scope to fluent packages when a registry is available.
	var imports map[string]string
	if l.registry != nil {
		imports = resolveImports(file)
	}

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		name := calleeName(call)
		if !names[name] {
			return true
		}

		// Scope check: verify the receiver traces back to a fluent package.
		if imports != nil && l.registry != nil {
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if _, found := chainPackage(sel.X, imports, l.registry); !found {
				return true
			}
		}

		if len(call.Args) == 0 {
			return true
		}
		if check.nargs > 0 && len(call.Args) != check.nargs {
			return true
		}

		arg := call.Args[0]
		if isStringLiteral(arg) {
			return true
		}

		var msg string
		if len(check.names) == 1 {
			msg = fmt.Sprintf(check.message, describeExpr(arg))
		} else {
			msg = fmt.Sprintf(check.message, name, describeExpr(arg))
		}

		diags = append(diags, Diagnostic{
			Pos:     fset.Position(arg.Pos()),
			End:     fset.Position(arg.End()),
			Message: msg,
			Fix:     check.fix,
		})

		return true
	})

	return diags
}
