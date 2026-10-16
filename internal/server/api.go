package server

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"slices"
	"strings"

	"prodjtools/internal/analysis"
	"prodjtools/internal/desktop"
	"prodjtools/internal/export"
	"prodjtools/internal/i18n"
	"prodjtools/internal/library"
	"prodjtools/internal/mixplan"
	"prodjtools/internal/naming"
	"prodjtools/internal/setlist"
	"prodjtools/web"
)

// Below these confidences the interface marks a value as uncertain.
const (
	bpmSureAt     = 0.3
	keySureAt     = 0.5
	maxSureJitter = 5 // ms
)

// Handler serves the interface and its API.
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(web.Files))
	mux.HandleFunc("GET /audio/{id}", a.handleAudio)
	mux.HandleFunc("GET /api/state", a.handleState)
	mux.HandleFunc("GET /api/curves", a.handleCurves)
	mux.HandleFunc("GET /api/i18n", a.handleI18n)
	mux.HandleFunc("POST /api/open", a.handleOpen)
	mux.HandleFunc("POST /api/pick-folder", a.handlePickFolder)
	mux.HandleFunc("POST /api/github", handleGitHub)
	mux.HandleFunc("POST /api/settings", a.handleSettings)
	mux.HandleFunc("POST /api/evaluate", a.handleEvaluate)
	mux.HandleFunc("POST /api/build", a.handleBuild)
	mux.HandleFunc("POST /api/suggest", a.handleSuggest)
	mux.HandleFunc("POST /api/export", a.handleExport)
	mux.HandleFunc("POST /api/restore", a.handleRestore)
	mux.HandleFunc("POST /api/log", handleLog)
	mux.HandleFunc("POST /api/mixplan", a.handleMixPlan)
	mux.HandleFunc("POST /api/cuesheet", a.handleCueSheet)
	mux.HandleFunc("POST /api/mixdown/plan", a.handleMixdownPlan)
	mux.HandleFunc("GET /api/mixdown/stream", a.handleMixdownStream)
	mux.HandleFunc("POST /api/mixdown/export", a.handleMixdownExport)
	mux.HandleFunc("GET /api/mixdown/job", a.handleMixdownJob)
	mux.HandleFunc("POST /api/mixdown/cancel", a.handleMixdownCancel)
	mux.HandleFunc("GET /api/mixdown/file", a.handleMixdownFile)
	mux.HandleFunc("GET /api/alive", a.life.serve)
	return guard(mux)
}

// guard keeps other web pages out. The server listens on localhost only, but
// any site open in a browser could still send it requests: changes must
// carry a header that cross-site requests cannot set, and the Host header
// must be local, which defeats DNS rebinding.
func guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil || (host != "127.0.0.1" && host != "localhost") {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if r.Method == http.MethodPost && r.Header.Get("X-ProDJ") != "1" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		next.ServeHTTP(w, r)
	})
}

// trackView is a track as the page sees it, with tempo already folded into
// the chosen octave range.
type trackView struct {
	ID       string  `json:"id"`
	File     string  `json:"file"`
	Number   int     `json:"number"` // set number in the file name, 0 if none
	Artist   string  `json:"artist"`
	Title    string  `json:"title"`
	Duration float64 `json:"duration"`
	BPM      float64 `json:"bpm"`
	BPMSure  bool    `json:"bpmSure"`
	Key      string  `json:"key"`
	KeyName  string  `json:"keyName"`
	KeySure  bool    `json:"keySure"`
	Energy   float64 `json:"energy"`
	Wave     []int   `json:"wave"`
}

// view must be called with a.mu held.
func (a *App) view(t *library.Track) trackView {
	r := t.Analysis
	wave := make([]int, len(r.Overview))
	for i, v := range r.Overview {
		wave[i] = int(v)
	}
	return trackView{
		ID:       t.ID,
		File:     t.File,
		Number:   naming.Number(t.File),
		Artist:   t.Artist,
		Title:    t.Title,
		Duration: t.Duration,
		BPM:      analysis.FoldBPM(tempo(r), a.settings.BPMFloor),
		BPMSure:  bpmSure(r),
		Key:      r.Key.String(),
		KeyName:  r.Key.Name(),
		KeySure:  r.KeyConfidence >= keySureAt,
		Energy:   a.energy[t.ID],
		Wave:     wave,
	}
}

