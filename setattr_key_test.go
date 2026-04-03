package flint

import (
	"strings"
	"testing"
)

func TestCheckSetAttributeKey(t *testing.T) {
	old := activeRegistry
	activeRegistry = FluentRegistry()
	defer func() { activeRegistry = old }()

	tests := []struct {
		name    string
		imports []string
		body    string
		want    string // empty means no diagnostic expected
	}{
		{
			name:    "SetAttribute with class is flagged",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `d := div.New(); d.SetAttribute("class", "container")`,
			want:    `SetAttribute("class", ...) bypasses the dedicated field; use .Class() instead`,
		},
		{
			name:    "SetAttribute with id is flagged",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `d := div.New(); d.SetAttribute("id", "main")`,
			want:    `SetAttribute("id", ...) bypasses the dedicated field; use .ID() instead`,
		},
		{
			name:    "SetAttribute with href on anchor is flagged",
			imports: []string{"github.com/jpl-au/fluent/html5/a"},
			body:    `d := a.New(); d.SetAttribute("href", "/home")`,
			want:    `SetAttribute("href", ...) bypasses the dedicated field; use .Href() instead`,
		},
		{
			name:    "SetAttribute with style is flagged",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `d := div.New(); d.SetAttribute("style", "color:red")`,
			want:    `SetAttribute("style", ...) bypasses the dedicated field; use .Style() instead`,
		},
		{
			name:    "SetAttribute with data- prefix suggests SetData",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `d := div.New(); d.SetAttribute("data-id", "123")`,
			want:    `SetAttribute("data-id", ...) should use SetData("id", ...) instead`,
		},
		{
			name:    "SetAttribute with aria- prefix suggests SetAria",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `d := div.New(); d.SetAttribute("aria-label", "close")`,
			want:    `SetAttribute("aria-label", ...) should use SetAria("label", ...) instead`,
		},
		{
			name:    "SetAttribute with custom attribute is fine",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `d := div.New(); d.SetAttribute("hx-get", "/items")`,
		},
		{
			name:    "SetAttribute with x-data is fine",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `d := div.New(); d.SetAttribute("x-data", "{}")`,
		},
		{
			name:    "SetAttribute with variable key is not checked",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `key := "class"; d := div.New(); d.SetAttribute(key, "x")`,
		},
		{
			name:    "using Class method directly is fine",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `_ = div.New().Class("container")`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := wrapWithImports(tt.imports, tt.body)
			diags, err := Source("test.go", src)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}

			if tt.want == "" {
				for _, d := range diags {
					if strings.Contains(d.Message, "bypasses the dedicated field") {
						t.Errorf("unexpected SetAttribute key diagnostic: %s", d.Message)
					}
				}
				return
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
