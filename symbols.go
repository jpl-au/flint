package flint

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
)

// checkSymbols reports references to symbols that do not exist in the
// registry. It resolves imports in the source file, then checks every
// selector expression (pkg.Symbol) against the registered API surface.
//
// This check requires a registry to be set. If no registry is configured
// the check is silently skipped.
func checkSymbols(fset *token.FileSet, file *ast.File) []Diagnostic {
	if activeRegistry == nil {
		return nil
	}

	// Build a map from local package name to import path.
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

		localName := ident.Name
		symbolName := sel.Sel.Name

		importPath, known := imports[localName]
		if !known {
			// Not an import we track - could be a variable or receiver.
			return true
		}

		pkg, registered := activeRegistry.Packages[importPath]
		if !registered {
			// Import path is not in the registry. We only validate
			// packages we know about, so skip silently.
			return true
		}

		// Check if the symbol is a known function, type, or variable.
		_, isFunc := pkg.Functions[symbolName]
		if isFunc || pkg.Types[symbolName] || pkg.Vars[symbolName] {
			return true
		}

		diags = append(diags, Diagnostic{
			Pos:     fset.Position(sel.Sel.Pos()),
			End:     fset.Position(sel.Sel.End()),
			Message: fmt.Sprintf("%s.%s does not exist", lastSegment(importPath), symbolName),
			Fix:     fmt.Sprintf("Check the %s package for available functions and variables", lastSegment(importPath)),
		})

		return true
	})

	// Check method calls: expr.Method() where expr is a call to a
	// registered constructor. We walk again looking for chained calls.
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		// Find the root package of a method chain by walking left.
		pkg, found := resolveChainPackage(sel.X, imports, activeRegistry)
		if !found {
			return true
		}

		methodName := sel.Sel.Name
		if pkg.Methods == nil || pkg.Methods[methodName] {
			return true
		}

		// Also allow functions - some calls look like methods but are
		// package-level (e.g., after a constructor returns).
		if _, ok := pkg.Functions[methodName]; ok {
			return true
		}

		diags = append(diags, Diagnostic{
			Pos:     fset.Position(sel.Sel.Pos()),
			End:     fset.Position(sel.Sel.End()),
			Message: fmt.Sprintf("method %s does not exist on this element", methodName),
			Fix:     "Check the element package for available methods",
		})

		return true
	})

	return diags
}

// resolveImports builds a map from local package names to import paths
// for all imports in the file.
func resolveImports(file *ast.File) map[string]string {
	imports := make(map[string]string)
	for _, imp := range file.Imports {
		// Remove quotes from import path.
		path := strings.Trim(imp.Path.Value, `"`)

		var localName string
		if imp.Name != nil {
			localName = imp.Name.Name
		} else {
			// Default: last segment of the import path.
			localName = lastSegment(path)
		}

		imports[localName] = path
	}
	return imports
}

// resolveChainPackage walks leftward through a selector/call chain to
// find the originating package. For example, in div.New().Class("x"),
// starting from the .Class selector's X (which is div.New()), this
// resolves back to the div package.
func resolveChainPackage(expr ast.Expr, imports map[string]string, reg *Registry) (Package, bool) {
	switch e := expr.(type) {
	case *ast.CallExpr:
		// Recurse into the function being called.
		return resolveChainPackage(e.Fun, imports, reg)

	case *ast.SelectorExpr:
		// Could be pkg.Constructor or expr.Method - try pkg first.
		if ident, ok := e.X.(*ast.Ident); ok {
			if importPath, ok := imports[ident.Name]; ok {
				if pkg, ok := reg.Packages[importPath]; ok {
					return pkg, true
				}
			}
		}
		// Otherwise recurse left.
		return resolveChainPackage(e.X, imports, reg)

	case *ast.Ident:
		if importPath, ok := imports[e.Name]; ok {
			if pkg, ok := reg.Packages[importPath]; ok {
				return pkg, true
			}
		}
	}

	return Package{}, false
}

// lastSegment returns the last path segment of an import path.
func lastSegment(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}
