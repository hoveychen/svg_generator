package gen

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// PNGPath derives the preview PNG path from an SVG output path:
// "boat.svg" -> "boat.png", "boat" -> "boat.png".
func PNGPath(svgPath string) string {
	ext := filepath.Ext(svgPath)
	if strings.EqualFold(ext, ".svg") {
		return svgPath[:len(svgPath)-len(ext)] + ".png"
	}
	return svgPath + ".png"
}

// RenderPNG rasterizes svgPath to pngPath at the given square pixel size,
// trying each available renderer in order of fidelity. It returns an error
// listing what was attempted when no renderer is available or all fail.
func RenderPNG(svgPath, pngPath string, size int) error {
	if size <= 0 {
		size = 1024
	}

	var tried []string

	if bin, err := exec.LookPath("rsvg-convert"); err == nil {
		tried = append(tried, "rsvg-convert")
		szArg := fmt.Sprintf("%d", size)
		cmd := exec.Command(bin, "-w", szArg, "-h", szArg, "-a", "-o", pngPath, svgPath)
		if out, runErr := cmd.CombinedOutput(); runErr == nil {
			return nil
		} else {
			recordFail(&tried, "rsvg-convert", runErr, out)
		}
	}

	if bin, err := exec.LookPath("qlmanage"); err == nil {
		tried = append(tried, "qlmanage")
		if err := renderWithQLManage(bin, svgPath, pngPath, size); err == nil {
			return nil
		} else {
			tried[len(tried)-1] = "qlmanage(" + err.Error() + ")"
		}
	}

	if len(tried) == 0 {
		return fmt.Errorf("no SVG renderer found (install rsvg-convert or use macOS qlmanage)")
	}
	return fmt.Errorf("all renderers failed: %s", strings.Join(tried, "; "))
}

// renderWithQLManage uses macOS Quick Look, which always writes
// "<basename>.png" into the output directory; we render into a temp dir and
// move the result to the requested path.
func renderWithQLManage(bin, svgPath, pngPath string, size int) error {
	tmpDir, err := os.MkdirTemp("", "generate_svg_png_*")
	if err != nil {
		return fmt.Errorf("temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	cmd := exec.Command(bin, "-t", "-s", fmt.Sprintf("%d", size), "-o", tmpDir, svgPath)
	if out, runErr := cmd.CombinedOutput(); runErr != nil {
		return fmt.Errorf("%v: %s", runErr, strings.TrimSpace(string(out)))
	}

	produced := filepath.Join(tmpDir, filepath.Base(svgPath)+".png")
	data, err := os.ReadFile(produced)
	if err != nil {
		return fmt.Errorf("qlmanage produced no thumbnail")
	}
	if err := os.WriteFile(pngPath, data, 0o644); err != nil {
		return fmt.Errorf("write png: %w", err)
	}
	return nil
}

func recordFail(tried *[]string, name string, err error, out []byte) {
	detail := strings.TrimSpace(string(out))
	if detail == "" {
		detail = err.Error()
	}
	(*tried)[len(*tried)-1] = name + "(" + detail + ")"
}
