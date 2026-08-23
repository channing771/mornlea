# Tiered Swords Combat Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task with fresh implementers, independent SPEC/QUALITY reviews, and a ledger. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在现有持续 primary-action 近战上交付木/石/铁三把剑、统一攻击者与受击者冷却、确定性目标选择、水平击退、耐久和只由服务端确认驱动的命中反馈，供 PvP 与夜行者共用。

**Architecture:** `core` 固定武器数值与耐久映射；`sim` 每 tick 先从最多 72 个候选冻结玩家攻击意图，再按攻击者和 target kind+ID 全序筛选/结算，命中统一走已有 `applyDamage`；玩家与敌对生物用 tagged target 值而非通用 ECS/interface。协议只私发 `CombatHit`，客户端不预测伤害、耐久或击退。

**Tech Stack:** Go 1.26、现有 Rust physics 入口、Memory/TCP 协议、HUD/audio、OpenSpec。

**Spec:** `docs/superpowers/specs/2026-08-23-first-night-survival-parallel-wave-design.md`

## Global Constraints

- 基于 batch 共享契约 SHA；保留持续 `PlayerInput.Mining` 作为 primary action，不新增攻击键或输入位。
- 批次执行时本计划 Task 1 先在 integration 分支创建并评审，功能 worktree 从共享契约提交继续并直接从 Task 2 开始；独立执行本计划时才在本分支创建 Task 1 产物。
- 固定数值：空手/普通物品伤害 2；木/石/铁剑伤害 4/5/6；耐久 59/131/250；攻击者冷却 10 ticks；受击无敌 10 ticks；水平击退速度增量 0.35。
- 命中距离 3 格；固体阻挡、fluid 穿透；最近优先，等距按 `TargetKind`（player=1、hostile=2）再按稳定 ID。
- 每名玩家每 tick 至多一个 intent、每个受击者每 tick至多一次结算；所有 intent 先冻结再改变 health/velocity/durability。
- 不做暴击、蓄力条、连击、格挡、附魔、护甲、范围攻击、投射物或客户端预测。

---

## Task 1：建立 OpenSpec change

**Files:**

- Create: `openspec/changes/tiered-swords-combat/.openspec.yaml`
- Create: `openspec/changes/tiered-swords-combat/proposal.md`
- Create: `openspec/changes/tiered-swords-combat/design.md`
- Create: `openspec/changes/tiered-swords-combat/tasks.md`
- Create: `openspec/changes/tiered-swords-combat/ledger.md`
- Create: `openspec/changes/tiered-swords-combat/specs/tiered-swords-combat/spec.md`
- Create: `openspec/changes/tiered-swords-combat/specs/authoritative-player-melee/spec.md`
- Create: `openspec/changes/tiered-swords-combat/specs/tool-durability/spec.md`
- Create: `openspec/changes/tiered-swords-combat/specs/local-audio-feedback/spec.md`
- Create: `openspec/changes/tiered-swords-combat/specs/visual-verification/spec.md`

- [ ] **Step 1: 基线验证**

  若已存在共享提交，用 `shared_sha=$(git log -1 --format=%H --grep='^feat: reserve first-night survival contracts$')` 和 `git merge-base --is-ancestor "$shared_sha" HEAD` 验证；若尚不存在，本 Task 必须位于 `codex/first-night-survival-integration`。运行 `make rust`、`go test ./internal/core ./internal/sim ./internal/network ./internal/server ./cmd/mornlea -race -count=1`，记录 ledger。

- [ ] **Step 2: 写可观察规则**

  specs 必须覆盖三档伤害/耐久、损坏形态、攻击者/受击者冷却、72 候选上限、遮挡/流体、确定性裁决、同 tick 反击、击退、采掘分流、PvP、私有 `CombatHit` 和 `sword-combat`。

- [ ] **Step 3: 固化 tagged values**

  ```go
  type CombatTarget struct {
      Kind core.CombatTargetKind
      ID   uint64
  }
  ```

  `ID` 对 player 使用 `SessionID` 的无损值，对 hostile 使用 `HostileMobID`；不得引入只有两个实现的 actor interface。

- [ ] **Step 4: strict validate、双评审并提交**

  运行 `openspec validate --all --strict --no-interactive`、`git diff --check`，提交 `docs: propose tiered swords combat`。

