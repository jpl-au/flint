package flint

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// InfoSections lists the section names accepted by Info. Each section
// has a long-form canonical name and an optional short-form alias. The
// map key is the name the user types; the value is the canonical form.
var InfoSections = map[string]string{
	"types":              "types",
	"constructors":       "constructors",
	"ctors":              "constructors",
	"typed-constructors": "typed-constructors",
	"typed":              "typed-constructors",
	"methods":            "methods",
	"attributes":         "attributes",
	"attrs":              "attributes",
	"vars":               "vars",
	"elements":           "elements",
}

// Info writes the registry entry for the named element to w. The name
// is matched against the final path segment of each registered import
// path (e.g. "div" matches "github.com/jpl-au/fluent/html5/div"), or
// against the HTML tag an element package renders, so tags that
// collide with Go keywords resolve too (e.g. "select" matches the
// dropdown package).
//
// If sections is non-empty, only the listed sections are written.
// Accepted names (long and short forms) are defined by InfoSections.
// Unknown section names return an error.
func (r *Registry) Info(w io.Writer, name string, sections ...string) error {
	show, err := resolveSections(sections)
	if err != nil {
		return err
	}

	var pkg Package
	var importPath string
	var label string
	var children map[string]Package
	var found bool

	// Iterate import paths in sorted order so a name that could match more
	// than one package resolves the same way every run; map order would make
	// the answer arbitrary.
	paths := sortedKeys(r.Packages)

	// An explicit pkg:element query (e.g. svg:rect) resolves an element within a
	// named package. It reaches an element whose bare name is shadowed by a
	// package (svg:text, versus the text node package) and reads consistently for
	// every svg shape; bare names still resolve by the precedence below.
	if pkgPart, elemPart, prefixed := strings.Cut(name, ":"); prefixed {
		for _, path := range paths {
			p := r.Packages[path]
			if !strings.EqualFold(lastSegment(path), pkgPart) {
				continue
			}
			for tag, el := range p.Elements {
				if strings.EqualFold(tag, elemPart) {
					pkg, importPath, label, found = el, path, tag, true
					break
				}
			}
			break
		}
		if !found {
			return fmt.Errorf("unknown element %q", name)
		}
	}

	// Resolution is case-insensitive so discovery does not hinge on a shape's
	// exact casing (e.g. -info radialgradient resolves radialGradient); the header
	// always shows the canonical name. Match by package name (the import path's
	// last segment) first.
	for _, path := range paths {
		p := r.Packages[path]
		if strings.EqualFold(lastSegment(path), name) {
			importPath = path
			label = lastSegment(path)
			pkg = p
			// A multi-element package (svg) usually has a root element whose tag
			// equals the package name; show that element's own surface so its
			// specific methods appear, and list the children below.
			children = p.Elements
			for tag, el := range p.Elements {
				if strings.EqualFold(tag, name) {
					pkg = el
					label = tag
					break
				}
			}
			found = true
			break
		}
	}

	// The queried name may be an HTML tag rather than a package name: tags that
	// are Go keywords live in differently named packages (select in dropdown,
	// main in primary, var in variable, map in imagemap).
	if !found {
		for _, path := range paths {
			p := r.Packages[path]
			if p.Tag != "" && strings.EqualFold(p.Tag, name) {
				pkg = p
				importPath = path
				label = lastSegment(path)
				found = true
				break
			}
		}
	}

	// The queried name may be one element of a multi-element package, e.g. "rect"
	// within the svg package, whose import path ends in /svg, not /rect.
	if !found {
		for _, path := range paths {
			p := r.Packages[path]
			for tag, el := range p.Elements {
				if strings.EqualFold(tag, name) {
					pkg = el
					importPath = path
					label = tag
					found = true
					break
				}
			}
			if found {
				break
			}
		}
	}
	if !found {
		return fmt.Errorf("unknown element %q", name)
	}

	pw := &prefixWriter{w: w}

	if pkg.Tag != "" && pkg.Tag != label {
		pw.printf("Element: %s (renders <%s>; %q is a Go reserved word)\n", label, pkg.Tag, pkg.Tag)
	} else {
		pw.printf("Element: %s\n", label)
	}
	pw.printf("Import:  %s\n", importPath)

	if show("types") && len(pkg.Types) > 0 {
		pw.printf("\nTypes:\n")
		for _, t := range sortedKeys(pkg.Types) {
			pw.printf("  %s\n", t)
		}
	}

	if show("constructors") && len(pkg.Functions) > 0 {
		pw.printf("\nConstructors:\n")
		for _, fn := range sortedKeys(pkg.Functions) {
			arity := pkg.Functions[fn]
			if arity == -1 {
				pw.printf("  %s(...)  variadic\n", fn)
			} else {
				pw.printf("  %s(%d)\n", fn, arity)
			}
		}
	}

	if show("typed-constructors") && len(pkg.TypedConstructors) > 0 {
		pw.printf("\nTyped Constructors:\n")
		for _, fn := range sortedKeys(pkg.TypedConstructors) {
			pw.printf("  %s  accepts %s.Element children\n", fn, pkg.TypedConstructors[fn])
		}
	}

	if show("methods") && len(pkg.Methods) > 0 {
		pw.printf("\nMethods:\n")
		for _, m := range sortedKeys(pkg.Methods) {
			if tp, ok := pkg.TypedParams[m]; ok {
				pw.printf("  %s  (enum: %s)\n", m, tp)
			} else {
				pw.printf("  %s\n", m)
			}
		}
	}

	if show("attributes") && len(pkg.AttrMethods) > 0 {
		pw.printf("\nAttribute Mappings:\n")
		for _, attr := range sortedKeys(pkg.AttrMethods) {
			pw.printf("  %-30s -> %s\n", attr, pkg.AttrMethods[attr])
		}
	}

	if show("vars") && len(pkg.Vars) > 0 {
		pw.printf("\nVars:\n")
		for _, v := range sortedKeys(pkg.Vars) {
			pw.printf("  %s\n", v)
		}
	}

	if show("elements") && len(children) > 0 {
		pw.printf("\nElements:\n")
		for _, tag := range sortedKeys(children) {
			pw.printf("  %s\n", tag)
		}
	}

	return pw.err
}

// resolveSections returns a predicate that reports whether a canonical
// section name should be shown. If names is empty, every section is
// shown. Unknown names yield an error listing the accepted values.
func resolveSections(names []string) (func(string) bool, error) {
	if len(names) == 0 {
		return func(string) bool { return true }, nil
	}
	selected := make(map[string]bool, len(names))
	for _, n := range names {
		canon, ok := InfoSections[n]
		if !ok {
			return nil, fmt.Errorf("unknown section %q (valid: %s)", n, validSectionList())
		}
		selected[canon] = true
	}
	return func(s string) bool { return selected[s] }, nil
}

// validSectionList returns a comma-separated list of accepted section
// names in a stable order, suitable for error messages and help text.
func validSectionList() string {
	keys := sortedKeys(InfoSections)
	return strings.Join(keys, ", ")
}

// prefixWriter captures the first write error so callers can check
// once after a sequence of writes rather than after every call.
type prefixWriter struct {
	w   io.Writer
	err error
}

func (pw *prefixWriter) printf(format string, args ...any) {
	if pw.err != nil {
		return
	}
	_, pw.err = fmt.Fprintf(pw.w, format, args...)
}

// sortedKeys returns the keys of a string-keyed map in sorted order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
