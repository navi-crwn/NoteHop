// The NoteHop editor: a plain textarea with a live preview, a "/" command
// menu, list continuation, table smart-paste and PIN-locked blocks.
//
//   var ed = NHEditor(rootEl, {onChange, onSave});
//   ed.value / ed.value = '…' / ed.focus()
(function () {
  'use strict';

  function today() { return new Date().toLocaleDateString(NH.lang, { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' }); }
  function now() { return new Date().toLocaleTimeString(NH.lang, { hour: '2-digit', minute: '2-digit' }); }

  // CARET in inserted text marks where the caret lands. It is a control
  // character, not "|", so pasted or selected text containing "|" (tables!)
  // comes through intact.
  var CARET = '\u0001';

  // Each command matches its English keywords and the translated ones, so
  // "/table" works in every language.
  function col(n) { return NH.t('cmd.column', { n: n }); }
  var COMMANDS = [
    { label: NH.t('cmd.h1'), hint: '#', keys: 'h1 heading title big ' + NH.t('cmd.h1_keys'), text: '# ' + CARET },
    { label: NH.t('cmd.h2'), hint: '##', keys: 'h2 heading title ' + NH.t('cmd.h2_keys'), text: '## ' + CARET },
    { label: NH.t('cmd.h3'), hint: '###', keys: 'h3 subheading small ' + NH.t('cmd.h3_keys'), text: '### ' + CARET },
    { label: NH.t('ed.todo'), hint: '- [ ]', keys: 'todo task checklist check ' + NH.t('cmd.todo_keys'), text: '- [ ] ' + CARET },
    { label: NH.t('cmd.list'), hint: '-', keys: 'list bullet ul ' + NH.t('cmd.list_keys'), text: '- ' + CARET },
    { label: NH.t('cmd.olist'), hint: '1.', keys: 'numbered ordered ol number ' + NH.t('cmd.olist_keys'), text: '1. ' + CARET },
    { label: NH.t('cmd.quote'), hint: '>', keys: 'quote blockquote ' + NH.t('cmd.quote_keys'), text: '> ' + CARET },
    { label: NH.t('ed.code'), hint: '```', keys: 'code program script ' + NH.t('cmd.code_keys'), text: '```\n' + CARET + '\n```\n' },
    { label: NH.t('ed.table'), hint: NH.t('cmd.table_hint'), keys: 'table grid ' + NH.t('cmd.table_keys'), text: '| ' + col(1) + ' | ' + col(2) + ' | ' + col(3) + ' |\n| --- | --- | --- |\n| ' + CARET + ' | | |\n' },
    { label: NH.t('cmd.divider'), hint: '---', keys: 'divider hr line rule ' + NH.t('cmd.divider_keys'), text: '---\n' + CARET },
    { label: NH.t('ed.spoiler'), hint: NH.t('cmd.spoiler_hint'), keys: 'spoiler toggle hide fold details ' + NH.t('cmd.spoiler_keys'), text: ':::spoiler ' + NH.t('md.spoiler_default') + '\n' + CARET + '\n:::\n' },
    { label: NH.t('ed.locked'), hint: NH.t('cmd.locked_hint'), keys: 'lock pin secret password ' + NH.t('cmd.locked_keys'), action: 'locked' },
    { label: NH.t('cmd.date'), hint: '', keys: 'date today ' + NH.t('cmd.date_keys'), text: function () { return today() + CARET; } },
    { label: NH.t('cmd.time'), hint: '', keys: 'time now clock ' + NH.t('cmd.time_keys'), text: function () { return now() + CARET; } }
  ];

  // Caret pixel position inside a textarea, via a mirrored div.
  var MIRROR_PROPS = ['boxSizing', 'width', 'height', 'overflowX', 'overflowY', 'borderTopWidth', 'borderRightWidth', 'borderBottomWidth', 'borderLeftWidth',
    'paddingTop', 'paddingRight', 'paddingBottom', 'paddingLeft', 'fontStyle', 'fontVariant', 'fontWeight', 'fontStretch', 'fontSize', 'lineHeight',
    'fontFamily', 'textAlign', 'textTransform', 'textIndent', 'letterSpacing', 'wordSpacing', 'tabSize', 'whiteSpace', 'wordWrap'];
  function caretXY(ta, pos) {
    var div = document.createElement('div'), cs = getComputedStyle(ta);
    MIRROR_PROPS.forEach(function (p) { div.style[p] = cs[p]; });
    div.style.position = 'absolute'; div.style.visibility = 'hidden'; div.style.whiteSpace = 'pre-wrap'; div.style.wordWrap = 'break-word';
    div.textContent = ta.value.slice(0, pos);
    var span = document.createElement('span');
    span.textContent = ta.value.slice(pos) || '.';
    div.appendChild(span);
    document.body.appendChild(div);
    var r = { x: span.offsetLeft, y: span.offsetTop + parseFloat(cs.lineHeight || 20) };
    div.remove();
    return r;
  }

  // Tab-separated rows from Excel/Sheets → a Markdown table.
  function tsvToTable(text) {
    var rows = text.replace(/\r/g, '').replace(/\n+$/, '').split('\n');
    if (rows.length < 2 || !rows.every(function (r) { return r.indexOf('\t') !== -1; })) return null;
    var cells = rows.map(function (r) { return r.split('\t').map(function (c) { return c.trim().replace(/\|/g, '\\|'); }); });
    var w = Math.max.apply(null, cells.map(function (r) { return r.length; }));
    var line = function (r) { var a = r.slice(); while (a.length < w) a.push(''); return '| ' + a.join(' | ') + ' |'; };
    var sep = '| ' + new Array(w).fill('---').join(' | ') + ' |';
    return [line(cells[0]), sep].concat(cells.slice(1).map(line)).join('\n') + '\n';
  }

  var lockDlg;
  function lockDialog(onInsert) {
    if (!lockDlg) {
      lockDlg = NH.h('dialog.dlg', { 'aria-labelledby': 'lock-h' });
      var t = function (k) { return NH.t(k).replace(/[&<>"]/g, function (c) { return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]; }); };
      lockDlg.innerHTML =
        '<div class="dlg-head"><h2 id="lock-h">' + t('ed.locked') + '</h2><button type="button" class="icon-btn" data-close aria-label="' + t('common.close') + '"><svg><use href="#i-x"/></svg></button></div>' +
        '<p class="small muted">' + t('lock.lead') + '</p>' +
        '<form id="lock-form"><div class="field"><label for="lock-title">' + t('lock.title') + '</label><input type="text" id="lock-title" value="' + t('lock.secret') + '" maxlength="80"></div>' +
        '<div class="field"><label for="lock-text">' + t('lock.text') + '</label><textarea id="lock-text" rows="4" required></textarea></div>' +
        '<div class="field"><label for="lock-pin">PIN</label><input type="password" id="lock-pin" minlength="4" required autocomplete="new-password"><span class="hint">' + t('lock.pin_hint') + '</span></div>' +
        '<div class="btn-row end"><button class="btn primary" type="submit"><svg><use href="#i-lock"/></svg>' + t('lock.insert') + '</button></div></form>';
      document.body.appendChild(lockDlg);
    }
    var form = lockDlg.querySelector('form');
    form.reset();
    form.onsubmit = function (e) {
      e.preventDefault();
      var title = NH.$('#lock-title', lockDlg).value.replace(/[\r\n]/g, ' ').trim() || NH.t('lock.secret');
      NHC.pinSeal(NH.$('#lock-pin', lockDlg).value, NH.$('#lock-text', lockDlg).value).then(function (payload) {
        lockDlg.close();
        onInsert(':::locked ' + title + '\n' + payload.match(/.{1,76}/g).join('\n') + '\n:::\n');
      });
    };
    lockDlg.showModal();
    NH.$('#lock-text', lockDlg).focus();
  }

  window.NHEditor = function (root, opts) {
    opts = opts || {};
    var ta = NH.$('.ed-src', root), preview = NH.$('.ed-preview', root), count = NH.$('.ed-count', root), body = NH.$('.ed-body', root);
    var menu = NH.h('ul.slash', { role: 'listbox', 'aria-label': NH.t('kbar.commands') });
    body.appendChild(menu);
    var slash = null; // {start, items, active}
    var renderTimer;

    // ---------- views ----------
    function setView(v) {
      if (v === 'split' && innerWidth < 900) v = 'write';
      root.dataset.view = v;
      NH.$$('.seg [data-view]', root).forEach(function (b) { b.setAttribute('aria-selected', b.dataset.view === v); });
      NH.store.set('nh_view', v);
      if (v !== 'write') renderPreview();
    }
    NH.$$('.seg [data-view]', root).forEach(function (b) { b.addEventListener('click', function () { setView(b.dataset.view); }); });
    setView(NH.store.get('nh_view', innerWidth >= 1100 ? 'split' : 'write'));

    function renderPreview() {
      if (root.dataset.view === 'write') return;
      NHMD.render(preview, ta.value, {
        interactive: true,
        onCheck: function (i, checked) { ta.value = NHMD.toggleTask(ta.value, i, checked); changed(); }
      });
    }

    function refresh() {
      count.textContent = NH.t('ed.chars', { n: ta.value.length.toLocaleString(NH.lang) });
      clearTimeout(renderTimer);
      renderTimer = setTimeout(renderPreview, 150);
    }
    function changed() {
      refresh();
      opts.onChange && opts.onChange();
    }

    // ---------- inserting ----------
    // Replaces [from, to) with text, using execCommand so Ctrl+Z still works.
    function replace(from, to, text) {
      var caret = text.indexOf(CARET);
      text = text.replace(CARET, '');
      ta.focus();
      ta.setSelectionRange(from, to);
      if (!document.execCommand('insertText', false, text)) ta.setRangeText(text, from, to, 'end');
      var at = from + (caret >= 0 ? caret : text.length);
      ta.setSelectionRange(at, at);
      changed();
    }
    function lineStart(pos) { return ta.value.lastIndexOf('\n', pos - 1) + 1; }

    function insertBlock(text) {
      var s = ta.selectionStart, ls = lineStart(s);
      var prefix = ta.value.slice(ls, s).trim() ? '\n' : '';
      replace(s, ta.selectionEnd, prefix + text);
    }

    var CMDS = {
      bold: function () {
        var s = ta.selectionStart, e = ta.selectionEnd, sel = ta.value.slice(s, e);
        replace(s, e, sel ? '**' + sel + '**' + CARET : '**' + CARET + '**');
      },
      h2: function () { var ls = lineStart(ta.selectionStart); replace(ls, ls, '## ' + CARET); },
      todo: function () { var ls = lineStart(ta.selectionStart); replace(ls, ls, '- [ ] ' + CARET); },
      code: function () {
        var s = ta.selectionStart, e = ta.selectionEnd, sel = ta.value.slice(s, e);
        if (sel && sel.indexOf('\n') === -1) replace(s, e, '`' + sel + '`' + CARET);
        else insertBlock('```\n' + (sel || CARET) + '\n```\n');
      },
      table: function () { insertBlock(COMMANDS[8].text); },
      spoiler: function () {
        var s = ta.selectionStart, e = ta.selectionEnd, sel = ta.value.slice(s, e);
        insertBlock(':::spoiler ' + NH.t('md.spoiler_default') + '\n' + (sel || CARET) + '\n:::\n');
      },
      locked: function () {
        var s = ta.selectionStart, e = ta.selectionEnd;
        lockDialog(function (block) {
          ta.focus();
          ta.setSelectionRange(s, e);
          insertBlock(block);
        });
      }
    };
    NH.$$('[data-cmd]', root).forEach(function (b) {
      b.addEventListener('mousedown', function (e) { e.preventDefault(); });
      b.addEventListener('click', function () { CMDS[b.dataset.cmd](); });
    });

    // ---------- slash menu ----------
    function closeSlash() { slash = null; menu.classList.remove('open'); }
    function runCommand(cmd) {
      var s = slash.start, e = ta.selectionStart;
      closeSlash();
      if (cmd.action) { replace(s, e, ''); CMDS[cmd.action](); return; }
      replace(s, e, typeof cmd.text === 'function' ? cmd.text() : cmd.text);
    }
    function drawSlash() {
      menu.replaceChildren.apply(menu, slash.items.map(function (c, i) {
        var li = NH.h('li', { role: 'option', 'aria-selected': i === slash.active }, NH.h('span', c.label), NH.h('small', c.hint));
        li.addEventListener('mousedown', function (e) { e.preventDefault(); runCommand(c); });
        return li;
      }));
      var act = menu.children[slash.active];
      if (act) act.scrollIntoView({ block: 'nearest' });
    }
    function checkSlash() {
      var pos = ta.selectionStart, before = ta.value.slice(lineStart(pos), pos);
      var m = before.match(/(^|\s)\/([\p{L}\d-]{0,24})$/u);
      if (!m || ta.selectionStart !== ta.selectionEnd) return closeSlash();
      var q = m[2].toLowerCase();
      var items = COMMANDS.filter(function (c) { return !q || (c.label + ' ' + c.keys).toLowerCase().indexOf(q) !== -1; });
      if (!items.length) return closeSlash();
      var start = pos - m[2].length - 1;
      slash = { start: start, items: items, active: slash && slash.start === start ? Math.min(slash.active, items.length - 1) : 0 };
      var xy = caretXY(ta, start);
      menu.style.left = Math.min(xy.x + ta.offsetLeft, body.clientWidth - 250) + 'px';
      menu.style.top = (xy.y + ta.offsetTop - ta.scrollTop + 4) + 'px';
      menu.classList.add('open');
      drawSlash();
    }

    // ---------- keys ----------
    ta.addEventListener('keydown', function (e) {
      if (slash) {
        if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
          e.preventDefault();
          slash.active = (slash.active + (e.key === 'ArrowDown' ? 1 : -1) + slash.items.length) % slash.items.length;
          return drawSlash();
        }
        if (e.key === 'Enter' || e.key === 'Tab') { e.preventDefault(); return runCommand(slash.items[slash.active]); }
        if (e.key === 'Escape') { e.preventDefault(); return closeSlash(); }
      }
      var mod = e.ctrlKey || e.metaKey;
      if (mod && e.key.toLowerCase() === 'b') { e.preventDefault(); return CMDS.bold(); }
      if (mod && e.key.toLowerCase() === 's') { e.preventDefault(); return opts.onSave && opts.onSave(); }
      if (e.key === 'Enter' && !e.shiftKey && !mod && ta.selectionStart === ta.selectionEnd) {
        // Continue lists and to-dos; an empty item ends the list.
        var pos = ta.selectionStart, ls = lineStart(pos), line = ta.value.slice(ls, pos);
        var m = line.match(/^(\s*)([-*+]|(\d+)[.)])(\s+)(\[[ xX]\]\s+)?(.*)$/);
        if (m) {
          e.preventDefault();
          if (!m[6].trim()) return replace(ls, pos, '');
          var marker = m[3] ? (parseInt(m[3], 10) + 1) + m[2].slice(-1) : m[2];
          replace(pos, pos, '\n' + m[1] + marker + m[4] + (m[5] ? '[ ] ' : '') + CARET);
        }
      }
    });
    ta.addEventListener('input', function () { changed(); checkSlash(); });
    ta.addEventListener('click', checkSlash);
    ta.addEventListener('blur', function () { setTimeout(closeSlash, 100); });
    ta.addEventListener('scroll', closeSlash);
    ta.addEventListener('paste', function (e) {
      var cd = e.clipboardData;
      if (!cd) return;
      var table = tsvToTable(cd.getData('text/plain') || '');
      if (table) {
        e.preventDefault();
        insertBlock(table);
        NH.toast(NH.t('ed.table_pasted'), 'ok');
      }
    });

    // ---------- phone keyboard bar ----------
    // "/" and the other insert buttons are hard to reach on a phone keyboard,
    // so while typing on a touch screen a bar sits right above the keyboard.
    // The keyboard doesn't shrink the layout viewport, hence visualViewport.
    if (window.matchMedia && matchMedia('(pointer: coarse)').matches) {
      var kb = NH.h('div.kbar', { role: 'toolbar', 'aria-label': NH.t('ed.insert') });
      var kbBtn = function (label, icon, title, fn) {
        var b = NH.h('button', { type: 'button', title: title, 'aria-label': title }, icon ? NH.icon(icon) : null, label);
        b.addEventListener('pointerdown', function (e) { e.preventDefault(); }); // keep the keyboard open
        b.addEventListener('click', fn);
        return b;
      };
      kb.append(
        kbBtn('/', null, NH.t('kbar.commands'), function () {
          var s = ta.selectionStart, before = ta.value.slice(lineStart(s), s);
          replace(s, ta.selectionEnd, (before && !/\s$/.test(before) ? ' ' : '') + '/' + CARET);
          checkSlash();
        }),
        kbBtn('', 'check', NH.t('ed.todo'), CMDS.todo),
        kbBtn('', 'paste', NH.t('kbar.paste'), function () {
          if (!navigator.clipboard || !navigator.clipboard.readText) return NH.toast(NH.t('kbar.paste_hold'));
          navigator.clipboard.readText().then(function (t) {
            if (!t) return;
            var table = tsvToTable(t);
            if (table) insertBlock(table); else replace(ta.selectionStart, ta.selectionEnd, t + CARET);
          }, function () { NH.toast(NH.t('kbar.paste_denied')); });
        }),
        kbBtn('', 'lock', NH.t('ed.locked'), CMDS.locked),
        kbBtn(NH.t('kbar.done'), null, NH.t('kbar.close'), function () { ta.blur(); })
      );
      kb.lastChild.classList.add('kbar-done');
      document.body.appendChild(kb);
      var vv = window.visualViewport;
      var place = function () {
        if (!vv) return;
        var hidden = Math.max(0, innerHeight - vv.height - vv.offsetTop);
        kb.style.transform = 'translateY(' + (-hidden) + 'px)';
      };
      if (vv) { vv.addEventListener('resize', place); vv.addEventListener('scroll', place); }
      ta.addEventListener('focus', function () { place(); kb.classList.add('show'); document.body.classList.add('kbar-on'); });
      ta.addEventListener('blur', function () {
        setTimeout(function () { if (document.activeElement !== ta) { kb.classList.remove('show'); document.body.classList.remove('kbar-on'); } }, 150);
      });
    }

    var api = {
      get value() { return ta.value; },
      set value(v) { ta.value = v; refresh(); }, // programmatic: not a user edit
      focus: function () { ta.focus(); },
      textarea: ta
    };
    refresh();
    return api;
  };
})();
