# Panduan fitur NoteHop: fungsi, cara pakai, batasan, kasus, mitigasi

Panduan untuk pemilik (dan agen VPS) tentang **semua fitur** yang ada di NoteHop: 54 fitur Master Blueprint ditambah fitur pendukung (akun, nama link, moderasi). Tiap fitur dijelaskan dalam lima bagian:

- **Fungsi**: gunanya untuk apa.
- **Cara pakai**: jalur di UI (nama menu versi Bahasa Indonesia; situs default English).
- **Batasan**: angka dan hal yang memang tidak bisa.
- **Kasus**: contoh masalah nyata yang bisa terjadi.
- **Mitigasi**:
  - ✅ = sudah ada di kode.
  - 💡 = usulan, belum dibuat, menunggu persetujuan pemilik.

Status per fitur (✅/🟡/❌/⛔) dan cara cek di website ada di `docs/AUDIT-PRD.md`. Dokumen ini fokus ke **risiko dan solusi**.

---

## 0. Aturan dasar yang berlaku untuk semua fitur

### 0.1 Kunci dan zero-knowledge

- Isi catatan dienkripsi di browser (AES-256-GCM). Kuncinya ada di bagian `#k=…` link, dan bagian `#` **tidak pernah dikirim** ke server.
- Bukti boleh menulis (`X-Write`) dan token pemilik (`X-Admin`) disimpan server hanya dalam bentuk hash.

| Kasus | Mitigasi |
| --- | --- |
| Link dikirim lewat WA/Telegram, sehingga riwayat chat ikut menyimpan kunci. Siapa pun yang membuka HP itu bisa membaca catatan. | ✅ Opsi **Pakai sandi**: sandi dikirim lewat jalur lain, dan link saja tidak cukup.<br>✅ **Sekali baca**, kedaluwarsa.<br>✅ Matikan link lama di dialog Bagikan. |
| Link pemilik bocor, lalu orang lain bisa menghapus atau mengubah pengaturan. | ✅ Kotak peringatan merah di dialog Bagikan.<br>✅ Link pemilik terpisah dari link yang dikirim. |
| Pemilik kehilangan link pemilik. | Server **tidak bisa** memulihkan (sengaja: server tidak punya kunci).<br>✅ Catatan yang dibuat saat login tersimpan di brankas akun (dashboard).<br>✅ Ekspor JSON dari dashboard. |
| Lupa sandi catatan. | Tidak bisa dipulihkan (sengaja).<br>💡 Tulis di FAQ/UI dengan lebih tegas saat sandi diisi. |

### 0.2 Batas angka (rate limit dan ukuran)

| Batas | Nilai |
| --- | --- |
| Buat catatan | 30/menit per IP |
| Simpan/tulis lain | 120/menit per IP |
| Login/daftar/brankas | 15/menit per IP |
| Upload file | 20/menit per IP, dan `NOTEHOP_FILES_IP_MB` (default 100 MB) per IP per 24 jam |
| Baca `/raw` dan `/mock` | 120/menit per IP |
| Kotak masuk | 60/menit per IP |
| `/nearby` | 30 sambungan/menit per IP |
| SSH | 20/menit per IP |
| Ukuran catatan (setelah enkripsi) | 1 MB |
| Entri balasan | 64 KB per entri, 2000 entri |
| Riwayat | 100 versi per catatan; edit yang berjarak < 5 menit dihitung satu versi |
| Kedaluwarsa | maksimal 365 hari |
| Hapus setelah N kali dibuka | maksimal 1000 |

- Di belakang nginx/Caddy, **wajib** `NOTEHOP_TRUST_PROXY=true`. Kalau tidak, semua pengunjung terlihat sebagai `127.0.0.1` dan berbagi satu jatah rate limit.
- **Kasus: satu kantor/kampus di balik satu IP (CGNAT) kena rate limit bersama.** Batas di atas cukup longgar untuk pemakaian manusia.
  - 💡 Naikkan per bucket lewat env jika ada keluhan.

### 0.3 Penyalahgunaan dan moderasi

| Kasus | Mitigasi |
| --- | --- |
| Catatan **publik** dipakai untuk phishing, spam, atau konten ilegal. | ✅ `notehop takedown LINK` menghapus catatan dan **mengunci nama linknya 365 hari**.<br>✅ `notehop takedown -file ID` untuk lampiran.<br>✅ Halaman `/contact`, `/terms`, `/privacy`. |
| Catatan **terenkripsi** berisi konten ilegal. | Server tidak bisa membaca isinya. Penanganan berdasarkan laporan yang menyertakan link lengkap (dengan `#k=`): admin bisa membukanya di browser lalu takedown. |
| Pengguna menebak link orang lain. | Link acak 10 karakter dari 56 simbol (≈ 3×10¹⁷ kemungkinan) plus rate limit. Tanpa `#k=`, isi tetap tidak terbaca. |
| Banjir pendaftaran akun. | ✅ Rate limit 15/menit.<br>✅ `NOTEHOP_REGISTRATION=false` untuk menutup pendaftaran. |
| Disk penuh oleh lampiran. | ✅ Batas total `NOTEHOP_FILES_TOTAL_MB` (default 2048).<br>✅ File dihapus otomatis setelah 24 jam. |
| Disk penuh oleh catatan. | ✅ 1 MB per catatan, 100 versi.<br>💡 Kuota per IP per hari. |

