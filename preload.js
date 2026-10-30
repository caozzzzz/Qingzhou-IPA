const { contextBridge, ipcRenderer } = require('electron');
contextBridge.exposeInMainWorld('qingzhouAuth', Object.freeze({
  status: () => ipcRenderer.invoke('auth:status'),
  login: input => ipcRenderer.invoke('auth:login', { email: input.email, password: input.password, remember: input.remember }),
  submitCode: code => ipcRenderer.invoke('auth:code', { code }),
  cancel: () => ipcRenderer.invoke('auth:cancel'),
  logout: () => ipcRenderer.invoke('auth:logout')
}));
contextBridge.exposeInMainWorld('qingzhouStore', Object.freeze({
  search: input => ipcRenderer.invoke('store:search', { query: input.query, country: input.country, platform: input.platform }),
  versions: input => ipcRenderer.invoke('store:versions', { appId: input.appId, platform: input.platform, acquireLicense: input.acquireLicense === true, source: input.source }),
  metadata: input => ipcRenderer.invoke('store:metadata', { appId: input.appId, platform: input.platform, versionId: input.versionId }),
  enqueue: input => ipcRenderer.invoke('store:enqueue', { appId: input.appId, platform: input.platform, versionId: input.versionId, name: input.name, version: input.version, directoryToken: input.directoryToken, acquireLicense: input.acquireLicense === true }),
  state: () => ipcRenderer.invoke('store:state'),
  control: input => ipcRenderer.invoke('store:control', { id: input.id, action: input.action }),
  saveSettings: input => ipcRenderer.invoke('store:settings', { parallel: input.parallel }),
  chooseDirectory: scope => ipcRenderer.invoke('store:directory', { scope: scope === 'task' ? 'task' : 'settings' }),
  reveal: id => ipcRenderer.invoke('store:reveal', { id }),
  onUpdate: listener => { const handler = (_event, data) => listener(data); ipcRenderer.on('store:update', handler); return () => ipcRenderer.removeListener('store:update', handler); }
}));
