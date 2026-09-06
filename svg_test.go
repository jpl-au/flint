package flint

import (
	"bytes"
	"strings"
	"testing"
)

// svgImport is the single import path that hosts every svg element type.
const svgImport = "github.com/jpl-au/fluent/html5/svg"

// Enum packages referenced by the typed svg attributes.
const (
	unitsImport         = "github.com/jpl-au/fluent/html5/attr/units"
	textAnchorImport    = "github.com/jpl-au/fluent/html5/attr/textanchor"
	fillRuleImport      = "github.com/jpl-au/fluent/html5/attr/fillrule"
	vectorEffectImport  = "github.com/jpl-au/fluent/html5/attr/vectoreffect"
	animationFillImport = "github.com/jpl-au/fluent/html5/attr/animationfill"
)

// TestSVGValidChains checks that constructors and chained methods of the
// multi-element svg package validate against the element each constructor
// returns, including deep chains whose element-specific setters appear after
// the first hop.
func TestSVGValidChains(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name    string
		imports []string
		body    string
	}{
		{name: "rect fill", body: `_ = svg.Rect().Fill("red")`},
		{name: "rect deep chain", body: `_ = svg.Rect().X("0").Y("0").Width("40").Height("120").Fill("var(--blue)")`},
		{name: "circle setters", body: `_ = svg.Circle().Cx("60").Cy("60").R("50")`},
		{name: "group transform", body: `_ = svg.G(svg.Rect()).Transform("translate(10,10)")`},
		{name: "stop offset", body: `_ = svg.Stop().Offset("0").StopColor("#fff")`},
		{name: "root viewbox", body: `_ = svg.New().ViewBox("0 0 10 10")`},
		{name: "linear gradient", imports: []string{unitsImport}, body: `_ = svg.LinearGradient(svg.Stop()).GradientUnits(units.UserSpaceOnUse)`},
		{name: "raw shape", body: `_ = svg.Raw("<path/>")`},
		{name: "text anchor", imports: []string{textAnchorImport}, body: `_ = svg.Text("hi").TextAnchor(textanchor.Middle)`},
		{name: "events mixin reused from html5", body: `_ = svg.Rect().OnPointerDown("go()").SetEvent("onkeydown", "go()")`},
		{name: "core attributes", body: `_ = svg.New().Lang("en").AutoFocus().TransformOrigin("center")`},
		{name: "presentation enums", imports: []string{fillRuleImport, vectorEffectImport}, body: `_ = svg.Path().FillRule(fillrule.EvenOdd).VectorEffect(vectoreffect.NonScalingStroke)`},
		{name: "filter holds primitives", body: `_ = svg.Filter(svg.FeGaussianBlur().StdDeviation("2"), svg.FeOffset().Dx("1")).ID("blur")`},
		{name: "lighting primitive holds a light source", body: `_ = svg.FeDiffuseLighting(svg.FeDistantLight().Azimuth("45")).SurfaceScale("2")`},
		{name: "animation fill takes its own enum", imports: []string{animationFillImport}, body: `_ = svg.Animate().AttributeName("opacity").To("1").Fill(animationfill.Freeze)`},
		{name: "shape fill stays a plain string", body: `_ = svg.Rect().Fill("red")`},
		{name: "use and symbol", body: `_ = svg.Symbol(svg.Rect()).ID("icon")`},
		{name: "accessible name", body: `_ = svg.New(svg.Title("Sales for the year"), svg.Desc("A bar chart"))`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := wrapWithImports(append([]string{svgImport}, tt.imports...), tt.body)
			diags, err := l.Source("test.go", src)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			// Filter to only symbol diagnostics (ignore Static/RawText checks).
			var symbolDiags []Diagnostic
			for _, d := range diags {
				if d.Fix == "" {
					continue
				}
				if !strings.Contains(d.Fix, "replace Static with Text") &&
					!strings.Contains(d.Fix, "replace RawText with Text") {
					symbolDiags = append(symbolDiags, d)
				}
			}
			if len(symbolDiags) > 0 {
				t.Errorf("got %d unexpected diagnostics", len(symbolDiags))
				for _, d := range symbolDiags {
					t.Logf("  %s: %s", d.Pos, d.Message)
				}
			}
		})
	}
}

