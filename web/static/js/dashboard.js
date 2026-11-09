// "My notes": the server returns opaque blobs; this browser's vault key opens them.
// Every row is in one of four states, and the page says which one:
//   open      the vault opened the blob: title, key and owner link are known;
//   closed    the vault is not open in this browser (the blob is there);
//   mismatch  the vault is open but doesn't open the blob (another key);
//   nokey     the blob is empty: the note was made while the vault was closed.
// A basic note is named and linked from the server in every state, because the
// server can read it anyway. An encrypted note has no title without the key.
(function () {
  'use strict';
  var $ = NH.$, h = NH.h;
  var ul = $('#mine'), filter = $('#filter'), rows = [];
  var vault = NHC.vault();
  var sso = document.body.dataset.sso === '1';
  var unlockHref = (sso ? '/vault' : '/login') + '?next=/dashboard';
  function now() { return Date.now() / 1000; }
  function reload() { setTimeout(function () { location.reload(); }, 800); }
  function lockAgain() { NH.store.del('nh_vault'); location.reload(); }
  function enterAgain() { NH.store.del('nh_vault'); location.href = unlockHref; }

  // ---------- what the page says about the vault ----------
  $('#dash-lead').textContent = NH.t(vault ? 'dash.lead' : 'dash.lead_locked');
  (function vaultState() {
    var box = $('#vault-state');
    if (vault) {
      var lock = h('button.btn.sm.ghost', { type: 'button' }, NH.icon('lock'), NH.t('dash.vault_lock'));
      lock.addEventListener('click', lockAgain);
      box.replaceChildren(NH.icon('unlock'), h('span', NH.t('dash.vault_open')), lock);
    } else {
      box.replaceChildren(NH.icon('lock'), h('span', NH.t('dash.vault_closed')),
        h('a.btn.sm.primary', { href: unlockHref }, NH.icon('key'), NH.t('dash.vault_unlock')));
      $('#vault-locked').classList.remove('hide');
    }
  })();

  // The name a row is shown and searched by: from the vault, else from the
  // server (basic notes), else nothing.
  function name(r) { return r.title || r.n.title || ''; }
  function shownName(r) {
    if (name(r)) return name(r);
    if (!r.n.encrypted) return '/' + r.n.token;
    return r.locked ? NH.t('dash.title_' + r.why) : NH.t('common.untitled');
  }
  function lockedTitles() { return rows.filter(function (r) { return r.locked && r.n.encrypted; }).length; }

  // ---------- smart groups ----------
  // Groups that need the note text (to-dos, links, code) decrypt each note in
  // this browser with its admin link, which doesn't use up burn views.
  var group = 'all';
  // Notes untouched for 90 days are archived: out of "All", in "Archived".
  function archived(r) { return Math.max(r.n.updated, r.n.last_open || 0) < now() - 90 * 86400; }
  var GROUPS = {
    all: function (r) { return !archived(r); },
    archived: archived,
    todo: function (r) { return r.text != null && /^\s*(?:[-*+]|\d+[.)])\s+\[ \]/m.test(r.text); },
    links: function (r) { return r.text != null && /https?:\/\//.test(r.text); },
    code: function (r) { return r.text != null && /^\s*(```|~~~)/m.test(r.text); },
    timed: function (r) { return !!(r.n.expires || r.n.max_views); },
    snapshots: function (r) { return !!r.n.frozen; },
    basic: function (r) { return !r.n.encrypted && !archived(r); },
    encrypted: function (r) { return !!r.n.encrypted && !archived(r); }
  };
  var needsText = { todo: 1, links: 1, code: 1 };
  // A note's text: with its key as the owner (no view spent); a basic note
  // without the key as a plain visitor would read it, but as a refresh (poll)
  // so it costs no view either. A burn-after-reading note refuses that and
  // stays unread here. An encrypted note without its key can't be read at all.
  function readable(r) { return !!r.k || !r.n.encrypted; }
  function noteText(r) {
    if (r.text !== undefined) return Promise.resolve(r.text);
    if (!readable(r)) { r.text = null; return Promise.resolve(null); }
    var path = '/api/n/' + encodeURIComponent(r.n.token), hdr = r.a ? { 'X-Admin': r.a } : {};
    return NH.api('GET', path, undefined, hdr).then(function (meta) {
      if (!r.k) return NH.api('POST', path + '/open', { poll: true }).then(function (d) { return NHC.codec(false).decode(d.content); });
      return NHC.noteKeys(r.k, r.p || '', meta.salt).then(function (keys) {
        hdr['X-Write'] = keys.write;
        return NH.api('POST', path + '/open', { poll: !!(meta.max_views && !meta.admin) }, hdr).then(function (d) {
          return NHC.codec(meta.encrypted, keys.aes).decode(d.content);
        });
      });
    }).then(function (v) { r.text = v.b || ''; r.type = v.y ? { y: v.y, l: v.l } : null; return r.text; }, function () { r.text = null; return null; });
  }
  // A few at a time, so a big account doesn't flood the server.
  function loadTexts(list) {
    var queue = list.filter(function (r) { return r.text === undefined && readable(r); }), running = 0;
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
    var nodes = rows.filter(function (r) { return r.text != null; }), index = {};
    nodes.forEach(function (r, i) { r.gi = i; if (name(r)) index[name(r).toLowerCase()] = r; });
    var edges = [];
    nodes.forEach(function (r) {
      ((r.text || '').match(/\[\[([^\[\]\n]{1,120})\]\]/g) || []).forEach(function (m) {
        var t = index[m.slice(2, -2).trim().toLowerCase()];
        if (t && t !== r) edges.push([r.gi, t.gi]);
      });
    });
    if (!edges.length) {
      var kids = [h('p.muted', NH.t('dash.graph_empty'))], locked = lockedTitles();
      if (locked) kids.push(h('p.muted.small', NH.t('dash.graph_locked', { n: locked })));
      return box.replaceChildren.apply(box, kids);
    }
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
      a.setAttribute('href', rowHref(r));
      c.setAttribute('cx', P[i].x); c.setAttribute('cy', P[i].y); c.setAttribute('r', 7);
      t.setAttribute('x', P[i].x); t.setAttribute('y', P[i].y - 12);
      t.textContent = shownName(r);
      a.append(c, t);
      svg.appendChild(a);
    });
    box.replaceChildren(svg);
  }

  // The owner link when the key is known; a basic note's plain address otherwise.
  function rowHref(r) {
    if (r.k) return NH.noteURL(r.n.token, { k: r.k, a: r.a });
    return r.n.encrypted ? null : NH.noteURL(r.n.token, {});
  }

  function draw() {
    if (group === 'device') return drawDevice();
    var q = filter.value.trim().toLowerCase();
    if (!rows.length) {
      ul.replaceChildren(h('li.empty', h('p.muted', NH.t('dash.empty')), h('a.btn.primary', { href: '/new' }, NH.icon('plus'), NH.t('dash.write_first'))));
      return;
    }
    var shown = rows.filter(function (r) {
      return GROUPS[group](r) && (!q || shownName(r).toLowerCase().indexOf(q) !== -1 || r.n.token.toLowerCase().indexOf(q) !== -1);
    });
    if (!shown.length) {
      // Never an empty list without a word: what matched nothing, and whether locked titles are the reason.
      var kids = [h('p.muted', q ? NH.t('dash.no_results', { q: filter.value.trim() }) : NH.t('dash.group_empty'))], locked = lockedTitles();
      if (locked && (q || needsText[group])) {
        kids.push(h('p.muted.small', NH.t(q ? 'dash.search_locked' : 'dash.group_locked', { n: locked })));
        if (!vault) kids.push(h('a.btn.sm', { href: unlockHref }, NH.icon('key'), NH.t('dash.vault_unlock')));
      }
      ul.replaceChildren(h('li.empty', kids));
      return;
    }
    ul.replaceChildren.apply(ul, shown.map(function (r) {
      var n = r.n, info = NH.modeInfo[n.mode];
      var meta = [NH.t(n.encrypted ? 'pill.encrypted' : 'pill.public'), info.label, NH.t('meta.changed', { ago: NH.ago(n.updated) })];
      if (n.frozen) meta.unshift(NH.t('meta.snapshot'));
      if (n.expires) meta.push(NH.t('meta.disappears', { ago: NH.ago(n.expires) }));
      if (n.max_views) meta.push(NH.t('meta.opened', { n: n.views, max: n.max_views }));
      if (r.locked) meta.push(NH.t('dash.state_' + r.why));
      var href = rowHref(r);
      var title = h('b' + (r.locked && n.encrypted ? '.locked-title' : ''), shownName(r));
      var link = h(href ? 'a.note-link' : 'div.note-link', href ? { href: href } : null, title, h('small.muted', '/' + n.token + ' · ' + meta.join(' · ')));
      if (href && r.p) link.addEventListener('click', function () {
        try { sessionStorage.setItem('nh_pw_' + n.token, r.p); } catch (x) {}
      });
      var acts = h('span.btn-row');
      if (href) {
        var copy = h('button.btn.sm', { type: 'button', title: NH.t('dash.copy_link', { mode: info.label }) }, NH.icon('copy'), NH.t('dash.link'));
        // A basic note's read link is just its address; the key matters for encrypted notes and for writing.
        copy.addEventListener('click', function () {
          var keyed = !!r.k && (n.encrypted || n.mode !== 'read');
          NH.copy(NH.noteURL(n.token, keyed ? { k: r.k } : {}), NH.t('js.link_copied'));
          if (keyed) NH.toast(NH.t('copy.key_hint'));
        });
        acts.appendChild(copy);
      }
      var del = h('button.icon-btn.sm', { type: 'button', title: NH.t('common.delete'), 'aria-label': NH.t('dash.delete_note') }, NH.icon('trash'));
      del.addEventListener('click', function () {
        if (!confirm(NH.t('dash.confirm_delete', { title: name(r) || '/' + n.token }))) return;
        NH.api('DELETE', '/api/n/' + encodeURIComponent(n.token), undefined, r.a ? { 'X-Admin': r.a } : {}).then(function () {
          rows = rows.filter(function (x) { return x !== r; });
          NH.recent.remove(n.token);
          NH.toast(NH.t('dash.deleted'), 'ok');
          draw();
        }, function (x) { NH.toast(x.message, 'err'); });
      });
      acts.appendChild(del);
      return h('li.note-row', h('span.mode-ico', { title: info.label }, NH.icon(n.frozen ? 'snow' : info.icon)), link, acts);
    }));
  }

  // "On this device": the owner links this browser remembers, which is where a
  // note made without signing in (or before the vault was open) lives until it
  // is added to the account.
  function drawDevice() {
    var mine = {};
    rows.forEach(function (r) { mine[r.n.token] = true; });
    var list = NH.recent.all(), kids = [h('li.muted.small.device-hint', NH.t('dash.device_hint'))];
    if (!list.length) kids.push(h('li.empty', h('p.muted', NH.t('dash.device_empty'))));
    list.forEach(function (n) {
      var gone = n.expires && n.expires < now(), inAccount = !!mine[n.token];
      var meta = [NH.ago(n.created)];
      if (gone) meta.push(NH.t('new.expired'));
      if (inAccount) meta.push(NH.t('dash.in_account'));
      if (n.p) meta.push(NH.t('new.has_password'));
      var acts = h('span.btn-row');
      if (vault && n.a && !inAccount && !gone) {
        var add = h('button.btn.sm.primary', { type: 'button' }, NH.icon('plus'), NH.t('dash.claim_one'));
        add.addEventListener('click', function () { add.disabled = true; claimAll([n]).then(afterClaim); });
        acts.appendChild(add);
      }
      var rm = h('button.icon-btn.sm', { type: 'button', title: NH.t('new.forget_one'), 'aria-label': NH.t('new.forget_one') }, NH.icon('x'));
      rm.addEventListener('click', function () { NH.recent.remove(n.token); drawDevice(); offerClaim(); });
      acts.appendChild(rm);
      kids.push(h('li.note-row' + (gone ? '.gone' : ''), h('span.mode-ico', NH.icon('phone')),
        h('a.note-link', { href: NH.noteURL(n.token, { k: n.k, a: n.a }) }, h('b', n.t || NH.t('common.untitled')), h('small.muted', '/' + n.token + ' · ' + meta.join(' · '))),
        acts));
    });
    ul.replaceChildren.apply(ul, kids);
  }

  NH.api('GET', '/api/me/notes').then(function (res) {
    return Promise.all(res.notes.map(function (n) {
      if (!n.blob) return { n: n, locked: true, why: 'nokey' };
      if (!vault) return { n: n, locked: true, why: 'closed' };
      return vault.open(n.blob).then(function (v) { return { n: n, title: v.t, k: v.k, a: v.a, p: v.p }; },
        function () { return { n: n, locked: true, why: 'mismatch' }; });
    }));
  }).then(function (r) {
    rows = r;
    // A vault that opens none of the blobs it should: the key in this browser
    // is not the one these notes were sealed with. Say so instead of staying quiet.
    var sealed = rows.filter(function (x) { return x.n.blob; });
    if (vault && sealed.length && sealed.every(function (x) { return x.why === 'mismatch'; })) {
      var again = h('button.btn.sm', { type: 'button' }, NH.icon('key'), NH.t('dash.vault_enter_again'));
      again.addEventListener('click', enterAgain);
      $('#vault-mismatch').replaceChildren(NH.icon('lock'), h('span', NH.t('dash.vault_mismatch')), again);
      $('#vault-mismatch').classList.remove('hide');
    }
    draw();
    offerClaim();
    offerReseal();
  }, function (x) {
    ul.replaceChildren(h('li.muted', x.message));
  });
  filter.addEventListener('input', draw);

  // ---------- backup: export / import (keep the file in your own storage) ----------
  // Decrypted here; the file holds the notes' text and links, so keep it safe.
  $('#export-btn').addEventListener('click', function () {
    var btn = this, list = rows.filter(readable), locked = rows.length - list.length;
    if (!list.length) return NH.toast(locked ? NH.t('dash.export_locked', { n: locked }) : NH.t('dash.export_none'), locked ? 'err' : '');
    btn.disabled = true;
    NH.toast(NH.t('dash.analyzing'));
    loadTexts(list).then(function () {
      var notes = list.filter(function (r) { return r.text != null; }).map(function (r) {
        return { title: name(r), body: r.text, type: r.type || undefined, created: r.n.created, updated: r.n.updated,
          link: r.k ? NH.noteURL(r.n.token, { k: r.k }) : NH.noteURL(r.n.token, {}), admin: r.a ? NH.noteURL(r.n.token, { k: r.k, a: r.a }) : '' };
      });
      var skipped = rows.length - notes.length;
      if (!notes.length) return NH.toast(NH.t('dash.export_locked', { n: skipped }), 'err');
      var blob = new Blob([JSON.stringify({ format: 'notehop-export', version: 1, exported: Math.floor(now()), notes: notes }, null, 1)], { type: 'application/json' });
      var a = h('a', { href: URL.createObjectURL(blob), download: 'notehop-' + new Date().toISOString().slice(0, 10) + '.json' });
      document.body.appendChild(a);
      a.click();
      a.remove();
      NH.toast(NH.t('dash.exported', { n: notes.length }), 'ok');
      // Never a silent subset: say how many rows the file does not have.
      if (skipped) NH.toast(NH.t('dash.export_skipped', { n: skipped }));
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
            // A note type from the backup is kept only if it is one we know.
            var y = n.type && (n.type.y === 'plain' || n.type.y === 'code') ? { y: n.type.y, l: String(n.type.l || '').slice(0, 32) } : {};
            if (y.y === 'plain') delete y.l;
            return Promise.all([NHC.codec(true, keys.aes).encode(Object.assign({ t: title, b: String(n.body || '') }, y)), vault ? vault.seal({ t: title, k: k, a: admin, p: '' }) : '']).then(function (r) {
              return NH.api('POST', '/api/notes', { encrypted: true, password: false, salt: '', content: r[0], mode: 'read', admin: admin, write: keys.write, owner_blob: r[1] });
            });
          }).then(function () { return count + 1; }, function () { return count; });
        });
      }, Promise.resolve(0)).then(function (count) {
        NH.toast(NH.t('dash.imported', { n: count }), 'ok');
        reload();
      });
    }).catch(function (x) { NH.toast(x.message || NH.t('dash.import_bad'), 'err'); });
  });

  // ---------- keys this device holds for notes the account lacks ----------
  // Each note is claimed with the owner link this browser remembers. A note
  // that no longer exists (404) is forgotten here, so the offer can't come back
  // for it; nothing reloads when nothing was added.
  function claimAll(list) {
    return Promise.all(list.map(function (n) {
      return vault.seal({ t: n.t || '', k: n.k, a: n.a, p: '' }).then(function (blob) {
        return NH.api('POST', '/api/n/' + encodeURIComponent(n.token) + '/claim', { owner_blob: blob }, { 'X-Admin': n.a }).then(function () { return 'added'; },
          function (x) { if (x.status === 404) { NH.recent.remove(n.token); return 'gone'; } return 'failed'; });
      });
    })).then(function (res) {
      var out = { added: 0, gone: 0, failed: 0 };
      res.forEach(function (k) { out[k]++; });
      return out;
    });
  }
  function afterClaim(res) {
    if (res.gone) NH.toast(NH.t('dash.claim_gone', { n: res.gone }));
    if (res.failed) NH.toast(NH.t('dash.claim_failed'), 'err');
    if (res.added) { NH.toast(NH.t('dash.claimed', { n: res.added }), 'ok'); return reload(); }
    offerClaim();
    if (group === 'device') drawDevice();
  }
  // Notes made on this device before signing in: not in the account yet.
  function offerClaim() {
    var box = $('#claim');
    if (!vault) return box.classList.add('hide');
    var mine = {};
    rows.forEach(function (r) { mine[r.n.token] = true; });
    var loose = NH.recent.all().filter(function (n) { return n.a && !mine[n.token] && !(n.expires && n.expires < now()); });
    box.classList.toggle('hide', !loose.length);
    if (!loose.length) return;
    var btn = h('button.btn.sm.primary', { type: 'button' }, NH.icon('plus'), NH.t('dash.claim_btn'));
    box.replaceChildren(NH.icon('key'), h('span', NH.t('dash.claim', { n: loose.length })), btn);
    btn.addEventListener('click', function () { btn.disabled = true; claimAll(loose).then(afterClaim); });
  }
  // Notes in the account whose key never reached the vault (made while it was
  // closed): this device still has the owner link, so the key can be sealed now.
  function offerReseal() {
    var box = $('#reseal'), hint = $('#nokey');
    var rec = {};
    NH.recent.all().forEach(function (n) { if (n.a && n.k) rec[n.token] = n; });
    var nokey = rows.filter(function (r) { return r.locked && r.why === 'nokey' && r.n.encrypted; });
    var fixable = vault ? nokey.filter(function (r) { return rec[r.n.token]; }) : [];
    box.classList.toggle('hide', !fixable.length);
    hint.classList.toggle('hide', !vault || nokey.length === fixable.length);
    if (vault && nokey.length > fixable.length) hint.replaceChildren(NH.icon('key'), h('span', NH.t('dash.nokey_hint', { n: nokey.length - fixable.length })));
    if (!fixable.length) return;
    var btn = h('button.btn.sm.primary', { type: 'button' }, NH.icon('key'), NH.t('dash.reseal_btn'));
    box.replaceChildren(NH.icon('key'), h('span', NH.t('dash.reseal', { n: fixable.length })), btn);
    btn.addEventListener('click', function () {
      btn.disabled = true;
      claimAll(fixable.map(function (r) { return Object.assign({}, rec[r.n.token], { t: rec[r.n.token].t || r.n.title || '' }); })).then(function (res) {
        if (res.added) { NH.toast(NH.t('dash.resealed', { n: res.added }), 'ok'); return reload(); }
        NH.toast(NH.t('dash.claim_failed'), 'err');
        btn.disabled = false;
      });
    });
  }
})();
