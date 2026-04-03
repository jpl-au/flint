package flint

import (
	"fmt"
	"go/ast"
	"go/token"
)

// checkStaticLiteral reports calls to Static() where the argument is not
// a string literal. Static content is marked for JIT pre-rendering and
// must not contain dynamic values. Passing a variable, concatenation, or
// function call defeats this optimisation silently.
func checkStaticLiteral(fset *token.FileSet, file *ast.File) []Diagnostic {
	return checkLiteralArgs(fset, file, literalArgCheck{
		names:   []string{"Static"},
		nargs:   1,
		message: "Static() argument must be a string literal; got %s",
		fix:     "Use .Text() or .Textf() for dynamic content, or pass a string literal to .Static()",
	})
}

// checkRawTextLiteral reports calls to RawText() and RawTextf() where
// arguments are not string literals. Raw text is not HTML-escaped, so
// passing dynamic content risks cross-site scripting vulnerabilities.
func checkRawTextLiteral(fset *token.FileSet, file *ast.File) []Diagnostic {
	return checkLiteralArgs(fset, file, literalArgCheck{
		names:   []string{"RawText", "RawTextf"},
		nargs:   -1, // check first argument regardless of count
		message: "%s() first argument must be a string literal; got %s",
		fix:     "Use .Text() or .Textf() for dynamic content, or pass a string literal to .RawText()",
	})
}

// literalArgCheck describes a check that flags calls to named functions
// where the first argument is not a string literal.
type literalArgCheck struct {
	names   []string // function/method names to match
	nargs   int      // exact arg count to match, or -1 to check any call with at least one arg
	message string   // fmt pattern: for single-name checks gets describeExpr; for multi-name gets (name, describeExpr)
	fix     string
}

// checkLiteralArgs walks the AST and reports calls matching check where
// the first argument is not a string literal.
func checkLiteralArgs(fset *token.FileSet, file *ast.File, check literalArgCheck) []Diagnostic {
	var diags []Diagnostic

	names := make(map[string]bool, len(check.names))
	for _, n := range check.names {
		names[n] = true
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

// calleeName returns the simple name of the called function or method.
// For selector expressions (pkg.Func or recv.Method) it returns the
// selected name. For plain identifiers it returns the identifier name.
// For anything else it returns an empty string.
func calleeName(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.SelectorExpr:
		return fn.Sel.Name
	case *ast.Ident:
		return fn.Name
	}
	return ""
}

// isStringLiteral reports whether expr is a basic string literal.
func isStringLiteral(expr ast.Expr) bool {
	lit, ok := expr.(*ast.BasicLit)
	return ok && lit.Kind == token.STRING
}

// describeExpr returns a human-readable description of an expression
// for use in diagnostic messages.
func describeExpr(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return fmt.Sprintf("variable %q", e.Name)
	case *ast.BinaryExpr:
		return "binary expression"
	case *ast.CallExpr:
		return "function call"
	case *ast.IndexExpr:
		return "index expression"
	case *ast.SliceExpr:
		return "slice expression"
	case *ast.UnaryExpr:
		return "unary expression"
	case *ast.ParenExpr:
		return describeExpr(e.X)
	default:
		return "non-literal expression"
	}
}
