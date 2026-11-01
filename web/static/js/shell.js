/* Opens and closes the dashboard sidebar on small screens. */
(function () {
  'use strict';
  var root = document.getElementById('shell'), burger = document.getElementById('shell-burger'), scrim = document.getElementById('shell-scrim');
  if (!root || !burger) return;
  function set(open) {
    root.classList.toggle('open', open);
    burger.setAttribute('aria-expanded', String(open));
    if (scrim) scrim.hidden = !open;
    document.documentElement.style.overflow = open ? 'hidden' : '';
  }
  burger.addEventListener('click', function () { set(!root.classList.contains('open')); });
  if (scrim) scrim.addEventListener('click', function () { set(false); });
  document.addEventListener('keydown', function (e) { if (e.key === 'Escape') set(false); });
  document.getElementById('shell-side').addEventListener('click', function (e) { if (e.target.closest('a')) set(false); });
  matchMedia('(min-width: 1024px)').addEventListener('change', function (m) { if (m.matches) set(false); });
})();
