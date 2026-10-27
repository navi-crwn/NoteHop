# NoteHop: Roadmap Fitur

Status: ✅ ada di Fase 1 · 🔜 fase berikutnya · 💡 ide (belum dijadwalkan)

Prinsip yang dipegang semua fitur:
- **Zero-knowledge dulu.** Isi catatan terenkripsi di browser; server tidak bisa membacanya. Fitur yang butuh server membaca isi (ditandai ⚠️ E2E) hanya berlaku untuk catatan **publik** atau harus dikerjakan di browser.
- **Progressive disclosure.** Halaman awal 95% kosong; fitur muncul lewat `/`, `Ctrl+K`, toolbar seleksi, drawer Share, atau status bar.

---

## Fase 1: Inti (sedang dibangun)

| Fitur | Status |
| --- | --- |
| Go + SQLite, satu binary, aset di-embed, desain keluarga ImageApp/OtherApp | ✅ |
| Enkripsi end-to-end AES-256-GCM, kunci di `#fragment` | ✅ |
| Password tambahan (PBKDF2), juga syarat untuk membuka di server, jadi salah password tidak menghabiskan jatah baca | ✅ |
| Multi-link per catatan dengan izin berbeda: Edit, Baca saja, Tambah saja (append-only), Centang checklist | ✅ |
| Link admin terpisah (edit, bekukan, buat/hapus link, hapus catatan); link bocor bisa dihapus tanpa merusak link lain | ✅ |
| Bukti kunci (write key) wajib untuk setiap perubahan | ✅ |
| Freeze / Snapshot versi permanen | ✅ |
| Burn after reading (1/3/10×) + kedaluwarsa waktu | ✅ |
| Anti bot-preview: konten hanya diambil lewat tombol/JS (POST), jadi bot WA/Telegram/Discord tidak menghabiskan view | ✅ |
| Hitung view atomik (`UPDATE … WHERE views < max_views RETURNING`) | ✅ |
| Custom slug + QR code (dibuat di browser, kunci tidak bocor ke server) | ✅ |
| `/raw/slug` untuk catatan publik (curl) | ✅ |
| Markdown + slash command `/`, spoiler/toggle, blok terkunci PIN, syntax highlight otomatis | ✅ |
| Smart paste tabel dari Excel/Sheets jadi tabel Markdown | ✅ |
| Akun opsional: password tidak dikirim (PBKDF2 → vault key lokal + auth secret), daftar catatan terenkripsi | ✅ |
| `noindex, nofollow` di semua halaman catatan, CSP ketat, gambar pihak ketiga diblokir | ✅ |
| Deteksi bentrok edit (optimistic version) + auto-save | ✅ |

---

## Fase 2: Kolaborasi & editor

- ✅ Real-time: WebSocket di Go (coder/websocket) + **Yjs (CRDT)**, isi update tetap terenkripsi (server hanya relay ciphertext); penulis tertua di room yang menyimpan ke DB
- ✅ Anonymous guest cursors (nama samaran + warna, bisa diganti)
- ✅ Ephemeral mini-chat (terenkripsi, tidak pernah disimpan di server)
- ✅ Follow Presenter Mode (scroll dan slide sinkron)
- ✅ Diff slider / mesin waktu riwayat (riwayat versi terenkripsi)
- ✅ Side-by-side diff checker (bandingkan 2 teks, link baca saja)
- 🔜 Split-screen dua catatan
- ✅ Smart paste gambar → WebP terkompresi (lampiran terenkripsi, EXIF ikut terbuang)
- 🔜 Mermaid diagram & kanvas coretan
- 🔜 Command palette `Ctrl+K` + floating bubble toolbar saat teks diseleksi
- ✅ Sticky keyboard bar di HP (`/`, to-do, tempel, blok PIN, selesai), menempel di atas keyboard lewat `visualViewport`
- ✅ PWA offline: aplikasi bisa dipasang, catatan yang pernah dibuka bisa dibaca offline (service worker menyimpan ciphertext, bukan kunci; catatan burn tidak disimpan). Edit offline lalu sinkron: belum
- ✅ Mode "Usul Edit" (suggesting/pull request): pengunjung read-only mengusulkan, pemilik Terima/Tolak
- ✅ Copy button per baris/blok di mode baca (paragraf, item daftar, kutipan, tabel sebagai TSV)

## Fase 3: Kontrol waktu & akses lanjutan

