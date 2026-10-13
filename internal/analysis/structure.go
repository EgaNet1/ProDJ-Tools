package analysis

import (
	"math"
	"slices"
)

// Section is a part of a track: intro, drop, break or outro.
type Section struct {
	Bar  int    `json:"bar"`  // first bar, counted from Grid.FirstBeat (bar 0)
	Kind string `json:"kind"` // "intro", "drop", "break" or "outro"
}

const (
	minSection   = 8    // bars; dance music is built from 8-bar phrases
	minBoundary  = 0.16 // intensity step that counts as a new section
	dropShare    = 0.85 // a drop is at least this intense relative to the most intense section
	silenceBelow = 30   // dB under the loud parts: no music
)

// findStructure rates the intensity of every bar (0–255, for drawing),
// finds where the music starts and ends and splits it into sections.
// Sections start on bar multiples of four counted from the first bar of
// music, preferably on multiples of eight: that is how tracks are built, and
// how a DJ counts phrases.
func findStructure(f frameSeries, g Grid, duration float64) (bars []uint8, sections []Section, lastBar int) {
	if !g.Valid() {
		return nil, nil, 0
	}
	var loud, bass []float64
	for n := 0; g.BarTime(n+1) <= duration+0.01; n++ {
		from, to := g.BarTime(n), g.BarTime(n+1)
		loud = append(loud, decibels(math.Sqrt(f.mean(f.low, from, to)+f.mean(f.mid, from, to)+f.mean(f.high, from, to))))
		bass = append(bass, decibels(math.Sqrt(f.mean(f.low, from, to))))
	}
	if len(loud) < minSection {
		return nil, nil, 0
	}

	// Intensity mixes overall loudness and bass, each measured against the
	// loud parts of this very track (its 95th percentile).
	loudTop, bassTop := percentile(loud, 0.95), percentile(bass, 0.95)
	intensity := make([]float64, len(loud))
	bars = make([]uint8, len(loud))
	for i := range loud {
		intensity[i] = 0.5*clamp01((loud[i]-loudTop+18)/18) + 0.5*clamp01((bass[i]-bassTop+24)/24)
		bars[i] = uint8(math.Round(255 * intensity[i]))
	}

	start, end := -1, -1 // first bar of music, first bar after it
	for i, l := range loud {
		if l > loudTop-silenceBelow {
			if start < 0 {
				start = i
			}
			end = i + 1
		}
	}
	if start < 0 || end-start < minSection {
		return bars, nil, end
	}

	mean := func(from, to int) float64 {
		from, to = max(from, start), min(to, end)
		sum := 0.0
		for _, v := range intensity[from:to] {
			sum += v
		}
		return sum / float64(to-from)
	}

	// Candidate boundaries: every 4 bars, scored by how much the intensity
	// of the 4 bars after differs from the 4 bars before.
	type candidate struct {
		bar   int
		score float64
	}
	var candidates []candidate
	for b := start + 4; b+4 <= end; b += 4 {
		score := math.Abs(mean(b, b+4) - mean(b-4, b))
		if (b-start)%8 == 0 {
			score *= 1.25
		}
		candidates = append(candidates, candidate{b, score})
	}
	slices.SortFunc(candidates, func(a, b candidate) int {
		return cmpFloat(b.score, a.score) // strongest first
	})
	var cuts []int
	for _, c := range candidates {
		if c.score < minBoundary {
			break
		}
		if c.bar-start < minSection || end-c.bar < minSection {
			continue
		}
		farEnough := true
		for _, cut := range cuts {
			if abs(cut-c.bar) < minSection {
				farEnough = false
			}
		}
		if farEnough {
			cuts = append(cuts, c.bar)
		}
	}
	slices.Sort(cuts)

	// Name the sections by their intensity and their place in the track.
	bounds := append(append([]int{start}, cuts...), end)
	var strongest float64
	means := make([]float64, len(bounds)-1)
	for i := range means {
		means[i] = mean(bounds[i], bounds[i+1])
		strongest = max(strongest, means[i])
	}
	for i, m := range means {
		kind := "break"
		switch {
		case m >= dropShare*strongest:
			kind = "drop"
		case i == 0:
			kind = "intro"
		case i == len(means)-1:
			kind = "outro"
		}
		if len(sections) > 0 && sections[len(sections)-1].Kind == kind {
			continue // merge with the previous section of the same kind
		}
		sections = append(sections, Section{Bar: bounds[i], Kind: kind})
	}
	return bars, sections, end
}

func percentile(v []float64, p float64) float64 {
	s := slices.Clone(v)
	slices.Sort(s)
	return s[min(len(s)-1, int(p*float64(len(s))))]
}

func cmpFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
