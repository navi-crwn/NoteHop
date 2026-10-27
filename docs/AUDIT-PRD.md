# Audit PRD: Master Blueprint vs. yang sudah dibangun

Dokumen ini mencocokkan **Master Blueprint (PRD) pemilik** (54 fitur, 5 zona UI, rekomendasi teknis) dengan kode di branch `claude/relaxed-dijkstra-rnbh9p`. Statusnya diperiksa langsung ke kode, bukan dari ingatan.

- ✅ **Ada**, sesuai PRD (kadang bentuknya sedikit beda, ditulis di kolom catatan)
- 🟡 **Sebagian**: inti fitur ada, sebagian isi PRD belum
- ❌ **Belum ada**
- ⛔ **Sengaja tidak dibuat** (alasan di `docs/PROGRESS.md`)

Kolom **Cara cek** dipakai untuk audit langsung di <https://note.example.com>. Nama menu ditulis sesuai UI berbahasa Indonesia (ganti bahasa lewat ikon globe; default situs English).

**Ringkasan:** dari 54 fitur, **27 ✅**, **21 🟡**, **4 ❌**, **2 ⛔**. Untuk 5 zona UI: 1 ✅, 3 🟡, 1 ❌. Beberapa rekomendasi teknis PRD diganti dengan alternatif yang lebih cocok untuk satu VPS dan zero-knowledge (bagian 3).

---

## Bagian 1: 54 fitur

### A. Kontrol link, hak akses, privasi

| # | Fitur | Status | Catatan | Cara cek |
| --- | --- | --- | --- | --- |
| 1 | Multi-link per catatan | ✅ | Bukan otomatis 3 link: ada link admin + link utama, lalu tambah link per mode (Edit, Baca, Tambah, Centang, Usul) dan matikan satu per satu | Bagikan → tambah link mode lain → matikan satu, yang lain tetap jalan |
| 2 | Append-only | ✅ | Mode "Tambah" | Buat link Tambah → buka → hanya bisa menambah entri |
| 3 | Interactive read-only | ✅ | Mode "Centang" + polling bisa dipilih pembaca | Link Centang → centang checkbox, teks tetap terkunci |
| 4 | Usul edit | ✅ | Pembaca mengedit salinan lalu mengirim usulan; admin Terima/Tolak. Belum per blok teks yang disorot | Mode Usul → kirim usulan → buka sebagai admin → spanduk "usulan menunggu" |
| 5 | Freeze/snapshot | ✅ | | Bekukan versi → link baru baca-saja, asli tetap bisa diedit |
| 6 | Burn after reading | ✅ | 1/3/10× + kedaluwarsa | Buat dengan Hancur 1× → buka 2× |
| 7 | Partial burn | ✅ | Blok `:::burn` (menu `/` → Blok hancur sendiri) | Sisipkan blok hancur → buka di 2 browser |
| 8 | Scheduled lock | ✅ | Pengaturan lanjutan → Kunci otomatis | Set 2 menit → link edit jadi baca-saja |
| 9 | Time capsule | ✅ | Hitung mundur, isi baru dikirim server setelah waktunya | Set 2 menit → buka sebagai pembaca |
| 10 | Enkripsi di browser | ✅ | AES-256-GCM, kunci di `#k=`, password opsional (PBKDF2) | Di VPS: isi kolom `content` di DB harus acak |
| 11 | Decoy password | ✅ | Butuh password utama | Buat dengan password + password umpan → coba keduanya |
| 12 | Auto-redaction | 🟡 | Ada "Salinan tersensor" (email, HP, IP, kartu, API key) saat menyalin. Belum sebagai tampilan publik yang tersensor | Lainnya → Salinan tersensor |
| 13 | Sign-off lock | 🟡 | Nama + waktu + snapshot otomatis. **IP tidak dicatat** (pilihan privasi) | Lanjutan → Tanda tangan → pembaca tanda tangan |
| 14 | Geo-fencing / IP whitelist | 🟡 | IP/CIDR allowlist ada. Geo-fencing tidak (butuh database GeoIP) | Lanjutan → Daftar IP diizinkan, isi IP lain → ditolak |
| 15 | Honeypot / watermark | 🟡 | Zero-width berisi **token link** yang dipakai, bisa dilacak di kotak "Lacak kebocoran". Bukan IP/email pembuka | Nyalakan → salin dari link pembaca → tempel di Lacak |
| 16 | Steganografi PNG | ⛔ | Aplikasi chat mengompres ulang gambar, data rusak | |
| 17 | Dead man's switch | 🟡 | Catatan terbuka bagi pemegang link jika admin tidak muncul N hari. **Tidak mengirim email/Telegram** | Lanjutan → Dead man's switch |

