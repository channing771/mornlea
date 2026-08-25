# Design：权威近战夜行者

## Context

参见 `proposal.md` 的 Why。当前 `main`（工作树基线 `16ac2fe7`）现状与本设计相关的约束（均已实测核对）：

- `sim` 的权威 tick 已具备活动兴趣范围、既有阶段顺序（`advanceActivePlayers` → `advanceActiveCompanions` → `advancePlayerMelee` → `advanceDrops` → `advanceFluids` → `advanceCrops` → `finishChanges`），每实体固定顺序。
- **不存在物理批处理 API**：`physics.Step(state, input, source)` 是 per-actor 积分，玩家与伙伴都逐个调用（Rust 出口），夜行者沿用同一模式，不新建批处理接口。
- 寻路复用 `internal/companion`（`PathWindowHorizontalRadius=16`/`PathWindowVerticalRadius=4` 即 33×9×33、`MaxPathNodes=4096`、固定邻居展开序、`ChunkRevision` 重验、`ShouldUse` 策略与 `ReplanAfter`）；服务端已有 `companion_manager.go` 的 worker/generation/latest-wins 先例（其 A* 在 worker goroutine 跑、权威 tick 只发快照）。
- 方块发光与衰减表目前在 `internal/assets/blocks.go`（`Registry.Emission`，light block 15、其余 0）与 `internal/mesh`（`LightAttenuation`，流体 1、其余 0）；两者都不可被 `internal/sim` 导入（archcheck：`sim → {companion, core, fluid, physics, world}`）。新规则落在 `internal/core`，`assets`/`mesh` 改为委托（裁决 A-04-q2 路径 A）。
- `core` 无 `Materials`/`MaterialAirID`/`BlockEmission`；`splitmix64` 在 `internal/sim/crop.go`；`WorldTimeTicks` 是 `sim.Engine.worldTime`（atomic.Uint64），无任何偏移；客户端显示相位在 `internal/render/daylight.go`（`DayLengthTicks=24000`，`WorldTimeTicks % 24000`）。
- avatar 容量上限当前为 `internal/render/avatar.go` 的 `maxAvatars=11`（66 实例）且为**每帧 GPU 缓冲上限**，与伙伴数量无关；nametag 上限 12 且每名 32 runes。Rust 侧 `mornlea_client` 的 `AVATAR_MAX_INSTANCES=66`、`ENTITY_INSTANCE_BYTES=80`、GO 侧 `client ABI=v8`。
- 协议 v26；存档：玩家 schema v7、metadata v2、区块 schema v9、`companions.ai` v4；`internal/storage` 已有字节原语（`appendU32`/`byteDecoder` 等）与 CRC-32C 逐 codec 的实现先例。
- 批次流程裁决（控制会话 A-02-q1/A-02-q2 及 A-04-q1/q2 确认）：本分支自包含实现，推分支开 PR 但**不合并**（待集成，A-06 按固定顺序合流）；版本号（协议 v27、`hostile_mobs` v1 行、engine ABI v7、benchmark scenario v20）与 golden、`AGENTS.md`/`CLAUDE.md`/`progress.md` 全文基线归 A-07；本分支只做 client ABI v8→v9 的实际升版与两份基线该版本行的最小同步。

## Goals / Non-Goals

**Goals**
- 服务端权威的夜行者事实平面（生成/移动/攻击/灼烧/消失/掉落/持久化/网络同步/呈现），全部数量与预算有界、确定性可重放、失败原子。
- 为批次集成提供与 A-01/A-02/A-03/A-05 可合流的窄接口：`core` 发光/衰减单一表、`HostileMobID`、三类消息、`hostile_mobs.bin`。

**Non-Goals（设计层）**
- 不做统一 combat settlement（A-03 契约、A-06 接通）；本分支攻击走既有 `applyDamage` 入口 + 专属损伤测试 seam。
- 不做批次共享契约冻结提交（A-02 承担重建；本分支若其落地先行则 rebase 并对齐）。
- 不做 golden、协议版本升版、benchmark 基线（A-07/A-08）。
- 不做 ECS/通用 mob registry/第二类 mob/远程攻击/难度（见 proposal 延期与放弃）。

