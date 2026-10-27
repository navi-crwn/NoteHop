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
      ul.replaceChildren(h('li.empty', h('p.muted', NH.t('dash.empty')), h('a.btn.primary', { href: '/' }, NH.icon('plus'), NH.t('dash.write_first'))));
      return;
    }
    ul.replaceChildren.apply(ul, shown.map(function (r) {
      var n = r.n, info = NH.modeInfo[n.mode];
      var meta = [info.label, NH.t('meta.changed', { ago: NH.ago(n.updated) })];
      if (n.frozen) meta.unshift(NH.t('meta.snapshot'));
      if (n.expires) meta.push(NH.t('meta.disappears', { ago: NH.ago(n.expires) }));
      if (n.max_views) meta.push(NH.t('meta.opened', { n: n.views, max: n.max_views }));
      var href = r.k ? NH.noteURL(n.token, { k: r.k, a: r.a }) : null;
      var link = h(href ? 'a.note-link' : 'div.note-link', href ? { href: href } : null,
        h('b', r.locked ? NH.t('dash.locked') : (r.title || NH.t('common.untitled'))), h('small.muted', '/n/' + n.token + ' · ' + meta.join(' · ')));
      if (href && r.p) link.addEventListener('click', function () {
        try { sessionStorage.setItem('nh_pw_' + n.token, r.p); } catch (x) {}
      });
      var acts = h('span.btn-row');
      if (r.k) {
        var copy = h('button.btn.sm', { type: 'button', title: NH.t('dash.copy_link', { mode: info.label }) }, NH.icon('copy'), NH.t('dash.link'));
        copy.addEventListener('click', function () { NH.copy(NH.noteURL(n.token, { k: r.k }), NH.t('js.link_copied')); });
        acts.appendChild(copy);
      }
      var del = h('button.icon-btn.sm', { type: 'button', title: NH.t('common.delete'), 'aria-label': NH.t('dash.delete_note') }, NH.icon('trash'));
      del.addEventListener('click', function () {
        if (!confirm(NH.t('dash.confirm_delete', { title: r.title || NH.t('common.untitled') }))) return;
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
