#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

REPO="${GITHUB_REPO:-jbhd135/4x-ui}"
INSTALL_TAG="${INSTALL_VERSION:-latest}"
ROOT="${XUI_MAIN_FOLDER:-/usr/local/x-ui}"
DB="${XUI_DB_FOLDER:-/etc/x-ui}"
LOG="${XUI_LOG_FOLDER:-/var/log/x-ui}"
SERVICE="${XUI_SERVICE_NAME:-x-ui}"
UNIT="${XUI_SERVICE:-/etc/systemd/system}/$SERVICE.service"
ENV_FILE="${XUI_ENV_FILE:-/etc/default/x-ui}"
CLI="${XUI_CLI_BIN:-/usr/bin/x-ui}"
BACKUP_ROOT="${XUI_BACKUP_ROOT:-/root}"
USERNAME="${PANEL_USERNAME:-}"
PASSWORD="${PANEL_PASSWORD:-}"
PORT="${PANEL_PORT:-}"
BASE="${PANEL_WEB_BASE_PATH:-}"
HOST="${PANEL_PUBLIC_HOST:-}"
YES=false NO_PROMPT=false SKIP_START=false CONFIGURE=false
OFFLINE="" WORK="" BACKUP=""
CHANGED=false DONE=false WAS_ACTIVE=false WAS_ENABLED=false

log() { printf '[4x-ui] %s\n' "$*"; }
die() { printf '[4x-ui] Error: %s\n' "$*" >&2; exit 1; }
usage() {
  cat <<'EOF'
4x-ui installer (Linux x86_64, Debian 12+ / Ubuntu 22.04+ with systemd)
Usage: bash install.sh [options]
  --version TAG        Release tag; default: latest
  --repo OWNER/REPO    Default: jbhd135/4x-ui
  --username USER     Panel username (fresh installation prompts/generates)
  --password PASS     Prefer PANEL_PASSWORD environment variable
  --port PORT         Panel TCP port (fresh installation prompts/generates)
  --web-base-path PATH Panel URL path (fresh installation prompts/generates)
  --public-host HOST  Hostname/IP displayed in the final URL
  --configure         Also change settings of an existing installation
  --yes               Accept installation/upgrade confirmation
  --no-config-prompt  Use supplied settings or secure random defaults
  --skip-start        Configure/install files without starting the service
  --offline-dir DIR   Use an archive and SHA256SUMS from a local directory
  --help              Show this help

Existing settings/users are preserved unless --configure is specified.
This is not a side-by-side installer for a VPS already running x-ui.
EOF
}
while (($#)); do
  case "$1" in
    --repo|--version|--username|--password|--port|--web-base-path|--public-host|--offline-dir)
      if (($# < 2)) || [[ -z "$2" ]]; then die "Missing value for $1"; fi
      case "$1" in
        --repo) REPO="$2";; --version) INSTALL_TAG="$2";;
        --username) USERNAME="$2";; --password) PASSWORD="$2";;
        --port) PORT="$2";; --web-base-path) BASE="$2";;
        --public-host) HOST="$2";; --offline-dir) OFFLINE="$2";;
      esac
      shift 2;;
    --yes) YES=true; shift;;
    --no-config-prompt) NO_PROMPT=true; shift;;
    --skip-start) SKIP_START=true; shift;;
    --configure) CONFIGURE=true; shift;;
    --help|-h) usage; exit 0;;
    *) die "Unknown option: $1";;
  esac
done
[[ $EUID == 0 ]] || die "Run as root (sudo -i)."
[[ $(uname -s) == Linux && $(uname -m) == x86_64 ]] || die "This release supports Linux x86_64 only."
[[ "$REPO" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || die "Invalid repository."
[[ "$INSTALL_TAG" =~ ^[A-Za-z0-9_.-]+$ ]] || die "Invalid release tag."
[[ "$SERVICE" =~ ^[A-Za-z0-9_-]+$ ]] || die "Invalid service name."
for path in "$ROOT" "$DB" "$LOG" "$UNIT" "$ENV_FILE" "$CLI" "$BACKUP_ROOT"; do
  [[ "$path" =~ ^/[A-Za-z0-9_./-]+$ && "$path" != / && "$path" != *'..'* ]] || die "Unsafe installation path: $path"
done
if ! command -v systemctl >/dev/null || [[ ! -d /run/systemd/system ]]; then die "A running systemd installation is required."; fi
# shellcheck source=/dev/null
. /etc/os-release
case "$ID" in
  debian) (( ${VERSION_ID%%.*} >= 12 )) || die "Debian 12+ required.";;
  ubuntu) (( ${VERSION_ID%%.*} >= 22 )) || die "Ubuntu 22.04+ required.";;
  *) die "Supported systems: Debian 12+ and Ubuntu 22.04+.";;
