package flint

import (
	"strings"
	"testing"
)

func TestCheckTypedConstructors(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name    string
		imports []string
		body    string
		want    string // empty means no diagnostic expected
	}{
		{
			name:    "ul.New with li children suggests Items",
			imports: []string{"github.com/jpl-au/fluent/html5/ul", "github.com/jpl-au/fluent/html5/li"},
			body:    `_ = ul.New(li.Text("a"), li.Text("b"))`,
			want:    "use ul.Items(...) instead of ul.New(...) for type-safe child nesting",
		},
		{
			name:    "ol.New with li children suggests Items",
			imports: []string{"github.com/jpl-au/fluent/html5/ol", "github.com/jpl-au/fluent/html5/li"},
			body:    `_ = ol.New(li.Text("one"), li.Text("two"))`,
			want:    "use ol.Items(...) instead of ol.New(...) for type-safe child nesting",
		},
		{
			name:    "tbody.New with tr children suggests Rows",
			imports: []string{"github.com/jpl-au/fluent/html5/tbody", "github.com/jpl-au/fluent/html5/tr"},
			body:    `_ = tbody.New(tr.New())`,
			want:    "use tbody.Rows(...) instead of tbody.New(...) for type-safe child nesting",
		},
		{
			name:    "tr.New with td children suggests Cells",
			imports: []string{"github.com/jpl-au/fluent/html5/tr", "github.com/jpl-au/fluent/html5/td"},
			body:    `_ = tr.New(td.Text("a"), td.Text("b"))`,
			want:    "use tr.Cells(...) instead of tr.New(...) for type-safe child nesting",
		},
		{
			name:    "tr.New with th children suggests Headers",
			imports: []string{"github.com/jpl-au/fluent/html5/tr", "github.com/jpl-au/fluent/html5/th"},
			body:    `_ = tr.New(th.Col("Name"), th.Col("Age"))`,
			want:    "use tr.Headers(...) instead of tr.New(...) for type-safe child nesting",
		},
		{
			name:    "chained child calls still detected",
			imports: []string{"github.com/jpl-au/fluent/html5/ul", "github.com/jpl-au/fluent/html5/li"},
			body:    `_ = ul.New(li.Text("a").Class("item"))`,
			want:    "use ul.Items(...) instead of ul.New(...) for type-safe child nesting",
		},
		{
			name:    "mixed children not flagged",
			imports: []string{"github.com/jpl-au/fluent/html5/tr", "github.com/jpl-au/fluent/html5/td", "github.com/jpl-au/fluent/html5/th"},
			body:    `_ = tr.New(th.Col("Name"), td.Text("Alice"))`,
		},
		{
			name:    "New with no args not flagged",
			imports: []string{"github.com/jpl-au/fluent/html5/ul"},
			body:    `_ = ul.New()`,
		},
		{
			name:    "div.New with mixed children not flagged (no typed constructor)",
			imports: []string{"github.com/jpl-au/fluent/html5/div", "github.com/jpl-au/fluent/html5/p", "github.com/jpl-au/fluent/html5/span"},
			body:    `_ = div.New(p.Text("a"), span.Text("b"))`,
		},
		{
			name:    "already using typed constructor not flagged",
			imports: []string{"github.com/jpl-au/fluent/html5/ul", "github.com/jpl-au/fluent/html5/li"},
			body:    `_ = ul.Items(li.Text("a"), li.Text("b"))`,
		},
		{
			name:    "non-call arguments not flagged",
			imports: []string{"github.com/jpl-au/fluent/html5/ul"},
			body:    `var n node.Node; _ = ul.New(n)`,
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
					if strings.Contains(d.Message, "type-safe child nesting") {
						t.Errorf("unexpected typed constructor diagnostic: %s", d.Message)
					}
				}
				return
			}

			found := false
			for _, d := range diags {
				if d.Message == tt.want {
					found = true
					if d.Severity != Warning {
						t.Errorf("severity = %v, want Warning", d.Severity)
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

func TestCheckDeferredConstructors(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name    string
		imports []string
		body    string
		want    string // empty means no diagnostic expected
	}{
		{
			name:    "ul.New with node.Map suggests ItemsOf",
			imports: []string{"github.com/jpl-au/fluent/html5/ul", "github.com/jpl-au/fluent/node"},
			body:    `_ = ul.New(node.Map(users, mapper))`,
			want:    "use ul.ItemsOf(...) instead of ul.New(node.Map(...)) for type-safe child nesting",
		},
		{
			name:    "dropdown.New with node.Funcs suggests OptionsFunc",
			imports: []string{"github.com/jpl-au/fluent/html5/dropdown", "github.com/jpl-au/fluent/node"},
			body:    `_ = dropdown.New(node.Funcs(build))`,
			want:    "use dropdown.OptionsFunc(...) instead of dropdown.New(node.Funcs(...)) for type-safe child nesting",
		},
		// A keyed run is left alone in both forms. Moving the key onto the
		// element changes which region the Differ patches, so there is no
		// one-line fix to suggest.
		{
			name:    "keyed Map run not flagged",
			imports: []string{"github.com/jpl-au/fluent/html5/ul", "github.com/jpl-au/fluent/node"},
			body:    `_ = ul.New(node.Map(users, mapper).Dynamic("list"))`,
		},
		{
			name:    "keyed Funcs run not flagged",
			imports: []string{"github.com/jpl-au/fluent/html5/ul", "github.com/jpl-au/fluent/node"},
			body:    `_ = ul.New(node.Funcs(build).Dynamic("list"))`,
		},
		{
			name:    "untyped container not flagged",
			imports: []string{"github.com/jpl-au/fluent/html5/div", "github.com/jpl-au/fluent/node"},
			body:    `_ = div.New(node.Map(users, mapper))`,
		},
		{
			name:    "mixed children not flagged",
			imports: []string{"github.com/jpl-au/fluent/html5/ul", "github.com/jpl-au/fluent/html5/li", "github.com/jpl-au/fluent/node"},
			body:    `_ = ul.New(node.Map(users, mapper), li.Text("fixed"))`,
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
					if strings.Contains(d.Message, "type-safe child nesting") {
						t.Errorf("unexpected typed constructor diagnostic: %s", d.Message)
					}
				}
				return
			}

			found := false
			for _, d := range diags {
				if d.Message == tt.want {
					found = true
					if d.Severity != Warning {
						t.Errorf("severity = %v, want Warning", d.Severity)
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

// TestCheckDeferredConstructorsFixText pins the parts of the fix text the
// check must always carry: the mapper redeclaration, the nil-skip semantics,
// and the second plain constructor where one exists (tr).
func TestCheckDeferredConstructorsFixText(t *testing.T) {
	l := New(FluentRegistry())

	src := wrapWithImports(
		[]string{"github.com/jpl-au/fluent/html5/tr", "github.com/jpl-au/fluent/node"},
		`_ = tr.New(node.Map(cells, mapper))`,
	)
	diags, err := l.Source("test.go", src)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	for _, d := range diags {
		if !strings.Contains(d.Message, "tr.CellsOf") {
			continue
		}
		for _, frag := range []string{
			"redeclare the mapper to return *td.Element",
			"skips a nil result",
			"use tr.HeadersOf for *th.Element children",
		} {
			if !strings.Contains(d.Fix, frag) {
				t.Errorf("fix text missing %q:\n%s", frag, d.Fix)
			}
		}
		return
	}
	t.Fatalf("no CellsOf diagnostic reported; got %d diagnostics", len(diags))
}
