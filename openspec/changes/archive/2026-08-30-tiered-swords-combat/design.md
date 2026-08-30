# Tiered Swords And Unified Combat Design

## Context

本 artifact 只把已批准的 `docs/superpowers/specs/2026-08-28-tiered-swords-unified-combat-design.md` 固化为实施设计，不重新选择玩法或架构。当前玩家近战由持续 primary action、3 格射线和玩家目标 cooldown 组成；夜行者近战由 server manager 的追逐目标与独立即时伤害循环组成。两条路径共享玩家受击保护状态，却不能在同一 tick 冻结跨类型 actor、仲裁同一 victim、支持玩家攻击夜行者或保证互击。

实施基线已由控制器冻结：协议 v31、player schema v8、chunk schema v9、world metadata v3、`companions.ai` schema v4、`hostile_mobs` schema v1、engine ABI v8、client ABI v10、benchmark scenario v19；`ItemBed=46`、`ItemIDMax=47`、`RecipeBed=16`、Play S→C 尾号 24、正式 capture 23 项、HUD 最大关闭/打开/容量为 96/257/267。`openspec/config.yaml` 的版本 prose 已知过时，本任务按代码、测试、根 `AGENTS.md` 与批准设计取真；Task 6 才同步该文件。

## Goals / Non-Goals

**Goals:**

- 以 append-only 值登记三把完好剑、三个损坏形态和三条可制作配方。
- 在 `sim` 内建立单一固定容量战斗阶段，统一 player/hostile 冻结、cooldown、victim reservation、伤害、击退、副作用和死亡边界。
- 让玩家用既有 primary action 的 3 格射线攻击 player 或 hostile，保持夜行者既有 3 点、1.8 格、20 tick 行为。
- 协议升至 v32，以私有 `CombatHit=25` 驱动本地 cue 和 6 帧 marker；Memory/TCP 共享同一模拟与 publication 路径。
- 保持存档 schema、Rust ABI、benchmark scenario 和固定 HUD 上传容量不变，并增加第 24 个正式 capture 场景。

**Non-Goals:**

- 不加入暴击、蓄力、动画、连击、格挡、附魔、范围攻击、双持、护甲、投射物、远程敌怪、状态效果、难度分支或伙伴战斗。
- 不重写夜行者生成、pathfinding、持久化、快照、灼烧、远离消失或腐肉掉落，只替换其临时近战结算 seam 并修正受影响死亡阶段顺序。
- 不引入 ECS、通用 actor interface、武器 registry struct、数据文件、配置倍率、通用动画系统或战斗专用队列。
- 不修改 Rust 生产代码、物理积分、碰撞、engine ABI、client ABI、benchmark workload、配置格式或任何二进制美术/音频资产。

## Decisions

### 1. 独立意图生产者，共享单一结算内核

player producer 继续使用冻结位置、yaw/pitch、3 格 AABB 射线和方块遮挡；hostile producer 继续使用 manager 已选择的追逐目标并重验 1.8 格水平距离。两者都只生成 tagged value intent，全部 raw intent 形成后再做一次跨类型 victim reservation 与 settlement。

否决让所有 actor 共享同一候选/射线系统：夜行者的目标来自 pathfinding manager，不需要玩家射线，强制共享会重写 A-04 已验证行为。也否决保留两个伤害循环只抽 helper：它不能保证同 tick 冻结、跨类型同 victim 唯一胜者、互击或统一 overflow。

### 2. Core 只提供稳定 kind 与固定 sword switch

`core.CombatTargetKind` 使用 append-only `player=1`、`hostile=2`，未知值拒绝。新增 ItemID 固定为：

| ID | Item | Damage | Max durability |
|---:|---|---:|---:|
| 47 | `ItemWoodenSword` | 4 | 59 |
| 48 | `ItemStoneSword` | 5 | 131 |
| 49 | `ItemIronSword` | 6 | 250 |
| 50 | `ItemBrokenWoodenSword` | 2 | 无 |
| 51 | `ItemBrokenStoneSword` | 2 | 无 |
| 52 | `ItemBrokenIronSword` | 2 | 无 |
| 53 | `ItemIDMax` | - | - |

