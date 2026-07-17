package flint

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// dynamicAttr is the attribute name recorded when a SetAttribute key cannot be
// known statically (it is not a string literal), so a dynamic key still counts
// against its element without inventing a name.
const dynamicAttr = "(dynamic)"

// AttrPair is one use of an HTML attribute on a known fluent element. Element is
// the element's HTML tag (for example "div") and Attribute is the attribute name
// (for example "class"), or dynamicAttr when a SetAttribute key is not a string
// literal.
type AttrPair struct {
	Element   string
	Attribute string
}

// AttrPairs returns every (element, attribute) pair used across the fluent
// element chains in src, one entry per use so the caller can aggregate the
// counts. It records a pair only when the receiver resolves to a known fluent
// element package, so a same-named method on an unrelated type is ignored. A
// dedicated attribute method (Class, Href, ...) contributes its attribute name;
// a SetAttribute or SetAttributeRaw call contributes its string-literal key, or
// dynamicAttr when the key is not a literal. Without a registry it returns nil.
//
// The filename is used only for position information. An error is returned only
// if src cannot be parsed.
func (l *Linter) AttrPairs(filename string, src []byte) ([]AttrPair, error) {
	if l.registry == nil {
		return nil, nil
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.AllErrors)
	if err != nil {
		return nil, err
	}
	// Mirror Source: generated files are not the author's to change, so their
	// attribute usage is not counted.
	if ast.IsGenerated(file) {
		return nil, nil
	}

	imports := resolveImports(file)

	var pairs []AttrPair
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		// The element is only statically certain for an inline chain that roots
		// at a fluent element package, where the package's tag names the element.
		// A local of unknown element type, or a package with no tag, is skipped.
		pkg, found := chainPackage(sel.X, imports, l.registry)
		if !found || pkg.Tag == "" {
			return true
		}

		switch sel.Sel.Name {
		case "SetAttribute", "SetAttributeRaw":
			pairs = append(pairs, AttrPair{Element: pkg.Tag, Attribute: setAttrKey(call)})
		default:
			if attr, ok := l.attrByMethod[sel.Sel.Name]; ok {
				pairs = append(pairs, AttrPair{Element: pkg.Tag, Attribute: attr})
			}
		}
		return true
	})

	return pairs, nil
}

// setAttrKey returns the attribute name of a SetAttribute or SetAttributeRaw
// call: the first argument's string-literal value, or dynamicAttr when the key
// is not a string literal and so cannot be known statically.
func setAttrKey(call *ast.CallExpr) string {
	if len(call.Args) >= 1 {
		if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
			return strings.Trim(lit.Value, "\"'`")
		}
	}
	return dynamicAttr
}

// invertAttrMethods reverses an attribute-to-method map into a
// method-to-attribute map, so a method call like .Class(...) can be traced back
// to the "class" attribute it sets. The forward map is effectively one-to-one in
// the generated registry, so no meaningful collisions occur when reversing it.
func invertAttrMethods(attrMethods map[string]string) map[string]string {
	byMethod := make(map[string]string, len(attrMethods))
	for attr, method := range attrMethods {
		byMethod[method] = attr
	}
	return byMethod
}
