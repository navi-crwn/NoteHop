// Browser checks for live collaboration: two editors typing into the same
// note, a reader watching, presence, chat, saving by one tab, encryption on
// the wire, following a presenter and opened blocks surviving live redraws.
// Start a server on an empty database, then:
//   NOTEHOP_URL=http://127.0.0.1:8090 node scripts/e2e-live.js
const { chromium } = require('playwright');
const BASE = (process.env.NOTEHOP_URL || 'http://127.0.0.1:8090').replace(/\/$/, '');
// Menu ⋯: some items sit in a submenu (View, Save to device).
async function act(p, a) {
  await p.click('#more-menu > button');
  const sub = `.menu-sub:has([data-act=${a}])`;
  if (await p.$(sub) && !(await p.$eval(sub, d => d.open))) await p.click(sub + ' > summary');
  return p.click(`[data-act=${a}]`);
}
const R = []; const ok = (n, c, i) => R.push((c ? 'PASS' : 'FAIL') + ' | ' + n + ' | ' + (i || ''));
const errs = [];
async function pickMode(p, m) { await p.evaluate(() => { ['opt-who', 'more-modes'].forEach(id => { const d = document.getElementById(id); if (d) d.open = true; }); }); await p.check(`input[name=mode][value=${m}]`, { force: true }); }
async function page(b) {
  const p = await ((await b.newContext().then(async c => { await c.addInitScript(() => { try { localStorage.setItem('nh_tour', '1'); } catch (e) {} }); return c; }))).newPage();
  p.on('pageerror', e => errs.push(e.message));
  p.on('console', m => { if (m.type() === 'error' && !/status of 4\d\d/.test(m.text())) errs.push(m.text()); });
  return p;
}
const wait = (p, fn, arg, t) => p.waitForFunction(fn, arg, { timeout: t || 10000 });

