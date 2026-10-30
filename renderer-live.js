(() => {
  const api = window.qingzhouStore;
  const view = document.querySelector('#view');
  const detail = document.querySelector('#detail');
  const detailBody = document.querySelector('#detailbody');
  const escape = value => String(value ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  const size = bytes => Number(bytes) > 0 ? (bytes >= 1e9 ? `${(bytes / 1e9).toFixed(2)} GB` : `${(bytes / 1e6).toFixed(1)} MB`) : '未提供';
  const date = value => { const d = new Date(value); return value && !Number.isNaN(d.getTime()) && d.getFullYear() > 1980 ? d.toLocaleString('zh-CN', { hour12: false }) : '未提供'; };
  const platformName = value => value === 'ipad' ? 'iPad' : 'iPhone';
  let page = 'search', query = '', country = sessionStorage.getItem('country') || 'account', platform = sessionStorage.getItem('platform') || 'iphone';
  let apps = [], searched = false, searching = false, searchError = '', searchEpoch = 0;
  let snapshot = { jobs: [], settings: { parallel: 2, downloadDirectory: '' } };
  let selectedApp, selectedPlatform, versions = [], latestId = '', selectedId = '', versionPage = 0, versionEpoch = 0, detailEpoch = 0;
  const sourceNames = { auto: '自动（第三方优先）', combined: '合并全部来源', timbrd: 'Timbrd', bilin: 'Bilin', agzy: 'Agzy', apple: 'Apple 自动', 'apple-combined': 'Apple 通道合并', download: 'Apple 下载接口', compat: 'Apple 兼容接口', redownload: 'Apple 重下载接口' };
  let versionSource = 'auto', sourceResults = [], queryToken = 0, priceRegion = '';
  const price = app => { if (typeof app.price !== 'number' || !Number.isFinite(app.price)) return '未提供'; if (app.price === 0) return '免费'; try { return new Intl.NumberFormat('zh-CN', { style: 'currency', currency: app.currency }).format(app.price); } catch { return `${app.price} ${app.currency || '（币种未提供）'}`; } };
  let versionLoading = false, versionError = '', versionErrorCode = '', starting = false;
  let taskFolder = '', taskDirectoryToken, resetDetailScroll = false, choosingDirectory = false;
  const metadata = new Map(); const pageSize = 6;
  const loading = (label, large = false) => `<span class="loading-inline${large ? ' loading-large' : ''}" role="status"><span class="spinner" aria-hidden="true"></span><span>${label}</span></span>`;
  let catalogRecords = new Map();
  const sourceLabel = sources => (sources || []).map(name => sourceNames[name] || name).join(' / ');
  const versionName = id => metadata.get(metaKey(id))?.data?.version || catalogRecords.get(id)?.version || (id === latestId ? selectedApp.version : '') || '版本号待查询';
  const rowDate = m => m?.data?.dateSource === 'third-party' ? `第三方记录 · ${sourceLabel(m.data.sources)}` : m?.data?.dateSource === 'package-file-time' ? '包内文件时间' : '包内发布日期';
  const rowSize = m => size(m?.data?.sizeBytes) === '未提供' ? escape(m?.data?.sizeText || '未提供') : size(m.data.sizeBytes);
  const metaKey = id => `${selectedApp.id}:${selectedPlatform}:${id}`;
  function artwork(app) {
    let allowed = false;
    try { const u = new URL(app.artwork); allowed = u.protocol === 'https:' && u.hostname.endsWith('.mzstatic.com'); } catch {}
    return allowed ? `<img class="icon app-artwork" src="${escape(app.artwork)}" alt="" width="55" height="55" loading="lazy">` : `<div class="icon" style="--c:#285be7" aria-hidden="true">${escape(app.name?.slice(0, 1) || 'A')}</div>`;
  }
  function toast(text) { const el = document.querySelector('#toast'); el.textContent = text; el.style.display = 'block'; clearTimeout(window.toastTimer); window.toastTimer = setTimeout(() => el.style.display = 'none', 4000); }
  function navigate(next) { page = next; render(); }
  function render() {
    const focused = document.activeElement;
    const focusJob = focused?.dataset.jobId, focusAction = focused?.dataset.action;
    document.querySelectorAll('nav button').forEach(b => { b.classList.toggle('active', b.dataset.page === page); b.setAttribute('aria-current', b.dataset.page === page ? 'page' : 'false'); });
    const active = snapshot.jobs.filter(j => ['queued', 'preparing', 'downloading', 'processing'].includes(j.status)).length;
    document.querySelector('#count').textContent = active ? `(${active})` : '';
    if (page === 'search') renderSearch();
    if (page === 'downloads') renderDownloads(false);
    if (page === 'library') renderDownloads(true);
    if (page === 'settings') renderSettings();
    if (focusJob) [...view.querySelectorAll('[data-job-id]')].find(b => b.dataset.jobId === focusJob && b.dataset.action === focusAction)?.focus({ preventScroll: true });
  }
  function renderSearch() {
    view.innerHTML = `<h1>应用搜索</h1><p class="muted">按名称或 App ID 查询应用。</p><form class="search" id="searchForm"><input id="searchQuery" aria-label="应用名称或 App ID" placeholder="应用名称 / App ID，例如 微信、414478124" value="${escape(query)}" maxlength="400" required><select id="searchPlatform" aria-label="设备"><option value="iphone" ${platform === 'iphone' ? 'selected' : ''}>iPhone</option><option value="ipad" ${platform === 'ipad' ? 'selected' : ''}>iPad</option></select><select id="searchCountry" aria-label="商店地区">${window.QingzhouRegions.REGIONS.map(([value, text]) => `<option value="${value}" ${country === value ? 'selected' : ''}>${text}</option>`).join('')}</select><button class="primary" ${searching ? 'disabled' : ''}>${searching ? loading('查询中…') : '搜索应用'}</button></form><div class="sectionbar"><h2>${searching ? '正在查询 Apple…' : searched ? `查询结果 · ${apps.length} 款` : '搜索应用'}</h2><span class="small muted">${platformName(platform)} · App Store</span></div>${searchError ? `<div class="notice error" role="alert">${escape(searchError)}</div>` : ''}<div class="grid" id="cards" aria-busy="${searching}">${searching ? Array.from({length:6}, () => '<article class="loading-card" aria-hidden="true"><div class="skeleton-head"><span class="skeleton skeleton-icon"></span><span class="skeleton skeleton-title"></span></div><span class="skeleton skeleton-line"></span><span class="skeleton skeleton-line"></span><span class="skeleton skeleton-line short"></span><span class="skeleton skeleton-button"></span></article>').join('') : ''}${apps.map((app, i) => `<article class="card"><div class="apphead">${artwork(app)}<div class="appinfo"><div class="appname" title="${escape(app.name)}">${escape(app.name)}</div><div class="small muted" title="${escape(app.developer)}">${escape(app.developer)}</div></div></div><div class="catalog-meta"><span>App ID <strong>${app.id}</strong></span><span>最新版本 <strong>${escape(app.version || '未提供')}</strong></span><span>更新时间 <strong>${date(app.updatedAt)}</strong></span><span>软件大小 <strong>${size(app.sizeBytes)}</strong></span><span>价格 <strong>${escape(price(app))}</strong></span></div><div class="cardfoot"><span class="small muted">${app.platforms.length ? app.platforms.map(platformName).join(' / ') : '设备信息未提供'}${app.minimumOS ? ` · iOS ${escape(app.minimumOS)}+` : ''}</span><button data-app="${i}">选择版本</button></div></article>`).join('')}</div>${!apps.length && !searching && !searchError ? `<div class="empty"><h2>${searched ? '没有找到匹配的应用' : '输入名称或 App ID 开始搜索'}</h2><p class="muted">${searched ? '试试完整应用名称、数字 App ID，或切换商店地区与设备。' : '查询版本及下载需登录 Apple 账户。'}</p></div>` : ''}<div class="notice">更新时间和大小由 App Store 提供。版本与下载按账户地区查询。</div>`;
    document.querySelector('#searchForm').onsubmit = async event => {
      event.preventDefault();
      query = document.querySelector('#searchQuery').value.trim(); country = document.querySelector('#searchCountry').value; platform = document.querySelector('#searchPlatform').value;
      if (!query || searching) return;
      sessionStorage.setItem('country', country); sessionStorage.setItem('platform', platform);
      searching = true; searchError = ''; apps = []; const epoch = ++searchEpoch; render();
      try { const result = await api.search({ query, country, platform }); if (epoch !== searchEpoch) return; if (!result.ok) searchError = result.error; else apps = result.data || []; }
      catch { searchError = '查询失败，请检查网络后重试。'; }
      finally { if (epoch === searchEpoch) { searched = true; searching = false; if (page === 'search') render(); } }
    };
    view.querySelectorAll('[data-app]').forEach(b => b.onclick = () => openApp(apps[Number(b.dataset.app)]));
  }
  function renderDownloads(library) {
    const jobs = library ? snapshot.jobs.filter(j => j.status === 'completed') : snapshot.jobs;
    const labels = { queued: '等待下载', preparing: '准备账户与下载链接', downloading: '正在下载', processing: '校验并整理 IPA', completed: '已完成', failed: '下载失败', paused: '已暂停' };
    view.innerHTML = `<h1>${library ? '我的应用' : '下载队列'}</h1><p class="muted">${library ? '已下载的 IPA 文件。' : '管理下载任务。'}</p>${jobs.length ? jobs.map(j => `<article class="row"><div class="icon" style="--c:#285be7" aria-hidden="true">${escape(j.name.slice(0, 1))}</div><div class="info"><strong>${escape(j.name)}</strong> <span class="small muted">${platformName(j.platform)} · ${escape(j.version || `版本 ID ${j.versionId}`)}</span><div class="small muted">App ID ${j.appId} · 版本 ID ${j.versionId}</div>${j.status !== 'completed' ? `<progress max="1" ${['preparing', 'queued'].includes(j.status) ? '' : `value="${j.percent}"`} aria-label="${escape(j.name)} 下载进度"></progress>` : ''}<div class="small ${j.status === 'failed' ? 'error' : 'muted'}">${['queued','preparing','processing'].includes(j.status) ? loading(labels[j.status]) : labels[j.status]}${j.status === 'downloading' ? ` · ${(j.percent * 100).toFixed(1)}% · ${size(j.bytes)} / ${size(j.total)} · ${size(j.speed)}/s` : j.status === 'completed' ? ` · ${size(j.bytes)}` : ''}</div>${j.error ? `<div class="small error">${escape(j.error)}</div>` : ''}<div class="small muted file-path" title="${escape(j.file)}">${escape(j.file)}</div></div><div class="job-actions">${['queued', 'preparing', 'downloading', 'processing'].includes(j.status) ? `<button data-job-id="${j.id}" data-action="pause">暂停</button>` : ['paused', 'failed'].includes(j.status) ? `<button data-job-id="${j.id}" data-action="resume">${j.status === 'failed' ? '重试' : '继续'}</button>` : `<button data-job-id="${j.id}" data-action="reveal">查看文件</button>`}<button data-job-id="${j.id}" data-action="remove">移除记录</button></div></article>`).join('') : `<div class="empty"><h2>${library ? '还没有已完成的下载' : '还没有下载任务'}</h2><p class="muted">搜索应用并选择版本下载。</p><button class="primary" id="discover">去搜索应用</button></div>`}<div class="notice">移除记录不会删除文件。程序重启后，未完成的任务将暂停。</div>`;
    document.querySelector('#discover')?.addEventListener('click', () => navigate('search'));
    view.querySelectorAll('[data-job-id]').forEach(b => b.onclick = async () => {
      const previousLabel = b.textContent; b.disabled = true; b.innerHTML = loading('处理中…');
      try { const result = b.dataset.action === 'reveal' ? await api.reveal(b.dataset.jobId) : await api.control({ id: b.dataset.jobId, action: b.dataset.action }); if (!result.ok) toast(result.error); }
      catch { toast('任务操作失败，请重试。'); }
      finally { b.disabled = false; b.textContent = previousLabel; }
    });
  }
  function renderSettings() {
    view.innerHTML = `<h1>偏好设置</h1><p class="muted">设置默认保存目录和下载数量。</p><div class="card"><h2>下载设置</h2><div class="field">保存目录<div class="folder-choice"><code>${escape(snapshot.settings.downloadDirectory)}</code><button id="chooseFolder">更换目录</button></div></div><label class="field">同时下载任务数<select id="parallel">${[1, 2, 3, 5].map(n => `<option ${n === snapshot.settings.parallel ? 'selected' : ''}>${n}</option>`).join('')}</select></label><button class="primary" id="saveSettings">保存设置</button><p class="small muted" style="margin-top:16px">更换目录只影响新任务。已有任务继续使用创建时的目录。</p></div>`;
    document.querySelector('#chooseFolder').onclick = async event => {
      const button = event.currentTarget; button.disabled = true; button.innerHTML = loading('选择中…');
      try { const result = await api.chooseDirectory(); if (!result.ok && !result.cancelled) toast(result.error); }
      catch { toast('无法选择保存目录。'); }
      finally { button.disabled = false; button.textContent = '更换目录'; }
    };
    document.querySelector('#saveSettings').onclick = async event => {
      const button = event.currentTarget; button.disabled = true; button.innerHTML = loading('保存中…');
      try { const result = await api.saveSettings({ parallel: Number(document.querySelector('#parallel').value) }); toast(result.ok ? '下载设置已保存' : result.error); }
      catch { toast('保存失败，请重试。'); }
      finally { button.disabled = false; button.textContent = '保存设置'; }
    };
  }
  function openApp(app) {
    versionSource = 'auto'; sourceResults = []; catalogRecords = new Map(); priceRegion = window.QingzhouRegions.REGIONS.find(([value]) => value === country)?.[1] || country; selectedApp = app; selectedPlatform = platform; taskFolder = ''; taskDirectoryToken = undefined; versions = []; selectedId = ''; versionPage = 0; versionError = ''; versionErrorCode = ''; detailEpoch++;
    resetDetailScroll = true; detailBody.innerHTML = ''; detail.showModal(); loadVersions(false);
  }
  async function loadVersions(acquireLicense) {
    const epoch = detailEpoch, token = ++queryToken; versionEpoch++; versions = []; selectedId = ''; versionPage = 0; sourceResults = []; versionLoading = true; versionError = ''; versionErrorCode = ''; renderDetail();
    try {
      const result = await api.versions({ appId: selectedApp.id, platform: selectedPlatform, acquireLicense, source: versionSource });
      if (epoch !== detailEpoch || token !== queryToken) return;
      sourceResults = result.data?.sources || [];
      if (!result.ok) { versionError = result.error; versionErrorCode = result.errorCode || ''; }
      else {
        catalogRecords = new Map((result.data.records || []).map(record => [record.id, record]));
        for (const record of catalogRecords.values()) { const key = metaKey(record.id); if (!metadata.get(key)?.data || metadata.get(key)?.data?.dateSource === 'third-party') metadata.set(key, { data: record }); }
        latestId = String(result.data.latestId || (result.data.records || []).find(r => r.version === selectedApp.version)?.id || '');
        versions = [...new Set([latestId, ...(result.data.ids || []).map(String)].filter(id => /^\d{1,24}$/.test(id)))].sort((a, b) => a === latestId ? -1 : b === latestId ? 1 : BigInt(a) > BigInt(b) ? -1 : 1);
        selectedId = versions[0] || ''; if (!versions.length) versionError = 'Apple 没有返回可用版本。';
      }
    } catch { if (epoch === detailEpoch && token === queryToken) versionError = '版本查询失败，请检查网络后重试。'; }
    finally { if (epoch === detailEpoch && token === queryToken) { versionLoading = false; renderDetail(); hydratePage(); } }
  }
  function renderDetail() {
    if (!selectedApp) return;
    const scrollTop = resetDetailScroll ? 0 : (detailBody.querySelector('.detail-scroll')?.scrollTop || 0);
    resetDetailScroll = false;
    const activeElement = document.activeElement;
    const focusId = detailBody.contains(activeElement) ? activeElement.id : '';
    const focusedVersion = activeElement?.name === 'appVersion' ? activeElement.value : '';
    const pages = Math.max(1, Math.ceil(versions.length / pageSize));
    const pageIds = versions.slice(versionPage * pageSize, (versionPage + 1) * pageSize);
    const header = `<header class="detail-head"><div class="apphead">${artwork(selectedApp)}<div class="detail-title"><h2>${escape(selectedApp.name)}</h2><div class="small muted">App ID ${selectedApp.id} <span class="detail-separator">·</span> ${platformName(selectedPlatform)}</div></div></div><div class="detail-summary"><span>最新版本 <strong>${escape(selectedApp.version || '未提供')}</strong></span><span>更新于 ${date(selectedApp.updatedAt)}</span><span>${size(selectedApp.sizeBytes)}</span><span title="所选查询地区的 App Store 标价">${escape(priceRegion)} · ${escape(price(selectedApp))}</span></div></header>`;
    const sourceControl = `<div class="source-choice"><label for="versionSource">查询通道</label><button type="button" class="source-help-trigger" id="sourceHelpButton" aria-label="查看查询通道说明" aria-haspopup="dialog" aria-controls="sourceHelp" title="查询通道说明">?</button><select id="versionSource" aria-label="版本查询通道">${Object.entries(sourceNames).map(([id, name]) => `<option value="${id}" ${versionSource === id ? 'selected' : ''}>${name}</option>`).join('')}</select></div>`;
    let content = '', controls = '';
    if (versionLoading) {
      content = `<div class="version-loading">${loading('正在读取可用版本…', true)}<p class="small muted">正在查询版本来源，请稍候…</p><div class="version-skeleton" aria-hidden="true">${Array.from({length:4}, () => '<div class="skeleton-version"><span class="skeleton"></span><span class="skeleton"></span><span class="skeleton"></span></div>').join('')}</div></div>`;
    } else if (versionError) {
      content = `<div class="version-error"><p class="small muted">${sourceResults.map(r => `${escape(sourceNames[r.source] || r.source)}：${r.ok ? `${r.count} 个` : escape(r.error)}`).join('；')}</p><div class="notice error" role="alert">${escape(versionError)}</div><div class="error-actions"><button id="retryVersions">重新查询</button>${versionErrorCode === 'license' ? '<button class="primary" id="acquireLicense">获取免费许可并重试</button>' : ''}${versionErrorCode === 'login' ? '<button class="primary" id="loginFromVersion">登录账户</button>' : ''}</div></div>`;
    } else {
      content = `<div class="sectionbar version-section"><h2>选择版本</h2><span class="small muted">共 ${versions.length} 个版本 · 每页 ${pageSize} 条</span></div><p class="source-results small muted">${sourceResults.map(r => `${escape(sourceNames[r.source] || r.source)}：${r.ok ? `${r.count} 个` : escape(r.error || '查询失败')}`).join('；')}</p><div class="versions-table"><table><thead><tr><th>版本</th><th>时间 / 来源</th><th>IPA 大小</th></tr></thead><tbody>${pageIds.map(id => {
        const m = metadata.get(metaKey(id));
        const name = versionName(id);
        const time = m?.data ? `<time>${date(m.data.updatedAt)}</time><small>${escape(rowDate(m))}</small>` : m?.error ? `<button class="retry-metadata" data-meta="${id}" title="${escape(m.error)}">读取失败，重试</button>` : loading('正在读取…');
        return `<tr class="version-row ${id === selectedId ? 'is-selected' : ''}"><td><label class="version-choice"><input type="radio" name="appVersion" value="${id}" ${id === selectedId ? 'checked' : ''}><span><strong class="version-number">${escape(name)}</strong>${id === latestId ? ' <span class="pill">最新</span>' : ''}<small>ID ${id}${catalogRecords.get(id)?.sources?.length ? ` · ${escape(sourceLabel(catalogRecords.get(id).sources))}` : ''}</small></span></label></td><td class="version-time muted">${time}</td><td class="version-size">${m?.data ? rowSize(m) : m?.error ? '—' : '<span class="skeleton size-placeholder" aria-hidden="true"></span>'}</td></tr>`;
      }).join('')}</tbody></table></div><div class="pagination"><button id="previousPage" ${versionPage === 0 ? 'disabled' : ''}>上一页</button><span class="small muted">${versionPage + 1} / ${pages}</span><button id="nextPage" ${versionPage + 1 >= pages ? 'disabled' : ''}>下一页</button></div><p class="detail-note small muted">包内文件时间不等于 App Store 上架时间。第三方历史记录不保证仍可下载；下载按账户地区与许可校验。</p>`;
      controls = `<div class="download-location"><label>保存目录</label><div class="folder-choice"><span id="taskDirectory" title="${escape(taskFolder || snapshot.settings.downloadDirectory)}">${escape(taskFolder || snapshot.settings.downloadDirectory)}</span><button id="chooseTaskFolder" ${starting || choosingDirectory ? 'disabled' : ''}>${choosingDirectory ? loading('选择中…') : '选择目录'}</button></div>${taskDirectoryToken ? '<button class="text-button" id="useDefaultFolder">使用默认目录</button>' : ''}</div><button class="primary full" id="startDownload" ${!selectedId || starting ? 'disabled' : ''}>${starting ? loading('正在加入队列…') : `下载所选 ${platformName(selectedPlatform)} 版本`}</button>${selectedApp.price === 0 ? '<p class="small muted license-note">下载时会为当前账户获取免费许可。</p>' : ''}<div id="downloadError" class="small error" role="alert"></div>`;
    }
    if (!detailBody.querySelector('.detail-head')) {
      detailBody.innerHTML = `${header}<div class="detail-scroll" tabindex="0" aria-label="版本列表"></div><footer class="detail-footer" hidden></footer>`;
    }
    const scroller = detailBody.querySelector('.detail-scroll');
    const renderKey = versionLoading ? 'loading' : versionError ? `error:${versionErrorCode}:${versionError}` : `versions:${versionPage}:${pageIds.join(',')}`;
    if (scroller.dataset.renderKey !== renderKey) {
      scroller.innerHTML = sourceControl + content;
      scroller.dataset.renderKey = renderKey;
      scroller.scrollTop = scrollTop;
    }
    const footer = detailBody.querySelector('.detail-footer');
    footer.hidden = !controls;
    if (controls) footer.innerHTML = controls;
    if (focusId) document.getElementById(focusId)?.focus({preventScroll:true});
    else if (focusedVersion) [...detailBody.querySelectorAll('[name="appVersion"]')].find(input => input.value === focusedVersion)?.focus({preventScroll:true});
    document.querySelector('#sourceHelpButton').onclick = () => { const help = document.querySelector('#sourceHelp'); help.showModal(); help.querySelector('.source-help-scroll').scrollTop = 0; };
    document.querySelector('#versionSource').onchange = event => { versionSource = event.target.value; loadVersions(false); };
    document.querySelector('#retryVersions')?.addEventListener('click', () => loadVersions(false));
    document.querySelector('#acquireLicense')?.addEventListener('click', () => loadVersions(true));
    document.querySelector('#loginFromVersion')?.addEventListener('click', () => document.querySelector('#account').click());
    detailBody.querySelectorAll('[name="appVersion"]').forEach(r => r.onchange = () => { selectedId = r.value; detailBody.querySelectorAll('.version-row').forEach(row => row.classList.toggle('is-selected', Boolean(row.querySelector('input:checked')))); });
    detailBody.querySelectorAll('[data-meta]').forEach(b => b.onclick = () => readMetadata(b.dataset.meta, true));
    for (const [id, delta] of [['previousPage', -1], ['nextPage', 1]]) { const button = document.querySelector(`#${id}`); if (button) button.onclick = () => { versionPage += delta; resetDetailScroll = true; renderDetail(); hydratePage(); }; }
    document.querySelector('#chooseTaskFolder')?.addEventListener('click', async () => {
      const epoch = detailEpoch;
      choosingDirectory = true; renderDetail();
      try {
        const result = await api.chooseDirectory('task');
        if (epoch !== detailEpoch || !detail.open) return;
        if (result.ok) { taskFolder = result.directory; taskDirectoryToken = result.directoryToken; }
        else if (!result.cancelled) toast(result.error || '无法选择保存目录。');
      } catch { toast('无法选择保存目录。'); }
      finally { choosingDirectory = false; if (epoch === detailEpoch && detail.open) renderDetail(); }
    });
    document.querySelector('#useDefaultFolder')?.addEventListener('click', () => { taskFolder = ''; taskDirectoryToken = undefined; renderDetail(); });
    document.querySelector('#startDownload')?.addEventListener('click', async () => {
      if (starting) return;
      starting = true; renderDetail();
      try {
        const result = await api.enqueue({ appId: selectedApp.id, platform: selectedPlatform, versionId: selectedId, name: selectedApp.name, acquireLicense: selectedApp.price === 0, directoryToken: taskDirectoryToken, version: versionName(selectedId) });
        if (result.ok) { detail.close(); navigate('downloads'); toast('已加入下载队列'); }
        else { const el = document.querySelector('#downloadError'); if (el) el.textContent = result.error; }
      } catch { const el = document.querySelector('#downloadError'); if (el) el.textContent = '添加下载失败，请重试。'; }
      finally { starting = false; const b = document.querySelector('#startDownload'); if (b) { b.disabled = false; b.textContent = `下载所选 ${platformName(selectedPlatform)} 版本`; } }
    });
  }
  function patchVersionRow(id) {
    const row = [...detailBody.querySelectorAll('.version-row')].find(element => element.querySelector('[name="appVersion"]')?.value === id);
    if (!row) return;
    const m = metadata.get(metaKey(id));
    const name = versionName(id);
    row.querySelector('.version-number').textContent = name;
    const timeCell = row.querySelector('.version-time');
    if (m?.data) timeCell.innerHTML = `<time>${date(m.data.updatedAt)}</time><small>${escape(rowDate(m))}</small>`;
    else if (m?.error) timeCell.innerHTML = `<button class="retry-metadata" data-meta="${id}" title="${escape(m.error)}">读取失败，重试</button>`;
    else if (!timeCell.querySelector('.spinner')) timeCell.innerHTML = loading('正在读取…');
    row.querySelector('.version-size').innerHTML = m?.data ? rowSize(m) : m?.error ? '—' : '<span class="skeleton size-placeholder" aria-hidden="true"></span>';
    const retryButton = timeCell.querySelector('[data-meta]');
    if (retryButton) retryButton.onclick = () => { row.querySelector('[name="appVersion"]').focus({preventScroll:true}); readMetadata(id, true); };
  }
  async function readMetadata(id, force = false) {
    const key = metaKey(id); if (!force && metadata.has(key)) return;
    const epoch = detailEpoch; const app = selectedApp; const device = selectedPlatform;
    metadata.set(key, { loading: true });
    if (force && detail.open) patchVersionRow(id);
    try { const result = await api.metadata({ appId: app.id, platform: device, versionId: id }); metadata.set(key, result.ok ? { data: result.data } : { error: result.error }); }
    catch { metadata.set(key, { error: '读取失败' }); }
    if (epoch === detailEpoch && detail.open) patchVersionRow(id);
  }
  async function hydratePage() {
    const token = ++versionEpoch; const epoch = detailEpoch;
    for (const id of versions.slice(versionPage * pageSize, (versionPage + 1) * pageSize)) {
      if (token !== versionEpoch || epoch !== detailEpoch || !detail.open) break;
      await readMetadata(id);
    }
  }
  const sourceHelp = document.querySelector('#sourceHelp');
  sourceHelp.querySelectorAll('[data-close-source-help]').forEach(button => button.onclick = () => sourceHelp.close());
  sourceHelp.addEventListener('close', () => { if (detail.open) document.querySelector('#sourceHelpButton')?.focus({ preventScroll: true }); });
  detail.querySelector('.close').onclick = () => detail.close();
  detail.addEventListener('close', () => { detailEpoch++; versionEpoch++; starting = false; choosingDirectory = false; });
  window.addEventListener('qingzhou-account-changed', event => { if (event.detail?.account && detail.open && versionErrorCode === 'login') loadVersions(false); });
  document.querySelectorAll('nav button').forEach(b => b.onclick = () => navigate(b.dataset.page));
  if (api) {
    api.onUpdate(data => { snapshot = data; if (page !== 'search') render(); else document.querySelector('#count').textContent = snapshot.jobs.filter(j => ['queued', 'preparing', 'downloading', 'processing'].includes(j.status)).length || ''; });
    api.state().then(data => { snapshot = data; render(); }).catch(() => toast('无法读取应用设置，请重启程序。'));
  }
  render();
})();
