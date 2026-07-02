package flint

import (
	"fmt"
	"go/ast"
	"go/token"
)

// checkArity reports calls to registered functions where the number of
// arguments does not match the expected count. Variadic functions
// (arity -1) accept any number of arguments and are not checked.
func (l *Linter) checkArity(fset *token.FileSet, file *ast.File) []Diagnostic {
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

		pkg, registered := l.registry.Packages[importPath]
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
				Message: fmt.Sprintf("%s.%s() expects %d argument(s), got %d", pkgName(importPath), funcName, expected, got),
				Fix:     fmt.Sprintf("Check the %s.%s signature for the correct number of arguments", pkgName(importPath), funcName),
			})
		}

		return true
	})

	return diags
}

// checkMethodArity reports chained method calls whose argument count does not
// match the registry. The receiver's type is resolved the same way checkSymbols
// resolves it: the chain's root constructor return type when recorded
// (FuncReturns -> TypeMethods), else the flat method set - but the flat set is
// trusted only for element packages, where methods returning the element keep
// the receiver's type certain. Variadic methods (arity -1) are not checked, and
// a method absent from the resolved set is checkSymbols' report, not ours.
func (l *Linter) checkMethodArity(fset *token.FileSet, file *ast.File) []Diagnostic {
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
		if !ok {
			return true
		}

		// A direct pkg.Func call is function arity, handled by checkArity.
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

		expected, known := 0, false
		if _, fn, ok := chainRootFunc(sel.X, imports); ok {
			if ret, recorded := pkg.FuncReturns[fn]; recorded {
				if methods := l.registry.typeMethods(ret); methods != nil {
					e, exists := methods[method]
					if !exists {
						return true
					}
					expected, known = e, true
				}
			}
		}
		if !known {
			if !pkg.Types["Element"] && pkg.Tag == "" {
				return true
			}
			e, exists := pkg.Methods[method]
			if !exists {
				return true
			}
			expected = e
		}

		if expected < 0 {
			return true
		}
		// A spread argument's element count is unknowable statically.
		if call.Ellipsis.IsValid() {
			return true
		}

		got := len(call.Args)
		if got != expected {
			diags = append(diags, Diagnostic{
				Pos:     fset.Position(call.Lparen),
				End:     fset.Position(call.Rparen),
				Message: fmt.Sprintf(".%s() expects %d argument(s), got %d", method, expected, got),
				Fix:     fmt.Sprintf("Check the %s method's signature for the correct number of arguments", method),
			})
		}

		return true
	})

	return diags
}
