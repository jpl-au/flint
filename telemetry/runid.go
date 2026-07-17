package telemetry

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const (
	// crockford is the Crockford base32 alphabet: the decimal digits and the
	// uppercase letters excluding I, L, O and U. It is ordered by ascending
	// byte value, so a fixed-width encoding sorts lexicographically in the
	// same order as the numbers it represents.
	crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

	runTimeLen  = 7                 // Crockford characters encoding the Unix time
	runIDLen    = runTimeLen + 1    // plus one collision character
	seqLen      = 2                 // digits of the issue sequence number
	issueRefLen = runIDLen + seqLen // total characters in an issue reference
)

// timeCode encodes sec as runTimeLen Crockford base32 characters, zero-padded
// and most-significant first. The fixed width and ascending alphabet make the
// result sort lexicographically in chronological order.
func timeCode(sec int64) string {
	var b [runTimeLen]byte
	v := uint64(sec)
	for i := runTimeLen - 1; i >= 0; i-- {
		b[i] = crockford[v&0x1f]
		v >>= 5
	}
	// Values above 2^35 would need an eighth character; their high bits are
	// dropped here, which cannot happen until roughly the year 3000.
	return string(b[:])
}

// allocRunID returns a run ID for time t whose file does not yet exist in dir.
// The default collision character is '0'; on a clash it is bumped through the
// Crockford alphabet, so IDs allocated in the same second still sort in
// allocation order. It errors only if every collision character is taken.
func allocRunID(dir string, t time.Time) (string, error) {
	prefix := timeCode(t.Unix())
	for i := range len(crockford) {
		id := prefix + crockford[i:i+1]
		_, err := os.Stat(filepath.Join(dir, id+fileExt))
		switch {
		case errors.Is(err, os.ErrNotExist):
			return id, nil
		case err != nil:
			return "", fmt.Errorf("checking telemetry file for run %s: %w", id, err)
		}
		// err == nil: the file exists, so bump the collision character.
	}
	return "", fmt.Errorf("run ID collision space exhausted for time code %s", prefix)
}

// IssueRef builds the reference for the seq-th issue of the run identified by
// runID: the eight-character run ID followed by the sequence number as two
// zero-padded digits. seq must be between 1 and 99; RecordIssue enforces that
// bound.
func IssueRef(runID string, seq int) string {
	return fmt.Sprintf("%s%02d", runID, seq)
}

// ParseIssueRef splits a ten-character issue reference into its run ID and
// sequence number. Parsing is purely positional: the first eight characters
// are the run ID and the final two are the zero-padded sequence.
func ParseIssueRef(ref string) (runID string, seq int, err error) {
	if len(ref) != issueRefLen {
		return "", 0, fmt.Errorf("issue reference %q: want %d characters, got %d", ref, issueRefLen, len(ref))
	}
	runID = ref[:runIDLen]
	seq, err = strconv.Atoi(ref[runIDLen:])
	if err != nil {
		return "", 0, fmt.Errorf("issue reference %q: sequence %q is not numeric: %w", ref, ref[runIDLen:], err)
	}
	return runID, seq, nil
}
