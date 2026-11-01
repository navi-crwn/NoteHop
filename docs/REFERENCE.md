# NoteHop reference

Settings, commands, the encryption model and the HTTP API. For installing on a server, see [SELF-HOSTING.md](SELF-HOSTING.md).

## Settings

Every setting is an environment variable. On a VPS they live in `/etc/notehop/notehop.env`.

| Variable | Default | What it does |
| --- | --- | --- |
| `NOTEHOP_ADDR` | `:8080` | Address to listen on (or `-addr`) |
| `NOTEHOP_DB` | `notehop.db` | SQLite file (or `-db`) |
| `NOTEHOP_BASE_URL` | empty | Public URL, for example `https://note.example.com` |
| `NOTEHOP_SECURE_COOKIES` | `true` when BASE_URL is https | `Secure` cookies + HSTS |
| `NOTEHOP_TRUST_PROXY` | `false` | Use `X-Forwarded-For` for rate limits (turn on behind a reverse proxy) |
| `NOTEHOP_REGISTRATION` | `true` | Open or close sign-ups |
| `NOTEHOP_SITE` | `NoteHop` | Site name |
| `NOTEHOP_SHARE_URL` | empty | A short domain for shared links (for example `https://nhop.link`). Reader links become `https://nhop.link/TOKEN`, which redirects to `NOTEHOP_BASE_URL/TOKEN`; nothing is served on it. Owner links stay on `NOTEHOP_BASE_URL` |
| `NOTEHOP_FILE_MB` | `5` | Largest attachment (MB), capped at 5. `0` turns attachments off. All attachments uploaded for one note share a 5 MB ciphertext budget, including encryption overhead |
| `NOTEHOP_FILES_IP_MB` | `100` | Attachments one IP address may upload per 24 hours (MB). `0` = no limit |
| `NOTEHOP_FILES_TOTAL_MB` | `10240` | Shared budget across all users, capped at 10 GB. Nonpositive values use the cap. Expired blobs count until deleted by the janitor |
| `NOTEHOP_MIN_FREE_MB` | `1024` | Minimum free space reserved on the SQLite filesystem; nonpositive values use the default |
| `NOTEHOP_MIN_FREE_PERCENT` | `5` | Reserved disk percentage; the greater of this and the MB reserve applies |
| `NOTEHOP_AI` | `on` | Writing help: `on` (on-device AI + own API key; the editor page's `connect-src` opens for the providers in `middleware.go`), `device` (browser's built-in AI only), `off` (no AI button) |
| `NOTEHOP_AI_ORIGINS` | empty | Extra origins for the "Other" (OpenAI-compatible) provider, for example `https://ai.company.example`. Origins only, comma-separated |
| `NOTEHOP_GITHUB` | `navi-crwn/NoteHop` | GitHub repository (`owner/name`) whose numbers show on the home page through `GET /api/repo`. Fetched at most once an hour. `-` hides that section |
| `NOTEHOP_SSH_ADDR` | empty | Turn on SSH for public notes, for example `:2222` |
| `NOTEHOP_SSH_KEY` | `ssh_host_ed25519_key` next to the DB | SSH host key (made on first start) |
| `NOTEHOP_PRIVATE_HOOKS` | `false` | Allow Watch webhooks to private/loopback addresses and `http://` (own network only) |
| `TELEGRAM_BOT_TOKEN` | empty | Bot token from @BotFather; needs `NOTEHOP_BASE_URL` (the webhook is registered at start) |
| `NOTEHOP_ADMINS` | empty | Usernames given the **admin** role at start, if those accounts already exist (comma-separated). Admins may use short (< 7) and reserved link names, and reserved usernames. The list grants nothing by itself: the role lives in the `users.role` column, and is also set from the account server's roles (`owner`, `admin`, `admin:<client>`) at sign-on and through `/hop/profile`, so registering a listed name later makes nobody an admin |
| `NOTEHOP_HOP_CLIENTS` | `imperium,helink` | Apps that may call the signed `/hop/*` API with `NOTEHOP_SSO_SECRET`: the control center and the account server. See [Control center](#control-center-hop-app-protocol) |
| `NOTEHOP_RESERVED` | empty | Extra reserved names (comma-separated), besides `internal/app/names/reserved.txt` |
| `NOTEHOP_CONTACT` | empty | Email or URL for contact and abuse reports, shown on `/contact` |
| `NOTEHOP_SSO_URL` | empty | Account server for single sign-on ("Continue with …"). Empty = NoteHop's own accounts only |
| `NOTEHOP_SSO_SECRET` | empty | Secret shared with the account server (at least 32 characters). SSO also needs `NOTEHOP_BASE_URL` |
| `NOTEHOP_SSO_NAME` | `SSO` | Account server's name on the sign-in button |
| `NOTEHOP_SSO_PROVIDERS` | empty | Extra buttons straight to a provider on the account server, for example `google=Google`: visitors go to `<SSO_URL>/sso/<client>/google` |
| `NOTEHOP_SSO_CLIENT` | `notehop` | This app's name on the account server: visitors go to `<SSO_URL>/sso/<client>`, the ticket comes back to `/auth/sso/callback` |
| `NOTEHOP_LINKS` | empty | Related sites in the footer, `Name=URL`, comma-separated |

Uploads return HTTP 507 with `Retry-After: 60` when the shared budget would be exceeded or the server disk cannot safely accept the upload. The editor shows a warning, disables file selection and checks again every 30 seconds. No restart is needed after capacity recovers. Existing downloads and text editing remain available. Binary units are used (MB = MiB, GB = GiB).

Uploads before note creation require a random 43-character `X-Upload-Draft` capability. Send it as `upload_draft` when creating the note; it is claimed transactionally and cannot be reused. Uploads to an existing note use `POST /api/n/{token}/files` with the usual `X-Write` and optional `X-Admin` proofs. Per-note accounting includes all unexpired uploads, even when their reference is removed from the encrypted text. The server cannot inspect encrypted note text or prevent readers from copying existing attachment references.

Operational recovery: check filesystem capacity and the janitor, remove expired or unwanted attachments through the existing admin controls, or expand storage. SQLite may reuse freed pages without shrinking its file; disk reserve remains enforced independently of the attachment byte budget. Do not delete live database/WAL files to recover space.

The database schema migrates itself at start (version in `PRAGMA user_version`, steps in `migrations` in `internal/db/db.go`). An older binary refuses to open a database migrated by a newer one.

### Languages

The UI comes in 9 languages: English (default and fallback), 简体中文 (zh-CN), Deutsch, Bahasa Indonesia, 日本語, Português (pt-BR), Español, Français and Русский. The old codes `zh` and `pt` still work and map to `zh-CN` and `pt-BR`. Anonymous visitors get English regardless of `Accept-Language`; the globe menu stores a choice in the `nh_lang` cookie, and `?lang=xx` sets that cookie. Both explicit choices always win over the default. Texts live in `web/i18n/*.json`, and `go test ./internal/i18n` checks that every file has the same keys, placeholders and tags. API errors come as `{error, code, vars}` and are translated in the browser.

## Commands

```bash
notehop -addr 127.0.0.1:8090 -db notehop.db                         # run the server
notehop backup -db notehop.db copy.db                               # safe copy while it runs
notehop new -server https://note.example.com < note.md              # encrypted in the terminal, prints the link
notehop new -server https://… -password secret -expires 24h -burn 1 file.md
notehop get 'https://note.example.com/TOKEN#k=…'                    # download and decrypt in the terminal
ssh -p 2222 note.example.com < note.md                              # ⚠️ public note (when NOTEHOP_SSH_ADDR is set)
notehop takedown -db notehop.db https://note.example.com/xxxx       # moderation: delete + hold the link name for 365 days
notehop takedown -db notehop.db -file FILE_ID                       # moderation: delete one attachment
```

Link names and usernames (at least 7 characters, reserved names, a word filter in 9 languages, a 90-day hold after a note is deleted) are handled in `internal/app/names.go`.

## How the encryption works

```
K      = 32 random bytes                       → in the link: /<token>#k=<K>
secret = K, or PBKDF2(password, K‖salt, 310000) with a password
enc    = HMAC-SHA256(secret, "notehop/enc")    → AES-256-GCM key for the note and its replies
write  = HMAC-SHA256(secret, "notehop/write")  → sent as X-Write; the server keeps its SHA-256
admin  = 32 random bytes                       → owner link: #k=<K>&a=<admin>; the server keeps its SHA-256
```

- Every change needs `X-Write` (proof of the key) **and** a link mode that allows it, so an easy-to-guess link name still can't be written to without the key.
- Notes with a password: `POST /open` also needs `X-Write`, so a wrong password is refused **without** using up a view.
- Accounts: `PBKDF2(password, "notehop/account/"+username, 600000)` → 64 bytes: the first 32 are the vault key (stays in the browser), the last 32 the login secret (argon2id-hashed on the server).
- Single sign-on accounts: NoteHop never sees their password, so the vault key comes from a separate phrase: `vault = PBKDF2(phrase, "notehop/vault/"+sso_id, 600000)`. The server only gets `HMAC(vault, "notehop/vault-check")` (argon2id-hashed) to tell a right phrase from a typo.
- Password change and recovery: the vault key itself never changes. After a change, the first 32 bytes of the password's PBKDF2 become a wrapping key, and `users.vault_wrap = AES-GCM(that key, vault key)`. Recovery codes are 16 characters (80 bits); `HKDF-SHA256(code, salt "notehop/recovery/"+username)` → 32 bytes that seal the vault key (`recovery_codes.wrap`) + 32 bytes of proof, of which the server keeps only the SHA-256.
- PIN blocks: `salt.b64 + "." + AES-GCM(PBKDF2(PIN, salt, 250000))`.

What the server can see: the ciphertext's size, times, view counts, link modes, which checkboxes are ticked (index + true/false), and the number of replies.

## API

Everything is JSON. Headers: `X-Admin` (owner token), `X-Write` (proof of the key).

| Method & path | Who | What it does |
| --- | --- | --- |
| `POST /api/notes` | anyone | Create a note `{encrypted, password, salt, content, mode, slug, expires_in, max_views, admin, write, owner_blob}` |
| `GET /api/n/{token}` | anyone | Metadata without content (owner: + links, snapshots) |
| `POST /api/n/{token}/open` | anyone (+X-Write with a password) | Content + replies + checks; counts a view. `{"poll":true}` doesn't count (refused for delete-after-reading notes) |
| `PUT /api/n/{token}` | edit mode / owner + X-Write | Save `{content, version}`; 409 when the version is stale |
| `POST /api/n/{token}/entries` | edit/append/owner + X-Write | Add a reply |
| `DELETE /api/n/{token}/entries/{id}` | owner | Delete a reply |
| `POST /api/n/{token}/checks` | edit/check/owner + X-Write | `{index, checked}` |
| `POST /api/n/{token}/freeze` | owner | Make a frozen copy |
| `POST /api/n/{token}/config` | owner | Optional features, partial body (unix time, 0/empty = off): `lock_at`, `open_at` (425 before), `sign_off`, `ip_allow` (IP/CIDR), `reset` (`daily`/`weekly`/`monthly`), `dms_days`, `watermark`, `check_in` |
| `GET /api/n/{token}/versions` · `…/versions/{v}` | owner / edit mode + X-Write | Version history (old ciphertext, at most 100, at least 5 minutes apart) |
| `POST /api/n/{token}/suggestions` · `GET` · `DELETE …/{id}` | suggest mode + X-Write / owner | Encrypted suggested edits; the owner reviews them |
| `POST /api/n/{token}/signatures` | X-Write, notes with `sign_off` | Sign `{name}`; the first signature makes a frozen copy |
| `POST /api/n/{token}/votes` | X-Write | `{poll, option, voter}` for `:::poll` blocks; `voter` is a random browser ID, one vote per voter (can be changed), at most 20 new voters per network |
| `POST /api/n/{token}/burns` · `…/burns/{id}/reveal` | owner / X-Write | `:::burn` blocks: the block's key is stored apart, opened once, then deleted |
| `PUT` · `DELETE /api/n/{token}/decoy` | owner | Decoy password: a second ciphertext; its openings are logged for the owner |
| `POST /api/n/{token}/claim` | account owner + X-Admin | Move a guest note into the account |
| `POST /api/n/{token}/links` · `DELETE …/links/{t}` | owner | Add / turn off a link |
| `DELETE /api/n/{token}` | owner or account owner | Delete the note |
| `GET` · `PUT /api/margins/{ref}` | account | Private margin notes (vault-encrypted) next to anyone's note |
| `GET /api/n/{token}/live` (WebSocket) | key holders | Live collaboration. First message `{t:"auth", write, admin}`, then `{t, to, d}` with `d` encrypted. Types: `y` (Yjs update, writers only), `need`/`sync`/`nosync` (initial state), `aw` (presence/cursors), `chat`, `follow`. Not for frozen or delete-after-reading notes |
| `GET /api/nearby` (WebSocket) | anyone, per public IP + 6-digit `?code=` | WebRTC signaling for `/nearby` (`hi`, `sig`); pads and files go browser to browser |
| `POST /api/files` | anyone (20/min/IP) | Upload an attachment: the body is raw ciphertext (`application/octet-stream`). Answers `{id, expires}`; the key is only in the note's text (`nh-file:ID#KEY`) |
| `GET /api/files/{id}` | anyone who knows the ID | The attachment's ciphertext (404 after 24 hours) |
| `POST /api/n/{token}/inbox` · `DELETE` | owner | Turn on an inbox `{pub, priv}` (P-256 public key + the private key sealed by the browser) / turn it off |
| `POST /in/{token}` | anyone who knows the address (60/min/IP) | Raw text or JSON `{text, from}` → an encrypted reply |
| `POST /api/telegram/{secret}` | Telegram | Bot webhook (checked with `X-Telegram-Bot-Api-Secret-Token`) |
| `GET /mock/{token}/{path…}` | anyone | JSON from a public note, CORS `*` |
| `GET /raw/{token}` | anyone | Plain text (public notes only) |
| `GET /@{username}` | anyone | Public profile (the public notes its owner picked) |
| `GET /api/repo` | anyone | GitHub numbers for the home page (cached for an hour): `{repo, url, stars, forks, issues, license, pushed, langs, weeks}`. 404 when `NOTEHOP_GITHUB=-`, 503 before the first successful fetch |
| `POST /api/register` · `/api/login` · `/api/logout`, `GET /api/me`, `GET /api/me/notes` | | Accounts |
| `GET /auth/sso` · `GET /auth/sso/callback` | | Single sign-on (HMAC ticket, `aud`, `state` + `nh_sso` cookie, one-time `jti`) |
| `POST /api/vault` | SSO account | `{check}`: store the vault phrase check the first time (201), or verify it (200 / 401). The answer includes `vault_wrap` |
| `POST /api/me/recovery` | account | `{current, codes:[{proof, wrap}]×8}`: replace all recovery codes (needs the current secret) |
| `POST /api/me/secret` | account | `{current, next, vault_wrap}`: change the password (or the SSO vault key); other devices are signed out |
| `POST /api/recover` | public | `{username, proof}` (SSO accounts: no username, must be signed in): check a code → `{wrap, ticket}` |
| `POST /api/recover/finish` | public | `{ticket, next, vault_wrap}`: new password, the code is used up, every session ends and a new one starts |
| `GET /` · `/new` · `/diff` · `/nearby` · `/faq` | anyone | Home page, full create page, text compare, nearby devices, FAQ |
| `GET /sw.js` · `/manifest.webmanifest` · `/offline` | | Installable app (PWA) |
| `POST /api/n/{token}/report` | anyone (5 per hour per IP) | **Report this note** `{reason: illegal\|abuse\|spam\|copyright\|other, note (≤ 500)}`: queued for the operator, never includes the note's content |
| `GET /healthz` | anyone | Health check |
| `/hop/*` | control center / account server (signed) | See below |

## Control center (HOP App Protocol)

With single sign-on configured, NoteHop also speaks the HOP App Protocol v1 under `/hop/*`, so a control center can read and act on it and the account server can push account events. Every call is signed with `NOTEHOP_SSO_SECRET` by one of the apps in `NOTEHOP_HOP_CLIENTS` (`X-HOP-App`, `X-HOP-Timestamp`, `X-HOP-Nonce`, `X-HOP-Actor`, `X-HOP-Signature` = base64url HMAC-SHA256 of `hop1|app|ts|nonce|METHOD|path-with-query|sha256hex(body)`; 120 s max age, nonces work once). There are no cookies and no CSRF check here. Replies are `{ok:true, data, page?}` or `{ok:false, error, message}` (401 `bad_signature` / `stale_request` / `replay`, 404 `not_configured` while SSO is off). Times are ISO-8601 UTC; users are identified by their account-server id (`sub`) and `local_id` (password accounts: `local:<id>`). Everything that changes data lands in the `hop_audit` table.

**Zero-knowledge still holds:** an encrypted note is listed with its metadata and `encrypted: true`, never its content; `title` and `preview` are filled only for public notes.

| Endpoint | What it does |
| --- | --- |
| `GET /hop/health` · `/hop/manifest` · `/hop/stats` · `/hop/metrics` | Health, self-description (content types, actions, extensions, settings), KPIs with a 14-day series, Prometheus text |
| `GET /hop/users?q&cursor&limit` · `GET /hop/users/{sub}` | Accounts (username or email contains `q`): role, status, vault set, notes, sessions |
| `POST /hop/users/{sub}/actions` | `{action, reason, params}`: `revoke_sessions`, `delete_notes`, `purge`, `set_role` (`params.role` admin/user), `reset_vault` |
| `POST /hop/profile` | Profile push from the account server: email, name, avatar, `username` (only when free per the name rules, else `username_kept: true`), `roles` (promotes **and** demotes), `status` |
| `POST /hop/events` | `{id, type, at, data}`, idempotent by `id`: `profile.updated`, `account.status` (`suspended` signs out and blocks sign-in), `account.deleted` (purge), `session.revoked` |
| `GET /hop/content?type=note\|file&q&owner&status&cursor&limit` · `GET /hop/content/note/{id}` · `GET /hop/content/file/{id}` | Notes by link-name prefix or public title; `status` = encrypted, public, expiring, frozen, burn. Attachments: id, size, created, expires |
| `POST /hop/content/note/{id}/actions` · `…/file/{id}/actions` | Notes: `delete`, `takedown` (name held 365 days, like the CLI), `freeze` (read-only for everyone), `set_expiry` (`params.expires` ISO or null), `release_slug`, `disable_inbox`, `disable_watch`. Files: `delete` |
| `GET /hop/reports?status&cursor` · `POST /hop/reports/{id}/actions` | Reader reports; `dismiss`, `resolve`, `takedown` |
| `GET /hop/settings` · `PUT /hop/settings` | Environment values, read-only, plus `registration`, the one setting the control center may change (stored in the `settings` table, overriding `NOTEHOP_REGISTRATION`) |
| `GET /hop/search?q` · `GET /hop/audit?cursor` | Exact link name, public titles, usernames and emails; the audit log |
| `GET /hop/x/names` · `POST /hop/x/names/holds/{slug}/actions` | Reserved names, blocked-word count, holds and takedown holds; `release` |
| `GET /hop/x/files` · `/hop/x/realtime` · `/hop/x/integrations` | Attachment usage and budget; live rooms and nearby networks; Telegram, SSH and webhook status |

## Third-party code

Vendored in `web/static/vendor/`: marked 15.0.12 (MIT), DOMPurify 3.2.6 (Apache-2.0/MPL-2.0), highlight.js 11.11.1 (BSD-3-Clause), qrcode-generator 1.4.4 (MIT), Mermaid 11.12.0 (MIT, runs in a sandboxed frame), Yjs 13.6.27 + lib0 (MIT). Go modules: modernc.org/sqlite (BSD-3-Clause), golang.org/x/crypto (BSD-3-Clause), github.com/coder/websocket (ISC). Fonts in `web/static/fonts/` are under the SIL Open Font License.
