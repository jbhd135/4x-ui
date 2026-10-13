const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const html = fs.readFileSync(path.join(__dirname, '../web/html/inbounds.html'), 'utf8');
function method(name) {
  const match = html.match(new RegExp(`^      (?:async )?${name}\\(`, 'm'));
  assert.ok(match, `Missing method: ${name}`);
  const rest = html.slice(match.index);
  const end = rest.indexOf('\n      },') + '\n      },'.length;
  assert.ok(end > 0);
  return vm.runInThisContext(`({${rest.slice(0, end)}})`)[name];
}
const methods = Object.fromEntries([
  'getDBInbounds', 'loadDBInbounds', 'getLastOnlineMap', 'getOnlineUsers',
  'refreshAfterClientCreate', 'submit', 'addClient', 'serializeClientForUpdate',
  'submitQuickAddClient',
].map(name => [name, method(name)]));
const deferred = () => {
  let resolve, reject;
  const promise = new Promise((ok, fail) => { resolve = ok; reject = fail; });
  return { promise, resolve, reject };
};
const tick = () => new Promise(resolve => setImmediate(resolve));
function state() {
  const notices = [];
  return {
    ...methods, refreshing: false, notices, applied: [],
    onlineClients: [], lastOnlineMap: {},
    $message: Object.fromEntries(['success', 'warning', 'error'].map(type =>
      [type, text => notices.push({ type, text })])),
    $nextTick: callback => queueMicrotask(callback),
    setInbounds(rows) { this.applied.push(rows); },
  };
}
function fakeRequests() {
  const calls = [];
  const request = url => {
    const pending = deferred();
    calls.push({ url, ...pending });
    return pending.promise;
  };
  globalThis.HttpUtil = { get: request, post: request };
  return calls;
}
function finishBatch(calls, rows) {
  for (const call of calls) call.resolve({ success: true, obj: call.url.endsWith('/list') ? rows : [] });
}
function creator() {
  const app = state();
  const client = { email: '20271113-test01', id: 'test-uuid', limitIp: 1, expiryTime: 1826121599999 };
  app.quickAddClientModal = {
    visible: true, saving: false, inboundId: 2, clientName: '',
    expiryPresetMonths: 12, expiry: { valueOf: () => client.expiryTime }, deviceLimit: 1,
  };
  app.dbInbounds = [{ id: 2, toInbound: () => ({ clients: [] }) }];
  app.createQuickClient = () => client;
  app.getQuickClientShareLink = () => 'vless://test-uuid@example.invalid:443#test01';
  globalThis.ClipboardManager = { copyText: async () => true };
  return app;
}

