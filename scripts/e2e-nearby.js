// Browser check for /nearby: two devices behind one address find each
// other, connect over WebRTC, share the pad and send a file directly.
// Start a server on an empty database, then:
//   NOTEHOP_URL=http://127.0.0.1:8090 node scripts/e2e-nearby.js
const { chromium } = require('playwright');
const BASE = (process.env.NOTEHOP_URL || 'http://127.0.0.1:8090').replace(/\/$/, '');
const R = []; const ok = (n, c, i) => R.push((c ? 'PASS' : 'FAIL') + ' | ' + n + ' | ' + (i || ''));
const errs = [];
async function page(b) {
  const p = await ((await b.newContext().then(async c => { await c.addInitScript(() => { try { localStorage.setItem('nh_tour', '1'); } catch (e) {} }); return c; }))).newPage();
  p.on('pageerror', e => errs.push(e.message));
  return p;
}

(async () => {
  const b = await chromium.launch();
  try {
    const x = await page(b), y = await page(b);
    let viaServer = '';
    x.on('websocket', ws => ws.on('framesent', f => { viaServer += String(f.payload); }));
    await x.goto(BASE + '/nearby'); await y.goto(BASE + '/nearby');
    await x.waitForSelector('#nb-list .nearby-peer button:not([disabled])', { timeout: 20000 });
    await y.waitForSelector('#nb-list .nearby-peer button:not([disabled])', { timeout: 20000 });
    ok('peers connected directly', true);
    await x.fill('#nb-pad', 'kode wifi: rahasia123');
    await y.waitForFunction(() => document.querySelector('#nb-pad').value === 'kode wifi: rahasia123', null, { timeout: 10000 });
    ok('pad: shared', true);
    const [chooser] = await Promise.all([x.waitForEvent('filechooser'), x.click('#nb-list .nearby-peer button')]);
    const data = Buffer.alloc(200 * 1024, 7);
    await chooser.setFiles({ name: 'foto.bin', mimeType: 'application/octet-stream', buffer: data });
    const a = await y.waitForSelector('#nb-files a[download]', { timeout: 20000 });
    const [dl] = await Promise.all([y.waitForEvent('download'), a.click()]);
    const size = require('fs').statSync(await dl.path()).size;
    ok('file: received intact size', size === data.length, String(size));
    ok('server never saw pad text', !viaServer.includes('rahasia123'));
  } catch (e) {
    ok('FATAL', false, e.message.split('\n').slice(0, 3).join(' | '));
  } finally {
    await b.close();
    console.log(R.join('\n'));
    console.log('errors:', errs);
    process.exit(R.some(r => r.startsWith('FAIL')) || errs.length ? 1 : 0);
  }
})();
