package flint

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"
	"strings"
)

// checkTypedParams reports method calls where a string literal is passed
// to a method that expects a typed enum constant. For example,
// input.New().Type("email") should be input.New().Type(inputtype.Email).
// When the literal matches a predefined value the fix names the exact
// constant. It also reports an enum package's Custom() escape hatch
// re-creating a predefined constant, e.g. inputtype.Custom("email").
func (l *Linter) checkTypedParams(fset *token.FileSet, file *ast.File) []Diagnostic {
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

		methodName := sel.Sel.Name

		// pkg.Custom("value") re-creating a predefined constant: the enum's
		// escape hatch is for genuinely custom values, so a value the package
		// already names should use its constant.
		if ident, ok := sel.X.(*ast.Ident); ok {
			if importPath, known := imports[ident.Name]; known {
				if methodName == "Custom" && len(call.Args) == 1 && isStringLiteral(call.Args[0]) {
					lit := call.Args[0].(*ast.BasicLit)
					if val, err := strconv.Unquote(lit.Value); err == nil {
						if c, ok := enumConstant(l.registry.Packages[importPath].EnumValues, val); ok {
							name := pkgName(importPath)
							diags = append(diags, Diagnostic{
								Check:    "typed-params",
								Pos:      fset.Position(call.Pos()),
								End:      fset.Position(call.End()),
								Severity: Warning,
								Message:  fmt.Sprintf("%s.Custom(%s) re-creates the predefined constant %s.%s", name, lit.Value, name, c),
								Fix:      fmt.Sprintf("Replace %s.Custom(%s) with %s.%s", name, lit.Value, name, c),
							})
						}
					}
				}
				return true
			}
		}

		// Find the originating package for this method chain.
		pkg, found := chainPackage(sel.X, imports, l.registry)
		if !found {
			return true
		}

		// On a multi-element package, resolve against the element the chain
		// roots at rather than the package-wide union. Only flag a typed-param
		// when the method exists on that element, because symbols.go already
		// reports a nonexistent method and a typed-constant hint for it would
		// be misleading. The element's own entry also settles which enum a
		// method takes when elements disagree: on an animation element Fill is
		// the animation fill mode, while on a shape it is the paint, a string.
		typedParams := pkg.TypedParams
		if _, fn, ok := chainRootFunc(sel.X, imports); ok {
			if ret, known := pkg.FuncReturns[fn]; known {
				if methods := l.registry.typeMethods(ret); methods != nil {
					if _, exists := methods[methodName]; !exists {
						return true
					}
				}
			}
			if el, ok := elementOfFunc(pkg, fn); ok {
				typedParams = el.TypedParams
			}
		}

		// Check if this method expects a typed parameter.
		enumPkg, hasTyped := typedParams[methodName]
		if !hasTyped {
			return true
		}

		// Check if the first argument is a string literal.
		if len(call.Args) == 0 {
			return true
		}

		arg := call.Args[0]
		if !isStringLiteral(arg) {
			return true
		}

		lit := arg.(*ast.BasicLit)
		fix := fmt.Sprintf("Use a value from the %s package (e.g., %s.X) or %s.Custom(...)", enumPkg, enumPkg, enumPkg)
		if val, err := strconv.Unquote(lit.Value); err == nil {
			if c, ok := enumConstant(l.enumValues[enumPkg], val); ok {
				fix = fmt.Sprintf("Replace %s with %s.%s", lit.Value, enumPkg, c)
			}
		}
		diags = append(diags, Diagnostic{
			Check:    "typed-params",
			Pos:      fset.Position(arg.Pos()),
			End:      fset.Position(arg.End()),
			Severity: Warning,
			Message:  fmt.Sprintf(".%s() expects a typed constant, not a string literal %s", methodName, lit.Value),
			Fix:      fix,
		})

		return true
	})

	return diags
}

// enumConstant resolves a raw string to the enum constant that renders it: an
// exact match on the rendered value first, then a case-insensitive one.
// Charset names and similar are case-insensitive in HTML; values whose case is
// significant, like ol's "a" and "A" numbering types, are distinct options and
// so always hit the exact match first.
func enumConstant(values map[string]string, raw string) (string, bool) {
	if c, ok := values[raw]; ok {
		return c, true
	}
	for v, c := range values {
		if strings.EqualFold(v, raw) {
			return c, true
		}
	}
	return "", false
}
