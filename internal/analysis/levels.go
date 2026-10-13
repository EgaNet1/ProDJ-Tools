package analysis

import "math"

const overviewPoints = 96

// levels returns the average loudness in dBFS and a coarse loudness outline
// of the track, scaled so its loudest point is 255.
func levels(x []float32) (loudness float64, overview []uint8) {
	loudness = decibels(rms(x))

	chunk := len(x) / overviewPoints
	if chunk == 0 {
		return loudness, nil
	}
	values := make([]float64, overviewPoints)
	peak := 0.0
	for i := range values {
		values[i] = rms(x[i*chunk : (i+1)*chunk])
		peak = max(peak, values[i])
	}
	overview = make([]uint8, overviewPoints)
	if peak > 0 {
		for i, v := range values {
			overview[i] = uint8(math.Round(255 * v / peak))
		}
	}
	return loudness, overview
}

func rms(x []float32) float64 {
	if len(x) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range x {
		sum += float64(v) * float64(v)
	}
	return math.Sqrt(sum / float64(len(x)))
}

func decibels(v float64) float64 { return 20 * math.Log10(max(v, 1e-9)) }
