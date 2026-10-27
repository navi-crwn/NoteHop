// Nearby devices: find browsers on the same network through /api/nearby,
// then connect to each directly (WebRTC data channels, encrypted by DTLS).
// The shared pad and files go device to device; the server only passes on
// the connection handshake. No STUN/TURN servers: this is for one network,
// and nobody outside learns that two devices talked.
(function () {
  'use strict';
  var $ = NH.$, h = NH.h;
  if (!window.RTCPeerConnection || !window.WebSocket) { $('#nb-status').textContent = NH.t('nearby.unsupported'); return; }

  var CHUNK = 16 * 1024, MAX_FILE = 1024 * 1024 * 1024;
  var ws = null, me = '', peers = {}, retry = 0;
  var device = /Android|iPhone|iPad|Mobile/i.test(navigator.userAgent) ? 'phone' : 'computer';
  var name = NH.store.get('nh_nick', '') || NH.t('nearby.default_name', { n: 10 + Math.floor(Math.random() * 90) });
  var pad = $('#nb-pad'), padTs = 0, padTimer = null;
  $('#nb-name').textContent = name;

  function status(t) { $('#nb-status').textContent = t; }

  // ---------- signalling ----------
  function connect() {
    ws = new WebSocket((location.protocol === 'https:' ? 'wss://' : 'ws://') + location.host + '/api/nearby');
    ws.onopen = function () { retry = 0; hi(); };
    ws.onmessage = function (e) {
      var m;
      try { m = JSON.parse(e.data); } catch (x) { return; }
      if (m.t === 'hello') {
        me = m.id;
        status(NH.t('nearby.ready'));
        // The newcomer calls everyone already here.
        m.peers.forEach(function (p) { add(p).call(); });
      } else if (m.t === 'peer') {
        var p = peers[m.peer.id] || add(m.peer);
        p.name = m.peer.name; p.device = m.peer.device;
        draw();
      } else if (m.t === 'leave') {
        drop(m.id);
      } else if (m.t === 'sig') {
        (peers[m.from] || add({ id: m.from })).signal(m.d);
      }
    };
    ws.onclose = function () {
      Object.keys(peers).forEach(drop);
      status(NH.t('nearby.reconnecting'));
      setTimeout(connect, Math.min(30000, 1000 * Math.pow(2, retry++)));
    };
  }
  function hi() { if (ws && ws.readyState === 1) ws.send(JSON.stringify({ t: 'hi', name: name, device: device })); }
  function sig(to, d) { if (ws && ws.readyState === 1) ws.send(JSON.stringify({ t: 'sig', to: to, d: d })); }

  // ---------- peers ----------
  function add(info) {
    var p = peers[info.id] = { id: info.id, name: info.name || '…', device: info.device || '', state: 'connecting', pc: null, dc: null, incoming: {} };
    var pc = p.pc = new RTCPeerConnection({ iceServers: [] });
    pc.onicecandidate = function (e) { if (e.candidate) sig(p.id, { c: e.candidate.toJSON() }); };
    pc.onconnectionstatechange = function () {
      if (pc.connectionState === 'failed') { p.state = 'failed'; draw(); }
    };
    pc.ondatachannel = function (e) { channel(p, e.channel); };
    p.call = function () {
      channel(p, pc.createDataChannel('nh', { ordered: true }));
      pc.createOffer().then(function (o) { return pc.setLocalDescription(o); }).then(function () { sig(p.id, { sdp: pc.localDescription.toJSON() }); });
      return p;
    };
    var early = []; // candidates that arrive before the offer/answer is applied
    p.signal = function (d) {
      if (d && d.sdp) {
        pc.setRemoteDescription(d.sdp).then(function () {
          early.splice(0).forEach(function (c) { pc.addIceCandidate(c).catch(function () {}); });
          if (d.sdp.type !== 'offer') return;
          return pc.createAnswer().then(function (a) { return pc.setLocalDescription(a); }).then(function () { sig(p.id, { sdp: pc.localDescription.toJSON() }); });
        }).catch(function () { p.state = 'failed'; draw(); });
      } else if (d && d.c) {
        if (!pc.remoteDescription) early.push(d.c);
        else pc.addIceCandidate(d.c).catch(function () {});
      }
    };
    draw();
    return p;
  }
  function drop(id) {
    var p = peers[id];
    if (!p) return;
    try { p.pc.close(); } catch (x) {}
    delete peers[id];
    draw();
  }

  function channel(p, dc) {
    p.dc = dc;
    dc.binaryType = 'arraybuffer';
    dc.bufferedAmountLowThreshold = 1024 * 1024;
    dc.onopen = function () {
      p.state = 'open';
      draw();
      if (pad.value && padTs) dc.send(JSON.stringify({ type: 'pad', text: pad.value, ts: padTs }));
    };
    dc.onclose = function () { p.state = 'closed'; draw(); };
    dc.onmessage = function (e) { receive(p, e.data); };
  }

  // ---------- shared pad (last writer wins) ----------
  pad.addEventListener('input', function () {
    padTs = Date.now();
    clearTimeout(padTimer);
    padTimer = setTimeout(function () { broadcast({ type: 'pad', text: pad.value, ts: padTs }); }, 250);
  });
  function broadcast(obj) {
    var s = JSON.stringify(obj);
    Object.keys(peers).forEach(function (id) { var dc = peers[id].dc; if (dc && dc.readyState === 'open') dc.send(s); });
  }

  // ---------- files ----------
  var pending = null;
  $('#nb-file').addEventListener('change', function () {
    var files = Array.prototype.slice.call($('#nb-file').files);
    $('#nb-file').value = '';
    if (pending) files.forEach(function (f) { sendFile(pending, f); });
  });
  function sendFile(p, f) {
    if (f.size > MAX_FILE) return NH.toast(NH.t('nearby.too_big'), 'err');
    var id = Math.random().toString(36).slice(2), dc = p.dc, off = 0;
    var row = progressRow(NH.t('nearby.sending', { name: f.name, to: p.name }));
    dc.send(JSON.stringify({ type: 'file', id: id, name: f.name, size: f.size, mime: f.type }));
    function pump() {
      if (dc.readyState !== 'open') return row.fail();
      if (off < f.size && dc.bufferedAmount <= dc.bufferedAmountLowThreshold * 4) {
        var end = Math.min(f.size, off + CHUNK), slice = f.slice(off, end);
        off = end;
        // Chunks are read in order; send waits for the read so order holds.
        return slice.arrayBuffer().then(function (buf) {
          dc.send(buf);
          row.set(off / f.size);
          if (off >= f.size) { dc.send(JSON.stringify({ type: 'end', id: id })); row.done(); } else pump();
        });
      }
      if (off < f.size) dc.onbufferedamountlow = function () { dc.onbufferedamountlow = null; pump(); };
    }
    pump();
  }
  function receive(p, data) {
    if (typeof data !== 'string') {
      var cur = p.current;
      if (!cur) return;
      cur.parts.push(data);
      cur.got += data.byteLength;
      cur.row.set(cur.got / (cur.size || 1));
      return;
    }
    var m;
    try { m = JSON.parse(data); } catch (x) { return; }
    if (m.type === 'pad' && m.ts > padTs) {
      padTs = m.ts;
      var s = pad.selectionStart, e = pad.selectionEnd;
      pad.value = String(m.text);
      if (document.activeElement === pad) pad.setSelectionRange(s, e);
    } else if (m.type === 'file' && m.size <= MAX_FILE) {
      p.current = { id: m.id, name: String(m.name || 'file').slice(0, 200), mime: String(m.mime || ''), size: +m.size, parts: [], got: 0,
        row: progressRow(NH.t('nearby.receiving', { name: m.name, from: p.name })) };
    } else if (m.type === 'end' && p.current && p.current.id === m.id) {
      var c = p.current;
      p.current = null;
      var url = URL.createObjectURL(new Blob(c.parts, { type: c.mime || 'application/octet-stream' }));
      c.row.done(h('a.btn.sm.primary', { href: url, download: c.name }, NH.icon('download'), NH.t('nearby.save', { name: c.name })));
    }
  }
  function progressRow(label) {
    var bar = h('progress', { max: 1, value: 0 }), li = h('li', h('span', label), bar);
    $('#nb-files').prepend(li);
    return {
      set: function (v) { bar.value = v; },
      done: function (extra) { bar.remove(); if (extra) li.appendChild(extra); li.classList.add('done'); },
      fail: function () { bar.remove(); li.appendChild(h('span.faint', NH.t('nearby.failed'))); }
    };
  }

  // ---------- UI ----------
  function draw() {
    var ids = Object.keys(peers), list = $('#nb-list');
    $('#nb-empty').classList.toggle('hide', ids.length > 0);
    list.replaceChildren.apply(list, ids.map(function (id) {
      var p = peers[id], open = p.state === 'open';
      var send = h('button.btn.sm' + (open ? '.primary' : ''), { type: 'button', disabled: !open }, NH.icon('attach'), NH.t('nearby.send_file'));
      send.addEventListener('click', function () { pending = p; $('#nb-file').click(); });
      return h('li.card.nearby-peer', h('span.nearby-dev', NH.icon(p.device === 'phone' ? 'grid' : 'code')),
        h('span.nearby-who', h('b', p.name), h('span.small.faint', NH.t('nearby.state_' + p.state))), send);
    }));
  }

  $('#nb-rename').addEventListener('click', function () {
    var v = prompt(NH.t('live.rename'), name);
    if (!v || !v.trim()) return;
    name = v.trim().slice(0, 40);
    NH.store.set('nh_nick', name);
    $('#nb-name').textContent = name;
    hi();
  });
  draw();
  connect();
})();
