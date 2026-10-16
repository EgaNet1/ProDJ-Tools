package mixdown

import (
	"io"
	"math"
	"sync"

	"prodjtools/internal/audio"
)

// Loader returns a track of the set decoded at full quality.
type Loader func(track int) (*audio.PCM, error)

// Renderer plays a timeline into stereo samples, block by block, so that
// even a long set never sits in memory whole.
type Renderer struct {
	tl     *Timeline
	load   Loader
	frame  int64 // next frame to render, in mix frames
	end    int64 // first frame after the mix (and the limiter's tail)
	voices map[int]*voice
	lim    *limiter
	mixL   []float64
	mixR   []float64
}

// voice is a clip being played.
type voice struct {
	clip  *Clip
	pcm   *audio.PCM
	ratio float64 // file samples per mix sample at speed 1
	k     *kernel
	shelf shelf
}

// NewRenderer starts playing the timeline at mix time from.
func NewRenderer(tl *Timeline, load Loader, from float64) *Renderer {
	return newRenderer(tl, load, from, ceiling)
}

// newRenderer is NewRenderer with the limiter's ceiling (linear).
func newRenderer(tl *Timeline, load Loader, from, top float64) *Renderer {
	r := &Renderer{
		tl:     tl,
		load:   load,
		frame:  int64(math.Round(max(0, from) * Rate)),
		end:    int64(math.Ceil(tl.Length*Rate)) + lookahead,
		voices: map[int]*voice{},
		lim:    newLimiter(top),
	}
	// Prime the limiter's lookahead so the first frame out is the frame at
	// from.
	scratch := make([]float32, 2*lookahead)
	r.render(scratch, lookahead, false)
	return r
}

// Frames is the number of frames Read will return in all.
func (r *Renderer) Frames() int64 { return r.end - r.frame }

// Read fills out (interleaved stereo, -1…1) and returns the number of frames
// written; io.EOF once the mix is over.
func (r *Renderer) Read(out []float32) (int, error) {
	n := int(min(int64(len(out)/2), r.end-r.frame))
	if n <= 0 {
		return 0, io.EOF
	}
	if err := r.render(out, n, true); err != nil {
		return 0, err
	}
	return n, nil
}

func (r *Renderer) render(out []float32, n int, keep bool) error {
	if cap(r.mixL) < n {
		r.mixL, r.mixR = make([]float64, n), make([]float64, n)
	}
	mixL, mixR := r.mixL[:n], r.mixR[:n]
	clear(mixL)
	clear(mixR)
	t0 := float64(r.frame) / Rate
	t1 := float64(r.frame+int64(n)) / Rate

	for i := range r.tl.Clips {
		c := &r.tl.Clips[i]
		v, playing := r.voices[i]
		if c.End <= t0 || c.Start >= t1 {
			if playing && c.End <= t0 {
				delete(r.voices, i) // done: let its audio go
			}
			continue
		}
		if !playing {
			pcm, err := r.load(c.Track)
			if err != nil {
				return err
			}
			v = newVoice(c, pcm)
			r.voices[i] = v
		}
		v.play(r.frame, mixL, mixR)
	}

	for j := range n {
		l, rr := r.lim.process(mixL[j], mixR[j])
		if keep {
			out[2*j], out[2*j+1] = float32(l), float32(rr)
		}
	}
	r.frame += int64(n)
	return nil
}

func newVoice(c *Clip, pcm *audio.PCM) *voice {
	ratio := float64(pcm.Rate) / Rate
	fastest := 0.0
	for _, s := range c.rate {
		fastest = max(fastest, s.r0, s.r1)
	}
	// Anything above the new Nyquist limit would alias when the track is
	// sped up or resampled down; the kernel's cutoff keeps it out.
	return &voice{clip: c, pcm: pcm, ratio: ratio, k: kernelFor(min(0.95, 0.95/(fastest*ratio))), shelf: newShelf()}
}

// play adds the clip's frames [from, from+len(mixL)) to the mix.
func (v *voice) play(from int64, mixL, mixR []float64) {
	c := v.clip
	data, frames := v.pcm.Data, v.pcm.Frames()
	for j := range mixL {
		t := float64(from+int64(j)) / Rate
		if t < c.Start || t >= c.End {
			continue
		}
		pos := c.srcAt(t) * float64(v.pcm.Rate)
		l, r := v.k.at(data, frames, pos)
		l, r = v.shelf.process(l, r, c.bassAt(t))
		g := c.gainAt(t)
		mixL[j] += l * g
		mixR[j] += r * g
	}
}

// ── Resampling ──

// kernel is a windowed-sinc interpolator: taps samples around the position,
// tabulated for phases fractional positions between two samples.
type kernel struct {
	taps, phases int
	table        []float64 // (phases+1) rows of taps weights
}

const (
	kernelTaps   = 32
	kernelPhases = 512
)

var (
	kernelsMu sync.Mutex
	kernels   = map[int]*kernel{}
)

// kernelFor returns a kernel with the given cutoff, a fraction of the
// file's Nyquist frequency, rounded to a percent so kernels are shared.
func kernelFor(cutoff float64) *kernel {
	key := int(math.Round(cutoff * 100))
	kernelsMu.Lock()
	defer kernelsMu.Unlock()
	if k, ok := kernels[key]; ok {
		return k
	}
	fc := float64(key) / 100
	k := &kernel{taps: kernelTaps, phases: kernelPhases, table: make([]float64, (kernelPhases+1)*kernelTaps)}
	half := float64(kernelTaps / 2)
	for p := 0; p <= kernelPhases; p++ {
		frac := float64(p) / kernelPhases
		row := k.table[p*kernelTaps : (p+1)*kernelTaps]
		sum := 0.0
		for i := range row {
			d := float64(i) - (half - 1) - frac // distance from the position
			w := fc * sinc(fc*d) * kaiser(d/half, 9)
			row[i] = w
			sum += w
		}
		for i := range row {
			row[i] /= sum // unity gain at DC
		}
	}
	kernels[key] = k
	return k
}

