package flint

import (
	"strings"
	"testing"
)

func TestCheckURLSchemePositive(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "javascript scheme on a URL method",
			src: `package p

import "github.com/jpl-au/fluent/html5/a"

func f() {
	_ = a.New().Href("javascript:alert(1)")
}`,
			want: `.Href() is given a "javascript:" URL. Fluent rejects this scheme and renders "#fluent-unsafe-url" instead.`,
		},
		{
			name: "leading whitespace does not hide the scheme",
			src: `package p

import "github.com/jpl-au/fluent/html5/a"

func f() {
	_ = a.New().Href(" javascript:x")
}`,
			want: `.Href() is given a "javascript:" URL. Fluent rejects this scheme and renders "#fluent-unsafe-url" instead.`,
		},
		{
			name: "scheme match is case-insensitive",
			src: `package p

import "github.com/jpl-au/fluent/html5/a"

func f() {
	_ = a.New().Href("JavaScript:x")
}`,
			want: `.Href() is given a "JavaScript:" URL. Fluent rejects this scheme and renders "#fluent-unsafe-url" instead.`,
		},
		{
			name: "data scheme is rejected too",
			src: `package p

import "github.com/jpl-au/fluent/html5/a"

func f() {
	_ = a.New().Href("data:text/html,<script>x</script>")
}`,
			want: `.Href() is given a "data:" URL. Fluent rejects this scheme and renders "#fluent-unsafe-url" instead.`,
		},
		{
			name: "constructor URL param at a non-zero index",
			src: `package p

import "github.com/jpl-au/fluent/html5/a"

func f() {
	_ = a.Download("Report", "javascript:steal()", "report.pdf")
}`,
			want: `a.Download is given a "javascript:" URL. Fluent rejects this scheme and renders "#fluent-unsafe-url" instead.`,
		},
		{
			name: "URL method on a local assigned from a fluent chain",
			src: `package p

import "github.com/jpl-au/fluent/html5/a"

func f() {
	lnk := a.New()
	_ = lnk.Href("vbscript:x")
}`,
			want: `.Href() is given a "vbscript:" URL. Fluent rejects this scheme and renders "#fluent-unsafe-url" instead.`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags, err := l.Source("test.go", []byte(tt.src))
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			found := false
			for _, d := range diags {
				if d.Check == "url-scheme" && d.Message == tt.want {
					found = true
					if d.Severity != Warning {
						t.Errorf("severity = %v, want Warning", d.Severity)
					}
					if !strings.Contains(d.Fix, "http, https, mailto, tel, sms or a relative URL") {
						t.Errorf("fix = %q, missing allowlist guidance", d.Fix)
					}
					break
				}
			}
			if !found {
				t.Errorf("expected diagnostic %q", tt.want)
				for _, d := range diags {
					t.Logf("  %s: [%s] %s", d.Pos, d.Check, d.Message)
				}
			}
		})
	}
}

func TestCheckURLSchemeNegative(t *testing.T) {
	l := New(FluentRegistry())

	tests := []struct {
		name string
		src  string
	}{
		{
			name: "allowed and relative URLs are fine",
			src: `package p

import "github.com/jpl-au/fluent/html5/a"

func f() {
	_ = a.New().Href("https://example.com")
	_ = a.New().Href("mailto:me@example.com")
	_ = a.New().Href("tel:+61400000000")
	_ = a.New().Href("/docs/guide")
	_ = a.New().Href("?redirect=a:b")
	_ = a.New().Href("#t:30")
}`,
		},
		{
			name: "non-literal URLs are not judged",
			src: `package p

import "github.com/jpl-au/fluent/html5/a"

func f(u string) {
	_ = a.New().Href(u)
}`,
		},
		{
			name: "non-URL methods take colons freely",
			src: `package p

import "github.com/jpl-au/fluent/html5/div"

func f() {
	_ = div.New().Title("ratio 16:9")
}`,
		},
		{
			name: "non-fluent chains are not flint's concern",
			src: `package p

import "example.com/links"

func f() {
	_ = links.New().Href("javascript:x")
}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags, err := l.Source("test.go", []byte(tt.src))
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			for _, d := range diags {
				if d.Check == "url-scheme" {
					t.Errorf("unexpected diagnostic: %s", d.Message)
				}
			}
		})
	}
}
