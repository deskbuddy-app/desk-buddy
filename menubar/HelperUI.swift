// The helper's window, in the Kindle's own style (Settings on the Kindle): a top bar, a
// sidebar of sections (the open one inverted), black-and-white segmented choices and pill
// buttons, Atkinson Hyperlegible, short plain wording. Dark mode swaps black and white.
//
// HelperModel holds what the window shows; App (main.swift) fills it in and does the actions.

import AppKit
import SwiftUI

final class HelperModel: ObservableObject {
    struct PermRow: Identifiable {
        let id: Int
        let title, detail: String
        var status = "Checking…"
        var ok = false
        var button = "Allow…"
    }

    @Published var section = "home"
    // the welcome (first open, or after an update when macOS dropped a permission): its step,
    // nil when it's not showing; afterUpdate: say why it's back
    @Published var welcomeStep: Int? = nil
    @Published var afterUpdate = false
    // Home: the Kindle at a glance (from /api/desk and /api/kindle/status, every 10 s while open)
    @Published var showing = ""        // "Mac list"
    @Published var kindleLine = ""     // "86% · charging · seen just now"
    @Published var kindleSeen = true   // checked in within 10 minutes
    @Published var kindleOnline = false // checked in within the last ~3 minutes (Home's badge)
    @Published var kindleUpdate = false // an update is downloaded, waiting for a tap on the Kindle
    @Published var nowPlaying = ""
    @Published var nextEvent = ""
    @Published var tasksLine = ""
    struct TaskItem: Identifiable, Equatable {
        let id: Int
        var text: String
        var done: Bool
        var pinned: Bool
        var sub: Bool // a subtask (shown under its task)
        var parent: Int? = nil
        var kids = 0 // how many subtasks it has
    }
    @Published var tasks: [TaskItem] = [] // the list the Kindle shows, parents each followed by their subtasks
    @Published var needs: [String] = [] // things that need the user, shown at the top of Home
    // Kindle: its settings (backend ones, and the Kindle's own; nil = this Kindle can't sync them yet)
    @Published var settings: [String: Any] = [:]
    @Published var kindleSettings: [String: Any]? = nil
    @Published var chime = true
    @Published var status = "Starting…"
    @Published var lastDeck = ""
    @Published var calendar = ""
    @Published var paused = false
    @Published var local = false
    @Published var lanAddress = ""
    @Published var serverLine = ""
    @Published var perf = "Measuring…"
    @Published var perfCPU: Double? = nil // % of one core, averaged over the last minute
    @Published var perfEnergy: Double = 0 // Activity Monitor's Energy Impact
    @Published var perfMem: Double = 0   // MB
    @Published var perfServerMem: Double = 0 // MB of that for the server on this Mac (local mode), else 0
    @Published var perfParts = ""
    @Published var perfWarming = true // the first 90 s after the app starts (busy syncing)
    @Published var perms: [PermRow] = []
    @Published var version = ""
    @Published var computers: [String]? = nil // with the helper: "mac", "pc" (nil: not known yet)
    // About › Updates (Updater.swift): what came in each version, newest first (bundled), and
    // what the server offers
    struct NoteEntry: Identifiable { var id: String { version }; let version, date: String; let items: [String] }
    @Published var notes: [NoteEntry] = []
    @Published var notesPage = 0
    @Published var updVersion = ""          // a newer version on the server, "" when up to date
    @Published var updNotes: [NoteEntry] = [] // what's new in it (versions newer than this one)
    @Published var updCanInstall = false    // it comes with a download
    @Published var updStatus = ""           // "Downloading…", "Couldn't reach…"
    @Published var updBusy = false
    @Published var updChecked = ""          // "00:52": the last check that got an answer
    @Published var updChecking = false
    @Published var whatsNew = false
    var onCheckUpdate: () -> Void = {}
    @Published var startAtLogin = true
    var onStartAtLogin: (Bool) -> Void = { _ in }
    // Putting Desk Buddy on a Kindle over USB (KindleSetup.swift): "waiting" (no Kindle plugged
    // in), "found", "copying", "checking", "ejecting", "done", "failed"
    @Published var kSetupState = "waiting"
    @Published var kSetupDrive = ""
    @Published var kSetupHas = false // Desk Buddy is on it already
    @Published var kSetupNote = ""
    var onKindleInstall: () -> Void = {}
    var onKindleSaveFiles: () -> Void = {}
    var onInstallUpdate: () -> Void = {}
    @Published var goBackAsk = false // "Go back to version x?" open
    var onGoBack: () -> Void = {}

    var onPause: (Bool) -> Void = { _ in }
    var onMode: (String) -> Void = { _ in }
    var onSyncCalendar: () -> Void = {}
    var onOpenPage: () -> Void = {}
    var onPermission: (Int) -> Void = { _ in }
    var onQuit: () -> Void = {}
    var onUninstall: () -> Void = {}
    var onAddTask: (String) -> Void = { _ in }
    var onSetting: (String, Any) -> Void = { _, _ in }       // backend settings (music, focus, tasks)
    var onKindleSetting: (String, Any) -> Void = { _, _ in } // the Kindle's own (screen, heads-up…)
    var onChime: (Bool) -> Void = { _ in }
    // ("toggle" | "delete" | "pin" | "up" | "down" | "rename" | "addsub" | "place", id, text)
    // place: text is "parent|before" (either may be empty: top level / at the end)
    var onTask: (String, Int, String) -> Void = { _, _, _ in }
    var onWelcomeDone: () -> Void = {}
    // Server › Your own cloud server
    @Published var nodeInstalled = false
    @Published var phoneURL = "" // the phone page with the key, for the QR code (Server)
    @Published var cloudNote = ""
    var onCloudSetup: () -> Void = {}            // open Terminal running setup.sh
    var onCloudPrompt: () -> Void = {}           // copy the AI prompt
    var onCloudScript: () -> Void = {}           // open setup.sh in TextEdit, to read
    var onCopyAI: () -> Void = {}                // copy the instructions for AI assistants (This Mac)
    var onConnectComputer: () -> Void = {}       // a one-time code for connecting another computer
    @Published var connectCode = ""              // "AB12-CD34" while one is showing
    @Published var connectServer = ""            // the address to type next to it
    @Published var aiNote = ""
    var onCloudConnect: (String) -> Void = { _ in } // check the address, then switch to it
}

// MARK: the Kindle look

enum KD {
    static func registerFonts() {
        for name in ["AtkinsonHyperlegible-Regular", "AtkinsonHyperlegible-Bold"] {
            if let url = Bundle.main.url(forResource: name, withExtension: "ttf") {
                CTFontManagerRegisterFontsForURL(url as CFURL, .process, nil)
            }
        }
    }

    static func font(_ size: CGFloat, bold: Bool = false) -> Font {
        let name = bold ? "AtkinsonHyperlegible-Bold" : "AtkinsonHyperlegible-Regular"
        return NSFont(name: name, size: size) != nil ? .custom(name, size: size) : .system(size: size, weight: bold ? .bold : .regular)
    }
}

struct Ink {
    let scheme: ColorScheme
    // dark mode: a soft near-black (#161616) rather than pure black, and off-white text
    var ink: Color { scheme == .dark ? Color(white: 0.93) : .black }
    var paper: Color { scheme == .dark ? Color(white: 0.086) : .white }
    var grey: Color { scheme == .dark ? Color(white: 0.66) : Color(white: 0.3) }
    var line: Color { scheme == .dark ? Color(white: 0.24) : Color(white: 0.85) }
    var hover: Color { scheme == .dark ? Color(white: 0.14) : Color(white: 0.955) } // a row under the pointer
}

// "UPDATES": small bold capitals, spaced out, grey
struct SpacedLabel: View {
    @Environment(\.colorScheme) var scheme
    let text: String
    var body: some View {
        Text(text.uppercased()).font(KD.font(10, bold: true)).tracking(1.5).foregroundColor(Ink(scheme: scheme).grey)
    }
}

// On | Off: the chosen one filled black
struct Segmented: View {
    @Environment(\.colorScheme) var scheme
    let labels: [String]
    let selected: Int
    var enabled = true
    let pick: (Int) -> Void
    var body: some View {
        let c = Ink(scheme: scheme)
        HStack(spacing: 0) {
            ForEach(labels.indices, id: \.self) { i in
                if i > 0 { Rectangle().fill(c.ink).frame(width: 1) }
                Button { pick(i) } label: {
                    Text(labels[i]).font(KD.font(12, bold: true))
                        .foregroundColor(i == selected ? c.paper : c.ink)
                        .frame(minWidth: 50, maxWidth: .infinity, maxHeight: .infinity)
                        .padding(.horizontal, 9)
                        .background(i == selected ? c.ink : c.paper)
                        .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityAddTraits(i == selected ? .isSelected : [])
            }
        }
        .frame(height: 26)
        .fixedSize(horizontal: true, vertical: false)
        .clipShape(RoundedRectangle(cornerRadius: 6))
        .overlay(RoundedRectangle(cornerRadius: 6).stroke(c.ink, lineWidth: 1))
        .opacity(enabled ? 1 : 0.35)
        .disabled(!enabled)
    }
}

// a button: outlined, or solid black for the main action
struct Pill: View {
    @Environment(\.colorScheme) var scheme
    let title: String
    var solid = false
    let action: () -> Void
    var body: some View {
        let c = Ink(scheme: scheme)
        Button(action: action) {
            Text(title).font(KD.font(12, bold: true))
                .foregroundColor(solid ? c.paper : c.ink)
                .padding(.horizontal, 12).frame(minWidth: 64, minHeight: 26)
                .background(RoundedRectangle(cornerRadius: 6).fill(solid ? c.ink : c.paper))
                .overlay(RoundedRectangle(cornerRadius: 6).stroke(c.ink, lineWidth: 1))
                .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
    }
}

// a status: green when it's done ("✓ Allowed"), otherwise outlined and grey
struct Badge: View {
    @Environment(\.colorScheme) var scheme
    let text: String
    let ok: Bool
    static let green = Color(red: 0.12, green: 0.56, blue: 0.27)
    var body: some View {
        let c = Ink(scheme: scheme)
        Text(ok ? "✓ " + text : text).font(KD.font(11, bold: true))
            .foregroundColor(ok ? .white : c.grey)
            .padding(.horizontal, 8).padding(.vertical, 2)
            .background(Capsule().fill(ok ? Badge.green : .clear))
            .overlay(Capsule().stroke(ok ? Badge.green : c.line, lineWidth: 1.5))
    }
}

// QR codes (CoreImage): black on white, sharp when scaled
enum QR {
    static func image(_ text: String) -> NSImage? {
        guard !text.isEmpty, let f = CIFilter(name: "CIQRCodeGenerator") else { return nil }
        f.setValue(Data(text.utf8), forKey: "inputMessage")
        f.setValue("M", forKey: "inputCorrectionLevel")
        guard let out = f.outputImage?.transformed(by: CGAffineTransform(scaleX: 8, y: 8)) else { return nil }
        let rep = NSCIImageRep(ciImage: out)
        let img = NSImage(size: rep.size)
        img.addRepresentation(rep)
        return img
    }
}

// Server › Your own cloud server: an optional, foldable step-by-step guide to putting Desk Buddy
// on the user's own free Cloudflare account (setup.sh does the work), or a prompt for an AI.
struct CloudGuide: View {
    @Environment(\.colorScheme) var scheme
    @ObservedObject var model: HelperModel
    @AppStorage("cloudGuideFolded") var folded = false
    @State var address = ""

