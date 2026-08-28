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
- 本 change 尚无实现期裁决；Task 2–4 的实施裁决、临时导出清单与回收记录
  随各任务评审追加于本节之后。

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
