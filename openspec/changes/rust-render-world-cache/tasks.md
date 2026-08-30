# Tasks: Rust RenderWorld Cache

> Task 1 已建立 change、基线与 ledger。Tasks 2–5 是 feature 分支在独立 pre-main-integration
> 基线已经完成并评审的历史工作，其 v12 表述记录当时事实，不代表合并后的目标版本。
> 已评审 Task 6.1 保留固定父 `8b8891a3` / engine ABI v8 的历史事实，不得改写。Tasks 1–6.6
> 的 completed 状态、selected-main 证据与历史文字同样不得改写。旧 selected main
> `e1e2e287` 上的完整 PASS、`5e5243a7` 的 17/18 binding failure、`bcc053e8` 无关 planning
> sync，以及 `eccdca39` 因 `be5ff22b` contract drift 产生的 0/18 preflight cancellation 都是
> 收尾历史。冻结 selected-main parent `9bb84c68` 已把 capture 与 app frame-loop pump
> 分配给 client ABI v13，包含 `4c553f3b` 的 SDK option bits code fix、`a83192b7` 的 docs-only
> Task 2 review bookkeeping、`522c7d6a` 的 app pump 实现及 `9bb84c68` 的 docs-only Task 3
> review bookkeeping。Task 6.8 已完成 merge `6f622407`，其双亲为 feature `9dc22f1b` 与
> selected-main `9bb84c68`；该父现被冻结，`8646c313` 或任何更晚 main 均不再绑定本 change。
> Task 6.8 的实现与独立 reviews 仍暂停且 unchecked，Tasks 6.8–6.10 继续作为 pending 收尾；
> 当前进度保持 25 total / 22 complete / 3 remaining，最终目标是 client ABI v14 /
> inherited engine ABI v9。
> implementer、reviewer、两项 verdict、findings、修复轮次、验证和裁决必须先写入
> `ledger.md`，再勾选任务。控制会话不得直接实现。

## 2. 实现纯 Rust RenderWorld 与 MRW1 原子状态机

- [x] 2.1 在 `engine/crates/mornlea_client/src/render/` 先为 MRW1 的 24/32 字节布局、
  4 MiB/4096 限制、reserved、tag、payload、坐标、三态 storage、world reset 首 record、
  epoch/revision/tombstone 和非法 batch 原子失败建立 RED 测试；indexed 必须覆盖 4-bit 恰为
  256 words、8-bit 恰为 512 words、短缺/超额 word count 与尾随 bytes 的拒绝及原子性；验证：
  `cd engine && cargo test -p mornlea_client --locked render::world`。
- [x] 2.2 新增紧凑的 `RenderWorld` parser/cache，并在 renderer 内建立仅更新派生 cache 的
  内部入口；不展开 4096 block、不创建 worker/GPU pool、不接入 mesh、visibility、upload
  或 draw；验证：`cd engine && cargo test -p mornlea_client --locked render::world`。
- [x] 2.3 补全 malformed length、palette slot、packed word、overflow 与状态机边界的性质或
  fuzz 测试；验证：`cd engine && cargo test -p mornlea_client --locked`。
- [x] 2.4 取得该任务的单一独立 review，其报告同时包含 spec-compliance 与 quality verdict；
  以定点 Cargo 测试和 `git diff --check -- engine/crates/mornlea_client/src/render` 复核，
  将 verdict 与裁决记入 `ledger.md`。

## 3. 实现 Go MRW1 编码器

- [x] 3.1 在 `internal/client/` 先为 `world.ContainerSnapshot` 的 single、indexed、direct、
  column、tombstone、reset、4/8/15-bit 及坐标边界建立 MRW1 encoder RED 测试；验证：
  `go test ./internal/client -run 'Test(BuildRenderWorld|EncodeRenderWorld)' -count=1`。
- [x] 3.2 实现有界、checked 的 Go MRW1 batch encoder 与完整 chunk update 构造；它不导入
  network、不发送隐式 reset、不展开 4096 block，且尚不接入实时 app；验证：
  `go test ./internal/client -run 'Test(BuildRenderWorld|EncodeRenderWorld)' -count=1` 与
  `go test ./internal/world -run 'Test.*Snapshot' -count=1`。
