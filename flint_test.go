package flint

import "testing"

func TestSeverityConstants(t *testing.T) {
	// Error must be the zero value. Check functions that produce
	// errors rely on the Diagnostic{} default severity being Error.
	var zero Severity
	if zero != Error {
		t.Fatalf("zero value of Severity = %v, want Error", zero)
	}

	if Error.String() != "error" {
		t.Errorf("Error.String() = %q, want %q", Error.String(), "error")
	}
	if Warning.String() != "warning" {
		t.Errorf("Warning.String() = %q, want %q", Warning.String(), "warning")
	}
}

func TestMixedSeverity(t *testing.T) {
	l := New(FluentRegistry())

	// This source triggers both errors and warnings in one pass:
	//   - div.New().Static(name) => warning (checkStatic: dynamic content)
	//   - node.Fragment()        => error   (checkSymbols: does not exist)
	src := wrapWithImports(
		[]string{
			"github.com/jpl-au/fluent/html5/div",
			"github.com/jpl-au/fluent/node",
		},
		`name := "x"; _ = div.New().Static(name); _ = node.Fragment()`,
	)

	diags, err := l.Source("test.go", src)
	if err != nil {
		t.Fatalf("Source() returned error: %v", err)
	}

	var errors, warnings int
	for _, d := range diags {
		switch d.Severity {
		case Error:
			errors++
		case Warning:
			warnings++
		}
	}

	if errors == 0 {
		t.Error("expected at least one error diagnostic")
	}
	if warnings == 0 {
		t.Error("expected at least one warning diagnostic")
	}
}
