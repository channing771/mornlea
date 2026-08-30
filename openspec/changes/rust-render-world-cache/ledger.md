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
- Reviewer finding（spec ❌，quality needs fixes）：ABI 安全契约未完整覆盖 all-versioned-export
  version-first 行为和新输入入口的 validation 次序。已明确保留全部接受 ABI version 参数的
  既有 exports ABI matrix；新入口按 ABI、length、pointer、address range、handle、MRW1
  顺序校验，只有
  output-bearing entries 适用 output-capacity/overlap，panic 映射为 `PANIC` 且不留下部分
  状态。Task 4 现要求 matrix 测试与审计。
- `openspec validate --all --strict --no-interactive`：退出 0，78 passed、0 failed。
- `git diff --check -- openspec/changes/rust-render-world-cache`：退出 0 且无输出。
- 自审：proposal、delta spec、design、tasks 与 ledger 一致要求 4-bit=256、8-bit=512、
  exact payload consumption；无参数 identity export 的当前版本身份与所有接受 ABI version
  参数的 client exports version-first 契约及既有 ABI matrix 保留，新 input-only entry 的
  ordered validation、output-only 例外和 panic isolation
  均已覆盖；cache-only 与 fluid-excluded 边界不变。

## Task 1 Independent Review Record

- Implementer：`01a05073-773c-7243-b2dc-acaa499d4230`。
- Independent reviewer：`01a0507c-5bcb-7c03-9bc3-f8dfee30d6a4`。
- Initial review range：`2344ca8..d6c39ca`。spec verdict：❌；quality verdict：Needs fixes。
  两项 Important finding 为 indexed payload 未固定 4-bit=256 / 8-bit=512 packed `u64`
  并拒绝 trailing bytes，以及 all-versioned-export version-first / 新 update entry validation
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

## Task 4 Independent Review Record

- BASE：`301e375dfdb4a301c347daaebe5e0b93d397d1b2`。
- Implementer：`01a050a7-4f57-7253-94c6-df2c7fdaea80`。
- Independent reviewer：`01a050af-30ef-7181-aa6a-aba204e0f62b`。
- RED：先新增 Rust FFI 与 Go offscreen/bridge 测试。运行
  `cd engine && cargo test -p mornlea_client --locked
  apply_world_updates_rejects_wrong_abi_and_invalid_bytes` 退出 101，Rust 报
  `error[E0425]: cannot find function mornlea_client_render_apply_world_updates in this scope`；
  运行 `go test ./internal/client -run TestRendererApplyRenderWorldUpdates -count=1` 退出 1，
  编译器报告 `Renderer.ApplyRenderWorldUpdates undefined`，符合新 C export 与 Go method
  尚不存在的预期失败。
- Implementation commit：`06dd4439 feat(client): add render world update ABI`。client C header、
  Rust 常数/export 与 Go bridge 同步升级为 v12；新 input-only 入口按 ABI、length、pointer、
  address range、handle、MRW1 顺序校验并在 catcher 内调用 renderer-owned cache，不保存 Go
  pointer，也不接入 app、mesh/visibility、upload、frame 或 draw。
- GREEN：focused Rust update test 退出 0，1 passed、0 failed、218 filtered out；`make rust`
  退出 0并同步 release dylib；focused Go update tests 退出 0，2 passed。完整
  `cd engine && cargo test -p mornlea_client --locked` 退出 0，219 passed、0 failed，doc-tests
  0 failed；`go test ./internal/client -run
  'Test(RendererApplyRenderWorldUpdates|RendererRoundtripOrSkip|EncodeRenderFrameLayout)'
  -count=1 -v` 退出 0，4 tests passed；`go test ./internal/client -race -count=1` 退出 0。
- Format、symbol 与 scope：`cargo fmt --manifest-path engine/Cargo.toml --all --check &&
  test -z "$(gofmt -l .)" && git diff --check` 退出 0且无输出；release dylib 的 `nm -gU`
  结果包含 `_mornlea_client_render_apply_world_updates`；C header 与 Rust client 常数均为 v12，
  engine ABI 保持 v8。相对 BASE 的实现 diff 仅含 brief 授权的 8 个 header/client crate/Go
  bridge 与 current-version assertion 文件，`openspec`、app、`mornlea_engine` 与 fluid-aware
  源码均无实现 diff。
- GPU：无 Skip；本机 adapter 可用，Rust 真实 renderer、Go identity frame render/readback
  与既有 roundtrip 均实际执行并通过。相同 `RenderFrame` 的 encode/readback 字节在 apply
  前后相等，apply 不增加 frame/upload 计数。
- Independent review verdict：spec-compliance ✅；quality Approved；无 findings。reviewer 按
  指令未重跑 suites，因此报告中的 cannot-rerun ⚠️ 不是实现缺陷或未决 finding；controller
  裁决为 Task 4 detailed report 已提供 RED、GREEN、full Rust、Go focused/race、format、symbol、
  scope 与 GPU 实跑证据，满足该验证关注并接受 review verdict。
- Final scope proof：没有 v11 compatibility export 或 Go fallback；全部 versioned client
  exports 保留 ABI-first matrix；未知 handle 路径不解引用 input；panic 映射 `PANIC`；
  transactional MRW1 失败不留下部分状态。Task 5、proposal、delta spec、design 与根版本文档
  留待后续同步，本记录未提前修改。

## Task 5：版本事实、主规格同步与阶段验收

- 执行基线与全部阶段命令的 execution HEAD：
  `e4843642f169eb513f0ff91c911540c97a6bce67`。Task 5 是 documentation/verification，
  没有新增代码 RED，也没有虚构 RED；指定 gofmt 未产生生产 Go diff。
- `openspec status --change rust-render-world-cache --json` 恰在首次主规格写入前运行一次并
  退出 0；`planningHome.root` 为当前 worktree，唯一选中的
  `artifactPaths.specs.existingOutputPaths` 是
  `openspec/changes/rust-render-world-cache/specs/rust-client-render-cutover/spec.md`，未从目录猜测
  delta。
