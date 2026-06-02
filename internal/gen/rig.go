package gen

import (
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// The rig format is a Live2D-style decoupling of a drawing into three pieces:
//
//  1. an APPEARANCE SVG — a normal illustration whose movable parts are each
//     wrapped in a <g data-part="name" data-pivot="PX PY"> group, nested so the
//     DOM hierarchy literally is the bone chain. Part groups carry NO static
//     transform (the runtime owns transforms) and NO animation.
//  2. a RIG (rig.json) — the skeleton extracted from the SVG (parts, their
//     parents, their pivots) plus a set of named PARAMETERS. A parameter is a
//     scalar that, as it moves across its range, drives transforms on one or
//     more parts via bindings. Parameters are the decoupling seam: motion talks
//     to parameters, never directly to parts.
//  3. a MOTION (motion.json) — a timeline of keyframed tracks, each track
//     animating one parameter over time. Because a motion only references
//     parameter ids, the same rig can be driven by any number of swappable
//     motions, and the same motion can (in principle) drive any rig that
//     exposes the same parameter names.
//
// A small JS player reads all three at runtime and poses the parts; pointer
// input can additionally drive any parameter flagged with a Pointer axis.

// RigPart is one movable bone: a named <g data-part> group, its parent bone
// (empty for the root), and the joint it rotates around in absolute SVG
// user-space coordinates.
type RigPart struct {
	ID     string     `json:"id"`
	Parent string     `json:"parent,omitempty"`
	Pivot  [2]float64 `json:"pivot"`
}

// RigBinding maps a parameter's normalized position to a transform on one part.
// Each transform field is a [atMin, atMax] pair giving the value when the
// parameter sits at its Min and Max respectively; the runtime lerps between
// them. Rotate is in degrees, TranslateX/Y in SVG user units, Scale is a
// multiplier. Nil fields are not driven by this binding.
type RigBinding struct {
	Part       string      `json:"part"`
	Rotate     *[2]float64 `json:"rotate,omitempty"`
	TranslateX *[2]float64 `json:"translateX,omitempty"`
	TranslateY *[2]float64 `json:"translateY,omitempty"`
	Scale      *[2]float64 `json:"scale,omitempty"`
}

// RigParameter is a named scalar driver. Pointer optionally ties the parameter
// to a pointer axis ("x" or "y", normalized to [-1,1] over the canvas) so the
// rig can follow the cursor.
type RigParameter struct {
	ID       string       `json:"id"`
	Min      float64      `json:"min"`
	Max      float64      `json:"max"`
	Default  float64      `json:"default"`
	Pointer  string       `json:"pointer,omitempty"`
	Bindings []RigBinding `json:"bindings"`
}

// Rig is the skeleton + parameters: the contents of rig.json.
type Rig struct {
	Canvas     [2]float64     `json:"canvas"`
	Parts      []RigPart      `json:"parts"`
	Parameters []RigParameter `json:"parameters"`
}

// MotionKeyframe is a (time, value) point on a track. T is in seconds.
type MotionKeyframe struct {
	T float64 `json:"t"`
	V float64 `json:"v"`
}

// MotionTrack keyframes a single parameter over the motion timeline.
type MotionTrack struct {
	Parameter string           `json:"parameter"`
	Keyframes []MotionKeyframe `json:"keyframes"`
}

// Motion is one swappable animation clip: the contents of motion.json.
type Motion struct {
	Name     string        `json:"name"`
	Duration float64       `json:"duration"`
	Loop     bool          `json:"loop"`
	Tracks   []MotionTrack `json:"tracks"`
}

// minRigParts is the floor for a meaningful articulated rig: fewer than two
// movable parts is not a puppet.
const minRigParts = 2

// ExtractNamedParts walks the SVG and returns its movable parts in document
// order, deriving each part's parent from <g> nesting and its pivot from the
// data-pivot attribute. It does not validate; ValidateRigSVG does that.
func ExtractNamedParts(svg string) ([]RigPart, error) {
	dec := xml.NewDecoder(strings.NewReader(svg))

	var parts []RigPart
	// gStack tracks every open <g> by its part id ("" for groups without
	// data-part). A part's parent is the nearest ancestor <g> that has an id.
	var gStack []string

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("not well-formed XML: %v", err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if strings.ToLower(t.Name.Local) != "g" {
				continue
			}
			part := attr(t, "data-part")
			if part == "" {
				gStack = append(gStack, "")
				continue
			}
			pivot, perr := parsePivot(attr(t, "data-pivot"))
			if perr != nil {
				return nil, fmt.Errorf("part %q: %v", part, perr)
			}
			parent := nearestParent(gStack)
			parts = append(parts, RigPart{ID: part, Parent: parent, Pivot: pivot})
			gStack = append(gStack, part)
		case xml.EndElement:
			if strings.ToLower(t.Name.Local) == "g" && len(gStack) > 0 {
				gStack = gStack[:len(gStack)-1]
			}
		}
	}
	return parts, nil
}

