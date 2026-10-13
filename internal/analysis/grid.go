package analysis

import (
	"math"
	"slices"
)

// Grid is the beat grid: where every beat and every bar of a track falls,
// the way Rekordbox draws it over the waveform.
type Grid struct {
	BPM       float64 `json:"bpm"`       // quarter notes per minute
	FirstBeat float64 `json:"firstBeat"` // seconds to the first downbeat in the file
	Jitter    float64 `json:"jitter"`    // ms, typical distance of single drum hits from the grid; 0 = unknown
	Downbeat  float64 `json:"downbeat"`  // 0–1, how clearly the downbeat stood out
}

// Valid reports whether a grid was found.
func (g Grid) Valid() bool { return g.BPM > 0 }

// BeatLength is the length of one beat, in seconds.
func (g Grid) BeatLength() float64 { return 60 / g.BPM }

// BarLength is the length of one 4/4 bar, in seconds.
func (g Grid) BarLength() float64 { return 240 / g.BPM }

// DownbeatSure reports whether the downbeat was clear from the music itself
// rather than taken from where the music starts.
func (g Grid) DownbeatSure() bool { return g.Downbeat >= closeVote }

// BarTime is when bar n starts; bar 0 starts at FirstBeat.
func (g Grid) BarTime(n int) float64 { return g.FirstBeat + float64(n)*g.BarLength() }

const (
	// Grids are laid on quarter notes in the 88–176 BPM range, where 4/4
	// dance music lives, whatever octave the tempo is shown in.
	gridFloor = 88

	attackHop  = 16 // samples (≈ 1.5 ms) between points of the attack curve
	attackSpan = 24 // samples (≈ 2 ms) compared before and after an attack
	foldBins   = 256

	searchWidth = 0.6    // BPM searched either side of the tempo estimate
	coarseStep  = 0.005  // BPM
	fineStep    = 0.0005 // BPM
	wholeBPMTol = 0.97   // a whole BPM wins if it explains the beats this well

	jitterRadius = 0.012 // seconds searched around each beat when measuring jitter
	minHits      = 16
)

// findGrid lays a constant-tempo beat grid over the track.
//
// Single drum hits are hard to place precisely in a dense mix: bass,
// hi-hats and synths all have attacks too. So instead of placing beats one
// by one, the whole track is folded onto a single beat — every moment added
// to the point of the beat where it falls. With the exact tempo the attacks
// of hundreds of beats pile up in one place; with a tempo off by a few
// thousandths of a BPM they smear out over the length of the track. That
// gives the tempo to about a thousandth of a BPM, and the folded attack —
// an average of every beat in the track — gives the phase to a fraction of
// a millisecond. The downbeat is chosen last.
func findGrid(x []float32, f frameSeries, rawBPM, duration float64) Grid {
	if rawBPM <= 0 {
		return Grid{}
	}
	bpm := rawBPM
	for bpm < gridFloor {
		bpm *= 2
	}
	for bpm >= 2*gridFloor {
		bpm /= 2
	}

	curve := attackCurve(x)
	frameSec := float64(attackHop) / float64(f.rate)
	bpm = bestTempo(curve, frameSec, bpm)
	bpm = checkRelatives(curve, frameSec, bpm)
	period := 60 / bpm
	phase := attackPhase(x, f, period)

	g := Grid{BPM: bpm, FirstBeat: phase, Jitter: jitter(x, f.rate, phase, period)}
	g.FirstBeat, g.Downbeat = findDownbeat(f, g, duration)
	return g
}

// hpEnergy is the mean energy of the sample-to-sample differences in
// x[from:to]. Differencing is a simple high-pass filter: it keeps the click
// of a kick or a snare and removes the bass, whose slow waves would
// otherwise make short-term energy ripple.
func hpEnergy(x []float32, from, to int) float64 {
	from, to = max(from, 1), min(to, len(x))
	if to <= from {
		return 0
	}
	sum := 0.0
	for n := from; n < to; n++ {
		d := float64(x[n] - x[n-1])
		sum += d * d
	}
	return sum / float64(to-from)
}

