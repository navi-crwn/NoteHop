const { chromium } = require('playwright');
// Browser checks for writing help (ai.js). No real AI is called: requests to
// the provider are answered here, and the browser's built-in AI is faked.
// Start a server on an empty database, then:
//   NOTEHOP_URL=http://127.0.0.1:8090 node scripts/e2e-ai.js
const BASE = (process.env.NOTEHOP_URL || 'http://127.0.0.1:8090').replace(/\/$/, '');
const KEY = 'sk-ant-test-' + Date.now().toString(36);
const R = []; const ok = (n, c, i) => R.push((c ? 'PASS' : 'FAIL') + ' | ' + n + ' | ' + (i || ''));
const CORS = { 'access-control-allow-origin': '*', 'access-control-allow-headers': '*', 'access-control-allow-methods': 'GET, POST, OPTIONS', 'content-type': 'application/json' };

(async () => {
  const b = await chromium.launch(); const errs = [];
  const ctx = await b.newContext({ viewport: { width: 1280, height: 900 }, permissions: ['clipboard-read', 'clipboard-write'] });
  await ctx.addInitScript(() => { try { localStorage.setItem('nh_tour', '1'); } catch (e) {} });
  const p = await ctx.newPage();
  p.on('pageerror', e => errs.push(e.message));
  p.on('console', m => { if (m.type() === 'error' && !/40[0-9]|Failed to load resource/.test(m.text())) errs.push('console: ' + m.text()); });
  p.on('dialog', d => d.accept());
  const toNoteHop = []; p.on('request', r => { if (r.url().startsWith(BASE)) toNoteHop.push(r.url() + ' ' + (r.postData() || '') + ' ' + JSON.stringify(r.headers())); });

  // the provider, answered here
  let seen = [], reply = { content: [{ type: 'thinking', thinking: '' }, { type: 'text', text: 'Kumpul pukul 06.30.' }], stop_reason: 'end_turn' }, status = 200;
  await ctx.route('https://api.anthropic.com/**', async route => {
    const req = route.request();
    if (req.method() === 'OPTIONS') return route.fulfill({ status: 204, headers: CORS });
    seen.push({ url: req.url(), method: req.method(), headers: req.headers(), body: req.postData() });
    if (req.url().includes('/v1/models')) {
      return route.fulfill({ status: 200, headers: CORS, body: JSON.stringify({ data: [
        { id: 'claude-opus-5-5', display_name: 'Claude Opus 5.5', capabilities: { effort: { supported: true } } },
        { id: 'claude-haiku-5-5', display_name: 'Claude Haiku 5.5', capabilities: { effort: { supported: true } } }] }) });
    }
    return route.fulfill({ status, headers: CORS, body: JSON.stringify(status === 200 ? reply : { error: { type: 'authentication_error', message: 'invalid x-api-key' } }) });
  });

  await p.goto(BASE + '/new');
  ok('AI button in the editor', await p.isVisible('.ed-ai'));
  const csp = (await (await fetch(BASE + '/new')).headers.get('content-security-policy')) || '';
  ok('CSP lets the editor reach the providers', csp.includes('connect-src \'self\' https://api.anthropic.com'));
  await p.fill('.ed-src', 'Berangkat Sabtu pagi. Kumpul di stasiun paling lambat 06.30, karena tiket tidak bisa dipakai kalau terlambat.');
  await p.click('.ed-ai');
  ok('panel opens, off until chosen', await p.isVisible('.ai-panel.open') && (await p.textContent('.ai-engine')).includes('Off'));
  ok('off: nothing sent anywhere', (await p.textContent('.ai-priv')).includes('Nothing is sent'));

  // ---------- own key: Anthropic ----------
  await p.click('.ai-panel .ai-body .btn.primary'); // Set up AI
  await p.waitForSelector('dialog.ai-dlg[open]');
  await p.click('dialog.ai-dlg [role=tab]:has-text("My own API key")');
  ok('Anthropic is the default provider', await p.getAttribute('.ai-providers button:has-text("Anthropic")', 'aria-checked') === 'true');
  ok('warning names who reads the text', (await p.textContent('.ai-warn')).includes('Anthropic will read the text you send'));
  await p.fill('#ai-key', KEY);
  await p.click('dialog.ai-dlg button:has-text("Load models")');
  await p.waitForFunction(() => document.querySelector('.ai-ok').textContent.includes('Connected'));
  const list = seen.find(s => s.url.includes('/v1/models'));
  ok('models listed with the key, from the browser', list && list.headers['x-api-key'] === KEY && list.headers['anthropic-version'] === '2023-06-01' &&
    list.headers['anthropic-dangerous-direct-browser-access'] === 'true');
  ok('default model is Claude Opus 5.5', (await p.inputValue('#ai-model')) === 'claude-opus-5-5');
  await p.click('dialog.ai-dlg .btn.primary:has-text("Save")');
  await p.waitForSelector('dialog.ai-dlg', { state: 'detached' });
  ok('panel shows the provider', (await p.textContent('.ai-engine')).includes('Anthropic, with your key'));
  ok('privacy line names the provider', (await p.textContent('.ai-priv')).includes('straight to Anthropic'));

  // rewrite the selected sentence
  const v = await p.inputValue('.ed-src'), s = v.indexOf('Kumpul'), e = v.length;
  await p.evaluate(([s, e]) => { const t = document.querySelector('.ed-src'); t.focus(); t.setSelectionRange(s, e); t.dispatchEvent(new Event('select')); }, [s, e]);
  await p.click('.ai-panel .ai-scope [data-scope=sel]');
  ok('scope counts the selected words', (await p.textContent('.ai-scope [data-scope=sel] small')).includes('words'));
  await p.click('.ai-tasks button:has-text("Rewrite")');
  await p.click('.ai-tones button:has-text("Shorter")');
  seen = [];
  await p.click('.ai-go');
  await p.waitForSelector('.ai-out ins');
  const call = seen.find(x => x.url.endsWith('/v1/messages'));
  const body = call && JSON.parse(call.body);
  ok('request: Opus 5.5, server-side fallback, low effort', body && body.model === 'claude-opus-5-5' && body.fallbacks === 'default' &&
    call.headers['anthropic-beta'] === 'server-side-fallback-2026-07-01' && body.output_config && body.output_config.effort === 'low' && body.max_tokens === 16000);
  ok('request: only the selected text, marked as data', body.messages.length === 1 && body.messages[0].content.includes('<note>\nKumpul di stasiun') &&
    !body.messages[0].content.includes('Berangkat') && /not instructions/.test(body.system) && /shorter/.test(body.system));
  ok('result shown next to the original', (await p.textContent('.ai-out ins')) === 'Kumpul pukul 06.30.' && (await p.textContent('.ai-out del')).startsWith('Kumpul di stasiun'));
  ok('nothing changed before Replace', (await p.inputValue('.ed-src')) === v);
  await p.click('.ai-acts button:has-text("Replace")');
  ok('Replace puts it in place of the selection', (await p.inputValue('.ed-src')) === 'Berangkat Sabtu pagi. Kumpul pukul 06.30.');
  await p.keyboard.press('Control+z');
  ok('Ctrl+Z brings the old text back', (await p.inputValue('.ed-src')) === v);

  // summary goes below; refusal and errors are shown plainly
  await p.click('.ai-tasks button:has-text("Summarize")'); await p.click('.ai-scope [data-scope=all]');
  reply = { content: [{ type: 'text', text: 'Ringkasan singkat.' }], stop_reason: 'end_turn' };
  await p.click('.ai-go'); await p.waitForSelector('.ai-out p');
  await p.click('.ai-acts button:has-text("Insert below")');
  ok('Insert below adds after the text', (await p.inputValue('.ed-src')).endsWith('\n\nRingkasan singkat.'));
  reply = { content: [], stop_reason: 'refusal', stop_details: { type: 'refusal', category: null } };
  await p.click('.ai-go'); await p.waitForSelector('.ai-result .alert.err');
  ok('refusal is explained', (await p.textContent('.ai-result .alert')).includes('declined'));
  status = 401;
  await p.click('.ai-go'); await p.waitForFunction(() => /invalid x-api-key/.test((document.querySelector('.ai-result .alert') || {}).textContent || ''));
  ok('provider error is shown', true);
  status = 200;
  ok('the key never went to NoteHop', !toNoteHop.some(x => x.includes(KEY)));
  ok('the note text never went to NoteHop', !toNoteHop.some(x => x.includes('Kumpul di stasiun')));

  // right-click menu → AI; code notes have no AI button
  await p.keyboard.press('Escape');
  await p.evaluate(() => { const t = document.querySelector('.ed-src'); t.focus(); t.setSelectionRange(0, 9);
    t.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true, clientX: 300, clientY: 300 })); });
  await p.click('.ctx-it:has-text("Rewrite with AI")');
  ok('menu opens the panel on Rewrite', await p.isVisible('.ai-panel.open') && (await p.getAttribute('.ai-tasks button:has-text("Rewrite")', 'aria-checked')) === 'true');
  await p.click('.ai-head .icon-btn');
  await p.click('.ed-type-btn'); await p.click('.ed-type-list [data-type="code"]');
  ok('no AI on code notes', await p.isHidden('.ed-ai'));

  // ---------- on this device (the browser's built-in AI, faked) ----------
  const dctx = await b.newContext({ viewport: { width: 390, height: 844 }, isMobile: true });
  await dctx.addInitScript(() => {
    try { localStorage.setItem('nh_tour', '1'); } catch (e) {}
    window.Summarizer = { availability: async () => 'available',
      create: async o => ({ summarize: async t => (o.type === 'key-points' ? '- ' : 'RINGKAS: ') + t.slice(0, 12), destroy() {} }) };
    window.Rewriter = { availability: async () => 'downloadable', create: async () => ({ rewrite: async t => t.toUpperCase(), destroy() {} }) };
  });
  const q = await dctx.newPage(); q.on('pageerror', e => errs.push('device: ' + e.message));
  const net = []; q.on('request', r => { if (!r.url().startsWith(BASE)) net.push(r.url()); });
  await q.goto(BASE + '/new');
  await q.fill('.ed-src', 'Catatan rapat hari ini.');
  await q.click('.ed-ai');
  await q.click('.ai-panel .ai-body .btn.primary');
  await q.waitForSelector('dialog.ai-dlg[open]');
  await q.waitForSelector('.ai-caps li.yes');
  ok('device tab lists what the browser can do', (await q.textContent('.ai-caps')).includes('Summaries') && (await q.textContent('.ai-caps')).includes('Downloads on first use'));
  await q.click('dialog.ai-dlg .btn.primary:has-text("Save")');
  await q.waitForSelector('dialog.ai-dlg', { state: 'detached' });
  ok('device engine shown', (await q.textContent('.ai-engine')).includes('On this device') && (await q.textContent('.ai-priv')).includes('nothing is sent'));
  await q.click('.ai-tasks button:has-text("Summarize")');
  await q.click('.ai-go'); await q.waitForSelector('.ai-out p');
  ok('summary made on the device', (await q.textContent('.ai-out p')) === 'RINGKAS: Catatan rapa');
  await q.click('.ai-tasks button:has-text("Rewrite")'); await q.click('.ai-go'); await q.waitForSelector('.ai-out ins');
  ok('rewrite made on the device', (await q.textContent('.ai-out ins')) === 'CATATAN RAPAT HARI INI.');
  ok('panel is a bottom sheet on a phone', await q.evaluate(() => { const r = document.querySelector('.ai-panel').getBoundingClientRect(); return r.bottom >= innerHeight - 1 && r.width >= innerWidth - 1; }));
  await q.click('.ai-tasks button:has-text("Translate")'); await q.click('.ai-go'); await q.waitForSelector('.ai-result .alert.err');
  const why = await q.textContent('.ai-result .alert');
  ok('missing on-device skill is explained', why.includes("can't do this") || why.includes("doesn't support"), why);
  ok('device mode calls no network', net.length === 0, net.join(' '));

  console.log(R.join('\n')); console.log('errors:', errs); await b.close();
  process.exit(R.some(r => r.startsWith('FAIL')) || errs.length ? 1 : 0);
})().catch(e => { console.log(R.join('\n')); console.error('FATAL', e.message.split('\n').slice(0, 5).join(' | ')); process.exit(1); });
