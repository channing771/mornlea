# split-network-subpackages 设计

## Context

动机与背景见 `proposal.md`，行为契约见本 change delta specs 与
`openspec/specs/repository-code-organization/spec.md`。

现状：`internal/network` 根包 53 个 Go 文件（23 生产 + 30 测试，实测生产
5273 行 / 测试 7476 行），按关注点可分三簇——协议消息层（`packet.go`、
`message.go` 与 8 个 `message_*.go`、`registry.go`、`snapshot.go`）、编解码
层（`chunk_codec.go`、`codec.go`、`codec_client.go`、`codec_server.go`、
`codec_values.go`、`codec_primitives.go`、`frame.go`）、传输与会话
（`stream.go`、`transport.go`、`memory.go`、`login.go`）；`tcp/` 子包已由
PR #109 拆出且消费根包仅 6 类符号。测试入口 197 项：根包 164（151 Test +
7 Benchmark + 6 Fuzz，其中 `BenchmarkTCPLoopback*` 两个基准为根包对 tcp 的
测试回边，archcheck 只扫非测试文件不受影响）+ tcp 33 Test。消费面 9 个包
引用根包（符号级计数沿用执行计划在同基线 SHA 的先行实测）：
`internal/server` 127 符号 2033 处、`cmd/mornlea/app` 616、
`internal/client` 411、`cmd/mornlea/capture` 120、`cmd/mornlea/benchmark`
39、`cmd/mornlea` 36、`cmd/mornlea-server` 34、`internal/sim` 测试 5 处，
加 `tcp/` 生产代码 400+ 处。

## Goals / Non-Goals

**Goals:**

- 建立 `internal/network/{protocol,codec}` 两个子包，根包保留会话与传输
  编排并以 `types.go` 别名再导出迁出符号。
- 让协议/编解码域迭代可定点：`go test ./internal/network/<子包>` 不编译
  执行其他子包的测试。
- 依赖方向单向并由 archcheck 按任务增量登记，防漂移。
- 消费方生产代码零改动：既有 `network.X` 符号面逐符号保持可寻址、同名称、
  同类型/错误/常量身份。
- 文档按子包地图重组：根 `AGENTS.md` 总纲 + protocol/codec 各一份
  AGENTS.md（`tcp/AGENTS.md` 不动）。

**Non-Goals:**

- 不合并域、不再细拆（protocol 不再按 message 域细拆——密封接口与冻结包 ID
  表钉死同包；codec 不再按 primitive/frame/分发层细拆——同簇共享编码原语与
  wire 常量，拆开导出面爆炸且无测试提速收益）。
- 不改任何 wire 字节、包 ID、协议语义、校验规则、错误语义、Memory/TCP
  行为、`ProtocolVersion` v31、golden/fuzz 资产与版本化 bin fixture 字节。
- 不迁移任何消费方调用点，不在子包外新建兼容转发层。
- 不引入新的性能阈值；race 与微基准计时只记录不设门槛。

## 耦合事实（边界依据）

1. **密封接口钉死协议同包**：`ClientPacket`/`ServerPacket` 以 unexported
   marker（`clientPacket()`/`serverPacket()`）密封，全部 message DTO 与
   `snapshot.go` 实现之；主校验器 `ValidateClientPacket`/
   `ValidateServerPacket` 扇出到各 `Validate`。→ packet + message +
   registry + snapshot 必须同一包（protocol）。
2. **registry ↔ message 双向**：registry 映射消息类型与 `RejectReason`；
   `message_player.go` 的 `CommandRejected.Validate` 回调
   `commandRejectReasonID`。同包即无环。
3. **companion/hostile 二进制编解码错位**：`encodeChatEvent` 等 12 个 wire
   函数住在 message 文件里，被 `codec_server.go` 调度并借用其 unexported
   wire 常量。→ wire 函数归位 codec 包（其余所有消息的 wire 代码都在
   codec 侧，规范一致）；Validate 所需 wire 常量留在 protocol 导出。
4. **跨层泄漏**：`memory.go` 调用的 `validateDecodedClientWirePacket`（住
   `codec_client.go`）实为协议级校验——放行 `StateHandshake` 的
   `ClientHello` 与 `StateLogin` 的 `LoginStart` 以保留冻结的版本/身份拒绝
   路径，其余转发 `ValidateClientPacket`，不碰字节层。→ 归位 protocol 包
   导出。
