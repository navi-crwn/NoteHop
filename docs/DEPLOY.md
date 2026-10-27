# Deploy NoteHop ke VPS Linux

NoteHop adalah satu binary Go statis plus satu file SQLite. Di VPS ia berjalan sebagai service systemd di `127.0.0.1:8090`, di belakang Caddy atau nginx yang melayani HTTPS untuk domain Anda.

```
Internet ──HTTPS──► Caddy/nginx (:443) ──HTTP──► notehop (127.0.0.1:8090) ──► /var/lib/notehop/notehop.db
```

| File | Tujuan di VPS |
| --- | --- |
| `deploy/install.sh` | Pasang atau update dalam satu perintah |
| `deploy/notehop.service` | `/etc/systemd/system/notehop.service` (dengan hardening systemd) |
| `deploy/notehop.env.example` | `/etc/notehop/notehop.env` (semua pengaturan) |
| `deploy/notehop-backup.{service,timer}` + `deploy/backup.sh` | Backup harian ke `/var/lib/notehop/backups`, disimpan 14 hari |
| `deploy/Caddyfile.example` / `deploy/nginx.conf.example` | Reverse proxy + HTTPS |

## 1. Persiapan

- VPS Linux dengan systemd (Ubuntu/Debian/Rocky/dll.), akses root lewat SSH.
- DNS: buat record `A` (dan `AAAA` jika ada IPv6) untuk subdomain, misalnya `note.domainanda.com`, ke IP VPS.
- Firewall: buka hanya 22, 80, 443. Port 8090 **jangan** dibuka (NoteHop hanya mendengar di localhost).

```bash
sudo ufw allow OpenSSH && sudo ufw allow 80,443/tcp && sudo ufw enable
```

## 2. Pasang

**Cara A: build di VPS** (butuh Go 1.27+):

```bash
git clone https://github.com/navi-crwn/NoteHop.git && cd NoteHop
sudo DOMAIN=note.domainanda.com ./deploy/install.sh
```

**Cara B: build di komputer sendiri**, lalu kirim binary dan folder `deploy/`:

```bash
make linux                       # bin/notehop-linux-amd64 dan -arm64
scp -r bin/notehop-linux-amd64 deploy root@VPS:/root/notehop/
ssh root@VPS 'cd /root/notehop && mv notehop-linux-amd64 notehop && DOMAIN=note.domainanda.com ./deploy/install.sh ./notehop'
```

(Untuk cara B, `install.sh` dijalankan dari folder yang berisi `deploy/`.)

Skrip ini membuat user sistem `notehop`, memasang binary ke `/usr/local/bin/notehop`, membuat `/etc/notehop/notehop.env` (hanya jika belum ada), memasang dan menyalakan service + timer backup, lalu mengecek `/healthz`.

Periksa `/etc/notehop/notehop.env`, terutama `NOTEHOP_BASE_URL=https://note.domainanda.com`, lalu `sudo systemctl restart notehop`.

## 3. HTTPS

**Caddy** (paling mudah, sertifikat otomatis):

```bash
sudo apt install caddy
sudo cp deploy/Caddyfile.example /etc/caddy/Caddyfile   # ganti note.example.com
sudo systemctl reload caddy
```

Kalau Caddy sudah melayani situs lain, cukup tambahkan blok `note.domainanda.com { … }` ke Caddyfile yang ada.

**nginx** + certbot:

```bash
sudo cp deploy/nginx.conf.example /etc/nginx/sites-available/notehop   # ganti note.example.com
sudo ln -s /etc/nginx/sites-available/notehop /etc/nginx/sites-enabled/
sudo certbot --nginx -d note.domainanda.com
sudo nginx -t && sudo systemctl reload nginx
```

Kedua contoh ini dibuat supaya rate limit memakai IP asli pengunjung (`NOTEHOP_TRUST_PROXY=1`). Di nginx, `X-Forwarded-For` di-set ke `$remote_addr` (bukan ditambahkan), supaya pengunjung tidak bisa memalsukannya.

### Domain share terpisah (opsional)

Supaya link yang dibagikan ke orang lain memakai domain pendek terpisah (misalnya `nhop.link`), sementara login/dashboard tetap di `note.domainanda.com`:

