// Login / register. The password never leaves the browser: it is stretched
// into a vault key (kept here) and an auth secret (sent to the server).
(function () {
  'use strict';
  var $ = NH.$;
  var form = $('#auth-form');
  if (!form) return;
  var kind = form.dataset.kind, err = $('#auth-err'), btn = form.querySelector('button[type=submit]');

  function fail(msg) {
    err.textContent = msg;
    err.classList.remove('hide');
    btn.disabled = false;
    btn.querySelector('span').textContent = NH.t(kind === 'login' ? 'auth.sign_in' : 'auth.sign_up');
  }

  form.addEventListener('submit', function (e) {
    e.preventDefault();
    err.classList.add('hide');
    var username = $('#username').value.trim().toLowerCase(), password = $('#password').value;
    if (kind === 'register' && password !== $('#password2').value) return fail(NH.t('js.password_mismatch'));
    if (!NHC.available) return fail(NH.t('js.no_webcrypto'));
    btn.disabled = true;
    btn.querySelector('span').textContent = NH.t('js.deriving');
    NHC.account(username, password).then(function (k) {
      return NH.api('POST', '/api/' + kind, { username: username, auth: k.auth }).then(function (res) {
        NH.store.set('nh_vault', { u: res.username, k: k.vault });
        var next = new URLSearchParams(location.search).get('next') || '/dashboard';
        location.href = /^\/(?!\/)/.test(next) ? next : '/dashboard';
      });
    }).catch(function (x) { fail(x.message); });
  });
})();
