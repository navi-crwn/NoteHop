# NoteHop: goals dan persiapan lokal

Satu dokumen untuk dua hal: **apa yang ingin dicapai NoteHop** (goals) dan **cara menyiapkan, menjalankan, dan menguji NoteHop di komputer sendiri** sebelum dipasang di VPS.

Dokumen terkait: `README.md` (fitur, API, konfigurasi), `ROADMAP.md` (semua ide dan statusnya), `docs/PROGRESS.md` (progres per batch), `docs/DEPLOY.md` dan `docs/DEPLOY.md` (VPS), `CLAUDE.md` (aturan kode).

---

## Bagian 1: Goals

### 1.1 Tujuan produk

NoteHop adalah catatan online yang bisa dibagikan dengan link, untuk pasar internasional (UI 9 bahasa, default English, autodetect), sekeluarga dengan ImageApp, SSO, dan OtherApp.

| Prinsip | Artinya dalam praktik |
| --- | --- |
| **Zero-knowledge** | Isi dienkripsi di browser (AES-256-GCM). Kunci ada di bagian `#k=` link dan tidak pernah dikirim ke server. Server hanya menyimpan ciphertext. |
| **Satu link, izin berbeda** | Satu catatan bisa punya link Edit, Baca, Tambah (append), Centang, Usul edit. Link bocor bisa dimatikan sendiri-sendiri. |
| **Sederhana dipasang** | Satu binary Go + SQLite, aset di-embed. Tanpa database server, tanpa build frontend. |
| **Aman dari bot** | Isi baru diambil setelah aksi manusia, jadi preview WhatsApp/Telegram tidak menghabiskan jatah "hancur setelah dibaca". |
| **Jujur soal batas E2E** | Fitur yang membuat server membaca teks (SSH, Mock API, profil publik, bot Telegram) hanya untuk catatan publik atau ditandai ⚠️ di UI dan ROADMAP. |
| **UI konsisten** | Setiap fitur diaktifkan lewat pill toggle; fitur lanjutan di *Pengaturan lanjutan*; fitur yang bentrok ditolak di UI dan di server. |

### 1.2 Yang sudah tercapai

Semua fase yang direncanakan sudah dibangun dan diuji (rinciannya di `docs/PROGRESS.md`):

- **Dasar**: catatan terenkripsi, banyak link per catatan, snapshot beku, burn after reading, kedaluwarsa, akun opsional dengan brankas lokal, login SSO (sisi NoteHop).
- **Batch A, di browser**: template `{{variabel}}`, fork, ekspor MD/PDF, slide, teleprompter, baca nyaring, zen, command palette, kalkulator, Mermaid, wiki-link + graf, web clipper, redaksi otomatis.
- **Batch B, di server**: riwayat versi + diff, usul edit, tanda tangan, polling, blok hancur sendiri, password umpan, IP allowlist, checklist berulang, dead man's switch, watermark, klaim catatan tamu, coretan pribadi.
- **Batch C, lampiran**: file terenkripsi 24 jam, tempel gambar jadi WebP, penanda waktu `[1:23]` untuk audio/video.
- **Batch D, real-time**: edit bersama (Yjs + WebSocket, server hanya relay ciphertext), kursor dan nama tamu, chat, ikuti presenter, embed, formulir, papan Kanban, `/nearby` (WebRTC).
- **Batch E, integrasi**: CLI `notehop new/get`, webhook Pantau, kotak masuk, bot Telegram, profil `/@user`, Mock API, SSH, PWA offline, ekspor/impor.
- **Tes**: `go test -race ./...` dan 8 skrip browser (112 cek) lulus.

### 1.3 Goals berikutnya

Urutan yang disarankan (dari yang paling berdampak ke peluncuran):

| # | Goal | Ukuran selesai |
| --- | --- | --- |
| 1 | **Jalan di lokal** (Bagian 2 dokumen ini) | Server jalan, semua tes lulus di komputer Anda, uji manual 2.7 beres |
| 2 | **Live di VPS** dengan domain dan HTTPS | `docs/DEPLOY.md` bagian 1–4 selesai, laporan audit tanpa temuan kritis |
| 3 | **Login SSO (SSO)** | Route `/sso/notehop` dibuat di repo `navi-crwn/accounts`, login dua arah dicek di produksi |
| 4 | **Merge PR #1** ke `main` | Review selesai, CI hijau (jika ditambahkan), tag versi pertama (misalnya `v1.0.0`) |
| 5 | **Sisa ROADMAP yang "belum"** | Edit offline + sinkron, ekspor PNG, notifikasi Watch lewat Telegram/email, bot WhatsApp/Discord |
| 6 | **Operasional** | Backup harian terbukti bisa di-restore, monitoring uptime, prosedur update tertulis |

