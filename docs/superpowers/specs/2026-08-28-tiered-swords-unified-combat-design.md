# A-03 三级剑与统一战斗设计

## 状态

- 日期：2026-08-28
- backlog：A-03
- 设计分支：`docs/A-03-combat-design`
- 修订基线：A-04、A-05 与客户端/网络分包均已合入后的 `main`
- 内容确认：用户已批准任务选择、玩法数值、协议与反馈边界，以及 A-04 合入后的兼容优先统一战斗方案
- 实施状态：设计已按当前基线修订；A-03 仍为「排队」，晋升、认领和创建实现 change 之前不得实现

## 背景与进入条件

当前服务端同时存在两条近战路径：玩家路径以持续 primary action、3 格射线、固体遮挡、流体穿透和最近目标完成玩家对玩家攻击；夜行者路径由 server manager 冻结追逐目标，在 1.8 格水平距离内造成 3 点伤害。玩家路径在收集阶段立即写目标保护和疲劳，夜行者路径在独立阶段立即结算；二者共享玩家受击保护字段，却不能跨类型冻结、仲裁或支持玩家攻击夜行者。

A-04 与 A-05 已先于本行交付。当前真实基线为协议 v31、玩家 schema v8、区块 schema v9、世界 metadata v3、`companions.ai` schema v4、`hostile_mobs` schema v1、engine ABI v8、client ABI v10、benchmark scenario v19；物品尾部为 `ItemBed=46`，配方尾部为 `RecipeBed=16`，Play S→C registry 尾部为 ID 24，正式 capture 为 23 项。

原实现分支已经丢失，`docs/superpowers/plans/2026-08-23-tiered-swords-combat.md` 只保留历史行为背景。此前本设计把 A-04 当作未来消费者的内容也已经失效；本次修订以已合入的夜行者状态、持久化、协议和临时战斗 seam 为输入。

实施入口必须同时满足：

1. planner 或控制会话把 A-03 从「排队」晋升为「就绪」；
2. 实现者按流程认领 A-03，并声明与其它在途任务不冲突的独占文件集；
3. 从届时 `main` 再读一次版本、append-only 编号、capture 顺序和 HUD 容量；若与本设计冻结值不同，先更新设计并取得裁决；
4. 从最新 `main` 创建 isolation worktree 和 OpenSpec change `tiered-swords-combat`，不得复活旧批次 change。

## 目标

1. 交付木剑、石剑、铁剑及对应损坏形态和三条可实际制作的形状配方。
2. 为玩家攻击增加独立攻击冷却、分级伤害、水平击退和成功命中耐久损耗。
3. 把玩家与夜行者的攻击意图收敛到一个有界、确定的 tagged value 结算内核，删除夜行者临时伤害循环。
4. 允许玩家通过现有 3 格射线攻击夜行者，并让玩家、夜行者和跨类型竞争遵守同一冻结与 victim reservation 边界。
5. 用私有权威 `CombatHit` 确认驱动本地音效和 hit marker；客户端不得预测命中、伤害、击退或耐久。
6. 保持 Memory/TCP 业务结果一致，并由本行自行完成 capture、golden、OpenSpec、完整门禁和合入。

## 非目标

- 不做暴击、蓄力、挥剑动画、连击、格挡、附魔、范围攻击或双持。
- 不做护甲、投射物、远程敌怪、状态效果、难度分支或伙伴战斗。
- 不改变夜行者生成、寻路、持久化、网络快照、灼烧、远离消失或腐肉掉落规则，只收编其近战结算 seam 和受其影响的死亡阶段顺序。
- 不建设 ECS、通用 actor interface、插件式武器注册表、数据驱动伤害系统或通用动画系统。
- 不增加新的攻击键或输入位；继续复用 `PlayerInput.Mining` 表达持续 primary action。
- 不改变 Rust 物理积分、碰撞、engine ABI、client ABI 或 benchmark workload。
- 不导入二进制美术或音频资产。

## 方案选择

### 采用：独立意图生产者与单一结算内核

玩家和夜行者保留各自必要的意图生产逻辑：玩家从最多 72 个冻结 actor 快照中做 3 格射线选择；夜行者继续消费 server manager 已选择的追逐目标，并重验 1.8 格水平距离。两类生产者只写固定容量 tagged intent，全部意图形成后再统一做跨类型 victim reservation 和 settlement。