1. Arahkan DNS domain kedua ke VPS yang sama, lalu tambahkan bloknya di Caddy/nginx (contoh ada di `deploy/Caddyfile.example`).
2. Isi `NOTEHOP_SHARE_URL=https://nhop.link` di `/etc/notehop/notehop.env`, lalu restart.

Di domain share, NoteHop hanya melayani `/n/…`, `/raw/…`, API catatan, dan file statis. Halaman lain diarahkan ke domain utama, dan cookie sesi tidak pernah dikirim ke sana. Kalau domain share diblokir karena isi kiriman orang lain, situs utama tetap aman.

## 4. Cek

```bash
systemctl status notehop
journalctl -u notehop -f
curl -I https://note.domainanda.com/healthz
notehop -version
```

Buka situsnya, buat catatan, dan pastikan link `#k=…` bisa dibuka di browser lain.

## 5. Update

```bash
cd NoteHop && git pull
sudo ./deploy/install.sh            # atau: sudo ./deploy/install.sh ./notehop (binary baru)
```

Sebelum restart, skrip menyalin database ke `/var/lib/notehop/backups/pre-update-*.db`. Migrasi skema berjalan otomatis saat start. Binary lama menolak membuka database yang sudah dimigrasi oleh binary yang lebih baru; untuk mundur versi, pulihkan backup `pre-update`.

## 6. Backup & restore

- Otomatis setiap hari sekitar pukul 03:17 (waktu server): `systemctl list-timers notehop-backup`.
- Manual: `sudo systemctl start notehop-backup`, atau langsung `sudo -u notehop notehop backup -db /var/lib/notehop/notehop.db /tmp/salinan.db` (aman saat service berjalan, tidak butuh `sqlite3`).
- **Simpan juga di luar VPS.** Backup di disk yang sama tidak menolong kalau VPS hilang. Contoh dengan rclone (cron harian root):
  `rclone copy /var/lib/notehop/backups remote:notehop-backups --max-age 48h`
- Restore:

```bash
sudo systemctl stop notehop
gunzip -c /var/lib/notehop/backups/notehop-YYYYMMDD-HHMMSS.db.gz | sudo -u notehop tee /var/lib/notehop/notehop.db >/dev/null
sudo rm -f /var/lib/notehop/notehop.db-wal /var/lib/notehop/notehop.db-shm
sudo systemctl start notehop
```

Isi catatan di backup tetap terenkripsi; kuncinya hanya ada di link milik pengguna.

## 7. Opsional: single sign-on dengan server akun sendiri

NoteHop memakai akunnya sendiri selama `NOTEHOP_SSO_URL` kosong. Kalau Anda punya server akun yang bisa mengeluarkan tiket NoteHop:

1. Buat rahasia: `openssl rand -hex 32`.
2. Isi di `/etc/notehop/notehop.env`: `NOTEHOP_SSO_URL=https://akun.domainanda.com`, `NOTEHOP_SSO_SECRET=<rahasia>`, `NOTEHOP_SSO_NAME=<nama di tombol>`. Lalu `sudo systemctl restart notehop`.
3. Di server akun: daftarkan NoteHop dengan rahasia yang sama dan callback `https://note.domainanda.com/auth/sso/callback`.

Server akun menerima `GET <SSO_URL>/sso/<NOTEHOP_SSO_CLIENT>?state=…&return_to=…`, membuat pengguna masuk, lalu mengalihkan ke `return_to?ticket=…`. Tiket: `base64url(JSON).base64url(HMAC-SHA256(rahasia, bagian pertama))` dengan `sub`, `email`, `name`, `avatar`, `state`, `iat`, `exp` (≤ 2 menit), `jti`, dan opsional `aud` (= `NOTEHOP_SSO_CLIENT`).

## Docker (opsional)

Ada `Dockerfile` untuk yang lebih suka container: `docker build -t notehop .` lalu `docker run -d -p 127.0.0.1:8090:8080 -v notehop:/data -e NOTEHOP_BASE_URL=https://note.domainanda.com -e NOTEHOP_TRUST_PROXY=1 notehop`. Untuk VPS biasa, cara systemd di atas lebih sederhana.

## Mengerjakan langsung di VPS dengan Claude

Sesi Claude Code di cloud tidak bisa SSH ke VPS. Kalau ingin Claude menginstal dan mengetes langsung di server, pasang Claude Code di VPS lalu jalankan `claude remote-control` di folder repo. Sesi itu akan muncul di aplikasi Claude Code dan berjalan di VPS Anda.