## Task 2：登记剑数值、配方与耐久形态

**Files:**

- Modify: `internal/core/item.go`
- Modify: `internal/core/item_test.go`
- Modify: `internal/core/recipe.go`
- Modify: `internal/core/recipe_test.go`
- Modify: `internal/core/inventory_test.go`

- [ ] **Step 1: 写 item registry 失败测试**

  三把完好剑与三把损坏剑都 stack limit 1；只有完好剑有 59/131/250 耐久；`WeaponDamage` 返回 4/5/6，损坏剑和非剑返回 2；`DamageTool` 在最后一点后转换到对应 broken item、Count 保持 1、Durability 清零。

- [ ] **Step 2: 实现固定 switch**

  添加 `WeaponDamage(ItemID) int32` 和扩展现有耐久/损坏映射；不要创建武器 registry struct 或配置文件。

- [ ] **Step 3: 写三条 shape recipe 失败测试**

  木剑为纵向两 plank+stick，石剑为两 cobblestone+stick，铁剑为两 iron ingot+stick；外侧平移允许，水平镜像无差异，横放/倒放/多余材料失败，产物带满耐久。

- [ ] **Step 4: 最小登记 recipe 15..17**

  只在 crafting 的固定 recipe registry 加三项，复用 shape matcher；不得恢复自动扣 inventory 的旧路径。

- [ ] **Step 5: focused 验证与提交**

  运行 `gofmt -w internal/core`、`go test ./internal/core -race -count=1`；双评审后提交 `feat: register tiered swords`。

## Task 3：重构为有界冻结/结算战斗

**Files:**

- Modify: `internal/sim/combat.go`
- Modify: `internal/sim/combat_test.go`
- Modify: `internal/sim/player.go`
- Modify: `internal/sim/engine_step.go`
- Modify: `internal/sim/mining.go`
- Modify: `internal/sim/death_test.go`
- Create: `internal/sim/combat_weapon_test.go`

- [ ] **Step 1: 写攻击者冷却和采掘分流失败测试**

  持续 primary action 第 1 tick 命中、之后 9 tick 不再命中、第 11 tick 再命中；冷却中无合法 intent 时采掘照常推进。命中玩家时只抑制该 tick 采掘。

- [ ] **Step 2: 把旧 victim-only 状态拆成两个明确字段**

  `attackCooldownTicks uint8` 属攻击者，`hurtCooldownTicks uint8` 属受击者；每 tick 起始各减 1。删除旧 `meleeCooldownTicks`，更新已有 melee tests 而不放宽规则。

- [ ] **Step 3: 写 72 候选和稳定排序失败测试**

  用固定数组构造 player/hostile tagged candidates：最近距离优先，相同 float distance 按 kind 再 ID；固体在目标表面前阻挡，相等距离不阻挡；fluid 不阻挡；超出第 72 项必须由入口拒绝而非截断。

- [ ] **Step 4: 实现共享 candidate/intent 值**

  用 `[72]combatCandidate` 和 `[8]combatIntent`，先为每个 active player 选候选，再按 attacker session 排序；本分支 player 候选真实可用，hostile 候选入口接收空 slice，供集成接线。不要分配 map 或动态实体集合。

- [ ] **Step 5: 写同 tick 冻结结算失败测试**

  A/B 同 tick 互击且都将致死时两次伤害都成立；两名攻击者同 tick 命中同一 victim 时只稳定接受 attacker ID 更小者；intent 收集后目标移动/死亡不改变已冻结 hit；settle 后才设置冷却和死亡。

- [ ] **Step 6: 最小结算与剑耐久**

  成功结算顺序：`applyDamage`→水平 velocity 加 `normalizeXZ(target-attacker)*0.35`→设置 attacker/victim cooldown→对攻击者选中剑恰减 1 durability→记录 `CombatHit`。若目标在 settlement 前已因更早稳定 intent 不可结算，则不扣耐久、不发确认。

- [ ] **Step 7: focused 验证与提交**

  运行 `gofmt -w internal/sim`、`go test ./internal/sim -race -count=1`；双评审后提交 `feat: settle authoritative sword combat`。

## Task 4：完成命中协议、publication 与 transport parity

