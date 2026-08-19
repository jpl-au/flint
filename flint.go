// Package flint provides AST-based validation of Go source code that
// uses the fluent HTML framework. It catches common misuse patterns
// that defeat JIT optimisation or introduce security vulnerabilities.
//
// The linter operates on source code strings using go/parser and
// go/ast. It has no dependency on fluent itself.
//
// The generated registry (see FluentRegistry) also serves as a
// queryable catalogue of every element's constructors, methods, typed
// parameters, attribute mappings, typed constructors, deprecation
// notes, enum values, and URL-filtered parameters. The CLI exposes
// this via the -info flag.
package flint

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
)

// Severity classifies the importance of a diagnostic.
type Severity int

const (
	// Unset is the zero value. No check produces it. A diagnostic carrying
	// it omitted the Severity field, which TestSeverity rejects.
	Unset Severity = iota

	// Error indicates code that is incorrect: a missing symbol,
	// wrong arity, or a chain that will not compile.
	Error

	// Warning indicates code that compiles and runs but carries a real
	// reason to change: a security or correctness hazard, a silent bug,
	// a duplicate attribute, or a typed API sidestepped.
	Warning

	// Info is advisory: the code is correct and fine as written, and an
	// optional, idiomatic alternative exists. It never fails the run.
	Info
)

// String returns the lowercase name of the severity level.
func (s Severity) String() string {
	switch s {
	case Error:
		return "error"
	case Warning:
		return "warning"
	case Info:
		return "info"
	default:
		return "unset"
	}
}

// Diagnostic reports a single problem found in the source code.
type Diagnostic struct {
	// Check is the short, stable, lowercase name of the check that produced
	// this diagnostic, for example "static" or "setattr-key". It identifies a
	// diagnostic's origin for telemetry and reporting, independent of the
	// message wording.
	Check    string
	Pos      token.Position
	End      token.Position
	Severity Severity
	Message  string
	Fix      string
}

// Linter validates Go source code that uses the fluent HTML framework.
// Create one with New and reuse it across files.
type Linter struct {
	registry     *Registry
	attrMethods  map[string]string
	attrByMethod map[string]string
	tagAliases   map[string]string
	enumValues   map[string]map[string]string
}

// New creates a Linter with the given registry. Pass FluentRegistry()
// for full validation, or nil to run only Static and RawText checks.
func New(r *Registry) *Linter {
	l := &Linter{registry: r}
	if r != nil {
		l.attrMethods = mergeAttrMethods(r)
		l.attrByMethod = invertAttrMethods(l.attrMethods)
		l.tagAliases = r.TagAliases()
		l.enumValues = enumValuesByPackage(r)
	}
	return l
}

// enumValuesByPackage indexes each enum package's rendered-value-to-constant
// map by package name, e.g. "inputtype" -> {"email": "Email", ...}, so a
// TypedParams entry (which records only the package name) can resolve a raw
// string to the exact constant.
func enumValuesByPackage(r *Registry) map[string]map[string]string {
	byPkg := make(map[string]map[string]string)
	for path, pkg := range r.Packages {
		if len(pkg.EnumValues) > 0 {
			byPkg[lastSegment(path)] = pkg.EnumValues
		}
	}
	return byPkg
}

// Source analyses Go source code and returns all diagnostics found.
// The filename is used only for position information in diagnostics.
//
// An error is returned only if the source cannot be parsed. Lint
// diagnostics are returned in the slice, not as errors.
func (l *Linter) Source(filename string, src []byte) ([]Diagnostic, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.AllErrors|parser.ParseComments)
	if err != nil {
		return nil, err
	}

	// Generated files are skipped by the standard marker convention: a
	// "// Code generated ... DO NOT EDIT." comment line before the package
	// clause. A file that merely mentions the marker text (in a string
	// literal, say) is still linted.
	if ast.IsGenerated(file) {
		return nil, nil
	}

	var diags []Diagnostic
	diags = append(diags, l.checkStatic(fset, file)...)
	diags = append(diags, l.checkRawText(fset, file)...)
	diags = append(diags, l.checkImports(fset, file)...)
	diags = append(diags, l.checkSetAttrChain(fset, file)...)
	diags = append(diags, l.checkSetAttrKey(fset, file)...)
	diags = append(diags, l.checkDuplicateAttrs(fset, file)...)
	diags = append(diags, l.checkURLScheme(fset, file)...)
	diags = append(diags, l.checkTypedParams(fset, file)...)
	diags = append(diags, l.checkConstructors(fset, file)...)
	diags = append(diags, l.checkTypedConstructors(fset, file)...)
	diags = append(diags, l.checkSymbols(fset, file)...)
	diags = append(diags, l.checkDeprecated(fset, file)...)
	diags = append(diags, l.checkArity(fset, file)...)
	diags = append(diags, l.checkMethodArity(fset, file)...)
	diags = append(diags, l.checkNodeAppend(fset, file)...)
	diags = append(diags, l.checkBufferHint(fset, file)...)
	diags = append(diags, l.checkShadows(fset, file)...)
	diags = append(diags, l.checkNesting(fset, file)...)

	diags = applyAllowDirectives(fset, file, src, diags)

	sort.Slice(diags, func(i, j int) bool {
		if diags[i].Pos.Line != diags[j].Pos.Line {
			return diags[i].Pos.Line < diags[j].Pos.Line
		}
		return diags[i].Pos.Column < diags[j].Pos.Column
	})

	return diags, nil
}
