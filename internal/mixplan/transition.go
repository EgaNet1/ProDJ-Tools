package mixplan

import (
	"fmt"
	"math"

	"prodjtools/internal/analysis"
	"prodjtools/internal/i18n"
)

// Moment is one point in a track.
type Moment struct {
	Bar  int     `json:"bar"`  // 1-based
	Time float64 `json:"time"` // seconds from the start of the file
}

// Transition is a phrase-matched blend from track A into track B. Track B
// is played at A's tempo, so bars of both tracks run side by side: B's
// intro plays under A's outro and B's first drop lands exactly on a phrase
// boundary of A, which is where the basslines are swapped.
type Transition struct {
	Bars   int     `json:"bars"`   // length of the blend, start of B to the bass swap
	Pitch  float64 `json:"pitch"`  // percent to move B's tempo to match A
	StartA Moment  `json:"startA"` // in A: start B here…
	CueB   Moment  `json:"cueB"`   // …from this point of B (its MIX IN)
	SwapA  Moment  `json:"swapA"`  // in A: swap the bass here,
	SwapB  Moment  `json:"swapB"`  // which in B is this moment (usually its drop)
	EndA   Moment  `json:"endA"`   // in A: A is gone by here
	Note   string  `json:"note,omitempty"`
}

// Plan works out the transition from a into b. It reports false when
// either track has no usable grid or structure.
func Plan(a, b analysis.Result) (Transition, bool) {
	if !a.Grid.Valid() || !b.Grid.Valid() || len(a.Sections) == 0 || len(b.Sections) == 0 {
		return Transition{}, false
	}

	// The blend lasts as long as B's intro, rounded to whole phrases (one
	// to four), so that B's drop arrives at the swap.
	mixIn := b.Sections[0].Bar
	blend := 2 * phrase
	if drop := firstDrop(b); drop > mixIn {
		intro := drop - mixIn
		switch {
		case intro < phrase:
			blend = max(4, intro)
		default:
			blend = min(4*phrase, intro/phrase*phrase)
		}
	}

	// B starts when A's outro starts and drops while the outro still runs.
	// If the outro is shorter than B's intro, B starts earlier, under A's
	// last drop, so that B's drop replaces A's outro and the energy on the
	// floor does not sag.
	firstA := a.Sections[0].Bar
	start := outroBar(a)
	swap := start + blend
	if a.LastBar-start < blend {
		swap = start
		start = max(firstA, swap-blend)
		blend = swap - start
	}
	end := min(a.LastBar, swap+phrase)

	t := Transition{
		Bars:   blend,
		Pitch:  math.Round((a.Grid.BPM/b.Grid.BPM-1)*1000) / 10,
		StartA: moment(a.Grid, start),
		CueB:   moment(b.Grid, mixIn),
		SwapA:  moment(a.Grid, swap),
		SwapB:  moment(b.Grid, mixIn+blend),
		EndA:   moment(a.Grid, end),
	}
	switch p := math.Abs(t.Pitch); {
	case p > 8:
		t.Note = i18n.T("note.pitch_far", fmt.Sprintf("%.1f", p))
	case p > 4:
		t.Note = i18n.T("note.pitch_near", fmt.Sprintf("%.1f", p))
	}
	return t, true
}

func moment(g analysis.Grid, bar int) Moment {
	return Moment{Bar: bar + 1, Time: round3(g.BarTime(bar))}
}
