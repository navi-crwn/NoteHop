// Board view: a note with two or more "## Column" headings, each followed by
// a list, can be shown as a Kanban board. The text stays the source of
// truth; moving a card moves its lines in the Markdown (and saves, and
// travels to live collaborators like any edit).
(function () {
  'use strict';
  var N = window.NHNote, $ = NH.$, h = NH.h;
  if (!N) return;
  var FENCE = /^\s*(```|~~~)/, ITEM = /^([-*+]|\d+[.)])\s+(.*)$/;

  // parse(text) → [{title, line, items: [{from, to, text, done}]}] or null.
  function parse(text) {
    var lines = text.split('\n'), cols = [], col = null, item = null, fence = null, inItem = false;
    for (var i = 0; i < lines.length; i++) {
      var l = lines[i], f = l.match(FENCE);
      if (f && !fence) { fence = f[1]; inItem = !!item && /^\s/.test(l); }
      else if (f && f[1] === fence) fence = null;
      if (fence || f) {
        // Code indented under a card belongs to it; code at column 0 ends it.
        if (inItem) item.to = i + 1; else item = null;
        continue;
      }
      var hd = l.match(/^##\s+(.+?)\s*#*\s*$/);
      if (hd) { col = { title: hd[1], line: i, items: [] }; cols.push(col); item = null; continue; }
      if (/^#\s/.test(l)) { col = null; item = null; continue; }
      if (!col) continue;
      var m = l.match(ITEM);
      if (m) {
        var t = m[2].match(/^\[([ xX])\]\s*(.*)$/);
        item = { from: i, to: i + 1, text: t ? t[2] : m[2], done: !!(t && t[1] !== ' '), task: !!t };
        col.items.push(item);
      } else if (item && (/^\s+\S/.test(l) || (!l.trim() && /^\s+\S/.test(lines[i + 1] || '')))) {
        item.to = i + 1; // indented continuation or sub-list
      } else if (l.trim()) item = null;
    }
    return cols.length >= 2 && cols.some(function (c) { return c.items.length; }) ? cols : null;
  }

  // Moves card src (column ci, index ii) to column cj before index ij.
  function move(text, cols, ci, ii, cj, ij) {
    var lines = text.split('\n'), it = cols[ci].items[ii];
    var chunk = lines.slice(it.from, it.to);
    var target = cols[cj], at;
    if (ij < target.items.length) at = target.items[ij].from;
    else if (target.items.length) at = target.items[target.items.length - 1].to;
    else {
      at = target.line + 1;
      if (lines[at] !== undefined && !lines[at].trim()) at++;
    }
    if (ci === cj && (at === it.from || at === it.to)) return text;
    // Insert first (if after the source), then remove, so indexes stay right.
    if (at > it.from) {
      lines.splice(at, 0, chunk.join('\n'));
      lines.splice(it.from, it.to - it.from);
    } else {
      lines.splice(it.from, it.to - it.from);
      lines.splice(at, 0, chunk.join('\n'));
    }
    return lines.join('\n');
  }

  var board = null, switcher = null, shown = false, lastText = null;

  function text() { var ed = N.editor(); return ed ? ed.value : N.body(); }
  function editable() { return !!N.editor(); }
  function write(t) {
    var ed = N.editor();
    ed.value = t;
    N.edited();
    draw(true);
  }

  function draw(force) {
    var t = text();
    if (!force && t === lastText) return;
    lastText = t;
    var cols = parse(t);
    switcher.classList.toggle('hide', !cols);
    if (!cols) { if (shown) toggle(false); return; }
    if (!shown) return;
    var drag = null, can = editable();
    board.replaceChildren.apply(board, cols.map(function (c, ci) {
      var list = h('ul.cards');
      c.items.forEach(function (it, ii) {
        var body = h('div.card-text', { dir: 'auto' });
        NHMD.render(body, it.text, {});
        var card = h('li.kcard' + (it.done ? '.done' : ''), { draggable: can ? 'true' : null }, it.task ? NH.icon(it.done ? 'tick' : 'check') : null, body);
        if (can) {
          var sel = h('select.kmove', { 'aria-label': NH.t('board.move') }, cols.map(function (o, oi) { return h('option', { value: oi }, o.title); }));
          sel.value = ci;
          sel.addEventListener('change', function () { write(move(text(), cols, ci, ii, +sel.value, cols[+sel.value].items.length)); });
          card.appendChild(sel);
          card.addEventListener('dragstart', function (e) { drag = { ci: ci, ii: ii }; e.dataTransfer.effectAllowed = 'move'; e.dataTransfer.setData('text/plain', it.text); card.classList.add('dragging'); });
          card.addEventListener('dragend', function () { card.classList.remove('dragging'); });
          card.addEventListener('dragover', function (e) { if (drag) { e.preventDefault(); e.stopPropagation(); card.classList.add('over'); } });
          card.addEventListener('dragleave', function () { card.classList.remove('over'); });
          card.addEventListener('drop', function (e) {
            e.preventDefault(); e.stopPropagation();
            if (drag) write(move(text(), cols, drag.ci, drag.ii, ci, ii));
            drag = null;
          });
        }
        list.appendChild(card);
      });
      var col = h('section.kcol', h('header', h('b', { dir: 'auto' }, c.title), h('span.faint', String(c.items.length))), list);
      if (can) {
        col.addEventListener('dragover', function (e) { if (drag) e.preventDefault(); });
        col.addEventListener('drop', function (e) {
          e.preventDefault();
          if (drag) write(move(text(), cols, drag.ci, drag.ii, ci, c.items.length));
          drag = null;
        });
      }
      return col;
    }));
  }

  function toggle(on) {
    shown = on;
    NH.store.set('nh_view_' + N.token, on ? 'board' : '');
    NH.$$('button', switcher).forEach(function (b) { b.setAttribute('aria-selected', String((b.dataset.v === 'board') === on)); });
    board.classList.toggle('hide', !on);
    $('#n-view').classList.toggle('board-off', on);
    $('#n-edit').classList.toggle('board-off', on);
    draw(true);
  }

  N.onReady(function () {
    var wrap = $('#n-view').parentNode;
    switcher = h('div.seg.view-switch.hide', { role: 'tablist', 'aria-label': NH.t('board.view') },
      h('button', { type: 'button', role: 'tab', 'data-v': 'text', 'aria-selected': 'true' }, NH.t('board.text')),
      h('button', { type: 'button', role: 'tab', 'data-v': 'board', 'aria-selected': 'false' }, NH.icon('grid'), NH.t('board.board')));
    board = h('div.board.hide', { id: 'n-board' });
    wrap.insertBefore(switcher, wrap.firstChild);
    wrap.appendChild(board);
    switcher.addEventListener('click', function (e) {
      var b = e.target.closest('button');
      if (b) toggle(b.dataset.v === 'board');
    });
    draw(true);
    if (!switcher.classList.contains('hide') && NH.store.get('nh_view_' + N.token, '') === 'board') toggle(true);
    // Text changes from typing, polling or live collaborators.
    setInterval(function () { if (!document.hidden) draw(false); }, 700);
  });

  window.NHKanban = { parse: parse, move: move };
})();