// attackCurve says, every attackHop samples, how sharply the high-passed
// energy rises (in log units; 0 when it does not rise).
func attackCurve(x []float32) []float64 {
	out := make([]float64, len(x)/attackHop)
	prev := 0.0
	for i := range out {
		from := i * attackHop
		e := math.Log(hpEnergy(x, from, from+2*attackHop) + 1e-10)
		if i > 0 && e > prev {
			out[i] = e - prev
		}
		prev = e
	}
	return out
}

// foldScore folds the attack curve onto one beat of the given length and
// returns how sharp the highest pile of attacks is: its height relative to
// the average.
func foldScore(curve []float64, frameSec, bpm float64, bins []float64) float64 {
	clear(bins)
	step := frameSec / (60 / bpm) * foldBins // bins advanced per point
	pos, total := 0.0, 0.0
	for _, v := range curve {
		if v > 0 {
			bins[int(pos)] += v
			total += v
		}
		if pos += step; pos >= foldBins {
			pos -= foldBins
		}
	}
	if total == 0 {
		return 0
	}
	peak := 0.0
	for j := range bins {
		peak = max(peak, bins[(j+foldBins-1)%foldBins]+bins[j]+bins[(j+1)%foldBins])
	}
	return peak / (3 * total / foldBins)
}

// bestTempo searches around the estimate for the tempo whose folding piles
// the attacks up most sharply, and prefers a whole BPM when it does almost
// as well: tracks are produced at whole tempos far more often than not.
func bestTempo(curve []float64, frameSec, estimate float64) float64 {
	bins := make([]float64, foldBins)
	search := func(from, to, step float64) (best, score float64) {
		for bpm := from; bpm <= to; bpm += step {
			if s := foldScore(curve, frameSec, bpm, bins); s > score {
				best, score = bpm, s
			}
		}
		return best, score
	}
	coarse, _ := search(estimate-searchWidth, estimate+searchWidth, coarseStep)
	best, score := search(coarse-2*coarseStep, coarse+2*coarseStep, fineStep)
	if whole := math.Round(best); math.Abs(best-whole) < 0.05 && foldScore(curve, frameSec, whole, bins) >= wholeBPMTol*score {
		return whole
	}
	return math.Round(best*10000) / 10000
}

// checkRelatives guards against the rough estimate locking onto a rhythm
// related to the beat by 3:4 or 2:3 rather than by an octave. House with
// hats and bass on every off-beat repeats every 1.5 and 3 beats as strongly
// as every beat, and so can pass for two thirds of its tempo. Folding tells
// them apart: at the true tempo every kick lands on the same point of the
// beat, at a related one the kicks spread over three or four points. A
// related tempo replaces the estimate only when its attacks pile up clearly
// more sharply.
func checkRelatives(curve []float64, frameSec, bpm float64) float64 {
	bins := make([]float64, foldBins)
	best, top := bpm, foldScore(curve, frameSec, bpm, bins)
	for _, ratio := range []float64{3.0 / 4, 4.0 / 3, 2.0 / 3, 3.0 / 2} {
		c := bpm * ratio
		for c < gridFloor {
			c *= 2
		}
		for c >= 2*gridFloor {
			c /= 2
		}
		// The ratio carries the estimate's small error, so look around it.
		for b := c - 0.1; b <= c+0.1; b += 0.0025 {
			if s := foldScore(curve, frameSec, b, bins); s > top*relativeMargin {
				best, top = b, s/relativeMargin
			}
		}
	}
	if best == bpm {
		return bpm
	}
	return bestTempo(curve, frameSec, best)
}

