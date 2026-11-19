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
        if let url = Bundle.main.url(forResource: "SourceHanSansSC-Regular", withExtension: "otf") {
            CTFontManagerRegisterFontsForURL(url as CFURL, .process, nil)
        }
    }

    static func font(_ size: CGFloat, bold: Bool = false) -> Font {
        let name = bold ? "AtkinsonHyperlegible-Bold" : "AtkinsonHyperlegible-Regular"
        if let primary = NSFont(name: name, size: size), let chinese = NSFont(name: "SourceHanSansSC-Regular", size: size) {
            let descriptor = primary.fontDescriptor.addingAttributes([.cascadeList: [chinese.fontDescriptor]])
            return Font(NSFont(descriptor: descriptor, size: size) ?? primary)
        }
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
                        Text("你自己的云服务器").font(KD.font(14, bold: true)).foregroundColor(c.ink)
                        Text("可选 · Cloudflare 免费方案 · 约 10 分钟").font(KD.font(11)).foregroundColor(c.grey)
                    }
                    Spacer()
                    Image(systemName: folded ? "chevron.down" : "chevron.up").font(.system(size: 11, weight: .semibold)).foregroundColor(c.grey)
                }
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .help(folded ? "显示步骤" : "隐藏步骤")
            if !folded {
                Text("让 Kindle 和手机在本机休眠时也能随处使用。服务运行在你自己的 Cloudflare 账号中。")
                    .font(KD.font(12)).foregroundColor(c.grey).fixedSize(horizontal: false, vertical: true).padding(.top, 10)
                step(1, "创建免费 Cloudflare 账号", "使用邮箱和密码注册，免费方案即可。") {
                    Pill(title: "打开 Cloudflare") { NSWorkspace.shared.open(URL(string: "https://dash.cloudflare.com/sign-up")!) }
                }
                step(2, "安装 Node.js", "设置过程需要 Node.js，请选择 LTS 版本。") {
                    if model.nodeInstalled { Badge(text: "已安装", ok: true) }
                    else { Pill(title: "获取 Node.js") { NSWorkspace.shared.open(URL(string: "https://nodejs.org")!) } }
                }
                step(3, "部署服务器", "终端会完成后续操作，浏览器会打开以登录 Cloudflare，完成后自动复制地址。") {
                    Pill(title: "在终端中运行", solid: true) { model.onCloudSetup() }
                }
                codeBox(c)
                step(4, "粘贴地址", "地址以 .workers.dev 结尾，任务会复制过去，Kindle 也会随之连接。") { EmptyView() }
                HStack(spacing: 10) {
                    TextField("https://desk-buddy.….workers.dev", text: $address)
                        .textFieldStyle(.plain).font(KD.font(13)).foregroundColor(c.ink)
                        .padding(.vertical, 6).overlay(Rectangle().fill(c.ink).frame(height: 1), alignment: .bottom)
                        .onSubmit { model.onCloudConnect(address) }
                    Pill(title: "连接", solid: true) { model.onCloudConnect(address) }
                }
                .padding(.leading, 34).padding(.top, 2)
                if !model.cloudNote.isEmpty {
                    Text(model.cloudNote).font(KD.font(12, bold: true)).foregroundColor(c.ink).padding(.leading, 34).padding(.top, 6)
                }
                Rectangle().fill(c.line).frame(height: 1).padding(.top, 16)
                HStack(alignment: .center, spacing: 14) {
                    VStack(alignment: .leading, spacing: 2) {
                        Text("想让 AI 帮忙？").font(KD.font(13, bold: true)).foregroundColor(c.ink)
                        Text("将提示词粘贴到 Claude Code 或 Codex，可协助执行各个步骤。浏览器中的 ChatGPT 或 Claude 可逐步指导你运行命令。提示词不包含密钥。")
                            .font(KD.font(11)).foregroundColor(c.grey).fixedSize(horizontal: false, vertical: true)
                    }
                    Spacer(minLength: 8)
                    Pill(title: "复制提示词") { model.onCloudPrompt() }
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
            Text("终端将运行：").font(KD.font(11)).foregroundColor(c.grey)
            Text(CloudGuide.command).font(mono).foregroundColor(c.ink).textSelection(.enabled)
            Text("setup.sh 会获取 Cloudflare 的 Wrangler 工具并运行：").font(KD.font(11)).foregroundColor(c.grey).padding(.top, 4)
            Text(CloudGuide.inside).font(mono).foregroundColor(c.ink).textSelection(.enabled)
            HStack(spacing: 8) {
                Pill(title: "复制") {
                    NSPasteboard.general.clearContents()
                    NSPasteboard.general.setString(CloudGuide.command, forType: .string)
                }
                Pill(title: "打开脚本") { model.onCloudScript() }
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
            case "found": return ("已找到 Kindle", model.kSetupHas
                ? "设备已装有 Desk Buddy，将安装新版并设置为连接本机。"
                : "准备安装 Desk Buddy 并设置为连接本机。设备需已越狱并安装 hotfix（kindlemodding.org）。")
            case "copying": return ("正在复制到 Kindle…", "请勿拔出设备，大约需要半分钟。")
            case "checking": return ("正在核对复制的文件…", "Kindle 的 USB 传输有时会丢失文件，因此需要读回核验。")
            case "ejecting": return ("正在推出设备…", "即将完成。")
            case "done": return ("已完成，可以拔出 Kindle", (model.kSetupNote.isEmpty ? "" : model.kSetupNote + " ") + "然后在 Kindle 书库中打开 Desk Buddy。连接同一 Wi-Fi 后会自动连接本机。")
            case "failed": return ("操作未成功", model.kSetupNote)
            default: return ("将 Desk Buddy 安装到 Kindle", "用 USB 将已越狱并安装 hotfix 的 Kindle 连接到本机（kindlemodding.org），Desk Buddy 会复制程序并配置连接。")
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
                if st == "found" { Pill(title: "安装到此设备", solid: true) { model.onKindleInstall() } }
                if st == "failed" { Pill(title: "重试", solid: true) { model.onKindleInstall() } }
                if ["waiting", "found", "failed"].contains(st) { Pill(title: "另存安装文件…") { model.onKindleSaveFiles() } }
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
    case "Calendar": return "共享到你的 Desk Buddy 服务器：即将发生的日程，包括标题、时间、地点与备注。"
    case "Controlling Spotify", "Controlling Apple Music": return "仅在 Kindle 显示音乐时共享曲目和封面。"
    case "Spotify note": return "音乐应用可能不响应 macOS 的权限查询。若 Kindle 能显示曲目，即已获得授权。"
    case "Bluetooth": return "共享音箱与耳机名称，以便 Kindle 列出设备。"
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
                Text("数据始终属于你").font(KD.font(15, bold: true)).foregroundColor(c.ink)
            }
            Text(local
                 ? "Desk Buddy 运行在本机。Kindle 显示的数据保存在本机及你的 Wi-Fi 内；只有查询天气的城市名会发送到互联网。"
                 : "Kindle 显示的数据只发送到你自己的 Desk Buddy 服务器，无论它运行在 Cloudflare、本机还是另一台电脑上。没有 Desk Buddy 官方数据服务器，持有密钥才能读取。")
                .font(KD.font(12)).foregroundColor(c.grey).fixedSize(horizontal: false, vertical: true)
            HStack(spacing: 14) {
                ForEach(["无广告", "不追踪", "无统计分析", "不出售或共享数据"], id: \.self) { t in
                    HStack(spacing: 4) {
                        Image(systemName: "checkmark").font(.system(size: 9, weight: .heavy)).foregroundColor(Badge.green)
                        Text(t).font(KD.font(11, bold: true)).foregroundColor(c.ink)
                    }
                }
            }
            .padding(.top, 2)
            Text("只共享 Kindle 显示所需的数据，并先经 macOS 询问授权。你随时可以在系统设置中撤销权限。")
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
        ("home", "主页", "你的任务", "house"),
        ("kindle", "Kindle", "设备设置", "slider.horizontal.3"),
        ("status", "本机", "助手与日历", "laptopcomputer"),
        ("server", "服务器", "云端或本机", "cloud"),
        ("permissions", "权限", "macOS 授权", "lock"),
        ("performance", "性能", "资源占用", "gauge.with.dots.needle.33percent"),
        ("about", "关于", "版本与退出", "info.circle"),
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
                    .buttonStyle(.plain).help(collapsed ? "显示侧栏" : "隐藏侧栏")
                    .accessibilityLabel(collapsed ? "显示侧栏" : "隐藏侧栏")
                    Spacer()
                }
                .padding(.leading, 78) // clear of the window's close / minimise / zoom buttons
                HStack {
                    Spacer()
                    Button { model.onPause(!model.paused) } label: {
                        HStack(spacing: 5) {
                            Image(systemName: model.paused ? "play.fill" : "pause.fill").font(.system(size: 10, weight: .semibold))
                            Text(model.paused ? "继续" : "暂停").font(KD.font(12, bold: true))
                        }
                        .foregroundColor(model.paused ? c.paper : c.ink)
                        .padding(.horizontal, 10).frame(height: 24)
                        .background(Capsule().fill(model.paused ? c.ink : Color.clear))
                        .overlay(Capsule().stroke(c.ink, lineWidth: 1))
                        .contentShape(Capsule())
                    }
                    .buttonStyle(.plain)
                    .help(model.paused ? "重新连接本机与 Kindle" : "暂停本机与 Kindle 通信；恢复前不提供应用面板、音乐、日历或使用状态")
                    .accessibilityLabel(model.paused ? "继续" : "暂停")
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
            : ready ? (model.updCanInstall ? "版本 \(model.updVersion) 已就绪。点击安装后，Desk Buddy 将重新启动。" + (model.updStatus.isEmpty ? "" : " " + model.updStatus)
                                           : "版本 \(model.updVersion) 已发布，可从 github.com/deskbuddy-app/desk-buddy 下载。")
            : !model.updChecked.isEmpty ? "已是最新版本（检查于 \(model.updChecked)）。"
            : "每 6 小时向你的 Desk Buddy 服务器检查一次，仅在点击后安装。"
        SpacedLabel(text: "更新").padding(.top, 8)
        Row(title: "版本 \(model.version)", hint: status) {
            if model.updBusy {
                EmptyView()
            } else if ready {
                HStack(spacing: 8) {
                    Pill(title: "更新内容") { model.whatsNew.toggle() }
                    if model.updCanInstall { Pill(title: "安装", solid: true) { model.onInstallUpdate() } }
                }
            } else {
                Pill(title: model.updChecking ? "正在检查…" : "检查更新") { model.onCheckUpdate() }
            }
        }
        if model.notes.count > 1 && !model.updBusy {
            let prev = model.notes[1].version
            Row(title: model.goBackAsk ? "退回版本 \(prev)？" : "退回版本 \(prev)",
                hint: model.goBackAsk ? "Desk Buddy 将重新启动，并向 Kindle 提供配套版本。任务与设置会保留，之后可再次更新。"
                                      : "当前版本出现问题时使用") {
                if model.goBackAsk {
                    HStack(spacing: 8) {
                        Pill(title: "取消") { model.goBackAsk = false }
                        Pill(title: "退回", solid: true) { model.goBackAsk = false; model.onGoBack() }
                    }
                } else {
                    Pill(title: "退回…") { model.goBackAsk = true }
                }
            }
        }
        if ready && model.whatsNew && !model.updBusy {
            NoteBox {
                ForEach(model.updNotes) { e in NoteBlock(title: "版本 \(e.version)", date: e.date, items: e.items) }
            }
        }
        if !model.notes.isEmpty {
            let i = max(0, min(model.notesPage, model.notes.count - 1))
            let e = model.notes[i]
            NoteBox {
                HStack(spacing: 8) {
                    NoteBlock(title: i == 0 ? "此版本的更新内容" : "版本 \(e.version) 的更新内容", date: e.date, items: [])
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
        let names = ["极低", "低", "中等", "高"]
        let says = [
            "占用很低，低于菜单栏时钟。",
            "占用较低，类似小型菜单栏应用。",
            "使用电池时可能有影响，可在本机页面暂停并恢复。",
            "占用偏高，可在本机页面尝试暂停并恢复。",
        ]
        let green = Color(red: 0.16, green: 0.62, blue: 0.32)
        let tint = [green, green, Color(red: 0.93, green: 0.6, blue: 0.13), Color(red: 0.85, green: 0.25, blue: 0.22)][level]
        SpacedLabel(text: "性能")
        HStack(spacing: 10) {
            Text(warming ? "正在初始化…" : names[level]).font(KD.font(20, bold: true)).foregroundColor(c.ink)
            if !warming && level <= 1 { Badge(text: "运行正常", ok: true) }
        }
        .padding(.top, 10)
        Text(warming ? "启动后约一分钟内会同步日历、应用图标和音乐，正在测量…" : says[level])
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
            Text("低").font(KD.font(10)).foregroundColor(c.grey)
            Spacer()
            Text("高").font(KD.font(10)).foregroundColor(c.grey)
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
            ? String(format: "其中 %.0f MB 来自本机服务器，属于正常占用", model.perfServerMem)
            : mem < 150 ? "约等于一个浏览器标签页" : "超过一个浏览器标签页"
        HStack(alignment: .top, spacing: 12) {
            tile("CPU", warming ? "…" : String(format: cpu < 0.1 ? "%.2f%%" : "%.1f%%", cpu), "最近一分钟，单个核心的占比",
                 level: warming ? nil : cpuLv, fill: warming ? 0 : cpu / 25)
            tile("能耗", String(format: "%.1f", energy), "最近一分钟的活动监视器能耗影响",
                 level: warming ? nil : energyLv, fill: warming ? 0 : energy / 50)
            tile("内存", String(format: "%.0f MB", mem), memNote,
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
        let words = ["低", "中等", "高"]
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
            Text("更新 Kindle 应用（设置 › 更新）后，可在这里调整屏幕、时钟与会议提醒。音乐、专注和任务已可使用。")
                .font(KD.font(13)).foregroundColor(c.grey).padding(.bottom, 8).fixedSize(horizontal: false, vertical: true)
        }
        SpacedLabel(text: "主题")
        Row(title: "主题", hint: "Kindle 各个页面的外观") {
            Picker("", selection: Binding(get: { (k["style"] as? String) ?? "plain" }, set: { ks("style", $0) })) {
                ForEach([("plain", "简洁"), ("garden", "花园"), ("sketch", "手绘"), ("classic", "经典"), ("retro", "复古"),
                         ("notebook", "笔记本"), ("typewriter", "打字机"), ("gameboy", "Game Boy")], id: \.0) { Text($0.1).tag($0.0) }
            }
            .labelsHidden().pickerStyle(.menu).frame(width: 150).disabled(!ok)
        }.opacity(ok ? 1 : 0.45)
        SpacedLabel(text: "屏幕").padding(.top, 18)
        choice("主题", (k["theme"] as? String) == "auto" ? "跟随 Mac 深色模式，否则日落后开启" : "浅色或深色", ["浅色", "深色", "自动"], ["light", "dark", "auto"], k["theme"], enabled: ok) { ks("theme", $0) }
        choice("夜间亮度", "自动深色模式下的前光", ["相同", "关闭", "低", "中"], [-1, 0, 4, 10], k["night_light"], enabled: ok && (k["theme"] as? String) == "auto") { ks("night_light", $0) }
        choice("字体", "用于任务与标题", ["无衬线", "衬线", "Inter", "Bitter"], ["sans", "serif", "inter", "bitter"], k["font"], enabled: ok) { ks("font", $0) }
        choice("屏幕尺寸", "紧凑模式使用更大的文字和触摸区域，适用于 6 英寸入门款 Kindle（2022、2024 及更早型号）", ["标准", "紧凑"], [false, true], (k["compact"] as? Bool) ?? false, enabled: ok) { ks("compact", $0) }
        choice("方向", "将横屏旋转 180°，选择 Kindle 底边所在的一侧", ["左侧", "右侧"], [90, 270], (k["rotation"] as? Int) ?? 90, enabled: ok) { ks("rotation", $0) }
        choice("温度", "用于天气", ["°C", "°F"], [false, true], k["temp_f"], enabled: ok) { ks("temp_f", $0) }
        choice("屏幕刷新", ["不闪屏，可能有轻微残影", "较大变化时，仅在变化区域闪屏", "每次变化都闪屏，显示最清晰"][min(2, max(0, (k["refresh"] as? Int) ?? 1))], ["柔和", "标准", "清晰"], [0, 1, 2], k["refresh"], enabled: ok) { ks("refresh", $0) }
        SpacedLabel(text: "待机时钟").padding(.top, 22)
        choice("时钟", "24 小时制或 12 小时制", ["24h", "12h"], [false, true], k["clock_12h"], enabled: ok) { ks("clock_12h", $0) }
        choice("引言", "时钟下方的一行引言", ["开启", "关闭"], [true, false], k["clock_quote"], enabled: ok) { ks("clock_quote", $0) }
        choice("下个日程", "在时钟下方显示下个日程", ["开启", "关闭"], [true, false], k["clock_event"], enabled: ok) { ks("clock_event", $0) }
        SpacedLabel(text: "会议提醒").padding(.top, 22)
        choice("会议前提醒", "会前显示提醒卡，可打开 Zoom / Meet 链接", ["开启", "关闭"], [true, false], k["heads_up"], enabled: ok) { ks("heads_up", $0) }
        choice("提前多久", "会议开始前的分钟数", ["2", "5", "10"], [2, 5, 10], k["heads_up_min"], enabled: ok && (k["heads_up"] as? Bool) != false) { ks("heads_up_min", $0) }
        SpacedLabel(text: "音乐").padding(.top, 22)
        choice("音乐同步", "显示电脑正在播放的音乐", ["开启", "关闭"], [true, false], s["music_sync"]) { bs("music_sync", $0) }
        choice("音乐应用", "自动：跟随正在播放的应用", ["自动", "Spotify", "Apple Music"], ["auto", "spotify", "music"], s["music_app"], enabled: (s["music_sync"] as? Bool) != false) { bs("music_app", $0) }
        choice("音乐应用标志", "在曲目旁显示音乐应用标志", ["开启", "关闭"], [true, false], k["music_logo"] ?? false, enabled: ok && (s["music_sync"] as? Bool) != false) { ks("music_logo", $0) }
        choice("专辑封面", "在音乐页面显示", ["开启", "关闭"], [true, false], s["album_covers"], enabled: (s["music_sync"] as? Bool) != false) { bs("album_covers", $0) }
        SpacedLabel(text: "专注").padding(.top, 22)
        choice("专注时长", "每次番茄钟的分钟数", ["25", "45", "50"], [25, 45, 50], s["focus_min"]) { bs("focus_min", $0) }
        choice("休息时长", "两次专注间的休息分钟数", ["5", "10", "15"], [5, 10, 15], s["break_min"]) { bs("break_min", $0) }
        choice("电脑提示音", "专注或休息结束时播放", ["开启", "关闭"], [true, false], model.chime) { model.onChime(($0 as? Bool) ?? true) }
        SpacedLabel(text: "任务").padding(.top, 22)
        choice("独立任务列表", (model.computers ?? []).count < 2 ? "正在搜索其他设备…请先连接另一台电脑" : "Mac 与 Windows 电脑分别使用独立任务列表", ["开启", "关闭"], ["per_computer", "one"], s["task_lists"], enabled: (model.computers ?? []).count >= 2) { bs("task_lists", $0) }
        choice("自动删除已完成任务", "自动清理勾选完成的任务", ["开启", "关闭"], [true, false], s["auto_clear"]) { bs("auto_clear", $0) }
        choice("删除延迟", "勾选完成后等待多久", ["立即", "1 min", "5 min", "10 min"], [0, 1, 5, 10], s["auto_clear_min"], enabled: (s["auto_clear"] as? Bool) == true) { bs("auto_clear_min", $0) }
        SpacedLabel(text: "更新").padding(.top, 22)
        choice("在主屏幕显示", "有新版时在顶部显示更新按钮", ["开启", "关闭"], [true, false], k["update_show"], enabled: ok) { ks("update_show", $0) }
        choice("检查间隔", "通过 Wi-Fi 下载，点击后才安装", ["1 hour", "12 hours", "24 hours"], [1, 12, 24], k["update_every"], enabled: ok && (k["update_show"] as? Bool) != false) { ks("update_every", $0) }
        if !setupTop {
            SpacedLabel(text: "另一台 Kindle").padding(.top, 22).padding(.bottom, 10)
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
            Text(model.showing.isEmpty ? "任务" : model.showing.prefix(1).uppercased() + model.showing.dropFirst())
                .font(KD.font(20, bold: true)).foregroundColor(c.ink)
            Spacer()
            Text(model.tasksLine).font(KD.font(12)).foregroundColor(c.grey)
        }
        HStack(spacing: 8) {
            Text(model.kindleLine.isEmpty ? "Kindle 当前显示" : "Kindle 当前显示 · " + model.kindleLine)
                .font(KD.font(11)).foregroundColor(c.grey)
            if !model.kindleLine.isEmpty { // green while the Kindle is in touch
                Badge(text: model.kindleOnline ? "Kindle 已连接" : "Kindle 离线", ok: model.kindleOnline)
            }
        }
        .padding(.top, 2)
        HStack(spacing: 10) {
            TextField("添加任务", text: $newTask)
                .textFieldStyle(.plain).font(KD.font(14)).foregroundColor(c.ink)
                .padding(.vertical, 6).overlay(Rectangle().fill(c.ink).frame(height: 1), alignment: .bottom)
                .onSubmit(addTask)
            Pill(title: "添加", solid: true, action: addTask)
        }
        .padding(.top, 14).padding(.bottom, 6)
        if model.tasks.isEmpty {
            Text("暂无任务，在上方添加。").font(KD.font(13)).foregroundColor(c.grey).padding(.top, 10)
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
            Text("点击任务可编辑。拖动左侧的 ≡ 可排序，拖至另一任务可加入其分组。右键查看更多操作。")
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
            TextField("添加子任务", text: $subText)
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
                .help("拖动以移动")
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
                    .buttonStyle(.plain).accessibilityLabel(t.done ? "标记为未完成" : "标记为已完成")
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
                            .help("点击编辑")
                    }
                    if t.pinned {
                        Image(systemName: "pin.fill").font(.system(size: 10)).foregroundColor(c.ink).help("已置顶为 Kindle 当前任务")
                    }
                    HStack(spacing: 0) {
                        if !t.sub { iconButton("plus", "添加子任务", c) { startSub(t.id) } }
                        if !t.sub { iconButton(t.pinned ? "pin.slash" : "pin", t.pinned ? "取消置顶" : "在 Kindle 置顶", c) { model.onTask("pin", t.id, "") } }
                        iconButton("trash", "删除", c) { model.onTask("delete", t.id, "") }
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
            Button(t.done ? "标记为未完成" : "标记为已完成") { model.onTask("toggle", t.id, "") }
            Button("重命名") { editText = t.text; editing = t.id }
            if !t.sub {
                Button("添加子任务") { startSub(t.id) }
                Button(t.pinned ? "取消置顶" : "在 Kindle 置顶") { model.onTask("pin", t.id, "") }
            } else {
                Button("移出分组") { // to just after its group, at the top level
                    let tops = model.tasks.filter { !$0.sub }
                    let i = tops.firstIndex { $0.id == t.parent } ?? (tops.count - 1)
                    let next = i + 1 < tops.count ? String(tops[i + 1].id) : ""
                    model.onTask("place", t.id, "|" + next)
                }
            }
            Button("上移") { model.onTask("up", t.id, "") }
            Button("下移") { model.onTask("down", t.id, "") }
            Divider()
            Button("删除") { model.onTask("delete", t.id, "") }
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
            SpacedLabel(text: "服务器")
            Row(title: "Desk Buddy 运行位置", hint: model.local ? "本机：电脑唤醒且应用开启时可用" : "云端：本机休眠时仍可用") {
                Segmented(labels: ["云端", "本机"], selected: model.local ? 1 : 0) { model.onMode($0 == 1 ? "local" : "cloud") }
            }
            // Only this switch moves Desk Buddy: the Kindle follows by itself (it has nothing to pick).
            Row(title: "Kindle 上", hint: model.local
                ? "在这里切换后 Kindle 会自动跟随。新设备会在 Wi-Fi 中查找本机，出现提示时点击允许。"
                : "在这里切换后 Kindle 会自动跟随。")
            // Phone page: scan with the phone's camera; the page remembers the key after that
            Row(title: "手机网页", hint: model.local
                ? "用手机相机扫码查看和修改任务。本机开启且处于同一 Wi-Fi 时可用，手机会记住登录状态。"
                : "用手机相机扫码，即可随处查看和修改任务，手机会记住登录状态。") {
                if let qr = QR.image(model.phoneURL) {
                    Image(nsImage: qr).interpolation(.none).resizable().frame(width: 112, height: 112)
                        .padding(6).background(Color.white).cornerRadius(6)
                        .help("在手机打开并登录 Desk Buddy，请只向你自己的手机展示。")
                }
            }
            if !model.serverLine.isEmpty {
                Text(model.serverLine).font(KD.font(13)).foregroundColor(c.grey).padding(.top, 14)
            }
            if model.local {
                CloudGuide(model: model).padding(.top, 22)
            }
        case "permissions":
            SpacedLabel(text: "权限")
            PrivacyCard(local: model.local).padding(.top, 12)
            Text("Desk Buddy 所需的 macOS 权限。每项仅启用对应功能，其他功能不受影响。更新后 macOS 可能重新询问。")
                .font(KD.font(13)).foregroundColor(c.grey).padding(.top, 14).padding(.bottom, 4).fixedSize(horizontal: false, vertical: true)
            ForEach(model.perms) { p in
                let unsure = p.title.hasPrefix("Controlling") && p.status.hasPrefix("Couldn't")
                HStack(alignment: .top, spacing: 12) {
                    PermIcon(title: p.title)
                    Row(title: ChineseUI.permission(p.title), hint: ChineseUI.permission(p.detail) + "\n" + permShares(p.title) + (unsure ? "\n" + permShares("Spotify note") : ""),
                        badge: (unsure ? "无法确认（音乐应用的限制）" : ChineseUI.permission(p.status), p.ok)) {
                        Pill(title: ChineseUI.permission(p.button), solid: p.button == "Allow…") { model.onPermission(p.id) }
                    }
                }
            }
        case "performance":
            performancePane
        case "about":
            SpacedLabel(text: "关于")
            HStack(spacing: 16) {
                Buddy(height: 72)
                VStack(alignment: .leading, spacing: 4) {
                    Text("Desk Buddy").font(KD.font(18, bold: true)).foregroundColor(c.ink)
                    Text("让桌上的 Kindle 陪伴你。").font(KD.font(12)).foregroundColor(c.grey)
                }
            }
            .padding(.vertical, 14)
            updatesSection
            SpacedLabel(text: "Desk Buddy").padding(.top, 22)
            Row(title: "欢迎", hint: "首次使用导览：功能、权限与连接 Kindle") { Pill(title: "再次显示") { model.afterUpdate = false; model.welcomeStep = 0 } }
            Row(title: "登录时启动", hint: "登录本机后自动启动，方便 Kindle 随时连接") {
                Segmented(labels: ["开启", "关闭"], selected: model.startAtLogin ? 0 : 1) { model.onStartAtLogin($0 == 0) }
            }
            Row(title: "退出", hint: model.startAtLogin ? "停止运行，直到重新打开或下次登录" : "停止运行，直到重新打开") { Pill(title: "退出") { model.onQuit() } }
            Row(title: "卸载", hint: "从本机移除助手，操作前会确认。") { Pill(title: "卸载…") { model.onUninstall() } }
        default:
            SpacedLabel(text: "本机")
            Text(model.status).font(KD.font(16, bold: true)).foregroundColor(c.ink)
                .padding(.top, 12).padding(.bottom, 6).fixedSize(horizontal: false, vertical: true)
            Row(title: "助手", hint: "报告本机使用状态、打开面板应用并读取音乐") {
                Segmented(labels: ["开启", "已暂停"], selected: model.paused ? 1 : 0) { model.onPause($0 == 1) }
            }
            Row(title: "日历", hint: model.calendar) { Pill(title: "立即同步") { model.onSyncCalendar() } }
                .opacity(model.paused ? 0.35 : 1).disabled(model.paused)
            if !model.lastDeck.isEmpty {
                Row(title: "Kindle 最近操作", hint: model.lastDeck)
            }
            if let comps = model.computers {
                Row(title: "其他电脑", hint: comps.count < 2
                    ? "正在搜索其他设备…请在另一台电脑安装 Desk Buddy。"
                    : "你的 Windows 电脑也已连接。") {
                    Pill(title: "连接电脑…") { model.onConnectComputer() }
                }
            }
            if !model.connectCode.isEmpty {
                // what to type on the other computer (Desk Buddy for Windows asks for both)
                VStack(alignment: .leading, spacing: 6) {
                    Text("在另一台电脑打开 Desk Buddy 并输入：").font(KD.font(12)).foregroundColor(Color.secondary)
                    HStack(spacing: 18) {
                        VStack(alignment: .leading, spacing: 2) {
                            Text("服务器").font(KD.font(11)).foregroundColor(Color.secondary)
                            Text(model.connectServer).font(.system(size: 14, weight: .semibold, design: .monospaced)).textSelection(.enabled)
                        }
                        VStack(alignment: .leading, spacing: 2) {
                            Text("连接码").font(KD.font(11)).foregroundColor(Color.secondary)
                            Text(model.connectCode).font(.system(size: 20, weight: .bold, design: .monospaced)).textSelection(.enabled)
                        }
                    }
                    Text("连接码一次有效，有效期 10 分钟，输入后电脑会自动连接。").font(KD.font(11)).foregroundColor(Color.secondary)
                }
                .padding(12).frame(maxWidth: .infinity, alignment: .leading)
                .overlay(RoundedRectangle(cornerRadius: 8).stroke(Color.secondary.opacity(0.4), lineWidth: 1))
                .padding(.top, 8)
            }
            Row(title: "任务网页", hint: "在浏览器或手机中查看任务") { Pill(title: "打开") { model.onOpenPage() } }
            // Let Claude Code / Codex (running on this Mac) change the tasks: instructions to paste
            // into their instructions file. They read the address and key from Desk Buddy's own file.
            Row(title: "AI 助手", hint: model.aiNote.isEmpty
                ? "让 Claude Code 或 Codex 按你的要求添加、完成和修改任务。将说明复制到各自的指令文件（Claude Code：~/.claude/CLAUDE.md，Codex：~/.codex/AGENTS.md），其中不包含密钥。"
                : model.aiNote) {
                Pill(title: "复制说明") { model.onCopyAI() }
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
                if step > 0 { Pill(title: "返回") { model.welcomeStep = step - 1 } }
                if step < 2 {
                    Pill(title: step == 1 ? "继续" : "开始使用", solid: true) { model.welcomeStep = step + 1 }
                } else {
                    Pill(title: "完成", solid: true) { model.onWelcomeDone() }
                }
            }
            .padding(.horizontal, 24).frame(height: 56)
        }
        .background(c.paper)
    }

    func intro(_ c: Ink) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            Buddy(height: 92).padding(.bottom, 14)
            SpacedLabel(text: "欢迎")
            Text("Desk Buddy 将 Mac 与桌上的 Kindle 连接起来。")
                .font(KD.font(22, bold: true)).foregroundColor(c.ink).padding(.top, 10).fixedSize(horizontal: false, vertical: true)
            VStack(alignment: .leading, spacing: 14) {
                point("calendar", "在 Kindle 上查看日历", "今日安排、月历与会前提醒", c)
                point("music.note", "正在播放", "Spotify 或 Apple Music，支持播放、切歌与音量控制", c)
                point("square.grid.2x2", "从 Kindle 打开应用", "点击应用面板图块，即可在电脑打开", c)
                point("laptopcomputer", "识别你正在使用本机", "让 Kindle 显示对应的信息", c)
            }
            .padding(.top, 22)
            Text("它会在菜单栏中运行。接下来需要处理几项 macOS 权限。")
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
            SpacedLabel(text: model.afterUpdate ? "已更新" : "权限")
            Text(model.afterUpdate ? "Desk Buddy 已更新，请重新允许以下权限。" : "请授予以下权限以启用完整功能")
                .font(KD.font(22, bold: true)).foregroundColor(c.ink).padding(.top, 10).fixedSize(horizontal: false, vertical: true)
            Text("点击允许后 macOS 会询问授权。每项权限仅启用所述功能，其他功能不受影响。")
                .font(KD.font(12)).foregroundColor(c.grey).padding(.top, 6).fixedSize(horizontal: false, vertical: true)
            PrivacyCard(local: model.local).padding(.top, 14)
            VStack(spacing: 0) {
                ForEach(model.perms) { p in
                    let unsure = p.title.hasPrefix("Controlling") && p.status.hasPrefix("Couldn't")
                    HStack(alignment: .top, spacing: 12) {
                        PermIcon(title: p.title)
                        Row(title: ChineseUI.permission(p.title), hint: ChineseUI.permission(p.detail) + "\n" + permShares(p.title) + (unsure ? "\n" + permShares("Spotify note") : ""),
                            badge: (unsure ? "无法确认（音乐应用的限制）" : ChineseUI.permission(p.status), p.ok)) {
                            if !p.ok { Pill(title: ChineseUI.permission(p.button), solid: p.button == "Allow…") { model.onPermission(p.id) } }
                        }
                    }
                }
            }
            .padding(.top, 12)
            HStack(alignment: .top, spacing: 8) {
                Image(systemName: "info.circle").font(.system(size: 12)).foregroundColor(c.grey)
                Text("应用更新后 macOS 可能重新询问权限。如有功能停止工作，请回到侧栏的权限页面检查。")
                    .font(KD.font(12)).foregroundColor(c.grey).fixedSize(horizontal: false, vertical: true)
            }
            .padding(12).background(RoundedRectangle(cornerRadius: 6).stroke(c.line, lineWidth: 1)).padding(.top, 16)
        }
    }

    // the last step: whether the Kindle has really been in touch (not just which server is set)
    func connect(_ c: Ink) -> some View {
        let connected = model.kindleSeen && !model.kindleLine.isEmpty
        return VStack(alignment: .leading, spacing: 0) {
            SpacedLabel(text: "你的 Kindle")
            HStack(spacing: 10) {
                Text(connected ? "Kindle 已连接" : "连接你的 Kindle")
                    .font(KD.font(22, bold: true)).foregroundColor(c.ink).fixedSize(horizontal: false, vertical: true)
                if connected { Badge(text: "已连接", ok: true) }
            }
            .padding(.top, 10)
            if connected {
                Text("最近连接：" + model.kindleLine.replacingOccurrences(of: "seen ", with: "") + "。")
                    .font(KD.font(13)).foregroundColor(c.grey).padding(.top, 6)
                VStack(alignment: .leading, spacing: 14) {
                    point("arrow.triangle.2.circlepath", "改动约 10 秒内显示", "任务、设置与主题双向同步", c)
                    point("slider.horizontal.3", "在这里调整 Kindle", "设置和主题位于 Kindle 页面", c)
                }
                .padding(.top, 22)
            } else {
                KindleSetupCard(model: model).padding(.top, 14)
                Text("Kindle 已有 Desk Buddy？请打开应用，然后：")
                    .font(KD.font(13)).foregroundColor(c.grey).padding(.top, 6).fixedSize(horizontal: false, vertical: true)
                VStack(alignment: .leading, spacing: 14) {
                    if model.local {
                        point("1.circle", "在 Kindle 上点击查找", "在 Wi-Fi 中查找本机", c)
                        point("2.circle", "选中本机，然后在这里点击允许", "核对 Kindle 显示的连接码", c)
                        point("3.circle", "连接完成", "本窗口将显示已连接", c)
                    } else {
                        point("1.circle", "将 Kindle 接入同一 Wi-Fi", "设备的设置 › Wi-Fi", c)
                        point("2.circle", "在 Kindle 书库中打开 Desk Buddy", "设备会自动连接 Desk Buddy 服务器", c)
                        point("3.circle", "无法连接？", "在侧栏服务器页面切换到本机，然后配对 Kindle", c)
                    }
                }
                .padding(.top, 22)
            }
            HStack(alignment: .top, spacing: 10) {
                Image(systemName: "pc").font(.system(size: 15)).foregroundColor(c.ink).frame(width: 20)
                VStack(alignment: .leading, spacing: 2) {
                    Text("还有 Windows 电脑？").font(KD.font(13, bold: true)).foregroundColor(c.ink)
                    Text("从同一下载页面获取 Windows 版并安装。它会在 Wi-Fi 中找到本机，出现提示时点击允许；Kindle 随后会跟随你正在使用的电脑。")
                        .font(KD.font(12)).foregroundColor(c.grey).fixedSize(horizontal: false, vertical: true)
                }
            }
            .padding(12).background(RoundedRectangle(cornerRadius: 6).stroke(c.line, lineWidth: 1)).padding(.top, 22)
            Text("其他功能均在此窗口中，包括主页任务和 Kindle 设置，可随时从菜单栏的 Desk Buddy 打开。")
                .font(KD.font(12)).foregroundColor(c.grey).padding(.top, 22).fixedSize(horizontal: false, vertical: true)
        }
    }
}
