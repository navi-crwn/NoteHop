/* Opens and closes the dashboard sidebar on small screens. Escape, the dimmed page behind it and a link
   inside it all close it; the focus goes into the drawer when it opens and back to the button when it
   closes, and while it is closed and off the screen its links are out of the tab order. */
(function () {
  'use strict';
  var root = document.getElementById('shell'), burger = document.getElementById('shell-burger'), scrim = document.getElementById('shell-scrim');
  var side = document.getElementById('shell-side');
  if (!root || !burger || !side) return;
  var small = matchMedia('(max-width: 1023px)');
  function isOpen() { return root.classList.contains('open'); }
  function sync() { side.inert = small.matches && !isOpen(); }
  function set(open, refocus) {
    var was = isOpen();
    root.classList.toggle('open', open);
    burger.setAttribute('aria-expanded', String(open));
    if (scrim) scrim.hidden = !open;
    document.documentElement.style.overflow = open ? 'hidden' : '';
    sync();
    if (open && !was) { var first = side.querySelector('a'); if (first) first.focus(); }
    if (!open && was && refocus) burger.focus();
  }
  burger.addEventListener('click', function () { set(!isOpen(), true); });
  if (scrim) scrim.addEventListener('click', function () { set(false, true); });
  document.addEventListener('keydown', function (e) { if (e.key === 'Escape' && isOpen()) { e.preventDefault(); set(false, true); } });
  side.addEventListener('click', function (e) { if (e.target.closest('a')) set(false, false); });
  small.addEventListener('change', function () { set(false, false); });
  sync();
})();
