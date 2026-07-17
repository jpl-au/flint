package flint

import (
	"fmt"
	"go/ast"
	"go/token"
)

// literalArgCheck describes a check that flags calls to named functions
// where the first argument is not a string literal.
type literalArgCheck struct {
	name     string   // check name recorded on the diagnostic
	names    []string // function/method names to match
	nargs    int      // exact arg count to match, or -1 for any
	severity Severity
	message  string // fmt pattern for the diagnostic
	fix      string
}

// checkStatic reports calls to Static() where the argument is not a
// string literal. Static content is marked for JIT pre-rendering and
// must not contain dynamic values.
func (l *Linter) checkStatic(fset *token.FileSet, file *ast.File) []Diagnostic {
	return l.checkLiteralArgs(fset, file, literalArgCheck{
		name:     "static",
		names:    []string{"Static"},
		nargs:    1,
		severity: Warning,
		message:  "Static() argument must be a string literal; got %s",
		fix:      "Static() is for string literals only (JIT pre-rendering); replace Static with Text or Textf for dynamic content",
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
			Check:    check.name,
			Pos:      fset.Position(arg.Pos()),
			End:      fset.Position(arg.End()),
			Severity: check.severity,
			Message:  msg,
			Fix:      check.fix,
		})

		return true
	})

	return diags
}