- [x] 3.3 取得该任务的单一独立 review，其报告同时包含 spec-compliance 与 quality verdict；
  以 Go 定点/race 测试和 `go test ./internal/archcheck -count=1` 复核，将 verdict 与裁决
  记入 `ledger.md`。

## 4. 升级 client ABI 到 v12 并接通 cache-only 输入入口

- [x] 4.1 在 `engine/include/mornlea_client.h`、`engine/crates/mornlea_client/src/` 与
  `internal/client/` 保留全部接受 ABI version 参数的既有 client exports matrix，并为新输入入口
  建立 RED FFI/bridge matrix：ABI 优先，随后 non-zero/bounded length、non-null pointer、
  address range/no overflow、existing handle、MRW1 layout/capacity，再到合法 batch；验证：
  `cd engine && cargo test -p mornlea_client --locked` 与
  `go test ./internal/client -run TestRendererApplyRenderWorldUpdates -count=1`。
- [x] 4.2 同步升级 C header、Rust export 和 Go bridge，并新增
  `mornlea_client_render_apply_world_updates`；Rust 复制或规范化输入且不保存 Go pointer，
  无参数 identity export 报告当时的 v12；所有接受 ABI version 参数的 client exports 对非
  v12 先返回 `ABI_VERSION`。新 input-only `u8` 入口不执行
  output-capacity/overlap 检查（这些仅适用于带输出 entry），并在 panic catcher 内将 panic
  映射为 `PANIC`、不产生部分状态；engine ABI 保持 v8；验证：
  `make rust && go test ./internal/client -race -count=1`。
- [x] 4.3 以 test-only driver 验证合法 cache update 前后的 frame encoding/readback 字节不变，
  frame/upload 计数不因 update 增加；不修改 `RenderFrame.Visible`、app、Go mesh/visibility、
  upload、draw 或任何 fluid-aware 源码；验证：
  `make rust && cd engine && cargo test -p mornlea_client --locked && go test ./internal/client -race -count=1`。
- [x] 4.4 取得该任务的单一独立 review，其报告同时包含 spec-compliance 与 quality verdict；
  分别审计无参数 identity export 与全部 versioned exports 的既有 ABI checks、新入口
  validation matrix、input-only 例外、panic
  隔离、原子失败、无 v11 fallback、engine ABI v8 与 cache-only 边界，并将 verdict 与
  裁决记入 `ledger.md`。

## 5. 同步版本事实、完成验收并记录证据

- [x] 5.1 将已实现的 client ABI v12 事实同步到受影响版本说明、架构说明和本 change
  artifacts；只记录已实现的 RenderWorld cache，明确 Go mesh/visibility/upload/draw 尚未
  迁移，不改变协议、schema、benchmark scenario、engine ABI、流体或 golden。
- [x] 5.2 对照已验证实现更新 delta spec、design、tasks 与 ledger；每一项只在已经执行、
  验证并经独立 review 后勾选。验证：
  `openspec validate --all --strict --no-interactive`。
- [x] 5.3 执行 `make rust`、`make rust-check`、指定六个 Go 文件的 `gofmt -w`、
  `go vet ./...`、`go test ./internal/client -race -count=1`、
  `go test ./internal/mesh -race -count=1`、`go test ./internal/archcheck -count=1`、
  `make test-race-changed`、`go test ./... -race`、`make visual-check`、
  `openspec validate --all --strict --no-interactive` 与 `git diff --check`；逐项将实际输出、
  失败或 Skip 写入 `ledger.md`。
- [x] 5.4 取得该任务的单一独立终审，其报告同时包含 spec-compliance 与 quality verdict；
  审查 MRW1 24/32 字节与 4 MiB/4096 上限、v12/v8、流体零触碰、无共享 kernel、无实时
  app 接线与完整验证证据，并将最终裁决记入 `ledger.md`。

## 6. 合入 main 并建立统一 client ABI v13