### B. Kolaborasi real-time dan berbagi file

| # | Fitur | Status | Catatan | Cara cek |
| --- | --- | --- | --- | --- |
| 18 | Kursor tamu | ✅ | Nama samaran + warna, bisa diganti | Mode Edit di 2 perangkat, ketik bersamaan |
| 19 | Follow presenter | 🟡 | Scroll dan slide ikut. Highlight teks belum | Tombol Presentasi → perangkat lain klik Ikuti |
| 20 | Mini-chat | ✅ | Terenkripsi, tidak disimpan | Tombol Obrolan |
| 21 | Shadow margin | ✅ | Untuk akun; di menu Lainnya | Login → Lainnya → Catatan pinggir pribadi |
| 22 | Diff slider | 🟡 | Slider riwayat **versi** (tersimpan tiap ≥5 menit) + diff warna. Bukan putar ulang per ketikan dengan warna editor | Riwayat |
| 23 | Diff checker | ✅ | `/diff`, bisa dibagikan | Buka `/diff` |
| 24 | Split-screen 2 catatan | ❌ | | |
| 25 | P2P file drop (WebRTC) | 🟡 | Ada di `/nearby` untuk perangkat **satu jaringan**. Belum antar orang yang membuka catatan yang sama dari jaringan berbeda (butuh STUN/TURN) | `/nearby` di HP + laptop satu Wi-Fi |
| 26 | Lampiran 24 jam | ✅ | Terenkripsi per file | `/` → Lampirkan file |

### C. Editor dan blok interaktif