// TestSVGInvalidMethods checks that a method valid on one svg element but not
// on the one a chain roots at is flagged as an error. The resolution runs
// through the root constructor, so svg.Stop().Width(...) is caught even though
// Width is valid on svg.Rect().
func TestSVGInvalidMethods(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name string
		body string
	}{
		{name: "stop has no Width", body: `_ = svg.Stop().Width("1")`},
		{name: "circle has no D", body: `_ = svg.Circle().D("M0,0")`},
		{name: "rect has no Offset", body: `_ = svg.Rect().Offset("0")`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := wrapWithImports([]string{svgImport}, tt.body)
			diags, err := l.Source("test.go", src)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			found := false
			for _, d := range diags {
				if d.Severity == Error && strings.Contains(d.Message, "does not exist") {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected an Error diagnostic, got:")
				for _, d := range diags {
					t.Logf("  %s: %s", d.Pos, d.Message)
				}
			}
		})
	}
}

// TestSVGEnumAttributes checks that the typed svg enum attributes flag a bare
// string literal where a typed constant is expected. (The valid enum-value usage
// is exercised in TestSVGValidChains.)
func TestSVGEnumAttributes(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name string
		body string
	}{
		{name: "text-anchor string literal", body: `_ = svg.Text("x").TextAnchor("middle")`},
		{name: "stroke-linecap string literal", body: `_ = svg.Rect().StrokeLineCap("round")`},
		{name: "gradient-units string literal", body: `_ = svg.LinearGradient().GradientUnits("userSpaceOnUse")`},
		{name: "fill-rule string literal", body: `_ = svg.Path().FillRule("evenodd")`},
		{name: "dominant-baseline string literal", body: `_ = svg.Text("x").DominantBaseline("central")`},
		{name: "animation fill string literal", body: `_ = svg.Animate().Fill("freeze")`},
		{name: "blend mode string literal", body: `_ = svg.FeBlend().Mode("multiply")`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := wrapWithImports([]string{svgImport}, tt.body)
			diags, err := l.Source("test.go", src)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			found := false
			for _, d := range diags {
				if strings.Contains(d.Message, "typed constant") {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected a typed-constant diagnostic, got:")
				for _, d := range diags {
					t.Logf("  %s: %s", d.Pos, d.Message)
				}
			}
		})
	}
}

// TestSVGInfo checks that flint -info resolves an individual element of the
// multi-element svg package and presents only that element's surface, while
// the package itself lists its elements.
func TestSVGInfo(t *testing.T) {
	reg := FluentRegistry()

	tests := []struct {
		name    string
		element string
		want    []string // substrings that must appear
		notWant []string // substrings that must not appear
		wantErr string   // non-empty means Info should return an error containing this
	}{
		{
			name:    "rect element resolves within svg",
			element: "rect",
			want: []string{
				"Element: rect",
				"Import:  " + svgImport,
				// Match the standalone method line: every svg element carries
				// StrokeWidth and StrokeDashOffset methods, so bare "Width" and
				// "Offset" substrings are not specific to the Width and Offset
				// setters that distinguish rect from stop.
				"  Width(1)\n",
				"Fill",
			},
			notWant: []string{"  Offset(1)\n"},
		},
		{
			name:    "stop element resolves within svg",
			element: "stop",
			want: []string{
				"Element: stop",
				"  Offset(1)\n",
			},
			notWant: []string{"  Width(1)\n"},
		},
		{
			name:    "svg package lists its elements",
			element: "svg",
			want: []string{
				"Element: svg",
				"Elements:",
				"rect",
				"circle",
				"stop",
			},
		},
		{
			name:    "linearGradient element resolves within svg",
			element: "linearGradient",
			want:    []string{"Element: linearGradient"},
		},
		{
			name:    "svg:text prefix reaches the svg <text> element",
			element: "svg:text",
			want:    []string{"Element: text", "Import:  " + svgImport, "TextAnchor"},
		},
		{
			name:    "svg:rect prefix reaches the rect shape",
			element: "svg:rect",
			want:    []string{"Element: rect", "  Width(1)\n"},
			notWant: []string{"  Offset(1)\n"},
		},
		{
			name:    "bare text still resolves the text node package",
			element: "text",
			want:    []string{"Import:  github.com/jpl-au/fluent/text"},
			notWant: []string{"TextAnchor"},
		},
		{
			name:    "unknown shape returns error",
			element: "nonexistent-shape",
			wantErr: "unknown element",
		},
		{
			name:    "unknown prefixed element returns error",
			element: "svg:bogus",
			wantErr: "unknown element",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := reg.Info(&buf, tt.element)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error %q does not contain %q", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			out := buf.String()
			for _, s := range tt.want {
				if !strings.Contains(out, s) {
					t.Errorf("output missing %q", s)
				}
			}
			for _, s := range tt.notWant {
				if strings.Contains(out, s) {
					t.Errorf("output unexpectedly contains %q", s)
				}
			}
		})
	}
}
