#!/usr/bin/env bash
# NoteHop: one-command installer. Run it from a clone, or straight from GitHub:
#   curl -fsSL https://raw.githubusercontent.com/navi-crwn/NoteHop/main/install.sh | bash
# Add --yes to take every default. --help lists the options.
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
# Install NoteHop (a single Go program + one SQLite file). Safe to run again: it
# never overwrites an existing .env without asking, and never deletes anything.
# ---------------------------------------------------------------------------
claim_tty "$@"
SELF="${BASH_SOURCE[0]:-$0}"
SELF_DIR="$(cd "$(dirname "$SELF")" 2>/dev/null && pwd || pwd)"
DIR=''; NAME=''; URL=''; PORT=''; RUN=''; HUB=''; HUB_URL=''; HUB_SECRET="${HOP_SSO_SECRET:-}"; HUB_DIR=''; DATA=''
RUN_MODES='foreground (start.sh, default) | systemd (prints the unit and the commands) | docker (compose file) | none'
HUB_NOTE='  (default standalone: NoteHop has its own accounts)'

usage() { say "Usage: ./install.sh [options]"; say ""; common_usage; say "      --data DIR          Where the database and files live (default <folder>/data)"; say "      --hub-dir DIR       Where to install the hub (with --hub new)"; }
while [ $# -gt 0 ]; do
  case "$1" in
    -y|--yes) YES=1 ;; --dry-run) DRY=1 ;; --star) STAR=yes ;; --no-star) STAR=no ;;
    --dir) DIR="$2"; shift ;; --name) NAME="$2"; shift ;; --url) URL="$2"; shift ;; --port) PORT="$2"; shift ;;
    --run) RUN="$2"; shift ;; --hub) HUB="$2"; shift ;; --hub-url) HUB_URL="$2"; shift ;; --hub-secret) HUB_SECRET="$2"; shift ;;
    --data) DATA="$2"; shift ;; --hub-dir) HUB_DIR="$2"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) bad "Unknown option: $1"; usage; exit 2 ;;
  esac; shift
done

say "${B}NoteHop${Z} installer"
say "${D}Free and open source. Nothing is sent anywhere; this script only runs on your machine.${Z}"
[ "$(id -u)" = 0 ] && warn "You are running as root. Not needed: the program and its files work as a normal user."

# --- 1. Checks ---------------------------------------------------------------
hdr "Checking this computer"
PROBLEMS=0
need() { bad "$1"; say "     ${D}$2${Z}"; PROBLEMS=$((PROBLEMS + 1)); }
GOWANT="$(sed -n 's/^go \([0-9.]*\).*/\1/p' "$SELF_DIR/go.mod" 2>/dev/null | head -1 || true)"; GOWANT="${GOWANT:-1.27}"
HAVE_GO=0; HAVE_DOCKER=0
if have go; then
  GOV="$(go env GOVERSION 2>/dev/null | sed 's/^go//')"
  if version_ge "${GOV:-0}" "$GOWANT"; then ok "Go $GOV"; HAVE_GO=1; else warn "Go $GOV is older than $GOWANT; Go can fetch the right version itself (GOTOOLCHAIN=auto, default) when it has internet."; HAVE_GO=1; fi
fi
if have docker && docker info >/dev/null 2>&1; then ok "Docker"; HAVE_DOCKER=1; fi
[ "$HAVE_GO" = 0 ] && [ "$HAVE_DOCKER" = 0 ] && need "Neither Go nor Docker is installed" "Install Go (https://go.dev/dl, macOS: brew install go) or Docker (https://docs.docker.com/get-docker)"
have git && ok "git" || warn "git is not installed (only needed to download or update NoteHop)"
have curl && ok "curl" || warn "curl is not installed (only used for the self-test at the end)"
FREE="$(disk_free_mb "$SELF_DIR")"; [ "${FREE:-0}" -ge 700 ] 2>/dev/null && ok "Disk: ${FREE} MB free" || warn "Less than 700 MB free (${FREE:-?} MB)"
[ "$PROBLEMS" -gt 0 ] && die "$PROBLEMS thing(s) to fix first (see above). Run this again when done; it is safe to repeat."

