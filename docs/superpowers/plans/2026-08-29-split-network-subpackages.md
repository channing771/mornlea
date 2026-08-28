# internal/network 子包拆分执行计划（2026-08-29）

## 现状与动机

PR #109 已拆出 `tcp/`（6 文件）后，根包仍平铺 51 个 Go 文件（23 生产 ~5.3k 行 + 30 测试 ~7.5k 行），164 个测试入口（151 Test + 7 Benchmark + 6 Fuzz）同包编译；tcp/ 另有 33 个 Test。三类异质关注点共处一个命名空间：

- **协议消息层（~2.6k 行）**：`packet.go`（ProtocolVersion v31、State、密封 ClientPacket/ServerPacket、握手/登录包、ValidateClientPacket/ValidateServerPacket）+ `message.go` 与 8 个 `message_*.go`（域 DTO + Validate）+ `registry.go`（冻结包 ID 表，零导入）+ `snapshot.go`（区块 wire DTO）
- **编解码层（~1.7k 行）**：`chunk_codec.go`（公开 Codec 门面 + zstd + ChunkSnapshot 专路）、`codec.go`、`codec_client.go`/`codec_server.go`（双向分发）、`codec_values.go`、`codec_primitives.go`（byteEncoder/byteDecoder）、`frame.go`（帧封装）
- **传输与会话（~0.9k 行）**：`stream.go`（共享 stream 接口）、`transport.go`（ErrClosed、ClientEndpoint/ServerEndpoint 与适配器）、`memory.go`（Memory transport）、`login.go`（共享登录状态机）

痛点与 storage 拆分同构：迭代一个域为其他域的测试付费；archcheck 只见整包边，包内耦合不可见。消费面极大：9 个包引用根包（internal/server 127 符号 2033 处、cmd/mornlea/app 616、internal/client 411、capture 120、benchmark 39、cmd/mornlea 36、mornlea-server 34、sim 测试 5）+ tcp 400+ 处引用 → 必须走别名再导出保消费面零改动。

## 耦合事实（边界依据）

1. **密封接口钉死协议同包**：ClientPacket/ServerPacket 以 unexported marker（`clientPacket()`/`serverPacket()`）密封，全部 message DTO 与 snapshot.go 实现之；主校验器扇出到各 `Validate`。→ packet+message+registry+snapshot 必须同一包（protocol）。
2. **registry ↔ message 双向**：registry 映射消息类型与 RejectReason；`message_player.go:195` 的 `CommandRejected.Validate` 回调 `commandRejectReasonID`。同包即无环。
3. **companion/hostile 二进制编解码错位**：`encodeChatEvent` 等 12 个函数住在 message 文件里，被 codec_server.go:179-191/466-492 调度并借用其 unexported wire 常量（codec_server.go:209-221）。→ wire 函数归位 codec 包（其余所有消息的 wire 代码都在 codec 侧，规范一致）；Validate 所需 wire 常量留在 protocol 导出。
4. **memory.go:83 跨层泄漏**：`validateDecodedClientWirePacket`（codec_client.go:306）实为协议级校验——放行 StateHandshake 的 ClientHello 与 StateLogin 的 LoginStart 以保留冻结的版本/身份拒绝路径，其余转发 `ValidateClientPacket`，不碰字节层。→ 归位 protocol 包导出。
5. **frame 依赖原语**：帧封装复用 codec_primitives 的 canonical uvarint（frame.go:14,49,80-96）。→ frame.go 随 codec 包，编码原语全部保持 unexported，零额外导出面。
6. **tcp 生产代码只消费根包 6 类符号**：Codec/NewCodec、State、ClientPacket/ServerPacket、三 stream 接口、WriteFrame/ReadFrame、ErrClosed → 别名后 tcp 源码零改动、tcp→network archcheck 边不动。
7. 根包 benchmark_test.go 有 tcp 测试回边（BenchmarkTCPLoopback*）；archcheck 只扫非测试文件，不受影响。

## 目标布局

```
internal/network/
├── types.go                    # package doc + 别名再导出（消费面零改动）
├── stream.go / transport.go / memory.go / login.go   # 根包保留会话与传输编排
├── login_test.go / memory_test.go / benchmark_test.go / benchmark_helpers_test.go
├── protocol/   # packet.go, message*.go(9), registry.go, snapshot.go + 协议级测试
├── codec/      # chunk_codec.go, codec.go, codec_client/server/values/primitives.go, frame.go + 编解码测试 + testdata/
└── tcp/        # 完全不动
```

