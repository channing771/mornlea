# A-03 三级剑与统一战斗设计

## 状态

- 日期：2026-08-28
- backlog：A-03
- 设计分支：`docs/A-03-combat-design`
- 内容确认：用户已批准任务选择、最小架构、战斗规则、协议与反馈边界、失败路径、测试矩阵和交付拆分
- 实施状态：仅完成设计；A-02 合入并冻结真实基线前，不晋升、不认领、不创建实现 change

## 背景与进入条件

当前服务端已经提供最多八名玩家之间的权威徒手近战：持续 primary action、3 格射线、固体遮挡、流体穿透、最近目标、等距 `SessionID` 裁决、2 点伤害、10 tick 目标近战保护、同 tick 意图冻结和命中时采掘抑制。伤害继续走 `applyDamage`，死亡由紧随战斗之后的 `settleDeaths` 结算。

A-03 是当前基础玩法发布列车中 A-02 之后的下一项。原实现分支已经丢失，`docs/superpowers/plans/2026-08-23-tiered-swords-combat.md` 只保留历史行为背景；其中共享批次、预分配编号、统一集成分支和“不生成本行 golden”等假设均已失效。当前流程要求每行基于届时 `main` 独立完成版本、视觉基线、归档、PR 和合入。

本设计的实施入口必须同时满足：

1. A-02 已合入 `main`，其物品、配方、engine ABI、capture 顺序和 golden 已完成归档；
2. `main` 工作树干净，A-03 与其他已认领行没有文件冲突；
3. 从 A-02 后 `main` 读取 next append-only 编号和当前版本矩阵，不沿用历史计划里的绝对编号；
4. 重新建立 OpenSpec change `tiered-swords-combat`，以本设计和当前主规格为输入，不复活旧批次 change。

## 目标

1. 交付木剑、石剑、铁剑及对应损坏形态和三条可实际制作的形状配方。
2. 在现有玩家近战上增加攻击者冷却、目标近战保护、分级伤害、水平击退和成功命中耐久损耗。
3. 把目标选择和意图结算收敛为确定、有界的 tagged value 路径，为 A-04 追加夜行者候选与攻击意图提供唯一窄接点。
4. 用私有权威 `CombatHit` 确认驱动本地音效和 hit marker；客户端不得预测命中、伤害、击退或耐久。
5. 保持 Memory/TCP 业务结果一致，并由本行自行完成 capture、golden、OpenSpec、完整门禁和合入。

## 非目标

- 不做暴击、蓄力、挥剑动画、连击、格挡、附魔、范围攻击或双持。
- 不做护甲、投射物、远程敌怪、状态效果、难度分支或伙伴战斗。
- 不在本行实现夜行者状态、AI、生成、攻击、存档、网络或呈现；只冻结 A-04 所需的窄 combat seam。
- 不建设 ECS、通用 actor interface、插件式武器注册表、数据驱动伤害系统或通用动画系统。
- 不增加新的攻击键或输入位；继续复用 `PlayerInput.Mining` 表达持续 primary action。
- 不改变 Rust 物理积分、碰撞、engine ABI 或 client ABI。
- 不导入二进制美术或音频资产。

## 方案选择

### 采用：有界统一战斗内核

在 `internal/sim` 的现有近战路径内使用固定容量 candidate/intent 值，先冻结、再仲裁、最后结算。玩家是本行唯一真实攻击者和目标消费者；目标身份使用 `TargetKind + stable ID`，A-04 后续只能向同一 candidate builder 和 settlement switch 追加 hostile 分支，不能另建第二套命中循环。

这个方案完成 A-03 承诺的统一裁决，又不提前建设没有第二个消费者的通用实体框架。

### 否决：只给现有 PvP 增加剑

该方案改动更小，但 A-04 随后必须重写目标选择、意图身份和结算，不能满足发布列车要求的统一战斗接点。

### 否决：通用战斗 actor/weapon 框架

当前只有玩家一个 live actor 实现。接口、注册表、动态效果钩子和数据驱动武器不会改善本行体验，只会扩大迁移、测试和评审面。

