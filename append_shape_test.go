package flint

import (
	"strings"
	"testing"
)

// TestNodeAppendFixNamesBothIdioms checks that a body mixing a conditional and a
// loop names both the node.When and node.Map idioms, alongside the .Add fallback.
func TestNodeAppendFixNamesBothIdioms(t *testing.T) {
	l := New(FluentRegistry())
	body := `kids := []node.Node{}
if cond {
	kids = append(kids, div.New())
}
for _, it := range items {
	kids = append(kids, div.Text(it))
}
_ = div.New(kids...)`
	src := wrapWithImports([]string{nodePkg, divPkg}, body)
	diags, err := l.Source("test.go", src)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	d, ok := findNodeAppend(diags)
	if !ok {
		t.Fatal("expected a node-append diagnostic")
	}
	for _, want := range []string{"node.When", "node.Map", ".Add("} {
		if !strings.Contains(d.Fix, want) {
			t.Errorf("Fix = %q, want it to mention %q", d.Fix, want)
		}
	}
}

// TestNodeAppendElseIfSuggestsFuncs checks that an else-if chain, whose branch
// is guarded by its own condition, is steered to node.Funcs rather than
// node.Unless/node.Condition, which cannot express the extra condition.
func TestNodeAppendElseIfSuggestsFuncs(t *testing.T) {
	l := New(FluentRegistry())
	body := `kids := []node.Node{}
if a {
} else if b {
	kids = append(kids, div.New())
}
_ = div.New(kids...)`
	src := wrapWithImports([]string{nodePkg, divPkg}, body)
	diags, err := l.Source("test.go", src)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	d, ok := findNodeAppend(diags)
	if !ok {
		t.Fatal("expected a node-append diagnostic")
	}
	if !strings.Contains(d.Fix, "node.Funcs(func() []node.Node { ... }) for branching") {
		t.Errorf("Fix = %q, want it to suggest node.Funcs for an else-if", d.Fix)
	}
	if strings.Contains(d.Fix, "node.Unless") {
		t.Errorf("Fix = %q, must not suggest node.Unless for an else-if chain", d.Fix)
	}
}

// TestNodeAppendMakeZeroNoNote checks that a zero-length make, in any integer
// base or parenthesised, omits the nil-entries note, while a non-zero length
// keeps it.
func TestNodeAppendMakeZeroNoNote(t *testing.T) {
	l := New(FluentRegistry())
	tests := []struct {
		name     string
		length   string
		wantNote bool
	}{
		{"decimal zero", "0", false},
		{"hex zero", "0x0", false},
		{"parenthesised zero", "(0)", false},
		{"non-zero literal", "3", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `kids := make([]node.Node, ` + tt.length + `)
kids = append(kids, div.New())
_ = div.New(kids...)`
			src := wrapWithImports([]string{nodePkg, divPkg}, body)
			diags, err := l.Source("test.go", src)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			d, ok := findNodeAppend(diags)
			if !ok {
				t.Fatal("expected a node-append diagnostic")
			}
			if got := strings.Contains(d.Fix, "nil entries"); got != tt.wantNote {
				t.Errorf("nil-entries note present = %v, want %v (Fix = %q)", got, tt.wantNote, d.Fix)
			}
		})
	}
}