六种 item stack limit 都为 1。空手和普通物品伤害同样为 2。`core` 以固定 switch 提供 kind 校验、`IsIntactSword`、`WeaponDamage`、最大耐久和完好→损坏映射；不创建 registry struct、不增加配置、不虚构 `DamageTool` 公共 API。

三条 recipe 追加在 `RecipeBed=16` 后：木/石/铁剑分别为 17/18/19，形状 `Width:1, Height:3, Mirror:true`，cells 为两份橡木木板/圆石/铁锭加一根木棍，产物数量 1 且满耐久。matcher 上界延伸到 `RecipeIronSword`；允许归一化整体横向平移，拒绝横放、倒放、错料和多料，不增加 `RecipeIDMax`。

### 3. 战斗身份与固定容量是值边界

`sim` 使用私有 tagged values：

```go
type combatActor struct {
	kind core.CombatTargetKind
	id   uint64
}
```

player ID 无损承载 `SessionID`，hostile ID 承载稳定 hostile ID。生产路径用 `[72]combatActorSnapshot` 与 `[72]combatIntent`，上限来自 8 名玩家加 64 只夜行者。测试 seam `advanceCombatWithLimits` 只允许降低逻辑 limit 来证明下一次 append overflow；生产始终固定 72/72，不加配置项。

每 tick 进入战斗阶段先清零 active player 的 `meleeSuppressedMining`。snapshot 或 intent append 超过 limit 时整个阶段返回失败：不截断、不结算已收集 intent、不提交 cooldown 递减，也不改变 health、velocity、inventory、durability、fatigue 或 hit facts。采用 fail closed 而非截断，因为截断会让玩家数、hostile 数或 append 顺序改变战斗结果。

### 4. Tick 顺序先冻结，后统一提交

权威顺序固定为：

```text
hostile spawn -> consume hostile actions -> hostile movement
build <=72 actor snapshots
decrement player/hostile attack and hurt cooldowns in snapshot copies
freeze hostile intents, then player intents (<=72)
commit decremented cooldown copies
hostile-first global victim reservation
settle accepted intents
hostile burn -> hostile deaths -> distant despawn -> player deaths
```

snapshot 构造与全部 intent append 成功后，四类正 cooldown 各减 1 并提交。player 拆成运行态 `attackCooldownTicks` 与 `hurtCooldownTicks`，reset/重连清零；hostile 复用 schema v1 既有 `attackCooldown`、`hurtCooldown`。player 成功命中设置自身 attack=10、目标 hurt=10，因此持续按住时第 1 tick 命中，第 11 tick 再次可命中。hostile 成功命中设置自身 attack=20、player hurt=20。

server manager 在目标进入 1.8 格后每 tick enqueue hostile action，不再读取上一份 `AttackCooldown` 预过滤；sim 在递减后执行唯一准入，保证 cooldown 1→0 的同 tick 可攻击。

### 5. Player mixed target 使用显式全序

合法候选必须在冻结 snapshot 中 active、存活、同维、不是自己，射线进入 player/hostile AABB 的距离不超过 3 格。目标全序为 `(ray distance, TargetKind, stable ID)`：距离最小优先，精确等距时 player=1 先于 hostile=2，再按无符号 ID。固体表面严格位于目标表面之前才阻挡；表面等距不阻挡；流体不阻挡。

producer 先选出最近目标，再检查该 snapshot 的 hurt cooldown。因此最近目标受保护时本次攻击失败，不能穿透到后方目标。候选与遮挡只读冻结位置、yaw、pitch、health、cooldown 和选中栏位身份，不依赖 map 遍历或 goroutine 顺序。

### 6. Reservation 保持 hostile-first 兼容并允许互击

raw intent 生产顺序即全序：hostile 按 ID 升序在前，player 按 `SessionID` 升序在后。reservation 用固定数组线性扫描已保留 victim；同 victim 只接受第一条，不使用 map 或排序分配。这个 hostile-first 顺序保持 A-04 已发布竞争语义；同 kind 最小 stable ID 胜。

