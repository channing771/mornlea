# 服务端权威玩家近战实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 复用现有 primary-action 输入，为最多八名玩家提供确定性的服务端权威徒手近战：3 格、2 点伤害、10 tick 冷却、方块遮挡、同 tick 互杀，并在命中玩家时抑制同 tick 采掘。

**Architecture:** `network.PlayerInput.Mining` 的 wire 位不变，只把服务端语义扩展为“primary action held”；`sim.Engine.Step` 在物理与订阅收敛后、`settleDeaths` 前调用一个新的 `advancePlayerMelee`。该函数先从同一权威快照收集最多 8 条攻击意图，再按攻击者 `SessionID` 应用 `applyDamage`；玩家 AABB 相交留在 `internal/sim`，方块遮挡复用 Rust DDA 驱动的 `core.RaycastBlocks`。

**Tech Stack:** Go 1.26、`internal/sim` 权威 tick、现有 Rust raycast、Memory/TCP transport、OpenSpec、race tests。

**Spec:** `docs/superpowers/specs/2026-08-22-five-way-parallel-wave-design.md` §5、§3、§9。

## Global Constraints

- [ ] 仅在 `bedrock-survival-hud` 已归档后的共享 main 基线上执行；创建独立 worktree、`codex/authoritative-player-melee` 分支和同名 OpenSpec change。
- [ ] 不增加消息、字段、packet ID、拒绝码、武器、护甲、击退、伙伴受伤或攻击者命中特效。
- [ ] wire 字节形状不变，但 `Mining` 语义扩展使协议 v24→v25；玩家/区块/世界/伙伴 schema、benchmark scenario v19、engine ABI v6、client ABI v7 不变。
- [ ] 近战 PR 对长期文档只有一个机械例外：为通过 `TestBaselineVersionsMatchCode`，逐字节同步 `AGENTS.md`/`CLAUDE.md` 的当前协议号 v25；完整能力描述与 `progress.md` 留到串行归档。不修改或放宽 archcheck。
- [ ] 每个 task 由全新 implementer 执行；独立 reviewer 同时给出 SPEC/QUALITY 裁决，finding 追加修复且最多 5 轮，全部写入 ledger。

---

## Task 1: 建立 OpenSpec 契约并冻结 v25 的 wire 不变迁移

**Files:**
- Create: `openspec/changes/authoritative-player-melee/.openspec.yaml`
- Create: `openspec/changes/authoritative-player-melee/proposal.md`
- Create: `openspec/changes/authoritative-player-melee/design.md`
- Create: `openspec/changes/authoritative-player-melee/tasks.md`
- Create: `openspec/changes/authoritative-player-melee/ledger.md`
- Create: `openspec/changes/authoritative-player-melee/specs/authoritative-player-melee/spec.md`
- Create: `openspec/changes/authoritative-player-melee/specs/authoritative-mining/spec.md`
- Modify: `internal/network/packet.go`
- Modify: `internal/network/message_command.go`
- Modify: `internal/network/codec_golden_test.go`
- Modify: `internal/network/codec_fuzz_test.go`
- Modify: `internal/network/packet_test.go`
- Modify: `internal/network/worldtime_test.go`
- Modify: `internal/network/registry_test.go`
- Modify: `internal/network/drop_test.go`
- Modify: `internal/network/seed_test.go`
- Modify: `internal/network/transport_consistency_test.go`
- Modify: `cmd/mornlea/app_protocol_test.go`
- Modify: `cmd/mornlea-server/main_test.go`
- Modify: `AGENTS.md`
- Modify: `CLAUDE.md`

