"""Tells the Desk Buddy backend this computer is in use, so the Kindle follows you.

    python3 helper.py mac                       # on the MacBook
    python helper.py pc --server http://<mac-ip>:8787 --key <key>   # on the Windows PC

Says when this computer starts or stops being used (and every 2 min), and opens the apps tapped on
the Kindle's Deck the moment they're tapped (it keeps a request waiting at the backend).
Only ever opens an app by name (Mac) or a configured target (Windows); never runs commands.
Standard library only.
"""
import argparse
import json
import os
import platform
import re
import base64
import subprocess
import sys
import tempfile
import threading
import time
import urllib.error
import urllib.request
from pathlib import Path


def mac_dark():
    """Whether macOS is showing Dark appearance right now (also when it's on Auto)."""
    if platform.system() != "Darwin":
        return None
    r = subprocess.run(["defaults", "read", "-g", "AppleInterfaceStyle"], capture_output=True, text=True)
    return r.stdout.strip() == "Dark"


def idle_seconds():
    if platform.system() == "Darwin":
        out = subprocess.run(["ioreg", "-c", "IOHIDSystem"], capture_output=True, text=True).stdout
        for line in out.splitlines():
            if "HIDIdleTime" in line:
                return int(line.split("=")[-1]) / 1e9
        return 0.0
    if platform.system() == "Windows":
        import ctypes

        class LASTINPUTINFO(ctypes.Structure):
            _fields_ = [("cbSize", ctypes.c_uint), ("dwTime", ctypes.c_uint)]

        li = LASTINPUTINFO(cbSize=ctypes.sizeof(LASTINPUTINFO))
        ctypes.windll.user32.GetLastInputInfo(ctypes.byref(li))
        return (ctypes.windll.kernel32.GetTickCount() - li.dwTime) / 1000
    return 0.0


def call(a, method, path, body=None, timeout=5):
    req = urllib.request.Request(f"{a.server}{path}", method=method, headers={"Content-Type": "application/json", "X-Key": a.key, "User-Agent": "DeskBuddy-helper/1.0"},
                                 data=json.dumps(body).encode() if body is not None else None)
    with urllib.request.urlopen(req, timeout=timeout) as r:
        return json.load(r)


SAFE_NAME = re.compile(r"^[\w .&'+()-]{1,60}$")


# Meeting links the Kindle's "Join" may open (the same services the backend accepts).
MEETING = re.compile(r"https://(?:[a-z0-9-]+\.)*(?:zoom\.us|meet\.google\.com|teams\.microsoft\.com|teams\.live\.com|"
                     r"webex\.com|whereby\.com|meet\.jit\.si|facetime\.apple\.com)(?![a-z0-9.-])(?:/[^\s\"'<>()]*)?", re.I)


def open_app(item):
    """Opens a deck tile's app, or a meeting link. Returns an error message, or None if it worked."""
    url = item.get("url")
    if url is not None:  # Join from the Kindle's meeting heads-up
        m = MEETING.fullmatch(url)
        if not m:
            return "not a meeting link"
        if platform.system() == "Darwin":
            r = subprocess.run(["open", url], capture_output=True, text=True)
            return None if r.returncode == 0 else (r.stderr.strip() or "couldn't open it")[:80]
        if platform.system() == "Windows":
            os.startfile(url)
            return None
        return "this computer can't open links"
    if platform.system() == "Darwin":
        app = item.get("app") or ""
        if not SAFE_NAME.match(app):
            return "no Mac app set for this tile"
        r = subprocess.run(["open", "-a", app], capture_output=True, text=True)
        return None if r.returncode == 0 else (r.stderr.strip() or "couldn't open it")[:80]
    if platform.system() == "Windows":
        target = item.get("win") or ""
        if not target or any(c in target for c in '&|<>^"\n'):
            return "no Windows app set for this tile"
        try:
            os.startfile(target)  # an app name, a path, or a link like spotify:
            return None
        except OSError as e:
            return str(e)[:80]
    return "this computer can't open apps"


_alarm = {"ends": None, "timer": None}


def chime():
    """The Pomodoro finished: a sound on this computer (the Kindle has no speaker)."""
    print(time.strftime("%H:%M:%S"), "pomodoro: time's up", flush=True)
    try:
        if platform.system() == "Darwin":
            for _ in range(3):
                subprocess.run(["afplay", "/System/Library/Sounds/Glass.aiff"])
        elif platform.system() == "Windows":
            import winsound
            for _ in range(3):
                winsound.PlaySound("SystemExclamation", winsound.SND_ALIAS)
    except Exception as e:
        print("chime:", e, flush=True)


