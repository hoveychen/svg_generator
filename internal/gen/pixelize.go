package gen

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"sort"
	"strings"
)

// PixelizeOptions controls the SVG-preview-to-pixel-art post-processing pass.
//
// The pipeline is deliberately the Dead Cells recipe rather than a naive
// rasterize-and-shrink: render the vector art at high resolution (callers do
// that via RenderPNG), then *intelligently* downsample to a coarse logical
// grid, quantize to a constrained palette, dither gradients, and add a
// selective dark rim. The point is that real pixel art is a post-process, not
// something the generator can fake by drawing many tiny squares.
type PixelizeOptions struct {
	// Resolution is the target logical grid size on the longest side, in
	// "pixels" (the chunky kind). 128 reads as detailed-but-clearly-pixel-art;
	// 64 is blockier and more retro. Aspect ratio is preserved.
	Resolution int
	// Palette names the color set: "db16" (DawnBringer 16), "pico8" (PICO-8
	// 16), or "auto" (median-cut extraction from the image itself).
	Palette string
	// AutoColors is the palette size when Palette == "auto" (default 16).
	AutoColors int
	// Dither enables Bayer 4x4 ordered dithering before quantization. It is
	// applied *selectively* — only in locally high-variance (gradient/detail)
	// regions — so flat areas like skies stay clean instead of turning into a
	// carpet of checkerboard noise that drowns the silhouette.
	Dither bool
	// Cleanup runs a majority filter on the quantized grid to remove orphan
	// "salt-and-pepper" pixels, letting color regions read as solid clusters.
	// This is the single biggest readability win at low resolution.
	Cleanup bool
	// Outline darkens silhouette-boundary pixels, the selective dark rim that
	// makes sprites pop against a background.
	Outline bool
	// OutScale is the nearest-neighbor upscale factor for the written PNG so
	// the hard pixel blocks stay crisp. 0 = derive a factor that brings the
	// output back to roughly the source size.
	OutScale int
	// AlphaCutoff binarizes transparency: source alpha below this becomes fully
	// transparent, at/above becomes fully opaque. Hard edges, no fuzzy AA halo.
	// 0 uses the default (128).
	AlphaCutoff uint8
}

// namedPalettes holds the built-in constrained palettes as hex strings.
var namedPalettes = map[string][]string{
	// DawnBringer 16 — the canonical "looks like a real game" 16-color ramp.
	"db16": {
		"140c1c", "442434", "30346d", "4e4a4e", "854c30", "346524", "d04648", "757161",
		"597dce", "d27d2c", "8595a1", "6daa2c", "d2aa99", "6dc2ca", "dad45e", "deeed6",
	},
	// PICO-8 fantasy-console 16-color palette.
	"pico8": {
		"000000", "1d2b53", "7e2553", "008751", "ab5236", "5f574f", "c2c3c7", "fff1e8",
		"ff004d", "ffa300", "ffec27", "00e436", "29adff", "83769c", "ff77a8", "ffccaa",
	},
}

// PaletteNames lists the selectable --palette values.
func PaletteNames() []string { return []string{"db16", "pico8", "auto"} }

// ValidatePalette returns an error naming the valid options when name is not a
// recognized palette. Empty is treated as the default and accepted.
func ValidatePalette(name string) error {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "db16", "pico8", "auto":
		return nil
	default:
		return fmt.Errorf("unknown palette %q (choose one of: %s)", name, strings.Join(PaletteNames(), ", "))
	}
}

// PixelPNGPath derives the pixel-art output path from an SVG output path:
// "boat.svg" -> "boat-pixel.png", "boat" -> "boat-pixel.png".
func PixelPNGPath(svgPath string) string {
	base := svgPath
	if dot := strings.LastIndex(base, "."); dot >= 0 && strings.EqualFold(base[dot:], ".svg") {
		base = base[:dot]
	}
	return base + "-pixel.png"
}

// rgb is a working color in float space for distance math and averaging.
type rgb struct{ r, g, b float64 }

func (c rgb) toNRGBA(a uint8) color.NRGBA {
	clamp := func(v float64) uint8 {
		if v < 0 {
			return 0
		}
		if v > 255 {
			return 255
		}
		return uint8(v + 0.5)
	}
	return color.NRGBA{R: clamp(c.r), G: clamp(c.g), B: clamp(c.b), A: a}
}

func parseHexPalette(hexes []string) []rgb {
	pal := make([]rgb, 0, len(hexes))
	for _, h := range hexes {
		if len(h) != 6 {
			continue
		}
		var r, g, b int
		fmt.Sscanf(h, "%02x%02x%02x", &r, &g, &b)
		pal = append(pal, rgb{float64(r), float64(g), float64(b)})
	}
	return pal
}