## 固定玩法数值

| 项目 | 数值 |
|---|---:|
| 徒手、普通物品、损坏剑伤害 | 2 |
| 木剑伤害 / 耐久 | 4 / 59 |
| 石剑伤害 / 耐久 | 5 / 131 |
| 铁剑伤害 / 耐久 | 6 / 250 |
| 攻击者冷却 | 10 tick |
| 目标近战保护 | 10 tick |
| 最长命中距离 | 3 格 |
| 水平击退速度增量 | 0.35 |
| 最大候选数 | 72（8 玩家 + 64 敌怪） |
| 本行每 tick 最大玩家意图数 | 8 |
| hit marker 持续时间 | 6 帧 |

这些值是玩法契约，不增加配置项。本行不提供不同难度、服务器倍率或武器数据文件。

## 物品、损坏形态与配方

在 A-02 后的物品表尾部依次追加三把完好剑和三个对应损坏形态。六个物品的 stack limit 都是 1；只有完好剑具有最大耐久和武器伤害，损坏剑保留物品身份但按普通物品造成 2 点伤害。

`core` 用固定 switch 提供剑分类、伤害、最大耐久和完好→损坏映射，继续复用现有 `ItemStack` 耐久编码与 `DamageTool` 原子替换语义，不创建武器 registry struct。三把剑的 recipe ID 从 A-02 后配方表尾部顺序追加：

- 木剑：纵向两块木板，下接一根木棍；
- 石剑：纵向两块圆石，下接一根木棍；
- 铁剑：纵向两个铁锭，下接一根木棍。

配方复用既有 shape matcher：允许归一化后的整体横向平移；横放、倒放、材料错误或带多余材料均不匹配；产物具有满耐久。

剑只在成功结算实体命中后损耗 1 点耐久。现有采掘完成路径必须使用同一剑分类显式跳过剑耐久消耗；挥空、遮挡、距离超限、攻击冷却、目标保护、结算竞争失败和方块破坏都不消耗剑耐久。最后一点耐久的命中先按完好剑伤害成立，再把选中栈原子替换为对应损坏形态，数量保持 1、耐久清零。

## 身份与固定容量

稳定目标身份由两个值组成：

```go
type CombatTarget struct {
	Kind core.CombatTargetKind
	ID   uint64
}
```

`CombatTargetKind` 使用 append-only 稳定值：player=1、hostile=2。player 的 ID 无损承载 `SessionID`；A-04 的 hostile ID 将承载其稳定实体 ID。该值用于模拟内确定性裁决和 `CombatHit` 的 target kind 校验，不表达 actor 行为接口。

热路径使用固定数组：最多 72 个 candidate、本行最多 8 个玩家 intent。candidate builder 超过容量时返回失败，整个 tick 的战斗阶段 fail closed：不截断、不产生部分意图、不改变伤害、速度、冷却、耐久或事件。生产状态的 8 玩家和未来 64 敌怪硬上限保证该分支正常不可达，边界测试仍必须锁定它。

A-03 只实现 player target 的读取和写回。A-04 后续在同一个 builder 和 settlement switch 中增加 hostile 分支；不得保留平行的 nightwalker 专用 raycast、伤害或 victim reservation 路径。

## Tick 数据流

战斗继续位于统一物理推进和订阅收敛之后、`settleDeaths` 之前：

```text
active players + frozen primary input + frozen bodies/inventory
                         |
                         v
       build <=72 tagged target candidates
                         |
                         v
              decrement two cooldowns
                         |
                         v
        freeze <=8 raw player attack intents
                         |
                         v
  stable victim arbitration (one hit per victim)
                         |
                         v
 damage -> knockback -> cooldowns -> durability -> CombatHit
                         |
                         v
                    settleDeaths
```

每名 active 玩家拥有两个独立、仅由权威 tick 读写的运行期计数器：

- `attackCooldownTicks`：该玩家再次形成攻击意图前的等待；
- `hurtCooldownTicks`：该玩家再次成为成功近战目标前的保护。

