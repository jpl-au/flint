package flint

import (
	"fmt"
	"go/ast"
	"go/token"
)

// checkConstructors reports calls to pkg.New() followed by a chain
// of methods that includes one which exists as a package-level
// constructor. For example:
//
//	div.New().Text("hello")                // flagged
//	h3.New().Class("foo").Text("hello")    // flagged - Text could replace New()
//
// Both should use the direct constructor: div.Text("hello") or
// h3.Text("hello").Class("foo"). The linter walks the receiver chain
// of every method call to find a pkg.New() root, then checks whether
// any method in the chain is a package-level constructor.
func (l *Linter) checkConstructors(fset *token.FileSet, file *ast.File) []Diagnostic {
	if l.registry == nil {
		return nil
	}

	imports := resolveImports(file)
	var diags []Diagnostic
	reported := make(map[token.Pos]bool)

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

		// Walk the receiver chain looking for pkg.New() at the root.
		// The chain may contain any number of method calls between
		// New() and the method we're currently inspecting.
		root := sel.X
		hops := 0
		for {
			innerCall, ok := root.(*ast.CallExpr)
			if !ok {
				return true
			}
			innerSel, ok := innerCall.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if innerSel.Sel.Name == "New" {
				// Found the root. Validate it's pkg.New() with no args.
				pkgIdent, ok := innerSel.X.(*ast.Ident)
				if !ok {
					return true
				}
				if len(innerCall.Args) > 0 {
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
				// Is the current method name a constructor in that package?
				if _, isCtor := pkg.Functions[methodName]; !isCtor {
					return true
				}
				// Only report each New() call once, even if multiple
				// methods in the chain could replace it.
				if reported[innerCall.Pos()] {
					return true
				}
				reported[innerCall.Pos()] = true

				// Format the message differently depending on whether
				// the constructor is called directly after New() or
				// after a chain of other methods. Preserves the exact
				// wording of the direct case for backwards compatibility.
				var message, fix string
				if hops == 0 {
					message = fmt.Sprintf("use %s.%s(...) directly instead of %s.New().%s(...)", pkgIdent.Name, methodName, pkgIdent.Name, methodName)
					fix = fmt.Sprintf("%s.%s(...) is a constructor that replaces New().%s(...)", pkgIdent.Name, methodName, methodName)
				} else {
					message = fmt.Sprintf("use %s.%s(...) directly instead of %s.New()...%s(...)", pkgIdent.Name, methodName, pkgIdent.Name, methodName)
					fix = fmt.Sprintf("%s.%s(...) is a constructor that replaces New().%s(...); chain remaining methods on the result", pkgIdent.Name, methodName, methodName)
				}

				diags = append(diags, Diagnostic{
					Pos:      fset.Position(innerSel.Sel.Pos()),
					End:      fset.Position(call.End()),
					Severity: Warning,
					Message:  message,
					Fix:      fix,
				})
				return true
			}
			// Not yet at New() - keep walking up the chain.
			root = innerSel.X
			hops++
		}
	})

	return diags
}
