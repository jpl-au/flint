package flint

import (
	"fmt"
	"go/ast"
	"go/token"
	"sort"
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
				Check:    "typed-constructors",
				Pos:      fset.Position(sel.Sel.Pos()),
				End:      fset.Position(call.End()),
				Severity: Warning,
				Message:  fmt.Sprintf("use %s.%s(...) instead of %s.New(...) for type-safe child nesting", pkgIdent.Name, ctor, pkgIdent.Name),
				Fix:      fmt.Sprintf("%s.%s(...) accepts only %s elements, catching nesting errors at compile time", pkgIdent.Name, ctor, childPkg),
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

// checkDeferredConstructors reports pkg.New(...) whose only child is a
// direct node.Map or node.Funcs call, where the package has a deferred
// typed constructor for that child run (ItemsOf/ItemsFunc, RowsOf/RowsFunc,
// and so on). The deferred forms keep node.Map's render-time evaluation and
// add a compile-checked child type, so the call-site cost is redeclaring
// the mapper's return type - which the fix text states, because the
// mapper's declaration is not visible to a single-file AST check.
//
// A chained .Dynamic(...) on the node.Map or node.Funcs call keeps the
// tracking key on the child run; moving the run into a typed constructor
// moves the key to the element and changes the diff engine's patch region,
// so keyed runs are not reported.
func (l *Linter) checkDeferredConstructors(fset *token.FileSet, file *ast.File) []Diagnostic {
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
		if !ok || sel.Sel.Name != "New" || len(call.Args) != 1 {
			return true
		}
		pkgIdent, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		importPath, known := imports[pkgIdent.Name]
		if !known {
			return true
		}
		pkg, registered := l.registry.Packages[importPath]
		if !registered {
			return true
		}

		nodeAlias, nodeFn := directNodeCall(call.Args[0], imports)
		if nodeFn != "Map" && nodeFn != "Funcs" {
			return true
		}
		suffix := "Of"
		if nodeFn == "Funcs" {
			suffix = "Func"
		}

		// Constructors whose deferred form exists, in a deterministic order.
		// The registry lists the deferred form as a function, so its presence
		// is what marks a base. tr carries two (Cells and Headers); the first
		// is suggested and the rest are named in the fix text, because the
		// child type the mapper returns is not visible here.
		var bases []string
		for name := range pkg.TypedConstructors {
			if _, ok := pkg.Functions[name+suffix]; ok {
				bases = append(bases, name)
			}
		}
		if len(bases) == 0 {
			return true
		}
		sort.Strings(bases)

		alias := pkgIdent.Name
		base := bases[0]
		suggest := base + suffix
		childType := "*" + pkg.TypedConstructors[base] + ".Element"

		var msg, fix string
		if nodeFn == "Map" {
			msg = fmt.Sprintf("use %s.%s(...) instead of %s.New(%s.Map(...)) for type-safe child nesting", alias, suggest, alias, nodeAlias)
			fix = fmt.Sprintf("%s.%s runs the mapper at render time, skips a nil result, and checks the child type at compile time; redeclare the mapper to return %s instead of node.Node",
				alias, suggest, childType)
		} else {
			msg = fmt.Sprintf("use %s.%s(...) instead of %s.New(%s.Funcs(...)) for type-safe child nesting", alias, suggest, alias, nodeAlias)
			fix = fmt.Sprintf("%s.%s runs the function at render time and checks the child type at compile time; redeclare the function to return []%s instead of []node.Node",
				alias, suggest, childType)
		}
		for _, other := range bases[1:] {
			fix += fmt.Sprintf("; use %s.%s%s for %s children", alias, other, suffix, "*"+pkg.TypedConstructors[other]+".Element")
		}

		diags = append(diags, Diagnostic{
			Check:    "typed-constructors",
			Pos:      fset.Position(sel.Sel.Pos()),
			End:      fset.Position(call.End()),
			Severity: Warning,
			Message:  msg,
			Fix:      fix,
		})

		return true
	})

	return diags
}

// directNodeCall returns the import alias and function name of arg when it
// is a direct call into the fluent node package (node.Map, node.Funcs). A
// chained call such as node.Map(...).Dynamic("k") returns empty strings,
// which keeps keyed runs unreported.
func directNodeCall(arg ast.Expr, imports map[string]string) (alias, name string) {
	call, ok := arg.(*ast.CallExpr)
	if !ok {
		return "", ""
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", ""
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", ""
	}
	if imports[ident.Name] != nodeImportPath {
		return "", ""
	}
	return ident.Name, sel.Sel.Name
}
