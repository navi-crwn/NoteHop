// Browser checks for live embeds, Form-to-Note and the Kanban board.
// Start a server on an empty database, then:
//   NOTEHOP_URL=http://127.0.0.1:8090 node scripts/e2e-blocks.js
const { chromium } = require('playwright');
const BASE = (process.env.NOTEHOP_URL || 'http://127.0.0.1:8090').replace(/\/$/, '');
const R = []; const ok = (n, c, i) => R.push((c ? 'PASS' : 'FAIL') + ' | ' + n + ' | ' + (i || ''));
const errs = [];
async function pickMode(p, m) { await p.evaluate(() => { ['opt-who', 'more-modes'].forEach(id => { const d = document.getElementById(id); if (d) d.open = true; }); }); await p.check(`input[name=mode][value=${m}]`, { force: true }); }
async function page(b) {
  const p = await ((await b.newContext().then(async c => { await c.addInitScript(() => { try { localStorage.setItem('nh_tour', '1'); } catch (e) {} }); return c; }))).newPage();
  p.on('pageerror', e => errs.push(e.message));
  p.on('console', m => { if (m.type() === 'error' && !/status of 4\d\d/.test(m.text())) errs.push(m.text()); });
  return p;
}
async function create(p, title, body, mode) {
  await p.goto(BASE + '/new');
  await p.fill('#title', title); await p.fill('.ed-src', body);
  await pickMode(p, mode || 'read');
  await p.click('#create'); await p.waitForURL(/\/n\//); await p.waitForSelector('#note:not(.hide)');
  await p.keyboard.press('Escape');
  const u = new URL(p.url());
  return { admin: p.url(), visitor: BASE + u.pathname + '#k=' + new URLSearchParams(u.hash.slice(1)).get('k') };
}

(async () => {
  const b = await chromium.launch();
  try {
    const p = await page(b);
    // --- embed ---
    const src = await create(p, 'Jadwal', 'rapat jam 9', 'edit');
    const host = await create(p, 'Beranda', 'lihat:\n\n:::embed ' + src.visitor + '\n:::\n');
    const v = await page(b);
    await v.goto(host.visitor); await v.waitForSelector('#n-view .embed .embed-body');
    ok('embed: shows other note', (await v.textContent('#n-view .embed')).includes('rapat jam 9'));
    await p.goto(src.admin); await p.waitForSelector('#note:not(.hide)'); await p.keyboard.press('Escape');
    await p.fill('.ed-src', 'rapat jam 10'); await p.waitForTimeout(2000);
    await v.waitForFunction(() => document.querySelector('#n-view .embed').textContent.includes('jam 10'), null, { timeout: 15000 });
    ok('embed: refreshes when the source changes', true);

    // --- form ---
    const f = await create(p, 'RSVP', ':::form Datang?\n- Nama *\n- Jawaban [Ya|Tidak]\n:::\n');
    const link = await p.evaluate(async () => {
      const N = window.NHNote, r = await N.api('POST', '/links', { mode: 'append' });
      return location.origin + '/n/' + r.token + '#k=' + N.key();
    });
    const g = await page(b);
    await g.goto(f.visitor); await g.waitForSelector('#n-view .nhform');
    ok('form: disabled on a read link', await g.$eval('#n-view .nhform button[type=submit]', x => x.disabled));
    await g.goto(link); await g.reload(); await g.waitForSelector('#n-view .nhform input:not([disabled])');
    await g.fill('#n-view .nhform input', 'Budi'); await g.selectOption('#n-view .nhform select', 'Ya');
    await g.click('#n-view .nhform button[type=submit]');
    await g.waitForSelector('#entry-list .entry');
    const entry = await g.textContent('#entry-list .entry');
    ok('form: answer saved as entry', entry.includes('Budi') && entry.includes('Jawaban:') && entry.includes('Ya'), entry);

    // --- board ---
    await create(p, 'Tugas', '## Todo\n- [ ] desain\n- tulis\n\n## Doing\n\n## Done\n- [x] riset\n', 'edit');
    await p.waitForSelector('.view-switch:not(.hide)');
    await p.click('.view-switch [data-v=board]');
    await p.waitForSelector('#n-board .kcol');
    ok('board: columns', (await p.$$('#n-board .kcol')).length === 3);
    await p.selectOption('#n-board .kcol:nth-child(1) .kcard:nth-child(1) .kmove', '1');
    await p.waitForTimeout(300);
    ok('board: move by menu edits text', (await p.inputValue('.ed-src')).indexOf('## Doing\n\n- [ ] desain') !== -1, JSON.stringify(await p.inputValue('.ed-src')));
    await p.dragAndDrop('#n-board .kcol:nth-child(1) .kcard', '#n-board .kcol:nth-child(3) .kcard');
    await p.waitForTimeout(300);
    ok('board: drag and drop', /## Done\n- tulis\n- \[x\] riset/.test(await p.inputValue('.ed-src')), JSON.stringify(await p.inputValue('.ed-src')));
    await p.waitForTimeout(1800);
    await p.reload(); await p.waitForSelector('#n-board .kcol');
    ok('board: view remembered and saved', (await p.textContent('#n-board .kcol:nth-child(3)')).includes('tulis'));
  } catch (e) {
    ok('FATAL', false, e.message.split('\n').slice(0, 3).join(' | '));
  } finally {
    await b.close();
    console.log(R.join('\n'));
    console.log('errors:', errs);
    process.exit(R.some(r => r.startsWith('FAIL')) || errs.length ? 1 : 0);
  }
})();
