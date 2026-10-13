// Package export writes a finished set into the files themselves — numbered
// file names, numbered tags, playlists — so any DJ software sees the order.
// Everything it changes is backed up first and can be undone with Restore.
package export

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"prodjtools/internal/camelot"
	"prodjtools/internal/naming"
	"prodjtools/internal/safefile"
	"prodjtools/internal/tags"
)

// Track is a file as it goes into the set.
type Track struct {
	Path     string
	Artist   string
	Title    string // without any set number
	Duration float64
	BPM      float64
	Key      camelot.Key

	GridBPM   float64 // beat grid tempo; 0 = no grid
	GridStart float64 // seconds to the first downbeat
	Cues      []Cue
}

// Cue is a cue point to write into Rekordbox XML.
type Cue struct {
	Hot   int // hot cue button: 0 = A, 1 = B, …
	Name  string
	Time  float64 // seconds
	Color [3]uint8
}

// Options choose what to write.
type Options struct {
	Name      string `json:"name"`      // set name: album tag and playlist file name
	Rename    bool   `json:"rename"`    // "07 - Artist - Title.flac"
	Tags      bool   `json:"tags"`      // title "07 Title", album, track number
	BPMAndKey bool   `json:"bpmAndKey"` // also write BPM and INITIALKEY tags
	M3U       bool   `json:"m3u"`
	Rekordbox bool   `json:"rekordbox"`
	Cues      bool   `json:"cues"` // beat grid and cues in the XML, plus the cue sheet
	Sheet     string `json:"-"`    // cue sheet text, written when Cues is set
}

// Report tells what happened.
type Report struct {
	Log   []string          `json:"log"`
	Files map[string]string `json:"-"` // every touched file: old path → new path
}

func (r *Report) logf(format string, args ...any) {
	r.Log = append(r.Log, fmt.Sprintf(format, args...))
}

// move renames a file within its folder and records it.
func (r *Report) move(path, name string) (string, error) {
	target := filepath.Join(filepath.Dir(path), name)
	if target != path {
		if err := safefile.Rename(path, target); err != nil {
			return path, err
		}
	}
	r.Files[path] = target
	return target, nil
}

// Apply writes set (in playing order) into the folder dir. Tracks in rest are
// not part of the set; if an earlier export numbered them, they get their
// original names and tags back so they cannot be mistaken for set positions.
func Apply(dir string, set, rest []Track, opt Options) (Report, error) {
	rep := Report{Files: map[string]string{}}
	opt.Name = CleanName(opt.Name)

	// Files are only touched when renaming or tagging; playlists alone leave
	// them as they are and need no backup.
	touching := opt.Rename || opt.Tags
	var leaving []Track
	for _, t := range rest {
		if touching && naming.Number(filepath.Base(t.Path)) > 0 {
			leaving = append(leaving, t)
		}
	}

	b, err := loadBackup(dir)
	if err != nil {
		return rep, fmt.Errorf("резервная копия: %w", err)
	}
	if touching {
		for _, t := range slices.Concat(set, leaving) {
			if err := b.remember(t.Path); err != nil {
				return rep, fmt.Errorf("%s: %w", filepath.Base(t.Path), err)
			}
		}
		if err := b.save(dir); err != nil {
			return rep, fmt.Errorf("резервная копия: %w", err)
		}
		rep.logf("Исходные имена и теги сохранены в %s", backupFile)
	}

	width := naming.Width(len(set))
	final := make([]Track, len(set))
	tagged := 0
	for i, t := range set {
		n := i + 1
		if opt.Tags && tags.Writable(t.Path) {
			if err := tags.Write(t.Path, setFields(n, width, len(set), t, opt)); err != nil {
				return rep, fmt.Errorf("%s: теги: %w", filepath.Base(t.Path), err)
			}
			tagged++
		}
		if opt.Rename {
			if t.Path, err = rep.move(t.Path, naming.File(n, width, filepath.Base(t.Path))); err != nil {
				return rep, fmt.Errorf("%s: %w", filepath.Base(t.Path), err)
			}
		} else {
			rep.Files[t.Path] = t.Path
		}
		final[i] = t
	}
	if opt.Rename {
		rep.logf("Файлы пронумерованы: %d", len(set))
	}
	if opt.Tags {
		rep.logf("Номера записаны в теги: %d", tagged)
	}

	for _, t := range leaving {
		stable := naming.Strip(filepath.Base(t.Path))
		if original := b.Tracks[stable]; opt.Tags && len(original.Fields) > 0 && tags.Writable(t.Path) {
			// Back to the tags it had before ProDJ Tools, but without a set
			// number in the title even if it already carried one back then.
			n := naming.Number(original.Name)
			if n == 0 {
				n = naming.Number(filepath.Base(t.Path))
			}
			f := maps.Clone(original.Fields)
			f["TITLE"] = naming.CleanTitle(f["TITLE"], n)
			if err := tags.Write(t.Path, f); err != nil {
				return rep, fmt.Errorf("%s: теги: %w", stable, err)
			}
		}
		name := filepath.Base(t.Path)
		if opt.Rename {
			name = stable
		}
		if _, err := rep.move(t.Path, name); err != nil {
			return rep, fmt.Errorf("%s: %w", stable, err)
		}
	}
	if opt.Rename && len(leaving) > 0 {
		rep.logf("Треки вне сета потеряли старые номера: %d", len(leaving))
	}

	if opt.M3U {
		name := opt.Name + ".m3u8"
		if err := writeM3U(filepath.Join(dir, name), opt.Name, final); err != nil {
			return rep, err
		}
		rep.logf("Плейлист: %s", name)
	}
	if opt.Cues && opt.Sheet != "" {
		name := opt.Name + " — метки.txt"
		if err := WriteText(filepath.Join(dir, name), opt.Sheet); err != nil {
			return rep, err
		}
		rep.logf("Лист меток: %s", name)
	}
	if !opt.Cues {
		for i := range final {
			final[i].GridBPM, final[i].Cues = 0, nil // leave Rekordbox's own grid alone
		}
	}
	if opt.Rekordbox {
		name := opt.Name + ".xml"
		if err := writeRekordbox(filepath.Join(dir, name), opt.Name, final); err != nil {
			return rep, err
		}
		rep.logf("Rekordbox XML: %s", name)
	}
	return rep, nil
}

