// "New note" page: encrypt in the browser, create, then jump to the note as admin.
(function () {
  'use strict';
  var $ = NH.$, h = NH.h;
  // Keys of self-destructing blocks wait here until the note is created.
  var burnKeys = [];
  var ed = NHEditor($('[data-editor]'), {
    onSave: function () { $('#create').click(); },
    canUpload: function () { return !btn.disabled; },
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

  // ---------- settings ----------
  // One setting, one chip: type, password, deletion, view limit, what readers
  // can do. Each chip says "name: value" and its menu holds everything about
  // that setting, so nothing is in two places. The switches (data-feature) are
  // what gets sent; the menus set them.
  var RULES = [
    { feature: 'password', when: function () { return !on('encrypt'); }, why: NH.t('rule.password_e2e') },
    { feature: 'burn', when: function () { return mode() !== 'read'; }, why: NH.t('rule.burn_read') },
    { feature: 'lock', when: function () { return mode() === 'read'; }, why: NH.t('rule.lock_write') },
    { feature: 'decoy', when: function () { return !on('password'); }, why: NH.t('rule.decoy_password'), quiet: true }
  ];
  function toggle(name) { return form.querySelector('[data-feature="' + name + '"]'); }
  function on(name) { return toggle(name).checked; }
  function set(name, v) { toggle(name).checked = v; }
  function radio(name) { return form.querySelector('[name=' + name + ']:checked').value; }
  function pick(name, value) { var r = form.querySelector('[name=' + name + '][value="' + value + '"]'); if (r) r.checked = true; }
  function mode() { return radio('mode'); }
  function views() { return +radio('opens'); }
  function blockedBy(name) {
    for (var i = 0; i < RULES.length; i++) if (RULES[i].feature === name && RULES[i].when()) return RULES[i];
    return null;
  }

  function summary() {
    var parts = [NH.t('sum.' + mode()), NH.t(on('encrypt') ? 'sum.private' : 'sum.public')];
    if (on('password')) parts.push(NH.t('sum.password'));
    if (on('burn')) parts.push(NH.t('sum.burn', { n: views() }));
    if (on('expires')) parts.push(NH.t('sum.expires', { dur: $('#expires').selectedOptions[0].textContent }));
    if (!on('burn') && !on('expires')) parts.push(NH.t('sum.forever'));
    if (on('capsule') && unixOf('#open-at')) parts.push(NH.t('sum.opens', { time: unixOf('#open-at') }));
    if (on('lock') && unixOf('#lock-at')) parts.push(NH.t('sum.locks', { time: unixOf('#lock-at') }));
    if (on('signoff')) parts.push(NH.t('sum.signoff'));
    $('#summary').textContent = parts.join(' ');
  }

  // A chip's label is always the setting's name and its value; the name never
  // changes, so no setting can hide behind another one's label.
  function label(name, value) { return NH.t('chip.fmt', { name: NH.t(name), value: value }); }
  function chip(id, name, value, isSet, icon) {
    var box = $(id);
    box.querySelector('.pick-val').textContent = label(name, value);
    box.classList.toggle('is-set', !!isSet);
    if (icon) box.querySelector('.pick-ico use').setAttribute('href', '#i-' + icon);
  }
  function drawChips() {
    chip('#opt-kind', 'chip.kind', NH.t(on('encrypt') ? 'kind.enc' : 'kind.basic'), false, on('encrypt') ? 'lock' : 'doc');
    chip('#opt-password', 'chip.password', NH.t(!on('password') ? 'chip.none' : on('decoy') ? 'chip.pw_decoy' : 'chip.pw_set'), on('password'));
    chip('#opt-until', 'chip.delete', on('expires') ? $('#expires').selectedOptions[0].textContent : NH.t('chip.never'), on('expires'));
    chip('#opt-opens', 'chip.opens', views() ? NH.t('new.views', { n: views() }) : NH.t('chip.unlimited'), views() > 0);
    chip('#opt-who', 'chip.who', NH.modeInfo[mode()].label, mode() !== 'read', NH.modeInfo[mode()].icon);
    // What is on under "Other settings" gets a chip of its own: nothing is active without a trace.
    var extras = [], cw = form.querySelector('[name=cw_level]:checked');
    if (on('capsule')) extras.push(['clock', label('feat.capsule', unixOf('#open-at') ? NH.date(unixOf('#open-at')) : '…')]);
    if (on('lock')) extras.push(['lock', label('feat.lock', unixOf('#lock-at') ? NH.date(unixOf('#lock-at')) : '…')]);
    if (on('signoff')) extras.push(['check', NH.t('feat.signoff')]);
    if (on('warning')) extras.push(['shield', label('feat.content_warning', NH.t(cw && cw.value === 'adult' ? 'gate.adult_title' : 'gate.sensitive_title'))]);
    var box = $('#extra-chips');
    box.replaceChildren.apply(box, extras.map(function (x) {
      var b = h('button.btn.chip-btn.is-set', { type: 'button' }, NH.icon(x[0], 'pick-ico'), h('span.pick-val', x[1]));
      b.addEventListener('click', openAdv);
      return b;
    }));
    $('#adv-count').textContent = extras.length;
    $('#adv-count').classList.toggle('hide', !extras.length);
  }

  function applyOptions() {
    // The menus' choices become switches.
    set('encrypt', radio('kind') === 'encrypted');
    set('expires', radio('until') !== 'forever');
    if (radio('until') !== 'forever' && radio('until') !== 'other') $('#expires').value = radio('until');
    set('burn', views() > 0);
    RULES.forEach(function (r) {
      var input = toggle(r.feature), blocked = r.when(), lbl = input.closest('label');
      if (blocked && input.checked) {
        input.checked = false;
        if (r.feature === 'burn') pick('opens', '0');
        if (!r.quiet) NH.toast(r.why);
      }
      if (lbl) { lbl.classList.toggle('blocked', blocked); lbl.title = blocked ? r.why : ''; }
    });
    // A setting that can't be used here stays visible, with the reason and the way to change it.
    $('#why-password').classList.toggle('hide', on('encrypt'));
    $('#why-burn').classList.toggle('hide', mode() === 'read');
    NH.$$('[name=opens]', form).forEach(function (r) {
      var off = mode() !== 'read' && r.value !== '0';
      r.disabled = off;
      r.closest('label').classList.toggle('blocked', off);
    });
    NH.$$('[data-panel]', form).forEach(function (p) { p.classList.toggle('hide', !on(p.dataset.panel)); });
    $('#until-other').classList.toggle('hide', radio('until') !== 'other');
    $('[data-for="encrypt-off"]').classList.toggle('hide', on('encrypt'));
    // Only a basic note can have a link name of its own.
    $('#basic-slug').classList.toggle('hide', on('encrypt'));
    if (on('encrypt')) slug.value = '';
    if (['check', 'suggest'].indexOf(mode()) !== -1) $('#more-modes').open = true;
    drawChips();
    btn.querySelector('span').textContent = NH.t(on('encrypt') ? 'new.create' : 'new.create_public');
    summary();
    slugAdvice();
  }
  NH.$$('[data-feature]', form).forEach(function (input) {
    input.addEventListener('click', function (e) {
      // Switching on what a rule forbids: the reason is in the menu already; elsewhere, say it.
      var r = input.checked && blockedBy(input.dataset.feature);
      if (!r) return;
      e.preventDefault();
      if (input.dataset.feature !== 'password') NH.toast(r.why);
    });
  });
  // A message about a field goes away as soon as the form is touched again.
  function clearErr() { err.classList.add('hide'); }
  form.addEventListener('input', clearErr);
  form.addEventListener('change', function (e) {
    clearErr();
    applyOptions();
    if (e.target.dataset.feature === 'password' && e.target.checked) $('#password').focus();
    if (e.target.dataset.feature === 'decoy' && e.target.checked) $('#decoy-password').focus();
    // A choice closes its menu, unless it needs more input (a duration).
    var menu = e.target.type === 'radio' && e.target.closest('.pick');
    if (menu && !menu.querySelector('.feat:not(.hide)')) menu.open = false;
  });
  $('#to-encrypted').addEventListener('click', function () { pick('kind', 'encrypted'); applyOptions(); });
  $('#to-read').addEventListener('click', function () { pick('mode', 'read'); applyOptions(); });

  // ---------- the chips' menus: one open at a time ----------
  var picks = NH.$$('.pick', form), scrim = $('#scrim'), adv = $('#adv'), advBtn = $('#adv-btn');
  function phone() { return matchMedia('(max-width: 640px)').matches; }
  picks.forEach(function (d) {
    d.addEventListener('toggle', function () {
      d.classList.remove('flip', 'down');
      if (d.open) {
        picks.forEach(function (o) { if (o !== d) o.open = false; });
        // A menu that would leave the screen opens from the other edge, or downwards.
        var pop = d.querySelector('.pick-pop');
        if (pop && !phone()) {
          var r = pop.getBoundingClientRect();
          if (r.right > document.documentElement.clientWidth - 8) d.classList.add('flip');
          if (r.top < 8) d.classList.add('down');
        }
      }
      var any = picks.filter(function (o) { return o.open; })[0];
      // The phone sheets dim the page; the small menus don't.
      scrim.hidden = !any || !phone();
      document.body.classList.toggle('sheet-open', !!any && !scrim.hidden);
    });
  });
  function closePicks() { picks.forEach(function (d) { d.open = false; }); }
  scrim.addEventListener('click', closePicks);
  // "Other settings" is a modal dialog: the browser handles Escape, focus and the page behind it
  // (the close button and the backdrop are handled in app.js).
  function openAdv() { closePicks(); adv.showModal(); advBtn.setAttribute('aria-expanded', 'true'); }
  advBtn.addEventListener('click', openAdv);
  adv.addEventListener('close', function () { advBtn.setAttribute('aria-expanded', 'false'); });
  document.addEventListener('keydown', function (e) {
    var open = picks.filter(function (d) { return d.open; })[0];
    if (e.key === 'Escape' && open) { open.open = false; open.querySelector('summary').focus(); }
  });
  document.addEventListener('pointerdown', function (e) {
    picks.forEach(function (d) { if (d.open && !d.contains(e.target)) d.open = false; });
  });
  form.addEventListener('input', function (e) { if (e.target.type === 'datetime-local') { summary(); drawChips(); } });

  // ---------- starters: presets, never a step before writing ----------
  function starterBody(kind) {
    if (kind === 'checklist') return [1, 2, 3].map(function (i) { return '- [ ] ' + NH.t('start.item') + ' ' + i; }).join('\n') + '\n';
    if (kind === 'form') return ':::form ' + NH.t('start.form') + '\n- ' + NH.t('form.tpl_name') + ' *\n- ' + NH.t('form.tpl_email') + ' (email)\n- ' +
      NH.t('form.tpl_choice') + ' [' + NH.t('form.tpl_yes') + '|' + NH.t('form.tpl_no') + ']\n- ' + NH.t('form.tpl_note') + ' (long)\n:::\n';
    return '';
  }
  var STARTERS = {
    note: { mode: 'read' },
    checklist: { mode: 'check' },
    code: { mode: 'read', type: 'code' },
    web: { mode: 'read', type: 'web' },
    form: { mode: 'append' },
    together: { mode: 'edit' },
    secret: { mode: 'read', opens: 1 },
    signoff: { mode: 'read', signoff: true }
  };
  var startMore = $('#start-more');
  NH.$$('[data-starter]').forEach(function (b) {
    b.addEventListener('click', function () {
      var kind = b.dataset.starter, st = STARTERS[kind];
      NH.$$('[data-starter]').forEach(function (x) { x.setAttribute('aria-pressed', String(x === b)); });
      startMore.classList.toggle('is-set', startMore.contains(b));
      startMore.open = false;
      pick('mode', st.mode);
      // A view limit needs the note locked in the browser.
      if (st.opens) pick('kind', 'encrypted');
      pick('opens', String(st.opens || 0));
      set('signoff', !!st.signoff);
      // Code and Web are editor types; the other starters write a Document.
      var want = st.type || 'doc';
      if (ed.type !== want) ed.setType(st.type ? { y: st.type } : {}, !ed.value.trim());
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

  // A draft handed over by the home page's quick note ("Full mode"), or kept
  // across a trip to the vault page (see the vault notice below).
  try {
    var hand = JSON.parse(sessionStorage.getItem('nh_handoff') || 'null');
    sessionStorage.removeItem('nh_handoff');
    if (hand) {
      $('#title').value = String(hand.t || '').slice(0, 200);
      ed.value = String(hand.b || '');
      pick('mode', hand.m);
      if (hand.o) { pick('kind', 'encrypted'); pick('opens', '1'); }
      applyOptions();
    }
  } catch (x) {}

  // Signed in, but the vault is not open in this browser: the note would go
  // into the account without its key. Say so before the click, with the way to
  // open the vault; the draft comes along.
  var vaultNote = $('#vault-note');
  if (vaultNote && !NHC.vault()) {
    vaultNote.classList.remove('hide');
    vaultNote.querySelector('a').addEventListener('click', function () {
      try { sessionStorage.setItem('nh_handoff', JSON.stringify({ t: $('#title').value, b: ed.value, m: mode() })); } catch (x) {}
      submitted = true;
    });
  }

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

  // The message, and the field it is about: its menu opens and the field gets the focus.
  function fail(msg, field) {
    err.textContent = msg;
    err.classList.remove('hide');
    btn.disabled = false;
    if (field) {
      var menu = field.closest('.pick');
      if (menu) menu.open = true;
      field.focus();
    }
  }

  form.addEventListener('submit', function (e) {
    e.preventDefault();
    clearErr();
    if (ed.uploading) return fail(NH.t('file.uploading', { n: 1 }));
    var title = $('#title').value.trim(), body = ed.value;
    if (!title && !body.trim()) return fail(NH.t('new.empty'));
    var encrypted = on('encrypt'), password = encrypted && on('password') ? $('#password').value : '';
    if (on('password') && !password) return fail(NH.t('new.need_password'), $('#password'));
    var expiresIn = on('expires') ? +$('#expires').value : 0;
    var openAt = on('capsule') ? unixOf('#open-at') : 0, lockAt = on('lock') ? unixOf('#lock-at') : 0;
    var problem = timeProblem(openAt, lockAt, expiresIn);
    if (problem) return fail(problem);
    var burns = burnKeys.filter(function (k) { return body.indexOf(':::burn ' + k.id + ' ') !== -1; });
    if (burns.length && !encrypted) return fail(NH.t('rule.burn_block_e2e'));
    var decoyPw = on('decoy') ? $('#decoy-password').value : '';
    if (on('decoy') && (!decoyPw || decoyPw === password)) return fail(NH.t('new.need_decoy'), $('#decoy-password'));
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
        var cw = 'none', wr = '';
        if (on('warning')) {
          var cwEl = form.querySelector('[name=cw_level]:checked');
          cw = cwEl ? cwEl.value : 'sensitive';
          wr = ($('#warning-reason') ? $('#warning-reason').value : '').trim();
        }
        return NH.api('POST', '/api/notes', {
          encrypted: encrypted, password: !!password, salt: salt, content: r[0], mode: mode,
          upload_draft: $('[data-editor]').dataset.uploadDraft,
          slug: encrypted ? '' : slug.value.trim(), expires_in: expiresIn, max_views: on('burn') ? views() : 0,
          open_at: openAt, lock_at: lockAt, admin: admin, write: keys.write, owner_blob: r[1],
          burns: burns, sign_off: on('signoff'), decoy: decoy,
          content_warning: cw, warning_reason: wr
        });
      });
    }).then(function (res) {
      submitted = true;
      // With the vault closed the key did not go into the account: this device's list is then the
      // only place that still has it (My notes offers to save it), so it is kept whatever the switch says.
      if ($('#remember').checked || (document.body.dataset.user && !NHC.vault())) {
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

  // ---------- on this device (without an account; signed in, the list is in My notes) ----------
  var recentBox = $('#recent');
  function drawRecent() {
    var list = NH.recent.all(), ul = $('#recent-list');
    recentBox.classList.toggle('hide', !list.length);
    ul.replaceChildren.apply(ul, list.map(function (n) {
      var gone = n.expires && n.expires < Date.now() / 1000;
      var rm = h('button.icon-btn.sm', { type: 'button', title: NH.t('new.forget_one'), 'aria-label': NH.t('new.forget_one') }, NH.icon('x'));
      rm.addEventListener('click', function () { NH.recent.remove(n.token); drawRecent(); });
      return h('li.note-row' + (gone ? '.gone' : ''),
        h('a.note-link', { href: NH.noteURL(n.token, { k: n.k, a: n.a }) }, h('b', n.t || NH.t('common.untitled')),
          h('small.muted', '/' + n.token + ' · ' + NH.ago(n.created) + (gone ? ' · ' + NH.t('new.expired') : '') + (n.p ? ' · ' + NH.t('new.has_password') : ''))),
        rm);
    }));
  }
  if (recentBox) {
    $('#recent-clear').addEventListener('click', function () {
      if (confirm(NH.t('new.forget_confirm'))) { NH.recent.clear(); drawRecent(); }
    });
    drawRecent();
  }

  // ---------- first visit: a three-step tour (skippable, shown once) ----------
  // The editor is ready to type in at once; on a touch screen the keyboard is left to the reader's tap.
  function ready() { if (matchMedia('(hover: hover) and (pointer: fine)').matches) ed.focus(); }
  function tour() {
    var steps = [['.editor-card', 'tour.1'], ['#opt-who', 'tour.2'], ['#create', 'tour.3']], i = 0;
    var tip = h('div.tour', { role: 'dialog', 'aria-label': NH.t('tour.label'), 'aria-live': 'polite' });
    document.body.appendChild(tip);
    function esc(e) { if (e.key === 'Escape') { NH.$$('.tour-target').forEach(function (x) { x.classList.remove('tour-target'); }); end(); } }
    function end() { tip.remove(); NH.store.set('nh_tour', '1'); window.removeEventListener('resize', show); document.removeEventListener('keydown', esc); ready(); }
    function show() {
      var target = $(steps[i][0]);
      NH.$$('.tour-target').forEach(function (x) { x.classList.remove('tour-target'); });
      target.classList.add('tour-target');
      var next = h('button.btn.sm.primary', { type: 'button' }, NH.t(i < steps.length - 1 ? 'tour.next' : 'tour.done'));
      var skip = h('button.btn.sm.ghost', { type: 'button' }, NH.t('tour.skip'));
      next.addEventListener('click', function () {
        if (++i >= steps.length) { target.classList.remove('tour-target'); return end(); }
        show();
      });
      skip.addEventListener('click', function () { target.classList.remove('tour-target'); end(); });
      tip.replaceChildren(h('span.tour-step', (i + 1) + ' / ' + steps.length), h('p', NH.t(steps[i][1])), h('div.btn-row', skip, next));
      var r = target.getBoundingClientRect(), top = r.bottom + 10;
      if (top + 160 > innerHeight) top = Math.max(10, r.top - 170);
      tip.style.top = (top + scrollY) + 'px';
      tip.style.left = Math.max(10, Math.min(r.left + scrollX, scrollX + document.documentElement.clientWidth - 330)) + 'px';
      next.focus();
    }
    show();
    window.addEventListener('resize', show);
    document.addEventListener('keydown', esc);
  }
  if (!NH.store.get('nh_tour', '') && !NH.recent.all().length) tour();
  else ready();
})();
