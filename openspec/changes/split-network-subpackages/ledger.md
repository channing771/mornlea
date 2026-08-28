# Execution Ledger

## Baseline

- 基线提交：`6fd03011`（本 worktree `split-network-subpackages`，HEAD
  `6fd03011cda15b369c76617612a53ac1d690b6a2`，工作区除本 change 产物外
  干净）。
- 基线快照：`go test ./internal/network/... -list '.*'` 输出（剔除空行与
  ok 行，条目按原始输出逐行保留，以 `#` 分节注释标注包边界与 Test/
  Benchmark/Fuzz 分组）持久化于本 change 目录 `baseline-test-list.txt`，
  计数 **根包 164（Test 151 + Benchmark 7 + Fuzz 6）+ tcp 33（Test 33）
  = 197**，三类逐项核对一致；后续任务以「剥离 `#` 行后的逐名并集」与
  基线比对。
- 计时基线（同基线 SHA、本 worktree 实测，2026-08-28，darwin/arm64，
  go1.26.0）：
  - race：`go test ./internal/network -race -count=1` →
    `ok ... 2.797s`（首次实测）；复跑确认 `ok ... 2.629s`。均 PASS。
  - 非 race：`go test ./internal/network -count=1` → `ok ... 1.212s`。
  - 后续任务以本 ledger 记录的实测值为对照基准，计时只记录不设门槛。
- 文件计数基线：根包 **53 个 Go 文件**（实测 23 生产 5273 行 + 30 测试
  7476 行）；执行计划 Why 节写的「51 个 Go 文件」与其自身 23+30 分解矛盾，
  系算术笔误，以本实测 53 为准（分解数字 23/30 与行数约值与计划一致）。
  `tcp/` 6 文件不动。
- fixture 基线：`internal/network/testdata/chunk-snapshot-v1.bin` 1 个，
  随 `chunk_codec_test.go` 迁至 `internal/network/codec/testdata`
  （Task 3），实测全仓无其他引用。
- 消费面基线：9 个包引用根包——`internal/server` 127 符号 2033 处、
  `cmd/mornlea/app` 616、`internal/client` 411、`cmd/mornlea/capture`
  120、`cmd/mornlea/benchmark` 39、`cmd/mornlea` 36、`cmd/mornlea-server`
  34、`internal/sim` 测试 5 处，加 `internal/network/tcp` 生产代码 400+
  处（执行计划先行实测口径）；拆分期间消费方与 tcp 生产源码零改动。
- archcheck 基线（`internal/archcheck/dependency_test.go`）：
  `"internal/network": {"internal/companion", "internal/core"}`、
  `"internal/network/tcp": {"internal/network"}`。

## Rulings

- Ruling（preflight-1，控制器裁决）：Task 2 根包测试引用已迁移 unexported
  符号时，可就地 `protocol.` 限定或按被测主体提前随迁——两种处理均合规，
  由 implementer 按耦合事实选择；测试函数名与 `t.Run` 标签逐名保留是硬
  约束。
- Ruling（preflight-2，控制器裁决）：archcheck allowed 表按任务增量登记
  ——Task 2 登记 protocol 边并移除根包 internal/companion 边；Task 3 登记
  codec 边；Task 4 只做文档与 CI 门禁，不改 archcheck 表。
- Ruling：基线文件计数以实测 53（23 生产 + 30 测试）为准，执行计划
  「51 文件」系笔误——计划自身的 23+30 分解与行数实测一致；按「不强迫
  数字吻合、记录实测」纪律处理，proposal 采用实测值。

## Task 4 实现记录（门禁与文档收尾）

