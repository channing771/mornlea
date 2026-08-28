# Ledger：authoritative-bed-sleep

> 记录：控制会话裁决（ruling）、每 Task 评审结论与修复循环、最终验证输出摘要。

## 内容确认记录（brainstorming 硬门禁，2026-08-28）

- **分类**：architectural（新方块子系统 + 玩家 schema v8 + 世界 metadata v3 + 协议字段追加 + 显示相位语义扩展）。
- **探索**：backlog 行「床与睡眠」（原 `codex-implementer @ feat/A-05-authoritative-bed-sleep` 履历与共享契约 SHA `785ea07b` 均已丢失、本无实现损失）；现行主规格核对：`authoritative-daylight`（绝对时间 + metadata v2 + 客户端相位）、`authoritative-health`（死亡重生回出生锚点）、`internal/sim/door.go`（双格原子放置/交互/采掘先例）、`internal/sim/death.go`（`beginReset` 重生路径）、`internal/storage/metadata.go`（v2 定长布局 + v1 迁移先例）、`internal/storage/player_types.go`（v7 无重生点字段）、`internal/render/daylight.go`（`DayLengthTicks=24000`）。
- **Ruling: 并行互动（2026-08-28，用户裁决「解耦：无条件可睡」）** — 睡眠不检查夜行者；跳夜后白昼灼烧按夜行者既有规则自然结算；两行唯一共享契约为 `core.DisplayDayPhase(ticks, offset)`（A-04 交付、本行提供 offset 生产端）。为什么：契约面最小、两线真并行；靠近拒睡等耦合玩法留待后续行。
- **Ruling: A-05-approval（2026-08-28，approve）** — 按节呈现的设计（共享契约 / A-04 重定基线 / A-05 范围与配方 / 编排与门禁）经用户显式批准；床配方定为「顶排 3 小麦 + 下排 3 橡木木板 → 床 ×1」（麦秸床垫，材料可再生，与门 2×3 形状不冲突）。
- **Ruling: 合并序（2026-08-28）** — A-04 先合并（交付 `DisplayDayPhase` 与 S→C 22/23/24），本行 rebase 后合并；协议版本号由本行合并时基于届时 `main` 取下一空闲（A-04 取 v30 则本行 v31）；`bed-night` 场景插在 `torch-night` 之后、`ai-companion` 之前，与 A-04 的 `hostile-mob` 插入点互不冲突。

## 变更产物

- [x] `openspec/changes/authoritative-bed-sleep/`：proposal/5 delta specs/design/tasks/ledger 已建于本 worktree 功能分支。

## Task 1 基线验证（2026-08-28）

### 验证命令输出摘要（数值只记录）

- `git status --short`：worktree 干净（分支 `feat/A-05-authoritative-bed-sleep`，起始 HEAD `3e846534`）。
- `make rust`：通过（exit 0，release 构建 `mornlea_engine` 与 `mornlea_client`，约 1m 12s）。
- `go test ./internal/core ./internal/sim ./internal/storage ./internal/network ./internal/client ./internal/render ./internal/assets ./internal/mesh ./cmd/mornlea -race -count=1`：通过（exit 0，9 包全 ok）。耗时：`core` 2.155s、`sim` 68.709s、`storage` 35.083s、`network` 7.309s、`client` 8.544s、`render` 4.960s、`assets` 3.326s、`mesh` 36.844s、`cmd/mornlea` 337.818s。
- `openspec validate --all --strict --no-interactive`：通过（72 passed, 0 failed）。
- `git diff --check`：通过（无输出，exit 0）。

### 前置检查：`core.DisplayDayPhase` 状态

- `core.DisplayDayPhase` 当前在本分支不存在（`internal/`、`cmd/` 全量 grep 无匹配），与预期一致（夜行者行尚未合并交付）。
- 按本 change tasks 头部前置检查，记入**本行自带清单**：钉定签名 `DisplayDayPhase(worldTime uint64, offset uint16) uint16`；语义为先对 `worldTime` 做 `%24000`、再与 `offset` 相加后取模 24000；随实现交付边界测试；rebase 合并时与夜行者行交付的同一函数去重（保留一份）。

