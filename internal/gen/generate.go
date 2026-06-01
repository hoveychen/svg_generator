package gen

import (
	"context"
	"fmt"
	"io"
	"time"
)

// Options controls a single SVG generation run.
type Options struct {
	Request      string // what to draw (required)
	Model        string // claude model alias; empty = claude default
	Canvas       int    // square viewBox size hinted to the model
	MinElements  int    // drawable-element floor
	Retries      int    // max repair attempts after the first try
	RefineRounds int    // vision-critique redraw rounds (0 = off)
	Timeout      time.Duration
	Verbose      bool
	// Log receives human-readable progress lines (e.g. os.Stderr). May be nil.
	Log io.Writer
}

// Result is a successful generation.
type Result struct {
	SVG          string
	Elements     int
	Attempts     int // attempts on the initial (pre-refine) generation
	Score        int // best critique score when refined; -1 otherwise
	RefineRounds int // refine rounds actually completed
}

func (o Options) logf(format string, args ...any) {
	if o.Log != nil {
		fmt.Fprintf(o.Log, format+"\n", args...)
	}
}

func (o *Options) applyDefaults() {
	if o.Canvas <= 0 {
		o.Canvas = 1024
	}
	if o.MinElements <= 0 {
		o.MinElements = 8
	}
	if o.Retries < 0 {
		o.Retries = 0
	}
	if o.RefineRounds < 0 {
		o.RefineRounds = 0
	}
	if o.Timeout <= 0 {
		o.Timeout = 3 * time.Minute
	}
}

// Generate produces a validated SVG for the request. When RefineRounds > 0 it
// then runs a render->vision-critique->redraw loop and returns the best version.
func Generate(ctx context.Context, opts Options) (*Result, error) {
	opts.applyDefaults()

	runner := Runner{Model: opts.Model, Verbose: opts.Verbose}

	opts.logf("[generate_svg] initial generation (model=%s)", modelLabel(opts.Model))
	svg, attempts, err := runWithRepair(ctx, runner, SystemPrompt(opts.Canvas, opts.MinElements), UserPrompt(opts.Request), opts)
	if err != nil {
		return nil, err
	}

	res := &Result{SVG: svg, Elements: CountDrawable(svg), Attempts: attempts, Score: -1}
	if opts.RefineRounds > 0 {
		return refine(ctx, opts, runner, res)
	}
	return res, nil
}

// runWithRepair runs generate -> extract -> validate, retrying with a repair
// prompt on failure, up to Retries+1 attempts. It is shared by the initial
// generation and each refine redraw.
func runWithRepair(ctx context.Context, runner Runner, system, initialUser string, opts Options) (string, int, error) {
	user := initialUser
	var lastErr error
	maxAttempts := opts.Retries + 1
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		callCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
		raw, err := runner.Run(callCtx, system, user)
		cancel()
		if err != nil {
			// A CLI/transport failure is not something a repair prompt fixes.
			return "", attempt, err
		}

		svg, err := ExtractSVG(raw)
		if err != nil {
			lastErr = err
			opts.logf("[generate_svg] attempt %d rejected: %v", attempt, err)
			user = RepairPrompt(opts.Request, truncate(raw, 4000), err.Error())
			continue
		}
		if err := Validate(svg, opts.MinElements); err != nil {
			lastErr = err
			opts.logf("[generate_svg] attempt %d rejected: %v", attempt, err)
			user = RepairPrompt(opts.Request, truncate(svg, 4000), err.Error())
			continue
		}
		return svg, attempt, nil
	}
	return "", maxAttempts, fmt.Errorf("gave up after %d attempts: %w", maxAttempts, lastErr)
}

func modelLabel(m string) string {
	if m == "" {
		return "claude default"
	}
	return m
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n... [truncated]"
}
