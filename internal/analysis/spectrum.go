package analysis

import (
	"math"

	"gonum.org/v1/gonum/dsp/fourier"
)

// spectrum computes magnitude spectra of successive Hann-windowed frames.
// The FFT plan and all buffers are allocated once and reused for every frame.
type spectrum struct {
	size   int
	fft    *fourier.FFT
	window []float64
	frame  []float64
	coeffs []complex128
	mag    []float64
	norm   float64
}

func newSpectrum(size int) *spectrum {
	s := &spectrum{
		size:   size,
		fft:    fourier.NewFFT(size),
		window: make([]float64, size),
		frame:  make([]float64, size),
		coeffs: make([]complex128, size/2+1),
		mag:    make([]float64, size/2+1),
	}
	sum := 0.0
	for i := range s.window {
		w := 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(size))
		s.window[i] = w
		sum += w
	}
	s.norm = 2 / sum // so that a full-scale sine wave peaks at 1.0
	return s
}

// frames calls fn with the magnitude spectrum of every frame, hop samples
// apart. fn must not keep the slice: it is overwritten by the next frame.
func (s *spectrum) frames(x []float32, hop int, fn func(mag []float64)) {
	for start := 0; start+s.size <= len(x); start += hop {
		for i, w := range s.window {
			s.frame[i] = float64(x[start+i]) * w
		}
		s.fft.Coefficients(s.coeffs, s.frame)
		for k, c := range s.coeffs {
			re, im := real(c), imag(c)
			s.mag[k] = math.Sqrt(re*re+im*im) * s.norm
		}
		fn(s.mag)
	}
}
