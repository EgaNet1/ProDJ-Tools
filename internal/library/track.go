// Package library loads a folder of tracks: it decodes and analyses them in
// parallel and remembers the results so a folder opens instantly next time.
package library

import (
	"hash/fnv"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"prodjtools/internal/analysis"
	"prodjtools/internal/audio"
	"prodjtools/internal/naming"
	"prodjtools/internal/tags"
)

// Track is one analysed audio file.
type Track struct {
	ID       string          `json:"id"`
	Path     string          `json:"-"`
	File     string          `json:"file"`
	Artist   string          `json:"artist"`
	Title    string          `json:"title"`
	Duration float64         `json:"duration"`
	Analysis analysis.Result `json:"analysis"`
}

// IDFor derives a track's ID from its file name without the set number, so
// the ID survives renumbering.
func IDFor(file string) string {
	h := fnv.New64a()
	h.Write([]byte(strings.ToLower(naming.Strip(file))))
	return strconv.FormatUint(h.Sum64(), 36)
}

// load analyses one file, or takes the result from the cache.
func load(path string, cache *Cache) (*Track, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if t, ok := cache.Get(path, info); ok {
		return t, nil
	}
	sig, err := audio.Decode(path)
	if err != nil {
		return nil, err
	}
	file := filepath.Base(path)
	artist, title := names(path, file)
	t := &Track{
		ID:       IDFor(file),
		Path:     path,
		File:     file,
		Artist:   artist,
		Title:    title,
		Duration: sig.Duration,
		Analysis: analysis.Analyze(sig),
	}
	cache.Put(path, info, t)
	return t, nil
}

// names prefers the file's tags and falls back to "Artist - Title.ext".
func names(path, file string) (artist, title string) {
	artist, title, _ = tags.Names(path)
	number := naming.Number(file)
	title = naming.CleanTitle(title, number)

	base := strings.TrimSuffix(naming.Strip(file), filepath.Ext(file))
	fileArtist, fileTitle, found := strings.Cut(base, " - ")
	if !found {
		fileArtist, fileTitle = "", base
	}
	if artist == "" {
		artist = fileArtist
	}
	if title == "" {
		title = fileTitle
	}
	return artist, title
}
