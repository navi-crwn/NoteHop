// The page inside the sandboxed /run frame (see runFrame in app.go). It takes
// one page from the note page that framed it and becomes that page. Nothing
// here can reach the note page: the frame has an opaque origin.
(function () {
  'use strict';
  window.addEventListener('message', function once(e) {
    if (e.source !== window.parent || !e.data || e.data.nh !== 'run' || typeof e.data.html !== 'string') return;
    window.removeEventListener('message', once);
    document.open();
    document.write(e.data.html);
    document.close();
  });
  window.parent.postMessage({ nh: 'ready' }, '*');
})();
