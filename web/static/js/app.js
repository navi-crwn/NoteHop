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

  // Keys of private notes (#k=…) or attachments (nh-file:…#…) pasted into a
  // public note become readable by anyone with its address.
  NH.leaksKeys = function (text) { return /[#&]k=[A-Za-z0-9_-]{20,}|nh-file:[A-Za-z0-9]{20}#/.test(text || ''); };

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

  // Questions and name prompts use the HOP family's shared dialogs (hop-ui.js from hel.ink, see
  // base.html) and fall back to the browser's own when that script is not there: offline,
  // blocked, or not deployed yet. Both answer with a Promise.
  //   NH.confirm(text | {title, message}, {danger, subject, ok, anchor}) → true / false
  //   NH.prompt({title, label, value, maxlength})                        → the text, or null
  // A plain text is split after its first sentence: the question is the title, the rest the message.
  function parts(text) {
    var m = /^(.*?(?:[?？。!！]|\.(?=\s)))\s*([\s\S]*)$/.exec(String(text));
    return m && m[2] ? { title: m[1], message: m[2] } : { title: String(text) };
  }
  NH.confirm = function (text, opts) {
    var o = Object.assign(typeof text === 'string' ? parts(text) : text, opts || {});
    if (window.HopUI && HopUI.confirm) return HopUI.confirm(o);
    return Promise.resolve(window.confirm([o.title, o.message].filter(Boolean).join('\n\n')));
  };
  NH.prompt = function (opts) {
    if (window.HopUI && HopUI.prompt) return HopUI.prompt(opts);
    return Promise.resolve(window.prompt(opts.title || opts.label || '', opts.value || ''));
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

  // Notes live at /TOKEN. Owner links stay on the main site; links for
  // readers use the short domain when one is configured (it redirects to
  // the main site, and the browser keeps the #fragment).
  NH.noteURL = function (token, frag) {
    var q = new URLSearchParams();
    Object.keys(frag || {}).forEach(function (k) { if (frag[k]) q.set(k, frag[k]); });
    var s = q.toString(), d = document.body.dataset;
    var base = (frag && frag.a) || !d.share ? (d.main || location.origin) : d.share;
    return base + '/' + encodeURIComponent(token) + (s ? '#' + s : '');
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
  var burger = document.getElementById('burger'), nav = document.getElementById('nav');
  function setNav(open) { nav.classList.toggle('open', open); burger.setAttribute('aria-expanded', String(open)); }
  if (burger && nav) {
    burger.setAttribute('aria-expanded', 'false');
    burger.addEventListener('click', function (e) { e.stopPropagation(); setNav(!nav.classList.contains('open')); });
    document.addEventListener('click', function (e) { if (nav.classList.contains('open') && !e.target.closest('.nav-links')) setNav(false); });
    document.addEventListener('keydown', function (e) {
      if (e.key === 'Escape' && nav.classList.contains('open')) { setNav(false); burger.focus(); }
    });
  }
  // Dropdown menus (language, account, "More" on a note): the button says whether its menu is open,
  // Escape closes it and gives the focus back, and a menu that would run off the screen opens
  // from the other edge of its button instead.
  function closeMenus(except) {
    NH.$$('.menu.open').forEach(function (m) { if (m !== except) { m.classList.remove('open'); m.firstElementChild.setAttribute('aria-expanded', 'false'); } });
  }
  NH.$$('.menu').forEach(function (menu) {
    var btn = menu.firstElementChild;
    btn.setAttribute('aria-expanded', 'false');
    btn.addEventListener('click', function (e) {
      e.stopPropagation();
      closeMenus(menu);
      var open = menu.classList.toggle('open');
      btn.setAttribute('aria-expanded', String(open));
      menu.classList.remove('flip');
      var list = menu.querySelector('.menu-list');
      if (open && list) { var r = list.getBoundingClientRect(); if (r.left < 8 || r.right > innerWidth - 8) menu.classList.add('flip'); }
    });
  });
  document.addEventListener('click', function () { closeMenus(); });
  document.addEventListener('keydown', function (e) {
    if (e.key !== 'Escape') return;
    var open = document.querySelector('.menu.open');
    if (!open) return;
    closeMenus();
    open.firstElementChild.focus();
  });
  // Language: remember it in a cookie and reload in place, keeping the #key
  // part of note links (a plain ?lang= link would drop it).
  NH.$$('[data-lang]').forEach(function (a) {
    a.addEventListener('click', function (e) {
      e.preventDefault();
      document.cookie = 'nh_lang=' + a.dataset.lang + '; path=/; max-age=31536000; samesite=lax' + (location.protocol === 'https:' ? '; secure' : '');
      location.reload();
    });
  });
  // A HOP ID session the account server has not vouched for lately (the page says so with
  // data-stale): one quiet trip through it, on page load only. Same account: the page comes
  // back as it was; another account or none: the session here is replaced or ended.
  // Never from a page where text may be typed and not yet saved, never twice (the way back
  // carries _rc=1), and never from a note link: its #key is not something to send round.
  (function recheck() {
    var u = new URL(location.href);
    if (u.searchParams.has('_rc')) {
      u.searchParams.delete('_rc');
      history.replaceState(null, '', u.pathname + u.search + u.hash);
      return;
    }
    var b = document.body;
    if (b.dataset.stale !== '1') return;
    if (/^(note|new|login|register|recover|vault|error|offline)$/.test(b.dataset.page || '')) return;
    if (location.hash.length > 1) return;
    var typed = NH.$$('textarea, [contenteditable="true"]').some(function (f) {
      return (f.value !== undefined ? f.value : f.textContent).trim() !== '';
    });
    if (typed) return;
    location.replace('/auth/sso?prompt=none&next=' + encodeURIComponent(location.pathname + location.search));
  })();
  var logout = document.getElementById('logout');
  if (logout) logout.addEventListener('click', function () {
    NH.store.del('nh_vault');
    NH.api('POST', '/api/logout', {}).then(function (d) { location.href = (d && d.next) || '/'; }, function () { location.href = '/'; });
  });
  // The vault in the account menu: the key stays in this browser until Sign out
  // or "Lock"; "Unlock" asks for it again (the vault page for a HOP ID account,
  // the sign-in page for a local one) and comes back here.
  var mv = document.getElementById('menu-vault');
  if (mv && document.body.dataset.user) {
    var vaultOpen = !!NH.store.get('nh_vault', null), sso = document.body.dataset.sso === '1';
    mv.querySelector('span').textContent = NH.t(vaultOpen ? 'nav.vault_lock' : 'nav.vault_unlock');
    mv.querySelector('use').setAttribute('href', vaultOpen ? '#i-lock' : '#i-unlock');
    mv.classList.remove('hide');
    mv.addEventListener('click', function () {
      if (vaultOpen) { NH.store.del('nh_vault'); location.reload(); return; }
      location.href = (sso ? '/vault' : '/login') + '?next=' + encodeURIComponent(location.pathname);
    });
  }
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
