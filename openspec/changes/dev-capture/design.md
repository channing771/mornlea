# Design: dev-capture

## 总体形状

四层，自底向上：

1. **Rust 捕获原语**（`engine/crates/mornlea_client`）：新增
   `mornlea_client_window_capture` 单一导出，一次调用完成「取窗口合成图 →
   绘入 CGBitmapContext → 拷出 BGRA8 字节」。
2. **Go ABI 桥**（`internal/client`）：`Window.Capture` 包装两段式容量协议与
   状态码映射，是唯一接触新导出的包。
3. **帧循环捕获泵**（`cmd/mornlea/app`）：菜单/游戏两处循环每帧一次非阻塞
   待办检查，主线程按需执行捕获，像素立即移交。
4. **本地服务**（`cmd/mornlea/devcapture`，新包）：`http.Server` + 采样编排 +
   PNG/zip/GIF 编码 + 端口发现文件；实现 app 定义的 `CaptureCoordinator`
   接口。

## 数据所有权与并发边界

- **像素 buffer 单向移交**：捕获在帧循环线程（与 `Window.Poll` 同线程，沿用
  既有窗口 FFI 线程约束）执行，产出的 BGRA 切片经 channel 发给服务 goroutine；
  按「跨 goroutine 发送成功后的消息及其切片视为不可变」约定，发送后循环侧
  不再持有或改写。编码（`image/png`、zip、GIF）全部在服务 goroutine，帧循环
  永不编码。
- **单 outstanding 请求**：泵与服务的交接是一个容量 1 的请求 channel。
  `GET /screenshot` 与 `GET /record` 共用同一通道；录制不预先排队 N 个请求，
  而是服务侧「发一帧请求 → 等交付 → 按目标间隔等待 → 下一帧」，循环侧契约
  保持最简（每帧至多一个捕获、永不排队）。帧间隔等待在服务 goroutine，
  帧循环只承受捕获本身的按需开销（一次 GPU/窗口合成拷贝，约 10-30ms）。
- **超时与关闭**：HTTP handler 等待交付结果带超时（截图 10s；录制按
  `seconds×帧捕获耗时分位` 放宽并设上限），超限 503；帧循环退出后无人应答，
  同一超时路径自然收敛。服务随进程优雅关闭（`http.Server.Shutdown`），
  端口文件 `~/.mornlea/dev-capture.json` 在启动时写、退出时删。
- **录制丢帧**：交付 channel 满时（编码慢于采样）循环侧非阻塞放弃该帧请求，
  manifest 记录丢帧数；帧循环永远不被编码速度反压。

## 依赖方向

- 新边 `cmd/mornlea/devcapture → cmd/mornlea/app`：devcapture 实现 app 的
  `CaptureCoordinator` 并消费 app 状态访问器（phase、窗口尺寸）。app MUST NOT
  import devcapture（接口定义在 app 侧，consumer 侧接口模式，同
  `capture.SceneApplication` 先例的反向）。
- `cmd/mornlea/devcapture` 不 import `cmd/mornlea/capture`：BGRA8→NRGBA
  转换是约 15 行的字节序交换，capture 侧同名逻辑未导出且二者契约来源不同
  （golden 离屏 readback vs 窗口合成捕获），各自就近注释指向自己的 ABI 契约。
  仅为此注册 capture→devcapture 或反向边、或把未导出助手提升为跨包导出面，
  均大于收益，接受这份小体量重复。
- archcheck：`clientCommandAllowedEdges` 登记 `devcapture → app`。
- Rust 侧全部改动收敛在 `mornlea_client`（darwin 窗口域），`mornlea_engine`
  不动；Go 侧只有 `internal/client` 新增绑定。

## FFI 契约（client ABI v12→v13）

```
int32_t mornlea_client_window_capture(
    struct mornlea_client_window *window,
    uint64_t abi_version,
    uint8_t *out_pixels, uint64_t out_capacity,
    uint64_t *out_required,      /* 溢出时回填所需字节数 */
    uint32_t *out_width,         /* 成功与溢出时回填合成图尺寸 */
    uint32_t *out_height);
```

- 实现链：`NSWindow windowNumber` → `CGWindowListCreateImage`（best
  resolution，取 backing scale 尺寸）→ `CGBitmapContextCreate`（BGRA8
  premultiplied first / little endian，与既有 readback 字节序一致）→
  `CGContextDrawImage` → 拷贝行字节。Rust 不编码 PNG。
- 校验顺序镜像 `mornlea_client_render_readback`：abi_version → 句柄 →
  出参指针 → 容量；容量不足返回独立溢出状态并回填 `*out_required`，输出
  缓冲原样；捕获不可用（窗口号取不到、CGImage 为空、位图上下文创建失败）
  返回独立「不可用」状态——这是运行期预期条件，映射为 Go 类型化错误而非
  panic；其余违约沿用稳定中文文案 panic。
