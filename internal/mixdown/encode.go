package mixdown

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math/rand/v2"
	"os"

	"github.com/braheezy/shine-mp3/pkg/mp3"
	"github.com/mewkiz/flac"
	"github.com/mewkiz/flac/frame"
	"github.com/mewkiz/flac/meta"
)

// Formats the set can be saved in.
const (
	FLAC = "flac"
	MP3  = "mp3"
)

const block = 4608 // frames rendered at a time: a multiple of the MP3 frame (1152)

// quantizer turns float samples into 16-bit ones with triangular dither, so
// that fades into silence do not turn into steps.
type quantizer struct{ rng *rand.Rand }

func newQuantizer() quantizer { return quantizer{rand.New(rand.NewPCG(1, 2))} }

func (q quantizer) to16(in []float32, out []int16) {
	for i, v := range in {
		d := (q.rng.Float64() - q.rng.Float64()) / 32768
		s := (float64(v) + d) * 32767
		out[i] = int16(max(-32768, min(32767, s+0.5*sign(s))))
	}
}

func sign(v float64) float64 {
	if v < 0 {
		return -1
	}
	return 1
}

// WAVHeader is the header of a 16-bit stereo WAV stream of the given number
// of frames.
func WAVHeader(frames int64) []byte {
	size := uint32(min(frames*4, 1<<32-64))
	h := make([]byte, 44)
	copy(h[0:], "RIFF")
	binary.LittleEndian.PutUint32(h[4:], 36+size)
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1) // PCM
	binary.LittleEndian.PutUint16(h[22:], 2)
	binary.LittleEndian.PutUint32(h[24:], Rate)
	binary.LittleEndian.PutUint32(h[28:], Rate*4)
	binary.LittleEndian.PutUint16(h[32:], 4)
	binary.LittleEndian.PutUint16(h[34:], 16)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], size)
	return h
}

// StreamWAV writes what the renderer plays as a WAV stream, for
// listening; it stops when ctx is done (the listener went away).
func StreamWAV(ctx context.Context, w io.Writer, r *Renderer) error {
	if _, err := w.Write(WAVHeader(r.Frames())); err != nil {
		return nil
	}
	q := newQuantizer()
	buf := make([]float32, 2*block)
	pcm := make([]int16, 2*block)
	raw := make([]byte, 4*block)
	for ctx.Err() == nil {
		n, err := r.Read(buf)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		q.to16(buf[:2*n], pcm[:2*n])
		for i, v := range pcm[:2*n] {
			binary.LittleEndian.PutUint16(raw[2*i:], uint16(v))
		}
		if _, err := w.Write(raw[:4*n]); err != nil {
			return nil // the listener went away: jumped elsewhere or stopped
		}
	}
	return nil
}

// Export renders the whole mix into a file at path in the given format,
// reporting progress (0…1). The file appears only once it is complete.
func Export(ctx context.Context, path, format string, tl *Timeline, load Loader, progress func(float64)) error {
	tmp := path + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	err = encode(ctx, f, format, tl, load, progress)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	os.Remove(path)
	return os.Rename(tmp, path)
}

// sink takes the mix 16-bit frame by frame.
type sink interface {
	write(pcm []int16) error // interleaved stereo
	close() error
}

func encode(ctx context.Context, f *os.File, format string, tl *Timeline, load Loader, progress func(float64)) error {
	var out sink
	var err error
	switch format {
	case FLAC:
		out, err = newFLACSink(f)
	case MP3:
		out = newMP3Sink(f)
	default:
		return errors.New("unknown format " + format)
	}
	if err != nil {
		return err
	}
	top := ceiling
	if format == MP3 {
		top = mp3Ceiling
	}
	r := newRenderer(tl, load, 0, top)
	total := float64(max(1, r.Frames()))
	done := 0
	q := newQuantizer()
	buf := make([]float32, 2*block)
	pcm := make([]int16, 2*block)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := r.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		q.to16(buf[:2*n], pcm[:2*n])
		if err := out.write(pcm[:2*n]); err != nil {
			return err
		}
		done += n
		progress(float64(done) / total)
	}
	return out.close()
}

// ── FLAC ──

