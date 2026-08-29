# 三级剑与统一战斗 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付木剑、石剑、铁剑及损坏形态，把玩家与夜行者近战收敛到一个有界确定的权威结算内核，并用私有 `CombatHit` 确认驱动客户端音效、hit marker 和正式视觉基线。

**Architecture:** `core` 以固定 switch 持有稳定物品/配方编号、剑数值和 tagged combat kind；`sim` 保留玩家射线与夜行者追逐目标两个意图生产者，但把最多 72 个 actor 快照和 72 条 raw intent 交给唯一 hostile-first reservation/settlement 内核。`network` 只传递成功玩家攻击的私有事实，`app` 只消费严格递增确认并复用既有 audio/HUD pass，Memory 与 TCP 继续走同一模拟和 publication 路径。

**Tech Stack:** Go 1.26、现有 Rust 1.97.1 engine/client 构建产物、Memory/TCP packet codec、wgpu HUD/audio bridge、OpenSpec 1.7.0。

**Spec:** `docs/superpowers/specs/2026-08-28-tiered-swords-unified-combat-design.md`

## Handoff Prerequisite

本计划不能在只存在于 `docs/A-03-combat-design` 的状态下交给实现者。规划会话必须先把批准设计和本计划作为唯一变更提交，经 docs-only PR/CI 合入 `main`；该 PR 不晋升或认领 backlog，不创建 OpenSpec change，也不修改功能代码。实现控制会话从更新后的 `main` 开始，并在 admission 前确认两个文档 blob 都存在。

## Global Constraints

- A-03 当前仍是「排队」；只有上述 docs-only PR 已合入 `main` 后，控制会话才能把它晋升为「就绪」，实现者再认领并从该最新 `main` 创建 isolation worktree。任何 Go、golden 或 active change 修改都不得先于该门禁。
- 实现从最新 `main` 重新冻结真实基线；若协议 v31、`ItemBed=46`、`RecipeBed=16`、Play S→C 尾号 24、capture 23 项或 HUD `96/257/267` 任一已漂移，停止实现，先修订设计并取得裁决。
- 冻结增量为协议 v31→v32、`CombatHit=25`、物品 47..52 / `ItemIDMax=53`、配方 17..19、capture 23→24；玩家 schema v8、区块 schema v9、世界 metadata v3、`companions.ai` v4、`hostile_mobs` v1、engine ABI v8、client ABI v10、benchmark scenario v19 均不变。
- 固定玩法值：普通攻击 2，木/石/铁剑伤害 4/5/6、耐久 59/131/250；玩家 attack/hurt cooldown 都是 10 tick；夜行者伤害 3、范围 1.8、attack/hurt cooldown 都是 20 tick；玩家射程 3；水平击退增量 0.35。
- actor snapshot 与 raw intent 容量都固定为 72；任一 overflow 使整个战斗阶段 fail closed，除清零 tick-local `meleeSuppressedMining` 外不得有 cooldown、health、velocity、durability、fatigue 或事件副作用。
- attacker 仲裁固定 hostile ID 升序在前、player `SessionID` 升序在后；同 victim 只接受首条，不同 victim 的互击全部成立，死亡在全部 accepted intent 后结算。
- 玩家 producer 复用 `PlayerInput.Mining`、`core.RaycastBlocks` 和 3 格射线；夜行者 producer 只消费 manager 已冻结的追逐目标并重验水平 1.8 格，不建设 ECS、actor interface、武器 registry 或数据文件。
- 完好剑只在成功实体命中后损耗 1；采掘显式排除完好剑，战斗必须重验冻结栏位与 ItemID，最后一点命中先按完好剑伤害成立再转损坏形态。
- `CombatHit` 固定 10 bytes：`ServerTick u64`、`Damage u8`、`TargetKind u8`；只发攻击者，并排在同 tick inventory/container 与 `PlayerState` 之后。
- 客户端不从 input、raycast、health 或 inventory 推断命中；只接受严格递增的独立 combat tick。marker 4 quad，设计长度 8 px、厚度 2 px、中心到每条短线内缘 4 px，持续 6 个成功呈现帧。
- `CueCombatHit` 固定 1323 samples、520→180 Hz、amplitude 10500，little-endian PCM SHA-256 为 `17752cdda0232ebb88b0e6db1e39fa4a4889e5469bac0c28a07044b677710dae`。
- 保持 `maxHotbarQuads=267`、engine/client ABI 和 benchmark scenario 不变；marker 后关闭/打开最大值必须是 100/261，warmed `Prepare` 继续零分配。
- 不修改 Rust 生产代码、不增加依赖、不导入二进制资产、不放宽 race/fuzz/golden/overflow 门禁；自动验证不得启动或聚焦前台窗口。
- 代码注释、GoDoc 和 Rust doc comment 使用中文且不得出现 `A-03`；规划标识只留在 backlog、OpenSpec、ledger 和本计划。
- 不修改 F-04 独占的 `docs/notes/lan-server.md`，也不触碰其它 worktree 或用户无关改动。
- 控制会话执行时先调用 `superpowers:using-git-worktrees` 和 `superpowers:subagent-driven-development`；每个 Task 使用 fresh implementer，再做独立 SPEC 与 QUALITY 双评审，验证证据和所有 `Ruling:` 写入 `openspec/changes/tiered-swords-combat/ledger.md`。

---

### Task 1: 准入、认领与 OpenSpec 基线冻结

**Files:**
- Modify: `docs/feature-backlog.md`
- Create: `openspec/changes/tiered-swords-combat/.openspec.yaml`
- Create: `openspec/changes/tiered-swords-combat/proposal.md`
- Create: `openspec/changes/tiered-swords-combat/design.md`
- Create: `openspec/changes/tiered-swords-combat/tasks.md`
- Create: `openspec/changes/tiered-swords-combat/ledger.md`
- Create: `openspec/changes/tiered-swords-combat/specs/tiered-swords-combat/spec.md`
- Create: `openspec/changes/tiered-swords-combat/specs/authoritative-player-melee/spec.md`
- Create: `openspec/changes/tiered-swords-combat/specs/authoritative-hostile-nightwalker/spec.md`
- Create: `openspec/changes/tiered-swords-combat/specs/authoritative-mining/spec.md`
- Create: `openspec/changes/tiered-swords-combat/specs/authoritative-hunger/spec.md`
- Create: `openspec/changes/tiered-swords-combat/specs/tool-durability/spec.md`
- Create: `openspec/changes/tiered-swords-combat/specs/authoritative-crafting/spec.md`
- Create: `openspec/changes/tiered-swords-combat/specs/local-audio-feedback/spec.md`
- Create: `openspec/changes/tiered-swords-combat/specs/survival-hud-presentation/spec.md`
- Create: `openspec/changes/tiered-swords-combat/specs/container-ui-presentation/spec.md`
- Create: `openspec/changes/tiered-swords-combat/specs/visual-verification/spec.md`

**Interfaces:**
- Consumes: 已包含本计划和批准设计 `docs/superpowers/specs/2026-08-28-tiered-swords-unified-combat-design.md` 的最新 `main` 代码/测试。
- Produces: strict-valid active change `tiered-swords-combat`，其 `tasks.md` 精确镜像本计划 Tasks 2–5，并以 `ledger.md` 承接 SDD 证据和裁决。

- [ ] **Step 1: 控制会话执行 admission gate**

读取 `docs/feature-backlog.md`。只有 planner/控制会话已经把 A-03 改为「就绪」才继续；仍为「排队」时停止，不创建分支或 change。

```bash
git switch main
git pull --ff-only
git status --short --branch
git cat-file -e HEAD:docs/superpowers/specs/2026-08-28-tiered-swords-unified-combat-design.md
git cat-file -e HEAD:docs/superpowers/plans/2026-08-28-tiered-swords-unified-combat.md
rg '^\| A-03 \|' docs/feature-backlog.md
```

Expected: 两个 `git cat-file` 均退出 0，A-03 行状态为「就绪」，工作树没有待覆盖的相关改动；任一文档不存在都停止，不从规划分支或绝对路径旁路读取。

- [ ] **Step 2: 认领 A-03 并单独提交规划事实**

把 A-03 行改成「已认领」，认领人写 `opencode-implementer @ feat/A-03-tiered-swords-combat`，备注明确独占本计划列出的 core/sim/network/server/render/app/capture/OpenSpec/基线文档，且排除 `docs/notes/lan-server.md`。同步 Discussion 状态评论和正文镜像：

```bash
git add docs/feature-backlog.md
git commit -m "docs: claim A-03 tiered swords combat"
gh api graphql -f query='mutation($b:String!){ addDiscussionComment(input:{discussionId:"D_kwDOToJS8M4Aou6G", body:$b}){ comment { id } } }' -F b=$'【状态变更】A-03 → 已认领\n\n认领人：opencode-implementer @ feat/A-03-tiered-swords-combat\n独占：本实施计划列出的 core/sim/network/server/render/app/capture/OpenSpec/基线文档；排除 docs/notes/lan-server.md。'
python3 scripts/agents/refresh-discussion.py
python3 scripts/agents/refresh-discussion.py --update
```

