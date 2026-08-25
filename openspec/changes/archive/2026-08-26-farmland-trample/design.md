# Design: farmland-trample

## 数据所有权与依赖方向

- 全部改动收敛在 `internal/sim`：踩踏是权威模拟的世界交互知识，与耕地干湿转换、采掘掉落同域。不新增包，不改依赖白名单；`sim` 不接触渲染与网络。
- 事件暂存是 `Engine` 的瞬态字段（同 `cropCellScratch` 先例）：落地边沿发生在物理阶段，方块写入发生在区块写入区，跨阶段承载只能是 engine 字段；不持久化、不进快照/哈希，每 tick 结算后清空，重启无残留语义。包级变量不可行：测试会并行构造多个 `Engine`，包级暂存引入数据竞争。
- 掉落数量决策完全复用 `cropYieldRolls`（`crop.go`），不新增哈希流；掉落容量预演与提交复用 `PrepareDropBatch`/`CommitDropBatch`（破坏熔炉/箱子与成熟小麦采掘的既有原子模式）。

## 关键决策

### D1: 事件收集与结算两段分离（阶段顺序契约的强制）

`Step` 的阶段顺序契约要求一切区块写者位于 `reconcileSubscriptions` 之后（`engine_step.go` 的契约注释：订阅收缩会立即删除干净区块的 record，写在它之前的写者会在 `finishChanges` 取到 nil record 而崩溃）。而落地边沿在 `advanceActivePlayers`（物理阶段，位于 reconcile 之前）。因此踩踏必须拆成两段，跨阶段载体是 `Engine` 上的暂存切片：

1. **收集**：`player.go` 落地边沿（`!wasOnGround` 判定内、`applyFallDamage` 调用旁）插入一行 `engine.noteTrampleLanding(session, player)`——控制会话裁决许可的 `player.go` 最小受控重叠的全部内容。几何判定与暂存追加在 `trample.go` 内：枚举玩家碰撞盒水平覆盖（至多 2×2 列）的支撑层格 `floor(Y − ε)`，把 `(dimensionID, pos)` 追加进 `engine.tramplePending`。ε 取小正容差：站满格方块时脚底 Y 是整数格顶、站耕地时 Y 是格底 + 15/16，两者都必须落到正确的支撑格；实现时核对 `physics` 侧探针容差（`GroundProbe`）先例选值并写注释论证。
2. **结算**：`crop.go` 的 `advanceCrops` 首部插入 `engine.settleTramples(pending)`——位于 reconcile 之后的区块写入区、与耕地干湿转换同域共用同一份 pending（「生长与干湿转换要与其他方块变更共用同一批 revision、广播与存盘」对踩踏逐字成立），且 `engine_step.go`（A-01/A-04 双独占）零触碰。

结算早于同函数的随机 tick 抽样：本 tick 被踩成泥土的格不再是耕地，抽样天然跳过，语义自然。

**三处最小受控重叠清单**（均为 append-only 单行、不改既有内容，同构于控制会话对 `player.go` 的裁决；`Ruling` 见 ledger）：

| 文件 | 改动 | 为什么不可避免 |
|---|---|---|
| `internal/sim/player.go` | 落地边沿一行调用 | 落地边沿检测只存在于这里（摔落伤害同源） |
| `internal/sim/crop.go` | `advanceCrops` 首部一行调用 | 区块写入区内唯一语义同域且无在途独占的挂点 |
| `internal/sim/engine.go` | `Engine` 结构体追加一行暂存字段声明 | 跨阶段载体必须是 engine 字段（多引擎并发测试排除包级变量；Go 不允许跨文件拆结构体） |

### D2: 结算语义与原子性

对暂存序列（按收集序 = `advanceActivePlayers` 的 SessionID 升序，确定性）逐格结算：

1. 读该格方块；不是 `FarmlandDryID`/`FarmlandWetID` 则跳过（多玩家覆盖同格时天然幂等：首笔结算后变泥土，后续读到非耕地跳过，结果与次序无关）。
2. 读正上方格；若是作物（`core.IsCrop`），按采掘同形规则准备掉落：成熟小麦（`WheatStage7ID`）走 `cropYieldRolls(engine.seed, engine.tick.Load(), dimensionID, pos)` 双产物（tick 取值点与 `completeMining` 同一读取路径，同输入重放一致）；未成熟作物按采掘路径的既有形状（1 颗种子）。耕地转泥土是方块转换而非破坏，不掉落任何物品。
3. 若上方是作物：`PrepareDropBatch` 预演；容量不足（false）则**整格放弃**——耕地与作物保持原样，与采掘路径 `RejectDropCapacity`「任一堆放不下就整体返回 false，绝不半掉落」逐字同构。踩踏不是玩家命令、没有拒绝通道，放弃即可观察（耕地保持）且无信息丢失。
4. `SetBlock(耕地格, DirtID)` 成功后 `SetBlock(作物格, AirID)`、两次 `recordChange`、`CommitDropBatch`。两格是同区块同列竖向相邻，第二个 `SetBlock` 的失败路径按 `advanceCropCell` 先例处理（枚举范围与写入范围分歧才会走到，丢弃剩余写入比广播一条没落地的变更安全）——此时耕地已变泥土、作物留在原地，是既有不变式允许的状态（同采掘耕地的现状），不是数据丢失。