- `openspec instructions specs --change rust-render-world-cache --json` 在首次主规格写入前恰好
  运行一次并退出 0，返回有效 `artifactId: specs` JSON。合并遵守其三条 rules：中文可观察
  行为与 SHALL/MUST、每条 Requirement 的可判定 Given/When/Then、只在 spec 保留行为契约。
- 主规格 merge 保留原 Purpose 与原 4 条 Requirement，把选中 delta 的 4 条 ADDED
  Requirement 合入唯一 `## Requirements`；主规格现有 8 条 Requirement、无 delta operation
  header。delta 与新增主规格块逐字节内容仅边界空行不同，再次执行相同语义 merge 不产生
  diff；change 保持 active，未 archive。

### 当前事实与历史事实审计

- 根 AGENTS、双语 README、architecture、LAN 版本矩阵与两个受影响 main spec 的当前 client
  ABI 已由 v11 同步为 v12；`docs/notes/progress.md` 另增本 change 的 cache-only 编年记录。
- 进度日志中 `client-ui-vanilla-alignment` 当时“client ABI v11 不变”、
  `fix-gpu-benchmark-batch` 的 v10→v11，以及 `tiered-swords-combat` 合入时的 v11 基线是历史
  引入事实，保持原文 v11；delta 中“v11 动态库不能与 v12 bridge 混用”也是兼容性场景，
  不是陈旧当前值。
- `docs/architecture.md` 明确 RenderWorld 只是 renderer 独占、由 MRW1 原子更新的紧凑派生
  cache，Go Mirror 仍为逻辑真相；入口仅由测试驱动。Go 仍持有生产 mesh 调度、
  connectivity/visibility、逐 section upload 与 draw 输入，后续 change 才迁移。
- `proposal.md` 已把“完成时同步”收敛为 v12/current-spec 已同步且 change 等待独立终审；
  `design.md` 的 Verification 已与 binding brief 的完整逐项命令一致，并重申 cache-only
  当前边界。delta spec 已与代码一致，无需为了制造 diff 重写。`openspec/config.yaml` 的
  client ABI v11 版本矩阵是既有跨功能陈旧 context，按 binding brief 明确排除且保持字节不变。

### 阶段边界验证

下表命令均单独执行，HEAD 均为
`e4843642f169eb513f0ff91c911540c97a6bce67`；没有 Skip，也没有更新 golden：

| 命令 | 结果与关键计数/时长 |
| --- | --- |
| `make rust` | PASS，退出 0；release build 0.29s，命令墙钟约 0.41s，两个 dylib 重签名。 |
| `make rust-check` | **FAIL**，`make` 退出 2（clippy 101），约 21.0s；Rust 1.97.1 `-D warnings` 在 Task 2 的 `world.rs` 报 2 个 `large_enum_variant`：`ColumnState::Live([i16; 256])` 与 `ParsedRecord::ColumnUpsert.heights`。fmt 已通过，失败发生在 clippy，workspace tests 未开始。Task 5 禁止修改生产 Rust，也不得加 allow 或弱化 gate，因此未越权修复。 |
| `gofmt -w internal/client/render_world_update.go internal/client/render_world_update_test.go internal/client/render.go internal/client/render_test.go internal/client/window.go internal/client/window_test.go` | PASS，退出 0，<0.1s，无输出且六个文件相对 execution HEAD 零 diff。 |
| `go vet ./...` | PASS，退出 0，约 3.83s，无输出。 |
| `go test ./internal/client -race -count=1` | PASS，退出 0；package 5.219s。 |
| `go test ./internal/mesh -race -count=1` | PASS，退出 0；package 20.463s。 |
| `go test ./internal/archcheck -count=1` | PASS，退出 0；package 4.921s，动态版本门禁接受 client v12/engine v8，无需修改 `baseline_test.go`。 |
| `make test-race-changed` | PASS，退出 0；origin/main 识别 1 个改动包 `internal/client`，反向依赖闭包 10 包，9 个测试包通过、`cmd/gfxspike` 无测试；最慢 `internal/server` 221.482s，`internal/sim` 46.645s。 |
| `go test ./... -race` | **FAIL**，退出 1；42 个 package 输出中 38 PASS、1 FAIL、3 `[no test files]`，失败仅为既有 `internal/server/TestSwordCombatParity`（server package 249.275s）：velocity 3.65 vs TCP 1.15，差 2.5。 |
| `make visual-check` | PASS，退出 0，约 83s；本机 macOS/arm64 Metal adapter 实际离屏运行 25 个场景，每场 230400 pixels，全部最大通道差 0、差异像素 0（0.0000%）；无 GPU Skip、无 golden 更新。 |
| `openspec validate --all --strict --no-interactive` | PASS，退出 0；78 passed、0 failed，约 1.27s。 |
| `git diff --check` | PASS，退出 0，<0.1s，无输出。 |

附加诊断 `go test ./internal/server -race -run '^TestSwordCombatParity$' -count=1` 再次退出 1
（package 5.733s），且差异方向反转为 Memory 1.15 vs TCP 3.65，仍恰差 2.5。该测试由
`90188fbc feat(tiered-swords): unify swords and authoritative melee combat (#124)` 引入，自 Task 5
base 起零 diff；其 ready/spawn warmup 循环按 transport 收包时机决定推进多少个权威 tick，
随后却直接比较 hostile velocity，因此调度会改变采样前状态。该失败不是 cache ABI 路径，
本任务也无权修改战斗测试或协议实现；原始全量 FAIL 保留，不以 focused 结果替代。

### 版本、容量与范围证明

- C header `MORNLEA_CLIENT_ABI_VERSION` 与 Rust `CLIENT_ABI_VERSION` 均为 12；Go window/render
  bridge 直接编译并传递同一 header macro。engine header 仍为 ABI 8。