// nearest returns the index of the palette entry closest to c in squared
// Euclidean RGB distance.
func nearest(pal []rgb, c rgb) int {
	best, bestD := 0, math.MaxFloat64
	for i, p := range pal {
		dr, dg, db := c.r-p.r, c.g-p.g, c.b-p.b
		d := dr*dr + dg*dg + db*db
		if d < bestD {
			best, bestD = i, d
		}
	}
	return best
}

// bayer4 is the normalized 4x4 ordered-dither threshold matrix, centered on 0
// so it perturbs colors both up and down before quantization.
var bayer4 = func() [4][4]float64 {
	raw := [4][4]int{
		{0, 8, 2, 10},
		{12, 4, 14, 6},
		{3, 11, 1, 9},
		{15, 7, 13, 5},
	}
	var m [4][4]float64
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			m[y][x] = (float64(raw[y][x])+0.5)/16.0 - 0.5 // in [-0.5, 0.5)
		}
	}
	return m
}()

// Pixelize reads srcPNG, runs the pixel-art pipeline, and writes dstPNG.
func Pixelize(srcPNG, dstPNG string, opts PixelizeOptions) error {
	if opts.Resolution <= 0 {
		opts.Resolution = 128
	}
	if opts.AutoColors <= 0 {
		opts.AutoColors = 16
	}
	if opts.AlphaCutoff == 0 {
		opts.AlphaCutoff = 128
	}

	f, err := os.Open(srcPNG)
	if err != nil {
		return fmt.Errorf("open source png: %w", err)
	}
	src, err := png.Decode(f)
	f.Close()
	if err != nil {
		return fmt.Errorf("decode source png: %w", err)
	}

	small := downsample(src, opts.Resolution, opts.AlphaCutoff)
	pal := buildPalette(small, opts)
	w, h := small.Bounds().Dx(), small.Bounds().Dy()
	idx := quantize(small, pal, opts.Dither)
	if opts.Cleanup {
		cleanup(idx, w, h)
	}
	applyPalette(small, idx, pal)
	if opts.Outline {
		outline(small, pal)
	}

	out := upscale(small, deriveScale(src.Bounds(), small.Bounds(), opts.OutScale))

	g, err := os.Create(dstPNG)
	if err != nil {
		return fmt.Errorf("create dest png: %w", err)
	}
	defer g.Close()
	if err := png.Encode(g, out); err != nil {
		return fmt.Errorf("encode dest png: %w", err)
	}
	return nil
}

// downsample box-averages src into a logical grid whose longest side is res,
// preserving aspect ratio. Alpha is binarized at cutoff so sprite edges stay
// hard; fully transparent blocks contribute no color.
func downsample(src image.Image, res int, cutoff uint8) *image.NRGBA {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw == 0 || sh == 0 {
		return image.NewNRGBA(image.Rect(0, 0, 1, 1))
	}
	lw, lh := res, res
	if sw >= sh {
		lh = int(math.Round(float64(res) * float64(sh) / float64(sw)))
	} else {
		lw = int(math.Round(float64(res) * float64(sw) / float64(sh)))
	}
	if lw < 1 {
		lw = 1
	}
	if lh < 1 {
		lh = 1
	}

	dst := image.NewNRGBA(image.Rect(0, 0, lw, lh))
	for ly := 0; ly < lh; ly++ {
		for lx := 0; lx < lw; lx++ {
			x0 := b.Min.X + lx*sw/lw
			x1 := b.Min.X + (lx+1)*sw/lw
			y0 := b.Min.Y + ly*sh/lh
			y1 := b.Min.Y + (ly+1)*sh/lh
			if x1 <= x0 {
				x1 = x0 + 1
			}
			if y1 <= y0 {
				y1 = y0 + 1
			}
			var sr, sg, sb, sa, opaque float64
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					r, g, bl, a := src.At(x, y).RGBA()
					af := float64(a>>8) / 255.0
					sa += af
					if a>>8 >= 8 { // weight color by coverage, ignore near-empty
						sr += float64(r>>8) * af
						sg += float64(g>>8) * af
						sb += float64(bl>>8) * af
						opaque += af
					}
				}
			}
			n := float64((x1 - x0) * (y1 - y0))
			alpha := uint8(0)
			if sa/n*255 >= float64(cutoff) {
				alpha = 255
			}
			px := color.NRGBA{A: alpha}
			if alpha != 0 && opaque > 0 {
				px.R = clampU8(sr / opaque)
				px.G = clampU8(sg / opaque)
				px.B = clampU8(sb / opaque)
			}
			dst.SetNRGBA(lx, ly, px)
		}
	}
	return dst
}

