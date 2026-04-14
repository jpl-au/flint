package flint

import (
	"strings"
	"testing"
)

func TestCheckSetAttrChain(t *testing.T) {
	l := New(nil)

	tests := []struct {
		name    string
		imports []string
		body    string
		want    string
	}{
		{
			name:    "chaining method after SetAttribute is flagged",
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
			name:    "SetAttribute as final call is fine",
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
			diags, err := l.Source("test.go", src)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}

			if tt.want == "" {
				for _, d := range diags {
					if strings.Contains(d.Message, "SetAttribute does not return") {
						t.Errorf("unexpected diagnostic: %s", d.Message)
					}
				}
				return
			}

			found := false
			for _, d := range diags {
				if d.Message == tt.want {
					found = true
					if d.Severity != Error {
						t.Errorf("severity = %v, want Error", d.Severity)
					}
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

func TestCheckSetAttrChainScoped(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name    string
		imports []string
		body    string
		want    string
	}{
		{
			name:    "fluent package chain is flagged",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `_ = div.New().SetAttribute("x-data", "{}").Class("container")`,
			want:    "SetAttribute does not return the element; cannot chain .Class() after it",
		},
		{
			name:    "non-fluent package chain is not flagged",
			imports: []string{"example.com/mylib"},
			body:    `_ = mylib.New().SetAttribute("x-data", "{}").Class("container")`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := wrapWithImports(tt.imports, tt.body)
			diags, err := l.Source("test.go", src)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}

			if tt.want == "" {
				for _, d := range diags {
					if strings.Contains(d.Message, "SetAttribute does not return") {
						t.Errorf("unexpected diagnostic: %s", d.Message)
					}
				}
				return
			}

			found := false
			for _, d := range diags {
				if d.Message == tt.want {
					found = true
					if d.Severity != Error {
						t.Errorf("severity = %v, want Error", d.Severity)
					}
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
