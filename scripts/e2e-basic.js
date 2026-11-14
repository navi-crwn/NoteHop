const { chromium } = require('playwright');
// My notes puts the basic notes first and needs no vault; the Vault button leads to the encrypted ones; Edit takes
// over a basic note whose owner link nobody here has; Delete works from the row; a new note from here is basic.
// Start a server on an empty database, then:
//   NOTEHOP_URL=http://127.0.0.1:8090 node scripts/e2e-basic.js
// (NOTEHOP_BROWSER=msedge uses the installed Edge instead of the bundled Chromium.)
const BASE = (process.env.NOTEHOP_URL || 'http://127.0.0.1:8090').replace(/\/$/, '');
const R = []; const ok = (n, c, i) => R.push((c ? 'PASS' : 'FAIL') + ' | ' + n + ' | ' + (i || ''));
const guard = async (name, f) => { try { await f(); } catch (e) { ok(name + ': could not run', false, e.message.split('\n')[0]); } };

(async () => {
  const b = await chromium.launch(process.env.NOTEHOP_BROWSER ? { channel: process.env.NOTEHOP_BROWSER } : {});
  const c = await b.newContext({ viewport: { width: 1440, height: 900 } });
  await c.addInitScript(() => { try { localStorage.setItem('nh_tour', '1'); } catch (e) {} });
  const p = await c.newPage();
  require('./e2e-dialogs')(p);
  const errs = []; p.on('pageerror', e => errs.push(e.message));
  const user = 'bas' + Date.now().toString(36);
  const slug = 'basic-' + Date.now().toString(36);
  let oldAdmin = '';

  await guard('setup', async () => {
    await p.goto(BASE + '/register?lang=en'); await p.fill('#username', user); await p.fill('#password', 'basic-password-1'); await p.fill('#password2', 'basic-password-1');
    await p.click('form button[type=submit]'); await p.waitForURL(/\/dashboard/, { timeout: 30000 });
    // a new note started from My notes is basic
    await p.click('.page-head a.btn.primary'); await p.waitForURL(/\/new\?kind=basic/);
    ok('a new note from My notes starts as a basic note', await p.$eval('input[name=kind][value=basic]', e => e.checked));
    await p.fill('#title', 'Shopping list'); await p.fill('.ed-src', 'Oat milk, coffee, lemons'); await p.fill('#slug', slug);
    await p.click('#create'); await p.waitForURL(/#.*a=/, { timeout: 30000, waitUntil: 'commit' }); await p.waitForTimeout(1200);
    oldAdmin = new URLSearchParams(p.url().split('#')[1]).get('a');
    // and an encrypted one
    await p.goto(BASE + '/new?lang=en'); await p.fill('#title', 'Secret plan'); await p.fill('.ed-src', 'hidden');
    await p.click('#create'); await p.waitForURL(/#k=/, { timeout: 30000, waitUntil: 'commit' }); await p.waitForTimeout(1200);
    ok('both notes were made', !!oldAdmin);
  });

  await guard('basic view, vault closed', async () => {
    // this browser knows nothing: no vault key, no remembered owner links
    await p.evaluate(() => { localStorage.removeItem('nh_vault'); localStorage.removeItem('nh_recent'); });
    await p.goto(BASE + '/dashboard?lang=en'); await p.waitForTimeout(1200);
    ok('the basic note is listed without the vault', (await p.locator('#mine .note-row', { hasText: 'Shopping list' }).count()) === 1);
    ok('the encrypted note is not in this list', (await p.locator('#mine .note-row', { hasText: 'Secret plan' }).count()) === 0 && (await p.locator('#mine .note-row').count()) === 1);
    ok('no "locked" state and no vault banner on the home view', (await p.locator('#vault-state, #vault-locked').count()) === 0 && !(await p.textContent('#mine')).match(/locked/i));
    ok('only the five filters that mean something for basic notes', (await p.locator('#groups [data-group]').count()) === 5);
    ok('the Vault button shows how many encrypted notes there are', /Vault\s*·\s*1/.test(await p.textContent('#vault-btn')), await p.textContent('#vault-btn'));
    ok('the row has Link, Edit and Delete', (await p.locator('#mine .note-row').first().locator('button:has-text("Link"), button:has-text("Edit"), button[aria-label]').count()) >= 3);
    ok('no sideways scroll', await p.evaluate(() => document.documentElement.scrollWidth <= innerWidth));
  });

  await guard('edit takes the note over', async () => {
    await p.click('#mine .note-row button:has-text("Edit")'); // the shared dialog is answered yes by the suite helper
    await p.waitForURL(/#.*a=/, { timeout: 20000, waitUntil: 'commit' }); await p.waitForTimeout(1500);
    const newAdmin = new URLSearchParams(p.url().split('#')[1]).get('a');
    ok('Edit opened the note as its owner with a new owner link', !!newAdmin && newAdmin !== oldAdmin);
    ok('the editor is there', await p.locator('.ed-src, [data-editor]').first().isVisible());
    const st = await p.evaluate(async (a) => { const r = await fetch(location.pathname.replace(/^\//, '/api/n/'), { headers: { 'X-Admin': a } }); return (await r.json()).admin; }, newAdmin);
    const old = await p.evaluate(async (a) => { const r = await fetch(location.pathname.replace(/^\//, '/api/n/'), { headers: { 'X-Admin': a } }); return (await r.json()).admin; }, oldAdmin);
    ok('the new key is the owner key and the old owner link no longer is', st === true && old !== true, `new ${st} old ${old}`);
    await p.goto(BASE + '/dashboard?lang=en'); await p.waitForTimeout(1000);
    ok('afterwards this device has the owner link: Edit is a plain link', (await p.locator('#mine .note-row a.btn:has-text("Edit")').count()) === 1);
  });

  await guard('phone', async () => {
    const pc = await b.newContext({ viewport: { width: 390, height: 844 }, storageState: await c.storageState() });
    const pp = await pc.newPage(); await pp.goto(BASE + '/dashboard?lang=de'); await pp.waitForTimeout(1200);
    ok('390px, German: no sideways scroll on the basic view', await pp.evaluate(() => document.documentElement.scrollWidth <= innerWidth));
    await pc.close();
  });

  await guard('vault view', async () => {
    await p.goto(BASE + '/dashboard?lang=en'); await p.click('#vault-btn'); await p.waitForURL(/view=vault/); await p.waitForTimeout(1200);
    ok('the Vault lists the encrypted note and not the basic one', (await p.locator('#mine .note-row', { hasText: 'Secret plan' }).count()) + (await p.locator('#mine .note-row', { hasText: 'Locked' }).count()) >= 1 && (await p.locator('#mine .note-row', { hasText: 'Shopping list' }).count()) === 0);
    ok('the vault state line is on this view', (await p.locator('#vault-state').count()) === 1);
    ok('the sidebar marks the Vault as the current page', (await p.locator('#shell-side a[aria-current=page][href="/dashboard?view=vault"]').count()) === 1);
  });

  await guard('delete', async () => {
    await p.goto(BASE + '/dashboard?lang=en'); await p.waitForTimeout(1000);
    await p.click('#mine .note-row button[aria-label]'); await p.waitForTimeout(1200);
    ok('Delete works from the row, without the vault', (await p.locator('#mine .note-row', { hasText: 'Shopping list' }).count()) === 0);
    const r = await p.goto(BASE + '/' + slug); ok('its link is gone', r.status() === 404 || r.status() === 410, String(r.status()));
    ok('no script errors', errs.length === 0, errs.join(' | '));
  });

  await b.close();
  console.log(R.join('\n'));
  const fails = R.filter(r => r.startsWith('FAIL')).length;
  console.log(`\n${R.length - fails} passed, ${fails} failed`);
  process.exit(fails ? 1 : 0);
})();
