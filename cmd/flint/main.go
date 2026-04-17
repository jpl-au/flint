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
// attribute mappings, and vars. No linting is performed.
//
//	flint -info div
//	flint -info input
//	flint -info ol
//
// Pass one or more section names after the element to restrict the
// output. Each section accepts a long form and (where useful) a short
// form: types, constructors/ctors, typed-constructors/typed, methods,
// attributes/attrs, vars.
//
//	flint -info div methods
//	flint -info input ctors attrs
//	flint -info ol typed
//
// Flags:
//
//	-no-registry     Disable symbol validation (only run Static/RawText checks)
//	-include-tests   Include _test.go files in the analysis
//	-info <element>  Show registry info for an element and exit
//
// Exit codes:
//
//	0  No errors found (warnings may be present)
//	1  One or more errors found
//	2  Usage or I/O error (including unknown element for -info)
package main

import (
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/jpl-au/flint"
)

func main() {
	noRegistry := flag.Bool("no-registry", false, "Disable symbol validation")
	includeTests := flag.Bool("include-tests", false, "Include _test.go files")
	infoElement := flag.String("info", "", "Show registry info for an element (e.g. -info div)")
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
		fmt.Fprintf(os.Stderr, "  vars\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

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

	args := flag.Args()

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
			e, w, err := checkStdin(l)
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
			e, w, err := checkFile(l, path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "flint: %v\n", err)
				hadErrors = true
				continue
			}
			errors += e
			warnings += w
		}
	}

	if hadErrors {
		os.Exit(2)
	}
	if errors+warnings > 0 {
		printSummary(errors, warnings)
	}
	if errors > 0 {
		os.Exit(1)
	}
}

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

		// Skip hidden directories and testdata.
		if d.IsDir() {
			name := d.Name()
			if (name != "." && strings.HasPrefix(name, ".")) || name == "testdata" || name == "vendor" {
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
func checkFile(l *flint.Linter, path string) (int, int, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, err
	}
	return check(l, path, src)
}

// checkStdin reads source code from standard input and runs all lint checks.
func checkStdin(l *flint.Linter) (int, int, error) {
	src, err := io.ReadAll(os.Stdin)
	if err != nil {
		return 0, 0, fmt.Errorf("reading stdin: %w", err)
	}
	return check(l, "<stdin>", src)
}

// check runs all lint checks against src and prints diagnostics to stdout.
// Returns the number of errors and warnings found.
func check(l *flint.Linter, filename string, src []byte) (int, int, error) {
	diags, err := l.Source(filename, src)
	if err != nil {
		return 0, 0, fmt.Errorf("parsing %s: %w", filename, err)
	}

	var errors, warnings int
	for _, d := range diags {
		fmt.Printf("%s:%d:%d: %s: %s\n", d.Pos.Filename, d.Pos.Line, d.Pos.Column, d.Severity, d.Message)
		if d.Fix != "" {
			fmt.Printf("  fix: %s\n", d.Fix)
		}
		if d.Severity == flint.Warning {
			warnings++
		} else {
			errors++
		}
	}

	return errors, warnings, nil
}

// printSummary writes a summary line to stderr.
func printSummary(errors, warnings int) {
	var parts []string
	if errors > 0 {
		parts = append(parts, fmt.Sprintf("%d error(s)", errors))
	}
	if warnings > 0 {
		parts = append(parts, fmt.Sprintf("%d warning(s)", warnings))
	}
	fmt.Fprintf(os.Stderr, "\n%s found\n", strings.Join(parts, " and "))
}
