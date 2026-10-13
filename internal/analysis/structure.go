package analysis

import (
	"math"
	"slices"
)

// Section is a part of a track.
type Section struct {
	Bar  int    `json:"bar"`  // first bar, counted from Grid.FirstBeat (bar 0)
	Kind string `json:"kind"` // see the kinds below
}

// Kinds of sections, by how they feel on the dance floor.
const (
	Intro = "intro" // the beginning, before the first drop
	Drop  = "drop"  // the most intense parts: drops, hooks, choruses
	Verse = "verse" // calmer parts that keep the beat going
	Break = "break" // breakdowns: the kick drops out or the track nearly stops
	Outro = "outro" // the end, after the last drop
)

const (
	minSection   = 8    // bars; dance music is built from 8-bar phrases
	minBoundary  = 0.12 // feature change that counts as a new section
	dropMargin   = 1.25 // dB: a drop is at most this much quieter than the loudest section, overall and in the bass
	breakShare   = 0.55 // below this share of the most intense section, a section is a break
	kickPresent  = 0.2  // share of the track's kick strength above which the kick plays
	silenceBelow = 30   // dB under the loud parts: no music
	window       = 4    // bars compared either side of a candidate boundary
)

// barFeatures describes one bar. The first six values run from 0 (silent,
// absent) to 1 (as strong as the loud parts of this very track) and are
// what boundaries are found by.
type barFeatures struct {
	loud, low, mid, high float64 // band levels
	kick                 float64 // how hard the bass punches on the beats
	flux                 float64 // how much new sound arrives: rhythmic density

	// Overall and bass level in dB relative to the loud parts of the track,
	// unscaled: a drop and a busy intro can be 2 dB apart, which the 0–1
	// levels squeeze into a few hundredths.
	loudDB, lowDB float64
}

func (b barFeatures) vector() [6]float64 {
	return [6]float64{b.loud, b.low, b.mid, b.high, b.kick, b.flux}
}

// intensity is how hard a bar hits: overall loudness and bass, half and half.
func (b barFeatures) intensity() float64 { return 0.5*b.loud + 0.5*b.low }

// structure is what findStructure learns about a track.
type structure struct {
	bars      []uint8   // intensity of every bar, 0–255, for drawing
	sections  []Section // from the first bar of music to the last
	lastBar   int       // first bar after the music
	barParity int       // 0 or 1: which bars start real bars when the tempo is read at half speed
	novelty   []float64 // how much the music changes at the start of every bar
}

