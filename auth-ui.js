(() => {
  const api = window.qingzhouAuth;
  const dialog = document.querySelector('#accountDialog');
  const body = document.querySelector('#accountBody');
  const header = document.querySelector('#account');
  const footer = document.querySelector('#connection');
  let state = { account: null, canRemember: false }, phase = 'login', busy = false, epoch = 0;
  const escape = value => String(value || '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  function updateStatus() {
    window.dispatchEvent(new CustomEvent('qingzhou-account-changed', { detail: { account: state.account } }));
    header.textContent = state.account ? `账户 · ${state.account.name || state.account.email}` : '账户 · 未登录';
    header.title = state.account ? state.account.email : '登录 Apple 账户';
    footer.textContent = state.account ? `${state.restored ? '已恢复登录' : '已登录'}` : '未登录';
  }
  function message(text, error = false) {
    const element = document.querySelector('#authMessage');
    if (element) { element.textContent = text; element.className = `auth-message${error ? ' error' : ''}`; }
  }
  function setBusy(value) {
    busy = value;
    dialog.setAttribute('aria-busy', String(value));
    body.querySelectorAll('input,button[type="submit"]').forEach(element => element.disabled = value);
    const submit = document.querySelector('#authSubmit');
    if (submit) submit.innerHTML = value ? '<span class="spinner" aria-hidden="true"></span>正在连接 Apple…' : phase === 'code' ? '验证并登录' : '登录 App Store';
  }
  function draw() {
    if (state.account) {
      body.innerHTML = `<span class="pill">${state.restored ? '已恢复登录状态' : '已登录'}</span><h2 style="margin-top:17px">${escape(state.account.name || 'Apple 账户')}</h2><p class="muted">${escape(state.account.email)}</p><div class="notice">${state.restored ? '已恢复保存的登录状态；过期后需重新登录。' : '已登录 App Store。'}<br>${state.remembered ? '已在此电脑加密保存登录状态。' : '登录状态仅保留到退出程序。'}</div><p id="authMessage" role="status" aria-live="polite" class="auth-message">${escape(state.warning)}</p><button id="authLogout" class="full">退出登录并清除本机保存</button>`;
      document.querySelector('#authLogout').onclick = async () => {
        const button = document.querySelector('#authLogout'); button.disabled = true; button.innerHTML = '<span class="spinner" aria-hidden="true"></span>正在退出…';
        try { state = await api.logout(); phase = 'login'; updateStatus(); draw(); if (!state.ok) message(state.error, true); }
        catch { message('退出登录失败，请重试。', true); button.disabled = false; button.textContent = '退出登录并清除本机保存'; }
      };
      return;
    }
    const codePhase = phase === 'code';
    body.innerHTML = `<h2>${codePhase ? '输入双重认证验证码' : '登录 Apple 账户'}</h2><p class="muted">${codePhase ? '在受信任的 Apple 设备上允许登录，查看六位验证码。' : '使用可正常访问 App Store 的账户。'}</p><form id="authForm">${codePhase ? '<label class="field">六位验证码<input id="authCode" type="text" inputmode="numeric" autocomplete="one-time-code" pattern="[0-9]{6}" maxlength="6" required placeholder="000000"></label>' : '<label class="field">Apple 账号<input id="authEmail" type="text" autocomplete="username" maxlength="254" required placeholder="邮箱地址或手机号码" spellcheck="false"></label><label class="field">密码<input id="authPassword" type="password" autocomplete="current-password" maxlength="4096" required placeholder="Apple 账户密码"></label><label class="remember"><input id="authRemember" type="checkbox" ' + (state.canRemember ? '' : 'disabled') + '> 在此电脑记住登录状态</label><p class="small muted">' + (state.canRemember ? '加密保存登录状态，不保存密码。' : '系统加密不可用，本次登录仅保留到退出程序。') + '</p>'}<div id="authMessage" role="status" aria-live="polite" class="auth-message">${escape(codePhase ? '验证码等待有效期为 5 分钟。' : state.warning)}</div><button id="authSubmit" type="submit" class="primary full">${codePhase ? '验证并登录' : '登录 App Store'}</button></form><button id="authCancel" class="full" style="margin-top:10px">${codePhase ? '取消并返回' : '取消'}</button><div class="small muted" style="margin-top:15px">查询版本及下载按账户地区进行。</div>`;
    document.querySelector('#authForm').onsubmit = async event => {
      event.preventDefault(); if (busy) return;
      const token = ++epoch;
      let promise;
      if (phase === 'code') {
        const input = document.querySelector('#authCode'); const code = input.value.trim();
        if (!/^\d{6}$/.test(code)) { message('请输入六位数字验证码。', true); return; }
        promise = api.submitCode(code); input.value = '';
      } else {
        const email = document.querySelector('#authEmail').value.trim();
        const passwordInput = document.querySelector('#authPassword');
        promise = api.login({ email, password: passwordInput.value, remember: document.querySelector('#authRemember').checked });
        passwordInput.value = '';
      }
      setBusy(true); message('正在登录，首次连接可能需要几分钟。');
      try {
        const result = await promise;
        if (epoch !== token || result.cancelled) return;
        setBusy(false);
        if (result.need2FA) { phase = 'code'; draw(); message(result.error || result.message || '请输入六位验证码。', Boolean(result.error)); document.querySelector('#authCode').focus(); }
        else if (result.ok && result.account) { state = result; phase = 'login'; updateStatus(); draw(); }
        else { if (result.restart) { phase = 'login'; draw(); } message(result.error || '未能完成登录，请重试。', true); }
      } catch {
        if (epoch !== token) return;
        setBusy(false); message('无法连接本机认证服务，请重启程序后重试。', true);
      }
    };
    document.querySelector('#authCancel').onclick = cancel;
  }
  async function cancel() {
    epoch++; busy = false; phase = 'login';
    await api.cancel().catch(() => {});
    dialog.close(); draw();
  }
  header.onclick = async () => {
    const previousLabel = header.textContent; header.disabled = true; header.innerHTML = '<span class="spinner" aria-hidden="true"></span>读取账户…';
    try { state = await api.status(); updateStatus(); phase = state.need2FA ? 'code' : 'login'; draw(); dialog.showModal(); }
    catch { body.innerHTML = '<h2>认证服务暂不可用</h2><p class="muted">请关闭并重新启动程序。</p>'; dialog.showModal(); }
    finally { header.disabled = false; if (header.querySelector('.spinner')) header.textContent = previousLabel; }
  };
  dialog.querySelector('.close').onclick = cancel;
  dialog.addEventListener('cancel', event => { event.preventDefault(); cancel(); });
  if (api) api.status().then(result => { state = result; updateStatus(); }).catch(() => { footer.textContent = '认证服务暂不可用 · 请重启程序'; });
  else { header.disabled = true; footer.textContent = '请在 Windows 应用中登录'; }
})();