---

## 1. Nama link, username, dan recycle (fitur baru di patch ini)

**Fungsi.** Nama link sendiri (`/n/rapat-senin`) dan username (`/@budi`) diatur supaya:
- tidak bisa dipakai untuk menyamar,
- tidak bisa memakai kata kasar,
- **link lama tidak pernah membuka catatan orang lain**.

**Cara pakai.**
- Halaman Buat → Pengaturan lanjutan → **Nama link sendiri**.
- Dialog Bagikan → tambah link → isi nama.
- Username diisi saat daftar.

**Aturan.**

| Aturan | Isi |
| --- | --- |
| Panjang | Minimal **7 karakter** untuk nama link sendiri. Admin situs (`NOTEHOP_ADMINS`) boleh 3–6 karakter, misalnya untuk link marketing `/n/promo`. |
| Bentuk | Huruf kecil, angka, `-`, `_`. Tidak boleh diakhiri `-` atau berisi `--`. |
| Nama cadangan | Nama situs/sistem/merek: `admin`, `api`, `faq`, `help`, `contact`, `terms`, `privacy`, `login`, `notehop`, dll. (merek Anda sendiri lewat `NOTEHOP_RESERVED`).<br>Awalan `notehop-*`, `official-*`, `admin-*`, `support-*`.<br>Daftar di `internal/app/names/reserved.txt`; tambahan lewat env `NOTEHOP_RESERVED=promo,event2026`. |
| Kata kasar | Daftar di `internal/app/names/blocked.txt` (9 bahasa, huruf Latin). Dicek per kata utuh, jadi `class-notes` tidak ikut tertolak karena berisi "ass". |
| Masa tunggu (recycle) | Saat catatan dihapus atau kedaluwarsa, nama linknya **ditahan 90 hari**.<br>Pemilik akun yang sama boleh langsung memakainya lagi; orang lain menunggu.<br>Catatan yang di-takedown: ditahan **365 hari** untuk semua orang. |
| Link acak | Tidak perlu ditahan karena tidak akan pernah sama (lihat 0.3). |

**Kasus dan mitigasi.**

| Kasus | Mitigasi |
| --- | --- |
| Orang A menghapus `/n/undangan-nikah`. Besoknya orang B membuat nama yang sama berisi penipuan, dan tamu undangan A membuka link lama. | ✅ Ditahan 90 hari. |
| Penipu membuat `/n/notehop-support` atau `/@admin` untuk meminta sandi. | ✅ Nama cadangan + awalan.<br>💡 Tambahkan nama brand lain ke `NOTEHOP_RESERVED`. |
| Nama pendek habis dipakai orang (`/n/a`, `/n/cv`). | ✅ Minimal 7 karakter.<br>Nama pendek untuk admin saja. |
| Kata kasar di link publik. | ✅ Daftar blokir.<br>Batas: varian ejaan (`b4ngs4t`) dan huruf non-Latin tidak tertangkap. 💡 Tambahkan ke `blocked.txt` saat ditemukan. |
| Nama tertahan padahal pemiliknya ingin memakai lagi tanpa login. | Tanpa akun, server tidak tahu siapa pemiliknya, jadi tetap menunggu 90 hari. Admin bisa melepas lewat DB (`DELETE FROM slug_holds WHERE slug=…`). |
| Username dari single sign-on kena aturan (misal `admin`). | ✅ Otomatis diganti `admin_xxxxx`. |

---

## 2. Kontrol link, hak akses, privasi (fitur 1–17)

### 1. Banyak link per catatan
- **Fungsi.** Satu catatan bisa punya beberapa link dengan hak berbeda:
  - Bisa menulis
  - Hanya baca
  - Balas (tambah saja)
  - Centang
  - Usul
- **Cara pakai.** Dialog **Bagikan** → Pengaturan lanjutan → Link lain → pilih jenis → Tambah. Matikan per link dengan tombol hapus.
- **Batasan.** Link pemilik hanya satu dan tidak bisa diganti.
- **Kasus.** Link "bisa menulis" terkirim ke grup yang salah.
  - ✅ Matikan link itu saja; link lain tetap jalan.
  - ✅ Riwayat untuk mengembalikan isi.

### 2. Balas saja (append-only)
- **Fungsi.** Pembaca hanya bisa **menambah** entri di bawah (buku tamu, laporan harian).
- **Cara pakai.** Bagikan → link **Balas**.
- **Batasan.** 64 KB per entri, 2000 entri.
- **Kasus.** Spam entri.
  - ✅ Rate limit 120/menit.
  - ✅ Pemilik bisa menghapus entri.
  - 💡 Batas entri per IP.

### 3. Centang (interactive read-only)
- **Fungsi.** Pembaca bisa mencentang checkbox dan memilih di polling, tapi tidak bisa mengubah teks.
- **Cara pakai.** Bagikan → link **Centang**.
- **Batasan.** Maksimal 5000 checkbox. Checkbox di dalam blok kode tidak dihitung.
- **Kasus.** Dua orang mencentang bersamaan.
  - ✅ Setiap centang dikirim sebagai perubahan satu nomor, jadi tidak saling menimpa.

