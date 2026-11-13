const { chromium } = require('playwright');
// The top bar on narrow screens (360, 375 and 390 px), as a guest and as a member, in English and the
// two longest languages (German, Russian): nothing overlaps, nothing slides sideways, the bar stays one
// row, every drop-down opens whole inside the screen and the open burger list is not covered by anything.
// Also the pairing code on /nearby: all six digits are visible.
// Start a server on an empty database, then:
//   NOTEHOP_URL=http://127.0.0.1:8090 node scripts/e2e-topbar.js
// (NOTEHOP_BROWSER=msedge uses the installed Edge instead of the bundled Chromium.)
const BASE = (process.env.NOTEHOP_URL || 'http://127.0.0.1:8090').replace(/\/$/, '');
const R = []; const ok = (n, c, i) => R.push((c ? 'PASS' : 'FAIL') + ' | ' + n + ' | ' + (i || ''));
const guard = async (name, f) => { try { await f(); } catch (e) { ok(name + ': could not run', false, e.message.split('\n')[0]); } };

const GUEST_PAGES = ['/', '/new', '/nearby', '/diff', '/about', '/faq', '/features', '/changelog', '/contact', '/status',
  '/terms', '/privacy', '/login', '/register', '/recover'];
const MEMBER_PAGES = ['/dashboard', '/new', '/security', '/nearby', '/diff', '/about', '/faq', '/changelog', '/contact', '/status'];
const WIDTHS = [360, 375, 390];
const LANGS = ['en', 'de', 'ru'];

// What the bar looks like to the eye: the boxes of its visible children, in the page's own pixels.
const measure = () => {
  const bar = document.querySelector('header.nav, .shell-top');
  const r = e => { const b = e.getBoundingClientRect(); return { l: b.left, r: b.right, t: b.top, b: b.bottom }; };
  const vis = e => { const s = getComputedStyle(e), b = e.getBoundingClientRect(); return s.display !== 'none' && s.visibility !== 'hidden' && b.width > 0 && b.height > 0; };
  const row = bar.querySelector('.wrap, .shell-top-in') || bar;
  const kids = [];
  const take = (e, name) => { if (vis(e)) kids.push({ name, ...r(e) }); };
  for (const c of row.children) {
    if (c.matches('nav.nav-links')) continue; // the burger list is dropped down, checked separately
    if (c.matches('.nav-r, .shell-tools')) { for (const g of c.children) take(g, g.id || g.className.toString().split(' ')[0] || g.tagName); } else take(c, c.id || c.className.toString().split(' ')[0] || c.tagName);
  }
  const hit = [];
  for (let i = 0; i < kids.length; i++) for (let j = i + 1; j < kids.length; j++) {
    const a = kids[i], b = kids[j];
    if (a.l < b.r - 1 && b.l < a.r - 1 && a.t < b.b - 1 && b.t < a.b - 1) hit.push(a.name + ' x ' + b.name);
  }
  const bb = bar.getBoundingClientRect();
  return {
    sw: document.documentElement.scrollWidth, iw: innerWidth, barH: Math.round(bb.height),
    overlaps: hit, outside: kids.filter(k => k.l < -0.5 || k.r > innerWidth + 0.5).map(k => k.name),
    below: kids.filter(k => k.b > bb.bottom + 1).map(k => k.name)
  };
};
// An opened drop-down must lie whole inside the screen.
const inside = (p, sel) => p.evaluate(s => { const e = document.querySelector(s); if (!e) return 'missing'; const b = e.getBoundingClientRect(); return (b.left >= -0.5 && b.right <= innerWidth + 0.5 && b.top >= 0 && b.bottom <= innerHeight + 0.5) ? true : JSON.stringify({ l: Math.round(b.left), r: Math.round(b.right), t: Math.round(b.top), b: Math.round(b.bottom) }); }, sel);

async function page(b, w, lang, cookie) {
  const c = await b.newContext({ viewport: { width: w, height: 800 }, hasTouch: true });
  await c.addInitScript(() => { try { localStorage.setItem('nh_tour', '1'); } catch (e) {} });
  if (cookie) await c.addCookies([cookie]);
  const p = await c.newPage();
  p.on('dialog', d => d.dismiss().catch(() => {}));
  return p;
}

