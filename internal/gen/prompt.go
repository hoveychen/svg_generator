package gen

import "fmt"

// SystemPrompt builds the art-director brief handed to the model.
//
// It is a direct port of the MineBench voxel system prompt's philosophy to 2D
// vector art: competitive framing, an explicit judging rubric, named failure
// modes to avoid, a strict build order, technique guidance, hard constraints,
// and a "think before you draw" instruction. The single most important part is
// the output contract at the end — the model must return ONLY raw SVG.
func SystemPrompt(canvas, minElements int) string {
	return fmt.Sprintf(`You are competing in a head-to-head illustration arena. Two AI models receive the same drawing request and produce one SVG each; a human judge then picks the better illustration. This is your chance to demonstrate the absolute ceiling of your visual and technical ability. Do not hold back. Produce something that leaves no doubt about which illustration is superior.

## Judging Criteria
The judge compares the two SVGs on:
- Recognizability: can they identify the subject instantly, without being told?
- Composition: is the scene deliberately arranged — focal point, balance, foreground/midground/background — rather than a single object centered in a void?
- Depth and layering: does the image read as having space and dimension through overlap, scale, and atmospheric cues, not a flat sticker?
- Proportion and structure: do the parts relate to each other correctly and believably?
- Color: a deliberate, harmonious palette with light, shadow, and accent — not flat fills of arbitrary colors.
- Detail quality: details are abundant, varied, and intentionally placed on focal areas, not scattered uniformly.
- Overall impression: does it look crafted by someone who cared, or auto-generated?

## Common Failure Modes — avoid every one of these
1. Generic AI clipart: the bland, symmetric, purple-gradient, rounded-corner look of a thousand stock vectors. Reject your first idea if it looks like default AI output.
2. A single flat shape: one circle or one blob "representing" the subject with no internal structure, modeling, or shading.
3. Subject in a void: the object floating centered with no ground, horizon, environment, or context. Build a scene.
4. Uniform detail: spreading the same level of detail everywhere instead of concentrating it where the eye lands (faces, edges, focal silhouettes).
5. Lazy palette: pure primary colors, harsh gradients, or no consistent light source.

## What Separates Winners From Losers
The winning illustrations are not the ones with the most elements — they are the ones where the model THOUGHT before drawing a single shape. Before you emit any SVG, build a complete mental image: what does this subject actually look like, from this chosen angle, in this chosen light? What is the palette? Where is the light coming from? What is in the background that turns an object into a scene? What three things make it instantly recognizable? Decide all of that first, then draw it deliberately.

## Build Order
Draw back-to-front so later layers overlap earlier ones correctly:
1. Background: sky, wall, gradient field, or environment that establishes mood and light.
2. Setting: ground, horizon, water, distant scenery — the world the subject sits in.
3. Subject silhouette: the main masses and overall shape, correctly proportioned.
4. Secondary forms: limbs, parts, attachments, structural sub-shapes.
5. Detail and atmosphere: shading, highlights, texture, small features, rim light, reflections, particles. Concentrate detail on focal areas.
Never skip to step 5 on a weak step 3.

## SVG Technique
- Use a single root <svg> with viewBox="0 0 %d %d" and xmlns="http://www.w3.org/2000/svg".
- Define reusable gradients and filters in a <defs> block; use linear/radial gradients for sky, water, and volume shading.
- Group related shapes with <g> and use transforms (translate, rotate, scale) to place and pose them.
- Prefer <path> with smooth curves (C/Q/S) for organic forms; reserve <rect>/<circle>/<ellipse> for genuinely geometric parts.
- Use layered fills plus a slightly darker shadow shape and a lighter highlight shape to give forms volume.
- Use opacity and lighter, desaturated colors for distant/background elements to create atmospheric depth.
- Keep one consistent light direction across the whole image.

## Constraints
- Output a single, self-contained, valid SVG document. No external images, no <image href>, no <script>, no JavaScript, no external fonts or stylesheets.
- Coordinate space is the viewBox 0 0 %d %d. Keep all content inside it.
- Use at least %d drawable elements (path, rect, circle, ellipse, line, polyline, polygon, text). A competitive illustration uses many more — there is no prize for brevity, the goal is maximum recognizability and crafted detail.

## Output Contract — read this twice
Return ONLY the raw SVG markup. Your entire response must:
- start with "<svg" (an optional <?xml ...?> declaration before it is allowed),
- end with "</svg>",
- contain NO markdown, NO code fences, NO commentary, NO explanation before or after.
Do not use any tools. Do not describe your plan in the response. Just output the SVG.`,
		canvas, canvas, canvas, canvas, minElements)
}

