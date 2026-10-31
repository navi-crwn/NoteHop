// Home page. The quick note is also the demo: it is encrypted as you type,
// with the key the note will get, so the dark box shows exactly what the
// server receives when you press Create. The rest is motion (motion.js):
// a note's life and the feature tour follow the scroll, small loops show
// each feature, and the open-source numbers come from /api/repo.
(function () {
  'use strict';
  var $ = NH.$, h = NH.h, M = window.NHMotion || { once: function (e, f) { f(); }, scrub: function (e, f) { f(1); }, reduce: true, type: function (e, t) { e.textContent = t; }, count: function (e, n) { e.textContent = n; } };
  var reduce = M.reduce, wide = function () { return innerWidth > 960; };

  // ---------- quick note ----------
  (function quick() {
    var form = $('#quick-form');
    if (!form) return;
    var title = $('#quick-title'), body = $('#quick-body'), once = $('#quick-once'), err = $('#quick-err');
    var cipher = $('#quick-cipher'), size = $('#quick-size'), frag = $('#quick-frag'), url = $('#quick-url');
    var KEY = 'nh_quick', k = NHC.available ? NHC.newKey() : '', keysP = k ? NHC.noteKeys(k, '', '') : null;
    function mode() { return (form.querySelector('input[name=quick-mode]:checked') || {}).value || 'read'; }
    function draft() { return { t: title.value, b: body.value, m: mode(), o: once.checked }; }
    var d = NH.store.get(KEY, null);
    if (d) {
      title.value = d.t || ''; body.value = d.b || ''; once.checked = !!d.o;
      var r = form.querySelector('input[name=quick-mode][value="' + d.m + '"]');
      if (r) r.checked = true;
    } else if (!title.value && !body.value) {
      title.value = NH.t('home.l_note_t');
      body.value = $('#h-paper-text') ? $('#h-paper-text').dataset.text : '';
    }
    url.textContent = (document.body.dataset.share ? new URL(document.body.dataset.share).host : location.host) + '/…';
    if (k) frag.textContent = '#k=' + k.slice(0, 18) + '…';

    // The preview: real AES-GCM with this note's key, letters shuffling into place.
    var ABC = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_', shuffle = 0, wait = 0, last = null;
    function show(text) {
      var n = 0, steps = reduce ? 1 : 10;
      clearInterval(shuffle);
      shuffle = setInterval(function () {
        n++;
        var keep = Math.floor(text.length * n / steps), s = text.slice(0, keep);
        for (var i = keep; i < text.length; i++) s += ABC[(Math.random() * 64) | 0];
        cipher.textContent = s;
        if (n >= steps) clearInterval(shuffle);
      }, 28);
    }
    function preview() {
      if (!keysP) { cipher.textContent = NH.t('js.no_webcrypto'); return; }
      clearTimeout(wait);
      wait = setTimeout(function () {
        var t = title.value.trim(), b = body.value;
        keysP.then(function (keys) { return NHC.codec(true, keys.aes).encode({ t: t, b: b }); }).then(function (c) {
          last = { t: t, b: b, c: c };
          size.textContent = NH.t('home.d_bytes', { n: c.length.toLocaleString(NH.lang) });
          show(c.length > 560 ? c.slice(0, 560) + '…' : c);
        });
      }, 90);
    }
    form.addEventListener('input', function () { NH.store.set(KEY, draft()); preview(); });
    form.addEventListener('change', function () { NH.store.set(KEY, draft()); });
    preview();

    // Full mode: /new reads the draft once (see new.js) and the home page forgets it.
    $('#quick-full').addEventListener('click', function () {
      try { sessionStorage.setItem('nh_handoff', JSON.stringify(draft())); } catch (e) {}
      NH.store.del(KEY);
    });
    form.addEventListener('submit', function (e) {
      e.preventDefault();
      err.classList.add('hide');
      var t = title.value.trim(), b = body.value;
      if (!t && !b.trim()) { err.textContent = NH.t('home.quick_empty'); err.classList.remove('hide'); return body.focus(); }
      if (!keysP) return;
      var btn = $('#quick-create'), admin = NHC.newKey(), vault = NHC.vault();
      btn.disabled = true;
      keysP.then(function (keys) {
        // Send the very ciphertext the preview shows, unless the text changed since.
        var c = last && last.t === t && last.b === b ? last.c : NHC.codec(true, keys.aes).encode({ t: t, b: b });
        return Promise.all([c, vault ? vault.seal({ t: t, k: k, a: admin, p: '' }) : '']).then(function (r) {
          return NH.api('POST', '/api/notes', { encrypted: true, password: false, salt: '', content: r[0], mode: mode(), slug: '',
            expires_in: 0, max_views: once.checked ? 1 : 0, admin: admin, write: keys.write, owner_blob: r[1],
            content_warning: $('#quick-warning').value, warning_reason: $('#quick-warning-reason').value });
        });
      }).then(function (res) {
        NH.recent.add({ token: res.token, k: k, a: admin, t: t, created: Math.floor(Date.now() / 1000), expires: res.expires });
        NH.store.del(KEY);
        location.href = NH.noteURL(res.token, { k: k, a: admin, new: '1' });
      }, function (x) { btn.disabled = false; err.textContent = x.message; err.classList.remove('hide'); });
    });
  })();

  // ---------- hero: the gradient phrase turns into ciphertext as you scroll away ----------
  (function scramble() {
    var em = $('#h-title em'), hero = $('.h-hero');
    if (!em || reduce) return;
    var plain = em.textContent, ABC = 'x1.Qm9yPq8uZ1hT0dGlhc2EtYmFyYS1rZXlr';
    $('#h-title').setAttribute('aria-label', $('#h-title').textContent);
    var last = -1;
    addEventListener('scroll', function () {
      var p = Math.min(1, Math.max(0, scrollY / (hero.offsetHeight * .8)));
      var n = Math.round(plain.length * p);
      if (n === last) return;
      last = n;
      var s = '';
      for (var i = 0; i < plain.length; i++) s += i < n && plain[i] !== ' ' ? ABC[(i * 7 + n) % ABC.length] : plain[i];
      em.textContent = s;
    }, { passive: true });
  })();

  // ---------- a note's life ----------
  (function life() {
    var sec = $('#h-life');
    if (!sec) return;
    var steps = NH.$$('#h-steps li'), bar = $('#h-life-bar'), paper = $('#h-paper'), ptext = $('#h-paper-text'), chip = $('#h-chiplink');
    var phone = $('#h-phone'), pbody = $('#h-phone-body'), fire = $('#h-fire'), gone = $('#h-gone'), waitP = pbody.innerHTML;
    var NOTE = ptext.dataset.text, CIPH = 'x1.Qm9yPq8uZ1hT0dGlhc2EtYmFyYS1rZXlraW5nLWJpbmRhci1wYXBhbi1yb2RhLW1lc2FubGFoYWk';
    var title = paper.querySelector('b').textContent;
    function render(p) {
      var step = Math.min(4, Math.floor(p * 5)), f = p * 5 - step;
      steps.forEach(function (li, i) { li.classList.toggle('on', i === step); });
      bar.style.width = (p * 100).toFixed(1) + '%';
      paper.classList.toggle('locked', step >= 1);
      if (step === 0) ptext.replaceChildren(NOTE.slice(0, Math.round(NOTE.length * Math.min(1, f * 1.3))), h('span.h-caret'));
      else if (step === 1) { var k = Math.round(NOTE.length * Math.min(1, f * 1.4)); ptext.textContent = CIPH.slice(0, k) + NOTE.slice(k); }
      else ptext.textContent = CIPH;
      var move = step === 2 ? Math.min(1, f * 1.3) : step > 2 ? 1 : 0, dir = document.documentElement.dir === 'rtl' ? -1 : 1;
      chip.style.transform = 'translate(' + (dir * move * 120) + '%, ' + (-move * 260) + '%) scale(' + (1 - move * .5) + ')';
      chip.style.opacity = step === 2 ? String(1 - Math.max(0, move - .85) * 6) : '0';
      paper.style.transform = step >= 3 ? 'translateX(' + (-dir * 8) + '%) scale(.92)' : '';
      paper.style.opacity = step >= 3 ? '.55' : '1';
      if (step >= 3 && !pbody.dataset.on) { pbody.dataset.on = '1'; pbody.replaceChildren(h('h3', title), h('p', NOTE)); }
      if (step < 3 && pbody.dataset.on) { delete pbody.dataset.on; pbody.innerHTML = waitP; }
      phone.style.transform = step >= 3 ? 'translateY(-2%) scale(1.04)' : '';
      var burn = step === 4 ? Math.min(1, f * 1.4) : 0;
      fire.style.height = (burn * 110) + '%';
      pbody.style.opacity = String(1 - burn);
      gone.style.opacity = String(Math.max(0, burn - .5) * 2);
    }
    if (reduce) return render(.62);
    if (wide()) return M.scrub(sec, function (p) { render(Math.min(.999, p)); });
    // Narrow screens don't pin: the story plays by itself once it is in view.
    render(0);
    M.once(sec, function () { var p = 0; setInterval(function () { p = p >= 1 ? 0 : p + .004; render(Math.min(.999, p)); }, 30); }, .7);
  })();

  // ---------- feature tour ----------
  var MINI = {
    share: function (el) {
      var L = [[NH.t('mode.read'), '', '/n/RFbN3uTvAH'], [NH.t('mode.edit'), ' w', '/n/k82LmQ0zPa'], [NH.t('mode.append'), ' a', '/n/team-notes-2026']];
      var col = h('div'), rows = L.map(function (l) { var d = h('div.mlink', h('span.mmode' + l[1].replace(' ', '.'), l[0]), h('code', { dir: 'ltr' }, l[2])); col.appendChild(d); return d; });
      var cv = h('canvas', { width: 25, height: 25, 'aria-hidden': 'true' });
      el.append(h('div.mshare', col, h('figure.mqr', cv, h('figcaption', NH.t('home.m_qr')))));
      var g = cv.getContext('2d'), seed = 7, cells = [], n = 0, t = 0;
      function rnd() { seed = (seed * 9301 + 49297) % 233280; return seed / 233280; }
      function eye(x, y) { return (x < 7 && y < 7) || (x > 17 && y < 7) || (x < 7 && y > 17); }
      function ring(x, y) { var a = x % 18, b = y % 18; return a === 0 || a === 6 || b === 0 || b === 6 || (a > 1 && a < 5 && b > 1 && b < 5); }
      for (var y = 0; y < 25; y++) for (var x = 0; x < 25; x++) cells.push([x, y, eye(x, y) ? ring(x, y) : rnd() > .52]);
      return setInterval(function () {
        t++;
        for (var i = 0; i < 24 && n < cells.length; i++, n++) { var c = cells[n]; g.fillStyle = c[2] ? '#0d0f17' : '#fff'; g.fillRect(c[0], c[1], 1, 1); }
        if (t === 60) rows[2].classList.add('off');
        if (t === 110) { rows[2].classList.remove('off'); n = 0; g.clearRect(0, 0, 25, 25); t = 0; }
      }, 40);
    },
    private: function (el) {
      var cnt = h('div.mbig', '3'), lab = h('p.h-small', NH.t('home.m_left'));
      var clock = h('code'), dots = h('span.mdots');
      el.append(h('div.mrow', cnt, lab), h('div.mlink', h('span.mmode', NH.t('home.f_private_2')), clock), h('div.mlink', h('span.mmode.w', NH.t('home.f_private_3')), dots));
      el.children[1].style.marginTop = '16px';
      var left = 3 * 3600 - 19, k = 0;
      function tick() {
        k++; left--;
        var p = [Math.floor(left / 3600), Math.floor(left % 3600 / 60), left % 60].map(function (x) { return String(x).padStart(2, '0'); }).join(':');
        clock.textContent = NH.t('home.m_opens', { t: p });
        if (k % 4 === 0) { var n = +cnt.textContent; cnt.textContent = n > 0 ? String(n - 1) : '3'; lab.textContent = NH.t(n === 1 ? 'home.m_gone' : 'home.m_left'); }
        dots.textContent = '•'.repeat(k % 10);
      }
      tick();
      return setInterval(tick, 500);
    },
    together: function (el) {
      var a = h('span'), b = h('span'), doc = h('div.mdoc'), chat = h('div.mchat'), who = NH.t('home.names').split('|');
      doc.append(NH.t('home.m_t_1') + ' ', a, h('span.mcur.a', { 'data-n': who[0] }), '\n' + NH.t('home.m_t_2') + ' ', b, h('span.mcur.b', { 'data-n': who[1] }));
      el.append(doc, chat);
      var A = NH.t('home.m_t_a'), B = NH.t('home.m_t_b'), C = NH.t('home.m_t_chat').split('|'), i = 0, j = 0, t = 0;
      return setInterval(function () {
        t++;
        if (i < A.length && t % 2) a.textContent = A.slice(0, ++i);
        if (t > 14 && j < B.length && t % 2 === 0) b.textContent = B.slice(0, ++j);
        if (t === 30) chat.appendChild(h('div.mbub', who[1] + ': ' + C[0]));
        if (t === 46) chat.appendChild(h('div.mbub.me', who[0] + ': ' + (C[1] || '')));
        if (t > 90) { t = i = j = 0; a.textContent = b.textContent = ''; chat.replaceChildren(); }
      }, 90);
    },
    blocks: function (el) {
      var items = NH.t('home.m_b_items').split('|').map(function (x) { var d = h('div.mtask', h('i'), x); el.appendChild(d); return d; });
      var poll = h('div.mpoll'), pct = [62, 28, 10];
      el.appendChild(poll);
      var bars = NH.t('home.m_b_poll').split('|').map(function (o, i) { var bar = h('i'); poll.appendChild(h('div', bar, h('span', o + ' · ' + pct[i] + '%'))); return bar; });
      var t = 0;
      return setInterval(function () {
        t++;
        if (t <= items.length) items[t - 1].classList.add('done');
        if (t === 2) bars.forEach(function (b, i) { b.style.width = pct[i] + '%'; });
        if (t > 9) { t = 0; items.forEach(function (x) { x.classList.remove('done'); }); bars.forEach(function (b) { b.style.width = '0'; }); }
      }, 700);
    },
    dev: function (el) {
      var term = h('div.mterm'), host = location.host;
      el.appendChild(term);
      var L = [['p', '$ notehop new -t "Server" < notes.txt'], ['o', 'https://' + host + '/n/qR7xP2mLzA#k=8f…  # ' + NH.t('home.m_d_1')], ['p', '$ curl ' + host + '/raw/server-status'],
        ['o', NH.t('home.m_d_2')], ['p', '$ curl ' + host + '/mock/menu/0/name'], ['o', JSON.stringify(NH.t('home.m_d_3'))]], li = 0, ci = 0, cur = null;
      return setInterval(function () {
        if (li >= L.length) { if (++ci > 30) { term.replaceChildren(); li = ci = 0; } return; }
        if (!cur) { cur = h('span.' + L[li][0]); term.appendChild(cur); ci = 0; }
        if (L[li][0] === 'o') { cur.textContent = L[li][1] + '\n'; cur = null; li++; return; }
        cur.textContent = L[li][1].slice(0, ++ci);
        if (ci >= L[li][1].length) { cur.textContent += '\n'; cur = null; li++; }
      }, 45);
    },
    web: function (el) {
      var pre = h('pre'), btn = h('button', { type: 'button', tabindex: '-1' }, NH.t('home.m_w_btn')), out = h('p', NH.t('home.m_w_out', { n: 0 }));
      pre.textContent = "let n = 0;\nbtn.onclick = () => {\n  n++;\n  out.textContent =\n    'Clicked ' + n;\n};";
      el.append(h('div.mcode', pre, h('div.mprev', h('h4', NH.t('home.m_w_h')), btn, out)), h('div.mai', h('del', NH.t('home.m_w_before')), h('ins', NH.t('home.m_w_after'))));
      var n = 0;
      return setInterval(function () { n = n >= 8 ? 1 : n + 1; btn.style.transform = 'scale(.94)'; setTimeout(function () { btn.style.transform = ''; }, 120); out.textContent = NH.t('home.m_w_out', { n: n }); }, 1100);
    }
  };
  (function tour() {
    var sec = $('#h-tour');
    if (!sec) return;
    var tabs = NH.$$('.h-tabs [role=tab]', sec), panels = NH.$$('.h-panel', sec), dots = NH.$$('.h-dot', sec);
    var prev = $('.h-tprev', sec), next = $('.h-tnext', sec), cur = -1, loop = 0, n = tabs.length, pinned = false;
    function show(i) {
      i = Math.max(0, Math.min(n - 1, i));
      if (i === cur) return;
      cur = i;
      tabs.forEach(function (t, k) { t.setAttribute('aria-selected', k === i ? 'true' : 'false'); t.tabIndex = k === i ? 0 : -1; });
      dots.forEach(function (d, k) { if (k === i) d.setAttribute('aria-current', 'true'); else d.removeAttribute('aria-current'); });
      panels.forEach(function (p, k) { p.classList.toggle('hide', k !== i); });
      prev.disabled = i === 0;
      next.disabled = i === n - 1;
      clearInterval(loop);
      var box = panels[i].querySelector('.h-mini-in');
      box.replaceChildren();
      loop = MINI[panels[i].dataset.g](box);
      if (reduce) clearInterval(loop);
    }
    // While pinned, a group is one slice of the section's scroll; going to a
    // group scrolls there, so the dots, the tabs and the scroll always agree.
    function go(i) {
      i = Math.max(0, Math.min(n - 1, i));
      if (!pinned) return show(i);
      var top = sec.getBoundingClientRect().top + scrollY, total = sec.offsetHeight - innerHeight;
      scrollTo({ top: top + total * (i + .5) / n, behavior: reduce ? 'auto' : 'smooth' });
    }
    tabs.forEach(function (t, i) {
      t.addEventListener('click', function () { go(i); });
      t.addEventListener('keydown', function (e) {
        var k = e.key === 'ArrowRight' ? 1 : e.key === 'ArrowLeft' ? -1 : 0;
        if (document.documentElement.dir === 'rtl') k = -k;
        if (k) { e.preventDefault(); go((i + k + n) % n); tabs[(i + k + n) % n].focus(); }
      });
    });
    dots.forEach(function (d, i) { d.addEventListener('click', function () { go(i); }); });
    prev.addEventListener('click', function () { go(cur - 1); });
    next.addEventListener('click', function () { go(cur + 1); });
    show(0);
    pinned = wide() && !reduce;
    if (pinned) M.scrub(sec, function (p) { show(Math.floor(Math.min(.999, p) * n)); });
  })();

  // ---------- uses: three small loops ----------
  (function uses() {
    var wifi = $('#use-wifi'), list = $('#use-list'), meet = $('#use-meet');
    if (!wifi) return;
    var items = list.dataset.items.split('|').map(function (x) { var d = h('div.mtask', h('i'), x); list.appendChild(d); return d; });
    var chat = meet.querySelector('.h-chat'), lines = meet.dataset.items.split('|');
    if (reduce) { wifi.classList.add('burned'); items[0].classList.add('done'); chat.appendChild(h('div.mbub', lines[0])); return; }
    M.once(wifi, function () {
      var t = 0;
      setInterval(function () {
        t++;
        var s = t % 8;
        wifi.classList.toggle('burned', s >= 4);
        items.forEach(function (x, i) { x.classList.toggle('done', s > i * 2); });
        if (s === 1) chat.replaceChildren();
        if (s >= 1 && s <= lines.length) chat.appendChild(h('div.mbub' + (s === 2 ? '.me' : ''), lines[s - 1]));
      }, 900);
    });
  })();

  // ---------- built in the open ----------
  (function repo() {
    var box = $('#h-repo');
    if (!box) return;
    var weeks = $('#h-weeks');
    var COLORS = { Go: '#00add8', JavaScript: '#f1e05a', CSS: '#663399', HTML: '#e34c26', Shell: '#89e051', Makefile: '#427819' };
    NH.api('GET', '/api/repo').then(function (s) {
      var rel = new Intl.RelativeTimeFormat(NH.lang, { numeric: 'auto' }), age = Math.round((s.pushed - Date.now() / 1000) / 86400);
      var V = { stars: s.stars, forks: s.forks, issues: s.issues, license: s.license || '–', pushed: s.pushed ? rel.format(age, 'day') : '–' };
      NH.$$('dd[data-k]', box).forEach(function (dd) {
        var v = V[dd.dataset.k];
        if (typeof v === 'number') M.once(box, function () { M.count(dd, v, function (x) { return x.toLocaleString(NH.lang); }); });
        else dd.textContent = v;
      });
      var bar = $('#h-langbar'), legend = $('#h-legend');
      (s.langs || []).forEach(function (l) {
        var c = COLORS[l.name] || '#8a90a2', pct = Math.round(l.share * 1000) / 10;
        var seg = h('i', { title: l.name });
        seg.style.width = pct + '%';
        seg.style.background = c;
        bar.appendChild(seg);
        var lab = h('span', l.name + ' ' + pct.toLocaleString(NH.lang) + '%');
        lab.style.setProperty('--c', c);
        legend.appendChild(lab);
      });
      var w = (s.weeks || []).slice(-52), max = Math.max.apply(null, w.concat([1]));
      if (!w.length) { $('#h-weeks-p').classList.add('hide'); return; }
      w.forEach(function (v, i) {
        var cell = h('i' + (v ? '.l' + Math.min(3, Math.ceil(v / max * 3)) : ''), { title: NH.t('home.o_week', { n: v }) });
        cell.style.transitionDelay = (i * 12) + 'ms';
        weeks.appendChild(cell);
      });
      M.once(weeks, function () { weeks.classList.add('in'); });
    }, function () {
      NH.$$('dd[data-k]', box).forEach(function (dd) { dd.textContent = '–'; });
      $('#h-weeks-p').textContent = NH.t('home.o_offline');
    });
  })();

  // ---------- marquee ----------
  (function marquee() {
    var mq = $('#h-mq');
    if (!mq) return;
    var words = mq.dataset.words.split('|');
    for (var r = 0; r < 2; r++) words.forEach(function (w) { mq.append(h('span', w), h('span.dia', '◆')); });
  })();
})();