// pick looks tracks up by ID, in the given order. Must be called with a.mu held.
func (a *App) pick(ids []string) ([]*library.Track, error) {
	out := make([]*library.Track, 0, len(ids))
	for _, id := range ids {
		t, ok := a.byID[id]
		if !ok {
			return nil, i18n.Error("error.track_gone")
		}
		out = append(out, t)
	}
	return out, nil
}

// forSet converts tracks for the setlist package. Must be called with a.mu held.
func (a *App) forSet(tracks []*library.Track) []setlist.Track {
	out := make([]setlist.Track, len(tracks))
	for i, t := range tracks {
		out[i] = setlist.Track{
			ID:       t.ID,
			BPM:      analysis.FoldBPM(tempo(t.Analysis), a.settings.BPMFloor),
			Key:      t.Analysis.Key,
			Energy:   a.energy[t.ID],
			Duration: t.Duration,
		}
	}
	return out
}

// setTracks resolves IDs to setlist tracks in one locked step.
func (a *App) setTracks(ids []string) ([]setlist.Track, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	tracks, err := a.pick(ids)
	if err != nil {
		return nil, err
	}
	return a.forSet(tracks), nil
}

func (a *App) handleState(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	views := make([]trackView, 0, len(a.tracks))
	for _, t := range a.tracks {
		views = append(views, a.view(t))
	}
	reply(w, struct {
		Dir       string      `json:"dir"`
		Recent    []string    `json:"recent"`
		BPMFloor  float64     `json:"bpmFloor"`
		HasBackup bool        `json:"hasBackup"`
		Scan      scanStatus  `json:"scan"`
		Tracks    []trackView `json:"tracks"`
	}{a.dir, a.settings.Recent, a.settings.BPMFloor, a.dir != "" && export.HasBackup(a.dir), a.scan, views})
}

func (a *App) handleCurves(w http.ResponseWriter, r *http.Request) {
	type curveView struct {
		ID          string    `json:"id"`
		Name        string    `json:"name"`
		Description string    `json:"description"`
		Points      []float64 `json:"points"`
	}
	var out []curveView
	for _, c := range setlist.Curves {
		name, description := i18n.T("curve."+c.ID+".name"), i18n.T("curve."+c.ID+".description")
		out = append(out, curveView{c.ID, name, description, c.Targets(40)})
	}
	reply(w, out)
}

func (a *App) handleOpen(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Dir string `json:"dir"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := a.Open(req.Dir); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	reply(w, nil)
}

func (a *App) handlePickFolder(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	start := a.dir
	a.mu.Unlock()
	dir, err := desktop.PickFolder(start)
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	if dir != "" {
		if err := a.Open(dir); err != nil {
			fail(w, http.StatusBadRequest, err)
			return
		}
	}
	reply(w, map[string]string{"dir": dir})
}

// authorURL is the author's GitHub page, linked from the interface header.
const authorURL = "https://github.com/EgaNet1"

// handleGitHub opens the author's page in the user's own browser: a link
// followed inside the app window would open in its separate profile. The
// address is fixed so the API cannot be used to open arbitrary URLs.
func handleGitHub(w http.ResponseWriter, r *http.Request) {
	if err := desktop.OpenURL(authorURL); err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	reply(w, nil)
}

// handleSettings changes the settings present in the request.
func (a *App) handleSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BPMFloor *float64 `json:"bpmFloor"`
		Lang     *string  `json:"lang"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.BPMFloor != nil && (*req.BPMFloor < 50 || *req.BPMFloor > 120) {
		fail(w, http.StatusBadRequest, i18n.Error("error.bpm_floor"))
		return
	}
	if req.Lang != nil && !i18n.Supported(*req.Lang) {
		fail(w, http.StatusBadRequest, i18n.Error("error.bad_request"))
		return
	}
	a.mu.Lock()
	if req.BPMFloor != nil {
		a.settings.BPMFloor = *req.BPMFloor
	}
	if req.Lang != nil {
		a.settings.Lang = *req.Lang
		i18n.Set(*req.Lang)
	}
	a.saveSettings()
	a.mu.Unlock()
	reply(w, nil)
}

