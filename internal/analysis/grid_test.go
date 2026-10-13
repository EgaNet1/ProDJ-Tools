package analysis

import (
	"math"
	"math/rand/v2"
	"testing"

	"prodjtools/internal/audio"
)

type part struct {
	bars             int
	kick, bass, pads bool
}

// produced synthesises a track the way dance music is built: silence, then
// sections of whole bars with kick on every beat, a bass line and chords
// that change on every downbeat. It returns the track and the time of the
// first downbeat.
func produced(bpm, silence float64, parts []part) audio.Signal {
	beat := 60 / bpm
	total := 0
	for _, p := range parts {
		total += p.bars
	}
	seconds := silence + float64(total)*4*beat + 2
	x := make([]float32, int(seconds*testRate))
	rng := rand.New(rand.NewPCG(7, 7))
	chords := [][]float64{{220, 261.63, 329.63}, {174.61, 220, 261.63}, {196, 246.94, 293.66}, {164.81, 207.65, 246.94}}

	add := func(at, length float64, sample func(t float64) float64) {
		start := int(math.Round(at * testRate))
		for i := 0; i < int(length*testRate) && start+i < len(x); i++ {
			x[start+i] += float32(sample(float64(i) / testRate))
		}
	}
	bar := 0
	for _, p := range parts {
		for range p.bars {
			barStart := silence + float64(bar)*4*beat
			for b := range 4 {
				at := barStart + float64(b)*beat
				if p.kick {
					add(at, 0.25, func(t float64) float64 {
						click := 0.5 * math.Exp(-t*900) * (rng.Float64()*2 - 1)
						return click + 0.8*math.Exp(-t*18)*math.Sin(2*math.Pi*(50+80*math.Exp(-t*40))*t)
					})
					add(at+beat/2, 0.05, func(t float64) float64 { return 0.08 * math.Exp(-t*120) * (rng.Float64()*2 - 1) })
				}
			}
			chord := chords[bar%len(chords)]
			if p.bass {
				add(barStart, 4*beat, func(t float64) float64 { return 0.35 * math.Sin(2*math.Pi*chord[0]/4*t) })
			}
			if p.pads {
				add(barStart, 4*beat, func(t float64) float64 {
					v := 0.0
					for _, hz := range chord {
						v += 0.07 * math.Sin(2*math.Pi*hz*t)
					}
					return v * min(1, t*40)
				})
			}
			bar++
		}
	}
	return audio.Signal{Samples: x, Rate: testRate, Duration: seconds}
}

func TestGridAndStructure(t *testing.T) {
	const silence = 0.537
	parts := []part{
		{bars: 16, kick: true},                         // intro
		{bars: 32, kick: true, bass: true, pads: true}, // drop
		{bars: 16, pads: true},                         // break
		{bars: 32, kick: true, bass: true, pads: true}, // drop
		{bars: 16, kick: true},                         // outro
	}
	for _, bpm := range []float64{140, 128, 174} {
		r := Analyze(produced(bpm, silence, parts))
		g := r.Grid
		if math.Abs(g.BPM-bpm) > 0.001 {
			t.Errorf("%v BPM: grid tempo %.4f", bpm, g.BPM)
		}
		if d := math.Abs(g.FirstBeat - silence); d > 0.001 {
			t.Errorf("%v BPM: first downbeat at %.4f s, want %.4f (off by %.1f ms)", bpm, g.FirstBeat, silence, d*1000)
		}
		if g.Jitter > 1 {
			t.Errorf("%v BPM: jitter %.2f ms", bpm, g.Jitter)
		}
		want := []Section{{0, "intro"}, {16, "drop"}, {48, "break"}, {64, "drop"}, {96, "outro"}}
		if len(r.Sections) != len(want) {
			t.Errorf("%v BPM: sections %v, want %v", bpm, r.Sections, want)
			continue
		}
		for i := range want {
			if r.Sections[i] != want[i] {
				t.Errorf("%v BPM: sections %v, want %v", bpm, r.Sections, want)
				break
			}
		}
		if r.LastBar != 112 {
			t.Errorf("%v BPM: last bar %d, want 112", bpm, r.LastBar)
		}
	}
}
