// RigPlayer — a tiny runtime that poses a layered "rig" SVG the way Live2D
// poses an avatar: appearance (the SVG), skeleton + parameters (rig), and
// motion (a keyframed timeline) are three independent inputs. The player binds
// them at runtime so any motion can drive any compatible rig, and pointer input
// can drive parameters live.
//
// No dependencies, no build step. Used by the generated harness.html, but
// written as a plain class so it can be reused standalone.

(function (global) {
  'use strict';

  // lerp / clamp helpers.
  function lerp(a, b, t) { return a + (b - a) * t; }
  function clamp(v, lo, hi) { return v < lo ? lo : v > hi ? hi : v; }

  // sampleTrack returns the interpolated value of a keyframed track at time t
  // (seconds), clamping outside the keyframe range. Keyframes are assumed
  // sorted by .t; we sort defensively on load.
  function sampleTrack(track, t) {
    const k = track.keyframes;
    if (!k || k.length === 0) return 0;
    if (t <= k[0].t) return k[0].v;
    if (t >= k[k.length - 1].t) return k[k.length - 1].v;
    for (let i = 0; i < k.length - 1; i++) {
      const a = k[i], b = k[i + 1];
      if (t >= a.t && t <= b.t) {
        const span = b.t - a.t;
        const f = span <= 0 ? 0 : (t - a.t) / span;
        return lerp(a.v, b.v, f);
      }
    }
    return k[k.length - 1].v;
  }

  // map a parameter's raw value onto a binding's [atMin, atMax] output range.
  function bindOut(param, value, range) {
    const span = param.max - param.min;
    const f = span === 0 ? 0 : clamp((value - param.min) / span, 0, 1);
    return lerp(range[0], range[1], f);
  }

  class RigPlayer {
    // svgEl: the root <svg> element. rig: parsed rig.json. motion: parsed
    // motion.json (may be null — then the rig just sits at parameter defaults,
    // useful for pointer-only driving).
    constructor(svgEl, rig, motion) {
      this.svg = svgEl;
      this.rig = rig;
      this.setMotion(motion);

      // Resolve each part id to its <g> element once.
      this.partEls = {};
      for (const p of rig.parts) {
        const el = svgEl.querySelector('[data-part="' + p.id + '"]');
        if (el) this.partEls[p.id] = el;
      }

      // Current parameter values, seeded from defaults.
      this.params = {};
      for (const param of rig.parameters) this.params[param.id] = param.default || 0;

      // Pointer state, normalized to [-1,1] over the canvas; null until moved.
      this.pointer = null;
      this.followPointer = true;
      this.playing = true;
      this._t0 = 0;
      this._raf = null;
    }

    setMotion(motion) {
      this.motion = motion || null;
      if (this.motion) {
        for (const tr of this.motion.tracks) {
          if (tr.keyframes) tr.keyframes.sort((a, b) => a.t - b.t);
        }
      }
    }

    // Feed a pointer position in SVG user-space (or null to clear).
    setPointer(xUser, yUser) {
      if (xUser == null) { this.pointer = null; return; }
      const w = this.rig.canvas[0] || 1024;
      const h = this.rig.canvas[1] || 1024;
      this.pointer = [clamp((xUser / w) * 2 - 1, -1, 1), clamp((yUser / h) * 2 - 1, -1, 1)];
    }

    // Compute the active value of every parameter for time t (seconds), then
    // pose every part. Pointer-bound parameters override their motion track
    // when followPointer is on and the pointer is present.
    pose(t) {
      // 1) parameter values: motion tracks first, then pointer overrides.
      const vals = {};
      for (const param of this.rig.parameters) vals[param.id] = param.default || 0;

      if (this.motion) {
        const dur = this.motion.duration || 1;
        const tt = this.motion.loop ? (dur > 0 ? t % dur : 0) : Math.min(t, dur);
        for (const tr of this.motion.tracks) {
          if (tr.parameter in vals) vals[tr.parameter] = sampleTrack(tr, tt);
        }
      }

      if (this.followPointer && this.pointer) {
        for (const param of this.rig.parameters) {
          if (param.pointer === 'x') vals[param.id] = lerp(param.min, param.max, (this.pointer[0] + 1) / 2);
          else if (param.pointer === 'y') vals[param.id] = lerp(param.min, param.max, (this.pointer[1] + 1) / 2);
        }
      }
      this.params = vals;

      // 2) accumulate transforms per part from every binding.
      const acc = {}; // part id -> {rot, tx, ty, sx, sy}
      const ensure = (id) => acc[id] || (acc[id] = { rot: 0, tx: 0, ty: 0, sx: 1, sy: 1 });
      for (const param of this.rig.parameters) {
        const v = vals[param.id];
        for (const b of param.bindings || []) {
          const a = ensure(b.part);
          if (b.rotate) a.rot += bindOut(param, v, b.rotate);
          if (b.translateX) a.tx += bindOut(param, v, b.translateX);
          if (b.translateY) a.ty += bindOut(param, v, b.translateY);
          if (b.scale) { const s = bindOut(param, v, b.scale); a.sx *= s; a.sy *= s; }
        }
      }

      // 3) write transforms. DOM nesting composes the bone chain for us, so each
      // part only carries its own LOCAL transform about its absolute pivot.
      for (const p of this.rig.parts) {
        const el = this.partEls[p.id];
        if (!el) continue;
        const a = acc[p.id];
        if (!a) { el.removeAttribute('transform'); continue; }
        const [px, py] = p.pivot;
        const t = [];
        if (a.tx || a.ty) t.push('translate(' + a.tx.toFixed(3) + ' ' + a.ty.toFixed(3) + ')');
        if (a.rot) t.push('rotate(' + a.rot.toFixed(3) + ' ' + px + ' ' + py + ')');
        if (a.sx !== 1 || a.sy !== 1) {
          t.push('translate(' + px + ' ' + py + ') scale(' + a.sx.toFixed(4) + ' ' + a.sy.toFixed(4) + ') translate(' + (-px) + ' ' + (-py) + ')');
        }
        if (t.length) el.setAttribute('transform', t.join(' '));
        else el.removeAttribute('transform');
      }
    }

    start() {
      this.playing = true;
      const loop = (now) => {
        if (!this._t0) this._t0 = now;
        if (this.playing) this.pose((now - this._t0) / 1000);
        this._raf = global.requestAnimationFrame(loop);
      };
      this._raf = global.requestAnimationFrame(loop);
    }

    pause() { this.playing = false; }
    resume() { this.playing = true; }
    stop() { if (this._raf) global.cancelAnimationFrame(this._raf); this._raf = null; }
  }

  global.RigPlayer = RigPlayer;
})(typeof window !== 'undefined' ? window : globalThis);
