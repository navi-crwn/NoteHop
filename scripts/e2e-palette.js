const { chromium } = require('playwright');
// Buttons and colour palettes: the main (gradient) button has no pale line at its right end, its text and the
// accent colour pass WCAG AA (4.5:1) on every part of the button in light and dark for each palette, and the
// palette chosen in the menu stays on every page and after a reload.
// Start a server on an empty database, then:
//   NOTEHOP_URL=http://127.0.0.1:8090 node scripts/e2e-palette.js
// (NOTEHOP_BROWSER=msedge uses the installed Edge instead of the bundled Chromium.)
const BASE = (process.env.NOTEHOP_URL || 'http://127.0.0.1:8090').replace(/\/$/, '');
const R = []; const ok = (n, c, i) => R.push((c ? 'PASS' : 'FAIL') + ' | ' + n + ' | ' + (i || ''));
const guard = async (name, f) => { try { await f(); } catch (e) { ok(name + ': could not run', false, e.message.split('\n')[0]); } };
const lum = ([r, g, b]) => { const f = v => { v /= 255; return v <= .03928 ? v / 12.92 : Math.pow((v + .055) / 1.055, 2.4); }; return .2126 * f(r) + .7152 * f(g) + .0722 * f(b); };
const ratio = (a, b) => { const x = lum(a), y = lum(b); return (Math.max(x, y) + .05) / (Math.min(x, y) + .05); };

(async () => {
  const b = await chromium.launch(process.env.NOTEHOP_BROWSER ? { channel: process.env.NOTEHOP_BROWSER } : {});
  for (const scheme of ['light', 'dark']) for (const pal of ['', 'mist', 'sand']) await guard(`${scheme} ${pal || 'classic'}`, async () => {
    const name = `${scheme}/${pal || 'classic'}`;
    const c = await b.newContext({ viewport: { width: 1440, height: 900 }, colorScheme: scheme });
    await c.addInitScript(p => { try { localStorage.setItem('nh_tour', '1'); if (p) localStorage.setItem('nh_palette', p); } catch (e) {} }, pal);
    const p = await c.newPage();
    await p.goto(BASE + '/new?lang=en'); await p.waitForTimeout(600);
    const el = await p.$('#create');
    const png = (await el.screenshot()).toString('base64');
    const m = await p.evaluate(async png => {
      const im = new Image(); im.src = 'data:image/png;base64,' + png; await im.decode();
      const cv = document.createElement('canvas'); cv.width = im.width; cv.height = im.height; const g = cv.getContext('2d'); g.drawImage(im, 0, 0);
      const px = (x, y) => [...g.getImageData(x, y, 1, 1).data].slice(0, 3);
      const w = im.width, y = Math.floor(im.height / 2), row = [], ytop = 6; // the edge is read on the middle line (the corners are rounded)
      for (let x = 2; x < w - 2; x += Math.max(1, Math.floor(w / 40))) row.push(px(x, ytop));
      const cs = getComputedStyle(document.documentElement), rgb = s => (s.match(/[\d.]+/g) || []).slice(0, 3).map(Number);
      const probe = document.createElement('span'); document.body.append(probe);
      const col = v => { probe.style.color = ''; probe.style.color = 'var(' + v + ')'; return rgb(getComputedStyle(probe).color); };
      const out = { w, row, edge: [px(w - 1, y), px(w - 2, y), px(w - 4, y)], fg: rgb(getComputedStyle(document.getElementById('create')).color), accent: col('--accent'), bg: rgb(getComputedStyle(document.body).backgroundColor), panel: col('--panel'), pal: document.documentElement.dataset.palette || '' };
      probe.remove(); return out;
    }, png);
    ok(`${name}: the palette is applied`, m.pal === pal, m.pal);
    // the italic phrase of the home page headline follows the palette (the classic one is blue to violet)
    await p.goto(BASE + '/?lang=en'); await p.waitForTimeout(500);
    const ink = await p.$eval('.home em', e => getComputedStyle(e).backgroundImage);
    global.__ink = global.__ink || {};
    if (!pal) global.__ink[scheme] = ink; else ok(`${name}: the home headline accent is not the classic gradient`, ink !== global.__ink[scheme] && !/244, 114, 182|167, 139, 250|99, 102, 241|192, 38, 211/.test(ink), ink.slice(0, 90));
    const worst = Math.min(...m.row.map(c => ratio(m.fg, c)));
    const need = 4.5;
    ok(`${name}: button text has at least ${need}:1 on every part of the button`, worst >= need, 'lowest ' + worst.toFixed(2));
    const edge = Math.abs(lum(m.edge[1]) - lum(m.edge[2]));
    ok(`${name}: no pale line at the right end (the last columns match the ones before)`, edge < .08 && Math.abs(lum(m.edge[0]) - lum(m.edge[2])) < .12, `edge lum diff ${edge.toFixed(3)} / outer ${Math.abs(lum(m.edge[0]) - lum(m.edge[2])).toFixed(3)}`);
    ok(`${name}: accent colour (links, focus) has at least 4.5:1 on the page and on cards`, Math.min(ratio(m.accent, m.bg), ratio(m.accent, m.panel)) >= 4.5, `page ${ratio(m.accent, m.bg).toFixed(2)} card ${ratio(m.accent, m.panel).toFixed(2)}`);
    await c.close();
  });

  // the choice in the menu: stays on other pages and after a reload; the classic one clears it
  await guard('palette menu', async () => {
    const c = await b.newContext({ viewport: { width: 1440, height: 900 } });
    await c.addInitScript(() => { try { localStorage.setItem('nh_tour', '1'); } catch (e) {} });
    const p = await c.newPage();
    await p.goto(BASE + '/new?lang=en');
    ok('no palette set: the classic one', (await p.evaluate(() => document.documentElement.dataset.palette || '')) === '');
    await p.click('#lang-menu > button'); await p.click('[data-palette="mist"]'); await p.waitForTimeout(150);
    ok('choosing Mist applies it at once', (await p.evaluate(() => document.documentElement.dataset.palette)) === 'mist');
    await p.goto(BASE + '/about?lang=en');
    ok('it follows to another page', (await p.evaluate(() => document.documentElement.dataset.palette)) === 'mist');
    await p.reload();
    ok('it survives a reload, and the menu shows it pressed', (await p.evaluate(() => document.documentElement.dataset.palette)) === 'mist');
    await p.click('#lang-menu > button');
    ok('the menu marks the chosen one', (await p.getAttribute('[data-palette="mist"]', 'aria-pressed')) === 'true' && (await p.getAttribute('[data-palette=""]', 'aria-pressed')) === 'false');
    await p.click('[data-palette=""]'); await p.waitForTimeout(150);
    ok('Classic clears it', (await p.evaluate(() => document.documentElement.dataset.palette || '')) === '' && (await p.evaluate(() => localStorage.getItem('nh_palette'))) === null);
    // the menu fits a phone
    await p.setViewportSize({ width: 375, height: 800 }); await p.goto(BASE + '/new?lang=de');
    await p.click('#lang-menu > button');
    const r = await p.$eval('#lang-menu .menu-list', e => { const b = e.getBoundingClientRect(); return b.left >= 0 && b.right <= innerWidth + 1 && b.bottom <= innerHeight + 1; });
    ok('phone: the language menu with the palette row fits the screen', r);
    await c.close();
  });

  await b.close();
  console.log(R.join('\n'));
  const fails = R.filter(r => r.startsWith('FAIL')).length;
  console.log(`\n${R.length - fails} passed, ${fails} failed`);
  process.exit(fails ? 1 : 0);
})();