Expected: GraphQL 返回 comment ID，dry-run 与 update 均退出 0。

- [ ] **Step 3: 创建 isolation worktree 并记录基线 SHA**

调用 `superpowers:using-git-worktrees`，从包含认领提交的最新 `main` 创建 `feat/A-03-tiered-swords-combat`。在新 worktree 运行：

```bash
git status --short --branch
git rev-parse HEAD
make rust
go test ./internal/archcheck -run 'TestBaselineVersionsMatchCode|TestClientCommandSubpackageDependencyDirections' -count=1
```

Expected: tracked-clean；记录的 SHA 进入 `ledger.md` 的 `Baseline` 节。

- [ ] **Step 4: 重新冻结 append-only 值和场景/HUD 上限**

从代码而非历史计划核对：

```bash
rg 'ProtocolVersion uint32|ItemBed|ItemIDMax|RecipeBed|maxHotbarQuads|openWant != 257|closedWant != 96' internal cmd
go test ./internal/core ./internal/network ./internal/render/hud ./cmd/mornlea/capture -run 'Test(ItemIDsAppendOnly|RegisteredRecipeCellsStayInsideShapeBounds|ProtocolVersionPinned|HostileMessageIDsAreFrozen|HotbarLayoutStaysWithinFixedCapacity|CaptureSceneOrderAndAICompanionDeterminism)$' -count=1
```

Expected: v31、item 尾部 46/47、recipe 尾部 16、S→C 尾号 24、23 个场景、HUD 96/257/267。任一不同都停止并先更新批准设计，不能在 change 中静默改数字。

- [ ] **Step 5: 用批准设计生成完整 OpenSpec change**

调用 `openspec-propose` skill 创建 `tiered-swords-combat`，`.openspec.yaml` 使用仓库现行 `spec-driven` schema。`proposal.md` 必须写明目标、非目标、协议 v32、存档兼容、固定容量和无 Rust/ABI/benchmark 变化；`design.md` 以批准设计为来源，不重新决策。

delta specs 至少包含这些可判定场景：

```text
tiered-swords-combat: 六个 ItemID、三档伤害/耐久、72/72 fail-closed、hostile-first reservation、互击、击退、CombatHit 私有确认与兼容矩阵
authoritative-player-melee: mixed player/hostile target 全序、3 格遮挡/流体、10 tick attack/hurt cooldown、受保护最近目标不穿透
authoritative-hostile-nightwalker: manager 每 tick 冻结范围内意图、sim 唯一 cooldown 准入、20 tick 行为、burn→death→distant
authoritative-mining: 成功 combat 才抑制本 tick 采掘，完好剑采掘不耗耐久
authoritative-hunger: 只有成功玩家实体命中增加 100 milli fatigue
tool-durability: 成功命中恰减 1、最后一点转损坏、所有失败路径不耗损
authoritative-crafting: 当前 bed=16，剑 recipe=17..19，三列平移可匹配，横放/倒放/错料/多料拒绝
local-audio-feedback: 只由严格递增 CombatHit 触发固定 PCM cue
survival-hud-presentation: 4 quad marker、6 成功帧、100/261≤267、零分配
container-ui-presentation: 把过时 266 quad 修正为代码事实 267，加入 marker 后仍不扩容
visual-verification: 24 项清单和 ai-companion→sword-combat→hostile-mob→water-surface-slope 顺序
```

- [ ] **Step 6: 写 tasks 和 ledger 执行边界**

`tasks.md` 只包含可在 archive 前完成的 Tasks 2–5 与验证 checkbox；sync/archive/PR/CI/cleanup 留在本计划 Task 6 的 integration runbook。`ledger.md` 至少包含：

```markdown
# Ledger

## Baseline

## Rulings

## Task Reviews

## Validation Evidence

## Deferred And Abandoned
```

每条证据写命令、exit、摘要和对应 SHA；每轮评审写 SPEC/QUALITY 结论，裁决格式固定为 `Ruling: 决定 — 理由 — 原问题`，三个位置都替换成该轮真实内容。

- [ ] **Step 7: strict validate、双评审并提交 Task 1**

```bash
openspec status --change tiered-swords-combat
openspec validate --all --strict --no-interactive
git diff --check
```

Expected: strict 全绿，change 无空 Requirement/Scenario、无待定值。完成 fresh SPEC/QUALITY 双评审并把结论写入 ledger 后：

```bash
git add openspec/changes/tiered-swords-combat
git commit -m "docs: propose tiered swords combat"
```

### Task 2: 物品、配方、持久兼容与耐久 seam

**Files:**
- Create: `internal/core/combat.go`
- Create: `internal/core/combat_test.go`
- Modify: `internal/core/item.go`
- Modify: `internal/core/item_test.go`
- Modify: `internal/core/bed_test.go`
- Modify: `internal/core/recipe.go`
- Modify: `internal/core/recipe_test.go`
- Modify: `internal/core/recipe_shape_internal_test.go`
- Modify: `internal/storage/player_codec_test.go`
- Modify: `internal/storage/chunk_chest_test.go`
- Modify: `internal/storage/chunk_drop_test.go`
- Modify: `internal/storage/companion_codec_test.go`
- Modify: `internal/sim/mining.go`
- Modify: `internal/sim/mining_test.go`
- Modify: `openspec/changes/tiered-swords-combat/tasks.md`
- Modify: `openspec/changes/tiered-swords-combat/ledger.md`

**Interfaces:**
- Consumes: current `ItemStack.Valid`, `ItemStackLimit`, `ItemMaxDurability`, `ItemBrokenForm`, `RecipePattern`, `MatchCraftingGrid`, player/chunk/companion stack codecs and `consumeToolDurability`.
- Produces: `core.CombatTargetKind`, `CombatTargetPlayer`, `CombatTargetHostile`, `CombatTargetKind.Valid()`, `core.IsIntactSword(ItemID) bool`, `core.WeaponDamage(ItemID) int32`, sword ItemIDs/recipes, and sim-private `consumeToolDurabilityAt(*actorState, uint8, core.ItemID) bool` for Task 3.

- [ ] **Step 1: 写 combat kind 和 item registry RED tests**

在 `combat_test.go` 钉死 player=1、hostile=2、0/3 非法。在 `item_test.go` 表驱动断言六个 ID 47..52、`ItemIDMax=53`、stack limit 1、完好剑耐久 59/131/250、broken mapping、完好剑 damage 4/5/6、其它物品和损坏剑 damage 2，并更新所有以旧 `ItemIDMax` 为界的穷举断言。`bed_test.go` 保持 `ItemBed=46`，只把哨兵断言和说明从 47 更新为 53。

```bash
go test ./internal/core -run 'Test(CombatTargetKind|Sword|ItemID)' -count=1
```

Expected: FAIL，缺少 combat kind、剑常量和 helper。

- [ ] **Step 2: 实现最小 core tagged kind 与固定剑 switch**

`internal/core/combat.go` 使用稳定 append-only 值：

```go
package core

// CombatTargetKind 是战斗目标的稳定身份类别。
type CombatTargetKind uint8

const (
	CombatTargetPlayer CombatTargetKind = iota + 1
	CombatTargetHostile
)

// Valid 报告类别能否进入权威战斗与线上确认。
func (kind CombatTargetKind) Valid() bool {
	return kind == CombatTargetPlayer || kind == CombatTargetHostile
}
```

在 `ItemBed` 后、`ItemIDMax` 前依次追加 `ItemWoodenSword`、`ItemStoneSword`、`ItemIronSword`、`ItemBrokenWoodenSword`、`ItemBrokenStoneSword`、`ItemBrokenIronSword`。扩展三个现有 item switch，并添加：

```go
func IsIntactSword(item ItemID) bool {
	switch item {
	case ItemWoodenSword, ItemStoneSword, ItemIronSword:
		return true
	default:
		return false
	}
}

func WeaponDamage(item ItemID) int32 {
	switch item {
	case ItemWoodenSword:
		return 4
	case ItemStoneSword:
		return 5
	case ItemIronSword:
		return 6
	default:
		return 2
	}
}
```

不要增加 registry struct、配置、显示名表或公共 `DamageTool`。

- [ ] **Step 3: 运行 core item GREEN check**

```bash
gofmt -w internal/core/combat.go internal/core/combat_test.go internal/core/item.go internal/core/item_test.go
go test ./internal/core -run 'Test(CombatTargetKind|Sword|ItemID|Durability|Broken)' -count=1
```

Expected: PASS。

- [ ] **Step 4: 写三条 shape recipe RED tests**

