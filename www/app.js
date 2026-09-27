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

  /* ---- the mark ------------------------------------------------------
     goifc-mark.svg, brought to life: one solid in, measured parts out.

     Everything is laid out in the mark's own units — its 24-unit viewBox —
     so the resting frame is the mark: a slab 18 wide and 4 deep, and two
     parts 8 wide and 3 deep beneath it whose outer corners meet the slab's.
     Projection is isometric 2:1, the same as openblox.sh; a footprint unit
     (u or v = 1) is the slab's edge, 9 mark units on screen.

     Reading: STEP lines are read one at a time, each one landing as a strip
     of the slab, in alternating courses. Then the parts drop out beneath it.

     Hovering measures the parts. Both are the same box with the same
     opening, and they report different volumes, because they report
     different things: the left one carries the modeller's authored, net
     IfcElementQuantity (qto); the right one's volume is derived from the
     proxy mesh, which does not subtract the opening — gross, so it
     over-reports and never under-reports. That is drawn as what it is: a
     bound, dashed, around a solid that has a hole in it.

     The figures are one consistent illustrative element: 1.60 × 1.60 ×
     1.20 m is 3.072 m³ gross; the 0.60 × 0.60 opening through 1.60 m is
     0.576 m³; net is 2.496 m³.
  -------------------------------------------------------------------- */

  var host = document.getElementById('blocks');
  var canvas = document.getElementById('stage');
  var ctx = canvas.getContext('2d');
  var reduceQ = window.matchMedia('(prefers-reduced-motion: reduce)');

  var SLAB_H = 4, PART = 4 / 9, PART_TOP = -10.5, PART_H = 3;
  var COURSES = 3, STRIPS = 4;
  var READ_EVERY = 125, FALL = 460;

  var LINES = [
    "#1=IFCPROJECT('0YvctVUKr0kugbFTf53O9L',#2,'goifc',$,$,$,$,(#9),#8);",
    "#10=IFCSITE('2PtIzY$Nn0jfbYgZkWAXmR',#2,'Site',$,$,#11,$,$,.ELEMENT.);",
    "#20=IFCBUILDING('1_0wzUjrD1nw1fPn2V3y7o',#2,'Building',$,$,#21,$,$);",
    "#30=IFCBUILDINGSTOREY('3fhR3Q$7j3Bf2hQeYk9mYx',#2,'Level 1',$,$,#31);",
    "#40=IFCLOCALPLACEMENT(#31,#41);",
    "#41=IFCAXIS2PLACEMENT3D(#42,$,$);",
    "#42=IFCCARTESIANPOINT((0.,0.,1.2));",
    "#50=IFCRECTANGLEPROFILEDEF(.AREA.,$,#51,1.6,1.6);",
    "#52=IFCEXTRUDEDAREASOLID(#50,#53,#54,1.2);",
    "#60=IFCWALL('2O2Fr$t4X7Zf8NOew3FLOH',#2,'Wall',$,$,#40,#61,$,$);",
    "#70=IFCQUANTITYVOLUME('NetVolume',$,$,2.496,$);",
    "#71=IFCELEMENTQUANTITY('0PQ3kS9Rn1ueQ6fN3sRtAx',#2,'Qto_WallBaseQuantities',$,$,(#70));"
  ];

  // The slab, as COURSES courses of STRIPS strips each, bottom course first.
  // Courses alternate direction so it reads as built rather than cut.
  var STRIPS_ALL = [];
  (function build() {
    var ch = SLAB_H / COURSES, w = 1 / STRIPS, n = 0;
    for (var c = 0; c < COURSES; c++) {
      var zb = -SLAB_H + c * ch;
      for (var k = 0; k < STRIPS; k++) {
        var along = c % 2 === 0;
        STRIPS_ALL.push({
          u: along ? 0 : k * w, v: along ? k * w : 0,
          du: along ? 1 : w, dv: along ? w : 1,
          zb: zb, dh: ch, delay: n * READ_EVERY, line: LINES[n % LINES.length]
        });
        n++;
      }
    }
  })();
  var READ = (STRIPS_ALL.length - 1) * READ_EVERY + FALL;
  var PULSE = READ + 120, SPLIT = READ + 520, SPLIT_FOR = 720, RESTED = SPLIT + SPLIT_FOR;

  var PARTS = [
    { u: 0, v: 5 / 9, side: -1, v3: '2.50', tag: 'qto', note: 'authored · net' },
    { u: 5 / 9, v: 0, side: 1, v3: '3.07', tag: 'geometry', note: 'derived · gross' }
  ];

  var DPR = 1, w = 0, h = 0, K = 1, hover = 0, hoverTarget = 0;

  function resize() {
    var r = canvas.getBoundingClientRect();
    if (!r.width) return;
    DPR = Math.min(window.devicePixelRatio || 1, 2);
    w = r.width; h = r.height;
    canvas.width = Math.round(w * DPR);
    canvas.height = Math.round(h * DPR);
    K = Math.min(w, h) / 38;
  }

  // Footprint (u, v) at height z, both in the mark's units, to screen.
  function iso(u, v, z) { return [(u - v) * 9 * K, (u + v) * 4.5 * K - z * K]; }

  function poly(pts, fill, stroke, lw) {
    ctx.beginPath();
    ctx.moveTo(pts[0][0], pts[0][1]);
    for (var i = 1; i < pts.length; i++) ctx.lineTo(pts[i][0], pts[i][1]);
    ctx.closePath();
    if (fill) { ctx.fillStyle = fill; ctx.fill(); }
    if (stroke) { ctx.strokeStyle = stroke; ctx.lineWidth = lw || 1; ctx.stroke(); }
  }

  function line(a, b, stroke, lw, dash) {
    ctx.beginPath();
    ctx.setLineDash(dash || []);
    ctx.moveTo(a[0], a[1]);
    ctx.lineTo(b[0], b[1]);
    ctx.strokeStyle = stroke;
    ctx.lineWidth = lw || 1;
    ctx.stroke();
    ctx.setLineDash([]);
  }

  var RAMP, BG = [244, 245, 241];

  // Faces are opaque: ink mixed into the page ground rather than laid over it
  // with alpha. A translucent face shows every face behind it, and a slab
  // built from strips then reads as a lattice instead of one solid.
  function shade(ink, k) {
    k = Math.min(1, k);
    var i = ink.split(',');
    return 'rgb(' + [0, 1, 2].map(function (n) {
      return Math.round(BG[n] + (+i[n] - BG[n]) * k);
    }).join(',') + ')';
  }

  function rgb(hex) {
    var m = /^#?([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i.exec(hex.trim());
    return m ? [parseInt(m[1], 16), parseInt(m[2], 16), parseInt(m[3], 16)] : BG;
  }

  // A box: footprint [u, u+du] × [v, v+dv], from zb up by dh. Only the three
  // faces a viewer above and in front can see are drawn.
  // `a` fades the whole box in; `lift` brightens it for the pulse.
  function box(u, v, du, dv, zb, dh, ink, a, lift, edge) {
    var zt = zb + dh;
    ctx.globalAlpha = Math.min(1, a);
    poly([iso(u, v, zt), iso(u + du, v, zt), iso(u + du, v + dv, zt), iso(u, v + dv, zt)],
         shade(ink, RAMP.top * lift), edge);
    poly([iso(u, v + dv, zt), iso(u + du, v + dv, zt), iso(u + du, v + dv, zb), iso(u, v + dv, zb)],
         shade(ink, RAMP.left * lift), edge);
    poly([iso(u + du, v, zt), iso(u + du, v + dv, zt), iso(u + du, v + dv, zb), iso(u + du, v, zb)],
         shade(ink, RAMP.right * lift), edge);
    ctx.globalAlpha = 1;
  }

  // The opening, on a part's right face: 0.60 of its 1.60 width, 0.60 of
  // its 1.20 height, centred.
  function opening(p, zb) {
    var u = p.u + PART, v0 = p.v + PART * 0.3125, v1 = p.v + PART * 0.6875;
    var z0 = zb + PART_H * 0.25, z1 = zb + PART_H * 0.75;
    return [iso(u, v0, z1), iso(u, v1, z1), iso(u, v1, z0), iso(u, v0, z0)];
  }

  function ease(x) { return x < 0 ? 0 : x > 1 ? 1 : 1 - Math.pow(1 - x, 3); }

  var MONO = "'Plex Mono', ui-monospace, monospace";

  function label(text, x, y, align, color, size, weight) {
    ctx.font = (weight || 400) + ' ' + size + 'px ' + MONO;
    ctx.textAlign = align;
    ctx.textBaseline = 'middle';
    ctx.fillStyle = color;
    ctx.fillText(text, x, y);
  }

  // A dimension: the measured edge offset outward by `off`, with ticks at
  // both ends and the value beside it.
  function dimension(a, b, off, text, align, stroke, color, size) {
    var pa = [a[0] + off[0], a[1] + off[1]], pb = [b[0] + off[0], b[1] + off[1]];
    line(pa, pb, stroke, 1);
    var tx = off[0] * 0.35, ty = off[1] * 0.35;
    line([pa[0] - tx, pa[1] - ty], [pa[0] + tx, pa[1] + ty], stroke, 1);
    line([pb[0] - tx, pb[1] - ty], [pb[0] + tx, pb[1] + ty], stroke, 1);
    var m = [(pa[0] + pb[0]) / 2 + off[0] * 0.9, (pa[1] + pb[1]) / 2 + off[1] * 0.9];
    label(text, m[0], m[1], align, color, size);
  }

  function measure(p, zb, a, ink, acc) {
    var zt = zb + PART_H, u = p.u, v = p.v, P = PART;
    var mute = 'rgba(' + ink + ',' + 0.55 * a + ')';
    var text = 'rgba(' + ink + ',' + 0.9 * a + ')';
    var accent = 'rgba(' + acc + ',' + a + ')';
    var size = Math.max(9, K * 0.95), gap = K * 1.1;

    if (p.side < 0) {
      // Left part: height on its left corner, width along its left base.
      dimension(iso(u, v + P, zt), iso(u, v + P, zb), [-gap, 0], '1.20', 'right', mute, text, size);
      dimension(iso(u, v + P, zb), iso(u + P, v + P, zb), [-gap * 0.6, gap * 0.8], '1.60', 'right', mute, text, size);
    } else {
      // Right part: height on its right corner, width along its right base.
      dimension(iso(u + P, v, zt), iso(u + P, v, zb), [gap, 0], '1.20', 'left', mute, text, size);
      dimension(iso(u + P, v, zb), iso(u + P, v + P, zb), [gap * 0.6, gap * 0.8], '1.60', 'left', mute, text, size);
      // The bound: the whole box, opening and all, dashed.
      var o = 0.012, d = [K * 0.5, K * 0.4];
      var T = [iso(u - o, v - o, zt + 0.12), iso(u + P + o, v - o, zt + 0.12),
               iso(u + P + o, v + P + o, zt + 0.12), iso(u - o, v + P + o, zt + 0.12)];
      var B = [iso(u - o, v + P + o, zb - 0.12), iso(u + P + o, v + P + o, zb - 0.12),
               iso(u + P + o, v - o, zb - 0.12)];
      for (var i = 0; i < 4; i++) line(T[i], T[(i + 1) % 4], accent, 1.2, d);
      line(T[3], B[0], accent, 1.2, d); line(B[0], B[1], accent, 1.2, d);
      line(T[2], B[1], accent, 1.2, d); line(B[1], B[2], accent, 1.2, d);
      line(T[1], B[2], accent, 1.2, d);
    }

    // The quantity, under the part, on its outer side.
    var base = iso(p.side < 0 ? u : u + P, p.side < 0 ? v + P : v, zb);
    var x = base[0], y = base[1] + K * 4.6;
    var al = p.side < 0 ? 'left' : 'right';
    label('V ' + p.v3 + ' m³', x, y, al, text, size * 1.15, 500);
    label(p.tag, x, y + size * 1.55, al, p.side > 0 ? accent : text, size, 500);
    // On a narrow canvas the tag carries it; the gloss would collide.
    if (K >= 12) label(p.note, x, y + size * 2.9, al, mute, size);
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

    BG = rgb(bg);
    RAMP = isDark() ? { top: 0.60, left: 0.34, right: 0.20 }
                    : { top: 0.26, left: 0.58, right: 0.76 };

    hover += (hoverTarget - hover) * (still ? 1 : 0.12);

    ctx.setTransform(DPR, 0, 0, DPR, 0, 0);
    ctx.clearRect(0, 0, w, h);
    ctx.save();
    // The mark is 24 units tall; centre it, a little high, leaving room
    // for the reading line above and the quantities below.
    ctx.translate(w / 2, h / 2 - K * 12.5);

    // The line being read, above the slab. It goes once the parts are out.
    var n = Math.min(STRIPS_ALL.length - 1, Math.floor(t / READ_EVERY));
    var readA = still ? 0 : 1 - ease((t - RESTED + 400) / 500);
    if (readA > 0.01) {
      var fs = Math.max(9, K * 0.9);
      ctx.font = '400 ' + fs + 'px ' + MONO;
      var s = STRIPS_ALL[n].line, max = w * 0.86;
      while (s.length > 8 && ctx.measureText(s).width > max) s = s.slice(0, -2);
      if (s !== STRIPS_ALL[n].line) s = s.slice(0, -1) + '…';
      label(s, 0, -K * 4.2, 'center', 'rgba(' + acc + ',' + 0.9 * readA + ')', fs);
    }

    // The parts first: they come out from under the slab, and the slab is
    // drawn over them while they do.
    var sp = still ? 1 : ease((t - SPLIT) / SPLIT_FOR);
    var breath = !still && t > RESTED ? (Math.sin((t - RESTED) / 1600 * Math.PI) + 1) / 2 : 0;
    if (sp > 0) {
      PARTS.forEach(function (p) {
        // From tucked under the slab to resting where the mark puts it.
        var zb = (-SLAB_H - PART_H) + (PART_TOP + SLAB_H) * sp;
        box(p.u, p.v, PART, PART, zb, PART_H, ink, sp * 1.6, 1 + breath * 0.08, bg);
        // The opening. On the measured-by-geometry part, looking closer
        // fills it in: that volume is in its number.
        var hole = opening(p, zb);
        // A hole reads deeper than the face around it, in either theme.
        ctx.globalAlpha = Math.min(1, sp * 1.6);
        poly(hole, shade(ink, isDark() ? 0.06 : 0.95), null);
        ctx.globalAlpha = 1;
        if (p.side > 0 && hover > 0.01)
          poly(hole, 'rgba(' + acc + ',' + 0.55 * hover + ')', null);
      });
    }

    // The slab, strip by strip as each line is read, bottom course first.
    STRIPS_ALL.forEach(function (s) {
      var p = still ? 1 : Math.max(0, Math.min(1, (t - s.delay) / FALL));
      if (p <= 0) return;
      var drop = (1 - ease(p)) * 7;
      var dt = t - PULSE - (s.u + s.v) * 260;
      var lift = (!still && dt > 0 && dt < 520) ? 1 + 0.22 * Math.sin(dt / 520 * Math.PI) : 1;
      box(s.u, s.v, s.du, s.dv, s.zb + drop, s.dh, ink, p * 1.7, lift, bg);
    });

    if (hover > 0.01 && sp >= 1) {
      PARTS.forEach(function (p) { measure(p, PART_TOP - PART_H, hover, ink, acc); });
    }

    ctx.restore();

    if (!still && t > RESTED + 3000 && !looked) host.dataset.hint = '1';
  }

  function frame(now) {
    if (t0 === null) t0 = now;
    draw(now);
    requestAnimationFrame(frame);
  }

  // Under reduced motion there is no frame loop to pick up a new theme, a
  // resized canvas or a hover, so those paint the one still frame again.
  function repaint() { if (reduceQ.matches) draw(0); }
  function refit() { resize(); repaint(); }

  function measureOn(on) {
    hoverTarget = on ? 1 : 0;
    host.dataset.open = on ? '1' : '0';
    if (on) { looked = true; host.dataset.hint = '0'; }
    repaint();
  }
  host.addEventListener('pointerenter', function () { measureOn(true); });
  host.addEventListener('pointerleave', function () { measureOn(false); });
  host.addEventListener('focus', function () { measureOn(true); });
  host.addEventListener('blur', function () { measureOn(false); });

  if (window.ResizeObserver) new ResizeObserver(refit).observe(canvas);
  window.addEventListener('resize', refit);
  resize();
  if (document.fonts) document.fonts.ready.then(repaint);
  if (reduceQ.matches) { draw(0); } else { requestAnimationFrame(frame); }
})();
