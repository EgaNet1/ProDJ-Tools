package analysis

import "math"

const (
	onsetFrame = 1024
	onsetHop   = 128 // ≈ 11.6 ms at 11 kHz: fine enough to place drum hits

	minBPM = 60.0
	maxBPM = 200.0
)

// removeTrend subtracts a moving average and keeps only what rises above it,
// so slow swells in loudness do not look like beats.
func removeTrend(x []float64, radius int) []float64 {
	prefix := make([]float64, len(x)+1)
	for i, v := range x {
		prefix[i+1] = prefix[i] + v
	}
	out := make([]float64, len(x))
	for i, v := range x {
		lo, hi := max(0, i-radius), min(len(x), i+radius+1)
		avg := (prefix[hi] - prefix[lo]) / float64(hi-lo)
		out[i] = max(0, v-avg)
	}
	return out
}

func autocorrelation(x []float64, maxLag int) []float64 {
	ac := make([]float64, maxLag+1)
	for lag := range ac {
		sum := 0.0
		for i := 0; i+lag < len(x); i++ {
			sum += x[i] * x[i+lag]
		}
		ac[lag] = sum / float64(len(x)-lag)
	}
	return ac
}

// estimateTempo finds the beat period that best explains how the onset
// envelope repeats itself. It returns a raw BPM, which may be half or double
// the tempo a DJ would name, and a confidence between 0 and 1.
func estimateTempo(onsets []float64, fps float64) (bpm, confidence float64) {
	maxLag := min(len(onsets)/2, int(16*60/minBPM*fps)+2)
	if maxLag < 2 {
		return 0, 0
	}
	ac := autocorrelation(onsets, maxLag)

	at := func(lag float64) float64 {
		i := int(lag)
		if i+1 >= len(ac) {
			return 0
		}
		f := lag - float64(i)
		return ac[i]*(1-f) + ac[i+1]*f
	}
	// comb scores a tempo by how strongly the music repeats after 1, 2, 4…
	// beats of it.
	comb := func(bpm float64, beats ...float64) float64 {
		period := 60 / bpm * fps
		score := 0.0
		for _, b := range beats {
			score += at(period * b)
		}
		return score
	}

	// 1. Coarse search over the whole range.
	type candidate struct{ bpm, score float64 }
	var (
		candidates []candidate
		best       candidate
		total      float64
	)
	for b := minBPM; b <= maxBPM; b += 0.1 {
		c := candidate{b, comb(b, 1, 2, 4)}
		candidates = append(candidates, c)
		total += c.score
		if c.score > best.score {
			best = c
		}
	}
	if best.score <= 0 {
		return 0, 0
	}

	// Confidence: how far the winner stands above its strongest rival that
	// is not simply the same tempo counted in another octave.
	baseline := total / float64(len(candidates))
	rival := baseline
	for _, c := range candidates {
		r := c.bpm / best.bpm
		if near(r, 0.5) || near(r, 1) || near(r, 2) {
			continue
		}
		rival = max(rival, c.score)
	}
	confidence = clamp01((best.score - rival) / (best.score - baseline))

	// 2. Refine around the winner. Repetitions 4–16 beats apart pin the
	// period down far more precisely than a single beat can.
	longest := 60 / (best.bpm - 1) * fps
	var beats []float64
	for _, b := range []float64{4, 8, 16} {
		if longest*b < float64(len(ac)-1) {
			beats = append(beats, b)
		}
	}
	bpm = best.bpm
	if len(beats) == 0 {
		return bpm, confidence
	}
	top := comb(bpm, beats...)
	for b := best.bpm - 1; b <= best.bpm+1; b += 0.01 {
		if s := comb(b, beats...); s > top {
			bpm, top = b, s
		}
	}
	return bpm, confidence
}

func near(ratio, target float64) bool { return math.Abs(ratio/target-1) < 0.04 }

// FoldBPM moves a tempo into [floor, 2·floor) by doubling or halving it.
// Detectors cannot always tell 70 from 140 BPM, but a DJ knows which octave
// their genre lives in. Tempos within 0.2 BPM of a whole number are rounded
// to it: electronic music is almost always produced at integer tempos.
func FoldBPM(bpm, floor float64) float64 {
	if bpm <= 0 || floor <= 0 {
		return bpm
	}
	for bpm < floor {
		bpm *= 2
	}
	for bpm >= 2*floor {
		bpm /= 2
	}
	if r := math.Round(bpm); math.Abs(bpm-r) < 0.2 {
		return r
	}
	return math.Round(bpm*10) / 10
}
