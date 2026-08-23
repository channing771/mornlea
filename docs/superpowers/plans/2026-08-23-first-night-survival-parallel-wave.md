# First-night Survival Parallel Wave Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task with fresh implementers, independent SPEC/QUALITY reviews, and a ledger. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 以一个协议 v27 集成批次并行交付网格合成、可放置火把、分级剑与统一战斗、持久夜行者、床与多人跳夜五项完整生存能力。

**Architecture:** 五项能力各自拥有 OpenSpec change、隔离 worktree、实现与双重独立评审；一个极短的共享契约提交先冻结追加编号、协议消息和有限方块模型标签，随后五条实现线并行。最终只在 `codex/first-night-survival-integration` 合流、统一视觉 golden、版本基线与终审，不把任何不完整的 v27 分支合入 `main`。

**Tech Stack:** Go 1.26、Rust 1.97.1、既有权威 `sim`/`server`、二进制 `network`/`storage`、Rust wgpu 客户端、OpenSpec、Git worktree。

**Spec:** `docs/superpowers/specs/2026-08-23-first-night-survival-parallel-wave-design.md`

## Global Constraints

- 不加入 Mojang 资源、协议或存档；全部纹理、模型和“夜行者”外观必须原创。
- 不新增第三方依赖，不建立 ECS、插件层、任意方块模型系统、通用状态效果或跨区块事务框架。
- 五项能力最终共同使用：协议 v27、player schema v8、metadata v3、`hostile_mobs` schema v1、engine ABI v7、client ABI v8、benchmark scenario v20；chunk schema v9 与 companions schema v4 不变。
- `WorldTimeTicks` 继续每个权威 tick 单调加一；跳夜只修改持久化的 `DayTimeOffsetTicks`。
- 所有工作遵循 TDD；每个实现 Task 完成后先做 SPEC review、再做 QUALITY review，修复循环最多 5 轮，证据与 ruling 写入对应 change 的 `ledger.md`。
- 控制会话只派发、协调与裁决，不直接实现；五条线上每个 Task 都使用全新的 implementer，完成后再交给独立 reviewer。
- 新增测试按单一关注点落入独立文件并复用包内唯一 helper 中心；若必须触碰已经混装多个主题的测试文件，先记录 `go test -list` 集合，再用独立的零行为变化提交完成拆分。Rust `greedy` 测试继续使用同级主题模块与既有 `test_support.rs`，不新建镜像 `tests/` 目录。
- `AGENTS.md` 与 `CLAUDE.md` 只由最终集成 Task 同步修改并逐字节一致；五个功能分支不得分别改这两份高冲突基线。
- capture/golden 只由最终集成 Task 更新；功能分支只提交场景构造和测试，避免并行覆盖 PNG。

---

## Task 1：创建批次集成分支和共享契约提交

**Files:**

- Reference: `docs/superpowers/plans/2026-08-23-authoritative-grid-crafting.md`
- Reference: `docs/superpowers/plans/2026-08-23-placeable-torches.md`
- Reference: `docs/superpowers/plans/2026-08-23-tiered-swords-combat.md`
- Reference: `docs/superpowers/plans/2026-08-23-authoritative-hostile-nightwalker.md`
- Reference: `docs/superpowers/plans/2026-08-23-authoritative-bed-sleep.md`
- Modify: `internal/core/item.go`
- Modify: `internal/core/block.go`
- Modify: `internal/core/recipe.go`
- Create: `internal/core/entity.go`
- Modify: `internal/core/item_test.go`
- Modify: `internal/core/block_test.go`
- Modify: `internal/core/recipe_test.go`
- Modify: `internal/network/packet.go`
- Modify: `internal/network/registry.go`
- Modify: `internal/network/codec_client.go`
- Modify: `internal/network/codec_server.go`
- Create: `internal/network/message_combat.go`
- Create: `internal/network/message_hostile.go`
- Create: `internal/network/message_bed.go`
- Modify: `internal/network/registry_test.go`
- Modify: `internal/network/packet_test.go`
- Modify: `internal/mesh/registry.go`
- Modify: `internal/mesh/native_input.go`
- Modify: `internal/mesh/native_input_test.go`
- Modify: `engine/crates/mornlea_engine/src/input.rs`
- Test: `engine/crates/mornlea_engine/src/input.rs`

