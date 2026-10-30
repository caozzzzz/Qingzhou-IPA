const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const crypto = require('node:crypto');
const { AuthService } = require('../auth-service');

const account = { email: 'test@example.invalid', name: '测试账户', storefront: '143465' };
function success() { return { ok: true, account: { ...account }, session: { account: { email: account.email, passwordToken: 'fake-test-token', password: 'must-not-persist' }, cookies: [] } }; }
function fixture(t, handler, available = true, directory) {
  const dir = directory || fs.mkdtempSync(path.join(os.tmpdir(), 'qingzhou-auth-unit-'));
  const key = crypto.randomBytes(32); const calls = [];
  const safeStorage = { isEncryptionAvailable: () => available,
    encryptString(text) { const iv = crypto.randomBytes(12); const cipher = crypto.createCipheriv('aes-256-gcm', key, iv); const data = Buffer.concat([cipher.update(text, 'utf8'), cipher.final()]); return Buffer.concat([iv, cipher.getAuthTag(), data]); },
    decryptString(data) { const cipher = crypto.createDecipheriv('aes-256-gcm', key, data.subarray(0, 12)); cipher.setAuthTag(data.subarray(12, 28)); return Buffer.concat([cipher.update(data.subarray(28)), cipher.final()]).toString('utf8'); }
  };
  let stopCount = 0;
  const bridge = { request: async input => { calls.push({ ...input }); return handler(input); }, stop: () => { stopCount++; } };
  const service = new AuthService({ directory: dir, safeStorage, bridge });
  t.after(() => {
    service.close();
    if (!directory) {
      assert.equal(path.dirname(path.resolve(dir)), path.resolve(os.tmpdir()));
      assert.ok(path.basename(dir).startsWith('qingzhou-auth-unit-'));
      fs.rmSync(dir, { recursive: true, force: true });
    }
  });
  return { service, dir, safeStorage, bridge, calls, stops: () => stopCount };
}
const login = { email: account.email, password: 'unit-test-password', remember: false };

test('rejects invalid credentials and 2FA before contacting backend', async t => {
  const { service, calls } = fixture(t, () => { throw Error('not expected'); });
  assert.equal((await service.login({ email: '', password: '', remember: false })).ok, false);
  assert.equal((await service.submitCode({ code: '123456' })).restart, true);
  assert.equal(calls.length, 0);
});
test('2FA retains password only for pending challenge and never returns secrets', async t => {
  let count = 0;
  const { service, calls } = fixture(t, () => ++count === 1 ? { need2FA: true } : success());
  assert.equal((await service.login({ ...login })).need2FA, true);
  assert.equal((await service.submitCode({ code: 'abc' })).need2FA, true);
  const result = await service.submitCode({ code: '123456' });
  assert.equal(result.ok, true);
  assert.equal(calls[1].password, login.password);
  assert.equal(calls[1].code, '123456');
  assert.equal(service.pending, null);
  assert.equal(JSON.stringify(result).includes('password'), false);
  assert.equal(JSON.stringify(result).includes('token'), false);
});
test('remembered sessions are encrypted and exclude the Apple password', async t => {
  const { service, dir, safeStorage, bridge } = fixture(t, input => input.action === 'restore' ? { ok: true, account } : success());
  assert.equal((await service.login({ ...login, remember: true })).remembered, true);
  const raw = fs.readFileSync(service.sessionFile);
  assert.equal(raw.includes(Buffer.from(account.email)), false);
  assert.equal(raw.includes(Buffer.from('fake-test-token')), false);
  const decrypted = safeStorage.decryptString(raw);
  assert.equal(decrypted.includes('must-not-persist'), false);
  assert.equal(decrypted.includes('"password":'), false);
  const restored = new AuthService({ directory: dir, safeStorage, bridge });
  t.after(() => restored.close());
  await restored.initialize();
  assert.equal(restored.status().restored, true);
  assert.equal(restored.status().account.email, account.email);
  await restored.logout();
  assert.equal(fs.existsSync(service.sessionFile), false);
});
test('non-remembered login writes no credential file', async t => {
  const { service } = fixture(t, success);
  assert.equal((await service.login({ ...login })).ok, true);
  assert.equal(fs.existsSync(service.sessionFile), false);
});
test('closing account dialog preserves active session; logout stops it', async t => {
  const { service, stops } = fixture(t, success);
  await service.login({ ...login });
  const before = stops();
  service.cancel();
  assert.equal(stops(), before);
  assert.equal(service.status().account.email, account.email);
  await service.logout();
  assert.ok(stops() > before);
  assert.equal(service.status().account, null);
});
test('unavailable secure storage falls back to current session only', async t => {
  const { service } = fixture(t, success, false);
  const result = await service.login({ ...login, remember: true });
  assert.equal(result.ok, true); assert.equal(result.remembered, false); assert.ok(result.warning);
  assert.equal(fs.existsSync(service.sessionFile), false);
});
test('cancel invalidates delayed success and prevents persistence', async t => {
  let finish;
  const { service } = fixture(t, () => new Promise(resolve => { finish = resolve; }));
  const pending = service.login({ ...login, remember: true });
  await new Promise(resolve => setImmediate(resolve));
  service.cancel(); finish(success());
  assert.equal((await pending).cancelled, true);
  assert.equal(service.status().account, null);
  assert.equal(fs.existsSync(service.sessionFile), false);
});
test('expired 2FA clears the pending password', async t => {
  const { service, calls } = fixture(t, () => ({ need2FA: true }));
  await service.login({ ...login });
  const old = service.pending; old.expires = 0;
  assert.equal((await service.submitCode({ code: '123456' })).restart, true);
  assert.equal(old.password, ''); assert.equal(calls.length, 1);
});
test('failed Apple authentication is never treated as successful', async t => {
  const { service } = fixture(t, () => ({ ok: false, error: 'Apple 拒绝认证。' }));
  const result = await service.login({ ...login });
  assert.equal(result.ok, false); assert.equal(service.status().account, null);
});
test('corrupt encrypted sessions request fresh login', async t => {
  const { service } = fixture(t, () => { throw Error('not expected'); });
  fs.writeFileSync(service.sessionFile, 'corrupt');
  await service.initialize();
  assert.equal(service.status().account, null); assert.ok(service.status().warning);
});
