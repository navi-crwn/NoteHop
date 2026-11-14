const { chromium } = require('playwright');
// Full screen for the editor inside the dashboard frame, the draggable divider between the code and the result of a
// web note, and the choice of putting the file list above the code. Each choice is kept per browser; on a phone the
// panes stack and there is no divider.
// Start a server on an empty database, then:
//   NOTEHOP_URL=http://127.0.0.1:8090 node scripts/e2e-editor-layout.js
// (NOTEHOP_BROWSER=msedge uses the installed Edge instead of the bundled Chromium.)
const BASE = (process.env.NOTEHOP_URL || 'http://127.0.0.1:8090').replace(/\/$/, '');
const R = []; const ok = (n, c, i) => R.push((c ? 'PASS' : 'FAIL') + ' | ' + n + ' | ' + (i || ''));
const guard = async (name, f) => { try { await f(); } catch (e) { ok(name + ': could not run', false, e.message.split('\n')[0]); } };
const box = (p, sel) => p.$eval(sel, e => { const r = e.getBoundingClientRect(); return { l: Math.round(r.left), t: Math.round(r.top), w: Math.round(r.width), h: Math.round(r.height), r: Math.round(r.right), b: Math.round(r.bottom) }; });

(async () => {
  const b = await chromium.launch(process.env.NOTEHOP_BROWSER ? { channel: process.env.NOTEHOP_BROWSER } : {});
  const c = await b.newContext({ viewport: { width: 1440, height: 900 } });
  await c.addInitScript(() => { try { localStorage.setItem('nh_tour', '1'); } catch (e) {} });
  const p = await c.newPage();
  const errs = []; p.on('pageerror', e => errs.push(e.message));
  const user = 'lay' + Date.now().toString(36);
  await p.goto(BASE + '/register?lang=en'); await p.fill('#username', user); await p.fill('#password', 'layout-password-1'); await p.fill('#password2', 'layout-password-1');
  await p.click('form button[type=submit]'); await p.waitForURL(/\/dashboard/, { timeout: 30000 });

  await guard('full screen, document editor', async () => {
    await p.goto(BASE + '/new?lang=en'); await p.waitForTimeout(600);
    ok('the full screen button is in the editor bar (dashboard frame)', await p.locator('.ed-full').isVisible());
    await p.click('.ed-full'); await p.waitForTimeout(250);
    const e = await box(p, '[data-editor]');
    ok('the editor fills the whole window', e.l === 0 && e.t === 0 && e.w === 1440 && e.h === 900, JSON.stringify(e));
    ok('the sidebar, top bar and footer are out of reach (inert) and the button says it is on', await p.evaluate(() => ['#shell-side', '.shell-top', '.shell-foot'].every(s => document.querySelector(s).hasAttribute('inert'))) && (await p.getAttribute('.ed-full', 'aria-pressed')) === 'true');
    await p.fill('.ed-src', 'typing in full screen'); ok('the text can be typed', (await p.inputValue('.ed-src')) === 'typing in full screen');
    await p.keyboard.press('Escape'); await p.waitForTimeout(250);
    ok('Escape leaves it, the frame is back and the focus returns to the button', (await box(p, '#shell-side')).w > 200 && await p.evaluate(() => document.activeElement.classList.contains('ed-full') && !document.querySelector('#shell-side').hasAttribute('inert')));
    await p.click('.ed-full'); await p.click('.ed-full'); await p.waitForTimeout(150);
    ok('the same button leaves it too', !(await p.evaluate(() => document.querySelector('[data-editor]').classList.contains('wr-full'))));
    await p.click('.ed-full'); await p.goto(BASE + '/dashboard?lang=en'); await p.waitForTimeout(300);
    ok('it is not carried to the next page', await p.evaluate(() => !document.body.classList.contains('wr-full-on') && !document.querySelector('#shell-side').hasAttribute('inert')));
    const g = await b.newContext({ viewport: { width: 1440, height: 900 } }); const gp = await g.newPage(); await gp.goto(BASE + '/new?lang=en');
    ok('a guest (no dashboard frame) does not get the button', !(await gp.locator('.ed-full').isVisible()));
    await g.close();
  });

  await guard('web note: divider', async () => {
    await p.goto(BASE + '/new?lang=en'); await p.evaluate(() => localStorage.removeItem('nh_web_split')); await p.reload(); await p.waitForTimeout(500);
    await p.click('.ed-type-btn'); await p.click('.ed-type-list [data-type="web"]'); await p.waitForTimeout(700);
    ok('the divider is there, with a name and a value for screen readers', (await p.getAttribute('.wr-split', 'role')) === 'separator' && !!(await p.getAttribute('.wr-split', 'aria-label')) && (await p.getAttribute('.wr-split', 'aria-valuenow')) === '50');
    const s0 = await box(p, '.ed-src'), r0 = await box(p, '.wr-run');
    ok('by default the code and the result are about equal', Math.abs(s0.w - r0.w) < 40, `${s0.w} / ${r0.w}`);
    const d = await box(p, '.wr-split');
    await p.mouse.move(d.l + 5, d.t + 100); await p.mouse.down(); await p.mouse.move(d.l - 150, d.t + 100, { steps: 6 }); await p.mouse.up(); await p.waitForTimeout(150);
    const r1 = await box(p, '.wr-run'), s1 = await box(p, '.ed-src');
    ok('dragging left makes the result bigger and the code smaller', r1.w > r0.w + 100 && s1.w < s0.w - 100, `${r0.w} -> ${r1.w}`);
    const d2 = await box(p, '.wr-split');
    await p.mouse.move(d2.l + 5, d2.t + 100); await p.mouse.down(); await p.mouse.move(d2.l - 2000, d2.t + 100, { steps: 6 }); await p.mouse.up(); await p.waitForTimeout(150);
    const rmax = await box(p, '.wr-run'), body = await box(p, '.ed-body');
    ok('there is a largest size: the code keeps at least a fifth', await p.getAttribute('.wr-split', 'aria-valuenow') === '80' && (await box(p, '.ed-src')).w > 100, `result ${rmax.w} of ${body.w}`);
    await p.focus('.wr-split'); await p.keyboard.press('End'); await p.waitForTimeout(100);
    ok('End gives the smallest result (20)', (await p.getAttribute('.wr-split', 'aria-valuenow')) === '20');
    await p.keyboard.press('ArrowLeft'); await p.keyboard.press('ArrowLeft');
    ok('the arrow keys move it (left makes the result bigger)', (await p.getAttribute('.wr-split', 'aria-valuenow')) === '24');
    await p.reload(); await p.waitForTimeout(500); await p.click('.ed-type-btn'); await p.click('.ed-type-list [data-type="web"]'); await p.waitForTimeout(600);
    ok('the size is remembered in this browser', (await p.getAttribute('.wr-split', 'aria-valuenow')) === '24');
    await p.dblclick('.wr-split'); ok('a double click sets it back to equal', (await p.getAttribute('.wr-split', 'aria-valuenow')) === '50');
  });

  await guard('web note: files on top', async () => {
    const side = await box(p, '.wr-files'), src = await box(p, '.ed-src');
    ok('by default the files are beside the code', side.r <= src.l + 2 && side.w < 260, JSON.stringify(side));
    await p.click('.wr-files-btn'); await p.waitForTimeout(250);
    const top = await box(p, '.wr-files'), src2 = await box(p, '.ed-src'), body = await box(p, '.ed-body');
    ok('the files are a row above the code, across the editor', top.b <= src2.t + 2 && top.w > body.w - 20, JSON.stringify(top));
    ok('the button says what a click does next', /side/i.test(await p.getAttribute('.wr-files-btn', 'aria-label')));
    ok('the result is still beside the code', (await box(p, '.wr-run')).l >= src2.r - 2);
    await p.reload(); await p.waitForTimeout(500); await p.click('.ed-type-btn'); await p.click('.ed-type-list [data-type="web"]'); await p.waitForTimeout(600);
    ok('the choice is remembered in this browser', (await p.getAttribute('[data-editor]', 'data-files')) === 'top' && (await box(p, '.wr-files')).b <= (await box(p, '.ed-src')).t + 2);
    // full screen of the web note with the files on top
    await p.click('.wr-full-btn'); await p.waitForTimeout(250);
    const e = await box(p, '[data-editor]'), srcF = await box(p, '.ed-src');
    ok('web note full screen: the whole window, the code uses the room', e.w === 1440 && e.h === 900 && srcF.h > 500, JSON.stringify(srcF));
    await p.keyboard.press('Escape'); await p.waitForTimeout(200);
    ok('Escape leaves the web note full screen too', !(await p.evaluate(() => document.querySelector('[data-editor]').classList.contains('wr-full'))));
    await p.click('.wr-files-btn'); await p.waitForTimeout(200);
    ok('and back to the side', (await box(p, '.wr-files')).r <= (await box(p, '.ed-src')).l + 2);
  });

  await guard('phone', async () => {
    const pc = await b.newContext({ viewport: { width: 390, height: 844 }, storageState: await c.storageState() });
    await pc.addInitScript(() => { try { localStorage.setItem('nh_tour', '1'); } catch (e) {} });
    const pp = await pc.newPage(); await pp.goto(BASE + '/new?lang=en'); await pp.waitForTimeout(500);
    await pp.click('.ed-type-btn'); await pp.click('.ed-type-list [data-type="web"]'); await pp.waitForTimeout(600);
    ok('390px: no divider, panes stacked, no sideways scroll', !(await pp.locator('.wr-split').isVisible()) && await pp.evaluate(() => document.documentElement.scrollWidth <= innerWidth));
    await pc.close();
  });

  ok('no script errors', errs.length === 0, errs.join(' | '));
  await b.close();
  console.log(R.join('\n'));
  const fails = R.filter(r => r.startsWith('FAIL')).length;
  console.log(`\n${R.length - fails} passed, ${fails} failed`);
  process.exit(fails ? 1 : 0);
})();
