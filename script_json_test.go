package flint

import "testing"

func TestScriptJSONConstructor(t *testing.T) {
	l := New(FluentRegistry())
	src := wrapWithImports(
		[]string{"github.com/jpl-au/fluent/html5/script"},
		`data := "{}"; _ = script.JSON(data).ID("page-data")`,
	)
	diags, err := l.Source("test.go", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics for script.JSON: %v", diags)
	}
}