- 实现内容（纯文档与门禁同步，Go 代码零改动，`git status` 实证仅含下列
  文件）：
  - 重写 `internal/network/AGENTS.md` 为子树总纲（按
    `docs/agents-md-style.md` 总纲骨架）：开头作用域段（声明本子树不放
    `CLAUDE.md`）+ Directory Map（根包 types.go/stream.go/transport.go/
    memory.go/login.go 与 protocol/codec/tcp 子包行）+ Dependency
    Direction（根包 → {core, protocol, codec}；protocol → {core,
    companion}；codec → {protocol, core}；tcp → 根包；拒绝反向导入与
    codec→companion 边，强制点 `TestInternalDependenciesAreOneWay`）+
    别名再导出政策（`network/types.go` 闭集别名、`ProtocolVersion` 定义
    点在 `protocol/packet.go`）+ 既有信任边界/传输一致性/协议演进三大
    契约保留（传输一致性一节的包职责句更新为新结构，其余措辞沿用）+
    Documentation Sync Policy + Focused Verification 表（按改动域定点
    命令，全子树命令 `go test ./internal/network/... -race -count=1`）。
  - 新建 `internal/network/protocol/AGENTS.md`（密封接口与版本钉死、
    冻结包 ID 注册表、Validate 契约、companion 常量同源契约四节 + 回归
    测试组织 + 定点验证；节标题内嵌 `protocol/packet.go` 等真实路径，
    点名测试经 `-list` 核实）。
  - 新建 `internal/network/codec/AGENTS.md`（规范 uvarint 与字节原语、
    编码原子性与分发、帧封装、上限守卫与 chunk snapshot、fuzz 与 golden
    义务五节 + helper 中心与回归测试 + 定点验证；fuzz 冒烟示例命令
    `-fuzz FuzzSmallPacketCodec -fuzztime 10s` 实测可执行）。
  - `.github/workflows/ci.yml`「架构、存储与协议门禁」步骤
    `./internal/network` → `./internal/network/...`；:266 M3C 微基准
    步骤与其余行零改动（diff 实证）。`Makefile` `bench-multiplayer`
    零改动（核对确认）。`tcp/AGENTS.md` 零改动。
  - `docs/notes/test-quickstart.md` T0「协议 / 网络 / 存档」行
    `./internal/network` → `./internal/network/...`（storage 部分原样）；
    T3 race-required 域清单本就按包名列 `internal/network`，语义不变。
  - `docs/architecture.md` §3/§4 网络边界句更新为「根包会话与传输编排
    （stream 接口、endpoint 门面、登录状态机、Memory transport）+ 别名
    再导出门面；protocol 持协议消息层，codec 持编解码与帧封装；tcp 边
    不变」。
  - `docs/notes/compatibility.md` ProtocolVersion 指涉更新为「定义在
    `internal/network/protocol` 的 `ProtocolVersion`，根包
    `internal/network` 别名再导出」，逐版语义注释路径同步指向
    `internal/network/protocol/packet.go`。
- archcheck：按 preflight-2 裁决本任务不改 `allowed` 表（Task 2/3 已
  增量登记），仅复验通过。
- 验证（同实现工作区实测，2026-08-28，darwin/arm64，go1.26.0；计时只
  记录不设门槛）：
  - `go test ./internal/archcheck -count=1` → `ok ... 5.531s`。
  - `openspec validate --all --strict --no-interactive` →
    `Totals: 77 passed, 0 failed (77 items)`。
  - `go test ./internal/network/... -list '.*'` 剥离 `#` 行与空行后
    197 项与 `baseline-test-list.txt` 排序 diff 为空（逐名一致）；分布
    根包 43（Test 36 + Benchmark 7）、protocol 29、codec 92
    （Test 86 + Fuzz 6）、tcp 33。
  - `go test ./internal/network/... -bench . -benchtime 1x`（盲区预演，
    数值只记录）：4 包 ok，52 条基准结果行全部可运行（含根包
    `BenchmarkTCPLoopback*`）；代表值——`BenchmarkRemotePlayerStateCodec`
    Encode 7.6 µs / Decode 9.3 µs、`BenchmarkWorstLegalChunkSnapshot`
    Encode 205 µs / Decode 272 µs（compression-ratio 426.8、logical
    196749 B、wire 461 B）、`BenchmarkTCPLoopbackPlayerInput` 117 µs、
    `BenchmarkTCPLoopbackChunkSnapshot` 834 µs（0.55 MB/s）。1x 单样本
    噪声大，不与既有记录比对。
  - `make dev-check` → exit 0（wall 1m09s；内含 `gofmt -l .` 无输出与
    `go vet ./...` 清洁，即收尾门禁 4.3 的 gofmt/vet 项；Go short 全 ok
    含 network 四包；Rust fmt/clippy/单测 171+175 passed）。
  - `make test-race`（`go test ./... -race`）→ exit 0，38 包 ok 零
    FAIL；`internal/network` 2.786s、`codec` 2.236s、`protocol` 1.583s、
    `tcp` 2.113s。