func clampU8(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v + 0.5)
}

// buildPalette returns the working palette: a named constant set, or a
// median-cut extraction from the image's opaque pixels for "auto".
func buildPalette(img *image.NRGBA, opts PixelizeOptions) []rgb {
	name := strings.ToLower(strings.TrimSpace(opts.Palette))
	if name == "" {
		name = "db16"
	}
	if hexes, ok := namedPalettes[name]; ok {
		return parseHexPalette(hexes)
	}
	// auto: median-cut from opaque pixels.
	var pts []rgb
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := img.NRGBAAt(x, y)
			if c.A == 0 {
				continue
			}
			pts = append(pts, rgb{float64(c.R), float64(c.G), float64(c.B)})
		}
	}
	pal := medianCut(pts, opts.AutoColors)
	if len(pal) == 0 {
		// Degenerate (fully transparent) image; fall back so quantize has a target.
		return parseHexPalette(namedPalettes["db16"])
	}
	return pal
}

// medianCut reduces pts to at most n representative colors by repeatedly
// splitting the box with the widest channel range at that channel's median.
func medianCut(pts []rgb, n int) []rgb {
	if len(pts) == 0 || n <= 0 {
		return nil
	}
	boxes := [][]rgb{pts}
	for len(boxes) < n {
		// pick the box with the greatest single-channel range
		bi, bestRange := -1, -1.0
		var bestCh int
		for i, box := range boxes {
			if len(box) < 2 {
				continue
			}
			ch, rng := widestChannel(box)
			if rng > bestRange {
				bi, bestRange, bestCh = i, rng, ch
			}
		}
		if bi < 0 {
			break // nothing splittable
		}
		box := boxes[bi]
		sort.Slice(box, func(a, b int) bool { return channel(box[a], bestCh) < channel(box[b], bestCh) })
		mid := len(box) / 2
		boxes[bi] = box[:mid]
		boxes = append(boxes, box[mid:])
	}
	pal := make([]rgb, 0, len(boxes))
	for _, box := range boxes {
		pal = append(pal, average(box))
	}
	return pal
}

func channel(c rgb, ch int) float64 {
	switch ch {
	case 0:
		return c.r
	case 1:
		return c.g
	default:
		return c.b
	}
}

func widestChannel(box []rgb) (int, float64) {
	min := rgb{255, 255, 255}
	max := rgb{0, 0, 0}
	for _, c := range box {
		min.r, max.r = math.Min(min.r, c.r), math.Max(max.r, c.r)
		min.g, max.g = math.Min(min.g, c.g), math.Max(max.g, c.g)
		min.b, max.b = math.Min(min.b, c.b), math.Max(max.b, c.b)
	}
	rr, rg, rb := max.r-min.r, max.g-min.g, max.b-min.b
	if rr >= rg && rr >= rb {
		return 0, rr
	}
	if rg >= rb {
		return 1, rg
	}
	return 2, rb
}

func average(box []rgb) rgb {
	var s rgb
	for _, c := range box {
		s.r += c.r
		s.g += c.g
		s.b += c.b
	}
	n := float64(len(box))
	return rgb{s.r / n, s.g / n, s.b / n}
}

// quantize maps every pixel to a palette index (-1 for transparent), returning
// the index grid in row-major order. When dither is set, Bayer perturbation is
// applied *selectively* — only where the local neighborhood varies enough to be
// a gradient or detail — so flat regions quantize cleanly.
func quantize(img *image.NRGBA, pal []rgb, dither bool) []int {
	const spread = 56.0 // perturbation magnitude for dithering
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	idx := make([]int, w*h)

	var mask []bool
	if dither {
		mask = gradientMask(img)
	}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := img.NRGBAAt(b.Min.X+x, b.Min.Y+y)
			if c.A == 0 {
				idx[y*w+x] = -1
				continue
			}
			col := rgb{float64(c.R), float64(c.G), float64(c.B)}
			if dither && mask[y*w+x] {
				t := bayer4[y&3][x&3] * spread
				col.r += t
				col.g += t
				col.b += t
			}
			idx[y*w+x] = nearest(pal, col)
		}
	}
	return idx
}

