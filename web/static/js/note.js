// Note page: fetch metadata, unlock with the #k key (and password), then show
// the note in the mode this link allows: edit, read, append or check.
// Link fragment: #k=<note key>[&a=<admin token>]. Neither ever reaches the server
// as part of the URL; the admin token and the derived write proof travel in headers.
(function () {
  'use strict';
  var $ = NH.$, h = NH.h;
  var token = document.body.dataset.token;
  var frag = new URLSearchParams(location.hash.slice(1));
  var K = frag.get('k'), A = frag.get('a'), isNew = frag.get('new') === '1';
  if (isNew) {
    frag.delete('new');
    history.replaceState(null, '', location.pathname + location.search + '#' + frag.toString());
  }

  var S = {
    meta: null, keys: null, codec: null, password: '',
    version: 0, title: '', body: '', checks: {}, entries: [],
    mode: 'read', dirty: false, saving: false, pending: false, ready: false
  };
  var ed = null, saveTimer = null;

  function headers() {
    var hd = {};
    if (A) hd['X-Admin'] = A;
    if (S.keys) hd['X-Write'] = S.keys.write;
    return hd;
  }
  function api(method, path, body) { return NH.api(method, '/api/n/' + encodeURIComponent(token) + path, body, headers()); }

  // ---------- state card (loading, errors, password, burn warning) ----------
  function showState(icon, title, text) {
    var box = $('#state');
    var kids = [NH.h('div.state-ico', NH.icon(icon)), h('h1', title)];
    if (text) kids.push(h('p.muted', text));
    Array.prototype.slice.call(arguments, 3).forEach(function (k) { if (k) kids.push(k); });
    box.replaceChildren.apply(box, kids);
    box.classList.remove('hide');
    $('#note').classList.add('hide');
  }
  function gone(msg) {
    showState('x', 'Catatan tidak bisa dibuka', msg, h('a.btn.primary', { href: '/' }, NH.icon('plus'), 'Tulis catatan baru'));
  }

  function start() {
    if (!NHC.available) return gone('Browser ini tidak mendukung WebCrypto (butuh HTTPS atau localhost).');
    NH.api('GET', '/api/n/' + encodeURIComponent(token), undefined, A ? { 'X-Admin': A } : {}).then(function (meta) {
      S.meta = meta;
      if (K && !/^[A-Za-z0-9_-]{43}$/.test(K)) return gone('Kunci di link rusak. Pastikan link tersalin utuh.');
      if (meta.encrypted && !K) return gone('Bagian kunci (#k=…) tidak ada di link. Minta link lengkap dari pengirimnya; kunci itu tidak pernah disimpan di server.');
      if (meta.open_at && !meta.admin) return capsule(meta.open_at);
      if (meta.password) return askPassword();
      return unlock('');
    }, function (e) { gone(e.message); });
  }

  // Time capsule: the server won't hand out the content before open_at, so
  // count down here and try again when the time comes.
  var capsuleTimer = null;
  function capsule(at) {
    clearInterval(capsuleTimer);
    var clock = h('p.countdown');
    var tick = function () {
      var left = at - Math.floor(Date.now() / 1000);
      if (left <= 0) {
        clearInterval(capsuleTimer);
        clock.textContent = 'Membuka…';
        // A few seconds of slack for clocks that disagree with the server.
        setTimeout(start, 3000);
        return;
      }
      var d = Math.floor(left / 86400), hh = Math.floor(left % 86400 / 3600), mm = Math.floor(left % 3600 / 60), ss = left % 60;
      var p2 = function (n) { return String(n).padStart(2, '0'); };
      clock.textContent = (d ? d + ' hari ' : '') + p2(hh) + ':' + p2(mm) + ':' + p2(ss);
    };
    tick();
    capsuleTimer = setInterval(tick, 1000);
    showState('clock', 'Belum waktunya dibuka', 'Catatan ini dikunci waktu sampai ' + NH.date(at) + '. Biarkan halaman ini terbuka; catatan akan terbuka sendiri.', clock);
  }

  function askPassword(errMsg) {
    var handoff = null;
    try { handoff = sessionStorage.getItem('nh_pw_' + token); sessionStorage.removeItem('nh_pw_' + token); } catch (x) {}
    if (handoff && !errMsg) return unlock(handoff).catch(function () { askPassword(''); });
    var input = h('input', { type: 'password', placeholder: 'Password', 'aria-label': 'Password catatan', autocomplete: 'off', required: true });
    var err = h('p.locked-err', errMsg || '');
    var btn = h('button.btn.primary', { type: 'submit' }, NH.icon('unlock'), 'Buka');
    var form = h('form.pw-form', input, btn);
    form.addEventListener('submit', function (e) {
      e.preventDefault();
      btn.disabled = true;
      err.textContent = 'Menurunkan kunci…';
      unlock(input.value).catch(function (x) {
        btn.disabled = false;
        err.textContent = x.status === 403 || x.name === 'OperationError' ? 'Password salah.' : x.message;
        input.select();
      });
    });
    showState('lock', 'Catatan ini dilindungi password', 'Link saja tidak cukup. Masukkan password yang diberikan pengirim.', form, err);
    input.focus();
  }

  function unlock(pw) {
    return (K ? NHC.noteKeys(K, pw, S.meta.salt) : Promise.resolve(null)).then(function (keys) {
      S.keys = keys;
      S.password = pw;
      S.codec = NHC.codec(S.meta.encrypted, keys && keys.aes);
      if (S.meta.max_views > 0 && !S.meta.admin) return confirmBurn();
      return open(false);
    });
  }

  function confirmBurn() {
    var left = S.meta.max_views - S.meta.views;
    var btn = h('button.btn.primary.lg', { type: 'button' }, NH.icon('eye'), 'Buka catatan sekarang');
    btn.addEventListener('click', function () {
      btn.disabled = true;
      // A wrong password only shows up here: burn notes can't be test-opened
      // first. The server refuses without counting a view, so ask again.
      open(false).catch(function (x) {
        if (x.status === 403 && S.meta.password) return askPassword('Password salah.');
        gone(x.message);
      });
    });
    showState('flame', left <= 1 ? 'Catatan ini hancur setelah dibuka' : 'Catatan ini punya jatah baca',
      left <= 1 ? 'Begitu dibuka, catatan langsung dihapus dari server dan link ini mati. Pastikan kamu siap membacanya sekarang.'
        : 'Catatan bisa dibuka ' + left + ' kali lagi sebelum dihapus otomatis.', btn);
  }

  // ---------- loading content ----------
  function decode(data) {
    return Promise.all([
      S.codec.decode(data.content),
      Promise.all(data.entries.map(function (e) {
        return S.codec.decode(e.content).then(function (v) { return { id: e.id, created: e.created, n: v.n || '', b: v.b || '' }; },
          function () { return { id: e.id, created: e.created, n: '', b: '', broken: true }; });
      }))
    ]).then(function (r) { return { note: r[0], entries: r[1] }; }, function () {
      var err = new Error('Kunci tidak cocok dengan catatan ini. Link mungkin terpotong.');
      err.status = 400;
      throw err;
    });
  }

  function open(poll) {
    return api('POST', '/open', poll ? { poll: true } : {}).then(function (data) {
      return decode(data).then(function (d) {
        var fresh = !S.ready;
        var bodyChanged = data.version !== S.version;
        var checksChanged = JSON.stringify(data.checks || {}) !== JSON.stringify(S.checks);
        var entriesChanged = d.entries.length !== S.entries.length;
        Object.assign(S.meta, data);
        delete S.meta.content; delete S.meta.entries; delete S.meta.checks;
        S.checks = data.checks || {};
        S.entries = d.entries;
        if (fresh || !S.dirty) {
          S.version = data.version;
          S.title = d.note.t || '';
          S.body = d.note.b || '';
        }
        if (fresh) { S.ready = true; setup(data.burned); return; }
        // The scheduled lock just passed: reload into read-only mode.
        if (S.meta.locked && !S.meta.admin && S.mode !== 'read' && !S.dirty) return location.reload();
        if ((bodyChanged || checksChanged) && !S.dirty) drawBody(true);
        if (entriesChanged) drawEntries();
        drawMeta();
      });
    }, function (x) {
      if (x.status === 425 && !poll) return start();
      if (x.status === 403 && S.meta.password && !poll) throw x;
      if (poll) { if (x.status === 404) gone('Catatan ini baru saja dihapus atau kedaluwarsa.'); return; }
      gone(x.message);
    });
  }

  // ---------- page ----------
  function setup(burned) {
    var m = S.meta;
    S.mode = m.frozen ? 'read' : m.admin ? 'edit' : m.mode;
    if (m.locked && !m.admin) S.mode = 'read';
    if (!S.keys && S.mode !== 'read') S.mode = 'read';
    $('#state').classList.add('hide');
    $('#note').classList.remove('hide');

    if (burned) banner('flame', 'Catatan ini sudah dihancurkan dari server. Yang kamu lihat sekarang adalah satu-satunya salinan. Salin isinya sekarang jika perlu.', 'warn');
    else if (m.frozen) banner('snow', 'Snapshot beku, dibuat ' + NH.date(m.created) + '. Isinya tidak bisa diubah siapa pun.', 'info',
      m.admin && m.parent ? h('a', { href: NH.noteURL(m.parent, { k: K, a: A }) }, 'Buka catatan aslinya →') : null);
    else if (m.open_at && m.admin) banner('clock', 'Kapsul waktu: pembaca baru bisa membuka catatan ini ' + NH.date(m.open_at) + '. Kamu bisa melihatnya karena memakai link admin.', 'info');
    else if (m.locked && !m.admin && m.mode !== 'read') banner('lock', 'Catatan ini dikunci sejak ' + NH.date(m.lock_at) + '. Sekarang hanya bisa dibaca.', 'info');
    else if (!S.keys && m.mode !== 'read') banner('key', 'Link ini tanpa kunci (#k), jadi hanya bisa membaca.', 'info');

    NH.$$('[data-admin]').forEach(function (b) { b.classList.toggle('hide', !m.admin); });
    if (m.frozen) $('#act-freeze').classList.add('hide');
    $('#act-delete').classList.toggle('hide', !(m.admin || m.owned));
    if (!m.encrypted) { var raw = $('#act-raw'); raw.href = '/raw/' + encodeURIComponent(token); raw.classList.remove('hide'); }
    if (burned) NH.$$('#n-actions .btn').forEach(function (b) { if (b.id !== 'act-copy') b.classList.add('hide'); });

    if (S.mode === 'edit') {
      $('#n-view').classList.add('hide');
      $('#n-edit').classList.remove('hide');
      $('#n-title').classList.add('hide');
      var ti = $('#n-title-input');
      ti.classList.remove('hide');
      ed = NHEditor($('#n-edit [data-editor]'), { onChange: edited, onSave: saveNow });
      ti.addEventListener('input', edited);
    }
    drawBody(true);
    drawMeta();
    drawEntries();

    $('#act-copy').addEventListener('click', function () {
      var t = currentTitle(), b = NHMD.applyChecks(currentBody(), S.mode === 'edit' ? {} : S.checks);
      NH.copy((t ? '# ' + t + '\n\n' : '') + b, 'Teks catatan disalin');
    });
    $('#act-share').addEventListener('click', openShare);
    $('#act-freeze').addEventListener('click', freeze);
    $('#act-delete').addEventListener('click', removeNote);

    if (isNew && m.admin) { openShare(); }
    if (!(m.max_views > 0 && !m.admin) && !m.frozen && !burned) setInterval(poll, 12000);
  }

  function banner(icon, text, kind, extra) {
    var b = $('#banner');
    b.className = 'banner ' + (kind || '');
    b.replaceChildren(NH.icon(icon), h('span', text), extra || '');
  }

  function currentTitle() { return ed ? $('#n-title-input').value.trim() : S.title; }
  function currentBody() { return ed ? ed.value : S.body; }

  function drawBody(force) {
    var title = S.title || 'Tanpa judul';
    document.title = title + ' · NoteHop';
    $('#n-title').textContent = title;
    if (S.mode === 'edit') {
      if (force && !S.dirty) {
        $('#n-title-input').value = S.title;
        // Checklist ticks made through "check" links are folded into the text;
        // the next save makes them part of the note and clears the overrides.
        ed.value = NHMD.applyChecks(S.body, S.checks);
      }
      return;
    }
    var canCheck = S.mode === 'check' && !S.meta.frozen && !!S.keys;
    NHMD.render($('#n-view'), S.body, { checks: S.checks, interactive: canCheck, onCheck: toggleCheck, copy: true });
  }

  function toggleCheck(i, checked, input) {
    api('POST', '/checks', { index: i, checked: checked }).then(function (r) {
      S.checks = r.checks;
    }, function (x) {
      input.checked = !checked;
      input.closest('li') && input.closest('li').classList.toggle('done', !checked);
      NH.toast(x.message, 'err');
    });
  }

  function pill(icon, text, cls) { return h('span.pill' + (cls ? '.' + cls : ''), NH.icon(icon), text); }
  function drawMeta() {
    var m = S.meta, box = $('#n-meta'), kids = [];
    if (m.admin) kids.push(pill('key', 'Admin', 'acc'));
    else kids.push(pill(NH.modeInfo[S.mode].icon, NH.modeInfo[S.mode].label));
    kids.push(m.encrypted ? pill('shield', 'Terenkripsi', 'ok') : pill('eye', 'Publik', 'warn'));
    if (m.password) kids.push(pill('lock', 'Berpassword'));
    if (m.frozen) kids.push(pill('snow', 'Snapshot beku'));
    if (m.expires) kids.push(pill('clock', 'Hilang ' + NH.ago(m.expires), 'warn'));
    if (m.max_views) kids.push(pill('flame', 'Dibuka ' + m.views + '/' + m.max_views, 'bad'));
    if (m.open_at) kids.push(pill('clock', 'Dibuka ' + NH.ago(m.open_at), 'warn'));
    if (m.lock_at) kids.push(m.locked ? pill('lock', 'Dikunci') : pill('clock', 'Dikunci ' + NH.ago(m.lock_at), 'warn'));
    kids.push(h('span.faint.small', 'Diubah ' + NH.ago(m.updated)));
    box.replaceChildren.apply(box, kids);
  }

  // ---------- editing ----------
  function setSave(text, cls) {
    var s = $('#save-state');
    s.textContent = text;
    s.className = 'save-state ' + (cls || '');
  }
  function edited() {
    S.dirty = true;
    setSave('Belum tersimpan');
    clearTimeout(saveTimer);
    saveTimer = setTimeout(save, 1200);
  }
  function saveNow() { clearTimeout(saveTimer); save(); }

  function save(force) {
    if (S.saving) { S.pending = true; return; }
    if (!S.dirty && !force) return;
    S.saving = true;
    setSave('Menyimpan…');
    var t = currentTitle(), b = ed.value, vault = NHC.vault();
    var ownerBlob = S.meta.owned && S.meta.admin && vault ? vault.seal({ t: t, k: K, a: A, p: S.password }) : null;
    Promise.all([S.codec.encode({ t: t, b: b }), ownerBlob]).then(function (r) {
      var body = { content: r[0], version: S.version };
      if (r[1]) body.owner_blob = r[1];
      return api('PUT', '', body);
    }).then(function (res) {
      S.version = res.version;
      S.title = t; S.body = b; S.checks = {};
      S.dirty = currentBody() !== b || currentTitle() !== t;
      S.meta.updated = Math.floor(Date.now() / 1000);
      setSave(S.dirty ? 'Belum tersimpan' : 'Tersimpan', S.dirty ? '' : 'ok');
      document.title = (t || 'Tanpa judul') + ' · NoteHop';
      if (A) NH.recent.update(token, { t: t });
      drawMeta();
    }).catch(function (x) {
      if (x.status === 409) return conflict();
      setSave('Gagal menyimpan', 'err');
      NH.toast(x.message, 'err');
    }).finally(function () {
      S.saving = false;
      if (S.pending) { S.pending = false; save(); }
    });
  }

  function conflict() {
    setSave('Bentrok', 'err');
    var mine = h('button.btn.sm', { type: 'button' }, 'Timpa dengan versiku');
    var theirs = h('button.btn.sm.primary', { type: 'button' }, 'Pakai versi terbaru');
    banner('history', 'Orang lain menyimpan perubahan saat kamu mengetik.', 'warn', h('span.btn-row', theirs, mine));
    theirs.addEventListener('click', function () {
      NH.copy((currentTitle() ? '# ' + currentTitle() + '\n\n' : '') + ed.value, 'Versimu disalin ke clipboard');
      S.dirty = false;
      open(true).then(function () { drawBody(true); setSave('Versi terbaru dimuat', 'ok'); $('#banner').className = 'banner hide'; });
    });
    mine.addEventListener('click', function () {
      api('POST', '/open', { poll: true }).then(function (d) {
        S.version = d.version;
        $('#banner').className = 'banner hide';
        save(true);
      });
    });
  }

  function poll() {
    if (document.hidden || S.saving) return;
    open(true);
  }

  window.addEventListener('beforeunload', function (e) {
    if (S.dirty) { saveNow(); e.preventDefault(); e.returnValue = ''; }
  });

  // ---------- appended entries ----------
  function canAppend() { return !S.meta.frozen && !!S.keys && (S.mode === 'edit' || S.mode === 'append'); }

  function drawEntries() {
    var sec = $('#entries'), list = $('#entry-list');
    sec.classList.toggle('hide', !S.entries.length && !canAppend());
    $('#entry-form').classList.toggle('hide', !canAppend());
    $('#entries-count').textContent = S.entries.length;
    list.replaceChildren.apply(list, S.entries.map(function (e) {
      var body = h('div.md.entry-body');
      if (e.broken) body.appendChild(h('p.faint', 'Baris ini tidak bisa didekripsi.'));
      else NHMD.render(body, e.b, {});
      var head = h('div.entry-head', h('b', e.n || 'Anonim'), h('time.faint', { datetime: new Date(e.created * 1000).toISOString(), title: NH.date(e.created) }, NH.ago(e.created)));
      if (S.meta.admin && !S.meta.frozen) {
        var del = h('button.icon-btn.sm', { type: 'button', title: 'Hapus baris ini', 'aria-label': 'Hapus baris' }, NH.icon('trash'));
        del.addEventListener('click', function () {
          if (!confirm('Hapus baris dari ' + (e.n || 'Anonim') + '?')) return;
          api('DELETE', '/entries/' + e.id).then(function () {
            S.entries = S.entries.filter(function (x) { return x.id !== e.id; });
            drawEntries();
          }, function (x) { NH.toast(x.message, 'err'); });
        });
        head.appendChild(del);
      }
      return h('li.card.entry', head, body);
    }));
  }

  var nameInput = $('#entry-name'), entryText = $('#entry-text');
  nameInput.value = NH.store.get('nh_name', '');
  $('#entry-form').addEventListener('submit', function (e) {
    e.preventDefault();
    var b = entryText.value.trim(), n = nameInput.value.trim();
    if (!b) return;
    NH.store.set('nh_name', n);
    var btn = e.target.querySelector('button');
    btn.disabled = true;
    S.codec.encode({ n: n, b: b }).then(function (c) {
      return api('POST', '/entries', { content: c });
    }).then(function (res) {
      S.entries.push({ id: res.id, created: res.created, n: n, b: b });
      entryText.value = '';
      drawEntries();
      NH.toast('Ditambahkan', 'ok');
    }, function (x) { NH.toast(x.message, 'err'); }).finally(function () { btn.disabled = false; });
  });
  entryText.addEventListener('keydown', function (e) {
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) { e.preventDefault(); $('#entry-form').requestSubmit(); }
  });

  // ---------- admin: share, freeze, delete ----------
  function shareURL(t) { return NH.noteURL(t, { k: K }); }

  function linkRow(l, deletable) {
    var info = NH.modeInfo[l.mode];
    var url = shareURL(l.token);
    var copy = h('button.btn.sm', { type: 'button' }, NH.icon('copy'), 'Salin');
    copy.addEventListener('click', function () { NH.copy(url, 'Link ' + info.label.toLowerCase() + ' disalin'); });
    var qr = h('button.icon-btn.sm', { type: 'button', title: 'QR code', 'aria-label': 'QR code' }, NH.icon('qr'));
    qr.addEventListener('click', function () { showQR(url, info.label); });
    var row = h('li.link-row', h('span.link-mode', NH.icon(info.icon), info.label),
      h('code.link-url', { title: url }, '/n/' + l.token), h('span.btn-row', copy, qr));
    if (deletable) {
      var del = h('button.icon-btn.sm', { type: 'button', title: 'Hapus link ini', 'aria-label': 'Hapus link' }, NH.icon('trash'));
      del.addEventListener('click', function () {
        if (!confirm('Matikan link /n/' + l.token + '? Orang yang memegangnya tidak bisa membuka lagi. Link lain tetap jalan.')) return;
        api('DELETE', '/links/' + encodeURIComponent(l.token)).then(openShare, function (x) { NH.toast(x.message, 'err'); });
      });
      row.lastChild.appendChild(del);
    }
    return row;
  }

  function openShare() {
    var dlg = $('#share-dlg');
    api('GET', '').then(function (m) {
      Object.assign(S.meta, { links: m.links, snapshots: m.snapshots });
      var links = m.links || [];
      var list = $('#link-list');
      list.replaceChildren.apply(list, links.map(function (l) {
        // The admin link uses one token; don't offer to delete the one we're on.
        return linkRow(l, links.length > 1 && l.token !== token);
      }));
      var snaps = m.snapshots || [];
      $('#snap-box').classList.toggle('hide', !snaps.length);
      var sl = $('#snap-list');
      sl.replaceChildren.apply(sl, snaps.map(function (s) {
        var url = shareURL(s.token);
        var copy = h('button.btn.sm', { type: 'button' }, NH.icon('copy'), 'Salin');
        copy.addEventListener('click', function () { NH.copy(url, 'Link snapshot disalin'); });
        return h('li.link-row', h('span.link-mode', NH.icon('snow'), NH.date(s.created)),
          h('a.link-url', { href: NH.noteURL(s.token, { k: K, a: A }) }, '/n/' + s.token), h('span.btn-row', copy));
      }));
      NH.$$('#new-mode option').forEach(function (o) { o.disabled = S.meta.frozen && o.value !== 'read'; });
      drawTimed(Object.assign({}, S.meta, m));
      // A burn-after-reading note can only get read links.
      if (S.meta.max_views > 0) NH.$$('#new-mode option').forEach(function (o) { if (o.value !== 'read') o.disabled = true; });
      if (!dlg.open) dlg.showModal();
    }, function (x) { NH.toast(x.message, 'err'); });
  }

  $('#add-link').addEventListener('submit', function (e) {
    e.preventDefault();
    var slugEl = $('#new-slug');
    api('POST', '/links', { mode: $('#new-mode').value, slug: slugEl.value.trim() }).then(function (l) {
      slugEl.value = '';
      NH.copy(shareURL(l.token), 'Link baru dibuat dan disalin');
      openShare();
    }, function (x) { NH.toast(x.message, 'err'); });
  });
  $('#new-slug').addEventListener('input', function () {
    this.value = this.value.toLowerCase().replace(/[^a-z0-9-]/g, '-').replace(/^-+/, '');
  });
  $('#copy-admin').addEventListener('click', function () {
    NH.copy(NH.noteURL(token, { k: K, a: A }), 'Link admin disalin. Simpan baik-baik');
  });

  // ---------- timed features (admin): time capsule and scheduled lock ----------
  // Each is a pill toggle: switching it on shows a time to save, switching an
  // active one off cancels it on the server.
  function localInput(unix) {
    var d = new Date(unix * 1000), p = function (n) { return String(n).padStart(2, '0'); };
    return d.getFullYear() + '-' + p(d.getMonth() + 1) + '-' + p(d.getDate()) + 'T' + p(d.getHours()) + ':' + p(d.getMinutes());
  }
  var TIMED = {
    open: {
      field: 'open_at', input: '#open-at', def: 86400,
      state: function (m) {
        return m.open_at ? 'Aktif: pembaca baru bisa membuka ' + NH.date(m.open_at) + ' (' + NH.ago(m.open_at) + '). Server menahan isinya sampai saat itu.'
          : 'Belum aktif. Pilih waktu lalu Simpan; sampai saat itu server menahan isinya dan pembaca melihat hitung mundur.';
      },
      saved: 'Kapsul waktu disimpan', cleared: 'Kapsul waktu dibatalkan'
    },
    lock: {
      field: 'lock_at', input: '#lock-at', def: 7 * 86400,
      state: function (m) {
        return m.locked ? 'Sudah berlaku sejak ' + NH.date(m.lock_at) + '. Pengunjung hanya bisa membaca; link admin tetap bisa mengubah.'
          : m.lock_at ? 'Aktif: mulai ' + NH.date(m.lock_at) + ' (' + NH.ago(m.lock_at) + ') link edit, tambah, dan centang hanya bisa membaca.'
            : 'Belum aktif. Pilih waktu lalu Simpan; setelah itu link edit, tambah, dan centang hanya bisa membaca.';
      },
      // Read-only links have nothing to lock.
      blocked: function (m) {
        return (m.links || []).every(function (l) { return l.mode === 'read'; }) ? 'Kunci otomatis untuk link Edit, Tambah, atau Centang. Buat link seperti itu dulu.' : '';
      },
      saved: 'Kunci otomatis disimpan', cleared: 'Kunci otomatis dibatalkan'
    }
  };
  function drawTimed(m) {
    $('#share-adv').classList.toggle('hide', !!S.meta.frozen);
    Object.keys(TIMED).forEach(function (k) {
      var t = TIMED[k], toggle = $('[data-timed="' + k + '"]'), form = $('[data-timed-form="' + k + '"]');
      var active = !!m[t.field], why = t.blocked && !active ? t.blocked(m) : '';
      toggle.checked = active || (!why && toggle.checked);
      toggle.closest('.pill-opt').classList.toggle('blocked', !!why);
      toggle.closest('.pill-opt').title = why;
      form.classList.toggle('hide', !toggle.checked);
      var input = $(t.input);
      input.min = localInput(Math.floor(Date.now() / 1000) + 60);
      if (active) input.value = localInput(m[t.field]);
      else if (!input.value) input.value = localInput(Math.floor(Date.now() / 1000) + t.def);
      $('[data-timed-state="' + k + '"]').textContent = t.state(m);
    });
    if (m.open_at || m.lock_at) $('#share-adv').open = true;
  }
  function setTimed(k, at) {
    var t = TIMED[k], body = {};
    body[t.field] = at;
    return api('POST', '/config', body).then(function (m) {
      ['lock_at', 'locked', 'open_at'].forEach(function (f) { S.meta[f] = m[f]; });
      drawTimed(Object.assign({}, S.meta, m)); drawMeta();
      NH.toast(at ? t.saved : t.cleared, 'ok');
    }, function (x) { NH.toast(x.message, 'err'); drawTimed(S.meta); });
  }
  Object.keys(TIMED).forEach(function (k) {
    var t = TIMED[k], toggle = $('[data-timed="' + k + '"]');
    toggle.addEventListener('click', function (e) {
      var why = t.blocked && !S.meta[t.field] ? t.blocked(S.meta) : '';
      if (why) { e.preventDefault(); NH.toast(why); }
    });
    toggle.addEventListener('change', function () {
      if (!toggle.checked && S.meta[t.field]) { setTimed(k, 0); return; }
      $('[data-timed-form="' + k + '"]').classList.toggle('hide', !toggle.checked);
    });
    $('[data-timed-form="' + k + '"]').addEventListener('submit', function (e) {
      e.preventDefault();
      var ms = new Date($(t.input).value).getTime();
      if (ms) setTimed(k, Math.floor(ms / 1000));
    });
  });

  function showQR(url, label) {
    var qr = qrcode(0, 'M');
    qr.addData(url);
    qr.make();
    var src = qr.createDataURL(6, 2);
    var img = h('img', { src: src, alt: 'QR code untuk link ' + label, width: 240, height: 240 });
    var dl = h('a.btn.sm', { href: src, download: 'notehop-' + token + '.gif' }, NH.icon('qr'), 'Unduh QR');
    $('#qr-box').replaceChildren(img, dl);
    $('#qr-mode').textContent = 'Link ' + label.toLowerCase() + ', termasuk kuncinya. Siapa pun yang memindai bisa membuka.';
    $('#qr-dlg').showModal();
  }

  function freeze() {
    if (ed && S.dirty) saveNow();
    if (!confirm('Bekukan versi saat ini? Kamu akan mendapat link baca saja yang isinya tidak akan pernah berubah. Catatan ini tetap bisa kamu edit.')) return;
    var go = function () {
      api('POST', '/freeze').then(function (l) {
        NH.copy(shareURL(l.token), 'Snapshot dibuat, link-nya disalin');
        openShare();
      }, function (x) { NH.toast(x.message, 'err'); });
    };
    // Wait for an in-flight save so the snapshot has the latest text.
    (function wait() { if (S.saving || S.dirty) setTimeout(wait, 200); else go(); })();
  }

  function removeNote() {
    if (!confirm('Hapus catatan ini selamanya? Semua link-nya akan mati. Snapshot beku tetap ada.')) return;
    S.dirty = false;
    api('DELETE', '').then(function () {
      NH.recent.remove(token);
      location.href = '/';
    }, function (x) { NH.toast(x.message, 'err'); });
  }

  window.addEventListener('hashchange', function () { location.reload(); });
  start();
})();