依赖方向（archcheck allowed 表登记）：

- network（根）→ {protocol, codec, internal/core}（login.go 的 NormalizeDisplayName 保留 core 边；companion 边随 message_companion.go 移交 protocol）
- protocol → {internal/core, internal/companion}
- codec → {protocol, internal/core}
- tcp → network（既有边不动）

## 关键设计决策

1. **别名再导出保消费面零改动**：
   - 类型：`type PlayerState = protocol.PlayerState`、`type Codec = codec.Codec` 等全部消息/包/snapshot DTO 与密封接口；
   - 常量：ProtocolVersion、State*、RejectReason/LoginRejectCode/DisconnectCode/ChatEventKind 等枚举值、MaxCompressedSnapshot/MaxDecodedSnapshot/MaxFrameBytes/MaxItemDropBatch/MaxSmallPayload；
   - 函数（var 别名）：NewCodec、ValidateClientPacket、ValidateServerPacket、WriteFrame、ReadFrame；
   - **留守根包原生（无别名风险）**：会话 API——Identity、RemoteError、PendingLogin、BeginServerLogin/Accept/Reject、LoginClient/LoginClientWithSeed、NewMemoryPair/NewMemoryStreamPair、ClientEndpoint/ServerEndpoint、三 stream 接口、ErrClosed。消费面最高频符号（ClientEndpoint 147、Identity 103、NewMemoryPair 96）全部原生。
   - 类型别名保方法集（`msg.Validate` 继续可用），错误/常量别名保值身份。
2. **临时导出与回收（记 ledger）**：T2 建 protocol 时需临时导出包 ID 访问器（clientPacketID 等 6 个）、ValidateDecodedClientWirePacket、companion/hostile 的 wire 常量与 encode/decode 函数（供仍留守根包的 codec 文件过渡调用）；T3 codec 落位后 encode/decode 函数移入 codec 恢复 unexported；包 ID 访问器、ValidateDecodedClientWirePacket、wire 常量保持导出（codec 永久消费，属协议冻结契约面）。
3. **测试归属（跟随被测主体，歧义记 ledger）**：protocol 收 packet/registry/message Validate/snapshot 测试；codec 收 chunk_codec/codec_golden/invalid/inventory/fuzz/primitives/frame/drop/furnace/container/hunger/worldtime/place_block_succeeded 等 Codec 门面往返与全部 6 个 Fuzz；根包留 login_test/memory_test/benchmark_test/benchmark_helpers_test（跨域基准留守，CI M3C 步骤与 Makefile bench-multiplayer 零改动）与 seed_test（主体为 LoginClientWithSeed 会话路径）。混合文件（如 message_companion_test 同时覆盖 Validate 与 wire 往返）按主体拆分，测试函数名与子测试标签逐名保留。
4. **测试入口并集冻结**：拆分前 `go test ./internal/network/... -list '.*'` 全集 = 根 164（151+7+6）+ tcp 33 = 197 项逐名冻结进 change 的 baseline-test-list.txt（比对时剥离文件分节头）；每 Task 后并集必须逐名一致。
5. **门禁与文档同步**：archcheck allowed 表按任务同步（T2 登记 protocol、T3 登记 codec 及新边，根包 companion 边移除）；ci.yml:119 `./internal/network` → `./internal/network/...`；ci.yml:266 M3C 步骤与 Makefile:124 不动；test-quickstart.md T0 行改 `./internal/network/...`；重写 internal/network/AGENTS.md（会话/传输范围 + 子包地图）+ 新建 protocol/AGENTS.md、codec/AGENTS.md（按 docs/agents-md-style.md，子包不放 CLAUDE.md）；tcp/AGENTS.md 不动；docs/architecture.md:17,24 与 docs/notes/compatibility.md 的边界描述同步。

## 全局约束（Global Constraints）

