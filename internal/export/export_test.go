package export

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Tags of WAV files are never written, so these tests cover naming, the
// backup and the playlists without needing real FLAC files.
func setup(t *testing.T, names ...string) (string, []Track) {
	t.Helper()
	dir := t.TempDir()
	var tracks []Track
	for _, n := range names {
		path := filepath.Join(dir, n)
		if err := os.WriteFile(path, []byte("audio"), 0o644); err != nil {
			t.Fatal(err)
		}
		tracks = append(tracks, Track{Path: path, Artist: "A", Title: strings.TrimSuffix(strings.TrimPrefix(n, "A - "), ".wav"), Duration: 200, BPM: 140})
	}
	return dir, tracks
}

func listDir(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestApplyAndRestore(t *testing.T) {
	dir, tr := setup(t, "A - One.wav", "A - Two.wav", "05 - A - Old.wav", "A - Spare.wav")
	set := []Track{tr[1], tr[0]}
	rest := []Track{tr[2], tr[3]}

	rep, err := Apply(dir, set, rest, Options{Name: "My: Set", Rename: true, Tags: true, M3U: true, Rekordbox: true})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{backupFile, "01 - A - Two.wav", "02 - A - One.wav", "A - Old.wav", "A - Spare.wav", "My_ Set.m3u8", "My_ Set.xml"}
	if got := listDir(t, dir); !slices.Equal(got, want) {
		t.Fatalf("after Apply:\n got %v\nwant %v", got, want)
	}
	if len(rep.Files) != 3 {
		t.Errorf("touched %d files, want 3 (two in the set, one losing its number)", len(rep.Files))
	}

	m3u, _ := os.ReadFile(filepath.Join(dir, "My_ Set.m3u8"))
	if !strings.Contains(string(m3u), "#EXTINF:200,A - Two\n01 - A - Two.wav\n") {
		t.Errorf("unexpected playlist:\n%s", m3u)
	}
	xml, _ := os.ReadFile(filepath.Join(dir, "My_ Set.xml"))
	if !strings.Contains(string(xml), `Location="file://localhost/`) || !strings.Contains(string(xml), "01%20-%20A%20-%20Two.wav") {
		t.Errorf("unexpected Rekordbox XML:\n%s", xml)
	}

	// A second export must keep the original names from the first backup.
	again := []Track{{Path: filepath.Join(dir, "02 - A - One.wav"), Title: "One"}}
	if _, err := Apply(dir, again, nil, Options{Name: "S", Rename: true}); err != nil {
		t.Fatal(err)
	}

	if _, err := Restore(dir); err != nil {
		t.Fatal(err)
	}
	want = []string{"05 - A - Old.wav", "A - One.wav", "A - Spare.wav", "A - Two.wav", "My_ Set.m3u8", "My_ Set.xml"}
	if got := listDir(t, dir); !slices.Equal(got, want) {
		t.Fatalf("after Restore:\n got %v\nwant %v", got, want)
	}
	if HasBackup(dir) {
		t.Error("backup should be gone after a complete restore")
	}
}

func TestRestoreWithoutBackup(t *testing.T) {
	dir, _ := setup(t, "A - One.wav")
	if _, err := Restore(dir); err == nil {
		t.Error("expected an error when there is nothing to restore")
	}
}