    var body: some View {
        let c = Ink(scheme: scheme)
        VStack(alignment: .leading, spacing: 0) {
            Button { withAnimation(.easeOut(duration: 0.15)) { folded.toggle() } } label: {
                HStack(alignment: .center, spacing: 10) {
                    Image(systemName: "cloud").font(.system(size: 15, weight: .medium)).foregroundColor(c.ink)
                    VStack(alignment: .leading, spacing: 2) {
                        Text("Your own cloud server").font(KD.font(14, bold: true)).foregroundColor(c.ink)
                        Text("Optional · free on Cloudflare · about 10 minutes").font(KD.font(11)).foregroundColor(c.grey)
                    }
                    Spacer()
                    Image(systemName: folded ? "chevron.down" : "chevron.up").font(.system(size: 11, weight: .semibold)).foregroundColor(c.grey)
                }
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .help(folded ? "Show the steps" : "Hide the steps")
            if !folded {
                Text("Lets the Kindle and your phone work while this Mac sleeps, and from anywhere. It runs in your own Cloudflare account: nobody else's server is involved.")
                    .font(KD.font(12)).foregroundColor(c.grey).fixedSize(horizontal: false, vertical: true).padding(.top, 10)
                step(1, "Make a free Cloudflare account", "An email and a password. The free plan is plenty.") {
                    Pill(title: "Open Cloudflare") { NSWorkspace.shared.open(URL(string: "https://dash.cloudflare.com/sign-up")!) }
                }
                step(2, "Install Node.js", "The setup needs it. Pick the LTS version.") {
                    if model.nodeInstalled { Badge(text: "Installed", ok: true) }
                    else { Pill(title: "Get Node.js") { NSWorkspace.shared.open(URL(string: "https://nodejs.org")!) } }
                }
                step(3, "Put your server online", "Terminal opens and does the rest; your browser opens once to log in to Cloudflare. At the end the address is copied.") {
                    Pill(title: "Run in Terminal", solid: true) { model.onCloudSetup() }
                }
                codeBox(c)
                step(4, "Paste the address", "It ends in .workers.dev. Your tasks are copied over, and the Kindle follows.") { EmptyView() }
                HStack(spacing: 10) {
                    TextField("https://desk-buddy.….workers.dev", text: $address)
                        .textFieldStyle(.plain).font(KD.font(13)).foregroundColor(c.ink)
                        .padding(.vertical, 6).overlay(Rectangle().fill(c.ink).frame(height: 1), alignment: .bottom)
                        .onSubmit { model.onCloudConnect(address) }
                    Pill(title: "Connect", solid: true) { model.onCloudConnect(address) }
                }
                .padding(.leading, 34).padding(.top, 2)
                if !model.cloudNote.isEmpty {
                    Text(model.cloudNote).font(KD.font(12, bold: true)).foregroundColor(c.ink).padding(.leading, 34).padding(.top, 6)
                }
                Rectangle().fill(c.line).frame(height: 1).padding(.top, 16)
                HStack(alignment: .center, spacing: 14) {
                    VStack(alignment: .leading, spacing: 2) {
                        Text("Rather let an AI do it?").font(KD.font(13, bold: true)).foregroundColor(c.ink)
                        Text("Paste the prompt into Claude Code or Codex: they can run every step for you. ChatGPT or Claude in the browser can walk you through it, but you run the commands. Your key is never in the prompt.")
                            .font(KD.font(11)).foregroundColor(c.grey).fixedSize(horizontal: false, vertical: true)
                    }
                    Spacer(minLength: 8)
                    Pill(title: "Copy prompt") { model.onCloudPrompt() }
                }
                .padding(.top, 10)
            }
        }
        .padding(14)
        .overlay(RoundedRectangle(cornerRadius: 8).stroke(c.line, lineWidth: 1))
    }

    // What step 3 runs, so nothing is a surprise: the command, and the Cloudflare commands inside.
    static let command = "cd ~/Library/Application\\ Support/DeskBuddy/cloud-setup\nsh setup.sh"
    static let inside = "npm install\nnpx wrangler login\nnpx wrangler d1 create desk-buddy\nnpx wrangler d1 execute desk-buddy --remote --file schema.sql\nnpx wrangler deploy\nnpx wrangler secret put KEY"

    func codeBox(_ c: Ink) -> some View {
        let mono = Font.system(size: 11.5, design: .monospaced)
        return VStack(alignment: .leading, spacing: 6) {
            Text("What runs in Terminal:").font(KD.font(11)).foregroundColor(c.grey)
            Text(CloudGuide.command).font(mono).foregroundColor(c.ink).textSelection(.enabled)
            Text("Inside, setup.sh gets Cloudflare's own tool (Wrangler) and runs:").font(KD.font(11)).foregroundColor(c.grey).padding(.top, 4)
            Text(CloudGuide.inside).font(mono).foregroundColor(c.ink).textSelection(.enabled)
            HStack(spacing: 8) {
                Pill(title: "Copy") {
                    NSPasteboard.general.clearContents()
                    NSPasteboard.general.setString(CloudGuide.command, forType: .string)
                }
                Pill(title: "Open the script") { model.onCloudScript() }
            }
            .padding(.top, 4)
        }
        .padding(12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 6).fill(c.line.opacity(0.35)))
        .padding(.leading, 34).padding(.top, 8)
    }

    func step<T: View>(_ n: Int, _ title: String, _ hint: String, @ViewBuilder trailing: () -> T) -> some View {
        let c = Ink(scheme: scheme)
        return HStack(alignment: .center, spacing: 12) {
            Image(systemName: "\(n).circle").font(.system(size: 15, weight: .semibold)).foregroundColor(c.ink).frame(width: 22)
            VStack(alignment: .leading, spacing: 2) {
                Text(title).font(KD.font(13, bold: true)).foregroundColor(c.ink)
                Text(hint).font(KD.font(11)).foregroundColor(c.grey).fixedSize(horizontal: false, vertical: true)
            }
            Spacer(minLength: 8)
            trailing()
        }
        .padding(.top, 14)
    }

