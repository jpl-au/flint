package telemetry

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Log is the parsed contents of a .tlf telemetry file.
type Log struct {
	Version   string
	Module    string
	Timestamp time.Time
	Issues    []Issue
	Attrs     []Attr
}

// Issue is a single diagnostic recorded during a run.
type Issue struct {
	Seq     int
	Check   string
	Pos     string // source position as file:line:col
	Hash    string // first eight hex characters of the snippet's SHA-256 digest
	Message string
}

// Attr is an element and attribute pair with the number of times it was
// recorded during a run.
type Attr struct {
	Element   string
	Attribute string
	Count     int
}

// Parse reads and parses the .tlf telemetry file at path.
func Parse(path string) (*Log, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading telemetry file %q: %w", path, err)
	}
	return parse(data)
}

// parse turns the raw bytes of a telemetry file into a Log. It works line by
// line so that a format error can name the offending line number.
func parse(data []byte) (*Log, error) {
	lg := &Log{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	var region string
	line := 0
	for sc.Scan() {
		line++
		text := sc.Text()
		if text == "" {
			continue
		}
		switch text {
		case metaMarker, issuesMarker, attrsMarker:
			region = text
			continue
		}

		switch region {
		case metaMarker:
			if err := parseMeta(lg, text); err != nil {
				return nil, fmt.Errorf("line %d: %w", line, err)
			}
		case issuesMarker:
			is, err := parseIssue(text)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", line, err)
			}
			lg.Issues = append(lg.Issues, is)
		case attrsMarker:
			as, err := parseAttrs(text)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", line, err)
			}
			lg.Attrs = append(lg.Attrs, as...)
		default:
			return nil, fmt.Errorf("line %d: content before any region marker: %q", line, text)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scanning telemetry file: %w", err)
	}
	return lg, nil
}

// parseMeta reads one key-value line of the [meta] region into lg.
func parseMeta(lg *Log, line string) error {
	key, value, ok := strings.Cut(line, "\t")
	if !ok {
		return fmt.Errorf("meta line missing tab separator: %q", line)
	}
	switch key {
	case "version":
		lg.Version = value
	case "module":
		lg.Module = value
	case "timestamp":
		t, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return fmt.Errorf("meta timestamp %q: %w", value, err)
		}
		lg.Timestamp = t
	default:
		return fmt.Errorf("unknown meta key %q", key)
	}
	return nil
}

// parseIssue reads one tab-separated line of the [issues] region.
func parseIssue(line string) (Issue, error) {
	f := strings.SplitN(line, "\t", issueFields)
	if len(f) != issueFields {
		return Issue{}, fmt.Errorf("issue line has %d fields, want %d: %q", len(f), issueFields, line)
	}
	seq, err := strconv.Atoi(f[0])
	if err != nil {
		return Issue{}, fmt.Errorf("issue sequence %q is not numeric: %w", f[0], err)
	}
	return Issue{
		Seq:     seq,
		Check:   f[1],
		Pos:     f[2],
		Hash:    f[3],
		Message: f[4],
	}, nil
}

// parseAttrs reads one JSON line of the [attrs] region: an object keyed by
// element name whose value maps attribute names to counts. The flattened
// pairs are returned sorted by element then attribute, so parsed output is
// deterministic regardless of JSON key order.
func parseAttrs(line string) ([]Attr, error) {
	var grouped map[string]map[string]int
	if err := json.Unmarshal([]byte(line), &grouped); err != nil {
		return nil, fmt.Errorf("attr line is not a JSON object of counts: %q: %w", line, err)
	}
	var attrs []Attr
	for _, element := range slices.Sorted(maps.Keys(grouped)) {
		counts := grouped[element]
		for _, attribute := range slices.Sorted(maps.Keys(counts)) {
			attrs = append(attrs, Attr{Element: element, Attribute: attribute, Count: counts[attribute]})
		}
	}
	return attrs, nil
}
