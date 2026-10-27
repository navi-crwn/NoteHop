// Home hero: the pixel logo as 3D blocks, in plain WebGL (no library to
// load). The blocks fly in and settle, float a little, turn slowly and follow
// a drag. Without WebGL the picture behind the canvas stays; with "reduce
// motion" the logo is drawn once, still.
(function () {
  'use strict';
  var cv = document.getElementById('h3d');
  if (!cv) return;
  var gl = cv.getContext('webgl', { antialias: true, alpha: true });
  if (!gl) return;
  var reduce = window.matchMedia && matchMedia('(prefers-reduced-motion: reduce)').matches;

  var VS = 'attribute vec3 p;attribute vec3 n;uniform mat4 M;uniform mat4 VP;varying vec3 vn;varying vec3 vw;' +
    'void main(){vec4 w=M*vec4(p,1.);vw=w.xyz;vn=mat3(M)*n;gl_Position=VP*w;}';
  // Soft studio light: sky/ground fill, a key light, a cyan back light, a highlight and a violet rim.
  var FS = 'precision mediump float;uniform vec3 col;uniform vec3 eye;varying vec3 vn;varying vec3 vw;' +
    'void main(){vec3 N=normalize(vn);vec3 V=normalize(eye-vw);vec3 L=normalize(vec3(.45,.75,.9));vec3 B=normalize(vec3(-.7,-.35,-.6));' +
    'float d=max(dot(N,L),0.);float b=max(dot(N,B),0.);float hemi=.55+.45*N.y;float s=pow(max(dot(N,normalize(L+V)),0.),56.);' +
    'float rim=pow(1.-max(dot(N,V),0.),2.5);' +
    'vec3 c=col*(.38+.34*hemi)+col*d*.5+vec3(.13,.83,.93)*b*.3+vec3(1.)*s*.55+vec3(.6,.5,1.)*rim*.32;gl_FragColor=vec4(c,1.);}';
  function shader(type, src) { var s = gl.createShader(type); gl.shaderSource(s, src); gl.compileShader(s); return s; }
  var prog = gl.createProgram();
  gl.attachShader(prog, shader(gl.VERTEX_SHADER, VS));
  gl.attachShader(prog, shader(gl.FRAGMENT_SHADER, FS));
  gl.linkProgram(prog);
  if (!gl.getProgramParameter(prog, gl.LINK_STATUS)) return;
  gl.useProgram(prog);
  var U = {};
  ['M', 'VP', 'col', 'eye'].forEach(function (k) { U[k] = gl.getUniformLocation(prog, k); });
  var aP = gl.getAttribLocation(prog, 'p'), aN = gl.getAttribLocation(prog, 'n');

  // ---------- geometry: a cube (slightly bevel-less, the light does the work) and an octahedron ----------
  function mesh(data) {
    var b = gl.createBuffer();
    gl.bindBuffer(gl.ARRAY_BUFFER, b);
    gl.bufferData(gl.ARRAY_BUFFER, new Float32Array(data), gl.STATIC_DRAW);
    return { buf: b, count: data.length / 6 };
  }
  function cube(s) {
    var out = [], F = [[0, 0, 1], [0, 0, -1], [1, 0, 0], [-1, 0, 0], [0, 1, 0], [0, -1, 0]];
    F.forEach(function (n) {
      var u = n[0] ? [0, 1, 0] : [1, 0, 0], v = [n[1] * u[2] - n[2] * u[1], n[2] * u[0] - n[0] * u[2], n[0] * u[1] - n[1] * u[0]];
      var c = [[-1, -1], [1, -1], [1, 1], [-1, -1], [1, 1], [-1, 1]];
      c.forEach(function (q) {
        for (var i = 0; i < 3; i++) out.push(s * (n[i] + q[0] * u[i] + q[1] * v[i]));
        out.push(n[0], n[1], n[2]);
      });
    });
    return out;
  }
  function octa(s) {
    var P = [[1, 0, 0], [-1, 0, 0], [0, 1, 0], [0, -1, 0], [0, 0, 1], [0, 0, -1]], out = [];
    [[0, 2, 4], [2, 1, 4], [1, 3, 4], [3, 0, 4], [2, 0, 5], [1, 2, 5], [3, 1, 5], [0, 3, 5]].forEach(function (f) {
      var a = P[f[0]], b = P[f[1]], c = P[f[2]], n = [a[0] + b[0] + c[0], a[1] + b[1] + c[1], a[2] + b[2] + c[2]], l = Math.hypot(n[0], n[1], n[2]);
      [a, b, c].forEach(function (p) { out.push(p[0] * s, p[1] * s, p[2] * s, n[0] / l, n[1] / l, n[2] / l); });
    });
    return out;
  }
  var CUBE = mesh(cube(.46)), GEM = mesh(octa(1));

  // ---------- tiny matrix helpers (column-major) ----------
  function mul(a, b) {
    var o = new Float32Array(16);
    for (var c = 0; c < 4; c++) for (var r = 0; r < 4; r++) {
      o[c * 4 + r] = a[r] * b[c * 4] + a[4 + r] * b[c * 4 + 1] + a[8 + r] * b[c * 4 + 2] + a[12 + r] * b[c * 4 + 3];
    }
    return o;
  }
  function T(x, y, z) { return new Float32Array([1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, x, y, z, 1]); }
  function S(k) { return new Float32Array([k, 0, 0, 0, 0, k, 0, 0, 0, 0, k, 0, 0, 0, 0, 1]); }
  function RX(a) { var c = Math.cos(a), s = Math.sin(a); return new Float32Array([1, 0, 0, 0, 0, c, s, 0, 0, -s, c, 0, 0, 0, 0, 1]); }
  function RY(a) { var c = Math.cos(a), s = Math.sin(a); return new Float32Array([c, 0, -s, 0, 0, 1, 0, 0, s, 0, c, 0, 0, 0, 0, 1]); }
  function persp(fov, asp, n, f) {
    var t = 1 / Math.tan(fov / 2), nf = 1 / (n - f);
    return new Float32Array([t / asp, 0, 0, 0, 0, t, 0, 0, 0, 0, (f + n) * nf, -1, 0, 0, 2 * f * n * nf, 0]);
  }

  // ---------- the logo: 8×8 grid, filled columns per row; (row 2, col 5) is pink ----------
  var ROWS = [[2, 5], [2, 5], [1, 2, 3, 4, 5], [1, 2, 3, 4, 5, 6], [1, 5, 6], [1, 2, 3, 4, 5, 6], [1, 4, 5, 6], [1, 2, 3, 4, 5, 6]];
  var STOPS = [[.639, .902, .208], [.133, .827, .933], [.545, .361, .965]];
  function grad(t) { var a = t < .5 ? STOPS[0] : STOPS[1], b = t < .5 ? STOPS[1] : STOPS[2], k = t < .5 ? t * 2 : (t - .5) * 2; return [0, 1, 2].map(function (i) { return a[i] + (b[i] - a[i]) * k; }); }
  function rnd(a) { return (Math.random() - .5) * a; }
  var blocks = [];
  ROWS.forEach(function (cols, r) {
    cols.forEach(function (c) {
      blocks.push({ home: [c - 3.5, 3.5 - r, 0], from: [rnd(26), rnd(18), rnd(14)], spin: [Math.random() * 6, Math.random() * 6],
        delay: Math.random() * .5, col: r === 2 && c === 5 ? [.957, .447, .714] : grad((c + r) / 14) });
    });
  });
  var gems = [[[.133, .827, .933], -6, 4, -2, .5], [[.655, .545, .98], 6.2, -3.5, -1, .62], [[.957, .447, .714], 5.4, 4.6, -3, .4], [[.639, .902, .208], -5.6, -4.4, 0, .45], [[.506, .549, .973], 3.4, -6.2, -4, .36]];

  // ---------- drawing ----------
  var eye = [0, 0, 22], VP, dpr = Math.min(window.devicePixelRatio || 1, 2);
  function size() {
    var w = cv.clientWidth, h = cv.clientHeight;
    cv.width = Math.round(w * dpr); cv.height = Math.round(h * dpr);
    gl.viewport(0, 0, cv.width, cv.height);
    VP = mul(persp(32 * Math.PI / 180, w / Math.max(1, h), .1, 100), T(-eye[0], -eye[1], -eye[2]));
  }
  function draw(m, model, col) {
    gl.bindBuffer(gl.ARRAY_BUFFER, m.buf);
    gl.vertexAttribPointer(aP, 3, gl.FLOAT, false, 24, 0);
    gl.vertexAttribPointer(aN, 3, gl.FLOAT, false, 24, 12);
    gl.uniformMatrix4fv(U.M, false, model);
    gl.uniform3fv(U.col, col);
    gl.drawArrays(gl.TRIANGLES, 0, m.count);
  }
  gl.enableVertexAttribArray(aP);
  gl.enableVertexAttribArray(aN);
  gl.enable(gl.DEPTH_TEST);
  gl.enable(gl.CULL_FACE);

  var rotY = 0, rotX = 0, vel = .004, drag = null, tiltX = 0, tiltY = 0, t0 = performance.now(), visible = true, settled = false;
  function frame(now) {
    var t = (now - t0) / 1000, all = true;
    gl.clearColor(0, 0, 0, 0);
    gl.clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT);
    gl.uniformMatrix4fv(U.VP, false, VP);
    gl.uniform3fv(U.eye, eye);
    if (!drag && !reduce) { vel += (.004 - vel) * .02; rotY += vel; rotX *= .96; }
    var group = mul(RX(rotX * .4 + tiltX + (reduce ? 0 : Math.sin(t * .6) * .05)), RY(Math.sin(rotY) * .6 + tiltY));
    blocks.forEach(function (b) {
      var k = reduce ? 1 : Math.min(1, Math.max(0, (t - b.delay) / 1.4)), e = 1 - Math.pow(1 - k, 3);
      if (k < 1) all = false;
      var p = [0, 1, 2].map(function (i) { return b.from[i] + (b.home[i] - b.from[i]) * e; });
      if (k >= 1 && !reduce) p[2] = Math.sin(t * 1.6 + b.home[0] * .7 + b.home[1] * .5) * .18;
      draw(CUBE, mul(group, mul(T(p[0], p[1], p[2]), mul(RX(b.spin[0] * (1 - e)), RY(b.spin[1] * (1 - e))))), b.col);
    });
    gems.forEach(function (g, i) {
      var y = g[2] + (reduce ? 0 : Math.sin(t + i) * .35);
      draw(GEM, mul(T(g[1], y, g[3]), mul(mul(RX(reduce ? .4 : t * (.4 + i * .1)), RY(reduce ? .6 : t * .7)), S(g[4]))), g[0]);
    });
    settled = all;
    if (!reduce && visible) requestAnimationFrame(frame);
  }
  function kick() { if (reduce) requestAnimationFrame(frame); }

  cv.addEventListener('pointerdown', function (e) { drag = { x: e.clientX, y: e.clientY }; cv.setPointerCapture(e.pointerId); });
  cv.addEventListener('pointermove', function (e) {
    var r = cv.getBoundingClientRect();
    tiltY = ((e.clientX - r.left) / r.width - .5) * .5;
    tiltX = ((e.clientY - r.top) / r.height - .5) * .35;
    if (drag) { vel = (e.clientX - drag.x) * .004; rotY += vel; rotX += (e.clientY - drag.y) * .003; drag = { x: e.clientX, y: e.clientY }; }
    kick();
  });
  cv.addEventListener('pointerup', function () { drag = null; });
  cv.addEventListener('pointercancel', function () { drag = null; });
  addEventListener('resize', function () { size(); kick(); });
  // Stop drawing while it is off screen or the tab is hidden.
  new IntersectionObserver(function (es) {
    var was = visible;
    visible = es[0].isIntersecting && !document.hidden;
    if (visible && !was && !reduce) requestAnimationFrame(frame);
  }).observe(cv);
  document.addEventListener('visibilitychange', function () {
    var was = visible;
    visible = !document.hidden;
    if (visible && !was && !reduce) requestAnimationFrame(frame);
  });
  size();
  cv.classList.add('on');
  requestAnimationFrame(frame);
  window.NHHero3D = { settled: function () { return settled; } };
})();