这个方案消除平行伤害循环，同时不重写 A-04 的 pathfinding、目标选择或移动停靠逻辑。统一的是身份、冻结、冷却推进、竞争仲裁、伤害/击退提交和死亡边界，不是把不同攻击形状伪装成一个 raycast。

### 否决：全 actor 通用候选与射线系统

夜行者攻击依赖 pathfinding 选中的玩家和水平 1.8 格距离，不需要玩家的视线射线。强制两类攻击共用候选选择会重写已经验证的 A-04 行为，没有消费者收益。

### 否决：保留两条循环，只抽共享伤害 helper

该方案改动更小，但无法保证跨类型同 tick 冻结、同目标只命中一次、玩家与夜行者互击或统一 overflow 语义，不满足本行目标。

## 固定玩法数值

| 项目 | 数值 |
|---|---:|
| 徒手、普通物品、损坏剑伤害 | 2 |
| 木剑伤害 / 耐久 | 4 / 59 |
| 石剑伤害 / 耐久 | 5 / 131 |
| 铁剑伤害 / 耐久 | 6 / 250 |
| 玩家攻击者冷却 | 10 tick |
| 玩家命中后的目标保护 | 10 tick |
| 玩家最长命中距离 | 3 格 |
| 夜行者伤害 / 水平距离 | 3 / 1.8 格 |
| 夜行者攻击者冷却 | 20 tick |
| 夜行者命中玩家后的目标保护 | 20 tick |
| 水平击退速度增量 | 0.35 |
| 最大 actor 快照数 | 72（8 玩家 + 64 夜行者） |
| 最大 raw intent 数 | 72（8 玩家 + 64 夜行者） |
| hit marker 持续时间 | 6 个成功呈现帧 |

这些值是玩法契约，不增加配置项。本行不提供不同难度、服务器倍率或武器数据文件。

## 物品、损坏形态与配方

在当前物品表尾部依次追加：

| ID | 物品 |
|---:|---|
| 47 | `ItemWoodenSword` |
| 48 | `ItemStoneSword` |
| 49 | `ItemIronSword` |
| 50 | `ItemBrokenWoodenSword` |
| 51 | `ItemBrokenStoneSword` |
| 52 | `ItemBrokenIronSword` |
| 53 | `ItemIDMax` |

六个物品的 stack limit 都是 1。只有完好剑具有最大耐久和分级武器伤害；损坏剑保留物品身份但按普通物品造成 2 点伤害。`core` 用固定 switch 提供剑分类、伤害、最大耐久和完好→损坏映射，不创建武器 registry struct。

三把剑的 recipe ID 从当前配方表尾部顺序追加：

- `RecipeWoodenSword=17`：纵向两块橡木木板，下接一根木棍；
- `RecipeStoneSword=18`：纵向两块圆石，下接一根木棍；
- `RecipeIronSword=19`：纵向两个铁锭，下接一根木棍。

配方复用既有 shape matcher：允许归一化后的整体横向平移；横放、倒放、材料错误或带多余材料均不匹配；产物具有满耐久。

剑只在成功结算实体命中后损耗 1 点耐久。现有 `consumeToolDurability` 会磨损所有具有耐久上限的物品，因此实施必须把栏位原子扣耐久逻辑提取为 sim 私有 helper：采掘入口先显式排除完好剑再调用，战斗入口只对冻结时的完好剑及同一栏位/物品身份调用。不得虚构仓库中不存在的 `DamageTool` 公共 API。

挥空、遮挡、距离超限、攻击冷却、目标保护、结算竞争失败和方块破坏都不消耗剑耐久。最后一点耐久的命中先按完好剑伤害成立，再把选中栈原子替换为对应损坏形态，数量保持 1、耐久清零。

## 身份与固定容量

稳定战斗身份由两个值组成：

```go
type CombatActor struct {
	Kind core.CombatTargetKind
	ID   uint64
}
```

`CombatTargetKind` 使用 append-only 稳定值：player=1、hostile=2。player 的 ID 无损承载 `SessionID`，hostile 的 ID 承载夜行者稳定 ID。该值只用于确定性裁决、目标解析和 `CombatHit.TargetKind` 校验，不表达 actor 行为接口。

