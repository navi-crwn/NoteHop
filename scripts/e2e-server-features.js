// Browser checks for the server-backed note features: history, suggestions,
// sign-off, polls, self-destructing blocks, decoy password, advanced share
// settings, watermark tracing, claiming, private margins and the diff page.
// Start a server on an empty database, then:
//   NOTEHOP_URL=http://127.0.0.1:8090 node scripts/e2e-server-features.js
const { chromium } = require('playwright');
const BASE = (process.env.NOTEHOP_URL || 'http://127.0.0.1:8090').replace(/\/$/, '');
// Share dialog: settings live in tabs (time, privacy, together, expert).
const tab = (p, name) => p.click(`#share-tabs [data-tab=${name}]`);
const R = []; const ok = (n, c, i) => R.push((c ? 'PASS' : 'FAIL') + ' | ' + n + ' | ' + (i || ''));
const errs = [];
async function pickMode(p, m) { await p.evaluate(() => { ['opt-who', 'more-modes'].forEach(id => { const d = document.getElementById(id); if (d) d.open = true; }); }); await p.check(`input[name=mode][value=${m}]`, { force: true }); }
async function page(b, opts) {
  const ctx = (await b.newContext(Object.assign({ permissions: ['clipboard-read', 'clipboard-write'] }, opts || {})).then(async c => { await c.addInitScript(() => { try { localStorage.setItem('nh_tour', '1'); } catch (e) {} }); return c; }));
  const p = await ctx.newPage();
  p.on('pageerror', e => errs.push(e.message)); p.on('dialog', d => d.accept());
  p.on('console', m => { if (m.type() === 'error' && !/status of 4\d\d/.test(m.text())) errs.push(m.text()); });
  return p;
}
async function create(p, { title = '', body = '', mode = 'read', pw = '', pills = [], fill = {} } = {}) {
  await p.goto(BASE + '/new');
  if (title) await p.fill('#title', title);
  if (body) await p.fill('.ed-src', body);
  await pickMode(p, mode);
  if (pw) { await p.click('#opt-private > summary'); await p.click('label:has([name=privacy][value=password])'); await p.fill('#password', pw); }
  if (pills.length) await p.click('#adv summary');
  for (const x of pills) await p.click(`label:has([data-feature=${x}])`);
  for (const [sel, v] of Object.entries(fill)) await p.fill(sel, v);
  await closeMenus(p); await p.click('#create'); await p.waitForURL(/\/n\//, { timeout: 30000 }); await p.waitForSelector('#note:not(.hide)');
  await p.keyboard.press('Escape');
  return p.url();
}
const visitor = (adminURL) => { const u = new URL(adminURL); return BASE + u.pathname + '#k=' + new URLSearchParams(u.hash.slice(1)).get('k'); };

// The More drawer covers the bar's right end; people close it before Create link.
const closeMenus = p => p.evaluate(() => document.querySelectorAll('.pick[open]').forEach(d => { d.open = false; }));
(async () => {
  const b = await chromium.launch();
  try {
    // --- history ---
    const p = await page(b);
    const edit = await create(p, { title: 'Doc', body: 'baris satu\nbaris dua', mode: 'edit' });
    await p.fill('.ed-src', 'baris satu\nbaris DUA\nbaris tiga'); await p.waitForTimeout(2200);
    await p.click('#more-menu > button'); await p.click('#act-history'); await p.waitForSelector('#history-dlg[open]');
    ok('history: diff shown', (await p.$$('#history-diff .diff-del, #history-diff .diff-add')).length >= 2);
    await p.click('#history-restore'); await p.waitForTimeout(400);
    ok('history: restore', (await p.inputValue('.ed-src')) === 'baris satu\nbaris dua', JSON.stringify(await p.inputValue('.ed-src')));
    await p.waitForTimeout(1600);

    // --- suggestions ---
    const sug = await create(p, { title: 'Draft', body: 'Hello wrld', mode: 'suggest' });
    const v = await page(b); await v.goto(visitor(sug)); await v.waitForSelector('#suggest-start-row:not(.hide)');
    await v.click('#suggest-start'); await v.fill('#n-edit .ed-src', 'Hello world'); await v.click('#suggest-send'); await v.waitForTimeout(600);
    ok('suggest: visitor back to read view', await v.isVisible('#n-view'));
    await p.goto('about:blank'); await p.goto(sug); await p.waitForSelector('#suggest-banner', { timeout: 10000 }); await p.keyboard.press('Escape');
    await p.click('#suggest-banner button'); await p.waitForSelector('#suggest-dlg[open] .suggestion');
    await p.click('#suggest-dlg .suggestion .btn.primary'); await p.waitForTimeout(400);
    ok('suggest: accepted into editor', (await p.inputValue('.ed-src')) === 'Hello world');
    await p.keyboard.press('Escape'); await p.waitForTimeout(1600);

    // --- sign-off + poll + self-destructing block (created via the editor dialog) ---
    await p.goto(BASE + '/new'); await p.fill('#title', 'Kontrak');
    await p.fill('.ed-src', 'Setuju?\n\n:::poll Makan siang\n- Nasi\n- Mie\n:::\n\n');
    await p.click('.ed-src'); await p.keyboard.press('Control+End'); await p.keyboard.type('/burn'); await p.waitForSelector('.slash.open');
    await p.keyboard.press('Enter'); await p.waitForSelector('#burn-text');
    await p.fill('#burn-text', 'PIN brankas 9911'); await p.selectOption('#burn-views', '1'); await p.click('dialog[open] button[type=submit]');
    await p.waitForFunction(() => document.querySelector('.ed-src').value.includes(':::burn '));
    await p.click('#adv summary'); await p.click('label:has([data-feature=signoff])');
    await closeMenus(p); await p.click('#create'); await p.waitForURL(/\/n\//); await p.waitForSelector('#note:not(.hide)'); await p.keyboard.press('Escape');
    const k = p.url();
    const r1 = await page(b); await r1.goto(visitor(k)); await r1.waitForSelector('.poll-opt');
    await r1.click('.poll-opt >> nth=1'); await r1.waitForTimeout(500);
    ok('poll: vote counted', (await r1.textContent('.poll-opt >> nth=1')).includes('1 ·'));
    await r1.click('.poll-opt >> nth=0'); await r1.waitForTimeout(500);
    ok('poll: vote moved', (await r1.textContent('.poll-opt >> nth=0')).includes('1 ·') && (await r1.textContent('.poll-opt >> nth=1')).includes('0 ·'));
    await r1.click('.burnblk button'); await r1.waitForSelector('.burnblk .locked-body');
    ok('burn block: revealed', (await r1.textContent('.burnblk')).includes('PIN brankas 9911'));
    const r2 = await page(b); await r2.goto(visitor(k)); await r2.waitForSelector('.burnblk');
    ok('burn block: gone for next reader', (await r2.textContent('.burnblk')).includes('destroyed'));
    await r2.fill('#sign-name', 'Budi'); await r2.click('#sign-form button'); await r2.waitForSelector('#sign-list li');
    ok('sign-off: signature listed', (await r2.textContent('#sign-list')).includes('Budi'));

    // --- decoy ---
    const dec = await create(p, { title: 'Rahasia', body: 'isi asli', pw: 'asli123', pills: ['decoy'],
      fill: { '#decoy-password': 'palsu123', '#decoy-text': 'daftar belanja biasa' } });
    const d1 = await page(b); await d1.goto(visitor(dec)); await d1.waitForSelector('.pw-form');
    await d1.fill('.pw-form input', 'palsu123'); await d1.click('.pw-form button');
    await d1.waitForSelector('#note:not(.hide)', { timeout: 20000 });
    ok('decoy: decoy text shown', (await d1.textContent('#n-view')).includes('daftar belanja biasa') && !(await d1.textContent('#n-view')).includes('isi asli'));
    await p.goto('about:blank'); await p.goto(dec); await p.fill('.pw-form input', 'asli123'); await p.click('.pw-form button');
    await p.waitForSelector('#decoy-banner', { timeout: 10000 });
    ok('decoy: admin warned', (await p.textContent('#decoy-banner')).includes('1'));
    await p.keyboard.press('Escape');

    // --- advanced share settings + watermark trace ---
    const adv = await create(p, { title: 'Rutin', body: '- [ ] minum air', mode: 'check' });
    await p.click('#act-share'); await p.waitForSelector('#share-dlg[open]'); await tab(p, 'time');
    const pill = id => p.locator(`[data-adv=${id}]`), form = id => `[data-adv-form=${id}]`;
    await pill('reset').click(); await p.click(form('reset') + ' button[type=submit]'); await p.waitForTimeout(500);
    const rh = await p.textContent(form('reset') + ' .hint'); ok('reset: saved', rh.toLowerCase().includes('daily'), rh);
    await tab(p, 'privacy'); await pill('watermark').click(); await p.waitForTimeout(500);
    ok('watermark: on', await p.isVisible('#trace-box'));
    await tab(p, 'time'); await pill('dms').click(); await p.fill(form('dms') + ' input', '7'); await p.click(form('dms') + ' button[type=submit]'); await p.waitForTimeout(500);
    ok('dms: on blocks manual capsule', await pill('open').evaluate(e => e.classList.contains('blocked')));
    await pill('dms').click(); await p.waitForTimeout(500);
    ok('dms: off', !(await pill('dms').locator('input').isChecked()));
    await p.keyboard.press('Escape');
    const w = await page(b); await w.goto(visitor(adv)); await w.waitForSelector('#note:not(.hide)');
    await w.click('#act-copy'); const leaked = await w.evaluate(() => navigator.clipboard.readText());
    await p.click('#act-share'); await p.waitForSelector('#share-dlg[open]'); await tab(p, 'privacy'); await p.click('#trace-box summary');
    await p.fill('#trace-input', 'saya cuma salin: ' + leaked); await p.waitForTimeout(200);
    ok('watermark: traced to link', (await p.textContent('#trace-result')).includes('/n/'), await p.textContent('#trace-result'));
    await p.keyboard.press('Escape');

    // --- claim + margin ---
    const c = await page(b);
    await create(c, { title: 'Tamu', body: 'dibuat sebelum login' });
    await c.goto(BASE + '/register'); const user = 's' + Date.now().toString(36);
    await c.fill('#username', user); await c.fill('#password', 'katasandi-panjang'); await c.fill('#password2', 'katasandi-panjang');
    await c.click('#auth-form button[type=submit]'); await c.waitForURL(/dashboard/, { timeout: 30000 });
    await c.waitForSelector('#claim:not(.hide)'); await c.click('#claim button'); await c.waitForTimeout(1500); await c.waitForSelector('#mine .note-row', { timeout: 15000 });
    ok('claim: guest note now in account', (await c.textContent('#mine')).includes('Tamu'));
    await c.click('#mine .note-row a.note-link'); await c.waitForSelector('#note:not(.hide)');
    await c.click('#more-menu > button'); await c.click('[data-act=margin]'); await c.waitForSelector('#margin:not(.hide)');
    await c.fill('#margin-text', 'catatan pribadiku'); await c.waitForTimeout(1500);
    await c.reload(); await c.waitForSelector('#note:not(.hide)'); await c.click('#more-menu > button'); await c.click('[data-act=margin]');
    await c.waitForSelector('#margin:not(.hide)'); ok('margin: persisted', (await c.inputValue('#margin-text')) === 'catatan pribadiku');

    // --- diff page ---
    await c.goto(BASE + '/diff'); await c.fill('#diff-a', 'a\nb\nc'); await c.fill('#diff-b', 'a\nB\nc'); await c.waitForTimeout(300);
    ok('diff page: stats', (await c.textContent('#diff-stats')).includes('+1'));
    await c.click('#diff-share'); await c.waitForTimeout(1200);
    ok('diff page: shared', /\/n\/\w+#k=/.test(await c.evaluate(() => navigator.clipboard.readText())));
  } catch (e) {
    ok('FATAL', false, e.message.split('\n').slice(0, 4).join(' | '));
  }
  console.log(R.join('\n')); console.log('errors:', errs);
  await b.close();
  process.exit(R.some(r => r.startsWith('FAIL')) || errs.length ? 1 : 0);
})();
