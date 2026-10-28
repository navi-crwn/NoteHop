const { chromium } = require('playwright');
// End-to-end check of the Phase 1 flows in a real browser (Playwright).
// Start a server on a throwaway database first, then:
//   NOTEHOP_URL=http://127.0.0.1:8090 node scripts/e2e.js [screenshot-dir]
// The UI is checked in English, the default language.
// Exits non-zero when a check fails.
const BASE = (process.env.NOTEHOP_URL || 'http://127.0.0.1:8090').replace(/\/$/, '');
const OUT = process.argv[2] || require('os').tmpdir();
const USER = 'e2e_' + Date.now().toString(36);
const results = [];
const ok = (name, cond, info) => { results.push([cond ? 'PASS' : 'FAIL', name, info || '']); };
let errs = [];

async function pickMode(p, m) { await p.evaluate(() => { ['opt-who', 'more-modes'].forEach(id => { const d = document.getElementById(id); if (d) d.open = true; }); }); await p.check(`input[name=mode][value=${m}]`, { force: true }); }
async function newPage(b, opts) {
  const ctx = (await b.newContext(Object.assign({ permissions: ['clipboard-read', 'clipboard-write'] }, opts || {})).then(async c => { await c.addInitScript(() => { try { localStorage.setItem('nh_tour', '1'); } catch (e) {} }); return c; }));
  const p = await ctx.newPage();
  p.on('console', m => { if (m.type() === 'error') errs.push(p.url() + ' :: ' + m.text()); });
  p.on('pageerror', e => errs.push(p.url() + ' :: ' + e.message));
  p.on('dialog', d => d.accept());
  return p;
}
function visitorURL(adminURL) {
  const u = new URL(adminURL); const f = new URLSearchParams(u.hash.slice(1));
  return BASE + u.pathname + '#k=' + f.get('k');
}
async function openAdv(p) { if (!(await p.$eval('#adv', d => d.open))) await p.click('#adv summary'); }
async function create(p, { title = '', body = '', mode = 'read', pw = '', burn = '0', encrypt = true } = {}) {
  await p.goto(BASE + '/new');
  if (title) await p.fill('#title', title);
  if (body) await p.fill('.ed-src', body);
  await pickMode(p, mode);
  if (!encrypt) { await openAdv(p); await p.click('#expert-box summary'); await p.click('label:has(#encrypt)'); }
  if (pw) { await p.click('#opt-private > summary'); await p.click('label:has([name=privacy][value=password])'); await p.fill('#password', pw); }
  if (burn !== '0') { await openAdv(p); if (!(await p.isChecked('[data-feature=burn]'))) await p.click('label:has([data-feature=burn])'); await p.selectOption('#burn', burn); }
  await closeMenus(p); await p.click('#create');
  await p.waitForURL(/#k=/, { timeout: 20000 });
  await p.waitForSelector('#note:not(.hide)', { timeout: 20000 });
  return p.url();
}

// The More drawer covers the bar's right end; people close it before Create link.
const closeMenus = p => p.evaluate(() => document.querySelectorAll('.pick[open]').forEach(d => { d.open = false; }));
(async () => {
  const b = await chromium.launch();

  // 1. PIN block
  try {
    const p = await newPage(b);
    await p.goto(BASE + '/new');
    await p.fill('#title', 'Uji PIN');
    await p.click('.ed-src'); await p.keyboard.type('Pembuka\n/pin');
    await p.waitForSelector('.slash li', { timeout: 3000 });
    await p.keyboard.press('Enter');
    await p.waitForSelector('#lock-text');
    await p.fill('#lock-text', 'kode brankas 4321'); await p.fill('#lock-pin', '2468');
    await p.click('#lock-form button[type=submit]');
    await p.waitForFunction(() => document.querySelector('.ed-src').value.includes(':::locked'));
    await closeMenus(p); await p.click('#create'); await p.waitForURL(/#k=/);
    await p.waitForSelector('#note:not(.hide)');
    const v = await newPage(b); await v.goto(visitorURL(p.url()));
    await v.waitForSelector('.locked-form');
    await v.fill('.locked-form input', '0000'); await v.click('.locked-form button');
    await v.waitForFunction(() => /Wrong PIN/.test(document.querySelector('.locked-form').textContent), null, { timeout: 15000 });
    ok('PIN: wrong PIN rejected', true);
    await v.fill('.locked-form input', '2468'); await v.click('.locked-form button');
    await v.waitForFunction(() => document.querySelector('#n-view').textContent.includes('kode brankas 4321'), null, { timeout: 15000 });
    ok('PIN: right PIN opens block', true);
    await v.screenshot({ path: OUT + '/pin.png', fullPage: true });
  } catch (e) { ok('PIN flow', false, e.message.split('\n')[0]); }

  // 2. checklist via check link
  try {
    const p = await newPage(b);
    const admin = await create(p, { title: 'Belanja', body: '- [ ] telur\n- [ ] susu\n\n```\n- [ ] bukan tugas\n```\n\n- [ ] roti', mode: 'check' });
    const v = await newPage(b); await v.goto(visitorURL(admin));
    await v.waitForSelector('#n-view input[type=checkbox]');
    const boxes = await v.$$('#n-view input[type=checkbox]');
    ok('Checklist: 3 tasks (code block ignored)', boxes.length === 3, 'got ' + boxes.length);
    ok('Checklist: no editor for visitor', await v.isHidden('#n-edit'));
    await boxes[2].click();
    await v.waitForTimeout(800);
    await v.reload(); await v.waitForSelector('#n-view input[type=checkbox]');
    const states = await v.$$eval('#n-view input[type=checkbox]', xs => xs.map(x => x.checked));
    ok('Checklist: tick persists after reload', JSON.stringify(states) === '[false,false,true]', JSON.stringify(states));
    await p.reload(); await p.waitForSelector('#note:not(.hide)');
    const src = await p.$eval('.ed-src', t => t.value);
    ok('Checklist: admin editor shows tick on the right line', /- \[x\] roti/.test(src) && /- \[ \] bukan tugas/.test(src), JSON.stringify(src));
    // read-only link must not allow ticking
    const r = await p.evaluate(async () => {
      const tok = document.body.dataset.token; const f = new URLSearchParams(location.hash.slice(1));
      const res = await fetch('/api/n/' + tok + '/links', { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-Admin': f.get('a') }, body: JSON.stringify({ mode: 'read' }) });
      return (await res.json()).token;
    });
    const rv = await newPage(b); await rv.goto(BASE + '/n/' + r + '#k=' + new URLSearchParams(new URL(admin).hash.slice(1)).get('k'));
    await rv.waitForSelector('#n-view input[type=checkbox]');
    ok('Checklist: read link boxes disabled', await rv.$$eval('#n-view input[type=checkbox]', xs => xs.every(x => x.disabled)));
  } catch (e) { ok('Checklist flow', false, e.message.split('\n')[0]); }

  // 3. append mode
  try {
    const p = await newPage(b);
    const admin = await create(p, { title: 'Buku tamu', body: 'Tulis salam di bawah.', mode: 'append' });
    const v = await newPage(b); await v.goto(visitorURL(admin));
    await v.waitForSelector('#entry-form:not(.hide)');
    ok('Append: no body editor for visitor', await v.isHidden('#n-edit'));
    await v.fill('#entry-name', 'Andi'); await v.fill('#entry-text', 'Halo **semua**');
    await v.click('#entry-form button[type=submit]');
    await v.waitForSelector('#entry-list li');
    await v.reload(); await v.waitForSelector('#entry-list li');
    const t = await v.textContent('#entry-list');
    ok('Append: entry persists with name + markdown', t.includes('Andi') && t.includes('semua'), t.slice(0, 80));
    ok('Append: visitor has no delete button', (await v.$$('#entry-list .icon-btn')).length === 0);
    await p.reload(); await p.waitForSelector('#entry-list li .icon-btn');
    await p.click('#entry-list li .icon-btn'); await p.waitForTimeout(800);
    await v.reload(); await v.waitForSelector('#entry-form:not(.hide)');
    ok('Append: admin deleted entry', (await v.$$('#entry-list li')).length === 0);
  } catch (e) { ok('Append flow', false, e.message.split('\n')[0]); }

  // 4. password + burn
  try {
    const p = await newPage(b);
    const admin = await create(p, { title: 'Sekali baca', body: 'isi bom waktu', pw: 'rahasia123', burn: '1' });
    const v = await newPage(b); await v.goto(visitorURL(admin));
    await v.waitForSelector('.pw-form');
    await v.fill('.pw-form input', 'salah'); await v.click('.pw-form button');
    await v.waitForSelector('#state button.btn.primary', { timeout: 20000 });
    await v.click('#state button.btn.primary');
    await v.waitForFunction(() => document.querySelector('.locked-err') && /Wrong password/.test(document.querySelector('.locked-err').textContent), null, { timeout: 20000 });
    ok('Burn+pw: wrong password asks again', true);
    await v.fill('.pw-form input', 'rahasia123'); await v.click('.pw-form button');
    await v.waitForSelector('#state button.btn.primary', { timeout: 20000 });
    ok('Burn+pw: wrong password did not burn the note', true);
    await v.click('#state button.btn.primary');
    await v.waitForFunction(() => document.querySelector('#n-view') && document.querySelector('#n-view').textContent.includes('bom waktu'), null, { timeout: 15000 });
    ok('Burn+pw: content shown + burned banner', (await v.textContent('#banner')).includes('destroyed'));
    const v2 = await newPage(b); await v2.goto(visitorURL(admin));
    await v2.waitForSelector('main h1');
    const msg = await v2.textContent('main');
    ok('Burn+pw: second open is gone', /doesn't exist|can't be opened|not found/i.test(msg), msg.replace(/\s+/g, ' ').slice(0, 80));
  } catch (e) { ok('Burn flow', false, e.message.split('\n').slice(0,6).join(' ¦ ')); }

  // 5. account + dashboard
  try {
    const p = await newPage(b);
    await p.goto(BASE + '/register');
    await p.fill('#username', USER); await p.fill('#password', 'katasandi-panjang'); await p.fill('#password2', 'katasandi-panjang');
    await p.click('#auth-form button[type=submit]'); await p.waitForURL(/dashboard/, { timeout: 30000 });
    await create(p, { title: 'Catatan akun', body: 'milik penguji' });
    await p.goto(BASE + '/dashboard'); await p.waitForSelector('#mine .note-row', { timeout: 15000 });
    ok('Dashboard: shows decrypted title', (await p.textContent('#mine')).includes('Catatan akun'));
    // logout, login again
    await p.click('#menu > button'); await p.click('#logout'); await p.waitForURL(BASE + '/');
    await p.goto(BASE + '/login?next=/dashboard');
    await p.fill('#username', USER); await p.fill('#password', 'salah-salah-salah'); await p.click('#auth-form button[type=submit]');
    await p.waitForSelector('#auth-err:not(.hide)', { timeout: 30000 });
    ok('Login: wrong password message', (await p.textContent('#auth-err')).includes('Wrong'));
    await p.fill('#password', 'katasandi-panjang'); await p.click('#auth-form button[type=submit]');
    await p.waitForURL(/dashboard/, { timeout: 30000 }); await p.waitForSelector('#mine .note-row');
    ok('Dashboard: decrypts again after re-login', (await p.textContent('#mine')).includes('Catatan akun'));
    await p.click('#mine .note-row a.note-link'); await p.waitForSelector('#note:not(.hide)');
    ok('Dashboard: row opens note as admin', await p.isVisible('#act-share'));
    await p.goto(BASE + '/dashboard'); await p.waitForSelector('#mine .note-row');
    await p.click('#mine .note-row button[title=Delete]'); await p.waitForTimeout(800);
    await p.reload(); await p.waitForSelector('#mine li');
    ok('Dashboard: delete removes note', !(await p.textContent('#mine')).includes('Catatan akun'));
  } catch (e) { ok('Account flow', false, e.message.split('\n').slice(0,4).join(' ¦ ')); }

  // 6. QR + 7. freeze
  try {
    const p = await newPage(b);
    const admin = await create(p, { title: 'Rapat', body: 'versi satu', mode: 'edit' });
    await p.waitForSelector('#share-dlg[open]');
    await p.click('#link-list .link-row button[title="QR code"]');
    await p.waitForSelector('#qr-dlg[open] #qr-box img');
    const w = await p.$eval('#qr-box img', i => i.naturalWidth);
    ok('QR: image rendered', w > 100, 'width ' + w);
    // One popup for links and settings: one tab at a time keeps it one screen tall.
    const dh = await p.$eval('#share-dlg', d => d.getBoundingClientRect().height);
    ok('Share: links and settings fit one screen', dh <= 640, Math.round(dh) + 'px');
    await p.screenshot({ path: OUT + '/qr.png' });
    await p.keyboard.press('Escape'); await p.keyboard.press('Escape');
    const barH = await p.$eval('.note-bar', e => e.getBoundingClientRect().height);
    ok('Desktop: note controls on one line', barH < 60, Math.round(barH) + 'px');
    await p.click('#more-menu > button'); await p.click('#act-freeze');
    await p.waitForSelector('#snap-list li', { timeout: 10000 });
    const snapHref = await p.$eval('#snap-list a.link-url', a => a.href);
    await p.keyboard.press('Escape');
    await p.fill('.ed-src', 'versi dua'); await p.waitForTimeout(2500);
    const s = await newPage(b); await s.goto(snapHref.replace(/&a=[^&]*/, ''));
    await s.waitForSelector('#note:not(.hide)');
    const sv = await s.textContent('#n-view');
    ok('Freeze: snapshot keeps old text', sv.includes('versi satu') && !sv.includes('versi dua'), sv.slice(0, 60));
    ok('Freeze: snapshot is read-only with banner', (await s.isHidden('#n-edit')) && (await s.textContent('#banner')).includes('Frozen'));
    const o = await newPage(b); await o.goto(visitorURL(admin)); await o.waitForSelector('#note:not(.hide)');
    ok('Freeze: original has new text', (await o.$eval('.ed-src', t => t.value)) === 'versi dua');
  } catch (e) { ok('QR/freeze flow', false, e.message.split('\n')[0]); }

  // 7b. home page: quick note and the hand-over to /new
  try {
    const h = await newPage(b);
    const sent = [];
    h.on('request', r => { if (r.url().endsWith('/api/notes') && r.method() === 'POST') sent.push(r.postData()); });
    await h.goto(BASE + '/');
    ok('Home: quick note and full-notepad link', await h.isVisible('#quick-form') && (await h.getAttribute('a.h-pill:not(.dark)', 'href')) === '/new');
    await h.fill('#quick-title', 'Cepat'); await h.fill('#quick-body', 'rahasia kilat 123');
    await h.click('label:has(input[name=quick-mode][value=edit])');
    await h.click('#quick-create'); await h.waitForURL(/#k=/, { timeout: 20000 }); await h.waitForSelector('#note:not(.hide)');
    ok('Home: quick note opens as owner', await h.isVisible('#act-share'));
    ok('Home: only ciphertext sent', sent.length === 1 && !sent[0].includes('rahasia kilat'));
    await h.goto(BASE + '/'); await h.fill('#quick-body', 'lanjut di mode lengkap'); await h.click('#quick-full');
    await h.waitForURL(BASE + '/new'); await h.waitForTimeout(300);
    ok('Home: Full mode carries the draft to /new', (await h.inputValue('.ed-src')) === 'lanjut di mode lengkap');
  } catch (e) { ok('Home flow', false, e.message.split('\n')[0]); }

  // 8. mobile
  try {
    const m = await newPage(b, { viewport: { width: 375, height: 740 }, isMobile: true, hasTouch: true });
    for (const path of ['/', '/about', '/login', '/register']) {
      await m.goto(BASE + path); await m.waitForTimeout(300);
      const over = await m.evaluate(() => document.documentElement.scrollWidth - innerWidth);
      ok('Mobile: no horizontal scroll on ' + path, over <= 0, 'overflow ' + over + 'px');
      await m.screenshot({ path: OUT + '/m' + (path === '/' ? '-new' : path.replace('/', '-')) + '.png', fullPage: true });
    }
    const admin = await create(m, { title: 'HP', body: '# Judul\n\n- [ ] satu\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\n```js\nconsole.log("baris panjang sekali sekali sekali sekali sekali sekali")\n```', mode: 'check' });
    await m.keyboard.press('Escape');
    let over = await m.evaluate(() => document.documentElement.scrollWidth - innerWidth);
    ok('Mobile: no horizontal scroll on note (admin)', over <= 0, 'overflow ' + over + 'px');
    // Phones: actions sit in the dock; More opens the menu as a bottom sheet.
    const dock = await m.$$eval('#note-dock button:not([hidden])', x => x.map(y => y.dataset.for));
    ok('Mobile: dock with Share and More', dock.includes('act-share') && dock.includes('more-menu') && !(await m.isVisible('#act-share')), JSON.stringify(dock));
    await m.click('#note-dock [data-for=more-menu]');
    const sheet = await m.$eval('#more-menu .menu-list', l => { const r = l.getBoundingClientRect(); return getComputedStyle(l).position === 'fixed' && Math.abs(r.bottom - innerHeight) < 2 && r.height > 100; });
    ok('Mobile: More opens as a bottom sheet', sheet);
    await m.mouse.click(10, 10);
    await m.screenshot({ path: OUT + '/m-note-admin.png', fullPage: true });
    await m.goto(visitorURL(admin)); await m.waitForSelector('#note:not(.hide)');
    over = await m.evaluate(() => document.documentElement.scrollWidth - innerWidth);
    ok('Mobile: no horizontal scroll on note (visitor)', over <= 0, 'overflow ' + over + 'px');
    await m.screenshot({ path: OUT + '/m-note-visitor.png', fullPage: true });
    // Smallest common phone: the settings bar and Create link fit on one line.
    await m.setViewportSize({ width: 320, height: 700 }); await m.goto(BASE + '/new'); await m.waitForTimeout(300);
    const spill = await m.$$eval('.pick > summary, .share-bar #create', xs => xs.filter(s => s.getBoundingClientRect().right > innerWidth || s.scrollWidth > s.clientWidth + 1).map(s => s.textContent.trim()));
    ok('Mobile: 320px settings bar fits', spill.length === 0, JSON.stringify(spill));
  } catch (e) { ok('Mobile flow', false, e.message.split('\n')[0]); }

  await b.close();
  for (const r of results) console.log(r.join(' | '));
  // 401/403/404 from wrong passwords and burned notes are expected.
  console.log('console errors:', errs.filter(e => !/status of 40[134]/.test(e)));
  process.exit(results.some(r => r[0] === 'FAIL') ? 1 : 0);
})().catch(e => { console.error('FATAL', e); process.exit(1); });
