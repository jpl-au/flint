package flint

import (
	"fmt"
	"go/ast"
	"go/token"
)

// checkConstructors reports calls to pkg.New() immediately followed
// by a single method call that exists as a package-level constructor.
// For example, div.New().Text("hello") should be div.Text("hello").
func checkConstructors(fset *token.FileSet, file *ast.File) []Diagnostic {
	if activeRegistry == nil {
		return nil
	}

	imports := resolveImports(file)
	var diags []Diagnostic

	ast.Inspect(file, func(n ast.Node) bool {
		// Look for: pkg.New(...).Method(args...)
		// AST shape: CallExpr { Fun: SelectorExpr { X: CallExpr { Fun: SelectorExpr { X: Ident, Sel: "New" } }, Sel: Method } }
		outerCall, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		outerSel, ok := outerCall.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		methodName := outerSel.Sel.Name

		// The receiver of the method must be a call to New().
		innerCall, ok := outerSel.X.(*ast.CallExpr)
		if !ok {
			return true
		}

		innerSel, ok := innerCall.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		if innerSel.Sel.Name != "New" {
			return true
		}

		// The receiver of New() must be a package identifier.
		pkgIdent, ok := innerSel.X.(*ast.Ident)
		if !ok {
			return true
		}

		importPath, known := imports[pkgIdent.Name]
		if !known {
			return true
		}

		pkg, registered := activeRegistry.Packages[importPath]
		if !registered {
			return true
		}

		// Check if New() was called with no arguments (or only
		// node.Node arguments which the direct constructor also
		// accepts). For simplicity, only flag when New() has no args.
		if len(innerCall.Args) > 0 {
			return true
		}

		// Check if the chained method exists as a constructor.
		if _, ok := pkg.Functions[methodName]; !ok {
			return true
		}

		diags = append(diags, Diagnostic{
			Pos:     fset.Position(innerSel.Sel.Pos()),
			End:     fset.Position(outerCall.End()),
			Message: fmt.Sprintf("use %s.%s(...) directly instead of %s.New().%s(...)", pkgIdent.Name, methodName, pkgIdent.Name, methodName),
			Fix:     fmt.Sprintf("%s.%s(...) is a constructor that replaces New().%s(...)", pkgIdent.Name, methodName, methodName),
		})

		return true
	})

	return diags
}