    // The prompt for an AI helper. Works for tools that run commands (Claude Code, Codex) and for
    // chat-only ones (they walk the user through). The key stays in the kit's private file.
    static let prompt = """
    I'm setting up Desk Buddy (an app that turns a Kindle into a desk display) with its own free cloud server on Cloudflare. Please help me do it on this Mac.

    The Desk Buddy app has prepared everything in this folder:
    ~/Library/Application Support/DeskBuddy/cloud-setup
    It holds setup.sh (the setup script), the server's code, and a private file named "key". Don't open, print or copy the key file: setup.sh reads it by itself.

    Steps:
    1. Check Node.js is installed (node --version). If it isn't, tell me and help me install the LTS version from nodejs.org. Ask before installing anything.
    2. Make sure I have a Cloudflare account. If I don't, I'll make a free one at https://dash.cloudflare.com/sign-up
    3. In that folder, run: sh setup.sh
       It logs in to Cloudflare (my browser opens once and I click Allow), makes a database, puts the server online and locks it with my key.
    4. When it finishes it prints an address ending in .workers.dev. Give me that address: I'll paste it into Desk Buddy › Server › Your own cloud server and click Connect.

    Only work in that folder, and only run setup.sh and the commands it needs. If something fails, explain it in plain words and suggest the fix.
    If you can't run commands on my Mac, walk me through these steps one at a time and tell me exactly what to type into Terminal.
    """
}

// Put Desk Buddy on a Kindle: plug it in, one click (welcome + Kindle page)
struct KindleSetupCard: View {
    @Environment(\.colorScheme) var scheme
    @ObservedObject var model: HelperModel
    var body: some View {
        let c = Ink(scheme: scheme)
        let st = model.kSetupState
        let (title, hint): (String, String) = {
            switch st {
            case "found": return ("Kindle found", model.kSetupHas
                ? "Desk Buddy is on it already. This puts the newest version on, set up to connect to this Mac."
                : "Ready to put Desk Buddy on it, set up to connect to this Mac. It needs the jailbreak with the hotfix (kindlemodding.org).")
            case "copying": return ("Copying onto the Kindle…", "Don't unplug it. About half a minute.")
            case "checking": return ("Checking every file landed…", "USB with Kindles sometimes drops files, so it reads them back.")
            case "ejecting": return ("Ejecting…", "Nearly there.")
            case "done": return ("Done: unplug the Kindle", (model.kSetupNote.isEmpty ? "" : model.kSetupNote + " ") + "Then open Desk Buddy in the Kindle's library. With the Kindle on your Wi-Fi, it connects to this Mac by itself.")
            case "failed": return ("That didn't work", model.kSetupNote)
            default: return ("Put Desk Buddy on a Kindle", "Plug your jailbroken Kindle into this Mac with USB (jailbreak with the hotfix: kindlemodding.org). Desk Buddy copies itself onto it and sets it up to connect here.")
            }
        }()
        VStack(alignment: .leading, spacing: 10) {
            HStack(alignment: .top, spacing: 12) {
                Image(systemName: st == "done" ? "checkmark.circle" : st == "failed" ? "exclamationmark.triangle" : "cable.connector")
                    .font(.system(size: 18)).foregroundColor(c.ink).frame(width: 22)
                VStack(alignment: .leading, spacing: 3) {
                    Text(title).font(KD.font(14, bold: true)).foregroundColor(c.ink)
                    Text(hint).font(KD.font(12)).foregroundColor(c.grey).fixedSize(horizontal: false, vertical: true)
                }
            }
            HStack(spacing: 8) {
                Spacer().frame(width: 26)
                if st == "found" { Pill(title: "Put Desk Buddy on it", solid: true) { model.onKindleInstall() } }
                if st == "failed" { Pill(title: "Try again", solid: true) { model.onKindleInstall() } }
                if ["waiting", "found", "failed"].contains(st) { Pill(title: "Save the files instead…") { model.onKindleSaveFiles() } }
            }
        }
        .padding(12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 8).stroke(c.line, lineWidth: 1))
    }
}

// patch notes: a thin outlined box (About › Updates)
struct NoteBox<Content: View>: View {
    @Environment(\.colorScheme) var scheme
    @ViewBuilder let content: () -> Content
    var body: some View {
        VStack(alignment: .leading, spacing: 6, content: content)
            .padding(.horizontal, 12).padding(.vertical, 10)
            .frame(maxWidth: .infinity, alignment: .leading)
            .overlay(RoundedRectangle(cornerRadius: 8).stroke(Ink(scheme: scheme).line, lineWidth: 1))
            .padding(.top, 10)
    }
}

// one version's notes: "Version 1.1 · 9 Oct 2026" and a bullet per item
struct NoteBlock: View {
    @Environment(\.colorScheme) var scheme
    let title, date: String
    let items: [String]
    var body: some View {
        let c = Ink(scheme: scheme)
        VStack(alignment: .leading, spacing: 4) {
            if !title.isEmpty {
                Text(title).font(KD.font(13, bold: true)).foregroundColor(c.ink)
                    + Text(date.isEmpty ? "" : "  ·  " + NoteBlock.nice(date)).font(KD.font(11)).foregroundColor(c.grey)
            }
            ForEach(items, id: \.self) { t in
                HStack(alignment: .firstTextBaseline, spacing: 7) {
                    Text("•").font(KD.font(12)).foregroundColor(c.ink)
                    Text(t).font(KD.font(12)).foregroundColor(c.ink).fixedSize(horizontal: false, vertical: true)
                }
            }
        }
    }
    static func nice(_ d: String) -> String {
        let f = DateFormatter(); f.dateFormat = "yyyy-MM-dd"
        guard let t = f.date(from: d) else { return d }
        return DateFormatter.localizedString(from: t, dateStyle: .medium, timeStyle: .none)
    }
}

// a setting: title and a short grey hint on the left, its control on the right, a line under
struct Row<Trailing: View>: View {
    @Environment(\.colorScheme) var scheme
    let title: String
    var hint = ""
    var badge: (text: String, ok: Bool)? = nil // a status under the hint (Permissions)
    @ViewBuilder let trailing: () -> Trailing
    var body: some View {
        let c = Ink(scheme: scheme)
        VStack(spacing: 0) {
            HStack(alignment: .center, spacing: 14) {
                VStack(alignment: .leading, spacing: 2) {
                    Text(title).font(KD.font(13, bold: true)).foregroundColor(c.ink)
                    if !hint.isEmpty {
                        Text(hint).font(KD.font(11)).foregroundColor(c.grey).fixedSize(horizontal: false, vertical: true)
                    }
                    if let b = badge { Badge(text: b.text, ok: b.ok).padding(.top, 3) }
                }
                Spacer(minLength: 8)
                trailing()
            }
            .padding(.vertical, 9)
            Rectangle().fill(c.line).frame(height: 1)
        }
    }
}

extension Row where Trailing == EmptyView {
    init(title: String, hint: String = "") { self.init(title: title, hint: hint) { EmptyView() } }
}

// Desk Buddy himself (black on clear, tinted to the text colour so he flips in dark mode)
struct Buddy: View {
    @Environment(\.colorScheme) var scheme
    var height: CGFloat
    static let image: NSImage? = Bundle.main.url(forResource: "buddy", withExtension: "png").flatMap { NSImage(contentsOf: $0) }
    var body: some View {
        if let img = Buddy.image {
            Image(nsImage: img).renderingMode(.template).resizable().aspectRatio(contentMode: .fit)
                .foregroundColor(Ink(scheme: scheme).ink).frame(height: height).accessibilityLabel("Desk Buddy")
        }
    }
}

// What each permission sends, and where (shown under it)
func permShares(_ title: String) -> String {
    switch title {
    case "Calendar": return "Shares: your upcoming events (title, time, place, notes) with your Desk Buddy server."
    case "Controlling Spotify", "Controlling Apple Music": return "Shares: the song playing and its cover, only while music shows on the Kindle."
    case "Spotify note": return "macOS often can't check this one: the music app doesn't always answer it. If your song shows on the Kindle, it's allowed."
    case "Bluetooth": return "Shares: the names of your speakers and headphones, so the Kindle can list them."
    default: return ""
    }
}

// A permission's icon: a calendar, Spotify's mark, the Apple logo, headphones
struct PermIcon: View {
    @Environment(\.colorScheme) var scheme
    let title: String
    static let spotify: NSImage? = Bundle.main.url(forResource: "spotify-mark", withExtension: "png").flatMap { NSImage(contentsOf: $0) }
    var body: some View {
        let c = Ink(scheme: scheme)
        Group {
            if title.contains("Spotify"), let img = PermIcon.spotify {
                Image(nsImage: img).renderingMode(.template).resizable().aspectRatio(contentMode: .fit).frame(width: 18, height: 18)
            } else {
                Image(systemName: title.contains("Apple Music") ? "apple.logo" : title == "Calendar" ? "calendar" : "headphones")
                    .font(.system(size: 16, weight: .medium))
            }
        }
        .foregroundColor(c.ink).frame(width: 28, height: 28)
        .background(RoundedRectangle(cornerRadius: 7).stroke(c.line, lineWidth: 1))
        .padding(.top, 10)
    }
}

// The privacy promise, at the top of Permissions: true for both cloud and local mode.
struct PrivacyCard: View {
    @Environment(\.colorScheme) var scheme
    let local: Bool
    var body: some View {
        let c = Ink(scheme: scheme)
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 8) {
                Image(systemName: "lock.shield").font(.system(size: 15, weight: .semibold)).foregroundColor(c.ink)
                Text("Your data stays yours").font(KD.font(15, bold: true)).foregroundColor(c.ink)
            }
            Text(local
                 ? "Desk Buddy runs on this Mac. What the Kindle shows stays on this Mac and your Wi-Fi: nothing goes to the internet except the town for the weather."
                 : "What the Kindle shows goes only to your own Desk Buddy server, wherever you run it: your Cloudflare account, this Mac or another computer of yours. There's no Desk Buddy company server, and only someone with your key can read it.")
                .font(KD.font(12)).foregroundColor(c.grey).fixedSize(horizontal: false, vertical: true)
            HStack(spacing: 14) {
                ForEach(["No ads", "No tracking", "No analytics", "Nothing sold or shared"], id: \.self) { t in
                    HStack(spacing: 4) {
                        Image(systemName: "checkmark").font(.system(size: 9, weight: .heavy)).foregroundColor(Badge.green)
                        Text(t).font(KD.font(11, bold: true)).foregroundColor(c.ink)
                    }
                }
            }
            .padding(.top, 2)
            Text("Only what the Kindle needs to show is shared, and macOS asks you first. You can turn any of these off in System Settings at any time.")
                .font(KD.font(11)).foregroundColor(c.grey).fixedSize(horizontal: false, vertical: true)
        }
        .padding(14)
        .background(RoundedRectangle(cornerRadius: 8).stroke(c.line, lineWidth: 1))
    }
}

// MARK: the window

struct HelperWindow: View {
    @Environment(\.colorScheme) var scheme
    @ObservedObject var model: HelperModel
    var scrolls = true // previews (ImageRenderer) can't draw a ScrollView

    static let sections: [(id: String, name: String, hint: String, icon: String)] = [
        ("home", "Home", "Your tasks", "house"),
        ("kindle", "Kindle", "Its settings", "slider.horizontal.3"),
        ("status", "This Mac", "Helper, calendar", "laptopcomputer"),
        ("server", "Server", "Cloud or this Mac", "cloud"),
        ("permissions", "Permissions", "What macOS allows", "lock"),
        ("performance", "Performance", "How much it uses", "gauge.with.dots.needle.33percent"),
        ("about", "About", "Version, quit", "info.circle"),
    ]

    var body: some View {
        if let step = model.welcomeStep {
            WelcomeView(model: model, step: step, scrolls: scrolls)
        } else {
            main
        }
    }

