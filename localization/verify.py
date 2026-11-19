#!/usr/bin/env python3
"""Re-run host tests, differential render snapshots and offline Mac UI exports."""
from pathlib import Path
import argparse, concurrent.futures, io, json, os, plistlib, shutil, subprocess, tarfile, tempfile
root=Path(__file__).resolve().parents[1]
ap=argparse.ArgumentParser();ap.add_argument('--out',type=Path,required=True);args=ap.parse_args();out=args.out.resolve();out.mkdir(parents=True,exist_ok=True)
base='7245f1a344efd0584246b96f4d0023ce2666e2c3'
results=[]
def command(name,cmd,cwd,env=None,timeout=240):
 print('Running',name,flush=True)
 try:
  run=subprocess.run(cmd,cwd=cwd,env=env,text=True,capture_output=True,timeout=timeout)
  (out/(name+'.log')).write_text(run.stdout+run.stderr)
  status='PASS' if run.returncode==0 else 'FAIL'
  item={'name':name,'status':status,'exitCode':run.returncode,'log':name+'.log'}
 except subprocess.TimeoutExpired as e:
  (out/(name+'.log')).write_text('Timed out after '+str(timeout)+' seconds\n')
  item={'name':name,'status':'FAIL_TIMEOUT','log':name+'.log'}
 print(name,item['status'],flush=True);results.append(item);return item
with tempfile.TemporaryDirectory(prefix='deskbuddy-original-') as tmp:
 snapshot=Path(tmp)
 data=subprocess.check_output(['git','archive',base,'kindle/desk'],cwd=root)
 with tarfile.open(fileobj=io.BytesIO(data)) as tf:tf.extractall(snapshot,filter='data')
 shutil.copy2(root/'kindle/desk/localization_render_test.go',snapshot/'kindle/desk/localization_render_test.go')
 def render(kind,path):
  env=dict(os.environ,DESK_BUDDY_RENDER_OUT=str(out/(kind+'-previews')),DESK_BUDDY_RENDER_FIXTURE=str(root/'localization/fixtures/desk-zh.json'),DESK_BUDDY_RENDER_IMAGES='1' if kind=='zh' else '0')
  return command(kind+'-render',['go','test','-count=1','-run','TestRenderProtocolAndPreviews','-v','.'],path,env)
 jobs=[lambda:command('protected-source',['python3','localization/check-scope.py'],root),lambda:command('go-business-guards',['go','run','localization/tools/scope.go'],root),lambda:command('go-tests',['go','test','-count=1','-json','./...'],root/'kindle/desk'),lambda:command('arm-vet',['go','vet','./...'],root/'kindle/desk',dict(os.environ,CGO_ENABLED='0',GOOS='linux',GOARCH='arm',GOARM='7')),lambda:command('worker-differential',[str(root/'worker/node_modules/.bin/bun'),'test','localization/tests/worker.test.js'],root),lambda:render('original',snapshot/'kindle/desk'),lambda:render('zh',root/'kindle/desk')]
 with concurrent.futures.ThreadPoolExecutor(max_workers=5) as pool:
  for future in [pool.submit(job) for job in jobs]:future.result()
 if (out/'original-previews/actions.json').exists() and (out/'zh-previews/actions.json').exists():
  a=json.loads((out/'original-previews/actions.json').read_text());b=json.loads((out/'zh-previews/actions.json').read_text())
  mismatches=[x['Name'] for x,y in zip(a,b) if x!=y]
  verdict={'name':'kindle-action-parity','status':'PASS' if a==b else 'FAIL','compared':len(a),'differences':mismatches}
  (out/'kindle-action-diff.json').write_text(json.dumps(verdict,ensure_ascii=False,indent=2)+'\n');results.append(verdict)
# A separate executable renders real SwiftUI/AppKit controls without running App or Backend.
sdk=os.environ.get('SDKROOT','/Library/Developer/CommandLineTools/SDKs/MacOSX26.5.sdk')
if Path(sdk).is_dir():
 with tempfile.TemporaryDirectory(prefix='deskbuddy-offline-ui-') as tmp:
  app=Path(tmp)/'Preview.app/Contents';(app/'MacOS').mkdir(parents=True);(app/'Resources').mkdir()
  (app/'Info.plist').write_bytes(plistlib.dumps({'CFBundleIdentifier':'app.deskbuddy.localization.preview','CFBundleExecutable':'Preview','CFBundleName':'Offline UI Preview','CFBundlePackageType':'APPL'}))
  for name in ['AtkinsonHyperlegible-Regular.ttf','AtkinsonHyperlegible-Bold.ttf','SourceHanSansSC-Regular.otf']:
   shutil.copy2(root/'kindle/desk/fonts'/name,app/'Resources'/name)
  shutil.copy2(root/'brand/buddy-kindle.png',app/'Resources/buddy.png');shutil.copy2(root/'brand/spotify-mark-alpha.png',app/'Resources/spotify-mark.png')
  item=command('mac-preview-compile',['swiftc','-sdk',sdk,'-target','arm64-apple-macos13','-o',str(app/'MacOS/Preview'),'menubar/HelperUI.swift','menubar/Localization.swift','localization/tests/Preview.swift'],root)
  if item['status']=='PASS':command('mac-preview-export',[str(app/'MacOS/Preview'),str(out/'mac-previews')],root,timeout=60)
else:results.append({'name':'mac-preview','status':'NOT_RUN_BLOCKED','reason':'SDK unavailable; set SDKROOT'})
# Product/device acceptance is deliberately separate from this host-side verification.
report={'baseline':base,'checks':results,'kindleHardware':'NOT_RUN_BLOCKED','nativeIME':'NOT_RUN','human_verified':False,'productionAppLaunched':False,'installed':False}
(out/'verification.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
raise SystemExit(1 if any(x['status'].startswith('FAIL') for x in results) else 0)
