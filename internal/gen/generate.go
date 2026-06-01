package gen

import (
	"context"
	"fmt"
	"io"
	"time"
)

// Options controls a single SVG generation run.
type Options struct {
	Request     string // what to draw (required)
	Model       string // claude model alias; empty = claude default
	Canvas      int    // square viewBox size hinted to the model
	MinElements int    // drawable-element floor
	Retries     int    // max repair attempts after the first try
	Timeout     time.Duration
	Verbose     bool
	// Log receives human-readable progress lines (e.g. os.Stderr). May be nil.
	Log io.Writer
}

// Result is a successful generation.
type Result struct {
	SVG      string
	Elements int
	Attempts int
}

func (o Options) logf(format string, args ...any) {
	if o.Log != nil {
		fmt.Fprintf(o.Log, format+"\n", args...)
	}
}

// Generate produces a validated SVG for the request, retrying with repair
// prompts when the model returns invalid output.
func Generate(ctx context.Context, opts Options) (*Result, error) {
	if opts.Canvas <= 0 {
		opts.Canvas = 1024
	}
	if opts.MinElements <= 0 {
		opts.MinElements = 8
	}
	if opts.Retries < 0 {
		opts.Retries = 0
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 3 * time.Minute
	}

	runner := Runner{Model: opts.Model, Verbose: opts.Verbose}
	system := SystemPrompt(opts.Canvas, opts.MinElements)
	user := UserPrompt(opts.Request)

	var lastErr error
	maxAttempts := opts.Retries + 1
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		opts.logf("[generate_svg] attempt %d/%d (model=%s)", attempt, maxAttempts, modelLabel(opts.Model))

		callCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
		raw, err := runner.Run(callCtx, system, user)
		cancel()
		if err != nil {
			// A CLI/transport failure is not something a repair prompt fixes;
			// surface it immediately.
			return nil, err
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

		return &Result{SVG: svg, Elements: CountDrawable(svg), Attempts: attempt}, nil
	}

	return nil, fmt.Errorf("gave up after %d attempts: %w", maxAttempts, lastErr)
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
