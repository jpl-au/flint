package flint

import (
	"fmt"
	"strings"
	"testing"
)

// testRegistry returns a minimal registry for testing symbol validation.
func testRegistry() *Registry {
	return &Registry{
		Packages: map[string]Package{
			"github.com/jpl-au/fluent/html5/div": {
				Functions: map[string]int{"New": -1},
				Types:     map[string]bool{"Element": true},
				Methods: map[string]bool{
					"Class": true, "ID": true, "Text": true,
					"Static": true, "Add": true, "Dynamic": true,
				},
			},
			"github.com/jpl-au/fluent/html5/input": {
				Functions: map[string]int{
					"New": 0, "Text": 2, "Email": 1,
					"Password": 1, "Checkbox": 2,
				},
				Types: map[string]bool{"Element": true},
				Methods: map[string]bool{
					"Class": true, "ID": true, "Name": true,
					"Value": true, "Type": true, "Required": true,
					"Placeholder": true,
				},
			},
			"github.com/jpl-au/fluent/text": {
				Functions: map[string]int{
					"Static": 1, "Text": 1, "RawText": 1,
					"Textf": -1, "RawTextf": -1,
				},
				Types: map[string]bool{"Node": true},
			},
			"github.com/jpl-au/fluent/node": {
				Functions: map[string]int{
					"Condition": 1, "When": 2, "Unless": 2,
					"Func": 1, "Funcs": 1, "Memoise": 2,
				},
				Types: map[string]bool{
					"Node": true, "Element": true, "Dynamic": true,
					"Attribute": true, "Memoiser": true,
				},
			},
			"github.com/jpl-au/fluent/html5/attr/inputtype": {
				Vars: map[string]bool{
					"Text": true, "Email": true, "Password": true,
				},
				Functions: map[string]int{"Custom": 1},
			},
		},
	}
}

// wrapWithImports builds a valid Go file from a snippet with custom imports.
func wrapWithImports(imports []string, body string) []byte {
	var importBlock strings.Builder
	for _, imp := range imports {
		importBlock.WriteString(fmt.Sprintf("\t%q\n", imp))
	}
	return fmt.Appendf(nil, `package example

import (
%s)

func build() {
	%s
}
`, importBlock.String(), body)
}

func TestCheckSymbolsNoRegistry(t *testing.T) {
	// With no registry set, symbol checks should be skipped.
	old := activeRegistry
	activeRegistry = nil
	defer func() { activeRegistry = old }()

	src := wrapWithImports(
		[]string{"github.com/jpl-au/fluent/node"},
		`_ = node.Fragment()`,
	)
	diags, err := Source("test.go", src)
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
	old := activeRegistry
	activeRegistry = testRegistry()
	defer func() { activeRegistry = old }()

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
			diags, err := Source("test.go", src)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			// Filter to only symbol diagnostics (ignore Static/RawText checks).
			var symbolDiags []Diagnostic
			for _, d := range diags {
				if d.Fix == "" {
					continue
				}
				if d.Fix != "Use .Text() or .Textf() for dynamic content, or pass a string literal to .Static()" &&
					d.Fix != "Use .Text() or .Textf() for dynamic content, or pass a string literal to .RawText()" {
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
	old := activeRegistry
	activeRegistry = testRegistry()
	defer func() { activeRegistry = old }()

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
			diags, err := Source("test.go", src)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			found := false
			for _, d := range diags {
				if d.Message == tt.want {
					found = true
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
	old := activeRegistry
	activeRegistry = testRegistry()
	defer func() { activeRegistry = old }()

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
			diags, err := Source("test.go", src)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			found := false
			for _, d := range diags {
				if d.Message == tt.want {
					found = true
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

func TestCheckSymbolsAliasedImport(t *testing.T) {
	old := activeRegistry
	activeRegistry = testRegistry()
	defer func() { activeRegistry = old }()

	src := []byte(`package example

import d "github.com/jpl-au/fluent/html5/div"

func build() {
	_ = d.New().Class("x")
	_ = d.Fragment()
}
`)
	diags, err := Source("test.go", src)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	found := false
	for _, d := range diags {
		if d.Message == "div.Fragment does not exist" {
			found = true
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
	old := activeRegistry
	activeRegistry = testRegistry()
	defer func() { activeRegistry = old }()

	// Imports not in the registry should be silently ignored.
	src := wrapWithImports(
		[]string{"fmt"},
		`_ = fmt.Sprintf("hello")`,
	)
	diags, err := Source("test.go", src)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	for _, d := range diags {
		if d.Message == "fmt.Sprintf does not exist" {
			t.Error("should not flag imports outside the registry")
		}
	}
}
