# Progres implementasi roadmap

Permintaan pemilik: selesaikan **semua** fase (P1, P2 termasuk real-time), lalu buat zip + arahan untuk sesi Claude di VPS (install, testing, audit, update repo). Satu commit per batch; setiap batch: `go test -race ./...`, `node scripts/e2e.js`, katalog 8 bahasa lengkap.

Status: ✅ selesai · 🔨 dikerjakan · ⏳ antre · ⛔ sengaja tidak dibuat (alasan ditulis)

## Sudah selesai sebelumnya
✅ SSO SSO + frasa brankas · ✅ migrasi DB · ✅ deploy VPS · ✅ uji Fase 1 · ✅ sticky keyboard bar · ✅ salin per blok · ✅ OG/bot · ✅ domain share · ✅ config/hooks/worker · ✅ kunci otomatis · ✅ kapsul waktu · ✅ pill toggle + anti-bentrok · ✅ i18n 8 bahasa

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

## Tahap akhir
✅ Zip + arahan sesi Claude VPS (`docs/DEPLOY.md`; zip berisi sumber dari commit, binary linux amd64/arm64 + SHA256SUMS, HANDOFF.txt)

## Sengaja tidak dibuat
- ⛔ Paywalled note (D di tabel prioritas: pembayaran & kepatuhan, bukan inti)
- ⛔ Steganografi gambar (D: aplikasi chat mengompres ulang gambar)
- ⛔ AI magic action / voice-to-note (C: isi dikirim ke layanan AI; butuh persetujuan eksplisit & API key, ditunda ke Pro)
- ⛔ Micro-blog + custom domain (C+: tumpang tindih dengan SSO)
- ⛔ Live link checker & smart unfurling (C: server melihat semua URL di catatan)
- ⛔ Geo-fencing (butuh database GeoIP); diganti IP allowlist
- ⛔ Email-to-Note (butuh infrastruktur mail); bisa lewat Webhook-to-Note dari layanan inbound email