loser 不设置 attack/hurt cooldown、不收 100 milli fatigue、不抑制采掘、不耗耐久、不发 hit。A→B 与 B→A 的 victim 不同，双方都接受；即使其中一方先被扣至零血，所有 accepted intent 仍完成，死亡随后结算。

### 7. Settlement 在完整重验后按固定次序提交

每条 accepted intent 先解析 live attacker/target、维度和 player 冻结 slot/item/count；任一不变量不成立时只丢该 intent，其他 intent 继续。验证后提交顺序为：

1. player 目标走既有 `applyDamage`，hostile 目标走其私有正伤害钳零入口；
2. 给目标现有 XZ velocity 加 0.35 水平击退，Y 不变；
3. 设置来源相关 attack/hurt cooldown；
4. player attacker 增加 100 milli fatigue 并设置本 tick 采掘抑制；
5. 冻结物品为完好剑时，对同 slot/item/count 的 live stack 恰减 1 耐久并标记 inventory dirty；
6. player attacker 追加一条领域 `sim.CombatHit`。

击退通常取 `targetPosition-attackerPosition` 的 XZ 单位向量；水平位置完全重合时改用 `LookDirection(attackerYaw, 0)` 的 XZ，禁止归一化零向量和 NaN/Inf。物理阶段已在战斗之前，velocity 从下一 tick 产生位移，不修改 Rust `StepInput`。

耐久扣减复用 sim-private `consumeToolDurabilityAt(actor, slot, expectedItem)`。采掘 wrapper 对完好剑直接返回 false；战斗只在 accepted hit 且冻结栏位身份重验成功后调用。耐久 1 的剑先按冻结完好伤害命中，再原子替换为对应 broken item、Count 1、Durability 0。所有失败路径和方块破坏都不磨损剑。

### 8. Death 阶段保证 burn/death/distant 语义

统一战斗后依次推进 hostile burn、`settleHostileDeaths`、distant despawn、player deaths。这样 health=1、burn 到期、`DistantTicks=599` 的 hostile 先走死亡/腐肉掉落，不会被无掉落 distant 移除吞掉。互杀双方的伤害、击退、cooldown 与剑耐久先成立；死亡掉落观察结算后的 inventory。

### 9. CombatHit 是最小私有权威事实

协议升为 v32，在 Play S→C 尾部追加 ID 25：

```text
ServerTick u64
Damage     u8
TargetKind u8
```

固定 10 bytes，little-endian。校验 `ServerTick > 0`、`Damage` 为 `1..core.MaxHealth`、kind 仅 player/hostile，拒绝截断、尾随、unknown kind、wrong state 与 ID 26。领域 `sim.CombatHit` 只含 Session、Damage、TargetKind；不含 tick、target ID、位置、武器、EventID 或文本。`TickResult.CombatHits` 按 attacker `SessionID` 稳定顺序，每 session 每 tick至多一条。

server 用 `result.Tick` 填 wire tick。publication 保持 inventory/crafting/furnace/chest mirror 在前，随后 `PlayerState`，最后才扫描并私发当前 session 的 combat facts；victim、旁观者、trusted observer 和 hostile attack target 不收到。任一 enqueue 失败沿用现有 session outbox-full 断开，健康 session 不受影响；不加重试、dedupe map 或战斗专用队列。

Memory/TCP parity 必须逐字段比较 player/hostile health、velocity、选中剑 durability/broken form、确认 payload 和每 transport 内单调 tick；不能依赖不编码 durability 的 `PlayerHash`。

### 10. Client 只消费严格递增确认

`cmd/mornlea/app` 持有独立 `combatFeedback{lastServerTick, remainingFrames}`，不复用全局 `application.serverTick`。只有 `CombatHit.ServerTick` 严格增大时才接受，播放一次 `CueCombatHit` 并把 marker 重置为 6；重复/陈旧消息静默忽略。输入、预测 ray、health 和 inventory 镜像都不能触发。

