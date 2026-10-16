package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"

	"prodjtools/internal/audio"
	"prodjtools/internal/export"
	"prodjtools/internal/i18n"
	"prodjtools/internal/mixdown"
	"prodjtools/internal/mixplan"
	"prodjtools/internal/tags"
)

// mixdownState is the Ready set tab: the set as one continuous mix, to
// listen to from any point and to save as a file.
type mixdownState struct {
	mu   sync.Mutex
	gen  int // bumped with every plan, so a stream can tell it is stale
	tl   mixdown.Timeline
	load mixdown.Loader
	job  mixdownJob
	pcm  pcmCache
}

type mixdownJob struct {
	Running  bool    `json:"running"`
	Progress float64 `json:"progress"` // 0…1
	Ready    bool    `json:"ready"`    // a finished file waits to be downloaded
	File     string  `json:"file"`     // its name
	Error    string  `json:"error,omitempty"`
	path     string
	cancel   context.CancelFunc
}

type mixdownRequest struct {
	Order  []string `json:"order"`
	Ways   []string `json:"ways"` // ways[i]: how track i goes into track i+1, "outro" or "early"
	Format string   `json:"format"`
	Name   string   `json:"name"`
}

// mixdownPlan resolves the set and the way of every transition, the same
// way the Mixing tab does: the chosen way if the pair allows it, otherwise
// via the outro, otherwise early.
func (a *App) mixdownPlan(req mixdownRequest) ([]mixdown.Track, []*mixplan.Transition, error) {
	a.mu.Lock()
	picked, err := a.pick(req.Order)
	floor := a.settings.BPMFloor
	a.mu.Unlock()
	if err != nil {
		return nil, nil, err
	}
	if len(picked) == 0 {
		return nil, nil, i18n.Error("error.empty_set")
	}
	tracks := make([]mixdown.Track, len(picked))
	for i, t := range picked {
		tracks[i] = mixdown.Track{Path: t.Path, Artist: t.Artist, Title: t.Title, Duration: t.Duration, Result: t.Analysis.InMeter(floor)}
	}
	plans := make([]*mixplan.Transition, len(tracks)-1)
	for i := range plans {
		a, b := tracks[i].Result, tracks[i+1].Result
		outro, viaOutro := mixplan.Plan(a, b)
		early, isEarly := mixplan.EarlyPlan(a, b)
		chosen := ""
		if i < len(req.Ways) {
			chosen = req.Ways[i]
		}
		switch {
		case chosen == mixplan.Early && isEarly:
			plans[i] = &early
		case viaOutro:
			plans[i] = &outro
		case isEarly:
			plans[i] = &early
		}
	}
	return tracks, plans, nil
}

func (a *App) loader(tracks []mixdown.Track) mixdown.Loader {
	return func(i int) (*audio.PCM, error) { return a.md.pcm.get(tracks[i].Path) }
}

// handleMixdownPlan lays the set out as one mix and tells the page where
// every track and transition sits in it.
func (a *App) handleMixdownPlan(w http.ResponseWriter, r *http.Request) {
	var req mixdownRequest
	if !decode(w, r, &req) {
		return
	}
	tracks, plans, err := a.mixdownPlan(req)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	tl := mixdown.Build(tracks, plans)
	type clipView struct {
		ID    string  `json:"id"`
		Start float64 `json:"start"`
		End   float64 `json:"end"`
	}
	clips := make([]clipView, len(tl.Clips))
	for i, c := range tl.Clips {
		clips[i] = clipView{req.Order[c.Track], c.Start, c.End}
	}

	a.md.mu.Lock()
	a.md.gen++
	gen := a.md.gen
	a.md.tl, a.md.load = tl, a.loader(tracks)
	a.md.mu.Unlock()
	reply(w, map[string]any{"key": gen, "length": tl.Length, "clips": clips, "joins": tl.Joins})
}

