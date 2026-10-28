const { chromium } = require('playwright');
// Browser checks for notes at /TOKEN and a short share domain: reader links
// are short (share/TOKEN) and only redirect to the main site, where every
// note opens; old /n/ links of any kind, owner links and named links land on
// main/TOKEN with their #fragment (key and rights) intact.
// Needs the server started with two names for the same address:
//   NOTEHOP_BASE_URL=http://127.0.0.1:8090 NOTEHOP_SHARE_URL=http://localhost:8090 notehop -addr 127.0.0.1:8090 …
//   NOTEHOP_URL=http://127.0.0.1:8090 node scripts/e2e-share.js
const MAIN = (process.env.NOTEHOP_URL || 'http://127.0.0.1:8090').replace(/\/$/, '');
const SHARE = MAIN.replace('127.0.0.1', 'localhost');
const R = []; const ok = (n, c, i) => R.push((c ? 'PASS' : 'FAIL') + ' | ' + n + ' | ' + (i || ''));
const errs = [];

async function open(b) {
  const c = await b.newContext({ viewport: { width: 1280, height: 860 } });
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
  await p.click('#create'); await p.waitForURL(/#k=/, { timeout: 20000 }); await p.waitForSelector('#note:not(.hide)');
  await p.keyboard.press('Escape');
  const u = new URL(p.url()), f = new URLSearchParams(u.hash.slice(1)), token = u.pathname.slice(1);
  return { owner: p.url(), token, k: f.get('k'), a: f.get('a'), reader: await p.evaluate(([t, k]) => NH.noteURL(t, { k: k }), [token, f.get('k')]) };
}
async function shows(p, text) {
  await p.waitForFunction(t => ((document.querySelector('#n-view') || {}).textContent || '').includes(t) || ((document.querySelector('.ed-src') || {}).value || '').includes(t), text, { timeout: 15000 });
  return true;
}
const isOwner = p => p.isVisible('#act-share');
// open a link and land where it leads
async function land(b, link) { const r = await open(b); await r.goto(link); await r.waitForSelector('#note:not(.hide), #state:not(.hide)', { timeout: 15000 }); return r; }

(async () => {
  const b = await chromium.launch();
  const p = await open(b);

  const n = await create(p, { title: 'Rapat', body: 'agenda rapat senin' });
  ok('owner lands on main/TOKEN', p.url() === MAIN + '/' + n.token + '#k=' + n.k + '&a=' + n.a && await isOwner(p), p.url().split('#')[0]);
  ok('reader link is short, on the share domain', n.reader === SHARE + '/' + n.token + '#k=' + n.k, n.reader.split('#')[0]);

  let r = await land(b, n.reader);
  ok('short link opens the note on the main site', r.url() === MAIN + '/' + n.token + '#k=' + n.k && await shows(r, 'agenda rapat senin') && !(await isOwner(r)), r.url().split('#')[0]);
  await r.context().close();

  r = await land(b, MAIN + '/n/' + n.token + '#k=' + n.k);
  ok('old main/n/ link moves to main/TOKEN', r.url() === MAIN + '/' + n.token + '#k=' + n.k && await shows(r, 'agenda rapat senin'), r.url().split('#')[0]);
  await r.context().close();

  r = await land(b, SHARE + '/n/' + n.token + '#k=' + n.k);
  ok('share/n/ link lands on main/TOKEN', r.url() === MAIN + '/' + n.token + '#k=' + n.k && await shows(r, 'agenda rapat senin'), r.url().split('#')[0]);
  await r.context().close();

  r = await land(b, SHARE + '/' + n.token + '#k=' + n.k + '&a=' + n.a);
  ok('owner link through the short domain keeps owner rights', r.url() === n.owner && await shows(r, 'agenda rapat senin') && await isOwner(r), r.url().split('#')[0]);
  await r.context().close();

  r = await land(b, MAIN + '/n/' + n.token + '#k=' + n.k + '&a=' + n.a);
  ok('old owner link keeps owner rights', r.url() === n.owner && await isOwner(r), r.url().split('#')[0]);
  await r.context().close();

  // a "can write" link
  const w = await create(p, { title: 'Bareng', body: 'tulis bareng', mode: 'edit' });
  r = await land(b, w.reader);
  await r.waitForSelector('.ed-src', { state: 'visible', timeout: 15000 });
  ok('edit link opens for writing on the main site', r.url().startsWith(MAIN + '/' + w.token + '#k='));
  await r.fill('.ed-src', 'tulis bareng lewat link pendek');
  await r.waitForTimeout(2500);
  const o = await land(b, w.owner);
  ok('edit link saves', await shows(o, 'lewat link pendek').catch(() => false));
  await o.context().close(); await r.context().close();

  // a named link
  const slug = 'rapat-' + Date.now().toString(36);
  const s = await create(p, { title: 'Nama', body: 'link bernama', slug });
  ok('named link is share/name', s.reader === SHARE + '/' + slug + '#k=' + s.k && p.url().startsWith(MAIN + '/' + slug + '#'), s.reader.split('#')[0]);
  r = await land(b, s.reader);
  ok('named short link opens main/name', r.url().startsWith(MAIN + '/' + slug + '#k=') && await shows(r, 'link bernama'));
  await r.context().close();
  await p.goto(MAIN + '/new?lang=en');
  ok('the link name field shows the short domain', (await p.textContent('.prefix span')).trim() === new URL(SHARE).host + '/', await p.textContent('.prefix span'));

  // site pages stay site pages
  r = await open(b); await r.goto(SHARE + '/faq');
  ok('share/faq goes to main/faq', r.url().startsWith(MAIN + '/faq'), r.url());
  const res = await r.goto(MAIN + '/no-such-note-here');
  ok('a missing note is a 404', res.status() === 404, String(res.status()));
  await r.context().close();

  console.log(R.join('\n')); console.log('errors:', errs); await b.close();
  process.exit(R.some(x => x.startsWith('FAIL')) || errs.length ? 1 : 0);
})().catch(e => { console.log(R.join('\n')); console.error('FATAL', e.message.split('\n').slice(0, 8).join(' | ')); process.exit(1); });