- [ ] proposal/spec/design 锁定：目标仅 active 同维玩家；3 格、最近命中、`SessionID` 平局、方块遮挡、流体穿透、2 点、10 tick、同 tick 意图快照、命中抑制采掘；列出全部排除项。
- [ ] `authoritative-mining` delta 明确：primary action 未命中合法玩家时采掘完全沿用旧规则；命中玩家时只抑制当 tick，下一 tick 仍由持续输入决定。
- [ ] 先写协议失败测试：`ProtocolVersion == 25`；既有 `PlayerInput{Mining:true}` golden payload、packet ID 与载荷长度不变；v24 及更早在握手阶段拒绝。
- [ ] 更新 hello goldens：uvarint 24 的 `18` 改为 25 的 `19`；`PlayerInput`、`PlayerState` 和所有 play packet golden 字节不得改变。
- [ ] 在 `codec_fuzz_test.go` 增加当前 v25 与刚退役 v24 的 `ClientHello` 种子；保留 `ProtocolVersion-1` 的 Memory/TCP 拒绝覆盖，并把只描述“当前版本”的测试名/注释同步到 v25，历史 v24 hunger 名称不改。
- [ ] 最小把 `ProtocolVersion` 改为 25，并在注释说明 v25 只有既有 `Mining` 位的语义扩展、无 wire 字段变化。历史 v24 hunger 测试名称和字节断言保留。
- [ ] 给 `PlayerInput.Mining` 补中文字段注释：v25 起表示持续 primary action，目标与攻击/采掘分流均由服务端决定；不得改字段顺序或编码器。
- [ ] 同步所有“当前版本”字面断言；不要把历史 `TestProtocolV24...` 改名。
- [ ] 在 `AGENTS.md` 第一段只把“已经包含协议 v24（v24 ...）”改为“已经包含协议 v25（v24 ...）”，然后把逐字节相同内容同步到 `CLAUDE.md` 并通过 `cmp`；完整近战能力描述、milestone 段和 `progress.md` 留到串行归档。
- [ ] 运行并提交：

```bash
make rust
gofmt -w internal/network/packet.go internal/network/message_command.go \
  internal/network/codec_golden_test.go internal/network/codec_fuzz_test.go \
  internal/network/packet_test.go internal/network/worldtime_test.go \
  internal/network/registry_test.go internal/network/drop_test.go \
  internal/network/seed_test.go internal/network/transport_consistency_test.go \
  cmd/mornlea/app_protocol_test.go cmd/mornlea-server/main_test.go
go test ./internal/network ./cmd/mornlea ./cmd/mornlea-server -count=1
go test ./internal/archcheck -count=1
cmp -s AGENTS.md CLAUDE.md
openspec validate authoritative-player-melee --strict --no-interactive
git diff --check
git add openspec/changes/authoritative-player-melee internal/network cmd/mornlea/app_protocol_test.go cmd/mornlea-server/main_test.go AGENTS.md CLAUDE.md
git commit -m "feat: reserve protocol v25 primary action semantics"
```

- [ ] 完成 task 双裁决和 ledger 记录。

## Task 2: 实现确定性射线与玩家目标选择

**Files:**
- Create: `internal/sim/combat.go`
- Create: `internal/sim/combat_test.go`
- Modify: `internal/sim/player.go`

- [ ] 先用 `movementFlatChunk` 组装 2–8 名 active 玩家，写 `rayAABBDistance` 表驱动红测：正向命中、反向/超距、平行轴在盒外、起点在盒内、边界恰为 3。
- [ ] 再写目标选择红测：排除自己、pending、其他维度、生命已归零者和伙伴；选择最近玩家；距离完全相同按较小 `SessionID`；石块在玩家前方时遮挡；与玩家表面同距的方块不算“更近”；水块不遮挡；未 Ready 方块使该射线本 tick 不产生目标。
- [ ] 运行红测（先确保 native 已构建）：

```bash
make rust
go test ./internal/sim -run 'Test(RayAABBDistance|PlayerMeleeTarget)' -count=1
```

- [ ] 在 `combat.go` 增加固定常量，不做 tunable：

```go
const (
	playerMeleeReach         = float32(3)
	playerMeleeDamage        = int32(2)
	playerMeleeCooldownTicks = uint8(10)
)
```