# --- 2. Questions ------------------------------------------------------------
hdr "A few questions (press Enter for the suggestion)"
if [ -f "$SELF_DIR/go.mod" ] && [ -z "$DIR" ]; then DIR="$SELF_DIR"; fi
ask DIR "Install into which folder?" "$HOME/notehop"
case "$DIR" in "~"*) DIR="$HOME${DIR#\~}" ;; esac
ask NAME "What do you want to call your site?" "NoteHop"
PORT_SUGGEST="$(free_port 8090)"
ask PORT "Port?" "$PORT_SUGGEST"
case "$PORT" in ''|*[!0-9]*) die "Port must be a number" ;; esac
port_free "$PORT" || warn "Port $PORT is in use right now; pick another with --port if the self-test fails."
if [ -z "$URL" ] && [ -f "$DIR/.env" ]; then URL="$(get_env "$DIR/.env" NOTEHOP_BASE_URL)"; [ -n "$URL" ] && say "  ${D}Keeping the address from the existing .env: $URL${Z}"; fi
ask URL "Public address of NoteHop (https://... if it will be on the internet)?" "http://localhost:$PORT"
URL="${URL%/}"
ask DATA "Where should the database and files live?" "$DIR/data"
say ""
say "  ${B}Accounts.${Z} NoteHop has its own accounts. You can also let people sign in with a HOP ID hub:"
say "    standalone  = only NoteHop's own accounts (default)"
say "    existing    = also connect to a HOP ID hub (HEL.ink) I already run"
say "    new         = install a hub next to NoteHop now, then connect to it"
choose HUB "Which one?" standalone standalone existing new
case "$HUB" in
  existing)
    ask HUB_URL "Address of the hub (e.g. https://hub.example.com)?" ""
    [ -n "$HUB_URL" ] || die "The hub address is required with an existing hub"
    HUB_URL="${HUB_URL%/}"
    [ -n "$HUB_SECRET" ] || ask_secret HUB_SECRET "The secret of NoteHop's entry at the hub (Control center, Apps) (hidden)?"
    [ "${#HUB_SECRET}" -ge 32 ] || die "The secret must be at least 32 characters" ;;
  new) ask HUB_DIR "Install the hub into which folder?" "$(dirname "$DIR")/hel-hub" ;;
esac
RUN_DEFAULT=foreground
choose RUN "How will it run?" "$RUN_DEFAULT" foreground systemd docker none
if [ "$RUN" = docker ] && [ "$HAVE_DOCKER" = 0 ]; then die "Docker is not available here. Choose foreground or systemd, or install Docker."; fi
if [ "$RUN" != docker ] && [ "$HAVE_GO" = 0 ]; then die "Go is not installed: choose docker as the way to run it, or install Go."; fi

# --- 3. Summary --------------------------------------------------------------
hdr "Summary"
say "  Site name   $NAME"
say "  Folder      $DIR"
say "  Address     $URL   (port $PORT)"
say "  Data        $DATA"
say "  Accounts    $([ "$HUB" = standalone ] && echo "its own accounts" || { [ "$HUB" = new ] && echo "own accounts + a new hub in $HUB_DIR" || echo "own accounts + hub $HUB_URL"; })"
say "  Run         $RUN"
confirm "Install now?" y || { say "Nothing was changed."; exit 0; }
[ "$DRY" = 1 ] && say "${Y}Dry run: nothing below is executed.${Z}"

# --- 4. Get the code (when run from a download) ---------------------------------
if [ ! -f "$DIR/go.mod" ]; then
  have git || die "git is needed to download NoteHop"
  step "Downloading NoteHop into $DIR" git clone --depth 1 https://github.com/navi-crwn/NoteHop.git "$DIR"
fi
cd "$DIR" 2>/dev/null || { [ "$DRY" = 1 ] && exit 0; die "Cannot enter $DIR"; }