- 弃用 API 处置：`CGWindowListCreateImage` 已被 Apple 标记弃用（推荐
  ScreenCaptureKit），当前 macOS 仍可用。集中封装在单一 Rust 模块，未来
  被移除时只替换该模块实现，FFI 契约与 Go 侧不动。
- 线程约束：必须在窗口 poll 线程调用（与全部既有窗口导出一致），由 Go 帧
  循环串行化保证。

## Go 侧接线

- `cmd/mornlea/app`：新增 `dev_capture.go` 定义
  `CaptureRequest{Done chan<- CaptureOutcome}`、`CaptureOutcome{Pixels []byte,
  Width, Height int, Err error}` 与 `CaptureCoordinator` 接口
  （`PendingCapture() (CaptureRequest, bool)` + `CompleteCapture(...)`）；
  `Application` 增加可空协调器注入口（main 接线，nil 时零开销）；
  `interactive.go` 菜单与游戏两处循环在 `Poll()` 之后检查一次。
- `cmd/mornlea/devcapture`：`Service`（实现协调器、持有请求通道与状态）、
  HTTP mux（`/status`、`/screenshot`、`/record`）、采样编排器、
  zip/manifest/GIF 组装、端口文件读写。默认地址 `127.0.0.1:17790`，绑定失败
  自动顺延端口并把实际端口写入发现文件。
- `cmd/mornlea`：`--dev-capture`（bool，默认关）、`--dev-capture-addr`
  （string）；`parseMainOptions` 互斥矩阵追加与 `--benchmark`/`--capture`
  的互斥；`main.go` 在 app 启动后、进入循环前拉起服务并注册关闭钩子。

## 被否决的替代方案

- **WKWebView `takeSnapshot` + 窗口化 wgpu readback 合成**：免屏幕录制授权，
  但需要两个新 FFI（webview 快照为异步回调需阻塞转换、窗口化 surface 需
  blit-COPY_SRC 扩展）、Go 侧图层合成与对齐逻辑，且双源时间不同步可能撕裂。
  为调试工具引入三倍 ABI 面，否决。
- **shell 出 `screencapture -l <windowNumber>`**：零 Rust 改动，但游戏进程
  子进程旁路违反「Go 只经既定 ABI bridge 调用 Rust」的边界方向，且每次截图
  一次进程创建、录屏间隔受进程启动延迟支配，否决。
- **ScreenCaptureKit 流式录制**：帧回调持续投递适合高帧率录屏，但引入异步
  delegate/流生命周期管理，复杂度远超「UI 流程 ≤12fps 调试采样」的需求；
  作为 `CGWindowListCreateImage` 未来被移除时的后继方案记录在案。
- **服务旁路帧循环、Rust 侧自建捕获线程**：绕开 Go 帧循环的线程串行化会与
  winit pump_events 单线程模型冲突，且把编排散进 Rust；否决。

## 风险与回退

- **屏幕录制授权**：首次捕获可能触发系统授权弹窗；未授权时 macOS 可能返回
  不含窗口内容的桌面图而非报错。缓解：`/status` 暴露最近捕获结果与错误，
  文档写明「画面不含游戏窗口 → 检查系统设置的屏幕录制授权并重试」；服务对
  硬失败返回 503，不试图静默重试。
- **窗口最小化/遮挡**：合成图可能过期或失败；行为记录进文档，不做特殊处理
  （调试工具，观察者自己保证窗口可见）。
- **回退**：`--dev-capture` 默认关闭，整 change 单提交系列可 revert；ABI
  版本回退随 revert 自动复原。
- **验证方法**：Rust 模块层纯逻辑单测（校验顺序/溢出回填可用桩窗口覆盖的
  部分随既有 FFI 测试矩阵走）；Go 侧 `httptest` 全覆盖三端点（fake
  Coordinator，不启动真实窗口）；app 泵测试以计数桩断言空闲零调用与单帧
  单次上限；基线/钉位由既有 `internal/archcheck` 与 `window_test.go` 兜底。
  自动测试不启动前台游戏窗口；最终人工验收一次真实链路。

## 受影响文件

- `engine/include/mornlea_client.h`、
  `engine/crates/mornlea_client/src/{ffi.rs,window.rs,capture.rs(新),lib.rs}`
- `internal/client/{window.go,window_test.go}`
- `cmd/mornlea/app/{dev_capture.go(新),interactive.go,app_dependencies.go 或
  accessors.go}` + 泵测试
- `cmd/mornlea/devcapture/`（新包：service/http/record/portfile/bgra + 测试）
- `cmd/mornlea/{options.go,options_test.go,main.go}`
- `internal/archcheck/dependency_test.go`
- 根 `AGENTS.md`（client ABI v12→v13 基线行）
- `docs/notes/dev-capture.md`（新）、`docs/README.md`、
  `docs/agents/README.md`、`cmd/mornlea/AGENTS.md`、
  `cmd/mornlea/devcapture/AGENTS.md`（新）
