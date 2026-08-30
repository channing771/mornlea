# 开发捕获服务（dev-capture）

`--dev-capture` 让运行中的交互式客户端内嵌一个仅绑定回环地址的本地 HTTP 服务：agent 或用户用 curl 拉取当前窗口的合成截图与短录屏帧序列，把「观察游戏画面」变成一次 HTTP 请求。服务默认关闭——不带该 flag 时不监听任何端口、也不写端口发现文件。行为契约由 openspec change `dev-capture` 的 delta spec（`openspec/changes/dev-capture/specs/dev-capture/spec.md`）定义，实现住在 `cmd/mornlea/devcapture/`。

## 启动与端口发现

```bash
# 启动交互式客户端并打开捕获服务（实际监听地址打印到 stdout）
go run ./cmd/mornlea --dev-capture

# 自定义监听地址（主机仍必须是回环地址）
go run ./cmd/mornlea --dev-capture --dev-capture-addr 127.0.0.1:17790
```

- `--dev-capture` 默认关闭；`--dev-capture-addr` 默认 `127.0.0.1:17790`，单独给出（不带 `--dev-capture`）无任何效果。
- 可与 `--connect` 组合；与 `--benchmark`/`--capture` 互斥——那两条是无头路径，没有窗口可捕获，flag 解析层直接拒绝组合。
- 服务只接受回环主机（`127.0.0.1`、`::1`、`localhost`）：即使 `--dev-capture-addr` 传入了其他地址，也在绑定之前被服务内的防御闸拒绝，不依赖 flag 默认值兜底。
- 默认端口被占用时从请求端口起逐个 +1 顺延（至多 10 次），stdout 打印实际绑定的地址（`开发捕获服务已启动: http://…`）。
- 服务启动时写 `~/.mornlea/dev-capture.json`（字段 `pid`、`port`、`started_at`），进程优雅退出时删除——这是 stdout 之外的第二个发现渠道。启动失败（端口耗尽、发现文件不可写）只告警并禁用本次画面捕获，游戏照常运行。

## 端点契约

三个端点都只接受 GET（其余方法 405）；错误统一为 `{"error": <稳定中文文案>}`，文案稳定是契约的一部分，观察者按文案分流处理。

### `GET /status`

服务与客户端状态的 JSON 快照：

- `pid` 进程号，`port` 实际监听端口（确认目标进程用）；
- `phase` 客户端相位：`menu`/`settings`/`starting`/`paused`/`game`，未知为 `unknown`；
- `window` 当前窗口内容宽高（未知时缺省）；
- `recording` 是否有 `/record` 采样进行中；
- `last_capture` 最近一次捕获摘要：`at`（RFC3339）、`width`、`height`，失败时带 `error`；从未捕获为 `null`。捕获侧的异常（如授权缺失）在这里可见，不静默。

### `GET /screenshot`

单张窗口合成截图：

- 成功 200，`image/png`；
- 捕获不可用（屏幕录制授权缺失等运行期预期条件）→ 503，文案「窗口合成捕获不可用：请检查系统设置的屏幕录制授权后重试」；
- 等待交付超时（默认上限 10s，帧循环已停止或长期阻塞时触达）→ 503；
- 同一时刻只有一个 outstanding 捕获请求，并发请求立即 503（「已有捕获请求在执行或帧循环已停止，请稍后重试」），不排队。

### `GET /record`

有界短录屏帧序列：

- 查询参数：`seconds`（默认 5，`0 < seconds ≤ 20`）、`fps`（默认 8，`0 < fps ≤ 12`）、总帧数 `seconds×fps ≤ 240`、`format=png|gif`（默认 `png`）；参数不可解析或越界一律 400，且不产生任何帧捕获；
- 已有录制进行中 → 503；
- 成功 200，`application/zip`，内含：
  - `frames/frame-NNNN.png`——逐帧窗口合成图，自 1 起四位编号；
  - `manifest.json`——`seconds`/`fps`/`format` 回显请求参数；`requested_frames` 为请求帧数，`frame_count` 为实际入包帧数；`frames[]` 逐帧给出 `index`、`file`、`timestamp_ms`（相对录制开始的毫秒数，单调不减）、`width`、`height`；`dropped_frames` 为采样拍被并发请求占道而跳过的帧数；`gif` 标记有无预览动图；
  - `format=gif` 时附加 `preview.gif`（调色板量化的预览动图，方便快速回看）。
- 失败语义分两层：单帧捕获失败或单帧交付超时只终止采样，已收到的帧连同终止原因（`manifest.error`）仍以 200 zip 交付——局部证据好于全盘丢弃；整段录制超过总时长上限（名义时长 + 30s 固定余量 + 每帧 1s 捕获/编码预算 × 总帧数）仍完不成（帧循环不可用）才以 503 放弃整次录制，错误 JSON 回显公式三段数值。

## 语义与边界

- 截图内容是窗口的完整合成画面：世界视口 + wgpu HUD + WKWebView 菜单层。捕获经 client ABI 的窗口捕获桥在窗口 poll 线程按需执行；无待办请求的空闲帧零开销，PNG/zip/GIF 编码全部发生在帧循环之外的服务 goroutine。
- 录制是 ≤12fps 的低帧率采样（每帧「发请求 → 等交付 → 按目标间隔等待」串行推进），面向 UI 流程调试取证，不适合流畅视频。
- 屏幕录制授权：首次捕获可能触发 macOS 的一次性授权弹窗；未授权时 macOS 可能返回不含游戏窗口内容的桌面画面而不是报错。画面里没有游戏窗口时，检查「系统设置 → 隐私与安全性 → 屏幕录制」授权并重试；`/status` 的 `last_capture.error` 会同步显示捕获失败。
- 窗口最小化时合成画面可能过期或失败——观察者自行保证窗口可见。
- 底层 `CGWindowListCreateImage` 已被 Apple 标记弃用，当前 macOS 全量可用；弃用面集中封装在 `mornlea_client` 的捕获模块，未来替换为 ScreenCaptureKit 时 FFI 契约与 Go 侧不动（后继方案评估见 change `dev-capture` 的 design）。

## agent 工作流示例

```bash
# 实际端口优先读发现文件（pid 与端口一起确认目标进程）
cat ~/.mornlea/dev-capture.json
curl -s http://127.0.0.1:17790/status

# 截图
curl -s -o /tmp/mornlea-shot.png http://127.0.0.1:17790/screenshot

# 录制 2s @ 8fps 并解包逐帧查看
curl -s -o /tmp/mornlea-rec.zip 'http://127.0.0.1:17790/record?seconds=2&fps=8'
unzip -d /tmp/mornlea-rec /tmp/mornlea-rec.zip   # frames/frame-0001.png … manifest.json
```
