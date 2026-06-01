package gen

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
)

// GIFPath derives the GIF path from an SVG output path:
// "boat.svg" -> "boat.gif".
func GIFPath(svgPath string) string {
	ext := filepath.Ext(svgPath)
	if ext != "" {
		return svgPath[:len(svgPath)-len(ext)] + ".gif"
	}
	return svgPath + ".gif"
}

// chromeBinary locates a Chrome/Chromium executable for time-stepped rendering.
// Override with the GENERATE_SVG_CHROME environment variable.
func chromeBinary() (string, error) {
	if env := os.Getenv("GENERATE_SVG_CHROME"); env != "" {
		if _, err := os.Stat(env); err == nil {
			return env, nil
		}
		return "", fmt.Errorf("GENERATE_SVG_CHROME=%q does not exist", env)
	}
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	if runtime.GOOS == "darwin" {
		for _, p := range []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		} {
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
	}
	return "", fmt.Errorf("no Chrome/Chromium found (set GENERATE_SVG_CHROME to its path)")
}

// RenderGIF rasterizes the (animated) SVG into an animated GIF by screenshotting
// it at `frames` evenly spaced points across `durationMs` of its animation
// timeline (via Chrome's virtual time), then assembling the frames with ffmpeg
// (preferred) or ImageMagick.
func RenderGIF(svgPath, gifPath string, size, frames, durationMs int) error {
	if frames < 2 {
		frames = 2
	}
	if durationMs <= 0 {
		durationMs = 3000
	}
	if size <= 0 {
		size = 512
	}

	chrome, err := chromeBinary()
	if err != nil {
		return err
	}

	absSVG, err := filepath.Abs(svgPath)
	if err != nil {
		return err
	}
	url := "file://" + absSVG

	tmpDir, err := os.MkdirTemp("", "generate_svg_gif_*")
	if err != nil {
		return fmt.Errorf("temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	framePaths := make([]string, 0, frames)
	for i := 0; i < frames; i++ {
		vt := i * durationMs / frames
		if vt < 1 {
			vt = 1
		}
		framePath := filepath.Join(tmpDir, fmt.Sprintf("f%04d.png", i))
		cmd := exec.Command(chrome,
			"--headless=new", "--disable-gpu", "--hide-scrollbars",
			fmt.Sprintf("--window-size=%d,%d", size, size),
			fmt.Sprintf("--virtual-time-budget=%d", vt),
			"--screenshot="+framePath,
			url,
		)
		if out, runErr := cmd.CombinedOutput(); runErr != nil {
			return fmt.Errorf("chrome frame %d failed: %v: %s", i, runErr, string(out))
		}
		if _, statErr := os.Stat(framePath); statErr != nil {
			return fmt.Errorf("chrome produced no frame %d", i)
		}
		framePaths = append(framePaths, framePath)
	}

	fps := float64(frames) * 1000.0 / float64(durationMs)
	return assembleGIF(tmpDir, framePaths, gifPath, fps)
}

// assembleGIF turns the captured frames into a looping GIF, preferring ffmpeg
// (palette-based, high quality) and falling back to ImageMagick.
func assembleGIF(tmpDir string, framePaths []string, gifPath string, fps float64) error {
	if bin, err := exec.LookPath("ffmpeg"); err == nil {
		pattern := filepath.Join(tmpDir, "f%04d.png")
		fpsArg := strconv.FormatFloat(fps, 'f', 4, 64)
		cmd := exec.Command(bin, "-y",
			"-framerate", fpsArg,
			"-i", pattern,
			"-vf", "split[s0][s1];[s0]palettegen=stats_mode=diff[p];[s1][p]paletteuse=dither=bayer:bayer_scale=3",
			"-loop", "0",
			gifPath,
		)
		if out, runErr := cmd.CombinedOutput(); runErr == nil {
			return nil
		} else {
			// fall through to ImageMagick, but remember the ffmpeg error
			if mErr := magickAssemble(framePaths, gifPath, fps); mErr == nil {
				return nil
			}
			return fmt.Errorf("ffmpeg failed: %v: %s", runErr, string(out))
		}
	}
	return magickAssemble(framePaths, gifPath, fps)
}

func magickAssemble(framePaths []string, gifPath string, fps float64) error {
	delay := int(100.0/fps + 0.5) // centiseconds per frame
	if delay < 1 {
		delay = 1
	}
	bin := ""
	for _, name := range []string{"magick", "convert"} {
		if p, err := exec.LookPath(name); err == nil {
			bin = p
			break
		}
	}
	if bin == "" {
		return fmt.Errorf("no GIF assembler found (install ffmpeg or ImageMagick)")
	}
	args := []string{"-delay", strconv.Itoa(delay), "-loop", "0"}
	args = append(args, framePaths...)
	args = append(args, "-layers", "Optimize", gifPath)
	cmd := exec.Command(bin, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("imagemagick failed: %v: %s", err, string(out))
	}
	return nil
}