### 契约与编号核对（本行将取用的编号）

- `internal/core/block_properties.go`：`BlockEmission` 已存在（发光方块 15、火把五形态 14、其余 0）；`BlockLightAttenuation` 已存在（八个流体编号 1、其余 0）；`BlockOpaque` 不存在（按预期由夜行者行新增，rebase 后若已存在则直接消费、不重复定义）。
- 方块编号：`internal/core/block.go` 现方块段末为 `TorchWallNegZID` = 75，哨兵 `BlockIDMax` = 76。本行床 8 形态（床尾/床头 × 4 水平朝向）将取 76..83，`BlockIDMax` 顺延 8 至 84。
- 物品编号：`internal/core/item.go` 现物品段末为 `ItemTorch` = 44，哨兵 `ItemIDMax` = 45。本行 `ItemBed` 将取 45，`ItemIDMax` 顺延至 46。
- 配方编号：`internal/core/recipe.go` 现配方段末为 `RecipeTorch` = 15（`iota+1` 起始共 15 条，无哨兵常量）。本行 `RecipeBed` 将取 16。
- 世界 metadata：`internal/storage/metadata.go` 的 `currentMetadataVersion = 2`（符合预期）；本行将升至 3。
- 玩家存档 schema：`internal/storage/player_codec.go` 第 13 行 `currentPlayerSchema uint32 = 7`（v7 唯一定义点，`player_migration.go` 仅消费）；本行将升至 8。
- 协议版本：`internal/network/packet.go` 的 `ProtocolVersion uint32 = 29`（符合预期）；终值仍按合并序在合并期基于届时 `main` 重订。
- S→C 消息编号：`internal/network/registry.go` 的 `PlayerState` = 3（符合预期）；现 S→C 段末为 `CraftingState` = 21，夜行者行的 22..24 尚未占用。本行不新增 S→C 消息（`DayPhaseOffset` 尾部追加进 `PlayerState`）。

## 评审记录（Task 1 起，逐 Task 追加）

- （待逐 Task 填：SPEC 合规结论 / QUALITY 结论 / 修复轮 R1..Rn / 对应 Ruling）