为 recipe 17/18/19 逐条断言纵向 `material, material, stick`、三列横向平移、满耐久产物；横放、倒放、错误材料和任意多余材料都拒绝。把未知 ID 起点和注册表枚举上界从旧尾部改为 `RecipeIronSword+1`。

```bash
go test ./internal/core -run 'Test.*SwordRecipe' -count=1
```

Expected: FAIL，缺少 recipe 常量/模式。

- [ ] **Step 5: 追加 recipe 17..19 并延伸 matcher 上界**

在 `RecipeBed` 后追加三个常量；每条 `RecipePattern` 都是 `Width:1, Height:3, Mirror:true`，`Cells` 为两份对应材料加一根 `ItemStick`，输出 Count 1 和对应满耐久。把 `MatchCraftingGrid` 循环上界从 `RecipeBed` 改为 `RecipeIronSword`，把“固定 16 条”注释改为 19 条，不增加 `RecipeIDMax`。

```go
case RecipeWoodenSword:
	return RecipePattern{
		Width: 1, Height: 3, Mirror: true,
		Cells: [CraftingGridSlots]ItemID{
			ItemOakPlanks, ItemNone, ItemNone,
			ItemOakPlanks, ItemNone, ItemNone,
			ItemStick, ItemNone, ItemNone,
		},
		Output: ItemStack{Item: ItemWoodenSword, Count: 1, Durability: 59},
	}, true
```

石剑使用 `ItemCobblestone`/131，铁剑使用 `ItemIronIngot`/250。

- [ ] **Step 6: 写四条当前 schema sword round-trip tests**

分别新增：

```text
TestPlayerCodecCurrentSchemaRoundTripsSwordItems
TestChunkCodecRoundTripsSwordItemsInChests
TestChunkCodecRoundTripsSwordItemDrops
TestCompanionCodecCurrentSchemaRoundTripsSwordItems
```

每条覆盖六个 ID、磨损中的完好剑和耐久为零的损坏形态；在既有 invalid item cases 加 `core.ItemIDMax`。不要改 shared binary/golden fixtures，也不要升级 schema。

```bash
go test ./internal/storage -run 'Test.*SwordItems' -count=1
```

Expected: 在 core 登记完成后直接 PASS；若失败，只修复现有 codec 的通用 ItemID 校验，不增加剑专用 wire 分支。

- [ ] **Step 7: 写完好剑采掘不耗耐久 RED test**

`TestMiningIntactSwordsDoNotConsumeDurability` 对三把完好剑分别成功采掘 dirt，断言方块移除、完整 stack 不变且没有因耐久产生额外 inventory dirty。既有镐、锄、作物×锄头和伙伴采掘测试保持原断言。

```bash
go test ./internal/sim -run 'TestMiningIntactSwordsDoNotConsumeDurability' -count=1
```

Expected: FAIL，当前 `consumeToolDurability` 会磨损所有有耐久上限物品。

- [ ] **Step 8: 提取指定栏位/身份的原子 durability helper**

在 `mining.go` 保留现有 wrapper，并提取：

```go
func consumeToolDurabilityAt(actor *actorState, slot uint8, expected core.ItemID) bool {
	if slot >= core.HotbarSlots {
		return false
	}
	stack := actor.inventory.Hotbar.Slots[slot]
	if stack.Item != expected || stack.Count != 1 {
		return false
	}
	if _, ok := core.ItemMaxDurability(stack.Item); !ok {
		return false
	}
	if stack.Durability > 1 {
		stack.Durability--
		actor.inventory.Hotbar.Slots[slot] = stack
		return true
	}
	broken, ok := core.ItemBrokenForm(stack.Item)
	if !ok {
		return false
	}
	actor.inventory.Hotbar.Slots[slot] = core.ItemStack{Item: broken, Count: 1}
	return true
}
```

`consumeToolDurability` 读取 selected stack，先对 `core.IsIntactSword(stack.Item)` 返回 false，其余路径调用 `consumeToolDurabilityAt(actor, selected, stack.Item)`。Task 3 只能在 accepted player hit 的冻结身份重验后调用此 helper。

- [ ] **Step 9: focused race、双评审并提交 Task 2**

```bash
gofmt -w internal/core/combat.go internal/core/combat_test.go internal/core/item.go internal/core/item_test.go internal/core/bed_test.go internal/core/recipe.go internal/core/recipe_test.go internal/core/recipe_shape_internal_test.go internal/storage/player_codec_test.go internal/storage/chunk_chest_test.go internal/storage/chunk_drop_test.go internal/storage/companion_codec_test.go internal/sim/mining.go internal/sim/mining_test.go
go test ./internal/core ./internal/storage ./internal/sim -race -count=1
go test ./internal/archcheck -count=1
openspec validate --all --strict --no-interactive
git diff --check
```

Expected: PASS。更新 tasks/ledger，完成 fresh SPEC/QUALITY 双评审后：

```bash
git add internal/core/combat.go internal/core/combat_test.go internal/core/item.go internal/core/item_test.go internal/core/bed_test.go internal/core/recipe.go internal/core/recipe_test.go internal/core/recipe_shape_internal_test.go internal/storage/player_codec_test.go internal/storage/chunk_chest_test.go internal/storage/chunk_drop_test.go internal/storage/companion_codec_test.go internal/sim/mining.go internal/sim/mining_test.go openspec/changes/tiered-swords-combat/tasks.md openspec/changes/tiered-swords-combat/ledger.md
git commit -m "feat: register tiered swords"
```

### Task 3: 统一冻结、仲裁与战斗结算

**Files:**
- Modify: `internal/sim/combat.go`
- Delete: `internal/sim/hostile_melee.go`
- Modify: `internal/sim/player.go`
- Modify: `internal/sim/hostile.go`
- Modify: `internal/sim/hostile_action.go`
- Modify: `internal/sim/command.go`
- Modify: `internal/sim/engine_step.go`
- Modify: `internal/sim/hunger.go`
- Modify: `internal/sim/mining.go`
- Modify: `internal/sim/combat_test.go`
- Create: `internal/sim/combat_capacity_test.go`
- Create: `internal/sim/combat_resolution_test.go`
- Create: `internal/sim/combat_weapon_test.go`
- Create: `internal/sim/combat_knockback_test.go`
- Modify: `internal/sim/combat_exhaustion_test.go`
- Modify: `internal/sim/hostile_combat_test.go`
- Modify: `internal/sim/hostile_lifecycle_test.go`
- Modify: `internal/sim/mining_test.go`
- Modify: `internal/server/hostile_manager.go`
- Modify: `internal/server/hostile_manager_test.go`
- Modify: `openspec/changes/tiered-swords-combat/tasks.md`
- Modify: `openspec/changes/tiered-swords-combat/ledger.md`

**Interfaces:**
- Consumes: Task 2 `core.CombatTargetKind`, `core.IsIntactSword`, `core.WeaponDamage`, `consumeToolDurabilityAt`; existing `player.applyDamage`, `core.RaycastBlocks`, `physics.PlayerBounds`, `LookDirection`, `applyExhaustion`, hostile action target and death/drop paths.
- Produces: `sim.CombatHit{Session SessionID, Damage uint8, TargetKind core.CombatTargetKind}`, `TickResult.CombatHits []CombatHit`, source-specific cooldowns, unified bounded combat settlement consumed by Task 4.

- [ ] **Step 1: 写固定上限、seam overflow 和 cooldown RED tests**

增加 72 actor/72 raw intent 成功测试，再用可构造的双 actor/双 intent 夹具分别调用 `actorLimit=1` 和 `intentLimit=1`，证明下一次 append overflow。不要要求生产上限无法产生的第 73 个 actor 或 intent。overflow 前后比较玩家/hostile health、velocity、四类 cooldown、inventory、fatigue 和 `TickResult.CombatHits`；唯一允许变化是所有 active player 的 `meleeSuppressedMining=false`。另钉死玩家持续按住第 1/11 tick 和夜行者 20 tick 边界。

```bash
go test ./internal/sim -run 'TestCombat(Snapshot|Intent|PlayerCooldown|HostileCooldown|Cooldowns)' -count=1
```

Expected: FAIL，当前只有 `[8]meleeIntent`、玩家 victim cooldown 和独立 hostile loop。

- [ ] **Step 2: 拆分 player cooldown 并定义领域出口**

把 `meleeCooldownTicks` 替换为两个非持久字段，并在 `beginReset` 清零：

```go
attackCooldownTicks uint8
hurtCooldownTicks   uint8
```

在 `command.go` 增加：

```go
type CombatHit struct {
	Session    SessionID
	Damage     uint8
	TargetKind core.CombatTargetKind
}
```

在现有 `TickResult` 中把 `CombatHits []CombatHit` 放在 `Craftings` 后、`Tick` 前，其它字段及顺序不动。不要给领域事实加 tick、target ID、位置或武器。

