package audio

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	goaudio "github.com/go-audio/audio"
	"github.com/go-audio/wav"
)

// writeWAV creates a stereo 16-bit WAV with a 440 Hz tone.
func writeWAV(t *testing.T, path string, rate, seconds int) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	enc := wav.NewEncoder(f, rate, 16, 2, 1)
	data := make([]int, rate*seconds*2)
	for i := 0; i < len(data); i += 2 {
		v := int(16000 * math.Sin(2*math.Pi*440*float64(i/2)/float64(rate)))
		data[i], data[i+1] = v, v
	}
	buf := &goaudio.IntBuffer{Data: data, Format: &goaudio.Format{NumChannels: 2, SampleRate: rate}, SourceBitDepth: 16}
	if err := enc.Write(buf); err != nil {
		t.Fatal(err)
	}
	if err := enc.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestDecodeWAV(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tone.wav")
	writeWAV(t, path, 44100, 3)

	sig, err := Decode(path)
	if err != nil {
		t.Fatal(err)
	}
	if sig.Rate != 11025 {
		t.Errorf("rate = %d, want 11025", sig.Rate)
	}
	if math.Abs(sig.Duration-3) > 0.01 {
		t.Errorf("duration = %.3f, want 3", sig.Duration)
	}
	if want := 3 * 11025; math.Abs(float64(len(sig.Samples)-want)) > 2 {
		t.Errorf("%d samples, want about %d", len(sig.Samples), want)
	}
	peak := float32(0)
	for _, v := range sig.Samples {
		peak = max(peak, v)
	}
	// 16000/32768 ≈ 0.49, slightly lowered by the averaging low-pass.
	if peak < 0.3 || peak > 0.5 {
		t.Errorf("peak = %.3f, want about 0.45", peak)
	}
}

func TestSupported(t *testing.T) {
	for name, want := range map[string]bool{"a.FLAC": true, "b.mp3": true, "c.wav": true, "d.m3u8": false, "e.flac.prodj-tmp": false} {
		if Supported(name) != want {
			t.Errorf("Supported(%q) = %v", name, !want)
		}
	}
}
