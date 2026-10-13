package mixplan

import (
	"strings"
	"testing"

	"prodjtools/internal/analysis"
)

// track builds a result with a 140 BPM grid (bar = 12/7 s) starting at 0.5 s.
func track(lastBar int, sections ...analysis.Section) analysis.Result {
	return analysis.Result{
		Grid:     analysis.Grid{BPM: 140, FirstBeat: 0.5},
		Sections: sections,
		LastBar:  lastBar,
	}
}

func sec(bar int, kind string) analysis.Section { return analysis.Section{Bar: bar, Kind: kind} }

// classic is intro, drop, break, drop, outro.
func classic() analysis.Result {
	return track(112, sec(0, analysis.Intro), sec(16, analysis.Drop), sec(48, analysis.Break), sec(64, analysis.Drop), sec(96, analysis.Outro))
}

type wantCue struct {
	slot string
	bar  int
}

func checkCues(t *testing.T, r analysis.Result, want []wantCue) []Cue {
	t.Helper()
	cues := Cues(r)
	if len(cues) != len(want) {
		t.Fatalf("cues %+v, want %v", cues, want)
	}
	for i, w := range want {
		if cues[i].Slot != w.slot || cues[i].Bar != w.bar {
			t.Errorf("cue %d = %s bar %d, want %s bar %d", i, cues[i].Slot, cues[i].Bar, w.slot, w.bar)
		}
	}
	return cues
}

func TestCues(t *testing.T) {
	// The break right after the first drop is the early way out; there is
	// no separate breakdown.
	cues := checkCues(t, classic(), []wantCue{{"A", 1}, {"B", 17}, {"C", 49}, {"E", 65}, {"F", 97}})
	// Bar 17 (index 16) starts 16 bars × 12/7 s after the first downbeat.
	if got, want := cues[1].Time, round3(0.5+16*12.0/7); got != want {
		t.Errorf("DROP at %.3f, want %.3f", got, want)
	}
	if cues[2].Name != "EARLY OUT" || cues[4].Hot() != 5 {
		t.Errorf("cues %+v", cues)
	}
}

func TestCuesVerseThenBreak(t *testing.T) {
	r := track(128, sec(0, analysis.Intro), sec(16, analysis.Drop), sec(48, analysis.Verse),
		sec(64, analysis.Break), sec(80, analysis.Drop), sec(112, analysis.Outro))
	checkCues(t, r, []wantCue{{"A", 1}, {"B", 17}, {"C", 49}, {"D", 65}, {"E", 81}, {"F", 113}})
}

func TestCuesWithoutOutro(t *testing.T) {
	r := track(100, sec(2, analysis.Intro), sec(18, analysis.Drop))
	cues := Cues(r)
	last := cues[len(cues)-1]
	// 16 bars before the end, on a phrase counted from the first bar of music.
	if last.Name != "MIX OUT" || last.Bar != 83 {
		t.Errorf("mix out %+v, want bar 83", last)
	}
}

func TestPlan(t *testing.T) {
	a := track(112, sec(0, analysis.Intro), sec(16, analysis.Drop), sec(96, analysis.Outro))
	b := track(112, sec(0, analysis.Intro), sec(16, analysis.Drop))
	b.Grid.BPM = 145
	p, ok := Plan(a, b)
	if !ok {
		t.Fatal("no plan")
	}
	if p.Kind != ViaOutro || p.Bars != 16 || p.StartA.Bar != 97 || p.SwapA.Bar != 113 || p.SwapB.Bar != 17 {
		t.Errorf("plan %+v", p)
	}
	// A's outro is exactly 16 bars, so A runs out at its last bar.
	if p.EndA.Bar != 113 {
		t.Errorf("A ends at bar %d, want 113", p.EndA.Bar)
	}
	if p.Pitch != -3.4 {
		t.Errorf("pitch %v, want -3.4", p.Pitch)
	}
	if p.FromB != "A · MIX IN" {
		t.Errorf("B starts from %q", p.FromB)
	}
	// A has no calm part after its drop: no early way out.
	if _, ok := EarlyPlan(a, b); ok {
		t.Error("early plan without a calm part")
	}
}

func TestPlanShortOutro(t *testing.T) {
	// A has only 8 bars of outro; B needs 32 bars of intro.
	a := track(104, sec(0, analysis.Intro), sec(16, analysis.Drop), sec(96, analysis.Outro))
	b := track(160, sec(0, analysis.Intro), sec(32, analysis.Drop))
	p, _ := Plan(a, b)
	if p.Bars != 32 || p.StartA.Bar != 65 || p.SwapA.Bar != 97 {
		t.Errorf("plan %+v", p)
	}
}

func TestEarlyPlan(t *testing.T) {
	// The break after A's first drop is 16 bars; B's intro is 16 bars, so
	// B's drop lands exactly where A's second drop would have come.
	b := track(112, sec(0, analysis.Intro), sec(16, analysis.Drop))
	p, ok := EarlyPlan(classic(), b)
	if !ok {
		t.Fatal("no early plan")
	}
	if p.Kind != Early || p.StartA.Bar != 49 || p.CueB.Bar != 1 || p.SwapA.Bar != 65 || p.SwapB.Bar != 17 || p.EndA.Bar != 65 {
		t.Errorf("plan %+v", p)
	}
}

func TestEarlyPlanLongIntro(t *testing.T) {
	// B's intro is 32 bars but A's break only 16: B starts halfway into its
	// intro so that its drop still replaces A's second drop.
	b := track(160, sec(0, analysis.Intro), sec(32, analysis.Drop))
	p, ok := EarlyPlan(classic(), b)
	if !ok {
		t.Fatal("no early plan")
	}
	if p.Bars != 16 || p.StartA.Bar != 49 || p.CueB.Bar != 17 || p.SwapB.Bar != 33 || p.SwapA.Bar != 65 || p.FromB != "" {
		t.Errorf("plan %+v", p)
	}
}

func TestEarlyPlanShortIntro(t *testing.T) {
	// B drops after 8 bars; A's 16-bar break leaves time for A to fade out
	// under B's drop before A's own drop would return.
	b := track(112, sec(0, analysis.Intro), sec(8, analysis.Drop))
	p, _ := EarlyPlan(classic(), b)
	if p.Bars != 8 || p.SwapA.Bar != 57 || p.EndA.Bar != 65 {
		t.Errorf("plan %+v", p)
	}
}

func TestSheet(t *testing.T) {
	text := Sheet("TEST", []Entry{{"X", "One", classic()}, {"Y", "Two", classic()}})
	for _, want := range []string{"01. X — One", "A  MIX IN", "C  EARLY OUT", "0:00.500", "01 → 02   One → Two", "bar 97", "[Via the outro]", "[Early: after the first drop]"} {
		if !strings.Contains(text, want) {
			t.Errorf("sheet lacks %q:\n%s", want, text)
		}
	}
	if Clock(3725.4567) != "62:05.457" {
		t.Errorf("Clock = %s", Clock(3725.4567))
	}
}
