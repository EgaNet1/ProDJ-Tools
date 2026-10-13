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

// The ways of mixing one track into the next.
const (
	// ViaOutro is the classic blend: B comes in over A's outro and drops
	// as the outro ends.
	ViaOutro = "outro"
	// Early leaves A after its first drop: B comes in over the calm part
	// that follows (cue C, EARLY OUT) and its drop takes over before A's
	// second drop. Shorter sets, more energy, A's second half is skipped.
	Early = "early"
)

// Transition is a phrase-matched blend from track A into track B. Track B
// is played at A's tempo, so bars of both tracks run side by side: B's
// intro plays under A, and B's first drop lands exactly on a phrase
// boundary of A, which is where the basslines are swapped.
type Transition struct {
	Kind   string  `json:"kind"`   // ViaOutro or Early
	Bars   int     `json:"bars"`   // length of the blend, start of B to the bass swap
	Pitch  float64 `json:"pitch"`  // percent to move B's tempo to match A
	StartA Moment  `json:"startA"` // in A: start B here…
	CueB   Moment  `json:"cueB"`   // …from this point of B
	FromB  string  `json:"fromB"`  // the cue of B that CueB is on, if any (usually MIX IN)
	SwapA  Moment  `json:"swapA"`  // in A: swap the bass here,
	SwapB  Moment  `json:"swapB"`  // which in B is this moment (usually its drop)
	EndA   Moment  `json:"endA"`   // in A: A is gone by here
	Note   string  `json:"note,omitempty"`
}

// intro is where B comes in and how long its intro lasts until the drop,
// rounded to whole phrases (one to four) so that the drop lands on the
// swap. A track without a clear drop gets two phrases.
func intro(b analysis.Result) (mixIn, blend int) {
	mixIn = b.Sections[0].Bar
	blend = 2 * phrase
	if drop := firstDrop(b); drop > mixIn {
		switch n := drop - mixIn; {
		case n < phrase:
			blend = max(4, n)
		default:
			blend = min(4*phrase, n/phrase*phrase)
		}
	}
	return mixIn, blend
}

func usable(a, b analysis.Result) bool {
	return a.Grid.Valid() && b.Grid.Valid() && len(a.Sections) > 0 && len(b.Sections) > 0
}

// Plan works out the classic transition from a into b, over a's outro. It
// reports false when either track has no usable grid or structure.
func Plan(a, b analysis.Result) (Transition, bool) {
	if !usable(a, b) {
		return Transition{}, false
	}
	mixIn, blend := intro(b)

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
	return finish(a, b, ViaOutro, start, mixIn, swap, mixIn+blend, end), true
}

// EarlyPlan works out the transition that leaves a after its first drop:
// B starts at a's EARLY OUT cue and B's drop takes over before a's second
// drop would come. It reports false when a has no calm part after its
// first drop, or when that part is too short to mix over.
func EarlyPlan(a, b analysis.Result) (Transition, bool) {
	if !usable(a, b) {
		return Transition{}, false
	}
	l := layoutOf(a)
	if l.earlyOut < 0 {
		return Transition{}, false
	}
	// A must be gone before its energy comes back.
	until := a.LastBar
	switch {
	case l.drop2 > l.earlyOut:
		until = l.drop2
	case l.mixOut > l.earlyOut:
		until = l.mixOut
	}
	room := until - l.earlyOut
	if room < 4 {
		return Transition{}, false
	}

	mixIn, blend := intro(b)
	dropB := mixIn + blend
	if room >= blend {
		// B's whole intro fits into the calm part; A fades out under B's drop.
		swap := l.earlyOut + blend
		return finish(a, b, Early, l.earlyOut, mixIn, swap, dropB, min(swap+phrase, until)), true
	}
	// The calm part is shorter than B's intro: B starts later in its intro,
	// so that its drop lands exactly where A's second drop would have come.
	return finish(a, b, Early, l.earlyOut, dropB-room, until, dropB, until), true
}

// finish fills in a transition from bars (0-based) of both tracks.
func finish(a, b analysis.Result, kind string, startA, cueB, swapA, swapB, endA int) Transition {
	t := Transition{
		Kind:   kind,
		Bars:   swapA - startA,
		Pitch:  math.Round((foldRatio(a.Grid.BPM/b.Grid.BPM)-1)*1000) / 10,
		StartA: moment(a.Grid, startA),
		CueB:   moment(b.Grid, cueB),
		SwapA:  moment(a.Grid, swapA),
		SwapB:  moment(b.Grid, swapB),
		EndA:   moment(a.Grid, endA),
	}
	for _, c := range Cues(b) {
		if c.Bar == cueB+1 {
			t.FromB = c.Slot + " · " + c.Name
			break
		}
	}
	switch p := math.Abs(t.Pitch); {
	case p > 8:
		t.Note = i18n.T("note.pitch_far", fmt.Sprintf("%.1f", p))
	case p > 4:
		t.Note = i18n.T("note.pitch_near", fmt.Sprintf("%.1f", p))
	}
	return t
}

// foldRatio brings a tempo ratio into [0.75, 1.5): two tracks read in
// different octaves (a 75 BPM verse into a 140 BPM drop) are matched at the
// closest real speed.
func foldRatio(r float64) float64 {
	for r < 0.75 {
		r *= 2
	}
	for r >= 1.5 {
		r /= 2
	}
	return r
}

func moment(g analysis.Grid, bar int) Moment {
	return Moment{Bar: bar + 1, Time: round3(g.BarTime(bar))}
}