- AGENTS.md 自查（`docs/agents-md-style.md` 清单）：节标题内嵌路径逐一
  核实存在；点名测试函数全部经 `-list` 核实；无抄录的会漂移数值（上限
  一律点名常量与测试）；与父级 `internal/AGENTS.md`/本总纲无重复条文
  （跨包契约只在总纲陈述一次，子包指南以指针引用）；protocol/codec
  目录无 `CLAUDE.md`；无任务编号。


## Task 3 实现记录（codec 子包）

- 实现 SHA：`bc95ac24`（`refactor(network): extract codec subpackage with
  alias re-exports`，9 个生产文件 + `testdata/chunk-snapshot-v1.bin` 与 22 个
  测试文件经 `git mv` 迁移；6 个 protocol 侧拆分文件与
  archcheck/types.go 修改见下）。
- 实现期裁决（记入本节，均依 design.md「测试归属跟随被测主体」规则裁定）：
  1. **protocol 域纯主体测试随 Task 3 迁入 protocol/**：`packet_test.go`、
    `registry_test.go`（转 `package protocol`，限定名就地还原 unqualified）、
    `message_test.go`、`snapshot_test.go`（转 `package protocol_test`，外部
    测试形态保持，`network.` → `protocol.`）。Task 2 preflight-1 允许的
    「就地限定」过渡态就此收敛为 design 既定终态；根包测试仅留
    login/memory/seed/benchmark 四文件。
  2. **`benchmark_helpers_test.go` 随簇迁入 codec/**：brief 的根包留守清单
    列有此文件，但其内容（`worstLegalBenchmarkSnapshot` 夹具与
    `benchmarkPayload`）只被 chunk codec 域测试（`chunk_codec_test.go`）消费，
    且 `benchmark_test.go`（package network_test）自带同名副本；按被测主体
    随 `chunk_codec_test.go` 迁入 codec，CI M3C 步骤与 Makefile
    `bench-multiplayer` 指向的基准主体（benchmark_test.go）仍在根包，零影响。
    避免了「根包留死代码 + codec 侧再复制一份」的双份浪费。
  3. **`message_companion_limit_lock_test.go` 归 codec**（brief 混合文件清单
    未列）：两个锁定函数各自在单一函数体内混合 Validate 与 wire 往返，按函数
    拆分必须改名（违反逐名保留硬约束）；其 wire 侧（encode/decode 直呼）为
    定位性主体，归 codec 后可直接使用测试 helper（`taskChatEvent`/
    `companionSpeechEvent`，由 codec 侧 `message_companion_test.go` 提供），
    零 helper 复制。其 `internal/companion` import 仅为测试夹具取值
    （`testCompanionID` 同源），archcheck 只扫非测试文件，codec 生产边保持
    `{internal/core, internal/network/protocol}`（`go list` 实证）。
  4. **混合文件按「函数直接调用的生产符号所在域」逐函数拆分**（brief 允许，
    design 规则口径）：
    - `message_companion_test.go` → protocol 收
      `TestCompanionMessageIDsAreAppendOnly`、
      `TestChatEventTaskEnumsAreFrozen`（纯 registry/枚举，零 helper 复制）；
      其余 14 函数与全部 helper 归 codec（Validate + wire 往返混在单函数体内，
      protocol 无法触达 codec unexported 编码器，结构性只能归 codec）。
    - `message_hostile_test.go` → protocol 收
      `TestHostileMessageIDsAreFrozen`、
      `TestHostileMessagesValidateRejectsInvalidRecords`（纯 Validate 矩阵，
      按生产符号域归 protocol）；5 个冻结 DTO 夹具
      （`hostileSpawnFixture`/`hostileStateFixture`/`hostileSpawnMessage`/
      `hostileStateMessage`/`hostileDespawnMessage`，约 30 行）在两侧各持一份
      同值副本——按生产符号域判定归属的机械结果，取值被两侧测试自身钉死；
      整文件归 codec 的替代方案需复制 `assertServerRegistry`+
      `sameServerPacketType`（约 90 行），劣于本方案。其余 5 函数（含
      `FuzzHostileMessageCodec`）归 codec。
    - `drop_test.go` → protocol 收 `TestProtocolV4DropPacketIDsAreFrozen`、
      `TestItemDropMessagesValidateBoundedBatches`（+ `dropTestID`/
      `dropTestUpsert` 同值副本）；codec 收其余 5 函数；
      `TestProtocolV11CarriesWornToolDropOnCodecAndMemory` 单函数同时驱动
      codec wire 往返与 Memory transport（`NewMemoryStreamPair`），design 禁止
      子包测试导入根包，protocol/codec 均无法承载完整函数体 → 留守根包并入
      `memory_test.go`（`package network_test`）：wire 半经根包 `NewCodec`
      别名门面（`EncodeServer`/`DecodeServer` 对 StatePlay 非 snapshot packet
      与包内直呼路径逐分支一致），DropID 夹具就地内联（原 `dropTestID` 为
      codec 域 helper，不跨包复制）。
    - `furnace_test.go` → protocol 收 `TestProtocolV7FurnacePacketIDsAreFrozen`
      （零 helper 复制）；其余归 codec。
    - `container_test.go` → protocol 收
      `TestProtocolV12ChestStatePacketIDIsFrozen`、
      `TestMoveContainerStackChestUnifiedSlotRange`、
      `TestContainerNeutralMessagesRejectUnknownKind`（+ `testChestRef` 同值
      副本）；其余归 codec。
    - `worldtime_test.go` → protocol 收 `TestProtocolVersionPinned`（新建
      protocol/worldtime_test.go 单函数文件，保持「版本无关命名」约定所在
      文件语境）；根包收 `TestProtocolV26RejectsPriorVersionsBeforePlay`、
      `TestHandshakeAcceptsCurrentVersion`（主体为 `BeginServerLogin` 握手/
      会话路径，且依赖根包测试 helper `staticClientHelloStream`，并入
      `login_test.go`，函数体零改动）；其余 9 函数与 playerState 偏移 helper
      归 codec。`TestProtocolV10DropSelectedItemRegistryIsFrozen` 因含
      `encodeClientPacketPayload` 非法状态拒绝断言（codec unexported）归
      codec，registry 断言经 protocol 导出访问器照常生效。
  5. **seed_test.go 留守根包的两个 wire 前缀测试**（
    `TestProtocolV23LoginSuccessCarriesWorldSeed`、
    `TestProtocolV23LoginSuccessWorldSeedAcceptsFullRange`）改经根包
    `NewCodec` 别名门面驱动（brief 钉死 seed_test.go 留守根包，而
    `encodeServerControlPayload`/`decodeServerControlPayload` 迁入 codec 后
    不可再被根包直呼；EncodeServer/DecodeServer 对 StateLogin 分支与直呼
    逐分支一致），并以 `mustSeedPlayerID`/`mustSeedCodec` 本地 helper 替代
    迁走的 `mustCodecPlayerID`。
- 导出表面（Task 2 临时导出回收表逐项终态）：
  - 包 ID 访问器 6 个（`ClientPacketID`/`ClientPacketForID`/
    `ServerPacketID`/`ServerPacketForID`/`CommandRejectReasonID`/
    `CommandRejectReasonForID`）：转正（永久导出），消费者 codec 生产
    （codec_client/codec_server 分发）+ codec/protocol 域测试。
  - `ValidateDecodedClientWirePacket`：转正（永久导出），消费者根包
    memory.go + codec decodeClientPacketPayload 尾部校验。
  - companion wire 常量 8 个 + hostile wire 常量 7 个：转正（永久导出），
    消费者 codec 生产（codec_client 预分配拒绝 / codec_server 预分配拒绝 /
    companion_wire / hostile_wire）+ protocol `Validate`。
  - `ValidBlockID`/`SectionWords`/`ReadSectionPacked`：转正（永久导出）。
    消费者：`ValidBlockID`/`SectionWords` 为 codec 生产 chunk_codec.go；
    `ReadSectionPacked` 的生产消费者是 protocol 包内 snapshot.go，跨包消费
    仅 codec 测试 chunk_codec_test.go（test-only，与 `GridCraftingViewSlots`
    同类——导出由测试跨包消费需求正当化）。
  - `InvalidClientPacket`/`InvalidServerPacket`：转正（永久导出），消费者
    codec 生产 codec_client.go/codec_server.go 错误构造。
  - `ChatSpeechTextMaxBytes`：转正（永久导出），消费者 codec 生产
    companion_wire.go。
  - `MaxChunkBlockIndex`（drop_test.go 消费，随迁 codec）、
    `GridCraftingViewSlots`（codec_inventory_test.go 消费）：**保持导出**，
    终态理由「codec 域测试跨包消费 protocol 冻结值域上界」——两常量由
    protocol `Validate` 生产自用，撤销导出会使 codec 测试退回裸字面量，违背
    同源锁定意图；无生产消费者以外的导出面扩张。
    更正（T3-1 评审裁决）：`MaxChunkBlockIndex` 的消费前提记录有误——
    `TestItemDropMessagesValidateBoundedBatches` 拆分后落位 protocol 包内
    （in-package），codec 侧零引用，导出成为孤儿。回收导出：`message_chunk.go`
    更名 `maxChunkBlockIndex`（unexported，`ItemDrop.Validate` 值域检查与
    protocol 包内 drop 测试自用）。`GridCraftingViewSlots` 维持转正：
    `codec_inventory_test.go` 跨包消费属实，维持「codec 域测试跨包消费」
    终态理由。
  - wire encode/decode 函数 12+1 个：按 Task 2 裁决 `git mv`
    `companion_wire.go`/`hostile_wire.go` 进 codec，包内直呼，保持
    unexported，回收完成（无导出面）。
- 根包 `types.go` 增补 codec 侧别名（逐别名带中文注释）：`type Codec =
  codec.Codec`、`var NewCodec = codec.NewCodec`、常量
  `MaxCompressedSnapshot`/`MaxDecodedSnapshot`/`MaxSmallPayload`/
  `MaxFrameBytes`、`var WriteFrame = codec.WriteFrame`/`ReadFrame =
  codec.ReadFrame`；package doc 同步为 network → {protocol, codec} 双子包
  再导出保证。
- archcheck：根包边 `{"internal/core", "internal/network/codec",
  "internal/network/protocol"}`、新增 `"internal/network/codec":
  {"internal/core", "internal/network/protocol"}`；`go list` 实测 codec 生产
  import 无 internal/companion（其测试文件的 companion import 不入边）。
  `internal/archcheck/baseline_test.go` 无需改动（Task 2 已把 ProtocolVersion
  权威来源路径指向 `internal/network/protocol/packet.go`，本次无路径漂移）。
- 测试入口分布（`-list` 并集 197 逐名与基线一致，diff 为空）：根包 43
  （Test 36 + Benchmark 7）、protocol 29（Test 29）、codec 92（Test 86 +
  Fuzz 6）、tcp 33（Test 33）；Test 184 + Benchmark 7 + Fuzz 6 与基线三类
  计数逐项一致。全部 6 个 Fuzz 落位 codec；基准（含 `BenchmarkTCPLoopback*`）
  留守根包，CI M3C 与 Makefile `bench-multiplayer` 零改动。
- 验证（同实现工作区实测，2026-08-28，darwin/arm64，go1.26.0）：
  - `go test ./internal/network/... -race -count=1` → 根包 ok 1.668s、
    codec ok 2.147s、protocol ok 2.271s、tcp ok 2.456s，全部 PASS。
  - `go test ./internal/archcheck -count=1` → ok 5.826s。
  - `go build ./...` → 通过；`go vet ./internal/network/...` → 清洁。
  - `-list` 并集 197 项与 `baseline-test-list.txt`（剥离 `#` 行）排序 diff
    为空；每包分布 43/29/92/33。
  - `go test ./internal/network/... -bench . -benchtime 1x`（盲区预演，数值
    只记录不设门槛）：全部基准可运行（含根包 `BenchmarkTCPLoopback*`），
    代表值——`BenchmarkSmallPacketCodec` 子项 0.5–7.2 µs/op、
    `BenchmarkRemotePlayerStateCodec` Encode 14.7 µs / Decode 13.0 µs、
    `BenchmarkChatCommandCodec` Encode 12.8 µs / Decode 11.6 µs、
    `BenchmarkCompanionMessageCodec` 8.5–12.7 µs、
    `BenchmarkWorstLegalChunkSnapshot` Encode 366 µs / Decode 509 µs
    （compression-ratio 426.8、logical 196749 B、wire 461 B）、
    `BenchmarkTCPLoopbackPlayerInput` 3.75 ms、
    `BenchmarkTCPLoopbackChunkSnapshot` 0.98 ms（0.47 MB/s）。
  - `gofmt -l` 于 internal/network 与 internal/archcheck 无输出；消费方
    （internal/server、internal/client、internal/sim、cmd/）与 tcp 生产源码
    零改动（git status 实证，仅 internal/network/**、internal/archcheck/
    dependency_test.go 与本 change 产物变更）。


## Task 2 实现记录（protocol 子包）

- 实现 SHA：`38033d26`（`refactor(network): extract protocol message
  subpackage with alias re-exports`，12 个生产文件 `git mv` 至
  `internal/network/protocol/`，rename 检测全中）。
- 实现期裁决（用户裁决，2026-08-28）：设计临时导出表要求 companion/hostile
  的 12 个 wire encode/decode 函数在 Task 2 移入 protocol 并临时导出，但它们
  的签名全部携带 `*byteEncoder`/`*byteDecoder`（编解码原语被设计事实 5 钉死
  留守 codec 簇且保持 unexported），protocol 无法引用根包 unexported 原语，
  照字面执行无法编译。裁决：**wire 函数留守根包**——13 个函数（12 个 + 共用
  helper `decodeFixedID`）自 `message_companion.go`/`message_hostile.go` 提取
  至根包新文件 `companion_wire.go`/`hostile_wire.go`，保持 unexported、函数体
  逐语句不变（类型经 types.go 别名、常量经 protocol 导出名解析）；
  `codec_server.go` 调用点零改动。**Task 3 矫正**：3.1 的 wire 函数迁移改为
  「`git mv` `companion_wire.go`/`hostile_wire.go` 进 codec 并恢复包内直呼」，
  不再「自 protocol 归位」；终态与设计一致（codec 包内 unexported + 直呼）。
- 临时导出/新增导出清单（Task 3 核对回收或转正）：
  - 包 ID 访问器 6 个（设计既定转正）：`ClientPacketID`/`ClientPacketForID`/
    `ServerPacketID`/`ServerPacketForID`/`CommandRejectReasonID`/
    `CommandRejectReasonForID`（registry.go，PascalCase 直译）。
  - `ValidateDecodedClientWirePacket`（自 codec_client.go 归位 protocol/
    packet.go，Handshake/Login 放行逻辑原样，尾行 `validateClientWirePacket`
    直呼改为同义的 `ValidateClientPacket`；设计既定转正）。
  - companion wire 常量 8 个（设计既定转正，codec 预分配拒绝消费）：
    `ChatCommandMaxWireBytes`/`ChatCommandTextMaxBytes`/`ChatEventMaxWireBytes`/
    `CompanionSpawnMaxWireBytes`/`CompanionStateWireBytes`/`MaxCompanionStates`/
    `CompanionStatesMaxWireBytes`/`ChatSpeechTextMaxBytes`（末位为实施新增：
    根包 wire 文件与 Task 3 codec 均无 internal/companion 边，台词槽位上限
    随指令槽位上限一并经 protocol 导出，与 `validateSpeechText` 同源）。
  - hostile wire 常量 7 个（设计既定转正）：`MaxHostileRecords`/
    `HostileSpawnWireBytes`/`HostileStateWireBytes`/`HostileDespawnWireBytes`/
    `HostileSpawnMaxWireBytes`/`HostileStateMaxWireBytes`/
    `HostileDespawnMaxWireBytes`。
  - snapshot 位布局 helper 3 个（实施新增，chunk_codec.go 生产消费 + golden
    fixture 检视测试消费，预计随 codec 边转正）：`ValidBlockID`/`SectionWords`/
    `ReadSectionPacked`。
  - 固定错误构造器 2 个（实施新增，codec 分发点复用同一错误文本，预计转正）：
    `InvalidClientPacket`/`InvalidServerPacket`。
  - 值域上界 2 个（实施新增，仅根包测试消费，Task 3 测试随迁后可评估回收）：
    `MaxChunkBlockIndex`（drop_test.go）、`GridCraftingViewSlots`
    （codec_inventory_test.go）。
  - wire encode/decode 函数 12+1 个：**未导出**（按上述裁决留守根包
    `companion_wire.go`/`hostile_wire.go`），回收表该项以「Task 3 git mv 进
    codec」落定，无导出面。
- 根包 `types.go`：package doc（根包保留会话与传输编排 + 别名再导出保证）+
  迁出符号全量别名再导出（约 70 项：密封接口 4、类型 46、常量枚举 6 组、
  函数 var 别名 `ValidateClientPacket`/`ValidateServerPacket`）；逐别名带中文
  注释。根包 codec 簇与 memory/login/stream/transport 改 `protocol.` 限定
  调用；根包 9 个测试文件引用新导出符号处就地 `protocol.` 限定，测试函数名
  与 `t.Run` 标签逐名未动。
- archcheck：`"internal/network": {"internal/core", "internal/network/protocol"}`、
  `"internal/network/protocol": {"internal/companion", "internal/core"}`；根包
  internal/companion 边移除（wire 文件改经 protocol 常量取值后实测无该边）。
  范围说明：`internal/archcheck/baseline_test.go` 的 ProtocolVersion 权威来源
  路径随 `packet.go` 迁移同步更新（`internal/network/packet.go` →
  `internal/network/protocol/packet.go`），系 git mv 的机械后果，未改断言
  逻辑。
- 验证（同实现 SHA 实测）：`go test ./internal/network/... -race -count=1` →
  根包 ok 4.755s、tcp ok 6.130s（protocol 无测试文件）；`go test
  ./internal/archcheck -count=1` → ok；`go build ./...` → 通过；`go vet
  ./internal/network/...` → 清洁；`-list` 并集与基线 197 项逐名 diff 为空；
  附带 `go vet` 编译 9 个消费包（含测试）与 `gofmt -l` 全仓清洁；消费方与
  tcp 生产源码零改动（git status 实证）。


## 完成记录

- 2026-08-28：Task 1–4 全部完成，`tasks.md` 清单全勾。子树终态：
  `internal/network` 根包（types.go 别名门面 + stream/transport/memory/
  login + 会话测试与基准）、`protocol/`（12 生产文件 + 协议层测试）、
  `codec/`（9 生产文件含 companion_wire.go/hostile_wire.go + 编解码测试
  与 testdata）、`tcp/`（未动）。`-list` 并集终对照基线 197 项逐名一致；
  收尾门禁（archcheck、openspec strict、dev-check、test-race、bench
  预演）全绿，证据见 Task 4 节。待办：whole-branch review → 推送 →
  CI（微基准盲区留意）→ 合并 → openspec archive → 清理。