// handleMixdownStream plays the planned mix from ?from= seconds as a WAV
// stream; the page reloads it to jump elsewhere.
func (a *App) handleMixdownStream(w http.ResponseWriter, r *http.Request) {
	a.md.mu.Lock()
	gen, tl, load := a.md.gen, a.md.tl, a.md.load
	a.md.mu.Unlock()
	if key, _ := strconv.Atoi(r.URL.Query().Get("key")); key != gen || load == nil {
		http.Error(w, "stale mix", http.StatusConflict)
		return
	}
	from, _ := strconv.ParseFloat(r.URL.Query().Get("from"), 64)
	rd := mixdown.NewRenderer(&tl, load, from)
	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("Content-Length", strconv.FormatInt(44+4*rd.Frames(), 10))
	if err := mixdown.StreamWAV(r.Context(), w, rd); err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("mixdown stream: %v", err)
	}
}

// handleMixdownExport starts saving the whole set as one file.
func (a *App) handleMixdownExport(w http.ResponseWriter, r *http.Request) {
	var req mixdownRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Format != mixdown.FLAC && req.Format != mixdown.MP3 {
		fail(w, http.StatusBadRequest, i18n.Error("error.bad_request"))
		return
	}
	tracks, plans, err := a.mixdownPlan(req)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	name := export.CleanName(req.Name)
	if name == "" {
		name = "Mix"
	}
	dir := filepath.Join(a.dataDir, "mixdown")
	a.md.mu.Lock()
	busy := a.md.job.Running
	a.md.mu.Unlock()
	if !busy {
		os.RemoveAll(dir) // only the latest set is kept, also across restarts
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	file := name + "." + req.Format
	path := filepath.Join(dir, file)

	a.md.mu.Lock()
	if a.md.job.Running {
		a.md.mu.Unlock()
		fail(w, http.StatusConflict, i18n.Error("error.mixdown_busy"))
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.md.job = mixdownJob{Running: true, File: file, path: path, cancel: cancel}
	a.md.mu.Unlock()

	go func() {
		tl := mixdown.Build(tracks, plans)
		err := mixdown.Export(ctx, path, req.Format, &tl, a.loader(tracks), func(p float64) {
			a.md.mu.Lock()
			a.md.job.Progress = p
			a.md.mu.Unlock()
		})
		if err == nil {
			if terr := tags.Write(path, tags.Fields{"TITLE": name}); terr != nil {
				log.Printf("mixdown tags: %v", terr)
			}
		}
		a.md.mu.Lock()
		defer a.md.mu.Unlock()
		a.md.job.Running, a.md.job.cancel = false, nil
		switch {
		case errors.Is(err, context.Canceled):
			a.md.job = mixdownJob{}
		case err != nil:
			log.Printf("mixdown: %v", err)
			a.md.job.Error = err.Error()
		default:
			a.md.job.Ready, a.md.job.Progress = true, 1
		}
	}()
	reply(w, nil)
}

func (a *App) handleMixdownJob(w http.ResponseWriter, r *http.Request) {
	a.md.mu.Lock()
	job := a.md.job
	a.md.mu.Unlock()
	reply(w, job)
}

func (a *App) handleMixdownCancel(w http.ResponseWriter, r *http.Request) {
	a.md.mu.Lock()
	if a.md.job.cancel != nil {
		a.md.job.cancel()
	}
	a.md.mu.Unlock()
	reply(w, nil)
}

// handleMixdownFile hands the finished set to the browser as a download.
func (a *App) handleMixdownFile(w http.ResponseWriter, r *http.Request) {
	a.md.mu.Lock()
	job := a.md.job
	a.md.mu.Unlock()
	if !job.Ready {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(job.File))
	http.ServeFile(w, r, job.path)
}

// pcmCache keeps the last few decoded tracks: jumping around a mix keeps
// coming back to the same two or three.
type pcmCache struct {
	mu      sync.Mutex
	entries []pcmEntry // most recently used last
}

type pcmEntry struct {
	path string
	pcm  *audio.PCM
}

const pcmCached = 3

func (c *pcmCache) get(path string) (*audio.PCM, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if i := slices.IndexFunc(c.entries, func(e pcmEntry) bool { return e.path == path }); i >= 0 {
		e := c.entries[i]
		c.entries = append(slices.Delete(c.entries, i, i+1), e)
		return e.pcm, nil
	}
	pcm, err := audio.DecodePCM(path)
	if err != nil {
		return nil, err
	}
	c.entries = append(c.entries, pcmEntry{path, pcm})
	if len(c.entries) > pcmCached {
		c.entries = c.entries[1:]
	}
	return pcm, nil
}
