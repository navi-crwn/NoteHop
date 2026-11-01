#!/usr/bin/env bash
# NoteHop: update. Run it in the folder where it is installed. --help lists the options.
APP='NoteHop'; SLUG='notehop'; REPO='NoteHop'; PORT_DEFAULT=8090
# ---------------------------------------------------------------------------
# Shared helpers (the same in every HOP installer). Plain bash 3.2+, no extras.
# ---------------------------------------------------------------------------
set -eu
set -o pipefail

if [ -t 1 ]; then
  B=$'\033[1m'; D=$'\033[2m'; R=$'\033[31m'; G=$'\033[32m'; Y=$'\033[33m'; C=$'\033[36m'; Z=$'\033[0m'
else
  B=''; D=''; R=''; G=''; Y=''; C=''; Z=''
fi
say()  { printf '%s\n' "$*"; }
hdr()  { printf '\n%s== %s ==%s\n' "$B" "$*" "$Z"; }
ok()   { printf ' %s✓%s %s\n' "$G" "$Z" "$*"; }
warn() { printf ' %s!%s %s\n' "$Y" "$Z" "$*" >&2; }
bad()  { printf ' %s✗%s %s\n' "$R" "$Z" "$*" >&2; }
die()  { bad "$*"; exit 1; }
have() { command -v "$1" >/dev/null 2>&1; }

YES=0; DRY=0; STAR=ask; INTERACTIVE=0
[ -t 0 ] && INTERACTIVE=1
# Run through "curl ... | bash": stdin is the script, so ask on the terminal instead.
claim_tty() {
  [ "$INTERACTIVE" = 1 ] && return 0
  case " $* " in *" --yes "*|*" -y "*) return 0 ;; esac
  if (: </dev/tty) 2>/dev/null; then exec </dev/tty; INTERACTIVE=1; fi
}

# ask VAR "Question" "default": skips the question when VAR is already set
# (by a flag or the environment), and takes the default with --yes.
ask() {
  local var="$1" q="$2" def="${3-}" cur reply
  eval "cur=\${$var-}"
  [ -n "$cur" ] && return 0
  if [ "$YES" = 1 ] || [ "$INTERACTIVE" = 0 ]; then eval "$var=\$def"; return 0; fi
  if [ -n "$def" ]; then printf ' %s?%s %s %s[%s]%s ' "$C" "$Z" "$q" "$D" "$def" "$Z"; else printf ' %s?%s %s ' "$C" "$Z" "$q"; fi
  IFS= read -r reply || reply=''
  eval "$var=\${reply:-\$def}"
}
# ask_secret VAR "Question": no echo; empty means "generate" or "skip" for the caller.
ask_secret() {
  local var="$1" q="$2" cur reply
  eval "cur=\${$var-}"
  [ -n "$cur" ] && return 0
  if [ "$YES" = 1 ] || [ "$INTERACTIVE" = 0 ]; then eval "$var=''"; return 0; fi
  printf ' %s?%s %s ' "$C" "$Z" "$q"
  IFS= read -rs reply || reply=''; printf '\n'
  eval "$var=\$reply"
}
# choose VAR "Question" default opt...: the answer is one of the options.
choose() {
  local var="$1" q="$2" def="$3" cur a ok_=0; shift 3
  eval "cur=\${$var-}"
  if [ -n "$cur" ]; then for a in "$@"; do [ "$a" = "$cur" ] && ok_=1; done; [ $ok_ = 1 ] && return 0; die "--$var: '$cur' is not one of: $*"; fi
  if [ "$YES" = 1 ] || [ "$INTERACTIVE" = 0 ]; then eval "$var=\$def"; return 0; fi
  while :; do
    ask _pick "$q ($*)" "$def"; a="$_pick"; _pick=''
    for ok_ in "$@"; do [ "$ok_" = "$a" ] && { eval "$var=\$a"; return 0; }; done
    warn "Please type one of: $*"
  done
}
# confirm "Question" y|n: true when the answer is yes.
confirm() {
  local q="$1" def="${2:-y}" reply hint='[Y/n]'
  [ "$def" = n ] && hint='[y/N]'
  if [ "$YES" = 1 ] || [ "$INTERACTIVE" = 0 ]; then [ "$def" = y ]; return; fi
  printf ' %s?%s %s %s%s%s ' "$C" "$Z" "$q" "$D" "$hint" "$Z"
  IFS= read -r reply || reply=''
  case "${reply:-$def}" in y|Y|yes|YES|Yes) return 0 ;; *) return 1 ;; esac
}
# proceed "Question": like confirm (default no), but --yes means yes. For steps the person asked for by name.
proceed() { [ "$YES" = 1 ] && return 0; confirm "$1" n; }
rand_hex() { if have openssl; then openssl rand -hex "$1"; else LC_ALL=C tr -dc 'a-f0-9' </dev/urandom | head -c $(( $1 * 2 )); fi; }