- ✅ **Scheduled Lock**: link edit/tambah/centang otomatis jadi read-only pada jam tertentu (server-side, aman E2E); link admin tetap bisa mengubah
- ✅ **Time-Capsule / Embargo**: isi baru bisa dibuka setelah waktu tertentu, dengan countdown (server menahan ciphertext sampai waktunya; link admin tetap bisa membuka)
- ✅ **Partial Burn**: satu blok di dalam catatan hancur setelah dilihat 1× (blok disimpan terpisah dengan penghitung sendiri)
- ✅ **Decoy Password**: password kedua membuka catatan palsu + peringatan diam-diam ke pemilik (dua ciphertext, server tidak tahu mana yang asli)
- ✅ **Sign-Off Lock**: tombol setuju/tanda tangan, catat waktu (+IP opsional), lalu kunci catatan permanen (otomatis jadi snapshot)
- ✅ **Cron reset / catatan berulang**: centang lewat link checklist direset harian/mingguan/bulanan
- ✅ IP allowlist (IP/CIDR) per catatan · 💡 Geo-fencing (butuh database GeoIP)
- ✅ Honeypot: watermark tipis / zero-width per pengunjung (anti screenshot-sebar)
- ✅ Dead Man's Switch: kapsul waktu yang terus diundur setiap admin membuka catatan; jika admin tidak muncul N hari, isi terbuka bagi pemegang link (kunci sudah ada di link penerima, server tetap tidak tahu isinya)

## Fase 4: Smart blocks (catatan hidup)

- 🔜 Template fill-in-the-blanks `{{variabel}}`: pengunjung isi kolom, teks langsung siap di-copy (berjalan di browser, aman E2E)
- 🔜 Inline math / kalkulator teks gaya Numi/Soulver (`Konsumsi = 50 * 35.000` → `= 1.750.000`)
- ✅ Mini-poll / voting di dalam catatan read-only (seperti checklist: server simpan hitungan, bukan teks)
- ✅ Form-to-Note (`:::form`): catatan tampil sebagai formulir, setiap submit menjadi baris baru (memakai mekanisme append terenkripsi)
- ✅ Kanban view dari sub-judul `## Kolom` (drag-and-drop atau menu pindah untuk pemegang link edit)
- ✅ View switcher: Teks | Papan (Form ada di dalam teks, Slide di menu Lainnya, Diff di Riwayat)
- 🔜 Slide mode (`---` jadi slide) & teleprompter
- ✅ Timestamped media note: `Alt+T` atau `/` → `[04:12]` yang memutar lampiran audio/video (`Ctrl+T` milik browser; YouTube tidak, karena iframe pihak ketiga melanggar CSP dan membocorkan pembaca ke Google)
- 🔜 One-click text utilities: hapus duplikat, urutkan A–Z, ekstrak link/email, ubah huruf, format JSON/CSV
- 🔜 Live regex & mass find-replace studio
- 🔜 Wiki-links `[[Judul]]` + graph view (di browser, dari vault akun)
- ✅ Live embed antar catatan (`:::embed <link>`, diperbarui tiap 10 detik; satu tingkat, tidak untuk catatan berpassword/burn)
- ✅ Audio reader (Web Speech API, di browser)
- 🔜 Zen mode + ambient sound + Pomodoro + target jumlah kata
- ✅ Export: Markdown dan PDF (print) per catatan; ekspor/impor semua catatan akun sebagai JSON (backup ke penyimpanan sendiri). PNG estetik: belum

## Fase 5: Ekosistem, developer & pindah perangkat

