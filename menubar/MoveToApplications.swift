// Opened from Downloads (or anywhere outside Applications)? Offer to move itself to the
// Applications folder and open again from there, so nobody has to drag it in a Finder window.
// macOS runs a downloaded app from a hidden temporary copy at first ("App Translocation"), so
// the real place is asked from the Security framework before moving.

import AppKit

enum MoveToApplications {
    // true: it's moving and will open again (stop starting up); false: carry on as normal
    static func offer() -> Bool {
        let here = Bundle.main.bundleURL.resolvingSymlinksInPath()
        let homeApps = FileManager.default.homeDirectoryForCurrentUser.appendingPathComponent("Applications").path
        if here.path.hasPrefix("/Applications/") || here.path.hasPrefix(homeApps + "/") { return false }
        if UserDefaults.standard.bool(forKey: "noMoveOffer") { return false }

        NSApp.activate(ignoringOtherApps: true)
        let a = NSAlert()
        a.messageText = "将 Desk Buddy 移到应用程序文件夹？"
        a.informativeText = "建议从应用程序文件夹运行，并在登录时自动启动。移动后会立即重新打开。"
        a.addButton(withTitle: "移到应用程序")
        a.addButton(withTitle: "暂不移动")
        a.showsSuppressionButton = true
        a.suppressionButton?.title = "不再询问"
        let answer = a.runModal()
        if a.suppressionButton?.state == .on { UserDefaults.standard.set(true, forKey: "noMoveOffer") }
        guard answer == .alertFirstButtonReturn else { return false }

        let from = originalURL(here)
        let to = URL(fileURLWithPath: "/Applications").appendingPathComponent(from.lastPathComponent)
        do {
            if FileManager.default.fileExists(atPath: to.path) {
                try FileManager.default.trashItem(at: to, resultingItemURL: nil) // an older copy
            }
            try FileManager.default.moveItem(at: from, to: to)
        } catch {
            let e = NSAlert()
            e.messageText = "Couldn't move Desk Buddy"
            e.informativeText = "Drag it into Applications in Finder instead. (\(error.localizedDescription))"
            e.runModal()
            return false
        }
        // the user already allowed it to open; without this macOS would run the moved copy
        // from a hidden temporary place again
        let x = Process()
        x.executableURL = URL(fileURLWithPath: "/usr/bin/xattr")
        x.arguments = ["-d", "-r", "com.apple.quarantine", to.path]
        try? x.run()
        x.waitUntilExit()
        // open the moved copy once this one has quit
        let p = Process()
        p.executableURL = URL(fileURLWithPath: "/bin/sh")
        p.arguments = ["-c", "while kill -0 \(ProcessInfo.processInfo.processIdentifier) 2>/dev/null; do sleep 0.2; done; /usr/bin/open \"$0\"", to.path]
        try? p.run()
        NSApp.terminate(nil)
        return true
    }

    // Where the app really is, when macOS runs it from a translocated copy.
    private static func originalURL(_ url: URL) -> URL {
        guard url.path.contains("/AppTranslocation/"),
              let lib = dlopen("/System/Library/Frameworks/Security.framework/Security", RTLD_NOW),
              let sym = dlsym(lib, "SecTranslocateCreateOriginalPathForURL") else { return url }
        typealias Fn = @convention(c) (CFURL, UnsafeMutablePointer<Unmanaged<CFError>?>?) -> Unmanaged<CFURL>?
        let fn = unsafeBitCast(sym, to: Fn.self)
        guard let original = fn(url as CFURL, nil)?.takeRetainedValue() else { return url }
        return original as URL
    }
}
