# Design：床与睡眠

## Context

当前 `main`（分支基线 `fe3890ed`，2026-08-28 起草时核对）与本设计相关的约束（均已实测核对）：

- 显示相位目前完全由绝对时间决定：`internal/render/daylight.go` 以 `DayLengthTicks=24000` 对 `WorldTimeTicks` 取模；`authoritative-daylight` 主规格规定「客户端 MUST 以最新有效权威玩家状态中的绝对时间决定昼夜相位」。`PlayerState` 已携带绝对时间（协议 v29，尾部追加字段先例：`SaturationZero` 1 bool）。
- 引擎侧 `internal/sim/engine.go` 的 `worldTime atomic.Uint64` 由 metadata 恢复、每 tick 恰好 +1；`metadata.go` 的 `currentMetadataVersion=2`，payload 定长布局（`WorldTimeTicks` 在 `payload[20:28]`），v1→v2 有迁移先例。
- 双格方块先例：`internal/sim/door.go` 的 `tryPlaceDoor`（lower/upper 同区块、空气可替换、下方 `isSolidSupport`、跨区块未就绪整单拒绝、经 `pending` 原子双写）、`handleInteractDoor`/`executeInteractDoor`（右键交互路径）、采掘双清掉 1。
- 重生：`internal/sim/death.go` 经 `beginReset` 把玩家送回出生锚点；玩家 record（`internal/storage/player_types.go`）现含 `Dimension`/`Position`/`Yaw`/`Pitch`/`Health`/三层饥饿等，无重生点字段。
- 配方：`internal/core/recipe.go` 现有 15 条（`RecipeTorch` 居末），匹配「裁边 + 水平镜像位」；小麦与橡木木板均为既有可再生材料。
- 方块/物品编号：火把五形态 71..75 居方块段末（`BlockIDMax` 哨兵居末），`ItemTorch`=44 居物品段末；实现期以常量为准（撞号按 A-02 先例由后合并者重订）。
- 与并行行 A-04（夜行者）的唯一共享契约：`core.DisplayDayPhase(worldTime uint64, offset uint16) uint16`（A-04 交付；本行提供 offset 生产端）。视觉场景表：A-04 在 `ai-companion` 与 `water-surface-slope` 之间插入 `hostile-mob`；本行在 `torch-night` 之后插入 `bed-night`，两插入点互不相邻冲突，顺序不变量（`far-horizon` 倒数第二、`water-underwater` 唯一末）共同保持。

## Goals / Non-Goals

**Goals**
- 夜间入睡 → 全员到齐跳夜 → 个人重生点的最小闭环；跳夜与重生点持久化、可重放、失败原子。
- 与 A-04 真并行：除 `DisplayDayPhase(ticks, offset)` 外零共享文件语义冲突。

**Non-Goals（设计层）**
- 不做入睡的多人姿态呈现、不做靠近敌怪拒睡、不做跳夜对夜行者的直接清理（见 proposal 延期与放弃）。
- 不改绝对时间语义：作物生长、流体推进、掉落寿命继续由 `WorldTimeTicks` 驱动，跳夜不影响其节奏。

## Decisions

### D1：offset 是「显示相位偏移」，不是时间回拨

`DayPhaseOffset`（uint16，0..23999）只进入显示相位计算 `(worldTime + offset) % 24000`；`WorldTimeTicks` 的推进、持久化与全部既有消费者不变。跳夜设置 offset = `(24000 − worldTime % 24000) % 24000`，使相位立即落在周期起点（白昼）。再次入睡重新计算覆盖旧值。

**否决**：直接把 `WorldTimeTicks` 向前加（破坏「每 tick 恰好 +1」契约、错误加速作物/流体与掉落寿命）；引入负向时间（回放与确定性全部破坏）。

### D2：跳夜判定与状态机在 sim，最小状态

引擎持有 `dayPhaseOffset atomic.Uint64`（值域钳 0..23999）与每玩家 `sleeping` 布尔（复用既有 per-player 状态槽）。右键床命令（复用既有 use/interact 命令路径，门先例）在夜间相位设置 sleeping + 重生点；每 tick 在固定阶段检查「全部活跃玩家 sleeping」→ 设置 offset、清除全部 sleeping（含日志 cue 缺省不加）。移动输入或受击清除该玩家 sleeping。

