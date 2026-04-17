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
}

// Info writes the registry entry for the named element to w. The name
// is matched against the final path segment of each registered import
// path (e.g. "div" matches "github.com/jpl-au/fluent/html5/div").
//
// If sections is non-empty, only the listed sections are written.
// Accepted names (long and short forms) are defined by InfoSections.
// Unknown section names return an error.
func (r *Registry) Info(w io.Writer, name string, sections ...string) error {
	show, err := resolveSections(sections)
	if err != nil {
		return err
	}

	suffix := "/" + name

	var pkg Package
	var importPath string
	var found bool
	for path, p := range r.Packages {
		if strings.HasSuffix(path, suffix) {
			pkg = p
			importPath = path
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("unknown element %q", name)
	}

	pw := &prefixWriter{w: w}

	pw.printf("Element: %s\n", name)
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
