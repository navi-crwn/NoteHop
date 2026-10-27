// Note page extras behind the "More" menu: duplicate, export, print, slides,
// teleprompter, read aloud, zen mode with a pomodoro timer, and a redacted
// copy. Everything works on the decrypted text in this browser (NHNote).
(function () {
  'use strict';
  var $ = NH.$, h = NH.h, N = window.NHNote;
  var menu = $('#more-menu');
  if (!menu || !N) return;

  menu.querySelector('.menu-list').addEventListener('click', function (e) {
    var b = e.target.closest('[data-act]');
    if (!b) return;
    menu.classList.remove('open');
    if (ACTIONS[b.dataset.act]) ACTIONS[b.dataset.act]();
  });
  // Actions that create a note need the note key (not a key-less read link).
  var keyCheck = setInterval(function () {
    if (!N.meta()) return;
    clearInterval(keyCheck);
    NH.$$('[data-needs-key]', menu).forEach(function (b) { b.classList.toggle('hide', !N.hasKey()); });
  }, 300);

  function fileName(ext) {
    var t = (N.title() || 'note').replace(/[\\/:*?"<>|\u0000-\u001f]+/g, ' ').trim().slice(0, 80) || 'note';
    return t + ext;
  }
  function markdown() {
    var t = N.title(), out = (t ? '# ' + t + '\n\n' : '') + N.body();
    var entries = N.entries().filter(function (e) { return !e.broken; });
    if (entries.length) {
      out += '\n\n---\n\n' + entries.map(function (e) {
        return '**' + (e.n || NH.t('entry.anon')) + '** · ' + new Date(e.created * 1000).toISOString().slice(0, 16).replace('T', ' ') + '\n\n' + e.b;
      }).join('\n\n');
    }
    return out.replace(/\n*$/, '\n');
  }
  function download(name, text, type) {
    var url = URL.createObjectURL(new Blob([text], { type: type }));
    var a = h('a', { href: url, download: name });
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(function () { URL.revokeObjectURL(url); }, 2000);
  }

  // Creates a new note from text in this browser and returns its links.
  function createNote(o) {
    var k = NHC.newKey(), admin = NHC.newKey();
    return NHC.noteKeys(k, '', '').then(function (keys) {
      var codec = NHC.codec(o.encrypted, keys.aes), vault = NHC.vault();
      return Promise.all([codec.encode({ t: o.title, b: o.body }), vault ? vault.seal({ t: o.title, k: k, a: admin, p: '' }) : '']).then(function (r) {
        return NH.api('POST', '/api/notes', {
          encrypted: o.encrypted, password: false, salt: '', content: r[0], mode: o.mode, slug: '',
          expires_in: 0, max_views: 0, admin: admin, write: keys.write, owner_blob: r[1]
        });
      });
    }).then(function (res) {
      NH.recent.add({ token: res.token, k: k, a: admin, t: o.title, created: Math.floor(Date.now() / 1000), expires: 0 });
      return { token: res.token, k: k, a: admin };
    });
  }

  // ---------- slides ----------
  // "---" on its own line (outside code) starts a new slide.
  function slides(src) {
    var out = [[]], fence = null;
    src.split('\n').forEach(function (line) {
      var f = line.match(/^\s*(```|~~~)/);
      if (f) { if (!fence) fence = f[1]; else if (f[1] === fence) fence = null; }
      if (!fence && /^\s*---\s*$/.test(line)) out.push([]);
      else out[out.length - 1].push(line);
    });
    return out.map(function (l) { return l.join('\n').trim(); }).filter(Boolean);
  }
  // Overlays go full screen where that makes sense. Browsers use Escape to
  // leave full screen without telling the page, so leaving full screen also
  // closes the overlay.
  function overlay(cls) {
    var o = h('div.overlay.' + cls, { role: 'dialog', 'aria-modal': 'true' });
    document.body.appendChild(o);
    document.body.classList.add('has-overlay');
    if (o.requestFullscreen && matchMedia('(pointer: fine)').matches) {
      o.requestFullscreen().then(function () {
        o.onFullscreenExit = function () { if (!document.fullscreenElement && o.isConnected) o.closeBtn.click(); };
        document.addEventListener('fullscreenchange', o.onFullscreenExit);
      }, function () {});
    }
    return o;
  }
  function closeOverlay(o, onKey) {
    document.removeEventListener('keydown', onKey);
    if (o.onFullscreenExit) document.removeEventListener('fullscreenchange', o.onFullscreenExit);
    if (document.fullscreenElement) document.exitFullscreen().catch(function () {});
    o.remove();
    document.body.classList.remove('has-overlay');
  }

  function present() {
    var list = slides((N.title() ? '# ' + N.title() + '\n\n---\n\n' : '') + N.body());
    if (!list.length) return NH.toast(NH.t('md.empty'));
    var i = 0, o = overlay('present'), view = h('div.md.slide', { dir: 'auto' }), count = h('span.slide-count');
    var prev = h('button.icon-btn', { type: 'button', 'aria-label': NH.t('present.prev') }, '‹');
    var next = h('button.icon-btn', { type: 'button', 'aria-label': NH.t('present.next') }, '›');
    var close = h('button.icon-btn', { type: 'button', 'aria-label': NH.t('common.close') }, NH.icon('x'));
    o.append(view, h('div.overlay-bar', prev, count, next, close));
    function go(n) {
      i = Math.max(0, Math.min(list.length - 1, n));
      NHMD.render(view, list[i], {});
      count.textContent = (i + 1) + ' / ' + list.length;
      NH.emit && NH.emit('present', { slide: i });
    }
    function onKey(e) {
      if (e.key === 'Escape') return closeOverlay(o, onKey);
      if (['ArrowRight', 'PageDown', ' ', 'Enter'].indexOf(e.key) !== -1) { e.preventDefault(); go(i + 1); }
      if (['ArrowLeft', 'PageUp', 'Backspace'].indexOf(e.key) !== -1) { e.preventDefault(); go(i - 1); }
    }
    prev.addEventListener('click', function () { go(i - 1); });
    next.addEventListener('click', function () { go(i + 1); });
    close.addEventListener('click', function () { closeOverlay(o, onKey); });
    o.closeBtn = close;
    view.addEventListener('click', function (e) { if (!e.target.closest('a, button, input, summary')) go(i + 1); });
    document.addEventListener('keydown', onKey);
    go(0);
    NHNote.goSlide = go;
  }

  // ---------- teleprompter ----------
  function prompter() {
    var o = overlay('prompter'), view = h('div.md.prompter-text', { dir: 'auto' });
    NHMD.render(view, N.body(), {});
    var speed = 40, playing = false, last = 0, mirrored = false;
    var play = h('button.btn.sm.primary', { type: 'button' }, NH.t('prompter.play'));
    var slower = h('button.icon-btn', { type: 'button', 'aria-label': NH.t('prompter.slower') }, '−');
    var faster = h('button.icon-btn', { type: 'button', 'aria-label': NH.t('prompter.faster') }, '+');
    var mirror = h('button.btn.sm', { type: 'button' }, NH.t('prompter.mirror'));
    var close = h('button.icon-btn', { type: 'button', 'aria-label': NH.t('common.close') }, NH.icon('x'));
    var label = h('span.slide-count');
    o.append(view, h('div.overlay-bar', slower, label, faster, play, mirror, close));
    function show() { label.textContent = speed + ' px/s'; play.textContent = NH.t(playing ? 'prompter.pause' : 'prompter.play'); }
    function frame(t) {
      if (!playing) return;
      if (last) view.scrollTop += speed * (t - last) / 1000;
      last = t;
      if (view.scrollTop + view.clientHeight >= view.scrollHeight - 1) { playing = false; show(); return; }
      requestAnimationFrame(frame);
    }
    function toggle() { playing = !playing; last = 0; show(); if (playing) requestAnimationFrame(frame); }
    function onKey(e) {
      if (e.key === 'Escape') { playing = false; return closeOverlay(o, onKey); }
      if (e.key === ' ') { e.preventDefault(); toggle(); }
      if (e.key === 'ArrowUp') { speed = Math.min(400, speed + 10); show(); }
      if (e.key === 'ArrowDown') { speed = Math.max(10, speed - 10); show(); }
    }
    play.addEventListener('click', toggle);
    slower.addEventListener('click', function () { speed = Math.max(10, speed - 10); show(); });
    faster.addEventListener('click', function () { speed = Math.min(400, speed + 10); show(); });
    mirror.addEventListener('click', function () { mirrored = !mirrored; view.classList.toggle('mirrored', mirrored); });
    close.addEventListener('click', function () { playing = false; closeOverlay(o, onKey); });
    o.closeBtn = close;
    document.addEventListener('keydown', onKey);
    show();
  }

  // ---------- read aloud ----------
  function listen() {
    var synth = window.speechSynthesis;
    if (!synth) return NH.toast(NH.t('listen.unsupported'), 'err');
    if (synth.speaking) { synth.cancel(); return NH.toast(NH.t('listen.stopped')); }
    var text = ((N.title() ? N.title() + '. ' : '') + N.view().innerText).trim();
    if (!text) return;
    var u = new SpeechSynthesisUtterance(text);
    // The note's own language may differ from the interface; let the browser
    // pick a voice for the text unless the page language is a better guess.
    u.lang = document.documentElement.lang;
    synth.speak(u);
    NH.toast(NH.t('listen.started'));
  }

  // ---------- zen mode: no chrome, a pomodoro timer and soft noise ----------
  var zen = null;
  function zenMode() {
    if (zen) return zen.exit();
    document.body.classList.add('zen');
    var work = 25 * 60, rest = 5 * 60, left = work, resting = false, timer = null, audio = null;
    var clock = h('b.zen-clock'), start = h('button.btn.sm', { type: 'button' }), reset = h('button.btn.sm.ghost', { type: 'button' }, NH.t('zen.reset'));
    var noise = h('button.btn.sm.ghost', { type: 'button' }, NH.t('zen.noise')), exit = h('button.btn.sm', { type: 'button' }, NH.t('zen.exit'));
    var bar = h('div.zen-bar', h('span.zen-phase'), clock, start, reset, noise, exit);
    document.body.appendChild(bar);
    function draw() {
      clock.textContent = String(Math.floor(left / 60)).padStart(2, '0') + ':' + String(left % 60).padStart(2, '0');
      start.textContent = NH.t(timer ? 'zen.pause' : 'zen.start');
      bar.firstChild.textContent = NH.t(resting ? 'zen.break' : 'zen.focus');
      document.title = clock.textContent + ' · ' + (N.title() || 'NoteHop');
    }
    function tick() {
      if (--left > 0) return draw();
      resting = !resting;
      left = resting ? rest : work;
      NH.toast(NH.t(resting ? 'zen.break_now' : 'zen.focus_now'), 'ok');
      draw();
    }
    start.addEventListener('click', function () {
      if (timer) { clearInterval(timer); timer = null; } else timer = setInterval(tick, 1000);
      draw();
    });
    reset.addEventListener('click', function () { resting = false; left = work; draw(); });
    // Brown noise made on the fly: no audio files to license or download.
    noise.addEventListener('click', function () {
      if (audio) { audio.close(); audio = null; noise.classList.remove('on'); return; }
      var Ctx = window.AudioContext || window.webkitAudioContext;
      if (!Ctx) return;
      audio = new Ctx();
      var len = audio.sampleRate * 4, buf = audio.createBuffer(1, len, audio.sampleRate), d = buf.getChannelData(0), last = 0;
      for (var i = 0; i < len; i++) { last = (last + 0.02 * (Math.random() * 2 - 1)) / 1.02; d[i] = last * 3.5; }
      var src = audio.createBufferSource(), gain = audio.createGain();
      src.buffer = buf; src.loop = true; gain.gain.value = 0.25;
      src.connect(gain).connect(audio.destination);
      src.start();
      noise.classList.add('on');
    });
    zen = {
      exit: function () {
        clearInterval(timer);
        if (audio) audio.close();
        bar.remove();
        document.body.classList.remove('zen');
        document.title = (N.title() || NH.t('common.untitled')) + ' · NoteHop';
        zen = null;
      }
    };
    exit.addEventListener('click', zen.exit);
    draw();
  }

  // ---------- redacted copy ----------
  // Makes a read-only copy with likely secrets masked, for sharing more
  // widely. Done here, before anything is encrypted; the server never sees
  // the original or the masked text.
  var REDACT = [
    [/\b[\w.+-]+@[\w-]+(\.[\w-]+)+\b/g, 'email'],
    // Cards: 13–19 digits, in groups of four as they're usually written.
    [/\b\d{4}(?:[ -]?\d{4}){2}[ -]?\d{1,7}\b/g, 'card'],
    [/(?:\+|\b00)?\d[\d ()-]{7,}\d\b/g, 'phone'],
    [/\b(?:\d{1,3}\.){3}\d{1,3}\b/g, 'ip'],
    [/\b[A-Z]{2}\d{2}[A-Z0-9]{11,30}\b/g, 'iban'],
    [/\b(?:sk|pk|ghp|gho|xox[bap]|AKIA|AIza)[\w-]{10,}\b/g, 'key'],
    [/\b[A-Fa-f0-9]{32,}\b|\b[A-Za-z0-9_\-]{32,}\b/g, 'key'],
    [/((?:password|passwd|pwd|pass|secret|token|kata ?sandi|contraseña|senha|пароль|密码|パスワード|كلمة المرور)\s*[:=]\s*)\S+/gi, 'secret']
  ];
  function redactText(text) {
    var count = 0;
    REDACT.forEach(function (r) {
      text = text.replace(r[0], function (m, keep) {
        count++;
        var label = '[' + NH.t('redact.' + r[1]) + ']';
        return r[1] === 'secret' ? keep + label : label;
      });
    });
    return { text: text, count: count };
  }
  function redact() {
    var r = redactText(N.body()), dlg = h('dialog.dlg.wide', { 'aria-label': NH.t('more.redact') });
    var out = h('textarea.redact-out', { readonly: true, rows: 12, dir: 'auto' });
    out.value = r.text;
    var make = h('button.btn.primary', { type: 'button' }, NH.icon('copy'), NH.t('redact.create'));
    dlg.append(
      h('div.dlg-head', h('h2', NH.t('more.redact')), h('button.icon-btn', { type: 'button', 'data-close': '', 'aria-label': NH.t('common.close') }, NH.icon('x'))),
      h('p.muted.small', NH.t('redact.lead', { n: r.count })), out,
      h('p.hint', NH.t('redact.check')), h('div.btn-row.end', make));
    document.body.appendChild(dlg);
    dlg.addEventListener('close', function () { dlg.remove(); });
    make.addEventListener('click', function () {
      make.disabled = true;
      createNote({ title: N.title(), body: out.value, mode: 'read', encrypted: true }).then(function (n) {
        NH.copy(NH.noteURL(n.token, { k: n.k }), NH.t('redact.done'));
        dlg.close();
      }, function (x) { make.disabled = false; NH.toast(x.message, 'err'); });
    });
    dlg.showModal();
  }

  var ACTIONS = {
    duplicate: function () {
      createNote({ title: N.title(), body: N.body(), mode: 'edit', encrypted: N.encrypted() }).then(function (n) {
        location.href = NH.noteURL(n.token, { k: n.k, a: n.a, new: '1' });
      }, function (x) { NH.toast(x.message, 'err'); });
    },
    'export-md': function () { download(fileName('.md'), markdown(), 'text/markdown;charset=utf-8'); },
    print: function () { window.print(); },
    present: present,
    prompter: prompter,
    listen: listen,
    zen: zenMode,
    redact: redact
  };
  NH.redactText = redactText;
})();