// findStructure splits a track into sections and finds where the music
// starts and ends.
//
// Every bar is described by six features — band levels, kick strength and
// rhythmic density — so that a change of texture counts even when the
// loudness stays the same: a rap verse turning into a hook, hats dropping
// out, the kick coming back. At the start of every bar the features of the
// bars before are compared with the bars after; sections start where that
// difference peaks, which places boundaries on the exact bar.
//
// Phrases in dance music are 4 and 8 bars long and every section starts on
// one, so the phrase grid is found first — the offset, counted in bars,
// where the biggest changes keep happening — and boundaries on it are
// preferred. A track whose music starts a bar or two early, with a pickup or
// a riser, is then still cut on its real phrases.
func findStructure(f frameSeries, g Grid, duration float64) structure {
	if !g.Valid() {
		return structure{}
	}
	feats := measureBars(f, g, duration)
	n := len(feats)
	if n < minSection {
		return structure{}
	}
	var s structure
	s.bars = make([]uint8, n)
	for i, b := range feats {
		s.bars[i] = uint8(math.Round(255 * b.intensity()))
	}

	start, end := musicSpan(f, g, duration, n)
	s.lastBar = end
	if start < 0 || end-start < minSection {
		return s
	}

	// How much the music changes at the start of every bar.
	s.novelty = make([]float64, n)
	for b := start + 1; b < end; b++ {
		w := min(window, b-start, end-b)
		s.novelty[b] = distance(meanFeatures(feats[b-w:b]), meanFeatures(feats[b:b+w]))
	}

	// The phrase grid: the bar offset (mod 4, then mod 8) with the most change.
	phase4 := strongestPhase(s.novelty, start, end, 4, 0)
	phase8 := strongestPhase(s.novelty, start, end, 8, phase4)
	s.barParity = strongestPhase(s.novelty, start, end, 2, 0)

	// A pickup — a bar or two of music before the first phrase — belongs to
	// the intro but does not move the phrase grid. (Three bars more likely
	// mean the music came in a bar late, which must not be skipped.)
	first := start
	if lead := (phase4 - start%4 + 4) % 4; lead > 0 && lead <= 2 && end-(start+lead) >= minSection {
		first = start + lead
	}

	// Candidates are the bars where the change peaks, scored higher on the
	// phrase grid; the strongest are kept at least a section apart.
	type candidate struct {
		bar   int
		score float64
	}
	var candidates []candidate
	for b := first + 1; b < end; b++ {
		v := s.novelty[b]
		if v < s.novelty[b-1] || (b+1 < n && v < s.novelty[b+1]) {
			continue // not a peak
		}
		switch {
		case (b-phase8)%8 == 0:
			v *= 1.5
		case (b-phase4)%4 == 0:
			v *= 1.25
		default:
			v *= 0.8 // sections rarely start off the phrase grid
		}
		candidates = append(candidates, candidate{b, v})
	}
	slices.SortFunc(candidates, func(a, b candidate) int { return cmpFloat(b.score, a.score) })
	var cuts []int
	for _, c := range candidates {
		if c.score < minBoundary {
			break
		}
		if c.bar-first < minSection || end-c.bar < minSection {
			continue
		}
		if !slices.ContainsFunc(cuts, func(cut int) bool { return abs(cut-c.bar) < minSection }) {
			cuts = append(cuts, c.bar)
		}
	}
	slices.Sort(cuts)

	s.sections = nameSections(feats, append(append([]int{first}, cuts...), end))
	return s
}

// nameSections gives every span between bounds its kind. Drops are the
// loudest sections, overall and in the bass, within a dB or so of each
// other; a busy intro or a verse without the bass line is a couple of dB
// below. Breaks lose the kick or most of their intensity. The first and the
// last span are the intro and the outro unless they are drops. Neighbours of
// the same kind are merged.
func nameSections(feats []barFeatures, bounds []int) []Section {
	parts := len(bounds) - 1
	m := make([]barFeatures, parts)
	strongest, hardestKick := 0.0, 0.0
	loudest, deepest := math.Inf(-1), math.Inf(-1)
	for i := range parts {
		m[i] = meanFeatures(feats[bounds[i]:bounds[i+1]])
		strongest = max(strongest, m[i].intensity())
		hardestKick = max(hardestKick, m[i].kick)
		loudest, deepest = max(loudest, m[i].loudDB), max(deepest, m[i].lowDB)
	}
	var sections []Section
	for i := range parts {
		kind := Verse
		switch {
		case m[i].loudDB >= loudest-dropMargin && m[i].lowDB >= deepest-dropMargin:
			kind = Drop
		case i == 0:
			kind = Intro
		case i == parts-1:
			kind = Outro
		case m[i].intensity() < breakShare*strongest || m[i].kick < kickPresent*hardestKick:
			kind = Break
		}
		if len(sections) > 0 && sections[len(sections)-1].Kind == kind {
			continue
		}
		sections = append(sections, Section{Bar: bounds[i], Kind: kind})
	}
	return sections
}

