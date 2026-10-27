// NoteHop crypto, all in the browser with WebCrypto.
//
// Note key K: 32 random bytes, carried in the link fragment (#k=…), which
// browsers never send to servers. With a password, the working secret is
// PBKDF2(password, K ‖ salt); without one it is K itself. From that secret:
//   enc   = HMAC(secret, "notehop/enc")   → AES-256-GCM key for content
//   write = HMAC(secret, "notehop/write") → proof sent with edits; the server
//           stores only its SHA-256, so only key holders can change a note.
(function () {
  'use strict';
  var subtle = window.crypto && window.crypto.subtle;
  var enc = new TextEncoder(), dec = new TextDecoder();

  function b64u(bytes) {
    var s = '';
    bytes = new Uint8Array(bytes);
    for (var i = 0; i < bytes.length; i += 0x8000) s += String.fromCharCode.apply(null, bytes.subarray(i, i + 0x8000));
    return btoa(s).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
  }
  function unb64u(str) {
    str = String(str).replace(/\s+/g, '').replace(/-/g, '+').replace(/_/g, '/');
    while (str.length % 4) str += '=';
    var bin = atob(str), out = new Uint8Array(bin.length);
    for (var i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
    return out;
  }
  function rand(n) { return crypto.getRandomValues(new Uint8Array(n)); }
  function concat(a, b) { var o = new Uint8Array(a.length + b.length); o.set(a); o.set(b, a.length); return o; }

  function pbkdf2(password, salt, iterations, bits) {
    return subtle.importKey('raw', enc.encode(password), 'PBKDF2', false, ['deriveBits']).then(function (k) {
      return subtle.deriveBits({ name: 'PBKDF2', salt: salt, iterations: iterations, hash: 'SHA-256' }, k, bits);
    }).then(function (b) { return new Uint8Array(b); });
  }
  function hmac(keyBytes, label) {
    return subtle.importKey('raw', keyBytes, { name: 'HMAC', hash: 'SHA-256' }, false, ['sign']).then(function (k) {
      return subtle.sign('HMAC', k, enc.encode(label));
    }).then(function (b) { return new Uint8Array(b); });
  }
  function aesKey(bytes) { return subtle.importKey('raw', bytes, 'AES-GCM', false, ['encrypt', 'decrypt']); }

  function seal(key, text) {
    var iv = rand(12);
    return subtle.encrypt({ name: 'AES-GCM', iv: iv }, key, enc.encode(text)).then(function (ct) {
      return b64u(concat(iv, new Uint8Array(ct)));
    });
  }
  function unseal(key, data) {
    var raw = unb64u(data);
    return subtle.decrypt({ name: 'AES-GCM', iv: raw.subarray(0, 12) }, key, raw.subarray(12)).then(function (pt) {
      return dec.decode(pt);
    });
  }

  var NOTE_ITER = 310000, ACCOUNT_ITER = 600000, PIN_ITER = 250000;

  window.NHC = {
    available: !!subtle,
    b64u: b64u,
    unb64u: unb64u,
    newKey: function () { return b64u(rand(32)); },
    newSalt: function () { return b64u(rand(16)); },

    // Returns {aes, write} for a note. Rejects if k is malformed.
    noteKeys: function (k, password, salt) {
      var key = unb64u(k);
      if (key.length !== 32) return Promise.reject(new Error('bad key'));
      var secret = password ? pbkdf2(password, concat(key, unb64u(salt)), NOTE_ITER, 256) : Promise.resolve(key);
      return secret.then(function (s) {
        return Promise.all([hmac(s, 'notehop/enc').then(aesKey), hmac(s, 'notehop/write')]);
      }).then(function (r) { return { aes: r[0], write: b64u(r[1]) }; });
    },

    // A codec turns {t, b} objects into what the server stores and back.
    // Public notes are plain JSON so /raw and curl can read them.
    codec: function (encrypted, aes) {
      return {
        encode: function (obj) { return encrypted ? seal(aes, JSON.stringify(obj)) : Promise.resolve(JSON.stringify(obj)); },
        decode: function (str) { return (encrypted ? unseal(aes, str) : Promise.resolve(str)).then(JSON.parse); }
      };
    },

    // PIN-locked blocks inside a note: salt ‖ iv ‖ ciphertext, one string.
    pinSeal: function (pin, text) {
      var salt = rand(16);
      return pbkdf2(pin, salt, PIN_ITER, 256).then(aesKey).then(function (k) {
        return seal(k, text);
      }).then(function (body) { return b64u(salt) + '.' + body; });
    },
    pinOpen: function (pin, payload) {
      var parts = String(payload).replace(/\s+/g, '').split('.');
      if (parts.length !== 2) return Promise.reject(new Error('bad payload'));
      return pbkdf2(pin, unb64u(parts[0]), PIN_ITER, 256).then(aesKey).then(function (k) { return unseal(k, parts[1]); });
    },

    // Account: one PBKDF2 run, split into a local vault key and a login secret.
    account: function (username, password) {
      return pbkdf2(password, enc.encode('notehop/account/' + username.toLowerCase()), ACCOUNT_ITER, 512).then(function (b) {
        return { vault: b64u(b.subarray(0, 32)), auth: b64u(b.subarray(32)) };
      });
    },
    vault: function () {
      var v = window.NH && NH.store.get('nh_vault', null);
      var me = document.body.dataset.user;
      if (!v || !v.k || !me || v.u !== me) return null;
      var keyP = aesKey(unb64u(v.k));
      return {
        seal: function (obj) { return keyP.then(function (k) { return seal(k, JSON.stringify(obj)); }); },
        open: function (str) { return keyP.then(function (k) { return unseal(k, str); }).then(JSON.parse); }
      };
    }
  };
})();