`CueCombatHit` 追加在现有 cue enum 尾部并复用预分配队列，参数固定为 1323 samples、520→180 Hz、amplitude 10500，little-endian PCM SHA-256 为 `17752cdda0232ebb88b0e6db1e39fa4a4889e5469bac0c28a07044b677710dae`。音频设备不可用无声降级。

marker 在现有 HUD pass 追加 4 个白色不透明 untextured quad：上下 `2×8`，左右 `8×2`，内缘距中心 4 design px，随 `hudScale` 缩放。只有 native renderer 返回 true 才减一帧；零 framebuffer、HUD prepare error、entity overflow 或 renderer false 不减。断线、菜单、新 session、`PlayerState.Reset` 和 capture 场景切换都 reset combat feedback；共享 `resetSessionOwnedState` 同时 reset hostile mirror。reset 后同 tick 随后的合法 hit 仍可重新武装。

当前 marker-off 关闭/打开态为 96/257 quad；marker-on 为 100/261，仍小于固定 267。保持 700 glyph、48-byte instance、13312-byte glyph offset、46912-byte 总容量、256-byte 对齐、warmed `Prepare` 零分配；不改 shader、pipeline、atlas cell 或 client ABI。

### 11. Capture 增加唯一正式场景

`sword-combat` 使用固定 time/camera，选中 `ItemIronSword{Durability:125}`，以合法 UUIDv4 远端玩家镜像展示权威受击和 0.35 击退关系，并显示 marker。`PinVolatile` 在收敛后、最终帧前重新 arm 6 帧窗口；`resetCapturePresentation` 清 combat feedback。

24 项完整顺序为：

```text
terrain-noon
hud-hotbar-health
hud-survival-feedback
avatar-nametag
inventory-crafting
workbench-crafting
chest-container
furnace-container
debug-panel
skylight-tunnel
block-light-room
torch-night
bed-night
materials-showcase
target-block-feedback
oak-grove
ai-companion
sword-combat
hostile-mob
water-surface-slope
main-menu
settings-menu
far-horizon
water-underwater
```

本变更只新增 `sword-combat.png`。若共享 HUD 代码使既有 PNG 变化，必须逐图归因并明确批准；不得批量接受或放宽双阈值。自动 producer 不创建或聚焦前台窗口。

## Data Ownership And Dependency Direction

- `internal/core` 拥有稳定 ItemID、RecipeID、combat kind 与纯值 sword helpers，不感知 sim/network/render。
- `internal/sim` 独占 actor live state、snapshot/intent、cooldown、reservation、damage、knockback、fatigue、durability 与领域 hit facts。
- `internal/server` 只生产 hostile manager action并投影 `TickResult`；不复制战斗规则。
- `internal/network` 与 `internal/network/tcp` 拥有 v32 packet registry、codec 和 trust-boundary 校验，不决定命中。
- `internal/audio`、`internal/render` 与 `internal/render/hud` 只提供 cue、item color 和 marker 呈现资源。
- `cmd/mornlea/app` 拥有 session-local feedback 生命周期；`cmd/mornlea/capture` 只经 `SceneApplication` 最小消费接口装配固定场景。
- Go 生产包不导入 WebGPU binding；GPU 仍由 Rust client 独占。跨 goroutine enqueue 成功后的 packet、事实和 slice 视为不可变。

## Compatibility And Migration

| Contract | Baseline | After change |
|---|---:|---:|
| Protocol | v31 | v32 |
| Player schema | v8 | v8 |
| Chunk schema | v9 | v9 |
| World metadata | v3 | v3 |
| `companions.ai` schema | v4 | v4 |
| `hostile_mobs` schema | v1 | v1 |
| Engine ABI | v8 | v8 |
| Client ABI | v10 | v10 |
| Benchmark scenario | v19 | v19 |

旧协议在 Play 前拒绝。现有 `ItemStack` 已编码 item ID、数量和耐久，新剑通过当前 player/chunk/companion codec round trip，不升级 schema。含新 ItemID 的存档不能由旧程序安全解释，不提供降级写入。player cooldown 为短期 session state；hostile 继续使用 schema v1 字段和上限 20。

