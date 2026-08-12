package flint

import (
	"strings"
	"testing"
)

func TestCheckDeprecatedPositive(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name    string
		src     string
		want    string
		wantFix string
	}{
		{
			name: "deprecated constructor",
			src: `package p

import "github.com/jpl-au/fluent/html5/embed"

func f() {
	_ = embed.Flash("movie.swf", 100, 100)
}`,
			want:    "embed.Flash is deprecated",
			wantFix: "Flash is no longer supported by browsers.",
		},
		{
			name: "deprecated element attribute method in an inline chain",
			src: `package p

import "github.com/jpl-au/fluent/html5/table"

func f() {
	_ = table.New().Border(1)
}`,
			want:    "method Border is deprecated",
			wantFix: "Use CSS border properties instead.",
		},
		{
			name: "deprecated event mixin method on a global element",
			src: `package p

import "github.com/jpl-au/fluent/html5/div"

func f() {
	_ = div.New().OnMouseWheel("handle(event)")
}`,
			want:    "method OnMouseWheel is deprecated",
			wantFix: "Not in the WHATWG living standard. Use OnWheel instead.",
		},
		{
			name: "deprecated enum option",
			src: `package p

import (
	"github.com/jpl-au/fluent/html5/input"
	"github.com/jpl-au/fluent/html5/attr/inputtype"
)

func f() {
	_ = input.New().Type(inputtype.Datetime)
}`,
			want:    "inputtype.Datetime is deprecated",
			wantFix: "Use DatetimeLocal instead which provides a native date and time picker.",
		},
		{
			name: "deprecated method on a local assigned from a fluent chain",
			src: `package p

import "github.com/jpl-au/fluent/html5/table"

func f() {
	t := table.New()
	t.Border(1)
}`,
			want:    "method Border is deprecated",
			wantFix: "Use CSS border properties instead.",
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
				if d.Check == "deprecated" && d.Message == tt.want {
					found = true
					if d.Severity != Warning {
						t.Errorf("severity = %v, want Warning", d.Severity)
					}
					if d.Fix != tt.wantFix {
						t.Errorf("fix = %q, want %q", d.Fix, tt.wantFix)
					}
					break
				}
			}
			if !found {
				t.Errorf("expected diagnostic %q", tt.want)
				for _, d := range diags {
					t.Logf("  %s: [%s] %s", d.Pos, d.Check, d.Message)
				}
			}
		})
	}
}

func TestCheckDeprecatedNegative(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name string
		src  string
	}{
		{
			name: "same method name on a package where it is not deprecated",
			src: `package p

import (
	"github.com/jpl-au/fluent/html5/input"
	"github.com/jpl-au/fluent/html5/attr/inputtype"
)

func f() {
	_ = input.New().Type(inputtype.Email)
}`,
		},
		{
			name: "non-deprecated constructor and methods",
			src: `package p

import "github.com/jpl-au/fluent/html5/div"

func f() {
	_ = div.New().Class("box").ID("main")
}`,
		},
		{
			name: "deprecated-looking method on a non-fluent receiver",
			src: `package p

import "example.com/tables"

func f() {
	_ = tables.New().Border(1)
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
				if d.Check == "deprecated" {
					t.Errorf("unexpected diagnostic: %s", d.Message)
				}
			}
		})
	}

	t.Run("no registry means no deprecation checks", func(t *testing.T) {
		noReg := New(nil)
		src := `package p

import "github.com/jpl-au/fluent/html5/embed"

func f() {
	_ = embed.Flash("movie.swf", 100, 100)
}`
		diags, err := noReg.Source("test.go", []byte(src))
		if err != nil {
			t.Fatalf("unexpected parse error: %v", err)
		}
		for _, d := range diags {
			if d.Check == "deprecated" {
				t.Errorf("unexpected diagnostic without registry: %s", d.Message)
			}
		}
	})
}

func TestInfoShowsDeprecation(t *testing.T) {
	r := FluentRegistry()

	var b strings.Builder
	if err := r.Info(&b, "embed", "constructors"); err != nil {
		t.Fatalf("Info: %v", err)
	}
	if !strings.Contains(b.String(), "deprecated: Flash is no longer supported by browsers.") {
		t.Errorf("embed constructors output missing deprecation note:\n%s", b.String())
	}

	b.Reset()
	if err := r.Info(&b, "table", "methods"); err != nil {
		t.Fatalf("Info: %v", err)
	}
	if !strings.Contains(b.String(), "deprecated: Use CSS border properties instead.") {
		t.Errorf("table methods output missing deprecation note:\n%s", b.String())
	}

	b.Reset()
	if err := r.Info(&b, "inputtype", "vars"); err != nil {
		t.Fatalf("Info: %v", err)
	}
	if !strings.Contains(b.String(), "Datetime  deprecated: Use DatetimeLocal instead") {
		t.Errorf("inputtype vars output missing deprecation note:\n%s", b.String())
	}
}