热路径使用两个固定数组：最多 72 个 actor 快照、最多 72 个 raw intent。快照构造或 intent 追加超过容量时，整个 tick 的战斗阶段 fail closed：不截断、不产生部分意图、不递减攻击/受击 cooldown，也不改变伤害、速度、耐久、疲劳或事件。每 tick 的 `meleeSuppressedMining` 是派生标志，进入战斗阶段时仍须清零，避免上一 tick 的成功命中污染本 tick 采掘。

生产状态的 8 玩家和 64 夜行者硬上限保证第 73 个快照正常不可达，边界测试仍必须经测试 seam 锁定 overflow 行为。

## Tick 数据流

统一战斗位于夜行者生成/action/移动之后、两类死亡结算之前：

```text
hostile spawn -> consume hostile actions -> hostile movement
                              |
                              v
             build <=72 tagged actor snapshots
                              |
                              v
          decrement player/hostile attack + hurt cooldowns
                              |
                              v
     freeze hostile intents, then freeze player intents (<=72)
                              |
                              v
             global stable victim reservation
                              |
                              v
 damage -> knockback -> cooldowns -> player side effects -> CombatHit
                              |
                              v
 hostile burn -> hostile deaths -> distant despawn -> player deaths
```

player 保持两个独立运行期计数器：`attackCooldownTicks` 和 `hurtCooldownTicks`。hostile 继续使用现有持久字段 `attackCooldown` 和 `hurtCooldown`；本行激活 hostile `hurtCooldown` 的逐 tick 递减和玩家命中保护语义，不新增字段、不升级 `hostile_mobs` schema。

快照构造成功后，四类正值在本 tick 各减 1。玩家命中 tick 把自身攻击冷却和目标保护设为 10，因此持续按住时第 1 tick 命中、之后 9 tick 不再命中、第 11 tick 可再次命中。夜行者成功命中把自身攻击冷却和玩家目标保护设为 20，保持 A-04 已交付行为。

server manager 在目标进入 1.8 格后每 tick 都冻结夜行者攻击意图，不再用上一份 `HostileMob.AttackCooldown` 快照做前置过滤；sim 在 cooldown 递减后做唯一权威准入，避免冷却从 1 到 0 的边界额外等待一 tick。

raw intent 冻结攻击者/目标身份、伤害、攻击者与目标水平位置、攻击者 yaw、命中距离，以及玩家攻击者的选中栏位和物品身份。收集期间不得修改 health、velocity、cooldown、durability、疲劳或发布事件。

## 目标选择与仲裁

玩家合法目标必须在冻结快照中满足：active、存活、同维度、不是攻击者本人，且射线进入对应 player/hostile AABB 的距离不超过 3 格。

玩家选择使用全序 `(ray distance, TargetKind, stable ID)`：距离最小优先，精确等距时 player=1 先于 hostile=2，再按无符号稳定 ID。不得依赖 map 遍历、append 顺序或 goroutine 完成顺序。方块遮挡继续复用 `core.RaycastBlocks`：固体表面严格位于目标表面之前才阻挡；与目标表面等距不改写命中；流体不阻挡。

夜行者只消费 server manager 已冻结的追逐目标，并在 sim 中重验目标 active、存活、同维和水平距离 ≤1.8；不做方块射线或改选后方目标。

最近目标处于 `hurtCooldown > 0` 时，攻击不得穿透到后方目标。玩家 `attackCooldownTicks > 0`、夜行者 `attackCooldown > 0`、目标受保护、无目标、距离超限或玩家射线被遮挡时都不形成 accepted intent。

所有 raw intent 形成后，按 `(attacker kind order, stable ID)` 做 victim reservation。为保持 A-04 已发布行为，attacker kind order 固定 hostile 在 player 前；同 kind 内按 hostile ID 或 `SessionID` 升序。多个攻击者同 tick 指向同一目标时，只保留该全序最前的一条，其余意图无任何副作用。

A→B 与 B→A 的目标不同，双方意图都保留，即使任一方随后在该 tick 被打到零血。死亡只在全部 accepted intent 结算后处理。

## 结算顺序与原子边界

每条 accepted intent 先完整解析攻击者、目标及玩家冻结栏位身份；任一不变量失败时整条 intent fail closed，其他独立 intent 继续按稳定顺序处理。验证通过后按以下顺序提交：

