package main

import (
	"encoding/json"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jpl-au/flint"
)

// writeTree creates a directory tree of empty files under root.
func writeTree(t *testing.T, root string, paths ...string) {
	t.Helper()
	for _, p := range paths {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("package p\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// basenames strips each path to its final element for order-independent
// comparison.
func basenames(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = filepath.Base(p)
	}
	sort.Strings(out)
	return out
}

func TestFindGoFiles(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root,
		"a.go",
		"a_test.go",
		"sub/b.go",
		"sub/notgo.txt",
		".hidden/c.go",
		"testdata/d.go",
		"vendor/e.go",
	)

	t.Run("recursive skips hidden, testdata, vendor, tests", func(t *testing.T) {
		files, err := findGoFiles(root, true, false)
		if err != nil {
			t.Fatal(err)
		}
		got := basenames(files)
		want := []string{"a.go", "b.go"}
		if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("include-tests keeps test files", func(t *testing.T) {
		files, err := findGoFiles(root, true, true)
		if err != nil {
			t.Fatal(err)
		}
		got := basenames(files)
		want := []string{"a.go", "a_test.go", "b.go"}
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("non-recursive stays in the root directory", func(t *testing.T) {
		files, err := findGoFiles(root, false, false)
		if err != nil {
			t.Fatal(err)
		}
		got := basenames(files)
		if len(got) != 1 || got[0] != "a.go" {
			t.Errorf("got %v, want [a.go]", got)
		}
	})

	// A walk root whose name starts with a dot must not be skipped as a
	// hidden directory: ".." as a pattern, or an explicitly named hidden
	// directory, are deliberate choices by the user.
	t.Run("dot-dot root is walked", func(t *testing.T) {
		t.Chdir(filepath.Join(root, "sub"))
		files, err := findGoFiles("..", true, false)
		if err != nil {
			t.Fatal(err)
		}
		got := basenames(files)
		want := []string{"a.go", "b.go"}
		if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("explicit hidden root is walked", func(t *testing.T) {
		files, err := findGoFiles(filepath.Join(root, ".hidden"), true, false)
		if err != nil {
			t.Fatal(err)
		}
		got := basenames(files)
		if len(got) != 1 || got[0] != "c.go" {
			t.Errorf("got %v, want [c.go]", got)
		}
	})
}

func TestJSONPrinter(t *testing.T) {
	var b strings.Builder
	p := jsonPrinter{enc: json.NewEncoder(&b)}
	p.diagnostic(flint.Diagnostic{
		Check:    "deprecated",
		Pos:      token.Position{Filename: "views/home.go", Line: 12, Column: 8},
		End:      token.Position{Filename: "views/home.go", Line: 12, Column: 13},
		Severity: flint.Warning,
		Message:  "embed.Flash is deprecated",
		Fix:      "Flash is no longer supported by browsers.",
	})
	p.summary(0, 1)

	got := strings.TrimSpace(b.String())
	want := `{"file":"views/home.go","line":12,"column":8,"endLine":12,"endColumn":13,"severity":"warning","check":"deprecated","message":"embed.Flash is deprecated","fix":"Flash is no longer supported by browsers."}`
	if got != want {
		t.Errorf("got %s\nwant %s", got, want)
	}
	if strings.Contains(got, "\n") {
		t.Errorf("summary should write nothing in JSON mode, got %q", b.String())
	}
}

func TestJSONPrinterOmitsEmptyFix(t *testing.T) {
	var b strings.Builder
	p := jsonPrinter{enc: json.NewEncoder(&b)}
	p.diagnostic(flint.Diagnostic{Check: "symbols", Message: "x does not exist"})
	if strings.Contains(b.String(), `"fix"`) {
		t.Errorf("empty fix should be omitted, got %s", b.String())
	}
}

func TestTextPrinterExplainsEachFixOnce(t *testing.T) {
	var out, errw strings.Builder
	p := newTextPrinter(&out, &errw)

	shared := flint.Diagnostic{Check: "node-append", Message: `"children" is assembled with append`, Fix: "compose the children directly"}
	p.diagnostic(shared)
	p.diagnostic(shared)
	p.diagnostic(flint.Diagnostic{Check: "node-append", Message: `"items" is assembled with append`, Fix: "a different fix"})

	if got, want := strings.Count(out.String(), "compose the children directly"), 1; got != want {
		t.Errorf("repeated fix printed %d times, want %d:\n%s", got, want, out.String())
	}
	if !strings.Contains(out.String(), "a different fix") {
		t.Errorf("a distinct fix must still be explained, got:\n%s", out.String())
	}
	// Every diagnostic still reports; only the repeated explanation is dropped.
	if got, want := strings.Count(out.String(), "[node-append]"), 3; got != want {
		t.Errorf("printed %d diagnostics, want %d:\n%s", got, want, out.String())
	}
}

func TestTextPrinterSummary(t *testing.T) {
	var out, errw strings.Builder
	p := newTextPrinter(&out, &errw)

	p.summary(0, 0)
	if errw.Len() != 0 {
		t.Errorf("clean run should print no summary, got %q", errw.String())
	}

	p.summary(1, 2)
	if got, want := errw.String(), "\n1 error(s) and 2 warning(s) found\n"; got != want {
		t.Errorf("summary = %q, want %q", got, want)
	}
}

func TestParseFailOn(t *testing.T) {
	for _, tt := range []struct {
		value string
		want  failLevel
	}{
		{"error", failError},
		{"warning", failWarning},
		{"never", failNever},
	} {
		got, err := parseFailOn(tt.value)
		if err != nil {
			t.Errorf("parseFailOn(%q) returned error: %v", tt.value, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseFailOn(%q) = %v, want %v", tt.value, got, tt.want)
		}
	}

	// The zero value is the default level, so a failLevel left unset fails on
	// warnings rather than passing the run.
	var zero failLevel
	if zero != failWarning {
		t.Errorf("zero value of failLevel = %v, want failWarning", zero)
	}
	if !zero.reached(0, 1) {
		t.Error("zero-value failLevel passed a run holding a warning")
	}

	// parseFailOn returns the default level alongside its error, so a caller
	// that ignores the error still fails closed.
	if got, _ := parseFailOn("bogus"); got != failWarning {
		t.Errorf("parseFailOn(\"bogus\") = %v, want failWarning", got)
	}

	if _, err := parseFailOn("info"); err == nil {
		t.Error("parseFailOn(\"info\") returned no error; info never fails a run")
	}
	if _, err := parseFailOn(""); err == nil {
		t.Error("parseFailOn(\"\") returned no error")
	}
}

// TestFailLevelReached locks the exit verdict for each level. Info diagnostics
// are absent from the counts by design, so no level can act on them.
func TestFailLevelReached(t *testing.T) {
	for _, tt := range []struct {
		name     string
		level    failLevel
		errors   int
		warnings int
		want     bool
	}{
		{"error level ignores warnings", failError, 0, 3, false},
		{"error level catches errors", failError, 1, 0, true},
		{"warning level catches warnings", failWarning, 0, 1, true},
		{"warning level catches errors", failWarning, 2, 0, true},
		{"warning level passes a clean run", failWarning, 0, 0, false},
		{"never level ignores everything", failNever, 5, 5, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.level.reached(tt.errors, tt.warnings); got != tt.want {
				t.Errorf("reached(%d, %d) = %v, want %v", tt.errors, tt.warnings, got, tt.want)
			}
		})
	}
}
