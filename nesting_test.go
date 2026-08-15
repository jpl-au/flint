package flint

import "testing"

func TestCheckNestingPositive(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name    string
		src     string
		want    string
		wantFix string
	}{
		{
			name: "anchor built by the paired Static constructor inside a.New",
			src: `package p

import "github.com/jpl-au/fluent/html5/a"

func f() {
	_ = a.New(a.Static("Back")).Href("/home")
}`,
			want:    `a.Static(...) builds another <a> element, so this nests <a> inside <a>; HTML forbids it and browsers unnest the inner element`,
			wantFix: `For text content inside a.New, use text.Static or text.Text from fluent/text; a.Static is the paired constructor that builds a whole <a> element`,
		},
		{
			name: "form inside form",
			src: `package p

import "github.com/jpl-au/fluent/html5/form"

func f() {
	_ = form.New(form.New())
}`,
			want:    `form.New(...) builds another <form> element, so this nests <form> inside <form>; HTML forbids it and browsers unnest the inner element`,
			wantFix: `Restructure so the two <form> elements are siblings`,
		},
		{
			name: "anchor added through Add",
			src: `package p

import "github.com/jpl-au/fluent/html5/a"

func f() {
	_ = a.New().Add(a.Text("inner"))
}`,
			want: `a.Text(...) builds another <a> element, so this nests <a> inside <a>; HTML forbids it and browsers unnest the inner element`,
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
				if d.Check == "nesting" && d.Message == tt.want {
					found = true
					if d.Severity != Warning {
						t.Errorf("severity = %v, want Warning", d.Severity)
					}
					if tt.wantFix != "" && d.Fix != tt.wantFix {
						t.Errorf("fix = %q, want %q", d.Fix, tt.wantFix)
					}
				}
			}
			if !found {
				t.Errorf("expected nesting diagnostic %q", tt.want)
				for _, d := range diags {
					t.Logf("  [%s] %s", d.Check, d.Message)
				}
			}
		})
	}
}

func TestCheckNestingNegative(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name string
		src  string
	}{
		{
			name: "self-nesting elements HTML permits",
			src: `package p

import "github.com/jpl-au/fluent/html5/div"

func f() {
	_ = div.New(div.New(div.Text("deep")))
}`,
		},
		{
			name: "text package children inside an anchor",
			src: `package p

import (
	"github.com/jpl-au/fluent/html5/a"
	"github.com/jpl-au/fluent/text"
)

func f() {
	_ = a.New(text.Static("Back")).Href("/home")
}`,
		},
		{
			name: "different elements nest fine",
			src: `package p

import (
	"github.com/jpl-au/fluent/html5/a"
	"github.com/jpl-au/fluent/html5/span"
)

func f() {
	_ = a.New(span.Text("label")).Href("/home")
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
				if d.Check == "nesting" {
					t.Errorf("unexpected nesting diagnostic: %s", d.Message)
				}
			}
		})
	}
}

func TestNestingSuppressedByAllow(t *testing.T) {
	l := New(FluentRegistry())
	src := `package p

import "github.com/jpl-au/fluent/html5/a"

func f() {
	//flint:allow nesting demonstrating parser recovery in a test page
	_ = a.New(a.Static("Back"))
}`
	diags, err := l.Source("test.go", []byte(src))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	for _, d := range diags {
		if d.Check == "nesting" {
			t.Errorf("directive did not suppress: %s", d.Message)
		}
	}
}
