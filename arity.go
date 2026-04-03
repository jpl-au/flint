package flint

import (
	"fmt"
	"go/ast"
	"go/token"
)

// checkArity reports calls to registered functions where the number of
// arguments does not match the expected count. Variadic functions
// (arity -1) accept any number of arguments and are not checked.
func checkArity(fset *token.FileSet, file *ast.File) []Diagnostic {
	if activeRegistry == nil {
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

		ident, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}

		importPath, known := imports[ident.Name]
		if !known {
			return true
		}

		pkg, registered := activeRegistry.Packages[importPath]
		if !registered {
			return true
		}

		funcName := sel.Sel.Name
		expected, isFunc := pkg.Functions[funcName]
		if !isFunc {
			return true
		}

		// Variadic functions accept any count.
		if expected < 0 {
			return true
		}

		got := len(call.Args)
		if got != expected {
			diags = append(diags, Diagnostic{
				Pos:     fset.Position(call.Lparen),
				End:     fset.Position(call.Rparen),
				Message: fmt.Sprintf("%s.%s() expects %d argument(s), got %d", lastSegment(importPath), funcName, expected, got),
				Fix:     fmt.Sprintf("Check the %s.%s signature for the correct number of arguments", lastSegment(importPath), funcName),
			})
		}

		return true
	})

	return diags
}
