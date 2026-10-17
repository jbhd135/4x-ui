const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawnSync } = require('node:child_process');

const installer = fs.readFileSync(path.join(__dirname, 'install-selfhosted.sh'), 'utf8');
const fn = installer.match(/^migrate_client_ip_jail\(\) \{[\s\S]*?^\}/m)?.[0];
assert.ok(fn, 'migration function exists');
for (const active of ['true', 'false']) {
 for (const version of ['4.0.7', '4.0.9']) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), '4x-ui-jail-test-'));
  try {
    fs.mkdirSync(path.join(dir, 'backup'));
    fs.writeFileSync(path.join(dir, 'x-ui'), '#!/usr/bin/env bash\nprintf "%s\\n" "$TEST_VERSION"\n', { mode: 0o755 });
    fs.writeFileSync(path.join(dir, 'override'), '[3x-ipl]\nenabled = true\n');
    fs.writeFileSync(path.join(dir, 'sshd.local'), '[sshd]\nenabled = true\n');
    const script = `
      set -euo pipefail
      BACKUP="$TEST_DIR/backup"
      ROOT="$TEST_DIR"
      JAIL_OVERRIDE="$TEST_DIR/override"
      JAIL_WAS_ACTIVE=false
      JAIL_MIGRATED=false
      log() { :; }
      fail2ban-client() {
        printf '%s\\n' "$*" >> "$TEST_DIR/calls"
        case "$*" in
          "status 3x-ipl") $TEST_ACTIVE;;
          "stop 3x-ipl") return 0;;
          *) echo "Unexpected fail2ban operation" >&2; return 1;;
        esac
      }
      ${fn}
      migrate_client_ip_jail
      if [[ "$TEST_VERSION" == 4.0.9 ]]; then
        $JAIL_MIGRATED
        test "$JAIL_WAS_ACTIVE" = "$TEST_ACTIVE"
      else
        ! $JAIL_MIGRATED
      fi
    `;
    const result = spawnSync('bash', ['-c', script], { encoding: 'utf8', env: { ...process.env, TEST_DIR: dir, TEST_ACTIVE: active, TEST_VERSION: version } });
    assert.equal(result.status, 0, result.stderr);
    if (version === '4.0.7') {
      assert.equal(fs.readFileSync(path.join(dir, 'override'), 'utf8'), '[3x-ipl]\nenabled = true\n');
      assert.equal(fs.existsSync(path.join(dir, 'calls')), false);
      continue;
    }
    assert.equal(fs.readFileSync(path.join(dir, 'override'), 'utf8'), '[3x-ipl]\nenabled = false\n');
    assert.equal(fs.readFileSync(path.join(dir, 'backup/client-ip-jail'), 'utf8'), '[3x-ipl]\nenabled = true\n');
    assert.equal(fs.readFileSync(path.join(dir, 'sshd.local'), 'utf8'), '[sshd]\nenabled = true\n');
    assert.equal(fs.readFileSync(path.join(dir, 'calls'), 'utf8'), active === 'true' ? 'status 3x-ipl\nstop 3x-ipl\n' : 'status 3x-ipl\n');
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
 }
}
console.log('Legacy jail migration: backup, disable, selective stop and SSH isolation passed.');