type flacSink struct {
	enc  *flac.Encoder
	l, r []int32
}

func newFLACSink(f *os.File) (*flacSink, error) {
	info := &meta.StreamInfo{
		BlockSizeMin:  block,
		BlockSizeMax:  block,
		SampleRate:    Rate,
		NChannels:     2,
		BitsPerSample: 16,
	}
	// The encoder writes a few bytes at a time and, when it closes, seeks
	// back to the start to fill in the stream info: it gets a buffered
	// writer that can seek.
	enc, err := flac.NewEncoder(&seekBuffer{bufio.NewWriterSize(f, 1<<20), f}, info)
	if err != nil {
		return nil, err
	}
	return &flacSink{enc: enc}, nil
}

func (s *flacSink) write(pcm []int16) error {
	n := len(pcm) / 2
	s.l, s.r = s.l[:0], s.r[:0]
	for i := range n {
		s.l = append(s.l, int32(pcm[2*i]))
		s.r = append(s.r, int32(pcm[2*i+1]))
	}
	return s.enc.WriteFrame(&frame.Frame{
		Header: frame.Header{
			HasFixedBlockSize: true,
			BlockSize:         uint16(n),
			SampleRate:        Rate,
			Channels:          frame.ChannelsLR,
			BitsPerSample:     16,
		},
		Subframes: []*frame.Subframe{
			{SubHeader: frame.SubHeader{Pred: frame.PredVerbatim}, Samples: append([]int32(nil), s.l...), NSamples: n},
			{SubHeader: frame.SubHeader{Pred: frame.PredVerbatim}, Samples: append([]int32(nil), s.r...), NSamples: n},
		},
	})
}

func (s *flacSink) close() error { return s.enc.Close() }

// seekBuffer buffers writes to a file and flushes before seeking.
type seekBuffer struct {
	*bufio.Writer
	f *os.File
}

func (b *seekBuffer) Seek(offset int64, whence int) (int64, error) {
	if err := b.Flush(); err != nil {
		return 0, err
	}
	return b.f.Seek(offset, whence)
}

// Close flushes; the file itself is closed by Export.
func (b *seekBuffer) Close() error { return b.Flush() }

// ── MP3 ──

// mp3Sink encodes with shine at 320 kbit/s. Frames go to the encoder one by
// one: its Write method mishandles stereo input.
type mp3Sink struct {
	w       *bufio.Writer
	enc     *mp3.Encoder
	pending []int16
}

const mp3Frame = 1152 // samples per channel in an MPEG-1 layer III frame

func newMP3Sink(f *os.File) *mp3Sink {
	enc := mp3.NewEncoder(Rate, 2)
	// 320 kbit/s instead of the default 128 (index 14 of the MPEG-1 table).
	enc.Mpeg.Bitrate = 320
	enc.Mpeg.BitrateIndex = 14
	slots := float64(enc.Mpeg.GranulesPerFrame) * mp3.GRANULE_SIZE / Rate * 320 * 1000 / float64(enc.Mpeg.BitsPerSlot)
	enc.Mpeg.WholeSlotsPerFrame = int64(slots)
	enc.Mpeg.FracSlotsPerFrame = slots - float64(enc.Mpeg.WholeSlotsPerFrame)
	enc.Mpeg.SlotLag = -enc.Mpeg.FracSlotsPerFrame
	return &mp3Sink{w: bufio.NewWriterSize(f, 1<<20), enc: enc}
}

func (s *mp3Sink) write(pcm []int16) error {
	s.pending = append(s.pending, pcm...)
	for len(s.pending) >= 2*mp3Frame {
		if err := s.frame(s.pending[:2*mp3Frame]); err != nil {
			return err
		}
		s.pending = s.pending[2*mp3Frame:]
	}
	return nil
}

func (s *mp3Sink) frame(pcm []int16) error {
	data, n := s.enc.EncodeBufferInterleaved(pcm)
	_, err := s.w.Write(data[:n])
	return err
}

func (s *mp3Sink) close() error {
	if len(s.pending) > 0 {
		last := make([]int16, 2*mp3Frame) // padded with silence
		copy(last, s.pending)
		if err := s.frame(last); err != nil {
			return err
		}
	}
	return s.w.Flush()
}
