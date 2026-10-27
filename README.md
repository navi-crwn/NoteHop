# NoteHop

Catatan online yang bisa dibagikan dan **dienkripsi di browser**. Server hanya menyimpan acakan (ciphertext); kunci ada di bagian `#` link dan tidak pernah dikirim ke server. Satu binary Go + satu file SQLite, tanpa build step frontend.

Satu keluarga dengan [ImageApp](https://github.com/navi-crwn/imageapp), [SSO](https://github.com/navi-crwn/accounts), dan [OtherApp](https://github.com/navi-crwn/otherapp).

## Fitur (Fase 1)

- **Zero-knowledge**: AES-256-GCM di browser, kunci 256-bit di `#k=…`. Password tambahan opsional (PBKDF2 310k).
- **Banyak link per catatan, izin berbeda**: Edit penuh · Baca saja · **Tambah saja** (append-only) · **Centang checklist** (terkunci, tapi checkbox bisa dicentang). Link bocor bisa dimatikan tanpa mengganggu link lain.
- **Link admin** terpisah: edit, bagikan, bekukan, hapus.
- **Bekukan versi (snapshot)**: salinan permanen baca saja, catatan asli tetap bisa diedit.
- **Hancur setelah dibaca** (1/3/10×) dan **kedaluwarsa** (10 menit sampai 30 hari). Aman dari bot preview chat, dan hitungan view atomik.
- **Alamat kustom** `/n/rapat-senin` + **QR code** (dibuat di browser).
- **`/raw/slug`** untuk catatan publik (`curl`).
- **Editor**: Markdown, menu `/` (judul, to-do, tabel, kode, spoiler, blok PIN, tanggal…), lanjutan list otomatis, paste tabel dari Excel/Sheets jadi tabel Markdown, pratinjau berdampingan, highlight kode otomatis.
- **Spoiler** `:::spoiler` dan **blok terkunci PIN** `:::locked` (dienkripsi lagi dengan PIN).
- **Akun opsional**: password tidak dikirim ke server; daftar catatan dienkripsi dengan kunci brankas lokal.
- **Keamanan**: CSP ketat, tanpa inline script, `noindex`, `Referrer-Policy: no-referrer`, gambar pihak ketiga diblokir, tolak permintaan lintas situs, rate limit.

Rencana fitur lengkap ada di [ROADMAP.md](ROADMAP.md). Integrasi login SSO dan ImageApp ada di [docs/INTEGRASI-SSO-IMAGEAPP.md](docs/INTEGRASI-SSO-IMAGEAPP.md).

## Menjalankan

Butuh Go 1.27+.

```bash
go run ./cmd/notehop -addr 127.0.0.1:8090 -db data/dev.db
```

Buka http://127.0.0.1:8090. WebCrypto hanya jalan di **HTTPS atau localhost**.

```bash
make test      # go test -race ./...
make build     # bin/notehop
```

### Konfigurasi (env)

| Variabel | Default | Arti |
| --- | --- | --- |
| `NOTEHOP_ADDR` | `:8080` | Alamat listen (atau `-addr`) |
| `NOTEHOP_DB` | `notehop.db` | File SQLite (atau `-db`) |
| `NOTEHOP_BASE_URL` | kosong | URL publik, misalnya `https://note.example.com` |
| `NOTEHOP_SECURE_COOKIES` | otomatis `true` jika BASE_URL https | Cookie `Secure` + HSTS |
| `NOTEHOP_TRUST_PROXY` | `false` | Pakai `X-Forwarded-For` untuk rate limit (aktifkan di belakang reverse proxy) |
| `NOTEHOP_REGISTRATION` | `true` | Buka/tutup pendaftaran akun |
| `NOTEHOP_SITE` | `NoteHop` | Nama situs |
| `NOTEHOP_SSO_URL` | kosong | Alamat SSO untuk "Masuk dengan SSO", misalnya `https://accounts.example.com`. Kosong = fitur mati |
| `NOTEHOP_SSO_SECRET` | kosong | Rahasia bersama (minimal 32 karakter), sama dengan `NOTEHOP_SSO_SECRET` di SSO. SSO juga butuh `NOTEHOP_BASE_URL` |

Skema database dimigrasi otomatis saat start (nomor versi di `PRAGMA user_version`, daftar di `migrations` pada `internal/db/db.go`). Binary yang lebih lama menolak membuka database dengan versi skema yang lebih baru.

### Deploy ke VPS

Panduan lengkap (systemd, Caddy/nginx, backup harian, update, restore): **[`docs/DEPLOY.md`](docs/DEPLOY.md)**. Ringkasnya:

```bash
sudo DOMAIN=note.example.com ./deploy/install.sh   # pasang/update sebagai service systemd
make linux                                         # atau build binary statis untuk diunggah
notehop backup -db notehop.db salinan.db           # backup aman saat berjalan
```

## Cara kerja kriptografi

```
K      = 32 byte acak                      → di link: /n/<token>#k=<K>
secret = K, atau PBKDF2(password, K‖salt, 310000) jika ada password
enc    = HMAC-SHA256(secret, "notehop/enc")   → kunci AES-256-GCM isi & baris tambahan
write  = HMAC-SHA256(secret, "notehop/write") → dikirim di header X-Write; server simpan SHA-256-nya
admin  = 32 byte acak                      → link admin: #k=<K>&a=<admin>; server simpan SHA-256-nya
```

- Setiap perubahan butuh `X-Write` (membuktikan pemegang kunci) **dan** mode link yang mengizinkan. Jadi slug yang mudah ditebak tetap tidak bisa dipakai menulis tanpa kunci.
- Catatan berpassword: `POST /open` juga butuh `X-Write`, sehingga password salah ditolak server **tanpa** menghabiskan jatah baca.
- Akun: `PBKDF2(password, "notehop/account/"+username, 600000)` → 64 byte: 32 pertama = kunci brankas (tetap di browser), 32 terakhir = secret login (di-hash argon2id di server).
- Akun SSO (SSO): NoteHop tidak pernah melihat password-nya, jadi kunci brankas berasal dari frasa terpisah: `vault = PBKDF2(frasa, "notehop/vault/"+sso_id, 600000)`. Server hanya menerima `HMAC(vault, "notehop/vault-check")` (di-hash argon2id) untuk membedakan frasa benar dan salah ketik.
- Blok PIN: `salt.b64 + "." + AES-GCM(PBKDF2(PIN, salt, 250000))`.

Yang terlihat oleh server: ukuran ciphertext, waktu, jumlah view, mode link, status centang (indeks + benar/salah), jumlah baris tambahan.

## API

Semua JSON. Header: `X-Admin` (token admin), `X-Write` (bukti kunci).

| Method & path | Siapa | Fungsi |
| --- | --- | --- |
| `POST /api/notes` | siapa saja | Buat catatan `{encrypted, password, salt, content, mode, slug, expires_in, max_views, admin, write, owner_blob}` |
| `GET /api/n/{token}` | siapa saja | Metadata tanpa isi (admin: + links, snapshots) |
| `POST /api/n/{token}/open` | siapa saja (+X-Write jika berpassword) | Isi + entries + checks; menghitung view. `{"poll":true}` tidak menghitung (ditolak untuk catatan burn) |
| `PUT /api/n/{token}` | mode edit / admin + X-Write | Simpan `{content, version}`; 409 jika versi basi |
| `POST /api/n/{token}/entries` | edit/append/admin + X-Write | Tambah baris |
| `DELETE /api/n/{token}/entries/{id}` | admin | Hapus baris |
| `POST /api/n/{token}/checks` | edit/check/admin + X-Write | `{index, checked}` |
| `POST /api/n/{token}/freeze` | admin | Buat snapshot beku |
| `POST /api/n/{token}/links` · `DELETE …/links/{t}` | admin | Tambah/matikan link |
| `DELETE /api/n/{token}` | admin atau pemilik akun | Hapus catatan |
| `GET /raw/{token}` | siapa saja | Teks murni (hanya catatan publik) |
| `POST /api/register` · `/api/login` · `/api/logout`, `GET /api/me`, `GET /api/me/notes` | | Akun |
| `GET /auth/sso` · `GET /auth/sso/callback` | | Masuk dengan SSO (tiket HMAC, `state` + cookie `nh_sso`, `jti` sekali pakai) |
| `POST /api/vault` | akun SSO | `{check}`: simpan cek frasa brankas pertama kali (201), atau periksa (200 / 401) |

## Struktur

```
cmd/notehop/          main: config env, server, janitor
internal/db/          skema SQLite + query
internal/app/         handler HTTP, auth, middleware, tes
web/templates/        HTML (base, icons+editor partial, halaman)
web/static/js/        app.js (util), crypto.js, md.js, editor.js, new.js, note.js, auth.js, dashboard.js
web/static/css/       app.css
web/static/vendor/    marked, DOMPurify, highlight.js, qrcode-generator (di-vendor)
design-reference/     aset desain ImageApp / SSO / OtherApp (referensi, tidak di-build)
docs/                 dokumen integrasi
ROADMAP.md            semua rencana fitur
```

## Lisensi pihak ketiga

marked (MIT), DOMPurify (Apache-2.0/MPL-2.0), highlight.js (BSD-3-Clause), qrcode-generator (MIT), modernc.org/sqlite (BSD-3-Clause), golang.org/x/crypto (BSD-3-Clause).
