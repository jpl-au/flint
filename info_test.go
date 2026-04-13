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