def schedule_chime(p):
    """Keeps one timer set for the end of the current focus or break (if sound is on)."""
    ends = p.get("ends") if p and p.get("sound") and p.get("state") in ("focus", "break") else None
    if ends == _alarm["ends"]:
        return
    if _alarm["timer"]:
        _alarm["timer"].cancel()
    _alarm["ends"], _alarm["timer"] = ends, None
    if ends:
        delay = ends - p.get("now", time.time())  # the server's clock, so the Mac's doesn't matter
        if delay > -30:  # just ended and we only now heard: still chime
            t = threading.Timer(max(0, delay), chime)
            t.daemon = True
            t.start()
            _alarm["timer"] = t


# ---- music: what's playing in Spotify (or Apple Music), and controlling it ----

def osa(script):
    r = subprocess.run(["osascript", "-e", script], capture_output=True, text=True, timeout=5)
    return r.stdout.strip() if r.returncode == 0 else None


_music_pref = {"app": "auto"}  # Settings › Music › Music app: auto, spotify or music (Apple Music)


def music_app():
    """The music app to control: the one picked in Settings, or on Auto the one playing
    (else Spotify, then Apple Music). Only an app that's open; never launches one."""
    pref = _music_pref["app"]
    order = {"spotify": ["Spotify"], "music": ["Music"]}.get(pref, ["Spotify", "Music"])
    if pref == "auto" and (_music_last["state"] or {}).get("app") in order:
        last = _music_last["state"]["app"]
        order = [last] + [x for x in order if x != last]
    for app in order:
        if osa(f'application "{app}" is running') == "true":
            return app
    return None


_NOW_PLAYING = """
on track(appName)
  using terms from application "Spotify"
    tell application appName
      try
        return appName & tab & (player state as string) & tab & (name of current track) & tab & (artist of current track) & tab & (album of current track)
      on error
        return appName & tab & "stopped"
      end try
    end tell
  end using terms from
end track
{pick}
return ""
"""

# which app now_playing reads, per Settings › Music › Music app (Auto: the one playing)
_PICK = {
    "spotify": 'if application "Spotify" is running then return track("Spotify")',
    "music": 'if application "Music" is running then return track("Music")',
    "auto": """if application "Music" is running then
  tell application "Music" to set amPlaying to (player state is playing)
  if amPlaying then return track("Music")
end if
if application "Spotify" is running then return track("Spotify")
if application "Music" is running then return track("Music")""",
}


def now_playing():
    """One AppleScript run: which music app is open (Spotify first; never launches one) and
    what it's playing."""
    out = osa(_NOW_PLAYING.replace("{pick}", _PICK.get(_music_pref["app"], _PICK["auto"])))
    if not out:
        return {"running": False}
    app, state, title, artist, album = (out.split("\t") + ["", "", "", "", ""])[:5]
    if state == "stopped" and not title:
        # Spotify's AppleScript sometimes says "stopped" while it plays: use what it last
        # announced instead (the app passes that on), if it's about this app.
        a = _announced
        if a.get("app") == app and a.get("title") and a.get("state") in ("Playing", "Paused"):
            return {"app": app, "running": True, "playing": a["state"] == "Playing", "title": a["title"],
                    "artist": a.get("artist", ""), "album": a.get("album", ""), "outputs": outputs(), "volume": volume()}
        return {"app": app, "running": True, "playing": False}
    return {"app": app, "running": True, "playing": state == "playing", "title": title, "artist": artist, "album": album,
            "outputs": outputs(), "volume": volume()}


_cover_sent = {}  # the last cover sent: its song (key), artwork url and album


def music_artwork(path):
    """Apple Music: write the current track's artwork (stored with the song) to path.
    Returns an id for the track (so the same picture isn't sent twice), or "" if it has none."""
    path = path.replace('"', "")
    return osa(f'''
tell application "Music"
  set t to current track
  if (count of artworks of t) = 0 then return ""
  set d to raw data of artwork 1 of t
  set tid to persistent ID of t
end tell
set f to open for access (POSIX file "{path}") with write permission
set eof f to 0
write d to f
close access f
return tid''') or ""