    var main: some View {
        let c = Ink(scheme: scheme)
        return VStack(spacing: 0) {
            ZStack {
                HStack(spacing: 6) {
                    Buddy(height: 18)
                    Text("Desk Buddy").font(KD.font(15, bold: true)).foregroundColor(c.ink)
                }
                HStack {
                    Button { withAnimation(.easeOut(duration: 0.15)) { collapsed.toggle() } } label: {
                        Image(systemName: "sidebar.left").font(.system(size: 13, weight: .medium)).foregroundColor(c.grey)
                            .frame(width: 28, height: 24).contentShape(Rectangle())
                    }
                    .buttonStyle(.plain).help(collapsed ? "Show the sidebar" : "Hide the sidebar")
                    .accessibilityLabel(collapsed ? "Show the sidebar" : "Hide the sidebar")
                    Spacer()
                }
                .padding(.leading, 78) // clear of the window's close / minimise / zoom buttons
                HStack {
                    Spacer()
                    Button { model.onPause(!model.paused) } label: {
                        HStack(spacing: 5) {
                            Image(systemName: model.paused ? "play.fill" : "pause.fill").font(.system(size: 10, weight: .semibold))
                            Text(model.paused ? "Resume" : "Pause").font(KD.font(12, bold: true))
                        }
                        .foregroundColor(model.paused ? c.paper : c.ink)
                        .padding(.horizontal, 10).frame(height: 24)
                        .background(Capsule().fill(model.paused ? c.ink : Color.clear))
                        .overlay(Capsule().stroke(c.ink, lineWidth: 1))
                        .contentShape(Capsule())
                    }
                    .buttonStyle(.plain)
                    .help(model.paused ? "Connect this Mac to the Kindle again" : "Stop this Mac talking to the Kindle: no Deck, music, calendar or \"in use\" until you resume")
                    .accessibilityLabel(model.paused ? "Resume" : "Pause")
                }
                .padding(.trailing, 14)
            }
            .frame(maxWidth: .infinity).frame(height: 38)
            Rectangle().fill(c.line).frame(height: 1)
            HStack(spacing: 0) {
                sidebar(c)
                Rectangle().fill(c.line).frame(width: 1)
                let content = VStack(alignment: .leading, spacing: 0) { pane }
                    .padding(.horizontal, 24).padding(.vertical, 16)
                    .frame(maxWidth: .infinity, alignment: .leading)
                if scrolls {
                    ScrollView { content }
                } else {
                    content.frame(maxHeight: .infinity, alignment: .top)
                }
            }
        }
        .background(c.paper)
    }