### 1.4 Di luar cakupan (sengaja tidak dibuat)

Paywall, steganografi gambar, aksi AI, micro-blog/custom domain, link unfurling, geo-fencing, email-to-note. Alasannya ada di `docs/PROGRESS.md`. Kalau salah satunya ingin dibuat, tanyakan dulu dampaknya ke prinsip zero-knowledge.

---

## Bagian 2: Persiapan di lokal

### 2.1 Prasyarat

| Alat | Versi | Untuk apa | Wajib? |
| --- | --- | --- | --- |
| Git | apa saja | Ambil kode | Ya |
| Go | **1.27+** (lihat `go.mod`) | Menjalankan dan membangun server | Ya |
| Node.js | 18+ (diuji dengan 22) | Menjalankan tes browser | Hanya untuk tes browser |
| Playwright + Chromium | `playwright@1` | Tes browser | Hanya untuk tes browser |
| make | apa saja | Pintasan build (`make linux`) | Tidak |
| sqlite3 CLI | apa saja | Mengintip database saat debug | Tidak |

Sistem operasi: Linux dan macOS langsung jalan. Di Windows, pakai **WSL2** (Ubuntu) agar skrip shell dan perintah di dokumen ini sama persis.

Cek:

```bash
git --version
go version          # harus go1.27 atau lebih baru
node --version      # v18+ (opsional)
```

### 2.2 Ambil kode

```bash
git clone https://github.com/navi-crwn/NoteHop.git
cd NoteHop
git checkout claude/relaxed-dijkstra-rnbh9p    # branch PR #1; setelah di-merge, cukup main
```

### 2.3 Jalankan

```bash
go run ./cmd/notehop -addr 127.0.0.1:8090 -db data/dev.db
```

Buka <http://127.0.0.1:8090>. Folder `data/` dan file `*.db` sudah di-ignore git, jadi aman untuk data uji. Hapus `data/dev.db*` kapan saja untuk mulai dari kosong.

Aset (HTML, JS, CSS, katalog bahasa) di-embed ke binary. **Setelah mengubah file di `web/`, hentikan server lalu jalankan ulang** `go run`, kalau tidak browser tetap melihat versi lama.

Di VS Code / Claude Code, `.claude/launch.json` sudah berisi konfigurasi `notehop` (port 8090).

### 2.4 Konfigurasi opsional di lokal

Semua lewat environment variable (daftar lengkap di `README.md`). Yang berguna untuk mencoba di lokal:

```bash
# Lampiran lebih kecil, supaya cepat mencoba batas ukuran
NOTEHOP_FILE_MB=2 go run ./cmd/notehop -addr 127.0.0.1:8090 -db data/dev.db

# SSH untuk catatan publik
NOTEHOP_SSH_ADDR=127.0.0.1:2222 go run ./cmd/notehop -addr 127.0.0.1:8090 -db data/dev.db
#   lalu: ssh -p 2222 127.0.0.1 < README.md
#   (kunci host dibuat di data/ssh_host_ed25519_key)

# Webhook Pantau ke server lokal Anda sendiri (http dan alamat privat diizinkan)
NOTEHOP_PRIVATE_HOOKS=1 go run ./cmd/notehop -addr 127.0.0.1:8090 -db data/dev.db
```

Yang **tidak** bisa dicoba penuh di lokal tanpa alamat publik: bot Telegram (Telegram harus bisa memanggil server Anda) dan login SSO (butuh SSO yang sudah punya route SSO). Keduanya dicoba di VPS.

### 2.5 Coba CLI

```bash
go build -o bin/notehop ./cmd/notehop
printf '# Halo\n\ndari terminal' | ./bin/notehop new -server http://127.0.0.1:8090
#   baris pertama = link untuk dibagikan; "admin" di stderr = link pemilik (simpan)
./bin/notehop get 'http://127.0.0.1:8090/n/TOKEN#k=KUNCI'
./bin/notehop new -h        # semua opsi: -password, -expires, -burn, -mode, -public, -slug
```

Catatan yang dibuat di terminal bisa dibuka di browser, dan sebaliknya; enkripsinya sama persis.

### 2.6 Tes otomatis

```bash
# Go: unit + integrasi server (cepat, tidak butuh Node)
go vet ./... && go test -race ./...

# Browser: sekali saja pasang Playwright
npm i -D playwright@1
npx playwright install chromium        # Linux tanpa GUI: npx playwright install --with-deps chromium

# Semua 8 skrip browser, masing-masing dengan server dan database baru
./scripts/e2e-all.sh
# Hanya beberapa:
./scripts/e2e-all.sh e2e-live e2e-blocks
```