candidate 构造成功后，两个正值在本 tick 各减 1。命中 tick 设置为 10，因此持续按住时第 1 tick 命中、之后 9 tick 不再命中、第 11 tick 可再次命中。两个字段不持久化、不进入 wire；重连与新会话沿用既有运行期状态重建语义。candidate overflow 时整个战斗阶段跳过，两个计数器也不递减，与“零部分副作用”保持一致。

收集 raw intent 时冻结攻击者、目标身份、伤害、攻击者选中栏位和物品身份、攻击者/目标水平位置以及命中距离。收集期间不得修改 health、velocity、cooldown、durability 或发布事件。

所有 raw intent 形成后，按攻击者稳定 ID 顺序做 victim reservation。多个攻击者同 tick 指向同一目标时，只保留攻击者 ID 最小的一条；其余意图无任何副作用。A→B 与 B→A 的目标不同，双方意图都保留，即使任一方随后在该 tick 被打到零血。死亡只在全部已接受意图结算后处理。

## 目标选择

合法目标必须在冻结快照中满足：active、存活、同维度、不是攻击者本人，且射线进入目标 AABB 的距离不超过 3 格。

选择使用全序 `(ray distance, TargetKind, stable ID)`：距离最小优先，精确等距时 player=1 先于 hostile=2，再按无符号稳定 ID。不得依赖 map 遍历、append 顺序或 goroutine 完成顺序。

方块遮挡继续复用 `core.RaycastBlocks` 与既有 interaction target 语义：固体表面严格位于目标表面之前才阻挡；与目标表面等距不改写命中；流体不阻挡。最近物理目标若处于 `hurtCooldownTicks > 0`，攻击不穿透到后方目标。

攻击者 `attackCooldownTicks > 0`、目标受保护、无目标、距离超限或方块遮挡时都不形成已接受意图。只有已接受并实际结算的实体命中才把攻击者的 `meleeSuppressedMining` 设为 true；其他情况完整保留该 tick 既有采掘行为。

## 结算顺序与原子边界

每条已接受 intent 按以下固定顺序结算：

1. 通过目标类型对应的既有伤害入口应用冻结伤害；本行 player 分支调用 `applyDamage`；
2. 给目标现有水平 velocity 加上大小为 0.35 的击退向量；
3. 把攻击者 `attackCooldownTicks` 和目标 `hurtCooldownTicks` 都设为 10；
4. 若冻结的选中物品是完好剑，对权威背包中的同一选中栈恰减 1 点耐久，必要时变成损坏形态；
5. 为攻击者记录一条私有 `CombatHit` 事实。

正常击退方向是 `targetPosition - attackerPosition` 的 XZ 单位向量。两者水平位置完全重合时，使用只由攻击者 yaw 导出的水平单位朝向，不读取 pitch，也不归一化零向量。

物理阶段已经在战斗之前完成，写入 velocity 后从下一 tick 开始产生位移。这保持同 tick 命中快照稳定，也无需修改 Rust `StepInput` 或 engine ABI。

在单写者 tick 内，冻结和结算之间没有外部并发写者。若内部身份查找仍违反不变量，结算对该 intent fail closed，不扣耐久、不设冷却、不发确认；不得对错误目标应用部分副作用。

互杀时双方伤害、击退、冷却与剑耐久都先成立，再由 `settleDeaths` 处理两名玩家。死亡掉落观察到结算后的剑状态，因此最后一点耐久的互杀会掉落损坏剑而不是恢复成完好剑。

## 协议与发布

A-03 在届时 `main` 的 Play S→C registry 尾部追加 `CombatHit`，并把协议版本增加 1。若 A-02 按当前设计不改 wire，则预期是协议 v29→v30；实际常量和 packet ID 必须在 A-02 合入后从代码得出。

固定载荷为 10 bytes：

```text
ServerTick u64
Damage     u8
TargetKind u8
```

