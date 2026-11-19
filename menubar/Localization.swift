// Display-only projections: the original permission titles and status strings
// remain in the model because the upstream logic compares those English values.
import Foundation

enum ChineseUI {
    private static let permissionText: [String: String] = [
        "Calendar": "日历",
        "Controlling Spotify": "控制 Spotify",
        "Controlling Apple Music": "控制 Apple Music",
        "Bluetooth": "蓝牙",
        "Checking…": "正在检查…",
        "Allowed": "已允许",
        "Not asked yet": "尚未询问",
        "Allow…": "允许…",
        "Open Settings": "打开设置",
        "Shows your Mac's calendar on the Kindle: today's events, the month, meeting heads-ups.": "在 Kindle 显示 Mac 日历：今日安排、月历和会前提醒。",
        "Shows what's playing on the Kindle, and lets its buttons play, pause and skip.": "在 Kindle 显示当前曲目，并控制播放、暂停和切歌。",
        "The same for Apple Music, if you use it instead of Spotify.": "使用 Apple Music 时提供相同的音乐功能。",
        "Connects your headphones when you pick them on the Kindle's music screen.": "在 Kindle 音乐页选择耳机时连接设备。",
        "Not allowed: turn it on in System Settings": "未允许 · 请打开系统设置",
        "Only adding events is allowed: it needs full access": "目前仅允许添加日程，需要完整访问权限",
        "Couldn't tell right now": "暂时无法确认",
        "Couldn't check (a music app quirk)": "无法确认（音乐应用的限制）"
    ]
    static func permission(_ value: String) -> String {
        if let translated = permissionText[value] { return translated }
        if value.hasPrefix("Open ") && value.hasSuffix(" to check") {
            return "请打开 " + value.dropFirst(5).dropLast(9) + " 以检查权限"
        }
        return value
    }
}
