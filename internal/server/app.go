// Package server is the bridge between the interface and the engine: it
// keeps the open folder and its analysed tracks, and answers the page's
// requests over a small JSON API.
package server

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"prodjtools/internal/i18n"
	"prodjtools/internal/library"
)

// App is the running program.
type App struct {
	dataDir string
	version string
	cache   *library.Cache
	life    *lifeline

	mu       sync.Mutex
	settings settings
	dir      string
	tracks   []*library.Track
	byID     map[string]*library.Track
	energy   map[string]float64
	scan     scanStatus
	scanID   int // identifies the latest scan, so a stale one can tell it was superseded
}

type settings struct {
	Recent   []string `json:"recent"`
	BPMFloor float64  `json:"bpmFloor"`
	Lang     string   `json:"lang"` // interface language; empty until first detected
}

type scanStatus struct {
	Running bool     `json:"running"`
	Done    int      `json:"done"`
	Total   int      `json:"total"`
	Current string   `json:"current"`
	Errors  []string `json:"errors"`
}

const maxRecent = 8

// New prepares the program; dataDir holds the cache and settings, version
// is shown in the interface.
func New(dataDir, version string) (*App, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	a := &App{
		dataDir:  dataDir,
		version:  version,
		cache:    library.OpenCache(filepath.Join(dataDir, "analysis-cache.json")),
		life:     newLifeline(),
		settings: settings{BPMFloor: 88},
	}
	if data, err := os.ReadFile(a.settingsPath()); err == nil {
		json.Unmarshal(data, &a.settings)
	}
	i18n.Set(a.settings.Lang)
	return a, nil
}

// WatchWindow makes the program end shortly after its window is closed.
func (a *App) WatchWindow() { go a.life.watch() }

// Closed is closed once the window has gone away.
func (a *App) Closed() <-chan struct{} { return a.life.closed }

// Shutdown saves what is still in memory.
func (a *App) Shutdown() {
	if err := a.cache.Save(); err != nil {
		log.Printf("cache: %v", err)
	}
}

// Open starts loading the tracks in dir and returns at once; the page
// follows the progress through /api/state.
func (a *App) Open(dir string) error {
	dir = filepath.Clean(strings.Trim(strings.TrimSpace(dir), `"`))
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return i18n.Error("error.no_folder", dir)
	}

	a.mu.Lock()
	a.scanID++
	id := a.scanID
	a.dir, a.tracks, a.byID, a.energy = dir, nil, nil, nil
	a.scan = scanStatus{Running: true}
	a.settings.Recent = slices.DeleteFunc(a.settings.Recent, func(d string) bool { return strings.EqualFold(d, dir) })
	a.settings.Recent = append([]string{dir}, a.settings.Recent[:min(len(a.settings.Recent), maxRecent-1)]...)
	a.saveSettings()
	a.mu.Unlock()

	go a.scanFolder(id, dir)
	return nil
}

func (a *App) scanFolder(id int, dir string) {
	progress := func(done, total int, file string) {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.scanID == id {
			a.scan.Done, a.scan.Total, a.scan.Current = done, total, file
		}
	}
	tracks, errs := library.Scan(dir, a.cache, progress)
	if err := a.cache.Save(); err != nil {
		log.Printf("cache: %v", err)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.scanID != id {
		return // another folder was opened meanwhile
	}
	a.tracks = tracks
	a.byID = make(map[string]*library.Track, len(tracks))
	for _, t := range tracks {
		a.byID[t.ID] = t
	}
	a.energy = library.Energy(tracks)
	a.scan.Running = false
	for _, err := range errs {
		a.scan.Errors = append(a.scan.Errors, err.Error())
	}
}

func (a *App) settingsPath() string { return filepath.Join(a.dataDir, "settings.json") }

// saveSettings must be called with a.mu held.
func (a *App) saveSettings() {
	data, _ := json.MarshalIndent(a.settings, "", "  ")
	if err := os.WriteFile(a.settingsPath(), data, 0o644); err != nil {
		log.Printf("settings: %v", err)
	}
}
