package flint

import (
	"fmt"
	"strings"
	"testing"
)

// wrap places a Go expression inside a minimal valid file so the parser
// can handle it. The imports cover the packages that test snippets use.
func wrap(expr string) []byte {
	return fmt.Appendf(nil, `package example

import (
	"fmt"
	"github.com/jpl-au/fluent/html5/div"
	"github.com/jpl-au/fluent/text"
)

var _ = fmt.Sprintf
var _ = text.Static
var _ = div.New

func build() {
	%s
}
`, expr)
}

func TestCheckStaticLiteral(t *testing.T) {
	l := New(nil)

	tests := []struct {
		name  string
		src   []byte
		count int // expected number of diagnostics
	}{
		{
			name:  "string literal is valid",
			src:   wrap(`_ = div.New().Static("hello")`),
			count: 0,
		},
		{
			name:  "raw string literal is valid",
			src:   wrap("_ = div.New().Static(`raw content`)"),
			count: 0,
		},
		{
			name:  "package-level Static with literal is valid",
			src:   wrap(`_ = text.Static("hello")`),
			count: 0,
		},
		{
			name:  "variable is flagged",
			src:   wrap(`name := "world"; _ = div.New().Static(name)`),
			count: 1,
		},
		{
			name:  "binary expression is flagged",
			src:   wrap(`name := "world"; _ = div.New().Static("hello " + name)`),
			count: 1,
		},
		{
			name:  "fmt.Sprintf call is flagged",
			src:   wrap(`name := "world"; _ = div.New().Static(fmt.Sprintf("hello %s", name))`),
			count: 1,
		},
		{
			name:  "package-level Static with variable is flagged",
			src:   wrap(`name := "world"; _ = text.Static(name)`),
			count: 1,
		},
		{
			name:  "multiple violations are all reported",
			src:   wrap(`name := "x"; _ = div.New().Static(name); _ = text.Static(name)`),
			count: 2,
		},
		{
			name:  "no Static calls produces no diagnostics",
			src:   wrap(`name := "world"; _ = div.New().Text(name)`),
			count: 0,
		},
		{
			name:  "Text with dynamic content is fine",
			src:   wrap(`name := "world"; _ = div.New().Text("hello " + name)`),
			count: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags, err := l.Source("test.go", tt.src)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			if len(diags) != tt.count {
				t.Errorf("got %d diagnostics, want %d", len(diags), tt.count)
				for _, d := range diags {
					t.Logf("  %s: %s", d.Pos, d.Message)
				}
			}
		})
	}
}

func TestCheckRawTextLiteral(t *testing.T) {
	l := New(nil)

	tests := []struct {
		name  string
		src   []byte
		count int
	}{
		{
			name:  "RawText with string literal is valid",
			src:   wrap(`_ = div.New().RawText("<strong>bold</strong>")`),
			count: 0,
		},
		{
			name:  "RawTextf with string literal format is valid",
			src:   wrap(`_ = div.New().RawTextf("<em>%s</em>", "ok")`),
			count: 0,
		},
		{
			name:  "package-level RawText with literal is valid",
			src:   wrap(`_ = text.RawText("<br/>")`),
			count: 0,
		},
		{
			name:  "RawText with variable is flagged",
			src:   wrap(`html := "<b>x</b>"; _ = div.New().RawText(html)`),
			count: 1,
		},
		{
			name:  "RawTextf with variable format is flagged",
			src:   wrap(`tpl := "<b>%s</b>"; _ = div.New().RawTextf(tpl, "x")`),
			count: 1,
		},
		{
			name:  "RawText with binary expression is flagged",
			src:   wrap(`tag := "b"; _ = div.New().RawText("<" + tag + ">")`),
			count: 1,
		},
		{
			name:  "RawText with function call is flagged",
			src:   wrap(`_ = div.New().RawText(fmt.Sprintf("<b>%s</b>", "x"))`),
			count: 1,
		},
		{
			name:  "package-level RawText with variable is flagged",
			src:   wrap(`html := "<br/>"; _ = text.RawText(html)`),
			count: 1,
		},
		{
			name:  "mixed Static and RawText violations",
			src:   wrap(`v := "x"; _ = div.New().Static(v); _ = div.New().RawText(v)`),
			count: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags, err := l.Source("test.go", tt.src)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			if len(diags) != tt.count {
				t.Errorf("got %d diagnostics, want %d", len(diags), tt.count)
				for _, d := range diags {
					t.Logf("  %s: %s", d.Pos, d.Message)
				}
			}
		})
	}
}