### 4. Usul edit
- **Fungsi.** Pembaca mengedit salinan lalu mengirim usulan; pemilik memilih **Terima** atau **Tolak**.
- **Cara pakai.** Bagikan → link **Usul**. Pemilik melihat spanduk "usulan menunggu".
- **Batasan.** 200 usulan. Usulan berlaku untuk seluruh teks, bukan per kalimat.
- **Kasus.** Usulan dibuat dari versi lama, lalu diterima dan menimpa perubahan terbaru.
  - ✅ Pemilik melihat diff sebelum menerima.
  - ✅ Riwayat menyimpan versi sebelumnya.

### 5. Salinan tetap (freeze)
- **Fungsi.** Membuat link baca-saja dari isi **saat ini**; catatan asli tetap bisa diedit.
- **Cara pakai.** Menu **⋯** → Salinan tetap.
- **Kasus.** Kontrak dibekukan lalu asli diubah; pihak lain mengira link lama ikut berubah.
  - ✅ Salinan tidak ikut berubah, jadi berguna sebagai bukti.

### 6. Hapus setelah dibaca (burn)
- **Fungsi.** Catatan hilang setelah dibuka 1/3/10 kali (maks 1000).
- **Cara pakai.** Buat → "Seberapa rahasia?" → **Sekali baca**. Jumlah lain di Pengaturan lanjutan.
- **Batasan.** Pembaca tetap bisa memotret layar atau menyalin. "Hilang" berarti hilang dari server, bukan dari ingatan pembaca.

| Kasus | Mitigasi |
| --- | --- |
| Bot pratinjau WA/Telegram membuka link duluan sehingga jatah habis. | ✅ Isi baru diambil setelah orang menekan tombol "Buka". |
| Dua orang membuka bersamaan sehingga jatah 1 terpakai dua kali. | ✅ Transaksi atomik (diuji `TestBurnConcurrent`). |
| Penerima salah klik lalu menutup tab. | Tidak bisa dibuka lagi (sengaja).<br>💡 Kalimat peringatan sebelum tombol Buka sudah ada; bisa diperjelas. |

### 7. Blok hancur sendiri (partial burn)
- **Fungsi.** Hanya satu bagian catatan yang hilang setelah dibaca.
- **Cara pakai.** Ketik `/` → **Blok hancur sendiri**.
- **Batasan.** 50 blok per catatan.
- **Kasus.** Sama dengan nomor 6.

### 8. Kunci otomatis (scheduled lock)
- **Fungsi.** Link yang bisa menulis berubah jadi baca-saja pada waktu tertentu (misalnya batas pengumpulan tugas).
- **Cara pakai.** Pengaturan lanjutan → Waktu → **Kunci otomatis**.
- **Batasan.** Tidak bisa aktif bersamaan dengan fitur waktu yang bentrok; aturannya dicek di UI **dan** server.
- **Kasus.** Jam di HP pengguna salah.
  - ✅ Yang dipakai jam server.

### 9. Kapsul waktu
- **Fungsi.** Isi baru dikirim server setelah tanggal tertentu.
- **Cara pakai.** Pengaturan lanjutan → Waktu → **Buka pada tanggal**.
- **Batasan.** Pemilik tetap bisa membaca sebelum waktunya.
- **Kasus.** Pembaca mencoba membaca lebih awal lewat API.
  - ✅ Server menahan ciphertext sampai waktunya, jadi kunci di link saja tidak cukup.

### 10. Enkripsi di browser
- **Fungsi.** Server hanya menyimpan teks acak.
- **Cara pakai.** Otomatis. Sandi tambahan lewat **Pakai sandi** (PBKDF2 310.000 putaran).
- **Batasan.** Catatan **publik** (dipilih pembuat) tidak dienkripsi supaya `/raw`, SSH, dan Mock API bisa membacanya.

| Kasus | Mitigasi |
| --- | --- |
| Pengguna menempel link catatan privat (dengan `#k=`) ke catatan publik, sehingga kuncinya ikut terbuka. | ✅ **Baru:** peringatan saat membuat dan menyimpan catatan publik yang berisi `#k=` atau kunci lampiran. |
| Server dibobol. | Penyerang hanya mendapat ciphertext dan hash. Catatan publik tetap terbaca (memang publik). |
| JavaScript server diganti penyerang. | Risiko semua aplikasi web E2E.<br>✅ CSP ketat, tanpa CDN.<br>💡 Subresource integrity / ekstensi verifikasi (jangka panjang). |

### 11. Sandi umpan (decoy)
- **Fungsi.** Sandi kedua membuka isi palsu (untuk situasi dipaksa membuka).
- **Cara pakai.** Pengaturan lanjutan → Privasi ekstra → **Sandi umpan**. Butuh sandi utama.
- **Batasan.** Orang yang tahu NoteHop punya fitur ini bisa curiga. Isi umpan harus meyakinkan.
- **Kasus.** Pemaksa meminta "sandi yang satunya".
  - Tidak ada tanda di UI apakah catatan punya sandi umpan. Itu mitigasi terbaik yang mungkin.

### 12. Salinan tersensor (auto-redaction)
- **Fungsi.** Menyalin isi dengan email, nomor HP, IP, nomor kartu, dan API key disensor.
- **Cara pakai.** Menu **⋯** → Salinan tersensor.
- **Batasan.** Berbasis pola; nama orang dan alamat rumah tidak tertangkap. Tampilan publik tidak tersensor.
- **Kasus.** Format nomor tidak umum lolos.
  - 💡 Tambah pola sesuai laporan.
  - 💡 Tampilan publik tersensor (F3).

