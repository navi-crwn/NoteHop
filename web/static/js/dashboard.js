// "My notes": the server returns opaque blobs; this browser's vault key opens them.
(function () {
  'use strict';
  var $ = NH.$, h = NH.h;
  var ul = $('#mine'), filter = $('#filter'), rows = [];
  var vault = NHC.vault();
  if (!vault) $('#vault-locked').classList.remove('hide');

  function draw() {
    var q = filter.value.trim().toLowerCase();
    var shown = rows.filter(function (r) { return !q || (r.title || '').toLowerCase().indexOf(q) !== -1; });
    if (!rows.length) {
      ul.replaceChildren(h('li.empty', h('p.muted', 'Belum ada catatan di akun ini.'), h('a.btn.primary', { href: '/' }, NH.icon('plus'), 'Tulis yang pertama')));
      return;
    }
    ul.replaceChildren.apply(ul, shown.map(function (r) {
      var n = r.n, info = NH.modeInfo[n.mode];
      var meta = [info.label, 'diubah ' + NH.ago(n.updated)];
      if (n.frozen) meta.unshift('snapshot');
      if (n.expires) meta.push('hilang ' + NH.ago(n.expires));
      if (n.max_views) meta.push('dibuka ' + n.views + '/' + n.max_views);
      var href = r.k ? NH.noteURL(n.token, { k: r.k, a: r.a }) : null;
      var link = h(href ? 'a.note-link' : 'div.note-link', href ? { href: href } : null,
        h('b', r.locked ? 'Terkunci' : (r.title || 'Tanpa judul')), h('small.muted', '/n/' + n.token + ' · ' + meta.join(' · ')));
      if (href && r.p) link.addEventListener('click', function () {
        try { sessionStorage.setItem('nh_pw_' + n.token, r.p); } catch (x) {}
      });
      var acts = h('span.btn-row');
      if (r.k) {
        var copy = h('button.btn.sm', { type: 'button', title: 'Salin link ' + info.label.toLowerCase() }, NH.icon('copy'), 'Link');
        copy.addEventListener('click', function () { NH.copy(NH.noteURL(n.token, { k: r.k }), 'Link disalin'); });
        acts.appendChild(copy);
      }
      var del = h('button.icon-btn.sm', { type: 'button', title: 'Hapus', 'aria-label': 'Hapus catatan' }, NH.icon('trash'));
      del.addEventListener('click', function () {
        if (!confirm('Hapus "' + (r.title || 'Tanpa judul') + '" selamanya?')) return;
        NH.api('DELETE', '/api/n/' + encodeURIComponent(n.token), undefined, r.a ? { 'X-Admin': r.a } : {}).then(function () {
          rows = rows.filter(function (x) { return x !== r; });
          NH.recent.remove(n.token);
          draw();
        }, function (x) { NH.toast(x.message, 'err'); });
      });
      acts.appendChild(del);
      return h('li.note-row', h('span.mode-ico', { title: info.label }, NH.icon(n.frozen ? 'snow' : info.icon)), link, acts);
    }));
  }

  NH.api('GET', '/api/me/notes').then(function (res) {
    return Promise.all(res.notes.map(function (n) {
      if (!vault || !n.blob) return { n: n, locked: true };
      return vault.open(n.blob).then(function (v) { return { n: n, title: v.t, k: v.k, a: v.a, p: v.p }; },
        function () { return { n: n, locked: true }; });
    }));
  }).then(function (r) { rows = r; draw(); }, function (x) {
    ul.replaceChildren(h('li.muted', x.message));
  });
  filter.addEventListener('input', draw);
})();
