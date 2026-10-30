// Test harness only. Not included in packaged application.
const { app, BrowserWindow, ipcMain } = require('electron');
const path = require('node:path');
const fs = require('node:fs');
app.setPath('userData', path.join(app.getPath('temp'), `qingzhou-ui-check-${process.pid}`));
let account = null, pending = false;
const state = () => ({ ok: true, account, canRemember: true, remembered: false, restored: false });
ipcMain.handle('auth:status', state);
ipcMain.handle('store:state', () => ({ ok: true, jobs: [], settings: { parallel: 2, downloadDirectory: 'C:\\Fixture\\IPA' } }));
ipcMain.handle('auth:login', (_e, input) => {
  if (input.email !== 'ui-test@example.invalid' || input.password !== 'ui-fixture-password') throw Error('test fixture input mismatch');
  pending = true; return { need2FA: true, message: '请输入验证码。' };
});
ipcMain.handle('auth:code', (_e, input) => {
  if (!pending || input.code !== '123456') throw Error('test fixture code mismatch');
  pending = false; account = { name: '界面测试账户', email: 'ui-test@example.invalid' }; return state();
});
ipcMain.handle('auth:cancel', () => { pending = false; return { ok: true }; });
ipcMain.handle('auth:logout', () => { account = null; return state(); });
app.whenReady().then(async () => {
  const win = new BrowserWindow({ show: false, webPreferences: { preload: path.resolve(__dirname, '../preload.js'), nodeIntegration: false, contextIsolation: true, sandbox: true } });
  try {
    await win.loadFile(path.resolve(__dirname, '../index.html'));
    const result = await win.webContents.executeJavaScript(`(async () => {
      const wait = async selector => { for (let i=0;i<100;i++){const e=document.querySelector(selector);if(e)return e;await new Promise(r=>setTimeout(r,20))}throw Error('missing '+selector)};
      const assert=(value,msg)=>{if(!value)throw Error(msg)};
      await window.qingzhouAuth.status(); document.querySelector('#account').click();
      (await wait('#authEmail')).value='ui-test@example.invalid'; document.querySelector('#authPassword').value='ui-fixture-password';
      document.querySelector('#authForm').requestSubmit();
      const code=await wait('#authCode'); code.value='123456'; document.querySelector('#authForm').requestSubmit();
      await wait('#authLogout'); assert(document.querySelector('#account').textContent.includes('界面测试账户'),'header account');
      document.querySelector('#authLogout').click(); await wait('#authEmail');
      assert(document.querySelector('#account').textContent.includes('未登录'),'logout state');
      document.querySelector('#authPassword').value='sensitive-unsent-text';document.querySelector('#authCancel').click();
      await new Promise(r=>setTimeout(r,100));
      assert(!document.querySelector('#accountDialog').open,'cancel close');
      assert(document.querySelector('#authPassword').value==='','cancel clears password');
      return {passed:true,checks:['login form','2FA transition','successful account display','logout','cancel clears fields']};
    })()`);
    fs.writeFileSync(path.resolve(__dirname, '../../auth-ui-result.json'), JSON.stringify(result, null, 2));
    app.exit(0);
  } catch (e) { fs.writeFileSync(path.resolve(__dirname, '../../auth-ui-result.json'), JSON.stringify({ passed: false, error: String(e) })); app.exit(1); }
});
