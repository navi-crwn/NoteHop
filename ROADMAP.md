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

- 🔜 Real-time: WebSocket di Go (nhooyr/coder `websocket`) + **Yjs (CRDT)**, isi update tetap terenkripsi (relay ciphertext)
- 🔜 Anonymous guest cursors (nama samaran + warna)
- 🔜 Ephemeral mini-chat (hanya di memori, hilang saat semua keluar)
- 🔜 Follow Presenter Mode (scroll & highlight sinkron)
- 🔜 Diff slider / mesin waktu riwayat (riwayat versi terenkripsi)
- 🔜 Side-by-side diff checker (bandingkan 2 teks, link baca saja)
- 🔜 Split-screen dua catatan
- 🔜 Smart paste gambar → WebP terkompresi (lampiran terenkripsi)
- 🔜 Mermaid diagram & kanvas coretan
- 🔜 Command palette `Ctrl+K` + floating bubble toolbar saat teks diseleksi
- ✅ Sticky keyboard bar di HP (`/`, to-do, tempel, blok PIN, selesai), menempel di atas keyboard lewat `visualViewport`
- 🔜 Offline-first (PWA + IndexedDB, sinkron saat online lagi)
- 🔜 Mode "Usul Edit" (suggesting/pull request): pengunjung read-only mengusulkan, pemilik Terima/Tolak
- ✅ Copy button per baris/blok di mode baca (paragraf, item daftar, kutipan, tabel sebagai TSV)

## Fase 3: Kontrol waktu & akses lanjutan

- ✅ **Scheduled Lock**: link edit/tambah/centang otomatis jadi read-only pada jam tertentu (server-side, aman E2E); link admin tetap bisa mengubah
- 🔜 **Time-Capsule / Embargo**: isi baru bisa dibuka setelah waktu tertentu, dengan countdown (server menahan ciphertext sampai waktunya)
- 🔜 **Partial Burn**: satu blok di dalam catatan hancur setelah dilihat 1× (blok disimpan terpisah dengan penghitung sendiri)
- 🔜 **Decoy Password**: password kedua membuka catatan palsu + peringatan diam-diam ke pemilik (dua ciphertext, server tidak tahu mana yang asli)
- 🔜 **Sign-Off Lock**: tombol setuju/tanda tangan, catat waktu (+IP opsional), lalu kunci catatan permanen (otomatis jadi snapshot)
- 🔜 **Cron reset / catatan berulang**: misalnya setiap Senin 06:00 arsipkan ke riwayat lalu reset centang
- 💡 Geo-fencing / IP whitelist (Pro/Bisnis)
- 💡 Honeypot: watermark tipis / zero-width per pengunjung (anti screenshot-sebar)
- 💡 Dead Man's Switch: kirim catatan ke orang kepercayaan jika pemilik tidak aktif X bulan (kunci dititipkan terenkripsi untuk penerima)

## Fase 4: Smart blocks (catatan hidup)

- 🔜 Template fill-in-the-blanks `{{variabel}}`: pengunjung isi kolom, teks langsung siap di-copy (berjalan di browser, aman E2E)
- 🔜 Inline math / kalkulator teks gaya Numi/Soulver (`Konsumsi = 50 * 35.000` → `= 1.750.000`)
- 🔜 Mini-poll / voting di dalam catatan read-only (seperti checklist: server simpan hitungan, bukan teks)
- 🔜 Form-to-Note: catatan tampil sebagai formulir, setiap submit menjadi baris baru (memakai mekanisme append terenkripsi)
- 🔜 Kanban view dari sub-judul To-Do/Proses/Selesai (drag-and-drop untuk pemegang link edit)
- 🔜 View switcher pill: Teks | Kanban | Form | Slide | Diff
- 🔜 Slide mode (`---` jadi slide) & teleprompter
- 🔜 Timestamped media note (`Ctrl+T` → `[04:12]` yang memutar YouTube/audio)
- 🔜 One-click text utilities: hapus duplikat, urutkan A–Z, ekstrak link/email, ubah huruf, format JSON/CSV
- 🔜 Live regex & mass find-replace studio
- 🔜 Wiki-links `[[Judul]]` + graph view (di browser, dari vault akun)
- 🔜 Live synced blocks antar catatan (blok master di-embed ke banyak catatan)
- 🔜 Audio reader (Web Speech API, di browser)
- 🔜 Zen mode + ambient sound + Pomodoro + target jumlah kata
- 🔜 Export: PDF, Markdown, TXT, dan gambar PNG estetik (dibuat di browser)

