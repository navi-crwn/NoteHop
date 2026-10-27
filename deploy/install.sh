#!/bin/sh
# Install or update NoteHop on a Linux VPS with systemd. Run as root from
# the repository folder:
#
#   sudo DOMAIN=note.example.com ./deploy/install.sh            # build here (needs Go)
#   sudo DOMAIN=note.example.com ./deploy/install.sh ./notehop  # use a prebuilt binary
#
# Running it again updates the binary and units; it backs up the database
# before restarting and never overwrites /etc/notehop/notehop.env.
set -eu

[ "$(id -u)" = 0 ] || { echo "Jalankan sebagai root (sudo)." >&2; exit 1; }
command -v systemctl >/dev/null || { echo "Butuh systemd." >&2; exit 1; }
cd "$(dirname "$0")/.."

BIN_SRC="${1:-}"
if [ -z "$BIN_SRC" ]; then
	command -v go >/dev/null || { echo "Go tidak ada. Pasang Go, atau berikan binary: install.sh ./notehop" >&2; exit 1; }
	VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
	echo "Membangun notehop $VERSION…"
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o bin/notehop ./cmd/notehop
	BIN_SRC=bin/notehop
fi

id notehop >/dev/null 2>&1 || useradd --system --home-dir /var/lib/notehop --shell /usr/sbin/nologin notehop
install -d -m 0750 -o notehop -g notehop /var/lib/notehop
install -d -m 0755 /usr/local/lib/notehop
install -d -m 0750 -g notehop /etc/notehop

if [ ! -f /etc/notehop/notehop.env ]; then
	sed "s/note\.example\.com/${DOMAIN:-note.example.com}/" deploy/notehop.env.example > /etc/notehop/notehop.env
	chown root:notehop /etc/notehop/notehop.env
	chmod 0640 /etc/notehop/notehop.env
	echo "Dibuat: /etc/notehop/notehop.env (periksa NOTEHOP_BASE_URL)."
fi

UPDATE=0
[ -x /usr/local/bin/notehop ] && UPDATE=1
if [ "$UPDATE" = 1 ] && [ -f /var/lib/notehop/notehop.db ]; then
	# The new binary may migrate the schema; keep a copy from before.
	OUT="/var/lib/notehop/backups/pre-update-$(date -u +%Y%m%d-%H%M%S).db"
	install -d -m 0700 -o notehop -g notehop /var/lib/notehop/backups
	runuser -u notehop -- /usr/local/bin/notehop backup -db /var/lib/notehop/notehop.db "$OUT" && echo "Backup sebelum update: $OUT"
fi

install -m 0755 "$BIN_SRC" /usr/local/bin/notehop
install -m 0755 deploy/backup.sh /usr/local/lib/notehop/backup.sh
install -m 0644 deploy/notehop.service deploy/notehop-backup.service deploy/notehop-backup.timer /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now notehop-backup.timer >/dev/null
if [ "$UPDATE" = 1 ]; then systemctl restart notehop; else systemctl enable --now notehop; fi

ADDR="$(sed -n 's/^NOTEHOP_ADDR=//p' /etc/notehop/notehop.env)"
for i in 1 2 3 4 5 6 7 8 9 10; do
	if curl -fsS "http://${ADDR:-127.0.0.1:8090}/healthz" >/dev/null 2>&1; then
		echo "OK: $(/usr/local/bin/notehop -version) berjalan di ${ADDR:-127.0.0.1:8090}."
		exit 0
	fi
	sleep 1
done
echo "notehop belum menjawab. Lihat: journalctl -u notehop -n 50" >&2
exit 1
