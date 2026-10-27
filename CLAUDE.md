# CLAUDE.md: konteks untuk sesi berikutnya

Baca ini dulu sebelum menyentuh kode. Pemilik: **navi-crwn** (juga pemilik ImageApp, SSO, OtherApp). Bahasa percakapan dengan pemilik: **Bahasa Indonesia**. UI: **9 bahasa, default English, autodetect dari browser** (produk dijual internasional; jangan jadikan Indonesia default). Tampilan memakai **bahasa awam**; istilah teknis hanya di halaman FAQ (`/faq`) dan di mode "Tampilkan nama teknis".

## Apa ini

NoteHop: catatan online yang bisa dibagikan, **zero-knowledge** (isi dienkripsi di browser, server hanya menyimpan ciphertext). Satu binary Go + SQLite, aset di-embed. Desain sekeluarga dengan ImageApp / OtherApp / SSO (lihat `design-reference/`).

## Dokumen penting

| File | Isi |
| --- | --- |
| `README.md` | Fitur, cara jalan, konfigurasi, model kripto, API |
| `ROADMAP.md` | **Semua** ide fitur dari pemilik (Fase 1 sampai 6), status ✅/🔜/💡, catatan arsitektur |
| `docs/INTEGRASI-SSO-IMAGEAPP.md` | Rencana login dengan akun SSO (SSO) dan kombinasi dengan ImageApp |
| `design-reference/README.md` | Aset desain dari repo ImageApp, SSO, OtherApp, dan cara memakainya |
| `docs/DEPLOY.md` | Deploy ke VPS: `deploy/install.sh`, systemd, Caddy/nginx, backup, update, restore |
| `docs/PROMPT-LANJUTAN.md` | Prompt dan urutan kerja untuk sesi lanjutan |
| `docs/GOALS-DAN-PERSIAPAN-LOKAL.md` | Goals produk dan cara menyiapkan, menjalankan, menguji di komputer sendiri |
| `docs/DEPLOY.md` | Arahan untuk sesi Claude di VPS (produksi: note.example.com): update, uji, audit, aturan kerja |
| `docs/REKAP-STATUS.md` | Apa yang sudah/belum dibuat dan alasannya (seluruh percakapan) |
| `docs/ARAHAN-PATCH.md` | Arahan memasang patch Batch F (tampilan ramah, 9 bahasa) di VPS |
| `docs/PANDUAN-FITUR.md` | Semua fitur: fungsi, cara pakai, batasan, contoh kasus, mitigasi (✅ sudah / 💡 usulan) |
| `docs/AUDIT-PRD.md` | Master Blueprint 54 fitur vs. yang sudah dibangun, cara cek tiap fitur di website, kandidat berikutnya |

## Status saat ini (semua fase roadmap yang direncanakan selesai)

- Backend: `internal/db` (skema + migrasi 1–7: 6 = `slug_holds`, 7 = `poll_voters`), `internal/app` (handler, fitur, live relay, nearby, inbox, watch, SSH, Telegram), `internal/client` (CLI, kripto sama dengan browser), `cmd/notehop` (server + `backup`/`new`/`get`/`takedown`).
- Aturan nama (`internal/app/names.go` + `names/*.txt`): nama link sendiri minimal 7 karakter, nama cadangan + awalan, kata kasar, masa tunggu 90 hari (`slug_holds`; takedown 365 hari). Env `NOTEHOP_ADMINS`, `NOTEHOP_RESERVED`, `NOTEHOP_CONTACT`. Panduan fitur + risiko + mitigasi: `docs/PANDUAN-FITUR.md`.
- Frontend tanpa build step: `web/templates/*.html` + `web/static/js/*.js` + `web/static/css/app.css`. Vendor di `web/static/vendor/`: marked 15.0.12, DOMPurify 3.2.6, highlight.js 11.11.1, qrcode-generator 1.4.4, Mermaid 11.12.0 (di iframe sandbox), Yjs 13.6.27 (bundle IIFE `window.Y`).
- Progres per batch ada di `docs/PROGRESS.md` (A browser, B server, C lampiran, D real-time, E integrasi). Fitur yang sengaja tidak dibuat juga dicatat di sana beserta alasannya.
- Tes: `go test -race ./...` (Go) dan 8 skrip Playwright di `scripts/` (lihat Perintah). Jalankan semua setelah mengubah UI.
- Real-time: server hanya relay ciphertext (`live.go`); dokumen Yjs ada di browser; penulis tertua di room yang menyimpan (PUT biasa).