- [ ] 用 slab 算法实现包内 `rayAABBDistance`；方向分量绝对值小于 `1e-6` 时只检查 origin 是否在该轴范围内，其余轴更新 `near/far`，返回最早非负交点且不得超过 3。
- [ ] `advancePlayerMelee` 只调用一次 `engine.sortedActiveSessions()`；把该 slice 传给 `playerMeleeTarget`，后者不得再次借用同一 scratch。攻击者与目标都必须在该快照中且 `health > 0`；目标选择只读同维 `PlayerActive` 的 `physics.PlayerBounds(target.state.Position)`，先找玩家最近距离，再用以下遮挡判据：

```go
hit, blocked, err := core.RaycastBlocks(
	eye,
	LookDirection(player.yaw, player.pitch),
	playerMeleeReach,
	blockRaycastSampler(dimension),
)
if err != nil || blocked && hit.Distance < targetDistance {
	return 0, false
}
```

- [ ] 不向 `core` 或 `physics` 新增只服务一个调用者的抽象；不复制 DDA 或 `InteractionTarget`。
- [ ] 在 `playerState` 只增加 `meleeCooldownTicks uint8` 和 `meleeSuppressedMining bool` 两个瞬态字段；不进 `PlayerUpdate`、snapshot、hash 或存档。
- [ ] 格式化、复绿并提交：

```bash
gofmt -w internal/sim/combat.go internal/sim/combat_test.go internal/sim/player.go
go test ./internal/sim -run 'Test(RayAABBDistance|PlayerMeleeTarget)' -race -count=1
git diff --check
git add internal/sim/combat.go internal/sim/combat_test.go internal/sim/player.go
git commit -m "feat: select authoritative melee targets"
```

- [ ] 完成 task 双裁决和 ledger 记录。

## Task 3: 接入 tick、冷却、伤害、互杀与采掘互斥

**Files:**
- Modify: `internal/sim/combat.go`
- Modify: `internal/sim/combat_test.go`
- Modify: `internal/sim/engine_step.go`
- Modify: `internal/sim/mining.go`
- Modify: `internal/sim/player.go`
- Modify: `internal/sim/death.go`
- Modify: `internal/sim/player_health_test.go`
- Modify: `internal/sim/mining_test.go`

- [ ] 先写红测：按住 primary action 首 tick命中 2 点；随后 9 tick 不再扣血，第 10 个间隔 tick 再命中；松手不攻击；命中玩家时原采掘进度清零；没有合法玩家目标时既有采掘逐 tick 不变。
- [ ] 写伤害复用红测：近战重置 regen 计时、中断进食；致死同 tick 走现有掉落/回满/重生且发布中不出现 health 0。
- [ ] 写并发语义红测：两名 2 血玩家相向同 tick primary action 后双方都死亡结算；8 名玩家的意图按攻击者 `SessionID` 应用且每名最多一条。
- [ ] 运行红测：

```bash
go test ./internal/sim -run 'TestPlayerMelee|TestMelee.*Mining|TestMelee.*Death' -count=1
```

- [ ] 实现 `advancePlayerMelee`：先取得已排序 active session 快照；每名玩家开头清 `meleeSuppressedMining` 并把正冷却减 1；只有 `miningHeld && cooldown==0` 才选目标。
- [ ] 合法目标把攻击者 `meleeSuppressedMining=true`、`meleeCooldownTicks=10`，并把 `{attacker,target}` 放进固定 `[8]meleeIntent`；收集结束后按数组现有的攻击者顺序逐条调用 `target.applyDamage(2)`。应用阶段不得因攻击者已被前一意图打到 0 而取消其预收集意图。
- [ ] 在 `engine_step.go` 的 `reconcileSubscriptions` 后插入唯一调用：

```go
engine.advancePlayerMelee()
engine.settleDeaths(pending)
```

不得增加 `stepPhase`；更新相邻中文阶段注释，说明伤害意图先于死亡结算、全部区块写者仍位于订阅收敛之后。

