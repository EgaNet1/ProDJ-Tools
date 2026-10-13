package library

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"prodjtools/internal/analysis"
	"prodjtools/internal/safefile"
)

// Cache keeps analysis results between runs. An entry is valid while the
// file keeps its size and modification time and the analyser its version.
// It is safe for concurrent use.
type Cache struct {
	path    string
	mu      sync.Mutex
	entries map[string]cacheEntry
	dirty   bool
}

type cacheEntry struct {
	Size    int64 `json:"size"`
	ModTime int64 `json:"mtime"`
	Version int   `json:"version"`
	Track   Track `json:"track"`
}

// OpenCache loads the cache stored at path. A missing or damaged file
// simply gives an empty cache.
func OpenCache(path string) *Cache {
	c := &Cache{path: path, entries: map[string]cacheEntry{}}
	if data, err := os.ReadFile(path); err == nil {
		if json.Unmarshal(data, &c.entries) != nil {
			c.entries = map[string]cacheEntry{}
		}
	}
	return c
}

// Windows paths are case-insensitive.
func cacheKey(path string) string { return strings.ToLower(filepath.Clean(path)) }

// Get returns the cached track for an unchanged file.
func (c *Cache) Get(path string, info fs.FileInfo) (*Track, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[cacheKey(path)]
	if !ok || e.Size != info.Size() || e.ModTime != info.ModTime().UnixNano() || e.Version != analysis.Version {
		return nil, false
	}
	t := e.Track
	t.Path = path
	return &t, true
}

// Put stores a freshly analysed track.
func (c *Cache) Put(path string, info fs.FileInfo, t *Track) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[cacheKey(path)] = cacheEntry{
		Size:    info.Size(),
		ModTime: info.ModTime().UnixNano(),
		Version: analysis.Version,
		Track:   *t,
	}
	c.dirty = true
}

// Relocate carries a result over after ProDJ Tools itself renamed a file or
// rewrote its tags: the audio is the same, so it need not be analysed again.
func (c *Cache) Relocate(from, to string) {
	info, err := os.Stat(to)
	if err != nil {
		return
	}
	c.mu.Lock()
	e, ok := c.entries[cacheKey(from)]
	if ok {
		delete(c.entries, cacheKey(from))
	}
	c.mu.Unlock()
	if !ok || e.Version != analysis.Version {
		return
	}
	t := e.Track
	t.File = filepath.Base(to)
	c.Put(to, info, &t)
}

// Save writes the cache to disk if anything changed.
func (c *Cache) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.dirty {
		return nil
	}
	data, err := json.Marshal(c.entries)
	if err != nil {
		return err
	}
	err = safefile.Replace(c.path, func(tmp string) error { return os.WriteFile(tmp, data, 0o644) })
	if err == nil {
		c.dirty = false
	}
	return err
}