Hasil yang diharapkan:

| Skrip | Cek | Isi |
| --- | --- | --- |
| `e2e` | 30 | Alur dasar: buat, link, PIN, checklist, append, burn, dashboard, QR, freeze, HP 375px |
| `e2e-features` | 22 | Fitur browser (Batch A) |
| `e2e-server-features` | 20 | Fitur server (Batch B) |
| `e2e-files` | 7 | Lampiran, WebP, timestamp |
| `e2e-live` | 9 | Edit bersama, kursor, chat, presenter |
| `e2e-blocks` | 8 | Embed, formulir, Kanban |
| `e2e-nearby` | 4 | Perangkat terdekat (WebRTC) |
| `e2e-integrations` | 12 | CLI, kotak masuk, Pantau, profil, Mock API, ekspor/impor, offline |

`e2e-all.sh` memakai port 8099 dan folder sementara, jadi tidak mengganggu server dev di 8090. Ganti port dengan `PORT=9000 ./scripts/e2e-all.sh`.

### 2.7 Uji manual singkat (10 menit)

1. Buat catatan dengan password → buka link di jendela *incognito* → harus minta password, lalu terbuka.
2. Mode **Edit** → buka link yang sama di browser lain → ketik bersamaan: teks menyatu, kursor dan nama terlihat, chat jalan.
3. *Pengaturan lanjutan* → nyalakan dua fitur yang bentrok (misalnya Kapsul waktu dan Dead man's switch) → yang kedua harus redup dengan alasannya.
4. Tempel screenshot ke editor → muncul sebagai gambar; buka di browser lain → gambar terdekripsi.
5. Ganti bahasa ke العربية → tata letak berbalik kanan-ke-kiri dengan rapi.
6. Buka `/nearby` di dua tab → saling terlihat, coretan bersama tersinkron.
7. DevTools → Network → *Offline* → muat ulang catatan yang tadi dibuka → masih terbaca dengan spanduk offline.

### 2.8 Build untuk VPS

```bash
make linux          # bin/notehop-linux-amd64 dan bin/notehop-linux-arm64 (statis, tanpa cgo)
# tanpa make:
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o bin/notehop-linux-amd64 ./cmd/notehop
```

Binary ini yang disalin ke VPS dan dipasang dengan `sudo ./deploy/install.sh ./bin/notehop-linux-amd64` (lihat `docs/DEPLOY.md`).

### 2.9 Masalah umum

| Gejala | Penyebab dan solusi |
| --- | --- |
| Perubahan JS/CSS tidak terlihat | Aset di-embed: jalankan ulang server. Jika masih lama, service worker menyimpan versi lama: DevTools → Application → Service workers → *Unregister*, lalu muat ulang. |
| `address already in use` | Port 8090 dipakai: ganti `-addr 127.0.0.1:8091`, atau hentikan proses lama (`lsof -i :8090`). |
| Error CSP di console ("Refused to execute inline script") | Ada script/style inline di template. Aturan proyek: pindahkan ke file `.js`/`.css` (lihat `CLAUDE.md` aturan 4). |
| Edit bersama tidak tersambung | Pastikan membuka lewat alamat yang sama (`127.0.0.1` vs `localhost` dianggap beda). Di VPS: cek blok WebSocket di nginx. |
| `/nearby` tidak menemukan perangkat lain | Hanya bekerja untuk perangkat dengan IP publik yang sama (satu Wi-Fi). Di lokal, dua tab di komputer yang sama cukup untuk mencoba. |
| `go: go.mod requires go >= 1.27` | Perbarui Go dari <https://go.dev/dl/>. |
| `go test ./internal/i18n` gagal | Ada kunci teks yang belum ada di salah satu dari 9 file `web/i18n/*.json`, atau placeholder/tag berbeda. Tambahkan di semua bahasa. |

### 2.10 Checklist sebelum ke VPS

- [ ] `go vet ./... && go test -race ./...` lulus
- [ ] `./scripts/e2e-all.sh` lulus (112 cek)
- [ ] Uji manual 2.7 beres
- [ ] Binary `make linux` dibuat dari commit yang sudah di-push
- [ ] Domain sudah mengarah ke IP VPS (record A/AAAA)
- [ ] Sudah diputuskan: nginx atau Caddy, port SSH (jika dipakai), token bot Telegram (jika dipakai), rahasia SSO SSO (jika langsung disambung)
- [ ] Lanjut ke `docs/DEPLOY.md`
