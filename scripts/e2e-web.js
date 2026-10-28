const { chromium } = require('playwright');
// Browser checks for web notes (web.js): files, running in the sandboxed
// /run frame, the console, the reader's "run this code?" step, and that the
// code can't reach the note page, storage, cookies or the internet.
// Start a server on an empty database, then:
//   NOTEHOP_URL=http://127.0.0.1:8090 node scripts/e2e-web.js
const BASE = (process.env.NOTEHOP_URL || 'http://127.0.0.1:8090').replace(/\/$/, '');
const R = []; const ok = (n, c, i) => R.push((c ? 'PASS' : 'FAIL') + ' | ' + n + ' | ' + (i || ''));
const PROBE = `console.log('hello', {a: 1});
try { console.log(parent.document.title) } catch (e) { console.log('parent blocked') }
try { localStorage.getItem('nh_vault'); console.log('storage OPEN') } catch (e) { console.log('storage blocked') }
try { console.log('cookie=' + document.cookie) } catch (e) { console.log('cookie blocked') }
fetch('https://example.com').then(() => console.log('net OPEN'), () => console.log('net blocked'));
try { new WebSocket('wss://example.com'); console.log('ws made') } catch (e) { console.log('ws blocked') }
try { window.top.location = 'https://example.com'; } catch (e) { console.log('top blocked') }
console.warn('careful');
document.body.insertAdjacentHTML('beforeend', '<p id="x">from app.js</p>');
noSuchFn();`;
const logs = p => p.$$eval('.wr-log li', xs => xs.map(x => x.textContent));
async function readLink(p) {
  await p.waitForSelector('dialog[open] .link-url');
  const tok = (await p.locator('dialog[open] .link-url').first().innerText()).trim().replace(/^\//, '');
  const k = new URL(p.url()).hash.match(/k=([^&]+)/)[1];
  await p.keyboard.press('Escape');
  return BASE + '/n/' + tok + '#k=' + k;
}

(async () => {
  const b = await chromium.launch(); const errs = [];
  const ctx = await b.newContext({ viewport: { width: 1440, height: 900 }, acceptDownloads: true });
  await ctx.addInitScript(() => { try { localStorage.setItem('nh_tour', '1'); } catch (e) {} });
  const p = await ctx.newPage();
  // Errors thrown by the code under test are expected; the app's own are not.
  p.on('pageerror', e => errs.push(e.message));
  p.on('console', m => { const u = (m.location() || {}).url || ''; if (m.type() === 'error' && !u.includes('/run') && !/Content Security Policy|40[0-9]|noSuchFn|example\.com|Unsafe attempt to initiate navigation/.test(m.text())) errs.push('console: ' + m.text()); });
  p.on('dialog', d => d.accept());
  const net = []; p.on('request', r => { if (!r.url().startsWith(BASE)) net.push(r.url()); });

  await p.goto(BASE + '/new');
  await p.fill('#title', 'Belanja web');
  await p.click('.ed-type-btn'); await p.click('.ed-type-list [data-type="web"]');
  ok('Web type starts from a 3-file template', JSON.stringify(await p.$$eval('.wr-files .wr-name', xs => xs.map(x => x.textContent))) === '["index.html","style.css","app.js"]');
  ok('editor shows run tools, hides Markdown tools', await p.isVisible('.wr-run-btn') && !(await p.isVisible('.ed-tools')) && !(await p.isVisible('.ed-ai')));
  ok('index.html is open first', (await p.inputValue('.ed-src')).startsWith('<!doctype html>'));
  await p.click('.wr-file:has-text("app.js") .wr-open');
  await p.fill('.ed-src', PROBE);
  // a new file, picked up without a <link>
  await p.click('.wr-new');
  await p.fill('.wr-name-in', 'Bad Name');
  await p.keyboard.press('Enter');
  ok('bad file name refused', await p.isVisible('.wr-err:not(.hide)'));
  await p.fill('.wr-name-in', 'extra.css'); await p.keyboard.press('Enter');
  ok('new file opens', (await p.inputValue('.ed-src')) === '' && (await p.$$('.wr-file')).length === 4);
  await p.fill('.ed-src', 'p#x { color: rgb(200, 0, 0); }');
  await p.click('.wr-run-btn');
  const fr = p.frameLocator('.wr-frame');
  await fr.locator('#x').waitFor({ timeout: 10000 });
  ok('page runs with its CSS and JS', (await fr.locator('h1').textContent()) === 'Hello!' && (await fr.locator('h1').evaluate(e => getComputedStyle(e).color)) === 'rgb(15, 118, 110)');
  ok('a CSS file the page never links is still used', (await fr.locator('#x').evaluate(e => getComputedStyle(e).color)) === 'rgb(200, 0, 0)');
  await p.waitForFunction(() => document.querySelectorAll('.wr-log li').length >= 9, null, { timeout: 8000 });
  const L = await logs(p);
  ok('console shows logs with file:line', L.some(x => x.startsWith('hello {"a":1}') && x.endsWith('app.js:1')), L[0]);
  ok('the code cannot touch the note page', L.some(x => x.startsWith('parent blocked')));
  ok('no storage, no cookies', L.some(x => x.startsWith('storage blocked')) && L.some(x => x.startsWith('cookie blocked')));
  ok('no internet', L.some(x => x.startsWith('net blocked')) && !L.some(x => x.includes('net OPEN')));
  ok('cannot navigate the tab', L.some(x => x.startsWith('top blocked')) && /\/new/.test(p.url()));
  ok('errors and warnings are marked', !!(await p.$('.wr-log li.error')) && !!(await p.$('.wr-log li.warn')) && L.some(x => x.includes('noSuchFn') && x.endsWith('app.js:10')));
  ok('nothing went over the network', net.length === 0, net.join(' '));
  // the frame has no same-origin access
  ok('frame is sandboxed without same-origin', (await p.getAttribute('.wr-frame', 'sandbox')) === 'allow-scripts allow-forms allow-modals');
  // a fresh frame every run
  await p.click('.wr-run-btn');
  await fr.locator('#x').waitFor();
  ok('each run starts clean', (await fr.locator('#x').count()) === 1);
  // phone-width preview, full screen
  await p.click('.wr-devices button >> nth=0');
  ok('phone width', (await p.getAttribute('.wr-wrap', 'data-device')) === 'phone' && (await p.$eval('.wr-wrap', e => e.getBoundingClientRect().width)) <= 376);
  await p.click('.wr-full-btn');
  ok('full screen: files left, code middle, result right', await p.evaluate(() => {
    const f = document.querySelector('.wr-files').getBoundingClientRect(), c = document.querySelector('.ed-src').getBoundingClientRect(), r = document.querySelector('.wr-run').getBoundingClientRect();
    return f.right <= c.left + 1 && c.right <= r.left + 1 && r.width > 300;
  }));
  await p.keyboard.press('Escape');
  ok('Esc leaves full screen', !(await p.$('.editor.wr-full')));
  // rename and delete
  await p.hover('.wr-file:has-text("extra.css")');
  await p.click('.wr-file:has-text("extra.css") [aria-label^="Rename"]');
  await p.fill('.wr-name-in', 'more.css'); await p.keyboard.press('Enter');
  ok('rename', (await p.$$eval('.wr-files .wr-name', xs => xs.map(x => x.textContent))).includes('more.css'));
  await p.hover('.wr-file:has-text("more.css")');
  await p.click('.wr-file:has-text("more.css") [aria-label^="Delete"]');
  ok('delete', (await p.$$('.wr-file')).length === 3);
  // Ctrl+Enter runs; Tab indents like a code note
  await p.click('.wr-file:has-text("app.js") .wr-open');
  await p.click('.ed-src'); await p.keyboard.press('Control+Home'); await p.keyboard.press('Tab'); await p.keyboard.type("console.log('again');"); await p.keyboard.press('Enter');
  await p.keyboard.press('Control+Enter');
  await p.waitForFunction(() => [...document.querySelectorAll('.wr-log li')].some(x => x.textContent.startsWith('again')));
  ok('Ctrl+Enter runs; Tab indents', (await p.inputValue('.ed-src')).startsWith("  console.log('again');\n"));

  // ---------- saved, then opened by a reader ----------
  await p.click('#create'); await p.waitForURL(/#k=/);
  const read = await readLink(p);
  ok('owner editor keeps the files', (await p.getAttribute('[data-editor]', 'data-type')) === 'web' && (await p.$$('.wr-file')).length === 3);
  const r = await ctx.newPage(); r.on('pageerror', e => errs.push('reader: ' + e.message));
  await r.goto(read); await r.waitForSelector('.wr-view');
  ok('reader sees the files, read-only', (await r.$$('.wr-view .wr-file')).length === 3 && await r.isVisible('.wr-ro') && !(await r.$('.wr-view .wr-new')));
  ok('reader is asked before anything runs', await r.isVisible('.wr-ask') && !(await r.$('.wr-frame')));
  await r.click('.wr-view .wr-file:has-text("app.js") .wr-open');
  ok('reader can read each file', (await r.textContent('.wr-code .code-text')).includes("console.log('again');"));
  await r.click('.wr-ask .btn.primary');
  await r.frameLocator('.wr-frame').locator('#x').waitFor();
  ok('reader runs it after saying yes', (await r.frameLocator('.wr-frame').locator('h1').textContent()) === 'Hello!');
  await r.click('#more-menu > button'); await r.click('.menu-sub:has([data-act=export-txt]) > summary');
  const [dl] = await Promise.all([r.waitForEvent('download'), r.click('[data-act=export-txt]')]);
  const txt = require('fs').readFileSync(await dl.path(), 'utf8');
  ok('download is one .txt with every file', dl.suggestedFilename() === 'Belanja web.txt' && txt.includes('--- file: index.html ---') && txt.includes('--- file: app.js ---'), dl.suggestedFilename());

  // ---------- phone ----------
  const ph = await b.newContext({ viewport: { width: 390, height: 844 }, isMobile: true });
  await ph.addInitScript(() => { try { localStorage.setItem('nh_tour', '1'); } catch (e) {} });
  const q = await ph.newPage(); q.on('pageerror', e => errs.push('phone: ' + e.message));
  await q.goto(read); await q.waitForSelector('.wr-view');
  ok('phone: Code / Result / Console tabs', await q.isVisible('.wr-view .wr-tabs') && (await q.$$('.wr-view .wr-tabs button')).length === 3);
  await q.click('.wr-view .wr-tabs [data-pane="result"]');
  ok('phone: one pane at a time', await q.isVisible('.wr-ask') && !(await q.isVisible('.wr-code')));
  ok('phone: nothing wider than the screen', await q.evaluate(() => document.documentElement.scrollWidth <= innerWidth));

  // ---------- switching away keeps the text ----------
  await p.click('.ed-type-btn'); await p.click('.ed-type-list [data-type="code"]');
  ok('Web → Code keeps every file as text', (await p.inputValue('.ed-src')).startsWith('--- file: index.html ---'));
  await p.click('.ed-type-btn'); await p.click('.ed-type-list [data-type="web"]');
  ok('Code → Web gives the files back', (await p.$$('.wr-file')).length === 3);

  console.log(R.join('\n')); console.log('errors:', errs); await b.close();
  process.exit(R.some(x => x.startsWith('FAIL')) || errs.filter(e => !/noSuchFn/.test(e)).length ? 1 : 0);
})().catch(e => { console.log(R.join('\n')); console.error('FATAL', e.message.split('\n').slice(0, 5).join(' | ')); process.exit(1); });
