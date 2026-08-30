# dev-capture Ledger

## Setup

- OpenSpec change: `dev-capture`。
- 需求来源：控制会话与用户对话（2026-08-30），用户拍板两个取舍——像素来源
  取「窗口合成截图」（否决 wgpu readback 扩展与双源合成），录屏交付取
  「PNG 帧序列 zip（`format=gif` 附加预览）」。
- 执行基线：main HEAD = `e1e2e287cb3454e6bbca6bf5bcd7cf9e92482efc`，工作树
  干净；client ABI v12（`engine/include/mornlea_client.h` 12u，
  `internal/client/window_test.go` 钉位 12），本 change 升 v13。
- 执行方式：subagent-driven-development，控制会话不直接实现；每任务 fresh
  implementer + 独立规格/质量双评审，裁决记录于本 ledger。

## Task 1: OpenSpec change 产物

- 产物：proposal.md、design.md、specs/dev-capture/spec.md（4 条 ADDED
  Requirement、11 个 Scenario）、tasks.md、本 ledger。
- 验证：`openspec validate dev-capture --strict --no-interactive` 与
  `openspec validate --all --strict --no-interactive` 结果见任务勾选前回执
  （`Change 'dev-capture' is valid`；`Totals: 79 passed, 0 failed (79 items)`）。
- Commits: `bcc053e8` `docs: add dev-capture change products`。

## Task 2: Rust 捕获原语 + client ABI v13 + Go 桥接

- Round 1（implementer `be5ff22b` `feat(client): add window composite capture
  with client abi v13`）：
  - 实现：`capture.rs` 新模块（CG 最小 extern 绑定 + RAII + 行翻转/去 padding
    纯函数 6 单测）、`ffi.rs` 状态码 CAPTURE_OVERFLOW=8 / CAPTURE_UNAVAILABLE=9
    + 导出 `mornlea_client_window_capture` + FFI 测试、header 13u、
    `Window.Capture` 两段式 + `ErrCaptureUnavailable`、Go 钉位 13、
    根 AGENTS.md 基线 v13。验证：`make rust-check` 绿、client 97 passed、
    `go test ./internal/client -race` ok、archcheck ok。
  - 自报偏差：校验顺序按被镜像的 `mornlea_client_render_readback` 实际顺序
    （abi → 出参指针 → 句柄）而非 brief 箭头——评审裁决成立，design.md 箭头
    系笔误已由控制会话修正（abi_version → 出参指针判空与空-容一致性 → 句柄 →
    容量）。
  - 评审（独立评审子代理）：规格合规 FAIL + 代码质量 FAIL——Blocker B-1/B-2：
    `CG_WINDOW_LIST_OPTION_INCLUDING_WINDOW`（1<<0，实为 OnScreenOnly 位）、
    `CG_WINDOW_IMAGE_BEST_RESOLUTION`（1<<8，非文档位）与 SDK 头
    （均为 1<<3、u32）不符，真实窗口下会静默截错图/整屏；extern 形参宽度
    u64 与 CF_OPTIONS(uint32_t) 不符且 SAFETY 注释论断错误。溢出路径无头
    不可测的豁免成立，缺陷只会进人工验收。
  - 修复轮 1：`4c553f3b` `fix(client): align window capture cg option bits
    with sdk headers`——两个枚举常量改 u32 = 1 << 3（implementer 读本机 SDK
    `CGWindow.h` 逐项核实，并确认 1 << 0 即 `kCGWindowListOptionOnScreenOnly`
    的语义陷阱）、extern 形参改 u32、SAFETY 注释改写为
    `CF_OPTIONS(uint32_t, ...)` 论断、新增
    `window_list_option_values_match_sdk_header` 钉值测试顺带钉住位图布局
    手抄面。验证：`make rust-check` 绿、`cargo test -p mornlea_client capture
    --locked` 10 passed、`go test ./internal/client -race` ok 4.682s。
  - 控制会话核对修复 diff 逐项对应评审清单，Blocker 闭环；其余评审发现
    （Praise 若干、`#![allow(deprecated)]` 为意图标记的 Minor）不要求返修。
  - 双裁决终局：规格合规 PASS、代码质量 PASS。
- Commits: `be5ff22b` `feat(client): add window composite capture with client
  abi v13`、`4c553f3b` `fix(client): align window capture cg option bits with
  sdk headers`。
- 遗留（记录不阻塞）：溢出回填与真实画面内容无头不可测，留给 Task 7.2
  人工验收核对截图尺寸与图层内容。

## Task 3: app 帧循环捕获泵

