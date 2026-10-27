// Forgot the password (or the vault phrase): one recovery code opens the
// sealed vault key here, then a new password seals it again (see recovery.go).
(function () {
  'use strict';
  var $ = NH.$;
  var form = $('#rec-form');
  if (!form) return;
  var sso = form.dataset.sso || '', err = $('#rec-err'), btn = form.querySelector('button[type=submit]');
  var step = 1, found = null; // {username, ticket, vault}

  function busy(on) {
    btn.disabled = on;
    btn.querySelector('span').textContent = NH.t(on ? 'js.deriving' : step === 1 ? 'rec.next' : 'rec.save');
  }
  function fail(msg) { err.textContent = msg; err.classList.remove('hide'); busy(false); }

  form.addEventListener('submit', function (e) {
    e.preventDefault();
    err.classList.add('hide');
    if (!NHC.available) return fail(NH.t('js.no_webcrypto'));
    if (step === 1) {
      var username = sso ? form.dataset.user : $('#username').value.trim().toLowerCase();
      busy(true);
      NHC.recoveryKeys($('#code').value, username).then(function (rk) {
        return NH.api('POST', '/api/recover', { username: sso ? '' : username, proof: rk.proof }).then(function (res) {
          return rk.open(res.wrap).then(function (v) { found = { username: res.username, ticket: res.ticket, vault: v }; });
        });
      }).then(function () {
        step = 2;
        $('#rec-step1').classList.add('hide');
        $('#rec-step2').classList.remove('hide');
        $('#st1').classList.replace('on', 'done');
        $('#st2').classList.add('on');
        $('#rec-ok').textContent = NH.t('rec.ok', { user: found.username });
        $('#new1').required = $('#new2').required = true;
        busy(false);
        $('#new1').focus();
      }, function (x) { fail(x.message); });
      return;
    }
    var next = $('#new1').value;
    if (next !== $('#new2').value) return fail(NH.t(sso ? 'js.vault_mismatch' : 'js.password_mismatch'));
    busy(true);
    (sso ? NHC.vaultPhrase(next, sso) : NHC.account(found.username, next)).then(function (k) {
      return NHC.sealVault(k.vault, found.vault).then(function (wrap) {
        return NH.api('POST', '/api/recover/finish', { ticket: found.ticket, next: sso ? k.check : k.auth, vault_wrap: wrap });
      });
    }).then(function (res) {
      NHC.remember(res.username, found.vault);
      document.body.dataset.user = res.username;
      form.classList.add('hide');
      $('#rec-done-p').textContent = NH.t(res.left > 2 ? 'rec.done_p' : 'rec.done_low', { n: res.left });
      $('#rec-done').classList.remove('hide');
    }, function (x) { fail(x.message); });
  });
})();
