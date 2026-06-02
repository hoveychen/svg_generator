package gen

import (
	"os"
	"strings"
	"testing"
)

// A minimal but valid rig SVG: a 2-bone chain (torso -> head) with a decorative
// non-part group, joint pivots, baked coordinates, and no animation.
const goodRigSVG = `<svg viewBox="0 0 1000 800" xmlns="http://www.w3.org/2000/svg">
  <g id="bg"><rect x="0" y="0" width="1000" height="800" fill="#eee"/></g>
  <g data-part="torso" data-pivot="500 600">
    <rect x="440" y="400" width="120" height="220" fill="#888"/>
    <ellipse cx="500" cy="410" rx="70" ry="40" fill="#999"/>
    <g data-part="head" data-pivot="500 380">
      <circle cx="500" cy="320" r="80" fill="#ccc"/>
      <circle cx="475" cy="310" r="10" fill="#222"/>
      <circle cx="525" cy="310" r="10" fill="#222"/>
    </g>
  </g>
</svg>`

func TestExtractNamedParts_NestingAndPivots(t *testing.T) {
	parts, err := ExtractNamedParts(goodRigSVG)
	if err != nil {
		t.Fatalf("ExtractNamedParts: %v", err)
	}
	if len(parts) != 2 {
		t.Fatalf("got %d parts, want 2: %+v", len(parts), parts)
	}
	byID := map[string]RigPart{}
	for _, p := range parts {
		byID[p.ID] = p
	}
	if torso, ok := byID["torso"]; !ok || torso.Parent != "" {
		t.Errorf("torso should be the root (parent=\"\"): %+v", torso)
	}
	if head, ok := byID["head"]; !ok || head.Parent != "torso" {
		t.Errorf("head's parent should be torso (from nesting): %+v", head)
	}
	if got := byID["head"].Pivot; got != [2]float64{500, 380} {
		t.Errorf("head pivot = %v, want [500 380]", got)
	}
}

func TestValidateRigSVG_Good(t *testing.T) {
	// The 6-element fixture exercises the rig-structure checks; the drawable
	// floor itself is covered by the base Validate tests.
	if err := ValidateRigSVG(goodRigSVG, 4); err != nil {
		t.Fatalf("good rig SVG should validate: %v", err)
	}
}

func TestValidateRigSVG_Rejects(t *testing.T) {
	cases := []struct {
		name, svg, wantSubstr string
	}{
		{
			name:       "transform on part",
			svg:        `<svg viewBox="0 0 100 100"><g data-part="a" data-pivot="1 1" transform="rotate(5)"><rect x="0" y="0" width="9" height="9"/></g><g data-part="b" data-pivot="2 2"><rect x="0" y="0" width="9" height="9"/></g><circle cx="1" cy="1" r="1"/><circle cx="2" cy="2" r="1"/><circle cx="3" cy="3" r="1"/><circle cx="4" cy="4" r="1"/><circle cx="5" cy="5" r="1"/><circle cx="6" cy="6" r="1"/></svg>`,
			wantSubstr: "static transform",
		},
		{
			name:       "has animation",
			svg:        `<svg viewBox="0 0 100 100"><g data-part="a" data-pivot="1 1"><rect x="0" y="0" width="9" height="9"/><animateTransform attributeName="transform" type="rotate" values="0;5;0" dur="2s"/></g><g data-part="b" data-pivot="2 2"><rect x="0" y="0" width="9" height="9"/></g><circle cx="1" cy="1" r="1"/><circle cx="2" cy="2" r="1"/><circle cx="3" cy="3" r="1"/><circle cx="4" cy="4" r="1"/><circle cx="5" cy="5" r="1"/><circle cx="6" cy="6" r="1"/></svg>`,
			wantSubstr: "STATIC",
		},
		{
			name:       "too few parts",
			svg:        `<svg viewBox="0 0 100 100"><g data-part="only" data-pivot="1 1"><rect x="0" y="0" width="9" height="9"/></g><circle cx="1" cy="1" r="1"/><circle cx="2" cy="2" r="1"/><circle cx="3" cy="3" r="1"/><circle cx="4" cy="4" r="1"/><circle cx="5" cy="5" r="1"/><circle cx="6" cy="6" r="1"/><circle cx="7" cy="7" r="1"/></svg>`,
			wantSubstr: "movable part",
		},
		{
			name:       "two roots",
			svg:        `<svg viewBox="0 0 100 100"><g data-part="a" data-pivot="1 1"><rect x="0" y="0" width="9" height="9"/></g><g data-part="b" data-pivot="2 2"><rect x="0" y="0" width="9" height="9"/></g><circle cx="1" cy="1" r="1"/><circle cx="2" cy="2" r="1"/><circle cx="3" cy="3" r="1"/><circle cx="4" cy="4" r="1"/><circle cx="5" cy="5" r="1"/><circle cx="6" cy="6" r="1"/></svg>`,
			wantSubstr: "single bone tree",
		},
		{
			name:       "missing pivot",
			svg:        `<svg viewBox="0 0 100 100"><g data-part="a"><rect x="0" y="0" width="9" height="9"/></g><g data-part="b" data-pivot="2 2"><rect x="0" y="0" width="9" height="9"/></g><circle cx="1" cy="1" r="1"/><circle cx="2" cy="2" r="1"/><circle cx="3" cy="3" r="1"/><circle cx="4" cy="4" r="1"/><circle cx="5" cy="5" r="1"/><circle cx="6" cy="6" r="1"/></svg>`,
			wantSubstr: "data-pivot",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateRigSVG(tc.svg, 8)
			if err == nil {
				t.Fatalf("expected rejection, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("error %q does not mention %q", err.Error(), tc.wantSubstr)
			}
		})
	}
}

