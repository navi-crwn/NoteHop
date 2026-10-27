// NoteHop shared helpers: theme, nav, toasts, the API client, and the
// "on this device" list. Loaded on every page before page scripts.
(function () {
  'use strict';
  var NH = window.NH = {};

  // Interface text: the catalog for the chosen language comes from
  // /i18n/<lang>.js (window.NH_I18N); English fills any gap server-side.
  // t('key', {name: value}) fills "{name}"; a value named "time" is unix
  // seconds and is shown as a local date.
  var CAT = window.NH_I18N || {};
  NH.lang = window.NH_LANG || 'en';
  NH.t = function (key, vars) {
    var s = Object.prototype.hasOwnProperty.call(CAT, key) ? CAT[key] : key;
    if (vars) Object.keys(vars).forEach(function (k) {
      var v = k === 'time' ? NH.date(vars[k]) : vars[k];
      s = s.split('{' + k + '}').join(v);
    });
    return s;
  };

  NH.$ = function (s, r) { return (r || document).querySelector(s); };
  NH.$$ = function (s, r) { return Array.prototype.slice.call((r || document).querySelectorAll(s)); };

  // h('a.btn.sm', {href: '/'}, 'text', child) builds elements without innerHTML.
  NH.h = function (spec, attrs) {
    var parts = spec.split('.');
    var el = document.createElement(parts[0] || 'div');
    if (parts.length > 1) el.className = parts.slice(1).join(' ');
    var kids = Array.prototype.slice.call(arguments, 2);
    if (attrs && (typeof attrs !== 'object' || attrs.nodeType || Array.isArray(attrs))) { kids.unshift(attrs); attrs = null; }
    kids = [].concat.apply([], kids); // children may be passed as an array
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

  // Tiny event bus between page scripts (e.g. slides → live presenter).
  var handlers = {};
  NH.on = function (name, fn) { (handlers[name] = handlers[name] || []).push(fn); };
  NH.emit = function (name, data) { (handlers[name] || []).forEach(function (fn) { fn(data); }); };

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
    if (body instanceof Uint8Array || body instanceof Blob) {
      init.headers['Content-Type'] = 'application/octet-stream';
      init.body = body;
    } else if (body !== undefined) {
      init.headers['Content-Type'] = 'application/json';
      init.body = JSON.stringify(body);
    }
    return fetch(path, init).then(function (r) {
      return r.text().then(function (txt) {
        var data = null;
        try { data = txt ? JSON.parse(txt) : {}; } catch (e) { data = { error: txt }; }
        if (!r.ok) {
          var msg = data && data.code && Object.prototype.hasOwnProperty.call(CAT, data.code) ? NH.t(data.code, data.vars) : (data && data.error);
          var err = new Error(msg || ('HTTP ' + r.status));
          err.code = data && data.code;
          err.status = r.status;
          throw err;
        }
        return data;
      });
    }, function () {
      var err = new Error(NH.t('err.network'));
      err.status = 0;
      throw err;
    });
  };

  NH.copy = function (text, msg) {
    var done = function () { NH.toast(msg || NH.t('js.copied'), 'ok'); };
    if (navigator.clipboard && window.isSecureContext) {
      return navigator.clipboard.writeText(text).then(done, function () { fallback(); });
    }
    fallback();
    function fallback() {
      var ta = NH.h('textarea.sr', { readonly: true });
      ta.value = text;
      document.body.appendChild(ta);
      ta.select();
      try { document.execCommand('copy'); done(); } catch (e) { NH.toast(NH.t('js.copy_failed'), 'err'); }
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

  // Admin links stay on the main site (sessions, dashboard); links for
  // readers use the share domain when one is configured.
  NH.noteURL = function (token, frag) {
    var q = new URLSearchParams();
    Object.keys(frag || {}).forEach(function (k) { if (frag[k]) q.set(k, frag[k]); });
    var s = q.toString(), d = document.body.dataset;
    var base = (frag && frag.a) ? (d.main || location.origin) : (d.share || d.main || location.origin);
    return base + '/n/' + encodeURIComponent(token) + (s ? '#' + s : '');
  };

  var rtf = window.Intl && Intl.RelativeTimeFormat ? new Intl.RelativeTimeFormat(NH.lang, { numeric: 'auto' }) : null;
  // Offline support (sw.js). Only on secure origins, where browsers allow it.
  if ('serviceWorker' in navigator && window.isSecureContext) {
    window.addEventListener('load', function () { navigator.serviceWorker.register('/sw.js').catch(function () {}); });
  }

  // Server-rendered times: <time data-unix="…"> gets the reader's locale and zone.
  document.addEventListener('DOMContentLoaded', function () {
    NH.$$('time[data-unix]').forEach(function (t) {
      var u = +t.dataset.unix;
      if (!u) return;
      t.textContent = NH.date(u);
      t.dateTime = new Date(u * 1000).toISOString();
    });
  });

  NH.ago = function (unix) {
    var diff = unix - Date.now() / 1000, a = Math.abs(diff);
    var units = [[60, 'second', 1], [3600, 'minute', 60], [86400, 'hour', 3600], [604800, 'day', 86400], [2629800, 'week', 604800], [31557600, 'month', 2629800], [Infinity, 'year', 31557600]];
    for (var i = 0; i < units.length; i++) {
      if (a < units[i][0]) {
        var n = Math.round(diff / units[i][2]);
        return rtf ? rtf.format(n, units[i][1]) : new Date(unix * 1000).toLocaleString(NH.lang);
      }
    }
  };
  NH.date = function (unix) {
    return new Date(unix * 1000).toLocaleString(NH.lang, { dateStyle: 'medium', timeStyle: 'short' });
  };

  NH.modeInfo = {
    edit: { label: NH.t('mode.edit'), icon: 'edit' },
    read: { label: NH.t('mode.read'), icon: 'eye' },
    append: { label: NH.t('mode.append'), icon: 'append' },
    check: { label: NH.t('mode.check'), icon: 'check' },
    suggest: { label: NH.t('mode.suggest'), icon: 'suggest' }
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
  NH.$$('.menu').forEach(function (menu) {
    menu.firstElementChild.addEventListener('click', function (e) {
      e.stopPropagation();
      NH.$$('.menu.open').forEach(function (m) { if (m !== menu) m.classList.remove('open'); });
      menu.classList.toggle('open');
    });
  });
  document.addEventListener('click', function () { NH.$$('.menu.open').forEach(function (m) { m.classList.remove('open'); }); });
  // Language: remember it in a cookie and reload in place, keeping the #key
  // part of note links (a plain ?lang= link would drop it).
  NH.$$('[data-lang]').forEach(function (a) {
    a.addEventListener('click', function (e) {
      e.preventDefault();
      document.cookie = 'nh_lang=' + a.dataset.lang + '; path=/; max-age=31536000; samesite=lax' + (location.protocol === 'https:' ? '; secure' : '');
      location.reload();
    });
  });
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