    func sidebar(_ c: Ink) -> some View {
        VStack(spacing: 0) {
            ForEach(Self.sections, id: \.id) { s in
                let on = model.section == s.id
                let count = badge(s.id)
                Button { model.section = s.id } label: {
                    HStack(spacing: 10) {
                        Image(systemName: s.icon).font(.system(size: 13, weight: .semibold)).frame(width: 18)
                            .overlay(alignment: .topTrailing) { // folded sidebar: a dot on the icon
                                if collapsed && count > 0 { Circle().fill(Self.alert).frame(width: 7, height: 7).offset(x: 4, y: -3) }
                            }
                        if !collapsed {
                            Text(s.name).font(KD.font(13, bold: true))
                            Spacer()
                            if count > 0 { // something needs you here
                                Text("\(count)").font(KD.font(11, bold: true)).foregroundColor(.white)
                                    .frame(minWidth: 18, minHeight: 18).background(Capsule().fill(Self.alert))
                            }
                        }
                    }
                    .foregroundColor(on ? c.paper : c.ink)
                    .padding(.horizontal, collapsed ? 0 : 12).frame(maxWidth: .infinity).frame(height: 30)
                    .background(RoundedRectangle(cornerRadius: 6).fill(on ? c.ink : c.paper))
                    .padding(.horizontal, 8)
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .help(s.name + " · " + s.hint)
                .accessibilityLabel(s.name)
                .accessibilityAddTraits(on ? .isSelected : [])
            }
            Spacer()
        }
        .padding(.top, 10)
        .frame(width: collapsed ? 52 : 200)
    }

    @State private var newTask = ""

    static let alert = Color(red: 0.88, green: 0.27, blue: 0.22)

    // how many things need the user in a section: permissions not allowed, a Kindle update
    // waiting, a new Desk Buddy for Mac
    func badge(_ id: String) -> Int {
        switch id {
        case "permissions": return model.perms.filter { !$0.ok && !$0.status.hasPrefix("Couldn't") && !$0.status.hasPrefix("Checking") && !$0.status.hasPrefix("Open ") }.count
        case "kindle": return model.kindleUpdate ? 1 : 0
        case "about": return model.updVersion.isEmpty ? 0 : 1
        default: return 0
        }
    }
    @AppStorage("sidebarCollapsed") private var collapsed = false
    @State private var editing: Int? = nil   // the task being renamed
    @FocusState private var editFocus: Bool  // its text field (clicking away saves)
    @State private var editText = ""
    @State private var hovered: Int? = nil

    // a choice row for a setting: labels, their values, the current value
    func choice(_ title: String, _ hint: String, _ labels: [String], _ values: [Any], _ current: Any?, enabled: Bool = true, set: @escaping (Any) -> Void) -> some View {
        let sel = values.firstIndex { "\($0)" == "\(current ?? "")" } ?? 0
        return Row(title: title, hint: hint) {
            Segmented(labels: labels, selected: sel, enabled: enabled) { set(values[$0]) }
        }.opacity(enabled ? 1 : 0.45)
    }

    // Performance: a verdict in plain words, a light → heavy meter, and the three numbers
    // About › Updates, like the Kindle's Settings › Updates
    @ViewBuilder var updatesSection: some View {
        let ready = !model.updVersion.isEmpty
        let status = model.updBusy || (!model.updStatus.isEmpty && !ready) ? model.updStatus
            : ready ? (model.updCanInstall ? "Version \(model.updVersion) is ready. It installs when you click Install, then Desk Buddy restarts." + (model.updStatus.isEmpty ? "" : " " + model.updStatus)
                                           : "Version \(model.updVersion) is out. Download it from github.com/deskbuddy-app/desk-buddy.")
            : !model.updChecked.isEmpty ? "You're up to date (checked at \(model.updChecked))."
            : "Checked every 6 hours from your Desk Buddy server. It installs only when you click."
        SpacedLabel(text: "Updates").padding(.top, 8)
        Row(title: "Version \(model.version)", hint: status) {
            if model.updBusy {
                EmptyView()
            } else if ready {
                HStack(spacing: 8) {
                    Pill(title: "What's new?") { model.whatsNew.toggle() }
                    if model.updCanInstall { Pill(title: "Install", solid: true) { model.onInstallUpdate() } }
                }
            } else {
                Pill(title: model.updChecking ? "Checking…" : "Check for updates") { model.onCheckUpdate() }
            }
        }
        if model.notes.count > 1 && !model.updBusy {
            let prev = model.notes[1].version
            Row(title: model.goBackAsk ? "Go back to version \(prev)?" : "Go back to version \(prev)",
                hint: model.goBackAsk ? "Desk Buddy restarts, and the Kindle is offered the matching version. Your tasks and settings stay. You can update again any time."
                                      : "If this version gives you trouble") {
                if model.goBackAsk {
                    HStack(spacing: 8) {
                        Pill(title: "Cancel") { model.goBackAsk = false }
                        Pill(title: "Go back", solid: true) { model.goBackAsk = false; model.onGoBack() }
                    }
                } else {
                    Pill(title: "Go back…") { model.goBackAsk = true }
                }
            }
        }
        if ready && model.whatsNew && !model.updBusy {
            NoteBox {
                ForEach(model.updNotes) { e in NoteBlock(title: "Version \(e.version)", date: e.date, items: e.items) }
            }
        }
        if !model.notes.isEmpty {
            let i = max(0, min(model.notesPage, model.notes.count - 1))
            let e = model.notes[i]
            NoteBox {
                HStack(spacing: 8) {
                    NoteBlock(title: i == 0 ? "What's new in this version" : "What came in version \(e.version)", date: e.date, items: [])
                    Spacer()
                    Pill(title: "‹") { model.notesPage = i - 1 }.opacity(i > 0 ? 1 : 0.35).disabled(i == 0)
                    Pill(title: "›") { model.notesPage = i + 1 }.opacity(i < model.notes.count - 1 ? 1 : 0.35).disabled(i >= model.notes.count - 1)
                }
                NoteBlock(title: "", date: "", items: e.items)
            }
        }
    }

    @ViewBuilder var performancePane: some View {
        let c = Ink(scheme: scheme)
        let cpu = model.perfCPU ?? 0
        let warming = model.perfCPU == nil || model.perfWarming
        // the level from the CPU average over the last minute (ignoring the first 90 s after start)
        let level = cpu < 1 ? 0 : cpu < 5 ? 1 : cpu < 15 ? 2 : 3
        let names = ["Very light", "Light", "Moderate", "Heavy"]
        let says = [
            "You won't notice it: less than a clock in the menu bar.",
            "Barely noticeable, like a small menu bar app.",
            "Noticeable on battery. Pausing and resuming it in This Mac can settle it.",
            "Heavier than it should be. Pausing and resuming it in This Mac usually settles it.",
        ]
        let green = Color(red: 0.16, green: 0.62, blue: 0.32)
        let tint = [green, green, Color(red: 0.93, green: 0.6, blue: 0.13), Color(red: 0.85, green: 0.25, blue: 0.22)][level]
        SpacedLabel(text: "Performance")
        HStack(spacing: 10) {
            Text(warming ? "Warming up…" : names[level]).font(KD.font(20, bold: true)).foregroundColor(c.ink)
            if !warming && level <= 1 { Badge(text: "All good", ok: true) }
        }
        .padding(.top, 10)
        Text(warming ? "It's busy for a minute after it starts (calendar, app icons and music sync). Measuring…" : says[level])
            .font(KD.font(13)).foregroundColor(c.grey).padding(.top, 2).fixedSize(horizontal: false, vertical: true)
        // one bar, light → heavy, always showing at least a sliver of colour
        GeometryReader { g in
            let frac = warming ? 0.012 : min(1, max(0.012, cpu / 25)) // full bar = a quarter of a core
            ZStack(alignment: .leading) {
                Capsule().fill(c.line)
                Capsule().fill(warming ? c.grey.opacity(0.4) : tint).frame(width: max(6, g.size.width * frac))
            }
        }
        .frame(height: 8).padding(.top, 14)
        HStack {
            Text("Light").font(KD.font(10)).foregroundColor(c.grey)
            Spacer()
            Text("Heavy").font(KD.font(10)).foregroundColor(c.grey)
        }
        .padding(.top, 4)
        // each number: Light (green) / Moderate (orange) / Heavy (red), with a bar in that colour
        let mem = model.perfMem, energy = model.perfEnergy
        let cpuLv = cpu < 5 ? 0 : cpu < 15 ? 1 : 2
        let energyLv = energy < 5 ? 0 : energy < 20 ? 1 : 2 // most menu bar apps sit between 0 and 5
        // judged without the server on this Mac: running your own server takes memory, that's expected
        let appMem = mem - model.perfServerMem
        let memLv = appMem < 200 ? 0 : appMem < 500 ? 1 : 2
        let memNote = model.perfServerMem > 0
            ? String(format: "%.0f MB of it is the server on this Mac: normal when it runs here", model.perfServerMem)
            : mem < 150 ? "about one browser tab's worth" : "more than a browser tab"
        HStack(alignment: .top, spacing: 12) {
            tile("CPU", warming ? "…" : String(format: cpu < 0.1 ? "%.2f%%" : "%.1f%%", cpu), "of one core, over the last minute",
                 level: warming ? nil : cpuLv, fill: warming ? 0 : cpu / 25)
            tile("Energy", String(format: "%.1f", energy), "Activity Monitor's Energy Impact, over the last minute",
                 level: warming ? nil : energyLv, fill: warming ? 0 : energy / 50)
            tile("Memory", String(format: "%.0f MB", mem), memNote,
                 level: memLv, fill: mem / 2000) // against 2 GB: a light app shows a little
        }
        .fixedSize(horizontal: false, vertical: true) // all three as tall as the tallest
        .padding(.top, 20)
        Text("Counting \(model.perfServerMem > 0 ? "this app, its helper and the server on this Mac (only in This Mac mode)" : model.perfParts.isEmpty ? "this app" : model.perfParts). Updates every 10 seconds while this window is open; the window itself adds a little.")
            .font(KD.font(11)).foregroundColor(c.grey).padding(.top, 14).fixedSize(horizontal: false, vertical: true)
    }

    // a number in a box, with a coloured tag (Light / Moderate / Heavy) and a bar showing how
    // full it is (always at least a sliver); level nil while it's still measuring
    func tile(_ title: String, _ value: String, _ note: String, level: Int?, fill: Double) -> some View {
        let c = Ink(scheme: scheme)
        let tints = [Color(red: 0.16, green: 0.62, blue: 0.32), Color(red: 0.93, green: 0.6, blue: 0.13), Color(red: 0.85, green: 0.25, blue: 0.22)]
        let words = ["Light", "Moderate", "Heavy"]
        let tint = level.map { tints[$0] } ?? c.grey.opacity(0.4)
        return VStack(alignment: .leading, spacing: 6) {
            HStack {
                SpacedLabel(text: title)
                Spacer()
                if let level {
                    Text((level == 0 ? "✓ " : "") + words[level]).font(KD.font(11, bold: true)).foregroundColor(.white)
                        .padding(.horizontal, 8).padding(.vertical, 2).background(Capsule().fill(tints[level]))
                }
            }
            .frame(height: 20) // the same in every tile, tag or not, so the rows line up
            Text(value).font(KD.font(20, bold: true)).foregroundColor(c.ink)
            GeometryReader { g in
                ZStack(alignment: .leading) {
                    Capsule().fill(c.line)
                    Capsule().fill(tint).frame(width: max(5, g.size.width * min(1, max(0.03, fill))))
                }
            }
            .frame(height: 6)
            Text(note).font(KD.font(11)).foregroundColor(c.grey).fixedSize(horizontal: false, vertical: true)
            Spacer(minLength: 0)
        }
        .padding(14).frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .overlay(RoundedRectangle(cornerRadius: 8).stroke(c.line, lineWidth: 1))
    }

    @ViewBuilder var kindlePane: some View {
        let c = Ink(scheme: scheme)
        let k = model.kindleSettings ?? [:]
        let ok = model.kindleSettings != nil
        let s = model.settings
        let ks: (String, Any) -> Void = { model.onKindleSetting($0, $1) }
        let bs: (String, Any) -> Void = { model.onSetting($0, $1) }
        let setupTop = model.kSetupState != "waiting" || model.kindleLine.isEmpty // a Kindle plugged in, or none seen yet
        if setupTop {
            KindleSetupCard(model: model).padding(.bottom, 14)
        }
        if !ok {
            Text("Update the Kindle app (Settings › Updates on the Kindle) to change its screen, clock and heads-up settings here. Music, focus and tasks work already.")
                .font(KD.font(13)).foregroundColor(c.grey).padding(.bottom, 8).fixedSize(horizontal: false, vertical: true)
        }
        SpacedLabel(text: "Theme")
        Row(title: "Theme", hint: "The look of every screen on the Kindle") {
            Picker("", selection: Binding(get: { (k["style"] as? String) ?? "plain" }, set: { ks("style", $0) })) {
                ForEach([("plain", "Plain"), ("garden", "Garden"), ("sketch", "Sketch"), ("classic", "Classic"), ("retro", "Retro"),
                         ("notebook", "Notebook"), ("typewriter", "Typewriter"), ("gameboy", "Game Boy")], id: \.0) { Text($0.1).tag($0.0) }
            }
            .labelsHidden().pickerStyle(.menu).frame(width: 150).disabled(!ok)
        }.opacity(ok ? 1 : 0.45)
        SpacedLabel(text: "Screen").padding(.top, 18)
        choice("Theme", (k["theme"] as? String) == "auto" ? "Dark when your Mac is, else sunset to sunrise" : "Light or dark", ["Light", "Dark", "Auto"], ["light", "dark", "auto"], k["theme"], enabled: ok) { ks("theme", $0) }
        choice("Night brightness", "Front light while Auto is dark", ["Same", "Off", "Low", "Mid"], [-1, 0, 4, 10], k["night_light"], enabled: ok && (k["theme"] as? String) == "auto") { ks("night_light", $0) }
        choice("Font", "For tasks and headings", ["Sans", "Serif", "Inter", "Bitter"], ["sans", "serif", "inter", "bitter"], k["font"], enabled: ok) { ks("font", $0) }
        choice("Screen size", "Compact uses larger text and touch targets for 6″ Basic Kindles (2022, 2024 and older)", ["Standard", "Compact"], [false, true], (k["compact"] as? Bool) ?? false, enabled: ok) { ks("compact", $0) }
        choice("Orientation", "Flip the landscape screen 180° (which side the Kindle's bottom bezel is on)", ["Left", "Right"], [90, 270], (k["rotation"] as? Int) ?? 90, enabled: ok) { ks("rotation", $0) }
        choice("Temperature", "For the weather", ["°C", "°F"], [false, true], k["temp_f"], enabled: ok) { ks("temp_f", $0) }
        choice("Screen refresh", ["Never flashes; faint ghosts may stay", "Big changes flash, only where they changed", "Every change flashes; sharpest"][min(2, max(0, (k["refresh"] as? Int) ?? 1))], ["Gentle", "Normal", "Clean"], [0, 1, 2], k["refresh"], enabled: ok) { ks("refresh", $0) }
        SpacedLabel(text: "Standby clock").padding(.top, 22)
        choice("Clock", "24-hour or 12-hour", ["24h", "12h"], [false, true], k["clock_12h"], enabled: ok) { ks("clock_12h", $0) }
        choice("Quote", "A line under the clock", ["On", "Off"], [true, false], k["clock_quote"], enabled: ok) { ks("clock_quote", $0) }
        choice("Next event", "What's on next, under the clock", ["On", "Off"], [true, false], k["clock_event"], enabled: ok) { ks("clock_event", $0) }
        SpacedLabel(text: "Meeting heads-up").padding(.top, 22)
        choice("Before meetings", "A card before each event, with Join for Zoom / Meet links", ["On", "Off"], [true, false], k["heads_up"], enabled: ok) { ks("heads_up", $0) }
        choice("How early", "Minutes before it starts", ["2", "5", "10"], [2, 5, 10], k["heads_up_min"], enabled: ok && (k["heads_up"] as? Bool) != false) { ks("heads_up_min", $0) }
        SpacedLabel(text: "Music").padding(.top, 22)
        choice("Music sync", "Show what's playing on your computer", ["On", "Off"], [true, false], s["music_sync"]) { bs("music_sync", $0) }
        choice("Music app", "Auto: whichever one is playing", ["Auto", "Spotify", "Apple Music"], ["auto", "spotify", "music"], s["music_app"], enabled: (s["music_sync"] as? Bool) != false) { bs("music_app", $0) }
        choice("Music app logo", "Shows a small Spotify or Apple Music logo next to the song", ["On", "Off"], [true, false], k["music_logo"] ?? false, enabled: ok && (s["music_sync"] as? Bool) != false) { ks("music_logo", $0) }
        choice("Album covers", "On the music screen", ["On", "Off"], [true, false], s["album_covers"], enabled: (s["music_sync"] as? Bool) != false) { bs("album_covers", $0) }
        SpacedLabel(text: "Focus").padding(.top, 22)
        choice("Focus length", "Minutes per Pomodoro", ["25", "45", "50"], [25, 45, 50], s["focus_min"]) { bs("focus_min", $0) }
        choice("Break length", "Minutes between focuses", ["5", "10", "15"], [5, 10, 15], s["break_min"]) { bs("break_min", $0) }
        choice("Chime on computer", "Plays when a focus or break ends", ["On", "Off"], [true, false], model.chime) { model.onChime(($0 as? Bool) ?? true) }
        SpacedLabel(text: "Tasks").padding(.top, 22)
        choice("Separate lists", (model.computers ?? []).count < 2 ? "Searching for other devices… connect another computer first" : "Your Mac and Windows PC each get their own tasks", ["On", "Off"], ["per_computer", "one"], s["task_lists"], enabled: (model.computers ?? []).count >= 2) { bs("task_lists", $0) }
        choice("Auto-delete done tasks", "Ticked-off tasks go by themselves", ["On", "Off"], [true, false], s["auto_clear"]) { bs("auto_clear", $0) }
        choice("Delete after", "How long after you tick them off", ["Instant", "1 min", "5 min", "10 min"], [0, 1, 5, 10], s["auto_clear_min"], enabled: (s["auto_clear"] as? Bool) == true) { bs("auto_clear_min", $0) }
        SpacedLabel(text: "Updates").padding(.top, 22)
        choice("Show on main screen", "An Update button at the top when a new version is ready", ["On", "Off"], [true, false], k["update_show"], enabled: ok) { ks("update_show", $0) }
        choice("Check every", "Downloads over Wi-Fi; installs only when you tap", ["1 hour", "12 hours", "24 hours"], [1, 12, 24], k["update_every"], enabled: ok && (k["update_show"] as? Bool) != false) { ks("update_every", $0) }
        if !setupTop {
            SpacedLabel(text: "Another Kindle").padding(.top, 22).padding(.bottom, 10)
            KindleSetupCard(model: model)
        }
    }

    // Home: the task list the Kindle shows, to edit here (tick, add, rename, pin, move,
    // delete); a small line about the Kindle; anything that needs you on top.
    @ViewBuilder var homePane: some View {
        let c = Ink(scheme: scheme)
        ForEach(model.needs, id: \.self) { n in
            HStack(alignment: .top, spacing: 8) {
                Image(systemName: "exclamationmark.circle.fill").font(.system(size: 12)).foregroundColor(c.paper)
                Text(n).font(KD.font(12, bold: true)).foregroundColor(c.paper).fixedSize(horizontal: false, vertical: true)
                Spacer()
            }
            .padding(10).background(RoundedRectangle(cornerRadius: 6).fill(c.ink)).padding(.bottom, 8)
        }
        HStack(alignment: .firstTextBaseline) {
            Text(model.showing.isEmpty ? "Tasks" : model.showing.prefix(1).uppercased() + model.showing.dropFirst())
                .font(KD.font(20, bold: true)).foregroundColor(c.ink)
            Spacer()
            Text(model.tasksLine).font(KD.font(12)).foregroundColor(c.grey)
        }
        HStack(spacing: 8) {
            Text(model.kindleLine.isEmpty ? "On the Kindle now" : "On the Kindle now · " + model.kindleLine)
                .font(KD.font(11)).foregroundColor(c.grey)
            if !model.kindleLine.isEmpty { // green while the Kindle is in touch
                Badge(text: model.kindleOnline ? "Kindle connected" : "Kindle offline", ok: model.kindleOnline)
            }
        }
        .padding(.top, 2)
        HStack(spacing: 10) {
            TextField("Add a task", text: $newTask)
                .textFieldStyle(.plain).font(KD.font(14)).foregroundColor(c.ink)
                .padding(.vertical, 6).overlay(Rectangle().fill(c.ink).frame(height: 1), alignment: .bottom)
                .onSubmit(addTask)
            Pill(title: "Add", solid: true, action: addTask)
        }
        .padding(.top, 14).padding(.bottom, 6)
        if model.tasks.isEmpty {
            Text("No tasks yet. Add one above.").font(KD.font(13)).foregroundColor(c.grey).padding(.top, 10)
        }
        ForEach(model.tasks) { t in
            taskRow(t, c)
            // the new-subtask field: under the group's last subtask (or the task, without any)
            if let p = addingSub, t.id == (model.tasks.last(where: { $0.parent == p })?.id ?? p) {
                subField(p, c)
            }
        }
        // the end of the list: drop here to move a task to the bottom (out of any group)
        Rectangle().fill(dropEnd ? c.ink : Color.clear).frame(height: 2).padding(.top, 2)
        Color.clear.frame(height: 60).contentShape(Rectangle())
            .onDrop(of: [.text], isTargeted: $dropEnd) { items in
                take(items) { id in model.onTask("place", id, "|") }
                return true
            }
        if !model.tasks.isEmpty {
            Text("Click a task to edit it. To move one, drag the handle ≡ that shows on its left: onto another task to put it in that group, or between tasks. Right-click for more.")
                .font(KD.font(11)).foregroundColor(c.grey)
        }
    }

    // what a drag over a row would do: drop "before" it, or "into" its group
    @State private var dropOn: (id: Int, into: Bool)? = nil
    @State private var dropEnd = false
    @State private var addingSub: Int? = nil
    @FocusState private var subFocus: Bool
    @State private var subText = ""

    // the dragged task's id, from the drop
    func take(_ items: [NSItemProvider], _ done: @escaping (Int) -> Void) {
        items.first?.loadObject(ofClass: NSString.self) { obj, _ in
            if let s = obj as? String, let id = Int(s) { DispatchQueue.main.async { done(id) } }
        }
    }

    // where a task dropped on row t goes: into t's group (a top-level task, middle of the row),
    // or just before t (its top edge, or any subtask row)
    func dropped(on t: HelperModel.TaskItem, into: Bool, id: Int) {
        guard id != t.id else { return }
        let dragged = model.tasks.first { $0.id == id }
        if into && !t.sub && (dragged?.kids ?? 0) == 0 {
            model.onTask("place", id, "\(t.id)|") // last in that group
        } else {
            model.onTask("place", id, "\(t.parent.map(String.init) ?? "")|\(t.id)")
        }
    }

    // subField: type a subtask, Enter adds it and stays open for the next one (like Notion);
    // Esc, Enter on an empty field, or clicking away closes it
    func subField(_ parent: Int, _ c: Ink) -> some View {
        HStack(spacing: 10) {
            Image(systemName: "plus").font(.system(size: 10, weight: .bold)).foregroundColor(c.grey).frame(width: 20)
            TextField("Add a subtask", text: $subText)
                .textFieldStyle(.plain).font(KD.font(13)).foregroundColor(c.ink)
                .focused($subFocus)
                .onAppear { DispatchQueue.main.async { subFocus = true } }
                .onSubmit {
                    let v = subText.trimmingCharacters(in: .whitespaces)
                    if v.isEmpty { addingSub = nil; return }
                    model.onTask("addsub", parent, v)
                    subText = ""
                    DispatchQueue.main.async { subFocus = true }
                }
                .onExitCommand { addingSub = nil; subText = "" }
                .onChange(of: subFocus) { focused in
                    // the field moves down after each new subtask (a new view, which takes the
                    // focus back): only close when the focus is still gone a moment later
                    guard !focused else { return }
                    DispatchQueue.main.asyncAfter(deadline: .now() + 0.25) {
                        guard !subFocus, addingSub == parent else { return }
                        let v = subText.trimmingCharacters(in: .whitespaces)
                        if !v.isEmpty { model.onTask("addsub", parent, v) }
                        subText = ""; addingSub = nil
                    }
                }
        }
        .frame(minHeight: 26).padding(.vertical, 2).padding(.horizontal, 6).padding(.leading, 30)
        .overlay(Rectangle().fill(c.line).frame(height: 1).padding(.leading, 30), alignment: .bottom)
    }

    func startSub(_ id: Int) { subText = ""; addingSub = id }

    func taskRow(_ t: HelperModel.TaskItem, _ c: Ink) -> some View {
        let size: CGFloat = t.sub ? 14 : 17
        let target = dropOn?.id == t.id
        let show = hovered == t.id && editing != t.id
        // Like Notion: nothing moves on hover. The handle (the only part that drags) sits in the
        // margin left of the list, the buttons fade in on the right in space that's always kept
        // for them, and clicking the text edits it. Rows, dividers and the highlight line up.
        return HStack(spacing: 0) {
            Color.clear.frame(width: t.sub ? 30 : 0) // a subtask's indent
            Image(systemName: "line.3.horizontal").font(.system(size: 11, weight: .medium)).foregroundColor(c.grey)
                .frame(width: 24, height: 28).contentShape(Rectangle())
                .opacity(show ? 1 : 0)
                .help("Drag to move")
                .onDrag { NSItemProvider(object: String(t.id) as NSString) }
            VStack(spacing: 0) {
                // where it would land: a line above (before this task)
                Rectangle().fill(target && dropOn?.into == false ? c.ink : Color.clear).frame(height: 2)
                HStack(spacing: 10) {
                    Button { model.onTask("toggle", t.id, "") } label: {
                        ZStack {
                            Circle().stroke(c.ink, lineWidth: 1.5).frame(width: size, height: size)
                            if t.done {
                                Circle().fill(c.ink).frame(width: size, height: size)
                                Image(systemName: "checkmark").font(.system(size: size * 0.5, weight: .heavy)).foregroundColor(c.paper)
                            }
                        }
                        .frame(width: 20, height: 20).contentShape(Rectangle())
                    }
                    .buttonStyle(.plain).accessibilityLabel(t.done ? "Mark not done" : "Mark done")
                    if editing == t.id {
                        TextField("", text: $editText)
                            .textFieldStyle(.plain).font(KD.font(t.sub ? 13 : 14)).foregroundColor(c.ink)
                            .focused($editFocus)
                            .onAppear { editFocus = true }
                            .onSubmit { finishEdit(t) }
                            .onExitCommand { editing = nil }
                            .onChange(of: editFocus) { focused in if !focused { finishEdit(t) } }
                    } else {
                        Text(t.text).font(KD.font(t.sub ? 13 : 14)).foregroundColor(t.done ? c.grey : c.ink)
                            .strikethrough(t.done).lineLimit(1)
                            .frame(maxWidth: .infinity, alignment: .leading).contentShape(Rectangle())
                            .onTapGesture { editText = t.text; editing = t.id }
                            .help("Click to edit")
                    }
                    if t.pinned {
                        Image(systemName: "pin.fill").font(.system(size: 10)).foregroundColor(c.ink).help("Pinned: “Now working on” on the Kindle")
                    }
                    HStack(spacing: 0) {
                        if !t.sub { iconButton("plus", "Add a subtask", c) { startSub(t.id) } }
                        if !t.sub { iconButton(t.pinned ? "pin.slash" : "pin", t.pinned ? "Unpin" : "Pin to the Kindle", c) { model.onTask("pin", t.id, "") } }
                        iconButton("trash", "Delete", c) { model.onTask("delete", t.id, "") }
                    }
                    .frame(width: 72, alignment: .trailing)
                    .opacity(show ? 1 : 0).allowsHitTesting(show)
                }
                .frame(minHeight: t.sub ? 26 : 30)
                .padding(.vertical, t.sub ? 2 : 3).padding(.horizontal, 6)
                .background(RoundedRectangle(cornerRadius: 6).fill(hovered == t.id || editing == t.id ? c.hover : Color.clear))
                .overlay(RoundedRectangle(cornerRadius: 6).stroke(target && dropOn?.into == true ? c.ink : Color.clear, lineWidth: 1.5))
                Rectangle().fill(c.line).frame(height: 1)
            }
        }
        .padding(.leading, -24) // the handle hangs in the margin, so the rows line up with "Add a task"
        .contentShape(Rectangle())
        .onHover { inside in
            hovered = inside ? t.id : (hovered == t.id ? nil : hovered)
            if inside { dropOn = nil; dropEnd = false } // a drag that ended elsewhere left its marker
        }
        .onDrop(of: [.text], delegate: RowDrop(row: t, onMove: { into in dropOn = (t.id, into) },
                                                onLeave: { if dropOn?.id == t.id { dropOn = nil } },
                                                onDrop: { into, items in
                                                    dropOn = nil
                                                    take(items) { id in dropped(on: t, into: into, id: id) }
                                                }))
        .contextMenu {
            Button(t.done ? "Mark not done" : "Mark done") { model.onTask("toggle", t.id, "") }
            Button("Rename") { editText = t.text; editing = t.id }
            if !t.sub {
                Button("Add a subtask") { startSub(t.id) }
                Button(t.pinned ? "Unpin" : "Pin to the Kindle") { model.onTask("pin", t.id, "") }
            } else {
                Button("Move out of the group") { // to just after its group, at the top level
                    let tops = model.tasks.filter { !$0.sub }
                    let i = tops.firstIndex { $0.id == t.parent } ?? (tops.count - 1)
                    let next = i + 1 < tops.count ? String(tops[i + 1].id) : ""
                    model.onTask("place", t.id, "|" + next)
                }
            }
            Button("Move up") { model.onTask("up", t.id, "") }
            Button("Move down") { model.onTask("down", t.id, "") }
            Divider()
            Button("Delete") { model.onTask("delete", t.id, "") }
        }
    }

    func iconButton(_ symbol: String, _ help: String, _ c: Ink, _ action: @escaping () -> Void) -> some View {
        Button(action: action) {
            Image(systemName: symbol).font(.system(size: 11, weight: .medium)).foregroundColor(c.grey)
                .frame(width: 24, height: 22).contentShape(Rectangle())
        }
        .buttonStyle(.plain).help(help).accessibilityLabel(help)
    }

    // finishEdit saves a rename (Enter, or clicking away); empty or unchanged: nothing
    func finishEdit(_ t: HelperModel.TaskItem) {
        guard editing == t.id else { return }
        let v = editText.trimmingCharacters(in: .whitespaces)
        if !v.isEmpty && v != t.text { model.onTask("rename", t.id, v) }
        editing = nil
    }

    func addTask() {
        let t = newTask.trimmingCharacters(in: .whitespaces)
        guard !t.isEmpty else { return }
        model.onAddTask(t)
        newTask = ""
    }

    @ViewBuilder var pane: some View {
        let c = Ink(scheme: scheme)
        switch model.section {
        case "home":
            homePane
        case "kindle":
            kindlePane
        case "server":
            SpacedLabel(text: "Server")
            Row(title: "Where Desk Buddy runs", hint: model.local ? "On this Mac: works while it's awake and this app is open" : "In the cloud: works while this Mac sleeps") {
                Segmented(labels: ["Cloud", "This Mac"], selected: model.local ? 1 : 0) { model.onMode($0 == 1 ? "local" : "cloud") }
            }
            // Only this switch moves Desk Buddy: the Kindle follows by itself (it has nothing to pick).
            Row(title: "On the Kindle", hint: model.local
                ? "It follows by itself when you switch here. A new Kindle finds this Mac on the Wi-Fi: click Allow when this app asks."
                : "It follows by itself when you switch here.")
            // Phone page: scan with the phone's camera; the page remembers the key after that
            Row(title: "Phone page", hint: model.local
                ? "Scan with your phone's camera to see and change your tasks there. Works on this Wi-Fi while this Mac is on. Your phone remembers it after that."
                : "Scan with your phone's camera to see and change your tasks there, from anywhere. Your phone remembers it after that.") {
                if let qr = QR.image(model.phoneURL) {
                    Image(nsImage: qr).interpolation(.none).resizable().frame(width: 112, height: 112)
                        .padding(6).background(Color.white).cornerRadius(6)
                        .help("Opens Desk Buddy on your phone, already signed in. Only show it to your own phone.")
                }
            }
            if !model.serverLine.isEmpty {
                Text(model.serverLine).font(KD.font(13)).foregroundColor(c.grey).padding(.top, 14)
            }
            if model.local {
                CloudGuide(model: model).padding(.top, 22)
            }
        case "permissions":
            SpacedLabel(text: "Permissions")
            PrivacyCard(local: model.local).padding(.top, 12)
            Text("What Desk Buddy needs from macOS. Each one only adds what it says; everything else works without them. After an update, macOS may ask for them again.")
                .font(KD.font(13)).foregroundColor(c.grey).padding(.top, 14).padding(.bottom, 4).fixedSize(horizontal: false, vertical: true)
            ForEach(model.perms) { p in
                let unsure = p.title.hasPrefix("Controlling") && p.status.hasPrefix("Couldn't")
                HStack(alignment: .top, spacing: 12) {
                    PermIcon(title: p.title)
                    Row(title: p.title, hint: p.detail + "\n" + permShares(p.title) + (unsure ? "\n" + permShares("Spotify note") : ""),
                        badge: (unsure ? "Can't check (a music app quirk)" : p.status, p.ok)) {
                        Pill(title: p.button, solid: p.button == "Allow…") { model.onPermission(p.id) }
                    }
                }
            }
        case "performance":
            performancePane
        case "about":
            SpacedLabel(text: "About")
            HStack(spacing: 16) {
                Buddy(height: 72)
                VStack(alignment: .leading, spacing: 4) {
                    Text("Desk Buddy").font(KD.font(18, bold: true)).foregroundColor(c.ink)
                    Text("Your Kindle, on your desk, keeping you company.").font(KD.font(12)).foregroundColor(c.grey)
                }
            }
            .padding(.vertical, 14)
            updatesSection
            SpacedLabel(text: "Desk Buddy").padding(.top, 22)
            Row(title: "Welcome", hint: "The first-open guide: what it does, permissions, connecting the Kindle") { Pill(title: "Show again") { model.afterUpdate = false; model.welcomeStep = 0 } }
            Row(title: "Start at login", hint: "Opens by itself when you log in to this Mac, so the Kindle always has it") {
                Segmented(labels: ["On", "Off"], selected: model.startAtLogin ? 0 : 1) { model.onStartAtLogin($0 == 0) }
            }
            Row(title: "Quit", hint: model.startAtLogin ? "Stops until you open it again, or log in next time" : "Stops until you open it again") { Pill(title: "Quit") { model.onQuit() } }
            Row(title: "Uninstall", hint: "Removes the helper from this Mac. Asks first.") { Pill(title: "Uninstall…") { model.onUninstall() } }
        default:
            SpacedLabel(text: "This Mac")
            Text(model.status).font(KD.font(16, bold: true)).foregroundColor(c.ink)
                .padding(.top, 12).padding(.bottom, 6).fixedSize(horizontal: false, vertical: true)
            Row(title: "Helper", hint: "Tells the Kindle you're using this Mac, opens apps from the Deck, reads your music") {
                Segmented(labels: ["On", "Paused"], selected: model.paused ? 1 : 0) { model.onPause($0 == 1) }
            }
            Row(title: "Calendar", hint: model.calendar) { Pill(title: "Sync now") { model.onSyncCalendar() } }
                .opacity(model.paused ? 0.35 : 1).disabled(model.paused)
            if !model.lastDeck.isEmpty {
                Row(title: "Last from the Kindle", hint: model.lastDeck)
            }
            if let comps = model.computers {
                Row(title: "Other computers", hint: comps.count < 2
                    ? "Searching for other devices… Install Desk Buddy on another computer to connect it."
                    : "Your Windows PC is connected too.") {
                    Pill(title: "Connect a computer…") { model.onConnectComputer() }
                }
            }
            if !model.connectCode.isEmpty {
                // what to type on the other computer (Desk Buddy for Windows asks for both)
                VStack(alignment: .leading, spacing: 6) {
                    Text("On the other computer, open Desk Buddy and enter:").font(KD.font(12)).foregroundColor(Color.secondary)
                    HStack(spacing: 18) {
                        VStack(alignment: .leading, spacing: 2) {
                            Text("Server").font(KD.font(11)).foregroundColor(Color.secondary)
                            Text(model.connectServer).font(.system(size: 14, weight: .semibold, design: .monospaced)).textSelection(.enabled)
                        }
                        VStack(alignment: .leading, spacing: 2) {
                            Text("Code").font(KD.font(11)).foregroundColor(Color.secondary)
                            Text(model.connectCode).font(.system(size: 20, weight: .bold, design: .monospaced)).textSelection(.enabled)
                        }
                    }
                    Text("The code works once, for 10 minutes. The computer then connects by itself.").font(KD.font(11)).foregroundColor(Color.secondary)
                }
                .padding(12).frame(maxWidth: .infinity, alignment: .leading)
                .overlay(RoundedRectangle(cornerRadius: 8).stroke(Color.secondary.opacity(0.4), lineWidth: 1))
                .padding(.top, 8)
            }
            Row(title: "Tasks page", hint: "Your tasks in a web browser, also on your phone") { Pill(title: "Open") { model.onOpenPage() } }
            // Let Claude Code / Codex (running on this Mac) change the tasks: instructions to paste
            // into their instructions file. They read the address and key from Desk Buddy's own file.
            Row(title: "AI assistants", hint: model.aiNote.isEmpty
                ? "Let Claude Code or Codex add, tick and change your tasks when you ask. Copy these instructions into their instructions file (Claude Code: ~/.claude/CLAUDE.md, Codex: ~/.codex/AGENTS.md). Your key isn't in them."
                : model.aiNote) {
                Pill(title: "Copy instructions") { model.onCopyAI() }
            }
        }
    }
}

// A drop over a task row: the top third means "before this task", the rest "into its group"
// (for a top-level task; on a subtask it's always "before").
struct RowDrop: DropDelegate {
    let row: HelperModel.TaskItem
    let onMove: (Bool) -> Void
    let onLeave: () -> Void
    let onDrop: (Bool, [NSItemProvider]) -> Void
    func into(_ info: DropInfo) -> Bool { !row.sub && info.location.y > 14 }
    func dropUpdated(info: DropInfo) -> DropProposal? { onMove(into(info)); return DropProposal(operation: .move) }
    func dropExited(info: DropInfo) { onLeave() }
    func performDrop(info: DropInfo) -> Bool { onDrop(into(info), info.itemProviders(for: [.text])); return true }
}

// The welcome: what the helper does, the permissions it needs (allowed right there), and how
// the Kindle connects. Shown on the first open, and again at the permissions step after an
// update if macOS dropped one (an ad-hoc signed app looks new to macOS after every update).
struct WelcomeView: View {
    @Environment(\.colorScheme) var scheme
    @ObservedObject var model: HelperModel
    let step: Int
    var scrolls = true

