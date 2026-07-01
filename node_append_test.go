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
		name        string
		body        string
		wantIn      string // substring expected in the Fix
		wantWarning bool   // the nil-seeding slip is a Warning; plain composition is advisory Info
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
			wantIn:      "n nil entries before the appended children",
			wantWarning: true,
		},
		{
			name: "parenthesised constructor keeps the element wording",
			body: `kids := []node.Node{}
kids = append(kids, div.New())
_ = (div.New)(kids...)`,
			wantIn: "passing children directly to the constructor or via .Add(...)",
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
		{
			name: "loop that also seeds nil entries stays a warning",
			body: `rows := make([]node.Node, 2)
for _, it := range items {
	rows = append(rows, div.Text(it))
}
_ = div.New(rows...)`,
			wantIn:      "n nil entries before the appended children",
			wantWarning: true,
		},
		{
			name: "loop plus conditional is still advisory composition",
			body: `rows := []node.Node{}
for _, it := range items {
	rows = append(rows, div.Text(it))
}
if cond {
	rows = append(rows, div.New())
}
_ = div.New(rows...)`,
			wantIn: "node.When(cond, child)",
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
			wantSev := Info
			if tt.wantWarning {
				wantSev = Warning
			}
			if d.Severity != wantSev {
				t.Errorf("severity = %v, want %v", d.Severity, wantSev)
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
		{
			name:    "selector that is not a real exported type is not a node slice",
			imports: []string{nodePkg, divPkg},
			body: `kids := []div.Node{}
kids = append(kids, div.New())
_ = div.New(kids...)`,
		},
		{
			name:    "append with no element is not growth",
			imports: []string{nodePkg, divPkg},
			body: `kids := []node.Node{}
kids = append(kids)
_ = div.New(kids...)`,
		},
		{
			// The node.Funcs idiom itself: accumulate into a slice and return it.
			// There is no splat into a Fluent call, so the rule must stay silent -
			// flagging this would flag node.Funcs's own documented usage.
			name:    "accumulate-then-return is the node.Funcs idiom and is left alone",
			imports: []string{nodePkg, divPkg},
			body: `_ = node.Funcs(func() []node.Node {
	out := []node.Node{}
	for _, x := range xs {
		out = append(out, div.New())
	}
	return out
})`,
		},
		{
			// The slice comes from a function call, not a make/composite literal,
			// so its contents are unknown - the rule only handles slices it can see
			// being built, and must leave this alone.
			name:    "slice from a function call is left alone",
			imports: []string{nodePkg, divPkg},
			body: `kids := makeKids()
_ = div.New(kids...)`,
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

// TestNodeAppendLoopIsAdvisory locks that a plain gathering loop is advisory
// (info), not a defect: building the slice and splatting it is the cheapest
// render-once option, so flint should inform, not warn.
func TestNodeAppendLoopIsAdvisory(t *testing.T) {
	l := New(FluentRegistry())
	body := `rows := make([]node.Node, 0, 4)
for _, it := range items {
	rows = append(rows, div.Text(it))
}
_ = div.New(rows...)`
	src := wrapWithImports([]string{nodePkg, divPkg}, body)
	diags, err := l.Source("test.go", src)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	d, ok := findNodeAppend(diags)
	if !ok {
		t.Fatal("expected a node-append diagnostic")
	}
	if d.Severity != Info {
		t.Errorf("severity = %v, want Info", d.Severity)
	}
	if !strings.Contains(d.Message, "Fluent can compose these children directly") {
		t.Errorf("message = %q, want advisory phrasing", d.Message)
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