校验要求 `ServerTick > 0`、`Damage` 在 `1..core.MaxHealth`、`TargetKind` 只能是 player 或 hostile，并拒绝截断、尾随与未知枚举。消息不携带目标 ID、位置、武器或自由文本；本地反馈不需要这些值。

`sim.TickResult.CombatHits` 按攻击者 `SessionID` 稳定排序。`internal/server` 只把每条确认发送给对应攻击者 session，不广播给目标或其他在线玩家。一次成功结算恰好一条确认；失败、冷却和竞争淘汰均无确认。慢客户端继续沿用现有有界发送队列和断开策略，不增加战斗专用重试或缓冲。

Memory 与 TCP 使用同一模拟和 publication 路径。parity 必须比较权威 health、velocity、选中剑耐久/损坏形态、确认载荷和各 transport 内的 tick/EventID 单调性；不比较不同 transport 运行之间的绝对 EventID。

## 客户端确认反馈

客户端不得根据 primary input、预测射线、目标 health 镜像或背包耐久变化推断命中。只有合法且 `ServerTick` 严格大于上一条已接受 `CombatHit` 的私有确认才能触发反馈；重复或陈旧 tick 静默忽略。断线、退回主菜单或建立新会话时清零去重状态和剩余 marker 帧数。

每条有效确认触发：

1. 一个使用既有预分配音频队列和程序化 synth 的原创短促 `CueCombatHit`；
2. 准星四角各一个短线 quad，共 4 quad，固定显示 6 个呈现帧。

marker 复用现有 HUD pass、缩放和边界裁剪；不增加 atlas cell、纹理、shader、pipeline、动态 GPU 资源或通用动画系统。4 个临时 quad 必须装入既有固定 267-quad HUD 容量；若 A-02 合入后真实最坏组合不再有 4 个空位，实施必须先回到设计层裁决，不能静默扩大固定上传布局和 benchmark 身份。

三把剑和三个损坏形态在 `render.ItemColor` 的固定 switch 中登记原创程序化颜色，保证 HUD 与掉落物呈现不透明、可区分。完好与损坏形态不得使用相同颜色。

## 视觉验证

本行新增一个正式无窗口场景 `sword-combat`，固定展示：

- 选中铁剑及其非满耐久；
- 一名被权威确认命中的远端玩家；
- 处于 6 帧窗口内的 hit marker；
- 击退写入后的可观察姿态或位置关系。

场景放在既有末尾水景三项 `water-surface-slope`、`far-horizon`、`water-underwater` 之前，保持三者相对顺序和末尾约束。若 A-02 按计划新增 `torch-night`，A-03 完成后正式场景数预期为 22；真实数量以 A-02 后场景表为准并由顺序测试锁定。

A-03 自行运行正式 producer、生成 `sword-combat` golden 并逐图审核。既有 golden 原则上应逐字节不变；若因共享 HUD 代码产生差异，必须逐图归因和明确批准，不能批量接受或放宽阈值。自动测试不得启动或聚焦前台窗口。

## 兼容性与版本矩阵

- 协议：在实施基线上增加 1；预期 v29→v30。
- 玩家 schema：保持 v7；既有 `ItemStack` 已编码物品 ID、数量和耐久。
- 区块 schema：保持 v9。
- 世界 metadata：保持 v2。
- `companions.ai` schema：保持 v4。
- engine ABI：保持 A-02 合入后的值；本行不改 Rust engine 输入、输出或 registry。
- client ABI：保持 v9；反馈所需事件已经通过 Go 网络路径进入现有应用状态，HUD pass 无 ABI 扩展。
- benchmark scenario：保持 v19；不改变固定上传布局或固定 benchmark 工作负载。

协议旧客户端在握手阶段按现有版本不匹配规则拒绝。物品编号只追加，旧玩家存档按现有 ItemStack 解码读取；包含新剑的存档不能由不知道新 ItemID 的旧程序安全解释，项目不提供向后降级写入。

两个 cooldown 都是短期运行态，不持久化。重连会创建新会话并清零，和现有近战保护的运行期语义一致；为 0.5 秒战斗状态升级玩家 schema 不值得。

## 错误与安全边界

