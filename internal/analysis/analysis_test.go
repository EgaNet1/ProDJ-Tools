package analysis

import (
	"math"
	"math/rand/v2"
	"testing"

	"prodjtools/internal/audio"
)

const testRate = audio.AnalysisRate

// beatTrack synthesises a four-on-the-floor kick with off-beat hats.
func beatTrack(bpm, seconds float64) audio.Signal {
	x := make([]float32, int(seconds*testRate))
	period := 60 / bpm * testRate
	rng := rand.New(rand.NewPCG(1, 2))
	for beat := 0.0; beat*period < float64(len(x)); beat += 0.5 {
		start := int(beat * period)
		kick := math.Mod(beat, 1) == 0
		for i := 0; i < testRate/8 && start+i < len(x); i++ {
			t := float64(i) / testRate
			if kick {
				x[start+i] += float32(0.8 * math.Exp(-t*30) * math.Sin(2*math.Pi*55*t))
			} else {
				x[start+i] += float32(0.2 * math.Exp(-t*80) * (rng.Float64()*2 - 1))
			}
		}
	}
	return audio.Signal{Samples: x, Rate: testRate, Duration: seconds}
}

func TestTempo(t *testing.T) {
	for _, want := range []float64{124, 140, 174} {
		r := Analyze(beatTrack(want, 40))
		if got := FoldBPM(r.BPM, 88); math.Abs(got-want) > 0.5 {
			t.Errorf("tempo %v: got %v (raw %.2f, confidence %.2f)", want, got, r.BPM, r.BPMConfidence)
		}
	}
}

func TestKeyAMinor(t *testing.T) {
	// A minor triad with a quieter bass note on A.
	x := make([]float32, 20*testRate)
	notes := map[float64]float64{110: 0.3, 220: 0.25, 261.63: 0.2, 329.63: 0.2}
	for i := range x {
		t := float64(i) / testRate
		for hz, amp := range notes {
			x[i] += float32(amp * math.Sin(2*math.Pi*hz*t))
		}
	}
	r := Analyze(audio.Signal{Samples: x, Rate: testRate, Duration: 20})
	if r.Key.String() != "8A" {
		t.Errorf("key = %s (%s), want 8A (Am)", r.Key, r.Key.Name())
	}
}

func TestFoldBPM(t *testing.T) {
	cases := map[float64]float64{70: 140, 72.49: 145, 139.85: 140, 300: 150, 87: 174, 141.3: 141.3}
	for in, want := range cases {
		if got := FoldBPM(in, 88); got != want {
			t.Errorf("FoldBPM(%v) = %v, want %v", in, got, want)
		}
	}
}