(async () => {
  const b = await chromium.launch(process.env.NOTEHOP_BROWSER ? { channel: process.env.NOTEHOP_BROWSER } : {});

  // ---------- guests ----------
  for (const w of WIDTHS) for (const lang of LANGS) await guard(`guest ${w} ${lang}`, async () => {
    const p = await page(b, w, lang);
    const bad = [];
    for (const path of GUEST_PAGES) {
      await p.goto(BASE + path + (path.includes('?') ? '&' : '?') + 'lang=' + lang); await p.waitForTimeout(150);
      const m = await p.evaluate(measure);
      if (m.sw > m.iw) bad.push(`${path}: slides sideways (${m.sw} > ${m.iw})`);
      if (m.barH > 62) bad.push(`${path}: bar is ${m.barH}px tall`);
      if (m.overlaps.length) bad.push(`${path}: overlap ${m.overlaps.join(', ')}`);
      if (m.outside.length) bad.push(`${path}: outside the screen ${m.outside.join(', ')}`);
      if (m.below.length) bad.push(`${path}: below the bar ${m.below.join(', ')}`);
    }
    ok(`guest ${w}px ${lang}: ${GUEST_PAGES.length} pages, one row, no overlap, no sideways scroll`, bad.length === 0, bad.slice(0, 4).join(' || '));
    // the three drop-downs and the burger list, on one page
    await p.goto(BASE + '/new?lang=' + lang); await p.waitForTimeout(200);
    await p.click('#lang-menu > button'); await p.waitForTimeout(100);
    ok(`guest ${w}px ${lang}: language menu opens inside the screen`, (await inside(p, '#lang-menu .menu-list')) === true, String(await inside(p, '#lang-menu .menu-list')));
    await p.click('#lang-menu > button'); await p.waitForTimeout(100);
    await p.click('.family-switcher > summary'); await p.waitForTimeout(100);
    ok(`guest ${w}px ${lang}: family menu opens inside the screen`, (await inside(p, '.family-switcher nav')) === true, String(await inside(p, '.family-switcher nav')));
    await p.click('.family-switcher > summary'); await p.waitForTimeout(100);
    await p.click('#burger'); await p.waitForTimeout(150);
    const covered = await p.evaluate(() => [...document.querySelectorAll('.nav-links a')].filter(a => getComputedStyle(a).display !== 'none').map(a => {
      const r = a.getBoundingClientRect(), t = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2);
      return (t === a || a.contains(t)) ? null : (a.textContent.trim() + ' under ' + (t && (t.id || t.className)));
    }).filter(Boolean));
    ok(`guest ${w}px ${lang}: the open burger list is covered by nothing`, covered.length === 0, covered.join(', '));
    ok(`guest ${w}px ${lang}: the open burger list is inside the screen`, (await inside(p, '.nav-links')) === true, String(await inside(p, '.nav-links')));
    ok(`guest ${w}px ${lang}: the bar did not grow when the burger opened`, (await p.evaluate(measure)).barH <= 62);
    await p.context().close();
  });

  // ---------- members ----------
  const user = 'tb' + Date.now().toString(36);
  const reg = await page(b, 1280, 'en');
  await reg.goto(BASE + '/register'); await reg.fill('#username', user); await reg.fill('#password', 'tb-password-1'); await reg.fill('#password2', 'tb-password-1');
  await reg.click('form button[type=submit]'); await reg.waitForURL(/\/dashboard/, { timeout: 30000 }).catch(() => {});
  const session = (await reg.context().cookies()).find(c => c.name === 'nh_session');
  ok('a member signed in for the narrow checks', !!session);
  if (session) {
    for (const w of WIDTHS) for (const lang of LANGS) await guard(`member ${w} ${lang}`, async () => {
      const p = await page(b, w, lang, { name: session.name, value: session.value, url: BASE });
      const bad = [];
      for (const path of MEMBER_PAGES) {
        await p.goto(BASE + path + '?lang=' + lang); await p.waitForTimeout(150);
        const m = await p.evaluate(measure);
        if (m.sw > m.iw) bad.push(`${path}: slides sideways (${m.sw} > ${m.iw})`);
        if (m.barH > 62) bad.push(`${path}: bar is ${m.barH}px tall`);
        if (m.overlaps.length) bad.push(`${path}: overlap ${m.overlaps.join(', ')}`);
        if (m.outside.length) bad.push(`${path}: outside the screen ${m.outside.join(', ')}`);
      }
      ok(`member ${w}px ${lang}: ${MEMBER_PAGES.length} pages, one row, no overlap, no sideways scroll`, bad.length === 0, bad.slice(0, 4).join(' || '));
      await p.goto(BASE + '/dashboard?lang=' + lang); await p.waitForTimeout(200);
      for (const [name, sel, list] of [['language menu', '.shell-top .lang-menu > button, #lang-menu > button', '.shell-top .lang-menu .menu-list, #lang-menu .menu-list'], ['account menu', '.shell-top #menu > button, #menu > button', '.shell-top #menu .menu-list, #menu .menu-list']]) {
        const btn = await p.$(sel);
        if (!btn) { ok(`member ${w}px ${lang}: ${name} found`, false); continue; }
        await btn.click(); await p.waitForTimeout(100);
        ok(`member ${w}px ${lang}: ${name} opens inside the screen`, (await inside(p, list)) === true, String(await inside(p, list)));
        await btn.click(); await p.waitForTimeout(100);
      }
      await p.context().close();
    });
  }

  // ---------- /nearby: the whole pairing code is visible ----------
  for (const w of [1440, 390, 360]) await guard('nearby ' + w, async () => {
    const p = await page(b, w, 'en');
    await p.goto(BASE + '/nearby?lang=en'); await p.waitForTimeout(300);
    await p.fill('#nb-code', '739814');
    const m = await p.$eval('#nb-code', i => ({ sw: i.scrollWidth, cw: i.clientWidth, v: i.value }));
    ok(`nearby ${w}px: six digits fit in the code field`, m.sw <= m.cw && m.v === '739814', JSON.stringify(m));
    const room = await p.evaluate(() => document.documentElement.scrollWidth <= innerWidth);
    ok(`nearby ${w}px: no sideways scroll`, room);
    await p.context().close();
  });

  await b.close();
  console.log(R.join('\n'));
  const fails = R.filter(r => r.startsWith('FAIL')).length;
  console.log(`\n${R.length - fails} passed, ${fails} failed`);
  process.exit(fails ? 1 : 0);
})();
