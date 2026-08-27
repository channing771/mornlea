# Design：权威近战夜行者

## Context

参见 `proposal.md` 的 Why。当前 `main`（工作树基线 `fe3890ed`，2026-08-28 重定基线时核对）现状与本设计相关的约束（均已实测核对）：

- `sim` 的权威 tick 已具备活动兴趣范围、既有阶段顺序（`advanceActivePlayers` → `advanceActiveCompanions` → `advancePlayerMelee` → `advanceDrops` → `advanceFluids` → `advanceCrops` → `finishChanges`），每实体固定顺序。
- **不存在物理批处理 API**：`physics.Step(state, input, source)` 是 per-actor 积分，玩家与伙伴都逐个调用（Rust 出口），夜行者沿用同一模式，不新建批处理接口。
- 寻路复用 `internal/companion`（`PathWindowHorizontalRadius=16`/`PathWindowVerticalRadius=4` 即 33×9×33、`MaxPathNodes=4096`、固定邻居展开序、`ChunkRevision` 重验、`ShouldUse` 策略与 `ReplanAfter`）；服务端已有 `companion_manager.go` 的 worker/generation/latest-wins 先例（其 A* 在 worker goroutine 跑、权威 tick 只发快照）。
- 方块发光与衰减表目前在 `internal/assets/blocks.go`（`Registry.Emission`，light block 15、其余 0）与 `internal/mesh`（`LightAttenuation`，流体 1、其余 0）；两者都不可被 `internal/sim` 导入（archcheck：`sim → {companion, core, fluid, physics, world}`）。新规则落在 `internal/core`，`assets`/`mesh` 改为委托（裁决 A-04-q2 路径 A）。
- `core` 无 `Materials`/`MaterialAirID`/`BlockEmission`；`splitmix64` 在 `internal/sim/crop.go`；`WorldTimeTicks` 是 `sim.Engine.worldTime`（atomic.Uint64），无任何偏移；客户端显示相位在 `internal/render/daylight.go`（`DayLengthTicks=24000`，`WorldTimeTicks % 24000`）。
- avatar 容量上限当前为 `internal/render/avatar.go` 的 `maxAvatars=11`（66 实例）且为**每帧 GPU 缓冲上限**，与伙伴数量无关；nametag 上限 12 且每名 32 runes。Rust 侧 `mornlea_client` 的 `AVATAR_MAX_INSTANCES=66`、`ENTITY_INSTANCE_BYTES=80`、GO 侧 `client ABI=v9`。
- 协议 v29；存档：玩家 schema v7、metadata v2、区块 schema v9、`companions.ai` v4；`internal/storage` 已有字节原语（`appendU32`/`byteDecoder` 等）与 CRC-32C 逐 codec 的实现先例。S→C 已实占 0..21（21 为 `CraftingState`），空闲自 22 起。
- 流程裁决（原批次 A-04-q1/q2 批准 + 2026-08-28 控制会话重定基线）：批次合流模型已取消（原 A-06/A-07 集成职责已拆回各功能行）；本行自包含实现、PR 直接合并；协议 v29→v30、client ABI v9→v10、`hostile_mobs` v1、golden（21→22 张口径）与两份基线文档版本行由本行自带同步；benchmark scenario 不动；与并行行 A-05 的共享契约仅为 `core.DisplayDayPhase(ticks, offset)`（本行交付并消费，offset 恒 0）。

## Goals / Non-Goals

**Goals**
- 服务端权威的夜行者事实平面（生成/移动/攻击/灼烧/消失/掉落/持久化/网络同步/呈现），全部数量与预算有界、确定性可重放、失败原子。
- 为批次集成提供与 A-01/A-02/A-03/A-05 可合流的窄接口：`core` 发光/衰减单一表、`HostileMobID`、三类消息、`hostile_mobs.bin`。

**Non-Goals（设计层）**
- 不做统一 combat settlement（A-03 契约，其落地后收编）；本分支攻击走既有 `applyDamage` 入口 + 专属损伤测试 seam。
- 不做批次共享契约冻结提交（A-02 承担重建；本分支若其落地先行则 rebase 并对齐）。
- 不动 benchmark scenario 与存档 schema；协议 v30、client ABI v10 与 golden 由本行自带（见流程裁决）。
- 不做 ECS/通用 mob registry/第二类 mob/远程攻击/难度（见 proposal 延期与放弃）。

## Decisions

### D1：`sim` 持有排序 slice，server 持有 worker 与持久化，storage 持有 codec

数据所有权与现有 companion 骨架完全同构：

