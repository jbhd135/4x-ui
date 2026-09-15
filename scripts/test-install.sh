#!/usr/bin/env bash
# Run inside a disposable systemd network namespace with local release assets.
set -Eeuo pipefail
HERE=$(cd "$(dirname "$0")" && pwd)
ARTIFACTS="${1:?Usage: test-install.sh RELEASE_DIRECTORY}"
TEST_ROOT=/var/tmp/4x-ui-install-smoke
[[ ! -e "$TEST_ROOT" ]] || { echo 'Test directory already exists'; exit 1; }
mkdir -p "$TEST_ROOT"
export XUI_MAIN_FOLDER="$TEST_ROOT/program" XUI_DB_FOLDER="$TEST_ROOT/db"
export XUI_LOG_FOLDER="$TEST_ROOT/log" XUI_SERVICE_NAME=4x-ui-smoke
export XUI_CLI_BIN="$TEST_ROOT/x-ui" XUI_ENV_FILE="$TEST_ROOT/environment"
export XUI_BACKUP_ROOT="$TEST_ROOT"
DROPIN=/etc/systemd/system/4x-ui-smoke.service.d
cleanup() {
  systemctl disable --now 4x-ui-smoke >/dev/null 2>&1 || true
  rm -f /etc/systemd/system/4x-ui-smoke.service
  rm -rf "$DROPIN" "$TEST_ROOT"
  systemctl daemon-reload
}
trap cleanup EXIT
mkdir -p "$DROPIN"
cat > "$DROPIN/isolation.conf" <<'EOF'
[Unit]
JoinsNamespaceOf=4x-ui-smoke-runner.service
[Service]
PrivateNetwork=yes
EOF
run_install() { bash "$HERE/install-selfhosted.sh" --offline-dir "$ARTIFACTS" --yes --no-config-prompt "$@"; }
run_install --username smoke-admin --password 'Disposable-test-123' --port 28789 --web-base-path /smoke/
[[ $(sqlite3 "$XUI_DB_FOLDER/x-ui.db" 'select count(*) from inbounds;') == 0 ]]
curl -fsS http://127.0.0.1:28789/smoke/ -o "$TEST_ROOT/login.html"
grep -q '<html' "$TEST_ROOT/login.html"
curl -fsS -c "$TEST_ROOT/cookies" --data 'username=smoke-admin&password=Disposable-test-123' http://127.0.0.1:28789/smoke/login | jq -e '.success == true'
curl -fsS -b "$TEST_ROOT/cookies" http://127.0.0.1:28789/smoke/panel/api/inbounds/list | jq -e '.success == true and (.obj | length == 0)'
echo 'PASS: fresh install, empty database, HTTP login and authenticated API'
sqlite3 "$XUI_DB_FOLDER/x-ui.db" "INSERT INTO settings(key,value) VALUES('installTestSentinel','preserved');"
run_install
[[ $(sqlite3 "$XUI_DB_FOLDER/x-ui.db" "select value from settings where key='installTestSentinel';") == preserved ]]
curl -fsS --data 'username=smoke-admin&password=Disposable-test-123' http://127.0.0.1:28789/smoke/login | jq -e '.success == true'
echo 'PASS: upgrade preserves database and credentials'
mkdir "$TEST_ROOT/corrupt"
cp "$ARTIFACTS/x-ui-linux-amd64.tar.gz" "$TEST_ROOT/corrupt/"
printf '%064d  x-ui-linux-amd64.tar.gz\n' 0 > "$TEST_ROOT/corrupt/SHA256SUMS"
if bash "$HERE/install-selfhosted.sh" --offline-dir "$TEST_ROOT/corrupt" --yes --no-config-prompt; then exit 1; fi
systemctl is-active --quiet 4x-ui-smoke
echo 'PASS: checksum mismatch rejected without stopping the panel'
cat > "$DROPIN/failure.conf" <<'EOF'
[Service]
ExecStartPre=/usr/bin/false
EOF
systemctl daemon-reload
if run_install --configure --port 28790; then exit 1; fi
rm "$DROPIN/failure.conf"
systemctl daemon-reload
systemctl reset-failed 4x-ui-smoke || true
systemctl start 4x-ui-smoke
[[ $(sqlite3 "$XUI_DB_FOLDER/x-ui.db" "select value from settings where key='webPort';") == 28789 ]]
[[ $(sqlite3 "$XUI_DB_FOLDER/x-ui.db" "select value from settings where key='installTestSentinel';") == preserved ]]
echo 'PASS: failed startup restores previous database and settings'
echo 'ALL INSTALLATION TESTS PASSED'
