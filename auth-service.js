const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const { spawn } = require('node:child_process');

class BridgeClient {
  constructor(executable) { this.executable = executable; this.child = null; this.nextId = 0; this.pending = new Map(); }
  start() {
    if (this.child) return;
    const child = spawn(this.executable, [], { windowsHide: true, stdio: ['pipe', 'pipe', 'pipe'] });
    this.child = child;
    let buffer = '';
    child.stderr.resume(); // Never retain backend output that might contain authentication details.
    child.stdout.setEncoding('utf8');
    child.stdout.on('data', chunk => {
      buffer += chunk;
      if (buffer.length > 8 * 1024 * 1024) { this.stop('认证服务返回了过大的响应。'); return; }
      let newline;
      while ((newline = buffer.indexOf('\n')) >= 0) {
        const line = buffer.slice(0, newline); buffer = buffer.slice(newline + 1);
        if (!line.trim()) continue;
        let result;
        try { result = JSON.parse(line); } catch { this.stop('认证服务响应无效。'); return; }
        const request = this.pending.get(result.id);
        if (request && result.event === 'progress') { request.onProgress?.(result.data); continue; }
        if (request) { clearTimeout(request.timer); this.pending.delete(result.id); request.resolve(result); }
      }
    });
    const ended = () => {
      if (this.child !== child) return;
      this.child = null;
      this.rejectAll('本机认证服务已停止，请重新登录。');
    };
    child.on('error', ended); child.on('exit', ended);
    child.stdin.on('error', () => {});
  }
  request(input, options = {}) {
    this.start();
    return new Promise((resolve, reject) => {
      const id = ++this.nextId;
      const timer = setTimeout(() => this.stop('请求等待超时，请检查网络后重试。'), options.timeout || 240000);
      this.pending.set(id, { resolve, reject, timer, onProgress: options.onProgress });
      this.child.stdin.write(JSON.stringify({ ...input, id }) + '\n', error => {
        if (error && this.pending.has(id)) this.stop('无法连接本机认证服务。');
      });
    });
  }
  rejectAll(message) {
    for (const request of this.pending.values()) { clearTimeout(request.timer); request.reject(new Error(message)); }
    this.pending.clear();
  }
  stop(message = '登录已取消。') {
    const child = this.child; this.child = null;
    this.rejectAll(message);
    if (child) {
      child.stdin.destroy();
      return new Promise(resolve => {
        const timer = setTimeout(resolve, 2000);
        child.once('exit', () => { clearTimeout(timer); resolve(); });
        child.kill();
      });
    }
    return Promise.resolve();
  }
}

