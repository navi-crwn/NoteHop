// Home page: the quick note. It is encrypted here exactly like the full
// create page (/new); the draft stays in this browser until the link is made,
// and "Full mode" carries it over to /new.
(function () {
  'use strict';
  var $ = NH.$, form = $('#quick-form');
  if (!form) return;
  var title = $('#quick-title'), body = $('#quick-body'), once = $('#quick-once'), err = $('#quick-err');
  var KEY = 'nh_quick';

  function mode() { return (form.querySelector('input[name=quick-mode]:checked') || {}).value || 'read'; }
  function draft() { return { t: title.value, b: body.value, m: mode(), o: once.checked }; }
  function save() { NH.store.set(KEY, draft()); }

  var d = NH.store.get(KEY, null);
  if (d) {
    title.value = d.t || ''; body.value = d.b || ''; once.checked = !!d.o;
    var r = form.querySelector('input[name=quick-mode][value="' + d.m + '"]');
    if (r) r.checked = true;
  }
  form.addEventListener('input', save);
  form.addEventListener('change', save);

  // Full mode: /new reads the draft once (see new.js) and the home page forgets it.
  $('#quick-full').addEventListener('click', function () {
    try { sessionStorage.setItem('nh_handoff', JSON.stringify(draft())); } catch (e) {}
    NH.store.del(KEY);
  });

  form.addEventListener('submit', function (e) {
    e.preventDefault();
    err.classList.add('hide');
    var t = title.value.trim(), b = body.value;
    if (!t && !b.trim()) {
      err.textContent = NH.t('home.quick_empty');
      err.classList.remove('hide');
      return body.focus();
    }
    var btn = $('#quick-create'), k = NHC.newKey(), admin = NHC.newKey(), vault = NHC.vault();
    btn.disabled = true;
    NHC.noteKeys(k, '', '').then(function (keys) {
      return Promise.all([NHC.codec(true, keys.aes).encode({ t: t, b: b }), vault ? vault.seal({ t: t, k: k, a: admin, p: '' }) : '']).then(function (r) {
        return NH.api('POST', '/api/notes', { encrypted: true, password: false, salt: '', content: r[0], mode: mode(), slug: '',
          expires_in: 0, max_views: once.checked ? 1 : 0, admin: admin, write: keys.write, owner_blob: r[1] });
      });
    }).then(function (res) {
      NH.recent.add({ token: res.token, k: k, a: admin, t: t, created: Math.floor(Date.now() / 1000), expires: res.expires });
      NH.store.del(KEY);
      location.href = NH.noteURL(res.token, { k: k, a: admin, new: '1' });
    }, function (x) {
      btn.disabled = false;
      err.textContent = x.message;
      err.classList.remove('hide');
    });
  });
})();
