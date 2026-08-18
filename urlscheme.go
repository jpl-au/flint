package flint

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"
	"strings"
)

// checkURLScheme reports URL literals that fluent's runtime filter will
// reject. URL-valued sinks (href, src, action, ...) pass their value through
// a positive scheme allowlist at set time; a literal with any other scheme is
// silently replaced by the #fluent-unsafe-url sentinel, so the written URL
// never reaches the output. Nothing fails at build time, and the swap is
// visible only when someone follows the link.
func (l *Linter) checkURLScheme(fset *token.FileSet, file *ast.File) []Diagnostic {
	if l.registry == nil {
		return nil
	}

	imports := resolveImports(file)
	locals := fluentLocalPackages(file, imports, l.registry)

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

		// Resolve the call to a registry package and the index of its URL
		// parameter: a direct package function (embed.Flash), or a method
		// on a chain or local rooted at a fluent package (.Href).
		var idx int
		var isURL bool
		var context string
		if ident, ok := sel.X.(*ast.Ident); ok && imports[ident.Name] != "" {
			importPath := imports[ident.Name]
			idx, isURL = l.registry.Packages[importPath].URLFunctions[name]
			context = pkgName(importPath) + "." + name
		} else {
			pkg, found := chainPackage(sel.X, imports, l.registry)
			if !found {
				if id, ok := unparen(sel.X).(*ast.Ident); ok {
					pkg, found = locals[id.Name]
				}
			}
			if !found {
				return true
			}
			idx, isURL = pkg.URLMethods[name]
			context = "." + name + "()"
		}
		if !isURL || len(call.Args) <= idx || !isStringLiteral(call.Args[idx]) {
			return true
		}

		lit := call.Args[idx].(*ast.BasicLit)
		val, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}
		scheme, rejected := rejectedScheme(val)
		if !rejected {
			return true
		}

		diags = append(diags, Diagnostic{
			Check:    "url-scheme",
			Pos:      fset.Position(lit.Pos()),
			End:      fset.Position(lit.End()),
			Severity: Warning,
			Message:  fmt.Sprintf("%s is given a %q URL. Fluent rejects this scheme and renders \"#fluent-unsafe-url\" instead.", context, scheme+":"),
			Fix:      "Use http, https, mailto, tel, sms or a relative URL. To set a custom scheme on purpose, use SetAttribute, which escapes the value but does not check the scheme.",
		})
		return true
	})

	return diags
}

// rejectedScheme mirrors the scheme decision of fluent's node.FilterURL
// (flint deliberately has no dependency on fluent itself): trim leading and
// trailing bytes <= 0x20 as the WHATWG URL parser does, treat a missing colon
// or a '/', '?' or '#' before it as a relative URL, and allow only http,
// https, mailto, tel and sms. It returns the offending scheme and true when
// the runtime would reject the value.
func rejectedScheme(s string) (string, bool) {
	start, end := 0, len(s)
	for start < end && s[start] <= 0x20 {
		start++
	}
	for end > start && s[end-1] <= 0x20 {
		end--
	}
	t := s[start:end]
	if t == "" {
		return "", false
	}

	colon := strings.IndexByte(t, ':')
	if colon < 0 {
		return "", false
	}
	for i := range colon {
		switch t[i] {
		case '/', '?', '#':
			return "", false
		}
	}

	scheme := t[:colon]
	switch {
	case strings.EqualFold(scheme, "http"),
		strings.EqualFold(scheme, "https"),
		strings.EqualFold(scheme, "mailto"),
		strings.EqualFold(scheme, "tel"),
		strings.EqualFold(scheme, "sms"):
		return "", false
	}
	return scheme, true
}
