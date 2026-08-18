package flint

import (
	"strings"
	"testing"
)

func TestCheckDuplicateAttrsPositive(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name    string
		src     string
		want    string
		wantFix string
	}{
		{
			name: "repeated overwriting method in one chain",
			src: `package p

import "github.com/jpl-au/fluent/html5/div"

func f() {
	_ = div.New().ID("a").ID("b")
}`,
			want:    `.ID() overwrites the value that .ID() set on line 6. Only the last value renders.`,
			wantFix: `Keep a single .ID() call with the value you want rendered`,
		},
		{
			name: "repeated method on a local assigned from a fluent chain",
			src: `package p

import "github.com/jpl-au/fluent/html5/input"

func f() {
	i := input.New()
	_ = i.Name("a").Name("b")
}`,
			want:    `.Name() overwrites the value that .Name() set on line 7. Only the last value renders.`,
			wantFix: `Keep a single .Name() call with the value you want rendered`,
		},
		{
			name: "SetAttribute duplicating a dedicated method",
			src: `package p

import "github.com/jpl-au/fluent/html5/div"

func f() {
	div.New().Class("box").SetAttribute("class", "extra")
}`,
			want:    `SetAttribute("class", ...) repeats the .Class() call earlier in this chain. Both render, so the class attribute appears twice.`,
			wantFix: `Move the value into the .Class() call. A browser keeps the first copy of a duplicated attribute.`,
		},
		{
			name: "SetAttribute duplicating what the chain's constructor set",
			src: `package p

import "github.com/jpl-au/fluent/html5/input"

func f() {
	input.Text("e", "").SetAttribute("type", "email")
}`,
			want:    `SetAttribute("type", ...) sets type again after input.Text. Both render, and the browser uses the first, so this value has no effect.`,
			wantFix: `A browser keeps the first copy of a duplicated attribute. Use a constructor that sets the type you want, or set it once with .Type().`,
		},
		{
			name: "split statements: SetAttribute duplicating the local's constructor",
			src: `package p

import "github.com/jpl-au/fluent/html5/input"

func f(inputType string) {
	inp := input.Text("e", "")
	inp.SetAttribute("type", inputType)
	_ = inp
}`,
			want:    `SetAttribute("type", ...) sets type again after input.Text. Both render, and the browser uses the first, so this value has no effect.`,
			wantFix: `A browser keeps the first copy of a duplicated attribute. Use a constructor that sets the type you want, or set it once with .Type().`,
		},
		{
			name: "split statements: SetAttribute duplicating a method in the local's chain",
			src: `package p

import "github.com/jpl-au/fluent/html5/input"

func f() {
	box := input.New().Name("q").ID("q")
	box.SetAttribute("id", "search")
	_ = box
}`,
			want:    `SetAttribute("id", ...) sets id again after .ID() on line 6. Both render, and the browser uses the first, so this value has no effect.`,
			wantFix: `Move the value into the .ID() call. A browser keeps the first copy of a duplicated attribute.`,
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
				if d.Check == "duplicate-attr" && d.Message == tt.want {
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

func TestCheckDuplicateAttrsNegative(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name string
		src  string
	}{
		{
			name: "accumulating methods repeat legitimately",
			src: `package p

import "github.com/jpl-au/fluent/html5/div"

func f() {
	_ = div.New().Class("a").Class("b").Style("x: 1").Style("y: 2")
}`,
		},
		{
			name: "distinct attribute methods are fine",
			src: `package p

import "github.com/jpl-au/fluent/html5/div"

func f() {
	_ = div.New().ID("a").Title("t").Lang("en")
}`,
		},
		{
			name: "SetAttribute on an attribute the constructor does not set",
			src: `package p

import "github.com/jpl-au/fluent/html5/input"

func f() {
	inp := input.Text("e", "")
	inp.SetAttribute("placeholder", "Email")
	_ = inp
}`,
		},
		{
			name: "content constructors set no attributes",
			src: `package p

import "github.com/jpl-au/fluent/html5/div"

func f() {
	d := div.Text("hello")
	d.SetAttribute("lang", "en")
	_ = d
}`,
		},
		{
			name: "same method across separate statements is a deliberate override",
			src: `package p

import "github.com/jpl-au/fluent/html5/div"

func f(alt bool) {
	d := div.New().ID("a")
	if alt {
		d = d.ID("b")
	}
	_ = d
}`,
		},
		{
			name: "same method on separate elements",
			src: `package p

import (
	"github.com/jpl-au/fluent/html5/div"
	"github.com/jpl-au/fluent/html5/span"
)

func f() {
	_ = div.New().ID("a").Add(span.New().ID("a"))
}`,
		},
		{
			name: "non-fluent chains are not flint's concern",
			src: `package p

import "example.com/tables"

func f() {
	_ = tables.New().ID("a").ID("b")
}`,
		},
		{
			name: "SetAttribute without a dedicated-method conflict",
			src: `package p

import "github.com/jpl-au/fluent/html5/div"

func f() {
	div.New().ID("a").SetAttribute("data-x", "1")
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
				if d.Check == "duplicate-attr" {
					t.Errorf("unexpected diagnostic: %s", d.Message)
				}
			}
		})
	}

	t.Run("no registry means no duplicate checks", func(t *testing.T) {
		noReg := New(nil)
		src := `package p

import "github.com/jpl-au/fluent/html5/div"

func f() {
	_ = div.New().ID("a").ID("b")
}`
		diags, err := noReg.Source("test.go", []byte(src))
		if err != nil {
			t.Fatalf("unexpected parse error: %v", err)
		}
		for _, d := range diags {
			if strings.Contains(d.Message, "overwrites the value") {
				t.Errorf("unexpected diagnostic without registry: %s", d.Message)
			}
		}
	})
}
