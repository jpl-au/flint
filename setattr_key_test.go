package flint

import (
	"strings"
	"testing"
)

func TestCheckSetAttrKeyPositive(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name string
		src  []byte
		want string
	}{
		{
			name: "chained SetAttribute style as return value",
			src: wrapReturningFunc(
				[]string{"github.com/jpl-au/fluent/html5/span"},
				`return span.New().Class("workspace-dot").SetAttribute("style", "background:"+colour)`,
			),
			want: `SetAttribute("style", ...) bypasses the dedicated field; use .Style() instead`,
		},
		{
			name: "chained SetAttribute class as return value",
			src: wrapReturningFunc(
				[]string{"github.com/jpl-au/fluent/html5/div"},
				`return div.New().ID("x").SetAttribute("class", "container")`,
			),
			want: `SetAttribute("class", ...) bypasses the dedicated field; use .Class() instead`,
		},
		{
			name: "standalone SetAttribute class",
			src: wrapWithImports(
				[]string{"github.com/jpl-au/fluent/html5/div"},
				`d := div.New(); d.SetAttribute("class", "container")`,
			),
			want: `SetAttribute("class", ...) bypasses the dedicated field; use .Class() instead`,
		},
		{
			name: "standalone SetAttribute id",
			src: wrapWithImports(
				[]string{"github.com/jpl-au/fluent/html5/div"},
				`d := div.New(); d.SetAttribute("id", "main")`,
			),
			want: `SetAttribute("id", ...) bypasses the dedicated field; use .ID() instead`,
		},
		{
			name: "standalone SetAttribute href on anchor",
			src: wrapWithImports(
				[]string{"github.com/jpl-au/fluent/html5/a"},
				`d := a.New(); d.SetAttribute("href", "/home")`,
			),
			want: `SetAttribute("href", ...) bypasses the dedicated field; use .Href() instead`,
		},
		{
			name: "standalone SetAttribute style",
			src: wrapWithImports(
				[]string{"github.com/jpl-au/fluent/html5/div"},
				`d := div.New(); d.SetAttribute("style", "color:red")`,
			),
			want: `SetAttribute("style", ...) bypasses the dedicated field; use .Style() instead`,
		},
		{
			name: "data- prefix suggests SetData",
			src: wrapWithImports(
				[]string{"github.com/jpl-au/fluent/html5/div"},
				`d := div.New(); d.SetAttribute("data-id", "123")`,
			),
			want: `SetAttribute("data-id", ...) should use SetData("id", ...) instead`,
		},
		{
			name: "aria- prefix suggests SetAria",
			src: wrapWithImports(
				[]string{"github.com/jpl-au/fluent/html5/div"},
				`d := div.New(); d.SetAttribute("aria-label", "close")`,
			),
			want: `SetAttribute("aria-label", ...) should use SetAria("label", ...) instead`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags, err := l.Source("test.go", tt.src)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}

			found := false
			for _, d := range diags {
				if d.Message == tt.want {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected diagnostic %q", tt.want)
				for _, d := range diags {
					t.Logf("  %s: %s", d.Pos, d.Message)
				}
			}
		})
	}
}

func TestCheckSetAttrKeyNegative(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name    string
		imports []string
		body    string
	}{
		{
			name:    "custom attribute is fine",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `d := div.New(); d.SetAttribute("hx-get", "/items")`,
		},
		{
			name:    "x-data is fine",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `d := div.New(); d.SetAttribute("x-data", "{}")`,
		},
		{
			name:    "variable key is not checked",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `key := "class"; d := div.New(); d.SetAttribute(key, "x")`,
		},
		{
			name:    "using Class method directly is fine",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `_ = div.New().Class("container")`,
		},
		{
			name:    "using Style method directly is fine",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `_ = div.New().Style("color:red")`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := wrapWithImports(tt.imports, tt.body)
			diags, err := l.Source("test.go", src)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}

			for _, d := range diags {
				if strings.Contains(d.Message, "bypasses the dedicated field") ||
					strings.Contains(d.Message, "should use Set") {
					t.Errorf("unexpected diagnostic: %s", d.Message)
				}
			}
		})
	}

	// Without a registry, key checks are skipped entirely.
	t.Run("no registry means no key checks", func(t *testing.T) {
		noReg := New(nil)
		src := wrapWithImports(
			[]string{"github.com/jpl-au/fluent/html5/div"},
			`d := div.New(); d.SetAttribute("class", "x")`,
		)
		diags, err := noReg.Source("test.go", src)
		if err != nil {
			t.Fatalf("unexpected parse error: %v", err)
		}
		for _, d := range diags {
			if strings.Contains(d.Message, "bypasses the dedicated field") {
				t.Errorf("unexpected diagnostic without registry: %s", d.Message)
			}
		}
	})
}
