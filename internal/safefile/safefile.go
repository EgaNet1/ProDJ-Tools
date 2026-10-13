// Package safefile changes files carefully: a rewrite lands all at once, and
// short-lived locks (a player still reading the file, an antivirus scan) are
// waited out instead of failing straight away.
package safefile

import (
	"errors"
	"io/fs"
	"os"
	"time"
)

// Rename moves a file, retrying for up to a second while it is locked.
func Rename(from, to string) error {
	var err error
	for range 10 {
		if err = os.Rename(from, to); err == nil || errors.Is(err, fs.ErrNotExist) {
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}
	return err
}

// Replace rewrites path: write produces a complete new version in a
// temporary file next to it, which is then swapped in. A failure midway never
// leaves a half-written file behind.
func Replace(path string, write func(tmp string) error) error {
	tmp := path + ".prodj-tmp"
	if err := write(tmp); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
