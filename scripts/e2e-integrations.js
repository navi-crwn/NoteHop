// Browser checks for integrations: CLI ↔ browser, inbox (webhook-to-note),
// watch webhook settings, export/import, public profile, mock API and
// reading offline through the service worker.
// Start a server on an empty database, then:
//   NOTEHOP_URL=http://127.0.0.1:8090 NOTEHOP_BIN=bin/notehop node scripts/e2e-integrations.js
const { chromium } = require('playwright');
const { execFileSync } = require('child_process');
const fs = require('fs');
const BASE = (process.env.NOTEHOP_URL || 'http://127.0.0.1:8090').replace(/\/$/, '');
const BIN = process.env.NOTEHOP_BIN || 'bin/notehop';
const R = []; const ok = (n, c, i) => R.push((c ? 'PASS' : 'FAIL') + ' | ' + n + ' | ' + (i || ''));
const errs = [];
async function page(ctx) {
  const p = await ctx.newPage();
  p.on('pageerror', e => errs.push(e.message));
  p.on('console', m => { if (m.type() === 'error' && !/status of (4\d\d|503)|Failed to load resource/.test(m.text())) errs.push(m.text()); });
  p.on('dialog', d => d.accept());
  return p;
}
async function create(p, title, body, opts = {}) {
  await p.goto(BASE + '/');
  await p.fill('#title', title); await p.fill('.ed-src', body);
  if (opts.public) await p.click('label:has([data-feature=encrypt])');
  await p.click('#create'); await p.waitForURL(/\/n\//); await p.waitForSelector('#note:not(.hide)');
  await p.keyboard.press('Escape');
  return p.url();
}

(async () => {
  const b = await chromium.launch();
  try {
    const ctx = await b.newContext({ acceptDownloads: true });
    const p = await page(ctx);

    // --- CLI ↔ browser ---
    const out = execFileSync(BIN, ['new', '-server', BASE, '-password', 'pw123'], { input: '# Dari terminal\n\nisi rahasia\n' }).toString().trim();
    await p.goto(out); await p.waitForSelector('.pw-form');
    await p.fill('.pw-form input', 'pw123'); await p.click('.pw-form button');
    await p.waitForSelector('#note:not(.hide)');
    ok('cli → browser: decrypts', (await p.textContent('#n-title')) === 'Dari terminal' && (await p.textContent('#n-view')).includes('isi rahasia'));
    const made = await create(p, 'Dari browser', 'halo terminal');
    const u = new URL(made), share = BASE + u.pathname + '#k=' + new URLSearchParams(u.hash.slice(1)).get('k');
    const got = execFileSync(BIN, ['get', share]).toString();
    ok('browser → cli: decrypts', got === '# Dari browser\n\nhalo terminal\n', JSON.stringify(got));

    // --- inbox + watch (advanced settings) ---
    await p.goto(made); await p.waitForSelector('#note:not(.hide)'); await p.keyboard.press('Escape');
    await p.click('#act-share'); await p.waitForSelector('#share-dlg[open]'); await p.click('#share-adv summary');
    const pill = n => p.locator('#adv-pills .pill-opt').nth(n);
    await pill(9).click(); await p.click('#adv-forms form:nth-child(10) button[type=submit]');
    await p.waitForFunction(() => /\/in\//.test(document.querySelector('#adv-forms form:nth-child(10) .hint').textContent));
    const hint = await p.textContent('#adv-forms form:nth-child(10) .hint');
    const inbox = hint.match(/https?:\/\/\S+\/in\/[A-Za-z0-9]+/)[0];
    ok('inbox: address shown', !!inbox, inbox);
    await pill(8).click(); await p.fill('#adv-forms form:nth-child(9) input', 'http://example.com/x');
    await p.click('#adv-forms form:nth-child(9) button[type=submit]'); await p.waitForTimeout(400);
    ok('watch: http refused', (await p.textContent('#adv-forms form:nth-child(9) .hint')).startsWith('Off'));
    await p.fill('#adv-forms form:nth-child(9) input', 'https://example.com/hook');
    await p.click('#adv-forms form:nth-child(9) button[type=submit]'); await p.waitForTimeout(400);
    ok('watch: https saved with key', /key: [A-Za-z0-9]{32}/.test(await p.textContent('#adv-forms form:nth-child(9) .hint')));
    await p.keyboard.press('Escape');
    const res = await fetch(inbox, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ text: 'build gagal di main', from: 'CI' }) });
    ok('inbox: post accepted', res.status === 201, String(res.status));
    await p.reload(); await p.waitForSelector('#entry-list .entry');
    const entry = await p.textContent('#entry-list .entry');
    ok('inbox: entry decrypted in browser', entry.includes('CI') && entry.includes('build gagal di main'), entry);

    // --- account: export / import, profile ---
    await p.goto(BASE + '/register'); const user = 'e' + Date.now().toString(36);
    await p.fill('#username', user); await p.fill('#password', 'katasandi-panjang'); await p.fill('#password2', 'katasandi-panjang');
    await p.click('#auth-form button[type=submit]'); await p.waitForURL(/dashboard/, { timeout: 30000 });
    await create(p, 'Rahasia akun', 'isi untuk ekspor');
    const pub = await create(p, 'Menu', '```json\n{"menu":["kopi","teh"]}\n```', { public: true });
    await p.click('#act-share'); await p.waitForSelector('#share-dlg[open]'); await p.click('#share-adv summary');
    await pill(10).click(); await p.waitForTimeout(500); await p.keyboard.press('Escape');
    const prof = await (await ctx.newPage()).goto(BASE + '/@' + user);
    ok('profile: lists the public note', prof.status() === 200 && (await prof.text()).includes('Menu'));
    const token = new URL(pub).pathname.split('/')[2];
    const mock = await fetch(BASE + '/mock/' + token + '/menu/1');
    ok('mock api: value', mock.status === 200 && (await mock.json()) === 'teh');

    await p.goto(BASE + '/dashboard'); await p.waitForSelector('#mine .note-row');
    const [dl] = await Promise.all([p.waitForEvent('download'), p.click('#export-btn')]);
    const data = JSON.parse(fs.readFileSync(await dl.path(), 'utf8'));
    ok('export: decrypted notes in file', data.format === 'notehop-export' && data.notes.some(n => n.body === 'isi untuk ekspor'), String(data.notes.length));
    const before = (await p.$$('#mine .note-row')).length;
    await p.setInputFiles('#import-file', { name: 'x.json', mimeType: 'application/json', buffer: Buffer.from(JSON.stringify(data)) });
    await p.waitForFunction(n => document.querySelectorAll('#mine .note-row').length === n, before * 2, { timeout: 20000 });
    ok('import: notes recreated', true);

    // --- offline (service worker) ---
    const o = await page(ctx);
    await o.goto(share); await o.waitForSelector('#note:not(.hide)');
    await o.waitForFunction(() => navigator.serviceWorker && navigator.serviceWorker.controller, null, { timeout: 10000 }).catch(() => {});
    await o.reload(); await o.waitForSelector('#note:not(.hide)'); // now controlled: the open response is kept
    await o.waitForTimeout(500);
    await ctx.setOffline(true);
    await o.reload(); await o.waitForSelector('#note:not(.hide)', { timeout: 15000 });
    ok('offline: note readable', (await o.textContent('#n-view')).includes('halo terminal') && (await o.textContent('#banner')).toLowerCase().includes('offline'));
    await ctx.setOffline(false);
  } catch (e) {
    ok('FATAL', false, e.message.split('\n').slice(0, 3).join(' | '));
  } finally {
    await b.close();
    console.log(R.join('\n'));
    console.log('errors:', errs);
    process.exit(R.some(r => r.startsWith('FAIL')) || errs.length ? 1 : 0);
  }
})();
