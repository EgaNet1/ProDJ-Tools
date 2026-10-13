package tags

import (
	"strings"

	"github.com/bogem/id3v2/v2"
)

// ID3v2 frames for the managed fields. TRACKNUMBER and TRACKTOTAL share one
// frame, TRCK, written as "7/31".
var id3Frames = map[string]string{
	"TITLE":      "TIT2",
	"ALBUM":      "TALB",
	"DISCNUMBER": "TPOS",
	"BPM":        "TBPM",
	"INITIALKEY": "TKEY",
}

func mp3Names(path string) (artist, title string, err error) {
	tag, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		return "", "", err
	}
	defer tag.Close()
	return tag.Artist(), tag.Title(), nil
}

func mp3Snapshot(path string) (Fields, error) {
	tag, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		return nil, err
	}
	defer tag.Close()
	f := Fields{}
	for field, id := range id3Frames {
		f[field] = tag.GetTextFrame(id).Text
	}
	f["TRACKNUMBER"], f["TRACKTOTAL"], _ = strings.Cut(tag.GetTextFrame("TRCK").Text, "/")
	return f, nil
}

func mp3Write(path string, f Fields) error {
	tag, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		return err
	}
	defer tag.Close()

	encoding := id3v2.EncodingUTF16 // ID3v2.3 has no UTF-8
	if tag.Version() == 4 {
		encoding = id3v2.EncodingUTF8
	}
	set := func(id, value string) {
		tag.DeleteFrames(id)
		if value != "" {
			tag.AddTextFrame(id, encoding, value)
		}
	}
	for field, id := range id3Frames {
		if v, ok := f[field]; ok {
			set(id, v)
		}
	}
	if n, ok := f["TRACKNUMBER"]; ok {
		if total := f["TRACKTOTAL"]; n != "" && total != "" {
			n += "/" + total
		}
		set("TRCK", n)
	}
	return tag.Save()
}
