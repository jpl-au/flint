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
		kind   string // "func", "method", or "var"
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
		switch c.kind {
		case "func":
			_, found = pkg.Functions[c.symbol]
		case "method":
			found = pkg.Methods[c.symbol]
		case "var":
			found = pkg.Vars[c.symbol]
		}

		if !found {
			t.Errorf("registry missing %s %s.%s", c.kind, c.path, c.symbol)
		}
	}
}

func TestFluentRegistryRejectsInventedSymbols(t *testing.T) {
	old := activeRegistry
	activeRegistry = FluentRegistry()
	defer func() { activeRegistry = old }()

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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := wrapWithImports(tt.imports, tt.body)
			diags, err := Source("test.go", src)
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