# --- 5. The hub, if asked ---------------------------------------------------------
if [ "$HUB" = new ]; then
  hdr "Installing the hub first (HEL.ink)"
  if [ -f "$HUB_DIR/.env" ] && [ -f "$HUB_DIR/artisan" ]; then
    HUB_URL="$(get_env "$HUB_DIR/.env" APP_URL)"; ok "A hub is already installed in $HUB_DIR ($HUB_URL): using it"
  else
    HUB_PORT="$(free_port 8000)"; [ "$HUB_PORT" = "$PORT" ] && HUB_PORT="$(free_port $((PORT + 1)))"
    HUB_URL="http://localhost:$HUB_PORT"
    if [ ! -f "$HUB_DIR/artisan" ]; then step "Downloading HEL.ink into $HUB_DIR" git clone --depth 1 https://github.com/navi-crwn/HEL.ink.git "$HUB_DIR"; fi
    step "Installing the hub (needs PHP, Composer and Node)" bash "$HUB_DIR/install.sh" --yes --no-star --dir "$HUB_DIR" --name "HOP" --url "$HUB_URL" --port "$HUB_PORT" --db sqlite
  fi
  [ "$DRY" = 1 ] || HUB_SECRET="$(get_env "$HUB_DIR/.env" HOP_SSO_SECRET)"
  [ "$DRY" = 1 ] || [ -n "$HUB_SECRET" ] || die "The hub did not create a secret"
  if [ "$DRY" = 0 ]; then
    # NoteHop signs in with its own secret and callback from the hub's app registry: add its row there.
    HUB_SECRET="$(rand_hex 32)"
    (cd "$HUB_DIR" && HOP_APP_SECRET="$HUB_SECRET" HOP_APP_URL="$URL" php artisan tinker --execute='App\Models\HopApp::updateOrCreate(["slug" => "notehop"], ["name" => "NoteHop", "url" => getenv("HOP_APP_URL"), "callback_path" => "/auth/sso/callback", "start_path" => "/auth/sso", "status" => "active", "secret" => getenv("HOP_APP_SECRET"), "icon" => "note", "color" => "emerald", "sort_order" => 40]);' >/dev/null 2>&1) && ok "Registered NoteHop in the hub's app registry" || die "Could not register NoteHop at the hub"
  fi
  hdr "Back to NoteHop"
fi

# --- 6. Install --------------------------------------------------------------------
hdr "Installing"
mkdir -p "$DATA"
VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
if [ "$RUN" != docker ]; then
  mkdir -p bin
  step "Building NoteHop $VERSION" env CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o bin/notehop ./cmd/notehop
fi

if [ -f .env ] && [ "$DRY" = 0 ]; then
  if confirm ".env already exists. Keep it?" y; then KEEP_ENV=1; else cp .env ".env.backup-$(date +%Y%m%d%H%M%S)"; KEEP_ENV=0; ok "Old .env saved next to it"; fi
else KEEP_ENV=0; fi
if [ "$KEEP_ENV" = 0 ] && [ "$DRY" = 0 ]; then
  : >.env
  if [ "$RUN" = docker ]; then ADDRV=":8080"; DATAV="/data"; else ADDRV="127.0.0.1:$PORT"; DATAV="$DATA"; fi
  set_env .env NOTEHOP_ADDR "$ADDRV"
  set_env .env NOTEHOP_DB "$DATAV/notehop.db"
  set_env .env NOTEHOP_BASE_URL "$URL"
  set_env .env NOTEHOP_SITE "$NAME"
  case "$URL" in https://*) set_env .env NOTEHOP_TRUST_PROXY 1 ;; *) set_env .env NOTEHOP_TRUST_PROXY 0 ;; esac
  set_env .env NOTEHOP_REGISTRATION 1

  chmod 600 .env
  ok "Wrote .env (private to you; never committed)"
fi
if [ "$HUB" != standalone ] && [ "$DRY" = 0 ]; then
    set_env .env NOTEHOP_SSO_URL "$HUB_URL"; set_env .env NOTEHOP_SSO_SECRET "$HUB_SECRET"
    set_env .env NOTEHOP_SSO_NAME "HOP ID"; set_env .env NOTEHOP_SSO_CLIENT notehop
  chmod 600 .env
  ok "Connected .env to the hub $HUB_URL"
fi

if [ "$HUB" = new ] && [ "$DRY" = 0 ] && [ -f "$HUB_DIR/.env" ]; then
  set_env "$HUB_DIR/.env" NOTEHOP_URL "$URL"; set_env "$HUB_DIR/.env" NOTEHOP_ENABLED true
  (cd "$HUB_DIR" && php artisan config:clear >/dev/null 2>&1 && php artisan optimize >/dev/null 2>&1) && ok "Registered NoteHop in the hub's list of apps"
