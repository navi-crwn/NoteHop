// Applies the saved theme before the page paints, and writes the family cookies.
// hop_theme (light or dark) is shared on .hel.ink like hop_ui: cookie first, then the old
// localStorage key 'theme', then the system setting. Any other cookie value means the system.
(function () {
  window.hopPref = function (name, value) {
    try {
      document.cookie = name + '=' + encodeURIComponent(value) + '; path=/; max-age=31536000; samesite=lax' +
        (/(^|\.)hel\.ink$/.test(location.hostname) ? '; domain=.hel.ink; secure' : '');
    } catch (e) {}
  };
  try {
    var m = document.cookie.match(/(?:^|;\s*)hop_theme=([^;]*)/), t = m ? m[1] : null, old = null;
    try { old = localStorage.getItem('theme'); } catch (e) {}
    if (!m && (old === 'light' || old === 'dark')) { t = old; window.hopPref('hop_theme', old); }
    if (t === 'light' || t === 'dark') document.documentElement.dataset.theme = t;
  } catch (e) {}
  // The colour palette (classic | mist | sand) is a family setting in the hop_palette cookie, written like hop_theme.
  // NoteHop's own default, with no cookie, is mist; a cookie that exists always wins, "classic" included (it means no
  // attribute), and any other value is classic. An older choice kept in localStorage moves to the cookie once.
  try {
    var pm = document.cookie.match(/(?:^|;\s*)hop_palette=([^;]*)/), pal = pm ? decodeURIComponent(pm[1]) : 'mist', oldPal = null;
    try { oldPal = localStorage.getItem('nh_palette'); } catch (e) {}
    if (!pm && (oldPal === 'mist' || oldPal === 'sand')) { pal = oldPal; window.hopPref('hop_palette', oldPal); }
    try { if (oldPal !== null) localStorage.removeItem('nh_palette'); } catch (e) {}
    if (pal === 'mist' || pal === 'sand') document.documentElement.dataset.palette = pal;
  } catch (e) {}
})();