## Decisions

### D1：`sim` 持有排序 slice，server 持有 worker 与持久化，storage 持有 codec

数据所有权与现有 companion 骨架完全同构：

- `sim`：`hostileState`（cap 64 排序 slice、按 ID 二分插入/查找；+ 预分配 29³ BFS scratch + 2 份路径快照请求缓冲）；`Engine.Step` 新增 hostile 阶段（置于 `advanceActiveCompanions` 之后、`advancePlayerMelee` 之前：先结算夜行者近战意图再走玩家近战，便于同 tick 一致性；实际插入点将在实现期按 `engine_step.go` 既有 phase 常量扩展）。spawn/burn/despawn/掉落全在 sim。
- `server`：`hostileManager`（每 tick 至多 2 份快照、两槽 worker channel cap 2、`applyHostileOutcome` 在 tick 边界按 ID 序应用、generation/latest-wins）、`hostilePublication`（按会话订阅）、`hostilePersistence`（jobs/completions cap 1、revision、dirty/inFlight/retry、autosave、Flush/Close——形状复制 `companion_persistence.go`，不抽通用 generic）。
- `storage`：`hostile_codec.go`/`hostile_store.go` 新增接口 `LoadHostileMobs`/`SaveHostileMobs`，`WorldStore` 组合；文件 `hostile_mobs.bin`。
- `core`：`HostileMobID` uint64、`DisplayDayPhase`、发光/衰减表、`ItemRottenFlesh` 与食物行。

**否决**：把夜行者状态放进 `internal/world`（world 不该持有实体），或抽通用 persistence/实体框架（只有一个消费者，提前抽象）。

### D2：发光/衰减单一表迁入 `internal/core`（裁决 A-04-q2 路径 A）

`core` 新增 `BlockEmission(block core.BlockID) uint8` 与 `BlockLightAttenuation(block core.BlockID) uint8`（当前值：light block 15 / 其余 0；流体 1 / 其余 0；torch 14 属 A-02 追加段），`internal/assets` 与 `internal/mesh` 改为委托 core。若 A-02 契约提交先行落地（其 `core.BlockEmission` 已存在且语义一致），则本分支直接消费其表，不重复创建；实现期以「函数已存在且签名/值一致」为判据在 tasks 中留检测步骤。

**否决**：把表留在 assets/mesh 而 sim 复制同值表（双源漂移）；等待 A-02 阻塞（本分支需独立可验证）。

### D3：局部区块光用预分配 29³ 16-bucket BFS

规则：半径 14、初始值=发射值（发光方块 15、火把 14 属 A-02 段）、每格每步衰减 = 1 + `BlockLightAttenuation`（流体额外 1）、opaque 阻挡（有碰撞盒且不透明——按既有 `BlockCollisionBoxes`/透明度语义）、unknown/unloaded 按阻挡；取候选中心值为 ≤7 判定。`Engine` 持 29³ 的 light/visited/bucket scratch，每次调用零分配；不保存跨 tick 缓存。与客户端既有静态方块光传播的**位守恒差异必须在设计评审时以 oracle 测试钉住**（Rust 侧方块光实现作为 oracle；若存在逐位差异，以 sim 侧为权威并记录）。

**风险**：与客户端传播语义发生漂移 → mitigation：oracle 对照测试 + 设计评审记录差异来源。

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
- 攻击结算：sim 的 `advanceHostileMelee` 先冻结全部意图，再按 ID 升序经 `applyDamage` 结算 3 点 + 攻击冷却 20；本次的损伤测试 seam 为包内测试专用（`hostile_combat_test.go` 通过 sim 内部 `damageHostileTarget` 之类的 test-only 通道验证数值），A-06 接入统一 combat settlement 时删除。