// attackPhase folds the high-passed signal onto one beat sample by sample —
// an average of every beat in the track — and returns where in the beat the
// drums hit. Rekordbox and CDJs put a beat where the attack begins, so after
// finding the steepest rise the start of that rise is traced back.
func attackPhase(x []float32, f frameSeries, period float64) float64 {
	rate := float64(f.rate)
	perBeat := period * rate // samples, fractional
	n := int(math.Ceil(perBeat))
	folded := make([]float64, n)
	counts := make([]float64, n)
	pos := 0.0
	for i := 1; i < len(x); i++ {
		if pos += 1; pos >= perBeat {
			pos -= perBeat
		}
		d := float64(x[i] - x[i-1])
		folded[int(pos)] += d * d
		counts[int(pos)]++
	}
	for i := range folded {
		if counts[i] > 0 {
			folded[i] /= counts[i]
		}
	}
	at := func(i int) float64 { return folded[(i%n+n)%n] }
	sum := func(from, to int) float64 {
		s := 0.0
		for i := from; i < to; i++ {
			s += at(i)
		}
		return s
	}
	// steepest returns the position in [from, to) where the energy after it
	// most exceeds the energy before it. The rise is taken in absolute terms,
	// so that kick, snare and hats landing together beat a lone hi-hat.
	steepest := func(from, to int) int {
		best, where := math.Inf(-1), from
		for i := from; i < to; i++ {
			if rise := sum(i, i+attackSpan) - sum(i-attackSpan, i); rise > best {
				best, where = rise, i
			}
		}
		return (where%n + n) % n
	}
	beat := steepest(0, n)

	// Hi-hats often sit between the beats and are bright, so now and then the
	// sharpest attack is the off-beat. The kick decides: if the bass hits
	// clearly harder half a beat away, the beat is there.
	kick := kickProfile(f, period)
	if kick(float64(beat)/rate+period/2) > offBeatMargin*kick(float64(beat)/rate) {
		centre := beat + n/2
		beat = steepest(centre-n/8, centre+n/8)
	}

	// Trace the rise back to where it begins: the first point, at most
	// attackTrace before, at which the smoothed energy is already a tenth of
	// the way from the level before the hit to its peak.
	smooth := func(i int) float64 { return sum(i-4, i+4) / 8 }
	base := sum(beat-3*attackSpan, beat-attackSpan) / float64(2*attackSpan)
	peak := 0.0
	for i := beat; i < beat+2*attackSpan; i++ {
		peak = max(peak, smooth(i))
	}
	threshold := base + 0.1*(peak-base)
	start := beat
	for i := beat - 1; i > beat-int(attackTrace*rate) && smooth(i) > threshold; i-- {
		start = i
	}
	return float64(start) / rate
}

// kickProfile folds the rises in bass power onto one beat and returns how
// strongly the kick hits around a moment of the beat (within an eighth of a
// beat either side, which absorbs the coarse timing of bass frames).
func kickProfile(f frameSeries, period float64) func(t float64) float64 {
	bins := make([]float64, foldBins)
	prev := 0.0
	for i, p := range f.low {
		v := math.Log(p + 1e-12)
		if i > 0 && v > prev {
			pos := math.Mod(f.time(float64(i)), period) / period * foldBins
			bins[int(pos)%foldBins] += v - prev
		}
		prev = v
	}
	return func(t float64) float64 {
		centre := int(math.Mod(t, period) / period * foldBins)
		strongest := 0.0
		for d := -foldBins / 8; d <= foldBins/8; d++ {
			strongest = max(strongest, bins[((centre+d)%foldBins+foldBins)%foldBins])
		}
		return strongest
	}
}

// jitter measures how closely single drum hits follow the grid: the median
// distance, in ms, between each beat and the strongest attack within
// jitterRadius of it. 0 means too few hits to tell.
func jitter(x []float32, rate int, phase, period float64) float64 {
	const minRise = 4.0 // +6 dB
	radius := int(jitterRadius * float64(rate))
	var distances []float64
	for t := phase; int(t*float64(rate))+radius+attackSpan < len(x); t += period {
		centre := int(math.Round(t * float64(rate)))
		best, at := minRise, -1
		for n := max(attackSpan, centre-radius); n <= centre+radius; n++ {
			after := hpEnergy(x, n, n+attackSpan)
			before := hpEnergy(x, n-attackSpan, n)
			if rise := (after + 1e-12) / (before + 1e-12); rise > best {
				best, at = rise, n
			}
		}
		if at >= 0 {
			distances = append(distances, math.Abs(float64(at)-t*float64(rate))/float64(rate))
		}
	}
	if len(distances) < minHits {
		return 0
	}
	slices.Sort(distances)
	return math.Round(distances[len(distances)/2]*1e4) / 10
}