5. **frame 依赖原语**：帧封装复用 `codec_primitives` 的 canonical uvarint。
   → `frame.go` 随 codec 包，编码原语全部保持 unexported，零额外导出面。
6. **tcp 生产代码只消费根包 6 类符号**：`Codec`/`NewCodec`、`State`、
   `ClientPacket`/`ServerPacket`、三个 stream 接口、`WriteFrame`/
   `ReadFrame`、`ErrClosed` → 别名后 tcp 源码零改动、tcp → network
   archcheck 边不动。

## 目标布局与依赖方向

```
internal/network/
├── types.go                    # package doc + 别名再导出（消费面零改动）
├── stream.go / transport.go / memory.go / login.go   # 根包保留会话与传输编排
├── login_test.go / memory_test.go / seed_test.go / benchmark_test.go
├── protocol/   # packet.go, message.go, message_*.go(8), registry.go, snapshot.go + 协议级测试
├── codec/      # chunk_codec.go, codec.go, codec_client.go, codec_server.go, codec_values.go, codec_primitives.go, frame.go + 编解码测试（含 benchmark_helpers_test.go）+ testdata/
└── tcp/        # 完全不动
```

依赖方向（archcheck allowed 表按任务增量登记，preflight 裁决见 ledger）：

```
network（根）──→ protocol ──→ {internal/core, internal/companion}
    │               ▲
    │               │（codec → protocol 单向）
    ├──→ codec ─────┘
    ├──→ internal/core（login.go NormalizeDisplayName 既有边）
    └──→ tcp（反向：tcp → network 既有边不动，根不得依赖 tcp）
```

- network（根）→ {protocol, codec, internal/core}；根包 internal/companion
  边随 `message_companion.go` 移交 protocol 后移除。
- protocol → {internal/core, internal/companion}。
- codec → {protocol, internal/core}。
- tcp → network（既有边不动）。
- 禁止：子包 → 根包反向边；protocol → codec；根 → tcp。

## 导出面清单与别名策略

消费方既有 `network.X` 引用是别名再导出的验收锚点，拆分后必须逐符号继续以
`network.X` 可寻址、同名称、同类型/错误/常量身份：

- 留守根包原生（无别名风险）：会话与传输 API——`Identity`、`RemoteError`、
  `PendingLogin`、`BeginServerLogin`/`Accept`/`Reject`、`LoginClient`/
  `LoginClientWithSeed`、`NewMemoryPair`/`NewMemoryStreamPair`、
  `ClientEndpoint`/`ServerEndpoint`、三个共享 stream 接口、`ErrClosed`。
  消费面最高频符号（`ClientEndpoint` 147、`Identity` 103、
  `NewMemoryPair` 96 处）全部原生。
- 迁出 + 根包 `types.go` 别名再导出：
  - 类型（`type X = protocol.X` / `type Codec = codec.Codec` 形态，保方法
    集）：`ClientPacket`/`ServerPacket` 密封接口、`ProtocolVersion` 所在
    `packet.go` 的全部导出类型、`State`、全部 message DTO（`PlayerState`、
    `RemotePlayerState`、`ClientHello`、`LoginStart`、`LoginSuccess`、
    hotbar/inventory/drop/furnace/container/companion/hostile/chat 等）、
    `ChunkSnapshot` 及其 section 值类型、`Codec`；
  - 常量：`ProtocolVersion`、`State*`、`RejectReason`/`LoginRejectCode`/
    `DisconnectCode`/`ChatEventKind` 等枚举值、`MaxCompressedSnapshot`/
    `MaxDecodedSnapshot`/`MaxFrameBytes`/`MaxItemDropBatch`/
    `MaxSmallPayload`；
  - 函数（var 别名）：`NewCodec`、`ValidateClientPacket`、
    `ValidateServerPacket`、`WriteFrame`、`ReadFrame`。
- 子包新增导出（仅承接既有调用方与永久消费方，不为对称性加导出）：
  protocol 导出包 ID 访问器 6 个（现 `clientPacketID`/
  `clientPacketForID`/`serverPacketID`/`serverPacketForID`/
  `commandRejectReasonID`/`commandRejectReasonForID`，导出名由 implementer
  按仓库命名习惯定并记 ledger）、`ValidateDecodedClientWirePacket` 与
  companion/hostile 的 wire 常量（protocol `Validate` 与 codec 预分配拒绝
  双方永久消费）；codec 导出仅 `Codec`/`NewCodec` 等既有面。

别名策略：

