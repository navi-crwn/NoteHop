// Vault phrase for single sign-on accounts. The phrase stays here; the server only
// gets a check value so it can say "wrong phrase" or remember a new one.
(function () {
  'use strict';
  var $ = NH.$;
  var form = $('#vault-form');
  if (!form) return;
  var next = form.dataset.next || '/dashboard', isNew = !!form.dataset.new;
  if (!/^\/(?![\/\\])/.test(next)) next = '/dashboard';
  // Already unlocked in this browser: nothing to ask.
  if (NHC.vault()) return location.replace(next);
  form.classList.remove('hide');
  var err = $('#vault-err'), btn = form.querySelector('button[type=submit]'), label = btn.querySelector('span').textContent;

  function fail(msg) {
    err.textContent = msg;
    err.classList.remove('hide');
    btn.disabled = false;
    btn.querySelector('span').textContent = label;
  }

  form.addEventListener('submit', function (e) {
    e.preventDefault();
    err.classList.add('hide');
    var phrase = $('#phrase').value;
    if (isNew && phrase !== $('#phrase2').value) return fail(NH.t('js.vault_mismatch'));
    if (!NHC.available) return fail(NH.t('js.no_webcrypto'));
    btn.disabled = true;
    btn.querySelector('span').textContent = NH.t('js.deriving');
    NHC.vaultPhrase(phrase, form.dataset.sub).then(function (k) {
      return NH.api('POST', '/api/vault', { check: k.check }).then(function (res) {
        return NHC.vaultKey(k.vault, res.vault_wrap);
      }).then(function (v) {
        NHC.remember(document.body.dataset.user, v);
        location.href = next;
      });
    }).catch(function (x) { fail(x.message); });
  });
})();
