// Offline UI export only. Does not run App, Backend, permission requests or installers.
import AppKit
import SwiftUI

@main
struct Preview {
 @MainActor static func main() throws {
  NSApplication.shared.setActivationPolicy(.prohibited)
  KD.registerFonts()
  let out=URL(fileURLWithPath:CommandLine.arguments[1],isDirectory:true)
  try FileManager.default.createDirectory(at:out,withIntermediateDirectories:true)
  guard NSFont(name:"SourceHanSansSC-Regular",size:20) != nil else { fatalError("Chinese resource font was not registered") }
  for page in ["home","kindle","permissions","server","about"] {
   let model=HelperModel();model.section=page;model.status="运行中（离线样例）";model.version="1.3";model.showing="Mac 列表";model.tasksLine="已完成 0 / 2";model.kindleLine="86% · 测试样例";model.computers=["mac"];model.local=true;model.kindleSettings=["theme":"light","font":"sans","compact":false,"refresh":1,"style":"plain"]
   model.tasks=[.init(id:1,text:"准备中文任务评审材料，核对预算与交付清单。",done:false,pinned:true,sub:false),.init(id:2,text:"检查中文与 English 混排",done:false,pinned:false,sub:false)]
   model.perms=[.init(id:0,title:"Calendar",detail:"Shows your Mac's calendar on the Kindle: today's events, the month, meeting heads-ups.",status:"Allowed",ok:true,button:"Open Settings"),.init(id:1,title:"Controlling Spotify",detail:"Shows what's playing on the Kindle, and lets its buttons play, pause and skip.",status:"Not asked yet",ok:false,button:"Allow…")]
   for dark in [false,true] {
    let view=HelperWindow(model:model,scrolls:true).frame(width:900,height:760).environment(\.colorScheme,dark ? .dark:.light)
    let hosting=NSHostingView(rootView:view)
    hosting.frame=NSRect(x:0,y:0,width:900,height:760)
    let window=NSWindow(contentRect:hosting.frame,styleMask:[.borderless],backing:.buffered,defer:false)
    window.contentView=hosting
    hosting.layoutSubtreeIfNeeded()
    guard let bitmap=hosting.bitmapImageRepForCachingDisplay(in:hosting.bounds) else {fatalError("No bitmap")}
    hosting.cacheDisplay(in:hosting.bounds,to:bitmap)
    guard let png=bitmap.representation(using:.png,properties:[:]) else {fatalError("No preview image")}
    try png.write(to:out.appendingPathComponent("mac-\(page)-dark-\(dark).png"))
   }
  }
  print("Exported 10 offline Mac UI previews; no application services started")
 }
}