(async () => {
  const b = await chromium.launch();
  try {
    const a = await page(b);
    const frames = [];
    a.on('websocket', ws => ws.on('framesent', f => frames.push(String(f.payload))));
    await a.goto(BASE + '/new');
    await a.fill('#title', 'Rapat');
    await a.fill('.ed-src', 'agenda rahasia');
    await pickMode(a, 'edit');
    await a.click('#create'); await a.waitForURL(/#k=/); await a.waitForSelector('#note:not(.hide)');
    await a.keyboard.press('Escape');
    await a.waitForSelector('#live-bar:not(.hide)');
    const u = new URL(a.url()), k = new URLSearchParams(u.hash.slice(1)).get('k');
    const editURL = BASE + u.pathname + '#k=' + k;

    const c = await page(b);
    await c.goto(editURL); await c.waitForSelector('#note:not(.hide)');
    await wait(c, () => document.querySelectorAll('#live-people .live-av').length === 2);
    ok('presence: both see each other', (await a.$$('#live-people .live-av')).length === 2);

    await a.focus('.ed-src'); await a.keyboard.press('End'); await a.keyboard.type(' satu');
    await c.focus('.ed-src'); await c.keyboard.press('Home'); await c.keyboard.type('X ');
    await wait(a, () => document.querySelector('.ed-src').value === 'X agenda rahasia satu');
    await wait(c, () => document.querySelector('.ed-src').value === 'X agenda rahasia satu');
    ok('collab: concurrent edits merged', true);
    ok('carets: remote caret shown', await wait(a, () => document.querySelectorAll('.live-caret').length >= 1).then(() => true, () => false));

    ok('wire: no plaintext sent', frames.length > 0 && !frames.some(f => f.includes('rahasia') || f.includes('satu')));

    // Only one tab saves; the server ends up with the merged text.
    await a.waitForTimeout(2500);
    const r = await page(b);
    await r.addInitScript(() => { delete window.WebSocket; }); // read what the server stored
    await r.goto(editURL); await r.waitForSelector('#note:not(.hide)');
    ok('save: merged text persisted', (await r.inputValue('.ed-src')) === 'X agenda rahasia satu', await r.inputValue('.ed-src'));
    await r.close();

    // Read-only link: watches live.
    await a.click('#act-share'); await a.waitForSelector('#share-dlg[open]');
    const readURL = await a.evaluate(async () => {
      const N = window.NHNote;
      const res = await N.api('POST', '/links', { mode: 'read' });
      return location.origin + '/n/' + res.token + '#k=' + N.key();
    });
    await a.keyboard.press('Escape');
    const v = await page(b);
    await v.goto(readURL); await v.waitForSelector('#note:not(.hide)');
    await v.waitForSelector('#live-bar:not(.hide)');
    await a.focus('.ed-src'); await a.keyboard.press('End'); await a.keyboard.type(' dua');
    await wait(v, () => document.querySelector('#n-view').textContent.includes('satu dua'));
    ok('reader: sees edits live', true);

    // Chat
    await c.click('#live-chat-btn'); await c.fill('#live-chat-form input', 'halo semua'); await c.press('#live-chat-form input', 'Enter');
    await wait(a, () => document.querySelector('#live-unread').textContent === '1');
    await a.click('#live-chat-btn');
    ok('chat: delivered', (await a.textContent('#live-msgs')).includes('halo semua'));
    ok('chat: encrypted on the wire', !frames.some(f => f.includes('halo')));

    // Present / follow
    await a.click('#live-present');
    await wait(v, () => !document.querySelector('#live-follow').classList.contains('hide'));
    await v.click('#live-follow button');
    await act(a, 'present');
    await wait(v, () => !!document.querySelector('.overlay.present'));
    ok('follow: slides open for follower', true);
    await a.keyboard.press('Escape');
    await a.screenshot({ path: process.env.SHOT || '/dev/null' }).catch(() => {});

    // Opened blocks stay open for a reader while the author's tab is live:
    // the sync on joining and the author's edits redraw the reader's view.
    const w = await page(b);
    await w.goto(BASE + '/new');
    await w.fill('#title', 'Blok');
    await w.click('.ed-src'); await w.keyboard.type('Awal\n/pin');
    await w.waitForSelector('.slash li'); await w.keyboard.press('Enter');
    await w.waitForSelector('#lock-text'); await w.fill('#lock-text', 'kode 4321'); await w.fill('#lock-pin', '2468');
    await w.click('#lock-form button[type=submit]');
    await wait(w, () => document.querySelector('.ed-src').value.includes(':::locked'));
    await w.keyboard.press('Control+End'); await w.keyboard.type('\n/burn');
    await w.waitForSelector('.slash.open'); await w.keyboard.press('Enter');
    await w.waitForSelector('#burn-text'); await w.fill('#burn-text', 'sandi 5150'); await w.selectOption('#burn-views', '1');
    await w.click('dialog[open] button[type=submit]');
    await wait(w, () => document.querySelector('.ed-src').value.includes(':::burn '));
    await pickMode(w, 'read');
    await w.click('#create'); await w.waitForURL(/#k=/); await w.waitForSelector('#note:not(.hide)');
    await w.keyboard.press('Escape');
    const wu = new URL(w.url());
    const rd = await page(b); rd.on('dialog', d => d.accept());
    await rd.goto(BASE + wu.pathname + '#k=' + new URLSearchParams(wu.hash.slice(1)).get('k'));
    await rd.waitForSelector('.locked-form');
    await rd.fill('.locked-form input', '2468'); await rd.click('.locked-form button');
    await rd.click('.burnblk button');
    await wait(rd, () => /kode 4321/.test(document.querySelector('#n-view').textContent) && /sandi 5150/.test(document.querySelector('#n-view').textContent));
    await wait(rd, () => document.querySelectorAll('#live-people .live-av').length === 2);
    await w.focus('.ed-src'); await w.keyboard.press('Control+Home'); await w.keyboard.type('Baru ');
    await wait(rd, () => document.querySelector('#n-view').textContent.includes('Baru Awal'));
    const view = await rd.textContent('#n-view');
    ok('reader: opened PIN block stays open after live redraw', view.includes('kode 4321'));
    ok('reader: revealed burn block stays visible after live redraw', view.includes('sandi 5150'));
  } catch (e) {
    ok('FATAL', false, e.message.split('\n').slice(0, 3).join(' | '));
  } finally {
    await b.close();
    console.log(R.join('\n'));
    console.log('errors:', errs);
    process.exit(R.some(r => r.startsWith('FAIL')) || errs.length ? 1 : 0);
  }
})();