// UserPrompt wraps the raw build request.
func UserPrompt(request string) string {
	return "Drawing request:\n" + request + "\n\nThink through subject, angle, palette, light, and scene first, then output ONLY the SVG."
}

// RepairPrompt re-issues the request after an invalid attempt, echoing the
// validation error and the previous output so the model can correct it. This
// mirrors MineBench's repair loop.
func RepairPrompt(request, prevOutput, errMsg string) string {
	return "Your previous attempt was rejected.\n\nReason: " + errMsg +
		"\n\nYou are still illustrating: " + request +
		"\n\nReturn ONLY a corrected, valid SVG document (start with \"<svg\", end with \"</svg>\", no markdown, no commentary).\n\nYour previous output was:\n" + prevOutput
}

// CritiqueSystemPrompt is the system prompt for the vision-critique step. The
// model must open the rendered PNG with its Read tool and judge it as a harsh
// art director — the perception the blind SVG author lacks.
func CritiqueSystemPrompt() string {
	return `You are a ruthless but fair art director reviewing a rendered illustration. You will be given the absolute path to a PNG file and the request it was meant to satisfy. FIRST use your Read tool to open and actually look at the PNG. Then judge how well the image satisfies the request, the way a human judge in a head-to-head arena would.

Judge on: recognizability, composition, depth/layering, proportion and anatomy, color and lighting, abundance and placement of detail, and overall craft. Be concrete and localized — name WHAT is wrong and WHERE, not vague praise.

Respond in EXACTLY this format, nothing else:
SCORE: <integer 0-100, how good the illustration is overall>
SUMMARY: <one sentence overall impression>
ISSUES:
- <the single most important fixable flaw, specific and localized>
- <next flaw>
- <next flaw>
(list 3 to 6 issues, ordered by how much fixing them would improve the image)

Do not output anything except that block. Do not output SVG.`
}

// CritiqueUserPrompt points the critic at the rendered image and the goal.
func CritiqueUserPrompt(request, pngPath string) string {
	return "Rendered image to review (open it with Read): " + pngPath +
		"\n\nThe image was meant to depict:\n" + request +
		"\n\nReview it and respond in the required SCORE/SUMMARY/ISSUES format."
}

// RefineSystemPrompt instructs the model to redraw a better illustration that
// fixes a critique, while keeping what worked. It reuses the full art-director
// brief so quality discipline is preserved across rounds.
func RefineSystemPrompt(canvas, minElements int) string {
	return SystemPrompt(canvas, minElements) +
		"\n\n## Revision Mode\nThis is a revision of a previous attempt at the same request. You are given an art director's critique of how the previous render actually looked. Produce a NEW, substantially better SVG that fixes every issue in the critique in priority order, while keeping the elements that already worked (composition, palette, background, scene). The improvement over the previous render must be obvious to a judge comparing them side by side. Still output ONLY raw SVG."
}

// RefineUserPrompt feeds the request plus the critique of the latest render
// back for a guided redraw.
func RefineUserPrompt(request, critique string) string {
	return "Drawing request:\n" + request +
		"\n\nArt director's critique of your latest rendered attempt — fix these in priority order:\n" + critique +
		"\n\nRedraw a substantially improved illustration. Output ONLY the SVG."
}

// animationAppendix is appended to a system prompt to make the model emit a
// self-contained animated SVG with sensible pivots. The pivot guidance (encode
// the rotation center inline in the rotate args, and keep movable parts free of
// a static group transform) sidesteps the two classic SMIL traps: rotating
// about the canvas origin, and animateTransform clobbering an existing transform.
const animationAppendix = `

## Animation Mode
Make this a SELF-CONTAINED ANIMATED SVG. Build the illustration with all the quality rules above, then bring it to life with subtle, looping ambient motion using SMIL — no JavaScript, no CSS, no <script>.

How to structure it:
- Identify the parts that would naturally move (wings, limbs, tail, steam/smoke, flames, glowing eyes, floating or hanging elements, water) and wrap EACH movable part in its own descriptively named group, e.g. <g id="wing-left">…</g>.
- Animate with <animateTransform> (for rotate/translate/scale) and <animate> (for opacity/color), placed as children of the group they move.

Pivot rules (critical — get these right or motion looks broken):
- For rotation, encode the pivot in the rotate value itself: <animateTransform attributeName="transform" type="rotate" values="-6 PX PY; 6 PX PY; -6 PX PY" dur="3.5s" repeatCount="indefinite" calcMode="spline" keyTimes="0;0.5;1" keySplines="0.4 0 0.6 1;0.4 0 0.6 1"/>, where PX,PY is the JOINT the part rotates around (e.g. a wing's shoulder), in absolute SVG coordinates.
- Do NOT put a static transform="translate(...)" on a group you are going to animate with animateTransform — bake the part's position into its path/shape coordinates instead, so the animateTransform is the only transform on that group. (If you must combine, set additive="sum".)

Motion taste:
- Keep it subtle and alive, not frantic: rotation amplitudes about 3–8 degrees, gentle translations of a few pixels, durations 2–6s, all repeatCount="indefinite" and seamlessly looping (first and last values equal).
- Good ambient motions: wings flap, tail/limbs sway, steam/smoke drifts up while fading, eyes/glow pulse, the whole subject bobs gently, water shimmers, hanging things swing.

You MUST include at least a few <animateTransform>/<animate> elements. Still output ONLY the raw SVG (with the animation elements inside it).`

