// Package telemetry records local, opt-in diagnostics for a single flint
// run and writes them to a greppable text file beneath the user cache
// directory. Each run produces one <run-id>.tlf file with three regions,
// [meta], [issues] and [attrs], that later stats and report commands read
// back with Parse.
//
// A run ID is eight characters: the Unix time of the run in Crockford
// base32, zero-padded to seven characters, plus one collision character.
// The fixed width and ascending alphabet make lexicographic order match
// chronological order, so run files sort by time. An issue reference
// appends a two-digit sequence number, giving a stable ten-character handle
// for a single diagnostic within a run.
package telemetry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	// fileExt is the extension of a telemetry log file.
	fileExt = ".tlf"

	metaMarker   = "[meta]"
	issuesMarker = "[issues]"
	attrsMarker  = "[attrs]"

	issueFields = 5 // seq, check, position, hash, message

	// maxIssues is the number of issues a run can hold. The sequence number
	// is two digits, so references beyond 99 could no longer be parsed.
	maxIssues = 99
)

// fieldSanitiser collapses the tab, carriage return and newline characters
// that would otherwise break the line-oriented file format into single
// spaces. Diagnostic messages are single-line by construction; this guards
// against a stray control character corrupting the file.
var fieldSanitiser = strings.NewReplacer("\t", " ", "\n", " ", "\r", " ")

// Run accumulates telemetry for a single flint invocation in memory and
// writes it to a <run-id>.tlf file when Close is called. Create one with
// Open or OpenIn; the zero value is not usable.
type Run struct {
	dir     string
	id      string
	module  string
	version string
	started time.Time

	seq    int
	issues []Issue
	attrs  map[attrKey]int
}

// attrKey identifies an element and attribute pair for count aggregation.
type attrKey struct {
	element   string
	attribute string
}

// Open starts a telemetry run for the given module, storing its file
// beneath the user cache directory (see os.UserCacheDir). The module path
// selects a per-module subdirectory and version records the flint version
// in the file's metadata. Accumulated telemetry is written on Close.
func Open(module, version string) (*Run, error) {
	dir, err := cacheDir(module)
	if err != nil {
		return nil, err
	}
	return OpenIn(dir, module, version)
}

// OpenIn is Open with an explicit storage directory in place of the user
// cache directory. It exists so callers, and tests, can direct telemetry at
// a chosen location.
func OpenIn(dir, module, version string) (*Run, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating telemetry directory %q: %w", dir, err)
	}

	// One instant drives both the run ID and the recorded timestamp so they
	// always agree.
	now := time.Now()
	id, err := allocRunID(dir, now)
	if err != nil {
		return nil, err
	}

	return &Run{
		dir:     dir,
		id:      id,
		module:  module,
		version: version,
		started: now,
		attrs:   map[attrKey]int{},
	}, nil
}

// ID returns the run's eight-character identifier. Every issue reference the
// run hands out begins with this value, and the telemetry file is named
// <ID>.tlf.
func (r *Run) ID() string { return r.id }

// RecordIssue records one diagnostic against the run and returns its
// ten-character issue reference (the run ID plus a two-digit sequence
// number). check names the check that fired, pos locates the offending
// source, message describes the problem, and snippet is the flagged source
// text, which is stored only as a short hash. A run holds at most 99 issues;
// RecordIssue returns an error once that limit is reached rather than
// silently dropping the diagnostic.
func (r *Run) RecordIssue(check string, pos token.Position, message, snippet string) (string, error) {
	if r.seq >= maxIssues {
		return "", fmt.Errorf("run %s already holds the maximum of %d issues", r.id, maxIssues)
	}
	r.seq++
	r.issues = append(r.issues, Issue{
		Seq:     r.seq,
		Check:   fieldSanitiser.Replace(check),
		Pos:     fieldSanitiser.Replace(pos.String()),
		Hash:    snippetHash(snippet),
		Message: fieldSanitiser.Replace(message),
	})
	return IssueRef(r.id, r.seq), nil
}

// RecordAttr records one use of an attribute on an element. Counts for the
// same element and attribute pair accumulate over the run and are written to
// the [attrs] region, sorted by element then attribute, on Close.
func (r *Run) RecordAttr(element, attribute string) {
	r.attrs[attrKey{
		element:   fieldSanitiser.Replace(element),
		attribute: fieldSanitiser.Replace(attribute),
	}]++
}

// Close writes the accumulated telemetry to <ID>.tlf in the run's directory.
// It reports any error from rendering or the write and never discards one.
func (r *Run) Close() error {
	var b strings.Builder
	if err := r.render(&b); err != nil {
		return err
	}

	path := filepath.Join(r.dir, r.id+fileExt)
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("writing telemetry file %q: %w", path, err)
	}
	return nil
}

// render writes the run's three regions, [meta], [issues] and [attrs], to b
// in that order. The [attrs] region is JSON lines, one object per element in
// sorted order, so an element's name appears once however many attributes it
// carries. Encoding a line is the only step that can fail.
func (r *Run) render(b *strings.Builder) error {
	fmt.Fprintln(b, metaMarker)
	fmt.Fprintf(b, "version\t%s\n", r.version)
	fmt.Fprintf(b, "module\t%s\n", r.module)
	fmt.Fprintf(b, "timestamp\t%s\n", r.started.Format(time.RFC3339))

	fmt.Fprintln(b, issuesMarker)
	for _, is := range r.issues {
		fmt.Fprintf(b, "%02d\t%s\t%s\t%s\t%s\n", is.Seq, is.Check, is.Pos, is.Hash, is.Message)
	}

	fmt.Fprintln(b, attrsMarker)
	grouped := make(map[string]map[string]int)
	for key, count := range r.attrs {
		counts := grouped[key.element]
		if counts == nil {
			counts = make(map[string]int)
			grouped[key.element] = counts
		}
		counts[key.attribute] = count
	}
	for _, element := range slices.Sorted(maps.Keys(grouped)) {
		line, err := json.Marshal(map[string]map[string]int{element: grouped[element]})
		if err != nil {
			return fmt.Errorf("encoding attrs for element %q: %w", element, err)
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	return nil
}

// snippetHash returns the first eight hex characters of the SHA-256 digest
// of the flagged source snippet. It is short enough to keep the telemetry
// file readable while still distinguishing distinct snippets in practice.
func snippetHash(snippet string) string {
	sum := sha256.Sum256([]byte(snippet))
	return hex.EncodeToString(sum[:4])
}

// cacheDir returns the telemetry directory for module beneath the user cache
// directory, for example ~/Library/Caches/flint/<sanitised module>.
func cacheDir(module string) (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolving user cache directory: %w", err)
	}
	return filepath.Join(base, "flint", sanitise(module)), nil
}

// sanitise rewrites a module path into a single safe directory name by
// replacing every character that is not a letter, digit, dot, hyphen or
// underscore, path separators included, with an underscore.
func sanitise(module string) string {
	var b strings.Builder
	b.Grow(len(module))
	for _, r := range module {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.' || r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}