- 类型用类型别名保方法集（`msg.Validate` 继续可用），错误/常量用 `var`
  别名保值身份；别名只覆盖「迁出 + 别名再导出」清单，未列入的子包内导出
  不加别名，根包内部代码直接以 `protocol.`/`codec.` 限定名消费。
- 逐个别名带中文注释说明归属与身份保证（`types.go` 的 package doc 说明根
  包保留会话与传输编排 + 别名再导出保证）。
- 验收锚点：拆分前后全仓 `network.` 符号引用清单 diff 为空（消费方与 tcp
  生产代码零改动）；`go build ./...` 兜底。

## 临时导出与回收表

Task 2 建 protocol 时，仍留守根包的 codec 文件需要消费迁出符号，按下表
临时导出；Task 3 codec 落位后按终态回收或转正，全程记 ledger：

| 符号组 | Task 2（建 protocol） | Task 3（建 codec） | 终态 |
|---|---|---|---|
| 包 ID 访问器 6 个（`clientPacketID`/`clientPacketForID`/`serverPacketID`/`serverPacketForID`/`commandRejectReasonID`/`commandRejectReasonForID`） | 临时导出（导出名按仓库命名习惯定并记 ledger） | 保持导出 | 转正：codec 永久消费，属协议冻结契约面 |
| `validateDecodedClientWirePacket`（自 `codec_client.go` 归位 protocol，含 Handshake/Login 放行逻辑原样） | 导出为 `ValidateDecodedClientWirePacket` | 保持导出 | 转正：根包 `memory.go` 与 codec 永久消费 |
| companion/hostile wire 常量（`codec_server.go` 借用的那些） | 临时导出 | 留在 protocol 保持导出 | 转正：protocol `Validate` 与 codec 预分配拒绝双方消费 |
| companion/hostile wire encode/decode 函数（`codec_server.go` 调度的 12 个） | 临时导出（供根包 codec 文件过渡调用） | 移入 codec，调用点回包内直呼 | 回收：恢复 unexported |

## 测试归属规则

测试文件归属按「Test/Benchmark/Fuzz 函数直接调用的生产符号所在域」判定
（跟随被测主体），歧义由实施任务裁决并记 ledger，不凭文件名猜测；测试
函数名与 `t.Run` 标签逐名保留；子包测试不得导入根包（根 → 子包方向已
占用，反向导入会成环）。

- protocol 收：`packet_test.go`、`registry_test.go`、`message_test.go` 及
  message 域中纯 Validate 主体的测试。
- codec 收：`chunk_codec_test.go`(+fuzz)、`benchmark_helpers_test.go`
  （chunk codec 域基准夹具）、`codec_fuzz_test.go`、
  `codec_golden_test.go`、`codec_invalid_test.go`、
  `codec_inventory_test.go`、`codec_helpers_test.go`、
  `codec_primitives_test.go`(+fuzz)、`frame_test.go`(+fuzz)、
  `drop_test.go`、`furnace_test.go`、`container_test.go`、`hunger_test.go`、
  `worldtime_test.go`、`place_block_succeeded_test.go`、
  `message_companion_fuzz_test.go` 等 Codec 门面往返测试，及全部 6 个 Fuzz
  （`FuzzChunkSnapshotCodec`、`FuzzSmallPacketCodec`、
  `FuzzPrimitiveDecoder`、`FuzzReadFrame`、`FuzzCompanionMessageCodec`、
  `FuzzHostileMessageCodec`）。
- 根包留：`login_test.go`、`memory_test.go`、`seed_test.go`（主体为
  `LoginClientWithSeed` 会话路径）、`benchmark_test.go`（跨域基准留守，CI
  M3C 步骤与 Makefile `bench-multiplayer` 零改动）。
- 混合文件按主体拆分：如 `message_hostile_test.go` 同时承载 Validate、
  Codec 往返与 `FuzzHostileMessageCodec`，`message_companion_test.go` 同时
  覆盖 Validate 与 wire 往返——按被测主体拆入 protocol/codec，测试函数名
  与子测试标签逐名保留。
- `testdata/chunk-snapshot-v1.bin` 随 `chunk_codec_test.go` 迁至
  `internal/network/codec/testdata`（实测无外部引用），逐字节不变。

## Decisions

### 1. 根 + protocol + codec 三包，而不是「不拆」或「更细拆」

