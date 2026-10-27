# Progres implementasi roadmap

Permintaan pemilik: selesaikan **semua** fase (P1, P2 termasuk real-time), lalu buat zip untuk dipasang di VPS (install, testing, audit, update repo). Satu commit per batch; setiap batch: `go test -race ./...`, `node scripts/e2e.js`, katalog 8 bahasa lengkap.

Status: ✅ selesai · 🔨 dikerjakan · ⏳ antre · ⛔ sengaja tidak dibuat (alasan ditulis)

## Sudah selesai sebelumnya
✅ single sign-on + frasa brankas · ✅ migrasi DB · ✅ deploy VPS · ✅ uji Fase 1 · ✅ sticky keyboard bar · ✅ salin per blok · ✅ OG/bot · ✅ domain share · ✅ config/hooks/worker · ✅ kunci otomatis · ✅ kapsul waktu · ✅ pill toggle + anti-bentrok · ✅ i18n 8 bahasa

## Batch A: fitur di browser
✅ Template `{{variabel}}` · ✅ Fork/duplikat · ✅ Export MD + PDF (print) · ✅ Mode slide + teleprompter · ✅ Baca nyaring (TTS) · ✅ Zen + pomodoro + suara latar · ✅ Command palette `Ctrl+K` + bubble toolbar · ✅ Utilitas teks · ✅ Kalkulator ```calc · ✅ Mermaid (iframe sandbox) · ✅ Auto-grouping dashboard · ✅ Wiki-links `[[Judul]]` + graf · ✅ Web clipper (bookmarklet) · ✅ Auto-redaction salinan · uji: `scripts/e2e-features.js`

## Batch B: fitur server (hook)
✅ Partial Burn · ✅ Riwayat versi + diff · ✅ Diff checker · ✅ Saran edit (mode suggest) · ✅ Sign-off · ✅ Mini-poll · ✅ Read receipts · ✅ Claimable notes · ✅ Decoy password · ✅ IP allowlist · ✅ Cron reset checklist · ✅ Dead Man's Switch · ✅ Watermark zero-width · ✅ Auto-archive · ✅ Shadow margin · uji: `scripts/e2e-server-features.js`, `internal/app/extra_test.go`

## Batch C: lampiran
✅ Lampiran terenkripsi 24 jam · ✅ Smart paste gambar → WebP terenkripsi · ✅ Catatan media bertimestamp · uji: `scripts/e2e-files.js`, `internal/app/files_test.go`

## Batch D: real-time
✅ Kolaborasi E2E (Yjs + WebSocket) · ✅ Kursor + nama tamu · ✅ Mini-chat · ✅ Follow presenter · ✅ Embed blok live · ✅ Kanban · ✅ Form-to-Note · ✅ Same-WiFi pad · ✅ P2P file drop (WebRTC) · uji: `scripts/e2e-live.js`, `scripts/e2e-blocks.js`, `scripts/e2e-nearby.js`, `internal/app/live_test.go`

## Batch E: integrasi
✅ CLI `notehop` (enkripsi lokal) · ✅ Watch Note (webhook) · ✅ Webhook-to-Note · ✅ Profil publik `/@user` · ✅ Mock API JSON · ✅ SSH (catatan publik) · ✅ Bot Telegram ⚠️ · ✅ PWA offline · ✅ Backup/BYO storage · uji: `scripts/e2e-integrations.js`, `internal/client/client_test.go`, `internal/app/{watch,public,sshd,telegram}_test.go`

## Batch F: tampilan ramah (permintaan pemilik setelah audit)
✅ Bahasa awam di semua UI (istilah teknis pindah ke `/faq`) · ✅ Halaman Buat: kartu "Mau bikin apa?" (6 template), "Siapa pun yang punya link bisa…", "Seberapa rahasia?", "Simpan selama", kalimat ringkasan · ✅ Pengaturan lanjutan dikelompokkan (Waktu, Privasi ekstra, Kerja bareng, Perangkat ini, Untuk ahli & developer) dengan contoh pemakaian · ✅ Tur 3 langkah untuk pengunjung baru · ✅ Mode "Tampilkan nama teknis" · ✅ Halaman catatan: kalimat status, menu ⋯ berisi Riwayat/Salinan tetap/Hapus · ✅ Dialog Bagikan: link pemilik dengan peringatan, pengaturan berkelompok · ✅ Halaman FAQ (32 tanya jawab, bisa dicari) · ✅ Bahasa ke-9: Français · ✅ Deteksi bahasa otomatis dari browser

## Batch G: aturan nama + mitigasi (permintaan pemilik)
✅ Nama link sendiri minimal 7 karakter (admin situs bebas) · ✅ Nama cadangan + awalan (`names/reserved.txt`, `NOTEHOP_RESERVED`) · ✅ Daftar kata kasar 9 bahasa · ✅ Masa tunggu nama 90 hari setelah catatan dihapus/kedaluwarsa (migrasi 6) · ✅ `notehop takedown` (nama dikunci 365 hari) · ✅ Halaman `/contact`, `/terms`, `/privacy`, `/help` · ✅ Polling: buku pemilih, satu suara per pemilih, 20 pemilih baru per jaringan (migrasi 7) · ✅ `/nearby` dengan kode pasangan 6 angka · ✅ Peringatan kunci di catatan publik · panduan lengkap: `docs/PANDUAN-FITUR.md` · uji: `internal/app/names_test.go`, `extra_test.go`, `scripts/e2e-nearby.js`

## Batch H: tipe catatan + menu klik kanan (permintaan pemilik)
✅ Tipe Biasa / Dokumen / Kode (disimpan di isi terenkripsi, ikut ke duplikat, embed, ekspor/impor dashboard) · ✅ Kode: bahasa, nomor baris, Tab indentasi, tampil sebagai teks, unduhan `.txt` · ✅ Menu klik kanan + lembar format di HP: warna, stabilo, ukuran, rata, huruf besar/kecil, tautan, hapus format · ✅ 51 teks baru di 9 bahasa · uji: `scripts/e2e-types.js`

## Batch I: pemulihan akun + bantuan menulis (permintaan pemilik)
✅ Kode pemulihan (8, sekali pakai) + halaman `/security` dan `/recover`, ganti password/kunci brankas, kunci brankas dibungkus (migrasi 8) · ✅ Bantuan menulis: AI bawaan browser atau kunci API sendiri (Anthropic, OpenAI, Gemini, OpenRouter, Ollama), CSP dibuka hanya di halaman editor, `NOTEHOP_AI` · uji: `internal/app/{recovery,ai}_test.go`, `scripts/e2e-recovery.js`, `scripts/e2e-ai.js`

## Tahap akhir
✅ Zip handoff (berisi sumber dari commit, binary linux amd64/arm64 + SHA256SUMS, HANDOFF.txt)

## Sengaja tidak dibuat
- ⛔ Paywalled note (D di tabel prioritas: pembayaran & kepatuhan, bukan inti)
- ⛔ Steganografi gambar (D: aplikasi chat mengompres ulang gambar)
- ⛔ AI magic action / voice-to-note (C: isi dikirim ke layanan AI; butuh persetujuan eksplisit & API key, ditunda ke Pro)
- ⛔ Micro-blog + custom domain (C+: di luar inti catatan)
- ⛔ Live link checker & smart unfurling (C: server melihat semua URL di catatan)
- ⛔ Geo-fencing (butuh database GeoIP); diganti IP allowlist
- ⛔ Email-to-Note (butuh infrastruktur mail); bisa lewat Webhook-to-Note dari layanan inbound email
