// About › Updates: the version, what came in each version (patchnotes.json, bundled), and
// updating. Like the Windows app, it asks the Desk Buddy server it's connected to for
// /mac/release.json (menubar/release.sh puts it there): {version, notes, patchnotes, and, when
// there's a download, url (https://… e.g. a GitHub Release asset, or a path on the server),
// sha256, size}. Install downloads the zip, checks it, unpacks it, swaps in the new Contents
// (keeping the app itself, so it stays in the Dock), signs it ad hoc and starts it again.
// Without a url it only says a new version exists. It also asks GitHub for the newest public
// release (desk-buddy-latest.json, a file in every release since 1.2: {"mac": {version, url,
// sha256, size, patchnotes}, "windows": …}).

import AppKit
import CryptoKit

enum Updater {
    static var version: String { Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "" }

    // newer("1.10", "1.9") == true
    static func newer(_ a: String, _ b: String) -> Bool {
        let pa = a.split(separator: ".").map { Int($0) ?? 0 }, pb = b.split(separator: ".").map { Int($0) ?? 0 }
        for i in 0..<max(pa.count, pb.count) {
            let x = i < pa.count ? pa[i] : 0, y = i < pb.count ? pb[i] : 0
            if x != y { return x > y }
        }
        return false
    }

    static let latestURL = URL(string: "https://github.com/deskbuddy-app/desk-buddy/releases/latest/download/desk-buddy-latest.json")!

    // fromGitHub: the Mac part of the newest release, nil when GitHub can't be reached
    static func fromGitHub(_ done: @escaping ([String: Any]?) -> Void) {
        var req = URLRequest(url: latestURL, cachePolicy: .reloadIgnoringLocalCacheData, timeoutInterval: 20)
        req.setValue("DeskBuddy-mac/" + version, forHTTPHeaderField: "User-Agent")
        URLSession.shared.dataTask(with: req) { data, resp, _ in
            guard (resp as? HTTPURLResponse)?.statusCode == 200, let data,
                  let all = (try? JSONSerialization.jsonObject(with: data)) as? [String: Any],
                  let mac = all["mac"] as? [String: Any],
                  (mac["url"] as? String)?.hasPrefix("https://github.com/deskbuddy-app/") == true else { return done(nil) }
            done(mac)
        }.resume()
    }

    // fromTag: the Mac part of an older release (for "Go back"), nil when it has none (before 1.2)
    static func fromTag(_ version: String, _ done: @escaping ([String: Any]?) -> Void) {
        guard version.allSatisfy({ $0.isNumber || $0 == "." }),
              let url = URL(string: "https://github.com/deskbuddy-app/desk-buddy/releases/download/v\(version)/desk-buddy-latest.json") else { return done(nil) }
        var req = URLRequest(url: url, cachePolicy: .reloadIgnoringLocalCacheData, timeoutInterval: 20)
        req.setValue("DeskBuddy-mac/" + self.version, forHTTPHeaderField: "User-Agent")
        URLSession.shared.dataTask(with: req) { data, resp, _ in
            guard (resp as? HTTPURLResponse)?.statusCode == 200, let data,
                  let all = (try? JSONSerialization.jsonObject(with: data)) as? [String: Any],
                  let mac = all["mac"] as? [String: Any], mac["version"] as? String == version,
                  (mac["url"] as? String)?.hasPrefix("https://github.com/deskbuddy-app/") == true else { return done(nil) }
            done(mac)
        }.resume()
    }

    static func entries(_ any: Any?) -> [HelperModel.NoteEntry] {
        (any as? [[String: Any]] ?? []).compactMap { e in
            guard let v = e["version"] as? String else { return nil }
            return HelperModel.NoteEntry(version: v, date: e["date"] as? String ?? "", items: e["items"] as? [String] ?? [])
        }
    }

    static func bundledNotes(_ path: String) -> [HelperModel.NoteEntry] {
        guard let d = FileManager.default.contents(atPath: path) else { return [] }
        return entries(try? JSONSerialization.jsonObject(with: d))
    }