- Rust `world.rs` 与 Go encoder 分别固定 MRW1 4 MiB/4096；delta/main spec 同步固定 24-byte
  batch header、32-byte record header、4 MiB/4096 上限、非法 batch 原子失败、v11 混装早期
  拒绝，以及合法 update 前后 frame 编码不变。
- 协议、schema 与 benchmark 的代码权威值仍为 protocol 32、player 8、chunk 9、world
  metadata 3、companions 4、hostile 1、scenario 20；`git diff 2344ca85 --` 对
  `internal/network/protocol`、`internal/storage`、`cmd/mornlea/benchmark`、
  `cmd/mornlea/app`、`internal/fluid`、`engine/crates/mornlea_engine`、golden 目录和
  `openspec/config.yaml` 均为空。
- Task 5 相对 execution HEAD 的 diff 仅是 brief 授权的版本/架构/进度文档、change 的
  `proposal.md`/`design.md`/`tasks.md`/`ledger.md` 与两个 main specs；没有生产 Rust/Go、
  `internal/archcheck`、hook、
  protocol/schema/benchmark/fluid/kernel/app/golden 改动。全 change 也未创建共享 kernel、
  worker/GPU pool，未把 cache 连接到实时 app。

### 自审与待评审项

- 已逐项把 README/architecture/spec/ledger 的 v12/v8、cache-only、MRW1、frame/golden 与
  排除项主张回对代码常数、whole-change path diff 和实际命令输出；主规格结构与幂等性检查
  PASS，历史 v11 引入事实未被现代化。
- 阶段正确性未全绿：`make rust-check` 的两个 change 内 clippy finding 与全量 race 的既有
  sword parity test failure 均如实保留。前者需要后续获准修改 Task 2 生产 Rust，后者需要
  `tiered-swords-combat` 所有者修正测试前置状态；本任务没有绕过或削弱任何 gate。
- 5.1–5.3 已完成事实同步、spec sync、命令执行与证据记录；5.4 **保持未勾选**，明确等待
  controller 派发的独立 task review。本 change 保持 active。

## Task 5 Fix Round 1/5：阶段 gate 修复后的验收证据

### 原始 findings 与 reviewed fixes

- Task 5 implementation commit `a4f6dbf5 docs: record render world cache migration` 在
  execution HEAD `e4843642` 记录了两个真实失败：`make rust-check` 因 Task 2
  `world.rs` 的两个 `large_enum_variant` clippy finding 失败；`go test ./... -race` 因既有
  `TestSwordCombatParity` 使用 transport-dependent warmup tick 失败。Task 5 independent
  reviewer 对 `e484364..a4f6dbf` 的 verdict 为 spec ❌、quality Needs fixes，两项均为
  Important。
- Task 2 fix round 2 commit `8b0243c8 fix(client): reduce render world enum size` 以 boxed
  fixed arrays 消除两个 clippy finding，没有使用 `allow` 或改变 MRW1/cache 行为；reviewer
  `01a050d0-a776-7ce3-ad06-5a30d4557f04` 判定 spec ✅、quality Needs fixes，唯一 Important
  finding 是 staged `self.clone()` 会深拷贝并重新分配每个 live column payload。
- Task 2 fix round 3 commit `d170e198 perf(client): share render world column payloads` 改用不可变
  `Arc<[i16; 256]>` 并增加 shallow-staging pointer identity/release 回归测试。相同 reviewer 的
  scoped re-review 判定 spec ✅、quality Approved；前述 allocation finding 已解决，0 open。
  该修复链保持 24/32-byte MRW1、4 MiB/4096、原子 staged commit、epoch/revision/tombstone、
  v12/v8 与 cache-only 边界不变。
- parity gate commit `2e833ff6 test(server): stabilize sword combat parity` 只修改
  `internal/server/sword_combat_parity_test.go`，让 readiness 与固定 warmup 的每个
  `StepForTest` 都等待同 tick `PlayerState` barrier；没有 retry、没有放宽断言，也没有生产
  combat/network 改动。reviewer `01a050d9-ef20-7ce3-98c0-eaac06ea9eac` 对
  `d170e19..2e833ff` 判定 spec ✅、quality Approved，无 findings。

### 当前 HEAD 重验

以下命令均在 clean execution HEAD
`2e833ff603f88923229dcbe7ed9724937ba6611c` 单独运行；无 Skip：

| 命令 | 结果与关键计数/时长 |
| --- | --- |
| `make rust-check` | PASS，退出 0；workspace fmt 与 all-target clippy `-D warnings` 通过；`mornlea_client` 220 passed、0 failed（1.27s），`mornlea_engine` 175 passed、0 failed（0.50s），两 crate doc-tests 均 0 failed。 |
| `go test ./... -race` | PASS，退出 0；42 个 package 输出中 39 PASS、3 `[no test files]`，无失败；当前命令多数 package 命中同源码 Go test cache，`internal/archcheck` 实跑 34.338s。 |
| `go test ./internal/archcheck -count=1` | PASS，退出 0；fresh package 5.830s，client ABI v12 / engine ABI v8 与长期版本事实一致。 |
| `openspec validate --all --strict --no-interactive` | PASS，退出 0；78 passed、0 failed，约 1.27s。 |
| `git diff --check` | PASS，退出 0，<0.1s，无输出；验证前 worktree clean。 |

### Fix-round 裁决与范围

- 原 Task 5 的两个 Important findings 现均有 reviewed fix 与当前 HEAD PASS gate 证据；原失败
  记录保留，不被覆盖或改写成历史 PASS。
- 本 fix round implementer 只更新 validation/evidence 文档；不修改 production Rust/Go、测试、
  proposal/design、tasks、delta/main specs、app、fluid、protocol/schema/benchmark、golden、hook
  或 `openspec/config.yaml`。
- OpenSpec 5.1–5.3 保持已勾选；5.4 **仍保持未勾选**。本记录不预先宣称 Task 5 scoped
  re-review verdict，change 保持 active，等待 controller 派发本 round 独立复审。

## Task 5 Scoped Re-review 与完成裁决

