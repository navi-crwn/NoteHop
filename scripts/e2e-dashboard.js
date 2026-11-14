const { chromium } = require('playwright');
const crypto = require('crypto');
// The signed-in frame: the sidebar's pages stay inside it, "Part of Hop Easy Link" is the last line of the
// sidebar (once, not again in the footer), the notice card sits above the dashboard home and stays closed for a
// week once closed.
// Start a server on an empty database, then:
//   NOTEHOP_URL=http://127.0.0.1:8090 node scripts/e2e-dashboard.js
// The notice checks need the control center's secret (the server must run with NOTEHOP_SSO_URL, NOTEHOP_BASE_URL,
// NOTEHOP_SSO_SECRET and NOTEHOP_HOP_CLIENTS=imperium); without NOTEHOP_HOP_SECRET they are skipped.
// (NOTEHOP_BROWSER=msedge uses the installed Edge instead of the bundled Chromium.)
const BASE = (process.env.NOTEHOP_URL || 'http://127.0.0.1:8090').replace(/\/$/, '');
const R = []; const ok = (n, c, i) => R.push((c ? 'PASS' : 'FAIL') + ' | ' + n + ' | ' + (i || ''));
const guard = async (name, f) => { try { await f(); } catch (e) { ok(name + ': could not run', false, e.message.split('\n')[0]); } };

async function setNotice(text) {
  const body = JSON.stringify({ announcement: text }), P = '/hop/settings', M = 'PUT';
  const ts = String(Math.floor(Date.now() / 1000)), nonce = crypto.randomBytes(16).toString('base64url');
  const base = ['hop1', 'imperium', ts, nonce, M, P, crypto.createHash('sha256').update(body).digest('hex')].join('|');
  const sig = crypto.createHmac('sha256', process.env.NOTEHOP_HOP_SECRET).update(base).digest('base64url');
  const r = await fetch(BASE + P, { method: M, body, headers: { 'X-HOP-App': 'imperium', 'X-HOP-Timestamp': ts, 'X-HOP-Nonce': nonce, 'X-HOP-Actor': 'e2e', 'X-HOP-Signature': sig, 'Content-Type': 'application/json' } });
  return r.status;
}

