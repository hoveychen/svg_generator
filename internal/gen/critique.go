package gen

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Critique is a vision model's assessment of a rendered illustration.
type Critique struct {
	Score int    // 0-100 overall quality as judged from the render
	Text  string // SUMMARY + ISSUES block, fed back into the redraw
	Raw   string // full raw critic response
}

var scoreRe = regexp.MustCompile(`(?i)score\s*[:=]\s*(\d{1,3})`)

// Critique renders-aware review: the runner opens pngPath with its Read tool
// and judges it against request. Returns the parsed score and the critique text.
func (r Runner) Critique(ctx context.Context, request, pngPath string) (*Critique, error) {
	raw, err := r.RunVision(ctx, CritiqueSystemPrompt(), CritiqueUserPrompt(request, pngPath))
	if err != nil {
		return nil, err
	}

	c := &Critique{Score: parseScore(raw), Text: critiqueText(raw), Raw: raw}
	return c, nil
}

func parseScore(raw string) int {
	m := scoreRe.FindStringSubmatch(raw)
	if m == nil {
		return -1 // unknown; caller treats as "no usable score"
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n < 0 {
		return -1
	}
	if n > 100 {
		n = 100
	}
	return n
}

// critiqueText strips a leading SCORE: line so the redraw prompt gets the
// SUMMARY + ISSUES guidance without the bare number.
func critiqueText(raw string) string {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	kept := make([]string, 0, len(lines))
	for _, ln := range lines {
		if scoreRe.MatchString(ln) && strings.Contains(strings.ToUpper(ln), "SCORE") {
			continue
		}
		kept = append(kept, ln)
	}
	out := strings.TrimSpace(strings.Join(kept, "\n"))
	if out == "" {
		return strings.TrimSpace(raw)
	}
	return out
}

// ScoreLabel renders a score for logs, tolerating the unknown sentinel.
func ScoreLabel(score int) string {
	if score < 0 {
		return "n/a"
	}
	return fmt.Sprintf("%d", score)
}