- [ ] **Step 3: 建立固定值 snapshot/intent 形状和测试 limit seam**

在 `combat.go` 用 private tagged values，不加 interface：

```go
const (
	maxCombatActors  = 72
	maxCombatIntents = 72
)

type combatActor struct {
	kind core.CombatTargetKind
	id   uint64
}

type combatActorSnapshot struct {
	actor           combatActor
	dimension       core.DimensionID
	position        mgl32.Vec3
	yaw             float32
	pitch           float32
	health          uint8
	attackCooldown  uint8
	hurtCooldown    uint8
	attacking       bool
	targetSession   SessionID
	selectedSlot    uint8
	selectedItem    core.ItemID
}

type combatIntent struct {
	attacker         combatActor
	target           combatActor
	damage           int32
	distance         float32
	attackerPosition mgl32.Vec3
	targetPosition   mgl32.Vec3
	attackerYaw      float32
	selectedSlot     uint8
	selectedItem     core.ItemID
}

func (engine *Engine) advanceCombat(result *TickResult) {
	engine.advanceCombatWithLimits(result, maxCombatActors, maxCombatIntents)
}

func (engine *Engine) advanceCombatWithLimits(result *TickResult, actorLimit, intentLimit int) bool
```

`advanceCombatWithLimits` 只用于包内边界测试；生产调用固定 72/72，不加配置项。

- [ ] **Step 4: 写 mixed target 与冻结语义 RED tests**

在 `combat_test.go` 覆盖 `(ray distance, TargetKind, stable ID)` 全序、player 等距优先于 hostile、同 kind 最小 ID、非零 pitch 命中、固体严格在目标表面前才阻挡、表面等距不阻挡、流体穿透、最近受保护目标不穿透到后方，以及冻结后改变位置/yaw/pitch/health/栏位不改写目标选择或 intent 中的距离和伤害身份。

```bash
go test ./internal/sim -run 'TestPlayerCombat(Target|Protected|Frozen|Ray)' -count=1
```

Expected: FAIL，当前 producer 只扫描玩家。

- [ ] **Step 5: 写 hostile-first reservation、互杀和 loser 无副作用 RED tests**

`combat_resolution_test.go` 覆盖：hostile 与 player 同打一个 victim 时 hostile 胜；同 kind 最小 stable ID 胜；loser 不写 attack/hurt cooldown、不收 fatigue、不抑制采掘、不耗耐久、不发 hit；A→B/B→A 和玩家↔hostile 不同 victim 互杀都完整成立后才死亡。

```bash
go test ./internal/sim -run 'TestCombat(Reservation|Mutual|Loser)' -count=1
```

Expected: FAIL，尚无全局 reservation。

- [ ] **Step 6: 写伤害、副作用与击退 RED tests**

`combat_weapon_test.go` 覆盖 empty/ordinary/broken=2、wood=4、stone=5、iron=6、最后一点转损坏、玩家攻击 hostile；miss、遮挡、超距、cooldown、目标保护、reservation loser 和 frozen slot mismatch 都不耗耐久、不收 fatigue、不抑制采掘、不发 hit。保留既有工具和作物×锄头豁免测试。

`combat_knockback_test.go` 断言 XZ 方向单位化后乘 0.35、保留 Y velocity；水平重合时只用 attacker yaw 的 `LookDirection(yaw, 0)` XZ，所有分量有限。

```bash
go test ./internal/sim -run 'Test(PlayerCombatWeapon|CombatReservationLoser|Combat.*Mining|Combat.*Exhaustion|CombatKnockback)' -count=1
```

Expected: FAIL，统一 settlement 和击退尚未实现。

- [ ] **Step 7: 实现 snapshot、producer、reservation 与原子 settlement**

严格按以下事务顺序写 `advanceCombatWithLimits`：

```text
clear player meleeSuppressedMining
build <= actorLimit snapshots into [72]combatActorSnapshot; overflow => false
decrement attack/hurt cooldowns only in snapshot copies
append hostile intents by hostile ID, then player intents by SessionID into [72]combatIntent; overflow => false
commit decremented cooldown copies back to live actors
reserve victims in current hostile-first order
settle accepted intents in that same order
return true
```

player producer 只用冻结 snapshot 的位置、yaw 和 pitch 计算 `LookDirection` 并选择最近 tagged actor，再检查该目标 snapshot 的 `hurtCooldown`，因此受保护最近目标不会穿透。hostile producer 只解析 `targetSession`，重验 active/alive/same-dimension 和 `horizontalDistanceSq <= hostileAttackRangeSquared`。把 hostile 伤害/range 常量移入 `combat.go`，删除 `hostile_melee.go` 的平行结算函数。

producer 本身按 hostile ID、player SessionID 生成全序，因此 reservation 不再排序；对最多 72 条 intent 用固定数组线性扫描已保留 victim，命中同 victim 就丢弃当前条，不使用 map。

每条 accepted intent 在任何 live 写入前完整解析 attacker、target 和玩家冻结栏位身份；失败只丢该 intent。成功提交顺序固定为：

```text
apply target damage
add horizontal knockback
set source attack/hurt cooldowns
for player attacker: apply 100 milli exhaustion and suppress mining
for frozen intact sword: consumeToolDurabilityAt and mark inventory dirty
append sim.CombatHit
```

hostile 增加 private `applyDamage(damage int32)`，只做正伤害和钳零；玩家继续调用已有 `applyDamage`。玩家成功设置 attack=10；目标 hurt 由来源决定，player source=10、hostile source=20。最后一点剑耐久在 damage 后转 broken，死亡掉落读取转化后的 inventory。

```go
func combatKnockback(from, to mgl32.Vec3, yaw float32) mgl32.Vec3 {
	delta := mgl32.Vec3{to.X() - from.X(), 0, to.Z() - from.Z()}
	if delta.LenSqr() == 0 {
		look := LookDirection(yaw, 0)
		delta = mgl32.Vec3{look.X(), 0, look.Z()}
	}
	return delta.Normalize().Mul(0.35)
}
```

对目标 live velocity 做加法而非覆盖。

- [ ] **Step 8: 运行统一结算 GREEN checks**

```bash
go test ./internal/sim -run 'Test(CombatReservation|CombatMutual|CombatLoser|PlayerCombatWeapon|Combat.*Mining|Combat.*Exhaustion|CombatKnockback)' -count=1
```

Expected: PASS。

- [ ] **Step 9: 重排 hostile lifecycle 与 manager cooldown 边界**

把 `advanceHostiles` 收窄为 spawn→action→movement。在 `Engine.Step` 订阅收敛后接：

```go
engine.advanceHostiles(pending)
engine.advanceCombat(&result)
engine.advanceHostileBurn(engine.worldTime.Load())
engine.settleHostileDeaths(pending)
engine.advanceHostileDistant()
engine.settleDeaths(pending)
```

新增 burn health=1、distant=599 的同 tick 用例，证明先掉腐肉再移除。`hostile_manager.go` 删除 `mob.AttackCooldown == 0` 前置过滤；范围内每 tick enqueue action，sim 递减后唯一准入。`AttackCooldown=1` 的 manager 集成测试必须在同 tick 命中。

同步把 `hostile_action.go` 和 `hunger.go` 中指向旧结算函数的注释改为 `advanceCombat`，避免删除实现后留下失效标识符。

- [ ] **Step 10: 验证 CombatHits 稳定出口**

增加成功 player hits 按 `SessionID` 升序、每 session 每 tick 至多一条的测试；hostile attack 不产生 `CombatHit`。运行：

```bash
go test ./internal/sim -run 'TestCombatHits' -count=1
go test ./internal/server -run 'TestHostileChase' -count=1
```

Expected: PASS。

- [ ] **Step 11: focused race、双评审并提交 Task 3**

```bash
gofmt -w internal/sim/combat.go internal/sim/player.go internal/sim/hostile.go internal/sim/hostile_action.go internal/sim/command.go internal/sim/engine_step.go internal/sim/hunger.go internal/sim/mining.go internal/sim/combat_test.go internal/sim/combat_capacity_test.go internal/sim/combat_resolution_test.go internal/sim/combat_weapon_test.go internal/sim/combat_knockback_test.go internal/sim/combat_exhaustion_test.go internal/sim/hostile_combat_test.go internal/sim/hostile_lifecycle_test.go internal/sim/mining_test.go internal/server/hostile_manager.go internal/server/hostile_manager_test.go
go test ./internal/sim ./internal/server -race -count=1
go test ./internal/archcheck -count=1
openspec validate --all --strict --no-interactive
! rg -n 'advancePlayerMelee|advanceHostileMelee|damageHostileTarget' internal/sim --glob '*.go' --glob '!*_test.go'
git diff --check
```

Expected: PASS。确认生产代码中不再存在 `advancePlayerMelee`、`advanceHostileMelee` 或 `damageHostileTarget`。更新 tasks/ledger，双评审后：

