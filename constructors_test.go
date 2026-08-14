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
		{
			name:    "attributes identify a constructor no method names",
			imports: []string{"github.com/jpl-au/fluent/html5/link", "github.com/jpl-au/fluent/html5/attr/rel"},
			body:    `_ = link.New().Rel(rel.Stylesheet).Href("/app.css")`,
			want:    "use link.Stylesheet(...) directly instead of link.New().Rel(...).Href(...)",
		},
		{
			name:    "the pinned rel value picks between constructors that set the same attributes",
			imports: []string{"github.com/jpl-au/fluent/html5/link", "github.com/jpl-au/fluent/html5/attr/rel"},
			body:    `_ = link.New().Rel(rel.Icon).Href("/favicon.ico")`,
			want:    "use link.Icon(...) directly instead of link.New().Rel(...).Href(...)",
		},
		{
			name:    "a rel the linter cannot read leaves the chain alone",
			imports: []string{"github.com/jpl-au/fluent/html5/link", "github.com/jpl-au/fluent/html5/attr/rel"},
			body: `var r rel.Rel
	_ = link.New().Rel(r).Href("/app.css")`,
		},
		{
			name:    "text content counts too, so a.Link beats a.Text",
			imports: []string{"github.com/jpl-au/fluent/html5/a"},
			body:    `_ = a.New().Href("https://example.com").Text("Click here")`,
			want:    "use a.Link(...) directly instead of a.New().Href(...).Text(...)",
		},
		{
			name:    "a pinned scheme picks a.MailTo over a.Link",
			imports: []string{"github.com/jpl-au/fluent/html5/a"},
			body:    `_ = a.New().Href("mailto:jo@example.com").Text("Email Jo")`,
			want:    "use a.MailTo(...) directly instead of a.New().Href(...).Text(...)",
		},
		{
			name:    "the constructor that replaces more of the chain wins",
			imports: []string{"github.com/jpl-au/fluent/html5/img", "github.com/jpl-au/fluent/html5/attr/loading"},
			body:    `_ = img.New().Src("photo.jpg").Alt("A sunset").Loading(loading.Lazy)`,
			want:    "use img.Lazy(...) directly instead of img.New().Src(...).Alt(...).Loading(...)",
		},
		{
			name:    "the same chain without the loading attribute stops at img.Image",
			imports: []string{"github.com/jpl-au/fluent/html5/img"},
			body:    `_ = img.New().Src("photo.jpg").Alt("A sunset")`,
			want:    "use img.Image(...) directly instead of img.New().Src(...).Alt(...)",
		},
		{
			name:    "a bare boolean setter matches a constructor that pins it",
			imports: []string{"github.com/jpl-au/fluent/html5/dialog"},
			body:    `_ = dialog.New().Open()`,
			want:    "use dialog.Open(...) directly instead of dialog.New().Open(...)",
		},
		{
			name:    "children pass through to a constructor that still takes them",
			imports: []string{"github.com/jpl-au/fluent/html5/form", "github.com/jpl-au/fluent/html5/attr/method", "github.com/jpl-au/fluent/html5/input"},
			body:    `_ = form.New(input.Email("address")).Action("/save").Method(method.Post)`,
			want:    "use form.Post(...) directly instead of form.New(...).Action(...).Method(...)",
		},
		{
			name:    "children with nowhere to go leave the chain alone",
			imports: []string{"github.com/jpl-au/fluent/html5/a", "github.com/jpl-au/fluent/html5/span"},
			body:    `_ = a.New(span.Text("x")).Href("https://example.com").Text("Click here")`,
		},
		{
			name:    "mixed text content is left alone rather than reordered",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `_ = div.New().Text("a").RawText("<b>b</b>")`,
		},
		{
			name:    "a deprecated constructor is never suggested",
			imports: []string{"github.com/jpl-au/fluent/html5/embed"},
			body:    `_ = embed.New().Src("movie.swf").Type("application/x-shockwave-flash").Width(320).Height(240)`,
		},
		{
			name:    "a constructor that wraps its argument in another element is not a chain",
			imports: []string{"github.com/jpl-au/fluent/html5/details"},
			body:    `_ = details.New().Text("Disclosure")`,
			want:    "use details.Text(...) directly instead of details.New().Text(...)",
		},
		{
			name:    "a typed variadic can be left empty, so ol.Decimal still matches",
			imports: []string{"github.com/jpl-au/fluent/html5/ol", "github.com/jpl-au/fluent/html5/attr/listtype"},
			body:    `_ = ol.New().Type(listtype.Decimal)`,
			want:    "use ol.Decimal(...) directly instead of ol.New().Type(...)",
		},
		{
			name:    "children rule out a constructor whose variadic is typed",
			imports: []string{"github.com/jpl-au/fluent/html5/ol", "github.com/jpl-au/fluent/html5/li", "github.com/jpl-au/fluent/html5/attr/listtype"},
			body:    `_ = ol.New(li.Text("a")).Type(listtype.Decimal)`,
		},
		{
			name:    "what New fixes with no setter is not held against a constructor",
			imports: []string{"github.com/jpl-au/fluent/html5/html"},
			body:    `_ = html.New().Text("hello")`,
			want:    "use html.Text(...) directly instead of html.New().Text(...)",
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
					if d.Severity != Info {
						t.Errorf("severity = %v, want Info", d.Severity)
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

// TestConstructorAliasedEnum checks that a pinned value is compared through the
// file's imports, so an aliased enum package still identifies the constructor.
func TestConstructorAliasedEnum(t *testing.T) {
	src := []byte(`package example

import (
	"github.com/jpl-au/fluent/html5/link"
	rl "github.com/jpl-au/fluent/html5/attr/rel"
)

func build() {
	_ = link.New().Rel(rl.Prefetch).Href("/next")
}
`)

	diags, err := New(FluentRegistry()).Source("test.go", src)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	const want = "use link.Prefetch(...) directly instead of link.New().Rel(...).Href(...)"
	for _, d := range diags {
		if d.Message == want {
			return
		}
	}
	t.Errorf("expected diagnostic %q", want)
	for _, d := range diags {
		t.Logf("  %s: %s", d.Pos, d.Message)
	}
}
