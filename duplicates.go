package flint

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"
	"strings"
)

// checkDuplicateAttrs reports an attribute set twice within a single chain.
// Most attribute methods store one value, so the later call silently
// overwrites the earlier one: div.New().ID("a").ID("b") renders id="b" and the
// first call is dead. A SetAttribute whose key a dedicated method already set
// is worse: the field and the raw attribute both render, producing a duplicate
// attribute in the output (browsers keep the first and drop the rest).
// Accumulating methods (Class, Style) concatenate across calls and are exempt
// from the repeated-call rule.
func (l *Linter) checkDuplicateAttrs(fset *token.FileSet, file *ast.File) []Diagnostic {
	if l.registry == nil {
		return nil
	}

	imports := resolveImports(file)
	locals := fluentLocalPackages(file, imports, l.registry)

	var diags []Diagnostic
	seen := map[*ast.CallExpr]bool{}

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || seen[call] {
			return true
		}

		// Walk the chain spine leftward from its outermost call. Inspect
		// visits parents first, so an unseen call is a chain head; marking
		// the spine ensures each chain is processed exactly once. Arguments
		// are not part of the spine and form their own chains.
		var spine []*ast.CallExpr
		for cur := call; ; {
			seen[cur] = true
			sel, ok := cur.Fun.(*ast.SelectorExpr)
			if !ok {
				break
			}
			spine = append(spine, cur)
			inner, ok := sel.X.(*ast.CallExpr)
			if !ok {
				break
			}
			cur = inner
		}
		if len(spine) < 2 {
			return true
		}

		// Resolve the chain to a fluent package: rooted at an import, or at
		// a local assigned from one. Duplication is judged per package, so
		// an unresolvable receiver stays quiet rather than guessing.
		pkg, found := chainPackage(call, imports, l.registry)
		if !found {
			root := spine[len(spine)-1]
			if sel, ok := root.Fun.(*ast.SelectorExpr); ok {
				if id, ok := unparen(sel.X).(*ast.Ident); ok {
					pkg, found = locals[id.Name]
				}
			}
			if !found {
				return true
			}
		}

		attrMethodSet := make(map[string]bool, len(pkg.AttrMethods))
		for _, m := range pkg.AttrMethods {
			attrMethodSet[m] = true
		}

		// Replay the chain in source order, remembering the first call of
		// each overwriting attribute method.
		first := map[string]token.Position{}
		for i := len(spine) - 1; i >= 0; i-- {
			c := spine[i]
			sel := c.Fun.(*ast.SelectorExpr)
			m := sel.Sel.Name

			if m == "SetAttribute" || m == "SetAttributeRaw" {
				if len(c.Args) < 1 || !isStringLiteral(c.Args[0]) {
					continue
				}
				lit := c.Args[0].(*ast.BasicLit)
				key, err := strconv.Unquote(lit.Value)
				if err != nil {
					continue
				}
				dedicated := pkg.AttrMethods[strings.ToLower(key)]
				if dedicated == "" {
					continue
				}
				if _, set := first[dedicated]; set {
					diags = append(diags, Diagnostic{
						Check:    "duplicate-attr",
						Pos:      fset.Position(sel.Sel.Pos()),
						End:      fset.Position(c.End()),
						Severity: Warning,
						Message:  fmt.Sprintf("%s(%q, ...) duplicates the .%s() call earlier in this chain; both render, duplicating the %s attribute", m, key, dedicated, key),
						Fix:      fmt.Sprintf("Fold the value into the .%s() call; browsers keep only the first of a duplicated attribute", dedicated),
					})
				}
				continue
			}

			if !attrMethodSet[m] {
				continue
			}
			// Accumulating methods are recorded (a later SetAttribute on the
			// same key still duplicates them) but repeated calls are fine.
			prev, set := first[m]
			if set && !pkg.AccumulatingMethods[m] {
				diags = append(diags, Diagnostic{
					Check:    "duplicate-attr",
					Pos:      fset.Position(sel.Sel.Pos()),
					End:      fset.Position(c.End()),
					Severity: Warning,
					Message:  fmt.Sprintf(".%s() overwrites the value set by the earlier .%s() (line %d); only the last value is rendered", m, m, prev.Line),
					Fix:      fmt.Sprintf("Keep a single .%s() call with the value you want rendered", m),
				})
				continue
			}
			if !set {
				first[m] = fset.Position(sel.Sel.Pos())
			}
		}

		return true
	})

	return diags
}
