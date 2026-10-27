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
    btn.querySelector('span').textContent = kind === 'login' ? 'Masuk' : 'Daftar';
  }

  form.addEventListener('submit', function (e) {
    e.preventDefault();
    err.classList.add('hide');
    var username = $('#username').value.trim().toLowerCase(), password = $('#password').value;
    if (kind === 'register' && password !== $('#password2').value) return fail('Password tidak sama.');
    if (!NHC.available) return fail('Browser ini tidak mendukung WebCrypto (butuh HTTPS atau localhost).');
    btn.disabled = true;
    btn.querySelector('span').textContent = 'Menurunkan kunci…';
    NHC.account(username, password).then(function (k) {
      return NH.api('POST', '/api/' + kind, { username: username, auth: k.auth }).then(function (res) {
        NH.store.set('nh_vault', { u: res.username, k: k.vault });
        var next = new URLSearchParams(location.search).get('next') || '/dashboard';
        location.href = /^\/(?!\/)/.test(next) ? next : '/dashboard';
      });
    }).catch(function (x) { fail(x.message); });
  });
})();
