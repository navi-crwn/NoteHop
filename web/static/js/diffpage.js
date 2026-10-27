// The diff page: compare two texts in the browser, and share the result as
// an encrypted read-only note.
(function () {
  'use strict';
  var $ = NH.$, a = $('#diff-a'), b = $('#diff-b'), timer = null, ops = [];
  function run() {
    ops = NHDiff.lines(a.value, b.value);
    var st = NHDiff.stats(ops);
    $('#diff-stats').textContent = NH.t('diff.stats', { added: st.added, removed: st.removed });
    NHDiff.render($('#diff-out'), ops, { context: 3 });
  }
  [a, b].forEach(function (t) { t.addEventListener('input', function () { clearTimeout(timer); timer = setTimeout(run, 150); }); });
  $('#diff-swap').addEventListener('click', function () { var x = a.value; a.value = b.value; b.value = x; run(); });
  $('#diff-share').addEventListener('click', function () {
    if (!a.value && !b.value) return;
    var k = NHC.newKey(), admin = NHC.newKey(), title = NH.t('diffpage.title');
    var body = '```diff\n' + NHDiff.text(ops) + '\n```\n';
    NHC.noteKeys(k, '', '').then(function (keys) {
      return NHC.codec(true, keys.aes).encode({ t: title, b: body }).then(function (c) {
        return NH.api('POST', '/api/notes', { encrypted: true, password: false, salt: '', content: c, mode: 'read', slug: '',
          expires_in: 0, max_views: 0, admin: admin, write: keys.write, owner_blob: '' });
      });
    }).then(function (res) {
      NH.recent.add({ token: res.token, k: k, a: admin, t: title, created: Math.floor(Date.now() / 1000), expires: 0 });
      NH.copy(NH.noteURL(res.token, { k: k }), NH.t('diffpage.shared'));
    }, function (x) { NH.toast(x.message, 'err'); });
  });
  run();
})();