// ValidateRigSVG enforces the appearance-SVG contract on top of the base SVG
// rules: it must be a valid drawing, expose at least minRigParts movable parts,
// every part must have a usable pivot and a unique kebab-ish id, no part group
// may carry a static transform (the runtime owns transforms), the parts must
// form a single connected tree, and there must be no animation (motion is added
// later by the player). Errors are phrased for feedback to the model.
func ValidateRigSVG(svg string, minElements int) error {
	if err := Validate(svg, minElements); err != nil {
		return err
	}
	if CountAnimations(svg) > 0 {
		return fmt.Errorf("the rig SVG must be a STATIC posed illustration: remove all <animate>/<animateTransform> elements (motion is added later by the runtime)")
	}
	if err := checkNoTransformOnParts(svg); err != nil {
		return err
	}

	parts, err := ExtractNamedParts(svg)
	if err != nil {
		return err
	}
	if len(parts) < minRigParts {
		return fmt.Errorf("only %d movable part(s) tagged; wrap each articulating piece in its own <g data-part=\"name\" data-pivot=\"PX PY\"> (need at least %d)", len(parts), minRigParts)
	}

	ids := make(map[string]bool, len(parts))
	roots := 0
	for _, p := range parts {
		if p.ID == "" {
			return fmt.Errorf("a <g data-part> group has an empty name")
		}
		if ids[p.ID] {
			return fmt.Errorf("duplicate part name %q; every data-part must be unique", p.ID)
		}
		ids[p.ID] = true
	}
	for _, p := range parts {
		if p.Parent == "" {
			roots++
			continue
		}
		if !ids[p.Parent] {
			return fmt.Errorf("part %q lists parent %q which is not itself a data-part group", p.ID, p.Parent)
		}
	}
	if roots != 1 {
		return fmt.Errorf("the rig must form a single bone tree with exactly one root part; found %d root parts (nest child parts inside their parent's <g>)", roots)
	}
	return nil
}

// checkNoTransformOnParts fails if any <g data-part=...> also carries a static
// transform attribute, which would double up with the runtime's transform.
func checkNoTransformOnParts(svg string) error {
	dec := xml.NewDecoder(strings.NewReader(svg))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("not well-formed XML: %v", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok || strings.ToLower(se.Name.Local) != "g" {
			continue
		}
		if attr(se, "data-part") != "" && strings.TrimSpace(attr(se, "transform")) != "" {
			return fmt.Errorf("part %q carries a static transform=; bake its resting position into the shape coordinates and remove the group transform (the runtime owns transforms)", attr(se, "data-part"))
		}
	}
	return nil
}

// attr returns the value of the named attribute (case-insensitive local name)
// on a start element, or "".
func attr(se xml.StartElement, name string) string {
	for _, a := range se.Attr {
		if strings.EqualFold(a.Name.Local, name) {
			return a.Value
		}
	}
	return ""
}

// nearestParent returns the nearest non-empty id on the open-group stack.
func nearestParent(stack []string) string {
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i] != "" {
			return stack[i]
		}
	}
	return ""
}

// parsePivot parses "PX PY" (space- or comma-separated) into absolute coords.
func parsePivot(s string) ([2]float64, error) {
	var p [2]float64
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == ',' || r == '\t' })
	if len(fields) != 2 {
		return p, fmt.Errorf("data-pivot=%q must be two numbers \"PX PY\" (the joint this part rotates around)", s)
	}
	for i, f := range fields {
		v, err := strconv.ParseFloat(strings.TrimSpace(f), 64)
		if err != nil {
			return p, fmt.Errorf("data-pivot has non-numeric coordinate %q", f)
		}
		p[i] = v
	}
	return p, nil
}
