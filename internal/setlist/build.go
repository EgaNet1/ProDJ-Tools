package setlist

import (
	"math"
	"math/rand/v2"
	"runtime"
	"slices"
	"sync"

	"prodjtools/internal/camelot"
)

// How much each kind of imperfection costs. At default weights a key clash
// (5 × 1.2 = 6) hurts about as much as a 10% tempo jump or as a track sitting
// a whole unit away from the energy curve.
//
// Tempo is the backbone of the set: it follows the curve strongly and is
// discouraged from stepping against it, because DJs rarely let the tempo
// wander back and forth and because tempo is measured far more reliably than
// "energy". Energy then shapes the order within each tempo range.
const (
	tempoStepCost    = 0.6 // per percent of tempo change between neighbours
	tempoAgainstCost = 3   // extra, per percent, when the tempo moves against the curve
	keyCost          = 1.2 // times camelot.Relation.Cost
	energyCurveCost  = 6   // per unit of distance between energy and curve
	tempoCurveCost   = 16  // per unit of distance between tempo rank and curve

	// Tempos this many percent apart or closer are a touch of the pitch
	// fader apart; tempoRanks treats them as one.
	pitchNudge = 1.5
)

// Build orders tracks into a set.
//
// It uses simulated annealing: start from a random order and keep proposing
// small changes — swap two tracks, or move one elsewhere. Improvements are
// always kept; worse orders are sometimes kept early on, less and less often
// later, so the search does not get stuck in the first decent order it finds.
// One search runs per CPU core, each from a different start, and the best
// result wins. The outcome is deterministic for the same input.
func Build(tracks []Track, opt Options) []Track {
	if len(tracks) < 2 {
		return slices.Clone(tracks)
	}
	p := newProblem(tracks, opt)

	results := make([]solution, runtime.NumCPU())
	var wg sync.WaitGroup
	for i := range results {
		wg.Go(func() {
			results[i] = p.anneal(rand.New(rand.NewPCG(42, uint64(i))))
		})
	}
	wg.Wait()

	best := slices.MinFunc(results, func(a, b solution) int {
		if a.cost < b.cost {
			return -1
		}
		if a.cost > b.cost {
			return 1
		}
		return 0
	})
	return p.ordered(best.order)
}

// BuildFor builds a set that fits into the given number of seconds. It orders
// the whole pool, then repeatedly drops the track whose absence leaves the
// best-flowing set until the mix fits, and finally re-orders what remains.
// Pinned tracks are never dropped.
func BuildFor(tracks []Track, seconds, overlap float64, opt Options) []Track {
	ordered := Build(tracks, opt)
	p := newProblem(ordered, opt)
	keep := make([]int, len(ordered))
	for i := range keep {
		keep[i] = i
	}
	for len(keep) > 1 && mixLength(p.ordered(keep), overlap) > seconds {
		g := goalsFor(opt.Curve, len(keep)-1)
		drop, lowest := -1, math.Inf(1)
		for i, t := range keep {
			if _, pinned := opt.Pins[ordered[t].ID]; pinned {
				continue
			}
			if c := p.cost(slices.Delete(slices.Clone(keep), i, i+1), g); c < lowest {
				drop, lowest = i, c
			}
		}
		if drop < 0 {
			break
		}
		keep = slices.Delete(keep, drop, drop+1)
	}
	return Build(p.ordered(keep), opt)
}

// problem holds everything the search needs, precomputed once.
type problem struct {
	tracks []Track
	w      Weights
	pair   [][]float64 // pair[i][j]: cost of mixing track i into track j
	energy []float64   // per track
	tempo  []float64   // per track, 0–1 by rank within this pool's tempos
	goals  goals       // per position, from the curve
	start  []int       // initial order, pinned tracks already in place
	free   []int       // positions the search may change
}

type solution struct {
	order []int
	cost  float64
}

func newProblem(tracks []Track, opt Options) *problem {
	n := len(tracks)
	p := &problem{
		tracks: tracks,
		w:      opt.Weights,
		pair:   make([][]float64, n),
		energy: make([]float64, n),
		tempo:  make([]float64, n),
		goals:  goalsFor(opt.Curve, n),
	}

	p.tempo = tempoRanks(tracks)
	for i, a := range tracks {
		p.energy[i] = a.Energy
		p.pair[i] = make([]float64, n)
		for j, b := range tracks {
			if i != j {
				p.pair[i][j] = p.w.Tempo*tempoStepCost*math.Abs(tempoChange(a.BPM, b.BPM)) +
					p.w.Harmony*keyCost*camelot.Relate(a.Key, b.Key).Cost()
			}
		}
	}

	// Put pinned tracks in their places, then fill the other positions.
	p.start = slices.Repeat([]int{-1}, n)
	var unpinned []int
	for i, t := range tracks {
		pos, pinned := opt.Pins[t.ID]
		pos = min(max(pos, 0), n-1)
		if pinned && p.start[pos] < 0 {
			p.start[pos] = i
		} else {
			unpinned = append(unpinned, i)
		}
	}
	for pos := range p.start {
		if p.start[pos] < 0 {
			p.start[pos], unpinned = unpinned[0], unpinned[1:]
			p.free = append(p.free, pos)
		}
	}
	return p
}

