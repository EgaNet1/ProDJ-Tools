package audio

import (
	"bufio"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"strings"

	goaudio "github.com/go-audio/audio"
	"github.com/go-audio/wav"
	"github.com/hajimehoshi/go-mp3"
	"github.com/mewkiz/flac"

	"prodjtools/internal/i18n"
)

// PCM is a track decoded at full quality for playback and mixing:
// interleaved stereo at the file's own sample rate, 16 bits per sample.
// Mono files are doubled to both sides; files with more channels keep
// their first two.
type PCM struct {
	Data []int16 // left, right, left, right…
	Rate int
}

// Frames is the number of stereo frames.
func (p *PCM) Frames() int { return len(p.Data) / 2 }

// DecodePCM reads a FLAC, MP3 or WAV file at full quality.
func DecodePCM(path string) (*PCM, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	switch strings.ToLower(filepath.Ext(path)) {
	case ".flac":
		return pcmFLAC(f)
	case ".mp3":
		return pcmMP3(f)
	case ".wav":
		return pcmWAV(f)
	}
	return nil, i18n.Error("audio.unsupported", filepath.Ext(path))
}

// to16 brings a sample of any bit depth to 16 bits.
func to16(v int32, bits int) int16 {
	switch {
	case bits > 16:
		v >>= bits - 16
	case bits < 16:
		v <<= 16 - bits
	}
	return int16(max(-32768, min(32767, v)))
}

func pcmFLAC(f *os.File) (*PCM, error) {
	stream, err := flac.New(bufio.NewReaderSize(f, 1<<20))
	if err != nil {
		return nil, err
	}
	info := stream.Info
	if info.NChannels == 0 || info.BitsPerSample == 0 {
		return nil, i18n.Error("audio.bad_flac")
	}
	bits := int(info.BitsPerSample)
	p := &PCM{Rate: int(info.SampleRate), Data: make([]int16, 0, 2*int(info.NSamples))}
	for {
		frame, err := stream.ParseNext()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		left := frame.Subframes[0].Samples
		right := left
		if len(frame.Subframes) > 1 {
			right = frame.Subframes[1].Samples
		}
		for i := range frame.Subframes[0].NSamples {
			p.Data = append(p.Data, to16(left[i], bits), to16(right[i], bits))
		}
	}
	return p, nil
}

func pcmMP3(f *os.File) (*PCM, error) {
	dec, err := mp3.NewDecoder(bufio.NewReaderSize(f, 1<<20))
	if err != nil {
		return nil, err
	}
	// go-mp3 always produces 16-bit little-endian stereo.
	p := &PCM{Rate: dec.SampleRate(), Data: make([]int16, 0, max(0, int(dec.Length()/2)))}
	buf := make([]byte, 1<<16)
	for {
		n, err := io.ReadFull(dec, buf)
		for i := 0; i+1 < n; i += 2 {
			p.Data = append(p.Data, int16(binary.LittleEndian.Uint16(buf[i:])))
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	p.Data = p.Data[:len(p.Data)/2*2]
	return p, nil
}

func pcmWAV(f *os.File) (*PCM, error) {
	dec := wav.NewDecoder(f)
	if !dec.IsValidFile() {
		return nil, i18n.Error("audio.bad_wav")
	}
	if err := dec.FwdToPCM(); err != nil {
		return nil, err
	}
	channels := int(dec.NumChans)
	bits := int(dec.BitDepth)
	p := &PCM{Rate: int(dec.SampleRate)}
	buf := &goaudio.IntBuffer{
		Data:   make([]int, 8192*channels),
		Format: &goaudio.Format{NumChannels: channels, SampleRate: int(dec.SampleRate)},
	}
	for {
		n, err := dec.PCMBuffer(buf)
		if err != nil {
			return nil, err
		}
		if n == 0 {
			break
		}
		for i := 0; i+channels <= n; i += channels {
			left := to16(int32(buf.Data[i]), bits)
			right := left
			if channels > 1 {
				right = to16(int32(buf.Data[i+1]), bits)
			}
			p.Data = append(p.Data, left, right)
		}
	}
	return p, nil
}
