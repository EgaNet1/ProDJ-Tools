// Package analysis extracts what a DJ needs to know about a track: tempo,
// beat grid, musical key, structure and how intense it sounds.
package analysis

import (
	"prodjtools/internal/audio"
	"prodjtools/internal/camelot"
)

// Version changes whenever the same audio would produce different results,
// so that results cached by an older version are recomputed.
const Version = 4

// Result describes one track.
type Result struct {
	BPM           float64     `json:"bpm"`           // raw estimate, possibly an octave off; see FoldBPM
	BPMConfidence float64     `json:"bpmConfidence"` // 0–1
	Grid          Grid        `json:"grid"`
	Key           camelot.Key `json:"key"`
	KeyConfidence float64     `json:"keyConfidence"` // 0–1, fit to the key profile
	Loudness      float64     `json:"loudness"`      // average level, dBFS
	Punch         float64     `json:"punch"`         // density and strength of attacks
	Brightness    float64     `json:"brightness"`    // spectral centroid, Hz
	Overview      []uint8     `json:"overview"`      // loudness outline for drawing, 0–255
	Bars          []uint8     `json:"bars"`          // intensity of every bar, 0–255
	Sections      []Section   `json:"sections"`      // intro, drops, verses, breaks, outro
	LastBar       int         `json:"lastBar"`       // first bar after the music ends
	BarParity     int         `json:"barParity"`     // 0 or 1: which grid bars start real bars at half tempo; see InMeter
}

// Analyze measures a decoded track. Tracks shorter than a few seconds get an
// empty result: there is nothing reliable to say about them.
func Analyze(sig audio.Signal) Result {
	x := sig.Samples
	if len(x) < 5*sig.Rate {
		return Result{}
	}

	frames := measureFrames(x, sig.Rate)
	onsets := removeTrend(frames.flux, int(frames.fps()/2))
	bpm, bpmConfidence := estimateTempo(onsets, frames.fps())
	grid := findGrid(x, frames, bpm, sig.Duration)
	s := findStructure(frames, grid, sig.Duration)

	chroma, brightness := tonal(x, sig.Rate)
	key, keyConfidence := matchKey(chroma)

	loudness, overview := levels(x)

	return Result{
		BPM:           bpm,
		BPMConfidence: bpmConfidence,
		Grid:          grid,
		Key:           key,
		KeyConfidence: keyConfidence,
		Loudness:      loudness,
		Punch:         mean(onsets),
		Brightness:    brightness,
		Overview:      overview,
		Bars:          s.bars,
		Sections:      s.sections,
		LastBar:       s.lastBar,
		BarParity:     s.barParity,
	}
}

func mean(x []float64) float64 {
	if len(x) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range x {
		sum += v
	}
	return sum / float64(len(x))
}

func clamp01(v float64) float64 { return min(1, max(0, v)) }
