#!/usr/bin/env python3
"""Build packages inside this checkout. Does not install or launch Desk Buddy."""
from pathlib import Path
import os, shlex, shutil, subprocess, tempfile
root = Path(__file__).resolve().parents[1]
sdk = os.environ.get('SDKROOT', '/Library/Developer/CommandLineTools/SDKs/MacOSX26.5.sdk')
if not Path(sdk).is_dir():
    sdk = subprocess.check_output(['xcrun', '--sdk', 'macosx', '--show-sdk-path'], text=True).strip()
if not (root/'worker/node_modules/.bin/bun').exists():
    raise SystemExit('Run npm ci in worker with the existing package-lock.json first.')
(root/'worker/public/kindle').mkdir(exist_ok=True)
with tempfile.TemporaryDirectory(prefix='deskbuddy-build-tools-') as temp:
    swiftc = Path(temp)/'swiftc'
    compiler = subprocess.check_output(['xcrun','--find','swiftc'],text=True).strip()
    swiftc.write_text('#!/bin/sh\nexec '+shlex.quote(compiler)+' -sdk '+shlex.quote(sdk)+' "$@"\n')
    swiftc.chmod(0o755)
    env = dict(os.environ, PATH=temp+os.pathsep+os.environ['PATH'])
    print('Building candidate packages with SDK:',Path(sdk).name,flush=True)
    subprocess.run(['sh','package.sh'],cwd=root/'menubar',env=env,check=True,timeout=900)
print('Packages:',root/'menubar/dist',flush=True)
