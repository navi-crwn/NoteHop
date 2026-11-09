# Changelog

What changed in NoteHop, newest first. Each entry links to its pull request.

## Unreleased

### Fixed
- The local sign-in form says what it takes: a NoteHop username chosen at sign-up here, not an email address. An email typed into it gets a sentence that points at "Continue with HOP" instead of the username rule. Nothing about accounts or sign-in changes.
- Small copy: the password box on a reader's page says "Enter the password" (it said "Add a password"); "Private margin" asks a signed-in member to unlock the vault instead of to sign in; counts are written so that 1 reads right ("Suggestions waiting: 1", not "1 suggestions").
- My notes no longer shows a dead "Locked" row for every note the vault can't open. A basic note is listed by its name with a working link in every state; an encrypted one says why its title is locked (vault locked in this browser, a vault key that doesn't match, or a key that was never saved to the account) and the page offers the way out for each: unlock, enter the key again, or "Save the keys" from the device that made the note. The page says whether the vault is open, and it can be locked again without signing out (also from the account menu). Search matches the link name too and says when nothing matches; To-dos, Links, Code, the graph and Export read basic notes without the vault and say how many locked notes they left out, instead of an empty list or "No notes to export". The delete dialog names the note (never "Untitled" for a named one) and confirms with "Deleted.". "Add them" forgets notes that no longer exist instead of offering them again for ever. Notes remembered on the device are a group of their own, "On this device". One term, "vault key", replaces "account key" in all nine languages.
- `GET /api/me/notes` names a basic note from its own text (`title`), so the dashboard can show it when the owner blob is empty or the vault is closed; an encrypted note keeps an empty title without the key. Each row links through the note's first read-only link when it has one. `POST /api/n/{token}/claim` also lets the owner seal the key of a note already in the account whose blob is empty (made while the vault was closed).
- A decoy password now answers exactly as the note does to a stranger, even on the owner link and in the owner's signed-in browser: no Owner badge, no editor, no link list and no "someone opened the decoy" report. Until now the owner's own page gave the decoy away, which is the one situation it exists for. A decoy opening still spends a view like a real one, so a view counter can't tell them apart either; the settings now say so.

### Changed
- HOP family assets move to hop-assets v4: FileHop's new pixel mark in the family list, the app switcher and the footer (bundled manifest), and the copied home engine with FileHop's matching 3D map. `scripts/sync-hop-assets.sh` copies from v4.

### Security
- `HOP_PICKER_SECRET` (32+ characters) verifies `GET /api/v1/me/notes` with a secret the hub never uses. Until now NoteHop had to hold BioHop's sign-in secret (`HOP_SSO_SECRET_BIOHOP`) to verify the note picker, so a leak here could forge a hub ticket for BioHop. It is accepted next to the older secrets while the family moves; once BioHop signs with it (navi-crwn/BioHop `fix/picker-secret`), remove `HOP_SSO_SECRET_BIOHOP` from this server.

### Added
- FileHop (file.hel.ink), the sixth HOP app, is in the HOP family list: the dashboard sidebar, the app switcher and the footer (from the bundled asset manifest), the README family table in all nine languages, and the family table on the documentation site.
- The HOP family assets move to hop-assets v3, which adds FileHop: the bundled manifest, the copied home engine (`web/static/hop-home/`, with the FileHop 3D mark), `scripts/sync-hop-assets.sh` and the documentation site's font link. In the HOP family section, an app the 3D engine has no block map for keeps its flat mark instead of falling back to the NoteHop logo.
- `GET /api/v1/me/notes` for the note picker in the other HOP apps (BioHop's "Choose from NoteHop"). Only an app named in `NOTEHOP_NOTES_CLIENTS` (default `biohop`, empty turns it off) may call it, server to server, signed with the HOP App Protocol (its own `HOP_SSO_SECRET_<APP>` when it has one, else `NOTEHOP_SSO_SECRET`) and with `X-HOP-Actor` set to the member's HOP ID. It lists that member's public notes that have a read-only link, newest first, `{ok, data: {items: [{id, title, url, updated}], next}}`, with `?q=` on the title, `?limit=` up to 50 and a cursor. Never listed: encrypted notes (the key stays in the member's browser, and a keyless link shows visitors nothing), notes shared only with an edit, append, check or suggest link (a public page would hand those rights to everyone), notes with a content warning, burn-after-reading notes, expired notes and notes a feature holds back before reading. A browser session cannot use the route.

## [Unreleased]

