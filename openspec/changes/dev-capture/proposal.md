# Change: dev-capture

## Why

客户端 UI 的样式与交互流程调试目前只能靠人工路径：手动打开游戏窗口、人工截图、
再把图片交给 agent。agent 无法主动、快速地观察运行中的画面（React 菜单层、
wgpu HUD、世界视口各自如何呈现、交互流转是否正确），一轮样式修整被"开游戏—
截图—回看"的往返拖慢。仓库已有 offscreen golden 管线（`cmd/mornlea/capture`），
但它渲染的是独立离屏目标，拿不到挂在窗口上的 WKWebView 菜单层；agent-board
（`cmd/mornlea-agent-board`）证明了进程内 localhost 开发服务的先例，但没有任何
通道能看到游戏画面本身。

需要在运行中的交互式客户端内嵌一个默认关闭的本地调试服务：agent（或用户）随时
用 curl 拉取当前窗口合成截图与短录屏帧序列，把"观察游戏"变成一次 HTTP 请求。

## What Changes

- client ABI v12→v13 新增窗口合成捕获导出：`mornlea_client` 经 `NSWindow
  windowNumber` 用 `CGWindowListCreateImage` 抓取窗口完整合成画面（世界 +
  wgpu HUD + WKWebView 菜单层），绘制进 `CGBitmapContext` 后输出 BGRA8 原始
  字节；两段式容量协议，失败不触碰输出缓冲。Rust 侧不引入 PNG 编码器，PNG
  编码仍在 Go。
- `internal/client` 桥接新增 `Window.Capture` 包装：两段式重试、状态码映射
  （捕获不可用映射为类型化错误，契约违约仍以稳定中文文案 panic）。
- `cmd/mornlea/app` 帧循环新增捕获泵：菜单/游戏两处循环每帧非阻塞检查至多一个
  待执行捕获请求，主线程按需执行捕获，原始像素立即移交服务 goroutine；空闲帧
  零额外工作。app 定义 `CaptureCoordinator` 消费侧接口，不 import 服务包。
- 新增 `cmd/mornlea/devcapture` 子包：仅绑定 `127.0.0.1` 的 `http.Server`
  （agent-board 模式），端点 `GET /status`、`GET /screenshot`（PNG）、
  `GET /record`（PNG 帧序列 + `manifest.json` 的 zip，`format=gif` 追加
  `preview.gif`）；录制参数有界上限；实际端口写入 `~/.mornlea/dev-capture.json`
  （pid + port + 启动时间），优雅关闭时清理。
- 新增 flag `--dev-capture`（默认关）与 `--dev-capture-addr`（默认
  `127.0.0.1:17790`）；与 `--benchmark`/`--capture` 互斥（headless 无窗口）。
- `internal/archcheck` 登记 `cmd/mornlea/devcapture → cmd/mornlea/app` 新边。
- 文档同步：`docs/notes/dev-capture.md`、`docs/README.md` 索引、
  `docs/agents/README.md` 代理使用小节、`cmd/mornlea/AGENTS.md` 模式表、
  新建 `cmd/mornlea/devcapture/AGENTS.md`。

## Capabilities

### New Capabilities

- `dev-capture`: 运行中交互式客户端的本地捕获服务行为契约——按需窗口合成截图、
  有界短录屏帧序列、空闲帧零开销与生命周期清理、捕获桥 ABI 契约与版本同批
  钉位。

### Modified Capabilities

无。golden 视觉验证（`visual-verification`）、benchmark、协议与存档行为零改动；
本变更全部能力默认关闭，不改变任何既有默认路径的可观察行为。

## Impact

- 受影响包：`engine/crates/mornlea_client`、`engine/include`、
  `internal/client`、`cmd/mornlea/app`、`cmd/mornlea/devcapture`（新）、
  `cmd/mornlea`（options/main 接线）、`internal/archcheck`、
  `docs/notes/`、`docs/agents/`、`cmd/mornlea/AGENTS.md`。
- 兼容性：仅 client ABI bump（v12→v13）；Go binary 与 `libmornlea_client`
  保持不可跨版本混装的 release unit（既有启动检测兜底）。协议 v32、玩家/
  区块/世界 schema、engine ABI、benchmark scenario 零触碰。
- 性能：`--dev-capture` 默认关闭，关闭时零监听、帧循环零新增分支开销；启用时
  捕获按需执行（每帧至多一次、空闲零开销），PNG 编码与打包在帧循环外
  goroutine。benchmark 与 perfcheck 数值只记录，不改变退出状态。
- 并发：跨 goroutine 交付的像素 buffer 发送后视为不可变（所有权移交）；同一
  时刻至多一个 outstanding 捕获请求；交付路径非阻塞，帧循环永不等待编码。
- 平台：窗口合成捕获为 darwin 专属（`mornlea_client` 现状即 darwin 窗口
  专属）；首次使用可能触发 macOS「屏幕录制」一次性授权，失败映射为可观察的
  503 与文档化提示，不静默出假图。
