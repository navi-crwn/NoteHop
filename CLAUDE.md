# CLAUDE.md: konteks untuk sesi berikutnya

Baca ini dulu sebelum menyentuh kode. Pemilik: **navi-crwn** (juga pemilik ImageApp, SSO, OtherApp). Bahasa percakapan dan UI: **Bahasa Indonesia**.

## Apa ini

NoteHop: catatan online yang bisa dibagikan, **zero-knowledge** (isi dienkripsi di browser, server hanya menyimpan ciphertext). Satu binary Go + SQLite, aset di-embed. Desain sekeluarga dengan ImageApp / OtherApp / SSO (lihat `design-reference/`).

## Dokumen penting

| File | Isi |
| --- | --- |
| `README.md` | Fitur, cara jalan, konfigurasi, model kripto, API |
| `ROADMAP.md` | **Semua** ide fitur dari pemilik (Fase 1 sampai 6), status ✅/🔜/💡, catatan arsitektur |
| `docs/INTEGRASI-SSO-IMAGEAPP.md` | Rencana login dengan akun SSO (SSO) dan kombinasi dengan ImageApp |
| `design-reference/README.md` | Aset desain dari repo ImageApp, SSO, OtherApp, dan cara memakainya |
| `docs/PROMPT-LANJUTAN.md` | Prompt dan urutan kerja untuk sesi lanjutan |

## Status saat ini (Fase 1 selesai)

- Backend: `internal/db` (skema + query), `internal/app` (handler, auth, middleware), `cmd/notehop` (main).
- Frontend tanpa build step: `web/templates/*.html` + `web/static/js/*.js` + `web/static/css/app.css`. Library pihak ketiga di-vendor di `web/static/vendor/` (marked 15.0.12, DOMPurify 3.2.6, highlight.js 11.11.1, qrcode-generator 1.4.4).
- Tes: `go test -race ./...` lulus (izin link, burn atomik, konflik edit, freeze, raw, auth, CSRF, header).
- Sudah dicek di browser: membuat catatan, slash menu, lanjutan list, spoiler, pratinjau, redirect ke halaman catatan, dialog Bagikan.
- Login dengan SSO (SSO) + frasa brankas: sisi NoteHop selesai dan dicek di browser dengan SSO tiruan (masuk, buat frasa, salah frasa, perangkat kedua, dashboard membuka judul). Sisi SSO (`/sso/notehop`) belum dibuat, lihat `docs/INTEGRASI-SSO-IMAGEAPP.md`.
- **Belum dicek manual di browser**: blok PIN, centang lewat link checklist, mode append dari pengunjung, alur password + burn, dashboard akun, QR, freeze dari UI, tampilan HP. Kerjakan ini dulu sebelum menambah fitur.

## Aturan yang harus dijaga

1. **Jangan pernah kirim kunci/isi plaintext ke server.** Kunci ada di `#k=` (fragment). Fitur yang butuh server membaca isi hanya boleh untuk catatan publik (`encrypted=false`) dan harus ditandai ⚠️ E2E di ROADMAP.
2. Setiap perubahan isi wajib `X-Write` (bukti kunci) + mode link yang sesuai; aksi admin wajib `X-Admin`. Lihat `writable()` / `requireAdmin()` di `internal/app/notes.go`.
3. Halaman `/n/{token}` hanya kerangka; konten diambil lewat `POST /open` setelah aksi manusia (agar bot preview WA/Telegram tidak menghabiskan jatah burn).
4. CSP ketat: `script-src 'self'`, `style-src 'self'`, jadi **tidak boleh ada inline script/style** di template. Pakai class CSS.
5. Penomoran checkbox: urutan dokumen, di luar fenced code. Implementasinya ada di dua tempat dan harus identik: `applyChecks` (Go, `notes.go`) dan `eachTask` (JS, `md.js`).
6. Skema DB hanya berubah lewat migrasi baru di **akhir** daftar `migrations` (`internal/db/db.go`). Jangan ubah migrasi yang sudah ada.
7. Gaya kode: komentar singkat yang menjelaskan *kenapa*, pesan error untuk pengguna dalam Bahasa Indonesia, tanpa framework frontend.

## Perintah

```bash
go run ./cmd/notehop -addr 127.0.0.1:8090 -db data/dev.db   # dev
go test -race ./...                                          # tes
go build -o bin/notehop ./cmd/notehop                        # build
```

`.claude/launch.json` sudah berisi konfigurasi `notehop` (port 8090) untuk preview.

## Langkah berikutnya yang disarankan

1. Uji manual alur yang belum dicek (daftar di atas), perbaiki bug.
2. Sisi SSO untuk SSO: route `/sso/notehop` di repo `navi-crwn/accounts` (lihat `docs/INTEGRASI-SSO-IMAGEAPP.md`).
3. Dockerfile + contoh deploy (Caddy/nginx), sama seperti ImageApp.
4. Lanjut ke Fase 2 di ROADMAP (real-time, riwayat, sticky keyboard bar HP).