func TestExtractCanvas(t *testing.T) {
	if got := ExtractCanvas(goodRigSVG); got != [2]float64{1000, 800} {
		t.Errorf("canvas = %v, want [1000 800]", got)
	}
	if got := ExtractCanvas(`<svg width="640" height="480"></svg>`); got != [2]float64{640, 480} {
		t.Errorf("width/height canvas = %v, want [640 480]", got)
	}
	if got := ExtractCanvas(`<svg></svg>`); got != [2]float64{1024, 1024} {
		t.Errorf("fallback canvas = %v, want [1024 1024]", got)
	}
}

const goodSpecJSON = `{
  "parameters": [
    {"id":"head-turn","min":-1,"max":1,"default":0,"pointer":"x","bindings":[{"part":"head","rotate":[-12,12]}]},
    {"id":"breathe","min":0,"max":1,"default":0,"bindings":[{"part":"torso","scale":[1.0,1.03]}]}
  ],
  "motion": {"name":"idle","duration":4,"loop":true,"tracks":[
    {"parameter":"breathe","keyframes":[{"t":0,"v":0},{"t":2,"v":1},{"t":4,"v":0}]}
  ]}
}`

func mustParts(t *testing.T) []RigPart {
	t.Helper()
	parts, err := ExtractNamedParts(goodRigSVG)
	if err != nil {
		t.Fatalf("ExtractNamedParts: %v", err)
	}
	return parts
}

func TestParseAndValidateRigSpec_Good(t *testing.T) {
	params, motion, err := ParseRigSpec(goodSpecJSON)
	if err != nil {
		t.Fatalf("ParseRigSpec: %v", err)
	}
	if len(params) != 2 {
		t.Fatalf("got %d parameters, want 2", len(params))
	}
	if params[0].Pointer != "x" {
		t.Errorf("head-turn pointer = %q, want x", params[0].Pointer)
	}
	if params[0].Bindings[0].Rotate == nil {
		t.Errorf("head-turn binding should have a rotate range")
	}
	if err := ValidateRigSpec(mustParts(t), params, motion); err != nil {
		t.Fatalf("good spec should validate: %v", err)
	}
}

func TestParseRigSpec_StripsFences(t *testing.T) {
	fenced := "```json\n" + goodSpecJSON + "\n```"
	params, _, err := ParseRigSpec(fenced)
	if err != nil {
		t.Fatalf("ParseRigSpec with fences: %v", err)
	}
	if len(params) != 2 {
		t.Errorf("got %d parameters from fenced JSON, want 2", len(params))
	}
}

