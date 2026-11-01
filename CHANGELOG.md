# Changelog

What changed in NoteHop, newest first. Each entry links to its pull request.

## [Unreleased]

### Basic and encrypted notes
- "New note" now starts with one choice: **Encrypted** (the default; locked in the browser, the key stays in the link after `#k=`, so the server can't read it) or **Basic** (readable by the server, with a link name you choose, like `/monday-meeting`). Every kind of note (plain text, document, code, website) exists in both. What used to be the "public note" switch hidden under Expert is now this choice, and "public" wording became "basic" everywhere.
- Only a basic note can have a link name. An encrypted note keeps the long link: the server refuses a name for it (`err.slug_basic_only`), in a new note and in "Add link".
- The share dialog renames a link with **Save** and **Cancel** (pencil on the row); "Add link" only creates. The old name is held for the owner for 90 days (`PUT /api/n/{token}/links/{link}`).
- The owner can change a note between basic and encrypted, both ways, from the share dialog (`POST /api/n/{token}/convert`). The browser re-encodes the text and the server swaps it in; links, keys and settings stay, version history is cleared. A note with added lines, suggestions, a decoy, burn blocks, an inbox or sign-offs can't change type yet (the server says which); snapshots are read-only.
- No note is ever indexed: `X-Robots-Tag: noindex, nofollow, noarchive` on every note, API, raw, mock and profile response (the robots meta tag was already on note pages), a `robots.txt` that keeps crawlers off the machine paths (note addresses are left crawlable on purpose, so the noindex header is seen), no sitemap, and link previews show only the kind and status of a note, never its title or text.
- My notes: filters for Encrypted and Basic, a type label on each row, and a basic note's read link is just its address.
- The CLI refuses `-slug` without `-public` before it calls the server.
- Copy: claims that "the server can't read your note" now say encrypted notes; new text is English in all catalogs until translated (`docs/pending-translation-basic-notes.json`).

### Media and asset addresses
- The avatar the account server sends (in the sign-on ticket or a profile push) is stored and reported to the control center as an absolute http(s) address: a path such as `/storage/avatars/x.png` is resolved against `NOTEHOP_SSO_URL`, and anything unusable is dropped instead of being cut in half.
- The offline worker no longer keeps `/raw/…`, `/mock/…` or `/in/…` pages (they carry a public note's text and use up a burn note's view), and it caches only complete (200) responses.

### Dashboard chrome
- The signed-in dashboard (My notes, Vault key) has the same chrome as the other HOP dashboards: a sidebar with this app's menu and the HOP family (HOP ID, LinkHop, BioHop, PixelHop, with their marks), a top bar of settings-type controls (new note, language, theme, account menu) and a compact footer. The app grid is no longer in that top bar.
- The footer on every page spreads its five columns evenly; a flex rule had packed them to the left.

## 2026-10-28

### Notes at /name, short share links ([short-links-redirect](https://github.com/navi-crwn/NoteHop/tree/short-links-redirect))
- Notes live at `/TOKEN` or `/link-name` on the main site. The old `/n/TOKEN` redirects there, for links of every kind (read, write, reply, tick, owner); the browser keeps the `#…` part.
- `NOTEHOP_SHARE_URL` is now a short domain that only redirects: reader links are `share.example/TOKEN` and open `main/TOKEN`. Nothing is served on it. Owner links stay on the main site.
- Site pages (`/new`, `/faq`…) are reserved names, and a test makes sure every new page is one.
- `notehop new` prints the short reader link; `notehop get` and `takedown` accept every form.

### Short share links ([#10](https://github.com/navi-crwn/NoteHop/pull/10))
- First version of short reader links, with notes served on the share domain. Replaced by the entry above before it went live.

### A simpler create page ([new-compose](https://github.com/navi-crwn/NoteHop/tree/new-compose))
- The editor uses the full width; the side panel of settings is gone. Web notes show the result beside the code.
- "What do you want to make?" is one row of small buttons, with a one-line description of the chosen one.
- One bar under the editor: who can do what, how private, how long, **More**, and **Create link**. Each button opens a short menu that explains every choice; **More** opens the extra settings in a drawer and counts the ones that are on.
- On phones the bar stays at the bottom as icons, and every menu opens as a sheet.

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