- [x] 6.1 在 clean worktree 以 `git merge --no-commit --no-ff main` 合入执行时最新 main，
  重新读取 merged `AGENTS.md` 与目标目录局部指南；以 main 的 client ABI v12
  WKWebView/UI surface 为冲突解决基线，保留 `ui_push_state` 与版本化 JSON UI events，
  保持 `render_upload_ui_font`、frame TLV tag 9 和 UI layout v1–v4 退役，只叠加 MRW1 cache
  与 update 入口，不改变 main 现有行为。验证：`git status --short`、
  `git diff --check`，并把 merge base、main HEAD、冲突路径与逐项裁决记入 `ledger.md`。
- [x] 6.2 由 fresh implementer 在 clean worktree 先执行 `make rust`，并在 merge 命令前即时
  记录 latest local `main`。若其超过已审计的 `a23833f9`，先核验新增 commits/paths；只有不
  改变版本、ABI、client/MRW1 surface、fluid 所有权或排除项时才直接继续，否则先更新 planning
  并完成独立 planning review。随后以 `git merge --no-commit --no-ff main` 做 non-rewriting
  follow-up sync，重新读取 merged root/nearest guides，只逐项解决真实 overlap。actual
  selected-main-parent 的 `engine/crates/mornlea_engine/`、`engine/include/mornlea_engine.h`、
  `internal/fluid/`、`internal/nativeabi/` 与 `internal/sim/realm/` MUST 在最终 merge tree 中
  byte-for-byte 保持一致；根 identity/current docs 只允许 client ABI v13 与 cache-only 所需
  feature-side 差异。保留 main 的 engine ABI v9、伙伴 fluid 和四项测试稳定性修复，不修改或
  接管其实现；记录 parent、冲突与裁决，并取得独立 spec/quality review。验证：`make rust`、
  `go test ./internal/archcheck -count=1`、
  `git diff --exit-code <selected-main-parent>..HEAD -- engine/crates/mornlea_engine engine/include/mornlea_engine.h internal/fluid internal/nativeabi internal/sim/realm`、
  `openspec validate --all --strict --no-interactive`、`git diff --check`。
- [x] 6.3 将 `engine/include/mornlea_client.h`、`engine/crates/mornlea_client/src/`、
  `internal/client/` 的 header/Rust exports/Go bridge、identity test 与 all-versioned-export
  tests 统一为 client ABI v13，并同步根版本说明、当前 docs、`openspec/config.yaml` 和受影响
  main specs 为 client ABI v13 / engine ABI v9；engine v9 与 fluid 只继承 selected main，
  不修改 Task 6.2 的 protected paths。保留 main UI JSON surface、MRW1 24/32 字节与
  4 MiB/4096、原子 cache 和无 fallback。测试必须分别锁定：无参数
  `mornlea_client_abi_version()` 始终报告 13；v13 动态库的每个 versioned export 收 ABI 12
  时返回 `ABI_VERSION`；main v12 动态库的共有 versioned exports 收 ABI 13 时返回
  `ABI_VERSION`；v13-only MRW1 symbol 在 main v12 动态库中缺失时于 link/load/bind 阶段
  硬失败，不进入 FFI、不返回 client status、不改变状态或 fallback。MRW1 input-only `u8`
  入口只按 ABI、bounded nonzero length、non-null pointer、address-range overflow、handle、
  MRW1 校验，不增加 alignment、output capacity 或 overlap 检查。验证：
  `make rust && go test ./internal/client -race -count=1`、
  `go test ./internal/archcheck -count=1`、
  `rg -n 'CLIENT_ABI_VERSION|MORNLEA_CLIENT_ABI_VERSION|ENGINE_ABI_VERSION|MORNLEA_ENGINE_ABI_VERSION|client ABI v13|engine ABI v9' engine internal/client AGENTS.md README.md README.en.md docs openspec/config.yaml openspec/specs`，并取得独立 spec/quality review。
