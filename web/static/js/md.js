// Markdown rendering for NoteHop: marked + DOMPurify + highlight.js, plus
// two block extensions:
//   :::spoiler Title      →  click-to-open <details>
//   ...
//   :::
//   :::locked Title       →  PIN-encrypted block (payload made by NHC.pinSeal)
//   <payload>
//   :::
// Checkboxes are numbered in document order outside fenced code; the server's
// applyChecks uses the same rule.
(function () {
  'use strict';
  marked.setOptions({ gfm: true, breaks: true });

  var TASK = /^(\s*(?:[-*+]|\d+[.)])\s+)\[([ xX])\]/;
  var FENCE = /^\s*(```|~~~)/;

  function esc(s) {
    return String(s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }

  // Walks lines outside fenced code, calling fn(line, index) for task lines.
  function eachTask(src, fn) {
    var lines = src.split('\n'), fence = null, n = 0;
    for (var i = 0; i < lines.length; i++) {
      var f = lines[i].match(FENCE);
      if (f) {
        if (!fence) fence = f[1]; else if (f[1] === fence) fence = null;
        continue;
      }
      if (fence) continue;
      var m = lines[i].match(TASK);
      if (m) { var r = fn(lines[i], n, m); if (r != null) lines[i] = r; n++; }
    }
    return lines.join('\n');
  }

  function setTask(line, m, checked) {
    return m[1] + '[' + (checked ? 'x' : ' ') + ']' + line.slice(m[0].length);
  }

  function applyChecks(src, checks) {
    if (!checks || !Object.keys(checks).length) return src;
    return eachTask(src, function (line, i, m) {
      return Object.prototype.hasOwnProperty.call(checks, i) ? setTask(line, m, checks[i]) : null;
    });
  }

  function toggleTask(src, index, checked) {
    return eachTask(src, function (line, i, m) { return i === index ? setTask(line, m, checked) : null; });
  }

  // mapText applies fn to the prose of src: not fenced code, not `inline code`.
  function mapText(src, fn) {
    var fence = null;
    return src.split('\n').map(function (line) {
      var f = line.match(FENCE);
      if (f) { if (!fence) fence = f[1]; else if (f[1] === fence) fence = null; return line; }
      if (fence) return line;
      return line.split(/(`[^`]*`)/).map(function (part, i) { return i % 2 ? part : fn(part); }).join('');
    }).join('\n');
  }

  // Template variables: {{name}} in the text becomes a field readers fill in.
  var VAR = /\{\{\s*([\p{L}\p{N}_][\p{L}\p{N}_ .-]{0,39}?)\s*\}\}/gu;
  function templateVars(src) {
    var seen = [];
    mapText(src || '', function (t) {
      t.replace(VAR, function (m, n) { if (seen.indexOf(n) === -1) seen.push(n); return m; });
      return t;
    });
    return seen;
  }
  // Filled values replace their {{name}}; empty ones stay visible as a chip
  // (mark) or as {{name}} (for copying).
  function fillVars(src, vals, mark) {
    return mapText(src, function (t) {
      return t.replace(VAR, function (m, n) {
        if (vals && vals[n]) return vals[n];
        return mark ? '<span class="tpl-var">' + esc(n) + '</span>' : m;
      });
    });
  }

  // Wiki links: [[Title]] points at another of the reader's notes, found in
  // their own (vault-decrypted) list; unknown titles are shown as such.
  var WIKI = /\[\[([^\[\]\n]{1,120})\]\]/g;
  function wikiLinks(src, resolve) {
    return mapText(src, function (t) {
      return t.replace(WIKI, function (m, title) {
        var url = resolve(title.trim());
        return url ? '[' + title.replace(/[\[\]]/g, '') + '](' + url + ')' : '<span class="wiki-missing" title="' + esc(NH.t('md.wiki_missing')) + '">' + esc(title) + '</span>';
      });
    });
  }

  // Turns ::: blocks into HTML before marked sees them.
  function preprocess(src) {
    var lines = src.split('\n'), out = [], fence = null;
    for (var i = 0; i < lines.length; i++) {
      var line = lines[i], f = line.match(FENCE);
      if (f) {
        if (!fence) fence = f[1]; else if (f[1] === fence) fence = null;
        out.push(line);
        continue;
      }
      var b = !fence && line.match(/^:::(spoiler|locked|poll|burn|embed|form)\s*(.*)$/);
      if (!b) { out.push(!fence && window.NHFiles ? NHFiles.preprocess(line) : line); continue; }
      var inner = [], j = i + 1;
      while (j < lines.length && !/^:::\s*$/.test(lines[j])) inner.push(lines[j++]);
      if (j >= lines.length) { out.push(line); continue; } // unclosed: leave as text
      var title = b[2].trim();
      if (b[1] === 'spoiler') {
        out.push('', '<details class="spoiler"><summary>' + esc(title || NH.t('md.spoiler_default')) + '</summary>', '', inner.join('\n'), '', '</details>', '');
      } else if (b[1] === 'poll') {
        // :::poll Question, then one "- option" per line
        var opts = inner.map(function (l) { return l.replace(/^\s*(?:[-*+]|\d+[.)])\s+/, '').trim(); }).filter(Boolean).slice(0, 20);
        out.push('', '<div class="poll" data-q="' + esc(title) + '" data-opts="' + esc(JSON.stringify(opts)) + '"></div>', '');
      } else if (b[1] === 'embed') {
        // :::embed, then a note link (with #k=...) on its own line
        var link = (title || inner.join(' ')).trim().split(/\s+/)[0] || '';
        out.push('', '<div class="embed" data-src="' + esc(link) + '"></div>', '');
      } else if (b[1] === 'form') {
        // :::form Title, then one "- Field" per line; "(long)", "(email)",
        // "(number)", "(date)" or "[A|B|C]" after the label pick the input
        var fields = inner.map(function (l) { return l.replace(/^\s*(?:[-*+]|\d+[.)])\s+/, '').trim(); }).filter(Boolean).slice(0, 30);
        out.push('', '<div class="nhform" data-title="' + esc(title) + '" data-fields="' + esc(JSON.stringify(fields)) + '"></div>', '');
      } else if (b[1] === 'burn') {
        // :::burn <id> <views>, then the sealed text (see NHC.blockSeal)
        var parts = title.split(/\s+/);
        out.push('', '<div class="burnblk" data-id="' + esc(parts[0] || '') + '" data-views="' + esc(parts[1] || '1') + '" data-payload="' + esc(inner.join('').trim()) + '"></div>', '');
      } else {
        out.push('', '<div class="locked" data-title="' + esc(title || NH.t('md.locked_default')) + '" data-payload="' + esc(inner.join('').trim()) + '"></div>', '');
      }
      i = j;
    }
    return out.join('\n');
  }

  DOMPurify.addHook('uponSanitizeElement', function (node, data) {
    if (data.tagName === 'input' && node.getAttribute('type') !== 'checkbox') node.remove();
  });
  DOMPurify.addHook('afterSanitizeAttributes', function (node) {
    if (node.tagName === 'A' && node.getAttribute('href')) {
      node.setAttribute('target', '_blank');
      node.setAttribute('rel', 'noopener noreferrer nofollow');
    }
  });
  var PURIFY = { FORBID_TAGS: ['style', 'form', 'button', 'textarea', 'select', 'iframe'], FORBID_ATTR: ['style'] };

  function html(src) {
    return DOMPurify.sanitize(marked.parse(preprocess(src || '')), PURIFY);
  }

  function highlight(root) {
    root.querySelectorAll('pre code').forEach(function (code) {
      var m = (code.className || '').match(/language-([\w+-]+)/);
      if (m && m[1] === 'calc') return calcBlock(code);
      if (m && m[1] === 'mermaid') return mermaidBlock(code);
      try {
        if (m && hljs.getLanguage(m[1])) hljs.highlightElement(code);
        else if (code.textContent.length < 20000) {
          var r = hljs.highlightAuto(code.textContent);
          code.innerHTML = r.value;
          code.classList.add('hljs');
          if (r.language) code.parentNode.dataset.lang = r.language;
        }
      } catch (e) {}
      if (m) code.parentNode.dataset.lang = m[1];
      var btn = NH.h('button.code-copy', { type: 'button', title: NH.t('md.copy_code'), 'aria-label': NH.t('md.copy_code') }, NH.icon('copy'));
      btn.addEventListener('click', function () { NH.copy(code.textContent, NH.t('md.code_copied')); });
      code.parentNode.appendChild(btn);
    });
  }

  // External images are blocked by CSP (they'd leak readers' IPs); show what they were.
  function images(root) {
    root.querySelectorAll('img').forEach(function (img) {
      var src = img.getAttribute('src') || '';
      if (/^data:image\//.test(src)) return;
      var a = NH.h('a.img-blocked', { href: src, target: '_blank', rel: 'noopener noreferrer nofollow' }, NH.icon('eye'), NH.t('md.external_image', { name: img.alt || src }));
      img.replaceWith(a);
    });
  }

  function lockedBlocks(root) {
    root.querySelectorAll('.locked').forEach(function (box) {
      var title = box.dataset.title, payload = box.dataset.payload;
      var pin = NH.h('input', { type: 'password', inputmode: 'numeric', placeholder: 'PIN', 'aria-label': NH.t('md.pin_for', { title: title }), autocomplete: 'off' });
      var err = NH.h('span.locked-err');
      var form = NH.h('form.locked-form', pin, NH.h('button.btn.sm', { type: 'submit' }, NH.icon('unlock'), NH.t('vault.open_btn')), err);
      box.replaceChildren(NH.h('div.locked-head', NH.icon('lock'), NH.h('b', title)), form);
      form.addEventListener('submit', function (e) {
        e.preventDefault();
        if (!pin.value) return;
        err.textContent = NH.t('md.opening');
        NHC.pinOpen(pin.value, payload).then(function (text) {
          var body = NH.h('div.locked-body');
          body.innerHTML = html(text);
          body.querySelectorAll('input[type=checkbox]').forEach(function (c) { c.disabled = true; });
          highlight(body);
          images(body);
          box.replaceChildren(NH.h('div.locked-head.open', NH.icon('unlock'), NH.h('b', title)), body);
        }, function () { err.textContent = NH.t('md.bad_pin'); pin.select(); });
      });
    });
  }

  // Text of one block for its copy button: a list item without its sublists,
  // a table as tab-separated rows (pastes into spreadsheets).
  function blockText(el) {
    if (el.tagName === 'TABLE') {
      return Array.prototype.map.call(el.rows, function (r) {
        return Array.prototype.map.call(r.cells, function (c) { return c.textContent.trim(); }).join('\t');
      }).join('\n');
    }
    var c = el.cloneNode(true);
    c.querySelectorAll('ul, ol, .blk-copy').forEach(function (x) { x.remove(); });
    return c.textContent.replace(/[ \t]+\n/g, '\n').trim();
  }

  // Read mode: a copy button on each paragraph, list item, quote and table.
  // Shown on hover, or on tap where there is no hover.
  function copyButtons(el) {
    var blocks = el.querySelectorAll(':scope > p, :scope > blockquote, :scope > table, li, details.spoiler > p');
    Array.prototype.forEach.call(blocks, function (b) {
      if (b.closest('.locked') || !b.textContent.trim()) return;
      var btn = NH.h('button.blk-copy', { type: 'button', title: NH.t('md.copy_block'), 'aria-label': NH.t('md.copy_block') }, NH.icon('copy'));
      btn.addEventListener('click', function (e) {
        e.stopPropagation();
        NH.copy(blockText(b), NH.t('js.copied'));
      });
      b.classList.add('cp');
      b.appendChild(btn);
    });
    if (el.dataset.cpTap) return;
    el.dataset.cpTap = '1';
    el.addEventListener('click', function (e) {
      if (e.target.closest('a, input, button, summary')) return;
      var b = e.target.closest('.cp');
      el.querySelectorAll('.cp-on').forEach(function (x) { if (x !== b) x.classList.remove('cp-on'); });
      if (b) b.classList.toggle('cp-on');
    });
  }

  // ---------- ```calc: a small calculator ----------
  // Each line is "label = expression", "label: expression" or just an
  // expression; labels become variables for later lines, "total"/"sum" adds
  // up the results above. Numbers may use the reader's grouping (5.000.000
  // or 5,000,000). A hand-written parser: the CSP forbids eval anyway.
  var NUMFMT = (function () {
    var parts = new Intl.NumberFormat(NH.lang).formatToParts(12345.6);
    var g = parts.filter(function (p) { return p.type === 'group'; })[0], d = parts.filter(function (p) { return p.type === 'decimal'; })[0];
    return { group: g ? g.value : ',', decimal: d ? d.value : '.' };
  })();
  function parseNum(s) {
    var hasDot = s.indexOf('.') !== -1, hasComma = s.indexOf(',') !== -1;
    if (hasDot && hasComma) {
      var dec = s.lastIndexOf('.') > s.lastIndexOf(',') ? '.' : ',';
      s = s.split(dec === '.' ? ',' : '.').join('').replace(dec, '.');
    } else if (hasDot || hasComma) {
      var sep = hasDot ? '.' : ',';
      var grouped = new RegExp('^\\d{1,3}(\\' + sep + '\\d{3})+$').test(s);
      // "1.500" is ambiguous: follow the reader's locale.
      if (grouped && (sep !== '.' || NUMFMT.group === '.' || s.split('.').length > 2)) s = s.split(sep).join('');
      else s = s.replace(',', '.');
    }
    return parseFloat(s);
  }
  function calcLine(text, vars) {
    var toks = [], re = /\s*(\d[\d.,]*\d|\d|[\p{L}_][\p{L}\p{N}_]*|[-+*/×÷x^%()])/uy, m, pos = 0;
    text = text.trim();
    while (pos < text.length) {
      re.lastIndex = pos;
      m = re.exec(text);
      if (!m) throw new Error('?');
      toks.push(m[1]); pos = re.lastIndex;
      while (pos < text.length && /\s/.test(text[pos])) pos++;
    }
    var i = 0;
    function peek() { return toks[i]; }
    function expr() { var v = term(); while (peek() === '+' || peek() === '-') { var op = toks[i++], r = term(); v = op === '+' ? v + r : v - r; } return v; }
    function term() { var v = factor(); while (/^[*/×÷x]$/.test(peek() || '')) { var op = toks[i++], r = factor(); v = (op === '/' || op === '÷') ? v / r : v * r; } return v; }
    function factor() { var v = unary(); if (peek() === '^') { i++; v = Math.pow(v, factor()); } return v; }
    function unary() { if (peek() === '-') { i++; return -unary(); } if (peek() === '+') { i++; return unary(); } return primary(); }
    function primary() {
      var t = toks[i++];
      if (t === undefined) throw new Error('?');
      if (t === '(') { var v = expr(); if (toks[i++] !== ')') throw new Error(')'); return v; }
      if (/^\d/.test(t)) { var n = parseNum(t); if (peek() === '%') { i++; n /= 100; } return n; }
      var k = t.toLowerCase();
      if (Object.prototype.hasOwnProperty.call(vars, k)) return vars[k];
      throw new Error(t);
    }
    var v = expr();
    if (i !== toks.length) throw new Error(toks[i]);
    return v;
  }
  function calcBlock(code) {
    var vars = {}, results = [], fmt = new Intl.NumberFormat(NH.lang, { maximumFractionDigits: 10 });
    var rows = code.textContent.replace(/\n$/, '').split('\n').map(function (line) {
      if (!line.trim() || /^\s*(#|\/\/)/.test(line)) return NH.h('tr.calc-note', NH.h('td', { colspan: 2 }, line.replace(/^\s*(#|\/\/)\s?/, '')));
      var m = line.match(/^\s*([^=:]{1,40}?)\s*[=:]\s*(.+)$/), label = m ? m[1].trim() : '', ex = m ? m[2] : line;
      var key = label.toLowerCase().replace(/\s+/g, '_');
      var scope = Object.assign({}, vars, { total: results.reduce(function (a, b) { return a + b; }, 0) });
      scope.sum = scope.total;
      var out;
      try {
        var v = /^\s*(total|sum)\s*$/i.test(ex) && !label ? scope.total : calcLine(ex, scope);
        if (!isFinite(v)) throw new Error('∞');
        if (key) vars[key] = v;
        results.push(v);
        out = NH.h('td.calc-res', fmt.format(v));
      } catch (e) {
        out = NH.h('td.calc-res.calc-err', { title: NH.t('md.calc_error', { token: e.message }) }, '?');
      }
      return NH.h('tr', NH.h('td.calc-ex', line.trim()), out);
    });
    var table = NH.h('table.calc', NH.h('tbody'));
    table.firstChild.append.apply(table.firstChild, rows);
    code.parentNode.replaceWith(table);
  }

  // ---------- ```mermaid diagrams ----------
  // Rendered in a sandboxed frame (see mermaid-frame.js), loaded only when a
  // note has a diagram. The source goes there by postMessage, never to the
  // server; the SVG comes back and is shown as an image, so it stays inert.
  var frame = null, frameReady = null, jobs = {}, jobN = 0;
  function mermaidFrame() {
    if (!frameReady) frameReady = new Promise(function (ok) {
      frame = NH.h('iframe.mermaid-frame', { src: '/render/mermaid', sandbox: 'allow-scripts', title: 'Mermaid', 'aria-hidden': 'true', tabindex: '-1' });
      window.addEventListener('message', function (e) {
        if (!frame || e.source !== frame.contentWindow) return;
        var d = e.data || {};
        if (d.type === 'nh-mermaid-ready') return ok();
        if (d.type === 'nh-mermaid' && jobs[d.id]) { jobs[d.id](d); delete jobs[d.id]; }
      });
      document.body.appendChild(frame);
    });
    return frameReady;
  }
  function mermaidBlock(code) {
    var src = code.textContent, box = NH.h('div.mermaid-box', NH.h('p.faint', NH.t('common.loading')));
    code.parentNode.replaceWith(box);
    var dark = (document.documentElement.dataset.theme || (matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark')) === 'dark';
    mermaidFrame().then(function () {
      var id = ++jobN;
      jobs[id] = function (d) {
        if (d.svg) box.replaceChildren(NH.h('img', { src: 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(d.svg), alt: NH.t('md.diagram') }));
        else box.replaceChildren(NH.h('p.calc-err', NH.t('md.diagram_error')), NH.h('pre', NH.h('code', src)));
      };
      frame.contentWindow.postMessage({ type: 'nh-mermaid', id: id, src: src, dark: dark }, '*');
    });
  }

  // ---------- polls ----------
  // Counts come from the server (only numbers; the question and options are
  // in the encrypted text). A browser remembers its own vote locally.
  function polls(root, opts) {
    root.querySelectorAll('.poll').forEach(function (box, pi) {
      var q = box.dataset.q, options = [];
      try { options = JSON.parse(box.dataset.opts); } catch (e) {}
      var counts = (opts.votes && opts.votes[pi]) || {}, total = 0;
      options.forEach(function (o, i) { total += counts[i] || 0; });
      var mine = opts.myVote ? opts.myVote(pi) : -1;
      var list = NH.h('div.poll-opts');
      options.forEach(function (o, i) {
        var n = counts[i] || 0, pct = total ? Math.round(n * 100 / total) : 0;
        var btn = NH.h('button.poll-opt' + (mine === i ? '.mine' : ''), { type: 'button', disabled: !opts.onVote },
          NH.h('span.poll-bar'), NH.h('span.poll-label', o), NH.h('b.poll-n', n + ' · ' + pct + '%'));
        btn.firstChild.style.width = pct + '%';
        if (opts.onVote) btn.addEventListener('click', function () { opts.onVote(pi, mine === i ? -1 : i, mine); });
        list.appendChild(btn);
      });
      box.replaceChildren(NH.h('div.poll-q', NH.icon('check'), NH.h('b', q || NH.t('md.poll_default'))), list,
        NH.h('small.faint', NH.t('md.poll_total', { n: total })));
    });
  }

  // ---------- self-destructing blocks ----------
  function burnBlocks(root, opts) {
    root.querySelectorAll('.burnblk').forEach(function (box) {
      var id = box.dataset.id, info = opts.burnBlocks && opts.burnBlocks[id];
      var head = NH.h('div.locked-head', NH.icon('flame'), NH.h('b', NH.t('md.burn_title')));
      if (!info) { box.replaceChildren(head, NH.h('p.faint', NH.t('md.burn_gone'))); return; }
      var left = info.max_views - info.views;
      var btn = NH.h('button.btn.sm', { type: 'button', disabled: !opts.onReveal }, NH.icon('eye'), NH.t('md.burn_reveal'));
      var note = NH.h('span.locked-err', NH.t('md.burn_left', { n: left }));
      btn.addEventListener('click', function () {
        if (!confirm(NH.t(left <= 1 ? 'md.burn_confirm_last' : 'md.burn_confirm', { n: left }))) return;
        btn.disabled = true;
        opts.onReveal(id).then(function (r) {
          return NHC.blockOpen(r.key, box.dataset.payload).then(function (text) {
            var body = NH.h('div.locked-body');
            body.innerHTML = html(text);
            box.replaceChildren(NH.h('div.locked-head.open', NH.icon('flame'), NH.h('b', NH.t('md.burn_title'))), body,
              NH.h('p.hint.warn', r.left > 0 ? NH.t('md.burn_left_after', { n: r.left }) : NH.t('md.burn_now_gone')));
          });
        }).catch(function (x) { box.replaceChildren(head, NH.h('p.faint', x.message || NH.t('md.burn_gone'))); });
      });
      box.replaceChildren(head, NH.h('div.locked-form', btn, note));
    });
  }

  // render(el, src, {checks, interactive, onCheck, copy, vars, wiki})
  //   vars: values for {{name}} template fields; wiki: title → note URL
  function render(el, src, opts) {
    opts = opts || {};
    src = src || '';
    if (opts.vars) src = fillVars(src, opts.vars, true);
    if (opts.wiki) src = wikiLinks(src, opts.wiki);
    el.innerHTML = html(src);
    var boxes = Array.prototype.filter.call(el.querySelectorAll('input[type=checkbox]'), function (c) { return !c.closest('.locked'); });
    boxes.forEach(function (c, i) {
      var li = c.closest('li');
      if (li) li.classList.add('task');
      if (opts.checks && Object.prototype.hasOwnProperty.call(opts.checks, i)) c.checked = !!opts.checks[i];
      c.disabled = !opts.interactive;
      if (li) li.classList.toggle('done', c.checked);
      if (opts.interactive) c.addEventListener('change', function () {
        if (li) li.classList.toggle('done', c.checked);
        opts.onCheck && opts.onCheck(i, c.checked, c);
      });
    });
    highlight(el);
    images(el);
    lockedBlocks(el);
    polls(el, opts);
    burnBlocks(el, opts);
    if (window.NHFiles) NHFiles.render(el);
    if (window.NHBlocks) NHBlocks.render(el, opts);
    if (opts.copy) copyButtons(el);
    if (!el.textContent.trim() && !el.querySelector('.locked, input, hr, .nhfile, .attach, .embed, .nhform')) el.appendChild(NH.h('p.faint', NH.t('md.empty')));
  }

  window.NHMD = { render: render, html: html, applyChecks: applyChecks, toggleTask: toggleTask,
    templateVars: templateVars, fillVars: fillVars, calcLine: calcLine, parseNum: parseNum };
})();