    // install: download → check → unpack → hand over to a small script that waits for this app
    // to quit, swaps the Contents and starts it again. status reports each step (main thread).
    static func install(_ r: [String: Any], server: String, key: String, status: @escaping (String, Bool) -> Void) {
        let say = { (s: String, busy: Bool) in DispatchQueue.main.async { status(s, busy) } }
        guard let file = r["url"] as? String, let sha = r["sha256"] as? String,
              let url = URL(string: file.hasPrefix("http") ? file : server + file) else {
            return say("This version can't be installed from here yet.", false)
        }
        guard FileManager.default.isWritableFile(atPath: Bundle.main.bundleURL.path) else {
            NSWorkspace.shared.open(URL(string: "https://github.com/deskbuddy-app/desk-buddy/releases/latest")!)
            return say("Desk Buddy can't replace itself where it is. Download the new version from GitHub (just opened) and replace the app.", false)
        }
        say("Downloading…", true)
        var req = URLRequest(url: url, timeoutInterval: 600)
        req.setValue("DeskBuddy-helper/1.0", forHTTPHeaderField: "User-Agent")
        if !file.hasPrefix("http") { req.setValue(key, forHTTPHeaderField: "X-Key") } // only to our own server
        URLSession.shared.downloadTask(with: req) { tmp, resp, _ in
            guard let tmp, (resp as? HTTPURLResponse)?.statusCode == 200, let data = try? Data(contentsOf: tmp, options: .mappedIfSafe) else {
                return say("Couldn't download it. Try again in a moment.", false)
            }
            let got = SHA256.hash(data: data).map { String(format: "%02x", $0) }.joined()
            guard got == sha.lowercased() else { return say("The download didn't check out. Try again.", false) }
            say("Installing…", true)
            let dir = FileManager.default.temporaryDirectory.appendingPathComponent("DeskBuddyUpdate-" + UUID().uuidString)
            try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
            let zip = dir.appendingPathComponent("update.zip")
            try? data.write(to: zip)
            let unzip = Process()
            unzip.executableURL = URL(fileURLWithPath: "/usr/bin/ditto")
            unzip.arguments = ["-x", "-k", zip.path, dir.path]
            try? unzip.run()
            unzip.waitUntilExit()
            let new = dir.appendingPathComponent("Desk Buddy.app")
            guard unzip.terminationStatus == 0,
                  FileManager.default.isExecutableFile(atPath: new.appendingPathComponent("Contents/MacOS/DeskBuddy").path) else {
                return say("The download didn't have Desk Buddy in it. Try again later.", false)
            }
            let app = Bundle.main.bundleURL.path
            let script = dir.appendingPathComponent("swap.sh")
            // like install.sh: stop what the app started, replace Contents, sign, start at login again
            let sh = """
            #!/bin/sh
            while kill -0 \(ProcessInfo.processInfo.processIdentifier) 2>/dev/null; do sleep 0.3; done
            pkill -f "$1/Contents/Resources/helper.py" 2>/dev/null
            pkill -f "$1/Contents/MacOS/desk-buddy-server" 2>/dev/null
            sleep 1
            rm -rf "$1/Contents.new"; /usr/bin/ditto "$2/Contents" "$1/Contents.new" && rm -rf "$1/Contents" && mv "$1/Contents.new" "$1/Contents"
            /usr/bin/codesign --force --sign - "$1" >/dev/null 2>&1
            /bin/launchctl kickstart "gui/$(id -u)/\(agentLabel)" 2>/dev/null || /usr/bin/open "$1"
            rm -rf "$3"
            """
            try? sh.write(to: script, atomically: true, encoding: .utf8)
            // its own session, so it outlives this app (launchd stops the app's process group)
            let p = Process()
            p.executableURL = URL(fileURLWithPath: "/usr/bin/python3")
            p.arguments = ["-c", "import os,sys; os.setsid(); os.execv('/bin/sh', ['sh'] + sys.argv[1:])", script.path, app, new.path, dir.path]
            do { try p.run() } catch { return say("Couldn't start the installer: \(error.localizedDescription)", false) }
            say("Restarting…", true)
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.5) { NSApp.terminate(nil) }
        }.resume()
    }
}
