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
