const { chromium } = require('playwright');
// Browser checks for the create page (/new): the starters, the settings bar
// under the editor (menus, the "More" drawer, its counter), web notes with the
// result beside the code, phones (icons and sheets), RTL and a real create.
// Start a server on an empty database, then:
//   NOTEHOP_URL=http://127.0.0.1:8090 node scripts/e2e-compose.js
const BASE = (process.env.NOTEHOP_URL || 'http://127.0.0.1:8090').replace(/\/$/, '');
const R = []; const ok = (n, c, i) => R.push((c ? 'PASS' : 'FAIL') + ' | ' + n + ' | ' + (i || ''));
const errs = [];

async function open(b, opts) {
  const c = await b.newContext(Object.assign({ viewport: { width: 1440, height: 900 } }, opts || {}));
  await c.addInitScript(() => { try { localStorage.setItem('nh_tour', '1'); } catch (e) {} });
  const p = await c.newPage();
  p.on('pageerror', e => errs.push(e.message));
  p.on('console', m => { if (m.type() === 'error') errs.push(m.text()); });
  p.on('dialog', d => d.accept());
  return p;
}
const isOpen = (p, id) => p.$eval('#' + id, d => d.open);
const val = (p, id) => p.$eval('#' + id + ' .pick-val', e => e.textContent.trim());