- 无目标、冷却、受保护、遮挡和超距是正常未命中，不记 error。
- candidate overflow 是内部上限破坏，整个 tick 战斗 fail closed；测试必须证明无部分副作用。
- 目标身份在结算时无法解析是内部不变量破坏，该 intent fail closed；其他独立 intent 仍按稳定顺序处理。
- 非法 `CombatHit` 在现有 network trust boundary 拒绝，不能进入 app feedback 状态。
- 音频设备不可用继续无声降级；命中事实和 HUD 不依赖音频成功。
- 跨 goroutine 发布后的 packet、事件与切片不可变。
- 权威 tick 不分配无界集合、不阻塞 I/O，也不读取客户端反馈状态。

## 文件范围

预计生产范围：

- `internal/core`：物品、耐久映射和三条配方；
- `internal/sim`：combat candidate/intent、双 cooldown、结算、采掘耐久豁免和 tick result；
- `internal/network`：`CombatHit`、packet registry、codec 与 fuzz；
- `internal/server`：私有 publication 与 Memory/TCP parity；
- `internal/render`、`internal/render/hud`、`internal/audio`：物品颜色、marker 和 cue；
- `cmd/mornlea`：确认去重、生命周期复位、HUD/audio 接线与 capture；
- OpenSpec change `tiered-swords-combat`、backlog 本行和必要的长期基线记录。

不触碰 `internal/storage`、伙伴代码、A-04 hostile 状态、Rust engine/client 生产代码、配置格式或 benchmark producer。A-02 合入后若真实依赖迫使范围变化，必须先更新设计和 OpenSpec 产物。

## 测试设计

### Core

- 六个物品的 append-only ID、名称、stack limit、耐久和完好→损坏映射。
- 空手/普通物品/损坏剑伤害 2，木/石/铁剑伤害 4/5/6。
- 三条配方的平移、方向、错误材料、多余材料和满耐久产物。
- 完好剑采掘成功不耗耐久；其他既有工具规则逐字保持。

### Sim

- 攻击者与目标 cooldown 独立递减；第 1/11 tick 命中边界精确。
- 最多 72 个 mixed tagged candidates；第 73 个令整个构造失败且零副作用。
- 最近距离、kind、stable ID 全序；固体遮挡、表面等距和流体穿透。
- 最近目标受保护时不穿透命中后方目标。
- A/B 同 tick 互杀均成立；多攻击者同目标只接受最小 attacker ID。
- 冻结后改变测试夹具中的位置或 health 不改写已冻结射线与伤害身份。
- 三档剑伤害、普通物品伤害、最后一点耐久转损坏形态。
- 只有成功结算扣 1 耐久并抑制采掘；所有未命中路径保持背包和采掘状态。
- 普通水平击退和零水平距离 yaw 回退；不得产生 NaN/Inf。
- 互杀死亡掉落观察结算后的剑状态。

### Network 与 Server

- 10-byte `CombatHit` round trip、值域、截断、尾随、未知 kind 与 fuzz。
- 确认只发攻击者；受击者和旁观者不收到。
- Memory/TCP 对 health、velocity、耐久/损坏形态和确认事件投影一致。
- 冷却、遮挡、竞争淘汰和挥空不发布确认。

### Client、Audio、HUD 与 Capture

- 只有严格递增的确认 tick 触发；输入、预测、health 和 inventory 更新都不能触发。
- 重复/陈旧确认忽略；断线、退回菜单和新会话清零。
- 一次确认恰好一条 cue，音频不可用不影响其余状态。
- marker 恰好 4 quad、显示 6 帧、缩放后界内，固定容量不溢出。
- 六个剑物品颜色可见且完好/损坏可区分。
- `sword-combat` 场景顺序、状态构造和 golden；既有场景无未解释变化。

## 交付拆分

正式 OpenSpec tasks 按以下顺序拆分，每组由 fresh implementer 完成 TDD，并接受独立 SPEC 与 QUALITY 双评审：

