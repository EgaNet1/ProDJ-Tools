// Package tags reads and writes the handful of metadata fields ProDJ Tools
// cares about, in FLAC (Vorbis comments) and MP3 (ID3v2) files.
package tags

import (
	"path/filepath"
	"strings"
)

// Fields holds tag values under their Vorbis-comment names. An empty value
// means the tag is absent: writing it removes the tag.
type Fields map[string]string

// Managed lists every field ProDJ Tools may change; backups store exactly these.
var Managed = []string{"TITLE", "ALBUM", "TRACKNUMBER", "TRACKTOTAL", "DISCNUMBER", "BPM", "INITIALKEY"}

// Writable reports whether tags of this file type can be changed.
func Writable(path string) bool {
	switch ext(path) {
	case ".flac", ".mp3":
		return true
	}
	return false
}

// Names returns the artist and title stored in the file, or empty strings.
func Names(path string) (artist, title string, err error) {
	switch ext(path) {
	case ".flac":
		return flacNames(path)
	case ".mp3":
		return mp3Names(path)
	}
	return "", "", nil
}

// Snapshot returns the current value of every managed field.
func Snapshot(path string) (Fields, error) {
	switch ext(path) {
	case ".flac":
		return flacSnapshot(path)
	case ".mp3":
		return mp3Snapshot(path)
	}
	return Fields{}, nil
}

// Write sets the given fields and leaves all other tags untouched.
func Write(path string, f Fields) error {
	switch ext(path) {
	case ".flac":
		return flacWrite(path, f)
	case ".mp3":
		return mp3Write(path, f)
	}
	return nil
}

func ext(path string) string { return strings.ToLower(filepath.Ext(path)) }