**否决**：入睡倒计时/进度条（无玩家可见价值）；把跳夜做成可配置百分比多数决（backlog 范围是「全员睡眠跳夜」）。

### D3：重生点为「床尾格 + 有效性延迟校验」

玩家 record v8 追加 `respawnPresent bool + respawnPosition[3]f32 + respawnDimension`（u32）。入睡写床尾格；死亡重生时校验「两格仍为同属一床的床方块」才使用，否则回落出生锚点并清 present 位。床被采掘/支撑清除后两格不再是床，校验自然失败，无需事件式清除。

**否决**：床破坏事件反向通知玩家记录（引入方块变更→玩家状态的额外耦合面）；只存「床的存在性哈希」等间接编码（debug 与存档可读性差）。

### D4：协议走 `PlayerState` 尾部追加

`DayPhaseOffset` u16 尾部追加进 `PlayerState`（v29 `SaturationZero` 先例：字段追加、旧字段不动、解码拒绝越界值 >23999）；客户端 `render/daylight.go` 的相位计算改为消费最新权威状态中的 offset。协议版本号在本行合并时基于届时 `main` 取下一空闲版本，两份基线文档版本行同步。

**否决**：新增独立 S→C 消息（一条每 tick 必发的状态就能承载，避免消息数量膨胀与订阅问题）；经 ServerTick 消息承载（该消息无此字段先例且非所有传输路径同构）。

### D5：metadata v3 追加 offset；玩家 schema v8 追加重生点

metadata v3 = v2 payload 末尾追加 `DayPhaseOffset` u64（定长布局追加，`currentMetadataVersion=3`，v1/v2 迁移为 0）；玩家 schema v8 = v7 尾部追加 1+12+4 字节（present + position + dimension），v7 迁移为 present=0。两处迁移均为「旧档可读、读入即升级、行为不变」既有先例模式。

### D6：床方块与呈现

8 个 `BlockID`（床尾/床头 × 4 朝向，编号接方块段末）；半高 9/16 碰撞体经 `physics.BlockCollisionBoxes` 注册；mesh registry 追加 model tag 与程序化纹理层（容量内追加，不升 engine ABI——门先例）；`LayerBed` 程序化纹理（原创橡木配色，不引入任何版权资源）。夜间入睡无姿态呈现（非目标）。

### D7：与 A-04 的并行纪律

本行消费 `core.DisplayDayPhase`（A-04 交付；若本行先合并则该函数尚未存在——**合并序定为 A-04 先**，本行 rebase 后基于其接口实现；若集成期顺序颠倒，本行按同签名自带并声明与 A-04 收敛）。除此之外两行文件交集仅 `internal/core` 追加段与 `internal/network/registry.go` 常量段，append-only，后合并者 rebase 重订编号。

## Risks / Trade-offs

- [合并序依赖 A-04 先行] → 本行 tasks 把「核对 `DisplayDayPhase` 已存在」列为 Task 1 前置检查；顺序颠倒时按 D7 同签名收敛。
- [跳夜多人等待期玩家状态分歧] → sleeping 为权威单值、随 `PlayerState` 心跳隐式收敛；等待期只表现为「相位未变」，无半跳状态。
- [offset 与绝对时间双轨的理解成本] → spec 与注释统一措辞「offset 只影响显示相位」；`DisplayDayPhase` 是全仓唯一消费点（客户端 render 与 sim 判夜均经它）。
- [床碰撞体与既有 raycast/物理的边界] → 复用门「custom 碰撞 + 双格分流」的全部测试模式；9/16 高度夹具化测试锁定。

## Migration Plan

- metadata v2→v3、玩家 v7→v8 均为尾部追加迁移：旧档首次读入即升级、offset 0 / present 0 与现行行为逐字节一致；无导出/备份格式变更（backup 全树复制既有纪律不动）。
- 回退：PR 合并前任意 step 可撤销；合并后回退需按版本互斥纪律另立 change（offset 语义可整体退化为常量 0）。

## Open Questions

- 无（解耦裁决与配方裁决已于 2026-08-28 brainstorming 批准；实现期发现规格不成立先改本 change 产物再改码）。
