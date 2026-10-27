// "New note" page: encrypt in the browser, create, then jump to the note as admin.
(function () {
  'use strict';
  var $ = NH.$;
  // Keys of self-destructing blocks wait here until the note is created.
  var burnKeys = [];
  var ed = NHEditor($('[data-editor]'), {
    onSave: function () { $('#create').click(); },
    onBurnKey: function (k) { burnKeys.push(k); }
  });
  var form = $('#compose'), err = $('#compose-err'), btn = $('#create');
  var slug = $('#slug'), slugHint = $('#slug-hint');
  var submitted = false;

  if (!NHC.available) {
    err.textContent = NH.t('js.no_webcrypto') + ' ' + NH.t('new.cant_encrypt');
    err.classList.remove('hide');
    btn.disabled = true;
  }

  // ---------- options ----------
  // The friendly controls (who, how private, how long) are presets over the
  // feature switches in Advanced settings; both stay in sync, and the switches
  // are what gets sent.
  var RULES = [
    { feature: 'password', when: function () { return !on('encrypt'); }, why: NH.t('rule.password_e2e') },
    { feature: 'burn', when: function () { return mode() !== 'read'; }, why: NH.t('rule.burn_read') },
    { feature: 'lock', when: function () { return mode() === 'read'; }, why: NH.t('rule.lock_write') },
    { feature: 'decoy', when: function () { return !on('password'); }, why: NH.t('rule.decoy_password') }
  ];
  var PRIV_DESC = { normal: NH.t('priv.normal_d'), password: NH.t('priv.password_d'), once: NH.t('priv.once_d') };
  function toggle(name) { return form.querySelector('[data-feature="' + name + '"]'); }
  function on(name) { return toggle(name).checked; }
  function set(name, v) { toggle(name).checked = v; }
  function mode() { return form.querySelector('[name=mode]:checked').value; }
  function radio(name) { return form.querySelector('[name=' + name + ']:checked').value; }
  function pick(name, value) { var r = form.querySelector('[name=' + name + '][value="' + value + '"]'); if (r) r.checked = true; }
  function blockedBy(name) {
    for (var i = 0; i < RULES.length; i++) if (RULES[i].feature === name && RULES[i].when()) return RULES[i];
    return null;
  }

  // Presets → switches.
  function applyPrivacy() {
    var p = radio('privacy');
    if (p === 'once' && blockedBy('burn')) { NH.toast(blockedBy('burn').why); pick('privacy', 'normal'); p = 'normal'; }
    if (p === 'password' && blockedBy('password')) { NH.toast(blockedBy('password').why); pick('privacy', 'normal'); p = 'normal'; }
    set('password', p === 'password');
    set('burn', p === 'once');
    if (p === 'once') $('#burn').value = '1';
  }
  function applyUntil() {
    var u = radio('until');
    set('expires', u !== 'forever');
    if (u !== 'forever' && u !== 'other') $('#expires').value = u;
  }
  // Switches → presets (after someone changed a switch directly).
  function syncPresets() {
    pick('privacy', on('burn') ? 'once' : on('password') ? 'password' : 'normal');
    var u = !on('expires') ? 'forever' : ($('#expires').value === '86400' || $('#expires').value === '604800') && radio('until') !== 'other' ? $('#expires').value : 'other';
    pick('until', u);
  }

  function summary() {
    var parts = [NH.t('sum.' + mode()), NH.t(on('encrypt') ? 'sum.private' : 'sum.public')];
    if (on('password')) parts.push(NH.t('sum.password'));
    if (on('burn')) parts.push(NH.t('sum.burn', { n: $('#burn').value }));
    if (on('expires')) parts.push(NH.t('sum.expires', { dur: $('#expires').selectedOptions[0].textContent }));
    if (!on('burn') && !on('expires')) parts.push(NH.t('sum.forever'));
    if (on('capsule') && unixOf('#open-at')) parts.push(NH.t('sum.opens', { time: unixOf('#open-at') }));
    if (on('lock') && unixOf('#lock-at')) parts.push(NH.t('sum.locks', { time: unixOf('#lock-at') }));
    if (on('signoff')) parts.push(NH.t('sum.signoff'));
    $('#summary').textContent = parts.join(' ');
  }

  function applyOptions() {
    RULES.forEach(function (r) {
      var input = toggle(r.feature), blocked = r.when(), label = input.closest('label');
      if (blocked && input.checked) {
        input.checked = false;
        NH.toast(r.why);
      }
      label.classList.toggle('blocked', blocked);
      label.title = blocked ? r.why : '';
    });
    syncPresets();
    NH.$$('[data-panel]', form).forEach(function (p) { p.classList.toggle('hide', !on(p.dataset.panel)); });
    $('#until-other').classList.toggle('hide', radio('until') !== 'other');
    $('#priv-desc').textContent = PRIV_DESC[radio('privacy')];
    $('[data-for="encrypt-off"]').classList.toggle('hide', on('encrypt'));
    // Extra settings that are on keep the section open, so nothing is hidden.
    if (['capsule', 'lock', 'decoy', 'slug', 'signoff'].some(on) || !on('encrypt')) $('#adv').open = true;
    if (['check', 'suggest'].indexOf(mode()) !== -1) $('#more-modes').open = true;
    btn.querySelector('span').textContent = NH.t(on('encrypt') ? 'new.create' : 'new.create_public');
    summary();
    slugAdvice();
  }
  NH.$$('[data-feature]', form).forEach(function (input) {
    input.addEventListener('click', function (e) {
      var r = blockedBy(input.dataset.feature);
      if (r && !input.checked) { e.preventDefault(); NH.toast(r.why); }
    });
  });
  form.addEventListener('change', function (e) {
    if (e.target.name === 'privacy') applyPrivacy();
    if (e.target.name === 'until' || e.target.id === 'expires') applyUntil();
    applyOptions();
  });
  form.addEventListener('input', function (e) { if (e.target.type === 'datetime-local') summary(); });

  // ---------- "What do you want to make?" ----------
  function starterBody(kind) {
    if (kind === 'checklist') return [1, 2, 3].map(function (i) { return '- [ ] ' + NH.t('start.item') + ' ' + i; }).join('\n') + '\n';
    if (kind === 'form') return ':::form ' + NH.t('start.form') + '\n- ' + NH.t('form.tpl_name') + ' *\n- ' + NH.t('form.tpl_email') + ' (email)\n- ' +
      NH.t('form.tpl_choice') + ' [' + NH.t('form.tpl_yes') + '|' + NH.t('form.tpl_no') + ']\n- ' + NH.t('form.tpl_note') + ' (long)\n:::\n';
    return '';
  }
  var STARTERS = {
    note: { mode: 'read', privacy: 'normal' },
    checklist: { mode: 'check', privacy: 'normal' },
    together: { mode: 'edit', privacy: 'normal' },
    secret: { mode: 'read', privacy: 'once' },
    form: { mode: 'append', privacy: 'normal' },
    signoff: { mode: 'read', privacy: 'normal', signoff: true }
  };
  NH.$$('[data-starter]').forEach(function (b) {
    b.addEventListener('click', function () {
      var kind = b.dataset.starter, st = STARTERS[kind];
      NH.$$('[data-starter]').forEach(function (x) { x.setAttribute('aria-pressed', String(x === b)); });
      pick('mode', st.mode);
      pick('privacy', st.privacy);
      applyPrivacy();
      set('signoff', !!st.signoff);
      // Only fill an empty editor: never throw away what someone wrote.
      if (!ed.value.trim() && starterBody(kind)) ed.value = starterBody(kind);
      applyOptions();
      ed.focus();
    });
  });

  // Technical names next to the friendly ones, for people who know them.
  var expert = $('#expert');
  expert.checked = NH.store.get('nh_expert', '') === '1';
  document.body.classList.toggle('expert', expert.checked);
  expert.addEventListener('change', function () {
    NH.store.set('nh_expert', expert.checked ? '1' : '');
    document.body.classList.toggle('expert', expert.checked);
  });

  function slugAdvice() {
    slug.value = slug.value.toLowerCase().replace(/[^a-z0-9-]/g, '-').replace(/^-+/, '');
    if (slug.value && mode() !== 'read') {
      slugHint.textContent = NH.t('new.slug_warn', { mode: NH.modeInfo[mode()].label });
      slugHint.classList.add('warn');
    } else {
      slugHint.textContent = NH.t('new.slug_hint');
      slugHint.classList.remove('warn');
    }
  }
  slug.addEventListener('input', slugAdvice);

  function localInput(unix) {
    var d = new Date(unix * 1000), p = function (n) { return String(n).padStart(2, '0'); };
    return d.getFullYear() + '-' + p(d.getMonth() + 1) + '-' + p(d.getDate()) + 'T' + p(d.getHours()) + ':' + p(d.getMinutes());
  }
  function unixOf(id) {
    var t = new Date($(id).value).getTime();
    return t ? Math.floor(t / 1000) : 0;
  }
  var soon = Math.floor(Date.now() / 1000);
  $('#open-at').value = localInput(soon + 86400);
  $('#lock-at').value = localInput(soon + 7 * 86400);
  applyOptions();

  // Web clipper: the bookmarklet passes the page in the #fragment.
  if (location.hash.indexOf('#clip=') === 0) {
    try {
      var clip = JSON.parse(decodeURIComponent(location.hash.slice(6)));
      $('#title').value = String(clip.t || '').slice(0, 200);
      var quote = String(clip.s || '').trim();
      ed.value = (quote ? quote.split('\n').map(function (l) { return '> ' + l; }).join('\n') + '\n\n' : '') +
        NH.t('clip.source') + ': ' + String(clip.u || '');
    } catch (x) {}
    history.replaceState(null, '', location.pathname);
  }

  // A draft handed over by the home page's quick note ("Full mode").
  try {
    var hand = JSON.parse(sessionStorage.getItem('nh_handoff') || 'null');
    sessionStorage.removeItem('nh_handoff');
    if (hand) {
      $('#title').value = String(hand.t || '').slice(0, 200);
      ed.value = String(hand.b || '');
      var hm = document.querySelector('input[name=mode][value="' + hand.m + '"]');
      if (hm) { hm.checked = true; hm.dispatchEvent(new Event('change', { bubbles: true })); }
      if (hand.o) { var po = document.querySelector('input[name=privacy][value=once]'); if (po) { po.checked = true; po.dispatchEvent(new Event('change', { bubbles: true })); } }
    }
  } catch (x) {}

  // Times are checked together so they can't contradict each other.
  function timeProblem(openAt, lockAt, expiresIn) {
    var now = Math.floor(Date.now() / 1000);
    if (on('capsule') && openAt <= now) return NH.t('err.open_at_range');
    if (on('lock') && lockAt <= now) return NH.t('err.lock_at_range');
    if (on('capsule') && on('lock') && lockAt <= openAt) return NH.t('err.lock_before_open');
    if (on('capsule') && expiresIn && now + expiresIn <= openAt) return NH.t('err.expires_before_open');
    if (on('lock') && expiresIn && now + expiresIn <= lockAt) return NH.t('err.expires_before_lock');
    return '';
  }

  function fail(msg) {
    err.textContent = msg;
    err.classList.remove('hide');
    btn.disabled = false;
  }

  form.addEventListener('submit', function (e) {
    e.preventDefault();
    err.classList.add('hide');
    var title = $('#title').value.trim(), body = ed.value;
    if (!title && !body.trim()) return fail(NH.t('new.empty'));
    var encrypted = on('encrypt'), password = encrypted && on('password') ? $('#password').value : '';
    if (on('password') && !password) return fail(NH.t('new.need_password'));
    if (on('slug') && !slug.value.trim()) return fail(NH.t('new.need_slug'));
    var expiresIn = on('expires') ? +$('#expires').value : 0;
    var openAt = on('capsule') ? unixOf('#open-at') : 0, lockAt = on('lock') ? unixOf('#lock-at') : 0;
    var problem = timeProblem(openAt, lockAt, expiresIn);
    if (problem) return fail(problem);
    var burns = burnKeys.filter(function (k) { return body.indexOf(':::burn ' + k.id + ' ') !== -1; });
    if (burns.length && !encrypted) return fail(NH.t('rule.burn_block_e2e'));
    var decoyPw = on('decoy') ? $('#decoy-password').value : '';
    if (on('decoy') && (!decoyPw || decoyPw === password)) return fail(NH.t('new.need_decoy'));
    if (!encrypted && NH.leaksKeys(title + '\n' + body) && !confirm(NH.t('warn.public_keys'))) return;
    var mode = form.querySelector('[name=mode]:checked').value;
    var k = NHC.newKey(), admin = NHC.newKey(), salt = password ? NHC.newSalt() : '';
    var decoy = null;
    btn.disabled = true;

    NHC.noteKeys(k, password, salt).then(function (keys) {
      var codec = NHC.codec(encrypted, keys.aes), vault = NHC.vault();
      return Promise.all([
        codec.encode(Object.assign({ t: title, b: body }, ed.fields())),
        vault ? vault.seal({ t: title, k: k, a: admin, p: password }) : '',
        decoyPw ? NHC.decoy(k, decoyPw, salt, { t: title, b: $('#decoy-text').value }, { t: title, b: body }) : null
      ]).then(function (r) {
        decoy = r[2];
        return NH.api('POST', '/api/notes', {
          encrypted: encrypted, password: !!password, salt: salt, content: r[0], mode: mode,
          slug: on('slug') ? slug.value.trim() : '', expires_in: expiresIn, max_views: on('burn') ? +$('#burn').value : 0,
          open_at: openAt, lock_at: lockAt, admin: admin, write: keys.write, owner_blob: r[1],
          burns: burns, sign_off: on('signoff'), decoy: decoy
        });
      });
    }).then(function (res) {
      submitted = true;
      if ($('#remember').checked) {
        NH.recent.add({ token: res.token, k: k, a: admin, t: title, created: Math.floor(Date.now() / 1000), expires: res.expires, p: !!password });
      }
      // Hand the password to the next page without putting it in the URL.
      if (password) try { sessionStorage.setItem('nh_pw_' + res.token, password); } catch (x) {}
      location.href = NH.noteURL(res.token, { k: k, a: admin, new: '1' });
    }).catch(function (x) { fail(x.message || NH.t('err.create_failed')); });
  });

  window.addEventListener('beforeunload', function (e) {
    if (!submitted && (ed.value.trim() || $('#title').value.trim())) { e.preventDefault(); e.returnValue = ''; }
  });

  // ---------- on this device ----------
  function drawRecent() {
    var list = NH.recent.all(), box = $('#recent'), ul = $('#recent-list');
    box.classList.toggle('hide', !list.length);
    ul.replaceChildren.apply(ul, list.map(function (n) {
      var gone = n.expires && n.expires < Date.now() / 1000;
      var rm = NH.h('button.icon-btn.sm', { type: 'button', title: NH.t('new.forget_one'), 'aria-label': NH.t('new.forget_one') }, NH.icon('x'));
      rm.addEventListener('click', function () { NH.recent.remove(n.token); drawRecent(); });
      return NH.h('li.note-row' + (gone ? '.gone' : ''),
        NH.h('a.note-link', { href: NH.noteURL(n.token, { k: n.k, a: n.a }) }, NH.h('b', n.t || NH.t('common.untitled')),
          NH.h('small.muted', '/n/' + n.token + ' · ' + NH.ago(n.created) + (gone ? ' · ' + NH.t('new.expired') : '') + (n.p ? ' · ' + NH.t('new.has_password') : ''))),
        rm);
    }));
  }
  $('#recent-clear').addEventListener('click', function () {
    if (confirm(NH.t('new.forget_confirm'))) { NH.recent.clear(); drawRecent(); }
  });
  drawRecent();

  // ---------- first visit: a three-step tour (skippable, shown once) ----------
  function tour() {
    var steps = [['.editor-card', 'tour.1'], ['#opt-who', 'tour.2'], ['#create', 'tour.3']], i = 0;
    var tip = NH.h('div.tour', { role: 'dialog', 'aria-live': 'polite' });
    document.body.appendChild(tip);
    function end() { tip.remove(); NH.store.set('nh_tour', '1'); window.removeEventListener('resize', show); }
    function show() {
      var target = $(steps[i][0]);
      NH.$$('.tour-target').forEach(function (x) { x.classList.remove('tour-target'); });
      target.classList.add('tour-target');
      var next = NH.h('button.btn.sm.primary', { type: 'button' }, NH.t(i < steps.length - 1 ? 'tour.next' : 'tour.done'));
      var skip = NH.h('button.btn.sm.ghost', { type: 'button' }, NH.t('tour.skip'));
      next.addEventListener('click', function () {
        if (++i >= steps.length) { target.classList.remove('tour-target'); return end(); }
        show();
      });
      skip.addEventListener('click', function () { target.classList.remove('tour-target'); end(); });
      tip.replaceChildren(NH.h('span.tour-step', (i + 1) + ' / ' + steps.length), NH.h('p', NH.t(steps[i][1])), NH.h('div.btn-row', skip, next));
      var r = target.getBoundingClientRect(), top = r.bottom + 10;
      if (top + 160 > innerHeight) top = Math.max(10, r.top - 170);
      tip.style.top = (top + scrollY) + 'px';
      tip.style.left = Math.max(10, Math.min(r.left + scrollX, scrollX + document.documentElement.clientWidth - 330)) + 'px';
      next.focus();
    }
    show();
    window.addEventListener('resize', show);
  }
  if (!NH.store.get('nh_tour', '') && !NH.recent.all().length) tour();
  else $('#title').focus();
})();
