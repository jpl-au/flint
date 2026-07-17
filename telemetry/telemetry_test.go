package telemetry

import (
	"go/token"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestRunRoundTrip(t *testing.T) {
	dir := t.TempDir()
	run, err := OpenIn(dir, "github.com/jpl-au/flint", "1.2.3")
	if err != nil {
		t.Fatalf("OpenIn: %v", err)
	}

	pos := func(file string, line, col int) token.Position {
		return token.Position{Filename: file, Line: line, Column: col}
	}

	ref1, err := run.RecordIssue("checkStatic", pos("view.go", 10, 5), "Static() argument must be a string literal", "div.New().Static(name)")
	if err != nil {
		t.Fatalf("RecordIssue: %v", err)
	}
	if want := run.ID() + "01"; ref1 != want {
		t.Errorf("first issue ref = %q, want %q", ref1, want)
	}
	if _, err := run.RecordIssue("checkSymbols", pos("view.go", 12, 3), "node.Fragment does not exist", "node.Fragment()"); err != nil {
		t.Fatalf("RecordIssue: %v", err)
	}

	// The same element and attribute pair accumulates a count.
	run.RecordAttr("div", "class")
	run.RecordAttr("div", "class")
	run.RecordAttr("a", "href")
	run.RecordAttr("div", "id")

	if err := run.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	got, err := Parse(filepath.Join(dir, run.ID()+fileExt))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if got.Version != "1.2.3" {
		t.Errorf("Version = %q, want %q", got.Version, "1.2.3")
	}
	if got.Module != "github.com/jpl-au/flint" {
		t.Errorf("Module = %q, want %q", got.Module, "github.com/jpl-au/flint")
	}
	if wantTS := run.started.Format(time.RFC3339); got.Timestamp.Format(time.RFC3339) != wantTS {
		t.Errorf("Timestamp = %s, want %s", got.Timestamp.Format(time.RFC3339), wantTS)
	}

	if !reflect.DeepEqual(got.Issues, run.issues) {
		t.Errorf("issues round-trip mismatch:\n got  %+v\n want %+v", got.Issues, run.issues)
	}

	// Aggregated and sorted by element then attribute.
	wantAttrs := []Attr{
		{Element: "a", Attribute: "href", Count: 1},
		{Element: "div", Attribute: "class", Count: 2},
		{Element: "div", Attribute: "id", Count: 1},
	}
	if !reflect.DeepEqual(got.Attrs, wantAttrs) {
		t.Errorf("attrs round-trip mismatch:\n got  %+v\n want %+v", got.Attrs, wantAttrs)
	}
}

func TestRecordIssueSanitisesFields(t *testing.T) {
	dir := t.TempDir()
	run, err := OpenIn(dir, "m", "v")
	if err != nil {
		t.Fatalf("OpenIn: %v", err)
	}
	// A tab and newline in the message must not break the tab-separated,
	// line-oriented format.
	if _, err := run.RecordIssue("chk", token.Position{Filename: "f.go", Line: 1, Column: 1}, "line one\ttwo\nthree", "snip"); err != nil {
		t.Fatalf("RecordIssue: %v", err)
	}
	if err := run.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	got, err := Parse(filepath.Join(dir, run.ID()+fileExt))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(got.Issues) != 1 {
		t.Fatalf("got %d issues, want 1", len(got.Issues))
	}
	if want := "line one two three"; got.Issues[0].Message != want {
		t.Errorf("message = %q, want %q", got.Issues[0].Message, want)
	}
}

func TestRecordIssueLimit(t *testing.T) {
	run, err := OpenIn(t.TempDir(), "m", "v")
	if err != nil {
		t.Fatalf("OpenIn: %v", err)
	}
	pos := token.Position{Filename: "f.go", Line: 1, Column: 1}
	for i := range maxIssues {
		if _, err := run.RecordIssue("chk", pos, "msg", "snip"); err != nil {
			t.Fatalf("RecordIssue %d: %v", i+1, err)
		}
	}
	if _, err := run.RecordIssue("chk", pos, "msg", "snip"); err == nil {
		t.Fatalf("RecordIssue beyond %d: expected an error", maxIssues)
	}
}

func TestSnippetHash(t *testing.T) {
	const snippet = "div.New().Static(name)"
	got := snippetHash(snippet)
	if len(got) != 8 {
		t.Errorf("snippetHash length = %d, want 8", len(got))
	}
	for _, r := range got {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			t.Errorf("snippetHash produced non-hex character %q in %q", r, got)
		}
	}
	if again := snippetHash(snippet); again != got {
		t.Errorf("snippetHash is not stable: %q then %q", got, again)
	}
	if other := snippetHash(snippet + " "); other == got {
		t.Errorf("snippetHash did not distinguish different snippets: %q", got)
	}
}

func TestSanitise(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"github.com/jpl-au/flint", "github.com_jpl-au_flint"},
		{"example.com/x@v1.2.3", "example.com_x_v1.2.3"},
		{`a/b\c:d`, "a_b_c_d"},
	}
	for _, tt := range tests {
		if got := sanitise(tt.in); got != tt.want {
			t.Errorf("sanitise(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{"issue too few fields", "[issues]\n01\tcheck\tf.go:1:1\n"},
		{"issue bad sequence", "[issues]\nxx\tcheck\tf.go:1:1\tdeadbeef\tmsg\n"},
		{"attr bad count", "[attrs]\n{\"div\":{\"class\":\"nope\"}}\n"},
		{"attr not json", "[attrs]\ndiv\tclass\t3\n"},
		{"content before region", "hello world\n"},
		{"bad timestamp", "[meta]\ntimestamp\tnot-a-time\n"},
		{"unknown meta key", "[meta]\nwidth\t80\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parse([]byte(tt.data)); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}