    var body: some View {
        let c = Ink(scheme: scheme)
        VStack(spacing: 0) {
            HStack(spacing: 6) {
                Buddy(height: 18)
                Text("Desk Buddy").font(KD.font(15, bold: true)).foregroundColor(c.ink)
            }
            .frame(maxWidth: .infinity).frame(height: 38)
            Rectangle().fill(c.line).frame(height: 1)
            let page = VStack(alignment: .leading, spacing: 0) {
                switch step {
                case 0: intro(c)
                case 1: permissions(c)
                default: connect(c)
                }
            }
            .frame(maxWidth: 560, alignment: .leading)
            .padding(.horizontal, 32).padding(.vertical, 28)
            .frame(maxWidth: .infinity)
            if scrolls { ScrollView { page } } else { page.frame(maxHeight: .infinity, alignment: .top) }
            Rectangle().fill(c.line).frame(height: 1)
            HStack(spacing: 10) { // page dots, Back, Next / Done
                HStack(spacing: 6) {
                    ForEach(0..<3, id: \.self) { i in
                        Circle().fill(i == step ? c.ink : c.line).frame(width: 7, height: 7)
                    }
                }
                Spacer()
                if step > 0 { Pill(title: "Back") { model.welcomeStep = step - 1 } }
                if step < 2 {
                    Pill(title: step == 1 ? "Continue" : "Get started", solid: true) { model.welcomeStep = step + 1 }
                } else {
                    Pill(title: "Done", solid: true) { model.onWelcomeDone() }
                }
            }
            .padding(.horizontal, 24).frame(height: 56)
        }
        .background(c.paper)
    }

