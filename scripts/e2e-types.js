const { chromium } = require('playwright');
// Browser checks for note types (Plain, Document, Code) and the format menu
// (right-click on the text box, the bottom sheet on touch screens).
// Start a server on an empty database, then:
//   NOTEHOP_URL=http://127.0.0.1:8090 node scripts/e2e-types.js
const BASE = (process.env.NOTEHOP_URL || 'http://127.0.0.1:8090').replace(/\/$/, '');
const R = []; const ok = (n, c, i) => R.push((c ? 'PASS' : 'FAIL') + ' | ' + n + ' | ' + (i || ''));
async function act(p, a) {
  await p.click('#more-menu > button');
  const sub = `.menu-sub:has([data-act=${a}])`;
  if (await p.$(sub) && !(await p.$eval(sub, d => d.open))) await p.click(sub + ' > summary');
  return p.click(`[data-act=${a}]`);
}
// Right-click on the text box with [from, to) selected.
async function rightClick(p, from, to, extra) {
  return p.evaluate(([f, t, x]) => {
    const ta = document.querySelector('.ed-src'); ta.focus(); ta.setSelectionRange(f, t);
    const e = new MouseEvent('contextmenu', Object.assign({ bubbles: true, cancelable: true, clientX: 320, clientY: 320 }, x || {}));
    ta.dispatchEvent(e); return e.defaultPrevented;
  }, [from, to, extra]);
}
async function pick(p, subText, button) {
  await p.hover(`.ctx-sub:has-text("${subText}")`);
  await p.click(`.ctx-sub.open .ctx-panel ${button}`);
}
const val = p => p.inputValue('.ed-src');
// [start, end) of the first [text]{…} span in the text box.
const spanAt = async p => { const v = await val(p), s = v.indexOf('['); return [s, v.indexOf('}', s) + 1]; };
// The owner page shows the Share dialog first; its view-only link is used to read.
async function readLink(p) {
  await p.waitForSelector('dialog[open] :text("/n/")');
  const tok = (await p.locator('dialog[open]').innerText()).match(/\/n\/(\w{8,})/)[1];
  const k = new URL(p.url()).hash.match(/k=([^&]+)/)[1];
  await p.keyboard.press('Escape');
  return BASE + '/n/' + tok + '#k=' + k;
}

