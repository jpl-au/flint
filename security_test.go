package flint

import (
	"strings"
	"testing"
)

func TestCheckRawTextPositive(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name string
		src  []byte
		want string
	}{
		{
			name: "RawText with variable",
			src:  wrap(`html := "<b>x</b>"; _ = div.New().RawText(html)`),
			want: `RawText() first argument must be a string literal; got variable "html"`,
		},
		{
			name: "RawTextf with variable format",
			src:  wrap(`tpl := "<b>%s</b>"; _ = div.New().RawTextf(tpl, "x")`),
			want: `RawTextf() first argument must be a string literal; got variable "tpl"`,
		},
		{
			name: "RawText with binary expression",
			src:  wrap(`tag := "b"; _ = div.New().RawText("<" + tag + ">")`),
			want: "RawText() first argument must be a string literal; got binary expression",
		},
		{
			name: "RawText with function call",
			src:  wrap(`_ = div.New().RawText(fmt.Sprintf("<b>%s</b>", "x"))`),
			want: "RawText() first argument must be a string literal; got function call",
		},
		{
			name: "package-level RawText with variable",
			src:  wrap(`html := "<br/>"; _ = text.RawText(html)`),
			want: `RawText() first argument must be a string literal; got variable "html"`,
		},
		{
			name: "chained RawText with variable",
			src: wrapWithImports(
				[]string{"github.com/jpl-au/fluent/html5/div"},
				`html := "<b>x</b>"; _ = div.New().Class("content").RawText(html)`,
			),
			want: `RawText() first argument must be a string literal; got variable "html"`,
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

func TestCheckRawTextNegative(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name string
		src  []byte
	}{
		{
			name: "RawText with string literal is valid",
			src:  wrap(`_ = div.New().RawText("<strong>bold</strong>")`),
		},
		{
			name: "RawTextf with string literal format is valid",
			src:  wrap(`_ = div.New().RawTextf("<em>%s</em>", "ok")`),
		},
		{
			name: "package-level RawText with literal is valid",
			src:  wrap(`_ = text.RawText("<br/>")`),
		},
		{
			name: "non-fluent RawText is not flagged",
			src: wrapWithImports(
				[]string{"example.com/mylib"},
				`html := "<b>x</b>"; _ = mylib.New().RawText(html)`,
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags, err := l.Source("test.go", tt.src)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			for _, d := range diags {
				if strings.Contains(d.Message, "first argument must be a string literal") {
					t.Errorf("unexpected diagnostic: %s", d.Message)
				}
			}
		})
	}
}

func TestRawTextFixMessage(t *testing.T) {
	l := New(nil)
	src := wrap(`html := "<b>x</b>"; _ = div.New().RawText(html)`)
	diags, err := l.Source("test.go", src)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(diags) != 1 {
		t.Fatalf("got %d diagnostics, want 1", len(diags))
	}
	want := "RawText() bypasses HTML escaping and must use a string literal; use fluent-security's HTML(input) to sanitise untrusted HTML, or replace RawText with Text or Textf for plain-text content"
	if diags[0].Fix != want {
		t.Errorf("Fix = %q, want %q", diags[0].Fix, want)
	}
}