- [x] 6.4 在 selected-main-parent 后的同一 merged implementation HEAD 逐项执行并记录
  `make rust`、`make rust-check`、
  `gofmt -w internal/client/render_world_update.go internal/client/render_world_update_test.go internal/client/render.go internal/client/render_test.go internal/client/window.go internal/client/window_test.go internal/server/sword_combat_parity_test.go`、
  `go vet ./...`、`go test ./internal/client -race -count=1`、
  `go test ./internal/mesh -race -count=1`、`go test ./internal/archcheck -count=1`、
  `make test-race-changed`、`go clean -testcache`、`go test ./... -race -count=1`、
  `make visual-check`、`openspec validate --all --strict --no-interactive`、
  `git diff --exit-code <selected-main-parent>..HEAD -- engine/crates/mornlea_engine engine/include/mornlea_engine.h internal/fluid internal/nativeabi internal/sim/realm`、
  `git diff --exit-code <selected-main-parent>...HEAD -- cmd/mornlea/capture/testdata/golden` 与
  `git diff --check`；任何 Skip、真实失败、protected path feature-side diff 或相对 selected
  main 新增/修改 visual golden 都不得计为 PASS，release dylib、Go ABI 与视觉证据必须来自
  同一 merged baseline，并取得独立 spec/quality review。
- [x] 6.5 先由独立 main-side implementer 在 latest local main 上清理原终审确认的 25 处代码
  注释任务编号；若增加防回归 `internal/archcheck`，必须先以当前违规树取得 RED，再完成
  GREEN。取得未参与实现者的独立 spec/quality review 后，local main 只可 fast-forward 到该
  reviewed cleanup，不得 push。随后由 fresh feature implementer 在 merge 前即时记录 latest
  main；若它超过 reviewed cleanup，先审计新增 commits/paths，契约或范围变化先更新 planning。
  契约不变时以 `git merge --no-commit --no-ff main` 做 non-rewriting sync，记录实际
  selected-main-parent、merge 双亲、冲突与裁决，并证明相对新父的
  `engine/crates/mornlea_engine/`、`engine/include/mornlea_engine.h`、`internal/fluid/`、
  `internal/nativeabi/`、`internal/sim/realm/` 及 `cmd/mornlea/capture/testdata/golden/` 零 diff。
  最小修复 `internal/client/render.go` 的 Rust production GPU / Go CPU / test-only RenderWorld
  职责注释，以及 `engine/crates/mornlea_client/src/ffi.rs` 的 identity export / 28 versioned
  exports 注释，再取得独立 spec/quality review；验证：`make rust`、
  `go test ./internal/client -race -count=1`、`go test ./internal/archcheck -count=1`、
  `openspec validate --all --strict --no-interactive`、上述 protected/golden diff 与
  `git diff --check`。
- [x] 6.6 由 fresh validation implementer 在 Task 6.5 review 通过后的 immutable final HEAD
  完整重跑并逐项记录 Task 6.4 的 18 门禁：`make rust`、`make rust-check`、指定七个 Go 文件的
  `gofmt -w` 及零 diff、`go vet ./...`、client race、mesh race、archcheck、
  `make test-race-changed`、`go clean -testcache`、`go test ./... -race -count=1`、
  `make visual-check`、OpenSpec strict、new selected-main protected zero-diff、golden zero-diff、
  client symbol/identity/reverse-mix/v13-only bind/no-fallback/retired-UI/no-production-MRW1/RPATH/
  dylib-SHA 与代码注释任务编号零匹配审计、`git diff --check`、exact HEAD/final clean status；
  每项记录真实 wall time、exit status、计数与 Skip。`10f8e8ab` 的 Task 6.4 PASS 不得计入本项。
  取得未参与执行者的独立 spec/quality review，0 open 后才能勾选。
