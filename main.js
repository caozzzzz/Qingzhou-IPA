const { app, BrowserWindow, Menu, ipcMain, safeStorage, dialog, shell, screen } = require('electron');
const path = require('node:path');
const fs = require('node:fs');
const { fileURLToPath } = require('node:url');
const { AuthService } = require('./auth-service');
const { AppService } = require('./app-service');
const smoke = process.argv.includes('--smoke-test');
const report = process.argv.find(a => a.startsWith('--report='))?.slice(9);
let win, auth, store;
app.setName('轻舟 IPA');
app.setPath('userData', smoke ? path.join(app.getPath('temp'), `qingzhou-auth-smoke-${process.pid}`) : path.join(app.getPath('appData'), 'QingzhouIPA'));
if (!app.requestSingleInstanceLock()) app.quit();
else {
  app.on('second-instance', () => { if (win) { if (win.isMinimized()) win.restore(); win.show(); win.focus(); } });
  app.whenReady().then(async () => {
    Menu.setApplicationMenu(null);
    auth = new AuthService({ directory: app.getPath('userData'), safeStorage,
      executable: app.isPackaged ? path.join(process.resourcesPath, 'authbridge', 'qingzhou-auth.exe') : path.join(__dirname, 'bin', 'qingzhou-auth.exe') });
    store = new AppService({ auth, directory: app.getPath('userData'), executable: auth.bridge.executable,
      defaultDownloadDirectory: path.join(app.getPath('downloads'), 'QingzhouIPA'),
      onUpdate: data => { if (win && !win.isDestroyed()) win.webContents.send('store:update', data); } });
    const actions = {
      'auth:status': async () => { await auth.initialize(); return auth.status(); },
      'auth:login': input => auth.login(input), 'auth:code': input => auth.submitCode(input),
      'auth:cancel': () => auth.cancel(), 'auth:logout': async () => { await store.pauseAll(); return auth.logout(); },
      'store:search': input => store.search(input), 'store:versions': input => store.versions(input),
      'store:metadata': input => store.metadata(input), 'store:enqueue': input => store.enqueue(input),
      'store:state': () => store.snapshot(), 'store:control': input => store.control(input),
      'store:settings': input => store.saveSettings(input),
      'store:directory': async input => {
        const result = await dialog.showOpenDialog(win, { title: '选择 IPA 保存目录', defaultPath: store.settings.downloadDirectory, properties: ['openDirectory', 'createDirectory'] });
        return result.canceled ? { ok: false, cancelled: true } : input?.scope === 'task' ? store.selectTaskDirectory(result.filePaths[0]) : store.setDownloadDirectory(result.filePaths[0]);
      },
      'store:reveal': input => {
        const job = store.jobs.find(j => j.id === input?.id);
        if (!job || job.status !== 'completed' || !store.validPaths(job) || !fs.existsSync(job.file)) return { ok: false, error: '已完成的文件不存在。' };
        shell.showItemInFolder(job.file); return { ok: true };
      }
    };
    for (const [channel, handler] of Object.entries(actions)) {
      ipcMain.handle(channel, async (event, input) => {
        let trusted = false;
        try { trusted = event.sender === win?.webContents && event.senderFrame === win.webContents.mainFrame && fileURLToPath(event.senderFrame.url) === path.join(__dirname, 'index.html'); } catch {}
        if (!trusted) return { ok: false, error: '不允许的认证请求来源。' };
        try { return await handler(input); } catch { return { ok: false, error: '认证服务暂时不可用，请重试。' }; }
      });
    }
    const workArea = screen.getPrimaryDisplay().workAreaSize;
    const initialWidth = smoke ? 1240 : Math.min(1240, Math.max(520, workArea.width - 32));
    const initialHeight = smoke ? 850 : Math.min(850, Math.max(420, workArea.height - 32));
    win = new BrowserWindow({ width: initialWidth, height: initialHeight, minWidth: Math.min(760, initialWidth), minHeight: Math.min(520, initialHeight),
      icon: path.join(__dirname, 'icon.ico'), title: `轻舟 IPA · v${app.getVersion()}`, backgroundColor: '#f6f8fc', show: false,
      webPreferences: { preload: path.join(__dirname, 'preload.js'), nodeIntegration: false, contextIsolation: true, sandbox: true, spellcheck: false, offscreen: smoke && process.argv.includes('--screenshot'), backgroundThrottling: !smoke }
    });
    win.webContents.setWindowOpenHandler(() => ({ action: 'deny' }));
    win.webContents.on('will-navigate', event => event.preventDefault());
    win.webContents.session.setPermissionRequestHandler((_wc, _permission, callback) => callback(false));
    const errors = [];
    win.webContents.on('console-message', event => { if (event.level === 'error' || event.level === 3) errors.push('renderer error'); });
    await win.loadFile(path.join(__dirname, 'index.html'));
    if (!smoke) win.show();
    else {
      try {
        const ping = await auth.bridge.request({ action: 'ping' });
        if (!ping.ok) throw Error('real Go authentication bridge unavailable');
        if (!safeStorage.isEncryptionAvailable()) throw Error('Windows encryption unavailable');
        const encrypted = safeStorage.encryptString('qingzhou-encryption-self-check');
        if (safeStorage.decryptString(encrypted) !== 'qingzhou-encryption-self-check') throw Error('encryption roundtrip');
        const result = await win.webContents.executeJavaScript(`(async () => {
          const assert=(value,message)=>{if(!value)throw Error(message)};
          assert(document.querySelector('#searchQuery'),'real search form');
          assert(document.querySelectorAll('.card').length===0,'no demo apps');
          const invalid=await window.qingzhouStore.search({query:'',country:'cn',platform:'iphone'});
          assert(!invalid.ok,'invalid search rejected');
          const versions=await window.qingzhouStore.versions({appId:414478124,platform:'iphone'});
          assert(!versions.ok && versions.errorCode==='login','versions require auth');
          const download=await window.qingzhouStore.enqueue({appId:414478124,platform:'iphone',versionId:'123'});
          assert(!download.ok,'download requires auth');
          document.querySelector('[data-page="settings"]').click();
          assert(document.querySelector('#chooseFolder'),'native folder selector');
          await window.qingzhouAuth.status();document.querySelector('#account').click();
          await new Promise(r=>setTimeout(r,300));
          assert(document.querySelector('#authPassword').type==='password','real auth form');
          document.querySelector('#authCancel').click();await new Promise(r=>setTimeout(r,100));
          return {passed:true,checks:['real search form','no demo data','input validation','version/download login gate','download directory UI','real auth form','Go bridge ping','Windows encryption roundtrip'],nodeIntegration:typeof require};
        })()`);
        if (process.argv.includes('--live-catalog')) {
          const phone = await store.search({ query: '414478124', country: 'cn', platform: 'iphone' });
          const pad = await store.search({ query: '微信', country: 'cn', platform: 'ipad' });
          if (!phone.ok || !phone.data.some(a => a.id === 414478124 && a.sizeBytes > 0 && a.updatedAt)) throw Error('live App ID query failed');
          if (!pad.ok || !pad.data.some(a => a.id === 414478124 && a.platforms.includes('ipad'))) throw Error('live name/iPad search failed');
          result.liveCatalog = {appId:true,name:true,iphone:true,ipad:true,size:true,updateDate:true};
        }
        if (errors.length) throw Error(errors.join('\n'));
        if (report) {
          fs.writeFileSync(report, JSON.stringify(result, null, 2));
          if (process.argv.includes('--screenshot')) {
            if (process.argv.includes('--live-catalog')) {
              await win.webContents.executeJavaScript(`(async()=>{
                document.querySelector('[data-page="search"]').click();
                document.querySelector('#searchQuery').value='微信';
                document.querySelector('#searchCountry').value='cn';
                document.querySelector('#searchForm').requestSubmit();
                for(let i=0;i<500;i++){
                  if(document.querySelector('[data-app]'))return;
                  if(document.querySelector('.notice.error'))throw Error('preview catalog query failed');
                  await new Promise(r=>setTimeout(r,100));
                }
                throw Error('preview query timeout');
              })()`);
            } else {
              await win.webContents.executeJavaScript(`document.querySelector('[data-page="search"]').click(); document.querySelector('#account').click();`);
            }
            win.webContents.invalidate();
            await new Promise(r => setTimeout(r, 300));
            const screenshot = await win.webContents.capturePage();
            fs.writeFileSync(report + '.png', screenshot.toPNG());
          }
        }
        store.close(); auth.close();
        app.exit(0);
      } catch (err) {
        if (report) fs.writeFileSync(report, JSON.stringify({ passed: false, error: String(err), errors }, null, 2));
        store.close(); auth.close(); app.exit(1);
      }
    }
  }).catch(err => { if (report) fs.writeFileSync(report, String(err)); app.exit(1); });
  app.on('window-all-closed', () => app.quit());
  app.on('before-quit', () => { store?.close(); auth?.close(); });
}