实施顺序为 core/storage/durability seam → sim/server 统一结算 → network/publication/parity → app/audio/HUD/capture。Task 6 在全部实现与门禁完成后才 sync/archive delta，并同步根版本文档、`openspec/config.yaml`、README、progress、backlog 和 Discussion；本 change 建立阶段不修改这些长期入口。

## Risks / Trade-offs

- [72 容量实现被误写为截断] → 用可降低 limit 的 72/72 与下一次 append 测试证明整阶段无部分副作用。
- [hostile 20 tick 行为被统一成 player 10 tick] → cooldown 与 hurt protection 按来源设置，并锁定 player 第 1/11 tick 与 hostile 20 tick 边界。
- [manager 用陈旧 cooldown 提前过滤] → 范围内每 tick enqueue，sim 递减后唯一准入，并测试 cooldown 1→0 同 tick 命中。
- [受保护最近目标穿透] → 先按全序选最近 snapshot，再检查保护；测试后方未保护目标不被命中。
- [通用耐久路径误磨损剑] → 采掘 wrapper 明确排除完好剑，战斗只在 accepted hit 与冻结 slot/item 重验后调用私有 helper。
- [PlayerState reset 擦掉同 tick marker] → publication 固定 mirror/PlayerState 在 CombatHit 前，客户端 reset 后可接受随后更大 tick 的 hit。
- [把过时 266 当容量事实] → 以代码测试的 96/257/267 为基线，marker 后锁定 100/261，不扩大上传布局。
- [capture 收敛提前耗尽 marker] → `PinVolatile` 最终帧前 re-arm；只新增一张 golden 并逐图审核全部差异。
- [线性 reservation 为 O(n²)] → 最多 72 intent，固定上界可接受；不用 map 可保持零分配与确定顺序。

## Rollback

实现未发布时可按 Task 5→2 反序回退新场景/反馈、packet/publication、统一战斗和 sword registration，并恢复 A-04 临时 hostile melee 与原玩家 melee。协议 v32 一旦发布，正常回退必须发布下一协议版本，不能复用 v31/v32 造成对端误判。若存档已含新剑，回退构建必须保留新 ItemID 解码或明确拒绝，不能静默改为空气；没有 schema migration 可逆写入。

## Verification

任务级按 `tasks.md` 执行 red-green-refactor、focused race、archcheck、OpenSpec strict 和 `git diff --check`。整分支最终门禁由 Task 6 执行：

```bash
make rust
go test ./internal/core ./internal/storage ./internal/network ./internal/network/tcp ./internal/sim ./internal/server ./internal/audio ./internal/render ./internal/render/hud ./cmd/mornlea/app ./cmd/mornlea/capture -race -count=1
go test ./internal/archcheck -count=1
go vet ./...
test -z "$(gofmt -l .)"
go test ./... -race
make rust-check
make visual-check
scripts/agents/gates.sh
openspec validate --all --strict --no-interactive
git diff --check
```

## Affected Files

- `internal/core`: combat kind、items、recipes 与固定 sword helpers。
- `internal/sim`: unified combat、cooldown、knockback、durability、hunger、mining、death ordering 与 `TickResult`。
- `internal/server`: hostile manager seam、private publication 与 parity。
- `internal/network`、`internal/network/tcp`: v32、`CombatHit=25`、codec、fuzz 与 transcript。
- `internal/audio`、`internal/render`、`internal/render/hud`: cue、item colors、marker 与固定容量测试。
- `cmd/mornlea/app`: monotonic feedback、reset、render-frame consumption。
- `cmd/mornlea/capture`: minimal consumer interface、24-scene order、`sword-combat` 与 golden。
- 对应 `AGENTS.md`、本 active change 与 Task 6 最终同步的长期文档。

## Open Questions

无。玩法数值、容量、协议、兼容、场景顺序和反馈边界均已由批准设计决定。
