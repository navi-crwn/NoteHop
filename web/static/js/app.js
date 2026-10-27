// NoteHop shared helpers: theme, nav, toasts, the API client, and the
// "on this device" list. Loaded on every page before page scripts.
(function () {
  'use strict';
  var NH = window.NH = {};

  NH.$ = function (s, r) { return (r || document).querySelector(s); };
  NH.$$ = function (s, r) { return Array.prototype.slice.call((r || document).querySelectorAll(s)); };

  // h('a.btn.sm', {href: '/'}, 'text', child) builds elements without innerHTML.
  NH.h = function (spec, attrs) {
    var parts = spec.split('.');
    var el = document.createElement(parts[0] || 'div');
    if (parts.length > 1) el.className = parts.slice(1).join(' ');
    var kids = Array.prototype.slice.call(arguments, 2);
    if (attrs && (typeof attrs !== 'object' || attrs.nodeType)) { kids.unshift(attrs); attrs = null; }
    if (attrs) Object.keys(attrs).forEach(function (k) {
      var v = attrs[k];
      if (v == null || v === false) return;
      if (k.slice(0, 2) === 'on') el.addEventListener(k.slice(2), v);
      else if (k === 'text') el.textContent = v;
      else el.setAttribute(k, v === true ? '' : v);
    });
    kids.forEach(function (c) { if (c != null && c !== false) el.append(c.nodeType ? c : String(c)); });
    return el;
  };

  NH.icon = function (name, cls) {
    var s = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    if (cls) s.setAttribute('class', cls);
    var u = document.createElementNS('http://www.w3.org/2000/svg', 'use');
    u.setAttribute('href', '#i-' + name);
    s.appendChild(u);
    return s;
  };

  NH.toast = function (msg, kind) {
    var box = document.getElementById('toasts');
    if (!box) return;
    var t = NH.h('div.toast' + (kind ? '.' + kind : ''), { role: 'status' }, msg);
    box.appendChild(t);
    setTimeout(function () { t.classList.add('out'); }, 2600);
    setTimeout(function () { t.remove(); }, 3000);
  };

  NH.api = function (method, path, body, headers) {
    var init = { method: method, headers: Object.assign({}, headers || {}), credentials: 'same-origin' };
    if (body !== undefined) {
      init.headers['Content-Type'] = 'application/json';
      init.body = JSON.stringify(body);
    }
    return fetch(path, init).then(function (r) {
      return r.text().then(function (txt) {
        var data = null;
        try { data = txt ? JSON.parse(txt) : {}; } catch (e) { data = { error: txt }; }
        if (!r.ok) {
          var err = new Error((data && data.error) || ('HTTP ' + r.status));
          err.status = r.status;
          throw err;
        }
        return data;
      });
    }, function () {
      var err = new Error('Tidak bisa terhubung ke server.');
      err.status = 0;
      throw err;
    });
  };

  NH.copy = function (text, msg) {
    var done = function () { NH.toast(msg || 'Disalin', 'ok'); };
    if (navigator.clipboard && window.isSecureContext) {
      return navigator.clipboard.writeText(text).then(done, function () { fallback(); });
    }
    fallback();
    function fallback() {
      var ta = NH.h('textarea.sr', { readonly: true });
      ta.value = text;
      document.body.appendChild(ta);
      ta.select();
      try { document.execCommand('copy'); done(); } catch (e) { NH.toast('Gagal menyalin', 'err'); }
      ta.remove();
    }
    return Promise.resolve();
  };

  NH.store = {
    get: function (k, def) { try { var v = localStorage.getItem(k); return v == null ? def : JSON.parse(v); } catch (e) { return def; } },
    set: function (k, v) { try { localStorage.setItem(k, JSON.stringify(v)); } catch (e) {} },
    del: function (k) { try { localStorage.removeItem(k); } catch (e) {} }
  };

  // Admin links for notes made in this browser (opt-in at creation).
  NH.recent = {
    all: function () { return NH.store.get('nh_recent', []); },
    add: function (item) {
      var list = NH.recent.all().filter(function (x) { return x.token !== item.token; });
      list.unshift(item);
      NH.store.set('nh_recent', list.slice(0, 50));
    },
    update: function (token, patch) {
      NH.store.set('nh_recent', NH.recent.all().map(function (x) { return x.token === token ? Object.assign(x, patch) : x; }));
    },
    remove: function (token) { NH.store.set('nh_recent', NH.recent.all().filter(function (x) { return x.token !== token; })); },
    clear: function () { NH.store.del('nh_recent'); }
  };

  NH.noteURL = function (token, frag) {
    var q = new URLSearchParams();
    Object.keys(frag || {}).forEach(function (k) { if (frag[k]) q.set(k, frag[k]); });
    var s = q.toString();
    return location.origin + '/n/' + encodeURIComponent(token) + (s ? '#' + s : '');
  };

  var rtf = window.Intl && Intl.RelativeTimeFormat ? new Intl.RelativeTimeFormat('id', { numeric: 'auto' }) : null;
  NH.ago = function (unix) {
    var diff = unix - Date.now() / 1000, a = Math.abs(diff);
    var units = [[60, 'second', 1], [3600, 'minute', 60], [86400, 'hour', 3600], [604800, 'day', 86400], [2629800, 'week', 604800], [31557600, 'month', 2629800], [Infinity, 'year', 31557600]];
    for (var i = 0; i < units.length; i++) {
      if (a < units[i][0]) {
        var n = Math.round(diff / units[i][2]);
        return rtf ? rtf.format(n, units[i][1]) : new Date(unix * 1000).toLocaleString('id');
      }
    }
  };
  NH.date = function (unix) {
    return new Date(unix * 1000).toLocaleString('id', { dateStyle: 'medium', timeStyle: 'short' });
  };

  NH.modeInfo = {
    edit: { label: 'Edit penuh', icon: 'edit' },
    read: { label: 'Baca saja', icon: 'eye' },
    append: { label: 'Tambah saja', icon: 'append' },
    check: { label: 'Centang checklist', icon: 'check' }
  };

  // ---------- chrome ----------
  var themeBtn = document.getElementById('theme');
  if (themeBtn) themeBtn.addEventListener('click', function () {
    var root = document.documentElement;
    var dark = root.dataset.theme ? root.dataset.theme === 'dark' : !matchMedia('(prefers-color-scheme: light)').matches;
    root.dataset.theme = dark ? 'light' : 'dark';
    try { localStorage.setItem('theme', root.dataset.theme); } catch (e) {}
  });
  var burger = document.getElementById('burger');
  if (burger) burger.addEventListener('click', function () { document.getElementById('nav').classList.toggle('open'); });
  var menu = document.getElementById('menu');
  if (menu) {
    menu.firstElementChild.addEventListener('click', function (e) { e.stopPropagation(); menu.classList.toggle('open'); });
    document.addEventListener('click', function () { menu.classList.remove('open'); });
  }
  var logout = document.getElementById('logout');
  if (logout) logout.addEventListener('click', function () {
    NH.store.del('nh_vault');
    NH.api('POST', '/api/logout', {}).finally(function () { location.href = '/'; });
  });
  document.addEventListener('click', function (e) {
    var c = e.target.closest('[data-close]');
    if (c) { var d = c.closest('dialog'); if (d) d.close(); }
  });
  // Click on the backdrop closes a dialog.
  document.addEventListener('mousedown', function (e) {
    if (e.target.tagName === 'DIALOG') {
      var r = e.target.getBoundingClientRect();
      if (e.clientX < r.left || e.clientX > r.right || e.clientY < r.top || e.clientY > r.bottom) e.target.close();
    }
  });
})();