- **Task 1（基线验证）**：完成，提交 `c710d542`。`make rust` 与 9 包 `-race` 全绿；编号核对：床 8 形态取 76..83（`BlockIDMax`→84）、`ItemBed`=45、`RecipeBed`=16、metadata v2、玩家 schema v7（`player_codec.go:13`）、协议 29、`PlayerState`=3。`DisplayDayPhase` 缺位→按并行裁决记入本行自带清单。控制会话抽查通过。
- **Task 2（床方块/物品/配方/碰撞/纹理/模型）**：实现提交 `f8e4d938`→评审 QUALITY FAIL（床面专属层 60..67 在 mesh 输出不可达：`emit_bed` 全 quad 读 face 0 材质而床面层只挂 FacePosY——生产链缺陷， SPEC PASS）→R1 修复（方案 A：平顶读 face 3、侧板各读自身面、非同质 Rust 夹具、生产链穿透测试 `TestBedSurfaceLayerReachesMesherThroughProductionRegistry` 红→绿、修正 face 序注释、ABI header 注释同步）→amend `dc43e746`→R1 复核 SPEC PASS + QUALITY PASS + 容量专项维持成立（80→96 有原注释/门火把同批先例/两侧同步双守卫/+16 步距四点依据，不属 ABI 版本契约）。修复轮：R1（1 轮，原实现者完成）。
- **Task 7（bed-night 场景/全线验证）**：实现提交 `083d4b38`（7.1+7.2 完成）。golden 口径核实：分支开工 21 张、新增 bed-night 后 22 张、既有 21 张逐字节未动，`visual-check` 22/22 全绿；`go test ./... -race` 26 包全绿（首例 server flake 系同机 benchmark 资源竞争，隔离复跑绿）。**重要根因修复（超出 7.1/7.2 字面）**：首次全链路渲染暴露 Task 2 漏检的呈现链缺陷——`terrain.wgsl` 角高度分流门按 material 集合判别、床面层 60..67 不在集合内，床顶 quad 被解码为 9×9 巨型石板且侧板材质判别原理性失效；修复为 `bed_material` 集合 + `shaders.rs` 常量 + 角 2 结构判别 fallback + Go/Rust 两侧三处钉子测试（火把先例同构），TDD 红→绿。**该缺陷说明床 quads 在本场景前从未经过完整渲染链路，Task 2 评审的双侧夹具均未覆盖 shader 分流——已列为 7.3 终审专项核对项。**
- **Task 6（协议/客户端相位/基线收口）**：实现提交 `45b4cdce`；评审 SPEC PASS + QUALITY PASS，三项自报裁决成立（AGENTS.md 单份基线与夜行者行同裁决；下发链系 6.2 可观察契约的逻辑必需且次序颠倒即红；README 矩阵遗留有三次升版先例均为归档 sync 处理）。v30 撞号已按纪律在 `packet.go` 注释自我声明（后合并者重订 v31）。非阻塞建议四条留归档期参考（v26 命名测试两处、两侧语义分界注释互引、README/README.en 矩阵+徽章归档 sync、合并期重订联动）。修复轮：0。
- **Task 5（玩家 schema v8 + metadata v3）**：实现提交 `9b4a0ff3`；评审 SPEC PASS + QUALITY PASS，三项自报裁决成立（respawn present=1 走 `validatePlayerLocation`/present=0 规范为零的确定性编码；`RestoreDayPhaseOffset` 装配期先于 worker 与首 tick 使「晚恢复丢 offset」不可达；版本字面量连锁 4 生产+53 测试全必要无残留）。archcheck 红经核实仅基线文档滞后（Task 6.3 分工内）。非阻塞建议三条留归档期参考（v1 CRC 例、metadata/协议两侧 offset 越界策略互相引用注释、v2/v1 尾随对称例）。修复轮：0。
- **Task 4（入睡/跳夜/重生点）**：实现提交 `451b0efb`；评审 SPEC PASS + QUALITY PASS，五项自报裁决全部核实成立（offset 基准取 tick 完成后绝对时间；「未验证 ≠ 失效」经 spec 措辞+Requirement 标题+禁停摆三重佐证成立并以 `TestDeathWithUnverifiedRespawnKeepsRecord` 钉定；`CommandInteractDoor` 先例确无 wire 映射、本任务零碰 network；`SetWorldTimeForTest` 无生产调用；敌怪对拍无可接线点、判夜入口已收敛 `Engine.displayDayPhase()`）。评审指出的两处产物-代码不一致（design D1 公式缺 +1 基准、delta spec 缺「区块未就绪」场景）已由控制会话修订（`c7f3d76a`），另修一处注释空格。修复轮：0。
- **Task 3（放置/采掘/支撑）**：实现提交 `5eb7b581`；评审 SPEC PASS + QUALITY PASS，三项自报裁决经核实全部成立（支撑扫除比照火把系 spec 明文要求且门无先例可抄；伙伴采掘床双清分支防半床残留；现行命令集确无命中床的 use 路径）。非阻塞建议四条留归档期参考（`clearBedPair` 防御纵深不对称、`bedHalfPositions` 第二份坐标拷贝、级联举例、火把触发床失效当 tick 覆盖缺直测）。修复轮：0。实现者裁决：支撑扫除比照火把先例（门无运行时扫除而 spec 明文要求整床移除掉落）、伙伴采掘床双清分支（避免通用单清残留半床）、右键交互留待 Task 4。

## 最终验证输出摘要（7.3 整分支终审补全，2026-08-28）