func TestRawTextDiagnosticMessage(t *testing.T) {
	l := New(nil)
	src := wrap(`html := "<b>x</b>"; _ = div.New().RawText(html)`)
	diags, err := l.Source("test.go", src)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(diags) != 1 {
		t.Fatalf("got %d diagnostics, want 1", len(diags))
	}

	want := `RawText() first argument must be a string literal; got variable "html"`
	if diags[0].Message != want {
		t.Errorf("Message = %q, want %q", diags[0].Message, want)
	}
}

func TestRawTextfDiagnosticMessage(t *testing.T) {
	l := New(nil)
	src := wrap(`tpl := "<b>%s</b>"; _ = div.New().RawTextf(tpl, "x")`)
	diags, err := l.Source("test.go", src)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(diags) != 1 {
		t.Fatalf("got %d diagnostics, want 1", len(diags))
	}

	want := `RawTextf() first argument must be a string literal; got variable "tpl"`
	if diags[0].Message != want {
		t.Errorf("Message = %q, want %q", diags[0].Message, want)
	}
}

func TestSourceReturnsParseError(t *testing.T) {
	l := New(nil)
	_, err := l.Source("bad.go", []byte("not valid go"))
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestDiagnosticPositions(t *testing.T) {
	l := New(nil)
	src := wrap(`name := "world"; _ = div.New().Static(name)`)
	diags, err := l.Source("test.go", src)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(diags) != 1 {
		t.Fatalf("got %d diagnostics, want 1", len(diags))
	}

	d := diags[0]
	if d.Pos.Filename != "test.go" {
		t.Errorf("Pos.Filename = %q, want %q", d.Pos.Filename, "test.go")
	}
	if d.Pos.Line == 0 {
		t.Error("Pos.Line should not be zero")
	}
	if d.Pos.Column == 0 {
		t.Error("Pos.Column should not be zero")
	}
	if d.Message == "" {
		t.Error("Message should not be empty")
	}
	if d.Fix == "" {
		t.Error("Fix should not be empty")
	}
}

func TestDescribeExpr(t *testing.T) {
	l := New(nil)
	src := wrap(`name := "world"; _ = div.New().Static(name)`)
	diags, err := l.Source("test.go", src)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(diags) != 1 {
		t.Fatalf("got %d diagnostics, want 1", len(diags))
	}

	want := `Static() argument must be a string literal; got variable "name"`
	if diags[0].Message != want {
		t.Errorf("Message = %q, want %q", diags[0].Message, want)
	}
}

func TestCheckStaticScopedToFluent(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name    string
		imports []string
		body    string
		count   int
	}{
		{
			name:    "fluent Static with variable is flagged",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `name := "world"; _ = div.New().Static(name)`,
			count:   1,
		},
		{
			name:    "non-fluent Static with variable is not flagged",
			imports: []string{"example.com/mylib"},
			body:    `name := "world"; _ = mylib.New().Static(name)`,
			count:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := wrapWithImports(tt.imports, tt.body)
			diags, err := l.Source("test.go", src)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			staticDiags := 0
			for _, d := range diags {
				if strings.Contains(d.Message, "Static() argument must be a string literal") {
					staticDiags++
				}
			}
			if staticDiags != tt.count {
				t.Errorf("got %d static diagnostics, want %d", staticDiags, tt.count)
				for _, d := range diags {
					t.Logf("  %s: %s", d.Pos, d.Message)
				}
			}
		})
	}
}

func TestCheckRawTextScopedToFluent(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name    string
		imports []string
		body    string
		count   int
	}{
		{
			name:    "fluent RawText with variable is flagged",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `html := "<b>x</b>"; _ = div.New().RawText(html)`,
			count:   1,
		},
		{
			name:    "non-fluent RawText with variable is not flagged",
			imports: []string{"example.com/mylib"},
			body:    `html := "<b>x</b>"; _ = mylib.New().RawText(html)`,
			count:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := wrapWithImports(tt.imports, tt.body)
			diags, err := l.Source("test.go", src)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			rawDiags := 0
			for _, d := range diags {
				if strings.Contains(d.Message, "first argument must be a string literal") {
					rawDiags++
				}
			}
			if rawDiags != tt.count {
				t.Errorf("got %d raw text diagnostics, want %d", rawDiags, tt.count)
				for _, d := range diags {
					t.Logf("  %s: %s", d.Pos, d.Message)
				}
			}
		})
	}
}
