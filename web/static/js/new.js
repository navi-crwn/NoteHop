// "New note" page: encrypt in the browser, create, then jump to the note as admin.
(function () {
  'use strict';
  var $ = NH.$;
  var ed = NHEditor($('[data-editor]'), { onSave: function () { $('#create').click(); } });
  var form = $('#compose'), err = $('#compose-err'), btn = $('#create');
  var encrypt = $('#encrypt'), pwField = $('#pw-field'), slug = $('#slug'), slugHint = $('#slug-hint');
  var submitted = false;

  if (!NHC.available) {
    err.textContent = 'Browser ini tidak mendukung WebCrypto (butuh HTTPS atau localhost). Catatan tidak bisa dienkripsi.';
    err.classList.remove('hide');
    btn.disabled = true;
  }

  encrypt.addEventListener('change', function () {
    pwField.classList.toggle('hide', !encrypt.checked);
    btn.querySelector('span').textContent = encrypt.checked ? 'Kunci & buat link' : 'Buat link publik';
  });

  function slugAdvice() {
    var mode = form.querySelector('[name=mode]:checked').value;
    slug.value = slug.value.toLowerCase().replace(/[^a-z0-9-]/g, '-').replace(/^-+/, '');
    if (slug.value && mode !== 'read') {
      slugHint.textContent = 'Hati-hati: link mudah ditebak + mode ' + NH.modeInfo[mode].label.toLowerCase() + '. Untuk catatan terenkripsi, tanpa kunci #k orang tetap tidak bisa membaca atau mengubah isinya.';
      slugHint.classList.add('warn');
    } else {
      slugHint.textContent = 'Kosongkan untuk link acak. Link mudah ditebak cocoknya untuk mode baca.';
      slugHint.classList.remove('warn');
    }
  }
  slug.addEventListener('input', slugAdvice);
  form.addEventListener('change', slugAdvice);

  function fail(msg) {
    err.textContent = msg;
    err.classList.remove('hide');
    btn.disabled = false;
  }

  form.addEventListener('submit', function (e) {
    e.preventDefault();
    err.classList.add('hide');
    var title = $('#title').value.trim(), body = ed.value;
    if (!title && !body.trim()) return fail('Catatan masih kosong.');
    var encrypted = encrypt.checked, password = encrypted ? $('#password').value : '';
    var mode = form.querySelector('[name=mode]:checked').value;
    var k = NHC.newKey(), admin = NHC.newKey(), salt = password ? NHC.newSalt() : '';
    btn.disabled = true;

    NHC.noteKeys(k, password, salt).then(function (keys) {
      var codec = NHC.codec(encrypted, keys.aes), vault = NHC.vault();
      return Promise.all([
        codec.encode({ t: title, b: body }),
        vault ? vault.seal({ t: title, k: k, a: admin, p: password }) : ''
      ]).then(function (r) {
        return NH.api('POST', '/api/notes', {
          encrypted: encrypted, password: !!password, salt: salt, content: r[0], mode: mode,
          slug: slug.value.trim(), expires_in: +$('#expires').value, max_views: +$('#burn').value,
          admin: admin, write: keys.write, owner_blob: r[1]
        });
      });
    }).then(function (res) {
      submitted = true;
      if ($('#remember').checked) {
        NH.recent.add({ token: res.token, k: k, a: admin, t: title, created: Math.floor(Date.now() / 1000), expires: res.expires, p: !!password });
      }
      // Hand the password to the next page without putting it in the URL.
      if (password) try { sessionStorage.setItem('nh_pw_' + res.token, password); } catch (x) {}
      location.href = NH.noteURL(res.token, { k: k, a: admin, new: '1' });
    }).catch(function (x) { fail(x.message || 'Gagal membuat catatan.'); });
  });

  window.addEventListener('beforeunload', function (e) {
    if (!submitted && (ed.value.trim() || $('#title').value.trim())) { e.preventDefault(); e.returnValue = ''; }
  });

  // ---------- on this device ----------
  function drawRecent() {
    var list = NH.recent.all(), box = $('#recent'), ul = $('#recent-list');
    box.classList.toggle('hide', !list.length);
    ul.replaceChildren.apply(ul, list.map(function (n) {
      var gone = n.expires && n.expires < Date.now() / 1000;
      var rm = NH.h('button.icon-btn.sm', { type: 'button', title: 'Lupakan dari perangkat ini', 'aria-label': 'Lupakan' }, NH.icon('x'));
      rm.addEventListener('click', function () { NH.recent.remove(n.token); drawRecent(); });
      return NH.h('li.note-row' + (gone ? '.gone' : ''),
        NH.h('a.note-link', { href: NH.noteURL(n.token, { k: n.k, a: n.a }) }, NH.h('b', n.t || 'Tanpa judul'),
          NH.h('small.muted', '/n/' + n.token + ' · ' + NH.ago(n.created) + (gone ? ' · kedaluwarsa' : '') + (n.p ? ' · berpassword' : ''))),
        rm);
    }));
  }
  $('#recent-clear').addEventListener('click', function () {
    if (confirm('Hapus semua link admin yang tersimpan di perangkat ini? Catatannya sendiri tidak terhapus.')) { NH.recent.clear(); drawRecent(); }
  });
  drawRecent();
  $('#title').focus();
})();
