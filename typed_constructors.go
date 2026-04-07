package flint

import (
	"fmt"
	"go/ast"
	"go/token"
)

// checkTypedConstructors reports calls to pkg.New(children...) where all
// children come from the same child package and a typed constructor
// exists that accepts that child type directly. For example,
// ul.New(li.Text("a"), li.Text("b")) should be ul.Items(li.Text("a"), li.Text("b")).
func (l *Linter) checkTypedConstructors(fset *token.FileSet, file *ast.File) []Diagnostic {
	if l.registry == nil {
		return nil
	}

	imports := resolveImports(file)
	// Build reverse map: package alias -> short package name.
	aliasToShort := make(map[string]string)
	for alias, importPath := range imports {
		aliasToShort[alias] = lastSegment(importPath)
	}

	var diags []Diagnostic

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Match pkg.New(args...) where args is non-empty.
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "New" {
			return true
		}

		pkgIdent, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}

		if len(call.Args) == 0 {
			return true
		}

		importPath, known := imports[pkgIdent.Name]
		if !known {
			return true
		}

		pkg, registered := l.registry.Packages[importPath]
		if !registered || len(pkg.TypedConstructors) == 0 {
			return true
		}

		// Check if all arguments are calls to a single child package.
		childPkg := ""
		uniform := true
		for _, arg := range call.Args {
			cp := callPackage(arg)
			if cp == "" {
				uniform = false
				break
			}
			if childPkg == "" {
				childPkg = cp
			} else if cp != childPkg {
				uniform = false
				break
			}
		}

		if !uniform || childPkg == "" {
			return true
		}

		// Look up the short package name for the child alias.
		childShort, ok := aliasToShort[childPkg]
		if !ok {
			return true
		}

		// Find the best typed constructor for this child package.
		// When multiple constructors accept the same child type
		// (e.g. ol has Items, Decimal, LowerAlpha all accepting li),
		// prefer the plain collection constructor over styled variants.
		ctor := bestTypedConstructor(pkg.TypedConstructors, childShort)
		if ctor != "" {
			diags = append(diags, Diagnostic{
				Pos:     fset.Position(sel.Sel.Pos()),
				End:     fset.Position(call.End()),
				Message: fmt.Sprintf("use %s.%s(...) instead of %s.New(...) for type-safe child nesting", pkgIdent.Name, ctor, pkgIdent.Name),
				Fix:     fmt.Sprintf("%s.%s(...) accepts only %s elements, catching nesting errors at compile time", pkgIdent.Name, ctor, childPkg),
			})
		}

		return true
	})

	return diags
}

// plainConstructors lists the canonical typed constructor names that
// should be preferred when multiple constructors accept the same child
// type. These are the "collection" constructors added specifically for
// type safety, as opposed to styled variants like Decimal or LowerAlpha.
var plainConstructors = map[string]bool{
	"Items":   true,
	"Rows":    true,
	"Cells":   true,
	"Headers": true,
	"Options": true,
	"Cols":    true,
}

// bestTypedConstructor finds the best constructor to suggest for a
// given child package. Prefers plain collection constructors (Items,
// Rows, etc.) over styled variants (Decimal, LowerAlpha, etc.).
func bestTypedConstructor(ctors map[string]string, childPkg string) string {
	var fallback string
	for name, target := range ctors {
		if target != childPkg {
			continue
		}
		if plainConstructors[name] {
			return name
		}
		if fallback == "" || name < fallback {
			fallback = name
		}
	}
	return fallback
}

// callPackage returns the package alias for a call expression like
// pkg.Func(...) or pkg.Func(...).Method(...). Returns empty string
// if the expression is not a package-qualified call.
func callPackage(expr ast.Expr) string {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return ""
	}

	switch fn := call.Fun.(type) {
	case *ast.SelectorExpr:
		// pkg.Func(...) or expr.Method(...)
		if ident, ok := fn.X.(*ast.Ident); ok {
			return ident.Name
		}
		// Chained: pkg.Func(...).Method(...) - recurse into receiver
		return callPackage(fn.X)
	}

	return ""
}
