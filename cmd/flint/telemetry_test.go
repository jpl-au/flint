package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jpl-au/flint"
	"github.com/jpl-au/flint/telemetry"
)

// telemetrySrc lints to one "static" diagnostic (Static with a variable) and
// one (div, class) attribute use, so a run records both an issue and an attr.
const telemetrySrc = `package view

import "github.com/jpl-au/fluent/html5/div"

func build(name string) {
	_ = div.New().Static(name).Class("brand")
}
`

func TestTelemetryCommandSetAndStatus(t *testing.T) {
	dir := t.TempDir()

	if err := telemetryCommand(io.Discard, dir, "local"); err != nil {
		t.Fatalf("set local: %v", err)
	}
	if mode, err := telemetry.CurrentModeIn(dir); err != nil || mode != telemetry.Local {
		t.Fatalf("after set, mode = %v (err %v), want local", mode, err)
	}

	var out bytes.Buffer
	if err := telemetryCommand(&out, dir, "status"); err != nil {
		t.Fatalf("status: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "local" {
		t.Errorf("status printed %q, want %q", got, "local")
	}
}

func TestTelemetryCommandInvalidValue(t *testing.T) {
	if err := telemetryCommand(io.Discard, t.TempDir(), "sideways"); err == nil {
		t.Fatal("telemetryCommand with an invalid mode: expected an error")
	}
}

func TestTelemetryLocalRecordsRun(t *testing.T) {
	cfgDir := t.TempDir()
	tlfDir := t.TempDir()

	// Persist and read back the mode through the config-dir seam, then open the
	// run through the telemetry-dir seam: the same decision openTelemetry makes.
	if err := telemetryCommand(io.Discard, cfgDir, "local"); err != nil {
		t.Fatalf("set local: %v", err)
	}
	if mode, err := telemetry.CurrentModeIn(cfgDir); err != nil || mode != telemetry.Local {
		t.Fatalf("mode = %v (err %v), want local", mode, err)
	}

	run, err := telemetry.OpenIn(tlfDir, "example.com/mod", "v0.0.0-test")
	if err != nil {
		t.Fatalf("OpenIn: %v", err)
	}
	sink := &telemetrySink{run: run}

	l := flint.New(flint.FluentRegistry())
	src := []byte(telemetrySrc)
	diags, err := l.Source("view.go", src)
	if err != nil {
		t.Fatalf("Source: %v", err)
	}
	sink.record(l, "view.go", src, diags)
	sink.close()

	log := parseSoleTLF(t, tlfDir)
	if log.Module != "example.com/mod" {
		t.Errorf("module = %q, want %q", log.Module, "example.com/mod")
	}
	if log.Version != "v0.0.0-test" {
		t.Errorf("version = %q, want %q", log.Version, "v0.0.0-test")
	}

	if !slices.ContainsFunc(log.Issues, func(is telemetry.Issue) bool { return is.Check == "static" }) {
		t.Errorf("no issue with check %q recorded; issues = %+v", "static", log.Issues)
	}
	if !slices.ContainsFunc(log.Attrs, func(a telemetry.Attr) bool {
		return a.Element == "div" && a.Attribute == "class" && a.Count >= 1
	}) {
		t.Errorf("(div, class) attribute not recorded; attrs = %+v", log.Attrs)
	}
}

func TestTelemetryOffProducesNoFile(t *testing.T) {
	cfgDir := t.TempDir()
	tlfDir := t.TempDir()

	// No mode file means Off, so openTelemetry would return a nil sink and open
	// no run. A nil sink's record and close must be no-ops that write nothing.
	mode, err := telemetry.CurrentModeIn(cfgDir)
	if err != nil {
		t.Fatalf("CurrentModeIn: %v", err)
	}
	if mode != telemetry.Off {
		t.Fatalf("mode = %v, want off", mode)
	}

	var sink *telemetrySink
	l := flint.New(flint.FluentRegistry())
	src := []byte(telemetrySrc)
	diags, err := l.Source("view.go", src)
	if err != nil {
		t.Fatalf("Source: %v", err)
	}
	sink.record(l, "view.go", src, diags)
	sink.close()

	entries, err := os.ReadDir(tlfDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("mode off wrote %d telemetry file(s), want 0", len(entries))
	}
}

func TestModulePath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "module github.com/jpl-au/flint\n\ngo 1.25.0\n", "github.com/jpl-au/flint"},
		{"quoted", "module \"example.com/x\"\n", "example.com/x"},
		{"trailing comment", "module example.com/y // legacy\n", "example.com/y"},
		{"no module directive", "go 1.25.0\n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := modulePath([]byte(tt.in)); got != tt.want {
				t.Errorf("modulePath = %q, want %q", got, tt.want)
			}
		})
	}
}

// parseSoleTLF parses the single .tlf file in dir, failing if there is not
// exactly one.
func parseSoleTLF(t *testing.T, dir string) *telemetry.Log {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var tlfs []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tlf") {
			tlfs = append(tlfs, e.Name())
		}
	}
	if len(tlfs) != 1 {
		t.Fatalf("found %d .tlf files in %s, want 1", len(tlfs), dir)
	}
	log, err := telemetry.Parse(filepath.Join(dir, tlfs[0]))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return log
}