- 提交信息单行英文 `<type>(<scope>): <subject>`；代码注释/GoDoc 中文、反引号包裹 Go 标识符；注释不得出现任务编号（形如 `A-01`）。
- 消费方（internal/server、internal/client、cmd/mornlea* 全部、internal/sim）与 tcp/ 生产代码零改动；`network.X` 符号可寻址性与类型/错误/常量身份不变。
- 测试入口并集逐名不变（197 项基线）；测试函数名与 `t.Run` 标签逐名保留；wire 字节、包 ID、协议语义、校验规则、错误语义、Memory/TCP 行为、ProtocolVersion 全部不变。
- 每 Task 验收：`go test ./internal/network/... -race -count=1`（或定点子集）+ `go test ./internal/archcheck -count=1` + `go build ./...` + `-list` 并集对照，证据入 ledger。
- 阶段边界（T4）跑 `make dev-check` 与 `make test-race`；基准数值只记录不改变退出状态。
- 保护并行会话产物：不触碰 sim-subpackages / server-persistence-subpackage / pathfind 相关文件与主 worktree 未跟踪文件。

## OpenSpec change：`split-network-subpackages`

proposal.md（Why 量化 51 文件/164 入口/定点不可行/archcheck 不可见；非目标：不改任何 wire 字节、包 ID、协议语义、校验规则、错误语义、Memory/TCP 行为、协议版本）+ delta specs（repository-code-organization 新增 4 条 Requirements，沿用 storage 措辞模式：网络按域拆分子包 / 网络子包依赖方向单向 / 网络别名再导出保持消费面与协议语义 / 网络分包保持测试入口集合；TCP precedent 的 Memory/TCP parity 要求不放松）+ design.md + tasks.md + ledger.md + baseline-test-list.txt；`openspec validate --all --strict --no-interactive` 通过。

## 执行任务

纯 Go 任务不涉 Rust，worktree 已先跑 `make rust`。控制会话不直接实现，每 Task fresh implementer + SPEC/QUALITY 双评审，裁决记 ledger。

### Task 1: OpenSpec change 产物与基线快照

创建 `openspec/changes/split-network-subpackages/`：`proposal.md`（Why 量化：51 文件、164+33 测试入口、定点不可行、archcheck 包内耦合不可见；What Changes 以 **BREAKING（仓库内部源码结构）** 开头逐包枚举目标布局；非目标：不改任何 wire 字节、包 ID、协议语义、校验规则、错误语义、Memory/TCP 行为、协议版本）、`design.md`（导出面清单、临时导出与回收表、测试归属规则、依赖方向图）、`tasks.md`（对应下方 Task 2–4 的 checklist）、`specs/repository-code-organization/spec.md` delta（新增 4 条 ADDED Requirements：网络按域拆分子包 / 网络子包依赖方向单向 / 网络别名再导出保持消费面与协议语义 / 网络分包保持测试入口集合；措辞沿用 `openspec/changes/archive/2026-08-29-split-storage-subpackages/` 的模式，并保留 Memory/TCP parity 要求不放松）、`baseline-test-list.txt`（当前 `go test ./internal/network/... -list '.*'` 全集逐行：根包 164 项 = 151 Test + 7 Benchmark + 6 Fuzz，tcp 33 Test；注意 -list 输出含分节头，入基线时剥离头部保留逐名条目）、`ledger.md`。同时把根包 race 计时（`go test ./internal/network -race -count=1` 的耗时与结果）记入 change ledger。验收：`openspec validate --all --strict --no-interactive` 通过；不改任何 Go 代码。

### Task 2: protocol 子包（协议消息层）

以 `git mv` 迁移 `packet.go`、`message.go`、`message_chunk.go`、`message_command.go`、`message_companion.go`、`message_container.go`、`message_drop.go`、`message_hostile.go`、`message_inventory.go`、`message_player.go`、`registry.go`、`snapshot.go` 至 `internal/network/protocol/`（package protocol）。临时导出（记入 report 与 ledger，T3 回收或转正）：包 ID 访问器 6 个（clientPacketID/clientPacketForID/serverPacketID/serverPacketForID/commandRejectReasonID/commandRejectReasonForID，导出名由 implementer 按仓库命名习惯定并记录）、`ValidateDecodedClientWirePacket`（自 codec_client.go:306 归位 protocol，含 Handshake/Login 放行逻辑原样）、companion/hostile 的 wire 常量与 encode/decode 函数（codec_server.go:209-221 借用与 :179-191/:466-492 调度的那些，临时导出供根包 codec 文件过渡调用）。留守根包的 codec 文件改限定调用；根包 `package network` 测试中引用已迁移 unexported 符号处就地 `protocol.` 限定或按被测主体提前随迁（测试函数名与 `t.Run` 标签逐名保留）。新建根 `types.go`：package doc（根包保留会话与传输编排 + 别名再导出保证）+ 全部迁移符号的别名再导出（类型/常量/错误/var 函数别名，逐个别名带中文注释说明归属与身份保证）。archcheck allowed 表登记 `"internal/network/protocol": {"internal/core", "internal/companion"}` 并从根包边移除 internal/companion、新增 protocol 边。验收：`go test ./internal/network/... -race -count=1`、`go test ./internal/archcheck -count=1`、`go build ./...`、`-list` 并集与基线逐名一致；消费方与 tcp 生产代码零改动。

