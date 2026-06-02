package gen

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed player/player.js
var playerJS string

//go:embed player/harness.html
var harnessHTML string

// RigPath, MotionPath, HarnessPath derive the sibling artifact paths from the
// SVG output path: "hero.svg" -> "hero.rig.json" / "hero.motion.json" /
// "hero.html". A non-".svg" path just gets the suffix appended.
func RigPath(svgPath string) string     { return swapExt(svgPath, ".rig.json") }
func MotionPath(svgPath string) string  { return swapExt(svgPath, ".motion.json") }
func HarnessPath(svgPath string) string { return swapExt(svgPath, ".html") }

func swapExt(p, suffix string) string {
	ext := filepath.Ext(p)
	if strings.EqualFold(ext, ".svg") {
		return p[:len(p)-len(ext)] + suffix
	}
	return p + suffix
}

// WriteRig and WriteMotion serialize the JSON sidecar files (pretty-printed so
// they are human-readable and diff-friendly — they are meant to be edited and
// swapped).
func WriteRig(path string, rig Rig) error          { return writeJSON(path, rig) }
func WriteMotion(path string, motion Motion) error { return writeJSON(path, motion) }

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", filepath.Base(path), err)
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// WriteHarness writes a single self-contained HTML player next to the SVG. The
// SVG, rig, motion, and the player runtime are all inlined so the file opens
// directly from disk (file://) with no server and no fetch — a double-click
// shows the rig moving. The separate .rig.json / .motion.json still exist for
// swapping; the harness can load a different motion at runtime via file input.
func WriteHarness(path, title, svg string, rig Rig, motion Motion) error {
	rigJSON, err := json.Marshal(rig)
	if err != nil {
		return fmt.Errorf("marshal rig: %w", err)
	}
	motionJSON, err := json.Marshal(motion)
	if err != nil {
		return fmt.Errorf("marshal motion: %w", err)
	}

	// Inline-substitute the embedded template. Replace SVG/JS last and via a
	// func so a "$" or "__X__" inside the artwork can never be misread as a token.
	html := harnessHTML
	html = strings.ReplaceAll(html, "__TITLE__", htmlEscape(title))
	html = strings.ReplaceAll(html, "__RIG_JSON__", string(rigJSON))
	html = strings.ReplaceAll(html, "__MOTION_JSON__", string(motionJSON))
	html = replaceOnce(html, "__PLAYER_JS__", playerJS)
	html = replaceOnce(html, "__SVG__", inlineSVG(svg))

	return os.WriteFile(path, []byte(html), 0o644)
}

// replaceOnce substitutes the first occurrence only, using a literal replacer
// so regex-like metacharacters in the payload are irrelevant.
func replaceOnce(s, token, with string) string {
	i := strings.Index(s, token)
	if i < 0 {
		return s
	}
	return s[:i] + with + s[i+len(token):]
}

// inlineSVG strips any leading <?xml ...?> declaration so the SVG embeds
// cleanly inside the HTML body.
func inlineSVG(svg string) string {
	s := strings.TrimSpace(svg)
	if strings.HasPrefix(s, "<?xml") {
		if i := strings.Index(s, "?>"); i >= 0 {
			s = strings.TrimSpace(s[i+2:])
		}
	}
	return s
}

func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}
