package flint

import "testing"

func TestConvenienceConstructors(t *testing.T) {
	l := New(FluentRegistry())
	tests := []struct {
		name, pkg, body string
	}{
		{"InlineModule", "script", `code := "export default 1"; _ = script.InlineModule(code).Nonce("nonce")`},
		{"ImportMap", "script", `data := "{}"; _ = script.ImportMap(data).ID("imports")`},
		{"SpeculationRules", "script", `data := "{}"; _ = script.SpeculationRules(data).ID("rules")`},
		{"JSONLD", "script", `data := "{}"; _ = script.JSONLD(data).ID("metadata")`},
		{"Multipart empty", "form", `_ = form.Multipart("/upload")`},
		{"Multipart children", "form", `_ = form.Multipart("/upload", nil, nil).ID("upload")`},
		{"ModulePreload", "link", `_ = link.ModulePreload("/app.js").ID("module")`},
		{"Preconnect", "link", `_ = link.Preconnect("https://cdn.example.com").ID("cdn")`},
		{"Manifest", "link", `_ = link.Manifest("/app.webmanifest").ID("manifest")`},
		{"Meta Name", "meta", `_ = meta.Name("application-name", "My App").ID("app-name")`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := wrapWithImports([]string{"github.com/jpl-au/fluent/html5/" + tt.pkg}, tt.body)
			diags, err := l.Source("test.go", src)
			if err != nil {
				t.Fatal(err)
			}
			if len(diags) != 0 {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
		})
	}
}