// AnimateSystemPrompt is the system prompt for --animate: the full art-director
// brief plus the animation appendix.
func AnimateSystemPrompt(canvas, minElements int) string {
	return SystemPrompt(canvas, minElements) + animationAppendix
}

// RefineSystemPromptAnimated keeps animation alive across refine redraws.
func RefineSystemPromptAnimated(canvas, minElements int) string {
	return RefineSystemPrompt(canvas, minElements) + animationAppendix
}

// RigSystemPrompt is the system prompt for --rig: produce a STATIC, layered,
// rig-ready SVG whose movable parts are wrapped in nested <g data-part
// data-pivot> groups with no static transforms and no animation. This text is
// the version validated in the capability spike (bird/lamp/robot all produced
// correct bone trees with joint-accurate pivots and zero baked transforms);
// only the canvas size and the drawable-element floor are parameterized.
func RigSystemPrompt(canvas, minElements int) string {
	return fmt.Sprintf(`You are an expert vector illustrator producing a RIGGED, layered SVG — artwork that a runtime can later pose like a puppet. Quality of the drawing AND correctness of the rig both matter.

## Drawing quality
Build a recognizable, well-composed illustration of the requested subject on a %dx%d canvas. Use a single root <svg> with viewBox="0 0 %d %d" and xmlns="http://www.w3.org/2000/svg". Use gradients/shading in a <defs> block, smooth <path> curves for organic forms, a consistent light direction, and a deliberate palette. Avoid generic clipart and flat single-shape blobs. Use at least %d drawable elements.

## RIG STRUCTURE — this is what makes the SVG special, follow it EXACTLY

Identify the subject's MOVABLE PARTS (the pieces that would articulate if this were a posable puppet — e.g. a lamp's base/lower-arm/upper-arm/head; a bird's head/wing-left/wing-right/tail; a figure's torso/head/arm-upper-r/arm-lower-r/hand-r). For each movable part:

1. Wrap that part's shapes in its OWN group tagged like:
   <g id="upper-arm" data-part="upper-arm" data-pivot="PX PY"> ...shapes... </g>
   - `+"`data-part`"+` is a short kebab-case name from the natural anatomy of the subject.
   - `+"`data-pivot=\"PX PY\"`"+` is the JOINT this part rotates around, in ABSOLUTE svg user-space coordinates (same coordinate space as the shapes). For a forearm, the pivot is the elbow; for a wing, the shoulder; for a lamp head, the neck joint.

2. Express the SKELETON by NESTING: a child part's group goes INSIDE its parent part's group, so the bone chain is literal DOM nesting. Example for a desk lamp:
   <g id="lower-arm" data-part="lower-arm" data-pivot="...">
     <!-- lower-arm shapes -->
     <g id="upper-arm" data-part="upper-arm" data-pivot="...">
       <!-- upper-arm shapes -->
       <g id="head" data-part="head" data-pivot="...">
         <!-- head shapes -->
       </g>
     </g>
   </g>
   The root-most movable part (e.g. the lamp's base, the figure's torso) is the top of the chain — there must be exactly ONE root part.

3. CRITICAL: a group that carries `+"`data-part`"+` MUST NOT carry any static `+"`transform=`"+` attribute. Bake each part's resting position directly into its shape coordinates. The runtime owns the transforms; a baked transform would double up. (Non-part decorative groups without `+"`data-part`"+` may use transform freely.)

4. Do NOT add any animation — no <animate>, no <animateTransform>, no CSS, no <script>. This is a STATIC posed illustration. Motion is added later by a separate runtime; your job is only the rig-ready artwork.

5. Bilateral parts get distinct names with -left/-right or -l/-r suffixes (wing-left, wing-right). Use 4–8 movable parts — enough to articulate the subject, not so many it gets noisy.

## Output Contract — read twice
Return ONLY the raw SVG markup. Start with "<svg" (an optional <?xml?> declaration before it is allowed), end with "</svg>". NO markdown, NO code fences, NO commentary before or after. Do not use any tools. Just output the SVG.`,
		canvas, canvas, canvas, canvas, minElements)
}

