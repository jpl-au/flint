package flint

import (
	"strings"
	"testing"
)

func TestCheckConstructorUsage(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name    string
		imports []string
		body    string
		want    string // empty means no diagnostic expected
	}{
		{
			name:    "New().Text should use Text constructor",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `_ = div.New().Text("hello")`,
			want:    "use div.Text(...) directly instead of div.New().Text(...)",
		},
		{
			name:    "New().Static should use Static constructor",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `_ = div.New().Static("footer")`,
			want:    "use div.Static(...) directly instead of div.New().Static(...)",
		},
		{
			name:    "New().RawText should use RawText constructor",
			imports: []string{"github.com/jpl-au/fluent/html5/span"},
			body:    `_ = span.New().RawText("<b>bold</b>")`,
			want:    "use span.RawText(...) directly instead of span.New().RawText(...)",
		},
		{
			name:    "New().Textf should use Textf constructor",
			imports: []string{"github.com/jpl-au/fluent/html5/p"},
			body:    `_ = p.New().Textf("hello %s", "world")`,
			want:    "use p.Textf(...) directly instead of p.New().Textf(...)",
		},
		{
			name:    "direct Text constructor is fine",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `_ = div.Text("hello")`,
		},
		{
			name:    "direct Static constructor is fine",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `_ = div.Static("footer")`,
		},
		{
			name:    "New() with children then Text is fine (different pattern)",
			imports: []string{"github.com/jpl-au/fluent/html5/div", "github.com/jpl-au/fluent/html5/span"},
			body:    `_ = div.New(span.New()).Text("hello")`,
		},
		{
			name:    "New().Class is fine (Class is a method not a constructor)",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `_ = div.New().Class("container")`,
		},
		{
			name:    "New().Class().Text is flagged (Text constructor can replace New)",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `_ = div.New().Class("x").Text("hello")`,
			want:    "use div.Text(...) directly instead of div.New()...Text(...)",
		},
		{
			name:    "h3.New().Class().Text is flagged through a longer chain",
			imports: []string{"github.com/jpl-au/fluent/html5/h3"},
			body:    `_ = h3.New().Class("demo-title").Text("Error Boundary")`,
			want:    "use h3.Text(...) directly instead of h3.New()...Text(...)",
		},
		{
			name:    "input.New().Email not flagged (Email takes different args)",
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
					if strings.Contains(d.Message, "directly instead of") {
						t.Errorf("unexpected constructor diagnostic: %s", d.Message)
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
