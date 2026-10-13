package analysis

import (
	"math"
	"math/rand/v2"
	"testing"

	"prodjtools/internal/audio"
)

// offbeatTrack is house as it is often produced: a kick on every beat, hats
// and a short bass note on every off-beat, chords on top. The off-beats make
// the music repeat every 1.5 and 3 beats as strongly as every beat, which
// once fooled the tempo estimate into 4/3 of the tempo.
func offbeatTrack(bpm, seconds float64) audio.Signal {
	x := make([]float32, int(seconds*testRate))
	beat := 60 / bpm
	rng := rand.New(rand.NewPCG(3, 4))
	add := func(at, length float64, f func(t float64) float64) {
		s := int(at * testRate)
		for i := 0; i < int(length*testRate) && s+i < len(x); i++ {
			x[s+i] += float32(f(float64(i) / testRate))
		}
	}
	for k := 0; float64(k)*beat < seconds; k++ {
		at := 0.3 + float64(k)*beat
		add(at, 0.3, func(t float64) float64 {
			return 0.9*math.Exp(-t*16)*math.Sin(2*math.Pi*(48+90*math.Exp(-t*35))*t) + 0.3*math.Exp(-t*700)*(rng.Float64()*2-1)
		})
		add(at+beat/2, 0.06, func(t float64) float64 { return 0.12 * math.Exp(-t*90) * (rng.Float64()*2 - 1) })
		add(at+beat/2, beat/2, func(t float64) float64 { return 0.35 * math.Sin(2*math.Pi*69.3*t) * min(1, t*60) * math.Exp(-t*3) })
		if k%4 == 0 {
			add(at, 4*beat, func(t float64) float64 {
				return 0.06*math.Sin(2*math.Pi*277.18*t) + 0.06*math.Sin(2*math.Pi*329.63*t) + 0.06*math.Sin(2*math.Pi*415.3*t)
			})
		}
	}
	return audio.Signal{Samples: x, Rate: testRate, Duration: seconds}
}

func TestTempoOffbeats(t *testing.T) {
	for _, want := range []float64{122, 125, 127, 128} {
		r := Analyze(offbeatTrack(want, 60))
		if math.Abs(r.Grid.BPM-want) > 0.01 {
			t.Errorf("%v BPM: grid at %.3f (estimate %.2f)", want, r.Grid.BPM, r.BPM)
		}
	}
}