def send_cover(a, state):
    """Fetch the cover once (Spotify: from its link; Apple Music: the artwork stored with the
    song), shrink it in a temporary folder (deleted straight after), and send that one small
    image. Nothing is kept on the Mac."""
    if state.get("app") == "Music":
        with tempfile.TemporaryDirectory() as tmp:
            src, out = f"{tmp}/cover", f"{tmp}/small.jpg"
            tid = music_artwork(src)
            if not tid or not os.path.getsize(src):
                return False
            if not post_cover(a, state, src, out):
                return False
        _cover_sent.update(key=cover_key(state), url="music:" + tid, album=state.get("album"))
        return True
    if state.get("app") != "Spotify":
        return
    url = osa('tell application "Spotify" to return artwork url of current track') or ""
    if url == _cover_sent.get("url") and state.get("album") != _cover_sent.get("album"):
        url = ""  # Spotify still gives the previous song's cover
    if not url.startswith("https://") and _announced.get("track", "").startswith("spotify:track:") \
            and _announced.get("title") == state.get("title"):
        # no (fresh) answer from AppleScript: Spotify's public link preview has the cover too
        tid = _announced["track"].split(":")[-1]
        req = urllib.request.Request("https://open.spotify.com/oembed?url=https://open.spotify.com/track/" + tid,
                                     headers={"User-Agent": "DeskBuddy-helper/1.0"})
        try:
            with urllib.request.urlopen(req, timeout=8) as r:
                url = json.loads(r.read(200_000)).get("thumbnail_url", "")
        except Exception:
            url = ""
    if not url.startswith("https://"):
        return False  # try again on the next read
    with tempfile.TemporaryDirectory() as tmp:
        src, out = f"{tmp}/cover.jpg", f"{tmp}/small.jpg"
        req = urllib.request.Request(url, headers={"User-Agent": "DeskBuddy-helper/1.0"})
        with urllib.request.urlopen(req, timeout=10) as r, open(src, "wb") as f:
            f.write(r.read(2_000_000))
        if not post_cover(a, state, src, out):
            return False
    _cover_sent.update(key=cover_key(state), url=url, album=state.get("album"))
    return True


def post_cover(a, state, src, out):
    """Send the cover: Sharp (640 px) when the Kindle's "Cover quality" asks for it, else
    Standard (320 px). An older server only takes the small one, so that's the fallback."""
    for sharp in ([True, False] if _covers["sharp"] else [False]):
        image = small_cover(src, out, sharp)
        if not image:
            continue
        try:
            call(a, "POST", "/api/music/art", {"key": cover_key(state), "image": image}, timeout=15)
            return True
        except urllib.error.HTTPError as e:
            if e.code != 400 or not sharp:
                raise
    return False


def small_cover(src, out, sharp=False):
    """The cover shrunk to a JPEG, as base64 under the server's limit (240,000 characters;
    60,000 before 1.3). Busy covers (e.g. Tame Impala's Currents) stay too big at the first
    quality, so it steps down; the Kindle shows them in grey, where the difference doesn't show."""
    steps = ((("640", "75"), ("640", "60"), ("640", "45"), ("480", "50")), 235000) if sharp else \
        ((("320", "70"), ("320", "50"), ("320", "35"), ("240", "40")), 58000)
    for size, quality in steps[0]:
        subprocess.run(["sips", "-Z", size, "-s", "format", "jpeg", "-s", "formatOptions", quality, src, "--out", out],
                       capture_output=True, timeout=10)
        if not os.path.exists(out):
            return ""
        image = base64.b64encode(open(out, "rb").read()).decode()
        if len(image) <= steps[1]:
            return image
    return ""


def cover_key(state):
    """Which song a cover belongs to; the backend builds the same from what's playing."""
    return "|".join(str(state.get(k) or "")[:120] for k in ("title", "artist", "album"))[:200]


def audio_out(*args):
    """The bundled audio-out tool (next to the app's own binary): the Mac's sound outputs."""
    tool = os.path.join(os.path.dirname(sys.executable if getattr(sys, "frozen", False) else __file__), "..", "MacOS", "audio-out")
    if not os.path.exists(tool):
        return None
    r = subprocess.run([tool, *args], capture_output=True, text=True, timeout=15)
    return r.stdout if r.returncode == 0 else None


def volume():
    """The Mac's output volume, 0–100 (None if unknown, e.g. a device without one)."""
    v = osa("output volume of (get volume settings)")
    return int(v) if v and v.isdigit() else None


