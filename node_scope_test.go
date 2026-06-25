package flint

import (
	"strings"
	"testing"
)

// TestNodeAppendScopeFires covers cases that used to stay silent because an
// unrelated same-named variable confused the name-based scan, and now fire.
func TestNodeAppendScopeFires(t *testing.T) {
	l := New(FluentRegistry())
	tests := []struct {
		name string
		body string
	}{
		{
			name: "unrelated same-name in a sibling block",
			body: `kids := []node.Node{}
kids = append(kids, div.New())
if cond {
	kids := 42
	_ = kids
}
_ = div.New(kids...)`,
		},
		{
			name: "closure parameter shadows the name",
			body: `kids := []node.Node{}
kids = append(kids, div.New())
helper := func(kids int) { _ = kids }
_ = helper
_ = div.New(kids...)`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := wrapWithImports([]string{nodePkg, divPkg}, tt.body)
			diags, err := l.Source("test.go", src)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if _, ok := findNodeAppend(diags); !ok {
				t.Fatalf("expected a node-append diagnostic, got %d", len(diags))
			}
		})
	}
}

// TestNodeAppendScopeQuiet covers cases where the slice escapes or is used in
// another way: the rule must stay silent rather than suggest an unsafe rewrite.
func TestNodeAppendScopeQuiet(t *testing.T) {
	l := New(FluentRegistry())
	tests := []struct {
		name string
		body string
	}{
		{
			name: "read captured in a goroutine",
			body: `kids := []node.Node{}
kids = append(kids, div.New())
go func() { sink(kids) }()
_ = div.New(kids...)`,
		},
		{
			name: "appended in a plain helper closure",
			body: `kids := []node.Node{}
run := func() { kids = append(kids, div.New()) }
run()
_ = div.New(kids...)`,
		},
		{
			name: "re-sliced",
			body: `kids := []node.Node{}
kids = append(kids, div.New())
kids = kids[1:]
_ = div.New(kids...)`,
		},
		{
			name: "aliased to another variable",
			body: `kids := []node.Node{}
kids = append(kids, div.New())
other := kids
_ = other
_ = div.New(kids...)`,
		},
		{
			name: "spread into two elements",
			body: `kids := []node.Node{}
kids = append(kids, div.New())
_ = div.New(kids...)
_ = div.New(kids...)`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := wrapWithImports([]string{nodePkg, divPkg}, tt.body)
			diags, err := l.Source("test.go", src)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if d, ok := findNodeAppend(diags); ok {
				t.Errorf("unexpected diagnostic: %s", d.Message)
			}
		})
	}
}

// TestNodeAppendTooLate covers an append inside a defer/goroutine, which runs
// after the slice has been passed in, so the children never appear.
func TestNodeAppendTooLate(t *testing.T) {
	l := New(FluentRegistry())
	tests := []struct {
		name string
		body string
	}{
		{
			name: "defer",
			body: `kids := []node.Node{}
defer func() { kids = append(kids, div.New()) }()
_ = div.New(kids...)`,
		},
		{
			name: "goroutine",
			body: `kids := []node.Node{}
go func() { kids = append(kids, div.New()) }()
_ = div.New(kids...)`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := wrapWithImports([]string{nodePkg, divPkg}, tt.body)
			diags, err := l.Source("test.go", src)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			found := false
			for _, d := range diags {
				if strings.Contains(d.Message, "those children will not appear") {
					found = true
				}
			}
			if !found {
				t.Fatalf("expected a too-late diagnostic, got %d diagnostics", len(diags))
			}
		})
	}
}

// TestNodeAppendInnerClosureBothFire checks that an outer accumulator and an
// inner closure with its own same-named slice are flagged independently.
func TestNodeAppendInnerClosureBothFire(t *testing.T) {
	l := New(FluentRegistry())
	body := `kids := []node.Node{}
kids = append(kids, div.New())
helper := func() {
	kids := []node.Node{}
	kids = append(kids, div.New())
	_ = div.New(kids...)
}
_ = helper
_ = div.New(kids...)`
	src := wrapWithImports([]string{nodePkg, divPkg}, body)
	diags, err := l.Source("test.go", src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	count := 0
	for _, d := range diags {
		if strings.Contains(d.Message, "with append") {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("expected 2 node-append diagnostics (outer + inner), got %d", count)
	}
}
