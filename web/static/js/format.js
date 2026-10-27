// Formatting for the editor's text box: the right-click menu (mouse and
// keyboard) and, on touch screens, a bottom sheet opened from the keyboard bar.
// Everything here only edits the text: bold is **…**, colours and sizes are
// [text]{.red .large} (rendered by md.js), alignment is a :::center block.
//
//   var f = NHFormat(textarea, {type: () => 'doc', changed: fn});
//   f.open(x, y) / f.sheet()
(function () {
  'use strict';
  var h = NH.h;

  var COLORS = ['red', 'orange', 'green', 'cyan', 'blue', 'violet', 'pink', 'gray'];
  var MARKS = ['yellow', 'green', 'blue', 'pink', 'violet'];
  var SIZES = ['small', 'large', 'huge'];
  // Names that replace each other inside one [text]{…}.
  var GROUPS = { color: COLORS, mark: MARKS.map(function (m) { return 'hl-' + m; }), size: SIZES, script: ['sup', 'sub'], u: ['u'] };
  var ONE_SPAN = /^\[((?:\\.|[^[\]\\\n])+)\]\{((?:\s*\.[a-z][a-z-]*)+)\s*\}$/;
  var ANY_SPAN = /\[((?:\\.|[^[\]\\\n])+)\]\{(?:\s*\.[a-z][a-z-]*)+\s*\}/g;
  var ALIGN = /^:::(center|right|justify)\s*$/;

  function caseOf(kind, t) {
    if (kind === 'upper') return t.toLocaleUpperCase(NH.lang);
    if (kind === 'lower') return t.toLocaleLowerCase(NH.lang);
    return t.toLocaleLowerCase(NH.lang).replace(/(^|[\s(\-"'])(\p{L})/gu, function (m, a, c) { return a + c.toLocaleUpperCase(NH.lang); });
  }

  window.NHFormat = function (ta, opts) {
    var type = opts.type;

    // ---------- editing ----------
    // Replaces [from, to) and selects [selFrom, selTo) of the result, through
    // execCommand so Ctrl+Z undoes it.
    function put(from, to, text, selFrom, selTo) {
      ta.focus();
      ta.setSelectionRange(from, to);
      if (!document.execCommand('insertText', false, text)) ta.setRangeText(text, from, to, 'end');
      ta.setSelectionRange(from + (selFrom == null ? text.length : selFrom), from + (selTo == null ? text.length : selTo));
      opts.changed();
    }
    // The selection, or the word at the caret when nothing is selected.
    function sel() {
      var s = ta.selectionStart, e = ta.selectionEnd, v = ta.value;
      if (s === e) {
        while (s > 0 && /[\p{L}\p{N}_'-]/u.test(v[s - 1])) s--;
        while (e < v.length && /[\p{L}\p{N}_'-]/u.test(v[e])) e++;
      }
      if (s === e) { NH.toast(NH.t('fmt.select_first')); return null; }
      return { s: s, e: e, text: v.slice(s, e) };
    }

    // **bold**, _italic_, ~~strike~~: a second click takes the marks off.
    function wrap(mark) {
      var r = sel();
      if (!r) return;
      var t = r.text, n = mark.length;
      if (t.length > 2 * n && t.slice(0, n) === mark && t.slice(-n) === mark) return put(r.s, r.e, t.slice(n, -n), 0, t.length - 2 * n);
      if (ta.value.slice(r.s - n, r.s) === mark && ta.value.slice(r.e, r.e + n) === mark) return put(r.s - n, r.e + n, t, 0, t.length);
      put(r.s, r.e, mark + t + mark, 0, t.length + 2 * n);
    }

    // [text]{.names}: picking a colour again takes it off; picking another
    // one of the same kind replaces it. The result stays selected, so the
    // next pick (a highlight after a colour) lands in the same span.
    function span(name, group) {
      var r = sel();
      if (!r) return;
      var m = r.text.match(ONE_SPAN), inner = r.text, names = [];
      if (!m) { // the caret or selection may sit inside a span: take all of it
        var v = ta.value, open = v.lastIndexOf('[', r.s), close = v.indexOf(']{', r.e);
        var whole = open >= 0 && close >= 0 && v.slice(open, v.indexOf('}', close) + 1).match(ONE_SPAN);
        if (whole && open + whole[0].length >= r.e) { r = { s: open, e: open + whole[0].length }; m = whole; }
      }
      if (m) {
        inner = m[1];
        names = m[2].trim().split(/\s+/).map(function (x) { return x.slice(1); });
      }
      var had = names.indexOf(name) !== -1;
      names = names.filter(function (x) { return GROUPS[group].indexOf(x) === -1; });
      if (name && !had) names.push(name);
      if (!names.length) return put(r.s, r.e, inner, 0, inner.length);
      var out = '[' + inner + ']{' + names.map(function (x) { return '.' + x; }).join(' ') + '}';
      put(r.s, r.e, out, 0, out.length);
    }

    function changeCase(kind) {
      var r = sel();
      if (r) put(r.s, r.e, caseOf(kind, r.text), 0, r.text.length);
    }

    // Alignment works on whole lines: the selected ones go in a :::center
    // (…) block. Inside a block (or with it selected) the whole block changes
    // kind, and "left" takes the lines out of it.
    function align(kind) {
      var v = ta.value, L = v.split('\n'), starts = [], p = 0;
      L.forEach(function (l) { starts.push(p); p += l.length + 1; });
      function lineAt(pos) { var i = 0; while (i + 1 < starts.length && starts[i + 1] <= pos) i++; return i; }
      var ss = ta.selectionStart, se = ta.selectionEnd;
      var a = lineAt(ss), b = lineAt(se > ss ? se - 1 : ss), k = -1, m = -1, i;
      if (ALIGN.test(L[a])) k = a;
      else for (i = a - 1; i >= 0; i--) { if (ALIGN.test(L[i])) { k = i; break; } if (/^:::/.test(L[i])) break; }
      if (k >= 0) for (i = Math.max(b, k + 1); i < L.length; i++) { if (/^:::\s*$/.test(L[i])) { m = i; break; } if (/^:::/.test(L[i])) break; }
      var inner, from, to;
      if (k >= 0 && m >= 0) { inner = L.slice(k + 1, m); from = k; to = m; }
      else { if (kind === 'left') return; inner = L.slice(a, b + 1); from = a; to = b; }
      var text = inner.join('\n'), out = kind === 'left' ? text : ':::' + kind + '\n' + text + '\n:::';
      var off = kind === 'left' ? 0 : kind.length + 4;
      put(starts[from], starts[to] + L[to].length, out, off, off + text.length);
    }

    function link(wiki) {
      var r = sel();
      if (!r) return;
      if (wiki) return put(r.s, r.e, '[[' + r.text + ']]', 2, 2 + r.text.length);
      put(r.s, r.e, '[' + r.text + '](https://)', r.text.length + 3, r.text.length + 11);
    }

    function clear() {
      var r = sel();
      if (!r) return;
      var t = r.text.replace(ANY_SPAN, '$1').replace(/\*\*([^*\n]+)\*\*/g, '$1').replace(/~~([^~\n]+)~~/g, '$1').replace(/(^|[^\p{L}\p{N}_])_([^_\n]+)_(?![\p{L}\p{N}_])/gu, '$1$2');
      put(r.s, r.e, t, 0, t.length);
    }

    function clip(kind) {
      ta.focus();
      if (kind === 'all') return ta.select();
      if (kind !== 'paste') { if (!document.execCommand(kind)) NH.toast(NH.t('fmt.paste_keys')); else if (kind === 'cut') opts.changed(); return; }
      if (!navigator.clipboard || !navigator.clipboard.readText) return NH.toast(NH.t('fmt.paste_keys'));
      var s = ta.selectionStart, e = ta.selectionEnd;
      navigator.clipboard.readText().then(function (t) { if (t) put(s, e, t); }, function () { NH.toast(NH.t('fmt.paste_keys')); });
    }

    var ACT = {
      bold: function () { wrap('**'); }, italic: function () { wrap('_'); }, strike: function () { wrap('~~'); },
      u: function () { span('u', 'u'); }, sup: function () { span('sup', 'script'); }, sub: function () { span('sub', 'script'); },
      cut: function () { clip('cut'); }, copy: function () { clip('copy'); }, paste: function () { clip('paste'); }, all: function () { clip('all'); },
      link: function () { link(false); }, wiki: function () { link(true); }, clear: clear
    };

    // ---------- pieces shared by the menu and the sheet ----------
    var FMT = [['bold', 'B', 'ed.bold'], ['italic', 'I', 'ed.italic'], ['u', 'U', 'fmt.underline'], ['strike', 'S', 'fmt.strike'], ['sup', 'x²', 'fmt.sup'], ['sub', 'x₂', 'fmt.sub']];
    function fmtRow(done) {
      return h('div.fmt-row', FMT.map(function (f) {
        return item('button.fmt-b.fb-' + f[0], NH.t(f[2]), f[1], function () { ACT[f[0]](); done(false); }, true);
      }));
    }
    function swatches(label, names, prefix, group, done) {
      var row = h('div.fmt-sw', { role: 'group', 'aria-label': label });
      row.append(item('button.sw.sw-none', NH.t('fmt.none'), '', function () { span('', group); done(false); }, true));
      names.forEach(function (n) {
        row.append(item('button.sw.' + prefix + n, NH.t('fmt.' + n), '', function () { span(group === 'mark' ? 'hl-' + n : n, group); done(false); }, true));
      });
      return h('div', h('p.fmt-h', label), row);
    }
    function choices(list, done) {
      return h('div.fmt-seg', list.map(function (c) { return item('button', c[1], c[1], function () { c[0](); done(false); }); }));
    }
    function sizes(done) {
      return choices([[function () { span('small', 'size'); }, NH.t('fmt.small')], [function () { span('', 'size'); }, NH.t('fmt.normal')],
        [function () { span('large', 'size'); }, NH.t('fmt.large')], [function () { span('huge', 'size'); }, NH.t('fmt.huge')]], done);
    }
    function aligns(done) {
      return choices(['left', 'center', 'right', 'justify'].map(function (k) { return [function () { align(k); }, NH.t('fmt.' + k)]; }), done);
    }
    function cases(done) {
      return choices(['upper', 'lower', 'title'].map(function (k) { return [function () { changeCase(k); }, NH.t('util.' + k)]; }), done);
    }

    // A menu button. mousedown is cancelled so the text box keeps its selection.
    function item(tag, label, text, fn, iconOnly) {
      var b = h(tag, { type: 'button', role: 'menuitem', tabindex: '-1', title: iconOnly ? label : null, 'aria-label': iconOnly ? label : null }, text);
      b.addEventListener('mousedown', function (e) { e.preventDefault(); });
      b.addEventListener('click', fn);
      return b;
    }

    // ---------- right-click menu ----------
    var menu = null;
    function close(refocus) {
      if (!menu) return;
      menu.remove();
      menu = null;
      document.removeEventListener('mousedown', outside, true);
      if (refocus !== false) ta.focus();
    }
    function outside(e) { if (menu && !menu.contains(e.target)) close(false); }

    function line(icon, label, keys, fn) {
      var b = item('button.ctx-it', label, '', function () { fn(); close(); });
      b.append(icon ? NH.icon(icon) : h('span.ctx-ico'), h('span', label), keys ? h('kbd', keys) : '');
      return b;
    }
    // A row with a submenu: opens on hover, click, Enter or the right arrow.
    function sub(icon, label, build) {
      var wrapEl = h('div.ctx-sub'), panel = h('div.ctx-panel', { role: 'menu', 'aria-label': label });
      var b = item('button.ctx-it', label, '', function () { toggle(true); }, false);
      b.setAttribute('aria-haspopup', 'menu');
      b.setAttribute('aria-expanded', 'false');
      b.append(NH.icon(icon), h('span', label), h('span.ctx-arrow', { 'aria-hidden': 'true' }, '›'));
      panel.append(build(function () { close(); }));
      function toggle(on) {
        if (menu) NH.$$('.ctx-sub.open', menu).forEach(function (o) { if (o !== wrapEl) { o.classList.remove('open'); o.firstChild.setAttribute('aria-expanded', 'false'); } });
        wrapEl.classList.toggle('open', on);
        b.setAttribute('aria-expanded', on ? 'true' : 'false');
        if (on) placeSub(wrapEl, panel);
      }
      wrapEl.addEventListener('mouseenter', function () { toggle(true); });
      wrapEl.addEventListener('mouseleave', function () { toggle(false); });
      wrapEl.open = function () { toggle(true); var f = panel.querySelector('button'); if (f) f.focus(); };
      wrapEl.shut = function () { toggle(false); b.focus(); };
      wrapEl.append(b, panel);
      return wrapEl;
    }
    function placeSub(wrapEl, panel) {
      panel.classList.remove('flip');
      var r = panel.getBoundingClientRect();
      if (r.right > innerWidth - 8 || r.left < 8) panel.classList.add('flip');
      r = panel.getBoundingClientRect();
      if (r.bottom > innerHeight - 8) panel.style.top = Math.min(0, innerHeight - 8 - r.bottom) + 'px';
    }

    function open(x, y) {
      close(false);
      var t = type();
      menu = h('div.ctx', { role: 'menu', 'aria-label': NH.t('fmt.menu') });
      if (t === 'doc') menu.append(fmtRow(function () { close(); }));
      menu.append(line('cut', NH.t('fmt.cut'), 'Ctrl X', ACT.cut), line('copy', NH.t('fmt.copy'), 'Ctrl C', ACT.copy),
        line('paste', NH.t('fmt.paste'), 'Ctrl V', ACT.paste), line('', NH.t('fmt.select_all'), 'Ctrl A', ACT.all), h('hr'));
      if (t === 'doc') {
        menu.append(
          sub('palette', NH.t('fmt.color'), function (done) {
            return h('div.ctx-colors', swatches(NH.t('fmt.text_color'), COLORS, 'tc-', 'color', done), swatches(NH.t('fmt.highlight'), MARKS, 'hl-', 'mark', done));
          }),
          sub('size', NH.t('fmt.size'), sizes),
          sub('align', NH.t('fmt.align'), aligns),
          sub('case', NH.t('fmt.case'), cases),
          line('link', NH.t('ed.link'), '', ACT.link), line('note', NH.t('cmd.wiki'), '', ACT.wiki), line('eraser', NH.t('fmt.clear'), '', ACT.clear));
        if (opts.ai) menu.append(h('hr'), line('spark', NH.t('ai.ctx_rewrite'), '', function () { opts.ai('rewrite'); }),
          line('spark', NH.t('ai.ctx_summarize'), '', function () { opts.ai('summarize'); }));
      } else {
        menu.append(sub('case', NH.t('fmt.case'), cases));
      }
      menu.append(h('p.ctx-hint', NH.t('fmt.shift_hint')));
      document.body.appendChild(menu);
      var w = menu.offsetWidth, ht = menu.offsetHeight;
      menu.style.left = Math.max(8, Math.min(x, innerWidth - w - 8)) + 'px';
      menu.style.top = Math.max(8, Math.min(y, innerHeight - ht - 8)) + 'px';
      menu.addEventListener('keydown', keys);
      document.addEventListener('mousedown', outside, true);
      var first = menu.querySelector('.ctx-it');
      if (first) first.focus();
    }

    // Arrows move between rows, right/left open and close a submenu, Esc closes.
    function keys(e) {
      var inPanel = e.target.closest('.ctx-panel');
      var scope = inPanel || menu;
      var items = Array.prototype.filter.call(scope.querySelectorAll('button'), function (b) {
        return inPanel ? true : !b.closest('.ctx-panel');
      });
      var i = items.indexOf(e.target);
      if (e.key === 'Escape') { e.preventDefault(); if (inPanel) inPanel.parentNode.shut(); else close(); }
      else if (e.key === 'ArrowDown' || e.key === 'ArrowUp') { e.preventDefault(); items[(i + (e.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length].focus(); }
      else if (e.key === 'ArrowRight' && !inPanel && e.target.parentNode.classList.contains('ctx-sub')) { e.preventDefault(); e.target.parentNode.open(); }
      else if (e.key === 'ArrowLeft' && inPanel) { e.preventDefault(); inPanel.parentNode.shut(); }
      else if (e.key === 'Tab') { e.preventDefault(); close(); }
    }
    window.addEventListener('blur', function () { close(false); });
    window.addEventListener('resize', function () { close(false); });

    ta.addEventListener('contextmenu', function (e) {
      // Shift keeps the browser's own menu (spelling fixes, and so on); long
      // presses on touch screens keep the phone's selection menu.
      if (e.shiftKey || e.pointerType === 'touch' || e.pointerType === 'pen') return;
      e.preventDefault();
      var x = e.clientX, y = e.clientY;
      if (!x && !y) { // opened from the keyboard: at the caret
        var r = ta.getBoundingClientRect(), xy = NHEditor.caretXY(ta, ta.selectionStart);
        x = r.left + xy.x; y = r.top + xy.y - ta.scrollTop;
      }
      open(x, y);
    });
    ta.addEventListener('scroll', function () { close(false); });

    // ---------- bottom sheet (touch screens) ----------
    var sheetDlg = null;
    function sheet() {
      if (!sheetDlg) {
        sheetDlg = h('dialog.dlg.fmt-sheet', { 'aria-label': NH.t('fmt.sheet') });
        // The sheet stays open for the next pick, except after a link.
        var done = function (closeIt) { if (closeIt === true) sheetDlg.close(); };
        var shut = h('button.icon-btn', { type: 'button', 'aria-label': NH.t('common.close') }, NH.icon('x'));
        shut.addEventListener('click', function () { sheetDlg.close(); });
        sheetDlg.append(
          h('div.dlg-head', h('h2', NH.t('fmt.sheet')), shut),
          fmtRow(done),
          swatches(NH.t('fmt.text_color'), COLORS, 'tc-', 'color', done),
          swatches(NH.t('fmt.highlight'), MARKS, 'hl-', 'mark', done),
          h('p.fmt-h', NH.t('fmt.size')), sizes(done),
          h('p.fmt-h', NH.t('fmt.align')), aligns(done),
          h('p.fmt-h', NH.t('fmt.case')), cases(done),
          h('div.fmt-rows', item('button', NH.t('ed.link'), NH.t('ed.link'), function () { ACT.link(); done(true); }),
            item('button', NH.t('cmd.wiki'), NH.t('cmd.wiki'), function () { ACT.wiki(); done(true); }),
            item('button', NH.t('fmt.clear'), NH.t('fmt.clear'), function () { ACT.clear(); done(false); })));
        sheetDlg.addEventListener('click', function (e) { if (e.target === sheetDlg) sheetDlg.close(); });
        document.body.appendChild(sheetDlg);
      }
      sheetDlg.showModal();
    }

    return { open: open, close: close, sheet: sheet, act: ACT, span: span, align: align };
  };
})();
