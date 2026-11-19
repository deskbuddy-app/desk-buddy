#!/usr/bin/env python3
"""Read-only baseline and localization-scope checks; no security remediation."""
from pathlib import Path
import hashlib, json, subprocess, sys
root=Path(__file__).resolve().parents[1]
base='7245f1a344efd0584246b96f4d0023ce2666e2c3'
protected=['kindle/desk/api.go','kindle/desk/main.go','kindle/desk/updater.go','kindle/desk/netfix.go','kindle/desk/events.go','kindle/desk/outbox.go','kindle/desk/ksync.go','kindle/desk/device.go','kindle/desk/wifidiary.go','kindle/desk/go.mod','kindle/desk/go.sum','menubar/Backend.swift','menubar/Permissions.swift','menubar/CalendarSync.swift','menubar/IconSync.swift','menubar/Updater.swift','menubar/KindleSetup.swift','menubar/AudioOut.swift','backend/helper.py','backend/config.example.json','worker/local/server.js','worker/src/calendar.js','worker/src/weather.js','worker/src/demo.js','worker/schema.sql','worker/package.json','worker/package-lock.json','worker/wrangler.toml']
def original(name):return subprocess.check_output(['git','show',base+':'+name],cwd=root)
checks=[]
for name in protected:
 now=(root/name).read_bytes();checks.append({'path':name,'unchanged':original(name)==now,'sha256':hashlib.sha256(now).hexdigest()})
a=original('worker/src/index.js').decode();b=(root/'worker/src/index.js').read_text()
for name,marker,mode in [('worker business prefix','// ---------- phone page ----------','before'),('worker non-UI suffix','// The music playing most recently','after')]:
 x,y=a.index(marker),b.index(marker);same=a[:x]==b[:y] if mode=='before' else a[x:]==b[y:];checks.append({'surface':name,'unchanged':same})
allowed={f'kindle/desk/{x}.go' for x in ['about','calview','cover','deck','focus','gfx','headsup','keyboard','patchnotes','pomodoro','server','settings','side','tasks','themes','tour','ui','weather','widgets']}
allowed.update(['kindle/desk/build.sh','menubar/HelperUI.swift','menubar/MoveToApplications.swift','menubar/install.sh','menubar/main.swift','menubar/package.sh','worker/src/index.js','worker/src/release.json'])
baseline_files=set(subprocess.check_output(['git','ls-tree','-r','--name-only',base],cwd=root,text=True).splitlines())
all_changed=set(subprocess.check_output(['git','diff','--name-only',base],cwd=root,text=True).splitlines())
changed=all_changed & baseline_files
added=(all_changed-baseline_files) | set(subprocess.check_output(['git','ls-files','--others','--exclude-standard'],cwd=root,text=True).splitlines())
allowed_added={
 'LOCALIZATION.md','kindle/desk/chinese.go','kindle/desk/localization_test.go','kindle/desk/localization_render_test.go',
 'kindle/desk/fonts/SourceHanSansSC-Regular.otf','kindle/desk/fonts/OFL-SourceHanSansSC.txt','menubar/Localization.swift',
 'worker/public/fonts/SourceHanSansSC-Regular.otf','worker/public/fonts/OFL-SourceHanSansSC.txt',
 'localization/README.zh-Hans.md','localization/build-candidate.py','localization/changes.json','localization/check-scope.py',
 'localization/fixtures/desk-zh.json','localization/tests/Preview.swift','localization/tests/worker.test.js',
 'localization/tools/dates.go','localization/tools/scope.go','localization/verify.py','localization/zh-Hans.tsv'
}
checks.append({'surface':'source-file allowlist','unchanged':not bool((changed-allowed) | (added-allowed_added)),'unexpectedExisting':sorted(changed-allowed),'unexpectedAdded':sorted(added-allowed_added)})
# The build updates payload hashes/paths/size, while preserving release-note content.
new=json.loads((root/'worker/src/release.json').read_text())
notes=json.loads(original('kindle/desk/patchnotes.json'))[:5]
checks.append({'surface':'release notes from upstream packaging input','unchanged':new.get('notes')==notes})
payload=root/'worker/public'/new['file'].lstrip('/')
launcher=root/'worker/public'/new['run_file'].lstrip('/')
checks.append({'surface':'generated release payload metadata','unchanged':payload.is_file() and launcher.is_file() and hashlib.sha256(payload.read_bytes()).hexdigest()==new['sha256'] and payload.stat().st_size==new['size'] and hashlib.sha256(launcher.read_bytes()).hexdigest()==new['run_sha256']})
result={'baseline':base,'checks':checks,'status':'PASS' if all(x['unchanged'] for x in checks) else 'FAIL','existingChangedFiles':sorted(changed),'addedFiles':sorted(added),'note':'Named byte-level and allowlist checks, not a proof of all possible runtime behavior.'}
print(json.dumps(result,ensure_ascii=False,indent=2))
sys.exit(0 if result['status']=='PASS' else 1)
