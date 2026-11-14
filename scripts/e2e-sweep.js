const { chromium } = require('playwright');
// Browser checks for the problems found while going through every page at phone, tablet and
// desktop sizes: the "More settings" panel, console errors, text that does not wrap, controls
// that are too small or have no focus ring, low contrast, bars that cover the footer, and
// text that was never translated.
// Start a server on an empty database, then:
//   NOTEHOP_URL=http://127.0.0.1:8090 node scripts/e2e-sweep.js
const BASE = (process.env.NOTEHOP_URL || 'http://127.0.0.1:8090').replace(/\/$/, '');
const R = []; const ok = (n, c, i) => R.push((c ? 'PASS' : 'FAIL') + ' | ' + n + ' | ' + (i || ''));
const errs = [];
// A section that cannot run (a missing control, say) is a failed check, not a crash.
const guard = async (name, f) => { try { await f(); } catch (e) { ok(name + ': could not run', false, e.message.split('\n')[0]); } };

async function open(b, opts) {
  const c = await b.newContext(Object.assign({ viewport: { width: 1440, height: 900 } }, opts || {}));
  await c.addInitScript(() => { try { localStorage.setItem('nh_tour', '1'); } catch (e) {} });
  const p = await c.newPage();
  p.on('pageerror', e => errs.push(e.message));
  p.on('console', m => { if (m.type() === 'error' && !/status of (40[134]|503)|net::ERR_/.test(m.text())) errs.push(p.url() + ' :: ' + m.text()); });
  require('./e2e-dialogs')(p);
  return p;
}
const contrast = (p, sel, prop, bgSel) => p.evaluate(([sel, prop, bgSel]) => {
  const rgb = s => (s.match(/[\d.]+/g) || []).slice(0, 3).map(Number);
  const lum = c => { const [r, g, b] = c.map(v => { v /= 255; return v <= .03928 ? v / 12.92 : Math.pow((v + .055) / 1.055, 2.4); }); return .2126 * r + .7152 * g + .0722 * b; };
  const fg = rgb(getComputedStyle(document.querySelector(sel))[prop]), bg = rgb(getComputedStyle(document.querySelector(bgSel)).backgroundColor);
  const a = lum(fg), c = lum(bg); return (Math.max(a, c) + .05) / (Math.min(a, c) + .05);
}, [sel, prop, bgSel]);
async function create(p, { title, body, basic, slug }) {
  await p.goto(BASE + '/new?lang=en');
  await p.fill('#title', title); await p.fill('.ed-src', body);
  if (basic) { await p.click('#opt-kind > summary'); await p.click('label:has(input[name=kind][value=basic])'); await p.fill('#slug', slug); }
  await p.click('#create'); await p.waitForURL(/#k=/, { timeout: 20000, waitUntil: 'commit' }); await p.waitForTimeout(1500);
  const u = new URL(p.url()); return { owner: p.url(), view: BASE + u.pathname + '#k=' + u.hash.match(/k=([^&]+)/)[1] };
}

(async () => {
  const b = await chromium.launch(process.env.NOTEHOP_BROWSER ? { channel: process.env.NOTEHOP_BROWSER } : {});

  // ---------- "More settings": one dialog, all the groups in it, it scrolls inside ----------
  for (const [name, vp, touch] of [['desktop 1280x640', { width: 1280, height: 640 }, false], ['phone 360x640', { width: 360, height: 640 }, true]]) {
    await guard('More settings, ' + name, async () => {
    const p = await open(b, { viewport: vp, hasTouch: touch });
    await p.goto(BASE + '/new?lang=de'); await p.waitForTimeout(300);
    await p.click('#adv-btn'); await p.waitForTimeout(250);
    ok(name + ': More is a modal dialog', await p.$eval('#adv', d => d.open && d.matches(':modal')));
    ok(name + ': every settings group is inside it', await p.evaluate(() => {
      const d = document.getElementById('adv');
      const g = [...document.querySelectorAll('.adv-group')];
      // the five groups: time (capsule, lock), together (sign-off), content
      // warning, device (remember) and expert — checked by identity, not count
      const required = ['[data-feature=capsule]', '[data-feature=lock]', '[data-feature=signoff]', '[data-feature=warning]', '#remember', '#expert-box'];
      return g.length >= 5 && g.every(x => d.contains(x)) && required.every(s => !!d.querySelector(s));
    }));
    ok(name + ': it fits the screen', await p.$eval('#adv', d => { const r = d.getBoundingClientRect(); return r.top >= 0 && r.bottom <= innerHeight + 1 && r.left >= 0 && r.right <= innerWidth + 1; }));
    ok(name + ': the page behind does not scroll', await p.evaluate(() => getComputedStyle(document.body).overflow === 'hidden'));
    ok(name + ': the body of the panel scrolls, the last group can be reached', await p.evaluate(() => {
      const bd = document.querySelector('#adv .drawer-body'); const can = bd.scrollHeight > bd.clientHeight && /auto|scroll/.test(getComputedStyle(bd).overflowY);
      bd.scrollTop = bd.scrollHeight; const l = document.getElementById('expert-box').getBoundingClientRect(), r = bd.getBoundingClientRect();
      return can && l.bottom <= r.bottom + 1 && l.top >= r.top;
    }));
    ok(name + ': the head stays in place while scrolling', await p.evaluate(() => { const d = document.getElementById('adv').getBoundingClientRect(), h = document.querySelector('#adv .drawer-head').getBoundingClientRect(); return Math.abs(h.top - d.top) < 1 && h.height > 30; }));
    ok(name + ': no sideways scroll', await p.evaluate(() => document.documentElement.scrollWidth <= innerWidth && document.getElementById('adv').scrollWidth <= document.getElementById('adv').clientWidth));
    let strayed = false;
    for (let i = 0; i < 40; i++) { await p.keyboard.press('Tab'); if (await p.evaluate(() => document.activeElement !== document.body && !document.activeElement.closest('#adv'))) { strayed = true; break; } }
    ok(name + ': focus stays inside (trapped)', !strayed);
    await p.keyboard.press('Escape'); await p.waitForTimeout(100);
    ok(name + ': Escape closes it and focus goes back to More', await p.evaluate(() => !document.getElementById('adv').open && document.activeElement.id === 'adv-btn' && getComputedStyle(document.body).overflow !== 'hidden'));
    await p.click('#adv-btn'); await p.waitForTimeout(150); await p.mouse.click(touch ? 180 : 100, 60);
    ok(name + ': a click on the backdrop closes it', !(await p.$eval('#adv', d => d.open)));
    await p.context().close();
    });
  }

  // ---------- nothing in the console on the main pages ----------
  await guard('console', async () => {
    const p = await open(b);
    const before = errs.length;
    for (const path of ['/new', '/terms', '/privacy', '/', '/faq', '/about']) { await p.goto(BASE + path + '?lang=en'); await p.waitForTimeout(500); }
    ok('no console errors (CSP, scripts) on new, terms, privacy, home, faq, about', errs.length === before, errs.slice(before).join(' || ').slice(0, 300));
    await p.context().close();
  });

  // ---------- phone: header, footer, tap targets ----------
  for (const [w, h] of [[390, 844], [360, 640]]) {
    const m = await open(b, { viewport: { width: w, height: h }, isMobile: true, hasTouch: true });
    await m.goto(BASE + '/new?lang=ru'); await m.waitForTimeout(400);
    ok(w + 'px: header buttons are at least 44px', await m.evaluate(() => [...document.querySelectorAll('.nav .icon-btn, .nav-r .btn')].filter(e => e.getBoundingClientRect().width > 0).every(e => { const r = e.getBoundingClientRect(); return r.width >= 43.5 && r.height >= 43.5; })));
    ok(w + 'px: header stays on one row', await m.$eval('.nav', n => n.getBoundingClientRect().height < 70));
    await m.evaluate(() => window.scrollTo(0, 1e6)); await m.waitForTimeout(200);
    ok(w + 'px: the button at the bottom does not cover the last footer link', await m.evaluate(() => {
      const bar = document.getElementById('create').getBoundingClientRect(), l = [...document.querySelectorAll('.footer-mini a')].pop().getBoundingClientRect(); return l.bottom <= bar.top + 1;
    }));
    await m.context().close();
  }

  // ---------- notes: long titles, checklists, the Share dialog ----------
  await guard('notes', async () => {
    const p = await open(b);
    const long = await create(p, { title: 'Donaudampfschifffahrtsgesellschaftskapitaensmuetzenabzeichenverleihung und mehr', body: 'x'.repeat(200) });
    const check = await create(p, { title: 'List', body: '- [ ] eggs\n- [ ] milk\n' });
    const basic = await create(p, { title: 'Plain', body: 'hello', basic: true, slug: 'sweep-' + Date.now().toString(36) });
    // the Share dialog opens by itself on a new note: encrypted says the link is the key, basic does not
    await p.goto(long.owner); await p.waitForTimeout(1500);
    if (!(await p.$eval('#share-dlg', d => d.open))) await p.click('#act-share');
    await p.waitForFunction(() => document.getElementById('kind-title').textContent.trim() !== '', null, { timeout: 10000 });
    ok('Share (encrypted note): says the link is the key', await p.evaluate(() => { const e = document.getElementById('key-hint'); return !!e && getComputedStyle(e).display !== 'none' && /key/i.test(e.textContent); }));
    await p.goto(basic.owner); await p.waitForTimeout(1500);
    if (!(await p.$eval('#share-dlg', d => d.open))) await p.click('#act-share');
    await p.waitForFunction(() => document.getElementById('kind-title').textContent.trim() !== '', null, { timeout: 10000 });
    ok('Share (basic note): no key warning', await p.evaluate(() => getComputedStyle(document.getElementById('key-hint')).display === 'none'));
    await p.context().close();

    const m = await open(b, { viewport: { width: 360, height: 640 }, isMobile: true, hasTouch: true });
    await m.goto(long.view); await m.waitForTimeout(1800);
    ok('phone 360: a long title wraps, the page does not scroll sideways', await m.evaluate(() => document.documentElement.scrollWidth <= innerWidth), String(await m.evaluate(() => document.documentElement.scrollWidth)));
    await m.context().close();

    const v = await open(b);
    await v.goto(check.view); await v.waitForSelector('#n-view input[type=checkbox]', { timeout: 15000 });
    ok('checklist boxes have a name', await v.evaluate(() => [...document.querySelectorAll('#n-view input[type=checkbox]')].every(c => (c.getAttribute('aria-label') || '').trim().length > 0)));
    await v.context().close();
  });

  // ---------- focus ring, contrast ----------
  await guard('focus and contrast', async () => {
    const p = await open(b, { colorScheme: 'light' });
    await p.goto(BASE + '/new?lang=en'); await p.waitForTimeout(300);
    for (const sel of ['#title', '.ed-src']) {
      await p.evaluate(s => document.querySelector(s).focus({ focusVisible: true }), sel);
      ok('focus ring on ' + sel, await p.$eval(sel, e => { const c = getComputedStyle(e); return (c.outlineStyle !== 'none' && parseFloat(c.outlineWidth) > 0) || c.boxShadow !== 'none'; }));
    }
    await p.evaluate(() => { document.documentElement.dataset.theme = 'light'; });
    await p.goto(BASE + '/login?lang=en'); await p.evaluate(() => { document.documentElement.dataset.theme = 'light'; });
    const link = await contrast(p, 'a[href$="recover"]', 'color', '.card');
    const faint = await p.evaluate(() => { document.body.insertAdjacentHTML('beforeend', '<p class="faint" id="t-faint">x</p>'); return 1; }).then(() => contrast(p, '#t-faint', 'color', 'body'));
    ok('light theme: link text has 4.5:1 contrast', link >= 4.5, link.toFixed(2));
    ok('light theme: faint text has 4.5:1 contrast', faint >= 4.5, faint.toFixed(2));
    await p.evaluate(() => { document.documentElement.dataset.theme = 'dark'; });
    const dfaint = await contrast(p, '#t-faint', 'color', 'body');
    ok('dark theme: faint text has 4.5:1 contrast', dfaint >= 4.5, dfaint.toFixed(2));
    await p.context().close();
  });

  // ---------- text that was never translated, old link form in the home tour ----------
  await guard('translations', async () => {
    const p = await open(b);
    await p.goto(BASE + '/?lang=de'); await p.waitForTimeout(400);
    ok('footer labels follow the language', await p.evaluate(() => !document.querySelector('[aria-label="Support"], [aria-label="Policies"], [aria-label="Languages"], [aria-label="HOP family"]')));
    await p.goto(BASE + '/terms?lang=de');
    ok('terms: the notice about the master terms is translated', await p.evaluate(() => /Master-HOP/.test(document.querySelector('.legal-note').textContent) && !/These rules govern/.test(document.body.textContent)));
    const home = await (await p.request.get(BASE + '/static/js/home.js')).text();
    ok('home tour shows /TOKEN links, not the old /n/ ones', !/['"]\/n\/[A-Za-z0-9-]/.test(home) && !/https:\/\/' \+ host \+ '\/n\//.test(home));
    await p.context().close();
  });

  console.log(R.join('\n')); console.log('errors:', errs);
  await b.close();
  if (R.some(l => l.startsWith('FAIL')) || errs.length) process.exit(1);
})();