- Independent reviewer：`01a050c9-d4c4-7102-a03c-4ded50301019`。初审范围为
  `e484364..a4f6dbf`，初始 verdict 为 spec ❌、quality Needs fixes；两个 Important finding
  分别是 Task 2 `large_enum_variant` 阻塞 `make rust-check`，以及 transport-dependent
  `TestSwordCombatParity` 阻塞 `go test ./... -race`。
- 修复与证据范围为 `a4f6dbf..0a1f05c`。其中 `8b0243c8` 消除 clippy finding，独立 reviewer
  `01a050d0-a776-7ce3-ad06-5a30d4557f04` 判定 spec ✅、quality Needs fixes，并指出 staged
  column deep-copy allocation；`d170e198` 以 immutable `Arc` 消除该 allocation，同一 reviewer
  scoped re-review 判定 spec ✅、quality Approved、0 open；`2e833ff6` 以 test-only tick barrier
  稳定 parity gate，独立 reviewer `01a050d9-ef20-7ce3-98c0-eaac06ea9eac` 判定 spec ✅、
  quality Approved、无 findings；`0a1f05c5` 记录修复后的完整 current-HEAD gate 证据。
- Task 5 scoped re-review 同时复核初始范围与上述修复/证据范围，最终 verdict 为 spec ✅、
  quality Approved；两个原 Important findings 均 addressed，0 open。reviewer 明确确认当前
  evidence 适用，并确认终审前 change 保持 active、进度为 14/15。
- Controller 接受该 clean verdict；Task 5.4 现已完成，OpenSpec tasks 为 15/15。change 继续
  保持 active，不在本任务 archive；实现与验收已完成，等待 archive。
- Final bookkeeping validation 在 pre-commit HEAD `0a1f05c5` 单独执行且均退出 0：
  `openspec validate --all --strict --no-interactive` 为 78 passed、0 failed（real 2.53s）；
  `openspec instructions apply --change rust-render-world-cache --json` 为 15/15、remaining 0、
  `state: all_done`（real 2.96s）；`git diff --check` 无输出（real 0.02s）。

## Whole-branch Final Fix Wave：validation/docs

### Final review findings 与修复范围

- Whole-branch final reviewer：`01a050ea-070a-7932-b0aa-bef57a3353bf`；review range
  `2344ca8..f8d4f87`，package `review-final-2344ca8..f8d4f87.diff`（18 commits）。初始
  verdict 为 overall ❌、spec ❌、quality Needs fixes；本节不把它改写成 clean。
- 两个 Important findings：transactional staging 仍深拷贝 boxed indexed/direct section
  payload；最终 Rust representation change 后缺少同一最终实现基线的 release dylib rebuild、
  Go ABI 与 visual 完整闭环。一个 Minor finding：active `openspec/config.yaml` context 仍写
  client ABI v11。
- Rust fix commit `f8d51683 perf(client): share render world section payloads` 把 indexed
  palette/packed 与 direct packed 改为不可变 `Arc`，并以 pointer identity、release 与
  copy-on-replacement isolation regression 锁定 shallow staging。Task 2 fix round 4 已记录
  focused 32/32、client crate 221/221、strict clippy 与 `make rust-check` PASS。
- 本 validation/docs wave 只把 active `openspec/config.yaml` 当前矩阵的一处 client ABI
  v11 改为 v12，并更新本 ledger、proposal 与 ignored Task 5 report；不修改 implementation、
  main specs 或 `tasks.md`（保持 15/15），也不 archive change。

### 最终实现基线完整验证

以下命令均在最终源码 HEAD
`f8d51683a67b81bd8b0bc15f5ba4f4532b264f2d` 上逐条执行；运行时只有上述 config 一行文档
修正。除明确标注的 changed-race failure 外均退出 0；无环境 Skip：

| 命令 | 实际结果、计数与时长 |
| --- | --- |
| `make rust` | PASS；release profile 实际重新编译 `mornlea_client`，Cargo 4.26s、real 4.57s；重新签名 `libmornlea_engine.dylib` 与 `libmornlea_client.dylib`。 |
| `make rust-check` | PASS；workspace fmt 与 all-target clippy `-D warnings` clean；`mornlea_client` 221 passed、0 failed、0 ignored（1.32s），`mornlea_engine` 175 passed、0 failed、0 ignored（0.59s），两 crate doc-tests 0 failed；real 3.28s。 |
| `gofmt -w internal/client/render_world_update.go internal/client/render_world_update_test.go internal/client/render.go internal/client/render_test.go internal/client/window.go internal/client/window_test.go` | PASS；无输出、六文件零 diff；real 0.03s。 |
| `go vet ./...` | PASS；无输出；real 1.99s。 |
| `go test ./internal/client -race -count=1` | PASS；package 9.158s、real 10.33s。 |
| `go test ./internal/mesh -race -count=1` | PASS；package 36.193s、real 36.74s。 |
| `go test ./internal/archcheck -count=1` | PASS；fresh package 9.901s、real 10.53s；动态门禁接受 client ABI v12 / engine ABI v8。 |
| `make test-race-changed` | **FAIL**；exit 2、real 256.94s。2 个改动包形成 11 包闭包：9 个 tested packages PASS、`cmd/gfxspike` no-tests；唯一失败 `internal/server/TestSwordCombatParity`，Memory velocity 3.65 / TCP 1.15、difference 2.5；server 243.726s，sim 61.749s。 |
| `go clean -testcache` | PASS；real 0.01s；在精确 full race 前清除 Go test cache。 |
| `go test ./... -race` | PASS；清缓存后 42 package lines：39 PASS、3 no-tests；server 229.719s、sim 62.041s、client 4.581s、mesh 35.528s、archcheck 57.765s；real 252.63s。 |
| `make visual-check` | PASS；先再次 release build/sign（Cargo 0.29s），随后真实 GPU capture 25 场景，每场 230400 pixels，全部最大通道差 0、差异像素 0（0.0000%）；无 GPU Skip、无 golden 更新；real 82.27s。 |
| `openspec validate --all --strict --no-interactive` | PASS；78 passed、0 failed；real 2.68s。 |
| `git diff --check` | PASS；无输出；real 0.02s。 |