```bash
git add internal/sim/combat.go internal/sim/hostile_melee.go internal/sim/player.go internal/sim/hostile.go internal/sim/hostile_action.go internal/sim/command.go internal/sim/engine_step.go internal/sim/hunger.go internal/sim/mining.go internal/sim/combat_test.go internal/sim/combat_capacity_test.go internal/sim/combat_resolution_test.go internal/sim/combat_weapon_test.go internal/sim/combat_knockback_test.go internal/sim/combat_exhaustion_test.go internal/sim/hostile_combat_test.go internal/sim/hostile_lifecycle_test.go internal/sim/mining_test.go internal/server/hostile_manager.go internal/server/hostile_manager_test.go openspec/changes/tiered-swords-combat/tasks.md openspec/changes/tiered-swords-combat/ledger.md
git commit -m "feat: unify authoritative melee combat"
```

### Task 4: CombatHit 协议、私有发布与 Memory/TCP parity

**Files:**
- Create: `internal/network/message_combat.go`
- Create: `internal/network/message_combat_test.go`
- Modify: `internal/network/packet.go`
- Modify: `internal/network/packet_test.go`
- Modify: `internal/network/worldtime_test.go`
- Modify: `internal/network/registry.go`
- Modify: `internal/network/registry_test.go`
- Modify: `internal/network/codec_server.go`
- Modify: `internal/network/codec_golden_test.go`
- Modify: `internal/network/codec_fuzz_test.go`
- Modify: `internal/network/drop_test.go`
- Modify: `internal/network/place_block_succeeded_test.go`
- Modify: `internal/network/message_companion_test.go`
- Modify: `internal/network/message_hostile_test.go`
- Modify: `internal/network/tcp/transport_consistency_test.go`
- Modify: `internal/server/publication.go`
- Create: `internal/server/combat_hit_publication_test.go`
- Modify: `internal/server/player_melee_parity_test.go`
- Create: `internal/server/sword_combat_parity_test.go`
- Modify: `cmd/mornlea/app/app_protocol_test.go`
- Modify: `cmd/mornlea-server/main_test.go`
- Modify: `AGENTS.md`
- Modify: `openspec/changes/tiered-swords-combat/tasks.md`
- Modify: `openspec/changes/tiered-swords-combat/ledger.md`

**Interfaces:**
- Consumes: Task 2 `core.CombatTargetKind.Valid()` and Task 3 `TickResult.CombatHits` sorted by attacker session.
- Produces: `network.CombatHit{ServerTick uint64, Damage uint8, TargetKind core.CombatTargetKind}`, protocol v32 Play S→C ID 25, private ordered publication used by Task 5.

- [ ] **Step 1: 写 wire shape、值域和 registry RED tests**

`message_combat_test.go` 钉死：protocol 32、S→C ID 25、ID 26 unknown、10-byte round trip、tick `0x0102030405060708`/damage 6/hostile kind 2 的 little-endian payload `08070605040302010602`；拒绝 tick 0、damage 0/21、kind 0/3、所有截断、任意尾随和 wrong state。

```bash
go test ./internal/network -run 'TestCombatHit' -count=1
```

Expected: FAIL，`network.CombatHit` 不存在。

- [ ] **Step 2: 实现固定 10-byte message 与 Validate**

```go
package network

import (
	"fmt"

	"github.com/channing771/mornlea/internal/core"
)

const combatHitWireBytes = 10

type CombatHit struct {
	ServerTick uint64
	Damage     uint8
	TargetKind core.CombatTargetKind
}

func (CombatHit) serverMessage() {}
func (CombatHit) serverPacket()  {}

func (hit CombatHit) Validate() error {
	if hit.ServerTick == 0 {
		return fmt.Errorf("network: combat hit server tick is zero")
	}
	if hit.Damage == 0 || hit.Damage > core.MaxHealth {
		return fmt.Errorf("network: combat hit damage %d outside 1..%d", hit.Damage, core.MaxHealth)
	}
	if !hit.TargetKind.Valid() {
		return fmt.Errorf("network: combat hit target kind %d is invalid", hit.TargetKind)
	}
	return nil
}
```

encode 固定写 `u64/u8/u8`；decode 在任何分配前要求 payload 恰好 10 bytes，并让现有 decoder done-check 拒绝尾随。

- [ ] **Step 3: 升协议并只追加 registry/codec 分支**

把 `ProtocolVersion` 改为 32，注释新增 v32 的唯一变化。`serverPacketID`/`serverPacketForID` 对称追加 ID 25；`ValidateServerPacket`、server control encode/decode 接入 `CombatHit`。既有 ID、packet 形状和 C→S registry 一字不动。

更新协议 pin 和 handshake golden：`1f→20`、`1f01026e6f→2001026e6f`；所有旧 S→C unknown boundary 从 25 推到 26。fuzz seeds 加合法 CombatHit 和 current/previous protocol hello。

- [ ] **Step 4: 运行 network GREEN、golden 与 fuzz smoke**

```bash
gofmt -w internal/network/message_combat.go internal/network/message_combat_test.go internal/network/packet.go internal/network/packet_test.go internal/network/worldtime_test.go internal/network/registry.go internal/network/registry_test.go internal/network/codec_server.go internal/network/codec_golden_test.go internal/network/codec_fuzz_test.go internal/network/drop_test.go internal/network/place_block_succeeded_test.go internal/network/message_companion_test.go internal/network/message_hostile_test.go
go test ./internal/network/... -run 'Test(CombatHit|ProtocolVersionPinned|ServerPacket|CodecGolden|Transport)' -count=1
go test ./internal/network -run '^$' -fuzz '^FuzzSmallPacketCodec$' -fuzztime=5s
```

Expected: PASS，fuzz 无 crash/acceptance drift。

- [ ] **Step 5: 写 publication recipient/order/backpressure RED tests**

`combat_hit_publication_test.go` 证明：攻击者收到 inventory/container mirror → `PlayerState` → `CombatHit`；wire tick 等于 `result.Tick`；victim、旁观者和 trusted observer 都收不到；只够装 `PlayerState` 的慢 session 在追加 hit 时按既有策略断开，健康 session 仍收到自己的 hit。

```bash
go test ./internal/server -run 'TestCombatHitPublication' -count=1
```

Expected: FAIL，publication 尚未投影 combat facts。

- [ ] **Step 6: 在 PlayerState 成功后私发 hit**

保持 `publishLocalResult` 既有 inventory/crafting/furnace/chest 顺序。只有 `playerUpdate.Session == current.id` 且 `PlayerState` enqueue 成功后才扫描当前 session 的 sorted combat facts：

```go
for _, hit := range result.CombatHits {
	if hit.Session != current.id {
		continue
	}
	if !current.enqueue(network.CombatHit{
		ServerTick: result.Tick,
		Damage: hit.Damage,
		TargetKind: hit.TargetKind,
	}) {
		server.closePublicationSessionLocked(current, errSessionOutboxFull)
		return
	}
}
```

`PlayerState` enqueue 失败后也立即 return。不要增加重试、dedupe map、战斗专用队列或 trusted observer 特例。

- [ ] **Step 7: 扩展 player 与 player→hostile transport parity**

`player_melee_parity_test.go` 使用 durability=2 的 iron sword，逐 tick 从目标自己的 `PlayerState` 读取 health/velocity、从攻击者 inventory mirror 读取 selected sword stack，并从攻击者 wire 读取 player-kind hit；用相邻 `CombatHit.ServerTick` 的 10-tick 间隔和冷却期无确认验证线上节奏，字段级 cooldown 只在 sim 测试断言。drain 不能在 `PlayerState` 停止，必须继续到本 tick 预期 hit 数。

`sword_combat_parity_test.go` 复用现有 `openParityTransport` 和 restored hostile fixture，比较 hostile health/velocity、剑耐久/损坏形态、hostile-kind hit。Memory/TCP 业务 transcript 去掉绝对 tick origin 后相等，各 transport 内 hit tick 严格递增；不要使用不编码 durability 的 `PlayerHash`。

```bash
make rust
go test ./internal/server -run 'Test.*(MeleeParity|SwordCombatParity)' -race -count=1
go test ./internal/network/tcp -run 'Test.*TransportConsistency' -race -count=1
```

Expected: PASS。

- [ ] **Step 8: 同步协议基线、focused race、双评审并提交 Task 4**

把根 `AGENTS.md` 当前协议改为 v32；README、兼容性、gameplay 和 progress 留到 Task 6 一次性同步。

