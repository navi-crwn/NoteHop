// Note page features that need the server: advanced share settings (time
// capsule, scheduled lock, dead man's switch, recurring checklist, IP
// allowlist, sign-off, watermark, decoy, watch webhook, inbox, profile),
// version history, suggestions,
// signatures, watermark tracing and private margins. Text is encrypted and
// decrypted here; the server stores and counts.
(function () {
  'use strict';
  var $ = NH.$, h = NH.h, N = window.NHNote;
  if (!N) return;
  var S = N.state;

  function localInput(unix) {
    var d = new Date(unix * 1000), p = function (n) { return String(n).padStart(2, '0'); };
    return d.getFullYear() + '-' + p(d.getMonth() + 1) + '-' + p(d.getDate()) + 'T' + p(d.getHours()) + ':' + p(d.getMinutes());
  }
  function nowUnix() { return Math.floor(Date.now() / 1000); }

  // ---------- advanced share settings ----------
  // Each setting is a pill. Switching one on shows its form (or saves right
  // away for on/off settings); switching an active one off clears it. Pills
  // that would clash with what's set are dimmed and say why.
  function allRead(m) { return (m.links || []).every(function (l) { return l.mode === 'read'; }); }
  var ADV = [
    { id: 'open', icon: 'clock', label: 'feat.capsule', kind: 'time', field: 'open_at', def: 86400,
      active: function (m) { return !!m.open_at && !m.dms_days; },
      blocked: function (m) { return m.dms_days ? NH.t('rule.capsule_dms') : ''; },
      state: function (m) { return m.open_at && !m.dms_days ? NH.t('timed.open_on', { time: m.open_at, ago: NH.ago(m.open_at) }) : NH.t('timed.open_off'); },
      clear: { open_at: 0 }, saved: 'timed.open_saved', cleared: 'timed.open_cleared' },
    { id: 'lock', icon: 'lock', label: 'feat.lock', kind: 'time', field: 'lock_at', def: 7 * 86400,
      active: function (m) { return !!m.lock_at; },
      blocked: function (m) { return allRead(m) ? NH.t('timed.lock_blocked') : ''; },
      state: function (m) {
        return m.locked ? NH.t('timed.lock_done', { time: m.lock_at }) : m.lock_at ? NH.t('timed.lock_on', { time: m.lock_at, ago: NH.ago(m.lock_at) }) : NH.t('timed.lock_off');
      },
      clear: { lock_at: 0 }, saved: 'timed.lock_saved', cleared: 'timed.lock_cleared' },
    { id: 'dms', icon: 'clock', label: 'feat.dms', kind: 'number', field: 'dms_days', def: 30, min: 1, max: 365, unit: 'dms.days',
      active: function (m) { return m.dms_days > 0; },
      blocked: function (m) { return m.open_at && !m.dms_days ? NH.t('rule.dms_capsule') : ''; },
      state: function (m) { return m.dms_days ? NH.t('dms.on', { n: m.dms_days, time: m.open_at }) : NH.t('dms.off'); },
      clear: { dms_days: 0 }, saved: 'dms.saved', cleared: 'dms.cleared' },
    { id: 'reset', icon: 'history', label: 'feat.reset', kind: 'select', field: 'reset', options: ['daily', 'weekly', 'monthly'],
      active: function (m) { return !!m.reset; },
      blocked: function () { return ''; },
      state: function (m) { return m.reset ? NH.t('reset.on', { every: NH.t('reset.' + m.reset), time: m.reset_at }) : NH.t('reset.off'); },
      clear: { reset: '' }, saved: 'reset.saved', cleared: 'reset.cleared' },
    { id: 'ip', icon: 'shield', label: 'feat.ip', kind: 'list', field: 'ip_allow',
      active: function (m) { return (m.ip_allow || []).length > 0; },
      blocked: function () { return ''; },
      state: function (m) { return (m.ip_allow || []).length ? NH.t('ip.on', { n: m.ip_allow.length }) : NH.t('ip.off'); },
      clear: { ip_allow: [] }, saved: 'ip.saved', cleared: 'ip.cleared' },
    { id: 'signoff', icon: 'check', label: 'feat.signoff', kind: 'bool', field: 'sign_off',
      active: function (m) { return !!m.sign_off; }, blocked: function () { return ''; },
      state: function (m) { return NH.t(m.sign_off ? 'signoff.on' : 'signoff.off'); },
      clear: { sign_off: false }, saved: 'signoff.saved', cleared: 'signoff.cleared' },
    { id: 'watermark', icon: 'eye', label: 'feat.watermark', kind: 'bool', field: 'watermark',
      active: function (m) { return !!m.watermark; }, blocked: function () { return ''; },
      state: function (m) { return NH.t(m.watermark ? 'watermark.on' : 'watermark.off'); },
      clear: { watermark: false }, saved: 'watermark.saved', cleared: 'watermark.cleared' },
    { id: 'decoy', icon: 'key', label: 'feat.decoy', kind: 'decoy',
      active: function (m) { return !!m.decoy; },
      blocked: function (m) { return m.password ? '' : NH.t('rule.decoy_password'); },
      state: function (m) {
        return !m.decoy ? NH.t('decoy.off') : m.decoy.opened ? NH.t('decoy.used', { n: m.decoy.opened, time: m.decoy.last_opened }) : NH.t('decoy.on');
      },
      saved: 'decoy.saved', cleared: 'decoy.cleared' },
    { id: 'watch', icon: 'link', label: 'feat.watch', kind: 'url', field: 'watch',
      active: function (m) { return !!m.watch; }, blocked: function () { return ''; },
      state: function (m) { return m.watch ? NH.t('watch.on', { url: m.watch, key: m.watch_key }) : NH.t('watch.off'); },
      clear: { watch: '' }, saved: 'watch.saved', cleared: 'watch.cleared' },
    { id: 'inbox', icon: 'append', label: 'feat.inbox', kind: 'inbox',
      active: function (m) { return !!m.inbox; }, blocked: function () { return ''; },
      state: function (m) { return m.inbox ? NH.t('inbox.on', { url: m.inbox }) : NH.t('inbox.off'); },
      saved: 'inbox.saved', cleared: 'inbox.cleared' },
    { id: 'profile', icon: 'globe', label: 'feat.profile', kind: 'bool', field: 'profile',
      active: function (m) { return !!m.profile; },
      blocked: function (m) { return m.encrypted || !m.owned ? NH.t('rule.profile_public') : ''; },
      state: function (m) { return NH.t(m.profile ? 'profile.on' : 'profile.off', { name: document.body.dataset.user || '' }); },
      clear: { profile: false }, saved: 'profile.saved', cleared: 'profile.cleared' }
  ];

  var built = false;
  function build() {
    var pills = $('#adv-pills'), forms = $('#adv-forms');
    ADV.forEach(function (f) {
      f.input = h('input', { type: 'checkbox' });
      f.pill = h('label.pill-opt', f.input, h('span', NH.icon(f.icon), NH.t(f.label)));
      pills.appendChild(f.pill);
      f.form = h('form.feat.timed-form.hide');
      f.hint = h('span.hint');
      var row = h('div.timed-row');
      if (f.kind === 'time') f.field_el = h('input', { type: 'datetime-local', required: true });
      if (f.kind === 'number') f.field_el = h('input', { type: 'number', min: f.min, max: f.max, required: true });
      if (f.kind === 'select') f.field_el = h('select', f.options.map(function (o) { return h('option', { value: o }, NH.t('reset.' + o)); }));
      if (f.kind === 'list') f.field_el = h('textarea', { rows: 3, placeholder: '203.0.113.7\n10.0.0.0/8', dir: 'ltr' });
      if (f.kind === 'url') f.field_el = h('input', { type: 'url', required: true, placeholder: 'https://example.com/hook', dir: 'ltr' });
      if (f.kind === 'decoy') {
        f.pw = h('input', { type: 'password', autocomplete: 'new-password', placeholder: NH.t('new.decoy_password'), required: true });
        f.text = h('textarea', { rows: 3, dir: 'auto', placeholder: NH.t('new.decoy_text') });
        row = h('div.decoy-row', f.pw, f.text);
      }
      if (f.field_el) row.appendChild(f.field_el);
      if (f.kind !== 'bool') {
        row.appendChild(h('button.btn.sm', { type: 'submit' }, NH.t('common.save')));
        f.form.append(h('label', NH.t(f.label) + (f.unit ? ' (' + NH.t(f.unit) + ')' : '')), row);
      }
      f.form.appendChild(f.hint);
      forms.appendChild(f.form);

      f.input.addEventListener('click', function (e) {
        var why = !f.active(S.meta) && f.blocked(S.meta);
        if (why) { e.preventDefault(); NH.toast(why); }
      });
      f.input.addEventListener('change', function () {
        if (!f.input.checked && f.active(S.meta)) return clear(f);
        if (f.input.checked && f.kind === 'bool') { var b = {}; b[f.field] = true; return save(f, b); }
        f.form.classList.toggle('hide', !f.input.checked);
      });
      f.form.addEventListener('submit', function (e) {
        e.preventDefault();
        var b = {};
        if (f.kind === 'time') {
          var t = new Date(f.field_el.value).getTime();
          if (!t) return;
          b[f.field] = Math.floor(t / 1000);
        }
        if (f.kind === 'number') b[f.field] = +f.field_el.value;
        if (f.kind === 'select') b[f.field] = f.field_el.value;
        if (f.kind === 'list') b[f.field] = f.field_el.value.split(/[\s,]+/).filter(Boolean);
        if (f.kind === 'url') b[f.field] = f.field_el.value.trim();
        if (f.kind === 'decoy') return saveDecoy(f);
        if (f.kind === 'inbox') return saveInbox(f);
        save(f, b);
      });
    });
    built = true;
  }
  function draw(m) {
    if (!built) build();
    $('#share-adv').classList.toggle('hide', !!m.frozen);
    ADV.forEach(function (f) {
      var on = f.active(m), why = on ? '' : f.blocked(m);
      f.input.checked = on || (!why && f.input.checked && f.kind !== 'bool');
      f.pill.classList.toggle('blocked', !!why);
      f.pill.title = why;
      f.form.classList.toggle('hide', !f.input.checked && !on);
      f.hint.textContent = f.state(m);
      if (f.kind === 'time') {
        f.field_el.min = localInput(nowUnix() + 60);
        if (on) f.field_el.value = localInput(m[f.field]);
        else if (!f.field_el.value) f.field_el.value = localInput(nowUnix() + f.def);
      }
      if (f.kind === 'number' && !f.field_el.value) f.field_el.value = m[f.field] || f.def;
      if (f.kind === 'select' && on) f.field_el.value = m[f.field];
      if (f.kind === 'url' && on && document.activeElement !== f.field_el) f.field_el.value = m[f.field];
      if (f.kind === 'list' && on && document.activeElement !== f.field_el) f.field_el.value = (m.ip_allow || []).join('\n');
    });
    if (ADV.some(function (f) { return f.active(m); })) $('#share-adv').open = true;
    $('#trace-box').classList.toggle('hide', !m.watermark);
  }
  function applied(f, m, set) {
    N.applyMeta(m);
    draw(Object.assign({}, S.meta, m));
    NH.toast(NH.t(set ? f.saved : f.cleared), 'ok');
  }
  function save(f, body) {
    N.api('POST', '/config', body).then(function (m) { applied(f, m, true); }, function (x) { NH.toast(x.message, 'err'); draw(S.meta); });
  }
  function clear(f) {
    if (f.kind === 'decoy' || f.kind === 'inbox') {
      return N.api('DELETE', '/' + f.kind).then(function (m) { applied(f, m, false); }, function (x) { NH.toast(x.message, 'err'); draw(S.meta); });
    }
    N.api('POST', '/config', f.clear).then(function (m) { applied(f, m, false); }, function (x) { NH.toast(x.message, 'err'); draw(S.meta); });
  }
  function saveDecoy(f) {
    var pw = f.pw.value;
    if (!pw || pw === N.password()) return NH.toast(NH.t('new.need_decoy'), 'err');
    NHC.decoy(N.key(), pw, S.meta.salt, { t: N.title(), b: f.text.value }, { t: N.title(), b: N.body() }).then(function (d) {
      return N.api('PUT', '/decoy', d);
    }).then(function (m) { f.pw.value = ''; applied(f, m, true); }, function (x) { NH.toast(x.message, 'err'); });
  }
  // Inbox: a key pair made here; the server gets the public half and the
  // private half sealed with the note key (so every reader can open entries).
  function saveInbox(f) {
    NHC.inboxKeys().then(function (k) {
      return S.codec.encode({ jwk: k.jwk }).then(function (priv) { return N.api('POST', '/inbox', { pub: k.pub, priv: priv }); });
    }).then(function (m) { applied(f, m, true); }, function (x) { NH.toast(x.message, 'err'); });
  }
  window.NHAdv = { draw: draw };

  // ---------- version history ----------
  var hist = { list: [], cache: {} };
  $('#act-history').addEventListener('click', function () {
    N.api('GET', '/versions').then(function (r) {
      hist.list = r.versions;
      var range = $('#history-range');
      if (!hist.list.length) {
        $('#history-diff').replaceChildren(h('p.muted', NH.t('history.empty')));
        $('#history-restore').disabled = true;
        range.disabled = true;
      } else {
        range.disabled = false;
        $('#history-restore').disabled = false;
        range.max = hist.list.length - 1;
        range.value = 0;
        showVersion(0);
      }
      $('#history-dlg').showModal();
    }, function (x) { NH.toast(x.message, 'err'); });
  });
  $('#history-range').addEventListener('input', function () { showVersion(+this.value); });
  function loadVersion(v) {
    if (hist.cache[v.version]) return Promise.resolve(hist.cache[v.version]);
    return N.api('GET', '/versions/' + v.version).then(function (r) { return N.codec().decode(r.content); })
      .then(function (d) { hist.cache[v.version] = d; return d; });
  }
  function showVersion(i) {
    var v = hist.list[i];
    $('#history-label').textContent = NH.t('history.version', { n: v.version, time: v.created, ago: NH.ago(v.created) });
    loadVersion(v).then(function (d) {
      if (+$('#history-range').value !== i) return;
      var ops = NHDiff.lines(d.b || '', N.editor() ? N.editor().value : N.body()), st = NHDiff.stats(ops);
      $('#history-label').textContent += ' · ' + NH.t('diff.stats', { added: st.added, removed: st.removed });
      NHDiff.render($('#history-diff'), ops.map(function (o) { return { op: o.op === '+' ? '-' : o.op === '-' ? '+' : '=', text: o.text }; }), { context: 3 });
    }, function () { $('#history-diff').replaceChildren(h('p.calc-err', NH.t('entry.broken'))); });
  }
  $('#history-restore').addEventListener('click', function () {
    var v = hist.list[+$('#history-range').value];
    loadVersion(v).then(function (d) {
      if (!confirm(NH.t('history.confirm', { n: v.version }))) return;
      N.setText(d.t || '', d.b || '');
      $('#history-dlg').close();
      NH.toast(NH.t('history.restored'), 'ok');
    });
  });

  // ---------- suggestions ----------
  var sugEd = null;
  $('#suggest-start').addEventListener('click', function () {
    $('#n-view').classList.add('hide');
    $('#n-edit').classList.remove('hide');
    $('#suggest-start-row').classList.add('hide');
    $('#suggest-bar').classList.remove('hide');
    if (!sugEd) sugEd = NHEditor($('#n-edit [data-editor]'), {});
    sugEd.value = N.body();
    sugEd.focus();
  });
  function endSuggest() {
    $('#n-view').classList.remove('hide');
    $('#n-edit').classList.add('hide');
    $('#suggest-bar').classList.add('hide');
    $('#suggest-start-row').classList.remove('hide');
  }
  $('#suggest-cancel').addEventListener('click', endSuggest);
  $('#suggest-send').addEventListener('click', function () {
    var name = NH.store.get('nh_name', '');
    N.codec().encode({ t: N.title(), b: sugEd.value, n: name }).then(function (c) {
      return N.api('POST', '/suggestions', { content: c, base_version: S.version });
    }).then(function () { NH.toast(NH.t('suggest.sent'), 'ok'); endSuggest(); }, function (x) { NH.toast(x.message, 'err'); });
  });
  function reviewSuggestions() {
    N.api('GET', '/suggestions').then(function (r) {
      return Promise.all(r.suggestions.map(function (s) {
        return N.codec().decode(s.content).then(function (d) { s.d = d; return s; }, function () { return null; });
      }));
    }).then(function (list) {
      var box = $('#suggest-list');
      list = list.filter(Boolean);
      if (!list.length) box.replaceChildren(h('p.muted', NH.t('suggest.none')));
      else box.replaceChildren.apply(box, list.map(function (s) {
        var diff = h('div'), ops = NHDiff.lines(N.editor() ? N.editor().value : N.body(), s.d.b || ''), st = NHDiff.stats(ops);
        NHDiff.render(diff, ops, { context: 2 });
        var accept = h('button.btn.sm.primary', { type: 'button' }, NH.icon('tick'), NH.t('suggest.accept'));
        var reject = h('button.btn.sm', { type: 'button' }, NH.icon('x'), NH.t('suggest.reject'));
        var card = h('div.card.suggestion', h('div.entry-head', h('b', s.d.n || NH.t('entry.anon')),
          h('span.faint.small', NH.ago(s.created) + ' · ' + NH.t('diff.stats', { added: st.added, removed: st.removed }) +
            (s.base_version !== S.version ? ' · ' + NH.t('suggest.outdated') : ''))), diff, h('div.btn-row.end', reject, accept));
        accept.addEventListener('click', function () {
          N.setText(s.d.t != null ? s.d.t : N.title(), s.d.b || '');
          N.api('DELETE', '/suggestions/' + s.id).then(function () { card.remove(); S.meta.suggestions--; N.redraw(); refresh(); NH.toast(NH.t('suggest.accepted'), 'ok'); });
        });
        reject.addEventListener('click', function () {
          N.api('DELETE', '/suggestions/' + s.id).then(function () { card.remove(); S.meta.suggestions--; refresh(); });
        });
        return card;
      }));
      if (!$('#suggest-dlg').open) $('#suggest-dlg').showModal();
    }, function (x) { NH.toast(x.message, 'err'); });
  }

  // ---------- signatures ----------
  var signNames = {};
  function drawSignatures() {
    var on = !!S.meta.sign_off, list = S.signatures || [];
    $('#signs').classList.toggle('hide', !on);
    if (!on) return;
    $('#signs-count').textContent = list.length;
    $('#sign-form').classList.toggle('hide', !N.hasKey() || !!S.meta.frozen || !!S.meta.locked);
    Promise.all(list.map(function (s) {
      if (signNames[s.id] !== undefined) return signNames[s.id];
      return N.codec().decode(s.name).then(function (d) { return (signNames[s.id] = d.n || ''); }, function () { return (signNames[s.id] = '?'); });
    })).then(function (names) {
      var ul = $('#sign-list');
      ul.replaceChildren.apply(ul, list.map(function (s, i) {
        var stale = s.version !== S.version;
        return h('li.sign-row', NH.icon(stale ? 'history' : 'tick'), h('b', names[i] || NH.t('entry.anon')),
          h('span.faint.small', NH.t('sign.signed', { n: s.version, ago: NH.ago(s.created) })),
          stale ? h('span.pill.warn', NH.t('sign.changed')) : null,
          s.snapshot ? h('a.small', { href: NH.noteURL(s.snapshot, { k: N.key() }) }, NH.t('sign.snapshot')) : null);
      }));
    });
  }
  $('#sign-form').addEventListener('submit', function (e) {
    e.preventDefault();
    var name = $('#sign-name').value.trim();
    if (!name) return;
    N.codec().encode({ n: name }).then(function (c) { return N.api('POST', '/signatures', { name: c }); }).then(function (s) {
      S.signatures = (S.signatures || []).concat([s]);
      signNames[s.id] = name;
      $('#sign-name').value = '';
      drawSignatures();
      NH.toast(NH.t('sign.done'), 'ok');
    }, function (x) { NH.toast(x.message, 'err'); });
  });

  // ---------- watermark ----------
  // Copies made through a watermarked link carry the link's token, written
  // in zero-width characters. The admin can paste leaked text to see which
  // link it came from.
  var ZW = ['​', '‌'], ZW_START = '⁠‍', ZW_END = '‍⁠';
  function mark(tok) {
    var bits = Array.prototype.map.call(new TextEncoder().encode(tok), function (b) { return b.toString(2).padStart(8, '0'); }).join('');
    return ZW_START + bits.replace(/[01]/g, function (c) { return ZW[+c]; }) + ZW_END;
  }
  function unmark(text) {
    var m = String(text).match(/⁠‍([​‌]+)‍⁠/);
    if (!m) return null;
    var bits = m[1].replace(/​/g, '0').replace(/‌/g, '1'), bytes = [];
    for (var i = 0; i + 8 <= bits.length; i += 8) bytes.push(parseInt(bits.slice(i, i + 8), 2));
    return new TextDecoder().decode(new Uint8Array(bytes));
  }
  var marked = false;
  function watermark() {
    if (marked || !S.meta.watermark || S.meta.admin) return;
    marked = true;
    var tag = mark(N.token), copy = NH.copy;
    // Inside the text, after the first word, so trimming the ends keeps it.
    function stamp(t) { var i = t.search(/\s/); return i > 0 ? t.slice(0, i) + tag + t.slice(i) : t + tag; }
    NH.copy = function (text, msg) { return copy(/^https?:\/\//.test(text) ? text : stamp(text), msg); };
    $('#n-view').addEventListener('copy', function (e) {
      var sel = String(getSelection());
      if (!sel || !e.clipboardData) return;
      e.clipboardData.setData('text/plain', stamp(sel));
      e.preventDefault();
    });
  }
  $('#trace-input').addEventListener('input', function () {
    var tok = unmark(this.value), out = $('#trace-result');
    if (!this.value.trim()) return (out.textContent = '');
    if (!tok) return (out.textContent = NH.t('trace.none'));
    var l = (S.meta.links || []).filter(function (x) { return x.token === tok; })[0];
    out.textContent = NH.t('trace.found', { link: '/n/' + tok, mode: l ? NH.modeInfo[l.mode].label : '?' });
  });

  // ---------- private margin ----------
  // Your own notes beside any note, sealed with your vault key; stored under
  // a hash of the link so the margin isn't filed under the note's address.
  var marginRef = null, marginTimer = null;
  function sha256hex(s) {
    return crypto.subtle.digest('SHA-256', new TextEncoder().encode(s)).then(function (b) {
      return Array.prototype.map.call(new Uint8Array(b), function (x) { return x.toString(16).padStart(2, '0'); }).join('');
    });
  }
  function margin() {
    var vault = NHC.vault();
    if (!vault) return NH.toast(NH.t('margin.need_vault'), 'err');
    var box = $('#margin');
    if (!box.classList.contains('hide')) { box.classList.add('hide'); return; }
    sha256hex('notehop/margin/' + N.token).then(function (ref) {
      marginRef = ref;
      return NH.api('GET', '/api/margins/' + ref);
    }).then(function (r) {
      return r && r.content ? vault.open(r.content).then(function (d) { return d.m || ''; }) : '';
    }).then(function (text) {
      $('#margin-text').value = text;
      box.classList.remove('hide');
      $('#margin-text').focus();
    }, function (x) { NH.toast(x.message, 'err'); });
  }
  $('#margin-text').addEventListener('input', function () {
    var vault = NHC.vault(), text = this.value;
    $('#margin-state').textContent = NH.t('save.unsaved');
    clearTimeout(marginTimer);
    marginTimer = setTimeout(function () {
      (text ? vault.seal({ m: text }) : Promise.resolve('')).then(function (c) {
        return NH.api('PUT', '/api/margins/' + marginRef, { content: c });
      }).then(function () { $('#margin-state').textContent = NH.t('save.saved'); }, function (x) { $('#margin-state').textContent = x.message; });
    }, 800);
  });
  $('#more-menu .menu-list').addEventListener('click', function (e) {
    if (e.target.closest('[data-act=margin]')) { $('#more-menu').classList.remove('open'); margin(); }
  });

  // ---------- page state ----------
  function refresh() {
    var m = S.meta;
    if (!m) return;
    var canHistory = !m.frozen && !!S.keys && (m.admin || m.mode === 'edit');
    $('#act-history').classList.toggle('hide', !canHistory);
    $('#suggest-start-row').classList.toggle('hide', !(S.mode === 'suggest' && S.keys && !m.frozen && !m.locked) || !$('#suggest-bar').classList.contains('hide'));
    $('#act-margin').classList.toggle('hide', !document.body.dataset.user);
    var sb = $('#suggest-banner');
    if (m.admin && m.suggestions > 0) {
      if (!sb) {
        sb = h('div.banner.info', { id: 'suggest-banner' });
        $('#note').insertBefore(sb, $('#note').firstChild.nextSibling);
      }
      var open = h('button.btn.sm', { type: 'button' }, NH.t('suggest.review'));
      open.addEventListener('click', reviewSuggestions);
      sb.replaceChildren(NH.icon('suggest'), h('span', NH.t('suggest.waiting', { n: m.suggestions })), open);
    } else if (sb) sb.remove();
    var db = $('#decoy-banner');
    if (m.admin && m.decoy && m.decoy.opened) {
      if (!db) { db = h('div.banner.warn', { id: 'decoy-banner' }); $('#note').insertBefore(db, $('#note').firstChild.nextSibling); }
      db.replaceChildren(NH.icon('key'), h('span', NH.t('decoy.used', { n: m.decoy.opened, time: m.decoy.last_opened })));
    }
    drawSignatures();
    watermark();
  }
  window.NHFeat = { refresh: refresh };
  N.onReady(refresh);
})();
