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
