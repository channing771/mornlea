# 床与睡眠

## Why

当前 `main` 的夜晚只能干等或暂停：没有任何跳过夜晚的手段，死亡后也固定回到世界出生锚点。床与睡眠补上这条生存闭环：夜间入睡、全员到齐跳夜、个人重生点，让多人协作（`maxPlayers` ≤8）有明确的「过夜」节奏。本行与并行行（可放置夜行者）经 `core.DisplayDayPhase` 的 offset 窄接口解耦，二者可独立验证与合并。

## What Changes

- **床方块（8 形态）**：新增床尾/床头 × 4 水平朝向共 8 个 `BlockID`（接在现有方块段末、`BlockIDMax` 哨兵之前，实现期核实当前顶值），`ItemBed` 物品（堆叠 64，可放置）；原创橡木配色程序化纹理层与 mesh model tag（容量内追加，不升 engine ABI，门先例）。
- **同区块原子放置**：复用门先例——床尾格与其朝向侧的床头格必须同处已加载区块、均为空气且各自下方有实心支撑（既有 `isSolidSupport` 判据），任一不满足整单拒绝且不消耗物品；成功放置消耗 1 个 `ItemBed`。
- **采掘与回收**：采掘床头或床尾任一半（任意手持，与木板同价）同时清空两格并恰好掉落 1 个 `ItemBed`；支撑失效与门先例一致处理；流体把床两格视为占据格（与门一致），不新增流体破坏规则。
- **碰撞与选取**：床为半高（9/16）碰撞体，不可穿越；`raycast` 命中与指针选取覆盖床头床尾两格。
- **入睡与跳夜**：对床右键在显示相位 `13000..23000`（与夜行者生成窗口同一夜间定义，读 `core.DisplayDayPhase(ticks, offset)`）时入睡并记录该床为个人重生点；当全部活跃玩家同时处于入睡状态时，服务端在 tick 边界设置 `DayPhaseOffset` 使显示相位跳到周期起点（白昼），全部入睡状态清除。白天使用床被拒绝（`CommandRejected`，不消耗）。入睡玩家发出移动输入或受到伤害则取消其入睡状态；跳夜不检查夜行者（解耦裁决），跳夜后露天夜行者按其既有白昼灼烧规则自然结算。
- **个人重生点**：入睡即把重生点设为床尾格；死亡重生时若重生点两格仍为有效床则回到床尾格，否则回落世界出生锚点并清除失效重生点。玩家 schema v7→**v8** 追加重生点字段（旧档迁移为「无重生点」= 现行世界锚点语义，无行为变化）。
- **偏移持久化**：`DayPhaseOffset` 入世界 metadata v2→**v3**（旧档迁移为 offset 0，行为不变）；权威 `WorldTimeTicks` 推进语义完全不变（offset 只影响显示相位，不影响作物/流体等以绝对时间驱动的模拟）。
- **协议（升版，终值按合并序）**：`PlayerState` 尾部追加 `DayPhaseOffset`（u16，0..23999，v29 的 `SaturationZero` 尾部追加先例）；客户端显示相位改读 `(WorldTimeTicks + DayPhaseOffset) % 24000`。协议版本号在本行合并时基于届时 `main` 取下一空闲版本（A-04 先合并则 v31，否则 v30），版本互斥行纪律遵循 backlog 规则。
- **配方**：`RecipeBed`（编号接在现有配方段末，实现期核实）：3×3 网格顶排 3 个小麦、下排 3 个橡木木板，产物床 ×1，镜像位按「形状自身镜像等价」声明（3×2 形状与门 2×3 不冲突）。
- **视觉场景**：新增 `bed-night` 无窗口 capture 场景（固定夜间卧室内多朝向床形态呈现），位于 `torch-night` 之后、`ai-companion` 之前；自带 golden（合并时基于届时基线口径顺延一位），不创建或聚焦前台窗口。

## Capabilities

### New Capabilities

- `authoritative-bed-sleep`：床的放置/采掘/碰撞/选取、入睡与全员跳夜、个人重生点及其持久化。

### Modified Capabilities

- `authoritative-daylight`：显示相位引入 `DayPhaseOffset`（权威时间推进与自动保存语义不变）；世界时间与偏移经 metadata v3 持久化，v2 世界迁移为 offset 0。
- `authoritative-health`：死亡重生目标从固定出生锚点改为「有效个人重生点，否则出生锚点」。
- `authoritative-grid-crafting`：新增床配方（顶排 3 小麦 + 下排 3 橡木木板 → 床 ×1）。
- `visual-verification`：新增 `bed-night` 场景与 golden。

## Impact

- **受影响的包**：`internal/core`（方块/物品/配方/`DisplayDayPhase` 复用）、`internal/sim`（放置/采掘/入睡/跳夜/重生点/引擎 offset 状态）、`internal/assets`/`internal/mesh`（纹理层与 model tag）、`internal/storage`（玩家 schema v8、metadata v3）、`internal/network`（`PlayerState` 追加字段与协议版本）、`internal/client`/`internal/render`（offset 消费与显示相位）、`cmd/mornlea`（场景构造）；engine 侧无 ABI 变更（mesh registry 容量内追加，门先例）。
- **存档**：玩家 schema v7→v8（旧档迁移无行为变化）；世界 metadata v2→v3（旧档迁移 offset 0）；区块 schema 不动。
- **协议**：`PlayerState` 尾部追加 1 个 u16 字段并升协议版本（终值合并时锁定）；旧客户端握手拒绝语义随版本行维护。
- **并发与性能**：入睡/跳夜/重生点全部走既有命令与 tick 边界路径，无新 goroutine、无热路径新增有界外工作；`DayPhaseOffset` 为引擎内单值原子读。
- **验证**：`make rust`；受影响包定点 `-race`；`go test ./... -race` 合并前全量；`go test ./internal/archcheck -count=1`；`openspec validate --all --strict --no-interactive`；`visual-check` 全表比对全绿；`TestBaselineVersionsMatchCode` 兜底版本行。

## 延期与放弃

- 不做睡觉时的多人呈现（其他客户端看不到入睡姿态）、不做起床闹钟/雷暴、不做床爆炸（下界兼容性玩法不存在）。
- 不做床跨区块原子事务（B-21）：跨区块整单拒绝，出现第二个消费者再设计。
- 不做「靠近敌怪拒睡」（解耦裁决）：床与夜行者只共享显示相位契约。
- 不做跳夜对夜行者的直接清理：跳夜后由其既有灼烧/消失规则自然结算。
- 不做除显示相位外的时间加速：绝对 `WorldTimeTicks` 语义不变，作物/流体/掉落寿命不受跳夜影响。
