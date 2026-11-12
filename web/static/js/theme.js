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
})();
