package export

import (
	"encoding/xml"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func writeM3U(path, name string, set []Track) error {
	var b strings.Builder
	b.WriteString("#EXTM3U\n#PLAYLIST:" + name + "\n")
	for _, t := range set {
		fmt.Fprintf(&b, "#EXTINF:%d,%s - %s\n%s\n", int(math.Round(t.Duration)), t.Artist, t.Title, filepath.Base(t.Path))
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// The subset of Rekordbox's XML exchange format needed to bring in one
// playlist together with tempo, key, beat grid and cue points.
type (
	rbDocument struct {
		XMLName    xml.Name     `xml:"DJ_PLAYLISTS"`
		Version    string       `xml:"Version,attr"`
		Product    rbProduct    `xml:"PRODUCT"`
		Collection rbCollection `xml:"COLLECTION"`
		Playlists  rbNode       `xml:"PLAYLISTS>NODE"`
	}
	rbProduct struct {
		Name    string `xml:"Name,attr"`
		Version string `xml:"Version,attr"`
		Company string `xml:"Company,attr"`
	}
	rbCollection struct {
		Entries int       `xml:"Entries,attr"`
		Tracks  []rbTrack `xml:"TRACK"`
	}
	rbTrack struct {
		TrackID     int      `xml:"TrackID,attr"`
		Name        string   `xml:"Name,attr"`
		Artist      string   `xml:"Artist,attr"`
		Album       string   `xml:"Album,attr"`
		Kind        string   `xml:"Kind,attr"`
		TotalTime   int      `xml:"TotalTime,attr"`
		TrackNumber int      `xml:"TrackNumber,attr"`
		AverageBpm  string   `xml:"AverageBpm,attr"`
		Tonality    string   `xml:"Tonality,attr,omitempty"`
		Location    string   `xml:"Location,attr"`
		Tempo       *rbTempo `xml:"TEMPO,omitempty"`
		Marks       []rbMark `xml:"POSITION_MARK"`
	}
	// rbTempo anchors the beat grid: Inizio is the first beat, Battito its
	// place in the bar (1 = downbeat).
	rbTempo struct {
		Inizio  string `xml:"Inizio,attr"`
		Bpm     string `xml:"Bpm,attr"`
		Metro   string `xml:"Metro,attr"`
		Battito int    `xml:"Battito,attr"`
	}
	// rbMark is a cue point. Num 0–7 is hot cue A–H, -1 a memory cue.
	rbMark struct {
		Name  string `xml:"Name,attr"`
		Type  int    `xml:"Type,attr"`
		Start string `xml:"Start,attr"`
		Num   int    `xml:"Num,attr"`
		Red   uint8  `xml:"Red,attr"`
		Green uint8  `xml:"Green,attr"`
		Blue  uint8  `xml:"Blue,attr"`
	}
	rbNode struct {
		Type    int      `xml:"Type,attr"`
		Name    string   `xml:"Name,attr"`
		Count   int      `xml:"Count,attr,omitempty"`
		KeyType string   `xml:"KeyType,attr,omitempty"`
		Entries int      `xml:"Entries,attr,omitempty"`
		Nodes   []rbNode `xml:"NODE"`
		Tracks  []rbRef  `xml:"TRACK"`
	}
	rbRef struct {
		Key int `xml:"Key,attr"`
	}
)

func writeRekordbox(path, name string, set []Track) error {
	playlist := rbNode{Type: 1, Name: name, KeyType: "0", Entries: len(set)}
	doc := rbDocument{
		Version:    "1.0.0",
		Product:    rbProduct{Name: "ProDJ Tools", Version: "1.0"},
		Collection: rbCollection{Entries: len(set)},
	}
	for i, t := range set {
		id := i + 1
		doc.Collection.Tracks = append(doc.Collection.Tracks, rbTrack{
			TrackID:     id,
			Name:        t.Title,
			Artist:      t.Artist,
			Album:       name,
			Kind:        strings.ToUpper(strings.TrimPrefix(filepath.Ext(t.Path), ".")) + " File",
			TotalTime:   int(math.Round(t.Duration)),
			TrackNumber: id,
			AverageBpm:  fmt.Sprintf("%.2f", t.BPM),
			Tonality:    t.Key.Name(),
			Location:    fileURL(t.Path),
			Tempo:       t.tempo(),
			Marks:       t.marks(),
		})
		playlist.Tracks = append(playlist.Tracks, rbRef{Key: id})
	}
	doc.Playlists = rbNode{Type: 0, Name: "ROOT", Count: 1, Nodes: []rbNode{playlist}}

	data, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append([]byte(xml.Header), data...), 0o644)
}

func (t Track) tempo() *rbTempo {
	if t.GridBPM <= 0 {
		return nil
	}
	return &rbTempo{Inizio: fmt.Sprintf("%.3f", t.GridStart), Bpm: fmt.Sprintf("%.2f", t.GridBPM), Metro: "4/4", Battito: 1}
}

// marks lists every cue twice: as a hot cue on its button and as a memory
// cue, which CDJs show in their cue list.
func (t Track) marks() []rbMark {
	var marks []rbMark
	for _, c := range t.Cues {
		start := fmt.Sprintf("%.3f", c.Time)
		marks = append(marks,
			rbMark{Name: c.Name, Start: start, Num: c.Hot, Red: c.Color[0], Green: c.Color[1], Blue: c.Color[2]},
			rbMark{Name: c.Name, Start: start, Num: -1, Red: c.Color[0], Green: c.Color[1], Blue: c.Color[2]},
		)
	}
	return marks
}

// fileURL turns C:\Music\a b.flac into file://localhost/C:/Music/a%20b.flac,
// the form Rekordbox expects.
func fileURL(path string) string {
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Host: "localhost", Path: p}).String()
}
