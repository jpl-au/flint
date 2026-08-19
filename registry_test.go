package flint

import "testing"

func TestFluentRegistryLoads(t *testing.T) {
	reg := FluentRegistry()
	if reg == nil {
		t.Fatal("FluentRegistry() returned nil")
	}
	if len(reg.Packages) == 0 {
		t.Fatal("registry has no packages")
	}

	// Spot-check a few known entries.
	checks := []struct {
		path   string
		symbol string
		decl   string // "func", "method", or "var"
	}{
		{"github.com/jpl-au/fluent/html5/div", "New", "func"},
		{"github.com/jpl-au/fluent/html5/div", "Class", "method"},
		{"github.com/jpl-au/fluent/html5/div", "Static", "method"},
		{"github.com/jpl-au/fluent/html5/input", "Email", "func"},
		{"github.com/jpl-au/fluent/html5/input", "Required", "method"},
		{"github.com/jpl-au/fluent/html5/attr/inputtype", "Email", "var"},
		{"github.com/jpl-au/fluent/html5/attr/inputtype", "Custom", "func"},
		{"github.com/jpl-au/fluent/node", "Condition", "func"},
		{"github.com/jpl-au/fluent/node", "Func", "func"},
		{"github.com/jpl-au/fluent/text", "Static", "func"},
		{"github.com/jpl-au/fluent/text", "Textf", "func"},
	}

	for _, c := range checks {
		pkg, ok := reg.Packages[c.path]
		if !ok {
			t.Errorf("registry missing package %s", c.path)
			continue
		}

		var found bool
		switch c.decl {
		case "func":
			_, found = pkg.Functions[c.symbol]
		case "method":
			_, found = pkg.Methods[c.symbol]
		case "var":
			found = pkg.Vars[c.symbol]
		}

		if !found {
			t.Errorf("registry missing %s %s.%s", c.decl, c.path, c.symbol)
		}
	}
}

// TestFluentRegistryAcceptsCurrentAPI guards the false-positive direction. A
// missing registry entry turns compiling code into an error diagnostic, and the
// error tier is what fails a run, so a gap here is worse than a missed warning.
// Every symbol below was added or relocated recently, which is when the registry
// is most likely to have drifted behind the library.
func TestFluentRegistryAcceptsCurrentAPI(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name    string
		imports []string
		body    string
	}{
		{
			name:    "shared tag fragment",
			imports: []string{"github.com/jpl-au/fluent/html5"},
			body:    `_ = html5.TagDiv`,
		},
		{
			name:    "shared attribute fragment",
			imports: []string{"github.com/jpl-au/fluent/html5"},
			body:    `_ = html5.AttrID`,
		},
		{
			name:    "area.HrefLang",
			imports: []string{"github.com/jpl-au/fluent/html5/area"},
			body:    `_ = area.New().HrefLang("en-AU")`,
		},
		{
			name:    "img.Controls",
			imports: []string{"github.com/jpl-au/fluent/html5/img"},
			body:    `_ = img.New().Controls()`,
		},
		{
			name:    "template.ShadowRootOpen",
			imports: []string{"github.com/jpl-au/fluent/html5/template"},
			body:    `_ = template.ShadowRootOpen()`,
		},
		{
			name:    "shadowrootmode.Open",
			imports: []string{"github.com/jpl-au/fluent/html5/attr/shadowrootmode"},
			body:    `_ = shadowrootmode.Open`,
		},
		{
			name:    "svg.Element in a signature",
			imports: []string{"github.com/jpl-au/fluent/html5/svg"},
			body:    `var _ func() *svg.Element`,
		},
		{
			name:    "node URL filtering API",
			imports: []string{"github.com/jpl-au/fluent/node"},
			body:    `node.OnUnsafeURL(nil); _ = node.UnsafeURL; _ = node.FilterURL("/x")`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := wrapWithImports(tt.imports, tt.body)
			diags, err := l.Source("test.go", src)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			for _, d := range diags {
				if d.Severity == Error {
					t.Errorf("false positive on compiling code: %s: %s", d.Pos, d.Message)
				}
			}
		})
	}
}

func TestFluentRegistryRejectsInventedSymbols(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name    string
		imports []string
		body    string
		want    string
	}{
		{
			name:    "node.Fragment does not exist",
			imports: []string{"github.com/jpl-au/fluent/node"},
			body:    `_ = node.Fragment()`,
			want:    "node.Fragment does not exist",
		},
		{
			name:    "node.Group does not exist",
			imports: []string{"github.com/jpl-au/fluent/node"},
			body:    `_ = node.Group()`,
			want:    "node.Group does not exist",
		},
		{
			name:    "div.Email does not exist",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `_ = div.Email("x")`,
			want:    "div.Email does not exist",
		},
		{
			name:    "inputtype.Telephone does not exist",
			imports: []string{"github.com/jpl-au/fluent/html5/attr/inputtype"},
			body:    `_ = inputtype.Telephone`,
			want:    "inputtype.Telephone does not exist",
		},
		{
			// An element package exports no constants of its own. These were
			// emitted by the previous generator into a per-package constants.go
			// that nothing read and nothing regenerated; the render path has
			// always written the shared html5 fragments.
			name:    "div.AttrClass does not exist",
			imports: []string{"github.com/jpl-au/fluent/html5/div"},
			body:    `_ = div.AttrClass`,
			want:    "div.AttrClass does not exist",
		},
		{
			name:    "area.TagOpen does not exist",
			imports: []string{"github.com/jpl-au/fluent/html5/area"},
			body:    `_ = area.TagOpen`,
			want:    "area.TagOpen does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := wrapWithImports(tt.imports, tt.body)
			diags, err := l.Source("test.go", src)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
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
