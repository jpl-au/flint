package flint

import (
	"strings"
	"testing"
)

func TestCheckStaticPositive(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name string
		src  []byte
		want string
	}{
		{
			name: "variable argument",
			src:  wrap(`name := "world"; _ = div.New().Static(name)`),
			want: `Static() argument must be a string literal; got variable "name"`,
		},
		{
			name: "binary expression",
			src:  wrap(`name := "world"; _ = div.New().Static("hello " + name)`),
			want: "Static() argument must be a string literal; got binary expression",
		},
		{
			name: "fmt.Sprintf call",
			src:  wrap(`name := "world"; _ = div.New().Static(fmt.Sprintf("hello %s", name))`),
			want: "Static() argument must be a string literal; got function call",
		},
		{
			name: "a function not named for the idiom forwarding its parameter",
			src: []byte(`package p

import "github.com/jpl-au/fluent/text"

func Label(txt string) *text.Node {
	return text.Static(txt)
}`),
			want: `Static() argument must be a string literal; got variable "txt"`,
		},
		{
			name: "a Static wrapper passing something other than its parameter",
			src: []byte(`package p

import "github.com/jpl-au/fluent/text"

func Static(label string) *text.Node {
	derived := label + "!"
	return text.Static(derived)
}`),
			want: `Static() argument must be a string literal; got variable "derived"`,
		},
		{
			name: "package-level text.Static with variable",
			src:  wrap(`name := "world"; _ = text.Static(name)`),
			want: `Static() argument must be a string literal; got variable "name"`,
		},
		{
			name: "chained Static with variable",
			src: wrapWithImports(
				[]string{"github.com/jpl-au/fluent/html5/div"},
				`name := "world"; _ = div.New().Class("x").Static(name)`,
			),
			want: `Static() argument must be a string literal; got variable "name"`,
		},
		{
			name: "constructor Static with variable",
			src: wrapWithImports(
				[]string{"github.com/jpl-au/fluent/html5/div"},
				`name := "world"; _ = div.Static(name)`,
			),
			want: `Static() argument must be a string literal; got variable "name"`,
		},
		{
			name: "chained Static as return value",
			src: wrapReturningFunc(
				[]string{"github.com/jpl-au/fluent/html5/span"},
				`return span.New().Class("label").Static(colour)`,
			),
			want: `Static() argument must be a string literal; got variable "colour"`,
		},
		{
			name: "multiple violations all reported",
			src:  wrap(`name := "x"; _ = div.New().Static(name); _ = text.Static(name)`),
			want: `Static() argument must be a string literal; got variable "name"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags, err := l.Source("test.go", tt.src)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
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

func TestCheckStaticNegative(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name string
		src  []byte
	}{
		{
			name: "string literal is valid",
			src:  wrap(`_ = div.New().Static("hello")`),
		},
		{
			name: "raw string literal is valid",
			src:  wrap("_ = div.New().Static(`raw content`)"),
		},
		{
			name: "package-level Static with literal is valid",
			src:  wrap(`_ = text.Static("hello")`),
		},
		{
			name: "Text with dynamic content is fine",
			src:  wrap(`name := "world"; _ = div.New().Text(name)`),
		},
		{
			name: "Text with binary expression is fine",
			src:  wrap(`name := "world"; _ = div.New().Text("hello " + name)`),
		},
		{
			name: "non-fluent Static is not flagged",
			src: wrapWithImports(
				[]string{"example.com/mylib"},
				`name := "world"; _ = mylib.New().Static(name)`,
			),
		},
		{
			name: "pass-through idiom: Static wrapper forwarding its parameter",
			src: []byte(`package p

import "github.com/jpl-au/fluent/text"

func Static(label string) *text.Node {
	return text.Static(label)
}`),
		},
		{
			name: "pass-through idiom: Static-suffixed method forwarding its parameter",
			src: []byte(`package p

import "github.com/jpl-au/fluent/text"

type Form struct{}

func (f *Form) LabelStatic(txt string) *text.Node {
	return text.Static(txt)
}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags, err := l.Source("test.go", tt.src)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			for _, d := range diags {
				if strings.Contains(d.Message, "Static() argument must be a string literal") {
					t.Errorf("unexpected diagnostic: %s", d.Message)
				}
			}
		})
	}
}

func TestStaticFixMessage(t *testing.T) {
	l := New(nil)
	src := wrap(`name := "world"; _ = div.New().Static(name)`)
	diags, err := l.Source("test.go", src)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(diags) != 1 {
		t.Fatalf("got %d diagnostics, want 1", len(diags))
	}
	want := "Static() is for string literals only (JIT pre-rendering); replace Static with Text or Textf for dynamic content"
	if diags[0].Fix != want {
		t.Errorf("Fix = %q, want %q", diags[0].Fix, want)
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