### ABI、GPU、scope 与待复审裁决

- `libmornlea_client.dylib` 的最终 SHA-256 为
  `519ff7d27c69f6f79f0f3af6748e373800b0b07a011b9845c6b3f97dd254e898`。额外编译的 race
  client test binary 依赖 `@rpath/libmornlea_client.dylib`，其 LC_RPATH 精确指向本工作树
  `engine/target/release`；因此 Go ABI tests 消费的是本轮重建的 release dylib。额外 client
  JSON run 记录 332 pass actions、0 skip、0 fail。
- C header `MORNLEA_CLIENT_ABI_VERSION` 与 Rust `CLIENT_ABI_VERSION` 均为 12，Go bridge
  使用 header macro；engine header ABI 保持 8。active OpenSpec context 现与这些代码事实一致。
- `make visual-check` 实际执行 capture，25 个 tracked golden 均零 diff。gofmt 后 Go 文件零 diff；
  `f8d4f87..f8d51683` 只修改 Rust `render/world.rs` 与 `world_tests.rs`。本 docs wave 不修改
  main specs、tasks、app、protocol/schema/benchmark/fluid/kernel、hook 或 golden。
- `make test-race-changed` 的唯一 parity failure 与随后清缓存 full race PASS 发生在同一源码
  基线，说明该 gate 仍有调度型不稳定迹象；本 docs-only wave 不修改或绕过测试，并保留该
  concern 供 scoped final reviewer 裁决。
- Final review 目前**不是 clean**。change 保持 active、OpenSpec tasks 保持 15/15，等待
  reviewer `01a050ea-070a-7932-b0aa-bef57a3353bf` 对 `f8d4f87..` fix wave 做 scoped final
  re-review；本轮不 archive。

### Parity gate fix round 2 与 final HEAD 重验

- 上节 `make test-race-changed` 的真实 FAIL 保持原样，不以之后的 PASS 覆盖。诊断证明 round 1
  只对齐 packet tail，hostile 在 readiness 期间仍受 bounded A* worker 完成时机影响，导致 sword
  input 前的权威位置、速度与 path state 可按 scheduler 分叉。
- Test-only fix commit `1d458865 test(server): align sword combat authority state` 在 login ready
  后安装确定性 hostile fixture，以 `RunAtInputBoundary` 固定 input consumption boundary，并按
  input-relative tick 比较 transcript；没有 retry、Skip、timeout inflation 或生产 server/network/
  combat 变更。原 parity reviewer `01a050d9-ef20-7ce3-98c0-eaac06ea9eac` 判定 spec ✅、
  quality Approved，确认 A* timing gap addressed；唯一 Minor stale comment 已由 comment-only
  `93879482 test(server): clarify sword combat parity scope` 修正并 closed。
- `f8d51683..93879482` 只修改 `internal/server/sword_combat_parity_test.go`。因此上一节 final
  Rust baseline 上已经 PASS 的 `make rust`、`make rust-check`、client/mesh/archcheck race、
  release dylib ABI proof、25-scene visual capture、OpenSpec strict 与 diff evidence 继续适用；
  此后没有 Rust、ABI、GPU、golden、app、fluid、protocol/schema/benchmark 或 main-spec 变更。
- 以下门禁在 final HEAD
  `93879482990f434b2cb9d21d46daedf566764b7b` 重新逐条执行，均退出 0：

| 命令 | 实际结果、计数与时长 |
| --- | --- |
| `make test-race-changed` | PASS；2 个改动包形成 11 包闭包，10 个 tested packages PASS、`cmd/gfxspike` no-tests；server 227.786s、sim 67.137s、app 68.625s、archcheck 61.768s、client 7.785s；real 240.30s。脚本内 release build 0.24s 并重新签名两个 dylib。 |
| `go clean -testcache` | PASS；real 0.01s。 |
| `go test ./... -race` | PASS；清缓存后 42 package lines：39 PASS、3 no-tests；server 223.195s、sim 50.077s、app 49.549s、archcheck 47.427s、mesh 26.793s、client 2.711s；real 243.64s。 |
| `openspec validate --all --strict --no-interactive` | PASS；78 passed、0 failed；real 1.46s。 |
| `git diff --check` | PASS；无输出；real 0.02s。 |

- 两条 Go 门禁现均在 final HEAD PASS；此前 parity FAIL、根因、reviewed fix chain 与最终 PASS
  均保留为连续证据。当前没有未解决 gate failure。
- Whole-branch final review 仍**不得写成 clean**：change active、tasks 15/15、未 archive，等待
  reviewer `01a050ea-070a-7932-b0aa-bef57a3353bf` 对包含 `f8d51683`、`1d458865`、
  `93879482` 与本 validation/docs evidence 的 fix wave 做 scoped final re-review。

## Whole-branch Scoped Final Re-review 完成裁决

- Whole-branch reviewer：`01a050ea-070a-7932-b0aa-bef57a3353bf`；scoped final re-review
  range：`f8d4f871..ea2d8c97`。
- Final verdict：Overall ready ✅、spec ✅、quality Approved。初始 whole-branch review 的
  2 个 Important findings（section payload staging deep-copy；最终实现基线缺少 release dylib、
  Go ABI 与 visual 闭环）和 1 个 Minor finding（active OpenSpec context 仍写 client ABI v11）
  均已 addressed；0 open。
- Reviewer 接受 `f8d51683` 的 immutable section payload sharing、active config 的 client ABI
  v12、`1d458865` / `93879482` 的 reviewed parity round 2，以及 `ea2d8c97` 记录的 final Rust
  baseline 与 final-HEAD changed-race/full-race/OpenSpec/diff evidence。此前真实 parity FAIL、
  根因与修复链继续作为历史证据保留。