// findDownbeat decides which beat of the bar is "1" and returns the time of
// the first downbeat in the file. It asks three questions in turn and stops
// at the first clear answer:
//
//  1. Where does the bass come in hardest? Drops land on "1", so the
//     strongest jumps in bass level vote for their beat of the bar.
//  2. Where does the sound grow the most, across all bands? New
//     instruments and sections come in on "1" too. Only growth counts: the
//     cut before a drop, often on the last beat of a bar, would otherwise
//     vote for the wrong beat.
//  3. Where does the music start? Tracks almost always start on bar 1.
func findDownbeat(f frameSeries, g Grid, duration float64) (first, confidence float64) {
	beat := g.BeatLength()
	start := math.Mod(g.FirstBeat, beat)
	if start < 0 {
		start += beat
	}

	// Band levels of every beat in dB, floored 60 dB below the loudest so
	// that silence does not produce endless differences. Only frames lying
	// wholly inside a beat are used, so that the attack of the next beat
	// does not leak into this one.
	var peak float64
	for i := range f.low {
		peak = max(peak, f.total(i))
	}
	floor := peak * 1e-6
	level := func(p float64) float64 { return 10 * math.Log10(max(p, floor)) }
	margin := float64(onsetFrame/2) / float64(f.rate)
	var beats [][3]float64
	for t := start; t+beat <= duration; t += beat {
		from, to := t+margin, t+beat-margin
		beats = append(beats, [3]float64{
			level(f.mean(f.low, from, to)),
			level(f.mean(f.mid, from, to)),
			level(f.mean(f.high, from, to)),
		})
	}

	// 1. The strongest entries of the bass.
	type jump struct {
		beat int
		size float64
	}
	var jumps []jump
	for k := 1; k < len(beats); k++ {
		if d := beats[k][0] - beats[k-1][0]; d > 0 {
			jumps = append(jumps, jump{k, d})
		}
	}
	slices.SortFunc(jumps, func(a, b jump) int { return cmpFloat(b.size, a.size) })
	var bass [4]float64
	for _, j := range jumps[:min(len(jumps), max(8, len(beats)/32))] {
		bass[j.beat%4] += j.size * j.size
	}
	best, confidence := winner(bass)

	// 2. Growth across all bands.
	if confidence < closeVote {
		var growth [4]float64
		for k := 1; k < len(beats); k++ {
			g := 0.0
			for band := range 3 {
				g += max(0, beats[k][band]-beats[k-1][band])
			}
			growth[k%4] += g * g
		}
		best, confidence = winner(growth)
	}

	// 3. The start of the music.
	if confidence < closeVote {
		loudest := math.Inf(-1)
		for _, b := range beats {
			loudest = max(loudest, b[0], b[1], b[2])
		}
		for k, b := range beats {
			if max(b[0], b[1], b[2]) > loudest-silenceBelow {
				best = k % 4
				break
			}
		}
	}
	return start + float64(best)*beat, math.Round(confidence*100) / 100
}

// winner picks the beat of the bar with the highest score; confidence is
// how far it leads the runner-up (0 = tie, 1 = no contest).
func winner(score [4]float64) (best int, confidence float64) {
	for p := range 4 {
		if score[p] > score[best] {
			best = p
		}
	}
	second := 0.0
	for p := range 4 {
		if p != best {
			second = max(second, score[p])
		}
	}
	if score[best] > 0 {
		confidence = 1 - second/score[best]
	}
	return best, confidence
}

const (
	// closeVote: below this confidence the next question decides the downbeat.
	closeVote = 0.15
	// offBeatMargin: how much harder the kick must hit half a beat away to
	// move the grid there.
	offBeatMargin = 1.5
	// attackTrace: how far back the start of an attack is looked for.
	attackTrace = 0.005
	// relativeMargin: how much more sharply a 3:4 or 2:3 related tempo must
	// fold the attacks to replace the estimate.
	relativeMargin = 1.15
)