// gradientMask flags opaque pixels whose 3x3 neighborhood luminance variance
// exceeds a threshold — i.e. gradient or detail regions where dithering helps,
// as opposed to flat fills where it only adds noise.
func gradientMask(img *image.NRGBA) []bool {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	lum := func(x, y int) (float64, bool) {
		c := img.NRGBAAt(b.Min.X+x, b.Min.Y+y)
		if c.A == 0 {
			return 0, false
		}
		return 0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B), true
	}
	const threshold = 14.0 // std-dev of luminance above which we dither
	mask := make([]bool, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if _, ok := lum(x, y); !ok {
				continue
			}
			var sum, sumSq, n float64
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					nx, ny := x+dx, y+dy
					if nx < 0 || ny < 0 || nx >= w || ny >= h {
						continue
					}
					if l, ok := lum(nx, ny); ok {
						sum += l
						sumSq += l * l
						n++
					}
				}
			}
			if n < 2 {
				continue
			}
			mean := sum / n
			variance := sumSq/n - mean*mean
			if variance > threshold*threshold {
				mask[y*w+x] = true
			}
		}
	}
	return mask
}

// cleanup is a majority filter over palette indices: an opaque pixel whose own
// index is rare among its 8 neighbors while some other index dominates is
// replaced by that dominant index. This dissolves salt-and-pepper noise so
// color regions read as solid clusters — the biggest low-res readability win.
func cleanup(idx []int, w, h int) {
	src := make([]int, len(idx))
	copy(src, idx)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			self := src[y*w+x]
			if self < 0 {
				continue // leave transparency alone
			}
			counts := map[int]int{}
			selfCount := 0
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if dx == 0 && dy == 0 {
						continue
					}
					nx, ny := x+dx, y+dy
					if nx < 0 || ny < 0 || nx >= w || ny >= h {
						continue
					}
					ni := src[ny*w+nx]
					if ni < 0 {
						continue
					}
					counts[ni]++
					if ni == self {
						selfCount++
					}
				}
			}
			best, bestN := self, 0
			for k, n := range counts {
				if n > bestN {
					best, bestN = k, n
				}
			}
			// Replace only clear outliers: self barely present, neighbor strongly dominant.
			if selfCount <= 1 && bestN >= 5 {
				idx[y*w+x] = best
			}
		}
	}
}

// applyPalette writes the resolved palette colors back into img, preserving the
// binary alpha already set by downsample.
func applyPalette(img *image.NRGBA, idx []int, pal []rgb) {
	b := img.Bounds()
	w := b.Dx()
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < w; x++ {
			i := idx[y*w+x]
			if i < 0 {
				continue
			}
			img.SetNRGBA(b.Min.X+x, b.Min.Y+y, pal[i].toNRGBA(255))
		}
	}
}

// outline darkens silhouette-boundary pixels — opaque pixels with at least one
// transparent 4-neighbor — to the palette color nearest a dimmed version of
// themselves, producing the selective dark rim sprites rely on.
func outline(img *image.NRGBA, pal []rgb) {
	b := img.Bounds()
	type pt struct{ x, y int }
	var edges []pt
	transparent := func(x, y int) bool {
		if x < b.Min.X || y < b.Min.Y || x >= b.Max.X || y >= b.Max.Y {
			return true
		}
		return img.NRGBAAt(x, y).A == 0
	}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.NRGBAAt(x, y).A == 0 {
				continue
			}
			if transparent(x-1, y) || transparent(x+1, y) || transparent(x, y-1) || transparent(x, y+1) {
				edges = append(edges, pt{x, y})
			}
		}
	}
	for _, e := range edges {
		c := img.NRGBAAt(e.x, e.y)
		dim := rgb{float64(c.R) * 0.45, float64(c.G) * 0.45, float64(c.B) * 0.45}
		p := pal[nearest(pal, dim)]
		img.SetNRGBA(e.x, e.y, p.toNRGBA(255))
	}
}

// deriveScale picks the nearest-neighbor upscale factor. An explicit factor
// wins; otherwise we bring the logical grid back to roughly the source size so
// the preview is a familiar resolution with chunky blocks.
func deriveScale(srcB, smallB image.Rectangle, explicit int) int {
	if explicit > 0 {
		return explicit
	}
	longSrc := math.Max(float64(srcB.Dx()), float64(srcB.Dy()))
	longSmall := math.Max(float64(smallB.Dx()), float64(smallB.Dy()))
	if longSmall == 0 {
		return 1
	}
	s := int(math.Round(longSrc / longSmall))
	if s < 1 {
		s = 1
	}
	return s
}

// upscale nearest-neighbor expands img by an integer factor so every logical
// pixel becomes a solid scale×scale block — no interpolation, hard edges.
func upscale(img *image.NRGBA, scale int) *image.NRGBA {
	if scale <= 1 {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, w*scale, h*scale))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := img.NRGBAAt(b.Min.X+x, b.Min.Y+y)
			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					dst.SetNRGBA(x*scale+dx, y*scale+dy, c)
				}
			}
		}
	}
	return dst
}