### 13. Tanda tangan (sign-off)
- **Fungsi.** Pembaca menulis nama sebagai tanda setuju. Waktu dicatat dan salinan tetap dibuat otomatis.
- **Cara pakai.** Pengaturan lanjutan → Kerja bareng → **Tanda tangan**.
- **Batasan.** 1000 tanda tangan. **Nama tidak diverifikasi**, dan IP tidak dicatat (pilihan privasi).
- **Kasus.** Orang menandatangani atas nama orang lain.
  - Tanda tangan ini hanya bukti ringan, bukan tanda tangan hukum.
  - 💡 Tanda tangan dengan akun (nama = username terverifikasi).
  - 💡 IP opsional (data pribadi, perlu keputusan pemilik).

### 14. Hanya dari jaringan tertentu (IP allowlist)
- **Fungsi.** Catatan hanya bisa dibuka dari IP/CIDR tertentu (kantor, sekolah).
- **Cara pakai.** Pengaturan lanjutan → Privasi ekstra → **Hanya dari jaringan tertentu**.
- **Batasan.**
  - Butuh `NOTEHOP_TRUST_PROXY=true` di belakang proxy.
  - Geo-fencing (negara) tidak ada.
  - IP HP berubah-ubah.
- **Kasus.** Pemilik sendiri terkunci karena IP rumah berganti.
  - ✅ Link pemilik tetap bisa mengubah pengaturan.
- **Kasus.** Satu IP CGNAT dipakai ribuan orang, sehingga allowlist kurang ketat.
  - Gabungkan dengan sandi.

### 15. Pelacak kebocoran (watermark)
- **Fungsi.** Teks yang disalin membawa tanda tak terlihat berisi **link mana** yang dipakai, jadi pemilik tahu link siapa yang bocor.
- **Cara pakai.** Pengaturan lanjutan → Privasi ekstra → **Lacak kebocoran**. Tempel teks bocoran di kotak "Lacak".
- **Batasan.**
  - Tanda bisa hilang jika teks diketik ulang, difoto, atau dibersihkan alat tertentu.
  - Hanya tahu link, bukan orangnya.
- **Kasus.** Semua orang memakai link yang sama, jadi tidak ada yang bisa dibedakan.
  - ✅ Buat **satu link per penerima** (fitur 1).

### 16. Steganografi ⛔
- Tidak dibuat. Aplikasi chat mengompres ulang gambar sehingga data tersembunyi rusak.

### 17. Buka jika tidak aktif (dead man's switch)
- **Fungsi.** Jika pemilik tidak membuka catatan selama N hari, catatan terbuka bagi pemegang link.
- **Cara pakai.** Pengaturan lanjutan → Waktu → **Buka jika saya tidak aktif**.
- **Batasan.** **Tidak mengirim email/Telegram.** Penerima harus sudah memegang link dan mengeceknya sendiri.
- **Kasus.** Pemilik lupa membuka catatan karena liburan, lalu isi terbuka.
  - Pilih N yang cukup panjang.
  - 💡 Pengingat ke pemilik sebelum terbuka (butuh email/bot).

---

## 3. Kolaborasi real-time dan berbagi file (fitur 18–26)

### 18. Tulis bersama + kursor tamu
- **Fungsi.** Beberapa orang mengetik bersamaan. Kursor dan nama samaran terlihat.
- **Cara pakai.** Buka link "bisa menulis" di dua perangkat. Ganti nama lewat ikon orang.
- **Batasan.** 50 orang per catatan, 60 pesan/detik per orang. Server hanya meneruskan data terenkripsi.

| Kasus | Mitigasi |
| --- | --- |
| Koneksi putus saat mengetik. | ✅ Yjs menggabungkan saat tersambung lagi.<br>✅ Riwayat versi. |
| Yang menyimpan ke server adalah "penulis tertua di room"; jika tab-nya tertidur di HP, simpan bisa tertunda. | ✅ Saat penulis keluar, peran pindah otomatis. |
| Orang iseng memakai link tulis untuk menghapus semua isi. | ✅ Riwayat → kembalikan.<br>✅ Matikan link. |

### 19. Ikuti presenter
- **Fungsi.** Penonton mengikuti scroll dan slide presenter.
- **Cara pakai.** Tombol **Presentasi**; penonton klik **Ikuti**.
- **Batasan.** Sorotan teks belum ada.

### 20. Obrolan
- **Fungsi.** Chat terenkripsi di samping catatan; **tidak disimpan**.
- **Cara pakai.** Tombol **Obrolan**.
- **Kasus.** Orang mengira obrolan tersimpan, lalu kehilangan isi saat menutup tab.
  - ✅ Keterangan di panel.
  - Tulis hal penting di catatan.

### 21. Catatan pinggir pribadi
- **Fungsi.** Catatan pribadi di samping catatan orang lain; hanya terlihat oleh akun Anda.
- **Cara pakai.** Login → menu ⋯ → **Catatan pinggir**.
- **Batasan.** 64 KB, butuh akun.

