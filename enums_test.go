package flint

import (
	"strings"
	"testing"
)

func TestCheckTypedParams(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name    string
		imports []string
		body    string
		want    string // empty means no diagnostic expected
	}{
		{
			name:    "string literal for Type is flagged",
			imports: []string{"github.com/jpl-au/fluent/html5/input"},
			body:    `_ = input.New().Type("email")`,
			want:    `.Type() expects a typed constant, not a string literal "email"`,
		},
		{
			name:    "string literal for Loading is flagged",
			imports: []string{"github.com/jpl-au/fluent/html5/img"},
			body:    `_ = img.New().Loading("lazy")`,
			want:    `.Loading() expects a typed constant, not a string literal "lazy"`,
		},
		{
			name:    "string literal for Dir is flagged",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `_ = div.New().Dir("rtl")`,
			want:    `.Dir() expects a typed constant, not a string literal "rtl"`,
		},
		{
			name:    "string literal for Method on form is flagged",
			imports: []string{"github.com/jpl-au/fluent/html5/form"},
			body:    `_ = form.New().Method("post")`,
			want:    `.Method() expects a typed constant, not a string literal "post"`,
		},
		{
			name:    "typed constant for Type is fine",
			imports: []string{"github.com/jpl-au/fluent/html5/input", "github.com/jpl-au/fluent/html5/attr/inputtype"},
			body:    `_ = input.New().Type(inputtype.Email)`,
		},
		{
			name:    "Custom() for Type is fine",
			imports: []string{"github.com/jpl-au/fluent/html5/input", "github.com/jpl-au/fluent/html5/attr/inputtype"},
			body:    `_ = input.New().Type(inputtype.Custom("future"))`,
		},
		{
			name:    "string for Class is fine (not a typed param)",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `_ = div.New().Class("container")`,
		},
		{
			name:    "string for Name is fine",
			imports: []string{"github.com/jpl-au/fluent/html5/input"},
			body:    `_ = input.New().Name("email")`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := wrapWithImports(tt.imports, tt.body)
			diags, err := l.Source("test.go", src)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}

			if tt.want == "" {
				for _, d := range diags {
					if strings.Contains(d.Message, "expects a typed constant") {
						t.Errorf("unexpected typed param diagnostic: %s", d.Message)
					}
				}
				return
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

func TestTypedParamFixNamesConstant(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name    string
		imports []string
		body    string
		wantFix string
	}{
		{
			name:    "known value names the exact constant",
			imports: []string{"github.com/jpl-au/fluent/html5/input"},
			body:    `_ = input.New().Type("email")`,
			wantFix: `Replace "email" with inputtype.Email`,
		},
		{
			name:    "case-insensitive value still names the constant",
			imports: []string{"github.com/jpl-au/fluent/html5/input"},
			body:    `_ = input.New().Type("EMAIL")`,
			wantFix: `Replace "EMAIL" with inputtype.Email`,
		},
		{
			name:    "unknown value keeps the generic fix",
			imports: []string{"github.com/jpl-au/fluent/html5/input"},
			body:    `_ = input.New().Type("bogus")`,
			wantFix: `Use a value from the inputtype package (e.g., inputtype.X) or inputtype.Custom(...)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags, err := l.Source("test.go", wrapWithImports(tt.imports, tt.body))
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			found := false
			for _, d := range diags {
				if d.Check == "typed-params" && strings.Contains(d.Message, "expects a typed constant") {
					found = true
					if d.Fix != tt.wantFix {
						t.Errorf("fix = %q, want %q", d.Fix, tt.wantFix)
					}
					break
				}
			}
			if !found {
				t.Errorf("expected a typed-params diagnostic")
			}
		})
	}
}

func TestCustomRecreatesConstant(t *testing.T) {
	l := New(FluentRegistry())

	t.Run("Custom with a predefined value is flagged", func(t *testing.T) {
		src := wrapWithImports(
			[]string{"github.com/jpl-au/fluent/html5/input", "github.com/jpl-au/fluent/html5/attr/inputtype"},
			`_ = input.New().Type(inputtype.Custom("email"))`,
		)
		diags, err := l.Source("test.go", src)
		if err != nil {
			t.Fatalf("unexpected parse error: %v", err)
		}
		want := `inputtype.Custom("email") re-creates the predefined constant inputtype.Email`
		found := false
		for _, d := range diags {
			if d.Message == want {
				found = true
				if d.Severity != Warning {
					t.Errorf("severity = %v, want Warning", d.Severity)
				}
				if wantFix := `Replace inputtype.Custom("email") with inputtype.Email`; d.Fix != wantFix {
					t.Errorf("fix = %q, want %q", d.Fix, wantFix)
				}
				break
			}
		}
		if !found {
			t.Errorf("expected diagnostic %q", want)
			for _, d := range diags {
				t.Logf("  %s: [%s] %s", d.Pos, d.Check, d.Message)
			}
		}
	})

	t.Run("Custom with a genuinely custom value is fine", func(t *testing.T) {
		src := wrapWithImports(
			[]string{"github.com/jpl-au/fluent/html5/input", "github.com/jpl-au/fluent/html5/attr/inputtype"},
			`_ = input.New().Type(inputtype.Custom("future"))`,
		)
		diags, err := l.Source("test.go", src)
		if err != nil {
			t.Fatalf("unexpected parse error: %v", err)
		}
		for _, d := range diags {
			if strings.Contains(d.Message, "re-creates") {
				t.Errorf("unexpected diagnostic: %s", d.Message)
			}
		}
	})
}
