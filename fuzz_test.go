package flint

import "testing"

// FuzzSource feeds arbitrary bytes to the linter and asserts it never panics.
// A linter must survive any input - malformed Go, partial ASTs, unusual tokens -
// because it runs over whatever a developer (or an AI) happens to write. Parse
// failures are returned as errors, not panics, so the invariant under test is
// simply "does not crash": a real defect would be a nil dereference or an
// index-out-of-range inside one of the AST-walking checks.
//
// Run the seed corpus as a normal regression:
//
//	go test -run FuzzSource ./...
//
// Or fuzz for real:
//
//	go test -fuzz=FuzzSource -fuzztime=30s ./...
func FuzzSource(f *testing.F) {
	seeds := []string{
		// Empty and minimal.
		"",
		"package p",
		"package p\nfunc f() {}",

		// Valid fluent that exercises several checks at once.
		"package p\nimport \"github.com/jpl-au/fluent/html5/div\"\nfunc f() { _ = div.New().Text(\"x\") }",
		"package p\nimport \"github.com/jpl-au/fluent/html5/input\"\nfunc f() { _ = input.New().Type(\"email\") }",
		"package p\nimport \"github.com/jpl-au/fluent/html5/div\"\nfunc f() { d := div.New(); d.SetAttribute(\"class\", \"x\") }",
		"package p\nimport (\n\t\"github.com/jpl-au/fluent/node\"\n\t\"github.com/jpl-au/fluent/html5/div\"\n)\nfunc f(items []int) {\n\trows := make([]node.Node, 0, len(items))\n\tfor range items {\n\t\trows = append(rows, div.New())\n\t}\n\t_ = div.New(rows...)\n}",
		"package p\nimport (\n\t\"github.com/jpl-au/fluent/html5/ul\"\n\t\"github.com/jpl-au/fluent/html5/li\"\n)\nfunc f() { _ = ul.New(li.Text(\"a\"), li.Text(\"b\")) }",

		// Malformed and adversarial.
		"package",
		"package p\nfunc (",
		"package p\nimport\nfunc f() { _ = div.New().Text( }",
		"package p\nvar x = []node.Node{",
		"\x00\x01\x02\n// not go at all",
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}

	// With and without a registry drive different code paths: symbol/typed
	// validation only runs when a registry is present.
	withReg := New(FluentRegistry())
	noReg := New(nil)

	f.Fuzz(func(t *testing.T, src []byte) {
		// The only assertion is that neither call panics. Diagnostics and parse
		// errors are both acceptable outcomes.
		_, _ = withReg.Source("fuzz.go", src)
		_, _ = noReg.Source("fuzz.go", src)
	})
}
