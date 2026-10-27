# Changelog

What changed in NoteHop, newest first. Each entry links to its pull request.

## 2026-10-28

### Control center API and reports ([hop-admin-api](https://github.com/navi-crwn/NoteHop/tree/hop-admin-api))
- **Report this note** in every note's menu: a reason and a short message go to the site team, never the note's content. 5 reports per hour per address.
- A signed `/hop/*` API (HOP App Protocol v1) so a control center can see health, numbers, accounts, notes, attachments, reports, link-name holds and settings, and act on them; and so the account server can push profile changes, suspensions and deletions. Encrypted notes stay metadata only. Every change is written to an audit log. `NOTEHOP_HOP_CLIENTS` names the callers.
- Account roles: the admin role now lives in the database and follows the account server's roles (promotion and demotion). `NOTEHOP_ADMINS` only seeds the role for accounts that already exist; a listed name registered later grants nothing. Suspended accounts can't sign in.
- Database migrations 9 (role, status, audit, events, settings) and 10 (reports).


### Home page v2 ([#6](https://github.com/navi-crwn/NoteHop/pull/6))
- The logo as 3D blocks you can turn (plain WebGL; a picture when WebGL is missing).
- A live demo with real encryption: the scrambled text in the preview is exactly what **Create link** sends.
- Motion that follows the scroll: a note's life from typing to gone, and a feature tour of six groups with tabs, dots and up/down arrows. Nothing moves when "reduce motion" is on.
- Numbers from GitHub (stars, languages, weekly commits) through `GET /api/repo`, set with `NOTEHOP_GITHUB`.
- MIT license.

### Web notes ([#5](https://github.com/navi-crwn/NoteHop/pull/5))
- A new note type: HTML, CSS and JS files in one note, with a file explorer, templates, **Run**, the result and a console, phone and tablet widths, and full screen.
- The code runs only in a sealed frame (`/run`) with no internet and no access to the page, cookies or the note's key. Readers are asked before someone else's code runs.

### Account recovery and writing help ([#4](https://github.com/navi-crwn/NoteHop/pull/4))
- 8 one-time recovery codes and a `/security` page to change your password. The server never sees the codes, the password or the vault key.
- Optional writing help: summarize, rewrite, translate, fix spelling, with the browser's built-in AI or your own API key (Claude, ChatGPT, Gemini, OpenRouter, Ollama). `NOTEHOP_AI` turns it off or limits it to on-device.

### Note types and sign-in ([#3](https://github.com/navi-crwn/NoteHop/pull/3))
- Note types: Plain, Document and Code (line numbers, never run, downloads as `.txt`).
- A right-click format menu: colors, highlight, size, alignment, case, links.
- Sign-in buttons straight to a provider on your account server (`NOTEHOP_SSO_PROVIDERS`).
- A home page with a quick note; the full create page moved to `/new`.
- Limits on `/raw` and `/mock` reads and on attachments per address.

## 2026-10-27

### First public version ([#1](https://github.com/navi-crwn/NoteHop/pull/1))
- Notes locked in the browser (AES-256-GCM, key in the link), links with their own rights, owner links, frozen copies, delete after reading, expiry, your own link names and QR codes.
- Markdown editor with a `/` menu, spoilers, PIN blocks, checklists, polls, forms, calculator, Mermaid, Kanban, slides.
- Encrypted 24-hour attachments, live collaboration with chat, nearby devices over WebRTC.
- Timed and private extras: time capsules, scheduled locks, decoy passwords, network allow lists, leak tracer, sign-off, version history.
- Developer tools: `/raw`, the `notehop` CLI, mock API, webhooks in and out, SSH, Telegram bot, installable app (PWA).
- Optional zero-knowledge accounts, single sign-on, 9 languages, name rules and abuse tools, VPS installer with daily backups.
