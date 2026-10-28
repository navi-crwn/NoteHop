// Browser checks for encrypted attachments: file picker, pasted image →
// WebP, rendering for another reader, and [m:ss] timestamps seeking audio.
// Start a server on an empty database, then:
//   NOTEHOP_URL=http://127.0.0.1:8090 node scripts/e2e-files.js
const { chromium } = require('playwright');
const BASE = (process.env.NOTEHOP_URL || 'http://127.0.0.1:8090').replace(/\/$/, '');
const R = []; const ok = (n, c, i) => R.push((c ? 'PASS' : 'FAIL') + ' | ' + n + ' | ' + (i || ''));
const errs = [];

// 100 seconds of 8 kHz 8-bit silence: long enough to seek to 1:05.
function wav(seconds) {
  const n = 8000 * seconds, b = Buffer.alloc(44 + n, 128);
  b.write('RIFF', 0); b.writeUInt32LE(36 + n, 4); b.write('WAVEfmt ', 8); b.writeUInt32LE(16, 16);
  b.writeUInt16LE(1, 20); b.writeUInt16LE(1, 22); b.writeUInt32LE(8000, 24); b.writeUInt32LE(8000, 28);
  b.writeUInt16LE(1, 32); b.writeUInt16LE(8, 34); b.write('data', 36); b.writeUInt32LE(n, 40);
  return b;
}

(async () => {
  const b = await chromium.launch();
  try {
    const ctx = (await b.newContext().then(async c => { await c.addInitScript(() => { try { localStorage.setItem('nh_tour', '1'); } catch (e) {} }); return c; }));
    const p = await ctx.newPage();
    p.on('pageerror', e => errs.push(e.message));
    const bodies = [];
    p.on('request', r => { if (r.url().endsWith('/api/files') && r.method() === 'POST') bodies.push(r.postDataBuffer()); });
    await p.goto(BASE + '/new');
    await p.fill('#title', 'Rapat');
    await p.fill('.ed-src', '[1:05] intro\n');
    await p.setInputFiles('[data-editor] input[type=file]', { name: 'rekaman.wav', mimeType: 'audio/wav', buffer: wav(100) });
    await p.waitForFunction(() => /nh-file:/.test(document.querySelector('.ed-src').value), null, { timeout: 15000 });
    ok('picker: reference inserted', /!\[rekaman\.wav\]\(nh-file:[A-Za-z0-9]{20}#[\w-]{43}\)/.test(await p.inputValue('.ed-src')));
    // Paste a noisy 900×600 PNG, as a screenshot would arrive.
    await p.evaluate(async () => {
      const c = document.createElement('canvas'); c.width = 900; c.height = 600;
      const g = c.getContext('2d'), img = g.createImageData(900, 600);
      for (let i = 0; i < img.data.length; i += 4) { img.data[i] = (i * 7) % 255; img.data[i + 1] = (i >> 5) % 255; img.data[i + 2] = 90; img.data[i + 3] = 255; }
      g.putImageData(img, 0, 0);
      const blob = await new Promise(r => c.toBlob(r, 'image/png'));
      const dt = new DataTransfer(); dt.items.add(new File([blob], '', { type: 'image/png' }));
      const ta = document.querySelector('.ed-src'); ta.focus(); ta.setSelectionRange(ta.value.length, ta.value.length);
      ta.dispatchEvent(new ClipboardEvent('paste', { clipboardData: dt, bubbles: true, cancelable: true }));
    });
    await p.waitForFunction(() => /\.webp\]\(nh-file:/.test(document.querySelector('.ed-src').value), null, { timeout: 15000 });
    ok('paste: image converted to WebP', true);
    ok('upload: server got ciphertext only', bodies.length === 2 && bodies.every(x => !x.includes(Buffer.from('RIFF')) && !x.includes(Buffer.from('WEBP'))));
    await p.click('#create'); await p.waitForURL(/#k=/, { timeout: 30000 }); await p.waitForSelector('#note:not(.hide)');
    const u = new URL(p.url()), visitor = BASE + u.pathname + '#k=' + new URLSearchParams(u.hash.slice(1)).get('k');

    const v = await ((await b.newContext().then(async c => { await c.addInitScript(() => { try { localStorage.setItem('nh_tour', '1'); } catch (e) {} }); return c; }))).newPage();
    v.on('pageerror', e => errs.push(e.message));
    await v.goto(visitor); await v.waitForSelector('#n-view .attach-image img');
    ok('reader: image decrypted', await v.$eval('#n-view .attach-image img', i => i.src.startsWith('blob:') && i.naturalWidth > 0));
    await v.waitForSelector('#n-view .attach-audio audio');
    await v.$eval('#n-view .attach-audio audio', a => a.muted = true);
    await v.click('#n-view button.ts');
    await v.waitForTimeout(800);
    const t = await v.$eval('#n-view audio', a => a.currentTime);
    ok('timestamp: seeks audio to 1:05', t >= 65 && t < 70, String(t));
    ok('reader: download link named', (await v.textContent('#n-view .attach-audio .attach-meta a')) === 'rekaman.wav');

    // A missing file renders as expired, not as a broken image.
    await p.goto(BASE + '/new'); await p.fill('.ed-src', '![x](nh-file:AAAAAAAAAAAAAAAAAAAA#' + 'A'.repeat(43) + ')');
    await p.waitForSelector('.ed-preview .nhfile.gone', { timeout: 10000 }).catch(() => {});
    const seg = await p.$('.seg [data-view=preview]'); if (seg) await seg.click();
    await p.waitForSelector('.ed-preview .nhfile.gone', { timeout: 10000 });
    ok('missing file: shown as expired', true);
  } catch (e) {
    ok('FATAL', false, e.message.split('\n').slice(0, 3).join(' | '));
  } finally {
    await b.close();
    console.log(R.join('\n'));
    console.log('errors:', errs);
    process.exit(R.some(r => r.startsWith('FAIL')) || errs.length ? 1 : 0);
  }
})();