// The More drawer covers the bar's right end; people close it before Create link.
const closeMenus = p => p.evaluate(() => document.querySelectorAll('.pick[open]').forEach(d => { d.open = false; }));
(async () => {
  const b = await chromium.launch(); const errs = [];
  const ctx = await b.newContext({ acceptDownloads: true, viewport: { width: 1280, height: 900 } });
  await ctx.addInitScript(() => { try { localStorage.setItem('nh_tour', '1'); } catch (e) {} });
  const p = await ctx.newPage(); p.on('pageerror', e => errs.push(e.message)); p.on('dialog', d => d.accept());
  p.on('console', m => { if (m.type() === 'error' && !/40[134]/.test(m.text())) errs.push('console: ' + m.text()); });

  // ---------- Document: the right-click menu ----------
  await p.goto(BASE + '/new');
  ok('new note is a Document', (await p.getAttribute('[data-editor]', 'data-type')) === 'doc');
  await p.fill('.ed-src', 'Kumpul paling lambat 06.30.\nBaris kedua.');
  ok('menu opens on right-click', await rightClick(p, 7, 20) && await p.isVisible('.ctx'));
  ok('menu has format row', (await p.$$('.ctx .fmt-b')).length === 6);
  await pick(p, 'Colour', '.sw.tc-violet');
  ok('colour wraps the selection', (await val(p)).startsWith('Kumpul [paling lambat]{.violet} 06.30.'), await val(p));
  ok('menu closes after a pick', !(await p.isVisible('.ctx')));
  // the span stays selected, so a highlight lands in the same span
  await rightClick(p, ...(await spanAt(p))); await pick(p, 'Colour', '.sw.hl-yellow');
  ok('highlight joins the span', (await val(p)).startsWith('Kumpul [paling lambat]{.violet .hl-yellow} '), await val(p));
  await rightClick(p, 9, 9); await pick(p, 'Colour', '.sw.tc-red'); // caret inside the span
  ok('another colour replaces the first', (await val(p)).startsWith('Kumpul [paling lambat]{.hl-yellow .red} '), await val(p));
  await rightClick(p, ...(await spanAt(p))); await pick(p, 'Text size', 'button:has-text("Large")');
  ok('size added', (await val(p)).includes('{.hl-yellow .red .large}'), await val(p));
  const line2 = (await val(p)).indexOf('Baris');
  await rightClick(p, line2 + 1, line2 + 1); await pick(p, 'Alignment', 'button:has-text("Centre")');
  ok('centre wraps the line', (await val(p)).endsWith('\n:::center\nBaris kedua.\n:::'), JSON.stringify(await val(p)));
  await p.click('.seg [data-view="preview"]'); await p.waitForTimeout(300);
  ok('preview shows the styles', await p.isVisible('.ed-preview span.tc-red.hl-yellow.fs-large') && await p.isVisible('.ed-preview .al-center'));
  ok('no style attributes in preview', !(await p.$('.ed-preview [style]')));
  await p.click('.seg [data-view="write"]');
  const inBlock = (await val(p)).indexOf('Baris');
  await rightClick(p, inBlock + 2, inBlock + 2); await pick(p, 'Alignment', 'button:has-text("Right")');
  ok('alignment changes the whole block', (await val(p)).endsWith('\n:::right\nBaris kedua.\n:::'), JSON.stringify(await val(p)));
  await rightClick(p, inBlock + 2, inBlock + 2); await pick(p, 'Alignment', 'button:has-text("Left")');
  ok('left takes the block away', (await val(p)).endsWith('06.30.\nBaris kedua.'), JSON.stringify(await val(p)));
  await rightClick(p, 0, (await val(p)).indexOf('\n')); await p.click('.ctx-it:has-text("Clear formatting")');
  ok('clear formatting', (await val(p)).startsWith('Kumpul paling lambat 06.30.'), await val(p));
  await rightClick(p, 0, 6); await p.click('.ctx .fb-bold');
  ok('bold from the format row', (await val(p)).startsWith('**Kumpul**'), await val(p));
  await rightClick(p, 0, 10); await p.click('.ctx .fb-bold');
  ok('bold again takes it off', (await val(p)).startsWith('Kumpul paling'), await val(p));
  await rightClick(p, 0, 6); await pick(p, 'Upper/lower case', 'button:has-text("UPPERCASE")');
  ok('upper case', (await val(p)).startsWith('KUMPUL paling'), await val(p));
  // keyboard: arrows move, Esc closes and goes back to the text
  await rightClick(p, 0, 6);
  await p.keyboard.press('ArrowDown');
  ok('arrow moves focus', await p.evaluate(() => document.activeElement.textContent.includes('Copy')));
  await p.keyboard.press('Escape');
  ok('Esc closes and refocuses', !(await p.isVisible('.ctx')) && await p.evaluate(() => document.activeElement.classList.contains('ed-src')));
  ok('Shift + right-click keeps the browser menu', !(await rightClick(p, 0, 6, { shiftKey: true })) && !(await p.isVisible('.ctx')));
  ok('touch long-press keeps the phone menu', !(await p.evaluate(() => {
    const ta = document.querySelector('.ed-src'), e = new PointerEvent('contextmenu', { bubbles: true, cancelable: true, pointerType: 'touch' });
    ta.dispatchEvent(e); return e.defaultPrevented;
  })));
  await rightClick(p, 0, 0); await p.mouse.click(1200, 120);
  ok('click outside closes', !(await p.isVisible('.ctx')));

  // switching a styled Document to Plain asks, then drops the styles
  await p.fill('.ed-src', 'Ada [warna]{.red} di sini\n:::center\ntengah\n:::');
  await p.click('.ed-type-btn'); await p.click('.ed-type-list [data-type="plain"]');
  ok('to Plain drops styles', (await val(p)) === 'Ada warna di sini\ntengah', JSON.stringify(await val(p)));
  ok('Plain hides Markdown tools', !(await p.isVisible('.ed-tools')) && !(await p.isVisible('.seg')));
  await rightClick(p, 0, 3);
  ok('Plain menu has no colours', await p.isVisible('.ctx') && !(await p.$('.ctx .fmt-b')) && !(await p.$('.ctx-sub:has-text("Colour")')));
  await p.keyboard.press('Escape');
  await p.fill('.ed-src', '/tab');
  ok('no slash menu in Plain', !(await p.isVisible('.slash.open')));

  // ---------- Plain note ----------
  await p.fill('#title', 'Polos');
  await p.fill('.ed-src', '**bukan tebal**\n- [ ] bukan kotak\n<b>bukan html</b>');
  await closeMenus(p); await p.click('#create'); await p.waitForURL(/\/n\//);
  const plainRead = await readLink(p);
  ok('owner editor keeps Plain', (await p.getAttribute('[data-editor]', 'data-type')) === 'plain');
  await p.goto(plainRead); await p.waitForSelector('.plain-view');
  ok('Plain shows text as typed', (await p.textContent('.plain-view')) === '**bukan tebal**\n- [ ] bukan kotak\n<b>bukan html</b>');
  ok('Plain renders no Markdown/HTML', !(await p.$('#n-view strong, #n-view b, #n-view input')));

  // ---------- Code note ----------
  await p.goto(BASE + '/new');
  await p.fill('#title', 'api-client.js');
  await p.click('.ed-type-btn'); await p.click('.ed-type-list [data-type="code"]');
  ok('Code shows language and gutter', await p.isVisible('.ed-lang') && await p.isVisible('.ed-gutter'));
  await p.selectOption('.ed-lang', 'javascript');
  await p.click('.ed-src');
  await p.keyboard.type('function hi() {'); await p.keyboard.press('Enter'); await p.keyboard.press('Tab');
  await p.keyboard.type('return "<script>alert(1)</script><img src=x onerror=alert(2)>";'); await p.keyboard.press('Enter');
  ok('Tab indents, Enter keeps the indent', (await val(p)) === 'function hi() {\n  return "<script>alert(1)</script><img src=x onerror=alert(2)>";\n  ', JSON.stringify(await val(p)));
  await p.keyboard.press('Shift+Tab'); await p.keyboard.type('}');
  ok('Shift+Tab outdents', (await val(p)).endsWith('\n}'), JSON.stringify(await val(p)));
  ok('gutter counts lines', (await p.textContent('.ed-gutter')) === '1\n2\n3');
  ok('no spellcheck in Code', !(await p.$eval('.ed-src', t => t.spellcheck)));
  await p.keyboard.press('Escape'); await p.keyboard.press('Tab');
  ok('Esc then Tab leaves the box', await p.evaluate(() => !document.activeElement.classList.contains('ed-src')));
  const codeBody = await val(p);
  await closeMenus(p); await p.click('#create'); await p.waitForURL(/\/n\//);
  const owner = p.url(), codeRead = await readLink(p);
  ok('owner editor keeps Code + language', (await p.getAttribute('[data-editor]', 'data-type')) === 'code' && (await p.inputValue('.ed-lang')) === 'javascript');
  await p.goto(codeRead); await p.waitForSelector('.code-note');
  ok('Code view uses the language', (await p.getAttribute('.code-note', 'data-lang')) === 'javascript' && !!(await p.$('.code-text .hljs-keyword')));
  ok('Code view shows tags as text', (await p.textContent('.code-text')) === codeBody);
  ok('nothing from the code ends up in the page', !(await p.$('#n-view script, #n-view img, #n-view [onerror]')));
  ok('Code view has line numbers', (await p.textContent('.code-gutter')) === '1\n2\n3');
  ok('Code view says it is never run', await p.isVisible('.code-safe'));
  await p.click('#more-menu > button'); await p.click('.menu-sub:has([data-act=export-txt]) > summary');
  ok('no Markdown export for Code', await p.isHidden('#act-export-md'));
  const [dl] = await Promise.all([p.waitForEvent('download'), p.click('[data-act=export-txt]')]);
  const text = require('fs').readFileSync(await dl.path(), 'utf8');
  ok('download is a .txt with the text', dl.suggestedFilename() === 'api-client-js.txt' && text === codeBody, dl.suggestedFilename());
  await p.click('#act-copy');

  // owner switches the type back to Document: saved and shown as Markdown
  await p.goto(owner); await p.waitForSelector('[data-editor][data-type="code"]'); await p.keyboard.press('Escape');
  await p.fill('.ed-src', '# Judul\n\n[merah]{.red}');
  await p.click('.ed-type-btn'); await p.click('.ed-type-list [data-type="doc"]');
  await p.waitForFunction(() => /Saved|Tersimpan/.test(document.body.innerText), null, { timeout: 8000 }).catch(() => {});
  await p.waitForTimeout(1500);
  await p.goto(codeRead); await p.waitForSelector('#n-view h1');
  ok('type change is saved', await p.isVisible('#n-view span.tc-red') && !(await p.$('.code-note')));
  ok('Markdown export back for a Document', await p.evaluate(() => !document.getElementById('act-export-md').classList.contains('hide')));

  // ---------- public Code note: /raw is plain text ----------
  await p.goto(BASE + '/new');
  await p.fill('#title', 'publik');
  await p.click('.ed-type-btn'); await p.click('.ed-type-list [data-type="code"]');
  await p.fill('.ed-src', '<script>alert(1)</script>');
  await p.evaluate(() => { const d = document.getElementById('adv'); if (d) d.open = true; });
  await p.evaluate(() => { const c = document.getElementById('encrypt'); c.checked = false; c.dispatchEvent(new Event('change', { bubbles: true })); });
  await closeMenus(p); await p.click('#create'); await p.waitForURL(/\/n\//);
  const tok = (await p.locator('dialog[open]').innerText()).match(/\/n\/(\w{8,})/)[1];
  const raw = await fetch(BASE + '/raw/' + tok);
  ok('/raw is text/plain + nosniff', raw.headers.get('content-type').startsWith('text/plain') && raw.headers.get('x-content-type-options') === 'nosniff' &&
    (await raw.text()).includes('<script>alert(1)</script>'));

  // ---------- phone: the bottom sheet from the keyboard bar ----------
  const phone = await b.newContext({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true });
  await phone.addInitScript(() => { try { localStorage.setItem('nh_tour', '1'); } catch (e) {} });
  const q = await phone.newPage(); q.on('pageerror', e => errs.push('phone: ' + e.message));
  await q.goto(BASE + '/new');
  await q.fill('.ed-src', 'teks untuk diwarnai');
  await q.evaluate(() => { const t = document.querySelector('.ed-src'); t.focus(); t.setSelectionRange(0, 4); });
  await q.waitForSelector('.kbar.show');
  await q.click('.kbar button:has-text("Aa")');
  ok('phone sheet opens', await q.isVisible('dialog.fmt-sheet[open]'));
  await q.click('dialog.fmt-sheet .sw.tc-green');
  await q.click('dialog.fmt-sheet .sw.hl-pink');
  ok('sheet stays open and styles stack', await q.isVisible('dialog.fmt-sheet[open]') && (await q.inputValue('.ed-src')).startsWith('[teks]{.green .hl-pink}'), await q.inputValue('.ed-src'));
  ok('sheet fits the phone', await q.evaluate(() => document.querySelector('dialog.fmt-sheet').scrollWidth <= innerWidth));
  await q.screenshot({ path: require('os').tmpdir() + '/types-sheet.png' });

  console.log(R.join('\n')); console.log('errors:', errs); await b.close();
  process.exit(R.some(r => r.startsWith('FAIL')) || errs.length ? 1 : 0);
})().catch(e => { console.log(R.join('\n')); console.error('FATAL', e.message.split('\n').slice(0, 5).join(' | ')); process.exit(1); });
