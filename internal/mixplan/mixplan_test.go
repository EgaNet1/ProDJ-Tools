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

func TestCues(t *testing.T) {
	r := track(112,
		analysis.Section{Bar: 0, Kind: "intro"},
		analysis.Section{Bar: 16, Kind: "drop"},
		analysis.Section{Bar: 48, Kind: "break"},
		analysis.Section{Bar: 64, Kind: "drop"},
		analysis.Section{Bar: 96, Kind: "outro"},
	)
	want := []struct {
		slot string
		bar  int
	}{{"A", 1}, {"B", 17}, {"C", 49}, {"D", 65}, {"E", 97}}
	cues := Cues(r)
	if len(cues) != len(want) {
		t.Fatalf("cues %+v", cues)
	}
	for i, w := range want {
		if cues[i].Slot != w.slot || cues[i].Bar != w.bar {
			t.Errorf("cue %d = %s bar %d, want %s bar %d", i, cues[i].Slot, cues[i].Bar, w.slot, w.bar)
		}
	}
	// Bar 17 (index 16) starts 16 bars × 12/7 s after the first downbeat.
	if got, want := cues[1].Time, round3(0.5+16*12.0/7); got != want {
		t.Errorf("DROP at %.3f, want %.3f", got, want)
	}
}

func TestCuesWithoutOutro(t *testing.T) {
	r := track(100, analysis.Section{Bar: 2, Kind: "intro"}, analysis.Section{Bar: 18, Kind: "drop"})
	cues := Cues(r)
	last := cues[len(cues)-1]
	// 16 bars before the end, on a phrase counted from the first bar of music.
	if last.Name != "MIX OUT" || last.Bar != 83 {
		t.Errorf("mix out %+v, want bar 83", last)
	}
}

func TestPlan(t *testing.T) {
	a := track(112, analysis.Section{Bar: 0, Kind: "intro"}, analysis.Section{Bar: 16, Kind: "drop"}, analysis.Section{Bar: 96, Kind: "outro"})
	b := track(112, analysis.Section{Bar: 0, Kind: "intro"}, analysis.Section{Bar: 16, Kind: "drop"})
	b.Grid.BPM = 145
	p, ok := Plan(a, b)
	if !ok {
		t.Fatal("no plan")
	}
	if p.Bars != 16 || p.StartA.Bar != 97 || p.SwapA.Bar != 113 || p.SwapB.Bar != 17 {
		t.Errorf("plan %+v", p)
	}
	// A's outro is exactly 16 bars, so A runs out at its last bar.
	if p.EndA.Bar != 113 {
		t.Errorf("A ends at bar %d, want 113", p.EndA.Bar)
	}
	if p.Pitch != -3.4 {
		t.Errorf("pitch %v, want -3.4", p.Pitch)
	}
}

func TestPlanShortOutro(t *testing.T) {
	// A has only 8 bars of outro; B needs 32 bars of intro.
	a := track(104, analysis.Section{Bar: 0, Kind: "intro"}, analysis.Section{Bar: 16, Kind: "drop"}, analysis.Section{Bar: 96, Kind: "outro"})
	b := track(160, analysis.Section{Bar: 0, Kind: "intro"}, analysis.Section{Bar: 32, Kind: "drop"})
	p, _ := Plan(a, b)
	if p.Bars != 32 || p.StartA.Bar != 65 || p.SwapA.Bar != 97 {
		t.Errorf("plan %+v", p)
	}
}

func TestSheet(t *testing.T) {
	a := track(112, analysis.Section{Bar: 0, Kind: "intro"}, analysis.Section{Bar: 16, Kind: "drop"}, analysis.Section{Bar: 96, Kind: "outro"})
	text := Sheet("TEST", []Entry{{"X", "One", a}, {"Y", "Two", a}})
	for _, want := range []string{"01. X — One", "A  MIX IN", "0:00.500", "01 → 02   One → Two", "bar 97"} {
		if !strings.Contains(text, want) {
			t.Errorf("sheet lacks %q:\n%s", want, text)
		}
	}
	if Clock(3725.4567) != "62:05.457" {
		t.Errorf("Clock = %s", Clock(3725.4567))
	}
}