# set_env FILE KEY VALUE: replace "KEY=" or "#KEY=" in place, or append.
set_env() {
  local f="$1" k="$2" v="$3" tmp
  case "$v" in *[[:space:]\#\"\$\\]*|'') v="\"$(printf '%s' "$v" | sed 's/[\\"$]/\\&/g')\"" ;; esac
  tmp="$(mktemp)"
  if grep -qE "^#?[[:space:]]*${k}=" "$f" 2>/dev/null; then
    awk -v k="$k" -v v="$v" 'BEGIN{d=0} { if (!d && $0 ~ "^#?[ \t]*" k "=") { print k "=" v; d=1 } else print }' "$f" >"$tmp"
  else
    cat "$f" >"$tmp" 2>/dev/null || true; printf '%s=%s\n' "$k" "$v" >>"$tmp"
  fi
  cat "$tmp" >"$f"; rm -f "$tmp"
}
get_env() { sed -n "s/^$2=//p" "$1" 2>/dev/null | head -1 | sed 's/^"//; s/"$//'; }

port_free() {
  if have lsof; then ! lsof -nP -iTCP:"$1" -sTCP:LISTEN >/dev/null 2>&1
  elif have ss; then ! ss -ltn "sport = :$1" 2>/dev/null | grep -q LISTEN
  else ! (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null; fi
}
free_port() { local p="$1"; while ! port_free "$p"; do p=$((p + 1)); done; echo "$p"; }
disk_free_mb() { df -Pm "${1:-.}" 2>/dev/null | awk 'NR==2{print $4}'; }
version_ge() { [ "$(printf '%s\n%s\n' "$2" "$1" | sort -V | head -1)" = "$2" ]; }  # version_ge HAVE NEED
open_url() { if have open; then open "$1" >/dev/null 2>&1 || true; elif have xdg-open; then xdg-open "$1" >/dev/null 2>&1 || true; else say "   $1"; fi; }

# After a good install: one question, never silent, never repeated, nothing sent anywhere.
star_prompt() {
  local url="https://github.com/navi-crwn/$REPO"
  [ "$STAR" = no ] && { say "   ${D}If you like $APP: $url${Z}"; return 0; }
  if [ "$STAR" = yes ] || confirm "Open the GitHub page of $APP so you can star it?" y; then
    if have gh && gh auth status >/dev/null 2>&1 && confirm "Star it right now with your GitHub login (gh)?" n; then
      gh api -X PUT "user/starred/navi-crwn/$REPO" >/dev/null 2>&1 && ok "Starred. Thank you!" || { warn "Could not star it; opening the page instead."; open_url "$url"; }
    else
      open_url "$url"
    fi
  fi
  say "   ${D}GitHub: $url${Z}"
}

# Run a step, show it, stop with a clear message when it fails.
step() { local msg="$1"; shift; printf ' %s…%s %s\n' "$D" "$Z" "$msg"; if [ "$DRY" = 1 ]; then say "   (dry run) $*"; return 0; fi; "$@" || die "Step failed: $msg"; }

common_usage() {
cat <<USAGE
Options:
  -y, --yes             No questions: use the defaults and any flags below
      --dir DIR         Install into DIR (default: this folder, or ~/$SLUG when run from a download)
      --name NAME       Name of your site, shown in the pages
      --url URL         Public address (default http://localhost:PORT)
      --port N          Port for the built-in server (default $PORT_DEFAULT)
      --run MODE        $RUN_MODES
      --hub MODE        existing | new | standalone$HUB_NOTE
      --hub-url URL     Address of the HOP ID hub (with --hub existing)
      --hub-secret S    Shared app secret (or set HOP_SSO_SECRET in the environment)
      --star / --no-star
      --dry-run         Show the steps without running them
  -h, --help
USAGE
}
main() {
# ---------------------------------------------------------------------------
# Update NoteHop (Go): backup, new code, rebuild.
# ---------------------------------------------------------------------------
FORCE=0
while [ $# -gt 0 ]; do
  case "$1" in
    -y|--yes) YES=1 ;; --dry-run) DRY=1 ;; --force) FORCE=1 ;;
    -h|--help) say "Usage: ./update.sh [--yes] [--force] [--dry-run]"; exit 0 ;;
    *) bad "Unknown option: $1"; exit 2 ;;
  esac; shift
