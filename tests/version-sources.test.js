const { test } = require('node:test');
const assert = require('node:assert/strict');
const { fetchProvider, normalizeRecords, mergeRecords, requestJSON } = require('../version-sources');
test('normalizes source schemas and rejects unsafe or duplicate version IDs', () => {
  const records = normalizeRecords([{external_identifier: 893607403, bundle_version: '2.2.93', created_at: '2026-10-30', size: 55769088}, {versionId:'893607403',version:'duplicate'}, {id:'3e8',version:'1'}, {id:9007199254740992,version:'1'}, {version_id:'893607402',version:'2.2.92',fileSize:'52 MB'}], 'bilin');
  assert.equal(records.length,2); assert.equal(records[0].id,'893607403'); assert.equal(records[0].sizeBytes,55769088); assert.equal(records[1].sizeText,'52 MB'); assert.equal(records[0].dateSource,'third-party');
});
test('provider calls contain only public App ID and use fixed HTTPS hosts', async () => {
  const addresses=[];
  for(const source of ['timbrd','bilin','agzy']) await fetchProvider(source,932747118,async address=>{addresses.push(address);return [{id:'123',version:'2.0'}]});
  assert.equal(addresses.length,3); assert.ok(addresses.every(url=>url.includes('932747118')&&url.startsWith('https://')));
  await assert.rejects(fetchProvider('arbitrary',932747118)); await assert.rejects(fetchProvider('timbrd','bad'));
  await assert.rejects(requestJSON('https://example.com/')); await assert.rejects(requestJSON('http://api.timbrd.com/'));
  await assert.rejects(fetchProvider('bilin',932747118,async()=>'<html>'));
});
test('merge keeps distinct versions and joins provenance without duplicates', () => {
  const a=normalizeRecords([{id:'123',version:'1.0'}],'timbrd');
  const b=normalizeRecords([{id:'123',version:'1.0',size:400},{id:'124',version:'1.1'}],'bilin');
  const r=mergeRecords([a,b]);assert.equal(r.length,2);assert.deepEqual(r[0].sources,['timbrd','bilin']);assert.equal(r[0].sizeBytes,400);assert.deepEqual(a[0].sources,['timbrd']);
});
