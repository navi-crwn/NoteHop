#!/bin/sh
# Copies the live database to $NOTEHOP_BACKUP_DIR and deletes copies older
# than $NOTEHOP_BACKUP_KEEP_DAYS. Run by notehop-backup.service; also fine by hand:
#   sudo -u notehop env $(cat /etc/notehop/notehop.env | xargs) /usr/local/lib/notehop/backup.sh
set -eu
DB="${NOTEHOP_DB:-/var/lib/notehop/notehop.db}"
DIR="${NOTEHOP_BACKUP_DIR:-/var/lib/notehop/backups}"
KEEP="${NOTEHOP_BACKUP_KEEP_DAYS:-14}"
BIN="${NOTEHOP_BIN:-/usr/local/bin/notehop}"

mkdir -p "$DIR"
OUT="$DIR/notehop-$(date -u +%Y%m%d-%H%M%S).db"
"$BIN" backup -db "$DB" "$OUT"
gzip -9f "$OUT"
find "$DIR" -name 'notehop-*.db.gz' -type f -mtime +"$KEEP" -delete
echo "backup: $OUT.gz"
