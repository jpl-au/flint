package flint

import (
	"strings"
	"testing"
)

// TestConditionalBuilderValidChains checks that chained methods off the node
// conditional constructors validate against *ConditionalBuilder. Condition, When
// and Unless all return the builder, so a chain rooted at any of them resolves to
// its method set.
func TestConditionalBuilderValidChains(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name string
		body string
	}{
		{name: "condition true/false/dynamic", body: `_ = node.Condition(true).True(node.Empty()).False(node.Empty()).Dynamic("k")`},
		{name: "when dynamic", body: `_ = node.When(true, node.Empty()).Dynamic("k")`},
		{name: "unless true", body: `_ = node.Unless(false, node.Empty()).True(node.Empty())`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := wrapWithImports([]string{nodePkg}, tt.body)
			diags, err := l.Source("test.go", src)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			for _, d := range diags {
				if d.Severity == Error {
					t.Errorf("unexpected error diagnostic: %s: %s", d.Pos, d.Message)
				}
			}
		})
	}
}

// TestConditionalBuilderInvalidMethods checks that a method that does not exist
// on *ConditionalBuilder is flagged, whether the chain roots at Condition, When
// or Unless.
func TestConditionalBuilderInvalidMethods(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name string
		body string
	}{
		{name: "condition bogus", body: `_ = node.Condition(true).Frobnicate()`},
		{name: "when bogus", body: `_ = node.When(true, node.Empty()).Bogus()`},
		{name: "unless bogus", body: `_ = node.Unless(false, node.Empty()).Nope()`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := wrapWithImports([]string{nodePkg}, tt.body)
			diags, err := l.Source("test.go", src)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			found := false
			for _, d := range diags {
				if d.Severity == Error && strings.Contains(d.Message, "does not exist") {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected an Error diagnostic, got:")
				for _, d := range diags {
					t.Logf("  %s: %s", d.Pos, d.Message)
				}
			}
		})
	}
}
