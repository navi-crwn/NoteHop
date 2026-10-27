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
    showState('x', NH.t('note.cant_open'), msg, h('a.btn.primary', { href: '/' }, NH.icon('plus'), NH.t('error.write_new')));
  }

  function start() {
    if (!NHC.available) return gone(NH.t('js.no_webcrypto'));
    NH.api('GET', '/api/n/' + encodeURIComponent(token), undefined, A ? { 'X-Admin': A } : {}).then(function (meta) {
      S.meta = meta;
      if (K && !/^[A-Za-z0-9_-]{43}$/.test(K)) return gone(NH.t('note.bad_key'));
      if (meta.encrypted && !K) return gone(NH.t('note.no_key'));
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
        clock.textContent = NH.t('md.opening');
        // A few seconds of slack for clocks that disagree with the server.
        setTimeout(start, 3000);
        return;
      }
      var d = Math.floor(left / 86400), hh = Math.floor(left % 86400 / 3600), mm = Math.floor(left % 3600 / 60), ss = left % 60;
      var p2 = function (n) { return String(n).padStart(2, '0'); };
      clock.textContent = (d ? NH.t('note.days', { n: d }) + ' ' : '') + p2(hh) + ':' + p2(mm) + ':' + p2(ss);
    };
    tick();
    capsuleTimer = setInterval(tick, 1000);
    showState('clock', NH.t('note.capsule_title'), NH.t('note.capsule_text', { time: at }), clock);
  }

  function askPassword(errMsg) {
    var handoff = null;
    try { handoff = sessionStorage.getItem('nh_pw_' + token); sessionStorage.removeItem('nh_pw_' + token); } catch (x) {}
    if (handoff && !errMsg) return unlock(handoff).catch(function () { askPassword(''); });
    var input = h('input', { type: 'password', placeholder: NH.t('feat.password'), 'aria-label': NH.t('note.password_label'), autocomplete: 'off', required: true });
    var err = h('p.locked-err', errMsg || '');
    var btn = h('button.btn.primary', { type: 'submit' }, NH.icon('unlock'), NH.t('vault.open_btn'));
    var form = h('form.pw-form', input, btn);
    form.addEventListener('submit', function (e) {
      e.preventDefault();
      btn.disabled = true;
      err.textContent = NH.t('js.deriving');
      unlock(input.value).catch(function (x) {
        btn.disabled = false;
        err.textContent = x.status === 403 || x.name === 'OperationError' ? NH.t('err.bad_password') : x.message;
        input.select();
      });
    });
    showState('lock', NH.t('note.pw_title'), NH.t('note.pw_text'), form, err);
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
    var btn = h('button.btn.primary.lg', { type: 'button' }, NH.icon('eye'), NH.t('note.open_now'));
    btn.addEventListener('click', function () {
      btn.disabled = true;
      // A wrong password only shows up here: burn notes can't be test-opened
      // first. The server refuses without counting a view, so ask again.
      open(false).catch(function (x) {
        if (x.status === 403 && S.meta.password) return askPassword(NH.t('err.bad_password'));
        gone(x.message);
      });
    });
    showState('flame', NH.t(left <= 1 ? 'note.burn_title_once' : 'note.burn_title_many'),
      left <= 1 ? NH.t('note.burn_text_once') : NH.t('note.burn_text_many', { n: left }), btn);
  }

  // ---------- loading content ----------
  var inboxKeys = {};
  function inboxKey(sealed) {
    if (!sealed) return Promise.reject(new Error('no inbox key'));
    if (!inboxKeys[sealed]) inboxKeys[sealed] = S.codec.decode(sealed).then(function (o) { return o.jwk; });
    return inboxKeys[sealed];
  }
  function decode(data) {
    return Promise.all([
      S.codec.decode(data.content),
      Promise.all(data.entries.map(function (e) {
        // Entries posted to the inbox are sealed to the note's inbox key.
        var dec = /^x1\./.test(e.content) ? inboxKey(data.inbox_key).then(function (jwk) { return NHC.inboxOpen(jwk, e.content); }) : S.codec.decode(e.content);
        return dec.then(function (v) { return { id: e.id, created: e.created, n: v.n || '', b: v.b || '' }; },
          function () { return { id: e.id, created: e.created, n: '', b: '', broken: true }; });
      }))
    ]).then(function (r) { return { note: r[0], entries: r[1] }; }, function () {
      var err = new Error(NH.t('note.key_mismatch'));
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
        delete S.meta.votes; delete S.meta.burn_blocks; delete S.meta.signatures;
        S.checks = data.checks || {};
        S.entries = d.entries;
        var countsChanged = JSON.stringify(data.votes || {}) !== JSON.stringify(S.votes || {});
        S.votes = data.votes || {};
        S.burnBlocks = data.burn_blocks || {};
        S.signatures = data.signatures || [];
        var live = window.NHLive && NHLive.active();
        if (live) S.version = data.version;
        if (!live && (fresh || !S.dirty)) {
          S.version = data.version;
          S.title = d.note.t || '';
          S.body = d.note.b || '';
        }
        if (fresh) {
          S.ready = true;
          setup(data.burned);
          // Served by the service worker from an earlier visit.
          if (data.offline) banner('clock', NH.t('note.offline_copy', { time: data.offline_saved }), 'info');
          return;
        }
        // The scheduled lock just passed: reload into read-only mode.
        if (S.meta.locked && !S.meta.admin && S.mode !== 'read' && !S.dirty) return location.reload();
        if ((bodyChanged || checksChanged || countsChanged) && !S.dirty) drawBody(!live);
        if (window.NHFeat) NHFeat.refresh();
        if (entriesChanged) drawEntries();
        drawMeta();
      });
    }, function (x) {
      if (x.status === 425 && !poll) return start();
      if (x.status === 403 && S.meta.password && !poll) throw x;
      if (poll) { if (x.status === 404) gone(NH.t('note.just_gone')); return; }
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

    if (burned) banner('flame', NH.t('note.burned'), 'warn');
    else if (m.frozen) banner('snow', NH.t('note.frozen_banner', { time: m.created }), 'info',
      m.admin && m.parent ? h('a', { href: NH.noteURL(m.parent, { k: K, a: A }) }, NH.t('note.open_original')) : null);
    else if (m.open_at && m.admin) banner('clock', NH.t('note.capsule_admin', { time: m.open_at }), 'info');
    else if (m.locked && !m.admin && m.mode !== 'read') banner('lock', NH.t('err.locked', { time: m.lock_at }), 'info');
    else if (!S.keys && m.mode !== 'read') banner('key', NH.t('note.no_key_banner'), 'info');

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
      ed = NHEditor($('#n-edit [data-editor]'), {
        onChange: edited, onSave: saveNow,
        onBurnKey: function (k) {
          if (!S.meta.encrypted) return Promise.reject(new Error(NH.t('rule.burn_block_e2e')));
          return api('POST', '/burns', k).then(function () { S.burnBlocks = S.burnBlocks || {}; S.burnBlocks[k.id] = { id: k.id, max_views: k.max_views, views: 0 }; });
        }
      });
      ti.addEventListener('input', edited);
    }
    drawBody(true);
    drawMeta();
    drawEntries();

    $('#act-copy').addEventListener('click', function () {
      var t = currentTitle(), b = NHMD.applyChecks(currentBody(), S.mode === 'edit' ? {} : S.checks);
      NH.copy((t ? '# ' + t + '\n\n' : '') + b, NH.t('note.text_copied'));
    });
    $('#act-share').addEventListener('click', openShare);
    $('#act-freeze').addEventListener('click', freeze);
    $('#act-delete').addEventListener('click', removeNote);

    if (isNew && m.admin) { openShare(); }
    readyFns.forEach(function (fn) { fn(); });
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
    var title = S.title || NH.t('common.untitled');
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
    drawTemplate();
    var canWrite = !!S.keys && !S.meta.frozen && !S.meta.locked;
    NHMD.render($('#n-view'), S.body, { checks: S.checks, interactive: canCheck, onCheck: toggleCheck, copy: true,
      vars: S.vars, wiki: S.wiki ? function (t) { return S.wiki[t.toLowerCase()]; } : null,
      votes: S.votes, myVote: myVote, onVote: canWrite ? vote : null,
      burnBlocks: S.burnBlocks, onReveal: S.keys ? revealBurn : null });
  }

  // ---------- template fields ({{name}}) ----------
  // Readers fill the fields here; the values never leave this browser.
  S.vars = {};
  var tplTimer = null;
  function drawTemplate() {
    var names = NHMD.templateVars(S.body), box = $('#tpl-fields');
    $('#tpl').classList.toggle('hide', !names.length);
    if (!names.length || box.dataset.names === names.join('\u0000')) return;
    box.dataset.names = names.join('\u0000');
    box.replaceChildren.apply(box, names.map(function (n) {
      var input = h('input', { type: 'text', dir: 'auto', value: S.vars[n] || '', 'aria-label': n, placeholder: n });
      input.addEventListener('input', function () {
        S.vars[n] = input.value;
        clearTimeout(tplTimer);
        tplTimer = setTimeout(function () { drawBody(false); }, 120);
      });
      return h('label.tpl-field', h('span', n), input);
    }));
  }
  $('#tpl-copy').addEventListener('click', function () {
    NH.copy(NHMD.fillVars(NHMD.applyChecks(S.body, S.checks), S.vars, false), NH.t('tpl.copied'));
  });
  $('#tpl-clear').addEventListener('click', function () {
    S.vars = {};
    NH.$$('#tpl-fields input').forEach(function (i) { i.value = ''; });
    drawBody(false);
  });

  // ---------- wiki links ([[Title]]) ----------
  // Titles come from the reader's own vault, decrypted here, so [[links]]
  // only resolve for the account that owns the notes.
  function loadWiki() {
    var vault = NHC.vault();
    if (!vault) return;
    NH.api('GET', '/api/me/notes').then(function (res) {
      return Promise.all(res.notes.map(function (n) {
        return n.blob ? vault.open(n.blob).then(function (v) { return { n: n, v: v }; }, function () { return null; }) : null;
      }));
    }).then(function (rows) {
      var map = {};
      rows.forEach(function (r) { if (r && r.v.t && r.v.k) map[r.v.t.toLowerCase()] = NH.noteURL(r.n.token, { k: r.v.k, a: r.v.a }); });
      S.wiki = map;
      if (S.ready && S.mode !== 'edit' && /\[\[/.test(S.body)) drawBody(false);
    }, function () {});
  }

  // ---------- polls and self-destructing blocks ----------
  function voteKey(poll) { return 'nh_vote_' + token + '_' + poll; }
  function myVote(poll) { var v = NH.store.get(voteKey(poll), -1); return typeof v === 'number' ? v : -1; }
  // A random id per browser: the server remembers each voter's choice, so a
  // vote can be moved or taken back but not counted twice.
  function voter() {
    var v = NH.store.get('nh_voter', '');
    if (!/^[A-Za-z0-9_-]{16,64}$/.test(v)) { v = NHC.newKey(); NH.store.set('nh_voter', v); }
    return v;
  }
  function vote(poll, option) {
    api('POST', '/votes', { poll: poll, option: option, voter: voter() }).then(function (r) {
      S.votes = r.votes;
      NH.store.set(voteKey(poll), option);
      drawBody(false);
    }, function (x) { NH.toast(x.message, 'err'); });
  }
  function revealBurn(id) {
    return api('POST', '/burns/' + encodeURIComponent(id) + '/reveal').then(function (r) {
      if (S.burnBlocks && S.burnBlocks[id]) S.burnBlocks[id].views = S.burnBlocks[id].max_views - r.left;
      return r;
    });
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
    // One plain sentence first: who can open it, and what you can do here.
    $('#n-status').replaceChildren(NH.icon(m.encrypted ? 'lock' : 'globe'), ' ' + NH.t(m.encrypted ? 'status.private' : 'status.public') + ' ' +
      NH.t(m.admin ? 'status.owner' : 'status.' + S.mode));
    if (m.admin) kids.push(pill('key', NH.t('pill.admin'), 'acc'));
    else kids.push(pill(NH.modeInfo[S.mode].icon, NH.modeInfo[S.mode].label));
    kids.push(m.encrypted ? pill('shield', NH.t('pill.encrypted'), 'ok') : pill('eye', NH.t('pill.public'), 'warn'));
    if (m.password) kids.push(pill('lock', NH.t('pill.password')));
    if (m.frozen) kids.push(pill('snow', NH.t('pill.frozen')));
    if (m.expires) kids.push(pill('clock', NH.t('pill.disappears', { ago: NH.ago(m.expires) }), 'warn'));
    if (m.max_views) kids.push(pill('flame', NH.t('pill.opened', { n: m.views, max: m.max_views }), 'bad'));
    if (m.open_at) kids.push(pill('clock', NH.t('pill.opens', { ago: NH.ago(m.open_at) }), 'warn'));
    if (m.lock_at) kids.push(m.locked ? pill('lock', NH.t('pill.locked')) : pill('clock', NH.t('pill.locks', { ago: NH.ago(m.lock_at) }), 'warn'));
    if (m.reset) kids.push(pill('history', NH.t('pill.resets', { every: NH.t('reset.' + m.reset) })));
    if (m.ip_limited) kids.push(pill('shield', NH.t('pill.ip')));
    if (m.admin && m.last_open) kids.push(pill('eye', NH.t('pill.last_open', { n: m.views, ago: NH.ago(m.last_open) })));
    kids.push(h('span.faint.small', NH.t('pill.changed', { ago: NH.ago(m.updated) })));
    box.replaceChildren.apply(box, kids);
  }

  // ---------- editing ----------
  function setSave(text, cls) {
    var s = $('#save-state');
    s.textContent = text;
    s.className = 'save-state ' + (cls || '');
  }
  function edited() {
    // Live: changes travel to the others; only the leader writes to the server.
    if (window.NHLive && NHLive.edited() === 'follower') {
      S.dirty = false;
      clearTimeout(saveTimer);
      setSave(NH.t('live.saved_by_leader'), 'ok');
      return;
    }
    S.dirty = true;
    setSave(NH.t('save.unsaved'));
    clearTimeout(saveTimer);
    saveTimer = setTimeout(save, 1200);
  }
  function saveNow() { clearTimeout(saveTimer); save(); }

  function save(force) {
    if (S.saving) { S.pending = true; return; }
    if (!S.dirty && !force) return;
    S.saving = true;
    setSave(NH.t('save.saving'));
    var t = currentTitle(), b = ed.value, vault = NHC.vault();
    if (!S.meta.encrypted && !S.keyWarned && NH.leaksKeys(t + '\n' + b)) { S.keyWarned = true; NH.toast(NH.t('warn.public_keys'), 'err'); }
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
      setSave(NH.t(S.dirty ? 'save.unsaved' : 'save.saved'), S.dirty ? '' : 'ok');
      document.title = (t || NH.t('common.untitled')) + ' · NoteHop';
      if (A) NH.recent.update(token, { t: t });
      drawMeta();
    }).catch(function (x) {
      // Live: the shared document already holds everyone's edits; just
      // catch up with the version number and save it.
      if (x.status === 409 && window.NHLive && NHLive.active()) {
        return api('POST', '/open', { poll: true }).then(function (d) { S.version = d.version; S.dirty = true; S.pending = true; });
      }
      if (x.status === 409) return conflict();
      setSave(NH.t('err.save_failed'), 'err');
      NH.toast(x.message, 'err');
    }).finally(function () {
      S.saving = false;
      if (S.pending) { S.pending = false; save(); }
    });
  }

  function conflict() {
    setSave(NH.t('save.conflict'), 'err');
    var mine = h('button.btn.sm', { type: 'button' }, NH.t('save.keep_mine'));
    var theirs = h('button.btn.sm.primary', { type: 'button' }, NH.t('save.take_theirs'));
    banner('history', NH.t('save.conflict_text'), 'warn', h('span.btn-row', theirs, mine));
    theirs.addEventListener('click', function () {
      NH.copy((currentTitle() ? '# ' + currentTitle() + '\n\n' : '') + ed.value, NH.t('save.mine_copied'));
      S.dirty = false;
      open(true).then(function () { drawBody(true); setSave(NH.t('save.latest_loaded'), 'ok'); $('#banner').className = 'banner hide'; });
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
      var body = h('div.md.entry-body', { dir: 'auto' });
      if (e.broken) body.appendChild(h('p.faint', NH.t('entry.broken')));
      else NHMD.render(body, e.b, {});
      var head = h('div.entry-head', h('b', e.n || NH.t('entry.anon')), h('time.faint', { datetime: new Date(e.created * 1000).toISOString(), title: NH.date(e.created) }, NH.ago(e.created)));
      if (S.meta.admin && !S.meta.frozen) {
        var del = h('button.icon-btn.sm', { type: 'button', title: NH.t('entry.delete'), 'aria-label': NH.t('entry.delete') }, NH.icon('trash'));
        del.addEventListener('click', function () {
          if (!confirm(NH.t('entry.confirm_delete', { name: e.n || NH.t('entry.anon') }))) return;
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
      NH.toast(NH.t('entry.added'), 'ok');
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
    var copy = h('button.btn.sm', { type: 'button' }, NH.icon('copy'), NH.t('share.copy'));
    copy.addEventListener('click', function () { NH.copy(url, NH.t('share.link_copied', { mode: info.label })); });
    var qr = h('button.icon-btn.sm', { type: 'button', title: NH.t('share.qr'), 'aria-label': NH.t('share.qr') }, NH.icon('qr'));
    qr.addEventListener('click', function () { showQR(url, info.label); });
    var row = h('li.link-row', h('span.link-mode', NH.icon(info.icon), info.label),
      h('code.link-url', { title: url }, '/n/' + l.token), h('span.btn-row', copy, qr));
    if (deletable) {
      var del = h('button.icon-btn.sm', { type: 'button', title: NH.t('share.delete_link'), 'aria-label': NH.t('share.delete_link') }, NH.icon('trash'));
      del.addEventListener('click', function () {
        if (!confirm(NH.t('share.confirm_delete', { link: '/n/' + l.token }))) return;
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
        var copy = h('button.btn.sm', { type: 'button' }, NH.icon('copy'), NH.t('share.copy'));
        copy.addEventListener('click', function () { NH.copy(url, NH.t('share.snapshot_copied')); });
        return h('li.link-row', h('span.link-mode', NH.icon('snow'), NH.date(s.created)),
          h('a.link-url', { href: NH.noteURL(s.token, { k: K, a: A }) }, '/n/' + s.token), h('span.btn-row', copy));
      }));
      NH.$$('#new-mode option').forEach(function (o) { o.disabled = S.meta.frozen && o.value !== 'read'; });
      if (window.NHAdv) NHAdv.draw(Object.assign({}, S.meta, m));
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
      NH.copy(shareURL(l.token), NH.t('share.new_copied'));
      openShare();
    }, function (x) { NH.toast(x.message, 'err'); });
  });
  $('#new-slug').addEventListener('input', function () {
    this.value = this.value.toLowerCase().replace(/[^a-z0-9-]/g, '-').replace(/^-+/, '');
  });
  $('#copy-admin').addEventListener('click', function () {
    NH.copy(NH.noteURL(token, { k: K, a: A }), NH.t('share.admin_copied'));
  });

  function showQR(url, label) {
    var qr = qrcode(0, 'M');
    qr.addData(url);
    qr.make();
    var src = qr.createDataURL(6, 2);
    var img = h('img', { src: src, alt: NH.t('qr.alt', { mode: label }), width: 240, height: 240 });
    var dl = h('a.btn.sm', { href: src, download: 'notehop-' + token + '.gif' }, NH.icon('qr'), NH.t('qr.download'));
    $('#qr-box').replaceChildren(img, dl);
    $('#qr-mode').textContent = NH.t('qr.note', { mode: label });
    $('#qr-dlg').showModal();
  }

  function freeze() {
    if (ed && S.dirty) saveNow();
    if (!confirm(NH.t('freeze.confirm'))) return;
    var go = function () {
      api('POST', '/freeze').then(function (l) {
        NH.copy(shareURL(l.token), NH.t('freeze.done'));
        openShare();
      }, function (x) { NH.toast(x.message, 'err'); });
    };
    // Wait for an in-flight save so the snapshot has the latest text.
    (function wait() { if (S.saving || S.dirty) setTimeout(wait, 200); else go(); })();
  }

  function removeNote() {
    if (!confirm(NH.t('note.confirm_delete'))) return;
    S.dirty = false;
    api('DELETE', '').then(function () {
      NH.recent.remove(token);
      location.href = '/';
    }, function (x) { NH.toast(x.message, 'err'); });
  }

  window.addEventListener('hashchange', function () { location.reload(); });

  // For extras.js (duplicate, export, present, …): read-only access to the
  // decrypted note and the link's keys.
  window.NHNote = {
    title: function () { return currentTitle(); },
    body: function () { return NHMD.applyChecks(currentBody(), S.mode === 'edit' ? {} : S.checks); },
    entries: function () { return S.entries.slice(); },
    meta: function () { return S.meta; },
    hasKey: function () { return !!S.keys; },
    encrypted: function () { return !!(S.meta && S.meta.encrypted); },
    view: function () { return $('#n-view'); },
    // for features.js
    token: token, key: function () { return K; }, admin: function () { return A; },
    password: function () { return S.password; },
    state: S, api: api, codec: function () { return S.codec; },
    editor: function () { return ed; },
    redraw: function () { drawBody(false); drawMeta(); },
    setText: function (title, body) { $('#n-title-input').value = title; ed.value = body; edited(); },
    // for live.js
    edited: edited, mode: function () { return S.mode; }, keys: function () { return S.keys; },
    // The live sync on joining often brings the same text: don't redraw then.
    showBody: function (title, body) {
      if (title === S.title && body === S.body) return;
      S.title = title; S.body = body; drawBody(false);
    },
    // for blocks.js (forms): add an entry as this reader
    canAppend: function () { return canAppend(); },
    addEntry: function (n, b) {
      return S.codec.encode({ n: n, b: b }).then(function (c) { return api('POST', '/entries', { content: c }); }).then(function (res) {
        S.entries.push({ id: res.id, created: res.created, n: n, b: b });
        drawEntries();
      });
    },
    applyMeta: function (m) {
      ['lock_at', 'locked', 'open_at', 'sign_off', 'watermark', 'dms_days', 'reset', 'reset_at', 'ip_allow', 'ip_limited', 'decoy', 'suggestions',
        'watch', 'watch_key', 'inbox', 'inbox_key', 'profile']
        .forEach(function (f) { S.meta[f] = m[f]; });
      drawMeta();
    },
    onReady: function (fn) { readyFns.push(fn); if (S.ready) fn(); }
  };
  var readyFns = [];
  if (document.body.dataset.user) loadWiki();
  start();
})();
