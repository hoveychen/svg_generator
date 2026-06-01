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
}

// Run executes `claude -p --system-prompt <system> --output-format text`,
// feeding the user prompt on stdin, and returns the model's text response.
//
// The command runs from a neutral temp directory so it does not auto-discover a
// project-level CLAUDE.md from the caller's working directory.
func (r Runner) Run(ctx context.Context, system, user string) (string, error) {
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

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = os.TempDir()
	cmd.Stdin = strings.NewReader(user)

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
