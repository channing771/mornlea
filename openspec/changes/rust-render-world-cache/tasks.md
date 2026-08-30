# Tasks: Rust RenderWorld Cache

> Task 1 已建立 change、基线与 ledger。Tasks 2–5 是 feature 分支在独立 pre-main-integration
> 基线已经完成并评审的历史工作，其 v12 表述记录当时事实，不代表合并后的目标版本。
> Task 6 由新的 fresh implementer 执行；每项完成后，
> 一名未参与实现的独立 task reviewer 必须同时给出 spec-compliance 和 quality verdict。
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
- [ ] 6.2 将 `engine/include/mornlea_client.h`、`engine/crates/mornlea_client/src/`、
  `internal/client/` 的 header/Rust exports/Go bridge、identity test 与 all-versioned-export
  tests 统一为 client ABI v13，
  同步根版本说明、当前 docs、`openspec/config.yaml` 和受影响 main specs；保留 main UI JSON
  surface、MRW1 24/32 字节与 4 MiB/4096、原子 cache、无 fallback 和 engine ABI v8。测试必须
  分别锁定：无参数 `mornlea_client_abi_version()` 始终报告 13；v13 动态库的每个
  versioned export 收 ABI 12 时返回 `ABI_VERSION`；main v12
  动态库的共有 versioned exports 收 ABI 13 时返回 `ABI_VERSION`；v13-only MRW1 symbol 在
  main v12 动态库中缺失时于 link/load/bind 阶段硬失败，不进入 FFI、不返回 client status、
  不改变状态或 fallback。MRW1 input-only `u8` 入口只按 ABI、bounded nonzero length、non-null
  pointer、address-range overflow、handle、MRW1 校验，不增加 alignment、output capacity 或
  overlap 检查。验证：`make rust && go test ./internal/client -race -count=1`、
  `go test ./internal/archcheck -count=1`、`rg -n 'CLIENT_ABI_VERSION|MORNLEA_CLIENT_ABI_VERSION|client ABI v13' engine internal/client AGENTS.md README.md README.en.md docs openspec/config.yaml openspec/specs`。
- [ ] 6.3 在 merged implementation HEAD 逐项执行并记录 `make rust`、`make rust-check`、
  `gofmt -w internal/client/render_world_update.go internal/client/render_world_update_test.go internal/client/render.go internal/client/render_test.go internal/client/window.go internal/client/window_test.go internal/server/sword_combat_parity_test.go`、
  `go vet ./...`、
  `go test ./internal/client -race -count=1`、`go test ./internal/mesh -race -count=1`、
  `go test ./internal/archcheck -count=1`、`make test-race-changed`、`go clean -testcache`、
  `go test ./... -race -count=1`、`make visual-check`、
  `openspec validate --all --strict --no-interactive`、`git diff --check` 与
  `git diff --exit-code main...HEAD -- cmd/mornlea/capture/testdata/golden`；任何 Skip、真实失败
  或相对 main 新增/修改 visual golden 都不得计为 PASS，release dylib、Go ABI 与视觉证据
  必须来自同一 merged baseline。
- [ ] 6.4 取得一名未参与集成实现的独立 reviewer，对 merge conflict resolution、统一 v13
  export surface、main UI 保留/旧 UI 退役、MRW1/FFI/atomic/cache-only 边界、流体与其他排除项
  以及 6.3 的完整证据同时给出 spec-compliance 与 quality verdict；先把 reviewer identity、
  findings、修复轮次与最终裁决写入 `ledger.md`，再勾选本项并宣告 integration 完成。