- The report scrub also catches a key pasted with an escaped `#` (`%23k=`, `%23k%3D`) and a bare `k=...` or `a=...`. Reports already stored lose the key the next time the server starts; the rest of their text stays. Backups taken earlier still hold the old text until they age out.
- `deploy/notehop.env.example` names every variable the program reads. A test keeps it that way.
- CI builds use Go 1.27.3 (`go` line in `go.mod`), which has the fixes the 1.27.2 toolchain lacks.
- `install.sh --dry-run` no longer creates the data folder and `bin/`. It said nothing would be executed, and now nothing is.

## [1.0.0] - 2026-11-04

The first stable release. NoteHop has been public since 2026-10-27; 1.0.0 is the point where the feature set below is treated as the baseline that later releases build on.

- Notes are encrypted in the browser by default. The key stays in the link after `#k=`, so the server stores ciphertext it cannot read. A basic (readable) note is the other choice, and the owner can convert a note either way.
- One account for every HOP app: sign-in goes through HOP ID, and the hub tells NoteHop about profile changes, suspensions, deletions and revoked sessions, and NoteHop acts on them. Notes also work without an account.
- The interface, the READMEs and the documentation site are in nine languages: English, 简体中文, Deutsch, Bahasa Indonesia, 日本語, Português (Brasil), Español, Français and Русский.
- Self-hosting is one command (`install.sh`, with `update.sh` and `uninstall.sh`), and the app is a single Go binary with a SQLite database. Database migrations run at start (12 so far).
- The 2026-11-04 audit fixes are included: reports no longer keep a note's key, a sign-in ticket for a suspended account is refused, and a page that fails to render answers 500.

The sections below make up this release. The dated headings are the days the work landed, not separate versions.

### More settings and interface sweep ([#38](https://github.com/navi-crwn/NoteHop/pull/38))
- "More settings" on the New note page lost four of its six groups to the page behind it (a stray closing tag). It is now a proper dialog: it scrolls inside, the head stays, Escape, the backdrop and the close button close it, focus returns to the button, and the page behind does not scroll.
- The Share dialog of an encrypted note says the link is the key, in all nine languages.
- No inline styles any more (the console errors on New note, Terms, Privacy and the content warning screen), `/TOKEN` links in the home tour, translated footer labels and notice on Terms and Privacy.
- Readable contrast for links and faint text, names for checklist boxes and icon buttons, focus rings on the title and editor, 44 px controls and a one-row header on phones, menus that stay on screen, long titles that wrap, and less layout shift while a note loads.
- `scripts/e2e-sweep.js` and `internal/app/ui_sweep_test.go` check these.

### Copy edits ([#36](https://github.com/navi-crwn/NoteHop/pull/36))
- The error for a missing note is worded in parallel with the other errors, and the Russian export text has its punctuation fixed.

### Audit fixes ([#39](https://github.com/navi-crwn/NoteHop/pull/39))
- Live editing: peers are closed when a note is deleted, a link is revoked or a note expires. The icon-only formatting buttons have an accessible name.
- A report no longer keeps a note's key or owner token: `#k=…` and `#a=…` in the report text are replaced with `[removed]` before it is stored, and the report dialog tells people not to paste the full link of an encrypted note.
- A sign-in ticket that says the account is suspended, banned or locked is refused before any local account is made or linked.
- A page that fails while it renders now answers 500 instead of a 200 with half a page.
- A mistyped command (`notehop bakcup`) exits with an error instead of starting the server.
- A bug that panics in a handler now answers with an error page in the reader's language (or the JSON error for the API) and writes the panic to the log, instead of dropping the connection.
- A report keeps a keyed hash of the reporter's address (made with a secret in the database) instead of a plain hash that could be reversed by trying every IPv4 address. Reports already stored keep their old value and stop counting toward the hourly limit after an hour.
- Copying a link that holds a key from My notes, the diff page or the redaction dialog says that the link is the key. The embed's Open link no longer sends a referrer.
- The privacy page says that a report's reason and message are stored readable and that the owner part of a link is sent to the server (stored as a hash).

### Hardening ([#35](https://github.com/navi-crwn/NoteHop/pull/35))
- After signing in, `?next=` only follows paths on this site: a tab, line break or other control character, or a backslash, in the path sent the browser to another site (`/<tab>/host`). The server and the sign-in and vault scripts all refuse it now.
- `notehop backup` creates its copy readable by its owner only (0600), whatever the umask; a failed backup removes the half-written file. `deploy/backup.sh` and `update.sh` keep the backup folder at 0700, and the backup service sets the same state directory mode as the main one.
- Deleting a note, a takedown, an expired note and deleting an account remove the note's attachments with it instead of leaving them until their 24 hours are over.
- Deleting an account releases the link names held for it: they stay reserved for everyone, instead of for the next account that gets the same numeric id.
- Live editing: a person joining at the moment the last one leaves could end up in a room the note no longer pointed to, cut off from everyone else on that note.
- The license names the copyright holder, as the other HOP repositories do.

