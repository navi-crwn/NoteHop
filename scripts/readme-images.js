// Takes the screenshots used in the README files and the documentation site.
//
// It drives a real browser against a NoteHop server that you start on a throwaway
// database (the script makes notes and two demo accounts, so never point it at a
// real site):
//
//   go build -o /tmp/notehop ./cmd/notehop
//   NOTEHOP_ADDR=127.0.0.1:8099 NOTEHOP_DB=/tmp/readme.db NOTEHOP_GITHUB=- /tmp/notehop &
//   NOTEHOP_URL=http://note.localhost:8099 node scripts/readme-images.js [out-dir]
//
// Use a *.localhost address (as above) so the pictures do not show a bare IP address.
// Images are written as PNG to out-dir (default: the system temp folder) and, when
// the `cwebp` program is installed, also as WebP into docs/readme/ (and
// docs/assets/product-*.webp). Needs Playwright (npm i -D playwright@1 &&
// npx playwright install chromium); set CHROME=1 to use an installed Google Chrome.
const { chromium } = require('playwright');
const { execFileSync } = require('child_process');
const crypto = require('crypto');
const fs = require('fs');
const os = require('os');
const path = require('path');

const BASE = (process.env.NOTEHOP_URL || 'http://note.localhost:8099').replace(/\/$/, '');
const OUT = process.argv[2] || fs.mkdtempSync(path.join(os.tmpdir(), 'notehop-shots-'));
const ROOT = path.join(__dirname, '..');
const LANGS = { en: 'en', zh: 'zh-CN', de: 'de', id: 'id', ja: 'ja', pt: 'pt-BR', es: 'es', fr: 'fr', ru: 'ru' };
const TRIP = "## Trip to Bandung\n\nMeet at the station by **6:30**. Tickets don't work if you're late.\n\n- [x] Book the train\n- [x] Share the hotel address\n- [ ] Buy snacks\n- [ ] Charge the power bank\n\n| Day | Plan |\n|---|---|\n| Sat | Tea garden, night market |\n| Sun | Crater, back by 18:00 |\n\n:::poll Dinner on Saturday\n- Satay\n- Noodles\n- Fried rice\n:::\n";
const AGENDA = "## Agenda\n\n1. Last week's open points\n2. Launch date\n3. Who books the room\n\nBring your laptop. Notes go in this page.\n";

function webp(png, dest) {
  try {
    execFileSync('cwebp', ['-quiet', '-q', '82', '-m', '6', png, '-o', dest]);
  } catch (e) {
    if (!webp.warned) { webp.warned = true; console.log('cwebp not available: PNG files only'); }
  }
}

async function context(browser, theme, extra) {
  const c = await browser.newContext({ viewport: { width: 1280, height: 760 }, colorScheme: theme, reducedMotion: 'reduce', ...extra });
  // Skip the first-visit tour and pick the theme.
  await c.addInitScript((t) => { try { localStorage.setItem('theme', t); localStorage.setItem('nh_tour', '1'); } catch (e) {} }, theme);
  return c;
}

async function make(p, { title, text, basic, slug }) {
  await p.goto(BASE + '/new?lang=en', { waitUntil: 'networkidle' });
  if (basic) {
    await p.locator('label:has(input[name=kind][value=basic])').click();
    if (slug) await p.fill('#slug', slug);
  }
  await p.fill('#title', title);
  const ta = p.locator('textarea').first();
  await ta.fill(text);
  await ta.dispatchEvent('input');
  await p.waitForTimeout(700);
}

