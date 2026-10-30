const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const os = require('node:os');
const { AppService } = require('../app-service');

function setup(t, downloadHandler) {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'qingzhou-store-test-'));
  const auth = { guid: '020102030405', account: { email: 'store-test@example.invalid' }, busy: false, pending: null, initialize: async () => {},
    bridge: { request: async input => input.action === 'export' ? { ok: true, session: { account: { passwordToken: 'test-token' } } } : { ok: true, data: { ids: ['1', '2'], latestId: '2' } } } };
  const requests = []; const updates = [];
  const createClient = () => ({
    request: async (input, options) => { requests.push(input); if (input.action === 'restore') return { ok: true }; return downloadHandler(input, options); },
    stop: async () => {}
  });
  const service = new AppService({ auth, directory, defaultDownloadDirectory: path.join(directory, 'ipa'), createClient, onUpdate: state => updates.push(state) });
  t.after(() => { service.close(); assert.equal(path.dirname(path.resolve(directory)), path.resolve(os.tmpdir())); assert.ok(path.basename(directory).startsWith('qingzhou-store-test-')); fs.rmSync(directory, { recursive: true, force: true }); });
  return { service, auth, requests, updates, directory };
}
const selection = { appId: 414478124, platform: 'ipad', versionId: '12345', name: '测试应用', version: '1.0' };
async function until(predicate) { for (let n = 0; n < 100; n++) { if (predicate()) return; await new Promise(resolve => setTimeout(resolve, 10)); } throw Error('condition timed out'); }
test('download uses exact selected app/version/platform and exposes only final file', async t => {
  const { service, requests, updates } = setup(t, async (input, options) => {
    options.onProgress({ bytes: 10, total: 20, percent: 0.5, speed: 1 });
    assert.equal(fs.existsSync(service.jobs[0].file), false);
    fs.writeFileSync(input.outputPath, 'completed-ipa-fixture');
    return { ok: true, data: { path: input.outputPath } };
  });
  assert.equal((await service.enqueue(selection)).ok, true);
  await until(() => service.jobs[0].status === 'completed');
  const request = requests.find(r => r.action === 'download');
  assert.equal(request.appId, selection.appId); assert.equal(request.platform, 'ipad'); assert.equal(request.versionId, '12345');
  assert.equal(fs.existsSync(service.jobs[0].staging), false);
  assert.equal(fs.readFileSync(service.jobs[0].file, 'utf8'), 'completed-ipa-fixture');
  assert.equal(service.snapshot().jobs[0].owner, undefined);
  assert.equal(JSON.stringify(updates).includes('test-token'), false);
  await service.control({ id: service.jobs[0].id, action: 'remove' });
  assert.equal(service.jobs.length, 0);
});
test('license/server failures never become completed jobs', async t => {
  const { service } = setup(t, async () => ({ ok: false, errorCode: 'license', error: '需要许可' }));
  await service.enqueue(selection); await until(() => service.jobs[0].status === 'failed');
  assert.equal(service.jobs[0].errorCode, 'license'); assert.equal(fs.existsSync(service.jobs[0].file), false);
});
test('search accepts name/ID but rejects invalid device and blank input', async t => {
  const { service, requests } = setup(t, async () => ({ ok: true, data: [] }));
  assert.equal((await service.search({ query: '414478124', country: 'cn', platform: 'iphone' })).ok, true);
  assert.equal((await service.search({ query: '微信', country: 'cn', platform: 'ipad' })).ok, true);
  assert.equal((await service.search({ query: ' ', country: 'cn', platform: 'iphone' })).ok, false);
  assert.equal((await service.search({ query: 'x', country: 'cn', platform: 'mac' })).ok, false);
  assert.equal(requests.length, 2);
});
test('authentication is required for version and download operations', async t => {
  const { service, auth, requests } = setup(t, async () => ({ ok: true })); auth.account = null;
  assert.equal((await service.versions(selection)).errorCode, 'login');
  assert.equal((await service.enqueue(selection)).ok, false); assert.equal(requests.length, 0);
});
test('expanded regions and follow-account use the correct Apple catalog country', async t => {
  const { service, auth, requests } = setup(t, async () => ({ ok: true, data: [] }));
  auth.account.storefront = '143462-19,32';
  assert.equal((await service.search({ query: 'test', country: 'account', platform: 'iphone' })).ok, true);
  assert.equal(requests.at(-1).country, 'jp');
  for (const country of ['mo', 'kr', 'fr', 'de', 'it', 'au', 'ca', 'sg', 'my', 'th', 'vn', 'ru', 'in', 'cl', 'gr', 'ae', 'se', 'ch', 'za']) {
    assert.equal((await service.search({ query: 'test', country, platform: 'iphone' })).ok, true);
    assert.equal(requests.at(-1).country, country);
  }
  auth.account = null;
  assert.equal((await service.search({ query: 'test', country: 'account', platform: 'iphone' })).ok, false);
});
test('task directory selection is honored without changing the default', async t => {
  const { service, directory } = setup(t, async () => ({ ok: false, error: 'fixture' }));
  const original = service.settings.downloadDirectory;
  const chosen = service.selectTaskDirectory(path.join(directory, 'separate folder'));
  service.closed = true;
  assert.equal((await service.enqueue({ ...selection, directoryToken: chosen.directoryToken })).ok, true);
  assert.equal(service.jobs[0].folder, path.join(directory, 'separate folder'));
  assert.equal(service.settings.downloadDirectory, original);
  assert.equal((await service.enqueue({ ...selection, directoryToken: 'untrusted-path-token' })).ok, false);
});
test('pausing invalidates a late successful transfer; retry can complete', async t => {
  let finish;
  const { service } = setup(t, async input => new Promise(resolve => { finish = () => { fs.writeFileSync(input.outputPath, 'late-file'); resolve({ ok: true, data: { path: input.outputPath } }); }; }));
  await service.enqueue(selection); await until(() => Boolean(finish));
  const job = service.jobs[0]; await service.control({ id: job.id, action: 'pause' }); finish();
  await new Promise(resolve => setTimeout(resolve, 30));
  assert.equal(job.status, 'paused'); assert.equal(fs.existsSync(job.file), false);
});
test('jobs reload paused and settings persist', async t => {
  const { service, directory, auth } = setup(t, async () => ({ ok: false, error: 'fixture' }));
  service.settings.parallel = 1; service.workers.set('hold', {});
  await service.enqueue(selection); service.workers.delete('hold');
  service.saveSettings({ parallel: 3 }); await until(() => service.jobs[0].status === 'failed');
  const reopened = new AppService({ auth, directory, defaultDownloadDirectory: directory });
  assert.equal(reopened.settings.parallel, 3); assert.equal(reopened.jobs[0].status, 'paused');
});

