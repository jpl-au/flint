package flint

import (
	"fmt"
	"go/ast"
	"go/token"
)

// checkDeprecated reports uses of deprecated fluent APIs: package-level
// functions and vars recorded in DeprecatedFunctions/DeprecatedVars, and
// element methods recorded in DeprecatedMethods. The registry note names the
// replacement, so the fix carries it verbatim and an agent can self-correct
// without looking anything up.
func (l *Linter) checkDeprecated(fset *token.FileSet, file *ast.File) []Diagnostic {
	if l.registry == nil {
		return nil
	}

	imports := resolveImports(file)
	locals := fluentLocalPackages(file, imports, l.registry)

	var diags []Diagnostic

	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		name := sel.Sel.Name

		// pkg.Symbol: a deprecated function or var referenced directly.
		if ident, ok := sel.X.(*ast.Ident); ok {
			if importPath, known := imports[ident.Name]; known {
				pkg, registered := l.registry.Packages[importPath]
				if !registered {
					return true
				}
				note, deprecated := pkg.DeprecatedFunctions[name]
				if !deprecated {
					note, deprecated = pkg.DeprecatedVars[name]
				}
				if deprecated {
					diags = append(diags, Diagnostic{
						Check:    "deprecated",
						Pos:      fset.Position(sel.Sel.Pos()),
						End:      fset.Position(sel.Sel.End()),
						Severity: Warning,
						Message:  fmt.Sprintf("%s.%s is deprecated", pkgName(importPath), name),
						Fix:      note,
					})
				}
				return true
			}
		}

		// recv.Method: a deprecated method reached through an inline chain
		// rooting at a fluent package, or through a local assigned from one.
		// Deprecation is per package (Type is deprecated on ul, fine on
		// input), so the receiver must resolve to a specific package before
		// the method name means anything.
		pkg, found := chainPackage(sel.X, imports, l.registry)
		if !found {
			if id, ok := unparen(sel.X).(*ast.Ident); ok {
				pkg, found = locals[id.Name]
			}
			if !found {
				return true
			}
		}
		if note, ok := pkg.DeprecatedMethods[name]; ok {
			diags = append(diags, Diagnostic{
				Check:    "deprecated",
				Pos:      fset.Position(sel.Sel.Pos()),
				End:      fset.Position(sel.Sel.End()),
				Severity: Warning,
				Message:  fmt.Sprintf("method %s is deprecated", name),
				Fix:      note,
			})
		}
		return true
	})

	return diags
}

// fluentLocalPackages collects variables assigned anywhere in the file from an
// expression that chains back to a fluent package, mapped to that package's
// registry entry, e.g. t := table.New() records "t" -> the table package. The
// same file-wide, last-assignment-wins approximation as fluentLocals; the
// package identity is kept because deprecation is looked up per package.
func fluentLocalPackages(file *ast.File, imports map[string]string, reg *Registry) map[string]Package {
	locals := make(map[string]Package)
	record := func(lhs, rhs ast.Expr) {
		id, ok := lhs.(*ast.Ident)
		if !ok || id.Name == "_" {
			return
		}
		if _, ok := rhs.(*ast.CallExpr); !ok {
			return
		}
		if pkg, ok := chainPackage(rhs, imports, reg); ok {
			locals[id.Name] = pkg
		}
	}

	ast.Inspect(file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			if len(x.Lhs) == len(x.Rhs) {
				for i := range x.Lhs {
					record(x.Lhs[i], x.Rhs[i])
				}
			}
		case *ast.ValueSpec:
			if len(x.Names) == len(x.Values) {
				for i := range x.Names {
					record(x.Names[i], x.Values[i])
				}
			}
		}
		return true
	})

	return locals
}
