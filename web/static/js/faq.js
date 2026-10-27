// FAQ: open the question named in the address (#public) and filter by search.
(function () {
  'use strict';
  var items = NH.$$('.faq details');
  function openHash() {
    var d = location.hash && document.getElementById(location.hash.slice(1));
    if (d && d.tagName === 'DETAILS') { d.open = true; d.scrollIntoView({ block: 'start' }); }
  }
  openHash();
  window.addEventListener('hashchange', openHash);
  var input = NH.$('#faq-search');
  input.addEventListener('input', function () {
    var q = input.value.trim().toLowerCase(), shown = 0;
    items.forEach(function (d) {
      var hit = !q || d.textContent.toLowerCase().indexOf(q) !== -1;
      d.classList.toggle('hide', !hit);
      if (hit) shown++;
      if (q && hit) d.open = true;
    });
    NH.$$('.faq h2').forEach(function (h) {
      var el = h.nextElementSibling, any = false;
      while (el && el.tagName === 'DETAILS') { if (!el.classList.contains('hide')) any = true; el = el.nextElementSibling; }
      h.classList.toggle('hide', !any);
    });
    NH.$('#faq-none').classList.toggle('hide', shown > 0);
  });
})();