(async () => {
  const b = await chromium.launch();

  // ---------- desktop ----------
  const p = await open(b);
  await p.goto(BASE + '/new?lang=en'); await p.waitForTimeout(300);
  ok('one column: no side panel next to the editor', await p.evaluate(() => {
    const ed = document.querySelector('.editor-card').getBoundingClientRect();
    return ed.width >= document.getElementById('compose').getBoundingClientRect().width - 2 && !document.querySelector('aside.options');
  }));
  ok('bar shows the chosen options', await val(p, 'opt-who') === 'View only' && await val(p, 'opt-private') === 'Normal' && await val(p, 'opt-until') === 'Always',
    [await val(p, 'opt-who'), await val(p, 'opt-private'), await val(p, 'opt-until')].join('/'));
  ok('menus start closed', !(await isOpen(p, 'opt-who')) && !(await isOpen(p, 'adv')));

  await p.click('#opt-who > summary');
  ok('menu opens above the bar', await p.evaluate(() => {
    const pop = document.querySelector('#opt-who .pick-pop').getBoundingClientRect(), btn = document.querySelector('#opt-who > summary').getBoundingClientRect();
    return pop.height > 50 && pop.bottom <= btn.top;
  }));
  await p.click('#opt-private > summary');
  ok('only one menu open at a time', !(await isOpen(p, 'opt-who')) && await isOpen(p, 'opt-private'));
  await p.keyboard.press('Escape');
  ok('Escape closes the menu', !(await isOpen(p, 'opt-private')));

  await p.click('#opt-who > summary'); await p.click('label:has(input[name=mode][value=edit])');
  ok('a choice closes its menu and updates the button', !(await isOpen(p, 'opt-who')) && await val(p, 'opt-who') === 'Can write');
  ok('button icon follows the choice', await p.$eval('#opt-who .pick-ico use', u => u.getAttribute('href')) === '#i-edit');

  await p.click('#opt-private > summary'); await p.click('label:has(input[name=privacy][value=password])');
  ok('password choice keeps the menu open for the password', await isOpen(p, 'opt-private') && await p.isVisible('#password'));
  await p.click('.title-input');
  ok('clicking outside closes the menu', !(await isOpen(p, 'opt-private')));
  await p.click('#opt-private > summary'); await p.click('label:has(input[name=privacy][value=normal])');

  await p.click('#opt-until > summary'); await p.click('label:has(input[name=until][value=other])');
  ok('"Other…" asks for a duration', await isOpen(p, 'opt-until') && await p.isVisible('#expires'));
  await p.selectOption('#expires', '3600');
  ok('the button names the duration', await val(p, 'opt-until') === '1 hour', await val(p, 'opt-until'));
  await p.keyboard.press('Escape');

  ok('More has no counter yet', await p.$eval('#adv-count', e => e.classList.contains('hide')));
  await p.click('#adv > summary');
  ok('More opens a drawer at the side, page dimmed', await p.evaluate(() => {
    const d = document.querySelector('#adv .drawer').getBoundingClientRect();
    return Math.abs(d.right - innerWidth) < 2 && d.top < 2 && d.width >= 380 && !document.getElementById('scrim').hidden;
  }));
  await p.click('label:has([data-feature=slug])'); await p.fill('#slug', 'bar-test-' + Date.now().toString(36));
  await p.click('#adv [data-close]');
  ok('drawer closes with its button', !(await isOpen(p, 'adv')) && await p.$eval('#scrim', s => s.hidden));
  ok('More counts the settings that are on', await p.$eval('#adv-count', e => !e.classList.contains('hide') && e.textContent === '1'));
  await p.click('#adv > summary'); await p.waitForSelector('#scrim:not([hidden])'); await p.mouse.click(200, 300);
  ok('clicking the dimmed page closes the drawer', !(await isOpen(p, 'adv')));

  // a starter sets the bar and says what it is
  await p.click('[data-starter=secret]');
  ok('starter "Secret message" sets Read once', await val(p, 'opt-private') === 'Read once' && await val(p, 'opt-who') === 'View only');
  ok('starter description shown', /once/i.test(await p.textContent('#start-desc')), await p.textContent('#start-desc'));
  await p.click('[data-starter=signoff]');
  ok('sign-off starter lights the counter, drawer stays shut', !(await isOpen(p, 'adv')) && await p.textContent('#adv-count') === '2');

  // web note: the result sits beside the code
  await p.click('.ed-type-btn'); await p.click('[data-type=web]'); await p.waitForTimeout(400);
  ok('web note: result beside the code', await p.evaluate(() => {
    const src = document.querySelector('.ed-src').getBoundingClientRect(), run = document.querySelector('.wr-run').getBoundingClientRect();
    return run.left >= src.right - 2 && Math.abs(run.top - src.top) < 80;
  }));
  await p.click('.ed-type-btn'); await p.click('[data-type=doc]');

  // a real note, with the options from the bar
  await p.click('[data-starter=note]');
  await p.fill('#title', 'Bar test'); await p.fill('.ed-src', 'hello from the bar');
  await p.click('#opt-who > summary'); await p.click('label:has(input[name=mode][value=append])');
  await p.click('#create'); await p.waitForURL(/\/n\//, { timeout: 20000 }); await p.waitForSelector('#note:not(.hide)');
  ok('creates the note with the chosen options', /\/n\/bar-test-/.test(p.url()), p.url().split('#')[0]);
  await p.context().close();

  // ---------- phone ----------
  const m = await open(b, { viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true });
  await m.goto(BASE + '/new?lang=en'); await m.waitForTimeout(300);
  ok('phone: bar fixed at the bottom', await m.evaluate(() => {
    const r = document.querySelector('.share-bar').getBoundingClientRect(); return Math.abs(r.bottom - innerHeight) < 2 && r.height < 90;
  }));
  ok('phone: buttons are icons, names stay for screen readers', await m.evaluate(() =>
    [...document.querySelectorAll('.pick > summary')].every(s => s.getBoundingClientRect().width <= 46) &&
    document.querySelector('#opt-who .pick-val').textContent.trim() === 'View only'));
  ok('phone: no sideways scroll', await m.evaluate(() => document.documentElement.scrollWidth <= innerWidth));
  await m.tap('#opt-private > summary');
  ok('phone: menu opens as a sheet', await m.evaluate(() => {
    const r = document.querySelector('#opt-private .pick-pop').getBoundingClientRect();
    return Math.abs(r.bottom - innerHeight) < 2 && r.width >= innerWidth - 2 && !document.getElementById('scrim').hidden;
  }));
  await m.tap('label:has(input[name=privacy][value=once])');
  ok('phone: a choice closes the sheet', !(await isOpen(m, 'opt-private')) && await m.$eval('#scrim', s => s.hidden));
  await m.tap('#adv > summary');
  ok('phone: More opens as a sheet too', await m.evaluate(() => {
    const r = document.querySelector('#adv .drawer').getBoundingClientRect(); return Math.abs(r.bottom - innerHeight) < 2 && r.width >= innerWidth - 2;
  }));
  await m.context().close();

  // ---------- RTL ----------
  const a = await open(b);
  await a.goto(BASE + '/new?lang=ar'); await a.waitForTimeout(300);
  await a.click('#adv > summary');
  ok('RTL: drawer on the left', await a.evaluate(() => document.querySelector('#adv .drawer').getBoundingClientRect().left < 2));
  await a.keyboard.press('Escape');
  await a.click('#opt-who > summary');
  ok('RTL: menu lines up with its button', await a.evaluate(() => {
    const s = document.querySelector('#opt-who > summary').getBoundingClientRect(), pop = document.querySelector('#opt-who .pick-pop').getBoundingClientRect();
    return Math.abs(pop.right - s.right) < 2;
  }));
  await a.context().close();

  console.log(R.join('\n')); console.log('errors:', errs); await b.close();
  process.exit(R.some(x => x.startsWith('FAIL')) || errs.length ? 1 : 0);
})().catch(e => { console.log(R.join('\n')); console.error('FATAL', e.message.split('\n').slice(0, 5).join(' | ')); process.exit(1); });