- `sim`：`hostileState`（cap 64 排序 slice、按 ID 二分插入/查找；+ 预分配 29³ BFS scratch + 2 份路径快照请求缓冲）；`Engine.Step` 新增 hostile 阶段（置于 `advanceActiveCompanions` 之后、`advancePlayerMelee` 之前：先结算夜行者近战意图再走玩家近战，便于同 tick 一致性；实际插入点将在实现期按 `engine_step.go` 既有 phase 常量扩展）。spawn/burn/despawn/掉落全在 sim。
- `server`：`hostileManager`（每 tick 至多 2 份快照、两槽 worker channel cap 2、`applyHostileOutcome` 在 tick 边界按 ID 序应用、generation/latest-wins）、`hostilePublication`（按会话订阅）、`hostilePersistence`（jobs/completions cap 1、revision、dirty/inFlight/retry、autosave、Flush/Close——形状复制 `companion_persistence.go`，不抽通用 generic）。
- `storage`：`hostile_codec.go`/`hostile_store.go` 新增接口 `LoadHostileMobs`/`SaveHostileMobs`，`WorldStore` 组合；文件 `hostile_mobs.bin`。
- `core`：`HostileMobID` uint64、`DisplayDayPhase`、发光/衰减表、`ItemRottenFlesh` 与食物行。

**否决**：把夜行者状态放进 `internal/world`（world 不该持有实体），或抽通用 persistence/实体框架（只有一个消费者，提前抽象）。

### D2：发光/衰减单一表（裁决 A-04-q2 路径 A，重定基线后为「直接消费」路径）

`core.BlockEmission`（发光方块 15、火把 14）与 `core.BlockLightAttenuation`（流体 1、其余 0）已由现行 `internal/core/block_properties.go` 提供，本分支**直接消费、不重复创建**；只新增 `core.BlockOpaque(block core.BlockID) bool`（不透明谓词与既有 `assets.Registry.Opaque` 一致——registered 且非 air/glass/leaves/fluid/作物），`internal/assets` 与 `internal/mesh` 的不透明判定改为委托 core（发光/衰减两表的既有委托关系核对保持即可）。

**否决**：把表留在 assets/mesh 而 sim 复制同值表（双源漂移）；等待 A-02 阻塞（本分支需独立可验证）；以碰撞盒近似 opacity（glass/leaves 在客户端光传播中透光，碰撞盒近似会造成伺服不一致）。

### D3：局部区块光用预分配 29³ 16-bucket BFS

规则：半径 14、初始值=发射值（`core.BlockEmission`：发光方块 15、火把 14）、每格每步衰减 = 1 + `BlockLightAttenuation`（流体额外 1）、opaque 阻挡（单源谓词 `core.BlockOpaque`，与客户端光传播同一语义）、unknown/unloaded 按阻挡；取候选中心值为 ≤7 判定。`Engine` 持 29³ 的 light/visited/bucket scratch，每次调用零分配；不保存跨 tick 缓存。传播规则 MUST 与客户端/Rust 方块光 oracle 在固定小夹具上逐位一致；若实现中发现任何真实差异，记录差异来源并裁决（不得静默采用两套不一致规则）。

**风险**：与客户端传播语义发生漂移 → mitigation：oracle 对照测试 + 差异必须记录并裁决。

**否决**：服务端全世界光照缓存（内存与 tick 成本）；把判定委托给客户端（服务端不能读客户端）。

### D4：spawn 派生严格确定性

- 锚点：`activeSessions`（已排序）中第 `WorldTimeTicks % n` 个玩家的位置。
- 每 tick 恰好一个候选：`splitmix64(uint64(seed) ^ WorldTimeTicks)` 产出半径（24..48）与轴向；候选坐标 = 锚点 + 轴向量；`hash= splitmix64(splitmix64(uint64(seed)^tick) ^ uint32(X) ^ uint32(Z))` 再混 `uint32(Y)` 的传播；仅当 `hash&0xFF < 13` 时尝试（13/256 概率）。
- ID = 同一 hash 非零结果；冲突时最多重散列 64 次，仍冲突本 tick 放弃。
- 所有 hash 输入为整数、无浮点；重放一致由「相同 seed+tick+玩家集合 → 相同候选」测试锁定。
- 生成次序：tick 边界先于 physics（新 spawn 下一 tick 才移动）。

### D5：追逐与攻击

- 目标选择在 server manager（按会话镜像的 active 玩家），每 tick 为 ID 最小且到期的至多两只构造快照。
- waypoint 执行在 sim（经 `HostileActions` 轴量）——即 sim 消费 server 输出的 `HostileAction{MoveX,MoveZ,Jump,AttackTarget}`；移动经既有 per-actor `physics.Step`（顺序玩家→伙伴→夜行者 ID 序）；到 1.8 内停移 + 冻结攻击意图。
- 攻击结算：sim 的 `advanceHostileMelee` 先冻结全部意图，再按 ID 升序经 `applyDamage` 结算 3 点 + 攻击冷却 20；本次的损伤测试 seam 为包内测试专用（`hostile_combat_test.go` 通过 sim 内部 `damageHostileTarget` 之类的 test-only 通道验证数值），A-03 统一战斗落地收编时删除。
- 快照请求投递：权威 tick 端向两槽 channel 的发送 MUST 为非阻塞投递（select）；channel 满时该夜行者本次顺延、下一 tick 排入重规划（per-ID latest-wins 语义），MUST NOT 阻塞权威 tick 等待 A*。

