const { chromium } = require('playwright');
// Browser checks for the home page: the live encryption demo, scroll motion
// (reveals, the pinned note-life story and feature tour with its dots and
// arrows), the 3D logo, the GitHub numbers, phones, RTL and "reduce motion".
// /api/repo is answered in the browser, so nothing here reaches GitHub.
// Start a server on an empty database, then:
//   NOTEHOP_URL=http://127.0.0.1:8090 node scripts/e2e-home.js
const BASE = (process.env.NOTEHOP_URL || 'http://127.0.0.1:8090').replace(/\/$/, '');
const R = []; const ok = (n, c, i) => R.push((c ? 'PASS' : 'FAIL') + ' | ' + n + ' | ' + (i || ''));
const FAKE = { repo: 'someone/notehop', url: 'https://github.com/someone/notehop', stars: 1234, forks: 56, issues: 7, license: 'MIT', pushed: Math.floor(Date.now() / 1000) - 3600,
  langs: [{ name: 'Go', share: .5 }, { name: 'JavaScript', share: .3 }, { name: 'CSS', share: .2 }], weeks: Array.from({ length: 52 }, (_, i) => i % 4), fetched: 0 };
const errs = [];
async function open(b, opt, repo) {
  const ctx = await b.newContext(Object.assign({ viewport: { width: 1366, height: 860 } }, opt || {}));
  const p = await ctx.newPage();
  p.on('pageerror', e => errs.push(e.message));
  // the offline case answers 503 on purpose
  p.on('console', m => { if (m.type() === 'error' && !(repo === false && /503/.test(m.text()))) errs.push('console: ' + m.text()); });
  await p.route('**/api/repo', r => repo === false ? r.fulfill({ status: 503, json: { error: 'x' } }) : r.fulfill({ json: FAKE }));
  return p;
}
const topOf = (p, sel) => p.evaluate(s => document.querySelector(s).getBoundingClientRect().top + scrollY, sel);
// scrolls to a share (0..1) of a pinned section's scroll room
async function scrub(p, sel, f) {
  await p.evaluate(([s, f]) => { const e = document.querySelector(s); scrollTo(0, e.getBoundingClientRect().top + scrollY + (e.offsetHeight - innerHeight) * f); }, [sel, f]);
  await p.waitForTimeout(350);
}
const selected = p => p.evaluate(() => [...document.querySelectorAll('.h-tabs [role=tab]')].findIndex(t => t.getAttribute('aria-selected') === 'true'));
const current = p => p.evaluate(() => [...document.querySelectorAll('.h-dot')].findIndex(d => d.getAttribute('aria-current') === 'true'));
const visiblePanel = p => p.evaluate(() => [...document.querySelectorAll('.h-panel')].findIndex(x => !x.classList.contains('hide')));

