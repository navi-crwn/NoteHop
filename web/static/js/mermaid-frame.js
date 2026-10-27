// Runs inside the sandboxed /render/mermaid frame: Mermaid needs inline
// styles to lay out text, which the main page's CSP rightly forbids. The
// frame has no access to the note page; it only turns source into SVG.
(function () {
  'use strict';
  var ready = {};
  window.addEventListener('message', function (e) {
    var d = e.data || {};
    if (d.type !== 'nh-mermaid' || e.source !== window.parent) return;
    var theme = d.dark ? 'dark' : 'default';
    if (!ready[theme]) {
      window.mermaid.initialize({ startOnLoad: false, securityLevel: 'strict', theme: theme });
      ready = {}; ready[theme] = true;
    }
    window.mermaid.render('m' + d.id, String(d.src)).then(function (r) {
      e.source.postMessage({ type: 'nh-mermaid', id: d.id, svg: r.svg }, '*');
    }, function (err) {
      e.source.postMessage({ type: 'nh-mermaid', id: d.id, error: String(err && err.message || err) }, '*');
    });
  });
  window.parent.postMessage({ type: 'nh-mermaid-ready' }, '*');
})();
