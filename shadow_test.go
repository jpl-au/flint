package flint

import (
	"strings"
	"testing"
)

func TestCheckShadowsPositive(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "short declaration shadows element package",
			src: `package p

import "github.com/jpl-au/fluent/html5/input"

func f() {
	input := "email"
	_ = input
}`,
			want: `local variable "input" shadows the fluent package imported as "input"`,
		},
		{
			name: "var declaration shadows element package",
			src: `package p

import "github.com/jpl-au/fluent/html5/form"

func f() {
	var form string
	_ = form
}`,
			want: `local variable "form" shadows the fluent package imported as "form"`,
		},
		{
			name: "parameter shadows element package",
			src: `package p

import "github.com/jpl-au/fluent/html5/div"

func f(div string) {
	_ = div
}`,
			want: `parameter "div" shadows the fluent package imported as "div"`,
		},
		{
			name: "range variable shadows element package",
			src: `package p

import "github.com/jpl-au/fluent/html5/option"

func f(options []string) {
	for _, option := range options {
		_ = option
	}
}`,
			want: `range variable "option" shadows the fluent package imported as "option"`,
		},
		{
			name: "type switch binding shadows element package",
			src: `package p

import "github.com/jpl-au/fluent/html5/label"

func f(v interface{}) {
	switch label := v.(type) {
	default:
		_ = label
	}
}`,
			want: `local variable "label" shadows the fluent package imported as "label"`,
		},
		{
			name: "aliased import is shadowed by its alias",
			src: `package p

import d "github.com/jpl-au/fluent/html5/div"

func f() {
	d := 1
	_ = d
}`,
			want: `local variable "d" shadows the fluent package imported as "d"`,
		},
		{
			name: "shadow inside a function literal",
			src: `package p

import "github.com/jpl-au/fluent/html5/input"

var f = func(input string) {
	_ = input
}`,
			want: `parameter "input" shadows the fluent package imported as "input"`,
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

func TestCheckShadowsDedup(t *testing.T) {
	l := New(FluentRegistry())

	src := `package p

import "github.com/jpl-au/fluent/html5/input"

func f(ok bool) {
	if ok {
		input := 1
		_ = input
	} else {
		input := 2
		_ = input
	}
}`
	diags, err := l.Source("test.go", []byte(src))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	count := 0
	for _, d := range diags {
		if strings.Contains(d.Message, "shadows the fluent package") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("got %d shadow diagnostics, want 1 per name per function", count)
	}
}

func TestCheckShadowsNegative(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name string
		src  string
	}{
		{
			name: "non-fluent import is not flint's concern",
			src: `package p

import "example.com/xmllib"

func f() {
	xmllib := 1
	_ = xmllib
}`,
		},
		{
			name: "unrelated local names are fine",
			src: `package p

import "github.com/jpl-au/fluent/html5/div"

func f() {
	_ = div.New()
	count := 1
	_ = count
}`,
		},
		{
			name: "blank import cannot be shadowed",
			src: `package p

import _ "github.com/jpl-au/fluent/html5/div"

func f() {
	_ = 1
}`,
		},
		{
			name: "plain reassignment is a use, not a shadow",
			src: `package p

import "github.com/jpl-au/fluent/html5/div"

func f(s []string) {
	for i := range s {
		s[i] = "x"
	}
	_ = div.New()
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
				if strings.Contains(d.Message, "shadows the fluent package") {
					t.Errorf("unexpected diagnostic: %s", d.Message)
				}
			}
		})
	}

	t.Run("no registry means no shadow checks", func(t *testing.T) {
		noReg := New(nil)
		src := `package p

import "github.com/jpl-au/fluent/html5/input"

func f() {
	input := 1
	_ = input
}`
		diags, err := noReg.Source("test.go", []byte(src))
		if err != nil {
			t.Fatalf("unexpected parse error: %v", err)
		}
		for _, d := range diags {
			if strings.Contains(d.Message, "shadows the fluent package") {
				t.Errorf("unexpected diagnostic without registry: %s", d.Message)
			}
		}
	})
}
