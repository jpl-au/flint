package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jpl-au/flint"
	"github.com/jpl-au/flint/telemetry"
)

// telemetryCommand handles the -telemetry flag against the config directory dir.
// "status" prints the current mode to out; any valid mode name persists that
// mode. It returns a useful error for an unknown mode name or a persistence
// failure, which the caller turns into a usage exit.
func telemetryCommand(out io.Writer, dir, arg string) error {
	if arg == "status" {
		mode, err := telemetry.CurrentModeIn(dir)
		if err != nil {
			return err
		}
		fmt.Fprintln(out, mode)
		return nil
	}

	mode, err := telemetry.ParseMode(arg)
	if err != nil {
		return err
	}
	return telemetry.SetModeIn(dir, mode)
}

// telemetrySink accumulates telemetry for one flint invocation. A single run
// spans every file checked and is written on close. A nil sink records nothing,
// so callers need not special-case the telemetry-off mode.
type telemetrySink struct {
	run   *telemetry.Run
	muted bool // set after the first issue-recording failure so it is reported once, not per file
}

// openTelemetry returns a sink when the persisted mode enables collection, or a
// nil sink when telemetry is off. Telemetry is best-effort: a failure to read
// the mode or open the run is reported to stderr and yields a nil sink, so
// linting proceeds regardless. When the mode is off, no run is opened and no
// files are written.
func openTelemetry(version string) *telemetrySink {
	mode, err := telemetry.CurrentMode()
	if err != nil {
		warnTelemetry(err)
		return nil
	}
	if mode == telemetry.Off {
		return nil
	}

	run, err := telemetry.Open(moduleName(), version)
	if err != nil {
		warnTelemetry(err)
		return nil
	}
	return &telemetrySink{run: run}
}

// record adds one file's diagnostics and attribute usage to the run. Telemetry
// failures never affect lint results: each is reported to stderr and recording
// continues where it safely can. A nil sink is a no-op.
func (s *telemetrySink) record(l *flint.Linter, filename string, src []byte, diags []flint.Diagnostic) {
	if s == nil {
		return
	}

	for _, d := range diags {
		if s.muted {
			break
		}
		if _, err := s.run.RecordIssue(d.Check, d.Pos, d.Message, span(src, d.Pos.Offset, d.End.Offset)); err != nil {
			// The only expected failure is the per-run issue limit. Report it
			// once and stop recording issues; attribute usage has no such cap.
			warnTelemetry(err)
			s.muted = true
			break
		}
	}

	pairs, err := l.AttrPairs(filename, src)
	if err != nil {
		// Source already parsed this file, so a parse error here is unexpected.
		warnTelemetry(err)
		return
	}
	for _, p := range pairs {
		s.run.RecordAttr(p.Element, p.Attribute)
	}
}

// close writes the run's telemetry file, reporting any failure to stderr. A nil
// sink is a no-op.
func (s *telemetrySink) close() {
	if s == nil {
		return
	}
	if err := s.run.Close(); err != nil {
		warnTelemetry(err)
	}
}

// warnTelemetry reports a non-fatal telemetry problem to stderr. Telemetry is
// best-effort, so these never change lint results or exit codes, but they are
// never silently discarded either.
func warnTelemetry(err error) {
	fmt.Fprintf(os.Stderr, "flint: telemetry: %v\n", err)
}

// span returns src[start:end] when the offsets form a valid range within src,
// or the empty string otherwise. A telemetry snippet is only ever hashed, so an
// invalid span contributes an empty hash rather than panicking.
func span(src []byte, start, end int) string {
	if start < 0 || end < start || end > len(src) {
		return ""
	}
	return string(src[start:end])
}

// moduleName returns the module path from the nearest go.mod at or above the
// working directory, or "(none)" when no module is found. Telemetry groups runs
// by module, so a placeholder keeps runs from an unmodularised tree together
// rather than failing the run.
func moduleName() string {
	dir, err := os.Getwd()
	if err != nil {
		return "(none)"
	}
	for {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil {
			if mod := modulePath(data); mod != "" {
				return mod
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "(none)"
		}
		dir = parent
	}
}

// modulePath returns the path from a go.mod's module directive, or "" when the
// file has none. The module directive is always a single line, "module <path>",
// with the path optionally quoted and an optional trailing comment.
func modulePath(goMod []byte) string {
	sc := bufio.NewScanner(bytes.NewReader(goMod))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 || fields[0] != "module" {
			continue
		}
		path := fields[1]
		if unquoted, err := strconv.Unquote(path); err == nil {
			path = unquoted
		}
		return path
	}
	return ""
}
