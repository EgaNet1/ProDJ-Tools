package export

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"prodjtools/internal/naming"
	"prodjtools/internal/safefile"
	"prodjtools/internal/tags"
)

const (
	backupFile = ".prodj-backup.json"
	// Written by the one-off scripts used before this program existed:
	// file name → lower-case tag name → values.
	legacyBackupFile = "original_tags_backup.json"
)

// backup remembers how files looked before ProDJ Tools first touched them.
// Entries are keyed by file name without set number, so they still match
// after any number of re-exports, and are never overwritten.
type backup struct {
	Tracks map[string]original `json:"tracks"`
}

type original struct {
	Name   string      `json:"name"`             // file name before renaming
	Fields tags.Fields `json:"fields,omitempty"` // managed tags before changes
}

// HasBackup reports whether there is anything to restore in dir.
func HasBackup(dir string) bool {
	for _, name := range []string{backupFile, legacyBackupFile} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

func loadBackup(dir string) (*backup, error) {
	b := &backup{Tracks: map[string]original{}}
	data, err := os.ReadFile(filepath.Join(dir, backupFile))
	switch {
	case err == nil:
		return b, json.Unmarshal(data, b)
	case !errors.Is(err, fs.ErrNotExist):
		return nil, err
	}
	return b, b.importLegacy(dir)
}

func (b *backup) importLegacy(dir string) error {
	data, err := os.ReadFile(filepath.Join(dir, legacyBackupFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var legacy map[string]map[string][]string
	if err := json.Unmarshal(data, &legacy); err != nil {
		return fmt.Errorf("%s: %w", legacyBackupFile, err)
	}
	for name, values := range legacy {
		f := tags.Fields{}
		for _, key := range tags.Managed {
			f[key] = ""
			if v := values[strings.ToLower(key)]; len(v) > 0 {
				f[key] = v[0]
			}
		}
		stable := naming.Strip(name) // those scripts never renamed before backing up
		b.Tracks[stable] = original{Name: stable, Fields: f}
	}
	return nil
}

// remember records a file's current state unless it is already recorded.
func (b *backup) remember(path string) error {
	name := filepath.Base(path)
	stable := naming.Strip(name)
	if _, ok := b.Tracks[stable]; ok {
		return nil
	}
	o := original{Name: name}
	if tags.Writable(path) {
		f, err := tags.Snapshot(path)
		if err != nil {
			return err
		}
		o.Fields = f
	}
	b.Tracks[stable] = o
	return nil
}

func (b *backup) save(dir string) error {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return safefile.Replace(filepath.Join(dir, backupFile), func(tmp string) error {
		return os.WriteFile(tmp, data, 0o644)
	})
}

// discard removes the backup once everything has been restored from it.
func (b *backup) discard(dir string) {
	os.Remove(filepath.Join(dir, backupFile))
	os.Remove(filepath.Join(dir, legacyBackupFile))
}