1. **OpenSpec 与基线冻结**：读取 A-02 后版本、编号、场景和容量，创建 proposal、delta specs、design、tasks、ledger，strict validate。
2. **物品、配方与耐久**：core 登记、shape recipe、武器数值、损坏映射和采掘耐久豁免。
3. **有界冻结/结算战斗**：tagged candidate、双 cooldown、稳定仲裁、击退、耐久和死亡顺序。
4. **协议、publication 与 parity**：`CombatHit`、codec、私有发送、Memory/TCP 集成。
5. **客户端反馈与视觉闭环**：单调确认、生命周期复位、cue、marker、物品颜色、capture 和 golden。
6. **整分支收尾**：独立终审、完整门禁、OpenSpec sync/archive、backlog/Discussion、PR、CI、合入和工作树清理。

旧计划中的共享 integration controller、统一批次版本、功能分支不更新 golden 等步骤全部删除，不得复制到新 tasks。

## 验证门禁

任务级验证按真实受影响包选择 focused race。整分支至少运行：

```bash
make rust
go test ./internal/core ./internal/network ./internal/sim ./internal/server ./internal/audio ./internal/render/hud ./cmd/mornlea -race -count=1
go test ./internal/archcheck -count=1
go vet ./...
go test ./... -race
make rust-check
make visual-check
scripts/agents/gates.sh
openspec validate --all --strict --no-interactive
git diff --check
```

若新增或更新 golden，必须按仓库正式 producer 流程先更新再运行 `make visual-check`，并记录逐图审核结论。不得通过提高测试 timeout、放宽视觉阈值、跳过 race 或修改门禁来规避失败。

## 性能、风险与回退

战斗每 tick 最多检查 8 个攻击者 × 72 个候选，固定上界为 576 次 AABB 射线测试，再为实际有候选的攻击者执行既有有界方块 raycast。候选和意图使用固定数组，不引入 map、goroutine、磁盘、网络等待或模型调用。

主要风险：

1. A-02 合入后编号、HUD 容量或 capture 顺序与当前预期不同；通过进入条件重新冻结，不在设计分支猜值。
2. 把旧 victim-only cooldown 直接复用为双向状态会改变受击者出手；通过两个字段和独立边界测试避免。
3. 同目标竞争在收集阶段直接写 cooldown 会破坏互杀；通过 raw intent 全收集后再 reservation。
4. 剑进入通用耐久表后会被采掘路径误扣；通过同源剑分类和采掘直接测试锁定。
5. 客户端从非确认事实触发反馈会产生假命中；通过单一 `CombatHit` 入口和负面触发矩阵锁定。

回退时可删除新 packet、客户端反馈、剑登记和战斗扩展，并恢复原 `authoritative-player-melee` 实现。协议已经升版的已发布构建不能安全降级到旧客户端，因此正常回退应发布下一协议版本，而不是复用旧版本号。玩家存档没有 schema 迁移；若存档已包含新剑，回退构建必须保留 ItemID 解码或显式拒绝，不能把未知物品静默改为空气。

## 被否决的附加能力

1. 攻击蓄力条或客户端攻击动画：没有权威输入需求，本行只需要确认反馈。
2. `CombatHit` 携带目标 ID、坐标和武器：当前 UI 不消费，增加隐私和 codec 面。
3. 把 hit marker 做成新纹理或 render pass：4 个现有 HUD quad 足够。
4. 把 sword durability 做成可调表：三把固定武器没有配置收益。
5. 剑破坏方块也扣耐久：用户已裁决只有成功实体命中扣 1。
6. 预先实现 hostile actor 接口：A-04 尚未进入实现，tagged value 和唯一 switch 已提供足够接点。

## 已决结论

本设计没有待定玩法项。A-02 合入后的绝对编号、最终 ABI 值和场景总数按明确的“读取当前 `main` 后 append/保持”规则导出，不属于待定产品决策。若实现调查发现固定 HUD 容量、A-04 接点、版本影响或文件范围不成立，必须先更新本设计和 OpenSpec 产物并重新取得裁决，不能在代码中静默扩大范围。
