package flint

import (
	"strings"
	"testing"
)

func TestButtonSetterRegistry(t *testing.T) {
	pkg := FluentRegistry().Packages["github.com/jpl-au/fluent/html5/button"]
	if pkg.AttrMethods["type"] != "Type" {
		t.Fatal("Type must remain the canonical attribute setter")
	}
	for _, method := range []string{"Submit", "Reset", "Button"} {
		if arity, ok := pkg.Methods[method]; !ok || arity != 0 {
			t.Errorf("%s method arity = %d, present = %v", method, arity, ok)
		}
		if pkg.Functions[method] != 1 || pkg.SetterAliases[method] != "Type" {
			t.Errorf("%s constructor or alias registration is incorrect", method)
		}
		if pkg.Constructors[method].Pins["Type"] != `"`+strings.ToLower(method)+`"` {
			t.Errorf("%s constructor lost its Type mapping", method)
		}
	}
}

func TestButtonSetterChecks(t *testing.T) {
	l := New(FluentRegistry())
	tests := []struct {
		name, body, check, contains string
	}{
		{"submit", `_ = button.New().Submit()`, "", ""},
		{"reset", `_ = button.New().Reset()`, "", ""},
		{"button", `_ = button.New().Button()`, "", ""},
		{"constructor then setter", `_ = button.Reset("Save").Submit()`, "", ""},
		{"separate statements", `b := button.New().Submit(); b.Reset()`, "", ""},
		{"constructor arity", `_ = button.Submit()`, "arity", ""},
		{"method arity", `_ = button.New().Submit("Save")`, "method-arity", ""},
		{"mixed setters", `_ = button.New().Submit().Reset()`, "duplicate-attr", ".Reset() overwrites the value that .Submit()"},
		{"type then shortcut", `_ = button.New().Type("reset").Submit()`, "duplicate-attr", ".Submit() overwrites the value that .Type()"},
		{"shortcut then type", `_ = button.New().Button().Type("submit")`, "duplicate-attr", ".Type() overwrites the value that .Button()"},
		{"same shortcut", `_ = button.New().Reset().Reset()`, "duplicate-attr", ".Reset() overwrites"},
		{"raw attribute", `button.New().Submit().SetAttribute("type", "reset")`, "duplicate-attr", "repeats the .Submit()"},
		{"raw attribute on local", `b := button.New().Button(); b.SetAttributeRaw("type", "reset")`, "duplicate-attr", "after .Button()"},
		{"constructor raw attribute", `button.Reset("Save").SetAttribute("type", "submit")`, "duplicate-attr", "after button.Reset"},
		{"canonical suggestion", `button.New().SetAttribute("type", "reset")`, "setattr-key", "Type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := wrapWithImports([]string{"github.com/jpl-au/fluent/html5/button"}, tt.body)
			diags, err := l.Source("test.go", src)
			if err != nil {
				t.Fatal(err)
			}
			if tt.check == "" {
				if len(diags) != 0 {
					t.Fatalf("unexpected diagnostics: %v", diags)
				}
				return
			}
			for _, d := range diags {
				if d.Check == tt.check && strings.Contains(d.Message+" "+d.Fix, tt.contains) {
					return
				}
			}
			t.Fatalf("missing %s diagnostic containing %q: %v", tt.check, tt.contains, diags)
		})
	}
}
