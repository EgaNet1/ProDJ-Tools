package setlist

import (
	"cmp"
	"math"
	"slices"

	"prodjtools/internal/camelot"
)

// Suggestion is a candidate for the next track.
type Suggestion struct {
	ID          string     `json:"id"`
	Transition  Transition `json:"transition"`
	EnergyShift float64    `json:"energyShift"` // next track's energy minus the current one's
}

// Suggest ranks candidates as the track to play after from. direction says
// where the energy should go: -1 down, 0 stay, +1 up.
func Suggest(from Track, candidates []Track, direction int, w Weights, limit int) []Suggestion {
	desired := 0.15 * float64(direction)
	type scored struct {
		Suggestion
		cost float64
	}
	var list []scored
	for _, c := range candidates {
		if c.ID == from.ID {
			continue
		}
		shift := c.Energy - from.Energy
		cost := w.Tempo*tempoStepCost*math.Abs(tempoChange(from.BPM, c.BPM)) +
			w.Harmony*keyCost*camelot.Relate(from.Key, c.Key).Cost() +
			w.Energy*energyCurveCost/2*math.Abs(shift-desired)
		list = append(list, scored{Suggestion{c.ID, Judge(from, c), math.Round(shift*100) / 100}, cost})
	}
	slices.SortStableFunc(list, func(a, b scored) int { return cmp.Compare(a.cost, b.cost) })

	out := make([]Suggestion, 0, limit)
	for _, s := range list[:min(limit, len(list))] {
		out = append(out, s.Suggestion)
	}
	return out
}
