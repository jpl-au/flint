package flint

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// attrSource builds a valid Go file that imports the given packages and runs
// body inside a function. k is a string parameter so tests can exercise a
// dynamic SetAttribute key without a separate declaration.
func attrSource(imports []string, body string) []byte {
	var block strings.Builder
	for _, imp := range imports {
		fmt.Fprintf(&block, "\t%q\n", imp)
	}
	return fmt.Appendf(nil, `package example

import (
%s)

func build(k string) {
	%s
}
`, block.String(), body)
}

func TestAttrPairs(t *testing.T) {
	l := New(FluentRegistry())

	const (
		div    = "github.com/jpl-au/fluent/html5/div"
		anchor = "github.com/jpl-au/fluent/html5/a"
	)

	tests := []struct {
		name    string
		imports []string
		body    string
		want    []AttrPair
	}{
		{
			name:    "typed attribute method",
			imports: []string{div},
			body:    `div.New().Class("x")`,
			want:    []AttrPair{{"div", "class"}},
		},
		{
			name:    "several methods across elements",
			imports: []string{div, anchor},
			body:    `div.New().Class("x").ID("y"); a.New().Href("/p")`,
			want:    []AttrPair{{"a", "href"}, {"div", "class"}, {"div", "id"}},
		},
		{
			name:    "SetAttribute constant key",
			imports: []string{div},
			body:    `div.New().SetAttribute("data-role", "tab")`,
			want:    []AttrPair{{"div", "data-role"}},
		},
		{
			name:    "SetAttributeRaw constant key",
			imports: []string{div},
			body:    `div.New().SetAttributeRaw("translate", "no")`,
			want:    []AttrPair{{"div", "translate"}},
		},
		{
			name:    "dynamic SetAttribute key",
			imports: []string{div},
			body:    `div.New().SetAttribute(k, "tab")`,
			want:    []AttrPair{{"div", dynamicAttr}},
		},
		{
			name:    "typed method and SetAttribute on one chain",
			imports: []string{div},
			body:    `div.New().Class("x").SetAttribute("data-role", "tab")`,
			want:    []AttrPair{{"div", "class"}, {"div", "data-role"}},
		},
		{
			// The receiver resolves to no known fluent element package, so
			// nothing is recorded - not even the constant key.
			name:    "unknown element is skipped",
			imports: []string{"example.com/widget"},
			body:    `widget.New().SetAttribute("class", "x")`,
			want:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := l.AttrPairs("build.go", attrSource(tt.imports, tt.body))
			if err != nil {
				t.Fatalf("AttrPairs: %v", err)
			}
			assertPairs(t, got, tt.want)
		})
	}
}

func TestAttrPairsNoRegistry(t *testing.T) {
	got, err := New(nil).AttrPairs("build.go", attrSource(
		[]string{"github.com/jpl-au/fluent/html5/div"},
		`div.New().Class("x")`,
	))
	if err != nil {
		t.Fatalf("AttrPairs: %v", err)
	}
	if got != nil {
		t.Errorf("AttrPairs without a registry = %v, want nil", got)
	}
}

// assertPairs compares two pair sets order-independently, since AST traversal
// order is an implementation detail, not a promised contract.
func assertPairs(t *testing.T, got, want []AttrPair) {
	t.Helper()
	sortPairs(got)
	sortPairs(want)
	if !slices.Equal(got, want) {
		t.Errorf("pairs = %v, want %v", got, want)
	}
}

func sortPairs(pairs []AttrPair) {
	slices.SortFunc(pairs, func(a, b AttrPair) int {
		if a.Element != b.Element {
			return strings.Compare(a.Element, b.Element)
		}
		return strings.Compare(a.Attribute, b.Attribute)
	})
}