esac
command -v flock >/dev/null || die "Install util-linux first."
exec 9>"/run/lock/$SERVICE-install.lock"
flock -n 9 || die "Another installation is already running."
EXISTING=false
[[ ! -f "$DB/x-ui.db" ]] || EXISTING=true
if $EXISTING; then
  log "Existing x-ui detected. This upgrades the current panel, not a second instance."
  if ! $CONFIGURE && [[ -n "$USERNAME$PASSWORD$PORT$BASE" ]]; then
    die "Use --configure to change existing settings. Otherwise all settings are preserved."
  fi
fi
if ! $YES; then
  [[ -t 0 ]] || die "Non-interactive installation requires --yes."
  read -r -p 'Install 4x-ui? [y/N]: ' answer
  [[ "$answer" =~ ^[Yy]$ ]] || die "Cancelled."
fi
missing=false
for cmd in curl tar openssl jq sqlite3 ss sha256sum; do
  command -v "$cmd" >/dev/null || missing=true
done
if $missing; then
  apt-get update
  DEBIAN_FRONTEND=noninteractive apt-get install -y curl tar ca-certificates openssl jq sqlite3 iproute2
fi
random() { openssl rand -hex "$1"; }
port_busy() { ss -H -ltn | awk -v p=":$1$" '$4 ~ p {found=1} END {exit !found}'; }
if ! $EXISTING || $CONFIGURE; then
  if ! $EXISTING; then
    USERNAME="${USERNAME:-admin-$(random 3)}"
    PASSWORD="${PASSWORD:-$(random 12)}"
    BASE="${BASE:-/$(random 6)/}"
    if [[ -z "$PORT" ]]; then
      for ((i=0; i<100; i++)); do
        PORT=$((20000 + RANDOM % 40000))
        port_busy "$PORT" || break
      done
    fi
    if ! $NO_PROMPT && [[ -t 0 ]]; then
      read -r -p "Username [$USERNAME]: " input; USERNAME="${input:-$USERNAME}"
      read -rs -p 'Password [Enter = random]: ' input; printf '\n'; PASSWORD="${input:-$PASSWORD}"
      read -r -p "Panel port [$PORT]: " input; PORT="${input:-$PORT}"
      read -r -p "Panel path [$BASE]: " input; BASE="${input:-$BASE}"
    fi
  fi
  if [[ -n "$USERNAME$PASSWORD" ]]; then
    [[ -n "$USERNAME" && ${#PASSWORD} -ge 10 ]] || die "Supply both username and a password of at least 10 characters."
  fi
  if [[ -n "$PORT" ]]; then
    [[ "$PORT" =~ ^[0-9]{1,5}$ ]] || die "Invalid panel port."
    PORT=$((10#$PORT))
    ((PORT > 0 && PORT <= 65535)) || die "Panel port must be 1..65535."
    if ! $EXISTING; then
      ! port_busy "$PORT" || die "Panel port $PORT is already occupied."
      [[ "$PORT" != 2096 && "$PORT" != 62789 && "$PORT" != 11111 ]] || die "Panel port conflicts with a default internal/subscription port."
    fi
  fi
  if [[ -n "$BASE" ]]; then
    [[ "$BASE" =~ ^/?[A-Za-z0-9_/-]+/?$ ]] || die "Invalid panel path."
    BASE="/${BASE#/}"; BASE="${BASE%/}/"
  fi
fi
WORK=$(mktemp -d)
cp "${BASH_SOURCE[0]}" "$WORK/installer.sh"
rollback() {
  log "Installation failed; restoring the previous files and database."
  systemctl stop "$SERVICE" >/dev/null 2>&1 || true
  rm -rf "$ROOT" "$DB"
  [[ ! -d "$BACKUP/program" ]] || cp -a "$BACKUP/program" "$ROOT"
  [[ ! -d "$BACKUP/database" ]] || cp -a "$BACKUP/database" "$DB"
  if [[ -f "$BACKUP/unit" ]]; then cp -a "$BACKUP/unit" "$UNIT"; else rm -f "$UNIT"; fi
  if [[ -f "$BACKUP/cli" ]]; then cp -a "$BACKUP/cli" "$CLI"; else rm -f "$CLI"; fi
  systemctl daemon-reload
  if $WAS_ENABLED; then systemctl enable "$SERVICE" >/dev/null; else systemctl disable "$SERVICE" >/dev/null 2>&1 || true; fi
  if $WAS_ACTIVE; then systemctl start "$SERVICE" || log "Check the previous service manually: systemctl status $SERVICE"; fi
}
cleanup() {
  local status=$?
  trap - EXIT
  if $CHANGED && ! $DONE; then rollback || true; fi
  [[ -z "$WORK" ]] || rm -rf "$WORK"
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
ASSET=x-ui-linux-amd64.tar.gz
fetch() { curl --proto '=https' --tlsv1.2 -fLsS --retry 3 --connect-timeout 20 --max-time 900 "$1" -o "$2"; }
if [[ -n "$OFFLINE" ]]; then
  cp "$OFFLINE/$ASSET" "$WORK/$ASSET"
  cp "$OFFLINE/SHA256SUMS" "$WORK/SHA256SUMS"
else
  if [[ "$INSTALL_TAG" == latest ]]; then
    fetch "https://api.github.com/repos/$REPO/releases/latest" "$WORK/release.json"
    INSTALL_TAG=$(jq -er '.tag_name | select(type == "string")' "$WORK/release.json")
    [[ "$INSTALL_TAG" =~ ^[A-Za-z0-9_.-]+$ ]] || die "GitHub returned an invalid release tag."
  fi
  URL="https://github.com/$REPO/releases/download/$INSTALL_TAG"
  log "Downloading $REPO / $INSTALL_TAG"
  fetch "$URL/$ASSET" "$WORK/$ASSET"
  fetch "$URL/SHA256SUMS" "$WORK/SHA256SUMS"
fi
EXPECTED=$(awk -v file="$ASSET" '$2 == file {print $1}' "$WORK/SHA256SUMS")
[[ "$EXPECTED" =~ ^[a-fA-F0-9]{64}$ ]] || die "Missing or ambiguous archive checksum."
printf '%s  %s\n' "$EXPECTED" "$WORK/$ASSET" | sha256sum -c -
tar -tzf "$WORK/$ASSET" > "$WORK/files"
if grep -Eq '(^/|(^|/)\.\.(/|$))' "$WORK/files"; then die "Unsafe archive paths."; fi
mkdir "$WORK/extract"
tar -xzf "$WORK/$ASSET" --no-same-owner -C "$WORK/extract"
[[ -x "$WORK/extract/x-ui/x-ui" && -x "$WORK/extract/x-ui/bin/xray-linux-amd64" ]] || die "Incomplete release archive."
[[ ! -e "$WORK/extract/x-ui/bin/config.json" ]] || die "Release must not contain a production Xray configuration."
"$WORK/extract/x-ui/x-ui" -v
"$WORK/extract/x-ui/bin/xray-linux-amd64" version
BACKUP="$BACKUP_ROOT/4x-ui-backup-$(date +%Y%m%d-%H%M%S)-$$"
mkdir -p "$BACKUP"
[[ ! -d "$ROOT" ]] || cp -a "$ROOT" "$BACKUP/program"
[[ ! -f "$UNIT" ]] || cp -a "$UNIT" "$BACKUP/unit"
[[ ! -f "$CLI" ]] || cp -a "$CLI" "$BACKUP/cli"
[[ ! -f "$ENV_FILE" ]] || cp -a "$ENV_FILE" "$BACKUP/environment"
systemctl is-active --quiet "$SERVICE" && WAS_ACTIVE=true
systemctl is-enabled --quiet "$SERVICE" 2>/dev/null && WAS_ENABLED=true
if $WAS_ACTIVE; then systemctl stop "$SERVICE"; fi
# Include WAL and certificates by copying the entire stopped database directory.
if [[ -d "$DB" ]] && ! cp -a "$DB" "$BACKUP/database"; then
  if $WAS_ACTIVE; then systemctl start "$SERVICE"; fi
  die "Database backup failed; installation was not changed."
fi
CHANGED=true
rm -rf "$ROOT"
mkdir -p "$(dirname "$ROOT")" "$DB" "$LOG" "$(dirname "$UNIT")" "$(dirname "$CLI")"
cp -a "$WORK/extract/x-ui" "$ROOT"
export XUI_DB_FOLDER="$DB" XUI_LOG_FOLDER="$LOG" XUI_BIN_FOLDER="$ROOT/bin"
export TZ="${TZ:-Asia/Shanghai}"
if ! $EXISTING || $CONFIGURE; then
  args=()
  [[ -z "$USERNAME" ]] || args+=(-username "$USERNAME" -password "$PASSWORD")
  [[ -z "$PORT" ]] || args+=(-port "$PORT")
  [[ -z "$BASE" ]] || args+=(-webBasePath "$BASE")
  "$ROOT/x-ui" setting "${args[@]}" > "$WORK/settings.log" 2>&1 || { cat "$WORK/settings.log"; die "Could not configure panel."; }
fi
[[ -f "$DB/x-ui.db" ]] || die "Panel database was not initialized."
[[ $(sqlite3 "$DB/x-ui.db" 'PRAGMA quick_check;') == ok ]] || die "Database integrity check failed."
cat > "$UNIT" <<EOF
[Unit]
Description=4x-ui Panel (x-ui compatible)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=$ROOT
Environment="TZ=Asia/Shanghai"
Environment="XUI_DB_FOLDER=$DB"
Environment="XUI_LOG_FOLDER=$LOG"
Environment="XUI_BIN_FOLDER=$ROOT/bin"
Environment="XRAY_VMESS_AEAD_FORCED=false"
EnvironmentFile=-$ENV_FILE
ExecStart=$ROOT/x-ui
Restart=on-failure
RestartSec=5
UMask=0077

[Install]
WantedBy=multi-user.target
EOF
chmod 644 "$UNIT"
cat > "$CLI" <<EOF
#!/usr/bin/env bash
set -euo pipefail
export XUI_DB_FOLDER='$DB' XUI_LOG_FOLDER='$LOG' XUI_BIN_FOLDER='$ROOT/bin' TZ=Asia/Shanghai
case "\${1:-status}" in
  start|stop|restart|status|enable|disable) exec systemctl "\${1:-status}" '$SERVICE';;
  log) exec journalctl -u '$SERVICE' -e --no-pager;;
  update) exec bash '$ROOT/install-selfhosted.sh' "\${@:2}";;
  *) exec '$ROOT/x-ui' "\$@";;
esac
EOF
chmod 755 "$CLI"
install -m 755 "$WORK/installer.sh" "$ROOT/install-selfhosted.sh"
systemctl daemon-reload
systemctl enable "$SERVICE" >/dev/null
if ! $SKIP_START; then
  systemctl start "$SERVICE"
  ready=false
  for ((i=0; i<30; i++)); do
    ACTUAL_PORT=$(sqlite3 "$DB/x-ui.db" "SELECT value FROM settings WHERE key='webPort';")
    [[ -n "$ACTUAL_PORT" ]] || ACTUAL_PORT=2053
    if systemctl is-active --quiet "$SERVICE" && port_busy "$ACTUAL_PORT"; then ready=true; break; fi
    sleep 1
  done
  $ready || die "Panel listener did not become ready. See journalctl -u $SERVICE."
  sleep 2
  systemctl is-active --quiet "$SERVICE" || die "Panel exited after startup."
fi
DONE=true
log "Installed release: $INSTALL_TAG; program: $("$ROOT/x-ui" -v)"
log "Backup directory: $BACKUP"
"$ROOT/x-ui" setting -show
if [[ -n "$USERNAME" ]]; then log "Username: $USERNAME"; log "Password: $PASSWORD"; fi
if ! $EXISTING; then
  HOST="${HOST:-$(hostname -I | awk '{print $1}')}"
  log "Panel URL: http://${HOST:-YOUR_SERVER_IP}:$PORT$BASE"
  log "Configure HTTPS before using the panel over the public Internet."
fi
if $SKIP_START; then log "Service was not started (--skip-start)."; fi
log "Service: systemctl status $SERVICE; daily quota timezone defaults to Asia/Shanghai."
