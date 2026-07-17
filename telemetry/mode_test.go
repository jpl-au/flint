package telemetry

import (
	"os"
	"path/filepath"
	"testing"
)

func TestModeRoundTrip(t *testing.T) {
	for _, mode := range []Mode{Off, Local, On} {
		t.Run(mode.String(), func(t *testing.T) {
			dir := t.TempDir()
			if err := SetModeIn(dir, mode); err != nil {
				t.Fatalf("SetModeIn: %v", err)
			}
			got, err := CurrentModeIn(dir)
			if err != nil {
				t.Fatalf("CurrentModeIn: %v", err)
			}
			if got != mode {
				t.Errorf("round-trip mode = %v, want %v", got, mode)
			}
		})
	}
}

func TestCurrentModeInMissingFileIsOff(t *testing.T) {
	got, err := CurrentModeIn(t.TempDir())
	if err != nil {
		t.Fatalf("CurrentModeIn: %v", err)
	}
	if got != Off {
		t.Errorf("missing mode file = %v, want %v", got, Off)
	}
}

func TestCurrentModeInRejectsUnknownValue(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, modeFile), []byte("sideways\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := CurrentModeIn(dir); err == nil {
		t.Fatal("CurrentModeIn on an unknown value: expected an error")
	}
}

func TestParseModeRejectsUnknownValue(t *testing.T) {
	if _, err := ParseMode("sideways"); err == nil {
		t.Fatal("ParseMode on an unknown value: expected an error")
	}
}
