// Line diff (Myers' O(ND) algorithm) for history, suggestions and the diff
// page. Runs in the browser on decrypted text; the server never sees it.
//   NHDiff.lines(a, b) → [{op: '=', '+' or '-', text}]
//   NHDiff.render(el, ops, {context: 3})
(function () {
  'use strict';

  function split(s) { return s === '' ? [] : String(s).replace(/\r\n?/g, '\n').split('\n'); }

  function lines(a, b) {
    var A = split(a), B = split(b), N = A.length, M = B.length, max = N + M;
    // Strip the common head and tail first: most edits are small.
    var head = 0;
    while (head < N && head < M && A[head] === B[head]) head++;
    var tail = 0;
    while (tail < N - head && tail < M - head && A[N - 1 - tail] === B[M - 1 - tail]) tail++;
    var a2 = A.slice(head, N - tail), b2 = B.slice(head, M - tail);
    var mid = myers(a2, b2);
    var out = [];
    A.slice(0, head).forEach(function (t) { out.push({ op: '=', text: t }); });
    out = out.concat(mid);
    A.slice(N - tail).forEach(function (t) { out.push({ op: '=', text: t }); });
    return out;
  }

  function myers(A, B) {
    var N = A.length, M = B.length, max = N + M;
    if (!N) return B.map(function (t) { return { op: '+', text: t }; });
    if (!M) return A.map(function (t) { return { op: '-', text: t }; });
    // Very large changes: fall back to "all removed, all added".
    if (N * M > 4e7) return A.map(function (t) { return { op: '-', text: t }; }).concat(B.map(function (t) { return { op: '+', text: t }; }));
    var V = new Int32Array(2 * max + 1), off = max, trace = [];
    for (var d = 0; d <= max; d++) {
      trace.push(V.slice());
      for (var k = -d; k <= d; k += 2) {
        var x = (k === -d || (k !== d && V[off + k - 1] < V[off + k + 1])) ? V[off + k + 1] : V[off + k - 1] + 1;
        var y = x - k;
        while (x < N && y < M && A[x] === B[y]) { x++; y++; }
        V[off + k] = x;
        if (x >= N && y >= M) return backtrack(trace, A, B, off, d);
      }
    }
    return [];
  }

  function backtrack(trace, A, B, off, D) {
    var x = A.length, y = B.length, out = [];
    for (var d = D; d > 0; d--) {
      var V = trace[d], k = x - y;
      var prevK = (k === -d || (k !== d && V[off + k - 1] < V[off + k + 1])) ? k + 1 : k - 1;
      var px = V[off + prevK], py = px - prevK;
      while (x > px && y > py) { out.push({ op: '=', text: A[x - 1] }); x--; y--; }
      if (x === px) out.push({ op: '+', text: B[y - 1] }); else out.push({ op: '-', text: A[x - 1] });
      x = px; y = py;
    }
    while (x > 0 && y > 0) { out.push({ op: '=', text: A[x - 1] }); x--; y--; }
    return out.reverse();
  }

  function stats(ops) {
    var s = { added: 0, removed: 0 };
    ops.forEach(function (o) { if (o.op === '+') s.added++; else if (o.op === '-') s.removed++; });
    return s;
  }

  // Unchanged runs longer than 2×context are folded.
  function render(el, ops, opts) {
    var ctx = (opts && opts.context != null) ? opts.context : 3, rows = [];
    var keep = ops.map(function () { return false; });
    ops.forEach(function (o, i) {
      if (o.op === '=') return;
      for (var j = Math.max(0, i - ctx); j <= Math.min(ops.length - 1, i + ctx); j++) keep[j] = true;
    });
    var folded = 0;
    function flush() { if (folded) { rows.push(NH.h('div.diff-fold', NH.t('diff.folded', { n: folded }))); folded = 0; } }
    ops.forEach(function (o, i) {
      if (!keep[i] && o.op === '=') { folded++; return; }
      flush();
      rows.push(NH.h('div.diff-line.diff-' + (o.op === '+' ? 'add' : o.op === '-' ? 'del' : 'same'),
        NH.h('span.diff-sign', o.op === '=' ? ' ' : o.op), NH.h('span.diff-text', o.text || ' ')));
    });
    flush();
    if (!ops.some(function (o) { return o.op !== '='; })) rows = [NH.h('p.muted', NH.t('diff.same'))];
    el.replaceChildren.apply(el, rows);
    el.classList.add('diff');
  }

  // Unified-diff text, for sharing as a ```diff block.
  function text(ops) {
    return ops.map(function (o) { return (o.op === '=' ? ' ' : o.op) + ' ' + o.text; }).join('\n');
  }

  window.NHDiff = { lines: lines, render: render, stats: stats, text: text };
})();
