package flint

import (
	"strings"
	"testing"
)

func TestCheckSetAttributeChain(t *testing.T) {
	tests := []struct {
		name    string
		imports []string
		body    string
		want    string // empty means no diagnostic expected
	}{
		{
			name:    "chaining after SetAttribute is flagged",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `_ = div.New().SetAttribute("x-data", "{}").Class("container")`,
			want:    "SetAttribute does not return the element; cannot chain .Class() after it",
		},
		{
			name:    "chaining ID after SetAttribute is flagged",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `_ = div.New().SetAttribute("hx-get", "/items").ID("main")`,
			want:    "SetAttribute does not return the element; cannot chain .ID() after it",
		},
		{
			name:    "SetAttribute without chaining is fine",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `d := div.New().Class("x"); d.SetAttribute("hx-get", "/items")`,
		},
		{
			name:    "SetData chaining is fine",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `_ = div.New().SetData("id", "123").Class("container")`,
		},
		{
			name:    "SetAria chaining is fine",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `_ = div.New().SetAria("label", "Close").Class("btn")`,
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
					if strings.Contains(d.Message, "SetAttribute does not return") {
						t.Errorf("unexpected SetAttribute diagnostic: %s", d.Message)
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
