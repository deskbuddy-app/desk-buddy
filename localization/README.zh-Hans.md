# Desk Buddy 简体中文候选源码

基于官方 v1.3，固定 commit `7245f1a344efd0584246b96f4d0023ce2666e2c3`。本目录只包含汉化及其字体、排版、打包资源和验证工具；尚未完成 Kindle 实机、系统中文输入法及真实三端联调验收。

## 范围与兼容

- Kindle：任务、日历、设置、连接引导、导览、专注、音乐、天气等自有界面的静态文案，以及日期显示。原有英文键盘保留，不增加拼音输入法。
- Mac：主窗口、设置、权限说明、任务操作、菜单、连接／迁移／卸载确认，以及应用控制的系统权限用途说明。权限模型中的英文标题、状态及判断保持不变，在视图处翻译。
- 手机：任务与子任务、说明、置顶、排序、完成／删除、日历表单、无障碍标签与提示。保留表单名、路由、转义与原有提交逻辑。
- 中文字体为 Source Han Sans SC Regular 2.005R，全量原文件，无裁剪；英文仍使用原字体。字体由应用进程使用，不安装到系统字体目录。
- API／JSON／数据库键、profile、动作 ID、时区解析、输入长度限制、更新、网络、权限、数据同步逻辑保持上游实现。发现的原版安全问题仅记录。
- `worker/src/release.json` 及 `worker/public/kindle/` 由原有打包流程生成，载荷摘要、大小、路径和内嵌发布说明是构建元数据，不是新的更新机制。

保留英文：品牌及应用名称、用户任务／日历／音乐内容、原版历史更新说明和引言、英文文字时钟、部署 AI 提示词及部分运行时诊断。该候选版没有新增语言切换功能，不以替换用户内容来提高翻译覆盖率。

## 变更入口

`kindle/desk/chinese.go` 提供缺字回退、连续中文显示换行、显示日期和列表说明。字体测量与绘制使用同一 Face，原有英文分词路径保持原样。`menubar/Localization.swift` 仅转换权限界面的展示值；`HelperUI.swift` 注册进程内字体并设置回退。手机只改 `worker/src/index.js` 的页面／样式／相对时间显示区域。

`zh-Hans.tsv` 是工作术语表，含不同上下文的译文；不能不分上下文地批量替换源文件。正式差异以 Git diff、源文件及验证结果为准。字体来源、版本与摘要见仓库根目录 `LOCALIZATION.md`。

## 构建与验证

使用原有 Go、Swift 和 npm 锁文件依赖。没有新增生产依赖或框架。先在 `worker` 下按 `package-lock.json` 安装原有依赖；如使用 `npm ci --ignore-scripts`，还需执行 `node node_modules/bun/install.js` 完成既有 Bun 包的安装步骤。

构建可使用完整源码快照。原版差分验证还需要本地 Git 仓库中存在上述基线 commit；可在该版本的独立克隆应用本次补丁后运行，单独解压的源码 ZIP 不包含 .git。

在仓库根目录执行：

```sh
python3 localization/build-candidate.py
python3 localization/verify.py --out /absolute/path/to/verification
python3 localization/check-scope.py
go run localization/tools/scope.go
```

构建脚本只在本检出目录生成 `menubar/dist`，不安装或启动应用。默认使用本机已有的 macOS 26.5 SDK；也可通过 `SDKROOT` 指定。当前默认 27.0 SDK 缺少 SwiftUIMacros 插件的问题，在原版和候选版均复现；没有为绕过它改写产品代码。

产物为 `menubar/dist/Desk-Buddy-for-Mac.zip` 和 `Desk-Buddy-for-Kindle.zip`。Mac 为 ad hoc 签名，不等于 Developer ID 签名或公证；Intel 仅完成交叉构建，未运行验收。

## 使用与回滚边界

本候选版保留上游应用身份、数据路径、启动项及更新行为，不能仅凭更名就当作隔离实例。离线预览不能代替实机验收。此前本地 Mac 安装及启动已有代理观察的冒烟记录，但不代表人工验收通过。本次贡献整理没有启动生产主程序、迁移真实数据库、替换现有安装或写入 Kindle。

复核源码可用 `git diff 7245f1a344efd0584246b96f4d0023ce2666e2c3`。需要回看原版时，可从该 commit 另建干净检出，不覆盖当前修改。上游 MIT 许可和字体 OFL 保留在源码及候选打包资源中。