## Fase 5: Ekosistem, developer & pindah perangkat

- 🔜 SSH access (`echo "log" | ssh notehop.id append/slug`) dengan Charmbracelet Wish ⚠️ E2E: hanya catatan publik, atau klien CLI yang mengenkripsi sendiri
- 🔜 CLI resmi `notehop` (enkripsi lokal, jadi raw/curl tetap zero-knowledge)
- 🔜 Instant Mock API / JSON server (`/api/slug` + CORS) ⚠️ E2E: hanya catatan publik
- 🔜 Webhook-to-Note (URL unik per catatan, baris masuk otomatis) ⚠️ E2E: publik, atau payload dienkripsi pengirim dengan kunci publik catatan
- 🔜 Watch Note: notifikasi perubahan via webhook/Telegram/email (hanya metadata, bukan isi)
- 🔜 Bot Telegram/WhatsApp/Discord: kirim pesan → catatan baru
- 🔜 Email-to-Note ⚠️ E2E: isi diterima server sebelum dienkripsi (pakai kunci publik catatan)
- 🔜 Web clipper / bookmarklet "Kirim ke catatan"
- 🔜 Fork / Clone ke ruang sendiri (di browser, kunci baru)
- 🔜 Same-WiFi Instant Pad (pad sementara per IP publik, mirip PairDrop)
- 🔜 P2P file drop via WebRTC (server hanya signaling)
- 🔜 Lampiran file sementara terenkripsi, otomatis hilang dalam 24 jam
- 🔜 Bring Your Own Storage: backup `.md` ke Google Drive / Dropbox / GitHub Gist
- 💡 Steganografi: catatan disembunyikan di gambar PNG
- 💡 Smart link unfurling / kartu pintar ⚠️ E2E: server mengambil URL (bocor link) → opt-in
- 💡 Live link checker (🟢/🔴/🟡) ⚠️ E2E: opt-in, server melihat URL-nya
- 💡 Auto-redaction data sensitif di tampilan publik (di browser saat membuat link publik: buat salinan tersensor)
- ✅ Dynamic OpenGraph preview: status untuk catatan terenkripsi, judul untuk catatan publik (cuplikan isi belum)

## Fase 6: Akun & monetisasi

- 🔜 **Claimable notes**: catatan tamu otomatis pindah ke akun saat daftar/login (vault lokal → akun)
- 🔜 Auto-grouping pintar (berisi link / to-do / kode / sedang di-share), dihitung di browser
- 🔜 Link hub / profil publik `/@username` (catatan publik pilihan)
- 🔜 Auto-archive catatan yang lama tidak dibuka (cold archive)
- 🔜 Read receipts & access log (jumlah buka, terakhir diedit; lokasi kasar opsional)
- 🔜 Shadow margin: coretan pribadi di samping catatan orang lain (terenkripsi vault)
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
6. **Race condition.** ✅ hitung view atomik. 🔜 Saat real-time aktif: buffer perubahan di memori Go dan flush ke DB setiap 3–5 detik atau saat room kosong.
7. **Mobile-first.** ✅ Sticky keyboard bar karena `/` sulit dijangkau di keyboard HP.
8. **Pipeline hook di Go.** ✅ `internal/app/features.go`: `beforeRead`, `beforeWrite`, `afterUpdate`, `meta`, `every`. Fitur baru = satu field di `NoteConfig` + satu file `.go` (contoh: `lock.go`).
9. **Background worker.** ✅ janitor per menit (kedaluwarsa, sesi) + job fitur lewat `every`. 🔜 Dipakai untuk scheduled lock, time-capsule, cron reset, lampiran 24 jam.
10. **UI 5 zona.** Command palette (`/`, `Ctrl+K`) · bubble toolbar seleksi · drawer "Share & Shield" kanan (tab: Hak akses / Keamanan & waktu / Ekstra & integrasi) · view switcher pill · status bar bawah 24px.