func TestValidateRigSpec_Rejects(t *testing.T) {
	parts := mustParts(t)
	rng := func(a, b float64) *[2]float64 { return &[2]float64{a, b} }

	cases := []struct {
		name       string
		params     []RigParameter
		motion     Motion
		wantSubstr string
	}{
		{
			name:       "binding to unknown part",
			params:     []RigParameter{{ID: "p", Min: -1, Max: 1, Bindings: []RigBinding{{Part: "ghost", Rotate: rng(-5, 5)}}}},
			motion:     Motion{Duration: 2, Tracks: []MotionTrack{{Parameter: "p", Keyframes: []MotionKeyframe{{0, 0}, {2, 0}}}}},
			wantSubstr: "does not exist",
		},
		{
			name:       "track to unknown parameter",
			params:     []RigParameter{{ID: "p", Min: -1, Max: 1, Bindings: []RigBinding{{Part: "head", Rotate: rng(-5, 5)}}}},
			motion:     Motion{Duration: 2, Tracks: []MotionTrack{{Parameter: "nope", Keyframes: []MotionKeyframe{{0, 0}, {2, 0}}}}},
			wantSubstr: "not defined",
		},
		{
			name:       "degenerate range",
			params:     []RigParameter{{ID: "p", Min: 1, Max: 1, Bindings: []RigBinding{{Part: "head", Rotate: rng(-5, 5)}}}},
			motion:     Motion{Duration: 2, Tracks: []MotionTrack{{Parameter: "p", Keyframes: []MotionKeyframe{{0, 0}, {2, 0}}}}},
			wantSubstr: "real range",
		},
		{
			name:       "binding with no transform",
			params:     []RigParameter{{ID: "p", Min: -1, Max: 1, Bindings: []RigBinding{{Part: "head"}}}},
			motion:     Motion{Duration: 2, Tracks: []MotionTrack{{Parameter: "p", Keyframes: []MotionKeyframe{{0, 0}, {2, 0}}}}},
			wantSubstr: "no transform",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateRigSpec(parts, tc.params, tc.motion)
			if err == nil {
				t.Fatalf("expected rejection, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("error %q does not mention %q", err.Error(), tc.wantSubstr)
			}
		})
	}
}

func TestArtifactPaths(t *testing.T) {
	if got := RigPath("hero.svg"); got != "hero.rig.json" {
		t.Errorf("RigPath = %q, want hero.rig.json", got)
	}
	if got := MotionPath("a/b/hero.svg"); got != "a/b/hero.motion.json" {
		t.Errorf("MotionPath = %q", got)
	}
	if got := HarnessPath("hero.svg"); got != "hero.html" {
		t.Errorf("HarnessPath = %q, want hero.html", got)
	}
	if got := RigPath("noext"); got != "noext.rig.json" {
		t.Errorf("RigPath(noext) = %q", got)
	}
}

func TestWriteHarness_Inlines(t *testing.T) {
	parts := mustParts(t)
	params, motion, err := ParseRigSpec(goodSpecJSON)
	if err != nil {
		t.Fatal(err)
	}
	rig := Rig{Canvas: ExtractCanvas(goodRigSVG), Parts: parts, Parameters: params}

	dir := t.TempDir()
	path := dir + "/hero.html"
	if err := WriteHarness(path, "Hero", goodRigSVG, rig, motion); err != nil {
		t.Fatalf("WriteHarness: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read harness: %v", err)
	}
	html := string(raw)

	for _, want := range []string{
		"RigPlayer",        // player.js was inlined
		`data-part="head"`, // the SVG was inlined
		`"head-turn"`,      // the rig JSON was inlined
		`"name":"idle"`,    // the motion JSON was inlined
		"Hero",             // the title
	} {
		if !strings.Contains(html, want) {
			t.Errorf("harness HTML missing %q", want)
		}
	}
	// No unsubstituted tokens should survive.
	for _, tok := range []string{"__SVG__", "__RIG_JSON__", "__MOTION_JSON__", "__PLAYER_JS__", "__TITLE__"} {
		if strings.Contains(html, tok) {
			t.Errorf("harness HTML still contains unsubstituted token %q", tok)
		}
	}
}
