// Interactive blocks inside a rendered note:
//   :::embed <note link>  — another note shown inline, refreshed while open
//   :::form Title         — a form whose answers are added as entries
(function () {
  'use strict';
  var h = NH.h;

  // ---------- live embeds ----------
  // The link carries the other note's key (#k=); this note's text is
  // encrypted, so the key stays as private as the link itself. Only the
  // path is used (/TOKEN, or the old /n/TOKEN): the short domain is the
  // same server's notes.
  var embeds = {}; // token → {keys, codec, version, title, body, els: Set}

  function parse(src) {
    try {
      var u = new URL(src, location.href), m = u.pathname.match(/^\/(?:n\/)?([A-Za-z0-9_-]{3,64})$/);
      var k = new URLSearchParams(u.hash.slice(1)).get('k');
      return m && k ? { token: m[1], k: k } : null;
    } catch (x) { return null; }
  }

  function fetchEmbed(e) {
    return NH.api('POST', '/api/n/' + encodeURIComponent(e.token) + '/open', { poll: true }, { 'X-Write': e.keys.write }).then(function (d) {
      if (d.version === e.version) return false;
      return e.codec.decode(d.content).then(function (n) {
        e.version = d.version; e.title = n.t || ''; e.body = n.b || ''; e.type = { y: n.y, l: n.l };
        return true;
      });
    });
  }

  function drawEmbed(box, e, opts) {
    var body = h('div.md.embed-body', { dir: 'auto' });
    // One level only: an embed inside an embed stays a link.
    NHMD.renderAs(body, e.type, e.body, { noEmbed: true, copy: false });
    box.replaceChildren(h('div.embed-head', NH.icon('link'), h('b', e.title || NH.t('common.untitled')),
      h('a.embed-open', { href: box.dataset.src, target: '_blank', rel: 'noopener' }, NH.t('embed.open'))), body);
  }

  function embed(box, opts) {
    var ref = parse(box.dataset.src);
    if (!ref || opts.noEmbed) {
      box.replaceChildren(h('a', { href: box.dataset.src, target: '_blank', rel: 'noopener noreferrer' }, NH.icon('link'), box.dataset.src || NH.t('embed.bad')));
      return;
    }
    var e = embeds[ref.token];
    if (!e) {
      e = embeds[ref.token] = { token: ref.token, els: new Set(), ready: null };
      e.ready = NHC.noteKeys(ref.k, '', '').then(function (keys) {
        e.keys = keys;
        e.codec = NHC.codec(true, keys.aes);
        return NH.api('GET', '/api/n/' + encodeURIComponent(ref.token));
      }).then(function (m) {
        if (m.password) throw new Error(NH.t('embed.password'));
        if (m.max_views > 0) throw new Error(NH.t('embed.burn'));
        e.codec = NHC.codec(m.encrypted, e.keys.aes);
        return fetchEmbed(e);
      });
    }
    e.els.add(box);
    box.classList.add('loading');
    box.textContent = NH.t('embed.loading');
    e.ready.then(function () { box.classList.remove('loading'); drawEmbed(box, e, opts); }, function (x) {
      box.classList.remove('loading');
      box.replaceChildren(NH.icon('link'), h('span.faint', ' ' + (x.status === 404 ? NH.t('embed.gone') : x.message)));
    });
  }

  // Refresh every embed on screen every 10 s while the page is visible.
  setInterval(function () {
    if (document.hidden) return;
    Object.keys(embeds).forEach(function (t) {
      var e = embeds[t];
      e.els.forEach(function (b) { if (!b.isConnected) e.els.delete(b); });
      if (!e.els.size || !e.keys) return;
      fetchEmbed(e).then(function (changed) { if (changed) e.els.forEach(function (b) { drawEmbed(b, e, {}); }); }, function () {});
    });
  }, 10000);

  // ---------- forms ----------
  function field(spec, i) {
    var m = spec.match(/^(.*?)\s*(?:\((long|email|number|date)\)|\[([^\]]+)\])?\s*(\*)?$/) || [], label = (m[1] || spec).trim();
    var kind = m[2] || (m[3] ? 'choice' : 'text'), req = !!m[4], input;
    var id = 'f' + Math.random().toString(36).slice(2, 8) + i;
    if (kind === 'long') input = h('textarea', { id: id, rows: 3, dir: 'auto', maxlength: 4000 });
    else if (kind === 'choice') {
      input = h('select', { id: id }, [h('option', { value: '' }, '—')].concat(m[3].split('|').map(function (o) {
        o = o.trim();
        return h('option', { value: o }, o);
      })));
    } else input = h('input', { id: id, type: kind, dir: 'auto', maxlength: 500 });
    if (req) input.required = true;
    return { label: label, input: input, el: h('label.field', { for: id }, h('span', label + (req ? ' *' : '')), input) };
  }

  function form(box, opts) {
    var fields = [];
    try { fields = JSON.parse(box.dataset.fields || '[]'); } catch (x) {}
    var list = fields.map(field), N = window.NHNote, can = !!(N && N.canAppend && N.canAppend());
    var btn = h('button.btn.primary', { type: 'submit' }, NH.icon('check'), NH.t('form.send'));
    var f = h('form.nhform-body', list.map(function (x) { return x.el; }), h('div.btn-row', btn));
    if (!can) {
      list.forEach(function (x) { x.input.disabled = true; });
      btn.disabled = true;
      f.appendChild(h('p.hint', NH.t('form.needs_link')));
    }
    box.replaceChildren(h('div.nhform-head', NH.icon('edit'), h('b', box.dataset.title || NH.t('form.default_title'))), f);
    f.addEventListener('submit', function (e) {
      e.preventDefault();
      var lines = list.filter(function (x) { return x.input.value.trim(); }).map(function (x) {
        return '**' + x.label.replace(/\*/g, '') + ':** ' + x.input.value.trim().replace(/\n/g, '  \n');
      });
      if (!lines.length) return;
      var name = list.length && list[0].input.value.trim() ? list[0].input.value.trim().slice(0, 80) : '';
      btn.disabled = true;
      N.addEntry(name, (box.dataset.title ? '### ' + box.dataset.title + '\n\n' : '') + lines.join('\n')).then(function () {
        f.reset();
        NH.toast(NH.t('form.sent'), 'ok');
      }, function (x) { NH.toast(x.message, 'err'); }).finally(function () { btn.disabled = false; });
    });
  }

  window.NHBlocks = {
    render: function (root, opts) {
      opts = opts || {};
      NH.$$('.embed', root).forEach(function (b) { embed(b, opts); });
      NH.$$('.nhform', root).forEach(function (b) { form(b, opts); });
    }
  };
})();