- 分支实现与整分支终审现已完成，可交付；OpenSpec tasks 保持 15/15，change 保持 active，
  等待 archive。本 verdict 不构成 archive 或 merge 授权，本 bookkeeping 也不执行二者。
- Final bookkeeping validation 在 pre-commit HEAD `ea2d8c97` 执行且均退出 0：
  `openspec validate --all --strict --no-interactive` 为 78 passed、0 failed（real 1.49s）；
  `openspec instructions apply --change rust-render-world-cache --json` 为 15/15、remaining 0、
  `state: all_done`（real 1.48s）；`git diff --check` 无输出（real 0.02s）。

## Main integration planning：client ABI v13

### 已观察分叉与重叠面

- Planning start feature HEAD：`0f91b626623af9c8de1c4e60a2e810ff7edd52a7`；observed main
  HEAD：`8b8891a3d1068cedb02b74266ed2b1334fdcea69`；共同 merge base：
  `2344ca8551277317a31d4c1614c6086acd4ce328`。main 自 merge base 前进 74 commits，feature
  前进 23 commits。本 planning task 未 merge、rebase 或改写任一分支。
- `git diff --name-only` 两侧交集恰为 15 个路径：
  `AGENTS.md`、`README.en.md`、`README.md`、`docs/architecture.md`、
  `docs/notes/progress.md`、`engine/crates/mornlea_client/src/ffi.rs`、
  `engine/crates/mornlea_client/src/lib.rs`、`engine/crates/mornlea_client/src/render/mod.rs`、
  `engine/include/mornlea_client.h`、`internal/client/render.go`、
  `internal/client/render_test.go`、`internal/client/window.go`、
  `internal/client/window_test.go`、`internal/server/sword_combat_parity_test.go`、
  `openspec/specs/tiered-swords-combat/spec.md`。
- current main 的 C header 与 Rust `CLIENT_ABI_VERSION` 均为 12，Go bridge 使用 header macro。
  v12 的实际 UI surface 已退役 `mornlea_client_render_upload_ui_font` 与 frame TLV tag 9
  （UI layout v1–v4），保留 `mornlea_client_ui_push_state`，并让
  `mornlea_client_render_drain_ui_events` 传输 `{"v":1,"events":[...]}` 版本化 JSON
  信封。feature 则独立把同一个 v12 分配给 MRW1 update export；两者是不同 export surface，
  不能以相同 ABI identity 合并。

### 用户确认与规划裁决

- 用户明确确认：以 main v12 WKWebView/UI surface 为 predecessor，把合并后的统一 client ABI
  分配为 v13；冲突解决必须保留 main 已退役的 UI exports/TLV 状态以及当前
  `ui_push_state`/versioned JSON UI events，只叠加 MRW1 cache/update。若错误地继续共用 v12，
  header、dylib 与 Go bridge 会接受两个不兼容 surface，因此该替代方案被否决。
- engine ABI 保持 v8；MRW1 24/32 字节、4 MiB/4096、epoch/revision/tombstone、非法 batch
  原子失败、cache-only 与 no-fallback 契约不变。协议 v32、全部 schema/world metadata、
  benchmark scenario v20、流体、生产 app 接线、共享 kernel、mesh worker/GPU pool、Rust
  visibility、frame/draw 与 visual golden 继续排除。
- Tasks 2–5 保持已勾选，作为 pre-main-integration feature 基线的历史完成事实；新增 Task 6
  重新打开 change，依次要求 non-rewriting main merge、v13 current-fact/spec/test 同步、同一
  merged baseline 的 release/race/visual/OpenSpec 验证，以及独立 integration review。
- Planning implementer：`01a05146-946a-7e92-8a83-ff8d0a219468`。
- Independent planning reviewer：`01a0514d-8eda-7091-9cfb-6ae2fa4881b7`。
- 当前只完成 integration planning artifact 修订；没有运行或声称任何 merged-baseline
  implementation/ABI/race/visual 验证。archive 与 merge into main 仍未获授权，change 保持
  active；只有 Task 6 实现、验证和独立评审全部完成后才能重新申请 archive/merge 裁决。

### Planning independent review：Fix round 1/5

- Reviewed planning commit：`bd698ae213cef7a8a8cf2ece865b85829bf512a6`。Independent
  reviewer `01a0514d-8eda-7091-9cfb-6ae2fa4881b7` 的 initial verdict 为 spec ❌、
  quality Needs fixes；无 Critical，
  两项 Important。
- Important 1：原 delta/design/Task 6.2 把 main v12 动态库与 v13 bridge 的全部反向混装都
  描述成可进入 export 并返回 `ABI_VERSION`，但 v13-only
  `mornlea_client_render_apply_world_updates` 在 main v12 动态库中根本不存在，无法返回
  client status。Fix round 1 明确拆成三条：集成 v13 动态库的全部 versioned exports 收 ABI 12
  均先返回 `ABI_VERSION`；main v12 动态库的共有 versioned exports 收 ABI 13 均先返回
  `ABI_VERSION`；v13-only MRW1 symbol 对 main v12 动态库只能在 link/load/bind 阶段硬失败，
  不进入 FFI、不返回 status、不改变状态且没有兼容入口或 fallback。
- Important 2：原 design D1 声称 input 解引用前还做 capacity、alignment 和 overlap 检查，
  与 input-only `u8` 入口不符。Fix round 1 把唯一顺序统一为 ABI version、bounded nonzero
  length、non-null `u8` pointer、address-range overflow、existing handle、MRW1，并明确没有
  额外 alignment、output capacity 或 overlap 检查。
- 本 fix 只修改 existing delta spec、`design.md`、`tasks.md` 的 6.2 与本 ledger；proposal
  已保持正确的通用“早期硬拒绝”表述，无需修改。没有代码、main specs、merge、archive、
  current docs/config 或任何 Task 6 implementation/validation 改动；Tasks 仍为 15/19。

### Planning independent review：Fix round 2/5

- Reviewer `01a0514d-8eda-7091-9cfb-6ae2fa4881b7` 确认 fix round 1 的两项 Important
  findings 均 addressed；scoped re-review 新增 1 项 Important 与 1 项 Minor。