### Task 3: codec 子包（编解码层）

以 `git mv` 迁移 `chunk_codec.go`、`codec.go`、`codec_client.go`、`codec_server.go`、`codec_values.go`、`codec_primitives.go`、`frame.go` 至 `internal/network/codec/`（package codec）。companion/hostile 的 wire encode/decode 函数自 protocol 归位 codec（恢复 unexported；其调用点 codec_server.go/codec_client.go 回到包内直呼）；wire 常量留在 protocol 保持导出（protocol Validate 与 codec 预分配拒绝双方消费）；包 ID 访问器与 `ValidateDecodedClientWirePacket` 保持导出（codec 永久消费）。测试随迁（跟随被测主体）：chunk_codec_test(+fuzz)、codec_fuzz/golden/invalid/inventory/helpers、codec_primitives_test(+fuzz)、frame_test(+fuzz)、drop/furnace/container/hunger/worldtime/place_block_succeeded、message_companion_fuzz_test 及全部 6 个 Fuzz → codec；纯 Validate 主体测试 → protocol；根包留 login_test/memory_test/seed_test/benchmark_test/benchmark_helpers_test。混合文件按主体拆分，函数名逐名保留。`testdata/chunk-snapshot-v1.bin` 随 chunk_codec_test 迁至 codec（无外部引用）。根 types.go 增补 codec 侧别名（Codec/NewCodec/MaxCompressedSnapshot/MaxDecodedSnapshot/MaxSmallPayload/WriteFrame/ReadFrame/MaxFrameBytes 等）。archcheck 登记 `"internal/network/codec": {"internal/network/protocol", "internal/core"}`。验收：Task 2 全部验收项 + `go test ./internal/network/... -bench . -benchtime 1x`（微基准只在 CI 跑的盲区预演，数值只记录）。

### Task 4: 门禁与文档收尾

重写 `internal/network/AGENTS.md`（根包会话/传输范围 + protocol/codec/tcp 子包地图 + 依赖方向 + 既有信任边界/传输一致性/协议演进契约保留），新建 `protocol/AGENTS.md`、`codec/AGENTS.md`（按 `docs/agents-md-style.md`，子包不放 CLAUDE.md；tcp/AGENTS.md 不动）。同步 `.github/workflows/ci.yml:119`（`./internal/network` → `./internal/network/...`；:266 M3C 步骤不动）、`Makefile:124` 不动、`docs/notes/test-quickstart.md` T0 行改 `./internal/network/...`、`docs/architecture.md:17,24` 边界描述、`docs/notes/compatibility.md` ProtocolVersion 指涉（别名说明）。验收：`make dev-check`、`make test-race`、`go test ./internal/network/... -list '.*'` 终局对照基线逐名一致、`openspec validate --all --strict --no-interactive`、证据全量入 change ledger。

终审：whole-branch review → 推送 → CI（微基准盲区留意）→ 合并 → openspec archive → 清理。

## 验收标准

- `go test ./internal/network/... -list` 并集与基线 197 项逐名一致；
- 9 个消费包 + tcp 生产代码零改动，`network.X` 符号全部可寻址，类型/错误/常量身份不变；
- archcheck 新依赖方向全绿（protocol→companion 边、根包 companion 边移除）；
- Memory/TCP parity（TestTCPPlayerAndWorld/TestMemoryTCPParity）与 fuzz/golden 全绿且语义零修改；
- openspec strict 通过；AGENTS.md 体系按新规范就位。

## 风险

- A-03 并行分支重叠（用户已裁决现在拆）：纯移动+别名可被 rename 检测吸收；A-03 后续 rebase 时 message 文件冲突按移动后新路径解决。
- 别名遗漏 → 消费方编译失败：每 Task 原子提交 + `go build ./...` 兜底，评审盯别名清单完整性。
- 混合测试文件拆分争议：跟随被测主体规则 + ledger 裁决。
- 并行会话在途（sim-subpackages、server-persistence-subpackage、pathfind）：本 change 不触碰其文件；主 worktree 未跟踪产物保持不动。
