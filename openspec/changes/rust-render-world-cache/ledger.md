# Execution Ledger: rust-render-world-cache

## Task 1：change 设立、基线与验证

- 起始提交：`2344ca8551277317a31d4c1614c6086acd4ce328`。
- 起始工作区：`openspec/changes/rust-render-world-cache/` 是唯一未跟踪路径；其中
  `.openspec.yaml` 预置为 `schema: spec-driven`、`created: 2026-08-30`。没有已跟踪的
  生产代码或无关 OpenSpec 改动。
- 工具链：`rustc 1.97.1 (8bab26f4f 2026-07-14)`；
  `cargo 1.97.1 (c980f4866 2026-06-30)`；`go version go1.26.0 darwin/arm64`。
  `make rust` 明确使用 `rustup run 1.97.1 cargo build --locked --release`；基线的原样
  `cd engine && cargo test -p mornlea_client --locked` 成功，其输出不主张未捕获的默认
  Cargo toolchain 版本。

### 基线命令

| 命令 | 结果摘要 |
| --- | --- |
| `make rust` | 退出 0；`rustup run 1.97.1 cargo build --locked --release` 在 0.27s 完成，并更新两个 dylib 签名。 |
| `cd engine && cargo test -p mornlea_client --locked` | 退出 0；186 passed、0 failed、0 ignored，3.83s；doc-tests 0 passed、0 failed。 |
| `go test ./internal/client -race -count=1` | 退出 0；`ok github.com/channing771/mornlea/internal/client`，4.242s。 |
| `go test ./internal/archcheck -count=1` | 退出 0；`ok github.com/channing771/mornlea/internal/archcheck`，10.024s。 |

### 固定契约与范围

- MRW1 v1 的 24 字节 batch header、32 字节 record header、4 MiB/4096 上限、
  `ContainerSnapshot` 三态、world reset 首 record、epoch/revision/tombstone 和坐标裁决
  以 Task 1 binding brief 为唯一固定契约；不得以网络 packet 或未列出的兼容格式替代。
- `RenderWorld` 是 renderer 独占、可丢弃的派生缓存；Go Mirror 仍为逻辑真相来源。
  本 change 不允许它接入 app、mesh、visibility、upload 或 draw。
- client ABI 升至 v12 并拒绝 v11 混装；engine ABI 保持 v8。流体源码、流体语义与
  benchmark scenario v20 均不在范围内。
- Task 1 是文档/bootstrap 工作，没有代码 RED/GREEN 循环。控制器明确要求本任务不派发
  subagent；Tasks 2–5 每项使用 fresh implementer 和一名同时给出 spec-compliance、
  quality verdict 的独立 reviewer。

### Task 1 完成证据

- `openspec status --change rust-render-world-cache --json`：所有 proposal、specs、design 与
  tasks artifact 均为 `done`，`isComplete` 为 true。
- `openspec validate --all --strict --no-interactive`：退出 0，78 passed、0 failed。
- `git diff --check -- openspec/changes/rust-render-world-cache`：退出 0 且无输出。
- 自审确认 proposal、delta spec、design、tasks 与 ledger 均为 cache-only；没有生产
  Rust/Go、fluid-aware engine 源码、hook 或无关 OpenSpec change 改动。
- Task 1 提交：`4a93156d3d62dd89c6eacc635c848f9a6923e5fd`
  `docs(openspec): propose rust render world cache`。

## Task 1：后续写入收敛

- 提交后发现 `ledger.md` 与 `tasks.md` 有同范围未提交替换。收敛时保留了其与 approved
  scope 一致的 Task 2→3→4→5 顺序（纯 Rust cache、Go encoder、client ABI v12、版本事实
  与验收），删除了与项目 SDD 契约不一致的双 reviewer 流程、未证实 toolchain/时间数据、
  已通过检查的“pending”表述，以及超过 brief 的额外必需工作。
- `openspec status --change rust-render-world-cache --json`：全部 planning artifact 仍为
  `done`，`isComplete` 为 true。
- `openspec validate --all --strict --no-interactive`：退出 0，78 passed、0 failed。
- `git diff --check`：退出 0 且无输出。
- 自审：只修改本 change 的 `tasks.md` 与 `ledger.md`；任务严格按 Task 2→3→4→5 排列，
  每项仅要求一名独立 reviewer 同时给出 spec-compliance 与 quality verdict；没有引入
  未获 brief 支持的生产、archive 或额外强制工作。

## Tasks 2–5 执行记录

- 尚未开始。每个任务开始、RED/GREEN、review verdict、修复轮次、commit 和验证结果都
  必须按 `tasks.md` 追加在本节。

## Task 1 Fix Round 1