- [x] 6.7 由 fresh planning implementer 只修订本 change 的 `proposal.md`、唯一 delta spec、
  `design.md`、`tasks.md` 与 `ledger.md`，并写 ignored
  `task-6.7-v14-replanning-report.md`。完整记录并保留 Tasks 1–6.6 历史、17/18 exact-main
  binding failure、`bcc053e8` 无关 planning sync 与 0/18 preflight cancellation；精确审计
  `bcc053e8..9bb84c68` 的五个 commits/全部 paths 及 main capture header/FFI/window/Rust/Go
  tests；其中 `be5ff22b` 引入 v13 capture，`4c553f3b` 只修改 `capture.rs`（38+/11-），把
  CoreGraphics option FFI width 校正为 SDK `u32`、IncludingWindow/BestResolution 位值校正为
  `1<<3`/`1<<3` 并新增防回归测试；`a83192b7` 只修改 `dev-capture` design/ledger/tasks
  （43+/4-），记录 Task 2 review rounds 与已实现 validation order，不改变 production/test code、
  v13 export/identity/header/Go surface 或 contract。`a83192b7..522c7d6a` 恰有 1 个 app pump
  commit，只修改 `cmd/mornlea/app/app_dependencies.go`、`cmd/mornlea/app/dev_capture.go`、
  `cmd/mornlea/app/dev_capture_test.go`、`cmd/mornlea/app/interactive.go`（356+/0-），新增并锁定
  `CaptureCoordinator`/`SetCaptureCoordinator`、菜单与游戏两处 poll 后 render 前
  `pumpDevCapture`、nil/idle 非阻塞、single outstanding、pixels ownership/error 原样交付与
  7 个 tests；`522c7d6a..9bb84c68` 恰有 1 个 docs-only commit，只修改 dev-capture
  ledger/tasks（21+/1-），记录 Task 3 双 PASS 并勾选 3.1，不改变 app/ABI/contract。锁定
  selected-main v13 为 28 versioned + 1 identity、capture 新 export 恰为
  `mornlea_client_window_capture`，以及 final v14 union 为 29 versioned + 1 identity、总计 30。
  planning 必须裁决 v14/v13 双向 ABI-first、v14-only MRW1 bind hard failure、capture/fluid
  rollback 保留、latest-main drift gate、non-rewriting merge、四个 app paths exact selected-main
  zero-diff/gofmt/7 focused tests、protected/golden zero-diff 与 Tasks 6.8–6.10；不得修改生产/测试
  代码、main specs、current docs、config 或 main，不得 merge/rebase/archive/push。commit
  `6d69db84` 的首轮 independent planning review 所报 1 个 app-pump Important、原 implementer
  无可消费回执后的 fresh handoff、修订证据与复审裁决必须 append-only 写入 ledger。随后由未参与
  本轮修改的 independent planning reviewer 同时给出 spec-compliance 与 quality verdict；reviewer
  identity、findings、修复轮次与裁决写入 ledger 后，只有两项 verdict 均通过且 0 open findings
  才可勾选。本次 planning-fix implementer 不得自行 review，本提交前保持未勾选。
- [x] 6.8 已完成并必须保留 non-rewriting merge
  `6f622407b1078d264707d8643f7fec41c553a48e`；其双亲精确为 feature
  `9dc22f1b9a8106f71a5f6496ac2bd708c31c5584` 与冻结 selected-main
  `9bb84c6841b59a18b030256d5952ed60acc215da`。本任务不得执行第二次 main merge，不得审计
  adoption 或要求 parity 于 `8646c313` 或任何更晚 main；later-main movement 对本 change
  non-binding。先在本 planning amendment 取得 independent planning review，再由暂停的 fresh
  implementer 继续自己的 combined client ABI v14 实现；不得改写历史、修改 main、丢失已继承
  v13 capture/app-pump、触碰伙伴 fluid 或弱化门禁。继续核验 client header、`ffi.rs`、`lib.rs`、
  `window.rs`、Go `window.go`/`window_test.go`、current identity docs，以及
  `cmd/mornlea/app/app_dependencies.go`、`cmd/mornlea/app/dev_capture.go`、
  `cmd/mornlea/app/dev_capture_test.go`、`cmd/mornlea/app/interactive.go`。四个 app paths 必须
  相对 exact `9bb84c68` zero-diff，并全部纳入 `gofmt -w` 后零差异证明；运行
  `go test ./cmd/mornlea/app -race -run 'Test(PumpDevCapture|RunInteractive(Game|Menu)LoopPumpsPendingCaptureOnce)' -count=1`
  证明既有 7 tests。最小实现统一 client
  ABI v14 / inherited engine ABI v9：完整保留 selected-main v13 的 28 个 versioned exports、
  capture status/两段式容量/top-down BGRA8/Go bridge/Rust-Go tests、SDK `u32` option FFI width
  与 IncludingWindow/BestResolution=`1<<3`/`1<<3`、ABI→output pointers/zero-capacity
  consistency→handle→capacity validation order、WKWebView/UI surface，以及 app pump 的
  coordinator 注入、菜单/游戏两处 poll 后 render 前调用、nil/idle 非阻塞、single outstanding、
  pixels ownership 与 error 原样交付，只叠加 v14-only MRW1，形成 29 versioned + 1 identity；
  MRW1 保持 cache-only、无 production caller，且不实现或接管其余 dev-capture service、HTTP、
  options、recording 或 docs。最终 v14 全部 29 个 versioned exports 对 ABI 13 ABI-first，exact selected-main v13
  全部 28 个 versioned exports 对 ABI 14 ABI-first，v13 缺 MRW1 symbol 时 bind hard-fail，
  不得有动态加载或 Go fallback；五组 protected engine/fluid paths 与 golden 相对 exact
  `9bb84c68` 零 diff。若冻结裁决遗漏 later-main 的无关或兼容工作，留待独立后续集成，不改变
  本任务判定。完成后分别取得 fresh spec-compliance review 与 fresh quality review，0 open
  findings 后才可勾选；已完成 merge 本身不足以改变当前 unchecked 状态。
