# Running NoteHop on your own server

NoteHop is one static Go binary plus one SQLite file. On a VPS it runs as a systemd service on `127.0.0.1:8090`, behind Caddy or nginx, which serve HTTPS for your domain.

```
Internet ──HTTPS──► Caddy/nginx (:443) ──HTTP──► notehop (127.0.0.1:8090) ──► /var/lib/notehop/notehop.db
```

| File | Where it goes on the VPS |
| --- | --- |
| `deploy/install.sh` | Install or update in one command |
| `deploy/notehop.service` | `/etc/systemd/system/notehop.service` (with systemd hardening) |
| `deploy/notehop.env.example` | `/etc/notehop/notehop.env` (every setting) |
| `deploy/notehop-backup.{service,timer}` + `deploy/backup.sh` | Daily backup to `/var/lib/notehop/backups`, kept for 14 days |
| `deploy/Caddyfile.example` / `deploy/nginx.conf.example` | Reverse proxy + HTTPS |

## 1. Before you start

- A Linux VPS with systemd (Ubuntu, Debian, Rocky…) and root access over SSH.
- DNS: an `A` record (and `AAAA` with IPv6) for a subdomain, for example `note.example.com`, pointing at the VPS.
- Firewall: open only 22, 80 and 443. **Don't** open 8090: NoteHop only listens on localhost.

```bash
sudo ufw allow OpenSSH && sudo ufw allow 80,443/tcp && sudo ufw enable
```

## 2. Install

**Way A: build on the VPS** (needs Go 1.27+):

```bash
git clone https://github.com/navi-crwn/NoteHop.git && cd NoteHop
sudo DOMAIN=note.example.com ./deploy/install.sh
```

**Way B: build on your own computer**, then send the binary and the `deploy/` folder:

```bash
make linux                       # bin/notehop-linux-amd64 and -arm64
scp -r bin/notehop-linux-amd64 deploy root@VPS:/root/notehop/
ssh root@VPS 'cd /root/notehop && mv notehop-linux-amd64 notehop && DOMAIN=note.example.com ./deploy/install.sh ./notehop'
```

(For way B, run `install.sh` from the folder that holds `deploy/`.)

