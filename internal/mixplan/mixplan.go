// Package mixplan turns a track's beat grid and structure into what a DJ
// prepares before a gig: cue points on the important downbeats of every
// track, and for every transition the exact moments to start the next
// track, swap the bass and finish the blend — all on phrase boundaries.
package mixplan

import (
	"math"

	"prodjtools/internal/analysis"
)

// Cue is a hot cue on a downbeat.
type Cue struct {
	Slot  string   `json:"slot"`  // hot cue button, A–E
	Name  string   `json:"name"`  // MIX IN, DROP, BREAK, DROP 2, MIX OUT
	Bar   int      `json:"bar"`   // 1-based, counted from the first downbeat in the file
	Time  float64  `json:"time"`  // seconds from the start of the file
	Color [3]uint8 `json:"color"` // RGB, as Rekordbox stores it
}

var (
	green  = [3]uint8{40, 226, 20}
	red    = [3]uint8{230, 40, 40}
	blue   = [3]uint8{48, 90, 255}
	orange = [3]uint8{255, 140, 0}
	yellow = [3]uint8{224, 200, 0}
)

const phrase = 8 // bars

// Cues places the standard set of hot cues:
//
//	A  MIX IN   first downbeat of the music — where the track starts in a mix
//	B  DROP     the first drop
//	C  BREAK    the first breakdown after it
//	D  DROP 2   the drop after that breakdown
//	E  MIX OUT  start of the outro — where the next track comes in
//
// Cues that the track has no section for are left out.
func Cues(r analysis.Result) []Cue {
	if !r.Grid.Valid() || len(r.Sections) == 0 {
		return nil
	}
	var cues []Cue
	add := func(slot, name string, bar int, color [3]uint8) {
		cues = append(cues, Cue{slot, name, bar + 1, round3(r.Grid.BarTime(bar)), color})
	}

	add("A", "MIX IN", r.Sections[0].Bar, green)
	drops := 0
	breakSeen := false
	for _, s := range r.Sections {
		switch {
		case s.Kind == "drop" && drops == 0 && s.Bar > r.Sections[0].Bar:
			add("B", "DROP", s.Bar, red)
			drops++
		case s.Kind == "drop" && drops == 0:
			drops++ // the track opens straight on a drop: that is its MIX IN
		case s.Kind == "break" && drops == 1 && !breakSeen:
			add("C", "BREAK", s.Bar, blue)
			breakSeen = true
		case s.Kind == "drop" && breakSeen && drops == 1:
			add("D", "DROP 2", s.Bar, orange)
			drops++
		}
	}
	add("E", "MIX OUT", outroBar(r), yellow)
	return cues
}

// outroBar is where the outro starts. Tracks that end without an outro get
// a mix-out point 16 bars before the end, on a phrase boundary.
func outroBar(r analysis.Result) int {
	last := r.Sections[len(r.Sections)-1]
	if last.Kind == "outro" && len(r.Sections) > 1 {
		return last.Bar
	}
	first := r.Sections[0].Bar
	bar := first + (r.LastBar-16-first)/phrase*phrase
	return max(first, bar)
}

// firstDrop is the bar of the first drop after the start of the music, or -1.
func firstDrop(r analysis.Result) int {
	for _, s := range r.Sections[1:] {
		if s.Kind == "drop" {
			return s.Bar
		}
	}
	return -1
}

func round3(v float64) float64 { return math.Round(v*1000) / 1000 }
