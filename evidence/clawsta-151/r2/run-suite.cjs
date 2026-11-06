const fs = require('fs');
const path = require('path');
const { spawn, spawnSync } = require('child_process');
const label = process.argv[2];
const from = process.argv[3] || 'typecheck';
if (!['base','head'].includes(label)) throw Error('Expected base or head');
const repo = fs.realpathSync(process.env.R2_REPO || process.cwd());
if (!process.env.R2_NATIVE_RUNTIME) throw Error('Set R2_NATIVE_RUNTIME to the isolated native deployment');
const runtime = fs.realpathSync(process.env.R2_NATIVE_RUNTIME);
const cfg = JSON.parse(fs.readFileSync(path.join(runtime,'deployment.json'),'utf8').replace(/^\uFEFF/,''));
if (cfg.Database !== 'uworkers_clawsta151_r1' || Number(cfg.DatabasePort) !== 15651) throw Error('Expected isolated CLAWSTA-151 database');
const env = {...process.env};
for (const key of Object.keys(env)) if (key.startsWith('MULTICA_') || key.startsWith('OSS_')) delete env[key];
env.DATABASE_URL = `postgresql://${cfg.DatabaseUser}:${cfg.DatabasePassword}@127.0.0.1:${cfg.DatabasePort}/${cfg.Database}?sslmode=disable`;
env.GOMAXPROCS = '4';
env.CGO_ENABLED = '0';
env.GO_TELEMETRY_DISABLED = '1';
const logDir = process.env.R2_LOG_DIR || path.join(runtime,'logs','r2',label);
fs.mkdirSync(logDir,{recursive:true});
const summary = from === 'typecheck' ? {label, commit:spawnSync('git',['rev-parse','HEAD'],{cwd:repo,encoding:'utf8'}).stdout.trim(), node:process.version, database:'same isolated clawsta151_r1 / loopback:15651', steps:[]} : JSON.parse(fs.readFileSync(path.join(logDir,'summary.json'),'utf8'));
if (from !== 'typecheck') summary.steps = summary.steps.filter(step => step.name === 'typecheck');
async function run(name, exe, args, cwd=repo) {
  const file=fs.openSync(path.join(logDir,name+'.log'),'w');
  console.log(label+' '+name+' started');
  const started=Date.now();
  const code=await new Promise((resolve,reject)=>{
    const proc=spawn(exe,args,{cwd,env,windowsHide:true,stdio:['ignore',file,file]});
    proc.on('error',reject);proc.on('exit',resolve);
  });
  fs.closeSync(file);
  const result={name,command:[exe,...args].join(' '),code,seconds:Math.round((Date.now()-started)/1000)};
  summary.steps.push(result);fs.writeFileSync(path.join(logDir,'summary.json'),JSON.stringify(summary,null,2));
  console.log(JSON.stringify(result));
}
(async()=>{
 const turbo=path.join(repo,'node_modules/turbo/bin/turbo');
 if (from === 'typecheck') await run('typecheck',process.execPath,[turbo,'typecheck','--filter=!@multica/mobile','--concurrency=1','--continue=always','--force']);
 await run('lint',process.execPath,[turbo,'lint','--filter=!@multica/mobile','--concurrency=1','--continue=always','--force']);
 await run('frontend',process.execPath,[turbo,'test','--filter=!@multica/mobile','--concurrency=1','--continue=always','--force','--','--maxWorkers=1','--testTimeout=30000','--hookTimeout=30000']);
 await run('migrate','go',['run','./cmd/migrate','up'],path.join(repo,'server'));
 await run('go',process.env.R2_BASH || 'C:/Program Files/Git/bin/bash.exe',['scripts/go-test-with-agent-cli-guard.sh','--','go','-C','server','test','-json','-p','2','-parallel','2','-count=1','./...']);
 console.log(label+' suite complete');
 process.exitCode = summary.steps.some(step => step.code !== 0) ? 1 : 0;
})().catch(error=>{console.error(error.message);process.exitCode=1;});