1. player 目标调用现有 `applyDamage`；hostile 目标经其权威 health 入口扣除冻结伤害；
2. 给目标现有水平 velocity 加上大小为 0.35 的击退向量；
3. 按攻击者类型设置 attack cooldown，并按命中来源设置目标 hurt cooldown；
4. 若攻击者是 player，提交既有 100 milli 近战疲劳并设置本 tick 采掘抑制；
5. 若 player 冻结的选中物品是完好剑，对权威背包中的同一栏位/物品身份恰减 1 点耐久，必要时变成损坏形态；
6. 若攻击者是 player，为该 session 记录一条私有 `CombatHit` 事实。

正常击退方向是 `targetPosition - attackerPosition` 的 XZ 单位向量。两者水平位置完全重合时，使用只由攻击者 yaw 导出的水平单位朝向，不读取 pitch，也不归一化零向量。

物理阶段已经在战斗之前完成，写入 velocity 后从下一 tick 开始产生位移。这保持同 tick 命中快照稳定，也无需修改 Rust `StepInput` 或 engine ABI。

统一战斗后先推进 hostile burn，再调用 `settleHostileDeaths`，随后才做 distant despawn；这保证同 tick 烧死或被剑杀死的夜行者先走腐肉掉落，而不是被无掉落远离移除吞掉。`settleDeaths` 最后处理玩家死亡。玩家与夜行者互杀时双方伤害、击退、冷却和玩家剑耐久都先成立；死亡掉落观察结算后的剑状态。

## 协议与发布

A-03 把协议 v31 升为 v32，并在 Play S→C registry 尾部追加 `CombatHit=25`。固定载荷为 10 bytes：

```text
ServerTick u64
Damage     u8
TargetKind u8
```

校验要求 `ServerTick > 0`、`Damage` 在 `1..core.MaxHealth`、`TargetKind` 只能是 player 或 hostile，并拒绝截断、尾随与未知枚举。消息不携带目标 ID、位置、武器、EventID 或自由文本；每 session 每 tick 最多一条成功玩家攻击，`ServerTick` 足以做反馈去重。

`sim.TickResult.CombatHits` 按攻击者 `SessionID` 稳定排序，只记录 session、damage 和 target kind；server publication 使用最终 `result.Tick` 填充 wire `ServerTick`。每条确认只发送给对应攻击者 session，不广播给目标、旁观者或 trusted observer。

同 tick publication 必须先发送 inventory/container 镜像和 `PlayerState`，再发送 `CombatHit`。这样死亡 reset 先清空旧会话反馈，再由本 tick 合法确认触发 marker；确认不会被随后到达的同 tick reset 擦掉。慢客户端继续沿用现有有界发送队列和断开策略，不增加战斗专用重试或缓冲。

Memory 与 TCP 使用同一模拟和 publication 路径。parity 必须比较权威 health、velocity、选中剑耐久/损坏形态、hostile health、确认载荷和各 transport 内 `ServerTick` 单调性；不得只依赖当前不包含 durability 的 `PlayerHash`。

## 客户端确认反馈

客户端不得根据 primary input、预测射线、目标 health 镜像或背包耐久变化推断命中。`cmd/mornlea/app` 持有独立 combat feedback 状态，只接受 `ServerTick` 严格大于上一条已接受确认的 `CombatHit`；重复或陈旧 tick 静默忽略，不复用全局 `application.serverTick`。

每条有效确认触发：

1. 一个使用既有预分配音频队列和程序化 synth 的原创短促 `CueCombatHit`；固定参数为 1323 samples、520→180 Hz、amplitude 10500，little-endian PCM SHA-256 为 `17752cdda0232ebb88b0e6db1e39fa4a4889e5469bac0c28a07044b677710dae`；
2. 准星中心上、下、左、右各一个白色不透明短线 quad，共 4 quad；设计长度 8 px、厚度 2 px、中心间隙 4 px，随既有 HUD scale 缩放，显示 6 个成功呈现帧。

marker 复用现有 HUD pass、缩放和边界裁剪；不增加 atlas cell、纹理、shader、pipeline、动态 GPU 资源或通用动画系统。当前真实最大关闭态/打开态为 96/257 quad，加入 marker 后为 100/261，仍装入固定 267-quad 容量；必须修正主规格中过时的 266 数字，不得扩大固定上传布局、client ABI 或 benchmark scenario。

只有 `renderer.RenderFrame` 返回 true 后才消耗一个 marker 帧。零 framebuffer、HUD prepare 失败、entity overflow 或 renderer 返回 false 都不得消耗。新确认把剩余帧数重置为 6。

