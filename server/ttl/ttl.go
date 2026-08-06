package ttl

import (
	"errors"
	"time"
)

// TTL bounds (D4). The lower bound avoids misclicks; the upper bound caps
// custom TTLs at "1 year".
const (
	MinTTL = time.Minute
	MaxTTL = 365 * 24 * time.Hour
)

// Preset is a named TTL shortcut offered in the webapp dropdown and slash command.
type Preset struct {
	Label    string
	Duration time.Duration
}

// Presets are the offered TTL shortcuts (D4). Custom values within
// [MinTTL, MaxTTL] are also allowed.
var Presets = []Preset{
	{"5m", 5 * time.Minute},
	{"1h", time.Hour},
	{"8h", 8 * time.Hour},
	{"1d", 24 * time.Hour},
	{"1w", 7 * 24 * time.Hour},
	{"1mo", 30 * 24 * time.Hour},
}

// ErrInvalidTTL is returned when a TTL falls outside [MinTTL, MaxTTL].
var ErrInvalidTTL = errors.New("ttl: duration must be between 1m and 1y")

// ValidateTTL returns ErrInvalidTTL if d is outside [MinTTL, MaxTTL].
func ValidateTTL(d time.Duration) error {
	if d < MinTTL || d > MaxTTL {
		return ErrInvalidTTL
	}
	return nil
}

// PresetForLabel returns the preset whose label matches, or (zero, false).
func PresetForLabel(label string) (Preset, bool) {
	for _, p := range Presets {
		if p.Label == label {
			return p, true
		}
	}
	return Preset{}, false
}
