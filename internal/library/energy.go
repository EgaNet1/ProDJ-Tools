package library

import "sort"

// Energy rates every track from 0 (the calmest in the folder) to 1 (the most
// intense). Ratings are relative: the same track can rate differently in a
// chill folder and in a peak-time one, which is what ordering a set needs.
//
// Three measurements are combined, each by rank so that no single unit
// dominates: how loud the track is, how hard and dense its attacks are and
// how bright it sounds.
func Energy(tracks []*Track) map[string]float64 {
	rating := map[string]float64{}
	if len(tracks) == 0 {
		return rating
	}
	loud := ranks(tracks, func(t *Track) float64 { return t.Analysis.Loudness })
	punch := ranks(tracks, func(t *Track) float64 { return t.Analysis.Punch })
	bright := ranks(tracks, func(t *Track) float64 { return t.Analysis.Brightness })

	lo, hi := 1.0, 0.0
	combined := make([]float64, len(tracks))
	for i := range tracks {
		combined[i] = 0.45*loud[i] + 0.35*punch[i] + 0.2*bright[i]
		lo, hi = min(lo, combined[i]), max(hi, combined[i])
	}
	for i, t := range tracks {
		rating[t.ID] = 0.5
		if hi > lo {
			rating[t.ID] = (combined[i] - lo) / (hi - lo)
		}
	}
	return rating
}

// ranks maps each track's value to its position among all values, 0 to 1.
func ranks(tracks []*Track, value func(*Track) float64) []float64 {
	order := make([]int, len(tracks))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return value(tracks[order[a]]) < value(tracks[order[b]]) })
	out := make([]float64, len(tracks))
	for rank, i := range order {
		if len(tracks) > 1 {
			out[i] = float64(rank) / float64(len(tracks)-1)
		}
	}
	return out
}
