// Package setlist puts tracks into a playing order that flows: tempos move
// gradually, keys sit well together and the energy follows a chosen shape.
package setlist

import (
	"math"

	"prodjtools/internal/camelot"
)

// Track is what ordering needs to know about a song.
type Track struct {
	ID       string
	BPM      float64
	Key      camelot.Key
	Energy   float64 // 0–1, relative to the other tracks in the folder
	Duration float64 // seconds
}

// Weights say how much each goal matters. 1 is the default; 0 ignores a
// goal and 2 makes it twice as important.
type Weights struct {
	Harmony float64 `json:"harmony"` // keys that mix well
	Tempo   float64 `json:"tempo"`   // small tempo steps, tempo following the curve
	Energy  float64 `json:"energy"`  // energy following the curve
}

var DefaultWeights = Weights{Harmony: 1, Tempo: 1, Energy: 1}

// Options control how a set is built.
type Options struct {
	Curve   Curve
	Weights Weights
	Pins    map[string]int // track ID → position (0-based) it must keep
}

// Summary describes a whole set at a glance.
type Summary struct {
	Count     int       `json:"count"`
	Length    float64   `json:"length"`    // seconds, tracks played back to back
	MixLength float64   `json:"mixLength"` // seconds, with transitions overlapping
	BPMMin    float64   `json:"bpmMin"`
	BPMMax    float64   `json:"bpmMax"`
	Rough     int       `json:"rough"`  // transitions graded Rough
	Energy    []float64 `json:"energy"` // energy of each track, in order
	Target    []float64 `json:"target"` // what the curve asks for at each position
}

// Evaluate judges every transition of a set and sums it up. overlap is how
// many seconds two tracks play together during a transition.
func Evaluate(order []Track, curve Curve, overlap float64) ([]Transition, Summary) {
	transitions := make([]Transition, 0, len(order))
	s := Summary{
		Count:  len(order),
		Energy: make([]float64, len(order)),
		Target: curve.Targets(len(order)),
	}
	for i, t := range order {
		s.Length += t.Duration
		s.Energy[i] = math.Round(t.Energy*100) / 100
		if t.BPM > 0 && (s.BPMMin == 0 || t.BPM < s.BPMMin) {
			s.BPMMin = t.BPM
		}
		s.BPMMax = max(s.BPMMax, t.BPM)
		if i > 0 {
			tr := Judge(order[i-1], t)
			if tr.Grade == Rough {
				s.Rough++
			}
			transitions = append(transitions, tr)
		}
	}
	s.MixLength = mixLength(order, overlap)
	return transitions, s
}

func mixLength(order []Track, overlap float64) float64 {
	total := 0.0
	for _, t := range order {
		total += t.Duration
	}
	return total - overlap*float64(max(0, len(order)-1))
}

func clamp01(v float64) float64 { return min(1, max(0, v)) }