test('third-party sources merge with Apple and preserve partial failures', async t => {
  const {service,auth}=setup(t,async()=>({ok:true}));let calls=[];
  service.fetchVersionProvider=async(source,id)=>{calls.push({source,id});if(source==='agzy')throw Error('secret-server-error');return [{id:'10',version:'1.0',sources:[source],updatedAt:'',sizeBytes:0}];};
  const r=await service.versions({...selection,source:'combined'});
  assert.equal(r.ok,true);assert.deepEqual(new Set(r.data.ids),new Set(['1','2','10']));assert.deepEqual(r.data.records[0].sources,['timbrd','bilin']);assert.equal(r.data.sources.find(s=>s.source==='agzy').ok,false);assert.equal(JSON.stringify(r).includes('secret-server-error'),false);
  assert.equal(calls.length,3);assert.ok(calls.every(c=>c.id===selection.appId));
  calls=[];const auto=await service.versions({...selection,source:'auto'});assert.equal(auto.ok,true);assert.equal(calls.length,1);assert.equal(auto.data.records[0].version,'1.0');
  auth.bridge.request=async()=>({ok:false,errorCode:'license',error:'needs license'});
  assert.equal((await service.versions({...selection,source:'combined'})).ok,true);
  assert.equal((await service.versions({...selection,source:'apple'})).errorCode,'license');
  assert.equal((await service.versions({...selection,source:'unknown'})).ok,false);
});