- ✅ SSH access (`ssh -p 2222 host < file.md`, `ssh … get <link>`) dengan golang.org/x/crypto/ssh, aktif lewat `NOTEHOP_SSH_ADDR` ⚠️ E2E: hanya catatan publik (kedaluwarsa 30 hari); untuk catatan terenkripsi pakai CLI
- ✅ CLI resmi: `notehop new` / `notehop get` (enkripsi lokal, format sama persis dengan browser)
- ✅ Instant Mock API: `/mock/<link>/path/0` dari JSON di catatan (seluruh teks atau blok ```json), CORS terbuka ⚠️ E2E: hanya catatan publik
- ✅ Webhook-to-Note (Kotak masuk): `POST /in/<token>` jadi entri; server menyegel tiap entri ke kunci publik catatan (P-256 ECDH + AES-GCM), kunci privat hanya bisa dibuka pemegang kunci catatan ⚠️ server melihat teks saat tiba, menyimpan ciphertext
- ✅ Watch Note: webhook (hanya metadata: jenis perubahan, versi, waktu), ditandatangani HMAC, dikumpulkan per 5 detik, aman dari SSRF. Telegram/email: belum
- ✅ Bot Telegram ⚠️: `/connect <alamat kotak masuk>` lalu setiap pesan masuk ke catatan (terenkripsi saat disimpan; Telegram dan server melihat teks saat dikirim). WhatsApp/Discord: belum
- 🔜 Email-to-Note ⚠️ E2E: isi diterima server sebelum dienkripsi (pakai kunci publik catatan)
- 🔜 Web clipper / bookmarklet "Kirim ke catatan"
- 🔜 Fork / Clone ke ruang sendiri (di browser, kunci baru)
- ✅ Same-WiFi Instant Pad (`/nearby`, room per IP publik, isi lewat WebRTC langsung antar perangkat)
- ✅ P2P file drop via WebRTC (server hanya signaling, tanpa STUN/TURN: khusus satu jaringan)
- ✅ Lampiran file sementara terenkripsi, otomatis hilang dalam 24 jam (kunci per file hanya di teks catatan; pilih file, seret, atau tempel)
- 🔜 Bring Your Own Storage: backup `.md` ke Google Drive / Dropbox / GitHub Gist
- 💡 Steganografi: catatan disembunyikan di gambar PNG
- 💡 Smart link unfurling / kartu pintar ⚠️ E2E: server mengambil URL (bocor link) → opt-in
- 💡 Live link checker (🟢/🔴/🟡) ⚠️ E2E: opt-in, server melihat URL-nya
- 💡 Auto-redaction data sensitif di tampilan publik (di browser saat membuat link publik: buat salinan tersensor)
- ✅ Dynamic OpenGraph preview: status untuk catatan terenkripsi, judul untuk catatan publik (cuplikan isi belum)

## Fase 6: Akun & monetisasi

- ✅ **Claimable notes**: catatan tamu otomatis pindah ke akun saat daftar/login (vault lokal → akun)
- 🔜 Auto-grouping pintar (berisi link / to-do / kode / sedang di-share), dihitung di browser
- ✅ Profil publik `/@username`: catatan publik pilihan (pill "Di profil saya"), hanya link baca saja; 404 jika kosong
- ✅ Auto-archive catatan yang lama tidak dibuka (cold archive)
- ✅ Read receipts (jumlah buka + terakhir dibuka untuk admin; tanpa lokasi)
- ✅ Shadow margin: coretan pribadi di samping catatan orang lain (terenkripsi vault)
- ✅ Login dengan akun SSO (SSO) + frasa brankas (sisi NoteHop; sisi SSO masih perlu `/sso/notehop`)
- 🔜 Login dengan Google (lewat SSO, sudah tercakup SSO di atas)
- 💡 Micro-blog gaya Telegraph + custom domain (Pro)
- 💡 Paywalled note (form email / saweran)
- 💡 AI magic action & voice-to-structured-note ⚠️ E2E: dikirim ke AI hanya atas persetujuan eksplisit

---

## Catatan arsitektur & operasional

1. **Bot preview vs burn after reading.** Sudah ditangani di Fase 1: halaman `/n/…` hanya kerangka, isi diambil lewat `POST` setelah tombol diklik. ✅ User-Agent bot preview (`WhatsApp`, `TelegramBot`, `Discordbot`, `Slackbot`, dll.) mendapat halaman OpenGraph saja; kartu menampilkan status (terenkripsi, password, burn, batas waktu), judul hanya untuk catatan publik.
2. **Isolasi domain.** Domain utama untuk login/dashboard/editor, domain pendek terpisah untuk link publik/tamu. Kalau domain share dilaporkan (phishing/spam), aplikasi utama aman dan mirror bisa disiapkan. ✅ `NOTEHOP_SHARE_URL`: link untuk pembaca memakai domain share, link admin/login/dashboard di domain utama, cookie sesi hanya di domain utama.
3. **`noindex` default.** ✅ semua halaman catatan. 🔜 Opsi index hanya untuk profil publik akun terverifikasi.
4. **Migrasi skema.** ✅ Berurutan dan bernomor (`PRAGMA user_version`). Hanya boleh menambah di akhir daftar; cabang paralel yang sama-sama menambah migrasi harus menomori ulang saat merge.
5. **Skema DB tanpa 50 kolom.** ✅ Kolom `config TEXT` (JSON di SQLite / JSONB kalau pindah ke Postgres) yang di-map ke struct Go untuk fitur opsional (scheduled lock, decoy, webhook, sign-off, cron). Kolom yang diindeks tetap terpisah (`expires`, `owner_id`, link token).
6. **Race condition.** ✅ hitung view atomik. ✅ Saat real-time aktif: server tidak bisa membaca isi (E2E), jadi satu browser (penulis tertua) yang menyimpan; 409 diselesaikan otomatis karena dokumen Yjs sudah berisi semua edit.
7. **Mobile-first.** ✅ Sticky keyboard bar karena `/` sulit dijangkau di keyboard HP.
8. **Pipeline hook di Go.** ✅ `internal/app/features.go`: `beforeRead`, `beforeWrite`, `afterUpdate`, `meta`, `every`. Fitur baru = satu field di `NoteConfig` + satu file `.go` (contoh: `lock.go`).
9. **Background worker.** ✅ janitor per menit (kedaluwarsa, sesi) + job fitur lewat `every`. ✅ Dipakai untuk scheduled lock, time-capsule, cron reset, lampiran 24 jam.
10. **UI 5 zona.** Command palette (`/`, `Ctrl+K`) · bubble toolbar seleksi · drawer "Share & Shield" kanan (tab: Hak akses / Keamanan & waktu / Ekstra & integrasi) · view switcher pill · status bar bawah 24px.
