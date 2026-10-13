package server

import (
	"errors"
	"net/http"
	"path/filepath"

	"prodjtools/internal/analysis"
	"prodjtools/internal/export"
	"prodjtools/internal/mixplan"
)

// mixTrack is a track's grid, structure and cue points, as the "Сведение"
// tab shows them.
type mixTrack struct {
	ID           string             `json:"id"`
	BPM          float64            `json:"bpm"`
	FirstBeat    float64            `json:"firstBeat"`
	Jitter       float64            `json:"jitter"` // ms, 0 = unknown
	DownbeatSure bool               `json:"downbeatSure"`
	Cues         []mixplan.Cue      `json:"cues"`
	Sections     []analysis.Section `json:"sections"`
	Bars         []int              `json:"bars"` // intensity of every bar, 0–255
	LastBar      int                `json:"lastBar"`
}

type mixView struct {
	Tracks      []mixTrack            `json:"tracks"`
	Transitions []*mixplan.Transition `json:"transitions"` // nil where a plan is impossible
	Sheet       string                `json:"sheet"`       // the same, as printable text
}

// mixPlan prepares cue points and transitions for a set in playing order.
func (a *App) mixPlan(ids []string, name string) (mixView, error) {
	a.mu.Lock()
	tracks, err := a.pick(ids)
	a.mu.Unlock()
	if err != nil {
		return mixView{}, err
	}

	var view mixView
	entries := make([]mixplan.Entry, len(tracks))
	for i, t := range tracks {
		r := t.Analysis
		bars := make([]int, len(r.Bars))
		for j, v := range r.Bars {
			bars[j] = int(v)
		}
		view.Tracks = append(view.Tracks, mixTrack{
			ID:           t.ID,
			BPM:          r.Grid.BPM,
			FirstBeat:    r.Grid.FirstBeat,
			Jitter:       r.Grid.Jitter,
			DownbeatSure: r.Grid.DownbeatSure(),
			Cues:         mixplan.Cues(r),
			Sections:     r.Sections,
			Bars:         bars,
			LastBar:      r.LastBar,
		})
		entries[i] = mixplan.Entry{Artist: t.Artist, Title: t.Title, Result: r}
		if i > 0 {
			var plan *mixplan.Transition
			if p, ok := mixplan.Plan(tracks[i-1].Analysis, r); ok {
				plan = &p
			}
			view.Transitions = append(view.Transitions, plan)
		}
	}
	view.Sheet = mixplan.Sheet(name, entries)
	return view, nil
}

type mixRequest struct {
	Order []string `json:"order"`
	Name  string   `json:"name"`
}

func (a *App) handleMixPlan(w http.ResponseWriter, r *http.Request) {
	var req mixRequest
	if !decode(w, r, &req) {
		return
	}
	view, err := a.mixPlan(req.Order, export.CleanName(req.Name))
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	reply(w, view)
}

// handleCueSheet saves the cue sheet as a text file in the set's folder.
func (a *App) handleCueSheet(w http.ResponseWriter, r *http.Request) {
	var req mixRequest
	if !decode(w, r, &req) {
		return
	}
	name := export.CleanName(req.Name)
	view, err := a.mixPlan(req.Order, name)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	a.mu.Lock()
	dir := a.dir
	a.mu.Unlock()
	if dir == "" {
		fail(w, http.StatusBadRequest, errors.New("папка не открыта"))
		return
	}
	file := name + " — метки.txt"
	if err := export.WriteText(filepath.Join(dir, file), view.Sheet); err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	reply(w, map[string]string{"file": file})
}