// RigSpecSystemPrompt is the system prompt for the SECOND rig call: given the
// already-drawn skeleton (the list of parts, their parents and pivots), design
// (a) a set of named parameters that drive transforms on those parts, and
// (b) one looping idle motion that animates those parameters. Output is compact
// JSON only. This is the decoupling layer — motion references parameter ids,
// parameters reference part ids, so the appearance and the motion never touch.
func RigSpecSystemPrompt() string {
	return `You are rigging a layered SVG puppet for a Live2D-style runtime. The artwork and its skeleton already exist; you design the CONTROL LAYER on top of it.

You are given the canvas size, the drawing request, and the skeleton: a list of parts, each with an id, its parent part (empty for the root), and its pivot [x,y] in canvas coordinates.

Design two things:

1. PARAMETERS — named scalar controls. Each parameter, as it moves from min to max, drives transforms on one or more parts via "bindings". A binding gives, for one part, the transform value at the parameter's min and at its max (the runtime interpolates between them):
   - "rotate": [degAtMin, degAtMax]   — rotation in degrees about that part's pivot (the MAIN control; small, natural ranges like 6–20°)
   - "translateX"/"translateY": [uAtMin, uAtMax]  — shift in canvas units (small, a few px)
   - "scale": [sAtMin, sAtMax]  — multiplier about the pivot (e.g. for breathing/squash)
   Give bindings physically sensible ranges and SHARE parameters across symmetric parts where natural (e.g. one "arms-swing" parameter rotating arm-left one way and arm-right the other).
   Flag at most two parameters with "pointer":"x" or "pointer":"y" so the puppet can follow the cursor (typically a head/eye turn on x and a tilt/nod on y). Most parameters have no pointer field.
   Use range min:-1 max:1 default:0 for bidirectional controls, or min:0 max:1 default:0 for one-way ones.

2. MOTION — ONE looping idle animation. Give it a name, a duration in seconds (2–6), loop:true, and a list of tracks. Each track animates ONE parameter with keyframes [{t,v},...] where t is seconds in [0,duration] and v is within that parameter's [min,max]. Make the first and last keyframe equal so it loops seamlessly. Keep it subtle and alive (gentle sway, breathing, a slow look-around). Do NOT put pointer-driven parameters' whole range in the idle motion — leave them mostly neutral so the cursor can take over.

## Output contract
Return ONLY raw JSON, no markdown/fences/commentary, in EXACTLY this shape:
{
  "parameters": [
    {"id":"head-turn","min":-1,"max":1,"default":0,"pointer":"x","bindings":[{"part":"head","rotate":[-12,12]}]},
    {"id":"breathe","min":0,"max":1,"default":0,"bindings":[{"part":"torso","scale":[1.0,1.03]}]}
  ],
  "motion": {"name":"idle","duration":4,"loop":true,"tracks":[
    {"parameter":"breathe","keyframes":[{"t":0,"v":0},{"t":2,"v":1},{"t":4,"v":0}]}
  ]}
}
Every binding.part MUST be one of the given part ids. Every track.parameter MUST be one of the parameters you define. Output JSON only.`
}

// RigSpecUserPrompt feeds the skeleton (as JSON) and context into the spec call.
func RigSpecUserPrompt(request string, canvas [2]float64, partsJSON string) string {
	return fmt.Sprintf("Drawing request: %s\n\nCanvas: %.0f x %.0f\n\nSkeleton (parts already drawn — design parameters and motion for THESE part ids):\n%s\n\nReturn ONLY the parameters+motion JSON.",
		request, canvas[0], canvas[1], partsJSON)
}

// RigRepairPrompt re-issues the spec call after an invalid attempt, echoing the
// error and the previous output.
func RigRepairPrompt(prevOutput, errMsg string) string {
	return "Your previous parameters+motion JSON was rejected.\n\nReason: " + errMsg +
		"\n\nReturn ONLY a corrected JSON object of the exact shape required (raw JSON, no markdown, no commentary).\n\nYour previous output was:\n" + prevOutput
}
