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
      var b = !fence && line.match(/^:::(spoiler|locked)\s*(.*)$/);
      if (!b) { out.push(line); continue; }
      var inner = [], j = i + 1;
      while (j < lines.length && !/^:::\s*$/.test(lines[j])) inner.push(lines[j++]);
      if (j >= lines.length) { out.push(line); continue; } // unclosed: leave as text
      var title = b[2].trim();
      if (b[1] === 'spoiler') {
        out.push('', '<details class="spoiler"><summary>' + esc(title || 'Klik untuk membuka') + '</summary>', '', inner.join('\n'), '', '</details>', '');
      } else {
        out.push('', '<div class="locked" data-title="' + esc(title || 'Blok terkunci') + '" data-payload="' + esc(inner.join('').trim()) + '"></div>', '');
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
      var btn = NH.h('button.code-copy', { type: 'button', title: 'Salin kode', 'aria-label': 'Salin kode' }, NH.icon('copy'));
      btn.addEventListener('click', function () { NH.copy(code.textContent, 'Kode disalin'); });
      code.parentNode.appendChild(btn);
    });
  }

  // External images are blocked by CSP (they'd leak readers' IPs); show what they were.
  function images(root) {
    root.querySelectorAll('img').forEach(function (img) {
      var src = img.getAttribute('src') || '';
      if (/^data:image\//.test(src)) return;
      var a = NH.h('a.img-blocked', { href: src, target: '_blank', rel: 'noopener noreferrer nofollow' }, NH.icon('eye'), 'Gambar luar: ' + (img.alt || src));
      img.replaceWith(a);
    });
  }

  function lockedBlocks(root) {
    root.querySelectorAll('.locked').forEach(function (box) {
      var title = box.dataset.title, payload = box.dataset.payload;
      var pin = NH.h('input', { type: 'password', inputmode: 'numeric', placeholder: 'PIN', 'aria-label': 'PIN untuk ' + title, autocomplete: 'off' });
      var err = NH.h('span.locked-err');
      var form = NH.h('form.locked-form', pin, NH.h('button.btn.sm', { type: 'submit' }, NH.icon('unlock'), 'Buka'), err);
      box.replaceChildren(NH.h('div.locked-head', NH.icon('lock'), NH.h('b', title)), form);
      form.addEventListener('submit', function (e) {
        e.preventDefault();
        if (!pin.value) return;
        err.textContent = 'Membuka…';
        NHC.pinOpen(pin.value, payload).then(function (text) {
          var body = NH.h('div.locked-body');
          body.innerHTML = html(text);
          body.querySelectorAll('input[type=checkbox]').forEach(function (c) { c.disabled = true; });
          highlight(body);
          images(body);
          box.replaceChildren(NH.h('div.locked-head.open', NH.icon('unlock'), NH.h('b', title)), body);
        }, function () { err.textContent = 'PIN salah.'; pin.select(); });
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
      var btn = NH.h('button.blk-copy', { type: 'button', title: 'Salin bagian ini', 'aria-label': 'Salin bagian ini' }, NH.icon('copy'));
      btn.addEventListener('click', function (e) {
        e.stopPropagation();
        NH.copy(blockText(b), 'Disalin');
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

  // render(el, src, {checks, interactive, onCheck, copy})
  function render(el, src, opts) {
    opts = opts || {};
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
    if (opts.copy) copyButtons(el);
    if (!el.textContent.trim() && !el.querySelector('.locked, input, hr')) el.appendChild(NH.h('p.faint', 'Belum ada isi.'));
  }

  window.NHMD = { render: render, html: html, applyChecks: applyChecks, toggleTask: toggleTask };
})();