- [ ] 6.9 由 fresh validation implementer 在 Task 6.8 reviews 通过后的 immutable final HEAD
  完整重跑 Task 6.6 的 18 门禁并逐项记录真实命令、wall time、exit status、计数与 Skip；
  OpenSpec strict 使用执行时实际全量计数，client contract 审计精确覆盖 final v14 29+1、
  exact `9bb84c68` v13 28+1 reverse mix、capture symbol/status/capacity/BGRA8/production bridge/tests、
  SDK `u32` option FFI width 与两个 `1<<3` option bits、
  四个 app paths relative to exact `9bb84c68` zero-diff 与 gofmt 零差异、上述 7 个 app-pump focused tests、
  coordinator 注入、菜单/游戏两处 pump、nil/idle 非阻塞、single outstanding、pixels ownership/
  error 原样交付，
  v14-only MRW1 bind hard failure/no fallback、MRW1 atomic/cache-only/no production caller、
  frame/readback 不变、RPATH/release dylib SHA、identity
  protocol32/player8/chunk9/world3/companions4/hostile1/engine9/client14/benchmark20、
  relative to exact `9bb84c68` protected/golden zero-diff、代码注释纪律与 `git diff --check`。
  18 门禁中的 moving-ref/exact-main binding gate 改为 frozen-parent provenance gate：
  `6f622407` 必须是 exact implementation HEAD 的祖先且双亲仍精确为 `9dc22f1b` / `9bb84c68`；
  该 merge 后不得通过额外 main merge 引入 `9bb84c68` 之后的 main commits；exact implementation
  HEAD 与 tracked/staged/untracked clean status 仍 mandatory，local `main` 指向其他提交不得
  单独导致失败。任何 tracked post-validation fix 或 sync 都必须从 Gate 1 完整重跑；取得未参与执行者的独立
  spec/quality review且 0 open findings 后才可勾选。
- [ ] 6.10 由未参与 Task 6.7 planning/review、Task 6.8 sync/implementation/reviews 或 Task 6.9
  validation/review 的 fresh whole-integration reviewer 同时给出 spec-compliance 与 quality
  verdict。显式核验 immutable history、`6f622407` 的 exact frozen-parent provenance 与双亲、
  无 later-main merge、
  v14/v13 export 数与双向 ABI-first、capture/UI 与 app frame-loop pump 保留、四个 app paths
  relative to exact `9bb84c68` zero-diff、MRW1 hard bind/cache-only/no-production-caller、身份矩阵、
  rollback 只回 exact `9bb84c68` v13 capture/app-pump、fluid 排除、protected/golden zero-diff 与 18 门禁
  同基线证据；先将 reviewer identity、findings、修复轮次与最终裁决写入 ledger，只有两项
  verdict 均通过且 0 open findings 才可勾选并宣告 implementation complete；不得仅因 local
  `main` 指向 `8646c313` 或任何更晚提交而失败，仍不得 archive、push 或 merge feature into main。
