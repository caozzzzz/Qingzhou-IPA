const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const { fetchProvider, mergeRecords } = require('./version-sources');
const { BridgeClient } = require('./auth-service');
const { REGIONS, resolveAccountCountry } = require('./regions');

function validSelection(input) {
  return input && Number.isSafeInteger(input.appId) && input.appId > 0 && ['iphone', 'ipad'].includes(input.platform) &&
    (input.versionId === undefined || /^\d{1,24}$/.test(input.versionId));
}
class AppService {
  constructor({ auth, executable, directory, defaultDownloadDirectory, createClient, fetchVersionProvider = fetchProvider, onUpdate = () => {} }) {
    this.fetchVersionProvider = fetchVersionProvider; this.auth = auth; this.executable = executable; this.directory = directory; this.onUpdate = onUpdate;
    this.createClient = createClient || (() => new BridgeClient(executable));
    this.workers = new Map(); this.publicClients = new Set(); this.directoryChoices = new Map(); this.closed = false; this.jobs = []; this.nextRun = new Map();
    this.settingsFile = path.join(directory, 'download-settings.json');
    this.jobsFile = path.join(directory, 'downloads.json');
    this.settings = { downloadDirectory: defaultDownloadDirectory, parallel: 2 };
    try {
      const saved = JSON.parse(fs.readFileSync(this.settingsFile, 'utf8'));
      if (typeof saved.downloadDirectory === 'string' && path.isAbsolute(saved.downloadDirectory)) this.settings.downloadDirectory = saved.downloadDirectory;
      if ([1, 2, 3, 5].includes(saved.parallel)) this.settings.parallel = saved.parallel;
    } catch {}
    try {
      const saved = JSON.parse(fs.readFileSync(this.jobsFile, 'utf8'));
      if (Array.isArray(saved)) this.jobs = saved.filter(j => validSelection(j) && /^[a-f0-9-]{36}$/.test(j.id) && this.validPaths(j)).map(j => ({ ...j, status: j.status === 'completed' && fs.existsSync(j.file) ? 'completed' : 'paused', error: '', speed: 0 }));
    } catch {}
  }
  validPaths(job) {
    return typeof job.folder === 'string' && path.isAbsolute(job.folder) &&
      path.dirname(job.file || '') === job.folder && path.dirname(job.staging || '') === job.folder &&
      path.basename(job.staging || '') === `.qingzhou-${job.id}.staging` && path.extname(job.file || '').toLowerCase() === '.ipa';
  }
  snapshot() { return { ok: true, jobs: this.jobs.map(({ owner, ...job }) => job), settings: { ...this.settings } }; }
  persist() {
    fs.writeFileSync(this.jobsFile + '.tmp', JSON.stringify(this.jobs, null, 2));
    fs.renameSync(this.jobsFile + '.tmp', this.jobsFile);
  }
  update(persist = false) {
    if (persist) { try { this.persist(); } catch {} }
    this.onUpdate(this.snapshot());
  }
  async signedRequest(input) {
    await this.auth.initialize();
    if (!this.auth.account || this.auth.busy || this.auth.pending) return { ok: false, errorCode: 'login', error: '请先完成 Apple 账户登录。' };
    try { return await this.auth.bridge.request({ ...input, guid: this.auth.guid }); }
    catch { return { ok: false, error: 'App Store 请求失败，请检查网络；若会话已失效，请重新登录。' }; }
  }
  async search(input) {
    if (typeof input?.query !== 'string' || !input.query.trim() || input.query.length > 400 || !REGIONS.some(([code]) => code === input.country) || !['iphone', 'ipad'].includes(input.platform)) return { ok: false, error: '请输入应用名称或 App ID，并选择设备和地区。' };
    let country = input.country;
    if (country === 'account') {
      await this.auth.initialize();
      country = resolveAccountCountry(this.auth.account?.storefront);
      if (!country) return { ok: false, error: this.auth.account ? '无法识别账户地区，请手动选择。' : '请先登录账户，或手动选择地区。' };
    }
    // Separate public-catalog worker: searching works without login and never blocks an authentication challenge.
    const client = this.createClient();
    this.publicClients.add(client);
    try { return await client.request({ action: 'search', query: input.query.trim(), country, platform: input.platform, guid: this.auth.guid }, { timeout: 45000 }); }
    catch { return { ok: false, error: '无法连接 Apple 应用目录，请检查网络后重试。' }; }
    finally { this.publicClients.delete(client); client.stop(); }
  }
  async versions(input) {
    if (!validSelection(input)) return { ok: false, error: '应用或设备参数无效。' };
    const source = input.source || 'auto';
    if (!['auto', 'combined', 'timbrd', 'bilin', 'agzy', 'apple', 'apple-combined', 'download', 'compat', 'redownload'].includes(source)) return { ok: false, error: '查询通道无效。' };
    await this.auth.initialize();
    if (!this.auth.account || this.auth.busy || this.auth.pending) return { ok: false, errorCode: 'login', error: '请先完成 Apple 账户登录。' };
    const apple = async () => {
      const result = await this.signedRequest({ action: 'versions', source: source === 'apple-combined' ? 'combined' : ['download', 'compat', 'redownload'].includes(source) ? source : 'auto', appId: input.appId, platform: input.platform, acquireLicense: input.acquireLicense === true });
      if (!result.ok) return { result, diagnostic: { source: 'apple', ok: false, count: 0, error: result.error } };
      const ids = [...new Set([result.data.latestId, ...(result.data.ids || [])].filter(id => /^[1-9][0-9]{0,23}$/.test(String(id))).map(String))];
      return { result: { ...result, data: { ...result.data, ids } }, diagnostic: { source: 'apple', ok: true, count: ids.length } };
    };
    const third = async name => {
      try { const records = await this.fetchVersionProvider(name, input.appId); return { records, diagnostic: { source: name, ok: true, count: records.length } }; }
      catch (error) { const reason = /HTTP [0-9]{3}|连接超时|数据格式不匹配|无效数据|响应过大/.exec(error.message || '')?.[0]; return { records: [], diagnostic: { source: name, ok: false, count: 0, error: reason || '服务暂时不可用，请检查网络后重试' } }; }
    };
    if (!['auto', 'combined', 'timbrd', 'bilin', 'agzy'].includes(source)) return (await apple()).result;
    if (source === 'combined') {
      const [official, ...external] = await Promise.all([apple(), ...['timbrd', 'bilin', 'agzy'].map(third)]);
      const records = mergeRecords(external.map(r => r.records));
      const ids = [...new Set([...(official.result.ok ? official.result.data.ids : []), ...records.map(r => r.id)])];
      const sources = [...external.map(r => r.diagnostic), official.diagnostic];
      return ids.length ? { ok: true, data: { ids, latestId: official.result.ok ? official.result.data.latestId : '', records, sources } } : { ok: false, error: '各版本源均未返回可用记录，请切换来源重试。', data: { sources } };
    }
    const sources = [];
    for (const name of source === 'auto' ? ['timbrd', 'bilin', 'agzy'] : [source]) {
      const value = await third(name); sources.push(value.diagnostic);
      if (value.records.length) return { ok: true, data: { ids: value.records.map(r => r.id), latestId: '', records: value.records, sources } };
    }
    if (source === 'auto') { const value = await apple(); sources.push(value.diagnostic); return { ...value.result, data: { ...(value.result.data || {}), sources } }; }
    return { ok: false, error: '此版本源未返回可用记录，请尝试其他来源或合并查询。', data: { sources } };
  }

