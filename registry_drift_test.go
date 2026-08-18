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
// exported API of the sibling fluent checkout. The symbol check treats the
// registry as authoritative: a name absent from Functions, Types and Vars is
// reported as an error, so a symbol the registry misses becomes a false
// positive.
//
// Both directions are asserted. Every exported package-level symbol in the
// source must be registered, and every registered symbol must exist in the
// source. The test skips when the sibling checkout is absent, so flint tests
// standalone. fluent-generator's gen-check runs it with the siblings present.
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
		checkArities(t, path, "function", pkg.Functions, src.funcs)

		// Methods describes the package's primary type, which throughout
		// fluent is Element; TypeMethods covers the types constructors return.
		checkArities(t, path, "Element method", pkg.Methods, src.methods["Element"])
		for _, typeName := range sortedNames(pkg.TypeMethods) {
			checkArities(t, path, typeName+" method", pkg.TypeMethods[typeName], src.methods[typeName])
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
			if _, ok := src.funcs[name]; !ok {
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
	funcs   map[string]int // functions (methods excluded), keyed to their arity
	types   map[string]bool
	vars    map[string]bool           // vars and consts; the symbol check reads both from Vars
	methods map[string]map[string]int // exported receiver type, then method, keyed to arity
}

// exportedSymbols parses every non-test Go file in dir and collects its
// exported package-level declarations. Parsing is purely syntactic - no type
// checking - which is all the symbol check itself relies on.
func exportedSymbols(dir string) (symbols, error) {
	s := symbols{funcs: map[string]int{}, types: map[string]bool{}, vars: map[string]bool{}, methods: map[string]map[string]int{}}

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
				if !d.Name.IsExported() {
					continue
				}
				if d.Recv == nil {
					s.funcs[d.Name.Name] = declArity(d.Type)
					continue
				}
				if recv := receiverType(d.Recv); recv != "" {
					if s.methods[recv] == nil {
						s.methods[recv] = map[string]int{}
					}
					s.methods[recv][d.Name.Name] = declArity(d.Type)
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

// declArity is the argument count a function or method declaration accepts,
// with -1 for variadic - the same convention the registry records.
func declArity(t *ast.FuncType) int {
	if t.Params == nil {
		return 0
	}
	count := 0
	for _, field := range t.Params.List {
		if _, variadic := field.Type.(*ast.Ellipsis); variadic {
			return -1
		}
		// A field with no names is one unnamed parameter; otherwise it
		// declares one parameter per name.
		count += max(len(field.Names), 1)
	}
	return count
}

// receiverType is the exported type a method is declared on, or "" when the
// receiver is unexported or not a plain (possibly pointer) type name.
func receiverType(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	expr := recv.List[0].Type
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	name, ok := expr.(*ast.Ident)
	if !ok || !name.IsExported() {
		return ""
	}
	return name.Name
}

// checkArities compares one registered method set against the source over the
// names they share. The method-arity check reads these values, so a wrong arity
// reports correct code as an error. Names the source does not declare are left
// to the presence assertions. Subject names what is compared, such as
// "function" or "Element method".
func checkArities(t *testing.T, path, subject string, registered, source map[string]int) {
	t.Helper()

	for _, name := range sortedNames(registered) {
		want, ok := source[name]
		if !ok {
			continue
		}
		if got := registered[name]; got != want {
			t.Errorf("%s: %s %s records arity %d but the source declares %d; method-arity will falsely report correct calls", path, subject, name, got, want)
		}
	}
}

// sortedNames returns a set's names in order, for stable failure output.
func sortedNames[V any](set map[string]V) []string {
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