- Reviewer finding（spec ❌，quality needs fixes）：indexed section 的 payload 未固定 4-bit
  为 256 个 packed `u64`、8-bit 为 512 个，且未明确拒绝 payload 尾随 bytes。已在
  proposal、delta spec、design 与 Task 2 中补充 exact word count、exact consumption 及
  boundary/rejection/atomicity 测试。
- Reviewer finding（spec ❌，quality needs fixes）：ABI 安全契约未完整覆盖 all-export
  version-first 行为和新输入入口的 validation 次序。已明确保留全部既有 export 的 ABI
  matrix；新入口按 ABI、length、pointer、address range、handle、MRW1 顺序校验，只有
  output-bearing entries 适用 output-capacity/overlap，panic 映射为 `PANIC` 且不留下部分
  状态。Task 4 现要求 matrix 测试与审计。
- `openspec validate --all --strict --no-interactive`：退出 0，78 passed、0 failed。
- `git diff --check -- openspec/changes/rust-render-world-cache`：退出 0 且无输出。
- 自审：proposal、delta spec、design、tasks 与 ledger 一致要求 4-bit=256、8-bit=512、
  exact payload consumption；所有 client export 的 version-first 契约与既有 ABI matrix
  保留，新 input-only entry 的 ordered validation、output-only 例外和 panic isolation
  均已覆盖；cache-only 与 fluid-excluded 边界不变。

## Task 1 Independent Review Record

- Implementer：`01a05073-773c-7243-b2dc-acaa499d4230`。
- Independent reviewer：`01a0507c-5bcb-7c03-9bc3-f8dfee30d6a4`。
- Initial review range：`2344ca8..d6c39ca`。spec verdict：❌；quality verdict：Needs fixes。
  两项 Important finding 为 indexed payload 未固定 4-bit=256 / 8-bit=512 packed `u64`
  并拒绝 trailing bytes，以及 all-export version-first / 新 update entry validation
  matrix 未完整；详见本 ledger 的 Task 1 Fix Round 1。
- Fix round 1 commit：`7c913f44`
  `docs(openspec): tighten render world cache contracts`。
- Scoped re-review verdict：两项 finding 均为 ADDRESSED；没有新的 Critical/Important
  breakage，也没有 out-of-scope observations。
- Task 1 final commits：`4a93156d`、`d6c39caf`、`7c913f44`。
- Validation summary：基线 `make rust`、`cd engine && cargo test -p mornlea_client --locked`、
  `go test ./internal/client -race -count=1` 与
  `go test ./internal/archcheck -count=1` 均通过；所有 Task 1 strict validation 均为
  `openspec validate --all --strict --no-interactive` 78 passed、0 failed，target diff
  check 无输出。

## Task 2 Independent Review Record

- Implementer：`01a05084-0244-7ca0-acba-9ffe027a2c2a`。
- Independent reviewer：`01a0508d-02c1-7ad1-9d37-5dd6bbe06258`。
- BASE：`e3c2057c8f5def699a3f798271d0233a078e7dc5`。
- RED：先挂载 `render::world_tests` 且目标模块尚不存在，再运行
  `cd engine && cargo test -p mornlea_client --locked render::world_tests`；退出 101，
  `world_tests.rs:1` 报 `error[E0432]: unresolved import super::world`、
  `could not find world in super`，符合缺少 `RenderWorld`/MRW1 入口的预期失败。
- Implementation commit：`766c0de7 feat(client): add render world cache`。新增私有
  `RenderWorld`/MRW1 parser、renderer-owned cache 字段与内部更新方法，以及状态机测试；
  未连接 mesh、visibility、upload、frame、draw、app 或 fluid 路径。
- Implementation GREEN：focused `render::world_tests` 退出 0，26 passed、0 failed、
  186 filtered out；`cargo fmt --check` 退出 0 且无输出；crate regression
  `cargo test -p mornlea_client --locked` 退出 0，212 passed、0 failed，doc-tests
  0 passed、0 failed。
- Initial independent review：spec verdict ❌；quality verdict Needs fixes。三个 Important
  findings：
  1. `world_tests.rs` 的容量测试是 vacuous：超过 4 MiB 的输入同时 magic 非法，4097 count
     只有 header 而没有实际 records，删除对应 limit check 后测试仍可能通过。
  2. Task 2.3 要求 property 或 fuzz 覆盖，但初始测试没有对 truncation、declared length、
     packed word 与 palette slot 做性质式 mutation 并锁定 no panic/Invalid/atomic unchanged。
  3. 缺少 section 与 column 在同一 reset batch 内重复 key 的 ascending、descending、equal
     revision，以及 tombstone/upsert 两种顺序语义。
