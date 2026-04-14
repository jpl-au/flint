package flint

import (
	"fmt"
	"go/ast"
	"go/token"
)

// checkTypedParams reports method calls where a string literal is passed
// to a method that expects a typed enum constant. For example,
// input.New().Type("email") should be input.New().Type(inputtype.Email).
func (l *Linter) checkTypedParams(fset *token.FileSet, file *ast.File) []Diagnostic {
	if l.registry == nil {
		return nil
	}

	imports := resolveImports(file)
	var diags []Diagnostic

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		methodName := sel.Sel.Name

		// Find the originating package for this method chain.
		pkg, found := chainPackage(sel.X, imports, l.registry)
		if !found {
			return true
		}

		// Check if this method expects a typed parameter.
		enumPkg, hasTyped := pkg.TypedParams[methodName]
		if !hasTyped {
			return true
		}

		// Check if the first argument is a string literal.
		if len(call.Args) == 0 {
			return true
		}

		arg := call.Args[0]
		if !isStringLiteral(arg) {
			return true
		}

		lit := arg.(*ast.BasicLit)
		diags = append(diags, Diagnostic{
			Pos:      fset.Position(arg.Pos()),
			End:      fset.Position(arg.End()),
			Severity: Warning,
			Message:  fmt.Sprintf(".%s() expects a typed constant, not a string literal %s", methodName, lit.Value),
			Fix:      fmt.Sprintf("Use a value from the %s package (e.g., %s.X) or %s.Custom(...)", enumPkg, enumPkg, enumPkg),
		})

		return true
	})

	return diags
}