// handleI18n gives the interface its language and messages. Until the user
// picks a language, it is the best match for the window's preferred ones
// (navigator.languages, passed as ?prefer=ru-RU,en).
func (a *App) handleI18n(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	if !i18n.Supported(a.settings.Lang) {
		a.settings.Lang = i18n.Match(strings.Split(r.URL.Query().Get("prefer"), ","))
		i18n.Set(a.settings.Lang)
		a.saveSettings()
	}
	lang := a.settings.Lang
	a.mu.Unlock()
	reply(w, map[string]any{
		"lang":      lang,
		"rtl":       i18n.RTL(lang),
		"version":   a.version,
		"languages": i18n.Languages(),
		"messages":  i18n.Messages(lang),
	})
}

// orderRequest is the part every set-related request shares.
type orderRequest struct {
	Order   []string `json:"order"`
	Curve   string   `json:"curve"`
	Overlap float64  `json:"overlap"` // seconds two tracks play together
}

type evaluation struct {
	Order       []string             `json:"order"`
	Transitions []setlist.Transition `json:"transitions"`
	Summary     setlist.Summary      `json:"summary"`
}

func evaluate(order []setlist.Track, curve setlist.Curve, overlap float64) evaluation {
	ids := make([]string, len(order))
	for i, t := range order {
		ids[i] = t.ID
	}
	transitions, summary := setlist.Evaluate(order, curve, overlap)
	return evaluation{ids, transitions, summary}
}

func (a *App) handleEvaluate(w http.ResponseWriter, r *http.Request) {
	var req orderRequest
	if !decode(w, r, &req) {
		return
	}
	order, err := a.setTracks(req.Order)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	reply(w, evaluate(order, setlist.CurveByID(req.Curve), req.Overlap))
}

func (a *App) handleBuild(w http.ResponseWriter, r *http.Request) {
	var req struct {
		orderRequest
		Pins    map[string]int  `json:"pins"`
		Weights setlist.Weights `json:"weights"`
		Minutes float64         `json:"minutes"` // 0: use every track
	}
	if !decode(w, r, &req) {
		return
	}
	pool, err := a.setTracks(req.Order)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	opt := setlist.Options{Curve: setlist.CurveByID(req.Curve), Weights: req.Weights, Pins: req.Pins}
	var order []setlist.Track
	if req.Minutes > 0 {
		order = setlist.BuildFor(pool, req.Minutes*60, req.Overlap, opt)
	} else {
		order = setlist.Build(pool, opt)
	}
	reply(w, evaluate(order, opt.Curve, req.Overlap))
}

func (a *App) handleSuggest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		From      string          `json:"from"`
		Direction int             `json:"direction"`
		Weights   setlist.Weights `json:"weights"`
	}
	if !decode(w, r, &req) {
		return
	}
	a.mu.Lock()
	from, ok := a.byID[req.From]
	all := a.forSet(a.tracks)
	var current []setlist.Track
	if ok {
		current = a.forSet([]*library.Track{from})
	}
	a.mu.Unlock()
	if !ok {
		fail(w, http.StatusBadRequest, i18n.Error("error.track_not_found"))
		return
	}
	reply(w, setlist.Suggest(current[0], all, req.Direction, req.Weights, 8))
}