### D6：网络与呈现

- 消息：S→C 22/23/24（21 已被 `CraftingState` 占用；实现期以注册表实占空闲位为准，与并行行撞号时由后合并者重订）。
- publication 复用 `queueReadyAndResync` 的订阅判定与 `publishSession` 的每会话路径；新增 hostile 段（spawn/state/despawn 各每 tick 至多一包）。
- client：`internal/client/hostiles.go` latest-wins 镜像；插值复用远端时间边界；frame 组装在 avatar 段追加夜行者记录。
- Rust：`AVATAR_MAX_INSTANCES` 66→450、`maxAvatars` 11→75；新增 `EntityHostile`（Go 侧 kind 3）；head/body 比例 6 cuboids 不变；调色暗青/灰紫；nametag 集合不加入 hostile。
- client ABI `MORNLEA_CLIENT_ABI_VERSION 9→10`（`mornlea_client.h` 与 Go 侧校验同步）；两份基线文档同步协议 v30、client ABI v10、`hostile_mobs` v1 三处版本行（A-04-q1 裁决 + 重定基线扩展）。

### D7：持久化与备份

- 头 32 字节 = magic(4) + envelope u32(4) + schema u32(4) + revision u64(8) + count u32(4) + payloadLen u32(4) + CRC-32C(4)（与 `companion_codec.go` 的 `companionHeaderLength`=32 先例一致）；记录 72 字节固定（ID u64/dimension u32/State position+velocity 各 12/onGround 1/yaw f32/health·attack·hurt·burn 各 1/hasTarget 1/PlayerID 16/NextRepath u64/Distant u16 —— 恰好无填充）；CRC-32C 覆盖范围钉死为 `data[8:28]`（schema..revision..count..payloadLen 段）+ payload 段（同 `companionChecksum` 惯例）。world Y 越界判据：record 的 position.Y 不在 `[core.MinY, core.MaxY)`（即 `-64..319`）。
- 路径 `hostile_mobs.bin`；`replaceFileAtomicallyWithPatternAndHooks` 复用（temp+fsync+rename+目录 fsync、0600，临时文件名按既有带前导点约定 `.hostile_mobs.bin.tmp-*`）；backup 自动复制（全树复制，忽略 `.*.tmp-*`）。
- 损坏/未来版本：`Host` 构造失败（启动即拒），不得以空集合覆盖。

### D8：批量基线（用于 Task 1 的证明）

`make rust` + `go test ./internal/core ./internal/companion ./internal/physics ./internal/sim ./internal/server ./internal/storage ./internal/network ./internal/client ./internal/render ./internal/nativeabi ./cmd/mornlea -race -count=1` 作为 Task 1 基证据点；数值只记录。

## Risks / Trade-offs

- [A-04/A-05 并行改 `internal/core` 追加段与 `registry.go` 追加段] → append-only 不同段；共享契约仅为 `DisplayDayPhase(ticks, offset)`；后合并者 rebase 重订编号（A-02 撞号重订先例）。
- [新增 3 类 S→C 消息并升协议 v30] → 旧客户端握手拒绝语义由本行在 v30 维护；`TestBaselineVersionsMatchCode` 兜底版本行。
- [局部光与客户端传播漂移] → oracle 对照测试 + 差异记录（D3）。
- [capture 场景与 golden 同行交付] → 场景生成 golden PNG（torch-night 先例：固定种子收敛后导出），`compareAgainstGolden` 全绿后入库；后续场景顺序插入由对应行自行维护区间约束。
- [per-actor physics 与玩家/伙伴混批] → 保持一致顺序与合并（玩家→伙伴→夜行者），无新批处理接口（D5）。
- [夜晚生成与玩家小规模世界] → 候选条件严格；测试用固定种子与夹具世界全覆盖。

## Migration Plan

- 无旧档迁移：`hostile_mobs.bin` 缺失=空集合；未来版本拒载不覆盖。
- 回退：PR 不合并（等待集成）；分支内任何 step 可撤销，不影响 main。
- 兼容：协议 v29→v30 由本行升级（三类消息追加 + 值域校验随版）；`hostile_mobs.bin` v1 为新增文件、无迁移。

## Open Questions

- 无（所有影响 spec/设计/tasks 的决策已在设计评审裁决；实现期若发现规格不成立，先更新本 change 产物再改码）。
