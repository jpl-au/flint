package flint

import (
	"fmt"
	"go/ast"
	"go/token"
)

// selfNestingForbidden lists the elements HTML forbids nesting inside
// themselves: an anchor cannot contain an anchor, a button a button, a form a
// form, a label a label. The parser recovers by unnesting, so the rendered
// tree silently differs from the written one. The list is deliberately small
// and hard-coded - these four are the complete set of self-nesting
// prohibitions this check targets, not configuration.
var selfNestingForbidden = map[string]bool{
	"a":      true,
	"button": true,
	"form":   true,
	"label":  true,
}

// checkNesting reports a child built from the same element package passed
// into a constructor of one of the self-nesting-forbidden elements. The trap
// is the paired-constructor convention: a.Static("Back") reads like a text
// node but builds a whole <a> element, so a.New(a.Static("Back")) nests an
// anchor inside an anchor. Only direct arguments to New (and .Add on the same
// chain) are checked - a child passed through a variable is out of reach for
// a purely syntactic check, and the direct form is the shape the confusion
// actually produces.
func (l *Linter) checkNesting(fset *token.FileSet, file *ast.File) []Diagnostic {
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
		alias, path, ok := nestingReceiver(call, imports, l.registry)
		if !ok {
			return true
		}
		pkg := l.registry.Packages[path]
		if !selfNestingForbidden[pkg.Tag] {
			return true
		}

		for _, arg := range call.Args {
			childAlias, fn, ok := chainRootFunc(unparen(arg), imports)
			if !ok || imports[childAlias] != path {
				continue
			}
			if _, ctor := pkg.Functions[fn]; !ctor {
				continue
			}
			d := Diagnostic{
				Check:    "nesting",
				Pos:      fset.Position(arg.Pos()),
				End:      fset.Position(arg.End()),
				Severity: Warning,
				Message:  fmt.Sprintf("%s.%s(...) builds another <%s> element, which nests <%s> inside <%s>. HTML does not allow this, and the browser moves the inner element out.", childAlias, fn, pkg.Tag, pkg.Tag, pkg.Tag),
				Fix:      fmt.Sprintf("Restructure so the two <%s> elements are siblings", pkg.Tag),
			}
			if fn == "Static" || fn == "Text" || fn == "Textf" {
				d.Fix = fmt.Sprintf("For text content inside %s.New, use text.Static or text.Text from fluent/text; %s.%s is the paired constructor that builds a whole <%s> element", alias, childAlias, fn, pkg.Tag)
			}
			diags = append(diags, d)
		}
		return true
	})

	return diags
}

// nestingReceiver resolves a call that takes children into the element
// package it builds: pkg.New(...) directly, or .Add(...) on a chain rooted in
// the package. ok is false for every other call, including child-taking calls
// whose receiver flint cannot resolve.
func nestingReceiver(call *ast.CallExpr, imports map[string]string, reg *Registry) (alias, path string, ok bool) {
	sel, isSel := call.Fun.(*ast.SelectorExpr)
	if !isSel {
		return "", "", false
	}
	switch sel.Sel.Name {
	case "New":
		id, isIdent := sel.X.(*ast.Ident)
		if !isIdent {
			return "", "", false
		}
		p, imported := imports[id.Name]
		if !imported {
			return "", "", false
		}
		if _, registered := reg.Packages[p]; !registered {
			return "", "", false
		}
		return id.Name, p, true
	case "Add":
		a, fn, rooted := chainRootFunc(sel.X, imports)
		if !rooted {
			return "", "", false
		}
		p := imports[a]
		pkg, registered := reg.Packages[p]
		if !registered {
			return "", "", false
		}
		// Only constructors keep the receiver's type equal to the package's
		// element; a foreign-returning function (none among these packages
		// today) would make the receiver something else.
		if _, ctor := pkg.Functions[fn]; !ctor {
			return "", "", false
		}
		return a, p, true
	}
	return "", "", false
}
