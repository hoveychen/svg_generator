package gen

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// refine runs the perception loop: render the current SVG, have a vision model
// critique the render, redraw guided by the critique, and keep the
// highest-scoring version across all rounds.
//
// For N rounds it performs N redraws and N+1 critiques (every version, including
// the final redraw, is scored so it can win). Any render/critique/redraw failure
// stops the loop early and returns the best version found so far.
func refine(ctx context.Context, opts Options, runner Runner, initial *Result) (*Result, error) {
	tmpDir, err := os.MkdirTemp("", "generate_svg_refine_*")
	if err != nil {
		opts.logf("[refine] cannot create temp dir: %v (returning initial generation)", err)
		return initial, nil
	}
	defer os.RemoveAll(tmpDir)

	refineSys := RefineSystemPrompt(opts.Canvas, opts.MinElements)
	if opts.Animate {
		refineSys = RefineSystemPromptAnimated(opts.Canvas, opts.MinElements)
	}

	current := initial.SVG
	best := initial.SVG
	bestScore := -1
	roundsRun := 0

	for round := 0; round <= opts.RefineRounds; round++ {
		score, critText, err := renderAndCritique(ctx, opts, runner, current, tmpDir, round)
		if err != nil {
			opts.logf("[refine] round %d: %v (stopping, keeping best so far)", round, err)
			break
		}
		opts.logf("[refine] critique %d/%d: score=%s :: %s", round, opts.RefineRounds, ScoreLabel(score), firstLine(critText))

		if score > bestScore {
			bestScore = score
			best = current
		}

		if round == opts.RefineRounds {
			break // final version scored; nothing left to redraw
		}

		redrawn, _, err := runWithRepair(ctx, runner, refineSys, RefineUserPrompt(opts.Request, critText), opts)
		if err != nil {
			opts.logf("[refine] redraw after round %d failed: %v (keeping best so far)", round, err)
			break
		}
		current = redrawn
		roundsRun++
	}

	// If no critique ever produced a usable score, fall back to the initial.
	if bestScore < 0 {
		opts.logf("[refine] no usable critique scores; returning initial generation")
		best = initial.SVG
	}

	return &Result{
		SVG:          best,
		Elements:     CountDrawable(best),
		Attempts:     initial.Attempts,
		Score:        bestScore,
		RefineRounds: roundsRun,
	}, nil
}

// renderAndCritique writes svg to a temp file, rasterizes it, and returns the
// vision model's score and critique text.
func renderAndCritique(ctx context.Context, opts Options, runner Runner, svg, tmpDir string, round int) (int, string, error) {
	svgPath := filepath.Join(tmpDir, fmt.Sprintf("v%d.svg", round))
	pngPath := filepath.Join(tmpDir, fmt.Sprintf("v%d.png", round))

	if err := os.WriteFile(svgPath, []byte(svg), 0o644); err != nil {
		return 0, "", fmt.Errorf("write svg for critique: %w", err)
	}
	if err := RenderPNG(svgPath, pngPath, opts.Canvas); err != nil {
		return 0, "", fmt.Errorf("render for critique: %w", err)
	}

	cctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	crit, err := runner.Critique(cctx, opts.Request, pngPath)
	if err != nil {
		return 0, "", fmt.Errorf("critique: %w", err)
	}
	return crit.Score, crit.Text, nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}
