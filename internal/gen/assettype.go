package gen

import (
	"fmt"
	"sort"
	"strings"
)

// Pixel-art game assets are produced one sprite at a time at a resolution
// appropriate to the asset's kind, then assembled into scenes in a game engine.
// A pixelTypeSpec captures, per asset type, both the default logical resolution
// and the generation guidance that makes the SVG suit that kind of asset.
type pixelTypeSpec struct {
	resolution int    // default logical resolution on the longest side
	sprite     bool   // single-subject on a transparent background (vs full-bleed scene)
	guidance   string // generation guidance injected into the system prompt
}

// spriteCommon is shared by every single-subject asset type: the defining
// constraint that separates a game-asset sprite from a scene.
const spriteCommon = `This is a SINGLE GAME-ASSET SPRITE, not a scene — it will be cut out and composed with other sprites in a game engine. Therefore:
- Draw ONE subject only, centered and complete, filling most of the frame.
- NO BACKGROUND: do not draw a background rectangle, sky, ground, floor, scenery, vignette, or border. Everything around the subject must be empty so it reads as transparent. The very first shapes must NOT be a full-canvas fill.
- Give the subject a STRONG, READABLE SILHOUETTE, recognizable from its outline alone.
- Build from BOLD, FLAT color regions and a LIMITED PALETTE; avoid smooth gradients, soft blur, and atmospheric haze — they turn to mush when downsampled.
- Outline the subject so it pops against the empty background.`

// sceneGuidance is the full-bleed scene treatment (the pre-asset-type behavior).
const sceneGuidance = `This illustration will be downsampled to a scene-level pixel-art grid (~240–320px on the longest side), so it MUST be designed to read at that size. Pixel art is a design discipline, not a filter:
- Build from BOLD, FLAT color regions. Avoid smooth gradients, soft blurs, and atmospheric haze — they turn to mush when downsampled. A few hard-edged bands of color read as shading far better than a continuous gradient.
- Give every object a STRONG, READABLE SILHOUETTE: recognizable from its outline alone. Favor clean, deliberate, generous forms.
- Use a LIMITED PALETTE — a curated set of distinct colors, not dozens of near-identical shades blended together.
- Keep CLEAR FIGURE-GROUND separation: distinct subjects that stand apart from the background. At this resolution moderate detail survives (faces, signage, props), but avoid hair-thin lines and sub-pixel filigree that would disappear.
- Prefer crisp straight or boldly-curved edges; outline key shapes so they pop.`

var pixelTypes = map[string]pixelTypeSpec{
	"icon":      {resolution: 32, sprite: true, guidance: spriteCommon + "\n- This is a small ICON / pickup: keep the shape simple and instantly legible at a tiny size; one clear idea, no fine detail."},
	"item":      {resolution: 32, sprite: true, guidance: spriteCommon + "\n- This is an ITEM / prop: a single object rendered clean and iconic."},
	"character": {resolution: 64, sprite: true, guidance: spriteCommon + "\n- This is a CHARACTER: a clear pose with readable head, body, and limbs; expressive but chunky proportions that survive at sprite size."},
	"boss":      {resolution: 128, sprite: true, guidance: spriteCommon + "\n- This is a large BOSS / creature: imposing, with more internal detail allowed, but keep the overall silhouette bold and unmistakable."},
	"tile":      {resolution: 32, sprite: false, guidance: `This is a seamless TERRAIN TILE for a tilemap. It must TILE without visible seams: the pattern at the left edge must continue into the right edge, and the top into the bottom. Fill the whole canvas with the material (no centered subject, no empty margin). Use BOLD, FLAT color regions and a LIMITED PALETTE; keep texture chunky and evenly distributed, no single focal point.`},
	"scene":     {resolution: 240, sprite: false, guidance: sceneGuidance},
}

// PixelTypeNames returns the supported --pixel-type names, sorted.
func PixelTypeNames() []string {
	names := make([]string, 0, len(pixelTypes))
	for k := range pixelTypes {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// normalizePixelType lowercases/trims and maps empty to the default ("scene").
func normalizePixelType(name string) string {
	n := strings.TrimSpace(strings.ToLower(name))
	if n == "" {
		return "scene"
	}
	return n
}

// ValidatePixelType returns an error naming the valid options when name is not a
// recognized asset type. Empty is treated as the default ("scene") and accepted.
func ValidatePixelType(name string) error {
	if strings.TrimSpace(name) == "" {
		return nil
	}
	if _, ok := pixelTypes[normalizePixelType(name)]; !ok {
		return fmt.Errorf("unknown --pixel-type %q (choose one of: %s)", name, strings.Join(PixelTypeNames(), ", "))
	}
	return nil
}

// PixelTypeResolution returns the default logical resolution for an asset type.
func PixelTypeResolution(name string) int {
	return pixelTypes[normalizePixelType(name)].resolution
}

// PixelTypeIsSprite reports whether the asset type is a single-subject sprite on
// a transparent background (as opposed to a full-bleed scene or tile).
func PixelTypeIsSprite(name string) bool {
	return pixelTypes[normalizePixelType(name)].sprite
}

// PixelTypeAppendix returns the system-prompt block that co-designs the SVG for
// the given asset type (see --pixelize / --pixel-type). It is additive on top of
// any --style preset.
func PixelTypeAppendix(name string) string {
	return "\n\n## Pixel-art asset\n" + pixelTypes[normalizePixelType(name)].guidance
}
