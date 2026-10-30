// Local fixture-only UI integration. This harness is not included in the application.
const { app, BrowserWindow, ipcMain } = require('electron');
const path = require('node:path');
const fs = require('node:fs');
app.setPath('userData', path.join(app.getPath('temp'), `qingzhou-store-ui-${process.pid}`));
const account = { email: 'ui-test@example.invalid', name: 'UI fixture' };
const settings = { parallel: 2, downloadDirectory: 'C:\\Fixture\\IPA' };
const fixtureApp = { id: 414478124, name: '微信', version: '8.0.0', updatedAt: '2026-10-01T00:00:00Z', sizeBytes: 120000000, platforms: ['iphone', 'ipad'], developer: 'Test developer', minimumOS: '15', artwork: '' };
let win, submitted, jobs = [];
const send = () => win.webContents.send('store:update', { ok: true, jobs, settings });
ipcMain.handle('auth:status', () => ({ ok: true, account, canRemember: true }));
ipcMain.handle('store:state', () => ({ ok: true, jobs, settings }));
ipcMain.handle('store:search', (_e, input) => { if (!['微信', '414478124'].includes(input.query)) throw Error('query mismatch'); return { ok: true, data: [fixtureApp] }; });
ipcMain.handle('store:versions', () => ({ ok: true, data: { ids: ['222', '111'], latestId: '222' } }));
ipcMain.handle('store:metadata', (_e, input) => ({ ok: true, data: { id: input.versionId, version: input.versionId === '111' ? '7.0.0' : '8.0.0', updatedAt: '2026-10-01T00:00:00Z', sizeBytes: 100000000, dateSource: 'package-file-time' } }));
ipcMain.handle('store:directory', (_e, input) => {
  if (input?.scope !== 'task') throw Error('wrong directory scope');
  return { ok: true, directory: 'C:\\Selected Folder\\IPA', directoryToken: 'fixture-directory' };
});
ipcMain.handle('store:enqueue', (_e, input) => {
  submitted = input; jobs = [{ ...input, id: 'fixture-job', status: 'downloading', percent: 0.5, bytes: 500, total: 1000, speed: 100, file: 'C:\\Fixture\\app.ipa' }]; send(); return { ok: true, id: 'fixture-job' };
});
ipcMain.handle('store:control', () => ({ ok: true }));
app.whenReady().then(async () => {
  win = new BrowserWindow({ width: 1240, height: 850, show: false, webPreferences: { preload: path.resolve(__dirname, '../preload.js'), nodeIntegration: false, contextIsolation: true, sandbox: true } });
  try {
    await win.loadFile(path.resolve(__dirname, '../index.html'));
    await win.webContents.executeJavaScript(`(async () => {
      const wait=async test=>{for(let n=0;n<150;n++){if(test())return;await new Promise(r=>setTimeout(r,20))}throw Error('UI condition timed out')};
      const assert=(v,msg)=>{if(!v)throw Error(msg)};
      assert(document.querySelector('h1').textContent==='应用搜索','concise heading');
      assert(document.querySelector('.sidebottom').textContent==='v0.8.1','version only');
      const countries=[...document.querySelector('#searchCountry').options].map(o=>o.value);
      assert(countries.length===26&&countries.includes('account')&&countries.includes('mo')&&countries.includes('za'),'expanded regions');
      document.querySelector('#searchQuery').value='414478124';document.querySelector('#searchPlatform').value='ipad';document.querySelector('#searchForm').requestSubmit();
      await wait(()=>document.querySelector('[data-app]'));
      assert(document.querySelector('#cards').textContent.includes('120.0 MB'),'search size');
      assert(document.querySelector('#cards').textContent.includes('2026'),'search update date');
      document.querySelector('[data-app]').click();
      await wait(()=>document.querySelector('[name="appVersion"][value="111"]')&&document.querySelector('.versions-table').textContent.includes('7.0.0'));
      const old=document.querySelector('[name="appVersion"][value="111"]');old.checked=true;old.dispatchEvent(new Event('change'));
      document.querySelector('#chooseTaskFolder').click();
      await wait(()=>document.querySelector('#taskDirectory')?.textContent.includes('Selected Folder'));
      document.querySelector('#startDownload').click();
      await wait(()=>document.querySelector('[data-action="pause"]'));
      assert(document.querySelector('.row').textContent.includes('7.0.0'),'selected version displayed');
      assert(document.querySelector('.row').textContent.includes('50.0%'),'real progress event');
    })()`);
    if (submitted.appId !== 414478124 || submitted.platform !== 'ipad' || submitted.versionId !== '111' || submitted.directoryToken !== 'fixture-directory') throw Error('selected version or directory was not forwarded');
    jobs[0].status = 'completed'; jobs[0].percent = 1; send();
    await win.webContents.executeJavaScript(`document.querySelector('[data-page="library"]').click()`);
    await new Promise(resolve => setTimeout(resolve, 100));
    const library = await win.webContents.executeJavaScript(`document.querySelector('.row')?.textContent.includes('已完成')`);
    if (!library) throw Error('completed library item missing');
    await win.webContents.executeJavaScript(`document.querySelector('[data-page="search"]').click(); document.querySelector('[data-app]').click();`);
    await new Promise(resolve => setTimeout(resolve, 300));
    const screenshot = await win.webContents.capturePage(); fs.writeFileSync(path.resolve(__dirname, '../../store-version-ui.png'), screenshot.toPNG());
    fs.writeFileSync(path.resolve(__dirname, '../../store-ui-result.json'), JSON.stringify({ passed: true, checks: ['concise copy', '26 region options', 'version only', 'ID search form', 'date and size', 'iPad selection', 'historical version selection', 'task save directory', 'progress', 'completed library'] }, null, 2)); app.exit(0);
  } catch (error) { fs.writeFileSync(path.resolve(__dirname, '../../store-ui-result.json'), JSON.stringify({ passed: false, error: String(error) })); app.exit(1); }
});
