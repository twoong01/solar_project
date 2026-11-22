import {execFileSync} from 'node:child_process';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
const backend=path.join(root,'backend');
execFileSync(process.execPath,[path.join(root,'scripts','sync-backend.mjs')],{stdio:'inherit',cwd:root});
const os=(process.env.TARGET_OS||'windows').trim();
if(!['windows','darwin','linux'].includes(os))throw Error('TARGET_OS must be windows, darwin, or linux');
const ext=os==='windows'?'.exe':'';
const filename=os==='windows'?'SolarView.exe':`SolarView_${os}`;
execFileSync('go',['build','-trimpath','-ldflags=-s -w','-o',filename+(!filename.endsWith(ext)?ext:''),'.'],{
 cwd:backend,stdio:'inherit',env:{...process.env,GOOS:os,GOARCH:process.env.TARGET_ARCH||'amd64',CGO_ENABLED:'0'}
});
console.log(`완료: backend/${filename}${!filename.endsWith(ext)?ext:''}`);
