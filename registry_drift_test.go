package flint

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestRegistryMatchesFluentSource diffs the generated registry against the
// real exported API of the sibling fluent checkout. The symbol check treats
// the registry as authoritative - a name absent from Functions, Types and
// Vars is reported as an error - so any exported symbol the registry misses
// is a false positive waiting to happen (the svg.Element and node.OnUnsafeURL
// class from the hue-server field audit). Both directions are asserted: every
// exported package-level symbol in the source must be registered, and every
// registered symbol must exist in the source. The test skips when the sibling
// checkout is absent, so flint still tests standalone; fluent-generator's
// gen-check runs it with the siblings present.
func TestRegistryMatchesFluentSource(t *testing.T) {
	if _, err := os.Stat("../fluent/go.mod"); err != nil {
		t.Skipf("sibling fluent checkout not found: %v", err)
	}

	reg := FluentRegistry()
	paths := make([]string, 0, len(reg.Packages))
	for path := range reg.Packages {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	for _, path := range paths {
		pkg := reg.Packages[path]
		dir, ok := sourceDir(path)
		if !ok {
			t.Errorf("registered package %s has no source mapping; extend sourceDir", path)
			continue
		}
		if _, err := os.Stat(dir); err != nil {
			// fluent-security lives in its own sibling repo, which a partial
			// checkout may not have; skipping it only narrows coverage.
			t.Logf("skipping %s: %v", path, err)
			continue
		}

		src, err := exportedSymbols(dir)
		if err != nil {
			t.Fatalf("%s: %v", dir, err)
		}

		for _, name := range sortedNames(src.funcs) {
			if _, ok := pkg.Functions[name]; !ok {
				t.Errorf("%s: exported function %s missing from registry Functions; the symbol check will falsely report it as nonexistent", path, name)
			}
		}
		for _, name := range sortedNames(src.types) {
			if !pkg.Types[name] {
				t.Errorf("%s: exported type %s missing from registry Types; the symbol check will falsely report it as nonexistent", path, name)
			}
		}
		for _, name := range sortedNames(src.vars) {
			if !pkg.Vars[name] {
				t.Errorf("%s: exported var or const %s missing from registry Vars; the symbol check will falsely report it as nonexistent", path, name)
			}
		}

		for name := range pkg.Functions {
			if !src.funcs[name] {
				t.Errorf("%s: registry records function %s but the source does not export it; the entry is stale", path, name)
			}
		}
		for name := range pkg.Types {
			if !src.types[name] {
				t.Errorf("%s: registry records type %s but the source does not export it; the entry is stale", path, name)
			}
		}
		for name := range pkg.Vars {
			if !src.vars[name] {
				t.Errorf("%s: registry records var %s but the source does not export it; the entry is stale", path, name)
			}
		}
	}
}

// sourceDir maps a registered import path to its directory in the sibling
// checkout layout.
func sourceDir(importPath string) (string, bool) {
	const fluent = "github.com/jpl-au/fluent/"
	if rest, ok := strings.CutPrefix(importPath, fluent); ok {
		return filepath.Join("..", "fluent", filepath.FromSlash(rest)), true
	}
	if importPath == "github.com/jpl-au/fluent-security" {
		return filepath.Join("..", "fluent-security"), true
	}
	return "", false
}

// symbols holds the exported package-level names of one source package.
type symbols struct {
	funcs map[string]bool // functions (methods excluded)
	types map[string]bool
	vars  map[string]bool // vars and consts; the symbol check reads both from Vars
}

// exportedSymbols parses every non-test Go file in dir and collects its
// exported package-level declarations. Parsing is purely syntactic - no type
// checking - which is all the symbol check itself relies on.
func exportedSymbols(dir string) (symbols, error) {
	s := symbols{funcs: map[string]bool{}, types: map[string]bool{}, vars: map[string]bool{}}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return s, err
	}
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			return s, err
		}
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil && d.Name.IsExported() {
					s.funcs[d.Name.Name] = true
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch sp := spec.(type) {
					case *ast.TypeSpec:
						if sp.Name.IsExported() {
							s.types[sp.Name.Name] = true
						}
					case *ast.ValueSpec:
						for _, id := range sp.Names {
							if id.IsExported() {
								s.vars[id.Name] = true
							}
						}
					}
				}
			}
		}
	}
	return s, nil
}

// sortedNames returns a set's names in order, for stable failure output.
func sortedNames(set map[string]bool) []string {
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