断线、退回主菜单、建立新会话、权威 reset 和 capture 场景切换都清零 combat 去重状态与 marker 帧数。共享 `resetSessionOwnedState` 同时补清 hostile mirror，避免旧会话敌怪或 capture 夹具泄漏。音频设备不可用继续无声降级，不影响 marker 或命中事实。

三把剑和三个损坏形态在 `render.ItemColor` 的固定 switch 中登记原创程序化颜色，保证 HUD 与掉落物呈现不透明、可区分。完好与损坏形态不得使用相同颜色。

## 视觉验证

本行新增正式无窗口场景 `sword-combat`，固定展示：

- 选中非满耐久铁剑；
- 一名被权威确认命中的远端玩家；
- 处于 6 帧窗口内的 hit marker；
- 击退写入后的可观察姿态或位置关系。

场景顺序固定为：

```text
ai-companion
sword-combat
hostile-mob
water-surface-slope
```

正式场景总数从 23 增为 24；`far-horizon` 继续倒数第二，`water-underwater` 继续唯一末项。`PinVolatile` 在场景收敛后、最终帧前重新武装 marker，避免收敛帧提前耗尽；`resetCapturePresentation` 清除 combat feedback。

A-03 自行运行正式 producer、生成 `cmd/mornlea/capture/testdata/golden/sword-combat.png` 并逐图审核。既有 golden 原则上应保持在阈值内；若共享 HUD 代码产生差异，必须逐图归因和明确批准，不能批量接受或放宽阈值。自动测试不得启动或聚焦前台窗口。

## 兼容性与版本矩阵

| 契约 | 当前 | A-03 后 |
|---|---:|---:|
| 协议 | v31 | v32 |
| 玩家 schema | v8 | v8 |
| 区块 schema | v9 | v9 |
| 世界 metadata | v3 | v3 |
| `companions.ai` schema | v4 | v4 |
| `hostile_mobs` schema | v1 | v1 |
| engine ABI | v8 | v8 |
| client ABI | v10 | v10 |
| benchmark scenario | v19 | v19 |

协议旧客户端在握手阶段按现有版本不匹配规则拒绝。物品编号只追加，既有 `ItemStack` 编码已经包含物品 ID、数量和耐久；当前程序必须在玩家、区块容器/掉落和伙伴背包路径 round trip 新剑。包含新剑的存档不能由不知道新 ItemID 的旧程序安全解释，项目不提供向后降级写入。

player 两个 cooldown 是短期运行态，不持久化；重连创建新 session 并清零。hostile 的 attack/hurt cooldown 已经属于 schema v1 记录，本行只赋予现有 hurt 字段实际递减和受击保护语义，保持当前 codec 与上限 20，不升 schema。

归档时同步根 `AGENTS.md`、`openspec/config.yaml`、双语 README、`docs/notes/progress.md` 和对应主规格中的当前版本矩阵；历史归档文档不批量改写。

## 错误与安全边界

- 无目标、冷却、受保护、遮挡和超距是正常未命中，不记 error。
- actor snapshot 或 intent overflow 是内部上限破坏，整个 tick 战斗 fail closed；测试必须证明除 tick-local 采掘抑制复位外没有部分副作用。
- 身份、维度、冻结栏位或目标在结算时无法解析是内部不变量破坏，该 intent fail closed；其他独立 intent 继续稳定处理。
- 非法 `CombatHit` 在 network trust boundary 拒绝，不能进入 app feedback 状态。
- 音频设备不可用继续无声降级；命中事实和 HUD 不依赖音频成功。
- 跨 goroutine 发布后的 packet、事件与切片不可变。
- 权威 tick 不分配无界集合、不阻塞 I/O，也不读取客户端反馈状态。

## 文件范围

预计生产范围：

- `internal/core`：物品、剑分类/伤害/损坏映射、三条配方和 combat kind；
- `internal/sim`：统一 actor snapshot/intent、双类型 cooldown、仲裁、击退、耐久、疲劳、死亡顺序和 tick result；
- `internal/server`：hostile manager 意图边界、私有 publication 与 Memory/TCP parity；
- `internal/network`、`internal/network/tcp`：`CombatHit`、packet registry、codec、fuzz 与 TCP transcript；
- `internal/render`、`internal/render/hud`、`internal/audio`：物品颜色、marker 和 cue；
- `cmd/mornlea/app`：确认去重、生命周期复位、HUD/audio 接线；
- `cmd/mornlea/capture`：场景、消费端接口、顺序测试和 golden；
- OpenSpec change `tiered-swords-combat`、对应包的 `AGENTS.md`、backlog 本行和必要的长期基线记录。

