// Package mixdown renders a whole set into one audio file, every transition
// exactly as the Mixing tab plays it: B starts at A's tempo on the planned
// bar, its volume comes up by the middle of the blend, the bass swaps on the
// downbeat and A is gone by the end of the blend.
package mixdown

import (
	"math"
	"sort"

	"prodjtools/internal/analysis"
	"prodjtools/internal/mixplan"
)

// Rate is the sample rate of the mix.
const Rate = 44100

// Track is one track of the set.
type Track struct {
	Path          string
	Artist, Title string
	Duration      float64         // seconds
	Result        analysis.Result // in the DJ's meter (analysis.InMeter)
}

// Levels of the transition, the same as the preview in the Mixing tab
// (playTransition in web/app.js).
const (
	bassCutDB = -26  // low shelf gain while a bass line is out
	bassShelf = 150  // Hz, corner of the low shelf
	bassRamp  = 0.01 // seconds the bass takes to swap
	cutFade   = 0.005
	// Without a plan (no beat grid) tracks crossfade over this many seconds.
	plainCrossfade = 8.0
	// After A is gone, B glides back to its own tempo over this many of its
	// bars, so that its next transition starts at the tempo its plan
	// assumes.
	rideBars = 8
)

// Clip is a track placed on the timeline of the mix. Times without a suffix
// are seconds of the mix; source times are seconds of the track's file.
type Clip struct {
	Track      int
	Start, End float64
	rate       []rateSeg // playback speed over mix time, 1 = as recorded
	gain       []point   // volume, linear
	bass       []point   // low shelf gain, dB
}

type point struct{ t, v float64 }

// rateSeg is a stretch of mix time over which the playback speed changes
// linearly from r0 to r1; src0 is the source time at t0. The last segment
// runs on forever.
type rateSeg struct{ t0, t1, r0, r1, src0 float64 }

// Join is where a transition sits in the mix, for the interface.
type Join struct {
	Way   string  `json:"way"`   // mixplan.ViaOutro, mixplan.Early, or "" for a plain crossfade
	Start float64 `json:"start"` // B comes in
	Swap  float64 `json:"swap"`  // the bass swaps
	End   float64 `json:"end"`   // A is gone
}

// Timeline is the whole mix.
type Timeline struct {
	Clips  []Clip
	Joins  []Join
	Length float64
}

// Build lays out a set. plans[i] leads from track i to track i+1; nil means
// the pair has no beat grid and crossfades instead.
func Build(tracks []Track, plans []*mixplan.Transition) Timeline {
	var tl Timeline
	if len(tracks) == 0 {
		return tl
	}
	clips := make([]Clip, len(tracks))
	first := mixIn(tracks[0])
	clips[0] = Clip{Track: 0, rate: []rateSeg{{0, math.Inf(1), 1, 1, first}}, gain: []point{{0, 0}, {cutFade, 1}}, bass: []point{{0, 0}}}

	for i := 0; i+1 < len(tracks); i++ {
		a, b := &clips[i], &clips[i+1]
		*b = Clip{Track: i + 1}
		p := plans[i]
		if p == nil {
			end := a.outAt(musicEnd(tracks[i]))
			start := max(a.Start, end-plainCrossfade)
			a.End = end
			a.gain = append(a.gain, point{start, a.gainAt(start)}, point{end, 0})
			b.Start = start
			b.rate = []rateSeg{{start, math.Inf(1), 1, 1, mixIn(tracks[i+1])}}
			b.gain = []point{{start, 0}, {end, 1}}
			b.bass = []point{{start, 0}}
			tl.Joins = append(tl.Joins, Join{Start: start, Swap: (start + end) / 2, End: end})
			continue
		}

		// The plan's times are rounded to milliseconds; the grids give the
		// exact bars, so that B's drop lands on A's bar to the sample.
		gridA, gridB := tracks[i].Result.Grid, tracks[i+1].Result.Grid
		start := max(a.outAt(gridA.BarTime(p.StartA.Bar-1)), a.Start)
		swap := max(a.outAt(gridA.BarTime(p.SwapA.Bar-1)), start)
		end := max(a.outAt(gridA.BarTime(p.EndA.Bar-1)), swap+cutFade)
		speed := a.rateAt(start) * foldRatio(tracks[i].Result.Grid.BPM/tracks[i+1].Result.Grid.BPM)

		// A: full until the swap, bass out at the swap, gone by the end.
		a.End = end
		a.gain = append(a.gain, point{swap, 1}, point{end, 0})
		a.bass = append(a.bass, point{swap - bassRamp, 0}, point{swap, bassCutDB})

		// B: from its cue at A's tempo, volume up by the middle of the
		// blend, bass in at the swap.
		b.Start = start
		b.rate = []rateSeg{{start, math.Inf(1), speed, speed, gridB.BarTime(p.CueB.Bar - 1)}}
		b.gain = []point{{start, 0}, {(start + swap) / 2, 1}}
		b.bass = []point{{start, bassCutDB}, {swap - bassRamp, bassCutDB}, {swap, 0}}
		tl.Joins = append(tl.Joins, Join{Way: p.Kind, Start: start, Swap: swap, End: end})

		// Back to B's own tempo once A is gone, finishing a bar before B's
		// own transition begins.
		if math.Abs(speed-1) > 1e-4 {
			bar := gridB.BarLength()
			until := musicEnd(tracks[i+1])
			if i+1 < len(plans) && plans[i+1] != nil {
				until = gridB.BarTime(plans[i+1].StartA.Bar - 1)
			}
			room := until - bar - b.srcAt(end) // source seconds left for the glide
			if glide := min(rideBars*bar, room); glide >= 2*bar {
				b.ride(end, glide)
			}
		}
	}

	last := &clips[len(clips)-1]
	last.End = last.outAt(musicEnd(tracks[len(tracks)-1]))
	fade := min(2.0, (last.End-last.Start)/4)
	last.gain = append(last.gain, point{last.End - fade, last.gainAt(last.End - fade)}, point{last.End, 0})

	for i := range clips {
		sortPoints(clips[i].gain)
		sortPoints(clips[i].bass)
	}
	tl.Clips = clips
	tl.Length = last.End
	return tl
}