(async () => {
  const b = await chromium.launch();

  // ---------- desktop ----------
  const p = await open(b);
  const sent = [];
  p.on('request', r => { if (r.url().endsWith('/api/notes') && r.method() === 'POST') sent.push(JSON.parse(r.postData())); });
  await p.goto(BASE + '/');
  await p.waitForTimeout(600);
  ok('motion is on', await p.evaluate(() => document.documentElement.classList.contains('mo')));
  ok('headings split into words', (await p.$$('#tour-h .mw')).length >= 3);
  ok('far sections wait to come in', !(await p.evaluate(() => document.querySelector('#faq h2, .h-faq h2').classList.contains('in'))));
  ok('3D logo drawn (or picture kept)', await p.evaluate(() => document.getElementById('h3d').classList.contains('on') || !!document.querySelector('.h-art img')));

  // the demo: the preview is real ciphertext, and it is what Create sends
  await p.fill('#quick-title', 'Demo');
  await p.fill('#quick-body', 'kata sandi tamu 42');
  await p.waitForTimeout(700);
  const shown = await p.textContent('#quick-cipher');
  ok('preview is ciphertext, not the text', shown.length > 40 && !shown.includes('sandi') && !shown.includes('Demo'));
  ok('preview shows its size', /\d/.test(await p.textContent('#quick-size')));
  ok('only the link part after # holds the key', (await p.textContent('#quick-frag')).startsWith('#k='));

  // reveal on scroll
  await p.evaluate(() => document.querySelector('.h-faq, #faq').scrollIntoView({ block: 'center' }));
  await p.waitForTimeout(500);
  ok('a section comes in when scrolled to', await p.evaluate(() => document.querySelector('#faq h2, .h-faq h2').classList.contains('in')));

  // the note's life follows the scroll
  await scrub(p, '#h-life', .05);
  const s0 = await p.evaluate(() => [...document.querySelectorAll('#h-steps li')].findIndex(x => x.classList.contains('on')));
  await scrub(p, '#h-life', .9);
  const s4 = await p.evaluate(() => [...document.querySelectorAll('#h-steps li')].findIndex(x => x.classList.contains('on')));
  ok('note life: steps follow the scroll', s0 === 0 && s4 === 4, s0 + ' → ' + s4);
  ok('note life: heading stays on screen while pinned', await p.evaluate(() => { const r = document.getElementById('life-h').getBoundingClientRect(); return r.top > 0 && r.bottom < innerHeight; }));

  // the tour: each slice of scroll is one group, tabs and dots agree
  const groups = (await p.$$('.h-tabs [role=tab]')).length;
  let order = [];
  for (let i = 0; i < groups; i++) { await scrub(p, '#h-tour', (i + .5) / groups); order.push(await selected(p)); }
  ok('tour: groups come one after another while scrolling', order.join() === [...Array(groups).keys()].join(), order.join());
  ok('tour: dots follow the scroll', await current(p) === groups - 1 && await visiblePanel(p) === groups - 1);
  ok('tour: down arrow off on the last group', await p.isDisabled('.h-tnext'));
  await scrub(p, '#h-tour', .02);
  ok('tour: up arrow off on the first group', await p.isDisabled('.h-tprev') && await selected(p) === 0);
  await p.click('.h-dot[data-i="3"]'); await p.waitForTimeout(1200);
  ok('tour: clicking a dot goes to that group', await selected(p) === 3 && await current(p) === 3, 'at ' + await selected(p));
  await p.click('.h-tnext'); await p.waitForTimeout(1200);
  ok('tour: down arrow goes to the next group', await selected(p) === 4);
  await p.click('.h-tprev'); await p.waitForTimeout(1200);
  ok('tour: up arrow goes back', await selected(p) === 3);
  await p.click('.h-tabs [role=tab][data-i="1"]'); await p.waitForTimeout(1200);
  ok('tour: tabs scroll there too', await selected(p) === 1 && await visiblePanel(p) === 1);
  ok('tour: the group\'s little picture plays', await p.evaluate(() => document.querySelector('.h-panel:not(.hide) .h-mini-in').children.length > 0));
  await p.click('.h-dot[data-i="0"]'); await p.waitForTimeout(1200);
  ok('tour: share group draws its QR in the browser', await p.evaluate(() => { const c = document.querySelector('.h-panel:not(.hide) canvas, .h-panel:not(.hide) svg.qr, .h-panel:not(.hide) .mqr'); return !!c; }));

  // GitHub numbers
  await p.evaluate(() => document.getElementById('h-repo').scrollIntoView({ block: 'center' }));
  await p.waitForTimeout(1600);
  const dd = await p.evaluate(() => Object.fromEntries([...document.querySelectorAll('#h-repo dd[data-k]')].map(d => [d.dataset.k, d.textContent])));
  ok('repo: numbers count up to GitHub\'s', dd.stars === (1234).toLocaleString('en') && dd.forks === '56' && dd.license === 'MIT', JSON.stringify(dd));
  ok('repo: language bar and weeks', (await p.$$('#h-langbar i')).length === 3 && (await p.$$('#h-weeks i')).length === 52);

  // Create sends the previewed ciphertext
  await p.evaluate(() => scrollTo(0, 0));
  await p.click('#quick-create');
  await p.waitForURL(/\/n\//, { timeout: 20000 });
  ok('Create sends exactly the ciphertext shown', sent.length === 1 && sent[0].content.startsWith(shown.replace(/…$/, '')) && sent[0].content.length >= shown.length - 1);
  ok('nothing readable sent', sent.length === 1 && !JSON.stringify(sent[0]).includes('sandi'));
  await p.context().close();

  // ---------- GitHub unreachable ----------
  const off = await open(b, null, false);
  await off.goto(BASE + '/'); await off.waitForTimeout(500);
  ok('repo offline: shows dashes and a note', await off.evaluate(() => [...document.querySelectorAll('#h-repo dd[data-k]')].every(d => d.textContent === '–')) && (await off.textContent('#h-weeks-p')).length > 10);
  await off.context().close();

  // ---------- phone ----------
  const m = await open(b, { viewport: { width: 390, height: 800 }, isMobile: true, hasTouch: true });
  await m.goto(BASE + '/'); await m.waitForTimeout(500);
  ok('phone: no horizontal scroll', await m.evaluate(() => document.documentElement.scrollWidth <= innerWidth));
  ok('phone: sections are not pinned', await m.evaluate(() => getComputedStyle(document.querySelector('.h-tour-pin')).position === 'static'));
  ok('phone: dots run across', await m.evaluate(() => getComputedStyle(document.querySelector('.h-tnav')).flexDirection === 'row'));
  await m.evaluate(() => document.querySelector('.h-tnav').scrollIntoView({ block: 'center' }));
  await m.tap('.h-dot[data-i="2"]'); await m.waitForTimeout(300);
  ok('phone: a dot switches the group', await selected(m) === 2 && await visiblePanel(m) === 2);
  await m.tap('.h-tnext'); await m.waitForTimeout(300);
  ok('phone: arrows switch the group', await selected(m) === 3);
  await m.context().close();

  // ---------- Arabic ----------
  const a = await open(b, { locale: 'ar' });
  await a.goto(BASE + '/?lang=ar'); await a.waitForTimeout(500);
  ok('Arabic: right to left, no sideways scroll', await a.evaluate(() => document.documentElement.dir === 'rtl' && document.documentElement.scrollWidth <= innerWidth));
  await scrub(a, '#h-life', .5);
  ok('Arabic: the link chip stays inside the picture', await a.evaluate(() => { const s = document.querySelector('.h-scene').getBoundingClientRect(), c = document.getElementById('h-chiplink').getBoundingClientRect(); return c.left >= s.left && c.right <= s.right; }));
  await a.context().close();

  // ---------- reduce motion ----------
  const r = await open(b, { reducedMotion: 'reduce' });
  await r.goto(BASE + '/'); await r.waitForTimeout(500);
  ok('reduce motion: nothing hidden or moving', await r.evaluate(() => !document.documentElement.classList.contains('mo') && !document.querySelector('.mw') &&
    [...document.querySelectorAll('[data-rv]')].every(e => getComputedStyle(e).opacity === '1')));
  ok('reduce motion: no pinned scroll rooms', await r.evaluate(() => document.getElementById('h-tour').offsetHeight < innerHeight * 2));
  await r.click('.h-dot[data-i="5"]'); await r.waitForTimeout(200);
  ok('reduce motion: dots still switch groups', await selected(r) === 5);
  await r.context().close();

  console.log(R.join('\n')); console.log('errors:', errs); await b.close();
  process.exit(R.some(x => x.startsWith('FAIL')) || errs.length ? 1 : 0);
})().catch(e => { console.log(R.join('\n')); console.error('FATAL', e.message.split('\n').slice(0, 5).join(' | ')); process.exit(1); });