不触碰 Rust engine/client 生产代码、配置格式或 benchmark producer。若实现调查迫使范围变化，必须先更新设计和 OpenSpec 产物。

## OpenSpec 范围

新 change 至少包含一份新增的 `tiered-swords-combat` capability，并修改这些既有能力：

- `authoritative-player-melee`；
- `authoritative-hostile-nightwalker`；
- `authoritative-mining`；
- `authoritative-hunger`；
- `tool-durability`；
- `authoritative-crafting`；
- `local-audio-feedback`；
- `survival-hud-presentation` 与 `container-ui-presentation`；
- `visual-verification`。

主规格目前存在的配方计数/预留、HUD 266 quad、capture 21 项等漂移必须随 change 纠正，不能只追加新场景或新物品而保留互相矛盾的当前契约。

## 测试设计

### Core 与 Storage

- 六个 append-only ItemID、名称标识、stack limit、耐久和完好→损坏映射。
- 空手/普通物品/损坏剑伤害 2，木/石/铁剑伤害 4/5/6。
- 三条配方的平移、方向、错误材料、多余材料和满耐久产物。
- player、hostile kind 稳定值和未知值拒绝。
- 玩家、区块容器/掉落、伙伴背包对新 ItemID 的当前版本 round trip；未知 `ItemIDMax` 拒绝保持不变。

### Sim

- actor snapshot 与 intent 的 72/73 边界；overflow 时 cooldown、health、velocity、durability、疲劳和事件均不变。
- 玩家与 hostile cooldown 独立递减；玩家第 1/11 tick、夜行者 20 tick 边界精确。
- 玩家 mixed target 的距离、kind、stable ID 全序；固体遮挡、表面等距和流体穿透。
- 最近受保护目标不允许穿透命中后方目标。
- hostile-first 跨类型 reservation、同 kind 最小 ID、同目标唯一胜者和不同目标互杀。
- 冻结后改变夹具位置、health 或栏位不改写已冻结射线与伤害身份。
- 三档剑伤害、普通物品伤害、玩家攻击 hostile、最后一点耐久转损坏形态。
- 只有成功结算扣耐久、收疲劳并抑制采掘；所有未命中路径保持背包和采掘状态。
- 完好剑采掘成功不耗耐久；其他既有工具和作物×锄头豁免逐字保持。
- 普通水平击退和零水平距离 yaw 回退；不得产生 NaN/Inf。
- 玩家/hostile 互杀、灼烧与 distant 同 tick、死亡掉落观察结算后的状态。

### Network 与 Server

- protocol v32、S→C ID 25、10-byte `CombatHit` round trip、little-endian golden、值域、截断、尾随、未知 kind 与 fuzz。
- 确认只发玩家攻击者；受击者、旁观者、夜行者攻击目标和 trusted observer 不收到。
- inventory/`PlayerState` 先于 `CombatHit`，死亡 reset 不吞同 tick 确认。
- Memory/TCP 对 player/hostile health、velocity、耐久/损坏形态和确认投影一致。
- 冷却、遮挡、竞争淘汰和挥空不发布确认。
- server manager 不再用陈旧 cooldown 快照提前过滤，sim 在递减后做唯一准入。

### Client、Audio、HUD 与 Capture

- 只有严格递增的确认 tick 触发；输入、预测、health 和 inventory 更新都不能触发。
- 重复/陈旧确认忽略；断线、退回菜单、新会话和 reset 清零；hostile mirror 同时清空。
- 一次确认恰好一条 cue，PCM 参数/hash 稳定；音频不可用不影响 marker。
- marker 恰好 4 quad、显示 6 个成功呈现帧；失败呈现和零 framebuffer 不扣帧。
- 最大关闭/打开态带 marker 为 100/261，仍不超过 267；warmed `Prepare` 保持零分配。
- 六个剑物品颜色可见、互异且完好/损坏可区分。
- `sword-combat` 场景顺序、状态构造、24 项清单和 golden；既有场景无未解释变化。

