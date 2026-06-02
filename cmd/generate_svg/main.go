// Command generate_svg produces an SVG illustration from a text prompt by
// shelling out to the `claude` CLI, applying MineBench-style prompt engineering.
//
// Usage:
//
//	generate_svg -p "Generate a cute boat floating on a riverside" -o sample.svg
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/hoveychen/svg_generator/internal/gen"
)

func main() {
	var (
		prompt       = flag.String("p", "", "the build request / what to draw (required)")
		out          = flag.String("o", "", "output .svg file path (required)")
		model        = flag.String("m", "", "model alias for `claude --model` (e.g. opus, sonnet); empty = claude default")
		retries      = flag.Int("retries", 3, "max repair attempts when output is invalid")
		minElements  = flag.Int("min-elements", 8, "reject builds with fewer drawable elements")
		canvas       = flag.Int("canvas", 1024, "square viewBox size hinted to the model")
		timeout      = flag.Duration("timeout", 3*time.Minute, "per-attempt timeout for the claude call")
		png          = flag.Bool("png", false, "also render a PNG preview next to the SVG (needs rsvg-convert or macOS qlmanage)")
		pngSize      = flag.Int("png-size", 0, "PNG preview pixel size; 0 = use --canvas")
		refineRounds = flag.Int("refine-rounds", 0, "vision-critique redraw rounds: render, critique the image, redraw, keep best (needs a renderer)")
		animate      = flag.Bool("animate", false, "produce a self-contained animated SVG (SMIL): movable parts get pivots + looping motion")
		style        = flag.String("style", "", "style preset: flat, line-art, realistic, pixel, isometric, watercolor, low-poly, retro")
		gif          = flag.Bool("gif", false, "also export an animated GIF (needs Chrome + ffmpeg or ImageMagick); best with --animate")
		gifFrames    = flag.Int("gif-frames", 24, "number of frames to capture for the GIF")
		gifSeconds   = flag.Float64("gif-seconds", 3.0, "seconds of the animation timeline to sample into the GIF")
		pixelize     = flag.Bool("pixelize", false, "also render a true pixel-art PNG: high-res render → downsample → palette quantize → dither → outline (needs rsvg-convert or macOS qlmanage)")
		pixelType    = flag.String("pixel-type", "scene", "pixel-art asset type: icon, item, character, boss, tile, scene (sets resolution + co-designs the SVG)")
		palette      = flag.String("palette", "db16", "pixel-art palette: db16, pico8, or auto (median-cut from the image)")
		pixelRes     = flag.Int("pixel-res", 0, "override pixel-art logical resolution on the longest side; 0 = use the --pixel-type default")
		pixelOutline = flag.Bool("pixel-outline", true, "add a selective dark silhouette rim in pixel-art mode")
		pixelCleanup = flag.Bool("pixel-cleanup", true, "majority-filter the grid to dissolve orphan noise pixels (big readability win)")
		pixelDither  = flag.Bool("pixel-dither", false, "apply selective Bayer dithering to gradient regions only (off by default; flat areas stay clean)")
		verbose      = flag.Bool("v", false, "verbose: stream claude output and progress to stderr")
	)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "generate_svg — LLM-driven SVG generator (MineBench-style prompting)\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n  generate_svg -p \"<request>\" -o <file.svg> [flags]\n\nFlags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if strings.TrimSpace(*prompt) == "" || strings.TrimSpace(*out) == "" {
		flag.Usage()
		fmt.Fprintln(os.Stderr, "\nerror: both -p and -o are required")
		os.Exit(2)
	}
	if err := gen.ValidateStyle(*style); err != nil {
		fmt.Fprintf(os.Stderr, "generate_svg: %v\n", err)
		os.Exit(2)
	}
	if *pixelize {
		if err := gen.ValidatePalette(*palette); err != nil {
			fmt.Fprintf(os.Stderr, "generate_svg: %v\n", err)
			os.Exit(2)
		}
		if err := gen.ValidatePixelType(*pixelType); err != nil {
			fmt.Fprintf(os.Stderr, "generate_svg: %v\n", err)
			os.Exit(2)
		}
	}

	// Co-design the SVG for its asset type only when pixelizing.
	genPixelType := ""
	if *pixelize {
		genPixelType = *pixelType
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	res, err := gen.Generate(ctx, gen.Options{
		Request:      *prompt,
		Model:        *model,
		Canvas:       *canvas,
		MinElements:  *minElements,
		Retries:      *retries,
		RefineRounds: *refineRounds,
		Animate:      *animate,
		Style:        *style,
		PixelType:    genPixelType,
		Timeout:      *timeout,
		Verbose:      *verbose,
		Log:          os.Stderr,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate_svg: %v\n", err)
		os.Exit(1)
	}

	if dir := filepath.Dir(*out); dir != "" && dir != "." {
		if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
			fmt.Fprintf(os.Stderr, "generate_svg: cannot create output directory: %v\n", mkErr)
			os.Exit(1)
		}
	}
	if writeErr := os.WriteFile(*out, []byte(res.SVG+"\n"), 0o644); writeErr != nil {
		fmt.Fprintf(os.Stderr, "generate_svg: cannot write %s: %v\n", *out, writeErr)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "generate_svg: wrote %s (%d drawable elements, %d attempt(s))\n",
		*out, res.Elements, res.Attempts)
	if *refineRounds > 0 {
		fmt.Fprintf(os.Stderr, "generate_svg: refined over %d round(s); best critique score %s\n",
			res.RefineRounds, gen.ScoreLabel(res.Score))
	}
	if *animate {
		fmt.Fprintf(os.Stderr, "generate_svg: animated with %d SMIL motion element(s) — open the .svg in a browser to see it move\n",
			gen.CountAnimations(res.SVG))
	}

	if *png {
		size := *pngSize
		if size <= 0 {
			size = *canvas
		}
		pngPath := gen.PNGPath(*out)
		if err := gen.RenderPNG(*out, pngPath, size); err != nil {
			// The SVG is the primary deliverable and already written; a preview
			// failure is a warning, not a hard error.
			fmt.Fprintf(os.Stderr, "generate_svg: PNG preview skipped: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "generate_svg: rendered preview %s (%dpx)\n", pngPath, size)
		}
	}

	if *gif {
		if !*animate {
			fmt.Fprintln(os.Stderr, "generate_svg: note --gif without --animate; the SVG is static, so the GIF will not move")
		}
		size := *pngSize
		if size <= 0 {
			size = *canvas
		}
		gifPath := gen.GIFPath(*out)
		durationMs := int(*gifSeconds * 1000)
		if err := gen.RenderGIF(*out, gifPath, size, *gifFrames, durationMs); err != nil {
			fmt.Fprintf(os.Stderr, "generate_svg: GIF export skipped: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "generate_svg: exported %s (%d frames over %.1fs)\n", gifPath, *gifFrames, *gifSeconds)
		}
	}

	if *pixelize {
		// Resolution comes from the asset type unless explicitly overridden.
		res := *pixelRes
		if res <= 0 {
			res = gen.PixelTypeResolution(*pixelType)
		}
		// Render a high-resolution source first, then post-process it into pixel
		// art — the Dead Cells "render high, downsample smart" recipe. A render
		// or post-process failure is a warning; the SVG is already written.
		// Keep the source comfortably above the logical grid (~6x) so even a
		// 240–320px scene grid downsamples from real detail, not a near-1:1 image.
		srcSize := res * 6
		if srcSize < *canvas {
			srcSize = *canvas
		}
		if srcSize > 2048 {
			srcSize = 2048
		}
		if err := pixelizeFrom(*out, srcSize, gen.PixelizeOptions{
			Resolution: res,
			Palette:    *palette,
			Dither:     *pixelDither,
			Cleanup:    *pixelCleanup,
			Outline:    *pixelOutline,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "generate_svg: pixel-art render skipped: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "generate_svg: rendered pixel art %s (type %s, palette %s, %dpx grid)\n",
				gen.PixelPNGPath(*out), *pixelType, *palette, res)
		}
	}
}

// pixelizeFrom rasterizes the SVG to a high-resolution temporary PNG and runs
// the pixel-art pipeline on it, writing "<base>-pixel.png" next to the SVG.
func pixelizeFrom(svgPath string, srcSize int, opts gen.PixelizeOptions) error {
	tmpDir, err := os.MkdirTemp("", "generate_svg_pixel_*")
	if err != nil {
		return fmt.Errorf("temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	srcPNG := filepath.Join(tmpDir, "src.png")
	if err := gen.RenderPNG(svgPath, srcPNG, srcSize); err != nil {
		return err
	}
	return gen.Pixelize(srcPNG, gen.PixelPNGPath(svgPath), opts)
}