- `make rust`：通过（exit 0；release 增量构建 0.37s，`--locked`）。
- `go test ./... -race -count=1`（全量两次）：本分支交付包在两次全量中全绿（`sim` 73.7s/74.4s、`storage` 38.9s/34.0s、`network`、`core`、`render`、`client`、`assets`、`mesh`、`physics`、`fluid` 均 ok）。两次全量各出现 1..2 例**负载性 flake**，全部位于本分支 diff 之外的既有时序敏感测试：run1 `internal/server TestDropSurvivesShutdownAndRestart`（90.42s 重启后区块 Ready 超时）；run2 `internal/server TestDroppedItemSurvivesShutdownAndRestart`（90.56s 同类超时）+ `cmd/mornlea TestCraftingStateSizeThreeOpensWorkbenchUI`（0.00s UI 状态未达，包耗时 601.8s）。终审期间同机 load average 9..13（534 进程，多会话并行，与 Task 7 记录的同类资源竞争一致）。三例隔离复跑全绿（0.38s/2.49s/1.80s）；两个受影响包整包复跑全绿（`internal/server` 210.2s、213.3s；`cmd/mornlea` 288.2s）。裁决：环境性 flake，非本分支缺陷。
- `go test ./internal/archcheck -count=1`：ok 5.505s（`TestBaselineVersionsMatchCode` 等全绿）。
- `go vet ./...`：无输出，exit 0。
- `gofmt -l .`：无输出。
- `openspec validate --all --strict --no-interactive`：72 passed, 0 failed。
- `git diff --check`：无输出，exit 0。
- `make visual-check`：22/22 场景全绿，每场景「最大通道差 0，差异像素 0/230400」。

## 整分支终审（7.3，2026-08-28）

终审对象：全分支 diff `691a0b7e..28158229`（150 文件，+6221/−534）。终审员不修改生产代码；本节与 `tasks.md` 7.3 勾选为仅有的写动作。

### 专项一：shader 修复（Task 7 根因修复）

- **a) 闭合「床顶 9×9 巨型石板」缺陷：PASS**。`terrain.wgsl` 新增 `bed_material`（60..67 闭区间）接入角高度分流门；Rust 回归三件套直接锁缺陷：`bed_top_face_covers_only_its_own_cell`（9×9 石板回归的直接锁——有/无床两帧对拍，邻格 (9,8)/(8,9)/(9,9) 逐像素不变、床格自身必须变化防夹具空转）、`bed_top_edge_sinks_below_a_full_height_control`（9/16 下沉幅度 ≈28 行带宽 26..=30，锁 (8+1)/16 算式）、`corner_height_quads_route_regardless_of_material`（侧板处境复现：层 20 橡木板 + 四角 8 的 quad 不在短方块集合内也必须走角高度路径）。Go 侧 `TestBedSurfaceLayerReachesMesherThroughProductionRegistry` 穿透生产注册表验证顶面层可达 mesher，`TestNativeMeshBedQuadsRoundTripThroughGoUnpack` 逐条钉 5 条 quad 的面序/角高/材质。
- **b) fallback 不误吞既有语义：PASS**。新增析取 `corner_height(lo, hi, 2u) != 0u` 与 material 判别汇入**同一条**角高度分支（并集析取，命中哪条析取不改变解码）；耕地（29..30）/火把（59）仍走各自 material 门且其角 2 原值本就非零（耕地 14、火把薄板 8..13），行为零变化。结构判别的安全前提两侧同源强制：Rust `quad.rs pack` 普通 quad `high=0`（角 2 位恒 0，`corners==[0;4]` 走默认分支）+ 植物 material 反向断言；Go `UnpackQuad` 以「角 2 非零 ⟺ 角高度 quad」为信任边界（越界 panic），`TestQuadPackRoundTripCarriesCornerHeights` 对角 2 非零全组合穷举往返。普通 quad 不可能被误分流。
- **c) 两侧钉子同源防漂移：PASS（沿耕地/火把既定三方手工同步纪律）**。真值源为 Go 层枚举 `LayerBedFootSouth..LayerBedHeadEast`（60..67）；Go 钉子 `TestBedLayerNumbersMatchClientShaderContract` 硬编码 60/67 + 紧邻断言（`LayerTorch`=下界−1、`LayerCount`=上界+1，插层必撞）+ 八形态 PosY material 全落区间；Rust 钉子 `bed_range_constants_match_go_layer_enum` 硬编码同值（区间宽恰 7）+ `shader_sources_stay_pinned_to_the_range_constants` 扫 `terrain.wgsl` 字面量（`>= 60u`/`<= 67u`）与分流门接入门禁。与耕地先例同构：无共享生成定义、双侧失败报警点在注释中互相指名。

### 专项二：v30 撞号重订预案

