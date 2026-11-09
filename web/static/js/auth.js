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
    var typed = $('#username').value.trim(), username = typed.toLowerCase(), password = $('#password').value;
    // An email address typed here belongs to the account server, not to this form: say so
    // instead of answering with the username rule.
    if (typed.indexOf('@') !== -1) {
      var hub = form.dataset.ssoName;
      return fail(hub ? NH.t('auth.email_not_here', { name: hub }) : NH.t('err.bad_username'));
    }
    if (kind === 'register' && password !== $('#password2').value) return fail(NH.t('js.password_mismatch'));
    if (!NHC.available) return fail(NH.t('js.no_webcrypto'));
    btn.disabled = true;
    btn.querySelector('span').textContent = NH.t('js.deriving');
    NHC.account(username, password).then(function (k) {
      return NH.api('POST', '/api/' + kind, { username: username, auth: k.auth }).then(function (res) {
        return NHC.vaultKey(k.vault, res.vault_wrap).then(function (v) { NHC.remember(res.username, v); });
      }).then(function () {
        var next = new URLSearchParams(location.search).get('next') || '/dashboard';
        location.href = /^\/(?![\/\\])[^\x00-\x1f\x7f\\]*$/.test(next) ? next : '/dashboard';
      });
    }).catch(function (x) { fail(x.message); });
  });
})();