(async () => {
  const browser = await chromium.launch({ channel: process.env.CHROME ? 'chrome' : undefined, args: ['--use-angle=swiftshader', '--enable-unsafe-swiftshader', '--ignore-gpu-blocklist'] });
  const shot = (name, theme) => path.join(OUT, `${name}-${theme}.png`);
  const run = Date.now().toString(36);
  for (const theme of ['light', 'dark']) {
    const c = await context(browser, theme);
    const p = await c.newPage();
    // The home page in every language: the README hero pictures.
    for (const [short, code] of Object.entries(LANGS)) {
      await p.goto(`${BASE}/?lang=${code}`, { waitUntil: 'networkidle' });
      await p.evaluate(() => document.fonts.ready);
      await p.waitForTimeout(900);
      await p.screenshot({ path: shot('hero-' + short, theme), clip: { x: 0, y: 0, width: 1280, height: 720 } });
    }
    // The live demo and the feature tour.
    await p.goto(`${BASE}/?lang=en`, { waitUntil: 'networkidle' });
    await p.evaluate(() => document.fonts.ready);
    const quick = p.locator('#quick');
    await quick.scrollIntoViewIfNeeded();
    await p.waitForTimeout(1200);
    const box = await quick.evaluate((e) => { const r = e.getBoundingClientRect(); return { y: r.top + scrollY, h: r.height }; });
    await p.screenshot({ path: shot('demo', theme), fullPage: true, clip: { x: 0, y: Math.max(0, box.y + 10), width: 1280, height: box.h + 30 } });
    const tour = p.locator('#tour');
    await tour.scrollIntoViewIfNeeded();
    await p.waitForTimeout(1200);
    await tour.screenshot({ path: shot('tour', theme) });
    // New note page, then the note and its Share dialog.
    await make(p, { title: 'Weekend trip', text: TRIP });
    await p.screenshot({ path: shot('new', theme) });
    await p.click('#create');
    await p.waitForTimeout(2500);
    await p.locator('#share-dlg').waitFor();
    await p.locator('#share-dlg').screenshot({ path: shot('share', theme) });
    await p.keyboard.press('Escape');
    await p.waitForTimeout(800);
    await p.screenshot({ path: shot('note', theme) });
    const noteUrl = p.url();
    // The same note on a phone.
    const pc = await context(browser, theme, { viewport: { width: 390, height: 844 }, deviceScaleFactor: 2, isMobile: true, hasTouch: true });
    const pp = await pc.newPage();
    await pp.goto(noteUrl, { waitUntil: 'networkidle' });
    await pp.waitForTimeout(1500);
    try { await pp.locator('#share-dlg').waitFor({ timeout: 1000 }); await pp.keyboard.press('Escape'); await pp.waitForTimeout(600); } catch (e) {}
    await pp.screenshot({ path: shot('phone', theme) });
    await pc.close();
    // A basic note with a link name.
    await make(p, { title: 'Monday meeting', text: AGENDA, basic: true, slug: `monday-meeting-${theme}-${run}` });
    await p.click('#create');
    await p.waitForTimeout(2500);
    await p.locator('#share-dlg').waitFor();
    await p.keyboard.press('Escape');
    await p.waitForTimeout(800);
    await p.screenshot({ path: shot('basic', theme) });
    // A web note, after pressing Run.
    await p.goto(BASE + '/new?lang=en', { waitUntil: 'networkidle' });
    await p.getByRole('button', { name: /^Document/ }).first().click();
    await p.locator('text=HTML, CSS and JS files you can run').click();
    await p.waitForTimeout(1000);
    await p.getByRole('button', { name: /^Run/ }).first().click();
    await p.waitForTimeout(1500);
    await p.screenshot({ path: shot('web', theme) });
    await c.close();
    // My notes, signed in to a throwaway account.
    const dc = await context(browser, theme);
    const d = await dc.newPage();
    const pass = crypto.randomBytes(12).toString('base64url');
    await d.goto(BASE + '/register?lang=en', { waitUntil: 'networkidle' });
    await d.fill('#username', `shots_${theme}_${run}`);
    await d.fill('#password', pass);
    await d.fill('#password2', pass);
    await d.locator('form button[type=submit]').click();
    await d.waitForTimeout(4000);
    await make(d, { title: 'Weekend trip', text: '## Trip\n- [x] Book the train\n- [ ] Buy snacks' }); await d.click('#create'); await d.waitForTimeout(2200);
    await make(d, { title: 'Monday meeting', text: '## Agenda\n1. Open points\n2. Launch date', basic: true, slug: `weekly-agenda-${theme}-${run}` }); await d.click('#create'); await d.waitForTimeout(2200);
    await make(d, { title: 'Shared shopping list', text: '- [ ] Rice\n- [ ] Tea\n- [ ] Eggs' }); await d.click('#create'); await d.waitForTimeout(2200);
    await make(d, { title: 'Wi-Fi at the flat', text: 'Network and password for guests.' }); await d.click('#create'); await d.waitForTimeout(2200);
    await d.goto(BASE + '/dashboard?lang=en', { waitUntil: 'networkidle' });
    await d.waitForTimeout(2000);
    if (!d.url().includes('/dashboard')) throw new Error('could not sign in to the demo account: ' + d.url());
    await d.screenshot({ path: shot('dashboard', theme) });
    await dc.close();
    // The home page at 1280x900 for the documentation site.
    const hc = await context(browser, theme, { viewport: { width: 1280, height: 900 } });
    const h = await hc.newPage();
    await h.goto(`${BASE}/?lang=en`, { waitUntil: 'networkidle' });
    await h.evaluate(() => document.fonts.ready);
    await h.waitForTimeout(1200);
    await h.screenshot({ path: shot('product', theme) });
    await hc.close();
  }
  await browser.close();
  for (const theme of ['light', 'dark']) {
    for (const name of [...Object.keys(LANGS).map((s) => 'hero-' + s), 'demo', 'tour', 'new', 'share', 'note', 'phone', 'basic', 'web', 'dashboard']) {
      webp(shot(name, theme), path.join(ROOT, 'docs', 'readme', `${name}-${theme}.webp`));
    }
    webp(shot('product', theme), path.join(ROOT, 'docs', 'assets', `product-${theme}.webp`));
  }
  console.log('PNG files in', OUT);
})().catch((e) => { console.error(e); process.exit(1); });