class AuthService {
  constructor({ directory, safeStorage, executable, bridge }) {
    this.directory = directory; this.safeStorage = safeStorage;
    this.bridge = bridge || new BridgeClient(executable);
    this.account = null; this.remembered = false; this.restored = false; this.pending = null;
    this.busy = false; this.generation = 0; this.warning = ''; this.ready = null;
    fs.mkdirSync(directory, { recursive: true });
    this.sessionFile = path.join(directory, 'apple-session.encrypted');
    const identityPath = path.join(directory, 'device-id');
    try { this.guid = fs.readFileSync(identityPath, 'utf8').trim(); } catch {}
    if (!/^[a-f0-9]{12}$/.test(this.guid || '')) {
      const bytes = crypto.randomBytes(6); bytes[0] = 2; this.guid = bytes.toString('hex');
      fs.writeFileSync(identityPath, this.guid, { mode: 0o600 });
    }
  }
  status() {
    return { ok: true, account: this.account, remembered: this.remembered, restored: this.restored,
      busy: this.busy, need2FA: Boolean(this.pending), canRemember: this.safeStorage.isEncryptionAvailable(), warning: this.warning };
  }
  publicAccount(account) {
    if (!account || typeof account.email !== 'string' || !account.email || account.email.length > 254) throw Error('认证服务返回的账户信息无效。');
    return { email: account.email, name: String(account.name || '').slice(0, 200), storefront: String(account.storefront || '').slice(0, 100) };
  }
  async initialize() {
    if (this.ready) return this.ready;
    this.ready = (async () => {
      if (!fs.existsSync(this.sessionFile)) return;
      try {
        if (!this.safeStorage.isEncryptionAvailable()) throw Error();
        if (fs.statSync(this.sessionFile).size > 4 * 1024 * 1024) throw Error();
        const saved = JSON.parse(this.safeStorage.decryptString(fs.readFileSync(this.sessionFile)));
        if (saved.version !== 1 || saved.guid !== this.guid || !saved.session) throw Error();
        const reply = await this.bridge.request({ action: 'restore', guid: this.guid, session: saved.session });
        if (!reply.ok) throw Error();
        this.account = this.publicAccount(reply.account); this.remembered = true; this.restored = true;
      } catch {
        this.account = null; this.warning = '无法恢复上次保存的登录状态，请重新登录。';
        this.bridge.stop();
      }
    })();
    return this.ready;
  }
  async login(input) {
    await this.initialize();
    if (this.busy) return { ok: false, error: '已有登录请求正在进行。' };
    if (this.account) return { ok: false, error: '请先退出当前账户，再切换账号。' };
    if (!input || typeof input.email !== 'string' || typeof input.password !== 'string' ||
      !input.email.trim() || input.email.length > 254 || !input.password || input.password.length > 4096 || typeof input.remember !== 'boolean') {
      return { ok: false, error: '请输入 Apple 账号和密码。' };
    }
    this.clearPending();
    this.bridge.stop();
    return this.authenticate({ email: input.email.trim(), password: input.password, remember: input.remember }, '');
  }
  async submitCode(input) {
    if (this.busy) return { ok: false, error: '已有验证请求正在进行。' };
    if (!this.pending || Date.now() > this.pending.expires) { this.clearPending(); return { ok: false, restart: true, error: '验证等待已过期，请重新输入账号和密码。' }; }
    const code = typeof input?.code === 'string' ? input.code.replace(/\s/g, '') : '';
    if (!/^\d{6}$/.test(code)) return { ok: false, need2FA: true, error: '请输入六位数字验证码。' };
    const pending = { ...this.pending }; this.clearPending();
    return this.authenticate(pending, code);
  }
  async authenticate(credentials, code) {
    this.busy = true; this.warning = ''; const generation = this.generation;
    try {
      const reply = await this.bridge.request({ action: 'login', guid: this.guid, email: credentials.email, password: credentials.password, code });
      if (generation !== this.generation) return { ok: false, cancelled: true };
      if (reply.need2FA) {
        this.pending = { ...credentials, expires: Date.now() + 300000 };
        this.pendingTimer = setTimeout(() => this.clearPending(), 300000);
        return { ok: false, need2FA: true, message: '请在受信任的 Apple 设备上允许登录，并输入收到的六位验证码。' };
      }
      if (!reply.ok || !reply.session) return { ok: false, restart: Boolean(code), error: reply.error || 'Apple 未完成认证，请重试。' };
      const account = this.publicAccount(reply.account);
      this.remembered = false; this.restored = false;
      // Enforce the no-password persistence rule again at the process boundary.
      if (reply.session.account) delete reply.session.account.password;
      try {
        this.deleteSaved();
        if (credentials.remember && this.safeStorage.isEncryptionAvailable()) {
          const encrypted = this.safeStorage.encryptString(JSON.stringify({ version: 1, guid: this.guid, session: reply.session }));
          const temporary = this.sessionFile + '.tmp';
          try { fs.writeFileSync(temporary, encrypted, { mode: 0o600 }); fs.renameSync(temporary, this.sessionFile); }
          finally { if (fs.existsSync(temporary)) fs.unlinkSync(temporary); }
          this.remembered = true;
        } else if (credentials.remember) this.warning = '系统加密存储不可用，本次登录仅保留到退出程序。';
      } catch { this.warning = '已登录，但无法保存登录状态；退出程序后需要重新登录。'; }
      this.account = account;
      return this.status();
    } catch (error) {
      if (generation !== this.generation) return { ok: false, cancelled: true };
      return { ok: false, restart: Boolean(code), error: error.message || '本机认证服务不可用。' };
    } finally { credentials.password = ''; if (generation === this.generation) this.busy = false; }
  }
  clearPending() { clearTimeout(this.pendingTimer); if (this.pending) this.pending.password = ''; this.pending = null; }
  cancel() {
    const hasAuthenticationRequest = this.busy || Boolean(this.pending) || !this.account;
    this.generation++; this.clearPending();
    if (hasAuthenticationRequest) this.bridge.stop();
    this.busy = false; return { ok: true };
  }
  deleteSaved() { if (fs.existsSync(this.sessionFile)) fs.unlinkSync(this.sessionFile); }
  async logout() {
    await this.initialize();
    this.cancel(); this.bridge.stop(); this.account = null; this.remembered = false; this.restored = false; this.warning = '';
    try { this.deleteSaved(); return this.status(); }
    catch { this.warning = '已退出本次登录，但无法删除本机保存的登录状态。请检查应用数据目录权限。'; return { ...this.status(), ok: false, error: this.warning }; }
  }
  close() { this.cancel(); this.bridge.stop(); }
}
module.exports = { AuthService, BridgeClient };
