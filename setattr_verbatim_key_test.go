package flint

import (
	"strings"
	"testing"
)

// TestCheckVerbatimKeyPositive locks the calls that fire: a run-time key on
// any setter of the family, on an element method or a node package-level
// twin, on a chain or a fluent local.
func TestCheckVerbatimKeyPositive(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name string
		src  []byte
		want string
	}{
		{
			name: "variable key on chained SetAttribute",
			src: wrapWithImports(
				[]string{"github.com/jpl-au/fluent/html5/div"},
				`k := "x-data"; d := div.New(); d.SetAttribute(k, "v")`,
			),
			want: `SetAttribute key is variable "k"`,
		},
		{
			name: "variable key on SetData",
			src: wrapWithImports(
				[]string{"github.com/jpl-au/fluent/html5/div"},
				`k := "id"; _ = div.New().SetData(k, "v")`,
			),
			want: `SetData key is variable "k"`,
		},
		{
			name: "variable key on SetAria",
			src: wrapWithImports(
				[]string{"github.com/jpl-au/fluent/html5/div"},
				`k := "label"; _ = div.New().SetAria(k, "v")`,
			),
			want: `SetAria key is variable "k"`,
		},
		{
			name: "variable key on SetEvent",
			src: wrapWithImports(
				[]string{"github.com/jpl-au/fluent/html5/div"},
				`k := "onclick"; _ = div.New().SetEvent(k, "v")`,
			),
			want: `SetEvent key is variable "k"`,
		},
		{
			name: "concatenation with a variable",
			src: wrapWithImports(
				[]string{"github.com/jpl-au/fluent/html5/div"},
				`suffix := "get"; d := div.New(); d.SetAttribute("hx-"+suffix, "/items")`,
			),
			want: `SetAttribute key is binary expression`,
		},
		{
			name: "variable key on the node package-level SetData",
			src: wrapWithImports(
				[]string{
					"github.com/jpl-au/fluent/html5/div",
					"github.com/jpl-au/fluent/node",
				},
				`k := "id"; d := div.New(); node.SetData(d, k, "v")`,
			),
			want: `SetData key is variable "k"`,
		},
		{
			name: "variable key on SetAttributeRaw",
			src: wrapWithImports(
				[]string{"github.com/jpl-au/fluent/html5/div"},
				`k := "x-raw"; d := div.New(); d.SetAttributeRaw(k, "v")`,
			),
			want: `SetAttributeRaw key is variable "k"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags, err := l.Source("test.go", tt.src)
			if err != nil {
				t.Fatalf("Source() returned error: %v", err)
			}
			var found bool
			for _, d := range diags {
				if d.Check == "verbatim-key" && strings.Contains(d.Message, tt.want) {
					found = true
					if d.Severity != Warning {
						t.Errorf("severity = %v, want Warning", d.Severity)
					}
					break
				}
			}
			if !found {
				t.Errorf("no verbatim-key diagnostic containing %q\ngot:", tt.want)
				for _, d := range diags {
					t.Errorf("  [%s] %s", d.Check, d.Message)
				}
			}
		})
	}
}

// TestCheckVerbatimKeyNegative locks the calls that stay quiet: compile-time
// keys, and setters on receivers flint cannot trace to fluent.
func TestCheckVerbatimKeyNegative(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name string
		src  []byte
	}{
		{
			name: "literal key",
			src: wrapWithImports(
				[]string{"github.com/jpl-au/fluent/html5/div"},
				`d := div.New(); d.SetAttribute("x-data", "{}")`,
			),
		},
		{
			name: "concatenation of literals",
			src: wrapWithImports(
				[]string{"github.com/jpl-au/fluent/html5/div"},
				`d := div.New(); d.SetAttribute("hx-"+"get", "/items")`,
			),
		},
		{
			name: "parenthesised literal",
			src: wrapWithImports(
				[]string{"github.com/jpl-au/fluent/html5/div"},
				`d := div.New(); d.SetAttribute(("x-data"), "{}")`,
			),
		},
		{
			name: "non-fluent receiver",
			src: wrapWithImports(
				nil,
				`k := "x"; w := otherLib(); w.SetAttribute(k, "v")`,
			),
		},
		{
			name: "variable value is fine",
			src: wrapWithImports(
				[]string{"github.com/jpl-au/fluent/html5/div"},
				`v := userInput(); d := div.New(); d.SetAttribute("x-data", v)`,
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags, err := l.Source("test.go", tt.src)
			if err != nil {
				t.Fatalf("Source() returned error: %v", err)
			}
			for _, d := range diags {
				if d.Check == "verbatim-key" {
					t.Errorf("unexpected verbatim-key diagnostic: %s", d.Message)
				}
			}
		})
	}
}
