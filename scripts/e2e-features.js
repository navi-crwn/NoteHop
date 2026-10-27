const { chromium } = require('playwright');
const OUT = process.argv[2] || require('os').tmpdir(); // screenshots
// Browser checks for the note extras (templates, calc, diagrams, wiki links,
// slides, zen, palette, redaction, dashboard groups, clipper).
// Start a server on an empty database, then:
//   NOTEHOP_URL=http://127.0.0.1:8090 node scripts/e2e-features.js
const BASE = (process.env.NOTEHOP_URL || 'http://127.0.0.1:8090').replace(/\/$/, '');
// Menu ⋯: some items sit in a submenu (View, Save to device).
async function act(p, a) {
  await p.click('#more-menu > button');
  const sub = `.menu-sub:has([data-act=${a}])`;
  if (await p.$(sub) && !(await p.$eval(sub, d => d.open))) await p.click(sub + ' > summary');
  return p.click(`[data-act=${a}]`);
}
const USER = 'f_' + Date.now().toString(36);
const R = []; const ok = (n, c, i) => R.push((c ? 'PASS' : 'FAIL') + ' | ' + n + ' | ' + (i || ''));
async function pickMode(p, m) { await p.evaluate(() => { const d = document.getElementById('more-modes'); if (d) d.open = true; }); await p.check(`input[name=mode][value=${m}]`, { force: true }); }
(async () => {
  const b = await chromium.launch(); const errs = [];
  const ctx = (await b.newContext({ permissions: ['clipboard-read', 'clipboard-write'], acceptDownloads: true }).then(async c => { await c.addInitScript(() => { try { localStorage.setItem('nh_tour', '1'); } catch (e) {} }); return c; }));
  const p = await ctx.newPage(); p.on('pageerror', e => errs.push(e.message)); p.on('dialog', d => d.accept());
  p.on('console', m => { if (m.type() === 'error' && !/40[134]/.test(m.text())) errs.push('console: ' + m.text()); });
  // account so wiki + dashboard work
  await p.goto(BASE + '/register'); await p.fill('#username', USER); await p.fill('#password', 'katasandi-panjang'); await p.fill('#password2', 'katasandi-panjang');
  await p.click('#auth-form button[type=submit]'); await p.waitForURL(/dashboard/, { timeout: 30000 });
  async function mk(title, body) {
    await p.goto(BASE + '/new'); await p.fill('#title', title); await p.fill('.ed-src', body);
    await pickMode(p, 'read');
    await p.click('#create'); await p.waitForURL(/\/n\//); await p.waitForSelector('#note:not(.hide)'); await p.keyboard.press('Escape');
    return p.url();
  }
  const other = await mk('Resep', '- [ ] beli telur\nhttps://example.com');
  const main = await mk('Utama', 'Halo {{nama}}, total {{harga}}.\n\n```calc\nharga = 5.000 * 3\nongkir = 10%  * harga\ntotal\n```\n\n```mermaid\nflowchart LR\n A-->B\n```\n\nLihat [[Resep]] dan [[Tidak Ada]].\n\n---\n\nSlide dua');
  const u = new URL(main); const k = new URLSearchParams(u.hash.slice(1)).get('k');
  await p.goto(BASE + u.pathname + '#k=' + k); await p.waitForSelector('#note:not(.hide)');
  await p.waitForSelector('#tpl:not(.hide)');
  const inputs = await p.$$('#tpl-fields input'); ok('template: 2 fields', inputs.length === 2, inputs.length);
  await inputs[0].fill('Budi'); await p.waitForTimeout(800);
  ok('template: value rendered', (await p.textContent('#n-view')).includes('Halo Budi'));
  await p.click('#tpl-copy'); const copied = await p.evaluate(() => navigator.clipboard.readText());
  ok('template: copy keeps unfilled', copied.includes('Halo Budi') && copied.includes('{{harga}}'), copied.slice(0, 40));
  const calc = await p.$$eval('table.calc .calc-res', xs => xs.map(x => x.textContent));
  ok('calc results', calc.length === 3, JSON.stringify(calc));
  await p.waitForSelector('.mermaid-box img', { timeout: 20000 }).then(() => ok('mermaid renders as image', true), () => ok('mermaid renders as image', false, '').valueOf());
  await p.waitForSelector('#n-view a:has-text("Resep")', { timeout: 10000 }).then(() => ok('wiki link resolves', true), () => ok('wiki link resolves', false));
  ok('wiki missing marked', !!(await p.$('.wiki-missing')));
  await p.screenshot({ path: OUT + '/A-note.png', fullPage: true });
  // more menu
  const [dl] = await Promise.all([p.waitForEvent('download'), act(p, 'export-md')]);
  ok('export md', /Utama\.md$/.test(dl.suggestedFilename()), dl.suggestedFilename());
  await act(p, 'present'); await p.waitForSelector('.overlay.present');
  const c1 = await p.textContent('.slide-count'); await p.keyboard.press('ArrowRight'); const c2 = await p.textContent('.slide-count');
  ok('present slides', c1.startsWith('1 /') && c2.startsWith('2 /'), c1 + ' → ' + c2); await p.keyboard.press('Escape');
  await act(p, 'prompter'); await p.waitForSelector('.overlay.prompter'); ok('prompter opens', true); await p.keyboard.press('Escape');
  await act(p, 'zen'); ok('zen on', await p.evaluate(() => document.body.classList.contains('zen')) && await p.isHidden('.nav'));
  await p.click('.zen-bar button:has-text("Exit")'); ok('zen off', !(await p.evaluate(() => document.body.classList.contains('zen'))));
  await p.fill('#tpl-fields input >> nth=0', ''); 
  // redacted copy (admin page has key)
  await p.goto(main); await p.waitForSelector('#note:not(.hide)'); await p.keyboard.press('Escape');
  await p.evaluate(() => {}); 
  // duplicate
  await p.click('#more-menu > button'); await p.click('[data-act=duplicate]'); await p.waitForURL(u2 => !u2.href.includes(u.pathname)); await p.waitForSelector('#note:not(.hide)');
  ok('duplicate opens new admin note', (await p.inputValue('#n-title-input')) === 'Utama'); await p.keyboard.press('Escape');
  // editor palette + util on the duplicate (edit mode)
  await p.fill('.ed-src', 'pisang\napel\napel\nceri'); await p.click('.ed-src'); await p.keyboard.press('Control+k'); await p.waitForSelector('dialog.palette[open]');
  await p.keyboard.type('remove duplicate'); await p.keyboard.press('Enter'); await p.waitForTimeout(200);
  ok('palette dedupe', (await p.inputValue('.ed-src')).split('\n').length === 3, JSON.stringify(await p.inputValue('.ed-src')));
  await p.keyboard.press('Control+k'); await p.keyboard.type('Sort lines A'); await p.keyboard.press('Enter'); await p.waitForTimeout(200);
  ok('palette sort', (await p.inputValue('.ed-src')).startsWith('apel\nceri'), JSON.stringify(await p.inputValue('.ed-src')));
  // bubble
  await p.fill('.ed-src', 'kata tebal'); await p.click('.ed-src');
  await p.evaluate(() => { const t = document.querySelector('.ed-src'); t.focus(); t.setSelectionRange(5, 10); });
  await p.dispatchEvent('.ed-src', 'mouseup'); await p.waitForTimeout(100);
  ok('bubble shows', await p.isVisible('.bubble.open'));
  // redact
  await p.fill('.ed-src', 'mail budi@contoh.com, telp +62 812-3456-7890, password: rahasia123'); await p.waitForTimeout(1600);
  await act(p, 'redact'); await p.waitForSelector('.redact-out');
  const red = await p.inputValue('.redact-out'); ok('redact masks', !red.includes('budi@') && !red.includes('7890') && !red.includes('rahasia123'), red);
  await p.click('dialog[open] .btn.primary'); await p.waitForTimeout(1500); const rl = await p.evaluate(() => navigator.clipboard.readText()); ok('redact copy link', /\/n\/\w+#k=/.test(rl), rl.slice(0, 30));
  await p.keyboard.press('Escape');
  // report this note (from the menu; a thank-you toast, no reload)
  await act(p, 'report'); await p.waitForSelector('#report-dlg[open]');
  await p.selectOption('#report-reason', 'spam'); await p.fill('#report-note', 'uji laporan');
  await p.click('#report-send');
  // An earlier "copied" toast may still be fading: wait for this one by its text.
  await p.waitForSelector('.toast.ok:has-text("Thank you")', { timeout: 5000 });
  ok('report thanks', true);
  await p.waitForSelector('#report-dlg', { state: 'detached', timeout: 3000 });
  ok('report dialog closed', !(await p.$('#report-dlg[open]')));
  // dashboard groups + graph
  await p.goto(BASE + '/dashboard'); await p.waitForSelector('#mine .note-row');
  await p.click('[data-group=todo]'); await p.waitForTimeout(2500);
  const todoTitles = await p.$$eval('#mine .note-row b', xs => xs.map(x => x.textContent)); ok('group to-do', todoTitles.length === 1 && todoTitles[0] === 'Resep', JSON.stringify(todoTitles));
  await p.click('#graph-btn'); await p.waitForSelector('#graph-box svg.graph', { timeout: 15000 }); ok('graph drawn', (await p.$$('svg.graph line')).length >= 1);
  await p.screenshot({ path: OUT + '/A-graph.png' }); await p.keyboard.press('Escape');
  // clipper
  await p.goto(BASE + '/new#clip=' + encodeURIComponent(JSON.stringify({ t: 'Artikel', u: 'https://x.test/a', s: 'kutipan penting' })));
  await p.waitForTimeout(300);
  ok('clipper prefill', (await p.inputValue('#title')) === 'Artikel' && (await p.inputValue('.ed-src')).includes('> kutipan penting'));
  const href = await (await b.newPage()).goto(BASE + '/about').then(r => r.text()); ok('about has bookmarklet', /href="javascript:/.test(href));
  console.log(R.join('\n')); console.log('errors:', errs); await b.close();
  process.exit(R.some(r => r.startsWith('FAIL')) || errs.length ? 1 : 0);
})().catch(e => { console.log(R.join('\n')); console.error('FATAL', e.message.split('\n').slice(0, 5).join(' | ')); process.exit(1); });