- [ ] 在 `advanceMining` 的玩家入口条件加入 `player.meleeSuppressedMining`；命中玩家时清 `player.mining`，但不改 `miningHeld`，从而只抑制当前 tick。
- [ ] `beginReset` 清零两个近战瞬态字段；`settleDeath` 继续复用 `beginReset`，不另写重生分支。
- [ ] 格式化、复绿并提交：

```bash
gofmt -w internal/sim/combat.go internal/sim/combat_test.go internal/sim/engine_step.go internal/sim/mining.go internal/sim/player.go internal/sim/death.go internal/sim/player_health_test.go internal/sim/mining_test.go
go test ./internal/sim -race -count=1
go test ./internal/archcheck -count=1
git diff --check
git commit -am "feat: resolve player melee on authoritative ticks"
```

- [ ] 完成 task 双裁决和 ledger 记录。

## Task 4: 锁定 Memory/TCP、八玩家和旧采掘兼容

**Files:**
- Create: `internal/server/player_melee_parity_test.go`
- Modify: `internal/server/multiplayer_memory_integration_test.go`
- Modify: `internal/server/transport_parity_integration_test.go`
- Modify: `cmd/mornlea/benchmark_scenario_test.go`

- [ ] 用既有 `openParityTransport` 在 Memory/TCP 各运行同一双玩家脚本：登录、等 Ready、固定位置/朝向、发送 `PlayerInput{Mining:true}`、读取本人 `PlayerState.Health` 和死亡 reset；比较规范 transcript。
- [ ] parity 测试只能从 wire 观察本人 health/reset 和远端位置，不窥探 `playerState`；两传输结果必须逐项相同。
- [ ] 加八玩家集成用例：所有 session 同 tick 输入，确认服务端不 panic、不超过容量、最终 health 与按 `SessionID` 的意图次序一致。
- [ ] 在现有采掘 parity 脚本保持没有玩家位于 3 格射线上，证明 primary action 仍完成相同方块与掉落。
- [ ] 在 benchmark scenario test 明确断言固定 benchmark `PlayerInput` 的 `Mining=false` 且 `scenarioVersion==19`；因此本变更不添加 `19:20` 迁移。
- [ ] 运行并提交：

```bash
gofmt -w internal/server/player_melee_parity_test.go internal/server/multiplayer_memory_integration_test.go internal/server/transport_parity_integration_test.go cmd/mornlea/benchmark_scenario_test.go
go test ./internal/server -run 'Test(PlayerMelee|MemoryTCPParity|EightPlayer)' -race -count=10
go test ./cmd/mornlea -run 'Test.*Scenario' -count=1
go test ./internal/server -race -count=1
git diff --check
git add internal/server cmd/mornlea/benchmark_scenario_test.go
git commit -m "test: lock melee transport parity"
```

- [ ] 完成 task 双裁决和 ledger 记录。

## Task 5: 全量验证和整分支终审

**Files:**
- Modify: `openspec/changes/authoritative-player-melee/tasks.md`
- Modify: `openspec/changes/authoritative-player-melee/ledger.md`

- [ ] 检查 `git diff BASE..HEAD`：无新 wire 字段、packet ID、schema、ABI、scenario 迁移、武器/击退/伙伴路径；`AGENTS.md` 与 `CLAUDE.md` 必须逐字节相同。
- [ ] 运行：

```bash
make rust
make rust-check
go test ./internal/network ./internal/sim ./internal/server -race -count=1
go test ./internal/archcheck -count=1
go test ./... -race
go vet ./...
gofmt -l .
cmp -s AGENTS.md CLAUDE.md
openspec validate --all --strict --no-interactive
git diff --check
```

- [ ] `gofmt -l .` 无输出；记录 benchmark scenario 仍为 v19；不放宽正确性、容量或 parity 门禁。
- [ ] 提交 ledger/tasks，生成 `BASE..HEAD` committed review package 与 SHA-256，派全新 reviewer 做整分支 SPEC/QUALITY 终审，修复最多 5 轮。
- [ ] 正常 push 并创建独立 PR；不自行归档。
