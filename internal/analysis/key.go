package analysis

import (
	"math"

	"prodjtools/internal/camelot"
)

const (
	tonalFrame = 4096 // ≈ 0.37 s at 11 kHz: 2.7 Hz bins resolve semitones from ~A1 up
	lowestHz   = 55.0
	highestHz  = 1760.0
)

// Krumhansl–Kessler key profiles: how strongly each scale degree is felt to
// belong to a major or a minor key (Krumhansl & Kessler, 1982).
var (
	majorProfile = [12]float64{6.35, 2.23, 3.48, 2.33, 4.38, 4.09, 2.52, 5.19, 2.39, 3.66, 2.29, 2.88}
	minorProfile = [12]float64{6.33, 2.68, 3.52, 5.38, 2.60, 3.53, 2.54, 4.75, 3.98, 2.69, 3.34, 3.17}
)

// tonal returns the track's average pitch-class content (a chromagram folded
// over time) and its loudness-weighted spectral centroid.
func tonal(x []float32, rate int) (chroma [12]float64, brightness float64) {
	binHz := float64(rate) / tonalFrame

	// Precompute which pitch class each FFT bin belongs to, and how close
	// to the centre of that semitone it lies.
	type pitchBin struct {
		class  int
		weight float64 // 0 for bins outside the musical range
	}
	bins := make([]pitchBin, tonalFrame/2+1)
	for k := range bins {
		hz := float64(k) * binHz
		if hz < lowestHz || hz > highestHz {
			continue
		}
		midi := 69 + 12*math.Log2(hz/440)
		nearest := math.Round(midi)
		off := midi - nearest
		bins[k] = pitchBin{class: int(nearest) % 12, weight: math.Pow(math.Cos(math.Pi*off), 2)}
	}

	var centroidSum, levelSum float64
	s := newSpectrum(tonalFrame)
	s.frames(x, tonalFrame, func(mag []float64) {
		var frame [12]float64
		tonalTotal := 0.0
		for k, m := range mag {
			centroidSum += m * float64(k) * binHz
			levelSum += m
			if b := bins[k]; b.weight > 0 {
				frame[b.class] += m * b.weight
				tonalTotal += m * b.weight
			}
		}
		if tonalTotal < 1e-4 {
			return // silence
		}
		// Every frame gets an equal vote, so loud drops do not drown out
		// the quieter, often more melodic, parts.
		for c := range frame {
			chroma[c] += frame[c] / tonalTotal
		}
	})
	if levelSum > 0 {
		brightness = centroidSum / levelSum
	}
	return chroma, brightness
}

// matchKey picks the key whose profile correlates best with the chromagram.
func matchKey(chroma [12]float64) (camelot.Key, float64) {
	best, bestKey := 0.0, camelot.Key{}
	for tonic := range 12 {
		for _, minor := range []bool{false, true} {
			profile := majorProfile
			if minor {
				profile = minorProfile
			}
			var rotated [12]float64
			for degree, w := range profile {
				rotated[(degree+tonic)%12] = w
			}
			if r := correlation(chroma[:], rotated[:]); r > best {
				best, bestKey = r, camelot.FromPitch(tonic, minor)
			}
		}
	}
	return bestKey, clamp01(best)
}

// correlation is Pearson's r; 0 when either series is flat.
func correlation(a, b []float64) float64 {
	ma, mb := mean(a), mean(b)
	var cov, va, vb float64
	for i := range a {
		da, db := a[i]-ma, b[i]-mb
		cov += da * db
		va += da * da
		vb += db * db
	}
	if va == 0 || vb == 0 {
		return 0
	}
	return cov / math.Sqrt(va*vb)
}