三簇边界由耦合事实钉死（密封接口同包、registry ↔ message 双向、frame 依赖
原语），拆分后单域定点收益直接。被否决的替代方案：「仅拆 protocol」——
编解码域 1.7k 行仍与传输会话同包，迭代 codec 依旧为会话测试付费；「protocol
再按 message 域细拆」——密封接口与 `CommandRejected.Validate` 回调钉死
registry 同包，细拆导出面爆炸。

### 2. 别名再导出保持消费面，而不是同批迁移调用方

9 个包引用根包（8 消费包 + tcp），`internal/server` 单包 127 符号 2033 处；
同批迁移会把与本 change 目标无关的大面积调用点改写混入拆分 diff，违反
「拆分不改行为」的可回退性。别名是 Go 零成本机制，类型别名保方法集、
错误/常量别名保值身份，不产生运行时转发。被否决的替代方案：「不留别名、
消费方改用新限定名」——消费面 diff 巨大且无行为收益；若未来需要迁移调用方，
应另立 change。

### 3. packet + message + registry + snapshot 同包（protocol）

密封接口以 unexported marker 实现、registry 与 `CommandRejected.Validate`
双向引用、主校验器扇出到各 `Validate`——Go 的 unexported 密封与包内回调
机制要求四者同包，同包即无环。被否决的替代方案：「导出 marker 或改接口
密封方式」——改变协议冻结契约面，超出本 change 非目标。

### 4. companion/hostile wire 函数归位 codec，wire 常量留 protocol

12 个 wire 函数住错在 message 文件（其余所有消息的 wire 代码都在 codec
侧），归位 codec 恢复「wire 编解码归 codec」的规范一致；Validate 所需的
wire 常量是协议冻结契约面，留 protocol 导出由 protocol `Validate` 与
codec 预分配拒绝双方消费，避免 codec → protocol 之外的环或重复定义。过渡
期（Task 2）经临时导出让根包 codec 文件继续编译，Task 3 归位后回收。

### 5. `validateDecodedClientWirePacket` 归位 protocol 导出

它是协议级校验（放行 Handshake/Login 保留冻结的版本/身份拒绝路径），不是
字节层编解码；`memory.go` 跨层调用它是泄漏证据。归位 protocol 并导出为
`ValidateDecodedClientWirePacket`，放行逻辑原样迁移，语义零修改。

### 6. 测试归属跟随被测主体，入口并集逐名冻结

归属判定不凭文件名，混合文件按主体拆分、歧义记 ledger；拆分前 197 项入口
（根 164 = 151 Test + 7 Benchmark + 6 Fuzz，tcp 33 Test）逐名冻结进
`baseline-test-list.txt`，每 Task 后 `-list` 并集必须逐名一致。被否决的
替代方案：「测试文件整文件随生产文件走」——混合文件会把 Validate 测试与
wire 往返测试强行绑定到同一包，破坏定点性。

### 7. archcheck 按任务增量登记（preflight 裁决）

allowed 表在对应 task 内登记：Task 2 登记
`"internal/network/protocol": {"internal/core", "internal/companion"}` 并
从根包边移除 internal/companion、新增 protocol 边；Task 3 登记
`"internal/network/codec": {"internal/network/protocol", "internal/core"}`
；Task 4 只做文档与 CI 门禁、不改 archcheck 表。沿「不预先登记未使用的边」
惯例，方向契约以 delta spec 上限形式书写，实际登记以实施时 `go list` 实测
消费边为准。

### 8. 分任务可独立回退

Task 2（protocol + 根 `types.go` 别名 + archcheck）→ Task 3（codec + 测试
随迁 + 临时导出回收 + archcheck）→ Task 4（文档与门禁收尾）为独立提交
序列，每步结束仓库可编译、`-list` 并集与基线一致、消费面零改动；任一步
失败可单独回退而不拖垮整体。

## 风险

- 别名遗漏：编译期即失败（消费方引用无法解析），风险低、无需运行时守卫；
  每 Task 原子提交 + `go build ./...` 兜底，评审盯别名清单完整性。
- 混合测试文件拆分争议：跟随被测主体规则 + ledger 裁决；测试函数名与
  `t.Run` 标签逐名保留是评审硬检查项。
- 临时导出未回收：Task 3 验收必须核对回收表逐项终态，评审记 ledger。
- 计时基线漂移：race 与 `-bench . -benchtime 1x` 微基准数值只记录对照，
  不设门槛（见 ledger Baseline）。
- 并行会话在途（sim-subpackages、server-persistence-subpackage、pathfind
 ）：本 change 不触碰其文件；主 worktree 未跟踪产物保持不动。