## 交付拆分

正式 OpenSpec tasks 按以下顺序拆分，每组由 fresh implementer 完成 TDD，并接受独立 SPEC 与 QUALITY 双评审：

1. **准入、OpenSpec 与基线冻结**：晋升/认领后读取当前版本、编号、场景、分包路径和容量，创建 proposal、delta specs、design、tasks、ledger，strict validate。
2. **物品、配方、持久兼容与耐久**：core 登记、shape recipe、武器数值、损坏映射、storage round trip 和采掘耐久豁免。
3. **统一冻结与结算战斗**：tagged actor、72 snapshot/intent、双类型 cooldown、hostile/player 生产者、全局仲裁、击退、疲劳、耐久和死亡顺序。
4. **协议、publication 与 parity**：protocol v32、`CombatHit=25`、codec、私有发送、发布顺序和 Memory/TCP 集成。
5. **客户端反馈与视觉闭环**：单调确认、生命周期复位、cue、marker、物品颜色、capture、golden 和局部指南同步。
6. **整分支收尾**：独立终审、完整门禁、OpenSpec sync/archive、版本文档、backlog/Discussion、PR、CI、合入和工作树清理。

## 验证门禁

任务级验证按真实受影响包选择 focused race。整分支至少运行：

```bash
make rust
go test ./internal/core ./internal/storage ./internal/network ./internal/network/tcp ./internal/sim ./internal/server ./internal/audio ./internal/render ./internal/render/hud ./cmd/mornlea/app ./cmd/mornlea/capture -race -count=1
go test ./internal/archcheck -count=1
go vet ./...
go test ./... -race
make rust-check
make visual-check
scripts/agents/gates.sh
openspec validate --all --strict --no-interactive
git diff --check
```

若新增或更新 golden，必须按仓库正式 producer 流程生成候选、逐图审核后再更新 tracked baseline，并重新运行 `make visual-check`。不得通过提高测试 timeout、放宽视觉阈值、跳过 race 或修改门禁来规避失败。

## 性能、风险与回退

战斗每 tick 最多处理 72 个 actor snapshot 和 72 个 raw intent。最多 8 个玩家攻击者各扫描 71 个其它 actor，固定上界为 568 次 AABB 射线测试，再为实际有候选的玩家执行既有有界方块 raycast。夜行者只重验 manager 冻结的单一目标。热路径使用固定数组，不引入 map、goroutine、磁盘、网络等待或模型调用。

主要风险：

1. 实施前 `main` 再次追加版本、编号或 capture 场景；通过进入条件重新冻结，不复用本文数字猜测。
2. 只抽伤害 helper而保留两个循环，导致跨类型竞争和互击仍按阶段副作用决定；通过单一 raw intent 缓冲和 reservation 测试避免。
3. 把 hostile 20 tick 行为静默改成玩家 10 tick，破坏 A-04 兼容；通过来源相关 cooldown 和边界测试锁定。
4. manager 在 sim 之前过滤 cooldown，造成递减边界多等一 tick；通过移除前置过滤和第 20 tick 用例锁定。
5. 剑进入通用耐久表后被采掘路径误扣；通过共享栏位扣耐久 helper、采掘分类守卫和直接测试锁定。
6. `PlayerState.Reset` 在确认后到达，擦除本 tick marker；通过固定 publication 顺序和互杀集成测试锁定。
7. 把主规格中过时的 266 quad 当作代码事实而扩大上传布局；通过代码容量测试冻结 257→261/267，并同步修正规格。

回退时可删除新 packet、客户端反馈、剑登记和统一战斗扩展，并恢复 A-04 临时 hostile melee 与原玩家近战。协议已经升版的已发布构建不能安全降级到旧客户端，因此正常回退应发布下一协议版本，而不是复用旧版本号。玩家存档没有 schema 迁移；若存档已包含新剑，回退构建必须保留 ItemID 解码或显式拒绝，不能把未知物品静默改为空气。

## 已决结论

本设计没有待定玩法项。A-04 的 hostile-first、3 点伤害、1.8 格范围、20 tick 攻击/命中保护保持不变；A-03 的玩家 10 tick 攻击/命中保护、剑数值、私有确认和 6 帧反馈保持不变。当前 append-only 编号与版本已按合并基线冻结，但实施仍须按进入条件从最新 `main` 复核。
