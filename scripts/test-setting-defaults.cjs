const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const root = path.resolve(__dirname, '..');
for (const file of ['web/assets/js/util/index.js', 'web/assets/js/model/setting.js']) {
    vm.runInThisContext(fs.readFileSync(path.join(root, file), 'utf8'), { filename: file });
}
for (const data of [undefined, null, {}, { webPort: 12345 }]) {
    assert.equal(new AllSetting(data).restartXrayOnClientDisable, false);
}
for (const value of [true, false]) {
    const setting = new AllSetting({ restartXrayOnClientDisable: value });
    assert.equal(setting.restartXrayOnClientDisable, value);
    assert.equal(JSON.parse(JSON.stringify(setting)).restartXrayOnClientDisable, value);
}
console.log('Settings default and existing restart choices passed.');
