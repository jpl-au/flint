package flint

import (
	"fmt"
	"go/ast"
	"go/token"
)

// checkSymbols reports references to symbols that do not exist in the
// registry. It resolves imports in the source file, then checks every
// selector expression (pkg.Symbol) against the registered API surface.
func (l *Linter) checkSymbols(fset *token.FileSet, file *ast.File) []Diagnostic {
	if l.registry == nil {
		return nil
	}

	imports := resolveImports(file)

	var diags []Diagnostic

	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.SelectorExpr:
			ident, ok := n.X.(*ast.Ident)
			if !ok {
				return true
			}

			importPath, known := imports[ident.Name]
			if !known {
				return true
			}

			pkg, registered := l.registry.Packages[importPath]
			if !registered {
				return true
			}

			name := n.Sel.Name
			_, isFunc := pkg.Functions[name]
			if isFunc || pkg.Types[name] || pkg.Vars[name] {
				return true
			}

			diags = append(diags, Diagnostic{
				Pos:     fset.Position(n.Sel.Pos()),
				End:     fset.Position(n.Sel.End()),
				Message: fmt.Sprintf("%s.%s does not exist", lastSegment(importPath), name),
				Fix:     fmt.Sprintf("Check the %s package for available functions and variables", lastSegment(importPath)),
			})

		case *ast.CallExpr:
			sel, ok := n.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			pkg, found := chainPackage(sel.X, imports, l.registry)
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
		}

		return true
	})

	return diags
}
