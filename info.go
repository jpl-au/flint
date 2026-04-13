package flint

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// Info writes the registry entry for the named element to w. The name
// is matched against the final path segment of each registered import
// path (e.g. "div" matches "github.com/jpl-au/fluent/html5/div").
func (r *Registry) Info(w io.Writer, name string) error {
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

	if len(pkg.Types) > 0 {
		pw.printf("\nTypes:\n")
		for _, t := range sortedKeys(pkg.Types) {
			pw.printf("  %s\n", t)
		}
	}

	if len(pkg.Functions) > 0 {
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

	if len(pkg.TypedConstructors) > 0 {
		pw.printf("\nTyped Constructors:\n")
		for _, fn := range sortedKeys(pkg.TypedConstructors) {
			pw.printf("  %s  accepts %s.Element children\n", fn, pkg.TypedConstructors[fn])
		}
	}

	if len(pkg.Methods) > 0 {
		pw.printf("\nMethods:\n")
		for _, m := range sortedKeys(pkg.Methods) {
			if tp, ok := pkg.TypedParams[m]; ok {
				pw.printf("  %s  (enum: %s)\n", m, tp)
			} else {
				pw.printf("  %s\n", m)
			}
		}
	}

	if len(pkg.AttrMethods) > 0 {
		pw.printf("\nAttribute Mappings:\n")
		for _, attr := range sortedKeys(pkg.AttrMethods) {
			pw.printf("  %-30s -> %s\n", attr, pkg.AttrMethods[attr])
		}
	}

	if len(pkg.Vars) > 0 {
		pw.printf("\nVars:\n")
		for _, v := range sortedKeys(pkg.Vars) {
			pw.printf("  %s\n", v)
		}
	}

	return pw.err
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
