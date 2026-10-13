package tags

import (
	"bufio"
	"os"
	"strings"

	"github.com/go-flac/flacvorbis"
	goflac "github.com/go-flac/go-flac"

	"prodjtools/internal/safefile"
)

// readComments returns the raw "KEY=value" Vorbis comments of a FLAC file,
// reading only its metadata, not the audio.
func readComments(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	file, err := goflac.ParseMetadata(bufio.NewReader(f))
	if err != nil {
		return nil, err
	}
	for _, block := range file.Meta {
		if block.Type == goflac.VorbisComment {
			c, err := flacvorbis.ParseFromMetaDataBlock(*block)
			if err != nil {
				return nil, err
			}
			return c.Comments, nil
		}
	}
	return nil, nil
}

// values collects every value stored under key; Vorbis keys ignore case.
func values(comments []string, key string) []string {
	var out []string
	for _, c := range comments {
		if k, v, ok := strings.Cut(c, "="); ok && strings.EqualFold(k, key) {
			out = append(out, v)
		}
	}
	return out
}

func first(v []string) string {
	if len(v) == 0 {
		return ""
	}
	return v[0]
}

func flacNames(path string) (artist, title string, err error) {
	comments, err := readComments(path)
	if err != nil {
		return "", "", err
	}
	return strings.Join(values(comments, "ARTIST"), ", "), first(values(comments, "TITLE")), nil
}

func flacSnapshot(path string) (Fields, error) {
	comments, err := readComments(path)
	if err != nil {
		return nil, err
	}
	f := Fields{}
	for _, key := range Managed {
		f[key] = first(values(comments, key))
	}
	return f, nil
}

func flacWrite(path string, f Fields) error {
	file, err := goflac.ParseFile(path)
	if err != nil {
		return err
	}
	index := -1
	comments := flacvorbis.New()
	for i, block := range file.Meta {
		if block.Type == goflac.VorbisComment {
			if comments, err = flacvorbis.ParseFromMetaDataBlock(*block); err != nil {
				return err
			}
			index = i
			break
		}
	}
	comments.Comments = withFields(comments.Comments, f)
	block := comments.Marshal()
	if index >= 0 {
		file.Meta[index] = &block
	} else {
		file.Meta = append(file.Meta, &block)
	}
	return safefile.Replace(path, file.Save)
}

// withFields drops existing values of the fields being written and appends
// the new ones, keeping every other comment as it was.
func withFields(comments []string, f Fields) []string {
	var out []string
	for _, c := range comments {
		key, _, _ := strings.Cut(c, "=")
		if _, replaced := f[strings.ToUpper(key)]; !replaced {
			out = append(out, c)
		}
	}
	for _, key := range Managed {
		if v := f[key]; v != "" {
			out = append(out, key+"="+v)
		}
	}
	return out
}
