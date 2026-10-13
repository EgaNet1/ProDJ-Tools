// Package mixplan turns a track's beat grid and structure into what a DJ
// prepares before a gig: cue points on the important downbeats of every
// track, and for every transition the exact moments to start the next
// track, swap the bass and finish the blend — all on phrase boundaries.
//
// Results passed in should already be in the DJ's meter (analysis.InMeter),
// so that bars here are the bars the DJ counts.
package mixplan

import (
	"math"

	"prodjtools/internal/analysis"
)

// Cue is a hot cue on a downbeat.
type Cue struct {
	Slot  string   `json:"slot"`  // hot cue button, A–F
	Name  string   `json:"name"`  // as Rekordbox shows it: MIX IN, DROP, EARLY OUT, BREAK, DROP 2, MIX OUT
	Role  string   `json:"role"`  // what it is for, for the interface: mixIn, drop, earlyOut, break, drop2, mixOut
	Bar   int      `json:"bar"`   // 1-based, counted from the first downbeat in the file
	Time  float64  `json:"time"`  // seconds from the start of the file
	Color [3]uint8 `json:"color"` // RGB, as Rekordbox stores it
}

// Hot is the number of the cue's hot cue button: 0 for A, 1 for B…
func (c Cue) Hot() int { return int(c.Slot[0] - 'A') }

var (
	green  = [3]uint8{40, 226, 20}
	red    = [3]uint8{230, 40, 40}
	cyan   = [3]uint8{16, 196, 230}
	blue   = [3]uint8{48, 90, 255}
	orange = [3]uint8{255, 140, 0}
	yellow = [3]uint8{224, 200, 0}
)

const phrase = 8 // bars

// layout is where the parts of a track that matter for mixing start, in
// bars from the first downbeat (0-based); -1 where the track has no such
// part.
type layout struct {
	mixIn    int // the first phrase of music
	drop     int // the first drop, unless the track opens on it
	earlyOut int // the calm part right after the first drop
	brk      int // a breakdown after that calm part, if it is a separate section
	drop2    int // the drop that follows
	mixOut   int // the start of the outro
}

func layoutOf(r analysis.Result) layout {
	l := layout{mixIn: -1, drop: -1, earlyOut: -1, brk: -1, drop2: -1, mixOut: -1}
	s := r.Sections
	if len(s) == 0 {
		return l
	}
	l.mixIn = s[0].Bar
	first := -1 // index of the first drop
	for i, sec := range s {
		if sec.Kind == analysis.Drop {
			first = i
			break
		}
	}
	if first > 0 {
		l.drop = s[first].Bar
	}
	if first >= 0 && first+1 < len(s) {
		calm := s[first+1]
		if calm.Kind == analysis.Verse || calm.Kind == analysis.Break {
			l.earlyOut = calm.Bar
			for _, sec := range s[first+2:] {
				if sec.Kind == analysis.Drop {
					l.drop2 = sec.Bar
					break
				}
				if sec.Kind == analysis.Break && calm.Kind == analysis.Verse && l.brk < 0 {
					l.brk = sec.Bar
				}
			}
		}
	}
	l.mixOut = outroBar(r)
	if l.mixOut <= l.earlyOut {
		l.earlyOut, l.brk, l.drop2 = -1, -1, -1 // the calm part is the outro itself
	}
	return l
}

// Cues places the hot cues of a track, in the order they come:
//
//	A  MIX IN     the first phrase of music: start the track here
//	B  DROP       the first drop
//	C  EARLY OUT  the calm part (verse or break) right after the first drop:
//	              the next track can come in here, without waiting for the outro
//	D  BREAK      the breakdown before the second drop, when it follows a verse
//	E  DROP 2     the drop after the calm part
//	F  MIX OUT    the start of the outro: the usual place to bring the next track in
//
// Cues that the track has no part for are left out; the others keep their
// letters, so a button always means the same thing.
func Cues(r analysis.Result) []Cue {
	if !r.Grid.Valid() || len(r.Sections) == 0 {
		return nil
	}
	l := layoutOf(r)
	var cues []Cue
	add := func(slot, name, role string, bar int, color [3]uint8) {
		if bar >= 0 {
			cues = append(cues, Cue{slot, name, role, bar + 1, round3(r.Grid.BarTime(bar)), color})
		}
	}
	add("A", "MIX IN", "mixIn", l.mixIn, green)
	add("B", "DROP", "drop", l.drop, red)
	add("C", "EARLY OUT", "earlyOut", l.earlyOut, cyan)
	add("D", "BREAK", "break", l.brk, blue)
	add("E", "DROP 2", "drop2", l.drop2, orange)
	add("F", "MIX OUT", "mixOut", l.mixOut, yellow)
	return cues
}

// outroBar is where the outro starts. Tracks that end without an outro get
// a mix-out point 16 bars before the end, on a phrase boundary.
func outroBar(r analysis.Result) int {
	last := r.Sections[len(r.Sections)-1]
	if last.Kind == analysis.Outro && len(r.Sections) > 1 {
		return last.Bar
	}
	first := r.Sections[0].Bar
	bar := first + (r.LastBar-16-first)/phrase*phrase
	return max(first, bar)
}

// firstDrop is the bar of the first drop after the start of the music, or -1.
func firstDrop(r analysis.Result) int {
	for _, s := range r.Sections[1:] {
		if s.Kind == analysis.Drop {
			return s.Bar
		}
	}
	return -1
}

func round3(v float64) float64 { return math.Round(v*1000) / 1000 }
