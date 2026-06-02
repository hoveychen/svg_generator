package gen

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Runner invokes the `claude` CLI in non-interactive print mode.
//
// We shell out rather than calling an SDK so the tool has zero API-key handling
// and reuses whatever authentication the user's `claude` command already has.
type Runner struct {
	// Model alias forwarded to `claude --model` (e.g. "opus", "sonnet").
	// Empty means: let claude use its configured default.
	Model string
	// Verbose mirrors the claude stderr stream to our own stderr.
	Verbose bool
	// MaxOutputTokens, when > 0, raises claude's output token ceiling via the
	// CLAUDE_CODE_MAX_OUTPUT_TOKENS env var. Rig-mode SVGs are large enough to
	// blow past the default 32000 cap, so --rig sets this to ~64000.
	MaxOutputTokens int
}

// Run executes `claude -p --system-prompt <system> --output-format text`,
// feeding the user prompt on stdin, and returns the model's text response.
func (r Runner) Run(ctx context.Context, system, user string) (string, error) {
	return r.run(ctx, system, user, nil)
}

// RunVision is like Run but allowlists the Read tool so the model can open and
// look at local image files referenced (by absolute path) in the prompt. This
// is what lets the critique step actually see the rendered PNG.
func (r Runner) RunVision(ctx context.Context, system, user string) (string, error) {
	return r.run(ctx, system, user, []string{"--allowedTools", "Read"})
}

// run is the shared invocation. extraArgs are appended after the base args.
//
// The command runs from a neutral temp directory so it does not auto-discover a
// project-level CLAUDE.md from the caller's working directory. A custom
// --system-prompt is always passed, which also prevents the user's default
// system prompt / interaction modes from leaking into the output.
func (r Runner) run(ctx context.Context, system, user string, extraArgs []string) (string, error) {
	bin, err := exec.LookPath("claude")
	if err != nil {
		return "", fmt.Errorf("the `claude` CLI was not found on PATH: %w", err)
	}

	args := []string{
		"-p",
		"--output-format", "text",
		"--system-prompt", system,
	}
	if strings.TrimSpace(r.Model) != "" {
		args = append(args, "--model", r.Model)
	}
	args = append(args, extraArgs...)

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = os.TempDir()
	cmd.Stdin = strings.NewReader(user)
	if r.MaxOutputTokens > 0 {
		cmd.Env = append(os.Environ(), fmt.Sprintf("CLAUDE_CODE_MAX_OUTPUT_TOKENS=%d", r.MaxOutputTokens))
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	if r.Verbose {
		cmd.Stderr = os.Stderr
	} else {
		cmd.Stderr = &stderr
	}

	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("claude timed out: %w", ctx.Err())
		}
		if detail != "" {
			return "", fmt.Errorf("claude failed: %v: %s", err, detail)
		}
		return "", fmt.Errorf("claude failed: %w", err)
	}

	out := strings.TrimSpace(stdout.String())
	if out == "" {
		return "", fmt.Errorf("claude returned an empty response")
	}
	return out, nil
}