    func intro(_ c: Ink) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            Buddy(height: 92).padding(.bottom, 14)
            SpacedLabel(text: "Welcome")
            Text("Desk Buddy connects your Mac to the Kindle on your desk.")
                .font(KD.font(22, bold: true)).foregroundColor(c.ink).padding(.top, 10).fixedSize(horizontal: false, vertical: true)
            VStack(alignment: .leading, spacing: 14) {
                point("calendar", "Your calendar on the Kindle", "Today's events, the month, a heads-up before meetings", c)
                point("music.note", "What's playing", "Spotify or Apple Music, with play, skip and volume", c)
                point("square.grid.2x2", "Open apps from the Kindle", "Tap a tile on the Deck and it opens here", c)
                point("laptopcomputer", "It knows you're at this Mac", "So the Kindle shows the right things", c)
            }
            .padding(.top, 22)
            Text("It runs quietly in the menu bar (look for Desk Buddy up there). Next: a few permissions macOS asks for.")
                .font(KD.font(12)).foregroundColor(c.grey).padding(.top, 22).fixedSize(horizontal: false, vertical: true)
        }
    }

    func point(_ icon: String, _ title: String, _ hint: String, _ c: Ink) -> some View {
        HStack(alignment: .top, spacing: 12) {
            Image(systemName: icon).font(.system(size: 14, weight: .semibold)).foregroundColor(c.ink).frame(width: 22)
            VStack(alignment: .leading, spacing: 2) {
                Text(title).font(KD.font(14, bold: true)).foregroundColor(c.ink)
                Text(hint).font(KD.font(12)).foregroundColor(c.grey)
            }
        }
    }

    func permissions(_ c: Ink) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            SpacedLabel(text: model.afterUpdate ? "Updated" : "Permissions")
            Text(model.afterUpdate ? "Desk Buddy was updated. Please allow these again." : "Please allow these so it can fully work")
                .font(KD.font(22, bold: true)).foregroundColor(c.ink).padding(.top, 10).fixedSize(horizontal: false, vertical: true)
            Text("Click Allow and macOS asks once. Each one only adds what it says; everything else works without it.")
                .font(KD.font(12)).foregroundColor(c.grey).padding(.top, 6).fixedSize(horizontal: false, vertical: true)
            PrivacyCard(local: model.local).padding(.top, 14)
            VStack(spacing: 0) {
                ForEach(model.perms) { p in
                    let unsure = p.title.hasPrefix("Controlling") && p.status.hasPrefix("Couldn't")
                    HStack(alignment: .top, spacing: 12) {
                        PermIcon(title: p.title)
                        Row(title: p.title, hint: p.detail + "\n" + permShares(p.title) + (unsure ? "\n" + permShares("Spotify note") : ""),
                            badge: (unsure ? "Can't check (a music app quirk)" : p.status, p.ok)) {
                            if !p.ok { Pill(title: p.button, solid: p.button == "Allow…") { model.onPermission(p.id) } }
                        }
                    }
                }
            }
            .padding(.top, 12)
            HStack(alignment: .top, spacing: 8) {
                Image(systemName: "info.circle").font(.system(size: 12)).foregroundColor(c.grey)
                Text("After each update of Desk Buddy, macOS may ask for these again: it sees the updated app as new. If something stops working after an update, come back here (Permissions in the sidebar).")
                    .font(KD.font(12)).foregroundColor(c.grey).fixedSize(horizontal: false, vertical: true)
            }
            .padding(12).background(RoundedRectangle(cornerRadius: 6).stroke(c.line, lineWidth: 1)).padding(.top, 16)
        }
    }

    // the last step: whether the Kindle has really been in touch (not just which server is set)
    func connect(_ c: Ink) -> some View {
        let connected = model.kindleSeen && !model.kindleLine.isEmpty
        return VStack(alignment: .leading, spacing: 0) {
            SpacedLabel(text: "Your Kindle")
            HStack(spacing: 10) {
                Text(connected ? "Your Kindle is connected" : "Let's connect your Kindle")
                    .font(KD.font(22, bold: true)).foregroundColor(c.ink).fixedSize(horizontal: false, vertical: true)
                if connected { Badge(text: "Connected", ok: true) }
            }
            .padding(.top, 10)
            if connected {
                Text("It checked in " + model.kindleLine.replacingOccurrences(of: "seen ", with: "") + ".")
                    .font(KD.font(13)).foregroundColor(c.grey).padding(.top, 6)
                VStack(alignment: .leading, spacing: 14) {
                    point("arrow.triangle.2.circlepath", "Changes show within about 10 seconds", "Tasks, settings and themes, both ways", c)
                    point("slider.horizontal.3", "Change the Kindle from here", "Its settings and theme are in the Kindle section", c)
                }
                .padding(.top, 22)
            } else {
                KindleSetupCard(model: model).padding(.top, 14)
                Text("Desk Buddy already on your Kindle? With it running there:")
                    .font(KD.font(13)).foregroundColor(c.grey).padding(.top, 6).fixedSize(horizontal: false, vertical: true)
                VStack(alignment: .leading, spacing: 14) {
                    if model.local {
                        point("1.circle", "On the Kindle, tap Find it", "It finds this Mac on the Wi-Fi", c)
                        point("2.circle", "Pick this Mac, then click Allow here", "The Kindle shows a code: check it matches", c)
                        point("3.circle", "That's it", "This window shows it as connected", c)
                    } else {
                        point("1.circle", "Connect the Kindle to the same Wi-Fi", "Its own Settings › Wi-Fi", c)
                        point("2.circle", "Open Desk Buddy in the Kindle's library", "It connects to your Desk Buddy server by itself", c)
                        point("3.circle", "Not connecting?", "Server in the sidebar: switch to This Mac and pair the Kindle there", c)
                    }
                }
                .padding(.top, 22)
            }
            HStack(alignment: .top, spacing: 10) {
                Image(systemName: "pc").font(.system(size: 15)).foregroundColor(c.ink).frame(width: 20)
                VStack(alignment: .leading, spacing: 2) {
                    Text("Got a Windows PC too?").font(KD.font(13, bold: true)).foregroundColor(c.ink)
                    Text("Install Desk Buddy for Windows on it (from the same download page). It finds this Mac on the Wi-Fi by itself: click Allow here when it asks. Then the Kindle shows whichever computer you're using.")
                        .font(KD.font(12)).foregroundColor(c.grey).fixedSize(horizontal: false, vertical: true)
                }
            }
            .padding(12).background(RoundedRectangle(cornerRadius: 6).stroke(c.line, lineWidth: 1)).padding(.top, 22)
            Text("Everything else is in this window: your tasks on Home, the Kindle's settings, and more. Open it any time from Desk Buddy in the menu bar.")
                .font(KD.font(12)).foregroundColor(c.grey).padding(.top, 22).fixedSize(horizontal: false, vertical: true)
        }
    }
}
