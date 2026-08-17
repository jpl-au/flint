// Command flint validates Go source code that uses the fluent HTML framework.
//
// Usage:
//
//	flint [flags] <pattern>...
//	flint [flags] -
//	flint -info <element> [section]...
//
// Patterns follow Go conventions: ./... checks all Go files recursively,
// ./pkg checks a specific directory, or individual .go files can be named
// directly. When given "-" as the sole argument, it reads from stdin.
//
// The -info flag displays the registry entry for a named element,
// including its types, constructors, typed constructors, methods,
// attribute mappings, vars, and (for a multi-element package such as
// svg) its elements. No linting is performed. An element within a
// multi-element package resolves by bare name, or by the
// package:element form when the bare name is shadowed by a package
// (svg:text, versus the text node package).
//
//	flint -info div
//	flint -info input
//	flint -info svg
//	flint -info rect
//	flint -info svg:text
//
// Pass one or more section names after the element to restrict the
// output. Each section accepts a long form and (where useful) a short
// form: types, constructors/ctors, typed-constructors/typed, methods,
// attributes/attrs, vars, elements.
//
//	flint -info div methods
//	flint -info input ctors attrs
//	flint -info ol typed
//
// Flags:
//
//	-no-registry        Disable registry-backed validation (literal and SetAttribute-chain checks still run)
//	-include-tests      Include _test.go files in the analysis
//	-json               Emit diagnostics as JSON, one object per line, and no summary
//	-info <element>     Show registry info for an element and exit
//	-telemetry <value>  Set telemetry mode (off|local|on) or show it (status), then exit
//	-version            Print flint version and exit
//
// Telemetry is opt-in and off by default. When set to local or on, an ordinary
// lint run records its diagnostics and attribute usage to a local .tlf file; the
// chosen mode persists in the user config directory. The on mode reserves the
// "collect and upload" meaning and behaves as local until upload exists.
//
//	flint -telemetry local     Enable local collection
//	flint -telemetry off       Disable collection
//	flint -telemetry status    Print the current mode
//
// Exit codes:
//
//	0  No errors found (warnings may be present)
//	1  One or more errors found
//	2  Usage or I/O error (including unknown element for -info or unknown -telemetry mode)
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/jpl-au/flint"
	"github.com/jpl-au/flint/telemetry"
)