done
SELF_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
cd "$SELF_DIR"
[ -f go.mod ] && [ -f .env ] || die "Run this in the folder where NoteHop is installed (go.mod and .env not both found)."
have git && [ -d .git ] || die "This folder is not a git clone, so it cannot update itself. Download the new version and run ./install.sh again."
DOCKER=0; [ -f compose.local.yml ] && DOCKER=1
[ "$DOCKER" = 0 ] && ! have go && die "Go is not installed (needed to rebuild)."

hdr "Updating NoteHop"
BEFORE="$(git describe --tags --always 2>/dev/null || git rev-parse --short HEAD)"
if [ -n "$(git status --porcelain --untracked-files=no)" ] && [ "$FORCE" = 0 ]; then
  git status --short --untracked-files=no | head -10
  die "You changed files here (listed above). Commit or stash them, or run ./update.sh --force."
fi
confirm "Update now? (a copy of the database is saved first)" y || { say "Nothing was changed."; exit 0; }

DBF="$(get_env .env NOTEHOP_DB)"; [ -n "$DBF" ] || DBF="$(get_env .env NOTEHOP_DATA)/notehop.db"
case "$DBF" in /data/*) DBF="$(sed -n 's#.*"\(.*\):/data".*#\1#p' compose.local.yml 2>/dev/null | head -1)/${DBF#/data/}" ;; esac
if [ -f "$DBF" ] && [ "$DRY" = 0 ]; then
  mkdir -p backups; OUT="backups/notehop-$(date +%Y%m%d-%H%M%S).db"
  if have sqlite3; then sqlite3 "$DBF" ".backup '$OUT'" && ok "Database copied to $OUT"; else cp "$DBF" "$OUT" && ok "Database copied to $OUT"; warn "sqlite3 is not installed, so this was a plain file copy: best done while NoteHop is stopped."; fi
fi

git symbolic-ref -q HEAD >/dev/null || die "This checkout is not on a branch. Run:  git checkout main  and try again."
step "Fetching the new version" git pull --ff-only
VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
if [ "$DOCKER" = 1 ]; then
  step "Rebuilding and restarting (docker)" docker compose -f compose.local.yml up -d --build
else
  step "Rebuilding" env CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o bin/notehop ./cmd/notehop
fi
AFTER="$(git describe --tags --always 2>/dev/null || git rev-parse --short HEAD)"
hdr "Done"
ok "NoteHop updated: $BEFORE → $AFTER"
if [ "$DOCKER" = 0 ]; then
  if have systemctl && systemctl is-active --quiet notehop 2>/dev/null; then say "  It runs as a service: sudo systemctl restart notehop"; else say "  Stop it (Ctrl+C) and start it again with ./start.sh"; fi
fi

}
main "$@"; exit $?
