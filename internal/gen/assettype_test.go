package gen

import (
	"strings"
	"testing"
)

func TestValidatePixelType(t *testing.T) {
	for _, ok := range []string{"", "icon", "item", "character", "boss", "tile", "scene", "CHARACTER", " scene "} {
		if err := ValidatePixelType(ok); err != nil {
			t.Errorf("ValidatePixelType(%q) = %v, want nil", ok, err)
		}
	}
	if err := ValidatePixelType("monster"); err == nil {
		t.Error("ValidatePixelType(\"monster\") = nil, want error")
	}
}

func TestPixelTypeResolution(t *testing.T) {
	cases := map[string]int{
		"icon":      32,
		"item":      32,
		"character": 64,
		"boss":      128,
		"tile":      32,
		"scene":     240,
		"":          240, // empty defaults to scene
	}
	for name, want := range cases {
		if got := PixelTypeResolution(name); got != want {
			t.Errorf("PixelTypeResolution(%q) = %d, want %d", name, got, want)
		}
	}
}

func TestPixelTypeIsSprite(t *testing.T) {
	sprites := []string{"icon", "item", "character", "boss"}
	for _, s := range sprites {
		if !PixelTypeIsSprite(s) {
			t.Errorf("PixelTypeIsSprite(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"tile", "scene", ""} {
		if PixelTypeIsSprite(s) {
			t.Errorf("PixelTypeIsSprite(%q) = true, want false", s)
		}
	}
}

// Sprite appendices must demand a transparent/empty background; scene must not.
func TestPixelTypeAppendixContent(t *testing.T) {
	char := PixelTypeAppendix("character")
	if !strings.Contains(char, "transparent") && !strings.Contains(char, "EMPTY") && !strings.Contains(char, "empty") {
		t.Errorf("character appendix should require an empty/transparent background; got:\n%s", char)
	}
	if !strings.Contains(char, "OVERRIDE") {
		t.Errorf("sprite appendix should explicitly OVERRIDE the scene mandate; got:\n%s", char)
	}
	scene := PixelTypeAppendix("scene")
	if strings.Contains(scene, "transparent") {
		t.Errorf("scene appendix should NOT ask for transparency; got:\n%s", scene)
	}
	if !strings.Contains(PixelTypeAppendix("tile"), "seamless") && !strings.Contains(PixelTypeAppendix("tile"), "TILE") {
		t.Errorf("tile appendix should ask for a seamless tile")
	}
}