- [ ] **Step 1: 建立隔离集成分支**

  从当前已提交且干净的 `main` 创建 `codex/first-night-survival-integration` worktree。运行 `git status --short`、`make rust`、`go test ./internal/core ./internal/network ./internal/mesh -race -count=1`；Task 2 创建五份 ledger 后，把这组基线证据写入每份 ledger。

- [ ] **Step 2: 先创建并评审五个 OpenSpec changes**

  在 integration 分支依次执行五份功能计划的 Task 1，完成所有 proposal/spec/design/tasks/ledger、strict validate 与独立评审；把这批纯规划产物提交为 `docs: propose first-night survival wave`。后续功能 worktree 已包含这些产物，不重复创建。

- [ ] **Step 3: 先写追加编号与哨兵失败测试**

  锁定且只追加以下顺序：

  - Item 37..47：`Stick`、`Workbench`、`Torch`、三把剑、三把损坏剑、`RottenFlesh`、`WhiteBed`；`ItemIDMax == 48`。
  - Block 45..58：`Workbench`、站立火把、四向墙火把、四方向各 foot/head 的八个床块；`BlockIDMax == 59`。
  - Recipe 12..18：`Stick`、`Workbench`、`Torch`、三把剑、`WhiteBed`。

  此提交只登记稳定编号、名称和注册/栈上限/耐久的封闭表，以及 `core.HostileMobID uint64`、`core.CombatTargetKind` 两个跨协议值；`CombatTargetPlayer=1`、`CombatTargetHostile=2`。不加入配方匹配、放置、战斗或睡眠行为。

- [ ] **Step 4: 先写协议 v27 registry/codec 失败测试**

  冻结 Play 消息：

  ```text
  C->S 7  MoveCraftingStack     Sequence u64, From u8, To u8
  C->S 14 TakeCraftingOutput    Sequence u64
  C->S 15 UseBed                Sequence u64, Yaw f32, Pitch f32
  S->C 21 CraftingState         Size u8, Grid[9] ItemStack, Output ItemStack
  S->C 22 CombatHit             ServerTick u64, Damage u8, TargetKind u8
  S->C 23 HostileMobSpawn       bounded records
  S->C 24 HostileMobStates      bounded records
  S->C 25 HostileMobDespawn     bounded IDs
  ```

  v27 删除 `CraftRecipe` 的线上注册但保留编号 7；旧 v26 一律在握手拒绝。`PlayerState` 尾部追加 `DayTimeOffsetTicks u16` 与 `Sleeping u8`，值域分别为 0..23999 和 0/1。只写类型、codec、长度/值域门禁与往返测试；各服务端处理器留给功能分支，不写空 handler。

- [ ] **Step 5: 先写有限模型标签输入失败测试**

  在 `internal/mesh` 定义封闭 `BlockModel uint8`：cube、plant、五种 torch、八种 bed，共 15 个合法值；`RegistryReader.Model` 和 `BlockProperties.Model` 成为唯一查询入口。registry entry 在末尾追加 1 byte model，18→19 bytes，容量 48→64；Go 与 Rust 都拒绝未知 tag、重复 ID、超 64 项和截断输入。engine ABI 的实际升版与几何由火把分支完成，此步只冻结输入布局。

- [ ] **Step 6: 运行共享契约检查并提交**

  ```bash
  gofmt -w internal/core internal/network internal/mesh
  make rust
  go test ./internal/core ./internal/network ./internal/mesh -race -count=1
  go test ./internal/archcheck -count=1
  git diff --check
  ```

  通过独立 SPEC/QUALITY review 后提交 `feat: reserve first-night survival contracts`，记录 commit SHA。其余四个 worktree 必须从该 SHA 创建；crafting 实现也从该 SHA 继续。

## Task 2：并行派发五项功能

**Files:**

- Reference: `docs/superpowers/plans/2026-08-23-authoritative-grid-crafting.md`
- Reference: `docs/superpowers/plans/2026-08-23-placeable-torches.md`
- Reference: `docs/superpowers/plans/2026-08-23-tiered-swords-combat.md`
- Reference: `docs/superpowers/plans/2026-08-23-authoritative-hostile-nightwalker.md`
- Reference: `docs/superpowers/plans/2026-08-23-authoritative-bed-sleep.md`