func main() {
	noRegistry := flag.Bool("no-registry", false, "Disable symbol validation")
	includeTests := flag.Bool("include-tests", false, "Include _test.go files")
	jsonOut := flag.Bool("json", false, "Emit diagnostics as JSON, one object per line")
	infoElement := flag.String("info", "", "Show registry info for an element (e.g. -info div, or -info svg:rect for a shape within a package)")
	telemetryMode := flag.String("telemetry", "", "Set telemetry mode (off|local|on) or show it (status)")
	showVersion := flag.Bool("version", false, "Print flint version and exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: flint [flags] <pattern>...\n")
		fmt.Fprintf(os.Stderr, "       flint [flags] -                          (read from stdin)\n")
		fmt.Fprintf(os.Stderr, "       flint -info <element> [section]...       (show element info)\n\n")
		fmt.Fprintf(os.Stderr, "Patterns:\n")
		fmt.Fprintf(os.Stderr, "  ./...      Check all .go files recursively\n")
		fmt.Fprintf(os.Stderr, "  ./pkg      Check all .go files in a directory\n")
		fmt.Fprintf(os.Stderr, "  file.go    Check a specific file\n\n")
		fmt.Fprintf(os.Stderr, "Info sections (each accepts a long form and, where useful, a short form):\n")
		fmt.Fprintf(os.Stderr, "  types\n")
		fmt.Fprintf(os.Stderr, "  constructors, ctors\n")
		fmt.Fprintf(os.Stderr, "  typed-constructors, typed\n")
		fmt.Fprintf(os.Stderr, "  methods\n")
		fmt.Fprintf(os.Stderr, "  attributes, attrs\n")
		fmt.Fprintf(os.Stderr, "  vars\n")
		fmt.Fprintf(os.Stderr, "  elements\n\n")
		fmt.Fprintf(os.Stderr, "Telemetry (opt-in, off by default):\n")
		fmt.Fprintf(os.Stderr, "  -telemetry local     Record diagnostics and attribute usage to local .tlf files\n")
		fmt.Fprintf(os.Stderr, "  -telemetry off       Disable collection\n")
		fmt.Fprintf(os.Stderr, "  -telemetry status    Print the current mode\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showVersion {
		fmt.Println(version())
		return
	}

	if *telemetryMode != "" {
		dir, err := telemetry.ConfigDir()
		if err == nil {
			err = telemetryCommand(os.Stdout, dir, *telemetryMode)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "flint: %v\n", err)
			os.Exit(2)
		}
		return
	}

	if *infoElement != "" {
		reg := flint.FluentRegistry()
		if err := reg.Info(os.Stdout, *infoElement, flag.Args()...); err != nil {
			fmt.Fprintf(os.Stderr, "flint: %v\n", err)
			os.Exit(2)
		}
		return
	}

	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}

	var l *flint.Linter
	if *noRegistry {
		l = flint.New(nil)
	} else {
		l = flint.New(flint.FluentRegistry())
	}

	var p printer = newTextPrinter(os.Stdout, os.Stderr)
	if *jsonOut {
		p = jsonPrinter{enc: json.NewEncoder(os.Stdout)}
	}

	args := flag.Args()

	// A nil sink when telemetry is off, so recording is a no-op and no run is
	// opened. Closed once after the loop, before any os.Exit, so a single .tlf
	// covers the whole run (os.Exit does not run deferred calls).
	sink := openTelemetry(flintVersion())

	var errors, warnings int
	var hadErrors bool
	var stdinUsed bool

	for _, arg := range args {
		if arg == "-" {
			if stdinUsed {
				fmt.Fprintf(os.Stderr, "flint: stdin already read\n")
				hadErrors = true
				continue
			}
			stdinUsed = true
			e, w, err := checkStdin(l, sink, p)
			if err != nil {
				fmt.Fprintf(os.Stderr, "flint: %v\n", err)
				hadErrors = true
				continue
			}
			errors += e
			warnings += w
			continue
		}

		files, err := resolvePattern(arg, *includeTests)
		if err != nil {
			fmt.Fprintf(os.Stderr, "flint: %v\n", err)
			hadErrors = true
			continue
		}
		for _, path := range files {
			e, w, err := checkFile(l, sink, p, path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "flint: %v\n", err)
				hadErrors = true
				continue
			}
			errors += e
			warnings += w
		}
	}

	// Flush telemetry before any os.Exit below, which would skip a deferred close.
	sink.close()

	if hadErrors {
		os.Exit(2)
	}
	p.summary(errors, warnings)
	if errors > 0 {
		os.Exit(1)
	}
}

// printer writes diagnostics in one output format. The check loop hands it
// every diagnostic as it is found, and the run totals at the end.
type printer interface {
	diagnostic(d flint.Diagnostic)
	summary(errors, warnings int)
}

// textPrinter writes the human-readable format: one line per diagnostic with
// an indented fix line on out, and a run summary on err.
type textPrinter struct {
	out, err io.Writer

	// seenFixes records the fix paragraphs already written, so each one is
	// explained once per run. The fix text is the same for every site of the
	// same shape, and the advisory checks fire often: node-append alone
	// accounted for a whole run's output on porter, two thirds of it the same
	// paragraph repeated. Every diagnostic still prints; only the repeated
	// explanation is dropped.
	seenFixes map[string]bool
}

// newTextPrinter returns a text printer writing diagnostics to out and the run
// summary to err.
func newTextPrinter(out, err io.Writer) textPrinter {
	return textPrinter{out: out, err: err, seenFixes: map[string]bool{}}
}

func (p textPrinter) diagnostic(d flint.Diagnostic) {
	// The check name rides along in brackets so a diagnostic can be cited
	// (and suppressed with //flint:allow) without reaching for -json.
	fmt.Fprintf(p.out, "%s:%d:%d: %s[%s]: %s\n", d.Pos.Filename, d.Pos.Line, d.Pos.Column, d.Severity, d.Check, d.Message)
	if d.Fix != "" && !p.seenFixes[d.Fix] {
		p.seenFixes[d.Fix] = true
		fmt.Fprintf(p.out, "  fix: %s\n", d.Fix)
	}
}

func (p textPrinter) summary(errors, warnings int) {
	if errors+warnings == 0 {
		return
	}
	var parts []string
	if errors > 0 {
		parts = append(parts, fmt.Sprintf("%d error(s)", errors))
	}
	if warnings > 0 {
		parts = append(parts, fmt.Sprintf("%d warning(s)", warnings))
	}
	fmt.Fprintf(p.err, "\n%s found\n", strings.Join(parts, " and "))
}

