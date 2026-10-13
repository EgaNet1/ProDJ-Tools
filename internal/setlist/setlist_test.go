package setlist

import (
	"fmt"
	"slices"
	"testing"

	"prodjtools/internal/camelot"
)

func crate(n int) []Track {
	tracks := make([]Track, n)
	for i := range tracks {
		tracks[i] = Track{
			ID:       fmt.Sprint("t", i),
			BPM:      120 + float64((i*7)%20),
			Key:      camelot.Key{Number: (i*5)%12 + 1, Minor: i%2 == 0},
			Energy:   float64((i*3)%n) / float64(n),
			Duration: 300,
		}
	}
	return tracks
}

func ids(tracks []Track) []string {
	out := make([]string, len(tracks))
	for i, t := range tracks {
		out[i] = t.ID
	}
	return out
}

func TestBuildIsAPermutation(t *testing.T) {
	in := crate(25)
	out := Build(in, Options{Curve: Curves[0], Weights: DefaultWeights})
	got, want := ids(out), ids(in)
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("Build lost or duplicated tracks: %v", got)
	}
}

func TestBuildRespectsPins(t *testing.T) {
	in := crate(20)
	pins := map[string]int{"t7": 0, "t3": 19, "t11": 10}
	out := Build(in, Options{Curve: Curves[0], Weights: DefaultWeights, Pins: pins})
	for id, pos := range pins {
		if out[pos].ID != id {
			t.Errorf("%s should be at %d, found %s there", id, pos, out[pos].ID)
		}
	}
}

func TestBuildBeatsInputOrder(t *testing.T) {
	in := crate(20)
	opt := Options{Curve: Curves[0], Weights: DefaultWeights}
	p := newProblem(in, opt)
	identity := make([]int, len(in))
	for i := range identity {
		identity[i] = i
	}
	before := p.cost(identity, p.goals)

	out := Build(in, opt)
	q := newProblem(out, opt)
	if after := q.cost(identity, q.goals); after >= before {
		t.Errorf("optimised cost %.2f is not below input cost %.2f", after, before)
	}
}

func TestBuildForFitsLength(t *testing.T) {
	in := crate(20) // 20 × 5 min
	out := BuildFor(in, 60*60, 30, Options{Curve: Curves[0], Weights: DefaultWeights})
	if got := mixLength(out, 30); got > 60*60 {
		t.Errorf("mix is %.0f s, longer than an hour", got)
	}
	if len(out) < 10 {
		t.Errorf("dropped too much: %d tracks left", len(out))
	}
}
