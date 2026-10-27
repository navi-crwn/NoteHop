// "New note" page: encrypt in the browser, create, then jump to the note as admin.
(function () {
  'use strict';
  var $ = NH.$;
  var ed = NHEditor($('[data-editor]'), { onSave: function () { $('#create').click(); } });
  var form = $('#compose'), err = $('#compose-err'), btn = $('#create');
  var slug = $('#slug'), slugHint = $('#slug-hint');
  var submitted = false;

  if (!NHC.available) {
    err.textContent = 'Browser ini tidak mendukung WebCrypto (butuh HTTPS atau localhost). Catatan tidak bisa dienkripsi.';
    err.classList.remove('hide');
    btn.disabled = true;
  }

  // ---------- options: mode pills, feature toggles, conflicts ----------
  var MODE_DESC = {
    read: 'Hanya membaca. Tidak bisa mengubah apa pun.',
    edit: 'Siapa pun yang punya link bisa mengubah isinya.',
    append: 'Menambah baris di bawah; tidak bisa mengubah atau menghapus yang sudah ada.',
    check: 'Isinya terkunci, tapi checkbox bisa dicentang.'
  };
  // A feature that would clash with the current choices is switched off and
  // dimmed; clicking it explains why instead of turning it on.
  var RULES = [
    { feature: 'password', when: function () { return !on('encrypt'); }, why: 'Password butuh enkripsi E2E. Nyalakan Enkripsi E2E dulu.' },
    { feature: 'burn', when: function () { return mode() !== 'read'; }, why: 'Sekali baca hanya untuk mode Baca: pengedit akan menghabiskan jatah bukanya.' },
    { feature: 'lock', when: function () { return mode() === 'read'; }, why: 'Kunci otomatis untuk link Edit, Tambah, atau Centang; mode Baca sudah tidak bisa mengubah.' }
  ];
  function toggle(name) { return form.querySelector('[data-feature="' + name + '"]'); }
  function on(name) { return toggle(name).checked; }
  function mode() { return form.querySelector('[name=mode]:checked').value; }
  function blockedBy(name) {
    for (var i = 0; i < RULES.length; i++) if (RULES[i].feature === name && RULES[i].when()) return RULES[i];
    return null;
  }

  function applyOptions() {
    RULES.forEach(function (r) {
      var input = toggle(r.feature), blocked = r.when();
      if (blocked && input.checked) {
        input.checked = false;
        NH.toast(r.why);
      }
      input.closest('.pill-opt').classList.toggle('blocked', blocked);
      input.closest('.pill-opt').title = blocked ? r.why : '';
    });
    NH.$$('[data-panel]', form).forEach(function (p) { p.classList.toggle('hide', !on(p.dataset.panel)); });
    $('[data-for="encrypt-off"]').classList.toggle('hide', on('encrypt'));
    $('#mode-desc').textContent = MODE_DESC[mode()];
    btn.querySelector('span').textContent = on('encrypt') ? 'Kunci & buat link' : 'Buat link publik';
    slugAdvice();
  }
  NH.$$('[data-feature]', form).forEach(function (input) {
    input.addEventListener('click', function (e) {
      var r = blockedBy(input.dataset.feature);
      if (r) { e.preventDefault(); NH.toast(r.why); }
    });
  });
  form.addEventListener('change', applyOptions);

  function slugAdvice() {
    slug.value = slug.value.toLowerCase().replace(/[^a-z0-9-]/g, '-').replace(/^-+/, '');
    if (slug.value && mode() !== 'read') {
      slugHint.textContent = 'Hati-hati: link mudah ditebak + mode ' + NH.modeInfo[mode()].label.toLowerCase() + '. Untuk catatan terenkripsi, tanpa kunci #k orang tetap tidak bisa membaca atau mengubah isinya.';
      slugHint.classList.add('warn');
    } else {
      slugHint.textContent = 'Link mudah ditebak cocoknya untuk mode baca.';
      slugHint.classList.remove('warn');
    }
  }
  slug.addEventListener('input', slugAdvice);

  function localInput(unix) {
    var d = new Date(unix * 1000), p = function (n) { return String(n).padStart(2, '0'); };
    return d.getFullYear() + '-' + p(d.getMonth() + 1) + '-' + p(d.getDate()) + 'T' + p(d.getHours()) + ':' + p(d.getMinutes());
  }
  function unixOf(id) {
    var t = new Date($(id).value).getTime();
    return t ? Math.floor(t / 1000) : 0;
  }
  var soon = Math.floor(Date.now() / 1000);
  $('#open-at').value = localInput(soon + 86400);
  $('#lock-at').value = localInput(soon + 7 * 86400);
  applyOptions();

  // Times are checked together so they can't contradict each other.
  function timeProblem(openAt, lockAt, expiresIn) {
    var now = Math.floor(Date.now() / 1000);
    if (on('capsule') && openAt <= now) return 'Waktu buka kapsul harus di masa depan.';
    if (on('lock') && lockAt <= now) return 'Waktu kunci otomatis harus di masa depan.';
    if (on('capsule') && on('lock') && lockAt <= openAt) return 'Kunci otomatis harus setelah kapsul waktu terbuka.';
    if (on('capsule') && expiresIn && now + expiresIn <= openAt) return 'Catatan akan kedaluwarsa sebelum kapsul waktu terbuka. Perpanjang kedaluwarsa atau majukan waktu buka.';
    if (on('lock') && expiresIn && now + expiresIn <= lockAt) return 'Catatan akan kedaluwarsa sebelum kunci otomatis berlaku.';
    return '';
  }

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
    var encrypted = on('encrypt'), password = encrypted && on('password') ? $('#password').value : '';
    if (on('password') && !password) return fail('Isi password, atau matikan fitur Password.');
    if (on('slug') && !slug.value.trim()) return fail('Isi alamat kustom, atau matikan fitur Alamat kustom.');
    var expiresIn = on('expires') ? +$('#expires').value : 0;
    var openAt = on('capsule') ? unixOf('#open-at') : 0, lockAt = on('lock') ? unixOf('#lock-at') : 0;
    var problem = timeProblem(openAt, lockAt, expiresIn);
    if (problem) return fail(problem);
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
          slug: on('slug') ? slug.value.trim() : '', expires_in: expiresIn, max_views: on('burn') ? +$('#burn').value : 0,
          open_at: openAt, lock_at: lockAt, admin: admin, write: keys.write, owner_blob: r[1]
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
