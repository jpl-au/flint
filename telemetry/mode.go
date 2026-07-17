package telemetry

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// modeFile is the name of the file that persists the telemetry mode beneath the
// user config directory.
const modeFile = "mode"

// Mode selects how much telemetry a flint run collects.
type Mode int

const (
	// Off collects nothing. It is the default when no mode file exists.
	Off Mode = iota

	// Local collects telemetry to local .tlf files only.
	Local

	// On collects telemetry and additionally uploads it. Uploading does not
	// exist yet, so On behaves exactly like Local for now; the stored value
	// reserves the "collect and upload" meaning for when upload lands.
	On
)

// String returns the mode's lowercase name, which is the form written to the
// mode file.
func (m Mode) String() string {
	switch m {
	case Local:
		return "local"
	case On:
		return "on"
	default:
		return "off"
	}
}

// ParseMode converts a mode name ("off", "local" or "on") to a Mode, rejecting
// any other string with an error naming the valid values.
func ParseMode(s string) (Mode, error) {
	switch s {
	case "off":
		return Off, nil
	case "local":
		return Local, nil
	case "on":
		return On, nil
	default:
		return Off, fmt.Errorf("unknown telemetry mode %q: want off, local or on", s)
	}
}

// ConfigDir returns the directory beneath the user config directory (see
// os.UserConfigDir) where flint persists its telemetry mode. Config, not cache,
// so wiping the cache does not lose the choice.
func ConfigDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolving user config directory: %w", err)
	}
	return filepath.Join(base, "flint"), nil
}

// CurrentMode returns the persisted telemetry mode, or Off when no mode file
// exists, reading from the user config directory.
func CurrentMode() (Mode, error) {
	dir, err := ConfigDir()
	if err != nil {
		return Off, err
	}
	return CurrentModeIn(dir)
}

// CurrentModeIn is CurrentMode with an explicit config directory in place of the
// user config directory. It exists so callers, and tests, can direct the mode
// file at a chosen location.
func CurrentModeIn(dir string) (Mode, error) {
	data, err := os.ReadFile(filepath.Join(dir, modeFile))
	if errors.Is(err, os.ErrNotExist) {
		return Off, nil
	}
	if err != nil {
		return Off, fmt.Errorf("reading telemetry mode file: %w", err)
	}
	return ParseMode(strings.TrimSpace(string(data)))
}

// SetMode persists mode to the mode file under the user config directory,
// creating the directory if needed.
func SetMode(mode Mode) error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}
	return SetModeIn(dir, mode)
}

// SetModeIn is SetMode with an explicit config directory in place of the user
// config directory.
func SetModeIn(dir string, mode Mode) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating telemetry config directory %q: %w", dir, err)
	}
	path := filepath.Join(dir, modeFile)
	if err := os.WriteFile(path, []byte(mode.String()+"\n"), 0o644); err != nil {
		return fmt.Errorf("writing telemetry mode file %q: %w", path, err)
	}
	return nil
}