- `packet.go` 顶部注释明写重订纪律（「版本号撞号纪律……合并序居后者基于届时 main 把自己的行重订为下一个空闲版本（wire 形状不受影响）。当前 v30 与并行夜行者行撞号，合并时按此纪律重订」），与 A-02 先例一致：**PASS**。
- 重订面清单（合并期把 v30→空闲版本时逐处更新；全部为精确匹配断言/字面量，漏改必红）：生产 `internal/network/packet.go`（const + 顶部版本注释）、`internal/network/message_player.go`（「协议 v30 起」注释）、`internal/network/codec_server.go`（「v30：」注释）、`AGENTS.md` 契约行（`TestBaselineVersionsMatchCode` 八条映射联动，漏改 archcheck 红）；测试 `internal/network/packet_test.go:87`、`worldtime_test.go:15`、`registry_test.go:72 与 :105`、`drop_test.go:140`、`codec_golden_test.go:22/67/68`（golden hex `1e`=30，重订为 `1f` 等随版本字节联动）、`cmd/mornlea-server/main_test.go:104 与 :334`、`cmd/mornlea/app_protocol_test.go:34`。wire 形状零变化，重订面完全可控。

### 终审清单 11 项

1. **规格合规：PASS** — 五份 delta 的每个 Requirement/Scenario 均有实现与测试对应（明细见第 2..9 项）；proposal 延期条款五项逐一核实无突破：无入睡多人姿态（wire 无 sleeping 字段）、床跨区块仍整单拒绝（无 B-21 事务）、判夜/跳夜零敌怪查询（全分支无 hostile 符号）、跳夜无直接清怪、`WorldTimeTicks` 语义未动（作物对拍锁）。
2. **放置原子性矩阵：PASS** — `TestTryPlaceBedWritesBothHalvesPerDirection`（4 朝向双写）；`TestTryPlaceBedRejectionsWithoutWrites` 六例（床尾/床头被占、两侧下方非实心、两半流体占据=RejectOccupied）+ 非法 dir 两例，全部断言零写入零 pending；`TestTryPlaceBedCrossChunkRejected` + `TestBedPlacementViaCommandCrossChunkKeepsItem`（跨区块整单拒绝不消耗）；`TestBedPlacementViaCommand`（恰好消耗 1）；床头写入失败回滚床尾在 `tryPlaceBed` 实现内（门先例同构）。
3. **跳夜全员语义：PASS** — `TestSleepThroughNightJumpsPhaseToDayStart`（offset=(24000−(t+1)%24000)%24000，本 tick 完成后相位恰 0，绝对时间恰 18001）；`TestSleepThroughNightWaitsForAllActivePlayers`（全员判定、单人未睡不跳不清）；`TestSleepThroughNightOverwritesPreviousOffset`（覆盖旧值且二次入睡按旧 offset 折算相位）；`TestSleepThroughNightKeepsCropPaceIdentical`（跳/不跳双世界逐位一致）；`TestSleepThroughNightPublishesOffsetInSameTick`（同 tick 下发）+ `TestPlayerStatePublishesDayPhaseOffset`（两会话同值）；清醒扫描覆盖非当期活跃名单防残留位污染。
4. **重生点校验矩阵：PASS** — `TestDeathRespawnsAtBedFootWhenBedIntact`（床尾中心 + 9/16 站高 + 满血满饥饿 + 记录保留）；`TestDeathFallsBackToAnchorWhenBedMined`（整床破坏回落清记录）；`TestDeathFallsBackWhenBedHalfMissing`（半破坏同判）；`TestDeathFallsBackWhenBedSupportSwept`（真实采掘支撑触发扫除后回落）；`TestDeathWithUnverifiedRespawnKeepsRecord`（区块未就绪回落锚点但保留记录，与 delta「未验证 ≠ 失效」逐字对齐）；`TestRespawnPointsIndependentAcrossPlayers`（两床互不影响，另一玩家仍回自己床尾）。
5. **迁移：PASS** — v7→v8：17B 定长尾（1+12+4）、present=0 规范为零的确定性编码、冻结 fixture `testdata/player-v8.bin`（297B）`TestPlayerV8Fixture`、`TestPlayerV7FixtureMigratesToNoRespawn` + `TestPlayerV7MigrationClearsRespawn`、fuzz 不 panic；v2→v3：`TestMetadataV3GoldenBytes`/`TestMetadataV2LegacyGoldenBytes` 字节级 golden、`TestMetadataV2MigratesToV3WithZeroOffset`、`TestMetadataV1MigratesToV3WithZeroTimeAndOffset`、`TestOpenDiskMigratesV2MetadataFile`（**打开不落盘改写**：磁盘字节读回断言 v2 未动、保存后才升 v3）、`TestMetadataOffsetSurvivesRestart`；损坏矩阵 `TestMetadataVersionsRejectMalformedBytes`/`TestPlayerCodecRejectsCorruptEnvelope` 等沿用既有错误面。
6. **wire：PASS** — `PlayerState` 尾部 u16 落位 `SaturationZero` 与 `WorldTimeTicks` 之间（payload 尾字节断言 + 截断/尾随全拒）；`TestProtocolV30PlayerStateRejectsOutOfRangeDayPhaseOffset` 三处拒绝（Validate/编码/解码改字节）；两侧语义分界在 `message_player.go`/`server.go` 注释互指：wire 严格拒 >23999、metadata 装配 `%24000` 宽容归一；`TestProtocolV30PlayerStateCarriesDayPhaseOffset` + Memory/TCP transcript 一致性既有全量；v30 pin 连锁无漏改（`TestBaselineVersionsMatchCode` + 上述 pin 清单全绿）。
7. **客户端相位：PASS** — `DayNightAt(worldTime, offset)` 唯一算式经 `core.DisplayDayPhase`（客户端不自建，云漂移仍走绝对时间）；`app_celestial_test` 锁 offset 随下一份权威状态切换、旧/重复状态不回退（与 worldTimeTicks 同一 ServerTick 接受纪律）。
8. **`DisplayDayPhase` 双交付去重预案：PASS** — 签名与 tasks.md 头部钉定逐字一致 `(worldTime uint64, offset uint16) uint16`；语义「先 `%24000` 再相加取模」由 4 条边界测试锁（纯模对拍、MaxUint64 不溢出、23999+1 回绕 0、夜间窗 13000..23000 含两端）；`day_phase.go` 与测试文件均带去重标记（「rebase 合并时与夜行者行的同一函数去重，只保留一份」）。
9. **呈现：PASS** — `bed-night` 插 `torch-night` 后、`ai-companion` 前（`TestBedNightCaptureScenePosition` + 尾序不变量）；四朝向全摆（超出 spec 两种下限）且 `TestBedNightScenePixelsShowMultiOrientationBedsAtNight` 以床头亮带像素探针逐床验证（含石面对照扣照明）；夹具恢复直测（后续场景床/火把格回空气）；golden 逐字节零差（22/22 全 0 差异像素）；床纹理全程序化原创橡木配色，无版权资源。
10. **注释纪律：PASS** — 全 diff 新增注释 1207 条，非中文者 39 条全为分隔线/公式/跨行标识符片段（唯一含英文叙述者为 DayNightAt 的 sun 公式行，正当）；新增行无 `[A-F]-[0-9]{2}` 任务编号（全部命中均在 openspec 规划产物内，属允许区）；标识符反引号与意图式注释抽查合格。
11. **并行行边界：PASS** — 全分支无 `BlockOpaque`/夜行者/敌怪符号，`DisplayDayPhase` 为自带交付（含独立边界测试），不依赖 A-04 未合并代码；文件交集仅 `internal/core` 追加段（bed/day_phase 新文件）、`AGENTS.md` 基线行与（如夜行者行改）`registry.go` 常量段，append-only，撞号预案见专项二。

### 终审结论

**PASS（可进 PR）**。两项专项与 11 项清单全绿，无 FAIL 项。合并期待办（非缺陷，预案已备）：① 协议版本按届时 `main` 重订（专项二清单）；② `DisplayDayPhase` 与夜行者行去重保留一份；③ `bed-night` golden 基于届时基线口径顺延。验证数值见上节；两次全量 `-race` 的 3 例 flake 均已隔离与整包复跑证伪为同机高负载（load 9..13）环境因素，涉及测试均在本分支 diff 之外。
