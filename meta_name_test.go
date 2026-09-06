package flint

import (
	"strings"
	"testing"
)

func TestMetaNameConstructorChecks(t *testing.T) {
	l := New(FluentRegistry())
	tests := []struct {
		name, body, check, contains string
	}{
		{"generic constructor", `_ = meta.New().Name("application-name").Content("My App")`, "constructors", "use meta.Name(...)"},
		{"specialised constructor", `_ = meta.New().Name("description").Content("A description")`, "constructors", "use meta.Description(...)"},
		{"constructor arity", `_ = meta.Name("application-name")`, "arity", "expects 2 argument(s)"},
		{"method arity", `_ = meta.New().Name("application-name", "My App")`, "method-arity", "expects 1 argument(s)"},
		{"duplicate name", `meta.Name("application-name", "My App").SetAttribute("name", "description")`, "duplicate-attr", "after meta.Name"},
		{"duplicate content on local", `m := meta.Name("application-name", "My App"); m.SetAttribute("content", "Other App")`, "duplicate-attr", "after meta.Name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := wrapWithImports([]string{"github.com/jpl-au/fluent/html5/meta"}, tt.body)
			diags, err := l.Source("test.go", src)
			if err != nil {
				t.Fatal(err)
			}
			for _, d := range diags {
				if d.Check == tt.check && strings.Contains(d.Message, tt.contains) {
					return
				}
			}
			t.Fatalf("missing %s diagnostic containing %q: %v", tt.check, tt.contains, diags)
		})
	}
}