```bash
gofmt -w internal/network/message_combat.go internal/network/message_combat_test.go internal/network/packet.go internal/network/packet_test.go internal/network/worldtime_test.go internal/network/registry.go internal/network/registry_test.go internal/network/codec_server.go internal/network/codec_golden_test.go internal/network/codec_fuzz_test.go internal/network/drop_test.go internal/network/place_block_succeeded_test.go internal/network/message_companion_test.go internal/network/message_hostile_test.go internal/network/tcp/transport_consistency_test.go internal/server/publication.go internal/server/combat_hit_publication_test.go internal/server/player_melee_parity_test.go internal/server/sword_combat_parity_test.go cmd/mornlea/app/app_protocol_test.go cmd/mornlea-server/main_test.go
go test ./internal/network/... ./internal/sim ./internal/server ./cmd/mornlea-server ./cmd/mornlea/app -race -count=1
go test ./internal/archcheck -count=1
openspec validate --all --strict --no-interactive
git diff --check
```

Expected: PASS。更新 tasks/ledger，双评审后：

```bash
git add AGENTS.md internal/network/message_combat.go internal/network/message_combat_test.go internal/network/packet.go internal/network/packet_test.go internal/network/worldtime_test.go internal/network/registry.go internal/network/registry_test.go internal/network/codec_server.go internal/network/codec_golden_test.go internal/network/codec_fuzz_test.go internal/network/drop_test.go internal/network/place_block_succeeded_test.go internal/network/message_companion_test.go internal/network/message_hostile_test.go internal/network/tcp/transport_consistency_test.go internal/server/publication.go internal/server/combat_hit_publication_test.go internal/server/player_melee_parity_test.go internal/server/sword_combat_parity_test.go cmd/mornlea/app/app_protocol_test.go cmd/mornlea-server/main_test.go openspec/changes/tiered-swords-combat/tasks.md openspec/changes/tiered-swords-combat/ledger.md
git commit -m "feat: publish confirmed combat hits"
```

### Task 5: 客户端确认反馈、HUD、音频与 capture

**Files:**
- Modify: `internal/audio/cue.go`
- Modify: `internal/audio/cue_test.go`
- Modify: `internal/render/drop.go`
- Modify: `internal/render/drop_test.go`
- Modify: `internal/render/hud/atlas.go`
- Modify: `internal/render/hud/atlas_uv_stability_test.go`
- Create: `internal/render/hud/combat_marker.go`
- Modify: `internal/render/hud/renderer.go`
- Modify: `internal/render/hud/layout_test.go`
- Modify: `internal/render/hud/renderer_test.go`
- Modify: `internal/render/hud/atlas_test.go`
- Modify: `internal/render/hud/chat_test.go`
- Modify: `internal/render/hud/hunger_test.go`
- Modify: `internal/render/hud/eating_test.go`
- Create: `cmd/mornlea/app/combat_feedback.go`
- Create: `cmd/mornlea/app/combat_feedback_test.go`
- Modify: `cmd/mornlea/app/app.go`
- Modify: `cmd/mornlea/app/app_messages.go`
- Modify: `cmd/mornlea/app/app_lifecycle.go`
- Modify: `cmd/mornlea/app/app_frame.go`
- Modify: `cmd/mornlea/app/accessors.go`
- Create: `cmd/mornlea/app/combat_feedback_application_test.go`
- Modify: `cmd/mornlea/app/app_protocol_test.go`
- Modify: `cmd/mornlea/app/app_render_test.go`
- Modify: `cmd/mornlea/app/AGENTS.md`
- Modify: `cmd/mornlea/capture/capture.go`
- Modify: `cmd/mornlea/capture/capture_scene.go`
- Modify: `cmd/mornlea/capture/scene_application.go`
- Create: `cmd/mornlea/capture/capture_sword_combat.go`
- Create: `cmd/mornlea/capture/capture_sword_combat_test.go`
- Modify: `cmd/mornlea/capture/capture_hostile_mob.go`
- Modify: `cmd/mornlea/capture/capture_hostile_mob_test.go`
- Modify: `cmd/mornlea/capture/capture_scene_order_test.go`
- Modify: `cmd/mornlea/capture/AGENTS.md`
- Create: `cmd/mornlea/capture/testdata/golden/sword-combat.png`
- Modify: `openspec/changes/tiered-swords-combat/tasks.md`
- Modify: `openspec/changes/tiered-swords-combat/ledger.md`

**Interfaces:**
- Consumes: Task 4 `network.CombatHit`; existing `playLocalCue`, `HotbarRenderer.Prepare`, `Application.RenderFrame`, `SceneApplication`, capture `PinVolatile` and programmatic cue synth.
- Produces: independent monotonic `combatFeedback`, `CueCombatHit`, optional HUD marker input, six sword colors, three capture-only app methods, `sword-combat` golden and 24-scene order.

- [ ] **Step 1: 写 combat feedback 状态机 RED tests**

`combat_feedback_test.go` 断言 tick 1 接受并 arm 6 帧，tick 1/0 忽略，tick 2 重置为 6；`AfterRender(false)` 不减，六次 true 后不可见；`Reset` 清 tick 和帧。输入、health 和 inventory 变化本身不调用该状态机。

```bash
go test ./cmd/mornlea/app -run 'TestCombatFeedback' -count=1
```

Expected: FAIL，状态机不存在。

- [ ] **Step 2: 实现独立 combat feedback**

所有 app 新 Go 文件保留 `//go:build darwin`：

```go
const combatMarkerFrameCount uint8 = 6

type combatFeedback struct {
	lastServerTick  uint64
	remainingFrames uint8
}

func (feedback *combatFeedback) Observe(serverTick uint64) bool {
	if serverTick <= feedback.lastServerTick {
		return false
	}
	feedback.lastServerTick = serverTick
	feedback.remainingFrames = combatMarkerFrameCount
	return true
}

func (feedback *combatFeedback) ArmMarker() { feedback.remainingFrames = combatMarkerFrameCount }
func (feedback *combatFeedback) MarkerVisible() bool { return feedback.remainingFrames > 0 }
func (feedback *combatFeedback) AfterRender(rendered bool) {
	if rendered && feedback.remainingFrames > 0 {
		feedback.remainingFrames--
	}
}
func (feedback *combatFeedback) Reset() { *feedback = combatFeedback{} }
```

把该值直接放入 `Application`，不复用 `a.serverTick`，不加通用 animation manager。

- [ ] **Step 3: 写消息触发和生命周期 RED tests**

覆盖：只有严格新 `network.CombatHit` 播放一次 cue 并 arm marker；同 tick `PlayerState{Reset:true}` 先清旧状态、后续 hit 仍接受；disconnect、菜单返回、新 session、authoritative reset 都清 feedback；共享 session reset 同时清 hostile mirror。

```bash
go test ./cmd/mornlea/app -run 'TestApplicationCombat|Test.*Session.*Reset' -count=1
```

Expected: FAIL，app 尚不消费 `CombatHit`。

- [ ] **Step 4: 接通 message 和唯一 reset 边界**

`DrainServerMessages` 在 `PlayerState` 分支之后增加 `network.CombatHit` branch：

```go
if hit, ok := message.(network.CombatHit); ok {
	if a.combatFeedback.Observe(hit.ServerTick) {
		a.playLocalCue(audio.CueCombatHit)
	}
	continue
}
```

`PlayerState.Reset` 同时 reset audio/combat；`resetSessionOwnedState` reset combat 并在非 nil 时 reset `a.hostiles`。不要新增重复 lifecycle 调用。

- [ ] **Step 5: 写并实现固定 PCM cue**

`cue_test.go` 先断言 append-only cue value、参数和 hash；再在 `CueWaterSplash` 后追加：

```go
{samples: 1323, startHz: 520, endHz: 180, amplitude: 10500}
```

现有 `[cueCount][]int16` 自动扩展，不改音频设备或播放队列。

```bash
go test ./internal/audio -run '^TestCueCombatHitPCM$' -count=1
```

Expected: PASS 且 SHA-256 精确匹配设计。

- [ ] **Step 6: 写 HUD marker geometry/capacity/alloc RED tests**

断言无 inventory 确认时 marker 仍有 4 个白色不透明 untextured quad；设计中心 `(width/2,height/2)`，up/down 用 `2×8`，left/right 用 `8×2`，每条内缘距中心 `4*hudScale`。最大关闭/打开态分别 100/261，仍 `<=267`；warmed `Prepare` 零分配，所有实例在 framebuffer 内。

```bash
go test ./internal/render/hud -run 'Test.*(CombatMarker|FixedCapacity|ReusesLayout|Responsive)' -count=1
```

Expected: FAIL，Prepare 没有 marker 输入。

- [ ] **Step 7: 在现有 HUD pass 追加 4 quad**

给 `HotbarRenderer.Prepare` 在 `chat` 与尺寸参数之间追加 `combatMarker bool`，所有既有调用机械传 false。新 `appendCombatMarker` 只在 true 时 append 4 个 `hotbarInstance`，在 health/oxygen/hunger/chat 之后调用；不改 `maxHotbarQuads`、glyph offset、upload bytes、atlas、shader 或 pipeline。