The script creates a `notehop` system user, installs the binary to `/usr/local/bin/notehop`, creates `/etc/notehop/notehop.env` (only if it doesn't exist yet), installs and starts the service and the backup timer, then checks `/healthz`.

Check `/etc/notehop/notehop.env`, especially `NOTEHOP_BASE_URL=https://note.example.com`, then `sudo systemctl restart notehop`. Every setting is listed in [REFERENCE.md](REFERENCE.md#settings).

## 3. HTTPS

**Caddy** (easiest, certificates are automatic):

```bash
sudo apt install caddy
sudo cp deploy/Caddyfile.example /etc/caddy/Caddyfile   # replace note.example.com
sudo systemctl reload caddy
```

If Caddy already serves other sites, just add a `note.example.com { … }` block to the Caddyfile you have.

**nginx** + certbot:

```bash
sudo cp deploy/nginx.conf.example /etc/nginx/sites-available/notehop   # replace note.example.com
sudo ln -s /etc/nginx/sites-available/notehop /etc/nginx/sites-enabled/
sudo certbot --nginx -d note.example.com
sudo nginx -t && sudo systemctl reload nginx
```

Both examples make rate limits use the visitor's real IP (`NOTEHOP_TRUST_PROXY=1`). In nginx, `X-Forwarded-For` is *set* to `$remote_addr` (not appended), so visitors can't fake it.

### A short domain for shared links (optional)

Notes live at `note.example.com/link-name` (or `/TOKEN`). To make the links people share shorter, for example `nhop.link/link-name`:

1. Point the second domain's DNS at the same VPS and add its block in Caddy or nginx (there's an example in `deploy/Caddyfile.example`). It can proxy to NoteHop like the main domain, or simply redirect: `return 301 https://note.example.com$request_uri;`.
2. Set `NOTEHOP_SHARE_URL=https://nhop.link` in `/etc/notehop/notehop.env` and restart.

Links for readers then look like `https://nhop.link/rapat-senin#k=…`. Opening one redirects to `https://note.example.com/rapat-senin#k=…`, where the note opens; the browser keeps the `#…` part (key and rights). Owner links stay `note.example.com/…`. The short domain serves no pages and holds no sessions: everything on it goes to the same path on the main site.

Older links keep working: `note.example.com/n/TOKEN` redirects to `note.example.com/TOKEN`, whatever rights the link carries. Link names are the same on both domains, so removing or reusing a name affects its short link too.

## 4. Check

```bash
systemctl status notehop
journalctl -u notehop -f
curl -I https://note.example.com/healthz
notehop -version
```

Open the site, make a note, and check that the `#k=…` link opens in another browser.

## 5. Update

```bash
cd NoteHop && git pull
sudo ./deploy/install.sh            # or: sudo ./deploy/install.sh ./notehop (a new binary)
```

Before restarting, the script copies the database to `/var/lib/notehop/backups/pre-update-*.db`. Schema migrations run by themselves at start. An older binary refuses to open a database a newer one has migrated; to go back a version, restore the `pre-update` backup.

## 6. Backup and restore

- Automatic, every day at about 03:17 (server time): `systemctl list-timers notehop-backup`.
- By hand: `sudo systemctl start notehop-backup`, or `sudo -u notehop notehop backup -db /var/lib/notehop/notehop.db /tmp/copy.db` (safe while the service runs, no `sqlite3` needed).
- **Keep copies off the VPS too.** A backup on the same disk doesn't help if the VPS is gone. For example with rclone (daily root cron):
  `rclone copy /var/lib/notehop/backups remote:notehop-backups --max-age 48h`
- Restore:

```bash
sudo systemctl stop notehop
gunzip -c /var/lib/notehop/backups/notehop-YYYYMMDD-HHMMSS.db.gz | sudo -u notehop tee /var/lib/notehop/notehop.db >/dev/null
sudo rm -f /var/lib/notehop/notehop.db-wal /var/lib/notehop/notehop.db-shm
sudo systemctl start notehop
```

Notes stay encrypted in the backup; their keys exist only in the links people hold.

## 7. Optional: single sign-on with your own account server

NoteHop uses its own accounts while `NOTEHOP_SSO_URL` is empty. If you run an account server that can issue NoteHop tickets:

1. Make a secret: `openssl rand -hex 32`.
2. In `/etc/notehop/notehop.env`: `NOTEHOP_SSO_URL=https://accounts.example.com`, `NOTEHOP_SSO_SECRET=<secret>`, `NOTEHOP_SSO_NAME=<name on the button>`. Then `sudo systemctl restart notehop`.
3. On the account server: register NoteHop with the same secret and the callback `https://note.example.com/auth/sso/callback`.

The account server receives `GET <SSO_URL>/sso/<NOTEHOP_SSO_CLIENT>?state=…&return_to=…`, signs the user in, then redirects to `return_to?ticket=…`. The ticket is `base64url(JSON).base64url(HMAC-SHA256(secret, first part))` with `sub`, `email`, `name`, `avatar` (an absolute https address; a path is resolved against `NOTEHOP_SSO_URL`), `state`, `iat`, `exp` (≤ 2 minutes), `jti`, and optionally `aud` (= `NOTEHOP_SSO_CLIENT`).

## Docker (optional)

There's a `Dockerfile` for those who prefer containers:

```bash
docker build -t notehop .
docker run -d -p 127.0.0.1:8090:8080 -v notehop:/data -e NOTEHOP_BASE_URL=https://note.example.com -e NOTEHOP_TRUST_PROXY=1 notehop
```

On a plain VPS, the systemd setup above is simpler.
