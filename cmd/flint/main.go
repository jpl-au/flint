// Command flint validates Go source code that uses the fluent HTML framework.
//
// Usage:
//
//	flint [flags] <pattern>...
//	flint [flags] -
//
// Patterns follow Go conventions: ./... checks all Go files recursively,
// ./pkg checks a specific directory, or individual .go files can be named
// directly. When given "-" as the sole argument, it reads from stdin.
//
// Flags:
//
//	-no-registry     Disable symbol validation (only run Static/RawText checks)
//	-include-tests   Include _test.go files in the analysis
//
// Exit codes:
//
//	0  No diagnostics found
//	1  One or more diagnostics found
//	2  Usage or I/O error
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
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: flint [flags] <pattern>...\n")
		fmt.Fprintf(os.Stderr, "       flint [flags] -            (read from stdin)\n\n")
		fmt.Fprintf(os.Stderr, "Patterns:\n")
		fmt.Fprintf(os.Stderr, "  ./...      Check all .go files recursively\n")
		fmt.Fprintf(os.Stderr, "  ./pkg      Check all .go files in a directory\n")
		fmt.Fprintf(os.Stderr, "  file.go    Check a specific file\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

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

	if len(args) == 1 && args[0] == "-" {
		n, err := checkStdin(l)
		if err != nil {
			fmt.Fprintf(os.Stderr, "flint: %v\n", err)
			os.Exit(2)
		}
		if n > 0 {
			fmt.Fprintf(os.Stderr, "\n%d diagnostic(s) found\n", n)
			os.Exit(1)
		}
		return
	}

	files, err := resolvePatterns(args, *includeTests)
	if err != nil {
		fmt.Fprintf(os.Stderr, "flint: %v\n", err)
		os.Exit(2)
	}

	var found int
	for _, path := range files {
		n, err := checkFile(l, path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "flint: %v\n", err)
			os.Exit(2)
		}
		found += n
	}

	if found > 0 {
		fmt.Fprintf(os.Stderr, "\n%d diagnostic(s) found\n", found)
		os.Exit(1)
	}
}

// resolvePatterns expands Go-style patterns into concrete file paths.
// It handles ./... (recursive), directory paths, and individual files.
func resolvePatterns(patterns []string, includeTests bool) ([]string, error) {
	var files []string
	for _, pattern := range patterns {
		resolved, err := resolvePattern(pattern, includeTests)
		if err != nil {
			return nil, err
		}
		files = append(files, resolved...)
	}
	return files, nil
}

// resolvePattern expands a single pattern into file paths.
func resolvePattern(pattern string, includeTests bool) ([]string, error) {
	// Recursive pattern: ./... or path/...
	if before, ok := strings.CutSuffix(pattern, "/..."); ok {
		root := before
		if root == "." || root == "" {
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
			if strings.HasPrefix(name, ".") || name == "testdata" || name == "vendor" {
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
func checkFile(l *flint.Linter, path string) (int, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return check(l, path, src)
}

// checkStdin reads source code from standard input and runs all lint checks.
func checkStdin(l *flint.Linter) (int, error) {
	src, err := io.ReadAll(os.Stdin)
	if err != nil {
		return 0, fmt.Errorf("reading stdin: %w", err)
	}
	return check(l, "<stdin>", src)
}

// check runs all lint checks against src and prints diagnostics to stdout.
// Returns the number of diagnostics found.
func check(l *flint.Linter, filename string, src []byte) (int, error) {
	diags, err := l.Source(filename, src)
	if err != nil {
		return 0, fmt.Errorf("parsing %s: %w", filename, err)
	}

	for _, d := range diags {
		fmt.Printf("%s:%d:%d: %s\n", d.Pos.Filename, d.Pos.Line, d.Pos.Column, d.Message)
		if d.Fix != "" {
			fmt.Printf("  fix: %s\n", d.Fix)
		}
	}

	return len(diags), nil
}