marker 内缘公式固定为：

```text
up center    = (cx, cy - (4 + 8/2) * scale)
down center  = (cx, cy + (4 + 8/2) * scale)
left center  = (cx - (4 + 8/2) * scale, cy)
right center = (cx + (4 + 8/2) * scale, cy)
```

- [ ] **Step 8: 只在成功 native render 后消费 marker 帧**

`app_frame.go` 把 marker 纳入 `hudVisible` 并传入 `Prepare`。保留现有 `client.RenderFrame` literal 的全部字段，只在现有调用结果后增加：

```go
a.combatFeedback.AfterRender(rendered)
if !rendered {
	return false, nil
}
return true, nil
```

零 framebuffer、entity overflow、name-tag/HUD prepare error 都在 native render 前 return，因此不扣帧。扩展对应既有 frame tests 锁定这四个失败边界。

- [ ] **Step 9: 写并实现六个 sword item colors**

在 `render.ItemColor` 的固定 switch 增加六个原创、alpha=1、两两可区分颜色；测试断言非零、全互异、每对 intact/broken 不同且掉落呈现可见。HUD atlas 宽度由 `ItemIDMax` 自动从 960 扩到 1056，不改 atlas 生产计算。把 `atlas.go` 的旧 800 纹素说明改为引用动态 `hotbarTextureWidth` 与既有 `W <= 2^15` 适用域；把 UV 稳定性宽度集改为以下派生探针，避免首项继续伪装成当前真实宽度：

```go
var stabilityAtlasWidths = [...]int{
	hotbarTextureWidth,
	hotbarTextureWidth + hotbarTextureSize,
	hotbarTextureWidth + 2*hotbarTextureSize,
	2048,
	4096,
}
```

```bash
go test ./internal/render ./internal/render/hud -run 'TestSword|Test.*Atlas' -count=1
```

Expected: PASS。

- [ ] **Step 10: 写 `sword-combat` capture 状态 RED test**

新场景必须：selected iron sword `Durability:125`；一个合法 UUIDv4 远端玩家经 spawn/state 镜像从初始位置沿受击方向移动 0.35；固定 camera/time；marker 可见；`PinVolatile` 在收敛后重新 arm。顺序精确为 `ai-companion, sword-combat, hostile-mob, water-surface-slope`，总数 24，far-horizon 倒数第二，water-underwater 最后。同步更新 `capture_hostile_mob.go` 的排序说明和 `capture_hostile_mob_test.go` 原来“hostile 紧随 ai-companion”的断言。

```bash
go test ./cmd/mornlea/capture -run 'Test(SwordCombatCaptureState|CaptureSceneOrderAndAICompanionDeterminism)' -count=1
```

Expected: FAIL，场景和接口不存在。

- [ ] **Step 11: 增加最小 capture 消费接口和场景**

`Application` 只导出三个真实消费方法，并加入 `SceneApplication`：

```go
func (a *Application) ArmCombatMarker() { a.combatFeedback.ArmMarker() }
func (a *Application) ResetCombatFeedback() { a.combatFeedback.Reset() }
func (a *Application) CombatMarkerVisible() bool { return a.combatFeedback.MarkerVisible() }
```

`resetCapturePresentation` 调用 `ResetCombatFeedback`；场景复用现有开阔草地/air neighborhood helper，不复制 terrain builder。`PinVolatile` 调用 `ArmCombatMarker`，使最终抓帧落在 6 帧窗口内。同步 app/capture `AGENTS.md` 的文件地图、最小接口和 reset 责任，不复制会漂移的全场景清单。

- [ ] **Step 12: 生成候选、人工审核并加入唯一新 golden**

```bash
make rust
make visual-check VISUAL_OUT=build/visual-sword-combat
```

Expected before baseline: 仅因缺少 `sword-combat.png` 失败，并在输出目录生成候选。人工打开候选，确认非满耐久铁剑、远端玩家、hit marker 与击退关系都可见。批准后：

```bash
make visual-update VISUAL_OUT=build/visual-sword-combat-update
git status --short cmd/mornlea/capture/testdata/golden
make visual-check VISUAL_OUT=build/visual-sword-combat-check
```

Expected: tracked golden 只新增 `sword-combat.png`；若其它 PNG 改变，逐图归因并取得明确批准，否则恢复那些非预期生成结果，绝不放宽阈值。

- [ ] **Step 13: focused race、双评审并提交 Task 5**

```bash
gofmt -w internal/audio/cue.go internal/audio/cue_test.go internal/render/drop.go internal/render/drop_test.go internal/render/hud/atlas.go internal/render/hud/atlas_uv_stability_test.go internal/render/hud/combat_marker.go internal/render/hud/renderer.go internal/render/hud/layout_test.go internal/render/hud/renderer_test.go internal/render/hud/atlas_test.go internal/render/hud/chat_test.go internal/render/hud/hunger_test.go internal/render/hud/eating_test.go cmd/mornlea/app/combat_feedback.go cmd/mornlea/app/combat_feedback_test.go cmd/mornlea/app/combat_feedback_application_test.go cmd/mornlea/app/app.go cmd/mornlea/app/app_messages.go cmd/mornlea/app/app_lifecycle.go cmd/mornlea/app/app_frame.go cmd/mornlea/app/accessors.go cmd/mornlea/app/app_protocol_test.go cmd/mornlea/app/app_render_test.go cmd/mornlea/capture/capture.go cmd/mornlea/capture/capture_scene.go cmd/mornlea/capture/scene_application.go cmd/mornlea/capture/capture_sword_combat.go cmd/mornlea/capture/capture_sword_combat_test.go cmd/mornlea/capture/capture_hostile_mob.go cmd/mornlea/capture/capture_hostile_mob_test.go cmd/mornlea/capture/capture_scene_order_test.go
go test ./internal/audio ./internal/render ./internal/render/hud ./cmd/mornlea/app ./cmd/mornlea/capture -race -count=1
go test ./internal/archcheck -count=1
make visual-check
openspec validate --all --strict --no-interactive
git diff --check
```

Expected: PASS，capture 24/24。更新 tasks/ledger，双评审后：

```bash
git add internal/audio/cue.go internal/audio/cue_test.go internal/render/drop.go internal/render/drop_test.go internal/render/hud/atlas.go internal/render/hud/atlas_uv_stability_test.go internal/render/hud/combat_marker.go internal/render/hud/renderer.go internal/render/hud/layout_test.go internal/render/hud/renderer_test.go internal/render/hud/atlas_test.go internal/render/hud/chat_test.go internal/render/hud/hunger_test.go internal/render/hud/eating_test.go cmd/mornlea/app/combat_feedback.go cmd/mornlea/app/combat_feedback_test.go cmd/mornlea/app/combat_feedback_application_test.go cmd/mornlea/app/app.go cmd/mornlea/app/app_messages.go cmd/mornlea/app/app_lifecycle.go cmd/mornlea/app/app_frame.go cmd/mornlea/app/accessors.go cmd/mornlea/app/app_protocol_test.go cmd/mornlea/app/app_render_test.go cmd/mornlea/app/AGENTS.md cmd/mornlea/capture/capture.go cmd/mornlea/capture/capture_scene.go cmd/mornlea/capture/scene_application.go cmd/mornlea/capture/capture_sword_combat.go cmd/mornlea/capture/capture_sword_combat_test.go cmd/mornlea/capture/capture_hostile_mob.go cmd/mornlea/capture/capture_hostile_mob_test.go cmd/mornlea/capture/capture_scene_order_test.go cmd/mornlea/capture/AGENTS.md cmd/mornlea/capture/testdata/golden/sword-combat.png openspec/changes/tiered-swords-combat/tasks.md openspec/changes/tiered-swords-combat/ledger.md
git commit -m "feat: show confirmed combat feedback"
```

### Task 6: 整分支终审、归档与合入

**Files:**
- Modify: `openspec/changes/tiered-swords-combat/tasks.md`
- Modify: `openspec/changes/tiered-swords-combat/ledger.md`
- Modify via OpenSpec sync: `openspec/specs/tiered-swords-combat/spec.md`
- Modify via OpenSpec sync: `openspec/specs/authoritative-player-melee/spec.md`
- Modify via OpenSpec sync: `openspec/specs/authoritative-hostile-nightwalker/spec.md`
- Modify via OpenSpec sync: `openspec/specs/authoritative-mining/spec.md`
- Modify via OpenSpec sync: `openspec/specs/authoritative-hunger/spec.md`
- Modify via OpenSpec sync: `openspec/specs/tool-durability/spec.md`
- Modify via OpenSpec sync: `openspec/specs/authoritative-crafting/spec.md`
- Modify via OpenSpec sync: `openspec/specs/local-audio-feedback/spec.md`
- Modify via OpenSpec sync: `openspec/specs/survival-hud-presentation/spec.md`
- Modify via OpenSpec sync: `openspec/specs/container-ui-presentation/spec.md`
- Modify via OpenSpec sync: `openspec/specs/visual-verification/spec.md`
- Modify: `AGENTS.md`
- Modify: `openspec/config.yaml`
- Modify: `README.md`
- Modify: `README.en.md`
- Modify: `docs/notes/gameplay.md`
- Modify: `docs/notes/compatibility.md`
- Modify: `docs/notes/limitations.md`
- Modify: `docs/notes/visual-verification.md`
- Modify: `docs/notes/progress.md`
- Modify: `docs/feature-backlog.md`
- Remote update after merge: GitHub Discussion #71

