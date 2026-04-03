package flint

import (
	"strings"
	"testing"
)

func TestCheckArity(t *testing.T) {
	old := activeRegistry
	activeRegistry = testRegistry()
	defer func() { activeRegistry = old }()

	tests := []struct {
		name    string
		imports []string
		body    string
		want    string // empty means no diagnostic expected
	}{
		{
			name:    "input.Email with 1 arg is valid",
			imports: []string{"github.com/jpl-au/fluent/html5/input"},
			body:    `_ = input.Email("email")`,
		},
		{
			name:    "input.Email with 2 args is flagged",
			imports: []string{"github.com/jpl-au/fluent/html5/input"},
			body:    `_ = input.Email("email", "you@example.com")`,
			want:    "input.Email() expects 1 argument(s), got 2",
		},
		{
			name:    "input.Text with 2 args is valid",
			imports: []string{"github.com/jpl-au/fluent/html5/input"},
			body:    `_ = input.Text("name", "value")`,
		},
		{
			name:    "input.Checkbox with 1 arg is flagged",
			imports: []string{"github.com/jpl-au/fluent/html5/input"},
			body:    `_ = input.Checkbox("agree")`,
			want:    "input.Checkbox() expects 2 argument(s), got 1",
		},
		{
			name:    "div.New with 0 args is valid (variadic)",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `_ = div.New()`,
		},
		{
			name:    "div.New with 3 args is valid (variadic)",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `_ = div.New(nil, nil, nil)`,
		},
		{
			name:    "input.New with 0 args is valid",
			imports: []string{"github.com/jpl-au/fluent/html5/input"},
			body:    `_ = input.New()`,
		},
		{
			name:    "input.New with 1 arg is flagged",
			imports: []string{"github.com/jpl-au/fluent/html5/input"},
			body:    `_ = input.New("name")`,
			want:    "input.New() expects 0 argument(s), got 1",
		},
		{
			name:    "non-registry package is ignored",
			imports: []string{"fmt"},
			body:    `_ = fmt.Sprintf("hello", 1, 2, 3)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := wrapWithImports(tt.imports, tt.body)
			diags, err := Source("test.go", src)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}

			if tt.want == "" {
				for _, d := range diags {
					if strings.Contains(d.Message, "expects") && strings.Contains(d.Message, "argument") {
						t.Errorf("unexpected arity diagnostic: %s", d.Message)
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