(async () => {
  const b = await chromium.launch(process.env.NOTEHOP_BROWSER ? { channel: process.env.NOTEHOP_BROWSER } : {});
  const user = 'dash' + Date.now().toString(36);
  const open = async (w, h, scheme) => {
    const c = await b.newContext({ viewport: { width: w, height: h }, colorScheme: scheme || 'light' });
    await c.addInitScript(() => { try { localStorage.setItem('nh_tour', '1'); } catch (e) {} });
    return c;
  };
  const ctx0 = await open(1440, 900);
  const p0 = await ctx0.newPage();
  await p0.goto(BASE + '/register?lang=en'); await p0.fill('#username', user); await p0.fill('#password', 'dash-password-1'); await p0.fill('#password2', 'dash-password-1');
  await p0.click('form button[type=submit]'); await p0.waitForURL(/\/dashboard/, { timeout: 30000 });
  const state = await ctx0.storageState();
  await ctx0.close();

  // ---------- K-1: "Part of Hop Easy Link" is the last line of the sidebar ----------
  for (const [w, h, label, drawer] of [[1440, 900, 'desktop', false], [390, 844, 'phone, drawer open', true], [1440, 420, 'desktop, short window', false]]) {
    await guard('part of, ' + label, async () => {
      const c = await b.newContext({ viewport: { width: w, height: h }, storageState: state });
      await c.addInitScript(() => { try { localStorage.setItem('nh_tour', '1'); } catch (e) {} });
      const p = await c.newPage();
      await p.goto(BASE + '/dashboard?lang=en'); await p.waitForTimeout(400);
      if (drawer) { await p.click('#shell-burger'); await p.waitForTimeout(450); }
      const side = p.locator('#shell-side');
      ok(label + ': exactly one "Part of" line on the screen', (await p.locator('.part-of').count()) === 1);
      ok(label + ': it is inside the sidebar, not the footer', (await p.locator('#shell-side .part-of').count()) === 1 && (await p.locator('.shell-foot .part-of').count()) === 0);
      // the sidebar scrolls inside itself when it is taller than the window: the line is at the end of that scroll
      await side.evaluate(e => { e.scrollTop = e.scrollHeight; });
      const m = await p.evaluate(() => {
        const s = document.getElementById('shell-side').getBoundingClientRect(), l = document.querySelector('#shell-side .part-of').getBoundingClientRect();
        const items = [...document.querySelectorAll('#shell-side .shell-nav a, #shell-side .shell-side-foot p')].map(e => e.getBoundingClientRect());
        const first = document.querySelector('#shell-side .shell-nav a').getBoundingClientRect();
        return { inside: l.left >= s.left && l.right <= s.right && l.top >= s.top && l.bottom <= s.bottom + 1, gap: Math.round(s.bottom - l.bottom), below: items.every(r => r.bottom <= l.top + 1), leftAligned: Math.abs(l.left - (first.left + 12)) <= 14 };
      });
      ok(label + ': its box is inside the sidebar box', m.inside, JSON.stringify(m));
      ok(label + ': it ends within 24px of the sidebar bottom', m.gap >= 0 && m.gap <= 24, 'gap ' + m.gap);
      ok(label + ': it is below every menu entry and the foot links', m.below);
      ok(label + ': it lines up with the menu entries on the left', m.leftAligned, JSON.stringify(m));
      ok(label + ': the page does not slide sideways', await p.evaluate(() => document.documentElement.scrollWidth <= innerWidth));
      await c.close();
    });
  }

  // ---------- N-5: the sidebar's pages stay in the frame; the ways back go to the dashboard ----------
  await guard('frame', async () => {
    const c = await b.newContext({ viewport: { width: 1440, height: 900 }, storageState: state });
    await c.addInitScript(() => { try { localStorage.setItem('nh_tour', '1'); } catch (e) {} });
    const p = await c.newPage();
    const errs = []; p.on('pageerror', e => errs.push(e.message));
    for (const path of ['/nearby', '/diff', '/about', '/faq']) {
      await p.goto(BASE + '/dashboard?lang=en'); await p.waitForTimeout(150);
      await p.click(`#shell-side .shell-nav a[href="${path}"]`); await p.waitForURL('**' + path + '*'); await p.waitForTimeout(250);
      ok(path + ' opened from the sidebar stays in the frame', (await p.locator('#shell-side').count()) === 1 && (await p.locator('header.nav').count()) === 0);
      ok(path + ': its entry is lit and there is no way back to the home page', (await p.locator(`#shell-side a[href="${path}"][aria-current=page]`).count()) === 1 && (await p.locator('a.back').count()) === 0);
      await p.click('#shell-side .shell-brand'); await p.waitForURL(/\/dashboard/);
    }
    for (const path of ['/changelog', '/terms', '/privacy', '/contact', '/status']) {
      await p.goto(BASE + path + '?lang=en'); await p.waitForTimeout(150);
      ok(path + ': the way back and the logo go to the dashboard', (await p.getAttribute('a.back', 'href')) === '/dashboard' && (await p.getAttribute('header.nav a.brand', 'href')) === '/dashboard');
    }
    await p.goto(BASE + '/no-such-page-at-all?lang=en');
    ok('a missing page: its button does not send a member to the public home page', (await p.locator('.gone a.btn.primary').first().getAttribute('href')) !== '/');
    ok('no script errors on the way', errs.length === 0, errs.join(' | '));
    await c.close();
    // a guest keeps the public pages
    const g = await b.newContext({ viewport: { width: 1440, height: 900 } });
    const gp = await g.newPage();
    await gp.goto(BASE + '/nearby?lang=en');
    ok('a guest keeps the public /nearby with its way back home', (await gp.locator('#shell-side').count()) === 0 && (await gp.getAttribute('a.back', 'href')) === '/' && (await gp.getAttribute('header.nav a.brand', 'href')) === '/');
    await g.close();
  });

  // ---------- K-2: the notice card ----------
  if (!process.env.NOTEHOP_HOP_SECRET) {
    ok('notice card checks skipped: NOTEHOP_HOP_SECRET is not set', true, 'skipped');
  } else {
    await guard('notice', async () => {
      const text = 'Maintenance on Sunday 02:00-03:00 UTC. Saving may pause for a few minutes. ' + Date.now();
      ok('the control center set a notice', (await setNotice(text)) === 200);
      for (const [w, h] of [[1440, 900], [390, 844]]) {
        const c = await b.newContext({ viewport: { width: w, height: h }, storageState: state });
        await c.addInitScript(() => { try { localStorage.setItem('nh_tour', '1'); } catch (e) {} });
        const p = await c.newPage();
        await p.goto(BASE + '/dashboard?lang=en'); await p.waitForTimeout(400);
        const card = p.locator('#announce');
        ok(`${w}px: the card is shown with the text`, (await card.isVisible()) && (await card.textContent()).includes('Maintenance on Sunday'));
        const m = await p.evaluate(() => {
          const c = document.getElementById('announce').getBoundingClientRect(), h = document.querySelector('.page-head').getBoundingClientRect(), main = document.querySelector('.page-head').getBoundingClientRect();
          return { above: c.bottom <= h.top + 1, wide: c.width >= main.width - 2, fixed: getComputedStyle(document.getElementById('announce')).position };
        });
        ok(`${w}px: it is above the page heading, as wide as the content, and does not stick`, m.above && m.wide && m.fixed === 'static', JSON.stringify(m));
        ok(`${w}px: the page does not slide sideways`, await p.evaluate(() => document.documentElement.scrollWidth <= innerWidth));
        ok(`${w}px: the close button has a label`, !!(await p.getAttribute('#announce-close', 'aria-label')));
        for (const path of ['/new', '/security']) { await p.goto(BASE + path + '?lang=en'); ok(`${w}px: ${path} has no card`, (await p.locator('#announce').count()) === 0); }
        await p.goto(BASE + '/dashboard?lang=en'); await p.waitForTimeout(300);
        await p.click('#announce-close'); await p.waitForTimeout(150);
        ok(`${w}px: closing hides it`, !(await card.isVisible()));
        await p.reload(); await p.waitForTimeout(400);
        ok(`${w}px: after a reload it stays closed`, !(await p.locator('#announce').isVisible()));
        // a different notice is a new notice
        await setNotice(text + ' (second)');
        await p.reload(); await p.waitForTimeout(400);
        ok(`${w}px: a new notice is shown again`, await p.locator('#announce').isVisible());
        await setNotice(text);
        // seven days later it comes back
        await p.evaluate(() => { for (const k of Object.keys(localStorage)) if (k.startsWith('nh_announce_')) localStorage.setItem(k, String(Date.now() - 8 * 86400000)); });
        await p.reload(); await p.waitForTimeout(400);
        ok(`${w}px: after a week it is shown again`, await p.locator('#announce').isVisible());
        await c.close();
      }
      ok('the control center removed the notice', (await setNotice('')) === 200);
      const c = await b.newContext({ viewport: { width: 1440, height: 900 }, storageState: state });
      const p = await c.newPage();
      await p.goto(BASE + '/dashboard?lang=en');
      ok('with no notice there is no card at all', (await p.locator('#announce').count()) === 0);
      await c.close();
    });
  }

  await b.close();
  console.log(R.join('\n'));
  const fails = R.filter(r => r.startsWith('FAIL')).length;
  console.log(`\n${R.length - fails} passed, ${fails} failed`);
  process.exit(fails ? 1 : 0);
})();
