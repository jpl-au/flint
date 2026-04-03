package flint

import (
	"fmt"
	"go/ast"
	"go/token"
)

// checkSymbols reports references to symbols that do not exist in the
// registry. It resolves imports in the source file, then checks every
// selector expression (pkg.Symbol) against the registered API surface.
func checkSymbols(fset *token.FileSet, file *ast.File) []Diagnostic {
	if activeRegistry == nil {
		return nil
	}

	imports := resolveImports(file)

	var diags []Diagnostic

	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
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

		name := sel.Sel.Name
		_, isFunc := pkg.Functions[name]
		if isFunc || pkg.Types[name] || pkg.Vars[name] {
			return true
		}

		diags = append(diags, Diagnostic{
			Pos:     fset.Position(sel.Sel.Pos()),
			End:     fset.Position(sel.Sel.End()),
			Message: fmt.Sprintf("%s.%s does not exist", lastSegment(importPath), name),
			Fix:     fmt.Sprintf("Check the %s package for available functions and variables", lastSegment(importPath)),
		})

		return true
	})

	// Check method calls on chains originating from registered packages.
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		pkg, found := chainPackage(sel.X, imports, activeRegistry)
		if !found {
			return true
		}

		method := sel.Sel.Name
		if pkg.Methods == nil || pkg.Methods[method] {
			return true
		}
		if _, ok := pkg.Functions[method]; ok {
			return true
		}

		diags = append(diags, Diagnostic{
			Pos:     fset.Position(sel.Sel.Pos()),
			End:     fset.Position(sel.Sel.End()),
			Message: fmt.Sprintf("method %s does not exist on this element", method),
			Fix:     "Check the element package for available methods",
		})

		return true
	})

	return diags
}
