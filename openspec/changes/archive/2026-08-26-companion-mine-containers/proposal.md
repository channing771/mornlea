# Proposal: companion-mine-containers

## Why

M5C 交付伙伴 `mine` 时把容器（箱子/熔炉）与多掉落方块显式挡在防御清单外（`docs/superpowers/specs/2026-08-13-ai-native-companions-design.md`：「容器可能包含多组物品，与『产物直接进入伙伴背包』的原子容量语义不同，因此明确拒绝，留给后续单独设计」）。功能积压表 C-01 要求解除这一限制：玩家可以指挥伙伴清空并回收箱子/熔炉，而不是必须亲自处理每一个容器。现状 `completeCompanionMining` 只会结算「单一产物 1 件」，容器批量结算的原子容量语义（全或无）已在内容确认阶段由需求方裁决为方案 A。

## What Changes

- 伙伴 `mine` 的合法目标集合扩展：箱子与熔炉成为合法目标；其余普通方块仍要求具有单一 `BlockDrop`；农业十编号（八个作物阶段 + 干湿耕地）的显式拒绝**保持不变**（其语义属 C-11，另行裁决）。
- 容器采掘的批量结算：完成 tick 在伙伴背包副本上预演「容器本体 1 堆 + 全部内容物堆」（箱子最多 1+27 堆（`core.ChestSlots`）、熔炉 1+3 堆），任一堆放不下则该 tick 整体不结算——方块不变、容器内容物不变、耐久不变、背包不变、进度保持满格（全或无，方案 A）。
- 预演通过时同一权威 tick 内原子完成：目标方块改空气、停用对应容器槽（`DeactivateChest`/`DeactivateFurnace`，对齐玩家路径）、按既有规则扣工具耐久、背包提交产物。
- Task Runner 的满格饱和判定同步扩展：观察到进度满格且方块仍在时，用与 sim 同一的批量预演判定容量，不能容纳即以既有 `TaskFailInventoryFull` 失败（wire 枚举零变更）。
- Planner 提示词与计划契约校验（`planMineableBlock`）放开容器；观察快照与契约的其余约束不动。
- 满容器内容物堆数（最多 28 堆）可能超过伙伴背包余量是方案 A 的固有结果：实践中伙伴只能回收空/半空容器（同类物品并堆后所需格数 ≤ 28）。

### 用户可观察结果

- `@伙伴名 挖 <箱子坐标>` 对空箱子或内容物可并堆放进背包余量的箱子成功：箱子方块消失，箱子本体与全部内容物出现在伙伴背包，工具耐久扣减。
- 箱子内容物放不下时任务失败（`TaskFailInventoryFull`），箱子连同内容物原样保留，工具耐久不扣。
- 熔炉同构：本体 + 输入/燃料/输出三格一并入背包。
- 农业方块仍不可作为伙伴采掘目标（行为与现状一致）。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `companion-world-actions`: 「伙伴采掘复用玩家计时规则且三方原子」条目修改——容器从双重拒绝清单移入合法目标集合，新增容器批量「全或无」结算契约与容量不足整体不结算的行为；农业拒绝升格为显式 Scenario 锁定。

## Impact

- **代码**：`internal/sim/mining.go`（`companionMineableBlock` 放开容器；`completeCompanionMining` 容器批量分支与容器槽停用）、`internal/companion/plan_types.go`（`planMineableBlock`）、`internal/companion/planner.go`（提示词约束文案）、`internal/server/companion_interact.go`（Runner 饱和分支批量预演）、同包新增测试文件。
- **兼容性**：无协议、存档、区块 schema、`companions.ai` schema、物品/方块编号或 `core.BlockDrop` 表形状变更；`TaskFailInventoryFull` 为既有 v18 wire 枚举，零升版；已存档世界无需迁移，容器采掘是即时结算不入档。
- **性能**：完成 tick 一次容器批量预演为内存 `AddStack` 逐堆操作（≤28 堆 × O(36) 格），无分配风暴、无热路径新预算；benchmark scenario v19 不变。
- **并行边界**：不触碰 A-01/A-04 已认领的 `internal/core`（`recipe.go`/`inventory.go`/`item.go`）、`engine*.go`/`drop.go`/`command.go`/`tunables.go`；不触碰 B-13 的 `combat.go`/`hunger.go`、B-31 的 `eating.go`、B-05/B-07 的 `crop.go` 与新建踩踏/冲毁文件；不触碰 A-02/A-04 的 `internal/mesh` 与 engine/client crate。
- **测试**：新增 sim 批量原子性与预演失败四方不变测试、companion 计划侧放开测试、server Runner 批量预演与 Memory/TCP parity 测试、农业拒绝回归测试。

## 延期与放弃

- **泛化的「多掉落方块」判据**：刻意不引入编号层面的多掉落泛化判据（成熟小麦的第二份产物在编号层面读不出，`companionMineableBlock` 注释已论证「巧合性安全不成立」）；本 change 的多掉落目标集合显式枚举为箱子与熔炉，未来新多掉落方块逐个裁决后进清单。
- **部分结算/掉落世界**：内容物放不下时不做「装得下的入背包、其余掉落世界」或玩家路径的批量掉落——伙伴没有世界掉落物拾取能力（C-02 未交付），掉世界等于销毁内容物；由方案 A 裁决排除。
- **农业十编号放开**：属 C-11（种什么/何时收/成熟度判断语义），本 change 显式不裁决。
- **`internal/sim/mining.go` 中伙伴容器分支与玩家容器路径的代码合并**：两路径结算去向不同（玩家 → 世界掉落物批量预演 `PrepareDropBatch`；伙伴 → 背包副本逐堆 `AddStack`），刻意保持两条独立分支避免用参数开关扭曲任一侧语义；待未来出现第三条同类路径再考虑共享抽象。
