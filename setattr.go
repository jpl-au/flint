package flint

import (
	"fmt"
	"go/ast"
	"go/token"
	"maps"
	"strings"
)

// checkSetAttrChain reports attempts to chain method calls after
// SetAttribute. SetAttribute does not return the element, so any
// subsequent method call on the result will fail to compile.
func checkSetAttrChain(fset *token.FileSet, file *ast.File) []Diagnostic {
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

		innerCall, ok := sel.X.(*ast.CallExpr)
		if !ok {
			return true
		}

		innerSel, ok := innerCall.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		if innerSel.Sel.Name != "SetAttribute" {
			return true
		}

		diags = append(diags, Diagnostic{
			Pos:     fset.Position(sel.Sel.Pos()),
			End:     fset.Position(sel.Sel.End()),
			Message: "SetAttribute does not return the element; cannot chain ." + sel.Sel.Name + "() after it",
			Fix:     "Call SetAttribute separately, or use SetData/SetAria which do support chaining",
		})

		return true
	})

	return diags
}

// prefixHelper maps an HTML attribute prefix to the dedicated fluent
// method that should be used instead of SetAttribute.
type prefixHelper struct {
	prefix string
	helper string
}

var prefixHelpers = []prefixHelper{
	{"data-", "SetData"},
	{"aria-", "SetAria"},
}

// checkSetAttrKey reports calls to SetAttribute where the key is a
// known HTML attribute that has a dedicated typed method.
func checkSetAttrKey(fset *token.FileSet, file *ast.File) []Diagnostic {
	if activeRegistry == nil {
		return nil
	}

	all := attrMethods(activeRegistry)
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

		if sel.Sel.Name != "SetAttribute" {
			return true
		}

		if len(call.Args) < 1 {
			return true
		}

		keyLit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || keyLit.Kind != token.STRING {
			return true
		}

		key := strings.Trim(keyLit.Value, "\"'`")

		for _, p := range prefixHelpers {
			if suffix, ok := strings.CutPrefix(key, p.prefix); ok {
				diags = append(diags, Diagnostic{
					Pos:     fset.Position(keyLit.Pos()),
					End:     fset.Position(call.End()),
					Message: fmt.Sprintf("SetAttribute(%q, ...) should use %s(%q, ...) instead", key, p.helper, suffix),
					Fix:     fmt.Sprintf("%s supports chaining and groups %s attributes; SetAttribute does not return the element", p.helper, strings.TrimSuffix(p.prefix, "-")),
				})
				return true
			}
		}

		method, known := all[key]
		if !known {
			return true
		}

		diags = append(diags, Diagnostic{
			Pos:     fset.Position(keyLit.Pos()),
			End:     fset.Position(call.End()),
			Message: fmt.Sprintf("SetAttribute(%q, ...) bypasses the dedicated field; use .%s() instead", key, method),
			Fix:     fmt.Sprintf(".%s() manages this attribute through a struct field; SetAttribute writes to the generic attribute slice and can produce duplicate attributes", method),
		})

		return true
	})

	return diags
}

// attrMethods builds a combined map of all known HTML attribute keys
// to their typed method names across all packages.
func attrMethods(reg *Registry) map[string]string {
	combined := make(map[string]string)
	for _, pkg := range reg.Packages {
		maps.Copy(combined, pkg.AttrMethods)
	}
	return combined
}