- [ ] **Step 1: 从共享 SHA 创建五个 worktree**

  分支名依次为 `codex/authoritative-grid-crafting`、`codex/placeable-torches`、`codex/tiered-swords-combat`、`codex/authoritative-hostile-nightwalker`、`codex/authoritative-bed-sleep`。

- [ ] **Step 2: 启动五条并行任务队列**

  五条线可同时推进，但每份功能计划中的每个 Task 都必须派发一个全新 implementer，不能让单个 implementer 包办整份计划。其唯一 brief 是当前 Task、共享契约 SHA、对应计划与 OpenSpec change；禁止自行派生代理。Task 完成后由独立 reviewer 依次作 SPEC/QUALITY 裁决，再进入该线下一 Task。

- [ ] **Step 3: 限制跨线写入所有权**

  - crafting 独占 `recipe.go` 的网格匹配与 recipe 1..18 形状、合成 UI 和 crafting 消息处理。
  - torches 独占 mesh model ABI/几何和核心方块光查询。
  - swords 独占共享战斗结算与命中确认。
  - nightwalker 独占 `hostile_mobs` 存储、AI、敌对实体消息与呈现。
  - bed 独占 metadata/player schema、睡眠状态与床复活点。

  nightwalker 在自己的分支使用最小的本地 mob damage 入口测试持久化/AI，不复制 player melee；bed 在自己的分支用 `HostileWithin` 函数参数/闭包测试阻睡，不实现假 mob。两处真实连接留到集成 Task 3。

## Task 3：按固定顺序合流并补两个真实连接

**Files:**

- Modify: `internal/sim/combat.go`
- Modify: `internal/sim/engine_step.go`
- Modify: `internal/sim/sleep.go`
- Modify: `engine/crates/mornlea_engine/src/greedy/model.rs`
- Create: `internal/sim/combat_hostile_test.go`
- Create: `internal/sim/sleep_hostile_test.go`
- Test: `internal/mesh/bed_test.go`
- Modify: `openspec/changes/*/ledger.md`

- [ ] **Step 1: 固定合流顺序**

  在 integration 分支依次合并 crafting → torches → swords → nightwalker → bed。每次合并后运行该功能计划列出的 focused tests；冲突按共享契约和对应 change spec 裁决，不机械选择 ours/theirs。

- [ ] **Step 2: TDD 接通剑与夜行者**

  先写测试证明同 tick 玩家/夜行者攻击意图都在结算前冻结、稳定按 target kind+ID 排序、死亡目标不被重复结算、剑耐久只在确认伤害后消耗。最小修改让 nightwalker 复用 swords 提供的同一 `applyDamage`/候选结算，不保留分支内临时入口。

- [ ] **Step 3: TDD 接通夜行者阻睡**

  先写测试证明附近存活夜行者拒绝睡眠、远处/死亡/已 despawn 夜行者不阻塞；最小修改把 bed 分支的查询闭包接到 sim 内已排序的 hostile snapshot，不引入空间索引。

- [ ] **Step 4: 接通 bed finite model dispatcher**

  在 torches 已交付的 ABI v7 dispatcher 中把 8 个 bed tag 接到 bed 分支的 `emit_bed`；运行完整 pair/孤半床 Go→Rust FFI 测试，确认 unknown tag 仍失败且 cube/plant/torch 输出不变。

- [ ] **Step 5: 执行集成回归**

  ```bash
  make rust
  go test ./internal/core ./internal/network ./internal/storage ./internal/sim ./internal/server ./internal/client ./internal/render/... ./cmd/mornlea -race -count=1
  go test ./internal/archcheck -count=1
  git diff --check
  ```

## Task 4：统一版本基线、benchmark 与 22 个视觉场景

**Files:**