fi

# --- 7. Run scripts ---------------------------------------------------------------------
if [ "$DRY" = 0 ]; then
  cat >start.sh <<START
#!/usr/bin/env bash
# Start NoteHop. Stop with Ctrl+C.
cd "\$(dirname "\$0")"
set -a; . ./.env; set +a
exec ./bin/notehop
START
  chmod +x start.sh
  if [ "$RUN" = docker ]; then
    cat >compose.local.yml <<COMPOSE
# Written by install.sh. Start: docker compose -f compose.local.yml up -d --build
services:
  notehop:
    build:
      context: .
      args:
        VERSION: "$VERSION"
    restart: unless-stopped
    ports:
      - "127.0.0.1:$PORT:8080"
    env_file: .env
    volumes:
      - "$DATA:/data"
COMPOSE
    ok "Wrote compose.local.yml"
  fi
  if [ "$RUN" = systemd ]; then
    mkdir -p deploy/generated
    cat >deploy/generated/notehop.service <<UNIT
[Unit]
Description=NoteHop
After=network-online.target
Wants=network-online.target

[Service]
User=$(id -un)
WorkingDirectory=$DIR
EnvironmentFile=$DIR/.env
ExecStart=$DIR/bin/notehop
Restart=on-failure
NoNewPrivileges=true
ProtectSystem=full
PrivateTmp=true

[Install]
WantedBy=multi-user.target
UNIT
    ok "Wrote deploy/generated/notehop.service"
  fi
fi

# --- 8. Self-test: start it, ask it, stop it ---------------------------------------------
if [ "$DRY" = 0 ] && [ "$RUN" != docker ] && have curl; then
  hdr "Self-test"
  ( set -a; . ./.env; set +a; exec ./bin/notehop ) >/tmp/notehop-selftest.log 2>&1 &
  TESTPID=$!
  UP=0; for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do
    if curl -fsS "http://127.0.0.1:$PORT/healthz" >/dev/null 2>&1; then UP=1; break; fi
    kill -0 "$TESTPID" 2>/dev/null || break
    sleep 1
  done
  kill "$TESTPID" 2>/dev/null || true; wait "$TESTPID" 2>/dev/null || true
  if [ "$UP" = 1 ]; then ok "NoteHop started and answered on port $PORT"; else bad "It did not answer. Log: /tmp/notehop-selftest.log"; tail -5 /tmp/notehop-selftest.log >&2; exit 1; fi
fi

hdr "Done"
if [ "$DRY" = 1 ]; then ok "Dry run finished: nothing was installed"; else ok "NoteHop is installed in $DIR"; fi
say ""
case "$RUN" in
  foreground) say "  ${B}Start it:${Z}   cd $DIR && ./start.sh" ;;
  systemd)    say "  ${B}Run it as a service:${Z}"; say "    sudo cp $DIR/deploy/generated/notehop.service /etc/systemd/system/ && sudo systemctl daemon-reload && sudo systemctl enable --now notehop" ;;
  docker)     say "  ${B}Start it:${Z}   cd $DIR && docker compose -f compose.local.yml up -d --build" ;;
  none)       say "  ${B}Start it:${Z}   cd $DIR && ./start.sh   ${D}(or run bin/notehop with the variables in .env)${Z}" ;;
esac
say "  ${B}Open it:${Z}    $URL"
if [ "$HUB" = new ]; then
  say "  ${B}Also start the hub:${Z}  cd $HUB_DIR && ./start.sh     (sign in there; NoteHop then offers \"Continue with HOP ID\")"
elif [ "$HUB" = existing ]; then
  say "  ${B}At the hub:${Z} add $URL to the apps list (NOTEHOP_URL=$URL, NOTEHOP_ENABLED=true in the hub's .env), then run php artisan optimize there."
fi
say ""
say "  ${B}On the internet:${Z} put nginx or Caddy in front for HTTPS (a ready example is in deploy/), then set the address with https:// in .env."
say "  Update later with ./update.sh · remove with ./uninstall.sh"
say ""
if [ "$DRY" = 0 ]; then star_prompt; fi

}
main "$@"; exit $?
