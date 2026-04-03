package flint

import (
	"fmt"
	"go/ast"
	"go/token"
	"maps"
	"strings"
)

// prefixHelper maps an HTML attribute prefix to the dedicated fluent
// method that should be used instead of SetAttribute.
type prefixHelper struct {
	prefix string // e.g. "data-"
	helper string // e.g. "SetData"
}

// prefixHelpers lists attribute prefixes that have dedicated chaining
// methods. When SetAttribute is called with a key matching one of
// these prefixes, the linter suggests the dedicated method instead.
var prefixHelpers = []prefixHelper{
	{"data-", "SetData"},
	{"aria-", "SetAria"},
}

// checkSetAttributeKey reports calls to SetAttribute where the key is a
// known HTML attribute that has a dedicated typed method. Using
// SetAttribute for standard attributes bypasses the struct field that
// fluent manages, which can produce duplicate attributes in the rendered
// output if the typed method is also called.
func checkSetAttributeKey(fset *token.FileSet, file *ast.File) []Diagnostic {
	if activeRegistry == nil {
		return nil
	}

	// Build a combined lookup of all known attribute keys across all
	// element packages. If any element has a typed method for a key,
	// SetAttribute should not be used for that key.
	allAttrMethods := buildAttrMethodsLookup(activeRegistry)

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

		// SetAttribute takes two string arguments: key and value.
		if len(call.Args) < 1 {
			return true
		}

		// The key must be a string literal for us to check it.
		keyLit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || keyLit.Kind != token.STRING {
			return true
		}

		// Strip quotes from the key.
		key := strings.Trim(keyLit.Value, "\"'`")

		// Prefixed attributes have dedicated helpers that support
		// chaining and keep related attributes grouped.
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

		method, known := allAttrMethods[key]
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

// buildAttrMethodsLookup builds a combined map of all known HTML
// attribute keys to their typed method names across all packages.
func buildAttrMethodsLookup(reg *Registry) map[string]string {
	combined := make(map[string]string)
	for _, pkg := range reg.Packages {
		maps.Copy(combined, pkg.AttrMethods)
	}
	return combined
}
