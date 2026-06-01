package gen

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// drawableElements are the SVG tags that put marks on the canvas. We count
// these to reject lazy "one shape" builds, mirroring MineBench's minBlocks gate.
var drawableElements = map[string]bool{
	"path":     true,
	"rect":     true,
	"circle":   true,
	"ellipse":  true,
	"line":     true,
	"polyline": true,
	"polygon":  true,
	"text":     true,
	"use":      true,
}

// ExtractSVG pulls the SVG document out of a raw model response, tolerating
// markdown code fences and any stray commentary around it.
func ExtractSVG(raw string) (string, error) {
	s := strings.TrimSpace(raw)

	// Strip a leading ```svg / ```xml / ``` fence and its closing fence if present.
	if strings.HasPrefix(s, "```") {
		if nl := strings.IndexByte(s, '\n'); nl >= 0 {
			s = s[nl+1:]
		}
		if i := strings.LastIndex(s, "```"); i >= 0 {
			s = s[:i]
		}
		s = strings.TrimSpace(s)
	}

	lower := strings.ToLower(s)
	start := strings.Index(lower, "<svg")
	if start < 0 {
		return "", fmt.Errorf("no <svg> element found in the response")
	}
	end := strings.LastIndex(lower, "</svg>")
	if end < 0 {
		return "", fmt.Errorf("response has an opening <svg> but no closing </svg>")
	}
	end += len("</svg>")
	if end <= start {
		return "", fmt.Errorf("malformed SVG: closing tag precedes opening tag")
	}
	return strings.TrimSpace(s[start:end]), nil
}

// Validate checks that svg is well-formed XML rooted at <svg>, carries a
// viewBox (or width+height), declares no forbidden content, and meets the
// drawable-element floor. The returned error is phrased so it can be fed back
// to the model as a repair instruction.
func Validate(svg string, minElements int) error {
	dec := xml.NewDecoder(strings.NewReader(svg))

	var (
		seenRoot     bool
		rootHasSize  bool
		drawableSeen int
	)

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("not well-formed XML: %v", err)
		}

		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		name := strings.ToLower(se.Name.Local)

		if !seenRoot {
			if name != "svg" {
				return fmt.Errorf("root element is <%s>, expected <svg>", se.Name.Local)
			}
			seenRoot = true
			hasViewBox := false
			hasW, hasH := false, false
			for _, a := range se.Attr {
				switch strings.ToLower(a.Name.Local) {
				case "viewbox":
					if strings.TrimSpace(a.Value) != "" {
						hasViewBox = true
					}
				case "width":
					hasW = true
				case "height":
					hasH = true
				}
			}
			rootHasSize = hasViewBox || (hasW && hasH)
		}

		switch name {
		case "script":
			return fmt.Errorf("forbidden <script> element present; output static SVG only")
		case "image":
			return fmt.Errorf("forbidden <image> element present; draw shapes, do not embed raster images")
		}

		if drawableElements[name] {
			drawableSeen++
		}
	}

	if !seenRoot {
		return fmt.Errorf("no <svg> root element found")
	}
	if !rootHasSize {
		return fmt.Errorf("the <svg> root has no viewBox (nor width+height); add viewBox=\"0 0 W H\"")
	}
	if drawableSeen < minElements {
		return fmt.Errorf("only %d drawable elements; need at least %d for a recognizable, detailed illustration", drawableSeen, minElements)
	}
	return nil
}

// CountDrawable returns the number of drawable elements in a (validated) SVG.
func CountDrawable(svg string) int {
	dec := xml.NewDecoder(strings.NewReader(svg))
	n := 0
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok && drawableElements[strings.ToLower(se.Name.Local)] {
			n++
		}
	}
	return n
}

// CountAnimations returns the number of SMIL animation elements (<animate>,
// <animateTransform>, <animateMotion>, <animateColor>) in the SVG.
func CountAnimations(svg string) int {
	dec := xml.NewDecoder(strings.NewReader(svg))
	n := 0
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok && strings.HasPrefix(strings.ToLower(se.Name.Local), "animate") {
			n++
		}
	}
	return n
}