**Files:**

- Modify: `internal/network/message_combat.go`
- Create: `internal/network/message_combat_test.go`
- Modify: `internal/network/codec_server.go`
- Modify: `internal/network/packet.go`
- Modify: `internal/network/registry.go`
- Modify: `internal/network/registry_test.go`
- Modify: `internal/network/codec_fuzz_test.go`
- Modify: `internal/sim/command.go`
- Modify: `internal/server/publication.go`
- Modify: `internal/server/player_melee_parity_test.go`
- Create: `internal/server/sword_combat_parity_test.go`

- [ ] **Step 1: 写 `CombatHit` 值域/codec 失败测试**

  `ServerTick>0`、`Damage` 1..`core.MaxHealth`、TargetKind 仅 player/hostile；固定载荷 10 bytes。截断、尾随、未知 kind 和零 damage 拒绝。

- [ ] **Step 2: 接通私有 tick 出口**

  `TickResult.CombatHits` 按攻击者 session 排序；server 只向命中者 session 发送，不广播 victim/其他玩家。一次伤害只有一条消息，慢客户端沿用既有队列/断开策略。

- [ ] **Step 3: Memory/TCP parity**

  同一脚本断言 health、velocity、selected sword durability/broken form、命中消息和 tick 完全一致；拒绝/冷却 tick 不发消息。

- [ ] **Step 4: focused 验证与提交**

  运行 `gofmt -w internal/network internal/sim internal/server`、`go test ./internal/network ./internal/sim ./internal/server -race -count=1`；双评审后提交 `feat: publish confirmed combat hits`。

## Task 5：客户端确认反馈与视觉构造

**Files:**

- Modify: `internal/audio/cue.go`
- Modify: `internal/audio/cue_test.go`
- Modify: `internal/render/hud/layout.go`
- Modify: `internal/render/hud/layout_test.go`
- Modify: `cmd/mornlea/app_messages.go`
- Modify: `cmd/mornlea/app_audio.go`
- Modify: `cmd/mornlea/app_audio_test.go`
- Modify: `cmd/mornlea/app_render.go`
- Create: `cmd/mornlea/combat_feedback.go`
- Create: `cmd/mornlea/combat_feedback_test.go`
- Modify: `cmd/mornlea/capture_scene.go`
- Modify: `cmd/mornlea/capture_scene_test.go`

- [ ] **Step 1: 写仅确认触发失败测试**

  primary input、预测射线、inventory 耐久变化和 victim health 变化都不能触发本地命中反馈；只接受单调 `CombatHit.ServerTick`，旧/重复消息忽略，断线重置。

- [ ] **Step 2: 复用现有 HUD/audio pass**

  添加短促原创 `CueCombatHit`，用现有 synth；HUD 在准星四角追加 4 个短线 quad，显示固定 6 帧后消失。不得加贴图、shader、pass 或动画系统。

- [ ] **Step 3: 加 `sword-combat` 场景构造**

  固定显示铁剑耐久、被确认命中的远端玩家、准星 hit marker 和击退后的姿态；测试只锁定状态/布局/场景顺序，本分支不生成 golden。

- [ ] **Step 4: focused 验证与提交**

  运行 `gofmt -w internal/audio internal/render/hud cmd/mornlea`、`go test ./internal/audio ./internal/render/hud ./cmd/mornlea -race -count=1`；双评审后提交 `feat: show confirmed sword hits`。

## Task 6：功能线终审

**Files:**

- Modify: `openspec/changes/tiered-swords-combat/tasks.md`
- Modify: `openspec/changes/tiered-swords-combat/ledger.md`

- [ ] **Step 1: 运行功能线验证**

  ```bash
  make rust
  go test ./internal/core ./internal/network ./internal/sim ./internal/server ./internal/audio ./internal/render/hud ./cmd/mornlea -race -count=1
  go test ./internal/archcheck -count=1
  openspec validate --all --strict --no-interactive
  git diff --check
  ```

- [ ] **Step 2: 独立终审并移交**

  reviewer 核对冻结/结算边界、双冷却、72/8 固定容量、遮挡、耐久只随确认伤害、私有消息和无客户端预测；写 ledger 后把 SHA 交给 integration controller，不自行接入 nightwalker、不更新 golden。
