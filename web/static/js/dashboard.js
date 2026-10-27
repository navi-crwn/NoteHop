// "My notes": the server returns opaque blobs; this browser's vault key opens them.
(function () {
  'use strict';
  var $ = NH.$, h = NH.h;
  var ul = $('#mine'), filter = $('#filter'), rows = [];
  var vault = NHC.vault();
  if (!vault) $('#vault-locked').classList.remove('hide');

  // ---------- smart groups ----------
  // Groups that need the note text (to-dos, links, code) decrypt each note in
  // this browser with its admin link, which doesn't use up burn views.
  var group = 'all';
  // Notes untouched for 90 days are archived: out of "All", in "Archived".
  function archived(r) { return Math.max(r.n.updated, r.n.last_open || 0) < Date.now() / 1000 - 90 * 86400; }
  var GROUPS = {
    all: function (r) { return !archived(r); },
    archived: archived,
    todo: function (r) { return r.text != null && /^\s*(?:[-*+]|\d+[.)])\s+\[ \]/m.test(r.text); },
    links: function (r) { return r.text != null && /https?:\/\//.test(r.text); },
    code: function (r) { return r.text != null && /^\s*(```|~~~)/m.test(r.text); },
    timed: function (r) { return !!(r.n.expires || r.n.max_views); },
    snapshots: function (r) { return !!r.n.frozen; }
  };
  var needsText = { todo: 1, links: 1, code: 1 };
  function noteText(r) {
    if (r.text !== undefined || !r.k) return Promise.resolve(r.text);
    var path = '/api/n/' + encodeURIComponent(r.n.token), hdr = r.a ? { 'X-Admin': r.a } : {};
    return NH.api('GET', path, undefined, hdr).then(function (meta) {
      return NHC.noteKeys(r.k, r.p || '', meta.salt).then(function (keys) {
        hdr['X-Write'] = keys.write;
        return NH.api('POST', path + '/open', { poll: !!(meta.max_views && !meta.admin) }, hdr).then(function (d) {
          return NHC.codec(meta.encrypted, keys.aes).decode(d.content);
        });
      });
    }).then(function (v) { r.text = v.b || ''; return r.text; }, function () { r.text = null; return null; });
  }
  // A few at a time, so a big account doesn't flood the server.
  function loadTexts(list) {
    var queue = list.filter(function (r) { return r.text === undefined && r.k; }), running = 0;
    return new Promise(function (done) {
      (function next() {
        if (!queue.length && !running) return done();
        while (running < 4 && queue.length) {
          running++;
          noteText(queue.shift()).then(function () { running--; next(); });
        }
      })();
    });
  }
  NH.$$('#groups [data-group]').forEach(function (b) {
    b.addEventListener('click', function () {
      group = b.dataset.group;
      NH.$$('#groups [data-group]').forEach(function (x) { x.setAttribute('aria-pressed', x === b); });
      if (needsText[group]) {
        ul.replaceChildren(h('li.muted', NH.t('dash.analyzing')));
        loadTexts(rows).then(draw);
      } else draw();
    });
  });

  // ---------- graph of [[wiki links]] ----------
  $('#graph-btn').addEventListener('click', function () {
    var dlg = $('#graph-dlg'), box = $('#graph-box');
    box.replaceChildren(h('p.muted', NH.t('dash.analyzing')));
    dlg.showModal();
    loadTexts(rows).then(function () { drawGraph(box); });
  });
  function drawGraph(box) {
    var nodes = rows.filter(function (r) { return r.k; }), index = {};
    nodes.forEach(function (r, i) { r.gi = i; if (r.title) index[r.title.toLowerCase()] = r; });
    var edges = [];
    nodes.forEach(function (r) {
      ((r.text || '').match(/\[\[([^\[\]\n]{1,120})\]\]/g) || []).forEach(function (m) {
        var t = index[m.slice(2, -2).trim().toLowerCase()];
        if (t && t !== r) edges.push([r.gi, t.gi]);
      });
    });
    if (!edges.length) return box.replaceChildren(h('p.muted', NH.t('dash.graph_empty')));
    // A small force layout: repel all pairs, pull linked ones together.
    var W = 640, H = 420, P = nodes.map(function (r, i) {
      var a = 2 * Math.PI * i / nodes.length;
      return { x: W / 2 + 150 * Math.cos(a), y: H / 2 + 150 * Math.sin(a), dx: 0, dy: 0 };
    });
    for (var it = 0; it < 300; it++) {
      P.forEach(function (p) { p.dx = (W / 2 - p.x) * 0.01; p.dy = (H / 2 - p.y) * 0.01; });
      for (var i = 0; i < P.length; i++) for (var j = i + 1; j < P.length; j++) {
        var dx = P[i].x - P[j].x, dy = P[i].y - P[j].y, d2 = dx * dx + dy * dy + 0.01, f = 900 / d2;
        P[i].dx += dx * f; P[i].dy += dy * f; P[j].dx -= dx * f; P[j].dy -= dy * f;
      }
      edges.forEach(function (e) {
        var a = P[e[0]], b = P[e[1]], dx = b.x - a.x, dy = b.y - a.y;
        a.dx += dx * 0.02; a.dy += dy * 0.02; b.dx -= dx * 0.02; b.dy -= dy * 0.02;
      });
      P.forEach(function (p) { p.x = Math.max(20, Math.min(W - 20, p.x + Math.max(-8, Math.min(8, p.dx)))); p.y = Math.max(20, Math.min(H - 20, p.y + Math.max(-8, Math.min(8, p.dy)))); });
    }
    var NS = 'http://www.w3.org/2000/svg', svg = document.createElementNS(NS, 'svg');
    svg.setAttribute('viewBox', '0 0 ' + W + ' ' + H);
    svg.setAttribute('class', 'graph');
    edges.forEach(function (e) {
      var l = document.createElementNS(NS, 'line');
      l.setAttribute('x1', P[e[0]].x); l.setAttribute('y1', P[e[0]].y); l.setAttribute('x2', P[e[1]].x); l.setAttribute('y2', P[e[1]].y);
      svg.appendChild(l);
    });
    nodes.forEach(function (r, i) {
      var a = document.createElementNS(NS, 'a'), c = document.createElementNS(NS, 'circle'), t = document.createElementNS(NS, 'text');
      a.setAttribute('href', NH.noteURL(r.n.token, { k: r.k, a: r.a }));
      c.setAttribute('cx', P[i].x); c.setAttribute('cy', P[i].y); c.setAttribute('r', 7);
      t.setAttribute('x', P[i].x); t.setAttribute('y', P[i].y - 12);
      t.textContent = r.title || NH.t('common.untitled');
      a.append(c, t);
      svg.appendChild(a);
    });
    box.replaceChildren(svg);
  }

  function draw() {
    var q = filter.value.trim().toLowerCase();
    var shown = rows.filter(function (r) { return GROUPS[group](r) && (!q || (r.title || '').toLowerCase().indexOf(q) !== -1); });
    if (!rows.length) {
      ul.replaceChildren(h('li.empty', h('p.muted', NH.t('dash.empty')), h('a.btn.primary', { href: '/' }, NH.icon('plus'), NH.t('dash.write_first'))));
      return;
    }
    ul.replaceChildren.apply(ul, shown.map(function (r) {
      var n = r.n, info = NH.modeInfo[n.mode];
      var meta = [info.label, NH.t('meta.changed', { ago: NH.ago(n.updated) })];
      if (n.frozen) meta.unshift(NH.t('meta.snapshot'));
      if (n.expires) meta.push(NH.t('meta.disappears', { ago: NH.ago(n.expires) }));
      if (n.max_views) meta.push(NH.t('meta.opened', { n: n.views, max: n.max_views }));
      var href = r.k ? NH.noteURL(n.token, { k: r.k, a: r.a }) : null;
      var link = h(href ? 'a.note-link' : 'div.note-link', href ? { href: href } : null,
        h('b', r.locked ? NH.t('dash.locked') : (r.title || NH.t('common.untitled'))), h('small.muted', '/n/' + n.token + ' · ' + meta.join(' · ')));
      if (href && r.p) link.addEventListener('click', function () {
        try { sessionStorage.setItem('nh_pw_' + n.token, r.p); } catch (x) {}
      });
      var acts = h('span.btn-row');
      if (r.k) {
        var copy = h('button.btn.sm', { type: 'button', title: NH.t('dash.copy_link', { mode: info.label }) }, NH.icon('copy'), NH.t('dash.link'));
        copy.addEventListener('click', function () { NH.copy(NH.noteURL(n.token, { k: r.k }), NH.t('js.link_copied')); });
        acts.appendChild(copy);
      }
      var del = h('button.icon-btn.sm', { type: 'button', title: NH.t('common.delete'), 'aria-label': NH.t('dash.delete_note') }, NH.icon('trash'));
      del.addEventListener('click', function () {
        if (!confirm(NH.t('dash.confirm_delete', { title: r.title || NH.t('common.untitled') }))) return;
        NH.api('DELETE', '/api/n/' + encodeURIComponent(n.token), undefined, r.a ? { 'X-Admin': r.a } : {}).then(function () {
          rows = rows.filter(function (x) { return x !== r; });
          NH.recent.remove(n.token);
          draw();
        }, function (x) { NH.toast(x.message, 'err'); });
      });
      acts.appendChild(del);
      return h('li.note-row', h('span.mode-ico', { title: info.label }, NH.icon(n.frozen ? 'snow' : info.icon)), link, acts);
    }));
  }

  NH.api('GET', '/api/me/notes').then(function (res) {
    return Promise.all(res.notes.map(function (n) {
      if (!vault || !n.blob) return { n: n, locked: true };
      return vault.open(n.blob).then(function (v) { return { n: n, title: v.t, k: v.k, a: v.a, p: v.p }; },
        function () { return { n: n, locked: true }; });
    }));
  }).then(function (r) { rows = r; draw(); offerClaim(); }, function (x) {
    ul.replaceChildren(h('li.muted', x.message));
  });
  filter.addEventListener('input', draw);

  // ---------- backup: export / import (keep the file in your own storage) ----------
  // Decrypted here; the file holds the notes' text and links, so keep it safe.
  $('#export-btn').addEventListener('click', function () {
    var btn = this, list = rows.filter(function (r) { return r.k; });
    if (!list.length) return NH.toast(NH.t('dash.export_none'));
    btn.disabled = true;
    NH.toast(NH.t('dash.analyzing'));
    loadTexts(list).then(function () {
      var notes = list.filter(function (r) { return r.text != null; }).map(function (r) {
        return { title: r.title || '', body: r.text, created: r.n.created, updated: r.n.updated,
          link: NH.noteURL(r.n.token, { k: r.k }), admin: r.a ? NH.noteURL(r.n.token, { k: r.k, a: r.a }) : '' };
      });
      var blob = new Blob([JSON.stringify({ format: 'notehop-export', version: 1, exported: Math.floor(Date.now() / 1000), notes: notes }, null, 1)], { type: 'application/json' });
      var a = h('a', { href: URL.createObjectURL(blob), download: 'notehop-' + new Date().toISOString().slice(0, 10) + '.json' });
      document.body.appendChild(a);
      a.click();
      a.remove();
      NH.toast(NH.t('dash.exported', { n: notes.length }), 'ok');
    }).finally(function () { btn.disabled = false; });
  });
  $('#import-file').addEventListener('change', function () {
    var file = this.files[0];
    this.value = '';
    if (!file) return;
    file.text().then(function (txt) {
      var data = JSON.parse(txt);
      if (!data || data.format !== 'notehop-export' || !Array.isArray(data.notes)) throw new Error(NH.t('dash.import_bad'));
      if (!confirm(NH.t('dash.import_confirm', { n: data.notes.length }))) return;
      // One at a time: new notes, new keys, encrypted here like any new note.
      return data.notes.slice(0, 500).reduce(function (p, n) {
        return p.then(function (count) {
          var k = NHC.newKey(), admin = NHC.newKey(), title = String(n.title || '').slice(0, 200);
          return NHC.noteKeys(k, '', '').then(function (keys) {
            return Promise.all([NHC.codec(true, keys.aes).encode({ t: title, b: String(n.body || '') }), vault ? vault.seal({ t: title, k: k, a: admin, p: '' }) : '']).then(function (r) {
              return NH.api('POST', '/api/notes', { encrypted: true, password: false, salt: '', content: r[0], mode: 'read', admin: admin, write: keys.write, owner_blob: r[1] });
            });
          }).then(function () { return count + 1; }, function () { return count; });
        });
      }, Promise.resolve(0)).then(function (count) {
        NH.toast(NH.t('dash.imported', { n: count }), 'ok');
        setTimeout(function () { location.reload(); }, 900);
      });
    }).catch(function (x) { NH.toast(x.message || NH.t('dash.import_bad'), 'err'); });
  });

  // ---------- claim notes made on this device before signing in ----------
  function offerClaim() {
    if (!vault) return;
    var mine = {};
    rows.forEach(function (r) { mine[r.n.token] = true; });
    var loose = NH.recent.all().filter(function (n) { return n.a && !mine[n.token] && !(n.expires && n.expires < Date.now() / 1000); });
    var box = $('#claim');
    box.classList.toggle('hide', !loose.length);
    if (!loose.length) return;
    var btn = h('button.btn.sm.primary', { type: 'button' }, NH.icon('plus'), NH.t('dash.claim_btn'));
    box.replaceChildren(NH.icon('key'), h('span', NH.t('dash.claim', { n: loose.length })), btn);
    btn.addEventListener('click', function () {
      btn.disabled = true;
      Promise.all(loose.map(function (n) {
        return vault.seal({ t: n.t || '', k: n.k, a: n.a, p: '' }).then(function (blob) {
          return NH.api('POST', '/api/n/' + encodeURIComponent(n.token) + '/claim', { owner_blob: blob }, { 'X-Admin': n.a }).then(function () { return 1; }, function () { return 0; });
        });
      })).then(function (res) {
        var n = res.reduce(function (a, b) { return a + b; }, 0);
        NH.toast(NH.t('dash.claimed', { n: n }), 'ok');
        setTimeout(function () { location.reload(); }, 800);
      });
    });
  }
})();