**Interfaces:**
- Consumes: completed Tasks 1–5, all task review rulings and validation evidence.
- Produces: final branch review, full gates, synced/archived OpenSpec change, current v32/24-scene documentation, completed backlog row, green PR/CI and cleaned implementation worktree.

- [ ] **Step 1: 完成整分支独立 SPEC/QUALITY 终审**

调用 `superpowers:requesting-code-review`，reviewer 必须逐项核对批准设计，重点包括：72/72 overflow 无部分副作用、hostile-first 但 20 tick 语义不变、受保护最近目标不穿透、互杀、栏位身份重验、burn/death/distant 顺序、私有 publication 顺序、独立 combat tick、失败 render 不耗 marker、100/261≤267、24 场景和无 Rust/ABI/benchmark 漂移。

把 findings 和修复轮次写入 ledger；任何 tracked 修复都回到对应 Task 的 fresh implementer 和 scoped 双评审，不由控制会话直接改代码。

- [ ] **Step 2: 运行完整本地门禁并记录 SHA 证据**

调用 `superpowers:verification-before-completion` 后运行：

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

Expected: 全部 exit 0；性能数值只记录，不因数值波动改退出状态。若高负载时敏测试失败，按 `docs/notes/test-quickstart.md` 单包复跑并在 ledger 归类，不用提高 timeout 修生产代码。

- [ ] **Step 3: 标记 active tasks 完成并提交 archive readiness**

只有 Tasks 2–5 checkbox、双评审和完整门禁都有同 SHA 证据时，才勾完 `tasks.md`。`ledger.md` 汇总全部 `Ruling:`、final review 和 gate 输出。

```bash
openspec status --change tiered-swords-combat
openspec validate --all --strict --no-interactive
git add openspec/changes/tiered-swords-combat
git commit -m "docs: record tiered swords verification"
```

- [ ] **Step 4: sync/archive 并同步长期基线**

调用 `openspec-sync-specs`，确认 delta 合入上列 11 个主规格；调用 `openspec-archive-change` 归档 `tiered-swords-combat`。随后把长期事实改为：protocol v32、三级剑/统一战斗、玩家可攻击夜行者、私有确认反馈、24 项 capture。`docs/notes/visual-verification.md` 同时把旧路径 `cmd/mornlea/capture.go` 修正为 `cmd/mornlea/capture/capture.go`。

`openspec/config.yaml` 和根/双语 README 使用完整不变矩阵：player v8、chunk v9、world v3、companions v4、hostile v1、engine v8、client v10、benchmark v19。`docs/notes/progress.md` 追加本 change 的最终实现/验证/归档事实，不改写历史段。不要修改 `docs/notes/lan-server.md`。

- [ ] **Step 5: 回填 backlog 为已完成并提交归档/文档**

把 A-03 状态改为「已完成」，保留认领履历，备注写 Task 6 Step 2 完整门禁对应的 code/gate SHA、归档 change、协议 v32、24-scene golden 和双评审/门禁结果；最终 PR head 会在随后提交产生，合并后只在 Discussion 记录 merge SHA，不在本提交里递归回填自身 SHA。

```bash
openspec validate --all --strict --no-interactive
go test ./internal/archcheck -count=1
git diff --check
archive_path=$(printf '%s\n' openspec/changes/archive/*-tiered-swords-combat)
test -d "$archive_path"
git add -A -- openspec/changes/tiered-swords-combat "$archive_path"
git add AGENTS.md openspec/config.yaml openspec/specs/tiered-swords-combat/spec.md openspec/specs/authoritative-player-melee/spec.md openspec/specs/authoritative-hostile-nightwalker/spec.md openspec/specs/authoritative-mining/spec.md openspec/specs/authoritative-hunger/spec.md openspec/specs/tool-durability/spec.md openspec/specs/authoritative-crafting/spec.md openspec/specs/local-audio-feedback/spec.md openspec/specs/survival-hud-presentation/spec.md openspec/specs/container-ui-presentation/spec.md openspec/specs/visual-verification/spec.md README.md README.en.md docs/notes/gameplay.md docs/notes/compatibility.md docs/notes/limitations.md docs/notes/visual-verification.md docs/notes/progress.md docs/feature-backlog.md
git diff --cached --name-status -- openspec/changes
git commit -m "docs: archive tiered swords combat"
```

Expected: active change 不再出现在 `openspec list`，archive 存在，主规格与代码版本一致。
`git diff --cached --name-status -- openspec/changes` 必须同时显示 active path 删除和唯一 archive path 新增；缺任一侧都不得提交。

- [ ] **Step 6: 创建 PR 并监听 CI 至全绿**

调用 `superpowers:finishing-a-development-branch`。创建 PR 前检查 status、diff、remote tracking、最近提交和相对 base 的完整 diff；默认 `AGENT_MODE=pr`，不 force-push。

PR 标题：

```text
feat(combat): add A-03 tiered swords and unified melee
```

PR body：

```markdown
## Summary

- add wooden, stone, and iron swords with deterministic unified player/nightwalker melee settlement
- bump protocol v31 to v32 with private CombatHit confirmations; keep all storage schemas, engine/client ABIs, and benchmark scenario unchanged
- add confirmed audio/HUD feedback and the 24th sword-combat visual golden

## Validation

- `scripts/agents/gates.sh` passed
- `make rust-check` passed
- `make visual-check` passed for 24/24 scenes
- `openspec validate --all --strict --no-interactive` passed
- OpenSpec change: [`tiered-swords-combat`](${change_url}) archived and synced
```

```bash
git push -u origin feat/A-03-tiered-swords-combat
archive_path=$(printf '%s\n' openspec/changes/archive/*-tiered-swords-combat)
test -d "$archive_path"
repo_url=$(gh repo view --json url --jq .url)
branch_sha=$(git rev-parse HEAD)
change_url="${repo_url}/tree/${branch_sha}/${archive_path}"
gh pr create --title "feat(combat): add A-03 tiered swords and unified melee" --body "## Summary

- add wooden, stone, and iron swords with deterministic unified player/nightwalker melee settlement
- bump protocol v31 to v32 with private CombatHit confirmations; keep all storage schemas, engine/client ABIs, and benchmark scenario unchanged
- add confirmed audio/HUD feedback and the 24th sword-combat visual golden

## Validation

- \`scripts/agents/gates.sh\` passed
- \`make rust-check\` passed
- \`make visual-check\` passed for 24/24 scenes
- \`openspec validate --all --strict --no-interactive\` passed
- OpenSpec change: [\`tiered-swords-combat\`](${change_url}) archived and synced"
gh pr checks --watch
```

CI 失败时用 `gh run view --log-failed` 定位，最多 10 轮；任何修复重新走 scoped implementer、双评审和受影响门禁，再提交推送。

- [ ] **Step 7: 合并、刷新 Discussion 并清理 worktree**

CI 全绿后：

```bash
pr_url=$(gh pr view --json url --jq .url)
gh pr merge --merge
merge_sha=$(gh pr view --json mergeCommit --jq .mergeCommit.oid)
test -n "$merge_sha"
main_worktree=$(dirname "$(git rev-parse --path-format=absolute --git-common-dir)")
test "$(git -C "$main_worktree" branch --show-current)" = main
git -C "$main_worktree" pull --ff-only
test "$(git -C "$main_worktree" rev-parse HEAD)" = "$merge_sha"
gh api graphql -f query='mutation($b:String!){ addDiscussionComment(input:{discussionId:"D_kwDOToJS8M4Aou6G", body:$b}){ comment { id } } }' -F b="【状态变更】A-03 → 已完成

PR: ${pr_url}
Merge: ${merge_sha}
OpenSpec: tiered-swords-combat 已归档并同步
验证: gates.sh、rust-check、24/24 visual-check、OpenSpec strict 全绿"
python3 scripts/agents/refresh-discussion.py
python3 scripts/agents/refresh-discussion.py --update
```

Expected: GraphQL 返回 comment ID，Discussion 正文刷新成功。确认远端 main/backlog/Discussion 都已更新后，按 `using-git-worktrees`/`finishing-a-development-branch` 安全删除本 change 的 worktree 和本地分支；不清理 sibling worktree，不删除用户无关文件。
