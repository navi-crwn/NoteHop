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
    newKey: function (bits) { return b64u(rand(bits === 128 ? 16 : 32)); },
    newKey128: function () { return b64u(rand(16)); },
    newSalt: function () { return b64u(rand(16)); },

    // Returns {aes, write} for a note. Supports both legacy 256-bit (32 bytes)
    // and shorter 128-bit (16 bytes) keys. Rejects if k is malformed.
    noteKeys: function (k, password, salt) {
      var key = unb64u(k);
      if (key.length !== 32 && key.length !== 16) return Promise.reject(new Error('bad key'));
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

    // Decoy: the same note key with a second password opens a harmless text.
    // Both texts are padded to similar sizes so their length doesn't tell
    // which is which. Returns {content, write} for the server.
    decoy: function (k, password, salt, fake, real) {
      var keyP = window.NHC.noteKeys(k, password, salt);
      var target = JSON.stringify(real).length + Math.floor(Math.random() * 64);
      var obj = Object.assign({}, fake);
      var gap = target - JSON.stringify(obj).length - 6;
      if (gap > 0) obj._ = new Array(gap + 1).join(' ');
      return keyP.then(function (keys) {
        return seal(keys.aes, JSON.stringify(obj)).then(function (c) { return { content: c, write: keys.write }; });
      });
    },

    // Self-destructing blocks: text sealed with its own random key. The
    // sealed text sits inside the note; the server keeps only the key and
    // hands it out a limited number of times.
    blockSeal: function (k, text) {
      return aesKey(unb64u(k)).then(function (key) { return seal(key, text); });
    },
    blockOpen: function (k, payload) {
      return aesKey(unb64u(k)).then(function (key) { return unseal(key, String(payload).replace(/\s+/g, '')); });
    },

    // Attachments: [4-byte header length][JSON {n: name, t: type}][bytes],
    // sealed with a fresh key that is written only into the note text.
    fileSeal: function (bytes, name, type) {
      var k = rand(32), head = enc.encode(JSON.stringify({ n: name, t: type })), iv = rand(12);
      var plain = new Uint8Array(4 + head.length + bytes.length);
      new DataView(plain.buffer).setUint32(0, head.length);
      plain.set(head, 4);
      plain.set(bytes, 4 + head.length);
      return aesKey(k).then(function (key) {
        return subtle.encrypt({ name: 'AES-GCM', iv: iv }, key, plain);
      }).then(function (ct) { return { key: b64u(k), data: concat(iv, new Uint8Array(ct)) }; });
    },
    fileOpen: function (k, data) {
      var raw = new Uint8Array(data);
      return aesKey(unb64u(k)).then(function (key) {
        return subtle.decrypt({ name: 'AES-GCM', iv: raw.subarray(0, 12) }, key, raw.subarray(12));
      }).then(function (pt) {
        var p = new Uint8Array(pt), n = new DataView(pt).getUint32(0), head = JSON.parse(dec.decode(p.subarray(4, 4 + n)));
        return { name: String(head.n || 'file'), type: String(head.t || 'application/octet-stream'), bytes: p.subarray(4 + n) };
      });
    },

    // Inbox: entries the server sealed to the note's P-256 key (see
    // internal/app/inbox.go). inboxKeys makes the pair; the private JWK is
    // then stored sealed with the note key.
    inboxKeys: function () {
      return subtle.generateKey({ name: 'ECDH', namedCurve: 'P-256' }, true, ['deriveBits']).then(function (kp) {
        return Promise.all([subtle.exportKey('raw', kp.publicKey), subtle.exportKey('jwk', kp.privateKey)]);
      }).then(function (r) { return { pub: b64u(r[0]), jwk: r[1] }; });
    },
    inboxOpen: function (jwk, content) {
      var p = String(content).split('.');
      return Promise.all([
        subtle.importKey('jwk', jwk, { name: 'ECDH', namedCurve: 'P-256' }, false, ['deriveBits']),
        subtle.importKey('raw', unb64u(p[1]), { name: 'ECDH', namedCurve: 'P-256' }, false, [])
      ]).then(function (k) {
        return subtle.deriveBits({ name: 'ECDH', public: k[1] }, k[0], 256);
      }).then(function (shared) {
        return subtle.importKey('raw', shared, 'HKDF', false, ['deriveBits']);
      }).then(function (ikm) {
        return subtle.deriveBits({ name: 'HKDF', hash: 'SHA-256', salt: new Uint8Array(0), info: enc.encode('notehop/inbox') }, ikm, 256);
      }).then(function (key) {
        var raw = unb64u(p[2]);
        return aesKey(new Uint8Array(key)).then(function (k) { return subtle.decrypt({ name: 'AES-GCM', iv: raw.subarray(0, 12) }, k, raw.subarray(12)); });
      }).then(function (pt) { return JSON.parse(dec.decode(pt)); });
    },

    // Account: one PBKDF2 run, split into a local vault key and a login secret.
    account: function (username, password) {
      return pbkdf2(password, enc.encode('notehop/account/' + username.toLowerCase()), ACCOUNT_ITER, 512).then(function (b) {
        return { vault: b64u(b.subarray(0, 32)), auth: b64u(b.subarray(32)) };
      });
    },
    // Single sign-on accounts: NoteHop never sees that password, so the vault
    // key comes from a separate phrase, salted with the account's user id. The
    // server only gets HMAC(vault, …) to tell a right phrase from a typo.
    vaultPhrase: function (phrase, sub) {
      return pbkdf2(phrase, enc.encode('notehop/vault/' + sub), ACCOUNT_ITER, 256).then(function (v) {
        return hmac(v, 'notehop/vault-check').then(function (c) { return { vault: b64u(v), check: b64u(c) }; });
      });
    },
    // The vault key is either the key from the password (or phrase) itself,
    // or, once the password was changed or reset, sealed with it (wrap).
    vaultKey: function (kek, wrap) {
      if (!wrap) return Promise.resolve(kek);
      return aesKey(unb64u(kek)).then(function (k) { return unseal(k, wrap); });
    },
    sealVault: function (kek, vault) { return aesKey(unb64u(kek)).then(function (k) { return seal(k, vault); }); },
    remember: function (username, vault) { NH.store.set('nh_vault', { u: username, k: vault }); },

    // Recovery codes: 16 characters from an alphabet without look-alikes
    // (80 bits). HKDF turns a code into a key that seals the vault key and a
    // proof the server keeps only as SHA-256. The server can't open the seal.
    newRecoveryCode: function () {
      var abc = 'ABCDEFGHJKLMNPQRSTUVWXYZ23456789', r = rand(16), out = '';
      for (var i = 0; i < 16; i++) out += (i && i % 4 === 0 ? '-' : '') + abc[r[i] % 32];
      return out;
    },
    recoveryKeys: function (code, username) {
      var norm = String(code).toUpperCase().replace(/[^A-Z0-9]/g, '');
      return subtle.importKey('raw', enc.encode(norm), 'HKDF', false, ['deriveBits']).then(function (ikm) {
        return subtle.deriveBits({ name: 'HKDF', hash: 'SHA-256', salt: enc.encode('notehop/recovery/' + username.toLowerCase()),
          info: enc.encode('notehop/recovery') }, ikm, 512);
      }).then(function (b) {
        b = new Uint8Array(b);
        return aesKey(b.subarray(0, 32)).then(function (k) {
          return { proof: b64u(b.subarray(32)), seal: function (v) { return seal(k, v); }, open: function (w) { return unseal(k, w); } };
        });
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
