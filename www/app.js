(function () {
  'use strict';

  var root = document.documentElement;

  /* ---- copy ---------------------------------------------------------- */

  var cmdtext = document.getElementById('cmdtext');
  var copy = document.getElementById('copy');
  // The button keeps its accessible name, so the change of label is not
  // announced; the result is said once, through a status region beside it.
  var copied = document.getElementById('copied');
  copy.addEventListener('click', function () {
    if (!navigator.clipboard) return;
    navigator.clipboard.writeText(cmdtext.textContent).then(function () {
      copy.textContent = 'Copied';
      copy.dataset.done = '1';
      copied.textContent = 'Copied to the clipboard';
      setTimeout(function () {
        copy.textContent = 'Copy'; copy.dataset.done = '0'; copied.textContent = '';
      }, 1600);
    }, function () {});
  });

  /* ---- theme --------------------------------------------------------- */

  var SUN = '<path d="M8 11a3 3 0 1 1 0-6 3 3 0 0 1 0 6Zm0-8.5a.6.6 0 0 1-.6-.6V.6a.6.6 0 0 1 1.2 0v1.3a.6.6 0 0 1-.6.6Zm0 13a.6.6 0 0 1-.6-.6v-1.3a.6.6 0 0 1 1.2 0v1.3a.6.6 0 0 1-.6.6ZM15.4 8.6h-1.3a.6.6 0 0 1 0-1.2h1.3a.6.6 0 0 1 0 1.2Zm-13.5 0H.6a.6.6 0 0 1 0-1.2h1.3a.6.6 0 0 1 0 1.2Zm11.3-4.9-.9.9a.6.6 0 0 1-.85-.85l.9-.9a.6.6 0 0 1 .85.85ZM4.05 13.1l-.9.9a.6.6 0 0 1-.85-.85l.9-.9a.6.6 0 0 1 .85.85Zm9.15.9-.9-.9a.6.6 0 0 1 .85-.85l.9.9a.6.6 0 0 1-.85.85ZM3.15 4.6l-.9-.9a.6.6 0 0 1 .85-.85l.9.9a.6.6 0 0 1-.85.85Z"/>';
  var MOON = '<path d="M13.9 9.6A6.2 6.2 0 0 1 6.4 2.1a.6.6 0 0 0-.83-.67 7.4 7.4 0 1 0 9 9 .6.6 0 0 0-.67-.83Z"/>';

  var themeBtn = document.getElementById('theme');
  var themeIcon = document.getElementById('theme-icon');
  function isDark() {
    return root.dataset.theme
      ? root.dataset.theme === 'dark'
      : window.matchMedia('(prefers-color-scheme: dark)').matches;
  }
  function syncTheme() {
    var d = isDark();
    themeIcon.innerHTML = d ? SUN : MOON;
    themeBtn.setAttribute('aria-label', d ? 'Switch to light theme' : 'Switch to dark theme');
  }
  themeBtn.addEventListener('click', function () {
    root.dataset.theme = isDark() ? 'light' : 'dark';
    syncTheme();
    repaint();
  });
  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', function () {
    syncTheme();
    repaint();
  });
  syncTheme();

  /* ---- the storey ----------------------------------------------------
     Isometric 2:1, the same projection as goifc-mark.svg and openblox.sh.

     It runs the pipeline in the order the packages do. step: the pieces
     arrive scattered, the way entities come off the file — present, not yet
     anywhere. model: each one resolves to its placement and the storey
     stands, a slab and two courses of wall. geometry: a plane comes down
     through the walls and the cut face turns to the accent — the closed
     rings SectionOn returns, an outer one and the hole inside it.

     Nothing here claims more than the library does. The cut is a section of
     the tessellated walls; it is not an exact solid, and it is not drawn as
     one — the parts above the plane go to glass, they do not vanish.
  -------------------------------------------------------------------- */

  var host = document.getElementById('blocks');
  var canvas = document.getElementById('stage');
  var ctx = canvas.getContext('2d');
  var reduceQ = window.matchMedia('(prefers-reduced-motion: reduce)');
  var stages = {
    step: document.getElementById('s-step'),
    model: document.getElementById('s-model'),
    geometry: document.getElementById('s-geometry')
  };

  var N = 4;                          // 4x4 slab, two courses of wall, top at z = 2
  var CUT_Z = 1.5;                    // where the plane settles: halfway up the top course
  var PLANE_FROM = 3.6;               // where it starts: clear above the walls
  var FALL = 520;
  var PIECES = [], BUILT = 0;

  (function build() {
    var step = 34, t = 0, seed = 7;
    // Deterministic scatter, so every visit assembles the same way.
    function rnd() { seed = (seed * 16807) % 2147483647; return seed / 2147483647 - 0.5; }
    function add(x, y, z) {
      PIECES.push({ x: x, y: y, z: z, dx: rnd() * 3.2, dy: rnd() * 3.2,
                    delay: t, land: t + FALL });
      t += step;
    }

    for (var s = 0; s <= (N - 1) * 2; s++) {
      for (var x = 0; x < N; x++) {
        var y = s - x;
        if (y >= 0 && y < N) add(x, y, 0);
      }
    }

    var ring = [];
    for (var i = 0; i < N; i++) ring.push([i, 0]);
    for (var j = 1; j < N; j++) ring.push([N - 1, j]);
    for (var k = N - 2; k >= 0; k--) ring.push([k, N - 1]);
    for (var m = N - 2; m >= 1; m--) ring.push([0, m]);
    for (var z = 1; z <= 2; z++) {
      for (var r = 0; r < ring.length; r++) add(ring[r][0], ring[r][1], z);
    }

    BUILT = t - step + FALL;
  })();

  var T = 22, DPR = 1, w = 0, h = 0, hover = 0, hoverTarget = 0;

  function resize() {
    var r = canvas.getBoundingClientRect();
    if (!r.width) return;
    DPR = Math.min(window.devicePixelRatio || 1, 2);
    w = r.width; h = r.height;
    canvas.width = Math.round(w * DPR);
    canvas.height = Math.round(h * DPR);
    T = Math.max(10, Math.min(w, h) / 10.4);
  }

  function poly(pts, fill, stroke) {
    ctx.beginPath();
    ctx.moveTo(pts[0][0], pts[0][1]);
    for (var i = 1; i < pts.length; i++) ctx.lineTo(pts[i][0], pts[i][1]);
    ctx.closePath();
    if (fill) { ctx.fillStyle = fill; ctx.fill(); }
    if (stroke) { ctx.strokeStyle = stroke; ctx.lineWidth = 1; ctx.stroke(); }
  }

  // A grid point (u, v) at height z, on screen.
  function iso(u, v, z) { return [(u - v) * T, (u + v) * T * 0.5 - z * T]; }

  function square(lo, hi, z) {
    return [iso(lo, lo, z), iso(hi, lo, z), iso(hi, hi, z), iso(lo, hi, z)];
  }

  var RAMP;

  // A cube whose top sits at height `top` and whose sides run `tall` units
  // down. A whole block is tall = 1; the two halves of a cut block are less.
  // `topFill` overrides the top face — it is how the cut face takes the accent.
  function cube(gx, gy, top, tall, ink, a, edge, hide, topFill) {
    var p = iso(gx, gy, top), sx = p[0], sy = p[1];
    var W = T, H = T * tall;
    var c = 'rgba(' + ink + ',';
    if (!(hide & 1))
      poly([[sx, sy], [sx + W, sy + W / 2], [sx, sy + W], [sx - W, sy + W / 2]],
           topFill || c + Math.min(1, RAMP.top * a) + ')', topFill ? null : edge);
    if (!(hide & 2))
      poly([[sx - W, sy + W / 2], [sx, sy + W], [sx, sy + W + H], [sx - W, sy + W / 2 + H]],
           c + Math.min(1, RAMP.left * a) + ')', edge);
    if (!(hide & 4))
      poly([[sx + W, sy + W / 2], [sx, sy + W], [sx, sy + W + H], [sx + W, sy + W / 2 + H]],
           c + Math.min(1, RAMP.right * a) + ')', edge);
  }

  var AT = {};
  PIECES.forEach(function (b) { AT[b.x + ',' + b.y + ',' + b.z] = b; });
  function settledAt(x, y, z, t) {
    var n = AT[x + ',' + y + ',' + z];
    return n && t >= n.land;
  }

  function ease(x) { return x < 0 ? 0 : x > 1 ? 1 : x * x * (3 - 2 * x); }

  // Once the storey stands, the plane comes through on its own every so
  // often: down, a pause long enough to read the rings, back out.
  var SWEEP_AT = BUILT + 600, SWEEP_EVERY = 9000;
  function autoCut(t) {
    if (t < SWEEP_AT) return 0;
    var c = (t - SWEEP_AT) % SWEEP_EVERY;
    if (c < 900) return ease(c / 900);
    if (c < 2500) return 1;
    if (c < 3400) return 1 - ease((c - 2500) / 900);
    return 0;
  }

  var lit = null;
  function light(name) {
    if (name === lit) return;
    lit = name;
    for (var k in stages) stages[k].dataset.on = k === name ? '1' : '0';
  }

  var t0 = null, looked = false;

  function draw(now) {
    if (!w || !h) { resize(); if (!w) return; }

    var cs = getComputedStyle(root);
    var ink = cs.getPropertyValue('--ink-rgb').trim() || '14,17,19';
    var acc = cs.getPropertyValue('--accent-rgb').trim() || '11,114,133';
    var bg = cs.getPropertyValue('--bg').trim() || '#f4f5f1';
    var still = reduceQ.matches;
    var t = still ? 1e6 : (t0 === null ? 0 : now - t0);

    RAMP = isDark() ? { top: 0.60, left: 0.34, right: 0.20 }
                    : { top: 0.26, left: 0.58, right: 0.76 };

    hover += (hoverTarget - hover) * (still ? 1 : 0.12);
    var cut = still ? 1 : Math.max(autoCut(t), t > BUILT ? hover : 0);
    var zc = PLANE_FROM - (PLANE_FROM - CUT_Z) * cut;

    if (still) light('geometry');
    else if (cut > 0.02) light('geometry');
    else if (t < FALL + 200) light('step');
    else if (t < BUILT + 250) light('model');
    else light(null);

    ctx.setTransform(DPR, 0, 0, DPR, 0, 0);
    ctx.clearRect(0, 0, w, h);
    ctx.save();
    ctx.translate(w / 2, h / 2 - T * 1.4);

    poly(square(-0.25, N + 0.25, -1.1), 'rgba(' + ink + ',0.055)', null);

    var solid = [], glass = [];

    PIECES.forEach(function (b) {
      var p = still ? 1 : Math.max(0, Math.min(1, (t - b.delay) / FALL));
      var q = (1 - p) * (1 - p);
      var it = { gx: b.x + b.dx * q, gy: b.y + b.dy * q, z: b.z + (1 - p * p) * 2.2,
                 b: b, a: Math.min(1, p * 1.8), landed: p >= 1 };
      if (it.a <= 0.01) return;

      it.hide = 0;
      if (it.landed) {
        if (settledAt(b.x, b.y, b.z + 1, t)) it.hide |= 1;
        if (settledAt(b.x, b.y + 1, b.z, t)) it.hide |= 2;
        if (settledAt(b.x + 1, b.y, b.z, t)) it.hide |= 4;
      }

      // Only a standing block is cut: the plane arrives after the storey does.
      var top = it.z, bottom = it.z - 1;
      if (!it.landed || top <= zc) { solid.push(it); return; }
      if (bottom >= zc) { glass.push({ it: it, top: top, tall: 1 }); return; }
      solid.push({ gx: it.gx, gy: it.gy, z: zc, tall: zc - bottom, a: it.a,
                   hide: it.hide & ~1, cutFace: true, b: b });
      glass.push({ it: it, top: top, tall: top - zc });
    });

    function order(a, b) { return (a.b.x + a.b.y) - (b.b.x + b.b.y) || a.z - b.z; }
    solid.sort(order);

    solid.forEach(function (s) {
      cube(s.gx, s.gy, s.z, s.tall || 1, ink, s.a, bg, s.hide,
           s.cutFace ? 'rgba(' + acc + ',' + (0.35 + 0.55 * cut) + ')' : null);
    });

    if (cut > 0.01) {
      // The plane itself, wider than the storey: it is a plane, not a lid.
      poly(square(-1.4, N + 1.4, zc), 'rgba(' + acc + ',' + 0.06 * cut + ')',
           'rgba(' + acc + ',' + 0.4 * cut + ')');
      // Once it is inside the walls, the two rings it returns: the outline
      // of the storey and the hole of the room within it.
      if (zc < 2) {
        var ra = 'rgba(' + acc + ',' + Math.min(1, cut * 1.2) + ')';
        ctx.lineWidth = 1.6;
        [square(0, N, zc), square(1, N - 1, zc)].forEach(function (ring) {
          poly(ring, null, null);
          ctx.strokeStyle = ra;
          ctx.stroke();
        });
      }
    }

    glass.forEach(function (g) {
      cube(g.it.gx, g.it.gy, g.top, g.tall, ink, g.it.a * (0.12 + 0.88 * (1 - cut)),
           cut > 0.3 ? null : bg, g.it.hide & ~1, null);
    });

    ctx.restore();

    if (!still && t > BUILT + 3800 && !looked) host.dataset.hint = '1';
  }

  function frame(now) {
    if (t0 === null) t0 = now;
    draw(now);
    requestAnimationFrame(frame);
  }

  function cutThrough(on) {
    hoverTarget = on ? 1 : 0;
    host.dataset.open = on ? '1' : '0';
    if (on) { looked = true; host.dataset.hint = '0'; }
  }
  host.addEventListener('pointerenter', function () { cutThrough(true); });
  host.addEventListener('pointerleave', function () { cutThrough(false); });
  host.addEventListener('focus', function () { cutThrough(true); });
  host.addEventListener('blur', function () { cutThrough(false); });
  host.addEventListener('click', function () { t0 = null; });

  // Under reduced motion there is no frame loop to pick up a new theme or a
  // resized canvas, so those paint the one still frame again themselves.
  function repaint() { if (reduceQ.matches) draw(0); }
  function refit() { resize(); repaint(); }

  if (window.ResizeObserver) new ResizeObserver(refit).observe(canvas);
  window.addEventListener('resize', refit);
  resize();
  if (reduceQ.matches) { draw(0); } else { requestAnimationFrame(frame); }
})();