func (a *App) handleExport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Order []string `json:"order"`
		export.Options
	}
	if !decode(w, r, &req) {
		return
	}

	a.mu.Lock()
	dir, scanning := a.dir, a.scan.Running
	inSet, err := a.pick(req.Order)
	var set, rest []export.Track
	for _, t := range a.tracks {
		if !slices.Contains(inSet, t) {
			rest = append(rest, a.exportTrack(t))
		}
	}
	for _, t := range inSet {
		set = append(set, a.exportTrack(t))
	}
	a.mu.Unlock()

	switch {
	case scanning:
		err = i18n.Error("error.wait_scan")
	case len(set) == 0 && err == nil:
		err = i18n.Error("error.empty_set")
	}
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}

	if req.Cues {
		plan, err := a.mixPlan(req.Order, export.CleanName(req.Name))
		if err != nil {
			fail(w, http.StatusBadRequest, err)
			return
		}
		req.Sheet = plan.Sheet
	}
	rep, err := export.Apply(dir, set, rest, req.Options)
	a.afterFileChanges(dir, rep)
	if err != nil {
		replyLog(w, rep.Log, err)
		return
	}
	reply(w, rep)
}

func (a *App) handleRestore(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	dir := a.dir
	a.mu.Unlock()
	rep, err := export.Restore(dir)
	a.afterFileChanges(dir, rep)
	if err != nil {
		replyLog(w, rep.Log, err)
		return
	}
	reply(w, rep)
}

// afterFileChanges points the cache at renamed or re-tagged files (their
// audio is unchanged) and reloads the folder, which is then instant.
func (a *App) afterFileChanges(dir string, rep export.Report) {
	for from, to := range rep.Files {
		a.cache.Relocate(from, to)
	}
	if err := a.Open(dir); err != nil {
		log.Print(err)
	}
}

// exportTrack must be called with a.mu held.
func (a *App) exportTrack(t *library.Track) export.Track {
	e := export.Track{
		Path:     t.Path,
		Artist:   t.Artist,
		Title:    t.Title,
		Duration: t.Duration,
		BPM:      analysis.FoldBPM(tempo(t.Analysis), a.settings.BPMFloor),
		Key:      t.Analysis.Key,
	}
	// The grid and cues go to Rekordbox in the meter the DJ reads the track
	// in, so that its bars match the ones in this program.
	r := t.Analysis.InMeter(a.settings.BPMFloor)
	if r.Grid.Valid() {
		e.GridBPM, e.GridStart = r.Grid.BPM, r.Grid.FirstBeat
	}
	for _, c := range mixplan.Cues(r) {
		e.Cues = append(e.Cues, export.Cue{Hot: c.Hot(), Name: c.Name, Time: c.Time, Color: c.Color})
	}
	return e
}

// bpmSure tells whether the tempo can be trusted: a beat grid whose single
// hits sit within a few milliseconds of it, or else a confident estimate.
func bpmSure(r analysis.Result) bool {
	if r.Grid.Valid() {
		return r.Grid.Jitter > 0 && r.Grid.Jitter <= maxSureJitter
	}
	return r.BPMConfidence >= bpmSureAt
}

// tempo is the most precise tempo known for a track: the beat grid's if
// there is one, otherwise the raw estimate.
func tempo(r analysis.Result) float64 {
	if r.Grid.Valid() {
		return r.Grid.BPM
	}
	return r.BPM
}

func (a *App) handleAudio(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	t, ok := a.byID[r.PathValue("id")]
	a.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, t.Path)
}

// handleLog writes an error reported by the page into the program's log.
func handleLog(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Message string `json:"message"`
	}
	if decode(w, r, &req) {
		log.Printf("page: %s", req.Message)
		reply(w, nil)
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(v); err != nil {
		fail(w, http.StatusBadRequest, i18n.Error("error.bad_request"))
		return false
	}
	return true
}

func reply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("reply: %v", err)
	}
}

func fail(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

// replyLog reports an error that happened partway, together with what was
// already done.
func replyLog(w http.ResponseWriter, done []string, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	json.NewEncoder(w).Encode(map[string]any{"error": err.Error(), "log": done})
}