- Commits: `522c7d6a` `feat(app): add frame loop capture pump`。
- 实现：`dev_capture.go` 定义 `CaptureOutcome`/`CaptureRequest`（`Done` 为
  `chan<-` 单向，类型系统阻止泵侧接收/关闭）与 `CaptureCoordinator` 接口；
  泵 `(*Application).pumpDevCapture()` 无状态零分配，经私有 `captureSource`
  接口收窄捕获来源（`*client.Window` 隐式满足，无头替身可注入，无捕获能力
  交付 `client.ErrCaptureUnavailable`）；注入口为 `Dependencies` 字段 +
  `SetCaptureCoordinator` setter（main 时序晚于 app 构造，构造期注入会环，
  setter 与 `runCapture` 适配器 consumer 先例一致）；`interactive.go`
  `runMenuPhase`/`runGamePhase` 在 `Poll()` 后渲染前各接入一次（暂停为
  `runGamePhase` 内覆盖层状态，无独立循环，两处覆盖暂停帧）。
- TDD：7 测试先行 RED（undefined 符号）→ GREEN，含两条经 `RunInteractive`
  真实走循环的集成断言（单帧恰好 1 检查/1 捕获/1 交付，凭据恒等 + 逐字段）。
- 验证：`go test ./cmd/mornlea/app -race -count=1` ok 66.7s、vet/gofmt 干净、
  `go build ./...` ok。
- 评审（独立评审子代理）：规格合规 PASS + 代码质量 PASS，零返修。评审提示
  下游必查项：4.1 需钉「`Done` 满时帧循环不被阻塞」（任务 3.1 只以泵内无
  channel 操作间接钉住非阻塞语义）。

## Task 4: devcapture 服务包

- 实现：新包 `cmd/mornlea/devcapture`（全文件 `//go:build darwin`，与 app 包
  门禁一致）——`Service` 实现 `app.CaptureCoordinator`（容量 1 Done +
  select/default 满丢弃计数，nil `Done` 同防御路径）；`/status`、
  `/screenshot`（10s 上限）、`/record`（单帧推进编排、假时钟注入、越界 400、
  zip manifest、GIF 可选、并发录制 503 互斥）；`bgra.go` 转换与
  `mornlea_client` 字节契约对齐（未 import capture 包）；端口发现文件
  `~/.mornlea/dev-capture.json`（TempDir 可注入）；archcheck 登记
  `devcapture → app` allowed + required 双边与 drift 合成同步。上游必查项
  （Done 满不阻塞帧循环）以 watchdog 测试闭环。
- TDD：27→30 测试；首轮 RED 两处为语义错误（超时残留请求自愈语义、截止检查
  位置），修实现结构后全绿。
- 验证：`go test ./cmd/mornlea/devcapture -race -count=1` ok、
  `go test ./internal/archcheck -count=1` ok、`go build ./...` ok。
- 评审（独立评审子代理）：规格合规 PASS + 代码质量 PASS，1 项 Important
  （/record 并发互斥 503 分支零测试覆盖）+ 6 项 Minor（重复 Start 覆写端口
  文件、busy 文案误导、serveStatus 死代码、Stop 注释夸大、回环绑定仅靠约定
  与 spec MUST 有距、bgra 色彩空间出处强于源）。
- 修复轮 1：`4afda845` `fix(devcapture): harden record mutex and start
  idempotence`——7 项全落地：补 `TestRecordRejectsConcurrentRecording`；Start
  幂等检查提前至 listen 前（失败零副作用，端口文件逐字段断言）；busy 文案补
  「或帧循环已停止」；删死代码；Stop 注释改为进程退出兜底；`listen()` 增加
  回环防御闸（host ∉ {127.0.0.1, ::1, localhost} 绑定前拒绝，三例拒绝测试 +
  拒绝路径不写发现文件），spec「MUST 仅绑定回环地址」由默认值 + 防御闸双保险
  闭合；bgra 色彩空间表述改为与 `cmd/mornlea/capture` 同口径的准确版本。验证：
  devcapture -race 30 测试全绿、archcheck 绿、build 绿。控制会话核对 diff
  逐项对应评审清单，闭环。双裁决终局：规格合规 PASS、代码质量 PASS。
- Commits: `8646c313` `feat(devcapture): add local capture http service`、
  `4afda845` `fix(devcapture): harden record mutex and start idempotence`。
- 勘定（控制会话记入 tasks.md 5.1）：app 现无 phase 访问器，
  `devcapture.StatusSource` 的 phase 契约需 app 补一个最小并发安全访问器，
  归 5.1 交付（Files 增 `cmd/mornlea/app/{dev_capture.go,accessors.go}`）。
