package flint

import (
	"strings"
	"testing"
)

func TestCheckArity(t *testing.T) {
	l := New(testRegistry())

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
		{
			name:    "node.Map with 2 args is valid",
			imports: []string{"github.com/jpl-au/fluent/node"},
			body:    `_ = node.Map([]int{1, 2}, func(int) node.Node { return nil })`,
		},
		{
			name:    "node.Map with 1 arg is flagged",
			imports: []string{"github.com/jpl-au/fluent/node"},
			body:    `_ = node.Map([]int{1, 2})`,
			want:    "node.Map() expects 2 argument(s), got 1",
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
					if d.Severity != Error {
						t.Errorf("severity = %v, want Error", d.Severity)
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

func TestCheckMethodArity(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name    string
		imports []string
		body    string
		want    string // empty means no diagnostic expected
	}{
		{
			name:    "Class with 1 arg is valid",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `_ = div.New().Class("container")`,
		},
		{
			name:    "Class with 2 args is flagged",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `_ = div.New().Class("container", "extra")`,
			want:    ".Class() expects 1 argument(s), got 2",
		},
		{
			name:    "Class with 0 args is flagged",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `_ = div.New().Class()`,
			want:    ".Class() expects 1 argument(s), got 0",
		},
		{
			name:    "mid-chain method is checked",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `_ = div.New().Class("a").ID()`,
			want:    ".ID() expects 1 argument(s), got 0",
		},
		{
			// Dynamic used to be variadic with a keyless "_" sentinel;
			// the key is now required (it is the element's identity).
			name:    "Dynamic with 0 args is flagged",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `_ = div.New().Dynamic()`,
			want:    ".Dynamic() expects 1 argument(s), got 0",
		},
		{
			name:    "Memoise with 1 arg is valid",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `_ = div.New().Dynamic("k").Memoise(3)`,
		},
		{
			name:    "variadic Add with many args is valid",
			imports: []string{"github.com/jpl-au/fluent/html5/div", "github.com/jpl-au/fluent/html5/span"},
			body:    `_ = div.New().Add(span.New(), span.New(), span.New())`,
		},
		{
			name:    "spread argument is not checked",
			imports: []string{"github.com/jpl-au/fluent/html5/div", "github.com/jpl-au/fluent/node"},
			body:    `kids := []node.Node{}; _ = div.New().Add(kids...)`,
		},
		{
			name:    "SetAttribute with 1 arg is flagged",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `div.New().SetAttribute("class")`,
			want:    ".SetAttribute() expects 2 argument(s), got 1",
		},
		{
			name:    "ConditionalBuilder True with 0 args is flagged",
			imports: []string{"github.com/jpl-au/fluent/node"},
			body:    `_ = node.Condition(true).True()`,
			want:    ".True() expects 1 argument(s), got 0",
		},
		{
			name:    "ConditionalBuilder Dynamic takes exactly one key",
			imports: []string{"github.com/jpl-au/fluent/node"},
			body:    `_ = node.Condition(true).True(nil).Dynamic()`,
			want:    ".Dynamic() expects 1 argument(s), got 0",
		},
		{
			name:    "BufferHint takes exactly one hint",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `_ = div.New().BufferHint()`,
			want:    ".BufferHint() expects 1 argument(s), got 0",
		},
		{
			name:    "chained BufferHint with one hint is valid",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `_ = div.New().BufferHint(4096).Class("doc")`,
		},
		{
			name:    "unresolvable local receiver is not checked",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `_ = div.New(); d := makeThing(); d.Class()`,
		},
		{
			name:    "non-element package without return data is not checked",
			imports: []string{"github.com/jpl-au/fluent-security"},
			body:    `_ = security.New().Clean()`,
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
					if d.Severity != Error {
						t.Errorf("severity = %v, want Error", d.Severity)
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