// ride glides the clip's speed back to 1 from mix time t, covering src
// seconds of the track.
func (c *Clip) ride(t, src float64) {
	last := &c.rate[len(c.rate)-1]
	r := last.r0
	at := c.srcAt(t)
	last.t1, last.r1 = t, r
	// With speed changing linearly from r to 1, the track advances by
	// duration·(r+1)/2.
	d := 2 * src / (r + 1)
	c.rate = append(c.rate, rateSeg{t, t + d, r, 1, at}, rateSeg{t + d, math.Inf(1), 1, 1, at + src})
}

// srcAt is the source time playing at mix time t.
func (c *Clip) srcAt(t float64) float64 {
	s := c.segAt(t)
	dt := t - s.t0
	if math.IsInf(s.t1, 1) || s.r0 == s.r1 {
		return s.src0 + s.r0*dt
	}
	return s.src0 + s.r0*dt + (s.r1-s.r0)*dt*dt/(2*(s.t1-s.t0))
}

// rateAt is the playback speed at mix time t.
func (c *Clip) rateAt(t float64) float64 {
	s := c.segAt(t)
	if math.IsInf(s.t1, 1) || s.r0 == s.r1 {
		return s.r0
	}
	return s.r0 + (s.r1-s.r0)*(t-s.t0)/(s.t1-s.t0)
}

func (c *Clip) segAt(t float64) rateSeg {
	for _, s := range c.rate {
		if t < s.t1 {
			return s
		}
	}
	return c.rate[len(c.rate)-1]
}

// outAt is the mix time at which source time src plays.
func (c *Clip) outAt(src float64) float64 {
	for _, s := range c.rate {
		var srcEnd float64
		if math.IsInf(s.t1, 1) {
			srcEnd = math.Inf(1)
		} else {
			srcEnd = s.src0 + (s.r0+s.r1)/2*(s.t1-s.t0)
		}
		if src > srcEnd {
			continue
		}
		d := src - s.src0
		if math.IsInf(s.t1, 1) || s.r0 == s.r1 {
			return s.t0 + d/s.r0
		}
		// d = r0·x + k·x², k = (r1−r0)/(2·(t1−t0)): the root in [0, t1−t0].
		k := (s.r1 - s.r0) / (2 * (s.t1 - s.t0))
		x := (-s.r0 + math.Sqrt(s.r0*s.r0+4*k*d)) / (2 * k)
		return s.t0 + x
	}
	return math.Inf(1)
}

func (c *Clip) gainAt(t float64) float64 { return envelope(c.gain, t) }
func (c *Clip) bassAt(t float64) float64 { return envelope(c.bass, t) }

// envelope interpolates linearly between points; before the first and
// after the last it holds their values.
func envelope(points []point, t float64) float64 {
	if len(points) == 0 {
		return 1
	}
	if t <= points[0].t {
		return points[0].v
	}
	for i := 1; i < len(points); i++ {
		if t < points[i].t {
			p, q := points[i-1], points[i]
			return p.v + (q.v-p.v)*(t-p.t)/(q.t-p.t)
		}
	}
	return points[len(points)-1].v
}

func sortPoints(p []point) { sort.SliceStable(p, func(i, j int) bool { return p[i].t < p[j].t }) }

// mixIn is where a track's music starts: its MIX IN cue, or the start of
// the file when it has no beat grid.
func mixIn(t Track) float64 {
	if cues := mixplan.Cues(t.Result); len(cues) > 0 && cues[0].Slot == "A" {
		return cues[0].Time
	}
	return 0
}

// musicEnd is where a track's music ends: a bar after its last bar, or the
// end of the file.
func musicEnd(t Track) float64 {
	g := t.Result.Grid
	if !g.Valid() || t.Result.LastBar == 0 {
		return t.Duration
	}
	return min(t.Duration, g.BarTime(t.Result.LastBar)+g.BarLength())
}

// foldRatio brings a tempo ratio into [0.75, 1.5), as the plans do.
func foldRatio(r float64) float64 {
	for r < 0.75 {
		r *= 2
	}
	for r >= 1.5 {
		r /= 2
	}
	return r
}
