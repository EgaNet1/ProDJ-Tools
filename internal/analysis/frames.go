package analysis

import "math"

// frameSeries holds measurements of every short frame of a track, onsetHop
// samples (≈ 11.6 ms) apart. One pass of the FFT feeds tempo detection, the
// downbeat search and the structure analysis.
type frameSeries struct {
	rate int
	flux []float64 // spectral flux: how much new sound appears (onsets)
	low  []float64 // power below 150 Hz: kick and bass
	mid  []float64 // 150–2000 Hz
	high []float64 // above 2000 Hz
}

func measureFrames(x []float32, rate int) frameSeries {
	s := newSpectrum(onsetFrame)
	binHz := float64(rate) / onsetFrame
	lowTop, midTop := int(150/binHz), int(2000/binHz)
	n := len(x)/onsetHop + 1
	f := frameSeries{
		rate: rate,
		flux: make([]float64, 0, n),
		low:  make([]float64, 0, n),
		mid:  make([]float64, 0, n),
		high: make([]float64, 0, n),
	}

	prev := make([]float64, onsetFrame/2+1)
	first := true
	s.frames(x, onsetHop, func(mag []float64) {
		var flux, low, mid, high float64
		for k, m := range mag {
			// Flux on a log scale: drum hits and note attacks make sharp
			// peaks; their spacing reveals the tempo.
			v := math.Log1p(1000 * m)
			if d := v - prev[k]; d > 0 && !first {
				flux += d
			}
			prev[k] = v

			switch p := m * m; {
			case k < 2: // DC and rumble below ~20 Hz
			case k <= lowTop:
				low += p
			case k <= midTop:
				mid += p
			default:
				high += p
			}
		}
		first = false
		f.flux = append(f.flux, flux)
		f.low = append(f.low, low)
		f.mid = append(f.mid, mid)
		f.high = append(f.high, high)
	})
	return f
}

// fps is the number of frames per second.
func (f frameSeries) fps() float64 { return float64(f.rate) / onsetHop }

// time is the moment frame i stands for: the centre of its window.
func (f frameSeries) time(i float64) float64 {
	return (i*onsetHop + onsetFrame/2) / float64(f.rate)
}

// mean averages a series over the frames centred in [from, to).
func (f frameSeries) mean(series []float64, from, to float64) float64 {
	rate := float64(f.rate)
	lo := max(0, int(math.Ceil((from*rate-onsetFrame/2)/onsetHop)))
	hi := min(len(series), int(math.Ceil((to*rate-onsetFrame/2)/onsetHop)))
	if hi <= lo {
		return 0
	}
	sum := 0.0
	for _, v := range series[lo:hi] {
		sum += v
	}
	return sum / float64(hi-lo)
}

// total is the power of all bands in one frame.
func (f frameSeries) total(i int) float64 { return f.low[i] + f.mid[i] + f.high[i] }
