// Desk Buddy: a menu bar app that keeps backend/helper.py running on the Mac.
// helper.py tells the backend you're using the MacBook (for Auto) and opens Deck apps.
// The menu can pause it, quit it, or uninstall everything (after a warning).

import AppKit
import ServiceManagement
import CoreAudio
import SwiftUI

let agentLabel = "app.deskbuddy.mac"
let agentPlist = FileManager.default.homeDirectoryForCurrentUser
    .appendingPathComponent("Library/LaunchAgents/\(agentLabel).plist")

final class App: NSObject, NSApplicationDelegate, NSWindowDelegate {
    let item = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
    let statusLine = NSMenuItem(title: "Starting…", action: nil, keyEquivalent: "")
    let lastLine = NSMenuItem(title: "", action: nil, keyEquivalent: "")
    let calLine = NSMenuItem(title: "Calendar: waiting for access…", action: nil, keyEquivalent: "")
    let model = HelperModel()
    lazy var permissions = PermissionChecker(model: model)
    lazy var backend = Backend(bundledConfig: resource("config.json"))
    lazy var calendar = CalendarSync(server: backend.address, key: backend.settings.key)
    lazy var icons = IconSync(server: backend.address, key: backend.settings.key)
    var kindleSetup: KindleSetup?
    let pauseItem = NSMenuItem(title: "Pause", action: #selector(togglePause), keyEquivalent: "p")
    var child: Process?
    var childInput: FileHandle?
    var helperCPU = 0.0 // helper.py's CPU seconds so far, with the small programs it ran (it reports them)
    var restartTimer: Timer?
    var quitting = false
    let started = Date()

    var paused: Bool {
        get { UserDefaults.standard.bool(forKey: "paused") }
        set { UserDefaults.standard.set(newValue, forKey: "paused") }
    }

    func applicationDidFinishLaunching(_ n: Notification) {
        KD.registerFonts()
        // One copy only: if it's already running (started at login), ask that one to show
        // its window and quit this one. (Opening the app used to start a second copy.)
        let me = ProcessInfo.processInfo.processIdentifier
        let others = { NSRunningApplication.runningApplications(withBundleIdentifier: agentLabel).filter { $0.processIdentifier != me && !$0.isTerminated } }
        if !others().isEmpty { Thread.sleep(forTimeInterval: 1.5) } // one may just be quitting (a reinstall)
        if !others().isEmpty {
            DistributedNotificationCenter.default().postNotificationName(.init(agentLabel + ".show"), object: nil, userInfo: nil, deliverImmediately: true)
            exit(0)
        }
        if MoveToApplications.offer() { return } // opened from Downloads: move to Applications first
        DistributedNotificationCenter.default().addObserver(forName: .init(agentLabel + ".show"), object: nil, queue: .main) { [weak self] _ in
            self?.showWindow()
        }
        // Start at login: install.sh sets up a LaunchAgent; a downloaded copy registers itself
        // as a login item instead (System Settings › General › Login Items can turn it off),
        // unless it was switched off in About › Start at login
        if !FileManager.default.fileExists(atPath: agentPlist.path), !UserDefaults.standard.bool(forKey: "noLoginItem"),
           SMAppService.mainApp.status != .enabled {
            try? SMAppService.mainApp.register()
        }
        model.startAtLogin = App.startsAtLogin
        model.onStartAtLogin = { [weak self] on in
            App.setStartAtLogin(on)
            self?.model.startAtLogin = App.startsAtLogin
        }
        // About › Updates: the version and its patch notes; look for a newer one a minute after
        // starting, then every 6 hours (Updater.swift)
        model.version = Updater.version
        // putting Desk Buddy on a Kindle over USB (KindleSetup.swift): set up for where this Mac's
        // Desk Buddy runs (its address on the Wi-Fi in This Mac mode, else the cloud)
        kindleSetup = KindleSetup(model: model, resources: Bundle.main.resourceURL!) { [weak self] in
            guard let b = self?.backend else { return [:] }
            return ["server": b.local ? b.lanAddress : b.settings.cloud, "key": b.settings.key]
        }
        model.onKindleInstall = { [weak self] in self?.kindleSetup?.install() }
        model.onKindleSaveFiles = { [weak self] in self?.kindleSetup?.saveFiles() }
        model.notes = Updater.bundledNotes(resource("patchnotes.json"))
        DispatchQueue.main.asyncAfter(deadline: .now() + 60) { [weak self] in self?.checkUpdate(manual: false) }
        Timer.scheduledTimer(withTimeInterval: 6 * 3600, repeats: true) { [weak self] _ in self?.checkUpdate(manual: false) }
        let menu = NSMenu()
        let open = NSMenuItem(title: "Open Desk Buddy…", action: #selector(openWindow), keyEquivalent: "w")
        open.target = self
        menu.addItem(open)
        menu.addItem(.separator())
        statusLine.isEnabled = false
        lastLine.isEnabled = false
        lastLine.isHidden = true
        menu.addItem(statusLine)
        menu.addItem(lastLine)
        calLine.isEnabled = false
        menu.addItem(calLine)
        menu.addItem(.separator())
        pauseItem.target = self
        menu.addItem(pauseItem)
        let syncCal = NSMenuItem(title: "Sync calendar now", action: #selector(syncCalendar), keyEquivalent: "s")
        syncCal.target = self
        menu.addItem(syncCal)
        let page = NSMenuItem(title: "Open tasks page", action: #selector(openPage), keyEquivalent: "o")
        page.target = self
        menu.addItem(page)
        localItem.target = self
        menu.addItem(localItem)
        menu.addItem(.separator())
        let quit = NSMenuItem(title: "Quit (starts again at next login)", action: #selector(quit), keyEquivalent: "q")
        quit.target = self
        menu.addItem(quit)
        let uninstall = NSMenuItem(title: "Uninstall…", action: #selector(uninstall), keyEquivalent: "")
        uninstall.target = self
        menu.addItem(uninstall)
        item.menu = menu
        calendar.onStatus = { [weak self] text in
            self?.calLine.title = text
            self?.refresh()
        }
        listenForMusic()
        backend.onStatus = { [weak self] text in
            self?.serverLine = text
            self?.refresh()
        }
        backend.startServer() // local mode only
        backend.onPairRequest = { [weak self] id, code, name in self?.askToPair(id: id, code: code, name: name) }
        backend.watchPairing()
        calendar.enabled = !paused
        calendar.start()
        icons.enabled = !paused
        icons.start()
        if paused { refresh() } else { start() }
        // Started by hand (Dock, Finder) rather than at login: show the controls.
        if ProcessInfo.processInfo.environment["XPC_SERVICE_NAME"] != agentLabel { showWindow() }
        welcomeIfNeeded()
    }

    // MARK: helper.py

    func resource(_ name: String) -> String {
        Bundle.main.resourceURL!.appendingPathComponent(name).path
    }

    func start() {
        guard child == nil, !paused else { return }
        let p = Process()
        p.executableURL = URL(fileURLWithPath: "/usr/bin/python3")
        p.arguments = ["-u", resource("helper.py"), "mac", "--server", backend.address, "--key", backend.settings.key]
        let out = Pipe()
        p.standardOutput = out
        p.standardError = out
        let input = Pipe()
        p.standardInput = input
        childInput = input.fileHandleForWriting
        out.fileHandleForReading.readabilityHandler = { [weak self] h in
            let text = String(decoding: h.availableData, as: UTF8.self)
            DispatchQueue.main.async { self?.read(text) }
        }
        p.terminationHandler = { [weak self] ended in
            // only the current copy: an old one stopping after a restart must not forget the new one
            DispatchQueue.main.async { if self?.child === ended { self?.ended() } }
        }
        do {
            try p.run()
            child = p
            statusLine.title = "Running · connecting…"
        } catch {
            statusLine.title = "Couldn't start: \(error.localizedDescription)"
        }
        refresh()
    }

    func stop() {
        child?.terminate()
        child = nil
    }

    // Lines from helper.py: "20:52:45 → work", "deck: Slack → opened", "backend not reachable: …"
    func read(_ text: String) {
        for line in text.split(separator: "\n").map(String.init) {
            if let r = line.range(of: "stats cpu=") {
                helperCPU = Double(line[r.upperBound...].trimmingCharacters(in: .whitespaces)) ?? helperCPU
                continue
            }
            if let r = line.range(of: "switch: ") { // the Kindle asked (Settings › Server): move without asking here
                let to = line[r.upperBound...].trimmingCharacters(in: .whitespaces)
                if to == "cloud" || to == "local" { DispatchQueue.main.async { self.switchMode(to, copy: true) } }
                continue
            }
            if let r = line.range(of: "deck: ") {
                lastLine.title = "Deck: " + line[r.upperBound...]
                lastLine.isHidden = false
            } else if let r = line.range(of: "→ ") {
                statusLine.title = "Running · Kindle showing \(line[r.upperBound...].capitalized)"
            } else if line.contains("not reachable") {
                statusLine.title = "Running · can't reach the backend, retrying"
            }
        }
        refresh()
    }

    func ended() {
        child = nil
        if quitting || paused { return }
        statusLine.title = "Stopped unexpectedly · restarting…"
        refresh()
        restartTimer = Timer.scheduledTimer(withTimeInterval: 10, repeats: false) { [weak self] _ in self?.start() }
    }

    func refresh() {
        if paused { statusLine.title = "Paused · the Kindle won't follow this Mac" }
        pauseItem.title = paused ? "Resume" : "Pause"
        // Desk Buddy himself in the menu bar (a template: macOS colours it), faded while paused
        item.button?.image = App.menuBarImage ?? NSImage(systemSymbolName: paused ? "book.closed" : "book", accessibilityDescription: "Desk Buddy")
        item.button?.appearsDisabled = paused
        item.button?.toolTip = "Desk Buddy · " + statusLine.title
        localItem.state = backend.local ? .on : .off
        // the window (HelperUI.swift)
        model.status = statusLine.title
        model.lastDeck = lastLine.isHidden ? "" : lastLine.title.replacingOccurrences(of: "Deck: ", with: "")
        model.calendar = (paused ? "Paused" : calLine.title).replacingOccurrences(of: "Calendar: ", with: "")
        model.paused = paused
        model.local = backend.local
        let base = backend.local ? backend.lanAddress : backend.settings.cloud
        model.phoneURL = base.isEmpty ? "" : base + "/app?key=" + (backend.settings.key.addingPercentEncoding(withAllowedCharacters: .alphanumerics) ?? "")
        // Server › Your own cloud server, step 2: the usual places Node.js gets installed
        let nvm = FileManager.default.homeDirectoryForCurrentUser.appendingPathComponent(".nvm/versions/node").path
        model.nodeInstalled = ["/usr/local/bin/node", "/opt/homebrew/bin/node"].contains { FileManager.default.isExecutableFile(atPath: $0) }
            || FileManager.default.fileExists(atPath: nvm)
        model.lanAddress = backend.lanAddress
        model.serverLine = serverLine
    }

    // MARK: menu actions

    @objc func togglePause() {
        paused.toggle()
        calendar.enabled = !paused
        icons.enabled = !paused
        if !paused { icons.sync() }
        if paused { restartTimer?.invalidate(); stop() } else { start() }
        refresh()
    }

    @objc func syncCalendar() {
        calLine.title = "Calendar: syncing…"
        refresh()
        calendar.sync(force: true)
    }

    @objc func openPage() {
        let base = backend.local ? backend.lanAddress : backend.address
        guard let url = URL(string: "\(base)/app?key=\(backend.settings.key)") else { return }
        NSWorkspace.shared.open(url)
    }

    // MARK: music changes, without polling

    // Spotify and Apple Music announce every play / pause / track change, and macOS says when
    // the sound output changes: pass that to helper.py, which then reads the music app once.
    func listenForMusic() {
        let dnc = DistributedNotificationCenter.default()
        for (name, app) in [("com.spotify.client.PlaybackStateChanged", "Spotify"), ("com.apple.Music.playerInfo", "Music")] {
            dnc.addObserver(forName: .init(name), object: nil, queue: .main) { [weak self] n in
                // The announcement itself says what's playing: pass it on too, in case the
                // app's AppleScript doesn't (Spotify's sometimes says "stopped" while playing).
                let u = n.userInfo ?? [:]
                let info: [String: String] = [
                    "app": app,
                    "state": u["Player State"] as? String ?? "",
                    "title": u["Name"] as? String ?? "",
                    "artist": u["Artist"] as? String ?? "",
                    "album": u["Album"] as? String ?? "",
                    "track": u["Track ID"] as? String ?? "",
                ]
                if let d = try? JSONSerialization.data(withJSONObject: info), let j = String(data: d, encoding: .utf8) {
                    self?.tellHelper("music " + j)
                } else {
                    self?.nudge()
                }
            }
        }
        for sel in [kAudioHardwarePropertyDefaultOutputDevice, kAudioHardwarePropertyDevices] {
            var addr = AudioObjectPropertyAddress(mSelector: sel, mScope: kAudioObjectPropertyScopeGlobal, mElement: kAudioObjectPropertyElementMain)
            AudioObjectAddPropertyListenerBlock(AudioObjectID(kAudioObjectSystemObject), &addr, .main) { [weak self] _, _ in self?.nudge() }
        }
    }

    func nudge() { tellHelper("music") }

    func tellHelper(_ line: String) {
        try? childInput?.write(contentsOf: Data((line.replacingOccurrences(of: "\n", with: " ") + "\n").utf8))
    }

    // MARK: Performance: how little this uses (CPU, memory, macOS's Energy Impact)

    var perfTimer: Timer?
    var perfBusy = false
    var cpuSamples: [(Date, Double)] = []
    var energySamples: [(Date, Double)] = [] // top's 1-second readings, averaged over the last minute

    // Samples this app, helper.py and the local server with `top` (two samples a second
    // apart, like Activity Monitor) and `ps` (CPU time since start). Only while the window shows.
    func measure() {
        guard window?.isVisible == true, model.section == "performance", !perfBusy else { return } // only while it's on screen
        perfBusy = true
        let pids = [ProcessInfo.processInfo.processIdentifier, child?.processIdentifier, backend.serverPID].compactMap { $0 }
        DispatchQueue.global(qos: .utility).async { [weak self] in
            func run(_ path: String, _ args: [String]) -> String {
                let p = Process(), out = Pipe()
                p.executableURL = URL(fileURLWithPath: path)
                p.arguments = args
                p.standardOutput = out
                p.standardError = FileHandle.nullDevice
                guard (try? p.run()) != nil else { return "" }
                let d = out.fileHandleForReading.readDataToEndOfFile()
                p.waitUntilExit()
                return String(decoding: d, as: UTF8.self)
            }
            var cpu = 0.0, power = 0.0
            let top = run("/usr/bin/top", ["-l", "2", "-s", "1", "-stats", "pid,cpu,power"] + pids.flatMap { ["-pid", String($0)] })
            // the second sample is the real one: the lines after the last "PID" header
            if let last = top.components(separatedBy: "PID").last {
                for line in last.split(separator: "\n").dropFirst() {
                    let f = line.split(separator: " ").compactMap { Double($0.trimmingCharacters(in: CharacterSet(charactersIn: "+-*"))) }
                    if f.count >= 3 { cpu += f[1]; power += f[2] }
                }
            }
            var mem = 0.0, serverCPU = 0.0, serverMem = 0.0
            let ps = run("/bin/ps", ["-o", "rss=,time=", "-p", pids.map(String.init).joined(separator: ",")])
            func seconds(_ t: Substring) -> Double { // [[dd-]hh:]mm:ss(.ss)
                var total = 0.0
                let dayParts = t.split(separator: "-")
                for part in (dayParts.last ?? "").split(separator: ":") { total = total * 60 + (Double(part) ?? 0) }
                if dayParts.count == 2 { total += (Double(dayParts[0]) ?? 0) * 86400 }
                return total
            }
            for (i, line) in ps.split(separator: "\n").enumerated() {
                let f = line.split(separator: " ")
                guard f.count >= 2 else { continue }
                mem += (Double(f[0]) ?? 0) / 1024
                if pids.count == 3 && i == 2 { serverCPU = seconds(f[1]); serverMem = (Double(f[0]) ?? 0) / 1024 }
            }
            // CPU time of everything: this app (its own measuring tools not included), helper.py
            // with the small programs it ran, and the local server.
            var ru = rusage()
            getrusage(RUSAGE_SELF, &ru)
            let mine = Double(ru.ru_utime.tv_sec) + Double(ru.ru_utime.tv_usec) / 1e6 + Double(ru.ru_stime.tv_sec) + Double(ru.ru_stime.tv_usec) / 1e6
            let parts = pids.count == 3 ? "this app, its helper and the local server" : "this app and its helper"
            DispatchQueue.main.async {
                guard let self else { return }
                let total = mine + self.helperCPU + serverCPU
                let now = Date()
                self.cpuSamples.append((now, total))
                self.cpuSamples.removeAll { now.timeIntervalSince($0.0) > 65 }
                var avgText = "measuring…"
                if let first = self.cpuSamples.first, now.timeIntervalSince(first.0) >= 9 {
                    let span = now.timeIntervalSince(first.0)
                    let avg = (total - first.1) / span * 100
                    avgText = String(format: "%.2f%% on average over the last %@", avg, span >= 55 ? "minute" : "\(Int(span)) s")
                }
                let text = String(format: """
                Energy Impact: %.1f  (Activity Monitor's measure; under 1 is very light)
                CPU: %@
                Memory: %.0f MB
                Counting %@. Updates every 10 seconds while this window is open.
                It's higher for a minute or so after it starts (the calendar, app icons and music sync then); that's normal.
                """, power, avgText, mem, parts)
                self.model.perf = text
                self.energySamples.append((now, power))
                self.energySamples.removeAll { now.timeIntervalSince($0.0) > 65 }
                self.model.perfEnergy = self.energySamples.map(\.1).reduce(0, +) / Double(self.energySamples.count)
                self.model.perfMem = mem
                self.model.perfServerMem = serverMem
                self.model.perfParts = parts
                if let first = self.cpuSamples.first, now.timeIntervalSince(first.0) >= 9 {
                    self.model.perfCPU = (total - first.1) / now.timeIntervalSince(first.0) * 100
                }
                // the first 90 s after starting are busy (calendar, icons, music): not a fair measure
                self.model.perfWarming = now.timeIntervalSince(self.started) < 90
                self.perfBusy = false
            }
        }
    }

    // Instructions for AI assistants on this Mac (This Mac › AI assistants): how to read and change
    // the tasks through the server's API. The address and key come from Desk Buddy's own settings
    // file at run time, so they're never pasted anywhere and survive switching Cloud / This Mac.
    static func aiInstructions() -> String {
        let file = Backend.file.path.replacingOccurrences(of: FileManager.default.homeDirectoryForCurrentUser.path, with: "~")
        return """
        # Desk Buddy tasks

        My task list lives in Desk Buddy (shown on a Kindle on my desk). When I ask to add, change, tick off, pin, reorder or remove a task, do it through Desk Buddy's API. The Kindle updates within a few seconds.

        - Settings: `\(file)` (JSON). `mode` "local" → server `http://127.0.0.1:\(Backend.port)`; "cloud" → the address in `cloud`. The key is `key`: send it as an `X-Key` header. Never print, log or repeat the key.
        - Always send a User-Agent (e.g. `curl -A DeskBuddy`): the cloud server blocks some default ones.
        - Lists ("profile"): `work` = the Mac list, `home` = the Windows list. With one list (setting task_lists = one) it's always `work`. If it's unclear which list, ask.
        - `GET /api/state`: everything, with each task's `id`.
        - `POST /api/tasks` `{"profile", "text", "notes"?, "parent_id"?}`: add (`parent_id` makes it a subtask).
        - `PATCH /api/tasks/<id>` `{"text"?, "notes"?}`: edit.
        - `POST /api/tasks/<id>/toggle`: tick / untick (its subtasks follow).
        - `POST /api/tasks/<id>/pin`: make it "Now working on". `POST /api/unpin/<profile>`: unpin.
        - `POST /api/tasks/<id>/move/up` or `/move/down`.
        - `DELETE /api/tasks/<id>`: delete (with its subtasks). Ask first.

        Example (read the settings without printing the key):

        ```sh
        F="\(file.replacingOccurrences(of: "~", with: "$HOME"))"
        KEY=$(python3 -c "import json,sys;print(json.load(open(sys.argv[1]))['key'])" "$F")
        SERVER=$(python3 -c "import json,sys;d=json.load(open(sys.argv[1]));print(d['cloud'] if d.get('mode')=='cloud' else 'http://127.0.0.1:\(Backend.port)')" "$F")
        curl -s -A DeskBuddy -H "X-Key: $KEY" -H "Content-Type: application/json" -d '{"profile":"work","text":"Call Sam"}' "$SERVER/api/tasks"
        ```
        """
    }

    // MARK: Cloud / This Mac

    var serverLine = ""
    let localItem = NSMenuItem(title: "Run server on this Mac", action: #selector(toggleLocal), keyEquivalent: "")

    @objc func toggleLocal() {
        pickMode(backend.local ? "cloud" : "local")
    }

    func pickMode(_ mode: String) {
        guard mode != backend.settings.mode else { return }
        let a = NSAlert()
        if mode == "local" {
            a.messageText = "Run Desk Buddy on this Mac?"
            a.informativeText = """
            Your tasks, calendar and settings will live on this Mac instead of Cloudflare. \
            The Kindle and the phone page then only update while this Mac is awake, on the \
            same Wi-Fi, with this app open. Anything changed while it's off syncs once it's back.

            Copy everything from the cloud to this Mac first?
            """
        } else {
            a.messageText = "Move back to the cloud?"
            a.informativeText = "The Kindle will work while this Mac sleeps again.\n\nCopy everything from this Mac back to the cloud first? (Otherwise the cloud keeps what it had before.)"
        }
        a.addButton(withTitle: "Copy and switch")
        a.addButton(withTitle: "Switch without copying")
        a.addButton(withTitle: "Cancel")
        NSApp.activate(ignoringOtherApps: true)
        let r = a.runModal()
        guard r != .alertThirdButtonReturn else { return refresh() }
        switchMode(mode, copy: r == .alertFirstButtonReturn)
    }

    // Moves Desk Buddy (picked here, or asked for by the Kindle); the Kindle follows the note.
    func switchMode(_ mode: String, copy: Bool) {
        guard mode != backend.settings.mode else { return }
        serverLine = "Switching…"
        refresh()
        backend.switchTo(mode, copy: copy) { [weak self] err in
            guard let self else { return }
            if let err {
                self.serverLine = err
                let e = NSAlert()
                e.messageText = "Couldn't switch"
                e.informativeText = err
                e.runModal()
            } else {
                self.serverLine = self.backend.local ? "Local server running" : ""
            }
            // everything on this Mac follows the new address
            self.calendar.server = self.backend.address
            self.icons.server = self.backend.address
            self.calendar.sync()
            self.icons.sync()
            self.restartTimer?.invalidate()
            self.stop()
            self.start()
            self.refresh()
        }
    }

    // A Kindle on the Wi-Fi wants to connect to this Mac's server (first setup on the Kindle).
    func askToPair(id: String, code: String, name: String) {
        NSApp.activate(ignoringOtherApps: true)
        let a = NSAlert()
        let kindle = name.lowercased().contains("kindle")
        a.messageText = kindle ? "A Kindle wants to connect" : "\(name) wants to connect"
        a.informativeText = """
        Check that \(kindle ? "the Kindle" : "it") shows the code \(code).

        Allowing it lets \(kindle ? "the Kindle" : "that computer") show and change your tasks, calendar and the rest of Desk Buddy. Only allow \(kindle ? "a Kindle" : "a computer") you own.
        """
        a.addButton(withTitle: "Allow")
        a.addButton(withTitle: "Don't Allow")
        backend.answerPair(id, allow: a.runModal() == .alertFirstButtonReturn)
    }

    @objc func quit() {
        quitting = true
        stop()
        backend.stopServer()
        NSApp.terminate(nil)
    }

    @objc func uninstall() {
        NSApp.activate(ignoringOtherApps: true)
        let a = NSAlert()
        a.alertStyle = .warning
        a.messageText = "Uninstall Desk Buddy?"
        a.informativeText = """
        This stops the helper, removes it from login, and deletes the app.

        Afterwards the Kindle can't tell when you're using this Mac (Auto falls back to \
        the time of day), and Deck tiles won't open apps here. Your tasks stay safe on \
        the backend. You can reinstall it with menubar/install.sh.
        """
        a.addButton(withTitle: "Uninstall")
        a.addButton(withTitle: "Cancel")
        a.buttons[0].hasDestructiveAction = true
        guard a.runModal() == .alertFirstButtonReturn else { return }

        quitting = true
        stop()
        UserDefaults.standard.removePersistentDomain(forName: Bundle.main.bundleIdentifier ?? agentLabel)
        try? FileManager.default.removeItem(at: agentPlist)
        try? SMAppService.mainApp.unregister() // a downloaded copy's login item
        try? FileManager.default.removeItem(at: Bundle.main.bundleURL)
        // Take it out of launchd last: this ends this process too.
        let p = Process()
        p.executableURL = URL(fileURLWithPath: "/bin/launchctl")
        p.arguments = ["bootout", "gui/\(getuid())/\(agentLabel)"]
        try? p.run()
        p.waitUntilExit()
        NSApp.terminate(nil)
    }

    // Opening the app again (Finder, Spotlight, the Dock, `open`) shows the window: the
    // menu bar icon can be hidden behind the notch when the bar is full.
    @objc func openWindow() { showWindow() }

    // the menu bar icon: Desk Buddy at 18 pt (@1x and @2x), as a template image
    static let menuBarImage: NSImage? = {
        guard let one = Bundle.main.url(forResource: "menubar", withExtension: "png"),
              let img = NSImage(contentsOf: one) else { return nil }
        if let two = Bundle.main.url(forResource: "menubar@2x", withExtension: "png"),
           let rep = NSImage(contentsOf: two)?.representations.first {
            rep.size = img.size
            img.addRepresentation(rep)
        }
        img.isTemplate = true
        img.accessibilityDescription = "Desk Buddy"
        return img
    }()

    // MARK: the welcome

    // The first open: the welcome. After an update: macOS can drop permissions (this app is signed
    // ad hoc, so each build looks new to it); if one that was allowed isn't any more, the
    // welcome opens at the permissions step.
    func welcomeIfNeeded() {
        let d = UserDefaults.standard
        if !d.bool(forKey: "welcomed") {
            model.welcomeStep = 0
            showWindow()
            return
        }
        let build = Bundle.main.executableURL.flatMap { try? $0.resourceValues(forKeys: [.contentModificationDateKey]).contentModificationDate }
            .map { String($0.timeIntervalSince1970) } ?? ""
        let updated = d.string(forKey: "lastBuild") != build
        d.set(build, forKey: "lastBuild")
        permissions.refresh()
        // give the checks a few seconds (Automation can be slow to answer), then compare
        DispatchQueue.main.asyncAfter(deadline: .now() + 8) { [weak self] in
            guard let self else { return }
            let was = Set(d.stringArray(forKey: "allowedPerms") ?? [])
            let lost = self.model.perms.filter { was.contains($0.title) && !$0.ok && !$0.status.contains("Open ") && !$0.status.contains("Couldn't") }
            if updated && !lost.isEmpty && self.model.welcomeStep == nil {
                self.model.afterUpdate = true
                self.model.welcomeStep = 1
                self.showWindow()
            }
        }
    }

    // remember which permissions are allowed (to notice one dropped after an update)
    func rememberPermissions() {
        let ok = model.perms.filter(\.ok).map(\.title)
        guard !ok.isEmpty else { return }
        let d = UserDefaults.standard
        let was = Set(d.stringArray(forKey: "allowedPerms") ?? [])
        d.set(Array(was.union(ok)), forKey: "allowedPerms")
    }

    // Home and Kindle sections: the Kindle at a glance and its settings, from the backend
    // (every 10 s while the window is open).
    var currentList = "work"

    var offered: [String: Any]? // the newer version the server offers (its release.json)

    // checkUpdate asks GitHub (desk-buddy-latest.json, in every release since 1.2) and the server
    // for /mac/release.json, and keeps the newest (manual: from the button, says so if both fail)
    func checkUpdate(manual: Bool) {
        if model.updBusy { return }
        if manual { model.updChecking = true; model.updStatus = "" }
        let group = DispatchGroup()
        var found: [[String: Any]] = []
        var answered = false
        group.enter()
        Updater.fromGitHub { r in
            DispatchQueue.main.async {
                if let r { answered = true; found.append(r) }
                group.leave()
            }
        }
        group.enter()
        backend.send("GET", "/mac/release.json") { data in
            DispatchQueue.main.async {
                if let data, let r = (try? JSONSerialization.jsonObject(with: data)) as? [String: Any] { answered = true; found.append(r) }
                group.leave()
            }
        }
        group.notify(queue: .main) { [weak self] in
            guard let self else { return }
            self.model.updChecking = false
            guard answered else {
                if manual { self.model.updStatus = "Couldn't check for updates just now. Try again in a moment." }
                return
            }
            self.model.updChecked = DateFormatter.localizedString(from: Date(), dateStyle: .none, timeStyle: .short)
            self.model.updStatus = ""
            // the newest; one that can be installed wins a tie
            let r = found.max { a, b in
                let va = a["version"] as? String ?? "", vb = b["version"] as? String ?? ""
                return Updater.newer(vb, va) || (va == vb && a["url"] == nil && b["url"] != nil)
            } ?? [:]
            let v = r["version"] as? String ?? ""
            if Updater.newer(v, Updater.version) {
                self.offered = r
                self.model.updVersion = v
                let fresh = Updater.entries(r["patchnotes"]).filter { Updater.newer($0.version, Updater.version) }
                self.model.updNotes = fresh.isEmpty ? [.init(version: v, date: "", items: r["notes"] as? [String] ?? [])] : fresh
                self.model.updCanInstall = r["url"] is String && r["sha256"] is String
            } else {
                self.offered = nil
                self.model.updVersion = ""
            }
        }
    }

    // goBack installs the version before this one (About › Updates › Go back) from its GitHub release
    func goBack() {
        guard model.notes.count > 1, !model.updBusy else { return }
        let v = model.notes[1].version
        model.updBusy = true
        model.updStatus = "Finding version \(v)…"
        Updater.fromTag(v) { [weak self] r in
            DispatchQueue.main.async {
                guard let self else { return }
                guard let r else {
                    self.model.updBusy = false
                    self.model.updStatus = "Couldn't find version \(v) on GitHub. Download it from github.com/deskbuddy-app/desk-buddy/releases instead."
                    return
                }
                Updater.install(r, server: self.backend.address, key: self.backend.settings.key) { [weak self] s, busy in
                    self?.model.updStatus = s
                    self?.model.updBusy = busy
                }
            }
        }
    }

    func installUpdate() {
        guard let r = offered, !model.updBusy else { return }
        Updater.install(r, server: backend.address, key: backend.settings.key) { [weak self] s, busy in
            self?.model.updStatus = s
            self?.model.updBusy = busy
        }
    }

    // About › Start at login: the LaunchAgent (install.sh) or the login item (downloaded copies)
    static var startsAtLogin: Bool {
        FileManager.default.fileExists(atPath: agentPlist.path) || SMAppService.mainApp.status == .enabled
    }

    static func setStartAtLogin(_ on: Bool) {
        UserDefaults.standard.set(!on, forKey: "noLoginItem")
        if on {
            if !FileManager.default.fileExists(atPath: agentPlist.path) { try? SMAppService.mainApp.register() }
        } else {
            try? SMAppService.mainApp.unregister()
            // launchd keeps today's copy running; without the file it won't start it at the next login
            try? FileManager.default.removeItem(at: agentPlist)
        }
    }

    func refreshHome() {
        backend.get("/api/desk") { [weak self] data in
            guard let data, let d = try? JSONSerialization.jsonObject(with: data) as? [String: Any] else { return }
            DispatchQueue.main.async { self?.showDesk(d) }
        }
        backend.get("/api/kindle/status") { [weak self] data in
            guard let data, let k = try? JSONSerialization.jsonObject(with: data) as? [String: Any] else { return }
            DispatchQueue.main.async { self?.showKindle(k) }
        }
    }

    static func hm(_ t: Double) -> String {
        let f = DateFormatter()
        f.dateFormat = "HH:mm"
        return f.string(from: Date(timeIntervalSince1970: t))
    }

    func showDesk(_ d: [String: Any]) {
        let names = ["work": "Mac", "home": "Windows"]
        let profile = d["profile"] as? String ?? ""
        let comps = d["computers"] as? [String]
        model.computers = comps
        let one = (d["settings"] as? [String: Any])?["task_lists"] as? String == "one"
        if profile == "night" {
            model.showing = "night clock"
        } else {
            currentList = profile == "home" ? "home" : "work"
            model.showing = one ? "task list" : "\(names[currentList] ?? "Mac") list"
        }
        let all = d["tasks"] as? [[String: Any]] ?? []
        func isDone(_ x: [String: Any]) -> Bool { (x["done"] as? Bool) == true || (x["done"] as? Int) == 1 }
        func item(_ x: [String: Any], sub: Bool) -> HelperModel.TaskItem {
            let id = x["id"] as? Int ?? 0
            return .init(id: id, text: x["text"] as? String ?? "", done: isDone(x),
                         pinned: (x["pinned"] as? Bool) == true || (x["pinned"] as? Int) == 1, sub: sub,
                         parent: x["parent_id"] as? Int, kids: all.filter { ($0["parent_id"] as? Int) == id }.count)
        }
        let tasks = all.filter { $0["parent_id"] is NSNull || $0["parent_id"] == nil }
        // each task followed by its subtasks, in the Kindle's order
        model.tasks = tasks.flatMap { p in
            [item(p, sub: false)] + all.filter { ($0["parent_id"] as? Int) == (p["id"] as? Int) }.map { item($0, sub: true) }
        }
        let done = tasks.filter(isDone).count
        model.tasksLine = tasks.isEmpty ? "" : "\(done) of \(tasks.count) done"
        if let m = d["music"] as? [String: Any], m["running"] as? Bool != false, let t = m["title"] as? String, !t.isEmpty {
            let app = (m["app"] as? String) == "Music" ? "Apple Music" : (m["app"] as? String ?? "")
            let artist = (m["artist"] as? String ?? "")
            model.nowPlaying = t + (artist.isEmpty ? "" : " · " + artist) + " (" + app + ((m["playing"] as? Bool) == true ? ")" : ", paused)")
        } else {
            model.nowPlaying = ""
        }
        let now = Date().timeIntervalSince1970
        let next = (d["agenda"] as? [[String: Any]] ?? []).first {
            ($0["all_day"] as? Bool) != true && (($0["end"] as? Double) ?? 0) > now && Calendar.current.isDateInToday(Date(timeIntervalSince1970: ($0["start"] as? Double) ?? 0))
        }
        if let e = next, let st = e["start"] as? Double, let en = e["end"] as? Double {
            model.nextEvent = (e["title"] as? String ?? "") + " · " + App.hm(st) + "–" + App.hm(en) + (st <= now ? " (now)" : "")
        } else {
            model.nextEvent = ""
        }
        model.settings = d["settings"] as? [String: Any] ?? [:]
        model.kindleSettings = (d["kindle_settings"] as? [String: Any])?["values"] as? [String: Any]
        model.chime = ((d["pomodoro"] as? [String: Any])?["sound"] as? Bool) ?? true
        updateNeeds()
    }

    var kindleStatus: [String: Any] = [:]

    func showKindle(_ k: [String: Any]) {
        kindleStatus = k
        guard (k["seen"] as? Bool) == true, let at = k["at"] as? Double else {
            model.kindleLine = ""
            model.kindleOnline = false
            updateNeeds()
            return
        }
        let ago = Date().timeIntervalSince1970 - at
        model.kindleSeen = ago < 600
        model.kindleOnline = ago < 200 // the server notes a quiet Kindle every 2 minutes
        var parts: [String] = []
        if let b = k["battery"] as? Int, b >= 0 { parts.append("\(b)%" + ((k["charging"] as? Bool) == true ? " · charging" : "")) }
        parts.append(ago < 180 ? "seen just now" : ago < 3600 ? "seen \(Int(ago / 60)) min ago" : "last seen \(App.hm(at))")
        if let v = k["version"] as? String, !v.isEmpty { parts.append("version " + v) }
        model.kindleLine = parts.joined(separator: " · ")
        model.kindleUpdate = (k["up_to_date"] as? Bool) == false && !((k["sha"] as? String) ?? "").isEmpty
        updateNeeds()
    }

    // What needs the user, at the top of Home
    func updateNeeds() {
        var n: [String] = []
        if model.kindleUpdate { n.append("A new Desk Buddy version is ready: on the Kindle, tap Update (or Settings › Updates).") }
        if !model.kindleSeen, let at = kindleStatus["at"] as? Double {
            n.append("The Kindle hasn't checked in since \(App.hm(at)). Is it on, and on the Wi-Fi?")
        }
        for p in model.perms where p.status.hasPrefix("Not allowed") {
            n.append("macOS isn't allowing “\(p.title)”: see Permissions.")
        }
        if paused { n.append("The helper is paused: the Kindle won't follow this Mac. Turn it on in This Mac.") }
        model.needs = n
    }

    func windowWillClose(_ n: Notification) {
        NSApp.setActivationPolicy(.accessory)
    }

    func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows flag: Bool) -> Bool {
        showWindow()
        return false
    }

    var window: NSWindow?

    // The window, in the Kindle's style (HelperUI.swift).
    func showWindow() {
        if window == nil {
            model.onPause = { [weak self] p in if self?.paused != p { self?.togglePause() } }
            model.onMode = { [weak self] m in self?.pickMode(m) }
            model.onCloudSetup = { [weak self] in
                do { try self?.backend.runCloudSetup(); self?.model.cloudNote = "" }
                catch { self?.model.cloudNote = "Couldn't start the setup: \(error.localizedDescription)" }
            }
            model.onCloudPrompt = { [weak self] in
                do { _ = try self?.backend.prepareCloudKit() } catch {}
                NSPasteboard.general.clearContents()
                NSPasteboard.general.setString(CloudGuide.prompt, forType: .string)
                self?.model.cloudNote = "Prompt copied. Paste it into your AI helper."
            }
            model.onConnectComputer = { [weak self] in
                guard let self else { return }
                self.backend.post("/api/connect/offer", [:]) { data in
                    let code = data.flatMap { try? JSONSerialization.jsonObject(with: $0) as? [String: Any] }?["code"] as? String
                    DispatchQueue.main.async {
                        self.model.connectCode = code ?? ""
                        self.model.connectServer = self.backend.local ? self.backend.lanAddress : self.backend.settings.cloud
                        if code == nil { self.model.status = "Couldn't make a code: is the server reachable?" }
                        DispatchQueue.main.asyncAfter(deadline: .now() + 600) { // gone after 10 minutes
                            if self.model.connectCode == code { self.model.connectCode = "" }
                        }
                    }
                }
            }
            model.onCopyAI = { [weak self] in
                NSPasteboard.general.clearContents()
                NSPasteboard.general.setString(App.aiInstructions(), forType: .string)
                self?.model.aiNote = "Copied. Paste it into ~/.claude/CLAUDE.md (Claude Code) or ~/.codex/AGENTS.md (Codex)."
            }
            model.onCloudScript = { [weak self] in // to read, never to run: TextEdit
                guard let kit = try? self?.backend.prepareCloudKit() else { return }
                NSWorkspace.shared.open([kit.appendingPathComponent("setup.sh")], withApplicationAt: URL(fileURLWithPath: "/System/Applications/TextEdit.app"), configuration: NSWorkspace.OpenConfiguration())
            }
            model.onCloudConnect = { [weak self] url in
                guard let self else { return }
                self.model.cloudNote = "Checking…"
                self.backend.checkCloud(url) { err in
                    if let err { self.model.cloudNote = err; return }
                    self.backend.setCloud(url)
                    self.model.cloudNote = "Connected. Moving your tasks to the cloud…"
                    self.switchMode("cloud", copy: true) // the Kindle follows
                }
            }
            model.onSyncCalendar = { [weak self] in self?.syncCalendar() }
            model.onOpenPage = { [weak self] in self?.openPage() }
            model.onPermission = { [weak self] i in self?.permissions.tapped(i) }
            model.onQuit = { [weak self] in self?.quit() }
            model.onUninstall = { [weak self] in self?.uninstall() }
            model.onAddTask = { [weak self] text in
                guard let self else { return }
                self.backend.post("/api/tasks", ["profile": self.currentList, "text": text]) { [weak self] _ in
                    DispatchQueue.main.async { self?.refreshHome() }
                }
            }
            model.onSetting = { [weak self] key, value in
                self?.model.settings[key] = value // straight away; the backend confirms on the next refresh
                self?.backend.post("/api/settings", ["key": key, "value": value]) { [weak self] _ in
                    DispatchQueue.main.async { self?.refreshHome() }
                }
            }
            model.onKindleSetting = { [weak self] key, value in
                self?.model.kindleSettings?[key] = value // the Kindle picks it up within ~10 s
                self?.backend.post("/api/kindle/settings", ["values": [key: value]])
            }
            model.onWelcomeDone = { [weak self] in
                UserDefaults.standard.set(true, forKey: "welcomed")
                self?.rememberPermissions()
                self?.model.welcomeStep = nil
                self?.model.afterUpdate = false
                self?.model.section = "home"
            }
            model.onTask = { [weak self] act, id, text in
                guard let self else { return }
                // straight away here; the backend confirms with the next refresh
                if let i = self.model.tasks.firstIndex(where: { $0.id == id }) {
                    switch act {
                    case "toggle": self.model.tasks[i].done.toggle()
                    case "rename": self.model.tasks[i].text = text
                    case "delete": self.model.tasks.removeAll { $0.id == id }
                    default: break
                    }
                }
                let back: (Data?) -> Void = { [weak self] _ in DispatchQueue.main.async { self?.refreshHome() } }
                switch act {
                case "toggle": self.backend.send("POST", "/api/tasks/\(id)/toggle", nil, back)
                case "pin":
                    let pinned = self.model.tasks.first { $0.id == id }?.pinned == true
                    self.backend.send("POST", pinned ? "/api/unpin/\(self.currentList)" : "/api/tasks/\(id)/pin", nil, back)
                case "up", "down": self.backend.send("POST", "/api/tasks/\(id)/move/\(act)", nil, back)
                case "rename": self.backend.send("PATCH", "/api/tasks/\(id)", ["text": text], back)
                case "delete": self.backend.send("DELETE", "/api/tasks/\(id)", nil, back)
                case "addsub": self.backend.post("/api/tasks", ["profile": self.currentList, "text": text, "parent_id": id], back)
                case "place": // "parent|before", either empty
                    let p = text.split(separator: "|", omittingEmptySubsequences: false).map { Int($0) }
                    self.backend.post("/api/tasks/\(id)/place", ["parent_id": (p.first ?? nil) as Any? ?? NSNull(), "before": (p.count > 1 ? p[1] : nil) as Any? ?? NSNull()], back)
                default: break
                }
            }
            model.onChime = { [weak self] on in
                self?.model.chime = on
                self?.backend.post("/api/pomodoro", ["action": "sound", "on": on])
            }
            model.onCheckUpdate = { [weak self] in self?.checkUpdate(manual: true) }
            model.onInstallUpdate = { [weak self] in self?.installUpdate() }
            model.onGoBack = { [weak self] in self?.goBack() }
            let w = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 880, height: 660),
                             styleMask: [.titled, .closable, .miniaturizable, .resizable, .fullSizeContentView], backing: .buffered, defer: false)
            w.title = "Desk Buddy"
            w.titleVisibility = .hidden // the window's own "Desk Buddy" bar is the title
            w.titlebarAppearsTransparent = true
            w.isReleasedWhenClosed = false
            w.delegate = self
            w.minSize = NSSize(width: 760, height: 480)
            w.contentView = NSHostingView(rootView: HelperWindow(model: model))
            w.center()
            window = w
        }
        refresh()
        // a normal app while the window is open (Dock, Cmd-Tab), so it comes back after a macOS
        // prompt (Bluetooth…) instead of getting lost behind other windows; menu bar only again on close
        NSApp.setActivationPolicy(.regular)
        NSApp.activate(ignoringOtherApps: true)
        window?.makeKeyAndOrderFront(nil)
        if perfTimer == nil {
            perfTimer = Timer.scheduledTimer(withTimeInterval: 10, repeats: true) { [weak self] _ in self?.measure() }
            // permissions change in System Settings or a macOS prompt: look again when this app
            // comes back to the front, and every 15 s while the window shows (was every 3 s)
            NotificationCenter.default.addObserver(forName: NSApplication.didBecomeActiveNotification, object: nil, queue: .main) { [weak self] _ in
                if self?.window?.isVisible == true { self?.permissions.refresh(); self?.rememberPermissions() }
            }
            Timer.scheduledTimer(withTimeInterval: 15, repeats: true) { [weak self] _ in
                if self?.window?.isVisible == true { self?.permissions.refresh(); self?.rememberPermissions() }
            }
            Timer.scheduledTimer(withTimeInterval: 30, repeats: true) { [weak self] _ in
                if self?.window?.isVisible == true { self?.refreshHome() }
            }
        }
        measure()
        refreshHome()
    }

    func applicationWillTerminate(_ n: Notification) {
        quitting = true
        stop()
        backend.stopServer()
    }
}

let app = NSApplication.shared
let delegate = App()
app.delegate = delegate
// Stopped by launchd (install.sh, logout): take helper.py and the local server down too.
signal(SIGTERM, SIG_IGN)
let term = DispatchSource.makeSignalSource(signal: SIGTERM, queue: .main)
term.setEventHandler { NSApp.terminate(nil) }
term.resume()
app.setActivationPolicy(.accessory) // menu bar only, no Dock icon
app.run()
