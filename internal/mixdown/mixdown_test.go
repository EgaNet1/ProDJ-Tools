package mixdown

import (
	"context"
	"math"
	"path/filepath"
	"testing"

	"prodjtools/internal/analysis"
	"prodjtools/internal/audio"
	"prodjtools/internal/mixplan"
)

// track builds a 128 BPM track of the classic shape: intro, drop, break,
// drop, outro (112 bars, 210 s).
func track(name string, bpm float64) Track {
	sec := func(bar int, kind string) analysis.Section { return analysis.Section{Bar: bar, Kind: kind} }
	return Track{
		Path:     name,
		Title:    name,
		Duration: 112*240/bpm + 1,
		Result: analysis.Result{
			Grid:     analysis.Grid{BPM: bpm, FirstBeat: 0.5},
			Sections: []analysis.Section{sec(0, analysis.Intro), sec(16, analysis.Drop), sec(48, analysis.Break), sec(64, analysis.Drop), sec(96, analysis.Outro)},
			LastBar:  112,
		},
	}
}

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestClipTime(t *testing.T) {
	c := Clip{rate: []rateSeg{{10, math.Inf(1), 1.05, 1.05, 30}}}
	c.ride(20, 16) // glide back to speed 1 over 16 s of the track
	for _, src := range []float64{30, 35, 40.5, 45, 50, 56.5, 80} {
		if got := c.srcAt(c.outAt(src)); !near(got, src, 1e-9) {
			t.Errorf("srcAt(outAt(%v)) = %v", src, got)
		}
	}
	if r := c.rateAt(19); r != 1.05 {
		t.Errorf("speed before the glide %v", r)
	}
	end := 20 + 2*16/2.05
	if r := c.rateAt(end + 1); r != 1 {
		t.Errorf("speed after the glide %v", r)
	}
	if !near(c.srcAt(end), c.srcAt(20)+16, 1e-9) {
		t.Errorf("the glide covers %v s, want 16", c.srcAt(end)-c.srcAt(20))
	}
}

func TestBuild(t *testing.T) {
	a, b := track("a", 128), track("b", 124)
	p, ok := mixplan.Plan(a.Result, b.Result)
	if !ok {
		t.Fatal("no plan")
	}
	tl := Build([]Track{a, b}, []*mixplan.Transition{&p})
	ca, cb := tl.Clips[0], tl.Clips[1]

	// A starts at its MIX IN cue, so its bar 97 (the outro) comes 96 bars in.
	bar := 240.0 / 128
	if !near(cb.Start, 96*bar, 1e-6) {
		t.Errorf("B starts at %.3f s, want %.3f", cb.Start, 96*bar)
	}
	// B plays from its MIX IN at A's tempo.
	if !near(cb.srcAt(cb.Start), p.CueB.Time, 1e-9) || !near(cb.rateAt(cb.Start), 128.0/124, 1e-9) {
		t.Errorf("B from %.3f at speed %.4f", cb.srcAt(cb.Start), cb.rateAt(cb.Start))
	}
	// At the swap B is exactly at its drop: the bars line up.
	swap := tl.Joins[0].Swap
	if !near(cb.srcAt(swap), b.Result.Grid.BarTime(p.SwapB.Bar-1), 1e-9) {
		t.Errorf("at the swap B is at %.4f, its drop is at %.4f", cb.srcAt(swap), p.SwapB.Time)
	}
	// Levels as in the preview: B silent at the start, full by the middle;
	// A full until the swap, gone by its end; bass swapped at the swap.
	mid := (cb.Start + swap) / 2
	if cb.gainAt(cb.Start) != 0 || cb.gainAt(mid) != 1 || ca.gainAt(swap) != 1 || ca.gainAt(ca.End) != 0 {
		t.Errorf("levels: B %v→%v, A %v→%v", cb.gainAt(cb.Start), cb.gainAt(mid), ca.gainAt(swap), ca.gainAt(ca.End))
	}
	if cb.bassAt(swap-0.02) != bassCutDB || cb.bassAt(swap) != 0 || ca.bassAt(swap) != bassCutDB {
		t.Error("bass not swapped at the swap")
	}
	// After A is gone, B glides back to its own tempo.
	if r := cb.rateAt(tl.Length - 10); r != 1 {
		t.Errorf("B ends at speed %v", r)
	}
	if !near(tl.Length, cb.End, 1e-9) || tl.Length < cb.Start+60 {
		t.Errorf("length %.1f, B %.1f–%.1f", tl.Length, cb.Start, cb.End)
	}
}

// tone is a loader of sine tones, one frequency per track.
func tone(tracks []Track) Loader {
	return func(i int) (*audio.PCM, error) {
		rate := 48000 // not the mix rate, to exercise resampling
		n := int(tracks[i].Duration * float64(rate))
		p := &audio.PCM{Rate: rate, Data: make([]int16, 2*n)}
		hz := 220.0 * float64(i+1)
		for j := range n {
			v := int16(12000 * math.Sin(2*math.Pi*hz*float64(j)/float64(rate)))
			p.Data[2*j], p.Data[2*j+1] = v, v
		}
		return p, nil
	}
}

func TestExport(t *testing.T) {
	a, b := track("a", 128), track("b", 126)
	p, _ := mixplan.EarlyPlan(a.Result, b.Result)
	tracks := []Track{a, b}
	tl := Build(tracks, []*mixplan.Transition{&p})
	want := tl.Length * Rate

	for _, format := range []string{FLAC, MP3} {
		path := filepath.Join(t.TempDir(), "set."+format)
		last := 0.0
		if err := Export(context.Background(), path, format, &tl, tone(tracks), func(f float64) { last = f }); err != nil {
			t.Fatalf("%s: %v", format, err)
		}
		if last != 1 {
			t.Errorf("%s: progress ended at %v", format, last)
		}
		pcm, err := audio.DecodePCM(path)
		if err != nil {
			t.Fatalf("%s: decoding the result: %v", format, err)
		}
		if pcm.Rate != Rate {
			t.Errorf("%s: rate %d", format, pcm.Rate)
		}
		// MP3 works in frames of 1152 samples: the decoder's delay and the
		// padding of the last frame make it a little shorter or longer.
		tol := 2.0
		if format == MP3 {
			tol = 2 * 1152
		}
		if got := float64(pcm.Frames()); math.Abs(got-want) > tol {
			t.Errorf("%s: %0.f frames, want %.0f", format, got, want)
		}
		// Loud in the middle of the set, never clipped.
		var sum float64
		peak := 0
		mid := pcm.Frames() / 2
		for j := mid; j < mid+Rate; j++ {
			v := int(pcm.Data[2*j])
			sum += float64(v * v)
			peak = max(peak, v, -v)
		}
		if rms := math.Sqrt(sum / Rate); rms < 3000 || peak > 32700 {
			t.Errorf("%s: rms %.0f, peak %d", format, rms, peak)
		}
	}
}

func TestStream(t *testing.T) {
	a, b := track("a", 128), track("b", 128)
	p, _ := mixplan.Plan(a.Result, b.Result)
	tracks := []Track{a, b}
	tl := Build(tracks, []*mixplan.Transition{&p})
	r := NewRenderer(&tl, tone(tracks), tl.Length-5)
	if f := r.Frames(); !near(float64(f), 5*Rate, 2) {
		t.Errorf("5 s before the end: %d frames", f)
	}
}
