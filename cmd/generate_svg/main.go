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
		prompt      = flag.String("p", "", "the build request / what to draw (required)")
		out         = flag.String("o", "", "output .svg file path (required)")
		model       = flag.String("m", "", "model alias for `claude --model` (e.g. opus, sonnet); empty = claude default")
		retries     = flag.Int("retries", 3, "max repair attempts when output is invalid")
		minElements = flag.Int("min-elements", 8, "reject builds with fewer drawable elements")
		canvas      = flag.Int("canvas", 1024, "square viewBox size hinted to the model")
		timeout     = flag.Duration("timeout", 3*time.Minute, "per-attempt timeout for the claude call")
		png         = flag.Bool("png", false, "also render a PNG preview next to the SVG (needs rsvg-convert or macOS qlmanage)")
		pngSize     = flag.Int("png-size", 0, "PNG preview pixel size; 0 = use --canvas")
		verbose     = flag.Bool("v", false, "verbose: stream claude output and progress to stderr")
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	res, err := gen.Generate(ctx, gen.Options{
		Request:     *prompt,
		Model:       *model,
		Canvas:      *canvas,
		MinElements: *minElements,
		Retries:     *retries,
		Timeout:     *timeout,
		Verbose:     *verbose,
		Log:         os.Stderr,
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
}
