package main

// photoTone gets an album cover ready for e-ink, which shows photos flatter and darker
// than a phone does. The steps come from Kindle Photo Prep
// (github.com/kuslota/kindle-photo-frame, MIT), gentler than its "Medium" preset so dark
// covers stay dark: trim the darkest and lightest 1 % to pure black and white, an S-curve
// for contrast, lift the midtones (gamma 1.3), ease the contrast back a touch and sharpen.

import (
	"image"
	"math"
)

const (
	toneClip    = 0.01
	toneSigmoid = 3.5
	toneGamma   = 1.3
	toneLinear  = 0.95
	toneSharp   = 1.4
)

func photoTone(g *image.Gray) {
	w, h := g.Rect.Dx(), g.Rect.Dy()
	if w == 0 || h == 0 {
		return
	}
	var hist [256]int
	for y := 0; y < h; y++ {
		for _, p := range g.Pix[y*g.Stride : y*g.Stride+w] {
			hist[p]++
		}
	}
	cut := int(float64(w*h) * toneClip)
	lo, hi := 0, 255
	for n := 0; lo < 255 && n+hist[lo] <= cut; lo++ {
		n += hist[lo]
	}
	for n := 0; hi > 0 && n+hist[hi] <= cut; hi-- {
		n += hist[hi]
	}
	if hi <= lo {
		lo, hi = 0, 255
	}
	sLo := 1 / (1 + math.Exp(toneSigmoid*0.5))
	sHi := 1 / (1 + math.Exp(-toneSigmoid*0.5))
	var lut [256]float64
	for v := range lut {
		x := math.Min(1, math.Max(0, float64(v-lo)/float64(hi-lo)))
		x = (1/(1+math.Exp(-toneSigmoid*(x-0.5))) - sLo) / (sHi - sLo)
		lut[v] = 255 * math.Pow(x, 1/toneGamma)
	}
	// the curve, then the gentler contrast around the picture's average
	sum := 0.0
	for v, n := range hist {
		sum += lut[v] * float64(n)
	}
	mean := sum / float64(w*h)
	var lut8 [256]uint8
	for v := range lut {
		lut8[v] = clamp8(mean + (lut[v]-mean)*toneLinear)
	}
	src := make([]uint8, w*h)
	for y := 0; y < h; y++ {
		row := g.Pix[y*g.Stride : y*g.Stride+w]
		for x, p := range row {
			src[y*w+x] = lut8[p]
		}
	}
	// sharpen: push each pixel away from a soft blur of it (3×3, centre 5, others 1)
	for y := 0; y < h; y++ {
		row := g.Pix[y*g.Stride : y*g.Stride+w]
		for x := range row {
			c := float64(src[y*w+x])
			if x == 0 || y == 0 || x == w-1 || y == h-1 {
				row[x] = uint8(c)
				continue
			}
			s := 4 * c
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					s += float64(src[(y+dy)*w+x+dx])
				}
			}
			row[x] = clamp8(c + (c-s/13)*(toneSharp-1))
		}
	}
}

func clamp8(v float64) uint8 {
	return uint8(math.Round(math.Min(255, math.Max(0, v))))
}
