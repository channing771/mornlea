# Tasks: Rust RenderWorld Cache

> Task 1 已建立本 change 与基线 ledger。下列每个未勾选实现任务必须按项目
> subagent-driven-development 流程由 fresh implementer 完成，并接受彼此独立的规格与
> 质量评审；实现者、评审结论、发现、修复轮次和裁决必须先记入 `ledger.md`，再勾选。
> 控制会话不得直接实现。

## 2. MRW1 与 RenderWorld 原子状态机

- [ ] 2.1 在 `engine/crates/mornlea_client/src/` 新增 `RenderWorld` 与 MRW1 v1 decoder 的
  RED 单元/性质测试：24/32 字节 header、4 MiB/4096 上限、reserved、tag、payload、
  signed i32 X/Z/dimension 与 Y 坐标裁决均须先失败；以
  `cd engine && cargo test -p mornlea_client --locked render_world` 验证。
- [ ] 2.2 实现预检后一次性提交的 decoder 与紧凑 cache：`ContainerSnapshot` 的
  single/indexed/direct、column height、reset/epoch、revision/tombstone 和 overflow；测试
  合法首 reset、非法第二 record 及旧 revision 均不产生部分状态，以同一 Cargo 定点测试
  验证。
- [ ] 2.3 为 MRW1 parser 增加 malformed length、palette slot、packed word、epoch、
  reset 首记录与坐标边界 fuzz/性质测试；以
  `cd engine && cargo test -p mornlea_client --locked` 验证。

## 3. client ABI v12 与 Go bridge

- [ ] 3.1 在 `engine/crates/mornlea_client/src/ffi.rs`、`src/lib.rs` 和
  `engine/include/mornlea_client.h` 将 client ABI 同步升级到 v12，新增
  `mornlea_client_render_apply_world_updates`；先以错误 ABI、无效 pointer/bytes 和未知
  handle 的 RED FFI 测试锁定 ABI_VERSION 优先于输入读取，再实现入口；以
  `cd engine && cargo test -p mornlea_client --locked ffi` 验证。
- [ ] 3.2 在 `internal/client/` 更新动态库身份检查和 Go binding，新增同步
  `ApplyRenderWorldUpdates` bridge；测试 header/Rust/Go 版本一致、v11 混装 fail fast 与
  Go 内存不被 Rust 保留；以 `make rust && go test ./internal/client -race -count=1` 验证。

## 4. test-only 输入与绘制不变量

- [ ] 4.1 在 `internal/client/` 为 test-only driver 编码既有 `world.ContainerSnapshot` 的
  single/indexed/direct、column、tombstone 和 reset MRW1 batch；测试 4/8/15-bit 与所有
  坐标边界，不改 `cmd/mornlea/app`；以
  `go test ./internal/client -race -count=1` 验证。
- [ ] 4.2 在 `engine/crates/mornlea_client/src/` 与 `internal/client/` 添加离屏断言：应用
  合法 cache batch 前后的既有 frame encoding 与 readback 逐字节一致，且不增加 frame 或
  section upload；以 `make rust && go test ./internal/client -race -count=1` 与
  `cd engine && cargo test -p mornlea_client --locked` 验证。
- [ ] 4.3 审计 diff，确认没有修改 `internal/client/mesher.go`、Go visibility、
  `RenderFrame.Visible`、draw/upload 生产路径或任何 fluid-aware engine 源码；以
  `git diff --check`、`go test ./internal/archcheck -count=1` 与定点 Rust/Go 测试验证。

## 5. 收尾门禁与独立终审

- [ ] 5.1 执行 `make rust`、`cd engine && cargo test -p mornlea_client --locked`、
  `gofmt -l .`、`go vet ./...`、`go test ./... -race -count=1`、
  `go test ./internal/archcheck -count=1`、
  `openspec validate --all --strict --no-interactive` 与 `git diff --check`；将每项实际输出
  和任何环境性失败记入 `ledger.md`。
- [ ] 5.2 由未参与实现的规格与质量评审者分别对 proposal、delta spec、design、tasks、
  MRW1 wire 契约、cache-only 范围、流体零触碰、engine ABI v8 和 frame/draw 字节不变
  进行终审；记录 verdict、findings、修复轮次与最终裁决，再提交或移交 change。
