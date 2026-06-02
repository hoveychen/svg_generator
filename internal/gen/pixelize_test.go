package gen

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestValidatePalette(t *testing.T) {
	for _, ok := range []string{"", "db16", "pico8", "auto", "DB16", " pico8 "} {
		if err := ValidatePalette(ok); err != nil {
			t.Errorf("ValidatePalette(%q) = %v, want nil", ok, err)
		}
	}
	if err := ValidatePalette("nope"); err == nil {
		t.Error("ValidatePalette(\"nope\") = nil, want error")
	}
}

func TestPixelPNGPath(t *testing.T) {
	cases := map[string]string{
		"boat.svg":     "boat-pixel.png",
		"out/boat.svg": "out/boat-pixel.png",
		"boat":         "boat-pixel.png",
		"boat.SVG":     "boat-pixel.png",
	}
	for in, want := range cases {
		if got := PixelPNGPath(in); got != want {
			t.Errorf("PixelPNGPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNearestColor(t *testing.T) {
	pal := []rgb{{0, 0, 0}, {255, 255, 255}, {255, 0, 0}}
	cases := []struct {
		c    rgb
		want int
	}{
		{rgb{10, 10, 10}, 0},     // near black
		{rgb{240, 240, 240}, 1},  // near white
		{rgb{200, 20, 20}, 2},    // near red
	}
	for _, tc := range cases {
		if got := nearest(pal, tc.c); got != tc.want {
			t.Errorf("nearest(%v) = %d, want %d", tc.c, got, tc.want)
		}
	}
}

// The Bayer matrix must be centered (mean ~0) and bounded in [-0.5, 0.5) so it
// perturbs colors symmetrically without bias.
func TestBayerMatrix(t *testing.T) {
	var sum float64
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			v := bayer4[y][x]
			if v < -0.5 || v >= 0.5 {
				t.Errorf("bayer4[%d][%d] = %f out of [-0.5, 0.5)", y, x, v)
			}
			sum += v
		}
	}
	if math.Abs(sum) > 1e-9 {
		t.Errorf("bayer4 sum = %f, want ~0 (centered)", sum)
	}
}

// downsample must preserve aspect ratio with the longest side equal to res.
func TestDownsampleDimensions(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 800, 400))
	for y := 0; y < 400; y++ {
		for x := 0; x < 800; x++ {
			src.SetNRGBA(x, y, color.NRGBA{R: 100, G: 150, B: 200, A: 255})
		}
	}
	small := downsample(src, 64, 128)
	if got := small.Bounds().Dx(); got != 64 {
		t.Errorf("width = %d, want 64", got)
	}
	if got := small.Bounds().Dy(); got != 32 {
		t.Errorf("height = %d, want 32 (aspect-preserved)", got)
	}
}

// upscale must turn every logical pixel into a uniform scale×scale block — the
// defining property of crisp pixel art (no interpolation).
func TestUpscaleHardBlocks(t *testing.T) {
	small := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	small.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	small.SetNRGBA(1, 0, color.NRGBA{G: 255, A: 255})
	small.SetNRGBA(0, 1, color.NRGBA{B: 255, A: 255})
	small.SetNRGBA(1, 1, color.NRGBA{R: 255, G: 255, A: 255})

	const scale = 5
	big := upscale(small, scale)
	if big.Bounds().Dx() != 2*scale || big.Bounds().Dy() != 2*scale {
		t.Fatalf("upscaled bounds = %v, want %dx%d", big.Bounds(), 2*scale, 2*scale)
	}
	// Every scale×scale block must be a single uniform color matching the source.
	for by := 0; by < 2; by++ {
		for bx := 0; bx < 2; bx++ {
			want := small.NRGBAAt(bx, by)
			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					if got := big.NRGBAAt(bx*scale+dx, by*scale+dy); got != want {
						t.Fatalf("block (%d,%d) pixel (%d,%d) = %v, want %v", bx, by, dx, dy, got, want)
					}
				}
			}
		}
	}
}

// quantize must constrain every opaque pixel to a palette color.
func TestQuantizeConstrainsToPalette(t *testing.T) {
	pal := parseHexPalette(namedPalettes["db16"])
	palSet := map[color.NRGBA]bool{}
	for _, p := range pal {
		palSet[p.toNRGBA(255)] = true
	}

	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 16), G: uint8(y * 16), B: 128, A: 255})
		}
	}
	idx := quantize(img, pal, true)
	applyPalette(img, idx, pal)
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			c := img.NRGBAAt(x, y)
			if !palSet[c] {
				t.Fatalf("pixel (%d,%d) = %v not in palette", x, y, c)
			}
		}
	}
}

