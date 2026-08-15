package flint

import (
	"strings"
	"testing"
)

func TestAllowDirectiveSuppresses(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name string
		src  string
	}{
		{
			name: "standalone directive applies to the next line",
			src: `package p

import "github.com/jpl-au/fluent/text"

func ErrorStatic(markup string) *text.Node {
	//flint:allow raw-text trusted server-owned markup by documented contract
	return text.RawText(markup)
}`,
		},
		{
			name: "trailing directive applies to its own line",
			src: `package p

import "github.com/jpl-au/fluent/text"

func ErrorStatic(markup string) *text.Node {
	return text.RawText(markup) //flint:allow raw-text trusted server-owned markup
}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags, err := l.Source("test.go", []byte(tt.src))
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			for _, d := range diags {
				t.Errorf("unexpected diagnostic: [%s] %s", d.Check, d.Message)
			}
		})
	}
}

func TestAllowDirectiveScope(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name string
		src  string
		want string // a diagnostic that must survive
	}{
		{
			name: "a different check on the covered line still fires",
			src: `package p

import "github.com/jpl-au/fluent/html5/div"

func f(html string) {
	//flint:allow static this covers the wrong check on purpose
	_ = div.New().RawText(html)
}`,
			want: "RawText() first argument must be a string literal",
		},
		{
			name: "the same check on another line still fires",
			src: `package p

import "github.com/jpl-au/fluent/text"

func f(a, b string) {
	//flint:allow raw-text trusted markup
	_ = text.RawText(a)
	_ = text.RawText(b)
}`,
			want: `RawText() first argument must be a string literal; got variable "b"`,
		},
		{
			name: "a directive with no reason suppresses nothing and is flagged",
			src: `package p

import "github.com/jpl-au/fluent/text"

func f(a string) {
	//flint:allow raw-text
	_ = text.RawText(a)
}`,
			want: "//flint:allow needs a check name and a reason",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags, err := l.Source("test.go", []byte(tt.src))
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			found := false
			for _, d := range diags {
				if strings.Contains(d.Message, tt.want) {
					found = true
				}
			}
			if !found {
				t.Errorf("expected a diagnostic containing %q to survive", tt.want)
				for _, d := range diags {
					t.Logf("  [%s] %s", d.Check, d.Message)
				}
			}
		})
	}
}
