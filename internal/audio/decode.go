// Package audio turns music files into a compact mono signal for analysis.
package audio

import (
	"bufio"
	"encoding/binary"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	goaudio "github.com/go-audio/audio"
	"github.com/go-audio/wav"
	"github.com/hajimehoshi/go-mp3"
	"github.com/mewkiz/flac"

	"prodjtools/internal/i18n"
)

// AnalysisRate is the sample rate the decoder aims for. Everything tempo and
// key detection needs lies far below its 5.5 kHz Nyquist limit, and a quarter
// of CD rate makes every later step four times cheaper.
const AnalysisRate = 11025

// Signal is a decoded track: mono, at roughly AnalysisRate.
type Signal struct {
	Samples  []float32
	Rate     int     // samples per second in Samples
	Duration float64 // length of the original recording, seconds
}

var decoders = map[string]func(*os.File) (Signal, error){
	".flac": decodeFLAC,
	".mp3":  decodeMP3,
	".wav":  decodeWAV,
}

// Supported reports whether Decode can read the file, judging by its name.
func Supported(name string) bool {
	_, ok := decoders[strings.ToLower(filepath.Ext(name))]
	return ok
}

// Decode reads a FLAC, MP3 or WAV file.
func Decode(path string) (Signal, error) {
	decode, ok := decoders[strings.ToLower(filepath.Ext(path))]
	if !ok {
		return Signal{}, i18n.Error("audio.unsupported", filepath.Ext(path))
	}
	f, err := os.Open(path)
	if err != nil {
		return Signal{}, err
	}
	defer f.Close()
	return decode(f)
}

func decodeFLAC(f *os.File) (Signal, error) {
	stream, err := flac.New(bufio.NewReaderSize(f, 1<<20))
	if err != nil {
		return Signal{}, err
	}
	info := stream.Info
	if info.NChannels == 0 || info.BitsPerSample == 0 {
		return Signal{}, i18n.Error("audio.bad_flac")
	}
	scale := 1 / (float32(int64(1)<<(info.BitsPerSample-1)) * float32(info.NChannels))
	d := newDownsampler(int(info.SampleRate), int(info.NSamples))
	for {
		frame, err := stream.ParseNext()
		if err == io.EOF {
			break
		}
		if err != nil {
			return Signal{}, err
		}
		channels := frame.Subframes
		for i := range channels[0].NSamples {
			var sum float32
			for _, ch := range channels {
				sum += float32(ch.Samples[i])
			}
			d.add(sum * scale)
		}
	}
	return d.signal(), nil
}

func decodeMP3(f *os.File) (Signal, error) {
	dec, err := mp3.NewDecoder(bufio.NewReaderSize(f, 1<<20))
	if err != nil {
		return Signal{}, err
	}
	d := newDownsampler(dec.SampleRate(), 0)
	// go-mp3 always produces 16-bit little-endian stereo: 4 bytes per frame.
	buf := make([]byte, 1<<16)
	for {
		n, err := io.ReadFull(dec, buf)
		for i := 0; i+4 <= n; i += 4 {
			left := int16(binary.LittleEndian.Uint16(buf[i:]))
			right := int16(binary.LittleEndian.Uint16(buf[i+2:]))
			d.add((float32(left) + float32(right)) / 65536)
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return Signal{}, err
		}
	}
	return d.signal(), nil
}

func decodeWAV(f *os.File) (Signal, error) {
	dec := wav.NewDecoder(f)
	if !dec.IsValidFile() {
		return Signal{}, i18n.Error("audio.bad_wav")
	}
	if err := dec.FwdToPCM(); err != nil {
		return Signal{}, err
	}
	channels := int(dec.NumChans)
	scale := 1 / (float32(int64(1)<<(dec.BitDepth-1)) * float32(channels))
	d := newDownsampler(int(dec.SampleRate), 0)
	buf := &goaudio.IntBuffer{
		Data:   make([]int, 8192*channels),
		Format: &goaudio.Format{NumChannels: channels, SampleRate: int(dec.SampleRate)},
	}
	for {
		n, err := dec.PCMBuffer(buf)
		if err != nil {
			return Signal{}, err
		}
		if n == 0 {
			break
		}
		for i := 0; i+channels <= n; i += channels {
			var sum float32
			for c := range channels {
				sum += float32(buf.Data[i+c])
			}
			d.add(sum * scale)
		}
	}
	return d.signal(), nil
}

// downsampler mixes channels and averages groups of consecutive samples as
// they arrive, so a full-resolution copy of the track never sits in memory.
type downsampler struct {
	rate   int // input sample rate
	step   int // input frames per output sample
	sum    float32
	count  int
	frames int // input frames seen
	out    []float32
}

func newDownsampler(rate, expectedFrames int) *downsampler {
	d := &downsampler{rate: rate, step: max(1, int(math.Round(float64(rate)/AnalysisRate)))}
	if expectedFrames > 0 {
		d.out = make([]float32, 0, expectedFrames/d.step+1)
	}
	return d
}

// add takes one frame, already mixed to mono and scaled to [-1, 1].
func (d *downsampler) add(v float32) {
	d.frames++
	d.sum += v
	d.count++
	if d.count == d.step {
		d.out = append(d.out, d.sum/float32(d.step))
		d.sum, d.count = 0, 0
	}
}

func (d *downsampler) signal() Signal {
	return Signal{
		Samples:  d.out,
		Rate:     d.rate / d.step,
		Duration: float64(d.frames) / float64(d.rate),
	}
}