- Fix round 1 commit：`8b4feefe test(client): strengthen render world state coverage`。容量测试
  改为 510 条合法 direct records 组成的 otherwise-valid >4 MiB batch，以及 4097 条实际合法
  tombstones 组成且不超过 4 MiB 的 batch；新增 deterministic property-style mutation loops，
  以 `catch_unwind`、`Invalid` 和 snapshot 相等锁定失败语义；新增 section/column 同 batch
  重复 key 的 revision 与 tombstone/upsert 顺序测试。新测试未暴露生产实现缺陷。
- Fix round 1 GREEN：focused `render::world_tests` 退出 0，30 passed、0 failed、
  186 filtered out；`cargo fmt --check` 退出 0 且无输出；crate regression
  `cargo test -p mornlea_client --locked` 退出 0，216 passed、0 failed，doc-tests
  0 passed、0 failed。
- Scoped re-review：三个 Important findings 均为 ADDRESSED；没有新的 Critical/Important
  breakage，也没有 out-of-scope observations。
- Final scope proof：实现提交只修改
  `engine/crates/mornlea_client/src/render/{mod.rs,world.rs,world_tests.rs}`；fix round 1 只修改
  `world_tests.rs`。Task 2 没有修改 ABI/version/header/Go、app、`mornlea_engine`、fluid-aware
  source、依赖或 Tasks 3–5 实现；cache 仍未接管既有 mesh/visibility/upload/frame/draw 行为。

## Task 3 Independent Review Record

- BASE：`015c36e97d042cbd93a13920f0e1962e9742ebb2`。
- Implementer：`01a05099-a9dd-7852-a703-fb6b846b9c67`。
- Independent reviewer：`01a0509f-a0a3-7562-97fb-b78b37d6ef3b`。
- RED：先新增 encoder 测试，再运行
  `go test ./internal/client -run 'Test(BuildRenderWorld|EncodeRenderWorld)' -count=1`；退出 1，
  编译器报告 `undefined: client.BuildRenderWorldChunkBatch`、
  `client.RenderWorldSectionUpsert`、`client.RenderWorldColumnUpsert`、
  `client.EncodeRenderWorldBatch`、`client.RenderWorldBatch` 与
  `client.RenderWorldUpdate`，符合目标 API 尚不存在的预期。
- Implementation commit：`db655a7a feat(client): encode render world updates`。新增纯 Go MRW1
  值对象、全值预检与 checked sizing encoder、完整 chunk section/height-map batch builder，未接入
  app、network、cgo 或 ABI。
- GREEN：`gofmt -w internal/client/render_world_update.go internal/client/render_world_update_test.go`
  退出 0；focused
  `go test ./internal/client -run 'Test(BuildRenderWorld|EncodeRenderWorld)' -count=1` 退出 0，
  `ok github.com/channing771/mornlea/internal/client`；snapshot regression
  `go test ./internal/world -run 'Test.*Snapshot' -count=1` 退出 0，
  `ok github.com/channing771/mornlea/internal/world`；package race
  `go test ./internal/client -race -count=1` 退出 0，
  `ok github.com/channing771/mornlea/internal/client`；dependency check
  `go test ./internal/archcheck -count=1` 退出 0，
  `ok github.com/channing771/mornlea/internal/archcheck`。
- Initial independent review：spec verdict ❌；quality verdict Needs fixes。两个 Important finding：
  1. 容量覆盖只有 4097-record 拒绝，先触发 record-count gate，未用 otherwise-valid、
     <=4096-record 输入证明 4 MiB gate 本身；删除 size gate 后该测试仍可通过。
  2. 缺少不依赖 encoder/parser helper 的完整字节 oracle，24/32-byte header、reserved、payload
     length、signed little-endian 坐标/revision 与 section payload 的同源解析断言可能共同掩盖
     wire layout 回归。
- Fix round 1 commit：`89b58ab4 test(client): strengthen render world encoder coverage`。新增 510 条
  合法 direct section update（总长 4,198,344 bytes、少于 4096 records）的 4 MiB 拒绝测试，及
  reset + single section 的 96-byte literal MRW1 oracle。`gofmt -w
  internal/client/render_world_update_test.go` 退出 0；focused encoder 测试退出 0，
  `ok github.com/channing771/mornlea/internal/client 0.791s`；
  `go test ./internal/client -race -count=1` 退出 0，
  `ok github.com/channing771/mornlea/internal/client 2.821s`；`git diff --check` 无输出。
- Scoped re-review：两个 Important finding 均为 ADDRESSED；没有新的 Critical/Important breakage，
  也没有 out-of-scope observations。
- Final scope proof：Task 3 implementation 仅新增
  `internal/client/render_world_update.go` 与 `internal/client/render_world_update_test.go`；fix round 1
  仅修改测试文件。没有修改 proposal/spec/design、生产 app/network/world、Rust、ABI/header/version
  或 Tasks 4–5 的实现。