// at interpolates the stereo frame at fractional position pos.
func (k *kernel) at(data []int16, frames int, pos float64) (float64, float64) {
	i := math.Floor(pos)
	ph := (pos - i) * float64(k.phases)
	p := int(ph)
	a := ph - float64(p)
	w0 := k.table[p*k.taps : (p+1)*k.taps]
	w1 := k.table[(p+1)*k.taps : (p+2)*k.taps]
	first := int(i) - (k.taps/2 - 1)
	var l, r float64
	for t := range k.taps {
		f := first + t
		if f < 0 || f >= frames {
			continue
		}
		w := w0[t] + a*(w1[t]-w0[t])
		l += float64(data[2*f]) * w
		r += float64(data[2*f+1]) * w
	}
	return l / 32768, r / 32768
}

func sinc(x float64) float64 {
	if x == 0 {
		return 1
	}
	return math.Sin(math.Pi*x) / (math.Pi * x)
}

// kaiser is the Kaiser window at x in [-1, 1].
func kaiser(x, beta float64) float64 {
	if x <= -1 || x >= 1 {
		return 0
	}
	return bessel0(beta*math.Sqrt(1-x*x)) / bessel0(beta)
}

func bessel0(x float64) float64 {
	sum, term := 1.0, 1.0
	for k := 1; k < 30; k++ {
		term *= (x / (2 * float64(k))) * (x / (2 * float64(k)))
		sum += term
	}
	return sum
}

// ── Bass ──

// shelf is the low shelf the preview uses to take a bass line out (a Web
// Audio BiquadFilter of type "lowshelf": the RBJ cookbook shelf, slope 1).
type shelf struct {
	db                 float64
	b0, b1, b2, a1, a2 float64
	z1, z2             [2]float64 // transposed direct form II state per channel
}

func newShelf() shelf {
	s := shelf{db: math.NaN()}
	s.set(0)
	return s
}

func (s *shelf) set(db float64) {
	if db == s.db {
		return
	}
	s.db = db
	A := math.Pow(10, db/40)
	w := 2 * math.Pi * bassShelf / Rate
	cos, alpha := math.Cos(w), math.Sin(w)/2*math.Sqrt2
	sq := 2 * math.Sqrt(A) * alpha
	a0 := (A + 1) + (A-1)*cos + sq
	s.b0 = A * ((A + 1) - (A-1)*cos + sq) / a0
	s.b1 = 2 * A * ((A - 1) - (A+1)*cos) / a0
	s.b2 = A * ((A + 1) - (A-1)*cos - sq) / a0
	s.a1 = -2 * ((A - 1) + (A+1)*cos) / a0
	s.a2 = ((A + 1) + (A-1)*cos - sq) / a0
}

func (s *shelf) process(l, r, db float64) (float64, float64) {
	s.set(math.Round(db*100) / 100)
	return s.one(0, l), s.one(1, r)
}

func (s *shelf) one(ch int, x float64) float64 {
	y := s.b0*x + s.z1[ch]
	s.z1[ch] = s.b1*x - s.a1*y + s.z2[ch]
	s.z2[ch] = s.b2*x - s.a2*y
	return y
}

// ── Limiter ──

// While two tracks play together their sum can pass full scale. A
// look-ahead limiter turns the level down just before such peaks instead of
// clipping them.
const (
	lookahead = Rate * 5 / 1000 // frames
	ceiling   = 0.989           // -0.1 dBFS
	// Lossy encoding lets peaks grow a little: MP3 gets a dB of room.
	mp3Ceiling = 0.891 // -1 dBFS
)

type limiter struct {
	top    float64            // ceiling
	l, r   [lookahead]float64 // delay line
	need   []minEntry         // sliding minimum of the gain each frame needs
	head   int
	i      int64
	env    float64
	attack float64
	rel    float64
}

type minEntry struct {
	i    int64
	gain float64
}

func newLimiter(top float64) *limiter {
	return &limiter{
		top:    top,
		env:    1,
		attack: 1 - math.Exp(-1/(0.001*Rate)),
		rel:    1 - math.Exp(-1/(0.15*Rate)),
	}
}

func (m *limiter) process(l, r float64) (float64, float64) {
	gain := 1.0
	if peak := max(math.Abs(l), math.Abs(r)); peak > m.top {
		gain = m.top / peak
	}
	for len(m.need) > m.head && m.need[len(m.need)-1].gain >= gain {
		m.need = m.need[:len(m.need)-1]
	}
	m.need = append(m.need, minEntry{m.i, gain})
	for m.need[m.head].i < m.i-lookahead {
		m.head++
	}
	if m.head > 1024 { // compact now and then
		m.need = append(m.need[:0], m.need[m.head:]...)
		m.head = 0
	}
	target := m.need[m.head].gain
	if target < m.env {
		m.env += (target - m.env) * m.attack
	} else {
		m.env += (target - m.env) * m.rel
	}

	slot := int(m.i % lookahead)
	outL, outR := m.l[slot], m.r[slot] // the frame from lookahead frames ago
	m.l[slot], m.r[slot] = l, r
	m.i++
	clamp := func(v float64) float64 { return max(-m.top, min(m.top, v*m.env)) }
	return clamp(outL), clamp(outR)
}
