# Hitung anggota lama NoteHop (baca saja), untuk owner

Tujuan: sebelum login lokal NoteHop benar-benar dilipat (`/login` kini mengutamakan HOP ID), kita perlu tahu berapa
akun NoteHop lama yang masuk dengan sandi lokal dan belum punya HOP ID. Akun itu **tetap bisa masuk** lewat tautan
kecil "akun NoteHop lama"; angka ini hanya menentukan perlu tidaknya jalan pindah nanti.

- **Hanya membaca.** Basis data dibuka dalam mode baca-saja; perintah tulis ditolak oleh SQLite sendiri.
- **Tidak mencetak e-mail atau nama**, hanya angka. Hasil juga tersimpan di `~/hitung-notehop.txt` di VPS.
- NoteHop tidak mencatat "masuk terakhir". Pengganti yang jujur: akun dianggap **aktif** bila dalam 90 hari terakhir
  ia dibuat, atau salah satu catatannya diubah, atau sesinya masih berlaku (sesi hidup 30 hari).
- Dicoba pada basis data contoh di PC (akun SSO "Helena" dan akun lokal tiruan): angka cocok hitungan tangan, sha256
  berkas basis data sama sebelum dan sesudah, cabang "berkas tidak ada" hanya mencetak pesan. **Belum pernah dijalankan di VPS.**

## Langkah 1. VPS (tmux), bash: tempel seluruh blok ini

```bash
NH_DB="$HOME/apps/notehop/data/notehop.db"
if [ ! -f "$NH_DB" ]; then
  echo "Berkas tidak ada: $NH_DB (ubah NH_DB di baris pertama bila folder data NoteHop ada di tempat lain)"
else
python3 - "$NH_DB" <<'PY' 2>&1 | tee ~/hitung-notehop.txt
import sqlite3, sys

con = sqlite3.connect("file:" + sys.argv[1] + "?mode=ro", uri=True)
con.execute("PRAGMA query_only = ON")
D90 = "CAST(strftime('%s','now','-90 days') AS INTEGER)"
LOKAL = "(sso_id IS NULL OR sso_id = '')"

BAGIAN = [
    ("A. semua akun: tertaut HOP ID lawan sandi lokal saja", f"""
SELECT COUNT(*) AS total,
       SUM(NOT {LOKAL}) AS dengan_hop_id,
       SUM({LOKAL})     AS sandi_lokal_tanpa_hop_id
FROM users"""),
    ("B. sandi lokal tanpa HOP ID: menurut status dan peran", f"""
SELECT SUM(status = 'active') AS aktif, SUM(status <> 'active') AS tak_aktif, SUM(role = 'admin') AS admin
FROM users WHERE {LOKAL}"""),
    ("C. sandi lokal tanpa HOP ID: yang punya catatan", f"""
SELECT COUNT(*) AS akun_punya_catatan, COALESCE(SUM(basic),0) AS catatan_basic, COALESCE(SUM(enc),0) AS catatan_terenkripsi
FROM (SELECT u.id, SUM(n.encrypted = 0) AS basic, SUM(n.encrypted = 1) AS enc
      FROM users u JOIN notes n ON n.owner_id = u.id WHERE ({LOKAL.replace('sso_id','u.sso_id')}) AND n.frozen = 0 GROUP BY u.id)"""),
    ("D. sandi lokal tanpa HOP ID: aktif dalam 90 hari (dibuat, catatan diubah, atau sesi berlaku)", f"""
SELECT COUNT(*) AS aktif_90_hari,
       SUM(EXISTS (SELECT 1 FROM notes n WHERE n.owner_id = u.id)) AS dan_punya_catatan
FROM users u
WHERE {LOKAL.replace('sso_id','u.sso_id')}
  AND (u.created >= {D90}
       OR EXISTS (SELECT 1 FROM notes n WHERE n.owner_id = u.id AND n.updated >= {D90})
       OR EXISTS (SELECT 1 FROM sessions s WHERE s.user_id = u.id AND s.expires > CAST(strftime('%s','now') AS INTEGER)))"""),
    ("E. pembanding: sudah tertaut HOP ID", f"""
SELECT COUNT(*) AS tertaut, SUM(EXISTS (SELECT 1 FROM notes n WHERE n.owner_id = u.id)) AS punya_catatan
FROM users u WHERE NOT {LOKAL.replace('sso_id','u.sso_id')}"""),
]
for judul, sql in BAGIAN:
    cur = con.execute(sql)
    cols = [d[0] for d in cur.description]
    rows = cur.fetchall()
    print("\n" + judul)
    w = [max(len(str(c)), *(len(str(r[i])) for r in rows)) for i, c in enumerate(cols)]
    print("  " + "  ".join(str(c).ljust(w[i]) for i, c in enumerate(cols)))
    print("  " + "  ".join("-" * w[i] for i in range(len(cols))))
    for r in rows:
        print("  " + "  ".join(("0" if v is None else str(v)).rjust(w[i]) for i, v in enumerate(r)))
con.close()
PY
fi
```

Yang diharapkan: lima tabel kecil berjudul A sampai E, masing-masing satu baris angka.

## Langkah 2. Tempel seluruh keluaran ke chat koordinator

Tidak ada data pribadi di dalamnya.

## Bila ada pesan lain

| Pesan | Arti | Yang dilakukan |
|---|---|---|
| `Berkas tidak ada: …` | folder data NoteHop bukan di `~/apps/notehop/data` | beri tahu koordinator pesan itu; jangan mencari sendiri |
| `unable to open database file` | berkas milik pengguna lain | ganti baris pertama dengan `NH_DB="$(ls -1d ~/backups/hop/hop-*/notehop/notehop.db \| tail -1)"` (salinan malam, tetap baca saja) |
| `python3: command not found` | Python tidak ada di VPS | beri tahu koordinator |

## Cara membaca hasilnya

- **A `sandi_lokal_tanpa_hop_id`** = semua akun NoteHop lama yang masuk hanya dengan sandi lokal.
- **C** = berapa di antaranya punya catatan (catatan basic ada di DB, yang terenkripsi hanya bisa dibuka pemiliknya).
- **D** = yang paling penting: yang masih aktif. Merekalah yang akan terasa bila pintu lama dipersulit.
