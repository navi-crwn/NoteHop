// NoteHop service worker: the app opens offline, and notes opened before
// can be read offline. What's kept is what the server sends — ciphertext for
// encrypted notes; the key stays in the link (#k=), which never reaches here.
// Self-destructing notes are never kept. Served as /sw.js with the asset
// version filled in, so each release starts a fresh cache.
'use strict';
var VERSION = '__VERSION__', SHELL = 'nh-shell-' + VERSION, DATA = 'nh-data';

self.addEventListener('install', function (e) {
  e.waitUntil(caches.open(SHELL).then(function (c) { return c.addAll(['/offline']); }).then(function () { return self.skipWaiting(); }));
});
self.addEventListener('activate', function (e) {
  e.waitUntil(caches.keys().then(function (keys) {
    return Promise.all(keys.filter(function (k) { return k !== SHELL && k !== DATA; }).map(function (k) { return caches.delete(k); }));
  }).then(function () { return self.clients.claim(); }));
});

// Pages that are never kept: sign-in steps, and the plain-text and mock-API
// addresses, which serve a public note's text (and use up a burn note's view).
var NEVER_KEEP = /^\/(auth|logout|raw|mock|in)(\/|$)/;

function json(obj, status) {
  return new Response(JSON.stringify(obj), { status: status || 200, headers: { 'Content-Type': 'application/json' } });
}

self.addEventListener('fetch', function (e) {
  var req = e.request, url = new URL(req.url);
  if (url.origin !== location.origin) return;
  var open = req.method === 'POST' && url.pathname.match(/^\/api\/n\/([^/]+)\/open$/);
  if (open) return e.respondWith(openNote(req, '/__open/' + open[1]));
  if (req.method !== 'GET') return;
  var meta = url.pathname.match(/^\/api\/n\/([^/]+)$/);
  if (meta) return e.respondWith(networkFirst(req, DATA, '/__meta/' + meta[1]));
  if (url.pathname.indexOf('/api/') === 0 || url.pathname === '/sw.js') return;
  // Versioned files never change: cache first.
  if (url.pathname.indexOf('/static/') === 0 || url.pathname.indexOf('/i18n/') === 0) {
    return e.respondWith(caches.match(req).then(function (hit) {
      return hit || fetch(req).then(function (res) {
        // Only whole responses: cache.put() rejects a 206 (a Range request).
        if (res.status === 200) { var copy = res.clone(); caches.open(SHELL).then(function (c) { c.put(req, copy); }); }
        return res;
      });
    }));
  }
  if (req.mode === 'navigate') {
    e.respondWith(fetch(req).then(function (res) {
      if (res.status === 200 && !NEVER_KEEP.test(url.pathname)) { var copy = res.clone(); caches.open(SHELL).then(function (c) { c.put(req, copy); }); }
      return res;
    }, function () {
      return caches.match(req, { ignoreSearch: true }).then(function (hit) { return hit || caches.match('/offline'); });
    }));
  }
});

function networkFirst(req, cache, key) {
  return fetch(req).then(function (res) {
    if (res.ok) { var copy = res.clone(); caches.open(cache).then(function (c) { c.put(key, copy); }); }
    else if (res.status === 404) caches.open(cache).then(function (c) { c.delete(key); });
    return res;
  }, function () {
    return caches.open(cache).then(function (c) { return c.match(key); }).then(function (hit) { return hit || json({ error: 'offline', code: 'err.network' }, 503); });
  });
}

function openNote(req, key) {
  return fetch(req.clone()).then(function (res) {
    if (res.ok) {
      res.clone().json().then(function (d) {
        return caches.open(DATA).then(function (c) {
          if (d.max_views > 0 || d.burned) return c.delete(key); // never keep self-destructing notes
          d.offline_saved = Math.floor(Date.now() / 1000);
          return c.put(key, json(d));
        });
      }).catch(function () {});
    } else if (res.status === 404) caches.open(DATA).then(function (c) { c.delete(key); });
    return res;
  }, function () {
    return caches.open(DATA).then(function (c) { return c.match(key); }).then(function (hit) {
      if (!hit) return json({ error: 'offline', code: 'err.network' }, 503);
      return hit.json().then(function (d) { d.offline = true; return json(d); });
    });
  });
}
