// The NoteHop editor: a plain textarea with a live preview, a "/" command
// menu, list continuation, table smart-paste and PIN-locked blocks. A note
// is one of three types: Document (Markdown, all of the above), Plain (text
// as typed) or Code (a language, line numbers, Tab indents; never run).
//
//   var ed = NHEditor(rootEl, {onChange, onSave, onType});
//   ed.value / ed.value = '…' / ed.focus()
//   ed.fields() → {} | {y: 'plain'} | {y: 'code', l: lang}, stored in the note
//   ed.setType({y, l}, true) sets it without asking
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
    { label: NH.t('cmd.time'), hint: '', keys: 'time now clock ' + NH.t('cmd.time_keys'), text: function () { return now() + CARET; } },
    { label: NH.t('cmd.var'), hint: '{{ }}', keys: 'variable template field placeholder ' + NH.t('cmd.var_keys'), text: '{{' + CARET + '}}' },
    { label: NH.t('cmd.calc'), hint: '```calc', keys: 'calc calculator math sum total ' + NH.t('cmd.calc_keys'), text: '```calc\n' + CARET + '\n```\n' },
    { label: NH.t('cmd.diagram'), hint: 'mermaid', keys: 'diagram mermaid chart flow graph ' + NH.t('cmd.diagram_keys'), text: '```mermaid\nflowchart LR\n  A[' + CARET + '] --> B\n```\n' },
    { label: NH.t('cmd.wiki'), hint: '[[ ]]', keys: 'wiki link note page ' + NH.t('cmd.wiki_keys'), text: '[[' + CARET + ']]' },
    { label: NH.t('cmd.slide'), hint: '---', keys: 'slide present page break ' + NH.t('cmd.slide_keys'), text: '\n---\n\n' + CARET },
    { label: NH.t('cmd.burn'), hint: NH.t('cmd.burn_hint'), keys: 'burn destroy self secret once ' + NH.t('cmd.burn_keys'), action: 'burn' },
    { label: NH.t('cmd.poll'), hint: ':::poll', keys: 'poll vote survey choice ' + NH.t('cmd.poll_keys'), text: ':::poll ' + CARET + '\n- \n- \n:::\n' },
    { label: NH.t('cmd.form'), hint: ':::form', keys: 'form survey rsvp signup question ' + NH.t('cmd.form_keys'),
      text: function () { return ':::form ' + CARET + '\n- ' + NH.t('form.tpl_name') + ' *\n- ' + NH.t('form.tpl_email') + ' (email)\n- ' + NH.t('form.tpl_choice') + ' [' + NH.t('form.tpl_yes') + '|' + NH.t('form.tpl_no') + ']\n- ' + NH.t('form.tpl_note') + ' (long)\n:::\n'; } },
    { label: NH.t('cmd.embed'), hint: ':::embed', keys: 'embed include transclude live note ' + NH.t('cmd.embed_keys'), text: ':::embed ' + CARET + '\n:::\n' },
    { label: NH.t('cmd.file'), hint: NH.t('cmd.file_hint'), keys: 'file attach upload image photo audio video pdf ' + NH.t('cmd.file_keys'), action: 'file' },
    { label: NH.t('cmd.stamp'), hint: '[1:23]', keys: 'timestamp media audio video seek ' + NH.t('cmd.stamp_keys'), action: 'stamp' }
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

  // ---------- text utilities (command palette) ----------
  // Each works on the selection, or the whole text when nothing is selected.
  function lines(t) { return t.split('\n'); }
  var collator = new Intl.Collator(NH.lang, { numeric: true, sensitivity: 'base' });
  var UTILS = [
    { id: 'sort_az', run: function (t) { return lines(t).sort(collator.compare).join('\n'); } },
    { id: 'sort_za', run: function (t) { return lines(t).sort(collator.compare).reverse().join('\n'); } },
    { id: 'dedupe', run: function (t) { var seen = {}; return lines(t).filter(function (l) { if (seen[l]) return false; seen[l] = 1; return true; }).join('\n'); } },
    { id: 'trim', run: function (t) { return lines(t).map(function (l) { return l.trim(); }).join('\n'); } },
    { id: 'no_empty', run: function (t) { return lines(t).filter(function (l) { return l.trim(); }).join('\n'); } },
    { id: 'upper', run: function (t) { return t.toLocaleUpperCase(NH.lang); } },
    { id: 'lower', run: function (t) { return t.toLocaleLowerCase(NH.lang); } },
    { id: 'title', run: function (t) { return t.toLocaleLowerCase(NH.lang).replace(/(^|[\s(\-"'])(\p{L})/gu, function (m, a, c) { return a + c.toLocaleUpperCase(NH.lang); }); } },
    { id: 'links', run: function (t) { return (t.match(/https?:\/\/[^\s<>()"']+/g) || []).filter(function (u, i, a) { return a.indexOf(u) === i; }).join('\n'); } },
    { id: 'json', run: function (t) { return JSON.stringify(JSON.parse(t), null, 2); } },
    { id: 'json_min', run: function (t) { return JSON.stringify(JSON.parse(t)); } },
    { id: 'number', run: function (t) { return lines(t).map(function (l, i) { return (i + 1) + '. ' + l; }).join('\n'); } },
    { id: 'count', run: null }
  ];

  // Self-destructing block: sealed with a fresh key; the key goes to the
  // server (opts.onBurnKey) which hands it out only so many times.
  var burnDlg;
  function burnDialog(onInsert) {
    if (!burnDlg) {
      burnDlg = NH.h('dialog.dlg', { 'aria-label': NH.t('cmd.burn') });
      var t = function (k) { return NH.t(k).replace(/[&<>"]/g, function (c) { return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]; }); };
      burnDlg.innerHTML =
        '<div class="dlg-head"><h2>' + t('cmd.burn') + '</h2><button type="button" class="icon-btn" data-close aria-label="' + t('common.close') + '"><svg><use href="#i-x"/></svg></button></div>' +
        '<p class="small muted">' + t('burn.lead') + '</p>' +
        '<form><div class="field"><label for="burn-text">' + t('lock.text') + '</label><textarea id="burn-text" rows="4" required></textarea></div>' +
        '<div class="field"><label for="burn-views">' + t('new.burn_after') + '</label><select id="burn-views"><option value="1">1×</option><option value="3">3×</option><option value="10">10×</option></select></div>' +
        '<div class="btn-row end"><button class="btn primary" type="submit"><svg><use href="#i-flame"/></svg>' + t('lock.insert') + '</button></div></form>';
      document.body.appendChild(burnDlg);
    }
    var form = burnDlg.querySelector('form');
    form.reset();
    form.onsubmit = function (e) {
      e.preventDefault();
      var key = NHC.newKey(), views = +NH.$('#burn-views', burnDlg).value;
      var id = Array.prototype.map.call(crypto.getRandomValues(new Uint8Array(10)), function (b) { return 'abcdefghijkmnpqrstuvwxyz23456789'[b % 32]; }).join('');
      NHC.blockSeal(key, NH.$('#burn-text', burnDlg).value).then(function (payload) {
        burnDlg.close();
        onInsert(':::burn ' + id + ' ' + views + '\n' + payload.match(/.{1,76}/g).join('\n') + '\n:::\n', { id: id, key: key, max_views: views });
      });
    };
    burnDlg.showModal();
    NH.$('#burn-text', burnDlg).focus();
  }

  // Languages offered for code notes (when the bundled highlight.js has them).
  var LANGS = [['bash', 'Bash'], ['c', 'C'], ['cpp', 'C++'], ['csharp', 'C#'], ['css', 'CSS'], ['diff', 'Diff'], ['go', 'Go'], ['xml', 'HTML / XML'],
    ['ini', 'INI / TOML'], ['java', 'Java'], ['javascript', 'JavaScript'], ['json', 'JSON'], ['kotlin', 'Kotlin'], ['lua', 'Lua'], ['markdown', 'Markdown'],
    ['php', 'PHP'], ['python', 'Python'], ['r', 'R'], ['ruby', 'Ruby'], ['rust', 'Rust'], ['scss', 'SCSS'], ['shell', 'Shell'], ['sql', 'SQL'],
    ['swift', 'Swift'], ['typescript', 'TypeScript'], ['yaml', 'YAML']];
  // What Plain and Code can't show: [text]{.names} and :::center blocks.
  var STYLED = /\[((?:\\.|[^[\]\\\n])+)\]\{(?:\s*\.[a-z][a-z-]*)+\s*\}|^:::(?:center|right|justify)\s*$/m;
  function unstyle(src) {
    var lines = src.replace(/\[((?:\\.|[^[\]\\\n])+)\]\{(?:\s*\.[a-z][a-z-]*)+\s*\}/g, '$1').split('\n'), out = [], open = false;
    lines.forEach(function (l) {
      if (/^:::(?:center|right|justify)\s*$/.test(l)) { open = true; return; }
      if (open && /^:::\s*$/.test(l)) { open = false; return; }
      out.push(l);
    });
    return out.join('\n');
  }

  window.NHEditor = function (root, opts) {
    opts = opts || {};
    var ta = NH.$('.ed-src', root), preview = NH.$('.ed-preview', root), count = NH.$('.ed-count', root), body = NH.$('.ed-body', root);
    var gutter = NH.$('.ed-gutter', root), langSel = NH.$('.ed-lang', root), typeMenu = NH.$('.ed-type', root);
    var type = 'doc', escaped = false;
    root.dataset.type = type;
    LANGS.forEach(function (l) { if (window.hljs && hljs.getLanguage(l[0])) langSel.appendChild(NH.h('option', { value: l[0] }, l[1])); });
    var menu = NH.h('ul.slash', { role: 'listbox', 'aria-label': NH.t('kbar.commands') });
    body.appendChild(menu);
    var slash = null; // {start, items, active}
    var renderTimer;

    // ---------- views ----------
    function setView(v) {
      if (v === 'split' && innerWidth < 900) v = 'write';
      if (type !== 'doc') v = 'write'; // Plain and Code are what you type
      root.dataset.view = v;
      NH.$$('.seg [data-view]', root).forEach(function (b) { b.setAttribute('aria-selected', b.dataset.view === v); });
      if (type === 'doc') NH.store.set('nh_view', v);
      if (v !== 'write') renderPreview();
    }
    NH.$$('.seg [data-view]', root).forEach(function (b) { b.addEventListener('click', function () { setView(b.dataset.view); }); });
    setView(NH.store.get('nh_view', innerWidth >= 1100 ? 'split' : 'write'));

    // ---------- note type ----------
    function setType(t, quiet) {
      var y = t && (t.y === 'plain' || t.y === 'code') ? t.y : 'doc';
      if (y !== 'doc' && type === 'doc' && !quiet && STYLED.test(ta.value)) {
        if (!confirm(NH.t('type.strip_confirm', { type: NH.t('type.' + y) }))) return;
        ta.focus();
        ta.select();
        var plain = unstyle(ta.value);
        if (!document.execCommand('insertText', false, plain)) ta.value = plain;
        ta.setSelectionRange(0, 0);
      }
      var view = NH.store.get('nh_view', innerWidth >= 1100 ? 'split' : 'write');
      type = y;
      root.dataset.type = y;
      if (t && t.l != null && y === 'code') langSel.value = langSel.querySelector('option[value="' + t.l + '"]') ? t.l : '';
      ta.spellcheck = y !== 'code';
      if (y === 'code') ta.setAttribute('wrap', 'off'); else ta.removeAttribute('wrap');
      NH.$('.ed-type-ico use', root).setAttribute('href', '#i-' + (y === 'doc' ? 'doc' : y));
      NH.$('.ed-type-name', root).textContent = NH.t('type.' + y);
      NH.$$('[data-type]', typeMenu).forEach(function (b) { b.setAttribute('aria-checked', b.dataset.type === y); });
      setView(y === 'doc' ? view : 'write');
      closeSlash();
      root.dispatchEvent(new CustomEvent('nh-type'));
      refresh();
      if (!quiet && opts.onType) opts.onType();
    }
    NH.$$('[data-type]', typeMenu).forEach(function (b) {
      b.addEventListener('click', function () { setType({ y: b.dataset.type }); ta.focus(); });
    });
    langSel.addEventListener('change', function () { if (opts.onType) opts.onType(); });
    function fields() { return type === 'doc' ? {} : type === 'plain' ? { y: 'plain' } : { y: 'code', l: langSel.value }; }

    function renderPreview() {
      if (root.dataset.view === 'write') return;
      NHMD.render(preview, ta.value, {
        vars: {},
        interactive: true,
        onCheck: function (i, checked) { ta.value = NHMD.toggleTask(ta.value, i, checked); changed(); }
      });
    }

    function refresh() {
      count.textContent = NH.t('ed.chars', { n: ta.value.length.toLocaleString(NH.lang) });
      if (type === 'code') {
        var n = ta.value.split('\n').length;
        if (gutter.dataset.n !== String(n)) {
          var nums = [];
          for (var i = 1; i <= n; i++) nums.push(i);
          gutter.textContent = nums.join('\n');
          gutter.dataset.n = n;
        }
        gutter.scrollTop = ta.scrollTop;
      }
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

    // ---------- attachments ----------
    function attach(files, pasted) {
      if (!window.NHFiles) return;
      var s = ta.selectionStart, e = ta.selectionEnd;
      files = Array.prototype.slice.call(files, 0, 10);
      NH.toast(NH.t('file.uploading', { n: files.length }));
      files.reduce(function (p, f) {
        return p.then(function (out) { return NHFiles.upload(f, pasted).then(function (md) { return out.concat(md); }); });
      }, Promise.resolve([])).then(function (mds) {
        ta.focus();
        ta.setSelectionRange(s, e);
        insertBlock(mds.join('\n') + '\n' + CARET);
        NH.toast(NH.t('file.uploaded'), 'ok');
      }, function (x) { NH.toast(x.message, 'err'); });
    }
    var picker = NH.h('input.hide', { type: 'file', multiple: true });
    root.appendChild(picker);
    picker.addEventListener('change', function () { if (picker.files.length) attach(picker.files, false); picker.value = ''; });
    ta.addEventListener('dragover', function (e) { if (e.dataTransfer && Array.prototype.indexOf.call(e.dataTransfer.types, 'Files') !== -1) e.preventDefault(); });
    ta.addEventListener('drop', function (e) {
      if (!e.dataTransfer || !e.dataTransfer.files.length || type !== 'doc') return;
      e.preventDefault();
      attach(e.dataTransfer.files, false);
    });

    var CMDS = {
      file: function () { picker.click(); },
      stamp: function () {
        var t = window.NHFiles && NHFiles.stamp();
        if (!t) return NH.toast(NH.t('file.no_media'));
        replace(ta.selectionStart, ta.selectionEnd, t + ' ' + CARET);
      },
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
      },
      burn: function () {
        if (!opts.onBurnKey) return NH.toast(NH.t('burn.unavailable'), 'err');
        var s = ta.selectionStart, e = ta.selectionEnd;
        burnDialog(function (block, k) {
          Promise.resolve(opts.onBurnKey(k)).then(function () {
            ta.focus();
            ta.setSelectionRange(s, e);
            insertBlock(block);
          }, function (x) { NH.toast(x.message, 'err'); });
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
      if (type !== 'doc') return closeSlash();
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
      if (type === 'code') return codeKeys(e, mod);
      if (mod && e.key.toLowerCase() === 'b' && type === 'doc') { e.preventDefault(); return CMDS.bold(); }
      if (mod && e.key.toLowerCase() === 's') { e.preventDefault(); return opts.onSave && opts.onSave(); }
      if (e.key === 'Enter' && !e.shiftKey && !mod && ta.selectionStart === ta.selectionEnd && type === 'doc') {
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
    // Code: Tab and Shift+Tab indent the selected lines by two spaces, Enter
    // keeps the indent. Esc first lets Tab leave the box as usual.
    function codeKeys(e, mod) {
      if (mod && e.key.toLowerCase() === 's') { e.preventDefault(); return opts.onSave && opts.onSave(); }
      if (e.key === 'Escape') { escaped = true; return; }
      if (e.key === 'Tab' && !escaped && !mod && !e.altKey) {
        e.preventDefault();
        var v = ta.value, s = ta.selectionStart, en = ta.selectionEnd, ls = lineStart(s);
        if (s === en && !e.shiftKey) return replace(s, en, '  ');
        var le = v.indexOf('\n', en > s ? en - 1 : en); if (le < 0) le = v.length;
        var block = v.slice(ls, le).split('\n').map(function (l) { return e.shiftKey ? l.replace(/^( {1,2}|\t)/, '') : '  ' + l; }).join('\n');
        replace(ls, le, block);
        ta.setSelectionRange(ls, ls + block.length);
        return;
      }
      escaped = false;
      if (e.key === 'Enter' && !e.shiftKey && !mod && ta.selectionStart === ta.selectionEnd) {
        var pos = ta.selectionStart, ind = ta.value.slice(lineStart(pos), pos).match(/^[ \t]*/)[0];
        if (ind) { e.preventDefault(); replace(pos, pos, '\n' + ind + CARET); }
      }
    }
    ta.addEventListener('scroll', function () { if (type === 'code') gutter.scrollTop = ta.scrollTop; });
    // Writing help (ai.js), when the site allows it: a button, and two items in the format menu.
    var aiBtn = NH.$('.ed-ai', root), ai = null;
    function aiOpen(task) {
      if (!ai) ai = NHAI(ta, { mode: aiBtn.dataset.ai, custom: !!aiBtn.dataset.custom, changed: changed });
      ai.open(task);
    }
    if (aiBtn && window.NHAI) aiBtn.addEventListener('click', function () { aiOpen(); });
    var fmt = window.NHFormat ? NHFormat(ta, { type: function () { return type; }, changed: changed, ai: aiBtn && window.NHAI ? aiOpen : null }) : null;

    ta.addEventListener('input', function () { changed(); checkSlash(); });
    ta.addEventListener('click', checkSlash);
    ta.addEventListener('blur', function () { setTimeout(closeSlash, 100); });
    ta.addEventListener('scroll', closeSlash);
    ta.addEventListener('paste', function (e) {
      var cd = e.clipboardData;
      if (!cd) return;
      // Screenshots and copied images arrive as files with no text.
      if (type !== 'doc') return; // Plain and Code take text only, as typed
      if (cd.files && cd.files.length && !cd.getData('text/plain')) {
        e.preventDefault();
        attach(cd.files, true);
        return;
      }
      var table = tsvToTable(cd.getData('text/plain') || '');
      if (table) {
        e.preventDefault();
        insertBlock(table);
        NH.toast(NH.t('ed.table_pasted'), 'ok');
      }
    });

    // ---------- command palette (Ctrl+K) ----------
    function utilRun(u) {
      var s = ta.selectionStart, e = ta.selectionEnd, all = s === e;
      var text = all ? ta.value : ta.value.slice(s, e);
      if (!u.run) {
        var words = (text.match(/[\p{L}\p{N}][\p{L}\p{N}'’-]*/gu) || []).length;
        return NH.toast(NH.t('util.count_result', { words: words, chars: text.length, lines: text ? lines(text).length : 0 }));
      }
      var out;
      try { out = u.run(text); } catch (x) { return NH.toast(NH.t('util.json_error'), 'err'); }
      if (all) replace(0, ta.value.length, out + CARET); else replace(s, e, out + CARET);
      NH.toast(NH.t('util.done', { name: NH.t('util.' + u.id) }), 'ok');
    }
    var palette = null;
    function openPalette() {
      var items = COMMANDS.map(function (c) { return { label: c.label, hint: c.hint, keys: c.keys, run: function () {
        var p = ta.selectionStart;
        if (c.action) return CMDS[c.action]();
        replace(p, ta.selectionEnd, typeof c.text === 'function' ? c.text() : c.text);
      } }; }).concat(UTILS.map(function (u) {
        return { label: NH.t('util.' + u.id), hint: NH.t('util.group'), keys: u.id + ' ' + NH.t('util.' + u.id), run: function () { utilRun(u); } };
      }));
      if (!palette) {
        palette = NH.h('dialog.dlg.palette', { 'aria-label': NH.t('palette.title') });
        palette.append(NH.h('input', { type: 'search', placeholder: NH.t('palette.placeholder'), 'aria-label': NH.t('palette.title') }), NH.h('ul.palette-list', { role: 'listbox' }));
        document.body.appendChild(palette);
      }
      var input = palette.firstChild, list = palette.lastChild, shown = [], active = 0;
      function draw() {
        var q = input.value.trim().toLowerCase();
        shown = items.filter(function (it) { return !q || (it.label + ' ' + it.keys).toLowerCase().indexOf(q) !== -1; });
        active = Math.min(active, Math.max(0, shown.length - 1));
        list.replaceChildren.apply(list, shown.map(function (it, i) {
          var li = NH.h('li', { role: 'option', 'aria-selected': i === active }, NH.h('span', it.label), NH.h('small', it.hint));
          li.addEventListener('mousedown', function (e) { e.preventDefault(); run(it); });
          return li;
        }));
        var act = list.children[active];
        if (act) act.scrollIntoView({ block: 'nearest' });
      }
      function run(it) { palette.close(); ta.focus(); it.run(); }
      input.oninput = function () { active = 0; draw(); };
      input.onkeydown = function (e) {
        if (e.key === 'ArrowDown' || e.key === 'ArrowUp') { e.preventDefault(); active = (active + (e.key === 'ArrowDown' ? 1 : -1) + shown.length) % Math.max(1, shown.length); draw(); }
        if (e.key === 'Enter' && shown[active]) { e.preventDefault(); run(shown[active]); }
      };
      input.value = '';
      draw();
      palette.showModal();
      input.focus();
    }
    ta.addEventListener('keydown', function (e) {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); openPalette(); }
      // Alt+T: timestamp of the media playing (Ctrl+T belongs to the browser).
      if (e.altKey && !e.ctrlKey && !e.metaKey && e.code === 'KeyT') { e.preventDefault(); CMDS.stamp(); }
    });

    // ---------- bubble toolbar on a selection (mouse/trackpad only) ----------
    if (window.matchMedia && matchMedia('(pointer: fine)').matches) {
      var bubble = NH.h('div.bubble', { role: 'toolbar', 'aria-label': NH.t('ed.insert') });
      var bb = function (label, title, fn) {
        var b = NH.h('button', { type: 'button', title: title, 'aria-label': title }, label);
        b.addEventListener('mousedown', function (e) { e.preventDefault(); fn(); hideBubble(); });
        return b;
      };
      bubble.append(
        bb('B', NH.t('ed.bold'), CMDS.bold),
        bb('I', NH.t('ed.italic'), function () { var s = ta.selectionStart, e = ta.selectionEnd; replace(s, e, '_' + ta.value.slice(s, e) + '_' + CARET); }),
        bb('</>', NH.t('ed.code'), CMDS.code),
        bb('🔗', NH.t('ed.link'), function () { var s = ta.selectionStart, e = ta.selectionEnd; replace(s, e, '[' + ta.value.slice(s, e) + '](' + CARET + ')'); }),
        bb('{{ }}', NH.t('cmd.var'), function () { var s = ta.selectionStart, e = ta.selectionEnd; replace(s, e, '{{' + ta.value.slice(s, e) + '}}' + CARET); }),
        bb('⌘K', NH.t('palette.title'), openPalette)
      );
      body.appendChild(bubble);
      var hideBubble = function () { bubble.classList.remove('open'); };
      var showBubble = function () {
        if (type !== 'doc' || ta.selectionStart === ta.selectionEnd || document.activeElement !== ta) return hideBubble();
        var xy = caretXY(ta, ta.selectionStart);
        bubble.style.left = Math.max(0, Math.min(xy.x + ta.offsetLeft, body.clientWidth - 230)) + 'px';
        bubble.style.top = Math.max(0, xy.y + ta.offsetTop - ta.scrollTop - 62) + 'px';
        bubble.classList.add('open');
      };
      ta.addEventListener('mouseup', function () { setTimeout(showBubble, 0); });
      ta.addEventListener('keyup', function (e) { if (e.shiftKey) showBubble(); else hideBubble(); });
      ta.addEventListener('blur', hideBubble);
      ta.addEventListener('scroll', hideBubble);
      ta.addEventListener('input', hideBubble);
    }

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
        kbBtn('', 'attach', NH.t('cmd.file'), CMDS.file),
        kbBtn('Aa', null, NH.t('fmt.sheet'), function () { if (fmt) fmt.sheet(); }),
        kbBtn(NH.t('kbar.done'), null, NH.t('kbar.close'), function () { ta.blur(); })
      );
      kb.lastChild.classList.add('kbar-done');
      // "/", to-do, lock, attach and Aa only make sense in a Document.
      [0, 1, 3, 4, 5].forEach(function (i) { kb.children[i].classList.add('kbar-doc'); });
      kb.dataset.type = type;
      root.addEventListener('nh-type', function () { kb.dataset.type = type; });
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
      palette: function () { openPalette(); },
      get type() { return type; },
      setType: setType,
      fields: fields,
      format: fmt,
      textarea: ta
    };
    refresh();
    return api;
  };
  window.NHEditor.caretXY = caretXY;
})();