### Installers, READMEs and documentation site ([#34](https://github.com/navi-crwn/NoteHop/pull/34))
- `install.sh` is a one-command installer that also runs straight from GitHub with `curl … | bash`. It checks for Go or Docker, git, curl and free disk space, asks for the site name, address, port and data folder (every question has a default, and `--yes` takes them all), writes a private `.env`, builds the program, and starts it once to check `/healthz`. It can run NoteHop in the foreground, as a systemd unit, or with Docker Compose, and can connect to an existing HOP ID hub or install one next to NoteHop. `--dry-run` shows the steps without running them.
- `update.sh` copies the database into a private `backups/` folder (mode 0700), runs `git pull --ff-only` and rebuilds, or rebuilds the Docker image. It refuses to run over local changes unless you pass `--force`.
- `uninstall.sh` removes the program and keeps `.env`, the data folder and `backups/`. `--purge` deletes those too, after you type the site name.
- A README in each of the nine languages, and a documentation site for GitHub Pages in the nine languages (`docs/`, rendered from `docs/locales.ini` by `docs/.render.mjs`).

### Nine languages ([#33](https://github.com/navi-crwn/NoteHop/pull/33))
- The interface is available in English, 简体中文, Deutsch, Bahasa Indonesia, 日本語, Português (Brasil), Español, Français and Русский. German is new. The catalogs `zh` and `pt` are now `zh-CN` and `pt-BR`; the old codes `zh` and `pt` still work. Arabic and right-to-left layout are removed.
- `go test ./internal/i18n` checks that every catalog has the same keys, placeholders and tags as English.

### Shared home engine and HOP assets v2 ([#27](https://github.com/navi-crwn/NoteHop/pull/27), [#28](https://github.com/navi-crwn/NoteHop/pull/28), [#32](https://github.com/navi-crwn/NoteHop/pull/32))
- The home page's hero renderer and scroll motion come from the shared HOP home module, and the family manifest is synced to HOP assets v2 (`scripts/sync-hop-assets.sh`).

