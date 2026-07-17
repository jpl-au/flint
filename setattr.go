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
// subsequent method call on the result will fail to compile. Only
// flags calls on fluent elements (scoped via the registry).
func (l *Linter) checkSetAttrChain(fset *token.FileSet, file *ast.File) []Diagnostic {
	var diags []Diagnostic

	// Scope to fluent packages when a registry is available.
	var imports map[string]string
	if l.registry != nil {
		imports = resolveImports(file)
	}

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

		if innerSel.Sel.Name != "SetAttribute" && innerSel.Sel.Name != "SetAttributeRaw" {
			return true
		}

		// Scope check: verify the receiver traces back to a fluent package.
		if imports != nil && l.registry != nil {
			if _, found := chainPackage(innerSel.X, imports, l.registry); !found {
				return true
			}
		}

		diags = append(diags, Diagnostic{
			Check:   "setattr-chain",
			Pos:     fset.Position(sel.Sel.Pos()),
			End:     fset.Position(sel.Sel.End()),
			Message: innerSel.Sel.Name + " does not return the element; cannot chain ." + sel.Sel.Name + "() after it",
			Fix:     "Call " + innerSel.Sel.Name + " separately, or use SetData/SetAria which do support chaining",
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

// prefixHelpers lists the attribute key prefixes that map to a dedicated
// fluent helper method. The loop checks each prefix in order and returns
// on the first match, so more specific prefixes should appear first.
var prefixHelpers = []prefixHelper{
	{"data-", "SetData"},
	{"aria-", "SetAria"},
}

// checkSetAttrKey reports calls to SetAttribute or SetAttributeRaw where the
// key is a known HTML attribute that has a dedicated typed method.
func (l *Linter) checkSetAttrKey(fset *token.FileSet, file *ast.File) []Diagnostic {
	if l.registry == nil {
		return nil
	}

	imports := resolveImports(file)
	locals := fluentLocals(file, imports, l.registry)

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

		name := sel.Sel.Name
		if name != "SetAttribute" && name != "SetAttributeRaw" {
			return true
		}

		// Scope to fluent receivers: an inline chain that roots at a fluent
		// package, or a local assigned from one. SetAttribute is a common
		// method name in other libraries (DOM wrappers, XML builders), and
		// their attributes have no fluent typed method to suggest.
		if !l.isFluentReceiver(sel.X, imports, locals) {
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
					Check:    "setattr-key",
					Pos:      fset.Position(keyLit.Pos()),
					End:      fset.Position(call.End()),
					Severity: Warning,
					Message:  fmt.Sprintf("%s(%q, ...) should use %s(%q, ...) instead", name, key, p.helper, suffix),
					Fix:      fmt.Sprintf("%s supports chaining and groups %s attributes; %s does not return the element", p.helper, strings.TrimSuffix(p.prefix, "-"), name),
				})
				return true
			}
		}

		method, known := l.attrMethods[key]
		if !known {
			return true
		}

		// The raw variant deserves the sharper warning: it bypasses the typed
		// method, the set-time escaping AND the URL scheme filter, so a known
		// attribute written through it is the most exposed shape flint can see.
		fix := fmt.Sprintf(".%s() manages this attribute through a struct field, and for URL attributes filters the scheme against the allowlist; SetAttribute escapes the value but does not filter and can produce duplicate attributes, so reach for it only as the deliberate escaped-but-unfiltered override (SetAttributeRaw skips escaping too)", method)
		if name == "SetAttributeRaw" {
			fix = fmt.Sprintf(".%s() manages this attribute through a struct field, escapes the value and for URL attributes filters the scheme against the allowlist; SetAttributeRaw does none of that, so keep it only when these exact raw bytes are the point", method)
		}
		diags = append(diags, Diagnostic{
			Check:    "setattr-key",
			Pos:      fset.Position(keyLit.Pos()),
			End:      fset.Position(call.End()),
			Severity: Warning,
			Message:  fmt.Sprintf("%s(%q, ...) bypasses the dedicated field; use .%s() instead", name, key, method),
			Fix:      fix,
		})

		return true
	})

	return diags
}

// mergeAttrMethods builds a combined map of all known HTML attribute
// keys to their typed method names across all packages.
func mergeAttrMethods(reg *Registry) map[string]string {
	combined := make(map[string]string)
	for _, pkg := range reg.Packages {
		maps.Copy(combined, pkg.AttrMethods)
	}
	return combined
}

// isFluentReceiver reports whether expr plausibly evaluates to a fluent
// element: an inline chain rooting at a fluent package (div.New().ID("x")),
// or a bare local whose name was assigned from such a chain (d := div.New()).
// A local of unresolvable origin (a parameter, a call into user code) does
// not qualify, so the checks that use this stay quiet rather than guess.
func (l *Linter) isFluentReceiver(expr ast.Expr, imports map[string]string, locals map[string]bool) bool {
	if _, ok := chainPackage(expr, imports, l.registry); ok {
		return true
	}
	if id, ok := unparen(expr).(*ast.Ident); ok {
		return locals[id.Name]
	}
	return false
}

// fluentLocals collects the names of variables assigned anywhere in the file
// from an expression that chains back to a fluent package, e.g. d := div.New()
// or el = span.New().Class("x"). Names are collected file-wide, not per scope:
// a rare same-named non-fluent variable in another function may slip through,
// which errs towards the pre-existing behaviour only in files that already
// build fluent elements.
func fluentLocals(file *ast.File, imports map[string]string, reg *Registry) map[string]bool {
	locals := make(map[string]bool)
	record := func(lhs, rhs ast.Expr) {
		id, ok := lhs.(*ast.Ident)
		if !ok || id.Name == "_" {
			return
		}
		if _, ok := rhs.(*ast.CallExpr); !ok {
			return
		}
		if _, ok := chainPackage(rhs, imports, reg); ok {
			locals[id.Name] = true
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
