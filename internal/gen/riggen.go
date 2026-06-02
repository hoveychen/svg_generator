package gen

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// rigMaxOutputTokens raises claude's output ceiling for the rig SVG call. The
// capability spike showed detailed rig SVGs (lamp, robot) overflow the default
// 32000-token cap; 64000 cleared every spike subject.
const rigMaxOutputTokens = 64000

// rigMinTimeout floors the per-call timeout in rig mode. A detailed rig SVG at
// 64k output tokens routinely takes longer than the 3-minute default, so we
// give each rig call at least this long unless the caller asked for more.
const rigMinTimeout = 6 * time.Minute

// RigResult is a successful rig generation: the layered appearance SVG plus the
// extracted/designed rig and motion.
type RigResult struct {
	SVG          string
	Rig          Rig
	Motion       Motion
	Elements     int
	Parts        int
	SVGAttempts  int // attempts on the appearance-SVG call
	SpecAttempts int // attempts on the parameters+motion call
}

// GenerateRig produces a Live2D-style rig in two model calls:
//
//  1. an appearance call that draws a STATIC, layered SVG with nested
//     <g data-part data-pivot> groups (validated by ValidateRigSVG), and
//  2. a spec call that, given the extracted skeleton, designs the parameters
//     and one idle motion (validated by ValidateRigSpec).
//
// The skeleton (parts, parents, pivots) and canvas are derived mechanically
// from the SVG; only the control layer is asked of the second call. This keeps
// each call's output bounded and keeps motion decoupled from appearance.
func GenerateRig(ctx context.Context, opts Options) (*RigResult, error) {
	opts.applyDefaults()
	if opts.Timeout < rigMinTimeout {
		opts.Timeout = rigMinTimeout
	}
	runner := Runner{Model: opts.Model, Verbose: opts.Verbose, MaxOutputTokens: rigMaxOutputTokens}

	// --- call 1: the rig-ready appearance SVG ---
	system := RigSystemPrompt(opts.Canvas, opts.MinElements)
	system += opts.styleAppendix()
	opts.logf("[generate_svg] rig: drawing layered SVG (model=%s%s)", modelLabel(opts.Model), styleLabel(opts.Style))
	svg, svgAttempts, err := runRigSVGWithRepair(ctx, runner, system, UserPrompt(opts.Request), opts)
	if err != nil {
		return nil, err
	}

	parts, err := ExtractNamedParts(svg)
	if err != nil {
		return nil, fmt.Errorf("extracting rig parts: %w", err)
	}
	canvas := ExtractCanvas(svg)
	opts.logf("[generate_svg] rig: extracted %d movable part(s); designing parameters + motion", len(parts))

	// --- call 2: parameters + idle motion for the extracted skeleton ---
	params, motion, specAttempts, err := runRigSpecWithRepair(ctx, runner, parts, canvas, opts)
	if err != nil {
		return nil, err
	}

	return &RigResult{
		SVG:          svg,
		Rig:          Rig{Canvas: canvas, Parts: parts, Parameters: params},
		Motion:       motion,
		Elements:     CountDrawable(svg),
		Parts:        len(parts),
		SVGAttempts:  svgAttempts,
		SpecAttempts: specAttempts,
	}, nil
}

// runRigSVGWithRepair is runWithRepair specialized to the rig appearance SVG:
// extract -> ValidateRigSVG, retrying with a repair prompt on failure.
func runRigSVGWithRepair(ctx context.Context, runner Runner, system, initialUser string, opts Options) (string, int, error) {
	user := initialUser
	var lastErr error
	maxAttempts := opts.Retries + 1
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		callCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
		raw, err := runner.Run(callCtx, system, user)
		cancel()
		if err != nil {
			return "", attempt, err
		}
		svg, err := ExtractSVG(raw)
		if err == nil {
			err = ValidateRigSVG(svg, opts.MinElements)
		}
		if err != nil {
			lastErr = err
			opts.logf("[generate_svg] rig svg attempt %d rejected: %v", attempt, err)
			user = RepairPrompt(opts.Request, truncate(raw, 4000), err.Error())
			continue
		}
		return svg, attempt, nil
	}
	return "", maxAttempts, fmt.Errorf("rig SVG: gave up after %d attempts: %w", maxAttempts, lastErr)
}

// runRigSpecWithRepair runs the parameters+motion call, validating the result
// against the extracted skeleton and retrying with RigRepairPrompt on failure.
func runRigSpecWithRepair(ctx context.Context, runner Runner, parts []RigPart, canvas [2]float64, opts Options) ([]RigParameter, Motion, int, error) {
	partsJSON, _ := json.MarshalIndent(parts, "", "  ")
	system := RigSpecSystemPrompt()
	user := RigSpecUserPrompt(opts.Request, canvas, string(partsJSON))

	var lastErr error
	maxAttempts := opts.Retries + 1
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		callCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
		raw, err := runner.Run(callCtx, system, user)
		cancel()
		if err != nil {
			return nil, Motion{}, attempt, err
		}
		params, motion, perr := ParseRigSpec(raw)
		if perr == nil {
			perr = ValidateRigSpec(parts, params, motion)
		}
		if perr != nil {
			lastErr = perr
			opts.logf("[generate_svg] rig spec attempt %d rejected: %v", attempt, perr)
			user = RigRepairPrompt(truncate(raw, 4000), perr.Error())
			continue
		}
		return params, motion, attempt, nil
	}
	return nil, Motion{}, maxAttempts, fmt.Errorf("rig spec: gave up after %d attempts: %w", maxAttempts, lastErr)
}