// jsonPrinter writes one JSON object per diagnostic per line, for editors and
// CI tooling. It writes no summary: the stream is the whole output and the
// exit code carries the verdict.
type jsonPrinter struct {
	enc *json.Encoder
}

// jsonDiagnostic is the wire form of one diagnostic. Positions are 1-based;
// endLine/endColumn point one past the flagged expression, matching
// go/token.Position semantics.
type jsonDiagnostic struct {
	File      string `json:"file"`
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	EndLine   int    `json:"endLine"`
	EndColumn int    `json:"endColumn"`
	Severity  string `json:"severity"`
	Check     string `json:"check"`
	Message   string `json:"message"`
	Fix       string `json:"fix,omitempty"`
}

func (p jsonPrinter) diagnostic(d flint.Diagnostic) {
	p.enc.Encode(jsonDiagnostic{
		File:      d.Pos.Filename,
		Line:      d.Pos.Line,
		Column:    d.Pos.Column,
		EndLine:   d.End.Line,
		EndColumn: d.End.Column,
		Severity:  d.Severity.String(),
		Check:     d.Check,
		Message:   d.Message,
		Fix:       d.Fix,
	})
}

func (p jsonPrinter) summary(int, int) {}

// resolvePattern expands a single pattern into file paths.
func resolvePattern(pattern string, includeTests bool) ([]string, error) {
	// Recursive pattern: ./... or path/... or bare ...
	if before, ok := strings.CutSuffix(pattern, "/..."); ok || pattern == "..." {
		root := before
		if root == "" || pattern == "..." {
			root = "."
		}
		return findGoFiles(root, true, includeTests)
	}

	// Check if it's a directory.
	info, err := os.Stat(pattern)
	if err == nil && info.IsDir() {
		return findGoFiles(pattern, false, includeTests)
	}

	// Treat as a file path.
	if _, err := os.Stat(pattern); err != nil {
		return nil, err
	}
	return []string{pattern}, nil
}

// findGoFiles returns all .go files under root, excluding hidden
// directories, testdata, and vendor. Test files are excluded unless
// includeTests is true. If recursive is false, only the immediate
// directory is searched.
func findGoFiles(root string, recursive, includeTests bool) ([]string, error) {
	var files []string

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden directories and testdata - but never the walk root
		// itself, whose name may legitimately start with a dot (".." as a
		// pattern, or an explicitly named hidden directory).
		if d.IsDir() {
			name := d.Name()
			if path != root && (strings.HasPrefix(name, ".") || name == "testdata" || name == "vendor") {
				return filepath.SkipDir
			}
			// If not recursive, skip subdirectories.
			if !recursive && path != root {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if !includeTests && strings.HasSuffix(path, "_test.go") {
			return nil
		}

		files = append(files, path)
		return nil
	})

	return files, err
}

// checkFile reads a file and runs all lint checks against it.
func checkFile(l *flint.Linter, sink *telemetrySink, p printer, path string) (int, int, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, err
	}
	return check(l, sink, p, path, src)
}

// checkStdin reads source code from standard input and runs all lint checks.
func checkStdin(l *flint.Linter, sink *telemetrySink, p printer) (int, int, error) {
	src, err := io.ReadAll(os.Stdin)
	if err != nil {
		return 0, 0, fmt.Errorf("reading stdin: %w", err)
	}
	return check(l, sink, p, "<stdin>", src)
}

// check runs all lint checks against src and prints diagnostics through p,
// recording them to sink when telemetry is enabled. Returns the number of
// errors and warnings found.
func check(l *flint.Linter, sink *telemetrySink, p printer, filename string, src []byte) (int, int, error) {
	diags, err := l.Source(filename, src)
	if err != nil {
		return 0, 0, fmt.Errorf("parsing %s: %w", filename, err)
	}

	var errors, warnings int
	for _, d := range diags {
		p.diagnostic(d)
		switch d.Severity {
		case flint.Warning:
			warnings++
		case flint.Error:
			errors++
		}
		// flint.Info is advisory: printed above, but intentionally uncounted
		// so it never contributes to the summary or a non-zero exit.
	}

	sink.record(l, filename, src, diags)

	return errors, warnings, nil
}

// version returns a string of the form "flint <version>" suitable
// for the -version flag.
func version() string {
	return "flint " + flintVersion()
}

// flintVersion returns the bare flint version, for example "v0.5.0". It comes
// from the embedded module build info populated by go install (a module
// installed by tag) and falls back to "(devel)" for unstamped builds. Telemetry
// records it alongside each run, so -version and telemetry always agree.
func flintVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(devel)"
}
