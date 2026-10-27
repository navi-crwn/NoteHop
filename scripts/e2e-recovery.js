const { chromium } = require('playwright');
// Browser checks for recovery codes, password change and "Forgot password".
// The point: after a reset with a code, the old notes still open from the
// dashboard (the vault key survived), and the server never saw it.
// Start a server on an empty database, then:
//   NOTEHOP_URL=http://127.0.0.1:8090 node scripts/e2e-recovery.js
const BASE = (process.env.NOTEHOP_URL || 'http://127.0.0.1:8090').replace(/\/$/, '');
const USER = 'r_' + Date.now().toString(36);
const R = []; const ok = (n, c, i) => R.push((c ? 'PASS' : 'FAIL') + ' | ' + n + ' | ' + (i || ''));

(async () => {
  const b = await chromium.launch(); const errs = [];
  const newCtx = async () => {
    const c = await b.newContext({ acceptDownloads: true, permissions: ['clipboard-read', 'clipboard-write'] });
    await c.addInitScript(() => { try { localStorage.setItem('nh_tour', '1'); } catch (e) {} });
    return c;
  };
  const watch = p => { p.on('pageerror', e => errs.push(e.message)); p.on('console', m => { if (m.type() === 'error' && !/40[0-9]|429/.test(m.text())) errs.push('console: ' + m.text()); }); return p; };
  const ctx = await newCtx(); const p = watch(await ctx.newPage());
  const requests = []; p.on('request', r => { if (r.method() === 'POST') requests.push(r.postData() || ''); });

  // account + one note in it
  await p.goto(BASE + '/register'); await p.fill('#username', USER); await p.fill('#password', 'password-pertama'); await p.fill('#password2', 'password-pertama');
  await p.click('#auth-form button[type=submit]'); await p.waitForURL(/dashboard/, { timeout: 30000 });
  await p.goto(BASE + '/new'); await p.fill('#title', 'Rahasia lama'); await p.fill('.ed-src', 'isi lama');
  await p.click('#create'); await p.waitForURL(/\/n\//); await p.keyboard.press('Escape');
  const vaultBefore = await p.evaluate(() => JSON.parse(localStorage.getItem('nh_vault')).k);

  // ---------- security page: make codes ----------
  await p.goto(BASE + '/security');
  await p.waitForFunction(() => document.getElementById('rec-state').textContent.trim() !== '');
  ok('no codes yet', (await p.textContent('#rec-state')).includes('Not made'));
  await p.click('#rec-make'); await p.fill('#confirm-secret', 'salah-password-nih');
  await p.click('#confirm-form button[type=submit]'); await p.waitForSelector('#confirm-err:not(.hide)', { timeout: 30000 });
  ok('wrong password refused', (await p.textContent('#confirm-err')).includes('Wrong password'));
  await p.fill('#confirm-secret', 'password-pertama'); await p.click('#confirm-form button[type=submit]');
  await p.waitForSelector('#codes-box:not(.hide)', { timeout: 30000 });
  const codes = await p.$$eval('#code-grid li', xs => xs.map(x => x.textContent));
  ok('8 codes shown', codes.length === 8 && codes.every(c => /^[A-Z2-9]{4}(-[A-Z2-9]{4}){3}$/.test(c)), codes[0]);
  ok('codes are all different', new Set(codes).size === 8);
  ok('state says 8 of 8', (await p.textContent('#rec-state')).includes('8 of 8'));
  ok('no code ever sent to the server', !requests.some(d => codes.some(c => d.includes(c) || d.includes(c.replace(/-/g, '')))));
  ok('vault key never sent', !requests.some(d => d.includes(vaultBefore)));
  const [dl] = await Promise.all([p.waitForEvent('download'), p.click('#codes-dl')]);
  const txt = require('fs').readFileSync(await dl.path(), 'utf8');
  ok('download has every code', dl.suggestedFilename() === 'notehop-recovery-' + USER + '.txt' && codes.every(c => txt.includes(c)), dl.suggestedFilename());
  ok('Done waits for "I saved them"', await p.isDisabled('#codes-done'));
  await p.check('#codes-saved'); await p.click('#codes-done');
  ok('codes leave the page', await p.isHidden('#codes-box') && (await p.$$('#code-grid li')).length === 0);

  // ---------- forgot password, in a fresh browser ----------
  const ctx2 = await newCtx(); const q = watch(await ctx2.newPage());
  await q.goto(BASE + '/login'); await q.click('a[href="/recover"]'); await q.waitForURL(/\/recover/);
  await q.fill('#username', USER); await q.fill('#code', 'AAAA-BBBB-CCCC-DDDD'); await q.click('#rec-form button[type=submit]');
  await q.waitForSelector('#rec-err:not(.hide)');
  ok('wrong code refused', (await q.textContent('#rec-err')).includes("doesn't work"));
  await q.fill('#code', codes[2].toLowerCase().replace(/-/g, ' ')); // typed loosely
  await q.click('#rec-form button[type=submit]'); await q.waitForSelector('#rec-step2:not(.hide)');
  ok('code accepted (case and spaces ignored)', (await q.textContent('#rec-ok')).includes('@' + USER));
  await q.fill('#new1', 'password-kedua!'); await q.fill('#new2', 'password-beda'); await q.click('#rec-form button[type=submit]');
  await q.waitForSelector('#rec-err:not(.hide)');
  ok('mismatch caught', await q.isVisible('#rec-err'));
  await q.fill('#new2', 'password-kedua!'); await q.click('#rec-form button[type=submit]');
  await q.waitForSelector('#rec-done:not(.hide)', { timeout: 30000 });
  ok('done says 7 left', (await q.textContent('#rec-done-p')).includes('7'));
  ok('same vault key after reset', (await q.evaluate(() => JSON.parse(localStorage.getItem('nh_vault')).k)) === vaultBefore);
  await q.click('#rec-done a[href="/dashboard"]'); await q.waitForSelector('#mine .note-row', { timeout: 15000 });
  ok('old note still opens from the dashboard', (await q.textContent('#mine')).includes('Rahasia lama'));

  // the first browser was signed out; old password no longer works, new one does
  await p.goto(BASE + '/dashboard'); ok('other browser signed out', /\/login/.test(p.url()));
  const ctx3 = await newCtx(); const s = watch(await ctx3.newPage());
  await s.goto(BASE + '/login'); await s.fill('#username', USER); await s.fill('#password', 'password-pertama'); await s.click('#auth-form button[type=submit]');
  await s.waitForSelector('#auth-err:not(.hide)', { timeout: 30000 }); ok('old password refused', true);
  await s.fill('#password', 'password-kedua!'); await s.click('#auth-form button[type=submit]'); await s.waitForURL(/dashboard/, { timeout: 30000 });
  await s.waitForSelector('#mine .note-row', { timeout: 15000 });
  ok('new password unlocks the old note', (await s.textContent('#mine')).includes('Rahasia lama') && (await s.evaluate(() => JSON.parse(localStorage.getItem('nh_vault')).k)) === vaultBefore);
  // a used code is spent
  const ctx4 = await newCtx(); const u = watch(await ctx4.newPage());
  await u.goto(BASE + '/recover'); await u.fill('#username', USER); await u.fill('#code', codes[2]); await u.click('#rec-form button[type=submit]');
  await u.waitForSelector('#rec-err:not(.hide)'); ok('used code refused', await u.isVisible('#rec-err'));

  // ---------- change password on the security page ----------
  await s.goto(BASE + '/security'); await s.waitForFunction(() => document.getElementById('rec-state').textContent.includes('7 of 8'));
  ok('state says 7 of 8', true);
  await s.fill('#cur', 'password-kedua!'); await s.fill('#new1', 'password-ketiga'); await s.fill('#new2', 'password-ketiga');
  await s.click('#secret-form button[type=submit]');
  await s.waitForFunction(() => !document.querySelector('#secret-form button[type=submit]').disabled && document.getElementById('cur').value === '', null, { timeout: 30000 });
  ok('password changed, form cleared', await s.isHidden('#secret-err'));
  await q.goto(BASE + '/dashboard'); ok('changing signs other browsers out', /\/login/.test(q.url()));
  await q.fill('#username', USER); await q.fill('#password', 'password-ketiga'); await q.click('#auth-form button[type=submit]');
  await q.waitForSelector('#mine .note-row', { timeout: 30000 });
  ok('third password, same notes', (await q.textContent('#mine')).includes('Rahasia lama'));
  // a remaining code still works after the password change
  await u.fill('#code', codes[5]); await u.click('#rec-form button[type=submit]'); await u.waitForSelector('#rec-step2:not(.hide)');
  ok('codes survive a password change', true);
  // nav and page links
  ok('account menu links to Security', !!(await q.$('.menu-list a[href="/security"]')));
  await q.goto(BASE + '/security'); await q.click('#rec-make'); await q.fill('#confirm-secret', 'password-ketiga'); await q.click('#confirm-form button[type=submit]');
  await q.waitForSelector('#codes-box:not(.hide)', { timeout: 30000 });
  const fresh = await q.$$eval('#code-grid li', xs => xs.map(x => x.textContent));
  await u.goto(BASE + '/recover'); await u.fill('#username', USER); await u.fill('#code', codes[6]); await u.click('#rec-form button[type=submit]');
  await u.waitForSelector('#rec-err:not(.hide)'); ok('new codes replace the old ones', fresh.length === 8 && !fresh.includes(codes[6]));

  console.log(R.join('\n')); console.log('errors:', errs); await b.close();
  process.exit(R.some(r => r.startsWith('FAIL')) || errs.length ? 1 : 0);
})().catch(e => { console.log(R.join('\n')); console.error('FATAL', e.message.split('\n').slice(0, 5).join(' | ')); process.exit(1); });
