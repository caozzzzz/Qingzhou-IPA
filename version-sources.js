const https = require('node:https');
const PROVIDERS = Object.freeze({
  timbrd: id => `https://api.timbrd.com/apple/app-version/index.php?id=${id}`,
  bilin: id => `https://apis.bilin.eu.org/history/${id}`,
  agzy: id => `https://app.agzy.cn/searchVersion?appid=${id}`
});
const allowedHosts = new Set(['api.timbrd.com', 'apis.bilin.eu.org', 'app.agzy.cn']);
function requestJSON(address) {
  const url = new URL(address);
  if (url.protocol !== 'https:' || !allowedHosts.has(url.hostname) || url.port || url.username || url.password) return Promise.reject(new Error('不允许的版本源地址'));
  return new Promise((resolve, reject) => {
    // This public request never receives an Apple session, cookie, GUID or password.
    const request = https.get(url, { headers: { Accept: 'application/json', 'User-Agent': 'QingzhouIPA/0.8' } }, response => {
      if (response.statusCode !== 200) { response.resume(); reject(new Error(`服务返回 HTTP ${response.statusCode}`)); return; }
      let bytes = 0; const chunks = [];
      response.on('data', chunk => { bytes += chunk.length; if (bytes > 8 * 1024 * 1024) request.destroy(new Error('版本源响应过大')); else chunks.push(chunk); });
      response.on('end', () => { try { resolve(JSON.parse(Buffer.concat(chunks).toString('utf8'))); } catch { reject(new Error('版本源返回了无效数据')); } });
      response.on('error', reject);
    });
    const timer = setTimeout(() => request.destroy(new Error('版本源连接超时')), 20000);
    request.on('close', () => clearTimeout(timer)); request.on('error', reject);
  });
}
function normalizeRecords(data, source) {
  const items = Array.isArray(data) ? data : Array.isArray(data?.data) ? data.data : null;
  if (!items) throw new Error('版本源数据格式不匹配');
  const seen = new Set(), records = [];
  for (const item of items.slice(0, 20000)) {
    if (!item || typeof item !== 'object') continue;
    const rawId = item.external_identifier ?? item.versionId ?? item.version_id ?? item.id;
    if (typeof rawId === 'number' && (!Number.isSafeInteger(rawId) || rawId <= 0)) continue;
    const id = String(rawId ?? '').trim();
    const version = String(item.bundle_version ?? item.version ?? item.bundleShortVersionString ?? '').split(/[\r\n]/)[0].trim();
    if (!/^[1-9][0-9]{0,23}$/.test(id) || !version || version.length > 64 || !/\d/.test(version) || seen.has(id)) continue;
    seen.add(id);
    const rawSize = item.size ?? item.fileSize ?? item.fileSizeBytes;
    const sizeBytes = typeof rawSize === 'number' && Number.isSafeInteger(rawSize) && rawSize > 0 ? rawSize : 0;
    const sizeText = !sizeBytes && typeof rawSize === 'string' && rawSize.length < 40 ? rawSize : '';
    const rawDate = String(item.created_at ?? item.createTime ?? item.updateTime ?? item.date ?? item.time ?? '').slice(0, 100);
    records.push({ id, version, updatedAt: rawDate, sizeBytes, sizeText, dateSource: 'third-party', sources: [source] });
  }
  return records;
}
async function fetchProvider(source, appId, get = requestJSON) {
  if (!Object.hasOwn(PROVIDERS, source) || !Number.isSafeInteger(appId) || appId <= 0) throw new Error('应用或版本源无效');
  return normalizeRecords(await get(PROVIDERS[source](appId)), source);
}
function mergeRecords(groups) {
  const map = new Map();
  for (const group of groups) for (const record of group) {
    const previous = map.get(record.id);
    if (!previous) map.set(record.id, { ...record, sources: [...record.sources] });
    else { previous.sources = [...new Set([...previous.sources, ...record.sources])]; if (!previous.updatedAt) previous.updatedAt = record.updatedAt; if (!previous.sizeBytes) previous.sizeBytes = record.sizeBytes; if (!previous.sizeText) previous.sizeText = record.sizeText; }
  }
  return [...map.values()];
}
module.exports = { fetchProvider, normalizeRecords, mergeRecords, requestJSON };