  async metadata(input) {
    if (!validSelection(input) || !input.versionId) return { ok: false, error: '请选择有效版本。' };
    return this.signedRequest({ action: 'metadata', appId: input.appId, platform: input.platform, versionId: input.versionId });
  }
  async enqueue(input) {
    if (!validSelection(input) || !input.versionId) return { ok: false, error: '请选择应用版本。' };
    if (input.directoryToken !== undefined && !this.directoryChoices.has(input.directoryToken)) return { ok: false, error: '保存目录已失效，请重新选择。' };
    await this.auth.initialize();
    if (!this.auth.account) return { ok: false, error: '请先登录 Apple 账户。' };
    if (this.jobs.filter(j => !['completed', 'failed', 'paused'].includes(j.status)).length >= 30) return { ok: false, error: '下载队列已满，请等待部分任务完成。' };
    const owner = crypto.createHash('sha256').update(this.auth.account.email.toLowerCase()).digest('hex');
    if (this.jobs.some(j => j.appId === input.appId && j.platform === input.platform && j.versionId === input.versionId && !['completed', 'failed', 'paused'].includes(j.status))) return { ok: false, error: '该版本已在下载队列中。' };
    const id = crypto.randomUUID(); const folder = path.resolve(input.directoryToken ? this.directoryChoices.get(input.directoryToken) : this.settings.downloadDirectory);
    try { fs.mkdirSync(folder, { recursive: true }); } catch { return { ok: false, error: '无法创建下载目录，请在偏好设置中更换目录。' }; }
    const file = path.join(folder, `${input.appId}_${input.platform}_${input.versionId}_${id.slice(0, 8)}.ipa`);
    this.jobs.unshift({ id, appId: input.appId, platform: input.platform, versionId: input.versionId, name: String(input.name || input.appId).slice(0, 200), version: String(input.version || '').slice(0, 80), folder, file, staging: path.join(folder, `.qingzhou-${id}.staging`), owner, status: 'queued', percent: 0, bytes: 0, total: 0, speed: 0, error: '', createdAt: new Date().toISOString(), acquireLicense: input.acquireLicense === true });
    this.update(true); this.pump(); return { ok: true, id };
  }
  pump() {
    if (this.closed || !this.auth.account) return;
    while (this.workers.size < this.settings.parallel) {
      const job = this.jobs.find(j => j.status === 'queued' && !this.workers.has(j.id));
      if (!job) break;
      const worker = this.createClient(); this.workers.set(job.id, worker);
      this.run(job, worker);
    }
  }
  async run(job, worker) {
    const runId = crypto.randomUUID(); this.nextRun.set(job.id, runId);
    const current = () => this.nextRun.get(job.id) === runId;
    try {
      const owner = crypto.createHash('sha256').update(this.auth.account.email.toLowerCase()).digest('hex');
      if (owner !== job.owner) throw Error('此任务属于另一个 Apple 账户，请登录原账户后继续。');
      job.status = 'preparing'; job.error = ''; this.update(true);
      const exported = await this.signedRequest({ action: 'export' });
      if (!current()) return;
      if (!exported.ok || !exported.session) throw Error(exported.error || '无法读取当前登录会话，请重新登录。');
      const restored = await worker.request({ action: 'restore', guid: this.auth.guid, session: exported.session });
      if (!current()) return;
      if (!restored.ok) throw Error('无法准备下载会话，请重新登录。');
      job.status = 'downloading'; this.update();
      const result = await worker.request({ action: 'download', guid: this.auth.guid, appId: job.appId, platform: job.platform, versionId: job.versionId, outputPath: job.staging, acquireLicense: job.acquireLicense }, {
        timeout: 86400000,
        onProgress: progress => {
          if (!current()) return;
          job.bytes = Math.max(0, Number(progress.bytes) || 0); job.total = Math.max(0, Number(progress.total) || 0);
          job.percent = Math.max(0, Math.min(0.999, Number(progress.percent) || 0)); job.speed = Math.max(0, Number(progress.speed) || 0);
          job.status = Number(progress.percent) >= 1 ? 'processing' : 'downloading'; this.update();
        }
      });
      if (!current()) return;
      if (!result.ok) { job.errorCode = result.errorCode; throw Error(result.error || '下载未完成。'); }
      if (path.resolve(result.data?.path || '') !== job.staging) throw Error('下载结果路径无效。');
      const stat = fs.statSync(job.staging); if (stat.size <= 0) throw Error('下载文件为空。');
      fs.renameSync(job.staging, job.file);
      job.status = 'completed'; job.percent = 1; job.bytes = stat.size; job.total = stat.size; job.speed = 0; job.completedAt = new Date().toISOString();
      this.update(true);
    } catch (error) {
      if (current()) { job.status = 'failed'; job.error = error.message || '下载失败，请重试。'; job.speed = 0; this.update(true); }
    } finally {
      await worker.stop();
      if (this.workers.get(job.id) === worker) this.workers.delete(job.id);
      this.pump();
    }
  }
  async control(input) {
    const job = this.jobs.find(j => j.id === input?.id);
    if (!job || !['pause', 'resume', 'remove'].includes(input.action)) return { ok: false, error: '下载任务不存在或操作无效。' };
    if (input.action === 'pause' || input.action === 'remove') {
      this.nextRun.delete(job.id); const worker = this.workers.get(job.id);
      job.status = job.status === 'completed' ? 'completed' : 'paused'; job.speed = 0;
      if (worker) { await worker.stop(); if (this.workers.get(job.id) === worker) this.workers.delete(job.id); }
      if (input.action === 'remove') this.jobs = this.jobs.filter(j => j.id !== job.id);
      // Removing a record deliberately preserves downloaded files and partial files.
      this.update(true); this.pump(); return { ok: true };
    }
    if (job.status === 'completed') return { ok: false, error: '任务已完成。' };
    if (!['paused', 'failed'].includes(job.status)) return { ok: false, error: '该任务正在运行。' };
    if (!this.auth.account) return { ok: false, error: '请先登录原 Apple 账户。' };
    job.status = 'queued'; job.error = ''; this.update(true); this.pump(); return { ok: true };
  }
  saveSettings(input) {
    if (![1, 2, 3, 5].includes(input?.parallel)) return { ok: false, error: '同时下载数量无效。' };
    this.settings.parallel = input.parallel;
    fs.writeFileSync(this.settingsFile, JSON.stringify(this.settings, null, 2)); this.update(); this.pump(); return this.snapshot();
  }
  setDownloadDirectory(folder) {
    if (typeof folder !== 'string' || !path.isAbsolute(folder)) return { ok: false, error: '下载目录无效。' };
    this.settings.downloadDirectory = path.resolve(folder); return this.saveSettings({ parallel: this.settings.parallel });
  }
  selectTaskDirectory(folder) {
    if (typeof folder !== 'string' || !path.isAbsolute(folder)) return { ok: false, error: '保存目录无效。' };
    const directoryToken = crypto.randomUUID();
    this.directoryChoices.set(directoryToken, path.resolve(folder));
    if (this.directoryChoices.size > 64) this.directoryChoices.delete(this.directoryChoices.keys().next().value);
    return { ok: true, directory: path.resolve(folder), directoryToken };
  }
  async pauseAll() {
    for (const job of [...this.jobs]) if (['queued', 'preparing', 'downloading', 'processing'].includes(job.status)) await this.control({ id: job.id, action: 'pause' });
  }
  close() { this.closed = true; this.nextRun.clear(); for (const client of this.publicClients) client.stop(); for (const worker of this.workers.values()) worker.stop(); for (const j of this.jobs) if (['queued', 'preparing', 'downloading', 'processing'].includes(j.status)) j.status = 'paused'; this.update(true); }
}
module.exports = { AppService, validSelection };