### 22. Riwayat + diff
- **Fungsi.** Melihat dan mengembalikan versi lama, dengan perbedaan berwarna.
- **Cara pakai.** Menu ⋯ → **Riwayat**.
- **Batasan.**
  - 100 versi.
  - Edit berjarak < 5 menit digabung.
  - Bukan putar ulang per ketikan.
- **Kasus.** Perusak mengedit lebih dari 100 kali untuk mendorong versi bagus keluar dari riwayat.
  - Edit berdekatan digabung, jadi butuh > 8 jam edit terus-menerus.
  - 💡 Simpan versi pertama/terpanjang secara permanen.

### 23. Pembanding teks `/diff`
- **Fungsi.** Membandingkan dua teks; hasilnya bisa dibagikan.
- **Cara pakai.** Buka `/diff`.

### 24. Split-screen ❌
- Belum dibuat (kandidat F1).

### 25. Kirim ke perangkat dekat (`/nearby`)
- **Fungsi.** Kirim teks/file langsung antar perangkat satu jaringan (WebRTC). Server hanya mempertemukan perangkat.
- **Cara pakai.**
  - Buka `/nearby` di kedua perangkat.
  - **Baru:** isi **kode 6 angka** yang sama di kedua perangkat, atau buka link `/nearby#123456`.
- **Batasan.**
  - Hanya satu jaringan (tanpa server TURN).
  - 20 perangkat per room.
  - 30 sambungan/menit per IP.

| Kasus | Mitigasi |
| --- | --- |
| Di jaringan CGNAT/kantor/kampus, orang asing ber-IP publik sama melihat perangkat Anda dan bisa menerima file. | ✅ **Baru:** kode pasangan 6 angka. Hanya perangkat dengan kode yang sama yang saling melihat. |
| HP dan laptop beda jaringan (Wi-Fi vs data seluler). | Tidak tersambung.<br>💡 Server TURN (biaya, perlu keputusan). |

### 26. Lampiran 24 jam
- **Fungsi.** File/gambar dienkripsi per file; tempel screenshot langsung jadi WebP.
- **Cara pakai.** `/` → **Lampirkan file**, atau tempel gambar.
- **Batasan.**
  - 10 MB per file (`NOTEHOP_FILE_MB`).
  - Total 2 GB (`NOTEHOP_FILES_TOTAL_MB`).
  - **Hilang setelah 24 jam.**
  - 20 upload/menit.

| Kasus | Mitigasi |
| --- | --- |
| Catatan dibuka minggu depan, gambar sudah hilang. | Sesuai desain. Pembaca melihat keterangan "file sudah kedaluwarsa".<br>💡 Opsi masa simpan lebih lama untuk akun (butuh kuota). |
| Kuota total penuh, upload gagal untuk semua orang. | ✅ Pesan jelas.<br>💡 Kuota per IP. |
| File ilegal. | ✅ `notehop takedown -file ID`. |

---

## 4. Editor dan blok (fitur 27–40)

### 27. Markdown + menu `/`
- **Fungsi.** Format teks dan menyisipkan blok.
- **Cara pakai.** Ketik `/` di awal baris, atau tombol `/` di bar keyboard HP.

### 27b. Tipe catatan dan menu klik kanan ✅
- **Fungsi.** Pilih tipe dari tombol di kiri atas editor: **Biasa** (teks apa adanya), **Dokumen** (Markdown, bawaan) atau **Kode** (pilih bahasa, nomor baris, Tab/Shift+Tab untuk indentasi, Esc lalu Tab untuk keluar dari kotak). Di Dokumen, klik kanan pada teks membuka menu format; di HP tombol **Aa** di bilah keyboard membuka lembar yang sama.
- **Batasan.** Warna, stabilo, ukuran, dan rata disimpan sebagai teks (`[teks]{.red}`, `:::center`), jadi tampil di Pratinjau dan di halaman catatan, bukan langsung di kotak ketik. Blok rata tidak bisa berisi blok `:::` lain. Ganti dari Dokumen ke Biasa/Kode melepas gaya itu (ditanya dulu; Ctrl+Z mengembalikan). Tombol Tempel di menu butuh izin papan klip browser; kalau ditolak, pakai Ctrl+V.
- **Keamanan kode.** Kode tidak pernah dijalankan: tampil sebagai teks (highlight.js hanya mewarnai teks yang sudah di-escape), unduhan selalu `.txt`, `/raw` `text/plain` + `nosniff`. Isi catatan rahasia sampai di server sudah terenkripsi; catatan publik disimpan sebagai data dengan query berparameter.
- **Kasus.** Bagikan potongan konfigurasi ke rekan tanpa takut terjalankan; catatan rapat polos dari HP; dokumen dengan bagian penting distabilo.
- **Uji.** `scripts/e2e-types.js` (49 cek).

### 28. Tampilan Teks ⇄ Papan
- **Fungsi.** Catatan dengan `## Kolom` + daftar tampil sebagai papan Kanban; kartu bisa digeser.
- **Batasan.** Form, Slide, dan Diff ada di tempat lain (belum satu pill).

### 29. Tempel pintar
- **Fungsi.** Tabel dari Excel/Sheets jadi tabel Markdown; gambar jadi WebP.
- **Batasan.** Kode tidak otomatis jadi blok kode.

