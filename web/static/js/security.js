// Security page: recovery codes and changing the password (or the vault
// phrase of a single sign-on account). Codes are made here and shown once;
// the server gets only what crypto.js derives from them (see recovery.go).
(function () {
  'use strict';
  var $ = NH.$, h = NH.h;
  var page = $('#security');
  if (!page) return;
  var sub = page.dataset.sub, sso = !!sub, user = document.body.dataset.user;
  var me = null, codes = [];

  // The account's current secret: the login secret (password) or the vault
  // check (phrase), plus the vault key it unlocks.
  function current(secret) {
    var k = sso ? NHC.vaultPhrase(secret, sub) : NHC.account(user, secret);
    return k.then(function (k) {
      return NHC.vaultKey(k.vault, me.vault_wrap).then(function (v) { return { proof: sso ? k.check : k.auth, vault: v }; },
        function () { throw new Error(NH.t(sso ? 'err.vault_wrong' : 'err.password_wrong')); });
    });
  }

  function drawState() {
    var r = me.recovery, st = $('#rec-state');
    st.className = 'rec-state ' + (r.total ? (r.left > 2 ? 'good' : 'low') : 'none');
    st.textContent = r.total ? NH.t('sec.left', { n: r.left, total: r.total }) : NH.t('sec.none');
    $('#rec-make span').textContent = NH.t(r.total ? 'sec.remake' : 'sec.make');
    $('#confirm-warn').classList.toggle('hide', !r.total);
  }
  NH.api('GET', '/api/me').then(function (m) { me = m; drawState(); }, function (x) { NH.toast(x.message, 'err'); });

  // ---------- making codes ----------
  var dlg = $('#confirm-dlg'), cerr = $('#confirm-err');
  $('#rec-make').addEventListener('click', function () {
    if (!me) return;
    $('#confirm-form').reset();
    cerr.classList.add('hide');
    dlg.showModal();
    $('#confirm-secret').focus();
  });
  $('#confirm-form').addEventListener('submit', function (e) {
    e.preventDefault();
    var btn = this.querySelector('button[type=submit]'), label = btn.querySelector('span').textContent;
    cerr.classList.add('hide');
    btn.disabled = true;
    btn.querySelector('span').textContent = NH.t('js.deriving');
    var made = [];
    for (var i = 0; i < 8; i++) made.push(NHC.newRecoveryCode());
    current($('#confirm-secret').value).then(function (cur) {
      return Promise.all(made.map(function (c) {
        return NHC.recoveryKeys(c, user).then(function (rk) { return rk.seal(cur.vault).then(function (w) { return { proof: rk.proof, wrap: w }; }); });
      })).then(function (list) {
        return NH.api('POST', '/api/me/recovery', { current: cur.proof, codes: list });
      }).then(function (status) {
        NHC.remember(user, cur.vault); // the right secret: this browser can open the vault now
        me.recovery = status;
        codes = made;
        dlg.close();
        showCodes();
        drawState();
      });
    }).catch(function (x) {
      cerr.textContent = x.message;
      cerr.classList.remove('hide');
    }).finally(function () { btn.disabled = false; btn.querySelector('span').textContent = label; });
  });

  function showCodes() {
    $('#code-grid').replaceChildren.apply($('#code-grid'), codes.map(function (c) { return h('li', c); }));
    $('#codes-box').classList.remove('hide');
    $('#rec-make').classList.add('hide');
    $('#codes-saved').checked = false;
    $('#codes-done').disabled = true;
    $('#codes-box').scrollIntoView({ block: 'nearest' });
  }
  function codesText() {
    return [NH.t('sec.file_h', { user: user, site: location.host }), NH.t('sec.file_date', { date: new Date().toISOString().slice(0, 10) }),
      NH.t('sec.file_p'), ''].concat(codes.map(function (c, i) { return (i + 1) + '. ' + c; })).join('\n') + '\n';
  }
  $('#codes-dl').addEventListener('click', function () {
    var url = URL.createObjectURL(new Blob([codesText()], { type: 'text/plain;charset=utf-8' }));
    var a = h('a', { href: url, download: 'notehop-recovery-' + user + '.txt' });
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(function () { URL.revokeObjectURL(url); }, 2000);
  });
  $('#codes-copy').addEventListener('click', function () { NH.copy(codesText(), NH.t('sec.copied')); });
  $('#codes-print').addEventListener('click', function () {
    document.body.classList.add('print-codes');
    window.print();
    document.body.classList.remove('print-codes');
  });
  $('#codes-saved').addEventListener('change', function () { $('#codes-done').disabled = !this.checked; });
  // Once put away, the codes leave the page for good.
  $('#codes-done').addEventListener('click', function () {
    codes = [];
    $('#code-grid').replaceChildren();
    $('#codes-box').classList.add('hide');
    $('#rec-make').classList.remove('hide');
    NH.toast(NH.t('sec.done_toast'), 'ok');
  });
  window.addEventListener('beforeunload', function (e) { if (codes.length && !$('#codes-saved').checked) { e.preventDefault(); e.returnValue = ''; } });

  // ---------- new password / phrase ----------
  var sform = $('#secret-form'), serr = $('#secret-err');
  sform.addEventListener('submit', function (e) {
    e.preventDefault();
    serr.classList.add('hide');
    var next = $('#new1').value, btn = sform.querySelector('button[type=submit]'), label = btn.querySelector('span').textContent;
    var fail = function (msg) { serr.textContent = msg; serr.classList.remove('hide'); btn.disabled = false; btn.querySelector('span').textContent = label; };
    if (next !== $('#new2').value) return fail(NH.t(sso ? 'js.vault_mismatch' : 'js.password_mismatch'));
    if (!me) return;
    btn.disabled = true;
    btn.querySelector('span').textContent = NH.t('js.deriving');
    current($('#cur').value).then(function (cur) {
      var k = sso ? NHC.vaultPhrase(next, sub) : NHC.account(user, next);
      return k.then(function (k) {
        return NHC.sealVault(k.vault, cur.vault).then(function (wrap) {
          return NH.api('POST', '/api/me/secret', { current: cur.proof, next: sso ? k.check : k.auth, vault_wrap: wrap }).then(function () {
            me.vault_wrap = wrap;
            NHC.remember(user, cur.vault);
          });
        });
      });
    }).then(function () {
      sform.reset();
      btn.disabled = false;
      btn.querySelector('span').textContent = label;
      NH.toast(NH.t(sso ? 'sec.phrase_changed' : 'sec.password_changed'), 'ok');
    }, function (x) { fail(x.message); });
  });
})();
