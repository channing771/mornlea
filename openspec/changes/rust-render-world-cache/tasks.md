# Tasks: Rust RenderWorld Cache

> Task 1 已建立 change、基线与 ledger。Tasks 2–5 各由 fresh implementer 完成；每项完成后，
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

- [ ] 3.1 在 `internal/client/` 先为 `world.ContainerSnapshot` 的 single、indexed、direct、
  column、tombstone、reset、4/8/15-bit 及坐标边界建立 MRW1 encoder RED 测试；验证：
  `go test ./internal/client -run 'Test(BuildRenderWorld|EncodeRenderWorld)' -count=1`。
- [ ] 3.2 实现有界、checked 的 Go MRW1 batch encoder 与完整 chunk update 构造；它不导入
  network、不发送隐式 reset、不展开 4096 block，且尚不接入实时 app；验证：
  `go test ./internal/client -run 'Test(BuildRenderWorld|EncodeRenderWorld)' -count=1` 与
  `go test ./internal/world -run 'Test.*Snapshot' -count=1`。
- [ ] 3.3 取得该任务的单一独立 review，其报告同时包含 spec-compliance 与 quality verdict；
  以 Go 定点/race 测试和 `go test ./internal/archcheck -count=1` 复核，将 verdict 与裁决
  记入 `ledger.md`。

## 4. 升级 client ABI 到 v12 并接通 cache-only 输入入口

- [ ] 4.1 在 `engine/include/mornlea_client.h`、`engine/crates/mornlea_client/src/` 与
  `internal/client/` 保留全部既有 client ABI exports 的 ABI-version matrix，并为新输入入口
  建立 RED FFI/bridge matrix：ABI 优先，随后 non-zero/bounded length、non-null pointer、
  address range/no overflow、existing handle、MRW1 layout/capacity，再到合法 batch；验证：
  `cd engine && cargo test -p mornlea_client --locked` 与
  `go test ./internal/client -run TestRendererApplyRenderWorldUpdates -count=1`。
- [ ] 4.2 同步升级 C header、Rust export 和 Go bridge，并新增
  `mornlea_client_render_apply_world_updates`；Rust 复制或规范化输入且不保存 Go pointer，
  所有 client ABI 入口对非 v12 先返回 `ABI_VERSION`。新 input-only `u8` 入口不执行
  output-capacity/overlap 检查（这些仅适用于带输出 entry），并在 panic catcher 内将 panic
  映射为 `PANIC`、不产生部分状态；engine ABI 保持 v8；验证：
  `make rust && go test ./internal/client -race -count=1`。
- [ ] 4.3 以 test-only driver 验证合法 cache update 前后的 frame encoding/readback 字节不变，
  frame/upload 计数不因 update 增加；不修改 `RenderFrame.Visible`、app、Go mesh/visibility、
  upload、draw 或任何 fluid-aware 源码；验证：
  `make rust && cd engine && cargo test -p mornlea_client --locked && go test ./internal/client -race -count=1`。
- [ ] 4.4 取得该任务的单一独立 review，其报告同时包含 spec-compliance 与 quality verdict；
  审计全部 export 的既有 ABI checks 与新入口 validation matrix、input-only 例外、panic
  隔离、原子失败、无 v11 fallback、engine ABI v8 与 cache-only 边界，并将 verdict 与
  裁决记入 `ledger.md`。

## 5. 同步版本事实、完成验收并记录证据

- [ ] 5.1 将已实现的 client ABI v12 事实同步到受影响版本说明、架构说明和本 change
  artifacts；只记录已实现的 RenderWorld cache，明确 Go mesh/visibility/upload/draw 尚未
  迁移，不改变协议、schema、benchmark scenario、engine ABI、流体或 golden。
- [ ] 5.2 对照已验证实现更新 delta spec、design、tasks 与 ledger；每一项只在已经执行、
  验证并经独立 review 后勾选。验证：
  `openspec validate --all --strict --no-interactive`。
- [ ] 5.3 执行 `make rust`、`gofmt -l .`、`go vet ./...`、
  `go test ./internal/client -race -count=1`、`go test ./internal/archcheck -count=1`、
  `go test ./... -race`、`openspec validate --all --strict --no-interactive` 与
  `git diff --check`；逐项将实际输出、失败或 Skip 写入 `ledger.md`。
- [ ] 5.4 取得该任务的单一独立终审，其报告同时包含 spec-compliance 与 quality verdict；
  审查 MRW1 24/32 字节与 4 MiB/4096 上限、v12/v8、流体零触碰、无共享 kernel、无实时
  app 接线与完整验证证据，并将最终裁决记入 `ledger.md`。
