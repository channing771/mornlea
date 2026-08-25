# Design: companion-mine-containers

## D1 结算形状：方案 A 全或无（已裁决）

内容确认阶段（2026-08-25）需求方在「全或无」与「对齐玩家先例」之间裁决为**方案 A**：

- 容器采掘的产物集合 = 容器本体 1 堆 + 全部内容物堆（箱子 ≤ 1+27 堆（`core.ChestSlots`，63 是容器 UI 的玩家背包+箱格总格数、非存储格数），熔炉 ≤ 1+3 堆；`harvestable` 为假时本体不计，与玩家路径一致）。
- 预演：在伙伴背包**副本**上按固定序（本体在前，内容物按容器槽位序）逐堆 `AddStack`，任一堆 `leftover != 0` 即整体放弃——方块、容器内容物、工具耐久、背包、采掘进度（满格）全部不变。
- 提交：预演通过后在同一权威 tick 内 `SetBlock(air)` + `DeactivateChest`/`DeactivateFurnace` + 背包替换为副本 + `consumeToolDurability`，随后 `recordChange` 汇入既有 `pendingChunkChanges` 广播，不新增协议消息。

固定序的意义：`AddStack` 的并堆结果与插入顺序相关（先到堆优先合并），固定「本体在前、槽位序」使同一世界状态的重放逐字节一致，与全仓确定性纪律（无 map 遍历序依赖）对齐。

满容器 28 堆可能超过背包余量是方案 A 的固有结果：装不下即失败，实践中伙伴回收空/半空容器。这是裁决接受的权衡，不是缺陷。

## D2 防御清单：显式枚举，不泛化

`companionMineableBlock`（模拟侧）与 `planMineableBlock`（计划侧）的修改形状：

- 删除「箱子/熔炉一律拒绝」分支，改为：容器两类走新的容器合法分支；其余方块仍要求单一 `core.BlockDrop`；农业十编号的显式拒绝分支**原样保留**（C-11 未裁决）。
- 刻意**不**引入「多掉落方块」的编号层泛化判据：成熟小麦的第二份产物只存在于权威结算分支、编号层面读不出（既有注释已论证「巧合性安全不成立」）。本 change 的多掉落目标集合 = 显式枚举 {箱子, 熔炉}，未来新多掉落方块逐个裁决后追加。
- `core.BlockDrop` 表形状零变更：多产物知识只存在于权威结算路径（对齐 `crop-random-drop-count` design.md Ruling 5 的既有裁决，避免波及全部消费者）。

两侧清单必须同步放开：Planner 契约拒绝而模拟放行（或反之）都会让同一计划在两处判定漂移。`companionMineableBlock` 的 GoDoc 注释同步改写，删除「超出单一产物直入背包的结算形状」的过时理由，改述为容器批量的全或无语义。

## D3 Runner 饱和判定与 sim 同一预演

`holdCompanionMining` 的满格饱和分支现状用「与 sim 同一的 `AddStack` 单件预演」判定容量。容器放开后该分支同步扩为与 `completeCompanionMining` 完全相同的批量预演（同一产物集合构造、同一固定序、同一背包副本逐堆 `AddStack`）——「没有第二套规则」原则从单件推广到批量。判定为假容量即 `FailRun(TaskFailInventoryFull)`（既有 v18 wire 枚举，零升版）。

实现上预演逻辑放 sim 侧导出（如 `sim.CompanionMineYield` 风格的纯函数：给定方块编号、容器内容物快照与背包，返回产物堆序列与预演结果），Runner 调用同一函数，避免在 server 包重抄一份堆构造。函数签名与落位在 Task 内按最小实现收敛。

## D4 容器槽停用与同 tick 性

玩家路径先例（`completeMining` 的 Chest/Furnace 分支）：读取容器记录 → 批量预演 → `SetBlock` → `Deactivate` → 提交。伙伴路径复用同一顺序；容器记录的读取经 chunk record（`ChestAt`/`Chest`/`FurnaceAt`/`Furnace`），与玩家路径同源，不新增第二套容器访问。区块失效或方块已被同 tick 更早 actor 移除时对齐既有 `RejectNoTarget` 语义：清零进度、不结算、无容器槽泄漏。

## D5 并发边界与数据所有权

- 全部结算发生在权威 tick 单写者内（`completeCompanionMining` 既有模型），无新 goroutine、无锁、无跨 tick 持有的中间状态。
- 背包副本预演不共享：副本在栈上构造，提交前不写回 `entry.inventory`。
- `recordChange` 产出的广播消息切片沿用既有不可变约定。
- Runner（server 包）只读 sim 的公开观察信号与镜像容器内容物，不反向写 sim 状态。

## D6 兼容性

无协议（`TaskFailInventoryFull` 复用 v18 枚举）、无存档（容器采掘即时结算，`companions.ai` schema v4 不变）、无编号（物品/方块注册表零变更）、无 engine/client ABI、benchmark scenario v19 不变。已存档世界无需迁移。回退 = revert 本 change 单个提交序列。

## D7 被否决的替代方案

1. **部分结算掉落世界（对齐玩家 `PrepareDropBatch` 先例）**——否决：伙伴没有世界掉落物拾取能力（C-02 未交付），装不下的内容物掉世界等于事实销毁；且破坏「产物直入背包」的既有任务语义。
2. **编号层多掉落泛化判据（改 `core.BlockDrop` 为多产物表）**——否决：波及该表全部消费者（放置词表交叉校验、客户端镜像、防御清单），收益只有当前两类容器；维持 Ruling 5 的既有裁决。
3. **Runner 侧重抄堆构造以避免跨包导出**——否决：制造第二套产物集合构造，两侧漂移只是时间问题；D3 的共享函数是更小的耦合。
4. **顺手合并伙伴/玩家两条容器结算分支为参数化单实现**——否决：两者去向不同（世界掉落 vs 背包直入），参数开关会扭曲两侧语义；出现第三条同类路径前不做抽象（见 proposal「延期与放弃」）。

## D8 受影响文件

| 文件 | 变更 |
|---|---|
| `internal/sim/mining.go` | `companionMineableBlock` 放开容器并改写注释；`completeCompanionMining` 容器批量分支（含容器槽停用）；预演纯函数导出（D3） |
| `internal/companion/plan_types.go` | `planMineableBlock` 同步放开容器，注释改写 |
| `internal/companion/planner.go` | 提示词 mine 约束文案放开箱子/熔炉 |
| `internal/server/companion_interact.go` | `holdCompanionMining` 饱和分支改调 sim 导出的同一批量预演 |
| 同包新增测试文件 | sim 批量原子性/预演失败四方不变/熔炉；companion 计划侧放开；server Runner 饱和判定与 parity；农业拒绝回归 |

## D9 验证方法

- 定点：`go test ./internal/sim ./internal/companion ./internal/server -race -count=1`
- 全量门禁（收尾）：`gofmt -l .` 无输出、`go vet ./...`、`go test ./... -race`、`go test ./internal/archcheck -count=1`、`openspec validate --all --strict --no-interactive`。
- 不触及渲染/tick 热路径形状（采掘完成 tick 本就在既有预算内）、无 capture golden 影响；`internal/network` 零文件变更故无 codec fuzz 影响。