// tempoRanks places each track's tempo on a 0–1 scale by rank rather than
// by value, so one unusually fast track does not squeeze everyone else
// together. Tempos within 1.5% of each other — a nudge of the pitch fader —
// count as one and share a rank.
func tempoRanks(tracks []Track) []float64 {
	bpms := make([]float64, len(tracks))
	for i, t := range tracks {
		bpms[i] = t.BPM
	}
	slices.Sort(bpms)
	var groups []float64 // the slowest tempo of each group
	for _, b := range bpms {
		if len(groups) == 0 || b > groups[len(groups)-1]*(1+pitchNudge/100) {
			groups = append(groups, b)
		}
	}

	out := make([]float64, len(tracks))
	if len(groups) < 2 {
		return out
	}
	for i, t := range tracks {
		rank, found := slices.BinarySearch(groups, t.BPM)
		if !found {
			rank-- // inside the group that starts below it
		}
		out[i] = float64(rank) / float64(len(groups)-1)
	}
	return out
}

// goals are what the curve asks for at each position of a set of some length.
type goals struct {
	energy, tempo []float64
}

func goalsFor(c Curve, n int) goals { return goals{c.Targets(n), c.TempoTargets(n)} }

// cost of playing tracks in this order, measured against goals of the same length.
func (p *problem) cost(order []int, g goals) float64 {
	total := 0.0
	for pos, t := range order {
		if pos > 0 {
			prev := order[pos-1]
			total += p.pair[prev][t]
			// Tempo going down while the curve climbs (or up while it
			// falls) undoes the work of the previous transitions. Even small
			// steps count: several of them add up to a real drop.
			rising := g.tempo[pos] >= g.tempo[pos-1]
			change := tempoChange(p.tracks[prev].BPM, p.tracks[t].BPM)
			if (rising && change < -1) || (!rising && change > 1) {
				total += p.w.Tempo * tempoAgainstCost * math.Abs(change)
			}
		}
		total += p.w.Energy * energyCurveCost * math.Abs(p.energy[t]-g.energy[pos])
		total += p.w.Tempo * tempoCurveCost * math.Abs(p.tempo[t]-g.tempo[pos])
	}
	return total
}

func (p *problem) anneal(rng *rand.Rand) solution {
	const startTemp, endTemp = 3.0, 0.02
	steps := 3000 * len(p.start)
	cooling := math.Pow(endTemp/startTemp, 1/float64(steps))

	order := slices.Clone(p.start)
	for i := len(p.free) - 1; i > 0; i-- {
		j := rng.IntN(i + 1)
		a, b := p.free[i], p.free[j]
		order[a], order[b] = order[b], order[a]
	}
	current := p.cost(order, p.goals)
	best := solution{slices.Clone(order), current}

	next := make([]int, len(order))
	temp := startTemp
	for range steps {
		copy(next, order)
		p.mutate(next, rng)
		c := p.cost(next, p.goals)
		if c <= current || rng.Float64() < math.Exp((current-c)/temp) {
			order, next = next, order
			current = c
			if c < best.cost {
				best = solution{slices.Clone(order), c}
			}
		}
		temp *= cooling
	}
	return best
}

// mutate swaps two tracks, or moves one to another position, touching only
// positions that are not pinned.
func (p *problem) mutate(order []int, rng *rand.Rand) {
	if len(p.free) < 2 {
		return
	}
	i, j := rng.IntN(len(p.free)), rng.IntN(len(p.free))
	if rng.IntN(2) == 0 {
		order[p.free[i]], order[p.free[j]] = order[p.free[j]], order[p.free[i]]
		return
	}
	moving := order[p.free[i]]
	for ; i < j; i++ {
		order[p.free[i]] = order[p.free[i+1]]
	}
	for ; i > j; i-- {
		order[p.free[i]] = order[p.free[i-1]]
	}
	order[p.free[j]] = moving
}

func (p *problem) ordered(order []int) []Track {
	out := make([]Track, len(order))
	for pos, t := range order {
		out[pos] = p.tracks[t]
	}
	return out
}
