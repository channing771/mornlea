## Why

当前玩家与夜行者近战分别即时写入权威状态，无法在同一 tick 内统一冻结、跨类型仲裁或支持玩家攻击夜行者；游戏也缺少可制作、可磨损的分级近战武器和只由权威命中确认驱动的客户端反馈。A-04、A-05 已合入，当前协议 v31、物品/配方/消息尾号和固定 HUD/capture 容量已经重新冻结，因此本变更可以在不重写夜行者寻路与 Rust 边界的前提下交付一个可验证、可回退的统一战斗闭环。

## What Changes

- 新增木剑、石剑、铁剑及三个损坏形态，固定伤害分别为 4/5/6、耐久分别为 59/131/250，普通物品与损坏剑伤害保持 2；成功实体命中恰减 1 点耐久，最后一点先结算完好剑伤害再转损坏形态。
- 新增 recipe ID `17..19` 的三条纵向剑配方，产出满耐久单件武器；保留横向平移匹配，拒绝横放、倒放、错料和多料。
- 将最多 72 个 actor 快照和 72 条 raw intent 收敛到单一固定容量结算阶段；容量溢出时整个 tick 战斗 fail closed，不产生部分伤害、击退、冷却、耐久、疲劳或事件副作用。
- 玩家和夜行者保留各自意图生产规则，但统一 cooldown 准入、hostile-first victim reservation、互击、伤害、0.35 水平击退、死亡阶段和失败边界；玩家攻击支持 3 格内的 player/hostile mixed target 全序选择。
- 只有成功玩家实体命中增加 100 milli fatigue 并抑制本 tick 采掘；完好剑参与采掘不消耗耐久。
- **BREAKING** 协议从 v31 升为 v32，并在 Play S→C registry 尾部追加 ID 25 的 10-byte 私有 `CombatHit`；v31 及更早客户端在 Play 前按既有版本不匹配规则拒绝。
- 客户端只接受 `ServerTick` 严格递增的 `CombatHit`，据此播放固定原创 PCM cue 并显示 4 quad、持续 6 个成功呈现帧的 hit marker；输入、预测、health 或 inventory 镜像不得推断命中。
- 新增无窗口 `sword-combat` 场景，使正式 capture 从 23 项增为 24 项，并修正主规格中的过时 recipe、capture 和 HUD 容量口径。

## Goals

- 交付三档可制作、可持久化、可磨损的剑及损坏形态。
- 以单一有界、确定的 tagged intent 内核统一玩家与夜行者近战结算。
- 保持服务端权威和 Memory/TCP 业务一致，只用私有权威确认驱动本地反馈。
- 在不扩容的前提下完成 HUD、音频、物品颜色、capture 与 golden 验证闭环。

## Non-Goals

- 不做暴击、蓄力、挥剑动画、连击、格挡、附魔、范围攻击、双持、护甲、投射物、远程敌怪、状态效果、难度分支或伙伴战斗。
- 不改变夜行者生成、寻路、持久化、网络快照、灼烧、远离消失或腐肉掉落规则，只收编其近战结算 seam 和受影响的死亡阶段顺序。
- 不建设 ECS、通用 actor interface、插件式武器 registry、数据驱动伤害系统或通用动画系统。
- 不增加攻击键或输入位，继续复用 `PlayerInput.Mining` 表达持续 primary action。
- 不修改 Rust 物理积分、碰撞、engine ABI、client ABI、benchmark workload、配置格式，也不导入二进制美术或音频资产。

## Capabilities

### New Capabilities

- `tiered-swords-combat`：定义六个 append-only ItemID、三档剑伤害与耐久、统一战斗身份/容量/仲裁、击退、私有 `CombatHit` 及兼容矩阵。

### Modified Capabilities

- `authoritative-player-melee`：把玩家目标扩展为 player/hostile mixed target，并增加独立攻击冷却、全序选择、统一冻结与受保护最近目标不穿透。
- `authoritative-hostile-nightwalker`：让 manager 每 tick 冻结范围内意图，由 sim 唯一执行 cooldown 准入、统一仲裁与 burn/death/distant 顺序。
- `authoritative-mining`：只在成功 combat 结算后抑制本 tick 采掘，并明确完好剑采掘不磨损。
- `authoritative-hunger`：把近战疲劳边界收紧为成功结算的玩家实体命中，每次增加 100 milli fatigue。
- `tool-durability`：增加剑在成功命中时的原子耐久损耗、最后一点转损坏及全部失败路径无损耗规则。
- `authoritative-crafting`：以当前 `RecipeBed=16` 为尾部追加 recipe `17..19`，并规定剑配方形状、拒绝边界和满耐久产物。
- `local-audio-feedback`：增加只由严格递增私有 `CombatHit` 触发的固定原创 PCM cue。
- `survival-hud-presentation`：增加 4 quad、6 成功帧 hit marker，并锁定 100/261 不超过 267、稳定态零分配。
- `container-ui-presentation`：把最大打开态过时的 266 quad 修正为代码事实 257，并锁定加入 marker 后 261/267 且不扩容。
- `visual-verification`：新增 `sword-combat`，锁定 24 项正式清单、相邻顺序、状态夹具和 golden 审核。

## Impact

- 生产范围预计覆盖 `internal/core`、`internal/sim`、`internal/server`、`internal/network`、`internal/network/tcp`、`internal/audio`、`internal/render`、`internal/render/hud`、`cmd/mornlea/app` 与 `cmd/mornlea/capture`，以及对应局部指南和 OpenSpec 主规格。
- 协议 v32 是线上破坏性边界；`CombatHit=25` 只私发攻击者，Memory/TCP 复用同一模拟和 publication 路径。
- 玩家 schema v8、区块 schema v9、世界 metadata v3、`companions.ai` schema v4、`hostile_mobs` schema v1 均不升版。既有 `ItemStack` 已携带 item ID、数量与耐久；当前程序必须无损 round trip 新剑，旧程序不得解释含未知新 ItemID 的存档，也不提供降级写入。
- player attack/hurt cooldown 是会话运行态；hostile 复用 schema v1 既有 cooldown 字段。跨 goroutine 发布后的 packet、事件和切片保持不可变。
- 热路径固定上限为 72 个 actor snapshot、72 条 raw intent 和最多 568 次玩家 AABB 射线测试，不引入 map、goroutine、阻塞 I/O 或无界工作。
- HUD 最大关闭/打开态由 96/257 加 marker 后变为 100/261，仍使用固定 267 quad、700 glyph、48-byte instance、13312-byte glyph offset 和 46912-byte 总上传容量。
- engine ABI v8、client ABI v10、benchmark scenario v19 均不变化；没有 Rust 生产代码、ABI 或 benchmark 变更。

## Deferred And Abandoned

无。批准设计没有待定玩法数值、兼容性决策或新增范围。