- Important：delta 的“调用任一 client ABI 入口”错误包含无参数 identity export。Fix round 2
  把 version-first/status 契约限定为所有接受 ABI version 参数的 client exports，并把
  `mornlea_client_abi_version()` 单独固定为始终报告 13；proposal、design、Tasks 4/6 与本
  ledger 的同类 all-export 简写同步收窄为 identity + all-versioned-export 两部分。
- Minor：ignored planning report 仍保留 reviewer placeholder 和“等待初次评审”的过期状态；
  本轮使用真实 implementer/reviewer identity，并把报告状态推进到 fix round 2 scoped
  re-review pending。
- 本轮只修改既有 proposal、delta spec、design、tasks、ledger 与 ignored report；没有代码、
  main specs、merge、archive 或 Task 6 implementation 改动，任务进度保持 15/19。

### Planning independent review：最终 clean verdict

- Independent reviewer：`01a0514d-8eda-7091-9cfb-6ae2fa4881b7`。
- Initial review range：`0f91b626623af9c8de1c4e60a2e810ff7edd52a7..bd698ae213cef7a8a8cf2ece865b85829bf512a6`；
  planning commit `bd698ae2 docs(openspec): plan client ABI v13 integration`。Verdict 为
  spec ❌、quality Needs fixes；2 Important，0 Critical。
- Fix round 1 scoped range：`bd698ae213cef7a8a8cf2ece865b85829bf512a6..bf514af54cdfc28b6635ff033c392d441bf8bed2`；
  fix commit `bf514af5 docs(openspec): correct ABI v13 integration contract`。Reviewer 确认
  initial 2 Important addressed，并新增 identity-export 1 Important 与 stale-report 1 Minor。
- Fix round 2 scoped range：`bf514af54cdfc28b6635ff033c392d441bf8bed2..be7c3a1c546c8cce44b46b01a6dd313160e92f0c`；
  fix commit `be7c3a1c docs(openspec): clarify ABI v13 identity contract`。Final verdict：
  spec ✅、quality Approved；全部 findings addressed，0 new/open。
- Controller 接受该 clean planning verdict。client ABI v13 integration planning 已完成并可交给
  Task 6 apply 流程；OpenSpec tasks 仍为 15/19，6.1–6.4 保持未勾选。该 planning verdict
  不代表 main 已合并、集成实现/验证已完成，也不构成 archive 或 merge into main 授权。
- Final planning bookkeeping validation 在 pre-commit HEAD `be7c3a1c` 执行：
  `openspec validate --all --strict --no-interactive` 为 78 passed、0 failed；
  `openspec instructions apply --change rust-render-world-cache --json` 为 15/19、remaining 4、
  `state: ready`；`git diff --check` 无输出，tracked diff 仅本 ledger。

## Task 6.1：main merge、review fix 与完成裁决

### 实现与固定双亲

- Fresh implementer：`01a0515c-d1e0-7373-a6e5-fc6e5f2abd86`。
- Implementer 在 clean feature HEAD
  `a59d998f08f1ec25321a3c998eb2bfd5ae9b5a58` 先执行 `make rust` 并通过，再以
  `git merge --no-commit --no-ff main` 合入当时已核验的 local main
  `8b8891a3d1068cedb02b74266ed2b1334fdcea69`；共同 merge base 为
  `2344ca8551277317a31d4c1614c6086acd4ce328`。
- Merge commit：`3c8c5da18e67dc0768e545247cb993d4957053ad`
  `chore: merge main into rust render world cache`；双亲依次为 feature `a59d998f...` 与
  actual `MERGE_HEAD` `8b8891a3...`。冲突逐项以 main v12 WKWebView/JSON UI surface 为基线，
  只叠加 MRW1 cache/update 形成 client ABI v13；保留 hook 删除与其他 main 行为，没有接入
  production MRW1，也没有 feature-side fluid/kernel 改动。
- 实际实现与验证证据以 ignored
  `.superpowers/sdd/2026-08-30-rust-render-world-main-integration/merge-report.md` 为准。该报告记录
  10 个冲突路径及逐项裁决、ABI/symbol/retired-surface/cache-only 审计，并记录 pre/post/final
  `make rust`、`cargo fmt --all -- --check`、124 项 Rust client tests、
  `go test ./internal/client -race -count=1`、`go test ./internal/archcheck -count=1`、
  `go test ./internal/server -run '^TestSwordCombatParity$' -race -count=20`、OpenSpec strict
  78/78、release dylib symbol、production-scope、fixed-parent 与 diff checks 全部最终 PASS。

### 独立 review 与 fix round 1/5

- Independent reviewer：`01a0516d-4a5b-75f3-928a-dfc1ebb6b37c`。
- Initial verdict：spec ✅；quality Needs fixes。唯一 finding 为 1 个 Minor：merge 相对 second
  parent 对 4 个无关 main 文件做了 EOF normalization，使它们产生纯末尾空行 diff。
- Fix round 1 commit：`db4625b0fbcf0540e7dcbe24efdca14047de88c7`
  `fix(merge): preserve main file endings`。该提交只把 3 个 SDD process report 与
  `openspec/specs/chunk-persistence/spec.md` 逐字恢复为 second parent 内容；四路径相对
  `8b8891a3...` 零 diff，Task/ledger、fluid/kernel 均未改动。
- Scoped re-review verdict：spec ✅；quality Approved。原 Minor finding addressed，
  0 new、0 open。Controller 接受该 clean task verdict；Task 6.1 现完成并勾选，OpenSpec
  进度推进为 16/19。Task 6.2–6.4 仍未开始，本节不改写其任务或 engine ABI 契约。

### merge 期间 main 前进的后续裁决边界