| # | Fitur | Status | Catatan | Cara cek |
| --- | --- | --- | --- | --- |
| 27 | Markdown + `/` | ✅ | | Ketik `/` |
| 28 | View switcher | 🟡 | Teks ⇄ Papan (Kanban). Form = blok `:::form`, Slide/Teleprompter di menu Lainnya. Belum satu pill berisi semuanya | Catatan dengan `## Kolom` + list |
| 29 | Smart paste | 🟡 | Tabel Excel ✅, gambar → WebP ✅. Bahasa kode dideteksi saat ditampilkan, **bukan** saat paste (tidak otomatis jadi blok kode) | Paste dari Sheets; paste screenshot |
| 30 | Template `{{variabel}}` | ✅ | | Buat `Halo {{nama}}`, buka sebagai pembaca |
| 31 | Inline math | 🟡 | Blok ```` ```calc ```` (baris demi baris, dengan variabel). Belum di tengah kalimat biasa | `/` → Kalkulator |
| 32 | Spoiler / PIN | ✅ | | `/` → Spoiler, Blok terkunci PIN |
| 33 | Mini-poll | ✅ | | `/` → Polling |
| 34 | Mermaid + sketsa | 🟡 | Mermaid ✅. Kanvas sketsa ❌ | `/` → Diagram |
| 35 | Timestamped media | 🟡 | Untuk **lampiran audio/video**, `Alt+T` atau `/`. Bukan YouTube (iframe pihak ketiga melanggar CSP dan membocorkan pembaca ke Google) | Lampirkan audio, tulis `[0:10]` |
| 36 | Live synced blocks | 🟡 | `:::embed <link>` menampilkan catatan lain dan menyegarkan tiap 10 detik. Satu arah, seluruh catatan, bukan blok yang bisa diedit di banyak tempat | `/` → Sematkan catatan |
| 37 | Text utilities | ✅ | Urut, dedup, trim, besar/kecil, ekstrak link, format/minify JSON, nomori, hitung | `Ctrl+K` |
| 38 | Regex find-replace | ❌ | Tidak ada, baik di satu catatan maupun seluruh akun | |
| 39 | Copy per blok | ✅ | Per paragraf, item list, tabel, kutipan, kode | Buka sebagai pembaca, arahkan kursor ke paragraf |
| 40 | Zen + suara + pomodoro | 🟡 | Zen + pomodoro + satu suara (brown noise). Belum pilihan hujan/kafe dan target jumlah kata | Lainnya → Mode zen |

### D. Developer, automasi, monetisasi

| # | Fitur | Status | Catatan | Cara cek |
| --- | --- | --- | --- | --- |
| 41 | `/raw/id` | ✅ | Hanya catatan publik | `curl https://note.example.com/raw/<slug>` |
| 42 | SSH | 🟡 | Buat dan baca **catatan publik** (`NOTEHOP_SSH_ADDR`). Belum menambah ke catatan yang ada. Catatan terenkripsi pakai CLI `notehop new/get` | `ssh -p 2222 note.example.com < file.md` |
| 43 | Mock API | 🟡 | JSON (seluruh teks atau blok ```` ```json ````), CORS terbuka. YAML belum | `curl …/mock/<link>/path/0` |
| 44 | Webhook-to-note | ✅ | Kotak masuk `POST /in/<token>`, disimpan terenkripsi | Lanjutan → Kotak masuk → `curl -X POST` |
| 45 | Bot + Watch note | 🟡 | Bot **Telegram** mengirim pesan ke kotak masuk catatan (bukan membuat catatan baru). Watch = webhook saja. WA/Discord dan notifikasi email/bot belum | Butuh `TELEGRAM_BOT_TOKEN` |
| 46 | Email-to-note | ❌ | Butuh server mail. Alternatif: layanan inbound email → kotak masuk | |
| 47 | Cron reset | 🟡 | Centang direset harian/mingguan/bulanan. Arsip isi lama sebelum reset belum | Lanjutan → Checklist berulang |
| 48 | Link checker + unfurl | ❌ | Server akan melihat semua URL di catatan (bertentangan dengan zero-knowledge) | |
| 49 | Same-WiFi pad | ✅ | `/nearby`, isi lewat WebRTC langsung | 2 perangkat satu Wi-Fi |
| 50 | Fork/clone | ✅ | Lainnya → Duplikat | |
| 51 | Slug, QR, OpenGraph | ✅ | OG berisi judul (catatan publik) dan status kunci. Gambarnya **statis** (`og.png`), bukan thumbnail per catatan | Tempel link di WA/Telegram |
| 52 | Ekspor PNG, TTS, web clipper | 🟡 | TTS ✅, clipper berupa **bookmarklet** (bukan ekstensi), ekspor PNG ❌ | Lainnya → Dengarkan; halaman "Cara kerja" → bookmarklet |
| 53 | Manajemen akun | 🟡 | Grup otomatis, wiki-link + graf, arsip 90 hari, profil `/@user`, ekspor/impor JSON ✅. Backup otomatis ke Google Drive/Gist ❌ | Dashboard |
| 54 | AI, voice, custom domain, paywall | ⛔ | Isi harus dikirim ke layanan AI, pembayaran di luar inti; ditunda ke versi Pro | |

---

## Bagian 2: Zona UI dan detail UX

| Zona / detail | Status | Kenyataan sekarang |
| --- | --- | --- |
| 1. Command palette (`/`, `Ctrl+K`) | ✅ | Semua blok + utilitas teks. Belum regex dan AI |
| 2. Floating bubble toolbar | 🟡 | Muncul saat menyorot teks (mouse): tebal, miring, kode, link, `{{var}}`, palette. Belum warna, salin link baris, sensor, usul, AI |
| 3. Drawer "Share & Shield" | 🟡 | Berupa **dialog** Bagikan dengan daftar link + bagian **Pengaturan lanjutan** (pill). Bukan panel geser 3 tab |
| 4. View switcher pill | 🟡 | Teks/Papan saja (lihat #28) |
| 5. Smart status bar | ❌ | Status simpan ada di kepala catatan; tidak ada bar bawah berisi jumlah kata, tamu, Zen, RAW/API/SSH |
| Contextual UI ("Format JSON?", "Cek link?") | ❌ | |
| Mobile sticky keyboard bar | 🟡 | `/`, Checklist, Tempel, Blok PIN, Lampirkan, Selesai. Tombol Share belum |
| Silent onboarding (claimable) | 🟡 | Catatan tamu diingat **di browser** (bukan cookie sesi server); setelah login, dashboard menawarkan "Klaim" |

---

## Bagian 3: Rekomendasi teknis PRD vs. pilihan yang dipakai

| PRD | Dipakai | Alasan |
| --- | --- | --- |
| Golang | ✅ Go 1.27 | |
| Yjs + WebSocket (`nhooyr`/`gorilla`) | ✅ Yjs + `coder/websocket` | `coder/websocket` adalah kelanjutan resmi `nhooyr.io/websocket` |
| TipTap / BlockNote | Textarea Markdown + pratinjau | Tanpa build step dan tanpa framework (aturan proyek); semua blok berupa Markdown yang bisa dibaca CLI/raw |
| Charmbracelet Wish (SSH) | `golang.org/x/crypto/ssh` | Dependensi sudah ada, cukup untuk perintah sederhana |
| Asynq + Redis / gocron | Janitor di dalam proses (tiap menit) | Satu VPS, satu binary; tidak perlu Redis |
| PostgreSQL + Redis, kolom `settings JSONB` | SQLite + kolom `config` JSON (`NoteConfig`) | Sama idenya (fitur opsional di JSON, bukan 50 kolom). SQLite cukup untuk satu server dan backup cukup satu file |
| `plain_text` untuk full-text search | Tidak ada | Server tidak boleh melihat teks (zero-knowledge); pencarian dilakukan di browser (dashboard) |
| Debounce di Redis untuk edit bersama | Penulis tertua di room yang menyimpan | Server tidak bisa membaca/menggabungkan isi terenkripsi; Yjs di browser yang menggabungkan |
| `DECR` Redis untuk burn | Transaksi SQLite atomik | Diuji dengan tes konkuren (`TestBurnConcurrent`) |
| IndexedDB + PWA, edit offline | PWA, **baca** offline (Cache API) | Edit offline + sinkron belum dibuat |
| Filter User-Agent bot + tombol "Buka" | ✅ | |
| Domain share terpisah | ✅ `NOTEHOP_SHARE_URL` | |
| `X-Robots-Tag: noindex` | 🟡 | `<meta robots noindex>` di halaman catatan/dashboard; header `X-Robots-Tag` baru di halaman preview bot |
| Hooks `BeforeNoteRead` / `AfterNoteUpdate` | ✅ `beforeRead`, `beforeWrite`, `afterUpdate`, `meta`, `every` | `internal/app/features.go` |

---

## Bagian 4: Kandidat pekerjaan berikutnya

Dikelompokkan dari celah di atas. **Jangan dikerjakan tanpa persetujuan pemilik**; pilih dulu mana yang masuk.

| Kelompok | Isi | Catatan E2E |
| --- | --- | --- |
| F1 Editor | Regex find-replace (satu catatan; seluruh akun di browser), inline math di kalimat, deteksi kode saat paste, pilihan suara Zen + target kata, split-screen 2 catatan | Semua di browser, aman |
| F2 UI | Status bar bawah, view switcher satu pill (Teks/Papan/Form/Slide/Diff), drawer Share 3 tab, bubble toolbar lengkap, tombol Share di keyboard bar HP, contextual UI (JSON) | Aman |
| F3 Privasi | Tampilan publik tersensor, highlight di follow presenter, sign-off mencatat IP (opsional) | IP = data pribadi, perlu persetujuan |
| F4 Integrasi | Notifikasi Watch lewat Telegram/email, bot Discord, SSH append, Mock API YAML, arsip sebelum reset | Notifikasi hanya metadata |
| F5 Lanjutan | Ekspor PNG, sketsa kanvas, edit offline + sinkron, backup ke Gist/Drive, X-Robots-Tag di semua respons, thumbnail OG per catatan (publik saja) | Backup: dienkripsi di browser dulu |
| ⛔ | Steganografi, link checker/unfurl, email-to-note, AI, paywall, custom domain | Lihat `docs/PROGRESS.md` |