// measureBars describes every whole bar of the track.
func measureBars(f frameSeries, g Grid, duration float64) []barFeatures {
	beat := g.BeatLength()
	margin := float64(onsetFrame/2) / float64(f.rate)
	type raw struct{ loud, low, mid, high, kick, flux float64 }
	var bars []raw
	for n := 0; g.BarTime(n+1) <= duration+0.01; n++ {
		from, to := g.BarTime(n), g.BarTime(n+1)
		low, mid, high := f.mean(f.low, from, to), f.mean(f.mid, from, to), f.mean(f.high, from, to)
		// The kick: how much bass power each beat adds to what sounded just
		// before it. Taken as a difference of power, not of decibels, a
		// sustained bass line cancels out, so the kick counts the same in a
		// drop full of bass as in a bare intro.
		kick := 0.0
		for k := range 4 {
			t := from + float64(k)*beat
			before := f.mean(f.low, t-beat/4, t-margin)
			after := f.mean(f.low, t+margin, t+beat/4)
			kick += max(0, after-before)
		}
		bars = append(bars, raw{
			loud: decibels(math.Sqrt(low + mid + high)),
			low:  decibels(math.Sqrt(low)),
			mid:  decibels(math.Sqrt(mid)),
			high: decibels(math.Sqrt(high)),
			kick: kick / 4,
			flux: f.mean(f.flux, from, to),
		})
	}
	if len(bars) == 0 {
		return nil
	}

	// Levels are measured against the loud parts of this track (its 95th
	// percentile), so a quiet master and a loud one look the same.
	top := func(get func(raw) float64) float64 {
		v := make([]float64, len(bars))
		for i, b := range bars {
			v[i] = get(b)
		}
		return percentile(v, 0.95)
	}
	loudTop := top(func(b raw) float64 { return b.loud })
	lowTop := top(func(b raw) float64 { return b.low })
	midTop := top(func(b raw) float64 { return b.mid })
	highTop := top(func(b raw) float64 { return b.high })
	kickTop := top(func(b raw) float64 { return b.kick })
	fluxTop := top(func(b raw) float64 { return b.flux })
	level := func(v, top, rangeDB float64) float64 { return clamp01((v - top + rangeDB) / rangeDB) }
	ratio := func(v, top float64) float64 {
		if top <= 0 {
			return 0
		}
		return clamp01(v / top)
	}

	out := make([]barFeatures, len(bars))
	for i, b := range bars {
		out[i] = barFeatures{
			loud: level(b.loud, loudTop, 18),
			low:  level(b.low, lowTop, 24),
			mid:  level(b.mid, midTop, 24),
			high: level(b.high, highTop, 24),
			kick: ratio(b.kick, kickTop),
			flux: ratio(b.flux, fluxTop),
			// Floored like the 0–1 levels, so a silent bar does not drag a
			// section's average down without end.
			loudDB: max(b.loud-loudTop, -40),
			lowDB:  max(b.low-lowTop, -40),
		}
	}
	return out
}

// musicSpan is the first bar with music and the first bar after it.
func musicSpan(f frameSeries, g Grid, duration float64, n int) (start, end int) {
	loud := make([]float64, n)
	for i := range n {
		from, to := g.BarTime(i), g.BarTime(i+1)
		loud[i] = decibels(math.Sqrt(f.mean(f.low, from, to) + f.mean(f.mid, from, to) + f.mean(f.high, from, to)))
	}
	top := percentile(loud, 0.95)
	start, end = -1, -1
	for i, l := range loud {
		if l > top-silenceBelow {
			if start < 0 {
				start = i
			}
			end = i + 1
		}
	}
	return start, end
}

// strongestPhase is the offset, modulo period, at which the novelty is
// greatest; with a hint, only the offsets congruent to it modulo
// period/2 compete.
func strongestPhase(novelty []float64, start, end, period, hint int) int {
	score := make([]float64, period)
	for b := start + 1; b < end; b++ {
		score[b%period] += novelty[b] * novelty[b]
	}
	best := -1
	for p := range period {
		if period >= 8 && p%(period/2) != hint%(period/2) {
			continue
		}
		if best < 0 || score[p] > score[best] {
			best = p
		}
	}
	return best
}

func meanFeatures(bars []barFeatures) barFeatures {
	var m barFeatures
	for _, b := range bars {
		m.loud += b.loud
		m.low += b.low
		m.mid += b.mid
		m.high += b.high
		m.kick += b.kick
		m.flux += b.flux
		m.loudDB += b.loudDB
		m.lowDB += b.lowDB
	}
	k := float64(max(1, len(bars)))
	return barFeatures{m.loud / k, m.low / k, m.mid / k, m.high / k, m.kick / k, m.flux / k, m.loudDB / k, m.lowDB / k}
}

// distance is the root-mean-square difference of two bars' features.
func distance(a, b barFeatures) float64 {
	va, vb := a.vector(), b.vector()
	sum := 0.0
	for i := range va {
		d := va[i] - vb[i]
		sum += d * d
	}
	return math.Sqrt(sum / float64(len(va)))
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
