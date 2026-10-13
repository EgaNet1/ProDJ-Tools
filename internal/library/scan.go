package library

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"prodjtools/internal/audio"
	"prodjtools/internal/i18n"
)

// Progress is told about every finished file. It is called from one
// goroutine at a time.
type Progress func(done, total int, file string)

// Scan loads every supported audio file directly inside dir. Files are
// decoded on all CPU cores at once; files already in the cache are not
// decoded at all. Unreadable files are reported and skipped.
func Scan(dir string, cache *Cache, progress Progress) ([]*Track, []error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, []error{err}
	}
	var paths []string
	for _, e := range entries {
		if !e.IsDir() && audio.Supported(e.Name()) {
			paths = append(paths, filepath.Join(dir, e.Name()))
		}
	}
	if len(paths) == 0 {
		return nil, nil
	}

	type result struct {
		index int
		track *Track
		err   error
	}
	jobs := make(chan int)
	results := make(chan result)
	for range min(runtime.NumCPU(), len(paths)) {
		go func() {
			for i := range jobs {
				t, err := load(paths[i], cache)
				results <- result{i, t, err}
			}
		}()
	}
	go func() {
		for i := range paths {
			jobs <- i
		}
		close(jobs)
	}()

	loaded := make([]*Track, len(paths))
	var errs []error
	for done := 1; done <= len(paths); done++ {
		r := <-results
		name := filepath.Base(paths[r.index])
		if r.err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, r.err))
		}
		loaded[r.index] = r.track
		progress(done, len(paths), name)
	}

	// Keep directory order; drop failures and duplicates (the same song
	// present twice under different set numbers).
	tracks := make([]*Track, 0, len(loaded))
	seen := map[string]bool{}
	for _, t := range loaded {
		switch {
		case t == nil:
		case seen[t.ID]:
			errs = append(errs, fmt.Errorf("%s: %s", t.File, i18n.T("scan.duplicate")))
		default:
			seen[t.ID] = true
			tracks = append(tracks, t)
		}
	}
	return tracks, errs
}
