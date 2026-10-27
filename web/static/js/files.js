// Encrypted attachments. Each file is sealed in the browser with its own key
// (NHC.fileSeal) and uploaded as ciphertext; the note text holds
// nh-file:ID#KEY, so only readers of the note can open it. Files expire
// after 24 hours. Also: pasted images become WebP, and [m:ss] timestamps in
// the text seek the nearest audio or video above them.
(function () {
  'use strict';
  var h = NH.h;
  var REF = /(!?)\[([^\]\n]*)\]\(nh-file:([A-Za-z0-9]{20})#([A-Za-z0-9_-]{43})\)/g;
  var TS = /\[(?:(\d{1,2}):)?(\d{1,2}):(\d{2})\]/g, HAS_TS = /\[(?:\d{1,2}:)?\d{1,2}:\d{2}\]/;

  // id → Promise<{url, name, type, expires}>; decrypted once per page.
  var cache = {};
  // id → media element, reused across re-renders so playback survives typing.
  var media = {};

  function load(id, key) {
    if (!cache[id]) {
      cache[id] = fetch('/api/files/' + id, { credentials: 'same-origin' }).then(function (r) {
        if (!r.ok) throw new Error(NH.t('file.gone'));
        var exp = +r.headers.get('X-Expires') || 0;
        return r.arrayBuffer().then(function (buf) { return NHC.fileOpen(key, buf); }).then(function (f) {
          return { url: URL.createObjectURL(new Blob([f.bytes], { type: f.type })), name: f.name, type: f.type, expires: exp, size: f.bytes.length };
        });
      });
      cache[id].catch(function () { delete cache[id]; });
    }
    return cache[id];
  }

  function size(n) {
    return n < 1024 ? n + ' B' : n < 1048576 ? (n / 1024).toFixed(0) + ' KB' : (n / 1048576).toFixed(1) + ' MB';
  }

  // Markdown → placeholder spans, done before marked/DOMPurify (which drop
  // unknown URL schemes).
  function preprocess(line) {
    return line.replace(REF, function (_, bang, alt, id, key) {
      return '<span class="nhfile" data-id="' + id + '" data-key="' + key + '" data-alt="' + alt.replace(/[&<>"]/g, function (c) {
        return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c];
      }) + '"></span>';
    });
  }

  function draw(span) {
    var id = span.dataset.id, key = span.dataset.key, alt = span.dataset.alt;
    if (media[id]) { span.replaceWith(media[id]); return; }
    span.classList.add('loading');
    span.textContent = NH.t('file.loading', { name: alt || '…' });
    load(id, key).then(function (f) {
      var main = null, kind = f.type.split('/')[0];
      if (kind === 'image') main = h('img', { src: f.url, alt: alt || f.name, loading: 'lazy' });
      else if (kind === 'audio') main = h('audio', { src: f.url, controls: true, preload: 'metadata' });
      else if (kind === 'video') main = h('video', { src: f.url, controls: true, preload: 'metadata', playsinline: true });
      var dl = h('a', { href: f.url, download: f.name }, NH.icon('download'), f.name);
      var box = h('span.attach' + (main ? '.attach-' + kind : ''), main,
        h('span.attach-meta', dl, h('span.faint', size(f.size) + (f.expires ? ' · ' + NH.t('file.expires', { time: f.expires }) : ''))));
      if (main && kind !== 'image') media[id] = box;
      if (span.isConnected) span.replaceWith(box);
    }, function () {
      span.classList.remove('loading');
      span.classList.add('gone');
      span.replaceChildren(NH.icon('clock'), h('span', NH.t('file.gone_named', { name: alt || id })));
    });
  }

  function secs(m) { return (+(m[1] || 0)) * 3600 + (+m[2]) * 60 + (+m[3]); }

  // [1:23] / [1:02:03] → button that seeks the media above it (or the first one).
  function timestamps(root) {
    var walk = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
      acceptNode: function (n) {
        return n.parentNode.closest('pre, code, a, button, .locked, .burnblk') || !HAS_TS.test(n.nodeValue) ? NodeFilter.FILTER_REJECT : NodeFilter.FILTER_ACCEPT;
      }
    });
    var nodes = [];
    for (var n = walk.nextNode(); n; n = walk.nextNode()) nodes.push(n);
    nodes.forEach(function (node) {
      var text = node.nodeValue, out = [], last = 0, m;
      TS.lastIndex = 0;
      while ((m = TS.exec(text))) {
        if (m.index > last) out.push(document.createTextNode(text.slice(last, m.index)));
        var b = h('button.ts', { type: 'button', 'data-t': secs(m) }, NH.icon('play'), m[0].slice(1, -1));
        out.push(b);
        last = TS.lastIndex;
      }
      if (last < text.length) out.push(document.createTextNode(text.slice(last)));
      node.replaceWith.apply(node, out);
    });
  }

  function seek(btn) {
    var root = btn.closest('.md') || document.body;
    var all = NH.$$('audio, video', root), target = null;
    all.forEach(function (m) { if (m.compareDocumentPosition(btn) & Node.DOCUMENT_POSITION_FOLLOWING) target = m; });
    target = target || all[0];
    if (!target) return NH.toast(NH.t('file.no_media'));
    target.currentTime = +btn.dataset.t;
    target.play().catch(function () {});
  }
  document.addEventListener('click', function (e) {
    var b = e.target.closest('button.ts');
    if (b) { e.preventDefault(); seek(b); }
  });

  // Media last played on the page: the source for the "timestamp" command.
  var lastMedia = null;
  document.addEventListener('play', function (e) { lastMedia = e.target; }, true);
  function stamp() {
    var m = lastMedia && lastMedia.isConnected ? lastMedia : NH.$('audio, video');
    if (!m) return '';
    var t = Math.floor(m.currentTime), hh = Math.floor(t / 3600), mm = Math.floor(t / 60) % 60, ss = t % 60;
    return '[' + (hh ? hh + ':' + String(mm).padStart(2, '0') : mm) + ':' + String(ss).padStart(2, '0') + ']';
  }

  function render(root) {
    NH.$$('span.nhfile', root).forEach(draw);
    timestamps(root);
  }

  // Images are re-encoded as WebP (max 2560px) when that makes them smaller:
  // screenshots shrink a lot, and EXIF (location!) is dropped on the way.
  function webp(file) {
    if (!/^image\/(png|jpeg|bmp)$/.test(file.type) || !window.createImageBitmap) return Promise.resolve(file);
    return createImageBitmap(file).then(function (img) {
      var scale = Math.min(1, 2560 / Math.max(img.width, img.height));
      var c = document.createElement('canvas');
      c.width = Math.round(img.width * scale); c.height = Math.round(img.height * scale);
      c.getContext('2d').drawImage(img, 0, 0, c.width, c.height);
      return new Promise(function (res) { c.toBlob(res, 'image/webp', 0.85); });
    }).then(function (b) {
      if (!b || b.type !== 'image/webp' || b.size >= file.size) return file;
      return new File([b], (file.name || 'image').replace(/\.\w+$/, '') + '.webp', { type: 'image/webp' });
    }, function () { return file; });
  }

  // upload(file) → markdown that embeds it.
  function upload(file, pasted) {
    var name = file.name || (pasted ? 'pasted' : 'file');
    if (pasted && /^image\//.test(file.type) && !file.name) name = 'image-' + new Date().toISOString().slice(0, 19).replace(/[-:T]/g, '');
    return (pasted ? webp(new File([file], name, { type: file.type })) : Promise.resolve(file)).then(function (f) {
      return f.arrayBuffer().then(function (buf) { return NHC.fileSeal(new Uint8Array(buf), f.name || name, f.type || 'application/octet-stream'); })
        .then(function (sealed) {
          return NH.api('POST', '/api/files', sealed.data).then(function (r) {
            var inline = /^(image|audio|video)\//.test(f.type);
            cache[r.id] = Promise.resolve({ url: URL.createObjectURL(f), name: f.name || name, type: f.type, expires: r.expires, size: f.size });
            return (inline ? '!' : '') + '[' + (f.name || name).replace(/[\[\]\n]/g, '') + '](nh-file:' + r.id + '#' + sealed.key + ')';
          });
        });
    });
  }

  window.NHFiles = { preprocess: preprocess, render: render, upload: upload, stamp: stamp };
})();
