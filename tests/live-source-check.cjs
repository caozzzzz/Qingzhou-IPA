const {app}=require('electron');
const path=require('node:path'),fs=require('node:fs'),os=require('node:os');
const arg=process.argv.find(v=>v.startsWith('--root='));const root=arg?arg.slice(7):path.resolve(__dirname,'..');
app.setPath('userData',path.join(os.tmpdir(),`qingzhou-live-sources-${process.pid}`));
app.whenReady().then(async()=>{const {AppService}=require(path.join(root,'app-service.js'));const dir=app.getPath('userData');fs.mkdirSync(dir,{recursive:true});
const auth={initialize:async()=>{},account:{email:'fixture@example.invalid'},guid:'fixture',bridge:{request:async()=>({ok:false,error:'Apple 未在本测试登录'})}};
const service=new AppService({auth,directory:dir,defaultDownloadDirectory:dir});
try{const result=await service.versions({appId:932747118,platform:'iphone',source:'combined'});if(!result.ok||result.data.records.length<1)throw Error('No public history returned');const report={passed:true,appId:932747118,count:result.data.ids.length,sources:result.data.sources,first:result.data.records[0],appleAuthenticated:false};fs.writeFileSync(path.resolve(__dirname,'../../v08-electron-live-sources.json'),JSON.stringify(report,null,2));console.log(JSON.stringify(report));service.close();app.exit(0)}catch(e){console.error(String(e));service.close();app.exit(1)}});