### D6：网络与呈现

- 消息：现注册表 S→C 21/22/23 预留（A-03 战斗确认/其他批次消息的编号互斥由 A-06 统一分配；本分支本地使用预留值，备注中声明「编号终值 A-06 锁」）。
- publication 复用 `queueReadyAndResync` 的订阅判定与 `publishSession` 的每会话路径；新增 hostile 段（spawn/state/despawn 各每 tick 至多一包）。
- client：`internal/client/hostiles.go` latest-wins 镜像；插值复用远端时间边界；frame 组装在 avatar 段追加夜行者记录。
- Rust：`AVATAR_MAX_INSTANCES` 66→450、`maxAvatars` 11→75；新增 `EntityHostile`（Go 侧 kind 3）；head/body 比例 6 cuboids 不变；调色暗青/灰紫；nametag 集合不加入 hostile。
- client ABI `MORNLEA_CLIENT_ABI_VERSION 8→9`（`mornlea_client.h` 与 Go 侧校验同步）；两份基线文档仅 client ABI 版本行同步（A-04-q1 裁决）。

### D7：持久化与备份

- 头 32 字节（magic `MHST` + envelope1 + schema1 + revision u64 + count u16 + payloadLen u32 + CRC-32C）；记录 72 字节固定（ID/状态/生命/冷却/…）；CRC 范围按 companion_codec 先例（header 段+payload 或 payload-only——实现期以 `companionChecksum` 的惯例为准并写 golden 测试）。
- 路径 `hostile_mobs.bin`；`replaceFileAtomicallyWithPatternAndHooks` 复用（temp+fsync+rename+目录 fsync、0600）；backup 自动复制（新增文件走既有全树复制，忽略 `*.tmp-*`）。
- 损坏/未来版本：`Host` 构造失败（启动即拒），不得以空集合覆盖。

### D8：批量基线（用于 Task 1 的证明）

`make rust` + `go test ./internal/core ./internal/companion ./internal/physics ./internal/sim ./internal/server ./internal/storage ./internal/network ./internal/client ./internal/render ./internal/nativeabi ./cmd/mornlea -race -count=1` 作为 Task 1 基证据点；数值只记录。

## Risks / Trade-offs

- [A-02/A-04 并行改 `internal/core` 的 item/发光段] → append-only 不同段；若 A-02 契约先行落地则 rebase + 对齐（D2 检测步骤）；合并冲突由 A-06 按固定合流序裁决。
- [本分支新增 3 类 S→C 消息但协议版本不变] → 分支外 main 不受影响（PR 不合并）；v27 由 A-07 一次性升，旧客户端握手拒绝语义由集成任务维护。
- [局部光与客户端传播漂移] → oracle 对照测试 + 差异记录（D3）。
- [capture 场景新增但分支无 golden] → 场景表顺序/夹具不变量测试通过；golden 生成由 A-07 统一执行；`compareAgainstGolden` 对缺失 golden 的容错路径在实现 Task 7 时按「分支模式跳过缺失 golden 的逐图比较、顺序与不变量测试照常」落地，并在此 change 的 tasks 中写清。
- [per-actor physics 与玩家/伙伴混批] → 保持一致顺序与合并（玩家→伙伴→夜行者），无新批处理接口（D5）。
- [夜晚生成与玩家小规模世界] → 候选条件严格；测试用固定种子与夹具世界全覆盖。

## Migration Plan

- 无旧档迁移：`hostile_mobs.bin` 缺失=空集合；未来版本拒载不覆盖。
- 回退：PR 不合并（等待集成）；分支内任何 step 可撤销，不影响 main。
- 兼容：协议 v26 不变（消息追加、值域校验已含）；v27 行由 A-07 升级。

## Open Questions

- 无（所有影响 spec/设计/tasks 的决策已在设计评审裁决；实现期若发现规格不成立，先更新本 change 产物再改码）。