def outputs():
    """Where the Mac's sound can go: [{id, name, kind, current, connected}] (only when there's a choice)."""
    try:
        out = json.loads(audio_out("list") or "[]")
    except Exception:
        return []
    return out if len(out) > 1 else []


def media(action):
    if action.startswith("volume:"):  # the Mac's output volume, in steps of 10 (Kindle music screen)
        step = 10 if action == "volume:up" else -10
        ok = osa(f"set volume output volume ((output volume of (get volume settings)) + {step})") is not None
        return None if ok else "couldn't change the volume"
    if action.startswith(("output:", "connect:")):  # pick where the sound goes (Kindle music screen)
        verb, _, oid = action.partition(":")
        ok = audio_out("set" if verb == "output" else "connect", oid) is not None
        return None if ok else "couldn't switch the sound to " + oid
    app = music_app()
    if not app or platform.system() != "Darwin":
        return "no music app open"
    cmd = {"playpause": "playpause", "next": "next track", "previous": "previous track"}[action]
    return None if osa(f'tell application "{app}" to {cmd}') is not None else "couldn't control " + app


_music_wanted = threading.Event()  # set while the Kindle is showing the Music widget
_covers = {"on": True, "sharp": True}  # the "Album covers" and "Cover quality" settings


_music_last = {"state": None, "sent_at": 0}
_music_lock = threading.Lock()


def music_tick(a, force=False):
    """Read the music app; send the cover (new song) and what's playing when it changed."""
    with _music_lock:
        last = _music_last["state"]
        state = now_playing()
        if not state.get("title"):
            _cover_sent.clear()  # the backend drops the cover now; send it again if the song comes back
        cover_for_song(a, state)  # before the title so they arrive together
        if force or state != last or time.time() - _music_last["sent_at"] > 60:
            call(a, "POST", "/api/music/state", {"machine": a.machine, **state})
            _music_last["state"], _music_last["sent_at"] = state, time.time()


def cover_for_song(a, state):
    """Send the playing song's cover unless it's already sent (once per song; retried on
    the next read when it failed or Spotify hadn't caught up)."""
    key = cover_key(state)
    if not state.get("title") or not _covers["on"] or _cover_sent.get("key") == key:
        return
    tries = _cover_tries[key] = _cover_tries.get(key, 0) + 1
    if len(_cover_tries) > 1:  # only the current song's count matters
        for k in [k for k in _cover_tries if k != key]:
            del _cover_tries[k]
    if tries > 5:
        return
    try:
        sent = send_cover(a, state)
    except Exception as e:
        print(time.strftime("%H:%M:%S"), "cover:", e, flush=True)
        sent = False
    if sent is False:
        threading.Timer(2.5, _nudge.set).start()  # look again shortly


_cover_tries = {}  # how often we've tried to send the current song's cover


def after_media(a):
    """Right after a skip / play / pause from the Kindle: the new song and cover, quickly
    (Spotify takes a moment to change track, so look twice)."""
    for delay in (0.6, 1.5):
        time.sleep(delay)
        try:
            music_tick(a)
        except Exception as e:
            print(time.strftime("%H:%M:%S"), "music:", e, flush=True)


_announced = {}  # the last song the music app announced (app, state, title, artist, album, track)
_nudge = threading.Event()  # the app says something changed (a song, play/pause, the sound output)


def listen_to_app():
    """The Desk Buddy app writes "music" on our stdin when Spotify / Apple Music
    announce a change, or the Mac's sound output changes: so we read the music app only then."""
    for line in sys.stdin:
        line = line.strip()
        if line.startswith("music {"):  # the announcement's own details (see now_playing)
            try:
                _announced.update(json.loads(line[6:]), at=time.time())
            except ValueError:
                pass
        if line.startswith("music"):
            _nudge.set()


def watch_music(a):
    """While the Kindle shows the Music widget or screen: reads the music app when it announces
    a change (via the app, see listen_to_app), and every 20 s as a fallback. Otherwise nothing."""
    while True:
        _music_wanted.wait()  # sleeps here, doing nothing, until the widget is opened
        try:
            if platform.system() == "Darwin":
                music_tick(a)
        except Exception as e:
            print(time.strftime("%H:%M:%S"), "music:", e, flush=True)
        _nudge.wait(20)
        _nudge.clear()
        time.sleep(0.3)  # let the app finish changing track before reading it


