package telemetry

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTimeCode(t *testing.T) {
	tests := []struct {
		sec  int64
		want string
	}{
		{0, "0000000"},
		{1, "0000001"},
		{31, "000000Z"},
		{32, "0000010"},
		{33, "0000011"},
	}
	for _, tt := range tests {
		got := timeCode(tt.sec)
		if len(got) != runTimeLen {
			t.Errorf("timeCode(%d) length = %d, want %d", tt.sec, len(got), runTimeLen)
		}
		if got != tt.want {
			t.Errorf("timeCode(%d) = %q, want %q", tt.sec, got, tt.want)
		}
	}
}

func TestTimeCodeOrderingMatchesTime(t *testing.T) {
	// Later timestamps must produce lexicographically greater codes, so run
	// files sort chronologically.
	secs := []int64{0, 1, 100, 1000, 1_000_000, 1_700_000_000, 1_752_000_000, 1_800_000_000}
	for i := 1; i < len(secs); i++ {
		lo, hi := timeCode(secs[i-1]), timeCode(secs[i])
		if lo >= hi {
			t.Errorf("timeCode(%d)=%q should sort before timeCode(%d)=%q", secs[i-1], lo, secs[i], hi)
		}
	}
}

func TestAllocRunIDCollision(t *testing.T) {
	dir := t.TempDir()
	at := time.Unix(1_752_000_000, 0)
	prefix := timeCode(at.Unix())

	// With an empty directory the run takes the default collision character.
	id, err := allocRunID(dir, at)
	if err != nil {
		t.Fatalf("allocRunID: %v", err)
	}
	if want := prefix + "0"; id != want {
		t.Fatalf("first run ID = %q, want %q", id, want)
	}

	// A file for that ID present, the next allocation bumps to '1'.
	touch(t, dir, prefix+"0")
	id, err = allocRunID(dir, at)
	if err != nil {
		t.Fatalf("allocRunID after one collision: %v", err)
	}
	if want := prefix + "1"; id != want {
		t.Fatalf("second run ID = %q, want %q", id, want)
	}

	// Two files present, the allocation bumps to '2'.
	touch(t, dir, prefix+"1")
	id, err = allocRunID(dir, at)
	if err != nil {
		t.Fatalf("allocRunID after two collisions: %v", err)
	}
	if want := prefix + "2"; id != want {
		t.Fatalf("third run ID = %q, want %q", id, want)
	}
}

func TestAllocRunIDExhausted(t *testing.T) {
	dir := t.TempDir()
	at := time.Unix(1_752_000_000, 0)
	prefix := timeCode(at.Unix())
	for _, c := range crockford {
		touch(t, dir, prefix+string(c))
	}
	if _, err := allocRunID(dir, at); err == nil {
		t.Fatal("allocRunID: expected an error when every collision character is taken")
	}
}

func TestIssueRefRoundTrip(t *testing.T) {
	// The run IDs here are arbitrary ten-character shapes: ParseIssueRef is
	// purely positional and does not validate the alphabet.
	tests := []struct {
		runID string
		seq   int
	}{
		{"0SVLQ2K0", 1},
		{"0SVLQ2K0", 7},
		{"0SVLQ2K0", 99},
		{"ZZZZZZZ0", 42},
	}
	for _, tt := range tests {
		ref := IssueRef(tt.runID, tt.seq)
		if len(ref) != issueRefLen {
			t.Errorf("IssueRef(%q, %d) length = %d, want %d", tt.runID, tt.seq, len(ref), issueRefLen)
		}
		gotID, gotSeq, err := ParseIssueRef(ref)
		if err != nil {
			t.Fatalf("ParseIssueRef(%q): %v", ref, err)
		}
		if gotID != tt.runID || gotSeq != tt.seq {
			t.Errorf("round-trip of %q,%d via %q = %q,%d", tt.runID, tt.seq, ref, gotID, gotSeq)
		}
	}
}

func TestParseIssueRefErrors(t *testing.T) {
	for _, ref := range []string{"", "0SVLQ2K0", "0SVLQ2K0XX", "0SVLQ2K012345"} {
		if _, _, err := ParseIssueRef(ref); err == nil {
			t.Errorf("ParseIssueRef(%q): expected an error", ref)
		}
	}
}

// touch creates an empty telemetry file for the given run ID in dir.
func touch(t *testing.T, dir, id string) {
	t.Helper()
	path := filepath.Join(dir, id+fileExt)
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("creating %q: %v", path, err)
	}
}
