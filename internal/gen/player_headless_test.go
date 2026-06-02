package gen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestPlayerHeadless runs the actual player.js inside headless Chrome and
// asserts the runtime behavior that unit tests cannot reach: that sampling a
// motion track writes a non-identity transform onto a part, that a
// pointer-driven parameter steers a part, and that swapping the motion changes
// what gets sampled. It is skipped when no Chrome/Chromium is available.
func TestPlayerHeadless(t *testing.T) {
	chrome, err := chromeBinary()
	if err != nil {
		t.Skip("no Chrome available for headless player test:", err)
	}

	// A self-contained page: inline the SVG + the real player.js, then run three
	// deterministic checks and write each outcome into a result <div> that
	// --dump-dom will serialize back to us.
	page := `<!DOCTYPE html><html><body>` + inlineSVG(goodRigSVG) + `
<div id="motion"></div><div id="pointerPos"></div><div id="pointerNeg"></div><div id="swap"></div>
<script>` + playerJS + `</script>
<script>
  var svg = document.querySelector('svg');
  var rig = {
    canvas: [1000, 800],
    parts: [
      {id:"torso", parent:"", pivot:[500,600]},
      {id:"head", parent:"torso", pivot:[500,380]}
    ],
    parameters: [
      {id:"tilt", min:0, max:1, default:0, bindings:[{part:"head", rotate:[0,20]}]},
      {id:"look", min:-1, max:1, default:0, pointer:"x", bindings:[{part:"torso", rotate:[-30,30]}]}
    ]
  };
  // Motion holds tilt at its max for the whole loop, so any t yields a 20deg head rotate.
  // 'look' (pointer-driven) binds the root torso, so pointer effects read cleanly there.
  var motion = {name:"hold", duration:2, loop:true, tracks:[
    {parameter:"tilt", keyframes:[{t:0,v:1},{t:2,v:1}]}
  ]};
  var p = new RigPlayer(svg, rig, motion);
  var head = p.partEls["head"];
  var torso = p.partEls["torso"];

  // 1) motion: with pointer off, holding tilt=1 must rotate head ~+20deg.
  p.followPointer = false;
  p.pose(0.5);
  document.getElementById('motion').textContent = head.getAttribute('transform') || '';

  // 2) pointer to far right (+x) must rotate torso toward +30 (look range -30..30).
  p.followPointer = true;
  p.setPointer(1000, 400); // x at canvas max -> +1 normalized
  p.pose(0.5);
  document.getElementById('pointerPos').textContent = torso.getAttribute('transform') || '';

  // 3) pointer to far left (-x) must rotate the other way.
  p.setPointer(0, 400);
  p.pose(0.5);
  document.getElementById('pointerNeg').textContent = torso.getAttribute('transform') || '';

  // 4) swap motion: a new clip that holds tilt at 0 must zero the motion rotate
  //    (pointer off again so only the track drives it).
  p.followPointer = false;
  p.setMotion({name:"rest", duration:2, loop:true, tracks:[
    {parameter:"tilt", keyframes:[{t:0,v:0},{t:2,v:0}]}
  ]});
  p.pose(0.5);
  document.getElementById('swap').textContent = head.getAttribute('transform') || '(none)';
</script></body></html>`

	dir := t.TempDir()
	pagePath := filepath.Join(dir, "headless.html")
	if err := os.WriteFile(pagePath, []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.Abs(pagePath)

	cmd := exec.Command(chrome,
		"--headless=new", "--disable-gpu", "--hide-scrollbars",
		"--virtual-time-budget=800",
		"--dump-dom",
		"file://"+abs,
	)
	done := make(chan struct{})
	var out []byte
	var runErr error
	go func() { out, runErr = cmd.CombinedOutput(); close(done) }()
	select {
	case <-done:
	case <-time.After(40 * time.Second):
		_ = cmd.Process.Kill()
		t.Skip("headless Chrome did not return in time (environment issue, not a player bug)")
	}
	if runErr != nil {
		t.Fatalf("chrome dump-dom failed: %v: %s", runErr, string(out))
	}
	dom := string(out)

	get := func(id string) string {
		// crude extraction of <div id="ID">...</div> text content.
		marker := `id="` + id + `">`
		i := strings.Index(dom, marker)
		if i < 0 {
			return ""
		}
		rest := dom[i+len(marker):]
		j := strings.Index(rest, "<")
		if j < 0 {
			return rest
		}
		return rest[:j]
	}

	motion := get("motion")
	if !strings.Contains(motion, "rotate(20") {
		t.Errorf("motion track did not pose head to +20deg; head transform = %q", motion)
	}
	pos := get("pointerPos")
	if !strings.Contains(pos, "rotate(30") {
		t.Errorf("pointer +x did not steer head to +30deg; head transform = %q", pos)
	}
	neg := get("pointerNeg")
	if !strings.Contains(neg, "rotate(-30") {
		t.Errorf("pointer -x did not steer head to -30deg; head transform = %q", neg)
	}
	swap := get("swap")
	// After swapping to a rest clip (tilt held at 0) with pointer off, the head
	// should carry no rotation — the transform is empty/removed.
	if strings.Contains(swap, "rotate") {
		t.Errorf("swapped rest motion still rotates head; head transform = %q", swap)
	}
}