(async () => {
  // All three reads start together; repeated refresh signals share one batch.
  let app = state(), calls = fakeRequests();
  const first = app.getDBInbounds();
  assert.equal(calls.length, 3, 'list and online requests must not be sequential');
  assert.equal(app.getDBInbounds(), first);
  assert.equal(calls.length, 3);
  finishBatch(calls, [{ id: 2 }]);
  assert.equal(await first, true);
  assert.equal(app.refreshing, false);
  assert.equal(app._inboundsRefreshPromise, null);
  assert.deepEqual(app.applied, [[{ id: 2 }]]);

  // A save finishing during an old fetch must trigger a new snapshot.
  app = state(); calls = fakeRequests();
  const old = app.getDBInbounds();
  const afterSave = app.getDBInbounds({ fresh: true });
  app.getDBInbounds({ fresh: true });
  assert.equal(old, afterSave);
  finishBatch(calls.slice(0, 3), [{ id: 'stale' }]);
  await tick();
  assert.equal(calls.length, 6, 'coalesce mutations into one additional batch');
  assert.equal(app.applied.length, 0, 'do not paint pre-save data');
  finishBatch(calls.slice(3), [{ id: 'fresh' }]);
  await afterSave;
  assert.deepEqual(app.applied, [[{ id: 'fresh' }]]);

  // Failed requests release the refresh lock and preserve existing rows.
  app = state(); calls = fakeRequests();
  const failed = app.getDBInbounds();
  calls[0].resolve({ success: false });
  finishBatch(calls.slice(1), []);
  assert.equal(await failed, false);
  assert.equal(app.refreshing, false);
  assert.equal(app.applied.length, 0);
  const rejected = app.getDBInbounds();
  calls[3].reject(new Error('offline'));
  finishBatch(calls.slice(4), []);
  await assert.rejects(rejected, /offline/);
  assert.equal(app.refreshing, false);
  assert.equal(app._inboundsRefreshPromise, null);
  const retry = app.getDBInbounds();
  finishBatch(calls.slice(6), [{ id: 'recovered' }]);
  assert.equal(await retry, true);

  // Creation is confirmed before slow list reads, with one POST even on double-click.
  app = creator(); calls = fakeRequests();
  let posts = 0, posted;
  const save = deferred();
  HttpUtil.postWithModal = (url, payload) => {
    posts++;
    posted = { url, payload };
    return save.promise;
  };
  const creating = app.submitQuickAddClient();
  await app.submitQuickAddClient();
  assert.equal(posts, 1);
  assert.equal(app.quickAddClientModal.saving, true);
  assert.equal(calls.length, 0);
  save.resolve({ success: true });
  await creating;
  await tick();
  assert.equal(app.quickAddClientModal.visible, false);
  assert.equal(app.quickAddClientModal.saving, false);
  assert.equal(calls.length, 3);
  assert.equal(app.refreshing, true, 'background fetch must still be pending');
  assert.equal(app.notices.at(-1).type, 'success');
  assert.equal(posted.url, '/panel/api/inbounds/addClient');
  assert.deepEqual(JSON.parse(posted.payload.settings).clients[0], {
    email: '20271113-test01', id: 'test-uuid', limitIp: 1,
    expiryTime: 1826121599999, socksProxyEnabled: false,
  });
  finishBatch(calls, [{ id: 2 }]);
  await app._inboundsRefreshPromise;

  // Server refusal keeps the dialog editable and never fetches a success snapshot.
  app = creator(); calls = fakeRequests();
  HttpUtil.postWithModal = async () => ({ success: false });
  await app.submitQuickAddClient();
  assert.equal(app.quickAddClientModal.visible, true);
  assert.equal(app.quickAddClientModal.saving, false);
  assert.equal(calls.length, 0);
  assert.equal(app.notices.length, 0);
  HttpUtil.postWithModal = async () => { throw new Error('save failed'); };
  await assert.rejects(app.submitQuickAddClient(), /save failed/);
  assert.equal(app.quickAddClientModal.saving, false);

  // Clipboard permission prompts cannot keep the creation spinner active.
  app = creator(); calls = fakeRequests();
  const clipboard = deferred();
  ClipboardManager.copyText = () => clipboard.promise;
  HttpUtil.postWithModal = async () => ({ success: true });
  const copying = app.submitQuickAddClient();
  await tick();
  assert.equal(app.quickAddClientModal.visible, false);
  assert.equal(app.quickAddClientModal.saving, false);
  clipboard.resolve(true);
  await copying;
  calls[0].resolve({ success: false });
  finishBatch(calls.slice(1), []);
  await tick();
  assert.match(app.notices.at(-1).text, /已创建.*不要重复创建/);

  // Standard edit/add flows keep their awaited refresh semantics.
  app = state(); calls = fakeRequests();
  HttpUtil.postWithModal = async () => ({ success: true });
  let done = false;
  const standard = app.submit('/edit', {}).then(() => { done = true; });
  await tick();
  assert.equal(done, false);
  finishBatch(calls, []);
  await standard;
  assert.equal(done, true);
  console.log('Client creation: early confirmation, duplicate protection, clipboard, refresh races and failures passed.');
})().catch(error => { console.error(error); process.exitCode = 1; });
