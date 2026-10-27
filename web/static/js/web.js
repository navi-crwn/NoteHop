// Web notes (type "web"): HTML, CSS and JS files kept in the note text as one
// bundle ("--- file: name ---" before each file), so encryption, live editing,
// history, /raw and the .txt download work as for any note. The result runs
// only in this browser, in the sandboxed /run frame (see runFrame in app.go):
// an opaque origin with no network, no storage, no access to this page.
//
//   NHWeb.editor(editorRoot, textarea, {changed, refresh}) → {load, value, show, hide, run}
//   NHWeb.view(el, bundle)  read-only files + a result pane that asks first
(function () {
  'use strict';
  var h = NH.h;
  var MARK = /^--- file: (.+?) ---$/;
  var NAME = /^[A-Za-z0-9_][A-Za-z0-9_.-]{0,39}\.[A-Za-z0-9]{1,5}$/;
  var KINDS = { html: 'html', htm: 'html', css: 'css', js: 'js', mjs: 'js', json: 'json', txt: 'txt', md: 'md' };
  var HL = { html: 'xml', css: 'css', js: 'javascript', json: 'json', md: 'markdown' };
  var MAX_FILES = 20;

  function kind(n) { var m = String(n).match(/\.([a-z0-9]+)$/i); return (m && KINDS[m[1].toLowerCase()]) || 'txt'; }
  function isBundle(s) { return MARK.test(String(s || '').split('\n', 1)[0]); }
  function parse(s) {
    var files = [], cur = null;
    String(s || '').split('\n').forEach(function (line) {
      var m = line.match(MARK);
      if (m) { cur = { n: m[1].trim(), c: [] }; files.push(cur); } else if (cur) cur.c.push(line);
    });
    return files.map(function (f) { return { n: f.n, c: f.c.join('\n') }; });
  }
  function join(files) { return files.map(function (f) { return '--- file: ' + f.n + ' ---\n' + f.c; }).join('\n'); }

  var TPL = {
    web: [
      { n: 'index.html', c: '<!doctype html>\n<html>\n<head>\n  <meta charset="utf-8">\n  <link rel="stylesheet" href="style.css">\n</head>\n<body>\n  <h1>Hello!</h1>\n  <button id="btn">Click me</button>\n  <p id="out"></p>\n  <script src="app.js"></script>\n</body>\n</html>' },
      { n: 'style.css', c: 'body {\n  font-family: system-ui, sans-serif;\n  margin: 2rem;\n}\n\nh1 {\n  color: #0f766e;\n}\n\nbutton {\n  padding: .6rem 1rem;\n  border: 0;\n  border-radius: 8px;\n  background: #0f766e;\n  color: #fff;\n}' },
      { n: 'app.js', c: "let clicks = 0;\n\ndocument.querySelector('#btn').addEventListener('click', () => {\n  clicks++;\n  document.querySelector('#out').textContent = 'Clicked ' + clicks + ' times';\n  console.log('click', clicks);\n});" }
    ],
    blank: [{ n: 'index.html', c: '<!doctype html>\n<html>\n<head>\n  <meta charset="utf-8">\n</head>\n<body>\n\n</body>\n</html>' }],
    canvas: [
      { n: 'index.html', c: '<!doctype html>\n<html>\n<head>\n  <meta charset="utf-8">\n  <style>body { margin: 0; background: #111; } canvas { display: block; margin: auto; }</style>\n</head>\n<body>\n  <canvas id="c" width="480" height="320"></canvas>\n  <script src="app.js"></script>\n</body>\n</html>' },
      { n: 'app.js', c: "const ctx = document.querySelector('#c').getContext('2d');\nlet x = 40, y = 40, dx = 3, dy = 2;\n\nfunction frame() {\n  ctx.fillStyle = '#111';\n  ctx.fillRect(0, 0, 480, 320);\n  ctx.fillStyle = '#a3e635';\n  ctx.beginPath();\n  ctx.arc(x, y, 14, 0, Math.PI * 2);\n  ctx.fill();\n  if (x < 14 || x > 466) dx = -dx;\n  if (y < 14 || y > 306) dy = -dy;\n  x += dx; y += dy;\n  requestAnimationFrame(frame);\n}\nframe();" }
    ]
  };
  function copyTpl(name) { return TPL[name].map(function (f) { return { n: f.n, c: f.c }; }); }
  // Text from another note type becomes a project: a bundle as it is, HTML
  // as index.html, anything else as app.js next to a page that loads it.
  function toProject(text) {
    if (isBundle(text)) return parse(text);
    if (!String(text || '').trim()) return copyTpl('web');
    if (/<[a-z!][^>]*>/i.test(text)) return [{ n: 'index.html', c: text }];
    var page = copyTpl('blank')[0];
    page.c = page.c.replace('<body>\n\n</body>', '<body>\n  <script src="app.js"></script>\n</body>');
    return [page, { n: 'app.js', c: text }];
  }

  // ---------- the page that runs ----------
  // Reports console output and errors to the note page. The file name and line
  // come from //# sourceURL on each inlined script.
  var HOOK = '<script>(function(){var P=parent;function f(v){try{if(typeof v==="string")return v;if(v instanceof Error)return v.name+": "+v.message;' +
    'var s=JSON.stringify(v);return s===undefined?String(v):s.slice(0,4000)}catch(e){return String(v)}}' +
    'function at(){var m=String(new Error().stack||"").split("\\n").slice(3).join("\\n").match(/([\\w.-]+\\.(?:m?js|html?)):(\\d+)/);return m?m[1]+":"+m[2]:""}' +
    'function send(l,a,w){try{P.postMessage({nh:"log",level:l,args:Array.prototype.map.call(a,f),at:w||at()},"*")}catch(e){}}' +
    '["log","info","warn","error","debug"].forEach(function(l){var o=console[l];console[l]=function(){send(l==="debug"?"log":l,arguments);if(o)o.apply(console,arguments)}});' +
    'addEventListener("error",function(e){send("error",[e.message],e.filename?String(e.filename).split("/").pop()+":"+e.lineno:"")});' +
    'addEventListener("unhandledrejection",function(e){send("error",["Uncaught (in promise) "+f(e.reason)],"")})})();<\/script>';

  function esc(s) { return String(s).replace(/[&<>"]/g, function (c) { return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]; }); }
  function page(files) {
    var by = {}, used = {};
    files.forEach(function (f) { by[f.n] = f; });
    var main = by['index.html'] || files.filter(function (f) { return kind(f.n) === 'html'; })[0];
    var html = main ? main.c : '<!doctype html><html><head><meta charset="utf-8"></head><body></body></html>';
    var style = function (f) { used[f.n] = 1; return '<style data-file="' + esc(f.n) + '">\n' + f.c.replace(/<\/style/gi, '<\\/style') + '\n</style>'; };
    var script = function (f, mod) {
      used[f.n] = 1;
      return '<script' + (mod ? ' type="module"' : '') + ' data-file="' + esc(f.n) + '">' + f.c.replace(/<\/script/gi, '<\\/script') + '\n//# sourceURL=' + f.n + '\n<\/script>';
    };
    // <link href="style.css"> and <script src="app.js"> of this note's files are inlined.
    html = html.replace(/<link\b[^>]*?\bhref\s*=\s*["']?(?:\.\/)?([^"'\s>]+)["']?[^>]*>/gi, function (m, n) {
      return by[n] && kind(n) === 'css' ? style(by[n]) : m;
    }).replace(/<script\b([^>]*?)\bsrc\s*=\s*["']?(?:\.\/)?([^"'\s>]+)["']?([^>]*)>\s*<\/script>/gi, function (m, a, n, b) {
      return by[n] && kind(n) === 'js' ? script(by[n], /type\s*=\s*["']?module/i.test(a + b)) : m;
    });
    // Files the page doesn't mention are added anyway: CSS in <head>, JS at the end.
    var css = files.filter(function (f) { return kind(f.n) === 'css' && !used[f.n]; }).map(style).join('\n');
    var js = files.filter(function (f) { return kind(f.n) === 'js' && !used[f.n]; }).map(function (f) { return script(f, false); }).join('\n');
    var head = HOOK + css;
    if (/<head[^>]*>/i.test(html)) html = html.replace(/<head[^>]*>/i, function (m) { return m + head; });
    else if (/<html[^>]*>/i.test(html)) html = html.replace(/<html[^>]*>/i, function (m) { return m + '<head>' + head + '</head>'; });
    else html = head + html;
    if (/<\/body>/i.test(html)) html = html.replace(/<\/body>(?![\s\S]*<\/body>)/i, js + '</body>');
    else html += js;
    return html;
  }

  // ---------- result pane ----------
  var DEVICES = [['phone', 'web.phone'], ['tablet', 'web.tablet'], ['monitor', 'web.desktop']];
  function Runner(opts) {
    var frame = null, frameWin = null, pending = null, count = 0, device = NH.store.get('nh_run_device', 'monitor');
    var el = h('div.wr-run'), stage = h('div.wr-stage'), wrap = h('div.wr-wrap'), list = h('ol.wr-log'), cnt = h('span.wr-cnt', '0');
    var devs = h('span.wr-devices', { role: 'radiogroup', 'aria-label': NH.t('web.width') }, DEVICES.map(function (d) {
      var b = h('button', { type: 'button', role: 'radio', 'aria-checked': d[0] === device ? 'true' : 'false', title: NH.t(d[1]), 'aria-label': NH.t(d[1]) }, NH.icon(d[0]));
      b.addEventListener('click', function () {
        device = d[0];
        NH.store.set('nh_run_device', device);
        NH.$$('button', devs).forEach(function (x) { x.setAttribute('aria-checked', x === b); });
        wrap.dataset.device = device;
      });
      return b;
    }));
    var again = h('button.icon-btn.sm', { type: 'button', title: NH.t('web.rerun'), 'aria-label': NH.t('web.rerun') }, NH.icon('history'));
    again.addEventListener('click', function () { if (opts.rerun) opts.rerun(); });
    var full = h('button.icon-btn.sm', { type: 'button', title: NH.t('web.full'), 'aria-label': NH.t('web.full') }, NH.icon('expand'));
    full.addEventListener('click', function () { if (opts.full) opts.full(); });
    var clear = h('button.link-btn', { type: 'button' }, NH.t('web.clear'));
    clear.addEventListener('click', function () { list.replaceChildren(); count = 0; cnt.textContent = '0'; });
    wrap.dataset.device = device;
    stage.appendChild(wrap);
    el.append(
      h('div.wr-bar', h('span.wr-addr', NH.icon('shield'), NH.t('web.result')),
        h('span.wr-net', { title: NH.t('web.no_net_hint') }, h('i'), NH.t('web.no_net')), devs, again, full),
      stage,
      h('div.wr-cons', h('div.wr-cons-head', NH.icon('code'), h('b', NH.t('web.console')), cnt, h('span.sp'), clear), list));
    function idle(node) { wrap.replaceChildren(node || h('p.wr-idle', NH.t('web.idle'))); }
    idle();

    function log(level, text, at) {
      if (count >= 500) list.firstChild && list.firstChild.remove();
      count++;
      cnt.textContent = String(count);
      list.appendChild(h('li.' + level, h('span', text), at ? h('span.wr-at', at) : ''));
      list.scrollTop = list.scrollHeight;
    }
    window.addEventListener('message', function (e) {
      if (!frameWin || e.source !== frameWin || !e.data) return;
      if (e.data.nh === 'ready' && pending != null) { frameWin.postMessage({ nh: 'run', html: pending }, '*'); pending = null; }
      else if (e.data.nh === 'log') log(['warn', 'error', 'info'].indexOf(e.data.level) !== -1 ? e.data.level : 'log',
        (e.data.args || []).map(String).join(' ').slice(0, 4000), String(e.data.at || '').slice(0, 60));
    });
    // A fresh frame every run: nothing carries over from the last one.
    function run(files) {
      pending = page(files);
      frame = h('iframe.wr-frame', { src: '/run?' + Date.now().toString(36), sandbox: 'allow-scripts allow-forms allow-modals',
        referrerpolicy: 'no-referrer', title: NH.t('web.result') });
      wrap.replaceChildren(frame);
      frameWin = frame.contentWindow;
      list.replaceChildren();
      count = 0;
      cnt.textContent = '0';
    }
    return { el: el, run: run, idle: idle, frame: function () { return frame; } };
  }

  // ---------- file list ----------
  function lines(c) { return c ? c.split('\n').length : 0; }
  function fileIcon(n) { var k = kind(n); return h('span.wr-ico.' + k, k === 'html' ? '<>' : k === 'js' ? 'JS' : k === 'css' ? 'CSS' : k.toUpperCase().slice(0, 3)); }
  function nameProblem(n, files, except) {
    if (!NAME.test(n)) return NH.t('web.bad_name');
    if (files.some(function (f, i) { return i !== except && f.n.toLowerCase() === n.toLowerCase(); })) return NH.t('web.taken', { name: n });
    return '';
  }
  function Explorer(o) {
    var el = h('aside.wr-files', { 'aria-label': NH.t('web.files') });
    function draw(editing) {
      var files = o.files(), active = o.active();
      var rows = files.map(function (f, i) {
        if (editing === i) return nameInput(f.n, function (n) { var p = nameProblem(n, o.files(), i); if (p) return p; o.rename(i, n); });
        var row = h('div.wr-file' + (i === active ? '.on' : ''));
        var open = h('button.wr-open', { type: 'button', 'aria-current': i === active ? 'true' : null }, fileIcon(f.n), h('span.wr-name', f.n), h('span.wr-lines', NH.t('web.lines', { n: lines(f.c) })));
        open.addEventListener('click', function () { o.open(i); });
        row.appendChild(open);
        if (o.rename) {
          var ren = h('button.icon-btn.sm', { type: 'button', title: NH.t('web.rename'), 'aria-label': NH.t('web.rename') + ' ' + f.n }, NH.icon('edit'));
          ren.addEventListener('click', function () { draw(i); });
          var del = h('button.icon-btn.sm', { type: 'button', title: NH.t('web.delete'), 'aria-label': NH.t('web.delete') + ' ' + f.n }, NH.icon('trash'));
          del.addEventListener('click', function () {
            if (o.files().length < 2) return NH.toast(NH.t('web.last_file'), 'err');
            if (confirm(NH.t('web.delete_confirm', { name: f.n }))) o.remove(i);
          });
          row.append(ren, del);
        }
        return row;
      });
      var kids = [h('p.wr-h', NH.t('web.files'))].concat(rows);
      if (o.add) {
        if (editing === -1) kids.push(nameInput(suggest(files), function (n) { var p = nameProblem(n, o.files(), -1); if (p) return p; o.add(n); }));
        else if (files.length < MAX_FILES) {
          var add = h('button.wr-new', { type: 'button' }, NH.icon('plus'), NH.t('web.new_file'));
          add.addEventListener('click', function () { draw(-1); });
          kids.push(add);
        }
        kids.push(h('div.wr-tpl', h('p.wr-h', NH.t('web.templates')), Object.keys(TPL).map(function (t) {
          var b = h('button', { type: 'button' }, NH.t('web.tpl_' + t));
          b.addEventListener('click', function () { if (confirm(NH.t('web.tpl_confirm'))) o.template(t); });
          return b;
        })));
      }
      el.replaceChildren.apply(el, kids);
      var inp = el.querySelector('input');
      if (inp) { inp.focus(); inp.setSelectionRange(0, inp.value.lastIndexOf('.') > 0 ? inp.value.lastIndexOf('.') : inp.value.length); }
    }
    function suggest(files) {
      var has = files.map(function (f) { return f.n; });
      return ['style.css', 'app.js', 'index.html'].filter(function (n) { return has.indexOf(n) === -1; })[0] || 'file' + (files.length + 1) + '.js';
    }
    function nameInput(value, done) {
      var inp = h('input.wr-name-in', { type: 'text', value: value, spellcheck: 'false', autocapitalize: 'off', 'aria-label': NH.t('web.file_name') });
      var err = h('p.wr-err.hide', { role: 'alert' });
      function commit() {
        var p = done(inp.value.trim());
        if (p) { err.textContent = p; err.classList.remove('hide'); inp.focus(); return; }
        draw();
      }
      inp.addEventListener('keydown', function (e) {
        if (e.key === 'Enter') { e.preventDefault(); commit(); }
        if (e.key === 'Escape') { e.preventDefault(); draw(); }
      });
      inp.addEventListener('blur', function () { setTimeout(function () { if (el.contains(inp)) draw(); }, 150); });
      return h('div.wr-file.editing', inp, err);
    }
    return { el: el, draw: draw };
  }

  // Phone: one pane at a time.
  function phoneTabs(root) {
    var bar = h('div.wr-tabs', { role: 'tablist' }, ['code', 'result', 'console'].map(function (p) {
      var b = h('button', { type: 'button', role: 'tab', 'aria-selected': p === 'code' ? 'true' : 'false', 'data-pane': p }, NH.t('web.p_' + p));
      b.addEventListener('click', function () { show(p); });
      return b;
    }));
    function show(p) {
      root.dataset.pane = p;
      NH.$$('button', bar).forEach(function (b) { b.setAttribute('aria-selected', b.dataset.pane === p); });
    }
    root.dataset.pane = 'code';
    return { el: bar, show: show };
  }
  // Full screen: the files on the left, code in the middle, the result on the
  // right. Esc or the button again leaves it.
  function fullToggle(root) {
    function set(on) {
      root.classList.toggle('wr-full', on);
      document.body.classList.toggle('wr-full-on', on);
      NH.$$('.wr-full-btn span', root).forEach(function (s) { s.textContent = NH.t(on ? 'web.exit_full' : 'web.full'); });
    }
    root.addEventListener('keydown', function (e) { if (e.key === 'Escape' && root.classList.contains('wr-full')) set(false); });
    return function () { set(!root.classList.contains('wr-full')); };
  }

  // ---------- in the editor ----------
  function editor(root, ta, opts) {
    var files = copyTpl('web'), active = 0, autoTimer = null, auto = !!NH.store.get('nh_run_auto', false);
    var body = NH.$('.ed-body', root), bar = NH.$('.ed-bar', root);
    var tabs = phoneTabs(root), toggleFull = fullToggle(root);
    var runner = Runner({ rerun: run, full: toggleFull });
    var ex = Explorer({
      files: function () { return files; }, active: function () { return active; },
      open: function (i) { open(i); },
      add: function (n) { sync(); files.push({ n: n, c: '' }); open(files.length - 1); opts.changed(); },
      rename: function (i, n) { files[i].n = n; ex.draw(); opts.changed(); },
      remove: function (i) { sync(); files.splice(i, 1); if (active >= files.length) active = files.length - 1; ta.value = files[active].c; ex.draw(); opts.refresh(); opts.changed(); },
      template: function (t) { files = copyTpl(t); active = 0; ta.value = files[0].c; ex.draw(); opts.refresh(); opts.changed(); }
    });
    var runBtn = h('button.btn.sm.primary.wr-run-btn', { type: 'button', title: 'Ctrl+Enter' }, NH.icon('play'), h('span', NH.t('web.run')));
    runBtn.addEventListener('click', function () { run(); tabs.show('result'); });
    var autoIn = h('input', { type: 'checkbox', checked: auto });
    autoIn.addEventListener('change', function () { auto = autoIn.checked; NH.store.set('nh_run_auto', auto); if (auto) run(); });
    var fullBtn = h('button.btn.sm.wr-full-btn', { type: 'button' }, NH.icon('expand'), h('span', NH.t('web.full')));
    fullBtn.addEventListener('click', function () { toggleFull(); });
    var tools = h('div.wr-tools', h('label.wr-auto', { title: NH.t('web.auto_hint') }, autoIn, h('span', NH.t('web.auto'))), fullBtn, runBtn);
    bar.appendChild(tools);
    body.insertBefore(ex.el, body.firstChild);
    body.insertBefore(tabs.el, body.firstChild);
    body.appendChild(runner.el);

    function sync() { if (files[active]) files[active].c = ta.value; }
    function open(i) {
      sync();
      active = Math.max(0, Math.min(i, files.length - 1));
      ta.value = files[active].c;
      ta.setSelectionRange(0, 0);
      ta.scrollTop = 0;
      ex.draw();
      opts.refresh();
      tabs.show('code');
      ta.focus();
    }
    function run() { sync(); runner.run(files.map(function (f) { return { n: f.n, c: f.c }; })); }
    return {
      // Text in → files; the open file stays open when it still exists.
      load: function (text) {
        var name = files[active] && files[active].n, sel = [ta.selectionStart, ta.selectionEnd];
        files = toProject(text);
        if (!files.length) files = copyTpl('web');
        var at = files.map(function (f) { return f.n; }).indexOf(name);
        var same = at !== -1 && files[at].c === ta.value;
        active = at !== -1 ? at : Math.max(0, files.map(function (f) { return kind(f.n); }).indexOf('html'));
        if (!same) ta.value = files[active].c;
        else ta.setSelectionRange(sel[0], sel[1]);
        ex.draw();
      },
      value: function () { sync(); return join(files); },
      changed: function () {
        sync();
        var row = ex.el.querySelector('.wr-file.on .wr-lines');
        if (row) row.textContent = NH.t('web.lines', { n: lines(ta.value) });
        if (!auto) return;
        clearTimeout(autoTimer);
        autoTimer = setTimeout(run, 900);
      },
      run: function () { run(); tabs.show('result'); },
      file: function () { return files[active] && files[active].n; }
    };
  }

  // ---------- read-only view (note page) ----------
  function view(el, text) {
    var files = toProject(text), active = Math.max(0, files.map(function (f) { return kind(f.n); }).indexOf('html'));
    var root = h('div.wr-view'), code = h('div.wr-code'), toggleFull = fullToggle(root), tabs = phoneTabs(root), asked = false;
    var runner = Runner({ rerun: function () { if (asked) runner.run(files); else ask(); }, full: toggleFull });
    var ex = Explorer({ files: function () { return files; }, active: function () { return active; }, open: function (i) { active = i; ex.draw(); drawCode(); tabs.show('code'); } });
    function drawCode() {
      var f = files[active] || { n: '', c: '' }, c = h('code', f.c), n = lines(f.c), nums = [];
      for (var i = 1; i <= n; i++) nums.push(i);
      try { if (HL[kind(f.n)] && hljs.getLanguage(HL[kind(f.n)])) { c.className = 'language-' + HL[kind(f.n)]; hljs.highlightElement(c); } } catch (e) {}
      code.replaceChildren(h('div.wr-code-head', fileIcon(f.n), h('b', f.n), h('span.wr-ro', NH.icon('eye'), NH.t('web.read_only'))),
        h('div.code-note', { dir: 'ltr' }, h('pre.code-gutter', { 'aria-hidden': 'true' }, nums.join('\n')), h('pre.code-text', c)));
    }
    // Someone else's code never runs by itself: the reader says so first.
    function ask() {
      var go = h('button.btn.primary', { type: 'button' }, NH.icon('play'), NH.t('web.ask_run'));
      var no = h('button.btn', { type: 'button' }, NH.t('web.ask_view'));
      go.addEventListener('click', function () { asked = true; runner.run(files); });
      no.addEventListener('click', function () { tabs.show('code'); runner.idle(); });
      runner.idle(h('div.wr-ask', h('span.wr-ask-ico', NH.icon('shield')), h('h3', NH.t('web.ask_h')), h('p', NH.t('web.ask_p')),
        h('ul', ['web.ask_1', 'web.ask_2', 'web.ask_3'].map(function (k) { return h('li', NH.icon('tick'), NH.t(k)); })), h('div.btn-row', go, no)));
    }
    var runBtn = h('button.btn.sm.primary', { type: 'button' }, NH.icon('play'), NH.t('web.run'));
    runBtn.addEventListener('click', function () { tabs.show('result'); if (asked) runner.run(files); else ask(); });
    root.append(h('div.wr-view-bar', h('span.wr-safe', NH.icon('shield'), NH.t('web.safe')), runBtn), tabs.el, h('div.wr-panes', ex.el, code, runner.el));
    ex.draw();
    drawCode();
    ask();
    el.replaceChildren(root);
  }

  window.NHWeb = { editor: editor, view: view, parse: parse, join: join, page: page, toProject: toProject, isBundle: isBundle };
})();
