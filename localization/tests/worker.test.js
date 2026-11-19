// Real SQLite and real requests to both worker implementations; no production server.
import { test, expect, afterAll } from 'bun:test';
import { Database } from 'bun:sqlite';
import { mkdtempSync, writeFileSync, mkdirSync, symlinkSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { execFileSync } from 'node:child_process';
import translated from '../../worker/src/index.js';
const root = resolve(import.meta.dir, '../..');
const temp = mkdtempSync(join(tmpdir(), 'desk-buddy-zh-test-'));
mkdirSync(join(temp,'src'));
for (const file of ['index.js','calendar.js','weather.js','demo.js','release.json']) {
  writeFileSync(join(temp,'src',file),execFileSync('git',['show','7245f1a344efd0584246b96f4d0023ce2666e2c3:worker/src/'+file],{cwd:root}));
}
symlinkSync(join(root,'worker/node_modules'),join(temp,'node_modules'));
const original = (await import(join(temp,'src/index.js'))).default;
afterAll(()=>rmSync(temp,{recursive:true,force:true}));
const fixedNow = 1794810600000;
Date.now = () => fixedNow;
const schema = `create table tasks(id integer primary key, profile text not null,text text not null,done integer not null default 0,pinned integer not null default 0,tag text,pos real not null,created real not null,parent_id integer,notes text,done_at real);create table kv(k text primary key,v text not null);`;
const ctx = {waitUntil(p){return p;}};
function environment() {
  const db = new Database(':memory:'); db.exec(schema);
  class Statement {
    constructor(sql,args=[]){this.sql=sql;this.args=args;}
    bind(...args){return new Statement(this.sql,args.map(a=>a===undefined?null:typeof a==='boolean'?(a?1:0):a));}
    runSync(){const r=db.query(this.sql).run(...this.args);return {success:true,meta:{changes:r.changes,last_row_id:Number(r.lastInsertRowid)}};}
    async run(){return this.runSync();}
    async first(col){const r=db.query(this.sql).get(...this.args);return r==null?null:col?r[col]:r;}
    async all(){return {success:true,results:db.query(this.sql).all(...this.args)};}
  }
  return {DB:{prepare(sql){return new Statement(sql);},async batch(stmts){return db.transaction(()=>stmts.map(s=>s.runSync()))();}},KEY:'localization-fixture-only',TZ:'Asia/Shanghai',LOCAL:'1',close(){db.close();}};
}
async function call(w,env,path,method='GET',body,auth=true) {
  const headers=auth?{'x-key':env.KEY}:{};
  if(body && !(body instanceof URLSearchParams))headers['content-type']='application/json';
  return w.fetch(new Request('http://fixture.invalid'+path,{method,headers,body:body?body instanceof URLSearchParams?body:JSON.stringify(body):undefined}),env,ctx);
}
async function response(r){return {status:r.status,headers:[...r.headers],body:await r.text()};}
async function compare(a,b,path,method='GET',body){const x=await response(await call(original,a,path,method,body));const y=await response(await call(translated,b,path,method,body));expect(y).toEqual(x);return x;}

test('Chinese API task lifecycle, subtask grouping, sorting, mode and settings match upstream',async()=>{
 const a=environment(),b=environment();try {
  const steps=[
    ['/api/settings'], ['/api/where'],
    ['/api/tasks','POST',{text:'中文任务 <script>alert(1)</script> & "引号"',profile:'work',notes:'说明\n第二行'}],
    ['/api/tasks','POST',{text:'中文子任务',profile:'work',parent_id:1}],
    ['/api/tasks','POST',{text:'待排序任务',profile:'work'}],
    ['/api/tasks/1','PATCH',{text:'修改后中文任务',notes:'保留 emoji 😀 与简体字'}],
    ['/api/tasks/3/place','POST',{before:1,parent_id:null}],
    ['/api/tasks/1/toggle','POST',{}], ['/api/tasks/3/pin','POST',{}],
    ['/api/settings','POST',{key:'focus_min',value:45}],
    ['/api/settings','POST',{key:'focus_min',value:99}],
    ['/api/kindle/settings','POST',{values:{theme:'dark',rotation:270,style:'garden',font:'serif'}}],
    ['/api/state'], ['/api/desk'], ['/api/kindle/settings'], ['/api/export'],
    ['/api/tasks/clear','POST',{profile:'work',done_only:true}], ['/api/export']
  ];
  for(const step of steps) await compare(a,b,...step);
 }finally{a.close();b.close();}
});

test('Phone form posts keep routes, field names, ret, cookie and task data unchanged',async()=>{
 const a=environment(),b=environment();try {
   await compare(a,b,'/do/add','POST',new URLSearchParams({profile:'work',text:'手机中文输入，保留任务内容',ret:'/app?list=work'}));
   await compare(a,b,'/do/notes/1','POST',new URLSearchParams({notes:'手机输入说明 <>&"',ret:'/app?list=work'}));
   await compare(a,b,'/do/toggle/1','POST',new URLSearchParams({ret:'/app?list=work'}));
   await compare(a,b,'/api/export');
   const data=JSON.parse((await response(await call(translated,b,'/api/state'))).body);
   expect(data.tasks.work[0].text).toBe('手机中文输入，保留任务内容');
   expect(data.tasks.work[0].notes).toBe('手机输入说明 <>&"');
 }finally{a.close();b.close();}
});

test('Phone labels translate while user text stays escaped and markup interactions match',async()=>{
 const a=environment(),b=environment();try {
   const payload='Calendar <script>alert("测试")</script> & 中文';
   await compare(a,b,'/api/tasks','POST',{text:payload,notes:'Task & Calendar',profile:'work'});
   const en=await (await call(original,a,'/app?key=localization-fixture-only')).text();
   const zhRes=await call(translated,b,'/app?key=localization-fixture-only');
   const zh=await zhRes.text();
   expect(zhRes.status).toBe(200);expect(zh).toContain('lang="zh-Hans"');expect(zh).toContain('placeholder="添加任务"');
   expect(zh).toContain('Calendar &lt;script&gt;alert(&quot;测试&quot;)&lt;/script&gt; &amp; 中文');
   expect(zh).toContain('Task &amp; Calendar');expect(zh).not.toContain('<script>alert(');
   function controls(html){return [...html.matchAll(/<[^>]+>/g)].map(m=>m[0].replace(/\s(?:aria-label|placeholder|lang)="[^"]*"/g,''));}
   expect(controls(zh)).toEqual(controls(en));
 }finally{a.close();b.close();}
});

test('Existing UTF-16 120/1000 limits are retained, including existing split-surrogate boundary',async()=>{
 const a=environment(),b=environment();try{
   await compare(a,b,'/api/tasks','POST',{text:'甲'.repeat(119)+'😀',notes:'乙'.repeat(999)+'😀',profile:'work'});
   const x=JSON.parse((await compare(a,b,'/api/export')).body);
   // SQLite round-trip converts the split UTF-16 surrogate to replacement characters.
   // Comparing both implementations above is the invariant; this is a recorded upstream defect.
   expect(x.tasks[0].text.slice(0,119)).toBe('甲'.repeat(119));
   expect(x.tasks[0].text.slice(119)).toBe('���');
   await compare(a,b,'/api/tasks','POST',{text:'丙'.repeat(121),notes:'丁'.repeat(1001),profile:'work'});
   const plain=JSON.parse((await compare(a,b,'/api/export')).body).tasks.find(t=>t.id===2);
   expect(plain.text.length).toBe(120);expect(plain.notes.length).toBe(1000);
 }finally{a.close();b.close();}
});

test('Unauthenticated and invalid-profile requests retain upstream status and response',async()=>{
 const a=environment(),b=environment();try{
   expect(await response(await call(translated,b,'/api/state','GET',undefined,false))).toEqual(await response(await call(original,a,'/api/state','GET',undefined,false)));
   await compare(a,b,'/api/tasks','POST',{text:'测试',profile:'invalid'});
 }finally{a.close();b.close();}
});
