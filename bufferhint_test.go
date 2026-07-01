package flint

import (
	"strings"
	"testing"
)

const (
	pPkg    = "github.com/jpl-au/fluent/html5/p"
	textPkg = "github.com/jpl-au/fluent/text"
)

func TestCheckBufferHintPositive(t *testing.T) {
	l := New(FluentRegistry())
	big := strings.Repeat("x", bufferHintThreshold+100)

	tests := []struct {
		name string
		body string
	}{
		{
			name: "single large static literal",
			body: `_ = div.Static("` + big + `")`,
		},
		{
			name: "large content summed across children",
			body: `_ = div.New(
	p.Static("` + strings.Repeat("a", 2100) + `"),
	p.Static("` + strings.Repeat("b", 2100) + `"),
)`,
		},
		{
			name: "large content on a chained element still fires",
			body: `_ = div.Static("` + big + `").Class("page")`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := wrapWithImports([]string{divPkg, pPkg}, tt.body)
			diags, err := l.Source("test.go", src)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			d, ok := findBufferHint(diags)
			if !ok {
				t.Fatalf("expected a buffer-hint diagnostic; got %d diagnostics", len(diags))
			}
			if d.Severity != Info {
				t.Errorf("severity = %v, want Info", d.Severity)
			}
			if !strings.Contains(d.Fix, "BufferHint(") {
				t.Errorf("Fix = %q, want it to suggest BufferHint(...)", d.Fix)
			}
		})
	}
}

func TestCheckBufferHintNegative(t *testing.T) {
	l := New(FluentRegistry())
	big := strings.Repeat("x", bufferHintThreshold+100)

	tests := []struct {
		name    string
		imports []string
		body    string
	}{
		{
			name:    "small element is left alone",
			imports: []string{divPkg},
			body:    `_ = div.Static("a small amount of content")`,
		},
		{
			name:    "element that already has BufferHint is not flagged again",
			imports: []string{divPkg},
			body:    `_ = div.Static("` + big + `").BufferHint(5000)`,
		},
		{
			name:    "large literal outside a Fluent element is left alone",
			imports: []string{},
			body:    `s := "` + big + `"; _ = s`,
		},
		{
			name:    "large text node has no buffer and is left alone",
			imports: []string{textPkg},
			body:    `_ = text.Static("` + big + `")`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := wrapWithImports(tt.imports, tt.body)
			diags, err := l.Source("test.go", src)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			if d, ok := findBufferHint(diags); ok {
				t.Errorf("unexpected buffer-hint diagnostic: %s", d.Message)
			}
		})
	}
}

func TestCheckBufferHintNeedsRegistry(t *testing.T) {
	// Without a registry the check cannot resolve Fluent elements, so it stays silent.
	l := New(nil)
	body := `_ = div.Static("` + strings.Repeat("x", bufferHintThreshold+100) + `")`
	src := wrapWithImports([]string{divPkg}, body)
	diags, err := l.Source("test.go", src)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if _, ok := findBufferHint(diags); ok {
		t.Error("unexpected diagnostic without registry")
	}
}

// findBufferHint returns the first buffer-hint diagnostic, identified by its
// distinctive message.
func findBufferHint(diags []Diagnostic) (Diagnostic, bool) {
	for _, d := range diags {
		if strings.Contains(d.Message, "bytes of static content") {
			return d, true
		}
	}
	return Diagnostic{}, false
}