// cleanup must dissolve an isolated orphan pixel surrounded by a dominant index.
func TestCleanupRemovesOrphans(t *testing.T) {
	const w, h = 3, 3
	idx := []int{
		0, 0, 0,
		0, 5, 0, // lone "5" orphan in a field of 0
		0, 0, 0,
	}
	cleanup(idx, w, h)
	if idx[4] != 0 {
		t.Errorf("orphan pixel = %d, want 0 (replaced by dominant neighbor)", idx[4])
	}
}

// cleanup must NOT erase a legitimate cluster (a 2x2 block is not an orphan).
func TestCleanupKeepsClusters(t *testing.T) {
	const w, h = 4, 4
	idx := []int{
		0, 0, 0, 0,
		0, 5, 5, 0,
		0, 5, 5, 0,
		0, 0, 0, 0,
	}
	cleanup(idx, w, h)
	for _, i := range []int{5, 6, 9, 10} {
		if idx[i] != 5 {
			t.Errorf("cluster pixel %d = %d, want 5 (clusters preserved)", i, idx[i])
		}
	}
}

// End-to-end: a synthetic gradient PNG through Pixelize yields an output whose
// every opaque color is in the chosen palette and whose size is the logical
// grid times an integer scale.
func TestPixelizeEndToEnd(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "src.png")
	dstPath := filepath.Join(dir, "out-pixel.png")

	const srcW, srcH = 512, 512
	src := image.NewNRGBA(image.Rect(0, 0, srcW, srcH))
	for y := 0; y < srcH; y++ {
		for x := 0; x < srcW; x++ {
			src.SetNRGBA(x, y, color.NRGBA{R: uint8(x / 2), G: uint8(y / 2), B: 80, A: 255})
		}
	}
	if err := writePNG(srcPath, src); err != nil {
		t.Fatal(err)
	}

	if err := Pixelize(srcPath, dstPath, PixelizeOptions{
		Resolution: 64, Palette: "db16", Dither: true, Outline: true,
	}); err != nil {
		t.Fatalf("Pixelize: %v", err)
	}

	f, err := os.Open(dstPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	out, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}

	// Output longest side should be 64 (logical) * derived scale (512/64 = 8) = 512.
	if got := out.Bounds().Dx(); got != 512 {
		t.Errorf("output width = %d, want 512 (64 grid × 8 scale)", got)
	}

	palSet := map[color.NRGBA]bool{}
	for _, p := range parseHexPalette(namedPalettes["db16"]) {
		palSet[p.toNRGBA(255)] = true
	}
	b := out.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := out.At(x, y).RGBA()
			if a == 0 {
				continue
			}
			c := color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(bl >> 8), A: 255}
			if !palSet[c] {
				t.Fatalf("output pixel (%d,%d) = %v not in db16 palette", x, y, c)
			}
		}
	}
}

// TestPixelizeAutoPalette exercises the median-cut path end-to-end.
func TestPixelizeAutoPalette(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "src.png")
	dstPath := filepath.Join(dir, "out-pixel.png")

	src := image.NewNRGBA(image.Rect(0, 0, 256, 256))
	for y := 0; y < 256; y++ {
		for x := 0; x < 256; x++ {
			src.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: uint8((x + y) / 2), A: 255})
		}
	}
	if err := writePNG(srcPath, src); err != nil {
		t.Fatal(err)
	}
	if err := Pixelize(srcPath, dstPath, PixelizeOptions{
		Resolution: 32, Palette: "auto", AutoColors: 8, Dither: false, Outline: false,
	}); err != nil {
		t.Fatalf("Pixelize auto: %v", err)
	}

	// The output must use at most AutoColors distinct opaque colors.
	f, _ := os.Open(dstPath)
	defer f.Close()
	out, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[color.NRGBA]bool{}
	b := out.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := out.At(x, y).RGBA()
			if a == 0 {
				continue
			}
			seen[color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(bl >> 8), A: 255}] = true
		}
	}
	if len(seen) > 8 {
		t.Errorf("auto palette produced %d colors, want <= 8", len(seen))
	}
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
