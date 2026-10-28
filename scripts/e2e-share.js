const { chromium } = require('playwright');
// Browser checks for a separate share domain: reader links are short
// (share/TOKEN), old long links of any kind land in the right place, owners
// stay on the main site, and a note page works there (translations, editing).
// Needs the server started with two names for the same address:
//   NOTEHOP_BASE_URL=http://127.0.0.1:8090 NOTEHOP_SHARE_URL=http://localhost:8090 notehop -addr 127.0.0.1:8090 …
//   NOTEHOP_URL=http://127.0.0.1:8090 node scripts/e2e-share.js
const MAIN = (process.env.NOTEHOP_URL || 'http://127.0.0.1:8090').replace(/\/$/, '');
const SHARE = MAIN.replace('127.0.0.1', 'localhost');
const R = []; const ok = (n, c, i) => R.push((c ? 'PASS' : 'FAIL') + ' | ' + n + ' | ' + (i || ''));
const errs = [];

async function open(b, opts) {
  const c = await b.newContext(Object.assign({ viewport: { width: 1280, height: 860 } }, opts || {}));
  await c.addInitScript(() => { try { localStorage.setItem('nh_tour', '1'); } catch (e) {} });
  const p = await c.newPage();
  p.on('pageerror', e => errs.push(e.message));
  p.on('console', m => { if (m.type() === 'error' && !/status of 404/.test(m.text())) errs.push(p.url() + ' :: ' + m.text()); });
  p.on('dialog', d => d.accept());
  return p;
}
async function create(p, { title, body, mode = 'read', slug = '' }) {
  await p.goto(MAIN + '/new?lang=en');
  await p.fill('#title', title); await p.fill('.ed-src', body);
  if (mode !== 'read') {
    await p.evaluate(() => ['opt-who', 'more-modes'].forEach(id => { const d = document.getElementById(id); if (d) d.open = true; }));
    await p.check(`input[name=mode][value=${mode}]`, { force: true });
  }
  if (slug) {
    await p.click('#adv > summary'); await p.click('label:has([data-feature=slug])'); await p.fill('#slug', slug);
    await p.evaluate(() => document.querySelectorAll('.pick[open]').forEach(d => { d.open = false; }));
  }
  await p.click('#create'); await p.waitForURL(/\/n\//, { timeout: 20000 }); await p.waitForSelector('#note:not(.hide)');
  await p.keyboard.press('Escape');
  const u = new URL(p.url()), f = new URLSearchParams(u.hash.slice(1));
  return { owner: p.url(), token: u.pathname.slice(3), k: f.get('k'), a: f.get('a'), reader: await p.evaluate(t => NH.noteURL(t, { k: new URLSearchParams(location.hash.slice(1)).get('k') }), u.pathname.slice(3)) };
}
async function shows(p, text) {
  await p.waitForFunction(t => ((document.querySelector('#n-view') || {}).textContent || '').includes(t) || ((document.querySelector('.ed-src') || {}).value || '').includes(t), text, { timeout: 15000 });
  return true;
}
const isOwner = p => p.isVisible('#act-share');

(async () => {
  const b = await chromium.launch();
  const p = await open(b);

  const n = await create(p, { title: 'Rapat', body: 'agenda rapat senin' });
  ok('owner lands on the main site', p.url().startsWith(MAIN + '/n/') && await isOwner(p), p.url().split('#')[0]);
  ok('reader link is short, on the share domain', n.reader === SHARE + '/' + n.token + '#k=' + n.k, n.reader.split('#')[0]);

  // the reader link itself
  let r = await open(b);
  await r.goto(n.reader);
  ok('reader link opens the note on the share domain', await shows(r, 'agenda rapat senin') && r.url() === n.reader && !(await isOwner(r)), r.url().split('#')[0]);
  ok('share domain: page text translated', (await r.textContent('body')).includes('Rapat') && !(await r.textContent('body')).includes('note.share'));
  await r.context().close();

  // an old reader link on the main site moves to the short one, key and all
  r = await open(b);
  await r.goto(MAIN + '/n/' + n.token + '#k=' + n.k);
  await r.waitForURL(u => u.toString().startsWith(SHARE + '/'), { timeout: 10000 });
  ok('old reader link moves to the share domain', r.url() === n.reader && await shows(r, 'agenda rapat senin'), r.url().split('#')[0]);
  await r.context().close();

  // the long form on the share domain becomes the short one
  r = await open(b);
  await r.goto(SHARE + '/n/' + n.token + '#k=' + n.k);
  ok('long form on the share domain becomes short', r.url() === n.reader && await shows(r, 'agenda rapat senin'), r.url().split('#')[0]);
  await r.context().close();

  // an owner link that ended up on the share domain goes back to the main site
  r = await open(b);
  await r.goto(SHARE + '/' + n.token + '#k=' + n.k + '&a=' + n.a);
  await r.waitForURL(u => u.toString().startsWith(MAIN + '/n/'), { timeout: 10000 });
  await shows(r, 'agenda rapat senin');
  ok('owner link moves to the main site and keeps owner rights', r.url() === n.owner && await isOwner(r), r.url().split('#')[0]);
  await r.context().close();

  // a "can write" link: editing works on the share domain
  const w = await create(p, { title: 'Bareng', body: 'tulis bareng', mode: 'edit' });
  ok('edit link is short too', w.reader.startsWith(SHARE + '/' + w.token + '#k='));
  r = await open(b);
  await r.goto(MAIN + '/n/' + w.token + '#k=' + w.k);
  await r.waitForURL(u => u.toString().startsWith(SHARE + '/'), { timeout: 10000 });
  await r.waitForSelector('.ed-src', { state: 'visible', timeout: 15000 });
  await r.fill('.ed-src', 'tulis bareng dari domain bagikan');
  await r.waitForTimeout(2500);
  const o = await open(b); await o.goto(w.owner);
  ok('edit link saves from the share domain', await shows(o, 'dari domain bagikan').catch(() => false));
  await o.context().close(); await r.context().close();

  // a named link
  const slug = 'rapat-' + Date.now().toString(36);
  const s = await create(p, { title: 'Nama', body: 'link bernama', slug });
  ok('named link is share/name', s.reader.startsWith(SHARE + '/' + slug + '#k='), s.reader.split('#')[0]);
  r = await open(b);
  await r.goto(MAIN + '/n/' + slug + '#k=' + s.k);
  await r.waitForURL(u => u.toString().startsWith(SHARE + '/' + slug), { timeout: 10000 });
  ok('old named link moves too', await shows(r, 'link bernama'));
  await r.context().close();

  // site pages are not notes: they go to the main site
  r = await open(b);
  await r.goto(SHARE + '/faq');
  ok('share domain /faq goes to the main site', r.url().startsWith(MAIN + '/faq'), r.url());
  await r.context().close();

  // the dashboard copies short reader links
  await p.goto(MAIN + '/dashboard?lang=en'); await p.waitForTimeout(800);
  ok('dashboard builds short reader links', await p.evaluate(() => NH.noteURL('abcdefghij', { k: 'x' })) === SHARE + '/abcdefghij#k=x');

  console.log(R.join('\n')); console.log('errors:', errs); await b.close();
  process.exit(R.some(x => x.startsWith('FAIL')) || errs.length ? 1 : 0);
})().catch(e => { console.log(R.join('\n')); console.error('FATAL', e.message.split('\n').slice(0, 8).join(' | ')); process.exit(1); });