## Aturan yang harus dijaga

1. **Jangan pernah kirim kunci/isi plaintext ke server.** Kunci ada di `#k=` (fragment). Fitur yang butuh server membaca isi hanya boleh untuk catatan publik (`encrypted=false`) dan harus ditandai ⚠️ E2E di ROADMAP.
2. Setiap perubahan isi wajib `X-Write` (bukti kunci) + mode link yang sesuai; aksi admin wajib `X-Admin`. Lihat `writable()` / `requireAdmin()` di `internal/app/notes.go`.
3. Halaman `/n/{token}` hanya kerangka; konten diambil lewat `POST /open` setelah aksi manusia (agar bot preview WA/Telegram tidak menghabiskan jatah burn).
4. CSP ketat: `script-src 'self'`, `style-src 'self'`, jadi **tidak boleh ada inline script/style** di template. Pakai class CSS.
5. Penomoran checkbox: urutan dokumen, di luar fenced code. Implementasinya ada di dua tempat dan harus identik: `applyChecks` (Go, `notes.go`) dan `eachTask` (JS, `md.js`).
6. Skema DB hanya berubah lewat migrasi baru di **akhir** daftar `migrations` (`internal/db/db.go`). Jangan ubah migrasi yang sudah ada.
7. Fitur opsional per catatan: tambah field di `NoteConfig` dan satu nilai `feature` di file sendiri (lihat `internal/app/features.go`, contoh `lock.go`), bukan kolom baru atau `if` di `notes.go`.
8. UI fitur: setiap fitur diaktifkan lewat **pill toggle** (`.pill-opt`), inputnya baru muncul setelah aktif. Fitur advance masuk **Pengaturan lanjutan** (`details.adv`). Fitur yang bentrok tidak boleh aktif bersamaan: aturannya di `RULES` (`new.js`) dan `TIMED` (`note.js`) untuk UI, dan **juga** dicek di server (`createReq.invalid`, `checkTimes`, `apiAddLink`).
9. **Teks UI selalu lewat katalog** `web/i18n/<lang>.json` (en, zh, es, ar, pt, fr, ja, ru, id; English = sumber dan fallback). Template: `{{.L.T "kunci"}}` / `{{.L.H "kunci"}}` (bermarkup). JS: `NH.t('kunci', {var})`. Go: `apiErr(w, status, "err.kunci", "var", nilai)`. Kunci baru wajib ditambahkan ke **semua 9 file**; `go test ./internal/i18n` memeriksa kunci, placeholder, dan tag. Arab = RTL: pakai properti CSS logis (`margin-inline-start`, bukan `margin-left`).
10. Gaya kode: komentar singkat yang menjelaskan *kenapa*, tanpa framework frontend.
11. **Bahasa UI awam.** Label menjelaskan hasil ("Hapus setelah dibaca"), bukan istilah ("burn after reading"). Detail teknis masuk `faq.*` di `/faq`; nama teknis boleh di atribut `data-tech` (tampil hanya di mode teknis). Halaman Buat: kartu "Mau bikin apa?", tiga pertanyaan (siapa boleh apa, seberapa rahasia, simpan selama) yang menjadi preset untuk toggle di Pengaturan lanjutan; toggle tetap sumber kebenaran yang dikirim ke server.

## Perintah

```bash
go run ./cmd/notehop -addr 127.0.0.1:8090 -db data/dev.db   # dev
go test -race ./...                                          # tes
go build -o bin/notehop ./cmd/notehop                        # build
./scripts/e2e-all.sh                                         # semua 8 skrip browser, server + DB baru per skrip
make linux                                                   # binary statis amd64/arm64 untuk VPS
notehop backup -db notehop.db salinan.db                     # backup saat berjalan
```

`.claude/launch.json` sudah berisi konfigurasi `notehop` (port 8090) untuk preview.

## Langkah berikutnya yang disarankan

1. Sisi SSO untuk SSO: route `/sso/notehop` di repo `navi-crwn/accounts` (lihat `docs/INTEGRASI-SSO-IMAGEAPP.md`).
2. Produksi sudah jalan di https://note.example.com: update, uji, dan audit mengikuti `docs/DEPLOY.md` + `docs/AUDIT-PRD.md`.
3. Celah terhadap Master Blueprint ada di `docs/AUDIT-PRD.md` bagian 4 (F1–F5). Kerjakan hanya yang disetujui pemilik.
