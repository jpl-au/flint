package flint

import (
	"strings"
	"testing"
)

const (
	nodePkg = "github.com/jpl-au/fluent/node"
	divPkg  = "github.com/jpl-au/fluent/html5/div"
)

func TestCheckNodeAppendPositive(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name   string
		body   string
		wantIn string // substring expected in the Fix
	}{
		{
			name: "conditional append suggests When",
			body: `kids := []node.Node{}
if cond {
	kids = append(kids, div.New())
}
_ = div.New(kids...)`,
			wantIn: "node.When(cond, child) for a conditional child",
		},
		{
			name: "loop append suggests Map",
			body: `rows := make([]node.Node, 0, 4)
for _, it := range items {
	rows = append(rows, div.Text(it))
}
_ = div.New(rows...)`,
			wantIn: "node.Map(slice, fn)",
		},
		{
			name: "var declaration with plain append",
			body: `var kids []node.Node
kids = append(kids, div.New())
_ = div.New(kids...)`,
			wantIn: "passing children directly to the constructor or via .Add(...)",
		},
		{
			name: "composite literal seed then append",
			body: `kids := []node.Node{div.New()}
kids = append(kids, div.New())
_ = div.New(kids...)`,
			wantIn: "passing children directly",
		},
		{
			name: "make with length notes the nil entries",
			body: `kids := make([]node.Node, 3)
kids = append(kids, div.New())
_ = div.New(kids...)`,
			wantIn: "n nil entries before the appended children",
		},
		{
			name: "non-fluent sink flagged with generic, non-element wording",
			body: `kids := []node.Node{}
kids = append(kids, div.New())
_ = render(kids...)`,
			wantIn: "composing the children with Fluent rather than assembling",
		},
		{
			name: "negated if suggests Unless",
			body: `kids := []node.Node{}
if !cond {
	kids = append(kids, div.New())
}
_ = div.New(kids...)`,
			wantIn: "node.Unless(cond, child)",
		},
		{
			name: "else-only append suggests Unless",
			body: `kids := []node.Node{}
if cond {
} else {
	kids = append(kids, div.New())
}
_ = div.New(kids...)`,
			wantIn: "node.Unless(cond, child)",
		},
		{
			name: "if/else appending in both branches suggests Condition",
			body: `kids := []node.Node{}
if cond {
	kids = append(kids, div.New())
} else {
	kids = append(kids, div.New())
}
_ = div.New(kids...)`,
			wantIn: "node.Condition(cond).True(child).False(child)",
		},
		{
			name: "switch append suggests Funcs",
			body: `kids := []node.Node{}
switch n {
case 1:
	kids = append(kids, div.New())
}
_ = div.New(kids...)`,
			wantIn: "node.Funcs(func() []node.Node { ... })",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := wrapWithImports([]string{nodePkg, divPkg}, tt.body)
			diags, err := l.Source("test.go", src)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			d, ok := findNodeAppend(diags)
			if !ok {
				t.Fatalf("expected a node-append diagnostic; got %d diagnostics", len(diags))
			}
			if d.Severity != Warning {
				t.Errorf("severity = %v, want Warning", d.Severity)
			}
			if !strings.Contains(d.Fix, tt.wantIn) {
				t.Errorf("Fix = %q, want it to contain %q", d.Fix, tt.wantIn)
			}
		})
	}
}

func TestCheckNodeAppendNegative(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name    string
		imports []string
		body    string
	}{
		{
			name:    "literal list with no append is fine",
			imports: []string{nodePkg, divPkg},
			body: `kids := []node.Node{div.New()}
_ = div.New(kids...)`,
		},
		{
			name:    "non-node slice is not flagged",
			imports: []string{},
			body: `xs := []string{}
xs = append(xs, "a")
_ = join(xs...)`,
		},
		{
			name:    "indexed accumulator is left alone",
			imports: []string{nodePkg, divPkg},
			body: `kids := []node.Node{}
kids = append(kids, div.New())
first := kids[0]
_ = div.New(kids...)
_ = first`,
		},
		{
			name:    "passed un-splatted is left alone",
			imports: []string{nodePkg, divPkg},
			body: `kids := []node.Node{}
kids = append(kids, div.New())
sink(kids)
_ = div.New(kids...)`,
		},
		{
			name:    "no splat means nothing to inline",
			imports: []string{nodePkg, divPkg},
			body: `kids := []node.Node{}
kids = append(kids, div.New())
_ = kids`,
		},
		{
			name:    "non-fluent node type is out of scope",
			imports: []string{"example.com/mylib"},
			body: `xs := []mylib.Node{}
xs = append(xs, mylib.New())
_ = mylib.Box(xs...)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := wrapWithImports(tt.imports, tt.body)
			diags, err := l.Source("test.go", src)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			if d, ok := findNodeAppend(diags); ok {
				t.Errorf("unexpected node-append diagnostic: %s", d.Message)
			}
		})
	}
}

func TestCheckNodeAppendNeedsRegistry(t *testing.T) {
	// Without a registry the check cannot scope to Fluent node types, so it stays
	// silent rather than guess.
	l := New(nil)
	body := `kids := []node.Node{}
kids = append(kids, div.New())
_ = div.New(kids...)`
	src := wrapWithImports([]string{nodePkg, divPkg}, body)
	diags, err := l.Source("test.go", src)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if d, ok := findNodeAppend(diags); ok {
		t.Errorf("unexpected diagnostic without registry: %s", d.Message)
	}
}

// findNodeAppend returns the first node-append diagnostic, identified by its
// distinctive message.
func findNodeAppend(diags []Diagnostic) (Diagnostic, bool) {
	for _, d := range diags {
		if strings.Contains(d.Message, "with append") {
			return d, true
		}
	}
	return Diagnostic{}, false
}