func setFields(n, width, total int, t Track, opt Options) tags.Fields {
	f := tags.Fields{
		"TITLE":       naming.Title(n, width, t.Title),
		"ALBUM":       opt.Name,
		"TRACKNUMBER": strconv.Itoa(n),
		"TRACKTOTAL":  strconv.Itoa(total),
		"DISCNUMBER":  "1",
	}
	if opt.BPMAndKey {
		f["BPM"] = strconv.FormatFloat(t.BPM, 'f', -1, 64)
		f["INITIALKEY"] = t.Key.Name()
	}
	return f
}

// Restore gives every file recorded in the backup its original name and tags.
func Restore(dir string) (Report, error) {
	rep := Report{Files: map[string]string{}}
	b, err := loadBackup(dir)
	if err != nil {
		return rep, err
	}
	if len(b.Tracks) == 0 {
		return rep, errors.New("в этой папке нет резервной копии")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return rep, err
	}
	current := map[string]string{} // name without set number → path now
	for _, e := range entries {
		current[naming.Strip(e.Name())] = filepath.Join(dir, e.Name())
	}

	restored, failed := 0, 0
	for _, stable := range sortedKeys(b.Tracks) {
		original := b.Tracks[stable]
		path, ok := current[stable]
		if !ok {
			rep.logf("Не найден: %s", stable)
			failed++
			continue
		}
		if len(original.Fields) > 0 && tags.Writable(path) {
			if err := tags.Write(path, original.Fields); err != nil {
				rep.logf("%s: %v", stable, err)
				failed++
				continue
			}
		}
		if _, err := rep.move(path, original.Name); err != nil {
			rep.logf("%s: %v", stable, err)
			failed++
			continue
		}
		restored++
	}
	rep.logf("Возвращено как было: %d", restored)
	if failed > 0 {
		return rep, fmt.Errorf("не удалось вернуть %d файл(ов), резервная копия сохранена", failed)
	}
	b.discard(dir)
	return rep, nil
}

// CleanName makes a set name safe to use as a file name.
func CleanName(name string) string {
	name = strings.TrimSpace(strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) || r < ' ' {
			return '_'
		}
		return r
	}, name))
	if name == "" {
		return "DJ Set"
	}
	return name
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// WriteText saves text as UTF-8 with a byte order mark, which Notepad and
// older editors need to show Cyrillic correctly.
func WriteText(path, text string) error {
	return os.WriteFile(path, append([]byte{0xEF, 0xBB, 0xBF}, text...), 0o644)
}
