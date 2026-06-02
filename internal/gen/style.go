package gen

import (
	"fmt"
	"sort"
	"strings"
)

// styles maps a --style preset name to a guidance block appended to the system
// prompt. Each block steers the visual treatment without changing the subject.
var styles = map[string]string{
	"flat": "Render in FLAT DESIGN: solid color fills and bold simple shapes, minimal or no gradients, clean geometric forms, crisp edges, a small harmonious palette. No photorealistic shading or texture.",

	"line-art": "Render as LINE ART: confident black (or single-color) ink outlines on a plain light background, little to no fill, expressive varying stroke weight, like a clean pen-and-ink drawing. Convey form through contour and a few hatching strokes, not solid color.",

	"realistic": "Render with PAINTERLY REALISM: rich layered gradients, soft shadows and highlights, careful modeling of a single consistent light source, believable depth, material and texture cues. Smooth, sculpted, lifelike surfaces.",

	"pixel": "Render as PIXEL ART: build everything from small uniform square blocks snapped to a coarse grid, hard edges, no smooth curves or gradients, a limited retro palette, visible dithering for shading. It should read like low-resolution game sprite art.",

	"isometric": "Render in ISOMETRIC projection: a consistent 2:1 isometric grid with ~30-degree axes, objects drawn as clean geometric volumes seen from a fixed 3/4 top-down angle, flat-shaded faces with a lighter top, mid side, and darker side for volume.",

	"watercolor": "Render as WATERCOLOR: soft translucent washes of layered transparent color, gentle blended gradients, organic irregular edges that bleed slightly, light paper-like texture, a delicate restrained palette.",

	"low-poly": "Render as LOW-POLY: compose forms from flat triangular and polygonal facets, each facet a slightly different flat shade to suggest volume, faceted geometric stylization, crisp straight edges between facets.",

	"retro": "Render as a RETRO / VINTAGE POSTER: a limited muted period palette, bold simple shapes, mid-century print aesthetic, subtle grain or halftone feel, strong graphic composition.",
}

// StyleNames returns the supported --style preset names, sorted.
func StyleNames() []string {
	names := make([]string, 0, len(styles))
	for k := range styles {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// StyleAppendix returns the system-prompt guidance for a style preset and
// whether the name is known. An empty name is valid and yields no appendix.
func StyleAppendix(style string) (string, bool) {
	s := strings.TrimSpace(strings.ToLower(style))
	if s == "" {
		return "", true
	}
	block, ok := styles[s]
	if !ok {
		return "", false
	}
	return "\n\n## Style\n" + block, true
}

// pixelFriendlyGuidance steers generation toward art that survives downsampling
// to a coarse pixel grid: the "design tokens" of real pixel art. A post-process
// filter alone can't rescue a richly-detailed painterly SVG at 64px — the
// source has to be built to read at that size (the real Dead Cells discipline).
const pixelFriendlyGuidance = `This illustration will be downsampled to a scene-level pixel-art grid (~240–320px on the longest side), so it MUST be designed to read at that size. Pixel art is a design discipline, not a filter:
- Build from BOLD, FLAT color regions. Avoid smooth gradients, soft blurs, and atmospheric haze — they turn to mush when downsampled. A few hard-edged bands of color read as shading far better than a continuous gradient.
- Give every object a STRONG, READABLE SILHOUETTE: recognizable from its outline alone. Favor clean, deliberate, generous forms.
- Use a LIMITED PALETTE — a curated set of distinct colors, not dozens of near-identical shades blended together.
- Keep CLEAR FIGURE-GROUND separation: distinct subjects that stand apart from the background. At this resolution moderate detail survives (faces, signage, props), but avoid hair-thin lines and sub-pixel filigree that would disappear.
- Prefer crisp straight or boldly-curved edges; outline key shapes so they pop.`

// PixelFriendlyAppendix returns the system-prompt block that co-designs the SVG
// for pixelization (see --pixelize). It is additive on top of any --style preset.
func PixelFriendlyAppendix() string {
	return "\n\n## Pixel-friendly design\n" + pixelFriendlyGuidance
}

// ValidateStyle returns an error if style is a non-empty unknown name.
func ValidateStyle(style string) error {
	if _, ok := StyleAppendix(style); !ok {
		return fmt.Errorf("unknown --style %q; valid styles: %s", style, strings.Join(StyleNames(), ", "))
	}
	return nil
}
