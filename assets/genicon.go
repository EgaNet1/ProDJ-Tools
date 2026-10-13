//go:build ignore

// Genicon draws the ProDJ Tools logo — a record with an amber label on a dark
// tile — and writes it as assets/icon.ico (for the exe and the installer),
// assets/icon.png and web/icon.png (for the app window).
//
// Run from the project root:  go run assets/genicon.go
//
// Every size is drawn separately rather than scaled down from a large
// picture, so the 16 px icon stays crisp.
package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
)

const supersample = 4 // samples per pixel side, for smooth edges

var (
	tileTop    = rgb(0x2a, 0x2e, 0x35)
	tileBottom = rgb(0x15, 0x17, 0x1b)
	vinyl      = rgb(0x0b, 0x0c, 0x0e)
	groove     = rgb(0x1d, 0x20, 0x25)
	rim        = rgb(0x4a, 0x4f, 0x58)
	amber      = rgb(0xf0, 0xa3, 0x30)
	amberDark  = rgb(0xc9, 0x7f, 0x18)
	hole       = rgb(0x15, 0x17, 0x1b)
)

type rgba struct{ r, g, b, a float64 }

func rgb(r, g, b uint8) rgba { return rgba{float64(r), float64(g), float64(b), 1} }

// over paints c with opacity a on top of dst.
func over(dst, c rgba, a float64) rgba {
	out := dst.a + a*(1-dst.a)
	if out == 0 {
		return rgba{}
	}
	mix := func(d, s float64) float64 { return (s*a + d*dst.a*(1-a)) / out }
	return rgba{mix(dst.r, c.r), mix(dst.g, c.g), mix(dst.b, c.b), out}
}

func lerp(a, b rgba, t float64) rgba {
	return rgba{a.r + (b.r-a.r)*t, a.g + (b.g-a.g)*t, a.b + (b.b-a.b)*t, 1}
}

// sample is the colour of the logo at (x, y), both from 0 to 1.
func sample(x, y float64, size int) rgba {
	var c rgba

	// Tile: a rounded square with a soft vertical gradient.
	const inset, radius = 0.02, 0.22
	qx := math.Max(math.Abs(x-0.5)-(0.5-inset-radius), 0)
	qy := math.Max(math.Abs(y-0.5)-(0.5-inset-radius), 0)
	if math.Hypot(qx, qy) > radius {
		return c
	}
	c = over(c, lerp(tileTop, tileBottom, y), 1)

	dx, dy := x-0.5, y-0.5
	r := math.Hypot(dx, dy)
	angle := math.Atan2(dy, dx)

	switch {
	case r <= 0.04: // spindle hole
		c = over(c, hole, 1)
	case r <= 0.165: // label, darker towards its edge
		c = over(c, lerp(amber, amberDark, math.Max(0, (r-0.11)/0.055)), 1)
	case r <= 0.385: // grooves; on small icons they would only blur
		c = over(c, vinyl, 1)
		if size >= 48 && math.Mod(r*100, 3) < 0.6 && r > 0.19 && r < 0.37 {
			c = over(c, groove, 1)
		}
		// Light catching the record: two opposite wedges.
		for _, centre := range []float64{-math.Pi * 3 / 4, math.Pi / 4} {
			d := math.Abs(math.Remainder(angle-centre, 2*math.Pi))
			if d < 0.35 {
				c = over(c, rgb(0xff, 0xff, 0xff), 0.07*(1-d/0.35))
			}
		}
	case r <= 0.40: // rim
		c = over(c, rim, 1)
	}
	return c
}

func render(size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	n := float64(supersample * supersample)
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			var sr, sg, sb, sa float64
			for sy := 0; sy < supersample; sy++ {
				for sx := 0; sx < supersample; sx++ {
					x := (float64(px) + (float64(sx)+0.5)/supersample) / float64(size)
					y := (float64(py) + (float64(sy)+0.5)/supersample) / float64(size)
					c := sample(x, y, size)
					sr += c.r * c.a
					sg += c.g * c.a
					sb += c.b * c.a
					sa += c.a
				}
			}
			if sa == 0 {
				continue
			}
			img.SetNRGBA(px, py, color.NRGBA{
				R: uint8(math.Round(sr / sa)),
				G: uint8(math.Round(sg / sa)),
				B: uint8(math.Round(sb / sa)),
				A: uint8(math.Round(sa / n * 255)),
			})
		}
	}
	return img
}

func encode(img image.Image) []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		log.Fatal(err)
	}
	return buf.Bytes()
}

// writeICO stores PNG images in an .ico file, which Windows Vista and later
// read at every size.
func writeICO(path string, sizes []int) {
	var head, body bytes.Buffer
	binary.Write(&head, binary.LittleEndian, [3]uint16{0, 1, uint16(len(sizes))})
	offset := 6 + 16*len(sizes)
	for _, s := range sizes {
		data := encode(render(s))
		dim := uint8(s % 256) // 0 means 256
		binary.Write(&head, binary.LittleEndian, struct {
			W, H, Colors, Reserved uint8
			Planes, Bits           uint16
			Size, Offset           uint32
		}{dim, dim, 0, 0, 1, 32, uint32(len(data)), uint32(offset)})
		body.Write(data)
		offset += len(data)
	}
	if err := os.WriteFile(path, append(head.Bytes(), body.Bytes()...), 0o644); err != nil {
		log.Fatal(err)
	}
}

func main() {
	writeICO("assets/icon.ico", []int{16, 20, 24, 32, 40, 48, 64, 128, 256})
	big := encode(render(256))
	for _, path := range []string{"assets/icon.png", "web/icon.png"} {
		if err := os.WriteFile(path, big, 0o644); err != nil {
			log.Fatal(err)
		}
	}
}
