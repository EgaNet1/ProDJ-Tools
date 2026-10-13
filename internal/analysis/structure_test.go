package analysis

import (
	"math"
	"slices"
	"testing"
)

func checkSections(t *testing.T, name string, got, want []Section) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("%s: sections %v, want %v", name, got, want)
	}
}

// A calm part with the kick still going is a verse; without the kick, a break.
func TestVerseAndBreak(t *testing.T) {
	parts := []part{
		{bars: 16, kick: true},                         // intro
		{bars: 32, kick: true, bass: true, pads: true}, // drop
		{bars: 16, kick: true, pads: true},             // verse: no bass line
		{bars: 16, pads: true},                         // break: no kick
		{bars: 32, kick: true, bass: true, pads: true}, // drop
		{bars: 16, kick: true},                         // outro
	}
	for _, bpm := range []float64{124, 140} {
		r := Analyze(produced(bpm, 0.25, parts))
		checkSections(t, "verse and break", r.Sections, []Section{
			{0, Intro}, {16, Drop}, {48, Verse}, {64, Break}, {80, Drop}, {112, Outro},
		})
	}
}

// A one-bar pickup before the first phrase does not shift the sections.
func TestPickup(t *testing.T) {
	parts := []part{
		{bars: 1, pads: true},                          // pickup
		{bars: 16, kick: true},                         // intro
		{bars: 32, kick: true, bass: true, pads: true}, // drop
		{bars: 16, pads: true},                         // break
		{bars: 32, kick: true, bass: true, pads: true}, // drop
		{bars: 16, kick: true},                         // outro
	}
	r := Analyze(produced(128, 0.4, parts))
	checkSections(t, "pickup", r.Sections, []Section{
		{1, Intro}, {17, Drop}, {49, Break}, {65, Drop}, {97, Outro},
	})
}

// A hip-hop track at 75 BPM is gridded at 150; read at 75, its bars and
// sections line up with the real bars, starting on the real downbeat.
func TestHalfTime(t *testing.T) {
	const silence = 0.31
	parts := []part{
		{bars: 8, kick: true},                          // intro
		{bars: 16, kick: true, bass: true, pads: true}, // hook
		{bars: 16, pads: true},                         // break
		{bars: 16, kick: true, bass: true, pads: true}, // hook
		{bars: 8, kick: true},                          // outro
	}
	r := Analyze(produced(75, silence, parts))
	if math.Abs(r.Grid.BPM-150) > 0.01 {
		t.Fatalf("grid at %.3f BPM, want 150", r.Grid.BPM)
	}
	m := r.InMeter(70)
	if math.Abs(m.Grid.BPM-75) > 0.01 {
		t.Errorf("read at %.3f BPM, want 75", m.Grid.BPM)
	}
	if d := math.Abs(m.Grid.FirstBeat - silence); d > 0.002 {
		t.Errorf("first real downbeat at %.4f s, want %.4f", m.Grid.FirstBeat, silence)
	}
	checkSections(t, "half time", m.Sections, []Section{{0, Intro}, {8, Drop}, {24, Break}, {40, Drop}, {56, Outro}})
	if m.LastBar != 64 {
		t.Errorf("last bar %d, want 64", m.LastBar)
	}
	// Read in the usual range, nothing changes.
	if n := r.InMeter(88); n.Grid != r.Grid || !slices.Equal(n.Sections, r.Sections) {
		t.Errorf("InMeter(88) changed the result")
	}
}

func TestDoubleTime(t *testing.T) {
	r := Result{
		Grid:     Grid{BPM: 95, FirstBeat: 0.2},
		Bars:     []uint8{1, 2},
		Sections: []Section{{0, Intro}, {8, Drop}},
		LastBar:  40,
	}
	m := r.InMeter(100) // 95 BPM read as 190
	if m.Grid.BPM != 190 || m.Grid.FirstBeat != 0.2 || m.LastBar != 80 {
		t.Errorf("grid %+v, last bar %d", m.Grid, m.LastBar)
	}
	checkSections(t, "double time", m.Sections, []Section{{0, Intro}, {16, Drop}})
	if !slices.Equal(m.Bars, []uint8{1, 1, 2, 2}) {
		t.Errorf("bars %v", m.Bars)
	}
}
