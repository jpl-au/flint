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
				Message: fmt.Sprintf("%s.%s does not exist", pkgName(importPath), name),
				Fix:     fmt.Sprintf("Check the %s package for available functions and variables", pkgName(importPath)),
			})

		case *ast.CallExpr:
			sel, ok := n.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			// If sel.X is a direct package identifier (e.g. security.CleanUGC),
			// the SelectorExpr branch above already produced the right
			// diagnostic. The chained-call check below is for chained
			// expressions like div.New().Class(x), where sel.X is itself a
			// CallExpr, not a package Ident.
			if ident, ok := sel.X.(*ast.Ident); ok {
				if _, isPkg := imports[ident.Name]; isPkg {
					return true
				}
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

			// When the receiver is a direct package-function call whose return
			// type is recorded, validate the method against that type exactly -
			// security.PlainText returns a node.Node, so .Render() is valid and
			// .Frobnicate() is a genuine error.
			if alias, fn, ok := rootPackageFunc(sel.X, imports); ok {
				if ret, known := pkg.FuncReturns[fn]; known {
					if methods := l.registry.typeMethods(ret); methods != nil {
						if methods[method] {
							return true
						}
						diags = append(diags, Diagnostic{
							Pos:     fset.Position(sel.Sel.Pos()),
							End:     fset.Position(sel.Sel.End()),
							Message: fmt.Sprintf("method %s does not exist on %s, returned by %s.%s", method, shortType(ret), alias, fn),
							Fix:     fmt.Sprintf("check the methods available on %s", shortType(ret)),
						})
						return true
					}
				}
			}

			// A non-element package groups functions and methods that may return
			// foreign types - security.PlainText returns a node.Node, and the
			// Cleaner's own Clean method does too. flint has no return-type
			// information beyond what FuncReturns records, so the receiver's type
			// here is a guess: hedge with a warning rather than assert a hard
			// error. Element packages keep the firm error - their constructors
			// and methods return the element by construction, so the registered
			// method set is authoritative.
			isElementPkg := pkg.Types["Element"] || pkg.Tag != ""
			if !isElementPkg {
				d := Diagnostic{
					Pos:      fset.Position(sel.Sel.Pos()),
					End:      fset.Position(sel.Sel.End()),
					Severity: Warning,
					Message:  fmt.Sprintf("method %s may not exist on this value (flint cannot resolve the receiver's type)", method),
					Fix:      fmt.Sprintf("verify the receiver's type; if it has %s this warning is a false positive", method),
				}
				if alias, fn, ok := rootPackageFunc(sel.X, imports); ok {
					d.Message = fmt.Sprintf("method %s may not exist on the value returned by %s.%s (flint cannot resolve its return type)", method, alias, fn)
					d.Fix = fmt.Sprintf("verify what %s.%s returns; if its type has %s (for example a node from another package) this warning is a false positive", alias, fn, method)
				}
				diags = append(diags, d)
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

// rootPackageFunc reports the package alias and function name when expr is a
// direct package-level function call, e.g. security.PlainText(s) yields
// ("security", "PlainText", true). It is false for method-chain receivers such
// as security.New().Allow(...), where the convention that methods return the
// element keeps the receiver's type reliable.
func rootPackageFunc(expr ast.Expr, imports map[string]string) (string, string, bool) {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return "", "", false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", "", false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", "", false
	}
	if _, isPkg := imports[id.Name]; !isPkg {
		return "", "", false
	}
	return id.Name, sel.Sel.Name, true
}

// shortType renders a path-qualified type as pkg.Type for diagnostics, e.g.
// "github.com/jpl-au/fluent/node.Node" becomes "node.Node".
func shortType(qualified string) string {
	i := strings.LastIndex(qualified, ".")
	if i < 0 {
		return qualified
	}
	return lastSegment(qualified[:i]) + qualified[i:]
}