def check_deck(a):
    try:
        item = call(a, "GET", f"/api/deck/next?machine={a.machine}&wait=20", timeout=30)
    except Exception:
        time.sleep(3)  # offline or an older backend: don't spin
        return
    schedule_chime(item.get("pomodoro"))
    if item.get("album_covers", True) and not _covers["on"]:
        _cover_sent.clear()  # covers back on: the backend deleted the last one, so send it again
    _covers["on"] = item.get("album_covers", True)
    if item.get("cover_sharp", True) != _covers["sharp"]:
        _cover_sent.clear()  # quality changed: send the current cover again
    _covers["sharp"] = item.get("cover_sharp", True)
    _music_pref["app"] = item.get("music_app") or "auto"
    if item.get("music_wanted"):
        _music_wanted.set()
    else:
        _music_wanted.clear()
    if not item.get("id") or item.get("wake"):
        return
    if item.get("switch") in ("cloud", "local"):  # the Kindle asks to move Desk Buddy: the app does it
        print(time.strftime("%H:%M:%S"), "switch:", item["switch"], flush=True)
        return
    if item.get("media"):  # play/pause/next/previous from the Kindle's Music widget
        err = media(item["media"])
        print(time.strftime("%H:%M:%S"), "music:", item["media"], "→", err or "done", flush=True)
        threading.Thread(target=after_media, args=(a,), daemon=True).start()
        return
    err = open_app(item)
    print(time.strftime("%H:%M:%S"), "deck:", item.get("label"), "→", err or "opened", flush=True)
    try:
        call(a, "POST", "/api/deck/done", {"id": item["id"], "ok": err is None, "error": err})
    except Exception:
        pass


def beat(a, idle, dark):
    try:
        print(time.strftime("%H:%M:%S"), "→", call(a, "POST", "/api/heartbeat", {"machine": a.machine, "idle": idle, "dark": dark})["profile"], flush=True)
        return True
    except Exception as e:  # server offline etc. Keep trying.
        print(time.strftime("%H:%M:%S"), "backend not reachable:", e, flush=True)
        return False


IN_USE = 240  # seconds without input before the Mac counts as not in use (the backend allows 300)


def heartbeats(a):
    """Tells the backend this computer is here, but only when it matters: when it starts or
    stops being used, when light / dark changes, and otherwise every 2 minutes (the backend
    forgets a computer after 3). Looking costs nothing over the network: every 10 s, locally."""
    last = {"at": 0, "used": None, "dark": None}
    while True:
        idle, dark = idle_seconds(), mac_dark()
        used = idle < IN_USE
        if time.time() - last["at"] >= 120 or used != last["used"] or dark != last["dark"]:
            if beat(a, idle, dark):
                last.update(at=time.time(), used=used, dark=dark)
            else:
                last["at"] = time.time() - 90  # try again in 30 s
        time.sleep(10)


def report_cpu():
    """For the app's Performance section: this script's CPU time so far, including the small
    programs it runs (osascript, audio-out), which a process list wouldn't count."""
    import resource
    while True:
        me, kids = resource.getrusage(resource.RUSAGE_SELF), resource.getrusage(resource.RUSAGE_CHILDREN)
        print(f"stats cpu={me.ru_utime + me.ru_stime + kids.ru_utime + kids.ru_stime:.3f}", flush=True)
        time.sleep(10)


def main():
    conf_file = Path(__file__).with_name("config.json")
    conf = json.loads(conf_file.read_text()) if conf_file.exists() else {}
    p = argparse.ArgumentParser()
    p.add_argument("machine", choices=["mac", "pc"])
    p.add_argument("--server", default=conf.get("server", "http://127.0.0.1:8787"))
    p.add_argument("--key", default=conf.get("key"))
    p.add_argument("--once", action="store_true", help="send one heartbeat and exit")
    a = p.parse_args()
    if not a.key:
        sys.exit("No key: pass --key, or run next to the server's config.json")
    if a.once:
        beat(a, idle_seconds(), mac_dark())
        return
    threading.Thread(target=heartbeats, args=(a,), daemon=True).start()
    threading.Thread(target=watch_music, args=(a,), daemon=True).start()
    threading.Thread(target=listen_to_app, daemon=True).start()
    threading.Thread(target=report_cpu, daemon=True).start()
    while True:
        check_deck(a)  # returns as soon as a tile is tapped, or after 20 s


if __name__ == "__main__":
    main()
