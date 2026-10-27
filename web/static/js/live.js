// Real-time collaboration over /api/n/{token}/live. Everything sent is
// encrypted with the note key first (the server only relays), the shared
// text is a Yjs document, and the oldest writer in the room saves it.
//   - presence: who is here, their caret and selection in the editor
//   - chat: short messages that are never stored
//   - present: others can follow your slides and scroll position
(function () {
  'use strict';
  var N = window.NHNote, $ = NH.$, h = NH.h;
  if (!N || !window.Y || !window.WebSocket) return;
  var S = N.state;

  var COLORS = ['#22d3ee', '#a78bfa', '#f472b6', '#fb923c', '#4ade80', '#facc15', '#60a5fa', '#f87171'];
  var ws = null, me = 0, canWrite = false, retry = 0, closed = false;
  var peers = {};           // id → {w, name, color, sel, title, presenting}
  var doc = null, ybody = null, ytitle = null, synced = false, waiting = [], needTimer = null;
  var last = { body: null, title: null };
  var nick = NH.store.get('nh_nick', '') || document.body.dataset.user || NH.t('live.guest', { n: 10 + Math.floor(Math.random() * 90) });
  var myColor = COLORS[Math.floor(Math.random() * COLORS.length)];
  var presenting = false, following = 0, unread = 0;

  // ---------- encryption ----------
  function seal(obj) { return S.codec.encode(obj); }
  function open(d) { return S.codec.decode(d); }
  function b64(u8) { return NHC.b64u(u8); }
  function unb64(s) { return NHC.unb64u(s); }

  function send(t, obj, to) {
    if (!ws || ws.readyState !== 1) return;
    seal(obj).then(function (d) { if (ws && ws.readyState === 1) ws.send(JSON.stringify({ t: t, to: to || 0, d: d })); });
  }

  // ---------- connection ----------
  function start() {
    var m = S.meta;
    if (!S.keys || m.frozen || m.max_views > 0 || closed) return;
    if (!navigator.onLine) { window.addEventListener('online', start, { once: true }); return; }
    var url = (location.protocol === 'https:' ? 'wss://' : 'ws://') + location.host + '/api/n/' + encodeURIComponent(N.token) + '/live';
    ws = new WebSocket(url);
    ws.onopen = function () {
      retry = 0;
      ws.send(JSON.stringify({ t: 'auth', write: S.keys.write, admin: N.admin() || '' }));
    };
    ws.onmessage = function (e) {
      var m;
      try { m = JSON.parse(e.data); } catch (x) { return; }
      receive(m);
    };
    ws.onclose = function () {
      ws = null;
      peers = {};
      // A doc from before the drop may have missed edits; start over and
      // take the room's state (or the saved text) on reconnect.
      resetDoc();
      drawPeers();
      if (closed) return;
      setStatus(NH.t('live.reconnecting'));
      if (!navigator.onLine) return window.addEventListener('online', start, { once: true });
      setTimeout(start, Math.min(30000, 1000 * Math.pow(2, retry++)));
    };
  }

  function resetDoc() {
    if (doc) doc.destroy();
    doc = ybody = ytitle = null;
    synced = false;
    waiting = [];
  }

  function receive(m) {
    switch (m.t) {
      case 'hello':
        me = m.id; canWrite = m.w;
        m.peers.forEach(function (p) { peers[p.id] = peer(p.id, p.w); });
        $('#live-bar').classList.remove('hide');
        need();
        announce();
        break;
      case 'join':
        peers[m.id] = peer(m.id, m.w);
        announce();
        // A reader waiting for a writer: ask the newcomer's room for state.
        if (m.w && !synced && !canWrite) need();
        break;
      case 'leave':
        if (following === m.id) stopFollowing();
        var wasLeader = leader() === me;
        delete peers[m.id];
        if (!synced) need(); // the peer we were waiting on may have left
        // Just became the one who saves: save now, the old leader may not have.
        else if (!wasLeader && leader() === me && N.editor()) N.edited();
        break;
      case 'nosync':
        clearTimeout(needTimer);
        // First writer here: the saved text is the starting point. A reader
        // just keeps the saved text until a writer shows up.
        if (canWrite) init(null);
        else setStatus(NH.t('live.watching'));
        break;
      case 'need':
        if (synced) sendState(m.from); else waiting.push(m.from);
        break;
      case 'sync':
        open(m.d).then(function (o) { clearTimeout(needTimer); init(unb64(o.s)); drawPeers(); }, function () {});
        break;
      case 'y':
        if (doc) open(m.d).then(function (o) { if (doc) Y.applyUpdate(doc, unb64(o.u), 'remote'); }, function () {});
        return;
      case 'aw':
        open(m.d).then(function (o) { awareness(m.from, o); }, function () {});
        break;
      case 'chat':
        open(m.d).then(function (o) { chatIn(m.from, o); }, function () {});
        break;
      case 'follow':
        open(m.d).then(function (o) { followIn(m.from, o); }, function () {});
        break;
      case 'ro':
        canWrite = false;
        location.reload();
        break;
    }
    drawPeers();
  }

  function peer(id, w) { return { id: id, w: w, name: '…', color: COLORS[id % COLORS.length] }; }

  function need() {
    clearTimeout(needTimer);
    if (!ws || ws.readyState !== 1) return;
    ws.send(JSON.stringify({ t: 'need' }));
    setStatus(NH.t('live.joining'));
    // No answer (the source left mid-reply): ask again.
    needTimer = setTimeout(function () { if (!synced) need(); }, 4000);
  }

  function sendState(to) {
    send('sync', { s: b64(Y.encodeStateAsUpdate(doc)) }, to);
  }

  // ---------- the shared document ----------
  function init(state) {
    if (synced) return;
    var queued = waiting;
    resetDoc();
    doc = new Y.Doc();
    ybody = doc.getText('b');
    ytitle = doc.getText('t');
    var ed = N.editor(), localBody = ed ? ed.value : S.body, localTitle = ed ? $('#n-title-input').value : S.title;
    if (state) {
      Y.applyUpdate(doc, state, 'remote');
    } else {
      doc.transact(function () { ytitle.insert(0, localTitle || ''); ybody.insert(0, localBody || ''); }, 'init');
    }
    synced = true;
    doc.on('update', function (u, origin) {
      if (origin === 'remote' || origin === 'init') return;
      send('y', { u: b64(u) });
    });
    ybody.observe(function (ev) { if (ev.transaction.origin === 'remote') remoteChange('b', ev); });
    ytitle.observe(function (ev) { if (ev.transaction.origin === 'remote') remoteChange('t', ev); });

    if (ed) {
      // Unsaved local typing from before joining is not merged: show it
      // as a copyable draft instead of silently losing it.
      var shared = ybody.toString();
      if (state && S.dirty && localBody !== shared) {
        NH.copy(localBody, NH.t('live.draft_copied'));
      }
      last.body = shared; last.title = ytitle.toString();
      if (ed.value !== shared) ed.value = shared;
      if ($('#n-title-input').value !== last.title) $('#n-title-input').value = last.title;
      bindCursor(ed);
    } else {
      N.showBody(ytitle.toString(), ybody.toString());
    }
    queued.forEach(sendState);
    setStatus('');
    announce();
  }

  // Local text → Yjs: one delete + one insert around the changed middle.
  function apply(ytext, before, after) {
    if (before === after) return;
    var a = 0, max = Math.min(before.length, after.length);
    while (a < max && before.charCodeAt(a) === after.charCodeAt(a)) a++;
    var b = 0;
    while (b < max - a && before.charCodeAt(before.length - 1 - b) === after.charCodeAt(after.length - 1 - b)) b++;
    doc.transact(function () {
      if (before.length - a - b > 0) ytext.delete(a, before.length - a - b);
      if (after.length - a - b > 0) ytext.insert(a, after.slice(a, after.length - b));
    }, 'local');
  }

  // Called by note.js on every local edit. Returns 'follower' when another
  // writer is the one saving.
  function edited() {
    if (!synced || !N.editor()) return '';
    var body = N.editor().value, title = $('#n-title-input').value;
    if (body !== last.body) { apply(ybody, last.body, body); last.body = body; }
    if (title !== last.title) { apply(ytitle, last.title, title); last.title = title; }
    sendCursor();
    return leader() === me ? 'leader' : 'follower';
  }

  // The oldest writer saves; with several tabs that's one PUT, not N
  // conflicting ones.
  function leader() {
    var ids = Object.keys(peers).filter(function (id) { return peers[id].w; }).map(Number);
    if (canWrite) ids.push(me);
    return ids.length ? Math.min.apply(null, ids) : 0;
  }

  // Remote change → textarea, keeping the local caret where it was in the text.
  function remoteChange(which, ev) {
    var ed = N.editor();
    if (!ed) {
      clearTimeout(remoteChange.t);
      remoteChange.t = setTimeout(function () { N.showBody(ytitle.toString(), ybody.toString()); }, 80);
      return;
    }
    // Web notes: the text box holds one file of the shared text, so offsets
    // don't map; the editor keeps the open file's caret itself.
    if (which === 'b' && ed.type === 'web') {
      var t = ybody.toString();
      ed.value = t; last.body = t;
      if (leader() === me) N.edited();
      return;
    }
    var el = which === 'b' ? ed.textarea : $('#n-title-input');
    var s = el.selectionStart, e = el.selectionEnd, pos = 0;
    ev.delta.forEach(function (op) {
      if (op.retain) pos += op.retain;
      else if (op.insert) {
        var n = op.insert.length;
        if (pos <= s) s += n;
        if (pos <= e) e += n;
        pos += n;
      } else if (op.delete) {
        if (pos < s) s -= Math.min(op.delete, s - pos);
        if (pos < e) e -= Math.min(op.delete, e - pos);
      }
    });
    var text = (which === 'b' ? ybody : ytitle).toString();
    var focused = document.activeElement === el, top = el.scrollTop;
    if (which === 'b') { ed.value = text; last.body = text; } else { el.value = text; last.title = text; }
    if (focused) el.setSelectionRange(s, e);
    el.scrollTop = top;
    // The leader marks the note dirty so it gets saved.
    if (leader() === me) N.edited();
    drawCarets();
  }

  // ---------- presence and carets ----------
  function announce() {
    send('aw', { name: nick, color: myColor, w: canWrite, presenting: presenting, sel: cursorJSON() });
  }
  function cursorJSON() {
    var ed = N.editor();
    if (!ed || !synced || document.activeElement !== ed.textarea || ed.type === 'web') return null;
    var ta = ed.textarea;
    return [ta.selectionStart, ta.selectionEnd].map(function (i) {
      return b64(Y.encodeRelativePosition(Y.createRelativePositionFromTypeIndex(ybody, i)));
    });
  }
  var cursorTimer = null;
  function sendCursor() {
    clearTimeout(cursorTimer);
    cursorTimer = setTimeout(announce, 120);
  }
  function bindCursor(ed) {
    if (bindCursor.done) return;
    bindCursor.done = true;
    var ta = ed.textarea;
    ['select', 'keyup', 'mouseup', 'focus', 'blur'].forEach(function (ev) { ta.addEventListener(ev, sendCursor); });
    ta.addEventListener('scroll', drawCarets);
    ta.parentNode.classList.add('live-host');
    carets = h('div.live-carets', { 'aria-hidden': 'true' });
    ta.parentNode.appendChild(carets);
    window.addEventListener('resize', drawCarets);
  }
  var carets = null;

  function awareness(id, o) {
    var p = peers[id] || (peers[id] = peer(id, !!o.w));
    p.name = String(o.name || '').slice(0, 40) || '…';
    if (/^#[0-9a-f]{6}$/i.test(o.color)) p.color = o.color;
    p.sel = o.sel;
    p.presenting = !!o.presenting;
    if (!p.presenting && following === id) stopFollowing();
    drawPeers();
    drawCarets();
  }

  function drawCarets() {
    if (!carets || !doc) return;
    var ta = N.editor().textarea, out = [];
    Object.keys(peers).forEach(function (id) {
      var p = peers[id];
      if (!p.sel) return;
      var idx = p.sel.map(function (r) {
        try {
          var abs = Y.createAbsolutePositionFromRelativePosition(Y.decodeRelativePosition(unb64(r)), doc);
          return abs ? abs.index : null;
        } catch (x) { return null; }
      });
      if (idx[1] == null) return;
      var xy = NHEditor.caretXY(ta, idx[1]), lh = parseFloat(getComputedStyle(ta).lineHeight) || 20;
      var c = h('span.live-caret', h('span.live-name', p.name));
      c.style.setProperty('--c', p.color);
      c.style.left = (ta.offsetLeft + xy.x - ta.scrollLeft) + 'px';
      c.style.top = (ta.offsetTop + xy.y - lh - ta.scrollTop) + 'px';
      c.style.height = lh + 'px';
      out.push(c);
    });
    carets.replaceChildren.apply(carets, out);
  }

  function drawPeers() {
    var list = Object.keys(peers).map(function (id) { return peers[id]; });
    var box = $('#live-people');
    var mine = h('button.live-av.me', { type: 'button', title: NH.t('live.rename'), 'aria-label': NH.t('live.rename') }, initials(nick));
    mine.style.setProperty('--c', myColor);
    mine.addEventListener('click', rename);
    box.replaceChildren.apply(box, [mine].concat(list.map(function (p) {
      var a = h('span.live-av', { title: p.name + (p.w ? '' : ' · ' + NH.t('live.reader')) }, initials(p.name));
      a.style.setProperty('--c', p.color);
      return a;
    })));
    var n = list.length;
    if (synced || !ws) setStatus(!ws ? NH.t('live.reconnecting') : n ? NH.t('live.with', { n: n }) : NH.t('live.alone'));
    var pres = list.filter(function (p) { return p.presenting; })[0], f = $('#live-follow');
    if (pres && !presenting) {
      var btn = h('button.btn.sm' + (following === pres.id ? '.primary' : ''), { type: 'button' },
        following === pres.id ? NH.t('live.following', { name: pres.name }) : NH.t('live.follow', { name: pres.name }));
      btn.addEventListener('click', function () { if (following === pres.id) stopFollowing(); else following = pres.id; drawPeers(); });
      f.replaceChildren(btn);
      f.classList.remove('hide');
    } else f.classList.add('hide');
  }
  function initials(name) {
    var w = String(name || '?').trim().split(/\s+/);
    return ((w[0] || '?').charAt(0) + (w[1] ? w[1].charAt(0) : '')).toUpperCase();
  }
  function setStatus(t) { $('#live-status').textContent = t; }
  function rename() {
    var v = prompt(NH.t('live.rename'), nick);
    if (!v || !v.trim()) return;
    nick = v.trim().slice(0, 40);
    NH.store.set('nh_nick', nick);
    announce();
    drawPeers();
  }

  // ---------- chat (never stored) ----------
  var chatBox = $('#live-chat'), msgs = $('#live-msgs'), chatForm = $('#live-chat-form');
  function chatLine(name, color, text, mine) {
    var li = h('li' + (mine ? '.mine' : ''), h('b', name), h('span', { dir: 'auto' }, text));
    li.firstChild.style.setProperty('--c', color);
    msgs.appendChild(li);
    while (msgs.children.length > 200) msgs.firstChild.remove();
    msgs.scrollTop = msgs.scrollHeight;
  }
  function chatIn(id, o) {
    var p = peers[id] || peer(id, false);
    chatLine(p.name, p.color, String(o.text || '').slice(0, 500), false);
    if (chatBox.classList.contains('hide')) { unread++; drawUnread(); }
  }
  function drawUnread() {
    var b = $('#live-unread');
    b.textContent = unread;
    b.classList.toggle('hide', !unread);
  }
  $('#live-chat-btn').addEventListener('click', function () {
    chatBox.classList.toggle('hide');
    if (!chatBox.classList.contains('hide')) { unread = 0; drawUnread(); chatForm.querySelector('input').focus(); }
  });
  $('#live-chat-close').addEventListener('click', function () { chatBox.classList.add('hide'); });
  chatForm.addEventListener('submit', function (e) {
    e.preventDefault();
    var input = chatForm.querySelector('input'), text = input.value.trim();
    if (!text || !ws) return;
    send('chat', { text: text });
    chatLine(nick, myColor, text, true);
    input.value = '';
  });

  // ---------- follow the presenter ----------
  var presentBtn = $('#live-present');
  presentBtn.addEventListener('click', function () {
    presenting = !presenting;
    presentBtn.setAttribute('aria-pressed', presenting);
    presentBtn.classList.toggle('primary', presenting);
    if (presenting) stopFollowing();
    announce();
    if (presenting) sendPosition();
  });
  var slide = -1;
  NH.on('present', function (d) { slide = d.slide; if (presenting) sendPosition(); });
  var scrollTimer = null;
  window.addEventListener('scroll', function () {
    if (!presenting) return;
    clearTimeout(scrollTimer);
    scrollTimer = setTimeout(sendPosition, 150);
  }, { passive: true });
  function sendPosition() {
    var max = document.documentElement.scrollHeight - innerHeight;
    var inSlides = !!document.querySelector('.overlay.present');
    send('follow', { y: max > 0 ? scrollY / max : 0, slide: inSlides ? slide : -1 });
  }
  function followIn(id, o) {
    if (following !== id) return;
    if (o.slide >= 0) {
      if (!document.querySelector('.overlay.present') && N.present) N.present();
      if (N.goSlide) N.goSlide(o.slide);
    } else {
      if (N.closeSlides) N.closeSlides();
      var max = document.documentElement.scrollHeight - innerHeight;
      window.scrollTo({ top: Math.round((+o.y || 0) * max), behavior: 'smooth' });
    }
  }
  function stopFollowing() { following = 0; }

  window.NHLive = {
    edited: edited,
    active: function () { return synced; },
    leader: function () { return synced && leader() === me; },
    peers: function () { return Object.keys(peers).length; }
  };
  window.addEventListener('pagehide', function () { closed = true; if (ws) ws.close(); });
  N.onReady(start);
})();