- Modify: `cmd/mornlea/benchmark.go`
- Modify: `cmd/mornlea/benchmark_scenario_test.go`
- Modify: `cmd/mornlea/benchmark_v5_test.go`
- Modify: `cmd/perfcheck/main.go`
- Modify: `cmd/perfcheck/compare.go`
- Modify: `cmd/perfcheck/migration_test.go`
- Modify: `cmd/perfcheck/cli_test.go`
- Modify: `cmd/perfcheck/transport_test.go`
- Modify: `cmd/mornlea/capture_scene.go`
- Modify: `cmd/mornlea/capture_scene_test.go`
- Create: `cmd/mornlea/testdata/golden/workbench-crafting.png`
- Create: `cmd/mornlea/testdata/golden/torch-night.png`
- Create: `cmd/mornlea/testdata/golden/sword-combat.png`
- Create: `cmd/mornlea/testdata/golden/hostile-mob.png`
- Create: `cmd/mornlea/testdata/golden/bed-sleep.png`
- Modify: `AGENTS.md`
- Modify: `CLAUDE.md`
- Modify: `README.md`
- Modify: `README.en.md`
- Modify: `docs/notes/perf-baseline.md`
- Modify: `docs/notes/perf-baseline-m5.md`
- Modify: `docs/notes/progress.md`

- [ ] **Step 1: 升 benchmark scenario 19→20**

  增加唯一迁移 `19:20`，退役 `18:19`；同步 `perfcheck` CLI 文案、严格授权、反向/同版/旧迁移拒绝和跨 transport 测试。场景覆盖9格crafting grid、72个战斗候选上限、64 mobs、75 avatars、64 registry entries的真实容量，不改变性能数值的退出语义，M2 v15/M5 v14 baseline JSON保持逐字节不变。

- [ ] **Step 2: 锁定最终 22 场景顺序**

  ```text
  terrain-noon, hud-hotbar-health, hud-survival-feedback, avatar-nametag,
  inventory-crafting, workbench-crafting, chest-container, furnace-container,
  debug-panel, skylight-tunnel, block-light-room, torch-night,
  materials-showcase, target-block-feedback, oak-grove, ai-companion,
  sword-combat, hostile-mob, bed-sleep, water-surface-slope,
  far-horizon, water-underwater
  ```

  `far-horizon` 必须倒数第二，`water-underwater` 必须唯一末场景。

- [ ] **Step 3: 在可用 GPU 环境生成并逐图验收**

  运行 `make visual-update VISUAL_OUT=build/visual-first-night`，逐张查看 5 张新增图以及相邻 HUD/光照场景；禁止调宽视觉阈值掩盖差异。提交 PNG 前运行普通 `make visual` 证明可复现。

- [ ] **Step 4: 同步长期基线**

  更新当前能力、协议/存档/ABI/scenario/场景数；复制保证 `cmp -s AGENTS.md CLAUDE.md`。只写已经集成且验证的事实，不写本批次非目标。

## Task 5：整分支终审、归档与推送 main

**Files:**

- Modify: `openspec/specs/**/spec.md`
- Move: `openspec/changes/{authoritative-grid-crafting,placeable-torches,tiered-swords-combat,authoritative-hostile-nightwalker,authoritative-bed-sleep}` to dated archive paths
- Modify: five `ledger.md` files

- [ ] **Step 1: 运行完整验证**

  ```bash
  make rust
  go test ./... -race
  go vet ./...
  test -z "$(gofmt -l .)"
  openspec validate --all --strict --no-interactive
  make visual VISUAL_OUT=build/visual-first-night-check
  git diff --check
  ```

- [ ] **Step 2: 独立整分支终审**

  reviewer 逐项核对五份 spec、共享资源上限、并发/持久化错误路径、协议 codec/fuzz/golden、Rust/Go ABI 对称、22 张视觉图和无版权资源；问题按最多 5 轮修复并写 ledger。

- [ ] **Step 3: 同步并归档五个 change**

  先执行 `openspec sync`/对应 sync skill 把 delta 沉淀到主规格，再逐个归档；归档后重跑 strict validate 与完整测试，确保没有丢失跨 change 的 MODIFIED requirement。

- [ ] **Step 4: 合并并推送**

  只在所有验证与终审通过后把 integration 分支 fast-forward/merge 到 `main`，推送 `origin/main`。不得强推；若远端 main 前进，先获取并在 integration 分支重放验证。
