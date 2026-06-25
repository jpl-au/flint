package flint

import (
	"strings"
	"testing"
)

func TestCheckSymbolsNoRegistry(t *testing.T) {
	l := New(nil)

	src := wrapWithImports(
		[]string{"github.com/jpl-au/fluent/node"},
		`_ = node.Fragment()`,
	)
	diags, err := l.Source("test.go", src)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	// Only Static/RawText checks run; no symbol check.
	for _, d := range diags {
		if d.Message == "node.Fragment does not exist" {
			t.Error("symbol check ran without a registry")
		}
	}
}

func TestCheckSymbolsValidCalls(t *testing.T) {
	l := New(testRegistry())

	tests := []struct {
		name    string
		imports []string
		body    string
	}{
		{
			name:    "div.New is valid",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `_ = div.New()`,
		},
		{
			name:    "input.Email is valid",
			imports: []string{"github.com/jpl-au/fluent/html5/input"},
			body:    `_ = input.Email("email")`,
		},
		{
			name:    "node.Condition is valid",
			imports: []string{"github.com/jpl-au/fluent/node"},
			body:    `_ = node.Condition(true)`,
		},
		{
			name:    "node.Map is valid",
			imports: []string{"github.com/jpl-au/fluent/node"},
			body:    `_ = node.Map([]int{1, 2}, func(int) node.Node { return nil })`,
		},
		{
			name:    "security.HTML is valid",
			imports: []string{"github.com/jpl-au/fluent-security"},
			body:    `_ = security.HTML("<b>x</b>")`,
		},
		{
			name:    "security.FromPolicy is valid",
			imports: []string{"github.com/jpl-au/fluent-security"},
			body:    `_ = security.FromPolicy(nil)`,
		},
		{
			name:    "text.Static is valid",
			imports: []string{"github.com/jpl-au/fluent/text"},
			body:    `_ = text.Static("hello")`,
		},
		{
			name:    "inputtype.Email is valid",
			imports: []string{"github.com/jpl-au/fluent/html5/attr/inputtype"},
			body:    `_ = inputtype.Email`,
		},
		{
			name:    "inputtype.Custom is valid",
			imports: []string{"github.com/jpl-au/fluent/html5/attr/inputtype"},
			body:    `_ = inputtype.Custom("x")`,
		},
		{
			name:    "method chain is valid",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `_ = div.New().Class("x").ID("y").Text("hello")`,
		},
		{
			name:    "node.Node type reference is valid",
			imports: []string{"github.com/jpl-au/fluent/node"},
			body:    `var _ node.Node`,
		},
		{
			name:    "node.Element type reference is valid",
			imports: []string{"github.com/jpl-au/fluent/node"},
			body:    `var _ node.Element`,
		},
		{
			name:    "div.Element type reference is valid",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `var _ *div.Element`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := wrapWithImports(tt.imports, tt.body)
			diags, err := l.Source("test.go", src)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			// Filter to only symbol diagnostics (ignore Static/RawText checks).
			var symbolDiags []Diagnostic
			for _, d := range diags {
				if d.Fix == "" {
					continue
				}
				if !strings.Contains(d.Fix, "replace Static with Text") &&
					!strings.Contains(d.Fix, "replace RawText with Text") {
					symbolDiags = append(symbolDiags, d)
				}
			}
			if len(symbolDiags) > 0 {
				t.Errorf("got %d unexpected diagnostics", len(symbolDiags))
				for _, d := range symbolDiags {
					t.Logf("  %s: %s", d.Pos, d.Message)
				}
			}
		})
	}
}

func TestCheckSymbolsInvalidPackageFunction(t *testing.T) {
	l := New(testRegistry())

	tests := []struct {
		name    string
		imports []string
		body    string
		want    string
	}{
		{
			name:    "node.Fragment does not exist",
			imports: []string{"github.com/jpl-au/fluent/node"},
			body:    `_ = node.Fragment()`,
			want:    "node.Fragment does not exist",
		},
		{
			name:    "security.CleanUGC does not exist",
			imports: []string{"github.com/jpl-au/fluent-security"},
			body:    `_ = security.CleanUGC("<b>x</b>")`,
			want:    "security.CleanUGC does not exist",
		},
		{
			name:    "security.NewCleaner does not exist",
			imports: []string{"github.com/jpl-au/fluent-security"},
			body:    `_ = security.NewCleaner(nil)`,
			want:    "security.NewCleaner does not exist",
		},
		{
			name:    "div.Email does not exist",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `_ = div.Email("x")`,
			want:    "div.Email does not exist",
		},
		{
			name:    "inputtype.Telephone does not exist",
			imports: []string{"github.com/jpl-au/fluent/html5/attr/inputtype"},
			body:    `_ = inputtype.Telephone`,
			want:    "inputtype.Telephone does not exist",
		},
		{
			name:    "text.Format does not exist",
			imports: []string{"github.com/jpl-au/fluent/text"},
			body:    `_ = text.Format("x")`,
			want:    "text.Format does not exist",
		},
		{
			name:    "node.Fragment type does not exist",
			imports: []string{"github.com/jpl-au/fluent/node"},
			body:    `var _ node.Fragment`,
			want:    "node.Fragment does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := wrapWithImports(tt.imports, tt.body)
			diags, err := l.Source("test.go", src)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
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
				t.Errorf("expected diagnostic %q, got:", tt.want)
				for _, d := range diags {
					t.Logf("  %s: %s", d.Pos, d.Message)
				}
			}
		})
	}
}