### 30. Template `{{variabel}}`
- **Fungsi.** Pembaca mengisi nilai; teks ikut berubah (surat, undangan).
- **Cara pakai.** Tulis `Halo {{nama}}`.
- **Kasus.** Pembaca mengira isiannya tersimpan.
  - Isian hanya di browser pembaca.
  - Untuk mengumpulkan jawaban, pakai `:::form`.

### 31. Kalkulator
- **Fungsi.** Blok ```` ```calc ```` menghitung per baris, bisa memakai variabel.
- **Batasan.** Tidak di tengah kalimat. Format angka mengikuti bahasa pembaca (5.000 vs 5,000).

### 32. Spoiler dan blok PIN
- **Fungsi.** Teks disembunyikan sampai diklik, atau dikunci PIN.
- **Cara pakai.** `/` → Spoiler / **Blok terkunci PIN**.
- **Batasan.** PIN pendek (4–6 angka) bisa ditebak oleh siapa pun yang **sudah memegang link catatan**, karena tebakan dilakukan di komputernya sendiri tanpa rate limit.
- **Kasus.** Memakai PIN 4 angka untuk rahasia penting.
  - Blok PIN untuk menyembunyikan ringan (misalnya jawaban kuis).
  - Untuk rahasia, pakai **sandi catatan** yang panjang.
  - 💡 Tulis peringatan ini di FAQ.

### 33. Polling
- **Fungsi.** Pembaca memilih satu opsi dan bisa mengganti pilihan.
- **Cara pakai.** `/` → **Polling**.
- **Batasan.**
  - 50 polling per catatan, 20 opsi.
  - Anonim: server tidak tahu siapa pemilihnya.

| Kasus | Mitigasi |
| --- | --- |
| Satu orang memilih berkali-kali (refresh, incognito). Dulu juga bisa **mengurangi suara opsi lain**. | ✅ **Baru:** server mencatat pilihan per pemilih (ID acak di browser). Ganti pilihan memindahkan satu suara, dan suara orang lain tidak bisa dikurangi.<br>✅ Maksimal **20 pemilih baru per jaringan per polling**. |
| Satu kantor > 20 orang memilih dari satu IP. | Pemilih ke-21 ditolak dengan pesan jelas. Untuk acara besar, buat beberapa polling atau naikkan `votersPerNet` di kode. |
| Bot dengan banyak IP. | Polling anonim tidak bisa sepenuhnya dicegah.<br>💡 Polling khusus akun. |

### 34. Diagram Mermaid
- **Fungsi.** Diagram dari teks.
- **Cara pakai.** `/` → **Diagram**.
- **Batasan.** Dijalankan di iframe sandbox (aman). Kanvas sketsa belum ada.

### 35. Penanda waktu media
- **Fungsi.** `[1:23]` melompat ke detik tertentu di lampiran audio/video.
- **Cara pakai.** `Alt+T` atau `/`.
- **Batasan.** Bukan YouTube (iframe pihak ketiga melanggar CSP dan membocorkan pembaca ke Google). Lampiran hilang setelah 24 jam.

### 36. Sematkan catatan lain (`:::embed`)
- **Fungsi.** Menampilkan catatan lain; disegarkan tiap 10 detik.
- **Batasan.** Satu arah, seluruh catatan.
- **Kasus.** Menyematkan catatan **privat** ke catatan **publik**, sehingga kuncinya terbuka untuk publik.
  - ✅ **Baru:** peringatan (lihat 10).

### 37. Utilitas teks
- **Fungsi.** Urutkan, hapus duplikat, rapikan, besar/kecil, ekstrak link, format JSON, nomori, hitung.
- **Cara pakai.** `Ctrl+K`.

### 38. Cari-ganti regex ❌
- Belum dibuat (F1).

### 39. Salin per blok
- **Fungsi.** Tombol salin muncul di paragraf, list, tabel, kutipan, dan kode.

### 40. Mode zen + pomodoro
- **Fungsi.** Layar bersih, timer, suara brown noise.
- **Cara pakai.** Menu ⋯ → **Mode zen**.
- **Batasan.** Satu jenis suara.

---

## 5. Developer, automasi (fitur 41–53)

### 41. `/raw/<link>`
- **Fungsi.** Teks mentah catatan **publik**.
- **Kasus.** Dipakai sebagai hosting gratis untuk skrip berbahaya (`curl … | sh`).
  - ✅ Hanya catatan publik.
  - ✅ 1 MB.
  - ✅ Takedown.
  - ✅ Dikirim sebagai `text/plain` (tidak dijalankan browser).
  - 💡 Rate limit GET.

### 42. SSH
- **Fungsi.** `ssh -p 2222 note.example.com < file.md` membuat catatan publik.
- **Batasan.**
  - Aktif hanya jika `NOTEHOP_SSH_ADDR` diisi.
  - Catatan **publik, anonim, kedaluwarsa 30 hari**.
  - 20/menit per IP.
- **Kasus.** Spam otomatis lewat SSH.
  - ✅ Rate limit.
  - ✅ 30 hari.
  - ✅ Takedown.
  - Jika tidak dipakai, biarkan mati.

### 43. Mock API
- **Fungsi.** Catatan publik berisi JSON bisa dipanggil sebagai API palsu untuk tes frontend (`/mock/<link>/path`).
- **Batasan.**
  - JSON saja (YAML belum).
  - CORS terbuka.
  - **GET tidak kena rate limit.**
- **Kasus.** Dipakai sebagai CDN gratis bertrafik tinggi, sehingga bandwidth VPS habis.
  - ✅ 1 MB per catatan.
  - 💡 Rate limit GET per IP.
  - 💡 Batas di nginx/Caddy.

### 44. Kotak masuk (webhook-to-note)
- **Fungsi.** Layanan lain bisa `POST /in/<token>`; pesan dienkripsi untuk catatan (kunci publik P-256) dan muncul di catatan.
- **Cara pakai.** Pengaturan lanjutan → Untuk ahli & developer → **Kotak masuk**.
- **Batasan.** 64 KB per pesan, 60/menit per IP.
- **Kasus.** Alamat kotak masuk bocor, lalu spam masuk.
  - ✅ Matikan kotak masuk lalu nyalakan lagi: alamatnya baru, yang lama mati.

### 45. Bot Telegram + Pantau (Watch)
- **Fungsi.**
  - Bot Telegram meneruskan pesan ke kotak masuk catatan.
  - **Pantau** mengirim webhook (bertanda tangan HMAC) saat catatan berubah.
- **Batasan.**
  - Bot butuh `TELEGRAM_BOT_TOKEN`.
  - Pantau hanya mengirim metadata (bukan isi).
  - Tidak ada email/WA/Discord.

| Kasus | Mitigasi |
| --- | --- |
| Webhook diarahkan ke alamat internal server (`127.0.0.1`, `169.254…`) untuk menyerang jaringan dalam (SSRF). | ✅ Dial guard menolak alamat privat, kecuali `NOTEHOP_PRIVATE_HOOKS=true`. |
| Penerima webhook tidak memeriksa tanda tangan, sehingga bisa dipalsukan. | Penerima wajib cek HMAC (dijelaskan di FAQ). |

### 46. Email-to-note ❌
- Butuh server mail. Alternatif: layanan inbound email → kotak masuk (44).

### 47. Checklist berulang
- **Fungsi.** Centang direset harian/mingguan/bulanan.
- **Cara pakai.** Pengaturan lanjutan → Waktu → **Ulangi checklist**.
- **Batasan.** Isi lama tidak diarsipkan; centang lama hilang saat reset.
- **Kasus.** Butuh bukti checklist kemarin.
  - 💡 Snapshot otomatis sebelum reset (F4).

### 48. Pengecek link / unfurl ⛔
- Server harus membaca isi catatan (bertentangan dengan zero-knowledge).

### 49. Pad satu Wi-Fi
- Sama dengan 25 (`/nearby`).

### 50. Duplikat
- **Fungsi.** Membuat catatan baru dari isi catatan ini.
- **Cara pakai.** Menu ⋯ → **Duplikat**.

### 51. Nama link, QR, pratinjau
- **Fungsi.**
  - Nama link sendiri (lihat bagian 1).
  - QR di dialog Bagikan.
  - Pratinjau WA/Telegram: judul hanya untuk catatan publik.
- **Batasan.** Gambar pratinjau statis.
- **Kasus.** Judul catatan privat bocor di pratinjau.
  - ✅ Tidak mungkin: server tidak tahu judulnya.

### 52. Ekspor, baca nyaring, clipper
- **Fungsi.**
  - Ekspor MD/PDF.
  - Baca nyaring (TTS bawaan browser).
  - Bookmarklet "simpan halaman ke NoteHop".
- **Batasan.** Ekspor PNG belum ada. Clipper berupa bookmarklet, bukan ekstensi.

### 53. Akun dan dashboard
- **Fungsi.** Brankas link (terenkripsi dengan sandi akun, PBKDF2 600.000), grup otomatis, wiki-link + graf, arsip setelah 90 hari tidak aktif, profil `/@user`, ekspor/impor, klaim catatan tamu, single sign-on (opsional).
- **Batasan.**
  - Lupa sandi tanpa kode pemulihan = brankas tidak bisa dibuka (zero-knowledge).
  - Backup otomatis ke Drive/Gist belum ada.

| Kasus | Mitigasi |
| --- | --- |
| Lupa sandi akun. | ✅ Kode pemulihan di `/security` (8 kode sekali pakai; akun SSO: memulihkan kunci brankas) dan halaman `/recover`.<br>✅ Ekspor JSON berkala dari dashboard. |
| Username menyamar. | ✅ Aturan nama (bagian 1). |
| Profil publik menampilkan catatan yang tidak sengaja dipublikasikan. | Hanya catatan yang pemiliknya pilih tampil di profil. |

### 53b. Kode pemulihan dan ganti password ✅
- **Fungsi.** Di `/security`: buat 8 kode pemulihan (diminta password/kunci brankas sekarang), unduh `.txt`, cetak, atau salin; kode hanya tampil sekali. Ganti password (atau kunci brankas akun SSO): perangkat lain otomatis keluar. Lupa password: `/recover` (tautan "Lupa password?" di halaman masuk), username + satu kode → password baru; catatan lama tetap terbuka.
- **Batasan.** Tanpa kode, password tetap tidak bisa direset. Akun SSO hanya memulihkan kunci brankas (password masuknya di server akun) dan harus masuk dulu. Membuat kode baru membatalkan semua kode lama; ganti password tidak membatalkan kode.
- **Mitigasi.** Kode 80 bit, server hanya menyimpan SHA-256 bukti; tebakan dibatasi per IP (20/menit) dan per akun (5/menit); kode dan sesi dipakai dalam satu transaksi; tiket langkah 2 sekali pakai, 10 menit; mengganti kode butuh secret sekarang (cookie curian tidak cukup).
- **Uji.** `internal/app/recovery_test.go`, `scripts/e2e-recovery.js`.

### 54a. Bantuan menulis (AI) ✅
- **Fungsi.** Tombol **AI** di editor (dan "Tulis ulang/Ringkas dengan AI" di menu klik kanan): ringkas, tulis ulang (santai/jelas/formal/lebih pendek), perbaiki ejaan, terjemahkan, lanjutkan tulisan, jadikan poin; untuk teks yang dipilih atau seluruh catatan. Hasil tampil di panel; baru masuk catatan lewat **Ganti teks** atau **Sisipkan di bawah** (Ctrl+Z membatalkan).
- **Mesin.** (1) AI bawaan browser (Chrome: Summarizer, Rewriter, Writer, Proofreader, Translator, LanguageModel), tidak ada yang keluar dari perangkat. (2) Kunci API sendiri: Anthropic (bawaan `claude-opus-5-5`, fallback server-side `default`), OpenAI, Google Gemini, OpenRouter, Ollama lokal, dan "Lainnya" bila operator mengisi `NOTEHOP_AI_ORIGINS`. Kunci disimpan di browser (disegel kunci brankas saat masuk) atau hanya selama tab terbuka.
- **Batasan.** ⚠️ E2E: dengan kunci API, penyedia itu membaca teks yang dikirim (peringatan selalu tampil). AI bawaan browser baru ada di Chrome desktop versi baru dan belum mendukung semua bahasa. Tidak ada AI untuk catatan Kode. Operator: `NOTEHOP_AI=device|off`.
- **Uji.** `internal/app/ai_test.go`, `scripts/e2e-ai.js` (penyedia dan AI bawaan disimulasikan).

### 54. AI, suara, domain sendiri, berbayar ⛔
- Ditunda: isi harus dikirim ke pihak ketiga, dan pembayaran di luar inti produk.

---

## 6. Halaman dan pengaturan situs (baru)

| Halaman | Isi | Yang harus dilakukan pemilik |
| --- | --- | --- |
| `/faq` | 33 tanya jawab, termasuk aturan nama link | |
| `/help` | Dialihkan ke `/faq` | |
| `/contact` | Kontak + cara melapor penyalahgunaan | **Isi `NOTEHOP_CONTACT`** (email atau URL). Tanpa itu, halaman menulis "pemilik situs belum mengisi alamat kontak". |
| `/terms`, `/privacy` | Draf syarat dan privasi (6 bagian, 9 bahasa) | **Baca dan sesuaikan.** Ini draf umum, bukan nasihat hukum. |

**Env baru:**

| Env | Contoh | Fungsi |
| --- | --- | --- |
| `NOTEHOP_ADMINS` | `navi,admin` | Username admin situs: boleh nama link pendek/cadangan dan username cadangan |
| `NOTEHOP_RESERVED` | `promo,event2026` | Tambahan nama cadangan |
| `NOTEHOP_CONTACT` | `abuse@example.com` | Kontak di `/contact` |

**Perintah baru:**

```bash
notehop takedown -db /var/lib/notehop/notehop.db https://note.example.com/n/xxxx   # hapus catatan + kunci nama 365 hari
notehop takedown -db /var/lib/notehop/notehop.db -file FILE_ID                  # hapus satu lampiran
```

---

## 7. Ringkasan mitigasi

### 7.1 Sudah dibuat di patch ini

1. Nama link ditahan 90 hari setelah catatan dihapus/kedaluwarsa (365 hari jika takedown).
2. Nama link sendiri minimal 7 karakter; nama cadangan + awalan; daftar kata kasar; username ikut aturan.
3. Halaman `/contact`, `/terms`, `/privacy`, `/help`.
4. Polling: satu pemilih satu suara (bisa ganti); suara orang lain tidak bisa dikurangi; maksimal 20 pemilih baru per jaringan.
5. `/nearby`: kode pasangan 6 angka + rate limit.
6. `notehop takedown` untuk moderasi.
7. Peringatan saat catatan publik berisi kunci catatan/lampiran lain.

### 7.2 Usulan (belum dibuat, pilih dulu)

| Prioritas | Usulan | Alasan |
| --- | --- | --- |
| Tinggi | Rate limit GET untuk `/mock` dan `/raw` | Mencegah bandwidth VPS habis |
| Tinggi | Kuota catatan/lampiran per IP per hari | Mencegah disk penuh oleh satu orang |
| Sedang | Kode pemulihan akun | Lupa sandi = brankas hilang |
| Sedang | Peringatan PIN pendek di FAQ/UI | PIN 4 angka mudah ditebak offline |
| Sedang | Snapshot sebelum checklist direset | Bukti checklist lama |
| Rendah | Tanda tangan/polling khusus akun | Identitas terverifikasi |
| Rendah | Server TURN untuk `/nearby` antar jaringan | Biaya, dan TURN meneruskan lalu lintas |
