// Package camelot models musical keys on the Camelot wheel, the notation DJs
// use for harmonic mixing: twelve numbered positions, "A" for minor keys and
// "B" for major. Neighbouring positions share most of their notes, so tracks
// sitting next to each other on the wheel can be layered without clashing.
package camelot

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Key is a position on the wheel. The zero value means "unknown".
type Key struct {
	Number int  // 1–12
	Minor  bool // inner ring, written "A"
}

var (
	majorNames = [12]string{"C", "Db", "D", "Eb", "E", "F", "F#", "G", "Ab", "A", "Bb", "B"}
	minorNames = [12]string{"Cm", "C#m", "Dm", "Ebm", "Em", "Fm", "F#m", "Gm", "G#m", "Am", "Bbm", "Bm"}
)

// FromPitch returns the key whose tonic is the given pitch class
// (0 = C, 1 = C#, … 11 = B).
func FromPitch(pitch int, minor bool) Key {
	major := pitch
	if minor {
		major = (pitch + 3) % 12 // a minor key shares its number with its relative major
	}
	// Each step clockwise is a fifth up (7 semitones); C major sits at 8.
	return Key{Number: (major*7+7)%12 + 1, Minor: minor}
}

// Valid reports whether the key is known.
func (k Key) Valid() bool { return k.Number >= 1 && k.Number <= 12 }

// tonic is the inverse of FromPitch.
func (k Key) tonic() int {
	major := ((k.Number-8)*7%12 + 12) % 12 // 7 is its own inverse modulo 12
	if k.Minor {
		return (major + 9) % 12
	}
	return major
}

// Name returns the key in musical notation, such as "Am" or "F#".
func (k Key) Name() string {
	switch {
	case !k.Valid():
		return ""
	case k.Minor:
		return minorNames[k.tonic()]
	default:
		return majorNames[k.tonic()]
	}
}

// String returns the Camelot code, such as "8A".
func (k Key) String() string {
	if !k.Valid() {
		return ""
	}
	if k.Minor {
		return strconv.Itoa(k.Number) + "A"
	}
	return strconv.Itoa(k.Number) + "B"
}

// Parse reads a Camelot code such as "8A" or "12b".
func Parse(s string) (Key, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "" {
		return Key{}, nil
	}
	n, err := strconv.Atoi(s[:len(s)-1])
	ring := s[len(s)-1]
	if err != nil || n < 1 || n > 12 || (ring != 'A' && ring != 'B') {
		return Key{}, fmt.Errorf("camelot: bad key %q", s)
	}
	return Key{Number: n, Minor: ring == 'A'}, nil
}

func (k Key) MarshalJSON() ([]byte, error) { return json.Marshal(k.String()) }

func (k *Key) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	parsed, err := Parse(s)
	*k = parsed
	return err
}