func TestCheckSymbolsInvalidMethod(t *testing.T) {
	l := New(testRegistry())

	tests := []struct {
		name    string
		imports []string
		body    string
		want    string
	}{
		{
			name:    "div has no Href method",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `_ = div.New().Href("/")`,
			want:    "method Href does not exist on this element",
		},
		{
			name:    "input has no Content method",
			imports: []string{"github.com/jpl-au/fluent/html5/input"},
			body:    `_ = input.Email("x").Content("y")`,
			want:    "method Content does not exist on this element",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := wrapWithImports(tt.imports, tt.body)
			diags, err := l.Source("test.go", src)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
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
				t.Errorf("expected diagnostic %q, got:", tt.want)
				for _, d := range diags {
					t.Logf("  %s: %s", d.Pos, d.Message)
				}
			}
		})
	}
}

// TestCheckSymbolsResolvesForeignReturn covers a method called on the result of
// a function whose return type is recorded (security.PlainText returns a
// node.Node, which has Render/Nodes). flint must follow the return type and
// stay silent - no diagnostic at all.
func TestCheckSymbolsResolvesForeignReturn(t *testing.T) {
	l := New(testRegistry())
	for _, body := range []string{
		`_ = security.PlainText("x").Render()`,
		`_ = security.HTML("x").Nodes()`,
	} {
		t.Run(body, func(t *testing.T) {
			src := wrapWithImports([]string{"github.com/jpl-au/fluent-security"}, body)
			diags, err := l.Source("test.go", src)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			for _, d := range diags {
				if strings.Contains(d.Message, "method ") {
					t.Errorf("unexpected method diagnostic: %s", d.Message)
				}
			}
		})
	}
}

// TestCheckSymbolsForeignReturnRejectsBadMethod checks that once the return
// type is known, a method that genuinely is not on it is a firm error -
// node.Node has no Frobnicate.
func TestCheckSymbolsForeignReturnRejectsBadMethod(t *testing.T) {
	l := New(testRegistry())
	src := wrapWithImports([]string{"github.com/jpl-au/fluent-security"},
		`_ = security.PlainText("x").Frobnicate()`)
	diags, err := l.Source("test.go", src)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	var found bool
	for _, d := range diags {
		if strings.Contains(d.Message, "method Frobnicate") {
			found = true
			if d.Severity != Error {
				t.Errorf("severity = %v, want Error (the return type is known)", d.Severity)
			}
			if !strings.Contains(d.Message, "node.Node") {
				t.Errorf("message = %q, want it to name the resolved type", d.Message)
			}
		}
	}
	if !found {
		t.Fatalf("expected a firm method diagnostic, got %d diagnostics", len(diags))
	}
}

// TestCheckSymbolsHedgesUnresolved covers methods on a non-element package whose
// receiver type flint cannot resolve - a function with no recorded return type
// (security.New) or a deeper method chain (Cleaner.Clean itself returns a
// node.Node, so the convention that methods return the element does not hold).
// Both must hedge with a Warning rather than assert a hard error.
func TestCheckSymbolsHedgesUnresolved(t *testing.T) {
	l := New(testRegistry())
	tests := []struct {
		name string
		body string
	}{
		{"unrecorded function return", `_ = security.New().Frobnicate()`},
		{"deeper method chain", `_ = security.New().Allow("b").Frobnicate()`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := wrapWithImports([]string{"github.com/jpl-au/fluent-security"}, tt.body)
			diags, err := l.Source("test.go", src)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			var found bool
			for _, d := range diags {
				if strings.Contains(d.Message, "method Frobnicate") {
					found = true
					if d.Severity != Warning {
						t.Errorf("severity = %v, want Warning (unresolved receiver)", d.Severity)
					}
					if !strings.Contains(d.Fix, "false positive") {
						t.Errorf("fix = %q, want it to flag the possible false positive", d.Fix)
					}
				}
			}
			if !found {
				t.Fatalf("expected a hedged method diagnostic, got %d diagnostics", len(diags))
			}
		})
	}
}

func TestCheckSymbolsAliasedImport(t *testing.T) {
	l := New(testRegistry())

	src := []byte(`package example

import d "github.com/jpl-au/fluent/html5/div"

func build() {
	_ = d.New().Class("x")
	_ = d.Fragment()
}
`)
	diags, err := l.Source("test.go", src)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	found := false
	for _, d := range diags {
		if d.Message == "div.Fragment does not exist" {
			found = true
			if d.Severity != Error {
				t.Errorf("severity = %v, want Error", d.Severity)
			}
			break
		}
	}
	if !found {
		t.Error("expected diagnostic for aliased import d.Fragment")
		for _, d := range diags {
			t.Logf("  %s: %s", d.Pos, d.Message)
		}
	}
}

func TestCheckSymbolsUnknownImportIgnored(t *testing.T) {
	l := New(testRegistry())

	// Imports not in the registry should be silently ignored.
	src := wrapWithImports(
		[]string{"fmt"},
		`_ = fmt.Sprintf("hello")`,
	)
	diags, err := l.Source("test.go", src)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	for _, d := range diags {
		if d.Message == "fmt.Sprintf does not exist" {
			t.Error("should not flag imports outside the registry")
		}
	}
}
