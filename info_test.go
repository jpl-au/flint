package flint

import (
	"bytes"
	"strings"
	"testing"
)

func TestInfo(t *testing.T) {
	reg := FluentRegistry()

	tests := []struct {
		name    string
		element string
		want    []string // substrings that must appear in the output
		wantErr string   // non-empty means Info should return an error containing this
	}{
		{
			name:    "div shows header and all sections",
			element: "div",
			want: []string{
				"Element: div",
				"Import:  github.com/jpl-au/fluent/html5/div",
				"Types:",
				"Element",
				"Constructors:",
				"New(...)  variadic",
				"Text(1)",
				"Static(1)",
				"Methods:",
				"Class",
				"ID",
				"Attribute Mappings:",
				"class",
			},
		},
		{
			name:    "div typed params show enum annotation",
			element: "div",
			want: []string{
				"Dir  (enum: dir)",
				"Translate  (enum: translate)",
			},
		},
		{
			name:    "ol shows typed constructors section",
			element: "ol",
			want: []string{
				"Typed Constructors:",
				"Items  accepts li.Element children",
				"Decimal  accepts li.Element children",
			},
		},
		{
			name:    "input shows element-specific constructors",
			element: "input",
			want: []string{
				"Email(1)",
				"Checkbox(2)",
				"Hidden(2)",
				"New(0)",
			},
		},
		{
			name:    "enum package shows vars",
			element: "inputtype",
			want: []string{
				"Vars:",
				"Email",
				"Tel",
			},
		},
		{
			name:    "constructors are sorted alphabetically",
			element: "div",
			want:    []string{"New(...)  variadic\n  RawText("},
		},
		{
			name:    "unknown element returns error",
			element: "nonexistent",
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
		})
	}
}

// TestInfoSections verifies the section filter argument accepts long
// and short forms and restricts the output to the chosen sections.
func TestInfoSections(t *testing.T) {
	reg := FluentRegistry()

	tests := []struct {
		name     string
		element  string
		sections []string
		want     []string // substrings that must appear
		notWant  []string // substrings that must not appear
		wantErr  string
	}{
		{
			name:     "methods only hides constructors and attributes",
			element:  "div",
			sections: []string{"methods"},
			want:     []string{"Element: div", "Methods:", "Class"},
			notWant:  []string{"Constructors:", "Attribute Mappings:", "Types:"},
		},
		{
			name:     "short form ctors matches constructors",
			element:  "div",
			sections: []string{"ctors"},
			want:     []string{"Constructors:", "New(...)"},
			notWant:  []string{"Methods:", "Attribute Mappings:"},
		},
		{
			name:     "multiple sections stack",
			element:  "div",
			sections: []string{"ctors", "attrs"},
			want:     []string{"Constructors:", "Attribute Mappings:"},
			notWant:  []string{"Methods:", "Types:"},
		},
		{
			name:     "typed short form selects typed constructors",
			element:  "ol",
			sections: []string{"typed"},
			want:     []string{"Typed Constructors:", "Items  accepts li.Element children"},
			notWant:  []string{"Methods:", "\nConstructors:"},
		},
		{
			name:     "vars only on enum package",
			element:  "inputtype",
			sections: []string{"vars"},
			want:     []string{"Vars:", "Email"},
			notWant:  []string{"Methods:", "Types:"},
		},
		{
			name:     "unknown section returns error",
			element:  "div",
			sections: []string{"bogus"},
			wantErr:  "unknown section",
		},
		{
			name:     "unknown section error lists valid names",
			element:  "div",
			sections: []string{"bogus"},
			wantErr:  "constructors",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := reg.Info(&buf, tt.element, tt.sections...)

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