- 固定双亲 merge 进行期间，local `main` 从 actual `MERGE_HEAD` `8b8891a3...` 前进 22 commits
  到 `a23833f92a80abb808b2b629c4dc043d2043f90a`，其中包含伙伴负责的 fluid 与 engine ABI v9
  工作。已启动 merge 的第二父提交保持 `8b8891a3...`，因此该 ref 漂移不使 Task 6.1 失败，
  也不允许改写既有 merge commit。
- 本 change 最终交付前必须先取得明确 OpenSpec 决策，再由 fresh implementer 做
  non-rewriting follow-up sync，并接受独立规划评审与实现评审；在 controller 获得用户确认前，
  不得自行新增或改写 Task 6.2–6.4，也不得改写当前 engine ABI 契约。
- 用户排除流体的含义是：后续 sync 原样继承所选 main 的 fluid/engine 结果，并以相对该 main
  的路径与语义审计证明 feature-side 零改动；本 change 不接管、重写或扩展流体实现。

### Bookkeeping validation

- 在 pre-commit HEAD `db4625b0fbcf0540e7dcbe24efdca14047de88c7` 与本次纯 planning
  working diff 上执行 `openspec validate --all --strict --no-interactive`：退出 0，
  78 passed、0 failed。
- `openspec instructions apply --change rust-render-world-cache --json`：退出 0；schema
  `spec-driven`，16/19 complete、3 remaining、`state: ready`；仅 Task 6.1 新增完成状态，
  6.2–6.4 内容与未完成状态不变。
- `git diff --check`：退出 0且无输出。`git diff --name-only` 的 tracked 范围恰为本 change 的
  `tasks.md` 与 `ledger.md`；ignored `progress.md` / `merge-report.md` 仅同步 review 事实，没有
  production、fluid/kernel、engine ABI、main spec 或其他 tracked 改动。

## Follow-up integration planning：继承 main engine ABI v9

### main 漂移只读审计与用户裁决

- Planning implementer：`01a05184-9302-7b31-90f9-feeb5509b005`。起始 feature HEAD 为
  `ef6c75919a85aa23d4e4d47c002ca08a1df11a8c`，worktree clean；local `main` 与
  `origin/main` 均为 `a23833f92a80abb808b2b629c4dc043d2043f90a`。
- Task 6.1 的 actual second parent 固定为
  `8b8891a3d1068cedb02b74266ed2b1334fdcea69`；`git merge-base 8b8891a3 a23833f9`
  返回同一个 `8b8891a3...`。`8b8891a3..a23833f9` 恰为 22 commits、42 paths、
  5722 insertions / 287 deletions。
- 该范围交付伙伴负责的 Rust engine fluid eval/rescan、Go `internal/fluid` 与
  `internal/nativeabi` bridge、`internal/sim/realm` 集成以及 engine ABI v9，并包含四项 main
  测试稳定性修复：glyph atlas exhaustion、dialogue shutdown cancellation、dropped-item
  restart client 与 fluid parity readiness。范围没有修改 `mornlea_client`、`internal/client`
  或 `cmd/mornlea`，没有改变 main client ABI v12 WKWebView/UI surface，也没有加入 MRW1。
- `git show main:engine/include/mornlea_engine.h` 证明 main engine ABI 为 9；
  `git show main:engine/include/mornlea_client.h` 证明 main client ABI 仍为 12。受保护实现范围
  固定为 `engine/crates/mornlea_engine/`、`engine/include/mornlea_engine.h`、
  `internal/fluid/`、`internal/nativeabi/` 与 `internal/sim/realm/`。
- 用户明确要求持续到完成，并确认最终目标为 client ABI v13、继承 main engine ABI v9。
  流体继续由伙伴负责：本 change 只通过 non-rewriting main sync 原样继承，不设计、修改、
  接管或扩展 engine/fluid 生产路径。若 follow-up merge 开始前 local main 再前进，implementer
  必须即时重审新增 commits/paths；契约或范围变化先回到 planning，否则选定并记录最新父。

### planning artifact 修订

- `proposal.md`、唯一 `rust-client-render-cutover` delta 与 `design.md` 的 present-tense/final
  target 已改为 client ABI v13 / inherited engine ABI v9；Task 6.1 固定父 engine ABI v8 仍作为
  历史事实保留，没有改写 Tasks 1–5 或既有验证证据。
- `design.md` 新增 follow-up sync 决策、main 再漂移闸门、selected-main-parent、五组 protected
  path byte-for-byte audit 与回退边界。rollback 只移除 MRW1/client-v13 增量并回到所选 main
  的 client v12 WKWebView predecessor，必须保留 engine ABI v9、伙伴 fluid 与 main 稳定性修复；
  不得恢复旧 egui/TLV UI。
- `tasks.md` 保留已完成 6.1；新增 6.2 non-rewriting follow-up main sync，原 pending 6.2–6.4
  顺延为 6.3–6.5。planning 后预期状态为 16/20 complete、4 remaining、`ready`。Tasks 6.2–6.4
  各要求 fresh implementer 与独立 spec/quality review，6.5 要求未参与 follow-up 的 fresh
  whole-integration reviewer。
- 本轮严格限定为 existing change artifacts 与本 ledger；没有修改生产/测试代码、current
  docs、main specs、`openspec/config.yaml`，没有 merge/rebase main，也没有 archive。当前分支
  root identity/config 中尚未同步的 engine ABI v9 属于 Task 6.3，而不是本 planning task 的
  越权修复。
- Follow-up planning independent review：pending。本节只记录 planning 实施事实，不把尚未
  发生的独立评审写成通过。

### planning validation

- `openspec validate --all --strict --no-interactive`：退出 0；78 passed、0 failed。
- `openspec instructions apply --change rust-render-world-cache --json`：退出 0；20 tasks、
  16 complete、4 remaining、`state: ready`，Task 6.1 保持完成，6.2–6.5 均未勾选。
- `git diff --check`：退出 0且无输出。`git diff --name-only` 的 tracked scope 恰为 existing
  `proposal.md`、唯一 delta spec、`design.md`、`tasks.md` 与 `ledger.md`；无代码、current
  docs/main specs/config、merge 或其他 tracked diff。