### Basic and encrypted notes ([#31](https://github.com/navi-crwn/NoteHop/pull/31))
- "New note" now starts with one choice: **Encrypted** (the default; locked in the browser, the key stays in the link after `#k=`, so the server can't read it) or **Basic** (readable by the server, with a link name you choose, like `/monday-meeting`). Every kind of note (plain text, document, code, website) exists in both. What used to be the "public note" switch hidden under Expert is now this choice, and "public" wording became "basic" everywhere.
- Only a basic note can have a link name. An encrypted note keeps the long link: the server refuses a name for it (`err.slug_basic_only`), in a new note and in "Add link".
- The share dialog renames a link with **Save** and **Cancel** (pencil on the row); "Add link" only creates. The old name is held for the owner for 90 days (`PUT /api/n/{token}/links/{link}`).
- The owner can change a note between basic and encrypted, both ways, from the share dialog (`POST /api/n/{token}/convert`). The browser re-encodes the text and the server swaps it in; links, keys and settings stay, version history is cleared. A note with added lines, suggestions, a decoy, burn blocks, an inbox or sign-offs can't change type yet (the server says which); snapshots are read-only.
- No note is ever indexed: `X-Robots-Tag: noindex, nofollow, noarchive` on every note, API, raw, mock and profile response (the robots meta tag was already on note pages), a `robots.txt` that keeps crawlers off the machine paths (note addresses are left crawlable on purpose, so the noindex header is seen), no sitemap, and link previews show only the kind and status of a note, never its title or text.
- My notes: filters for Encrypted and Basic, a type label on each row, and a basic note's read link is just its address.
- The CLI refuses `-slug` without `-public` before it calls the server.
- Copy: claims that "the server can't read your note" now say encrypted notes; the new text is translated in all nine catalogs (`docs/pending-translation-basic-notes.json` is the list of those keys and is no longer needed).

### Media and asset addresses ([#30](https://github.com/navi-crwn/NoteHop/pull/30))
- The avatar the account server sends (in the sign-on ticket or a profile push) is stored and reported to the control center as an absolute http(s) address: a path such as `/storage/avatars/x.png` is resolved against `NOTEHOP_SSO_URL`, and anything unusable is dropped instead of being cut in half.
- The offline worker no longer keeps `/raw/…`, `/mock/…` or `/in/…` pages (they carry a public note's text and use up a burn note's view), and it caches only complete (200) responses.

### Dashboard chrome ([#29](https://github.com/navi-crwn/NoteHop/pull/29))
- The signed-in dashboard (My notes, Vault key) has the same chrome as the other HOP dashboards: a sidebar with this app's menu and the HOP family (HOP ID, LinkHop, BioHop, PixelHop, with their marks), a top bar of settings-type controls (new note, language, theme, account menu) and a compact footer. The app grid is no longer in that top bar.
- The footer on every page spreads its five columns evenly; a flex rule had packed them to the left.

### Attachment storage limits ([#27](https://github.com/navi-crwn/NoteHop/pull/27))
- Attachments are limited per note (a shared 5 MB ciphertext budget for everything uploaded to one note), per address (`NOTEHOP_FILES_IP_MB`, 100 MB per 24 hours by default) and across the site (`NOTEHOP_FILES_TOTAL_MB`, at most 10 GB). The server also keeps a free-disk reserve (`NOTEHOP_MIN_FREE_MB`, `NOTEHOP_MIN_FREE_PERCENT`).
- When a limit would be exceeded, uploads answer `507` with `Retry-After: 60`. The editor shows a warning, disables file selection and checks again every 30 seconds.
- A file uploaded before its note exists is tied to the note by a one-time `X-Upload-Draft` capability. Database migration 12 adds `files.scope` and `file_drafts`.

## 2026-10-31

### Content warning gate ([#24](https://github.com/navi-crwn/NoteHop/pull/24))
- A note can carry a content warning: sensitive, or 18+ adult, with an optional reason of up to 255 characters. Readers see a gate with the reason and confirm before the note is decrypted; the confirmation is remembered for 24 hours on that device, and a form works without JavaScript.
- `/raw`, the mock API, SSH and live rooms honour the same gate. Link previews show the warning title instead of anything from the note.
- The label and the reason are stored readable, outside the ciphertext, so the gate can work before decryption.
- The control center can mark a note adult or sensitive, or clear the mark (`mark_nsfw`, `unmark_nsfw`, `mark_sensitive`).

### Product history and family experience ([#25](https://github.com/navi-crwn/NoteHop/pull/25), [#26](https://github.com/navi-crwn/NoteHop/pull/26))
- A `/changelog` page shows the commit, tag and pull request history from `web/changelog.json`, which `scripts/build-changelog.py` generates.
- The family switcher and the HOP family section list the other apps with their marks.

### Features grid, contact page and footer ([#23](https://github.com/navi-crwn/NoteHop/pull/23))
- `/features` is a responsive grid, `/contact` shows its channels as cards and links the private security advisory form, and every page has the same four-column footer with the language switcher.

### Documentation site ([#22](https://github.com/navi-crwn/NoteHop/pull/22))
- A GitHub Pages site for NoteHop in the nine languages, with theme-aware feature demos on the home page and screenshots in the README.

### Smaller changes ([#15](https://github.com/navi-crwn/NoteHop/pull/15) to [#20](https://github.com/navi-crwn/NoteHop/pull/20))
- English is the default language for new visitors. A language picked with `?lang=` or remembered in the cookie still wins; `Accept-Language` is no longer used ([#16](https://github.com/navi-crwn/NoteHop/pull/16)).
- Typography from the HOP family and a family section on the home page and in the footer ([#17](https://github.com/navi-crwn/NoteHop/pull/17)).
- The HOP family table in this README is checked against the canonical copy in HEL.ink by a CI job ([#18](https://github.com/navi-crwn/NoteHop/pull/18)), and PixelHop is listed at `pixel.hel.ink` ([#20](https://github.com/navi-crwn/NoteHop/pull/20)).
- `/features` and `/status` pages (version, start time and a live database check), back links on inner pages, and GitHub contact links on `/contact` ([#19](https://github.com/navi-crwn/NoteHop/pull/19)).
- `SECURITY.md` and structured issue forms ([#15](https://github.com/navi-crwn/NoteHop/pull/15)).

## 2026-10-29 and 2026-10-30

### Continuous integration ([#13](https://github.com/navi-crwn/NoteHop/pull/13))
- A GitHub Actions workflow runs `gofmt`, `go vet`, `go test -race` and the build on every push and pull request.

## 2026-10-28

### Chained logout ([#12](https://github.com/navi-crwn/NoteHop/pull/12))
- `GET /auth/sso/logout` ends the NoteHop session when you sign out at the account server or in another HOP app, then goes on to `?next=`, only when that is an https address on the account server's domain.

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

[Unreleased]: https://github.com/navi-crwn/NoteHop/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/navi-crwn/NoteHop/releases/tag/v1.0.0
