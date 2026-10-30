// Fixture-only visual and interaction checks; excluded from the application build.
const { app, BrowserWindow, ipcMain } = require('electron');
const path = require('node:path');
const fs = require('node:fs');
app.setPath('userData', path.join(app.getPath('temp'), `qingzhou-glass-ui-${process.pid}`));
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
const apps = [
  { id: 414478124, name: 'WeChat', developer: 'WeChat' },
  { id: 333206289, name: 'Alipay - Simplify Your Life', developer: 'Alipay (Hangzhou) Technology Co., Ltd.' },
  { id: 444934666, name: 'QQ', developer: 'Tencent Technology (Shenzhen) Company Limited' }
].map(a => ({ ...a, version: '8.0.80', updatedAt: '2026-10-23T01:46:47Z', sizeBytes: 998800000, platforms: ['iphone', 'ipad'], minimumOS: '15', price: 2.99, currency: 'USD', artwork: 'https://is1-ssl.mzstatic.com/test.png' }));
ipcMain.handle('auth:status', () => ({ ok: true, account: { name: 'UI fixture', email: 'fixture@example.invalid' }, canRemember: true }));
ipcMain.handle('store:state', () => ({ ok: true, jobs: [], settings: { parallel: 2, downloadDirectory: 'C:\\Downloads\\QingzhouIPA' } }));
ipcMain.handle('store:search', async () => { await delay(600); return { ok: true, data: apps }; });
let selectedSource;
ipcMain.handle('store:versions', async (_event, input) => { selectedSource=input.source; return (async () => { await delay(600); return { ok: true, data: { ids: ['666', '555', '444', '333', '222', '111'], latestId: '666', sources: [{source: input.source, count: 6, ok: true}] } }; })(); });
ipcMain.handle('store:metadata', async (_event, input) => { await delay(450); return { ok: true, data: { id: input.versionId, version: `8.0.${74 + Number(input.versionId) / 111}`, updatedAt: '2026-10-21T14:47:48Z', sizeBytes: 870700000, dateSource: 'package-file-time' } }; });
app.whenReady().then(async () => {
  const win = new BrowserWindow({ width: 1240, height: 850, show: false, webPreferences: { preload: path.resolve(__dirname, '../preload.js'), nodeIntegration: false, contextIsolation: true, sandbox: true, backgroundThrottling: false, offscreen: true } });
  const report = path.resolve(__dirname, '../../glass-ui-result.json');
  try {
    await win.loadFile(path.resolve(__dirname, '../index.html'));
    const search = await win.webContents.executeJavaScript(`(async () => {
      const wait=async fn=>{for(let n=0;n<200;n++){if(fn())return;await new Promise(r=>setTimeout(r,20))}throw Error('wait timed out')};
      const assert=(v,msg)=>{if(!v)throw Error(msg)};
      document.querySelector('#searchQuery').value='微信';document.querySelector('#searchForm').requestSubmit();
      await wait(()=>document.querySelector('.loading-card'));
      assert(getComputedStyle(document.querySelector('#searchForm .spinner')).animationName==='spin','search spinner');
      await wait(()=>document.querySelectorAll('[data-app]').length===3);
      const tops=selector=>[...document.querySelectorAll(selector)].map(e=>e.getBoundingClientRect().top);
      const aligned=values=>Math.max(...values)-Math.min(...values)<1;
      assert(getComputedStyle(document.querySelector('img.app-artwork')).borderRadius==='23%','artwork mask');
      assert(document.querySelector('.catalog-meta').textContent.includes('2.99'),'paid price missing');
      assert(getComputedStyle(document.querySelector('[data-app]')).color==='rgb(37, 75, 131)','button text contrast');
      assert(aligned(tops('#cards .icon')),'icons not aligned');
      assert(aligned(tops('#cards .catalog-meta')),'metadata not aligned');
      assert(aligned(tops('#cards [data-app]')),'buttons not aligned');
      assert(getComputedStyle(document.querySelector('#cards .card')).backdropFilter.includes('blur'),'card glass blur missing');
      return {icons:tops('#cards .icon'),buttons:tops('#cards [data-app]')};
    })()`);
    await delay(250);
    win.webContents.invalidate(); await delay(120);
    let shot = await win.webContents.capturePage(); fs.writeFileSync(path.resolve(__dirname, '../../glass-search-offscreen.png'), shot.toPNG());
    const detail = await win.webContents.executeJavaScript(`(async()=>{
      const wait=async fn=>{for(let n=0;n<200;n++){if(fn())return;await new Promise(r=>setTimeout(r,20))}throw Error('wait timed out')};
      const assert=(v,msg)=>{if(!v)throw Error(msg)};
      document.querySelector('[data-app]').click();
      await wait(()=>document.querySelector('.version-loading .spinner'));
      assert(getComputedStyle(document.querySelector('.version-loading .spinner')).animationName==='spin','version spinner');
      await wait(()=>document.querySelectorAll('.version-row').length===6);
      assert(document.querySelector('.detail-summary').textContent.includes('2.99'),'detail price missing');
      const source=document.querySelector('#versionSource');source.value='combined';source.dispatchEvent(new Event('change'));
      await wait(()=>document.querySelector('.version-loading'));await wait(()=>document.querySelectorAll('.version-row').length===6);
      assert(document.querySelector('.source-results').textContent.includes('6'),'source count');
      const scroller=document.querySelector('.detail-scroll');
      assert(scroller.scrollHeight>scroller.clientHeight,'version list must scroll');
      const headerY=document.querySelector('.detail-head').getBoundingClientRect().top;
      const footerY=document.querySelector('.detail-footer').getBoundingClientRect().top;
      scroller.scrollTop=180;await new Promise(r=>setTimeout(r,40));
      assert(Math.abs(document.querySelector('.detail-head').getBoundingClientRect().top-headerY)<1,'header moved during scroll');
      assert(Math.abs(document.querySelector('.detail-footer').getBoundingClientRect().top-footerY)<1,'footer moved during scroll');
      const expected=scroller.scrollTop;
      const headerNode=document.querySelector('.detail-head');
      const lastSpinner=[...document.querySelectorAll('.version-time .spinner')].at(-1);
      await new Promise(r=>setTimeout(r,1100));
      assert(headerNode===document.querySelector('.detail-head'),'async metadata must not rebuild header');
      assert(lastSpinner?.isConnected,'pending row animation must not restart when other rows finish');
      assert(Math.abs(document.querySelector('.detail-scroll').scrollTop-expected)<2,'metadata update reset scroll');
      assert(getComputedStyle(document.querySelector('#detail')).overflow==='hidden','dialog must clip rounded corners');
      assert(parseFloat(getComputedStyle(document.querySelector('#detail')).borderRadius)>=24,'rounded dialog');
      return {headerY,footerY,scrollPreserved:expected};
    })()`);
    await win.webContents.executeJavaScript(`(async()=>{for(let n=0;n<200;n++){if(!document.querySelector('.version-time .spinner'))return;await new Promise(r=>setTimeout(r,30))}throw Error('metadata wait')})()`);
    await delay(250);
    win.webContents.invalidate(); await delay(120);
    shot = await win.webContents.capturePage(); fs.writeFileSync(path.resolve(__dirname, '../../glass-detail-offscreen.png'), shot.toPNG());
    fs.writeFileSync(report, JSON.stringify({ passed: true, checks: ['glass card appearance','icons aligned','metadata aligned','buttons aligned','search skeleton/spinner','version spinner','fixed header','fixed download controls','scroll preserved after metadata','rounded clipping'], search, detail }, null, 2));
    app.exit(0);
  } catch (error) { fs.writeFileSync(report, JSON.stringify({ passed: false, error: String(error) })); app.exit(1); }
});
