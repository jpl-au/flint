package flint

import (
	"strings"
	"testing"
)

// TestSeverity verifies that every check type assigns the correct severity
// level. Warnings indicate improvable but functional code; errors indicate
// code that is incorrect or will not compile.
func TestSeverity(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name         string
		src          []byte
		wantMsg      string
		wantSeverity Severity
	}{
		// Warnings: code works but could be better.
		{
			name:         "checkStatic variable argument",
			src:          wrap(`name := "world"; _ = div.New().Static(name)`),
			wantMsg:      "Static() argument must be a string literal",
			wantSeverity: Warning,
		},
		{
			name:         "checkRawText variable argument",
			src:          wrap(`html := "<b>x</b>"; _ = div.New().RawText(html)`),
			wantMsg:      "first argument must be a string literal",
			wantSeverity: Warning,
		},
		{
			name: "checkSetAttrKey bypasses dedicated field",
			src: wrapReturningFunc(
				[]string{"github.com/jpl-au/fluent/html5/div"},
				`return div.New().SetAttribute("class", "x")`,
			),
			wantMsg:      "bypasses the dedicated field",
			wantSeverity: Warning,
		},
		{
			name: "checkTypedParams string instead of constant",
			src: wrapWithImports(
				[]string{"github.com/jpl-au/fluent/html5/input"},
				`_ = input.New().Type("email")`,
			),
			wantMsg:      ".Type() expects a typed constant",
			wantSeverity: Warning,
		},
		{
			name: "checkConstructors shorthand available",
			src: wrapWithImports(
				[]string{"github.com/jpl-au/fluent/html5/div"},
				`_ = div.New().Text("hello")`,
			),
			wantMsg:      "use div.Text(...) directly",
			wantSeverity: Warning,
		},
		{
			name: "checkTypedConstructors shorthand available",
			src: wrapWithImports(
				[]string{
					"github.com/jpl-au/fluent/html5/ul",
					"github.com/jpl-au/fluent/html5/li",
				},
				`_ = ul.New(li.Text("a"), li.Text("b"))`,
			),
			wantMsg:      "use ul.Items(...) instead",
			wantSeverity: Warning,
		},

		// Errors: code is incorrect.
		{
			name: "checkSymbols unknown package-level function",
			src: wrapWithImports(
				[]string{"github.com/jpl-au/fluent/node"},
				`_ = node.Fragment()`,
			),
			wantMsg:      "does not exist",
			wantSeverity: Error,
		},
		{
			name: "checkSymbols unknown method",
			src: wrapWithImports(
				[]string{"github.com/jpl-au/fluent/html5/div"},
				`_ = div.New().Href("/")`,
			),
			wantMsg:      "does not exist",
			wantSeverity: Error,
		},
		{
			name: "checkArity wrong argument count",
			src: wrapWithImports(
				[]string{"github.com/jpl-au/fluent/html5/input"},
				`_ = input.Email("email", "extra")`,
			),
			wantMsg:      "expects 1 argument(s), got 2",
			wantSeverity: Error,
		},
		{
			name:         "checkImports reserved keyword package",
			src:          []byte("package ui\n\nimport dropdown \"github.com/jpl-au/fluent/html5/select\"\n\nfunc build() { _ = dropdown.New() }\n"),
			wantMsg:      "Go reserved keyword",
			wantSeverity: Error,
		},
		{
			name: "checkSetAttrChain breaks method chain",
			src: wrapWithImports(
				[]string{"github.com/jpl-au/fluent/html5/div"},
				`_ = div.New().SetAttribute("x-data", "{}").Class("c")`,
			),
			wantMsg:      "SetAttribute does not return the element",
			wantSeverity: Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags, err := l.Source("test.go", tt.src)
			if err != nil {
				t.Fatalf("Source() returned error: %v", err)
			}

			var found bool
			for _, d := range diags {
				if !strings.Contains(d.Message, tt.wantMsg) {
					continue
				}
				found = true
				if d.Severity != tt.wantSeverity {
					t.Errorf("diagnostic %q: got severity %v, want %v",
						d.Message, d.Severity, tt.wantSeverity)
				}
				break
			}

			if !found {
				t.Errorf("no diagnostic containing %q\ngot:", tt.wantMsg)
				for _, d := range diags {
					t.Errorf("  [%v] %s", d.Severity, d.Message)
				}
			}
		})
	}
}