### D3: 边沿语义与重触发

判定挂在 `!wasOnGround → OnGround` 的转换沿上：持续站立不重复触发；跳起再落产生新边沿即重新判定。容量不足而放弃的格，须等玩家下一次落地边沿（跳一下）才有重试机会——这是「落地冲击」语义的自然读法，不是缺陷。

### D4: 客户端与协议零变更

耕地/作物/泥土的格变更经 `recordChange` 汇入本 tick 的 `pendingChunkChanges`，走既有方块变更广播；掉落经既有 drop 快照与拾取路径。客户端镜像无需任何改动即可呈现踩踏结果。capture 场景与 benchmark scenario 均无「落地踩踏耕地」的构造，golden 与数值不受影响（Task 2 核对）。

## 被否决的替代方案

| 方案 | 否决理由 |
|---|---|
| 在 `advanceActivePlayers` 内直接写方块 | 违反阶段顺序契约（区块写者必须位于 `reconcileSubscriptions` 之后），订阅收缩窗口下会在 `finishChanges` 崩溃 |
| 轮询式：`advanceCrops` 抽样时检查格上方是否站着玩家 | 踩踏延迟被随机抽样随机化（可能永远抽不到）；且把「与玩家数量相关」的成本带进「与作物数量无关」的作物阶段成本契约 |
| 只判玩家中心柱落脚格 | 跨格站立（碰撞盒横跨两格）漏判半边，MC 语义是碰撞盒水平相交即踩踏 |
| 上方作物不连带处理（只转耕地） | 踩踏是高频移动事件，麦田穿行会留下成片「作物站在泥土上」的悬空残留；连带掉落才是「毁田」的完整语义 |
| 伙伴一并实现 | companion 物理路径无现成落地边沿（无摔落伤害语义），新建边沿检测超出认领独占集与最小闭环；先例 B-13 冲刺半边延期 |
| 踩踏掉落走独立哈希流 | 破坏「同一株同一 tick 踩掉与挖掉数量相同」的确定性域统一；复用 `cropYieldRolls` 零新增哈希知识 |
| 暂存放包级变量 | 多 `Engine` 实例并发测试（`-race`）下是数据竞争；`cropCellScratch` 挂 engine 的先例正是为此 |

## 受影响文件

| 文件 | 变更 |
|---|---|
| 新增 `internal/sim/trample.go` | `noteTrampleLanding`（几何判定与暂存）、`settleTramples`（结算）与中文注释 |
| 新增 `internal/sim/trample_test.go` | 踩踏行为主题测试（变泥土、作物掉落、容量不足、边沿语义、跨格） |
| 新增 `internal/sim/property_trample_yield_parity_test.go` | 同格同 tick 踩踏与采掘掉落数量逐件相同的被证性质 |
| `internal/sim/player.go` | 落地边沿插入一行收集调用（三处最小受控重叠之一） |
| `internal/sim/crop.go` | `advanceCrops` 首部插入一行结算调用（之二） |
| `internal/sim/engine.go` | `Engine` 结构体追加一行暂存字段声明与注释（之三） |

## 风险与回退

- **`engine.go` 的字段追加与 A-04 在途独占的关系**：A-04 的清单为「修改 `engine.go`/`engine_step.go`/…（hostile 接线）」。本 change 在 `engine.go` 的改动严格限制为结构体内 append-only 一行字段（不改任何既有行），与 `player.go` 重叠裁决同构；合并序冲突（A-04 同期大改 `engine.go`）由集成裁决处理，git 层面两者不修改同一行即可无冲突合流。
- **回退方案**：本 change 无存档/协议足迹，revert 即完全回退；摘除三行挂钩（`player.go`/`crop.go` 调用与 `engine.go` 字段）即回到现状。
- **验证方法**：Task 1 主题测试与性质测试；Task 2 受影响包回归与 capture/benchmark 不触及核对；Task 3 全量门禁。
