package analysis

// InMeter returns the result with its grid and bars at the tempo the DJ
// reads the track at, given the lower bound of the tempo range (see
// FoldBPM).
//
// The grid is always laid at 88–176 BPM, where the drums of most dance music
// are clearest. A hip-hop track read at 75 BPM is gridded at 150, so one of
// its real bars spans two grid bars, and cue points counted in grid bars
// could land on beat 3. InMeter rebuilds the bars at the real tempo,
// starting on the grid bars where the music changes (BarParity). A hardcore
// track read at 190 BPM gets bars half as long instead.
func (r Result) InMeter(floor float64) Result {
	if !r.Grid.Valid() {
		return r
	}
	switch shown := FoldBPM(r.Grid.BPM, floor); {
	case shown < 0.75*r.Grid.BPM:
		return r.halfTime()
	case shown > 1.5*r.Grid.BPM:
		return r.doubleTime()
	}
	return r
}

// halfTime merges every two grid bars into one bar.
func (r Result) halfTime() Result {
	out := r
	// Real bar m spans grid bars off+2m and off+2m+1. The first real
	// downbeat may lie one grid bar before the grid's first downbeat.
	off := r.BarParity
	if off == 1 && r.Grid.BarTime(-1) >= 0 {
		off = -1
	}
	out.Grid.BPM = r.Grid.BPM / 2
	out.Grid.FirstBeat = r.Grid.BarTime(off)
	toReal := func(b int) int { return max(0, (b-off+1)/2) } // a section starting mid-bar starts with the next bar

	out.Bars = nil
	for m := 0; off+2*m < len(r.Bars); m++ {
		v := uint8(0)
		for _, b := range []int{off + 2*m, off + 2*m + 1} {
			if b >= 0 && b < len(r.Bars) {
				v = max(v, r.Bars[b])
			}
		}
		out.Bars = append(out.Bars, v)
	}
	out.Sections = nil
	for _, s := range r.Sections {
		s.Bar = toReal(s.Bar)
		if n := len(out.Sections); n > 0 && out.Sections[n-1].Bar >= s.Bar {
			out.Sections[n-1].Kind = s.Kind // two sections in one real bar: the later one wins
			continue
		}
		out.Sections = append(out.Sections, s)
	}
	out.LastBar = toReal(r.LastBar)
	out.BarParity = 0
	return out
}

// doubleTime splits every grid bar into two bars.
func (r Result) doubleTime() Result {
	out := r
	out.Grid.BPM = r.Grid.BPM * 2
	out.Bars = make([]uint8, 0, 2*len(r.Bars))
	for _, v := range r.Bars {
		out.Bars = append(out.Bars, v, v)
	}
	out.Sections = make([]Section, len(r.Sections))
	for i, s := range r.Sections {
		out.Sections[i] = Section{Bar: 2 * s.Bar, Kind: s.Kind}
	}
	out.LastBar = 2 * r.LastBar
	return out
}
