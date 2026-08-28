# B-28 权威岩浆与造石实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use `subagent-driven-development` to execute this plan task-by-task with a fresh implementer, a fresh independent reviewer issuing separate SPEC and QUALITY verdicts, fix loops, and an execution ledger. Use `test-driven-development` inside every implementation task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付服务端权威的八级岩浆、地下岩浆囊、水岩浆造石、等级 15 发光、玩家持续燃烧、权威协议状态与橙色 HUD 边缘，并保持单队列有界执行、Memory/TCP parity、整块/probe worldgen parity 和既有版本迁移纪律。

**Architecture:** `internal/core` 追加连续岩浆 ID 与封闭 `FluidKind`；`internal/fluid` 在唯一按 `(dueTick, position)` 排序的有界队列中按 kind 调度并在同一候选 reducer 内完成传播和反应；`internal/sim` 持有 tunables、权威写入、重扫、浸没和燃烧；Rust engine 生成地下岩浆囊并 mesh 同材质流体斜面；Go render scheduler 只把水送入既有 water pass，岩浆和黑曜石走 terrain pass；协议只追加 `PlayerState.Burning`，客户端只呈现权威镜像。

**Tech Stack:** Go 1.26、Rust 1.97.1、`mornlea_engine` C ABI、`mornlea_client` wgpu、OpenSpec、现有 binary protocol/chunk storage/capture/benchmark 工具链。

**Approved Design:** `docs/superpowers/specs/2026-08-28-authoritative-lava-stone-design.md`

## 执行规则

- 本计划现在只可提交为规划产物。A-04、A-05、B-11 未全部合入并归档前，禁止认领、创建 `authoritative-lava-stone` change 或修改功能代码。
- 进入 Task 1 前重新读取根及目标目录 `AGENTS.md`、`openspec/config.yaml`、`docs/development-process.md`、`docs/openspec.md` 和批准设计；设计中的 v29/v9/v8/v19、`BlockIDMax=76`、registry cap 80、`MGW1` layout 2/header 564 都只是历史基线。
- Task 1 从届时 `main` 定义符号 `P0`（协议）、`C0`（chunk schema）、`E0`（engine ABI）、`S0`（scenario）、`B0`（旧 `BlockIDMax`）、`R0`（registry cap）、`L0`（MGW1 layout）、`H0`（header bytes）和 `Q0`（HUD quad cap）。本 change 的最终值只能是 `P0+1`、`C0+1`、`E0+1`、`S0+1`、`B0+9`、`L0+1`、`H0+2`；其余 schema/ABI 保持 Task 1 实测值。
- 任一 Task 发现代码、主规格或依赖合流后的事实与批准设计冲突，先停下并修改本批准设计、proposal/delta/design/tasks，重新取得用户批准后再继续；不在代码中偷偷改契约。单队列、chunk schema 恒等迁移、client ABI 不变和 HUD 容量任一前提失效时必须走这条门禁。
- 控制会话不实现。每个 Task 使用 fresh implementer，随后由 fresh independent task reviewer 分别给出 SPEC 与 QUALITY verdict；修复循环最多 5 轮。执行开始时用 SDD 的 `sdd-workspace`/`progress.md` 保存恢复状态，项目级命令、SHA、评审和 `Ruling:` 同步写入 `openspec/changes/authoritative-lava-stone/ledger.md`。
- 每项行为遵循 red → green → refactor。测试与生产代码同目录；不要新建第二套共享 helper、第二条岩浆队列、动态流体 registry、通用状态效果、生产 fallback 或额外 goroutine。
- 每个 implementer 完成测试、自审并提交后，控制会话才用该 Task 的 BASE..HEAD 生成 review package，派发独立 task reviewer；评审必须同时给出 SPEC 和 QUALITY verdict。修复轮产生新提交并接受 scoped re-review，控制会话不代写实现。
- 每个 Task review clean 后，控制会话立即更新 change `ledger.md` 和对应 `tasks.md` checkbox，并提交 `docs(openspec): record reviewed task`；该 bookkeeping commit 必须在下一 Task BASE 之前产生，因此 final merge-base..HEAD package 不会遗漏累计证据。若 bookkeeping 改变契约而非只记录事实，按实现 diff 重新评审。
- Go/Rust 手工镜像常量必须在同一 Task 一起更新。涉及 Rust 的 clean checkout 先运行 `make rust`；任务闭环运行 focused Go/Cargo 测试，全量 race、workspace-wide `make rust-check` 和视觉门禁只在 Task 7 运行。
- 代码注释、GoDoc 和 Rust doc comment 使用中文，且不得出现 `B-28` 这样的任务编号；规划、OpenSpec、ledger、提交信息可使用任务编号。
- 自动验证不得打开或聚焦前台游戏窗口。程序化纹理与 capture 只能使用仓库自有 producer，不加入外部二进制资产。

## 固定契约

- BlockID 只在最终哨兵前依次 append：`LavaSourceID`、`LavaLevel1ID`..`LavaLevel7ID`、`ObsidianID`、新 `BlockIDMax`。圆石复用既有 `CobblestoneID`，不新增 ItemID。
- `FluidKind` 只有 `FluidNone`、`FluidWater`、`FluidLava`。水和岩浆均为 source level 0、flowing level 1..7。
- 水延迟保持 5 tick，岩浆延迟 30 tick，共享一个 `FluidUpdatesPerTick` 和一个 `FluidRescanCellsPerTick`。
- 给定相同初始世界、相同预算和相同 tick 序列，所有流体逐 tick 确定且不依赖入队/map/chunk 遍历顺序。跨预算、清队列重扫同平衡态只承诺不产生异种反应的同 kind 流体；混合流体不同预算或中途重启可形成不同但合法的固体布局。
- 岩浆 source 六邻含任意水变黑曜石；流动岩浆六邻含任意水变圆石；水不消耗。同一 `Advance` 实际求值的水、岩浆普通候选竞争同一空气格时变圆石。
- 每个处理位置最多 6 个候选；pending、排序和变化格上限为 `6×budget`；现有双通知路径合计最多 `84×budget` 次 kind-aware heap 操作。
- Worldgen 使用 `LAVA_POCKET_SALT = 0x6A09_E667_F3BC_C909` 和设计冻结的整数派生/椭球公式。seed 42 两条身份向量必须逐字段相等。
- 玩家燃烧最多 100 tick，每 20 tick 经 `applyDamage(1)`；水同 tick 熄灭优先；燃烧先于自然回血；运行态不进 player schema/snapshot/hash。
- `Burning=true` 时 HUD 恰有四条 `6×scale`、RGBA `(0.96, 0.30, 0.04, 0.35)` 的橙边；客户端不得从本地浸没、health、block 镜像或输入推断 burning。
- `lava-pocket` 插在 `water-surface-slope` 前；`far-horizon` 保持倒数第二，`water-underwater` 保持唯一末项。benchmark 只允许 `S0→S0+1` 的唯一 workload migration，且 benchmark world 继续 `fluidEnabled=false`。

## 文件结构

- Create: `openspec/changes/authoritative-lava-stone/{.openspec.yaml,proposal.md,design.md,tasks.md,ledger.md}`
- Create: Task 1 `Files` 中逐项列出的 19 个 `openspec/changes/authoritative-lava-stone/specs/<capability>/spec.md` delta。
- Modify: `internal/core/{block.go,block_name.go,fluid.go,block_properties.go}` — ID、kind、属性、光照。
- Modify: `internal/assets/{blocks.go,procedural.go}` — 材质层、registry、原创纹理。
- Modify: `internal/fluid/{queue.go,rules.go}` — kind-aware heap、传播、反应、reducer。
- Modify: `internal/config/config.go`、`internal/sim/{fluid.go,tunables.go,engine_changes.go,player.go,spawn.go,mining.go,companion_action.go}` — 调度、重扫、burning、危险流体和黑曜石边界。
- Modify: `internal/worldgen/generator.go`、`internal/lod/lod.go`、`internal/nativeabi/native.go`、`engine/crates/mornlea_engine/src/{worldgen.rs,ffi.rs,lod.rs}`、`engine/include/mornlea_engine.h` — `MGW1` 和岩浆囊。
- Modify: `internal/storage/{chunk_codec.go,migration.go}` — chunk schema 恒等迁移。
- Modify: `internal/mesh/native_input.go`（Task 2 cap）、`internal/assets/blocks.go`、`engine/crates/mornlea_engine/src/{input.rs,greedy/mod.rs,light.rs}` — registry、同材质角高、光照和边界；`internal/mesh/{registry.go,quad.go}` 仅检查布局不变。
- Modify: `internal/assets/blocks.go`、`internal/render/section_scheduler.go`、`engine/crates/mornlea_client/src/render/{mod.rs,shaders.rs}`、`engine/crates/mornlea_client/shaders/terrain.wgsl` — water/terrain pass 分流与 terrain 流体角高解码。
- Modify: `internal/physics/{types.go,submersion.go}`、`internal/client/{collision.go,predictor.go,predictor_advance.go,predictor_reconcile.go}`、`internal/network/{message_player.go,packet.go,codec_server.go}`、`internal/server/publication.go` — 浸没、协议镜像和 parity。
- Modify: `internal/render/hud/{layout.go,renderer.go,encode.go}`、`cmd/mornlea/{app_frame.go,app_messages.go,capture.go,capture_scene.go}` — burning 边缘和场景状态。
- Modify: `cmd/mornlea/{benchmark.go,benchmark_scenario_test.go}`、`cmd/perfcheck/{compare.go,migration_test.go,helpers_test.go}` — scenario 与唯一迁移。
- Modify at closeout only: Task 7 列出的 19 个主规格、根 `AGENTS.md`、`openspec/config.yaml`、`docs/notes/progress.md`、`docs/feature-backlog.md`；局部 `AGENTS.md` 与 `internal/archcheck/baseline_test.go` 仅检查。

---

## 执行前置门禁（不计入七组交付）

- [ ] **Preflight 1：验证依赖、迁移链和独占集合**

```bash
git status --short
git branch --show-current
git log --oneline -20
openspec list
```

检查 `main` 已包含 A-04、A-05、B-11 的合并和归档，且 A-05/B-11 对 metadata 的演进已收敛为代码、测试和主规格中的单一迁移链；`docs/feature-backlog.md` 中 B-28 已由 planner/控制会话从“设计候选/排队”晋升为“就绪”；逐行核对所有已认领任务的独占文件集，不存在另一个认领者占用协议、chunk/player/metadata schema、engine/client ABI、scenario、BlockID、material layer、capture、worldgen、fluid、player 或 HUD 文件。任一条件失败立即停止，不创建 change 或功能 worktree。

- [ ] **Preflight 2：认领并重新确认设计**

由指定 implementing agent 在基于最新 `main` 的干净协调分支只修改 B-28 行为 `已认领`，认领人写当前 agent 与计划中的功能分支，备注列出上述独占集合，然后做 docs-only 提交：

```bash
git add docs/feature-backlog.md
git commit -m "docs(backlog): claim B-28 lava and stone"
```

认领后调用 `brainstorming`，向用户重新呈现本批准设计、依赖合流后的任何差异和七组拆分；必须取得新的显式批准。未批准不得创建 worktree、change 或派发实现子代理。

- [ ] **Preflight 3：建立实现 worktree 与 SDD 恢复状态**

调用 `using-git-worktrees`，以认领提交为 HEAD 创建 `feat/B-28-authoritative-lava-stone` 隔离 worktree。调用 `subagent-driven-development` 的 `sdd-workspace` 建立本计划专属 `progress.md`，记录 plan identity、merge base、claim SHA，并按 skill 要求写完整 task/file/interface 冲突表和 preflight rulings。完成后才派发 Task 1 的 fresh implementer。

---

### Task 1：通过合流门禁并建立 OpenSpec change

**Files:**

- Create: `openspec/changes/authoritative-lava-stone/.openspec.yaml`
- Create: `openspec/changes/authoritative-lava-stone/proposal.md`
- Create: `openspec/changes/authoritative-lava-stone/design.md`
- Create: `openspec/changes/authoritative-lava-stone/tasks.md`
- Create: `openspec/changes/authoritative-lava-stone/ledger.md`
- Create: `openspec/changes/authoritative-lava-stone/specs/common-block-materials/spec.md`
- Create: `openspec/changes/authoritative-lava-stone/specs/authoritative-fluid/spec.md`
- Create: `openspec/changes/authoritative-lava-stone/specs/rust-engine-worldgen/spec.md`
- Create: `openspec/changes/authoritative-lava-stone/specs/authoritative-mining/spec.md`
- Create: `openspec/changes/authoritative-lava-stone/specs/companion-world-actions/spec.md`
- Create: `openspec/changes/authoritative-lava-stone/specs/fluid-survival/spec.md`
- Create: `openspec/changes/authoritative-lava-stone/specs/authoritative-health/spec.md`
- Create: `openspec/changes/authoritative-lava-stone/specs/authoritative-spawn-support/spec.md`
- Create: `openspec/changes/authoritative-lava-stone/specs/authoritative-farming/spec.md`
- Create: `openspec/changes/authoritative-lava-stone/specs/local-audio-feedback/spec.md`
- Create: `openspec/changes/authoritative-lava-stone/specs/tunable-constants/spec.md`
- Create: `openspec/changes/authoritative-lava-stone/specs/static-block-light/spec.md`
- Create: `openspec/changes/authoritative-lava-stone/specs/fluid-presentation/spec.md`
- Create: `openspec/changes/authoritative-lava-stone/specs/survival-hud-presentation/spec.md`
- Create: `openspec/changes/authoritative-lava-stone/specs/rust-engine-mesh/spec.md`
- Create: `openspec/changes/authoritative-lava-stone/specs/rust-client-render-terrain/spec.md`
- Create: `openspec/changes/authoritative-lava-stone/specs/visual-verification/spec.md`
- Create: `openspec/changes/authoritative-lava-stone/specs/bounded-benchmark-workload/spec.md`
- Create: `openspec/changes/authoritative-lava-stone/specs/rust-engine-lod-shell/spec.md`

**Interfaces:**

- Consumes: 已批准设计、届时 `main` 的代码/测试/主规格、A-04/A-05/B-11 已归档契约。
- Produces: 无未决绝对编号、可 strict-validate 的 change；定义 `P0/C0/E0/S0/B0/R0/L0/H0/Q0` 和本 change 的相对增量。

- [ ] **Step 1：冻结真实基线并验证依赖状态**

```bash
make rust
go test ./internal/core ./internal/fluid ./internal/worldgen ./internal/lod ./internal/physics ./internal/sim ./internal/network/... ./internal/server ./internal/client ./internal/assets ./internal/mesh ./internal/render ./internal/render/hud ./cmd/mornlea -race -count=1
go test ./internal/archcheck -count=1
openspec validate --all --strict --no-interactive
```

从定义点和对应守卫测试读取 `P0/C0/E0/S0/B0/R0/L0/H0/Q0`、player/world/companions/client 版本、材质层末项、capture 场景数和 HUD 当前最坏 quad 数。先把命令、完整基线 SHA、数值和输出摘要写进 Task 1 implementer report；不要用批准设计中的历史值替代实测。

- [ ] **Step 2：使用 `openspec-propose` 一次生成完整 change**

把 Step 1 的全部真实数值、批准设计和 Preflight reapproval 作为完整输入调用 `openspec-propose`；该调用一次生成 `.openspec.yaml`、完整 proposal、delta specs、design 和 tasks，不要求 skill 创建 ledger。`proposal.md` 必须写明目标、非目标、影响面和以下延期：桶/放置命令、火焰方块、通用 effect、非玩家燃烧、黑曜石物品/掉落/配方、洞穴/跨 chunk 湖泊、客户端 burning 预测、队列持久化、跨预算混合固体布局一致性。

- [ ] **Step 3：复核并补齐最容易误实现的 delta 场景**

`authoritative-fluid` 必须明确修改旧保证：

```markdown
### Requirement: Budgeted multi-fluid determinism
系统 MUST 在相同初始方块、相同处理预算与相同 tick 序列下产生逐 tick 相同结果，且结果不依赖入队、map 或区块遍历顺序。

#### Scenario: 同 kind 跨预算与重扫同平衡态
Given 不包含异种流体反应的未平衡水体或岩浆体
When 分别以受限和不受限预算运行，或清空队列后重扫
Then 最终平衡方块状态相同

#### Scenario: 混合不可逆反应不承诺跨预算相同布局
Given 会产生水岩浆反应的未平衡世界
When 使用不同预算或从已保存方块状态重扫
Then 每条执行都满足反应矩阵和各自的逐 tick 确定性，但系统不要求固体布局彼此相同
```

其余 delta 必须覆盖：八级岩浆/黑曜石属性、kind-aware deadline、同批竞争、`6×budget`/`84×budget`、地下岩浆囊向量、`BaseBlockAt`/`TerrainBlockAt` 边界、MGW1 原子失败、旧区块不重生成、chunk 恒等迁移、黑曜石无掉落与伙伴拒绝、spawn 拒绝岩浆、只有水湿润/冲毁作物、只有水触发 water splash、浸没分类、100/20 burning 顺序、严格 `Burning` bool、唯一 emission 表、HUD 四边、同材质 mesh、pass 分流、capture 尾序和 `S0→S0+1` 唯一迁移。

- [ ] **Step 4：复核 design/tasks 并手工增加 ledger**

`design.md` 以批准设计为来源，但把所有绝对版本改成 Task 1 实测值及增量；`tasks.md` 精确对应七组交付。Task 7 同时写出 capture/benchmark/最终验证的可勾选实现项，以及 sync/archive、PR、CI、workspace 清理的 post-implementation runbook；archive 前只要求前者完成，归档后的 PR/CI/清理事实记录到 PR 和 Discussion #71，不反向修改已合入的 archived ledger。手工创建 `ledger.md`，至少含 Baseline、Rulings、Review Log，并从 implementer report 复制 Step 1 证据。若 `B0+9 > R0`，在 design/tasks 明确 Task 2 将 Go/Rust cap 同步提升到最小可容纳值；若当前 HUD 最坏 quad 数 `+4 > Q0`，停止并回到设计裁决，不创建静默扩容方案。

- [ ] **Step 5：strict validate 并由 implementer 提交**

```bash
openspec status --change authoritative-lava-stone
openspec validate --all --strict --no-interactive
git diff --check
```

```bash
git add openspec/changes/authoritative-lava-stone
git commit -m "docs(openspec): propose authoritative lava and stone"
```

提交后由控制会话记录 Task 1 HEAD，生成 BASE..HEAD review package 并派发独立 SPEC/QUALITY task review；评审通过后提交 Task 1 ledger bookkeeping。随后调用 `openspec-apply-change` 进入已批准 change 的实现模式，确认 status 与 tasks 可读取，才派发 Task 2。

---

### Task 2：建立 Core 多流体模型、资产和静态玩法边界

**Files:**

- Modify: `internal/core/block.go`
- Modify: `internal/core/block_name.go`
- Modify: `internal/core/fluid.go`
- Modify: `internal/core/block_properties.go`
- Test: `internal/core/fluid_test.go`
- Test: `internal/core/block_name_test.go`
- Test: `internal/core/farming_test.go`
- Test: `internal/core/item_test.go`
- Test: `internal/core/block_properties_test.go`
- Test: `internal/core/light_block_test.go`
- Modify: `internal/assets/blocks.go`
- Modify: `internal/assets/procedural.go`
- Test: `internal/assets/blocks_test.go`
- Test: `internal/assets/procedural_test.go`
- Inspect and modify when `B0+9 > R0`: `internal/mesh/native_input.go`
- Inspect and modify when `B0+9 > R0`: `engine/crates/mornlea_engine/src/input.rs`
- Test: `internal/mesh/native_input_test.go`
- Test: `internal/mesh/native_capacity_test.go`
- Modify: `internal/sim/mining.go`
- Modify: `internal/sim/companion_action.go`
- Modify: `internal/sim/fluid_crop.go`
- Modify: `internal/sim/farmland_moisture.go`
- Test: `internal/sim/mining_test.go`
- Test: `internal/sim/companion_mining_test.go`
- Test: `internal/sim/fluid_crop_test.go`
- Test: `internal/sim/farmland_moisture_integration_test.go`

**Interfaces:**

```go
type FluidKind uint8

const (
	FluidNone FluidKind = iota
	FluidWater
	FluidLava
)

func FluidKindOf(BlockID) FluidKind
func FluidBlock(FluidKind, uint8) (BlockID, bool)
func IsFluid(BlockID) bool
func IsWater(BlockID) bool
func IsLava(BlockID) bool
func FluidLevel(BlockID) uint8
```

- Produces: append-only `LavaSourceID..LavaLevel7ID, ObsidianID`；`LayerLava`、`LayerObsidian`；统一 emission/attenuation/opaque/collision/raycast/placement 属性；黑曜石玩家可破坏但无掉落，伙伴拒绝。

- [ ] **Step 1：先写 append-only 与 fluid API 失败测试**

在 `internal/core/fluid_test.go` 穷举 `0..BlockIDMax+1`，断言：

```go
if ObsidianID != LavaSourceID+8 || BlockIDMax != LavaSourceID+9 {
	t.Fatal("岩浆与黑曜石必须只在旧哨兵前连续追加")
}
for level := uint8(0); level <= 7; level++ {
	id, ok := FluidBlock(FluidLava, level)
	if !ok || id != LavaSourceID+BlockID(level) || FluidKindOf(id) != FluidLava || FluidLevel(id) != level {
		t.Fatalf("lava level %d 映射错误: (%d, %v)", level, id, ok)
	}
}
```

另测 unknown kind、level 8、`BlockIDMax` 和未注册 ID fail closed；水既有 0..7 映射逐字不变。先运行并记录失败：

```bash
go test ./internal/core -run 'TestFluid|TestLava|TestObsidian' -count=1
```

- [ ] **Step 2：最小实现连续 ID 与固定 switch**

只在现有 const 尾部 append 九个 ID。`FluidKindOf` 和 `FluidBlock` 使用固定 switch/连续区间，不引入 map、接口或动态 registry。保留 `FluidLevel` 现有调用契约；若其现行签名不能报告错误，只允许在 `IsFluid` 已成立时调用，并让未知值返回不会被误认作 source 的防御值。

- [ ] **Step 3：先写属性、名称和无物品失败矩阵**

覆盖：八个岩浆 ID 无碰撞、可覆盖、射线穿透、emission 15、attenuation 1、无 ItemPlacement/BlockDrop；黑曜石完整碰撞、不透明、emission 0、采掘 30/15/8 tick、`Harvestable=false`、无 BlockDrop；所有注册 ID 有中文名。确认 ItemID/ItemIDMax 不变。

- [ ] **Step 4：实现唯一属性来源和 water-only 审计**

在 `core.BlockEmission` / `core.BlockLightAttenuation` 的现有表或 switch 中登记岩浆，不在 assets/sim/shader 复制 emission。审计全部 `IsFluid` 调用：农业湿润、作物水冲毁、氧气、水下视觉和水花语义改用 `IsWater`；碰撞减速、放置覆盖、射线穿透、spawn 危险流体拒绝和 mesh 高度保留 `IsFluid`。Task 6 才迁移 submersion API，本 Task 只改无需新 API 的 water-only 调用。

- [ ] **Step 5：先写材质层和程序化纹理失败测试**

断言 `LayerLava` 与 `LayerObsidian` 紧跟最终现有材质层、互不重叠；同 seed/输入生成逐字一致、非全透明、非同图；岩浆 alpha 不依赖 water pass，黑曜石为普通 opaque texture。不新增二进制资源。

- [ ] **Step 6：最小实现 assets 与 registry cap**

复用 `internal/assets/procedural.go` 已有像素纹理生成方式，只新增两个短函数/分支。若 `B0+9 > R0`，把 Go 与 Rust 固定 registry cap 同步提升到恰能容纳当前注册表的最小项目惯用上界，并添加“最终 count 成功、cap+1 在写输出前失败”的双侧测试；若无需提升，不改 cap。

- [ ] **Step 7：先写黑曜石采掘/伙伴失败测试，再实现固定分支**

玩家完成采掘时黑曜石置空气、revision/广播照常、drop 数为 0；石镐/铁镐只影响时长，不改变 harvest。该分支放在现有单物品 `BlockDrop` 调用前，不伪造 `ItemNone`。伙伴的采掘可接受性显式拒绝黑曜石；不要扩展伙伴状态或存储。

- [ ] **Step 8：focused 验证并由 implementer 提交**

```bash
gofmt -w internal/core internal/assets internal/mesh internal/sim
make rust
go test ./internal/core ./internal/assets ./internal/mesh ./internal/sim -race -count=1
CARGO_TARGET_DIR="$HOME/.cache/mornlea-cargo-target" cargo +1.97.1 test --locked --manifest-path engine/Cargo.toml -p mornlea_engine input::tests
git diff --check
```

```bash
git add internal/core internal/assets internal/mesh internal/sim engine/crates/mornlea_engine
git commit -m "feat: add lava and obsidian block contracts"
```

控制会话在提交后把 HEAD、命令输出和独立 task review 结论写入 ledger；未获 SPEC PASS / QUALITY APPROVED 前不进入 Task 3。

---

### Task 3：扩展单队列调度、造石 reducer 与固定点

**Files:**

- Modify: `internal/fluid/queue.go`
- Modify: `internal/fluid/rules.go`
- Test: `internal/fluid/queue_test.go`
- Test: `internal/fluid/queue_bounded_test.go`
- Test: `internal/fluid/rules_test.go`
- Test: `internal/fluid/replaceable_test.go`
- Test: `internal/fluid/property_order_test.go`
- Test: `internal/fluid/property_budget_test.go`
- Test: `internal/fluid/property_converge_test.go`
- Test: `internal/fluid/property_rescan_test.go`
- Test: `internal/fluid/e2e_test.go`
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`
- Modify: `internal/sim/tunables.go`
- Modify: `internal/sim/fluid.go`
- Modify: `internal/sim/engine_changes.go`
- Test: `internal/sim/fluid_test.go`
- Test: `internal/sim/fluid_perf_test.go`
- Test: `internal/server/fluid_transport_parity_test.go`

**Interfaces:**

```go
type schedule struct {
	position core.BlockPos
	kind     core.FluidKind
	dueTick  uint64
}

type Delays struct {
	Water uint64
	Lava  uint64
}

func (q *Queue) Enqueue(pos core.BlockPos, kind core.FluidKind, now, delay uint64)
func (q *Queue) Cancel(pos core.BlockPos)
func (q *Queue) Advance(now uint64, world FluidWorld, budget int, delays Delays) []core.BlockPos
```

`Queue` 可保留现有公开 API 名，但队列项必须携带 `kind`；enqueue/cancel 必须通过现有 `order/index` 双射做 O(log n) add/fix/remove，不能扫描全 heap。

- [ ] **Step 1：先写 kind-aware deadline 失败测试**

逐例锁定：未排队按最终 kind 添加；同 kind 保留较早 deadline；已到期水收到重复通知仍保持到期；水改岩浆精确替换为 `now+30`，即使比旧 deadline 更晚；岩浆改水精确替换为 `now+5`；最终非流体 O(log n) cancel；弹出时世界 kind 不匹配只按当前 kind 重排且本次不求值；`order/index` 双射始终成立。

- [ ] **Step 2：最小实现 kind-aware heap update**

复用现有 heap 和位置全序。为每个条目保存 `kind`，同 kind 时 `min(oldDue,newDue)`，kind 变化时赋精确新 deadline 并 `heap.Fix`，非流体时按 index `heap.Remove`。不持久化队列，不增加第二条队列或按 kind map。

- [ ] **Step 3：先写同种传播和替换矩阵失败测试**

水/岩浆分别覆盖 source、上方支撑、水平更强等级支撑、异种不支撑、level 7 不水平扩散、垂直优先；普通 target 允许空气、同 kind 更弱流动格和开启门。source、同 kind 更强/同级、异种、关闭门和实心拒绝。作物保持既有特例：水继续通过现有原子掉落路径冲毁作物，岩浆拒绝作物且不得产生水式掉落。

- [ ] **Step 4：把规则函数显式带入 kind**

保持固定 switch：由 `FluidKindOf`/`FluidBlock` 生成结果，不建立数据驱动表。普通传播每个位置最多 4 个候选；失去同 kind 支撑的非 source 写空气；source 不因普通传播消失。

- [ ] **Step 5：先写六邻反应和同批竞争失败测试**

测试 source lava 与六方向、全部水等级组合均写黑曜石；每个 flowing lava 等级与任意水写圆石；水格保持不变。单独锁定“只有水项在第 5 tick 到期、相邻 lava 的第 30 tick 尚未到期”时，求值水格会为每个相邻 lava 产出固体候选并抑制该水格普通传播，不能等待 lava deadline。另写：

```text
water - air - lava
budget=2: 两侧同批实际求值，中格归并为 CobblestoneID
budget=1: 位置全序先把中格写 water，后续 lava source 变 ObsidianID
```

每个预算分别反转入队顺序并逐 tick 相等；不得断言 budget 1 与 2 的最终固体布局相等。增加设计中的三来源夹具，证明两个预算都合法、可重复且可以不同。

- [ ] **Step 6：实现可交换、可结合的候选 reducer**

候选只含 target 和最终 BlockID/固定 fluid facts。reducer 固定优先级：反应固体高于普通流体/空气；同 kind 流体取较小 level；同批水+岩浆普通候选取 `CobblestoneID`；重复值幂等。使用排列测试穷举同一候选 multiset 的所有顺序，结果必须一致。处理 lava 时命中水只写当前 lava 并停止；处理 water 时为每个相邻 lava 写对应固体且本格不再普通传播。两类反应分支最多六候选且与普通传播互斥。

- [ ] **Step 7：先写 tunable、通知、重扫与固定点失败测试**

先在 `internal/config`/`internal/sim` 锁定 `LavaFlowDelayTicks` 默认 30、合法区间和现有 config clamp/round-trip/字段穷举；锁定每 tick 入口只快照一次水 5、岩浆 30 和共享 budget/rescan。再写失败测试证明 `sim.recordChange` 与 `Queue.Advance` 两条通知都按最终 BlockID kind 调度、重扫逐格按真实 kind 入队，且 source 五个传播方向均阻挡但第六邻是异种流体时仍不是 fixed point。

- [ ] **Step 8：接入 tunables、最终值通知和重扫**

在 `internal/config/config.go` 与 `sim.Tunables` 同步新增单个 `LavaFlowDelayTicks` 字段；不新增第二套流体预算。实现 Step 7 的通知和 fixed-point 行为，保持所有查询有界。

- [ ] **Step 9：先写收敛、顺序、重启和固定工作上界测试**

纯水与纯岩浆分别验证受限/不受限预算同平衡态、清队列重扫与不中断运行同平衡态。混合夹具只验证相同预算的重复运行、反向入队、不同 map/chunk 顺序逐 tick 一致；从已保存方块清队列后只验证合法确定性收敛。所有已平衡水体、岩浆体、圆石和黑曜石重扫零变化。

用现有 counting world/queue fixture 记录单次 `Advance` 的 processed、candidates、writes、neighbor visits 和 enqueue/cancel 请求。断言：

```go
if candidates > 6*budget || pendingHighWater > 6*budget || sortedHighWater > 6*budget ||
	writes > 6*budget || heapOps > 84*budget {
	t.Fatal("流体推进超过固定工作上界")
}
```

测试同时覆盖 `recordChange` 嵌套通知和 queue 自身通知。统计只做固定整数计数，不保留无界历史；`internal/sim/fluid_perf_test.go` 记录不同预算下 tick、队列 high-water 和收敛 active ticks，数值不转成易抖动硬阈值。

- [ ] **Step 10：完成最小实现、focused 验证并由 implementer 提交**

只补足 Step 9 暴露的 reducer/重扫缺口和固定整数 high-water 统计；统计在每次 `Advance` 重置，不保留历史 slice/map，不改变生产调度结果。

```bash
gofmt -w internal/config internal/fluid internal/sim
go test ./internal/config ./internal/fluid ./internal/sim -race -count=1
go test ./internal/server -run 'Fluid.*Parity|Fluid.*Publication' -race -count=1
git diff --check
```

```bash
git add internal/config internal/fluid internal/sim internal/server
git commit -m "feat: simulate deterministic lava and stone reactions"
```

控制会话在提交后记录 HEAD 和验证证据，完成独立 task review 与必要 fix/re-review，再进入 Task 4。

---

### Task 4：增加地下岩浆囊、MGW1/engine ABI 与 chunk 恒等迁移

**Files:**

- Modify: `internal/worldgen/generator.go`
- Test: `internal/worldgen/generator_test.go`
- Test: `internal/worldgen/fluid_test.go`
- Test: `internal/worldgen/material_test.go`
- Test: `internal/worldgen/parity_test.go`
- Add through the existing `-update` producer: `internal/worldgen/testdata/golden_seed42_fluid.txt`
- Modify: `internal/lod/lod.go`
- Test: `internal/lod/lod_test.go`
- Modify: `internal/nativeabi/native.go`
- Test: `internal/nativeabi/native_test.go`
- Modify: `engine/include/mornlea_engine.h`
- Modify: `engine/crates/mornlea_engine/src/worldgen.rs`
- Modify: `engine/crates/mornlea_engine/src/lod.rs`
- Modify: `engine/crates/mornlea_engine/src/ffi.rs`
- Modify: `internal/storage/chunk_codec.go`
- Modify: `internal/storage/migration.go`
- Test: `internal/storage/chunk_codec_roundtrip_test.go`
- Test: `internal/storage/chunk_codec_envelope_test.go`
- Test: `internal/storage/migration_test.go`
- Test: `internal/storage/chunk_fluid_test.go`
- Test: `internal/server/multiplayer_restart_test.go`

**Interfaces:**

```rust
const LAVA_POCKET_SALT: u64 = 0x6A09_E667_F3BC_C909;

pub struct Materials {
    // existing fields stay in order
    pub water: u16,
    pub lava: u16,
}
```

- Produces: `MGW1` material table 尾追加 lava、layout `L0+1`、header `H0+2`、engine ABI `E0+1`；`BaseBlockAt` 含 lava，`TerrainBlockAt` 不含；chunk schema `C0+1` 恒等迁移。

- [ ] **Step 1：先构建 Rust 并冻结派生 offsets**

```bash
make rust
```

从最终 `H0` 和材料表位置计算：lava 占新增尾部 u16，perm offset 后移 2；chunk input、probe prefix、LOD input 均只按其现有公式 `+2`。把最终数字写进 change design、双侧常量守卫和 ledger，不复制历史预计 566/574/570/582。

- [ ] **Step 2：先写岩浆囊身份、性质、parity 与 golden 失败测试**

锁定 seed 42：

```text
supercell (0,0): hash 0x30f84176f5be7362, host (0,1), center (4,-39,7), radius (2,2,2)
supercell (-1,-1): hash 0x4d98a136c73e7ec7, host (1,1), center (5,-39,10), radius (3,2,3)
```

再测 Euclidean division、每 supercell 恰一 host、每 chunk 至多一个候选、整数椭球体积 33..73、全部 cell 是 source、lava 及一格外壳留在 host chunk、外壳非 air/非 fluid、只替换 natural stone/ore。逐格比较 `GenerateChunk` 和 `BaseBlockAt` 的内部、边界、外壳；断言 `TerrainBlockAt` 永不返回 lava；地下 lava 不改变 LOD 表层结果。新增 fluid-enabled seed-42 整块 digest golden，与两条身份向量一起锁定全局生成身份；`fluidEnabled=false` 的既有 `golden_seed42.txt` 必须逐字不变。

- [ ] **Step 3：实现共享候选和整数椭球 helper**

复用现有 `ore_hash`，严格按批准设计从 hash 位派生 host/center/radius。不要使用 RNG 或浮点。`generate_chunk` 阶段顺序固定为 terrain/ore → lava pocket → trees → sea flood；单点 `base_block_at` 使用同一候选/椭球/外壳判定。`terrain_block_at` 保持纯自然材料，不返回树、水、岩浆。

- [ ] **Step 4：先写 MGW1/ABI 混装与失败原子性测试**

覆盖 `water==air` 且 `lava==air` 的关闭态、两者均独立于 air/其他材料的开启态，以及 water-only/lava-only 半启用、重复 material、unknown layout、错误 magic/length/range/output cap。另覆盖旧 Go+新 Rust、新 Go+旧 Rust、旧 layout+新 ABI、新 layout+旧 ABI、chunk/probe/LOD 的短 header、尾随和小 output；每个错误都断言调用失败且预填 output sentinel 未变化。

- [ ] **Step 5：扩展 Go header encoder 与 Rust parser**

材料验证必须满足：`water == air` 当且仅当 `lava == air`；开启时 water/lava 彼此不同，也不同于 air 和全部其他材料。半启用、重复、unknown layout、错误 magic/length/range/output cap 在任何输出写入前失败。保持 C 函数签名和 dense/probe 输出布局不变；同步 Go/Rust ABI version 和 `engine/include/mornlea_engine.h`。

- [ ] **Step 6：先写 chunk schema 与旧区块不重生成失败测试**

保存旧 chunk 后切换 generator，再加载时 block/revision 不变且不调用生成器；缺失邻区才按新规则生成。锁定旧 schema 加载的 `Migrated=true`、payload 全字段恒等、新增 lava/obsidian round trip 和 future schema 拒绝。

- [ ] **Step 7：登记 chunk 恒等迁移**

`currentChunkSchema=C0+1`。`chunkMigrations[C0]` 返回 DTO 原值；旧 schema 加载后 `Migrated=true`，block palette、revision、drops/furnaces/chests 等 payload 逐字段不变；下一次正常保存写新 envelope。新增 lava/obsidian round trip 保真，future schema 继续拒绝。不要给旧区块运行 worldgen。

- [ ] **Step 8：focused 验证并由 implementer 提交**

```bash
gofmt -w internal/worldgen internal/lod internal/nativeabi internal/storage internal/server
make rust
go test ./internal/worldgen ./internal/lod ./internal/nativeabi ./internal/storage ./internal/server -race -count=1
CARGO_TARGET_DIR="$HOME/.cache/mornlea-cargo-target" cargo +1.97.1 test --locked --manifest-path engine/Cargo.toml -p mornlea_engine worldgen::tests
CARGO_TARGET_DIR="$HOME/.cache/mornlea-cargo-target" cargo +1.97.1 test --locked --manifest-path engine/Cargo.toml -p mornlea_engine lod::tests
CARGO_TARGET_DIR="$HOME/.cache/mornlea-cargo-target" cargo +1.97.1 test --locked --manifest-path engine/Cargo.toml -p mornlea_engine ffi::tests
git diff --check
```

```bash
git add internal/worldgen internal/lod internal/nativeabi internal/storage internal/server engine
git commit -m "feat: generate enclosed underground lava pockets"
```

控制会话在提交后记录 HEAD 和验证证据，完成独立 task review 与必要 fix/re-review，再进入 Task 5。

---

### Task 5：接通岩浆光照、同材质 mesh 与 terrain pass

**Files:**

- Modify: `internal/assets/blocks.go`
- Test: `internal/assets/blocks_test.go`
- Inspect only: `internal/mesh/registry.go`
- Inspect only: `internal/mesh/native_input.go`
- Inspect only: `internal/mesh/quad.go`
- Test: `internal/mesh/native_input_test.go`
- Test: `internal/mesh/native_parity_test.go`
- Test: `internal/mesh/fluid_light_test.go`
- Test: `internal/mesh/greedy_oracle_test.go`
- Test: `internal/mesh/visibility_test.go`
- Create: `internal/mesh/lava_surface_test.go`
- Modify: `engine/crates/mornlea_engine/src/input.rs`
- Modify: `engine/crates/mornlea_engine/src/greedy/mod.rs`
- Create: `engine/crates/mornlea_engine/src/greedy/lava_corner_tests.rs`
- Modify: `engine/crates/mornlea_engine/src/light.rs`
- Modify: `internal/render/section_scheduler.go`
- Test: `internal/render/section_scheduler_test.go`
- Modify: `engine/crates/mornlea_client/src/render/mod.rs`
- Modify: `engine/crates/mornlea_client/src/render/shaders.rs`
- Modify: `engine/crates/mornlea_client/shaders/terrain.wgsl`
- Test: `engine/crates/mornlea_client/src/render/water_tests.rs`
- Create: `engine/crates/mornlea_client/src/render/lava_tests.rs`

**Interfaces:**

- Consumes: Task 2 的 `LayerLava`/`LayerObsidian` 和最终 registry count/cap；现有 20-byte registry entry、8-byte quad instance、opaque+water 两流上传 ABI。
- Produces: 同 material 才参与流体角高；水只进 water stream，lava/obsidian 只进 opaque/terrain stream；terrain shader 能解释 lava quad 的 corner height；临时异种边界只由 lava 侧出面。

- [ ] **Step 1：先写 registry 与 emission 单一来源失败测试**

断言八个 lava entry 的 `FluidHeight` 与 level 对应、`Emission=15`、`LightAttenuation=1`、六面 material=`LayerLava`；Obsidian fluid height 0、opaque、material=`LayerObsidian`。entry byte layout 和 client ABI 不变；最终 count 恰好可编码，cap+1 原子拒绝。

- [ ] **Step 2：先写同材质角高和异种边界失败测试**

复用 water corner fixtures，新增：相邻同等级/不同等级 lava 连续；water 不抬 lava、lava 不抬 water；上方同 material 流体才给满高；water/lava 邻面只产生一个 lava-side quad；同 kind 内面隐藏；反应固体替换后普通 solid visibility 恢复。测试直接锁定 `assets.Registry.FaceVisible` 烘焙出的 lava→water 可见、water→lava 隐藏规则，以及 Go greedy oracle 和 Rust native 的逐 quad parity。

- [ ] **Step 3：最小修改 Rust mesher 判定**

角高邻居条件从“任意 `fluid_height != 0`”收窄为“`fluid_height != 0` 且目标 material 相同”，Go oracle 与 Rust 实现同步。`assets.Registry.FaceVisible` 只让 lava 侧输出异种边界面。不要增加 fluid kind 字段或改变 20-byte registry entry，不引入第三条几何流。

- [ ] **Step 4：先写 pass 分流失败测试**

同一 section 放 water、lava、obsidian、stone，断言 water stream 只有 `LayerWater`，opaque stream 包含 lava/obsidian/stone；两流合计无丢失无重复；lava-only section 仍上传；water 8-byte instance 和 client upload 签名不变。`lava_tests.rs` 还要逐值钉住 Go `LayerLava`、Rust `LAVA_MATERIAL` 与 `terrain.wgsl` literal 三者相等。

- [ ] **Step 5：实现最小 Go 分流与 terrain shader 解码**

`section_scheduler.go` 继续只以 `q.Mat == assets.LayerWater` 选择 water stream，其余含 `LayerLava` 都走 opaque stream。`render/shaders.rs` 定义与 Go/WGSL 镜像的 `LAVA_MATERIAL`，terrain vertex 解码仅对该 material 使用 quad 已编码 corner heights；普通 terrain/plant/door/torch 保持既有尺寸语义。不要增加 pass、pipeline、动态 buffer 或 client ABI 字段。

- [ ] **Step 6：验证方块光边界**

用现有 light oracle 证明 lava 作为 emission 15 source，服务端/mesh registry 都读 `core.BlockEmission`；静态 block light 仍只穿 Air，不借此 change 改成穿水/岩浆。dirty/revision/generation/presence 上限保持不变。

- [ ] **Step 7：focused 验证并由 implementer 提交**

```bash
gofmt -w internal/assets internal/mesh internal/render
make rust
go test ./internal/assets ./internal/mesh ./internal/render -race -count=1
CARGO_TARGET_DIR="$HOME/.cache/mornlea-cargo-target" cargo +1.97.1 test --locked --manifest-path engine/Cargo.toml -p mornlea_engine greedy::
CARGO_TARGET_DIR="$HOME/.cache/mornlea-cargo-target" cargo +1.97.1 test --locked --manifest-path engine/Cargo.toml -p mornlea_engine light::
CARGO_TARGET_DIR="$HOME/.cache/mornlea-cargo-target" cargo +1.97.1 test --locked --manifest-path engine/Cargo.toml -p mornlea_client render::
git diff --check
```

```bash
git add internal/assets internal/mesh internal/render engine/crates/mornlea_engine engine/crates/mornlea_client
git commit -m "feat: render luminous lava in the terrain pass"
```

控制会话在提交后记录 HEAD 和验证证据，完成独立 task review 与必要 fix/re-review，再进入 Task 6。

---

### Task 6：实现玩家浸没、持续燃烧、协议和 HUD

**Files:**

- Modify: `internal/physics/types.go`
- Modify: `internal/physics/submersion.go`
- Test: `internal/physics/submersion_test.go`
- Modify: `internal/sim/player.go`
- Modify: `internal/sim/engine_step.go`
- Modify: `internal/sim/spawn.go`
- Create: `internal/sim/player_burning_test.go`
- Test: `internal/sim/player_health_test.go`
- Test: `internal/sim/submersion_parity_test.go`
- Test: `internal/sim/spawn_test.go`
- Modify: `internal/network/message_player.go`
- Modify: `internal/network/packet.go`
- Modify: `internal/network/codec_server.go`
- Create: `internal/network/burning_test.go`
- Modify: `internal/network/codec_fuzz_test.go`
- Test: `internal/network/packet_test.go`
- Test: `internal/network/codec_golden_test.go`
- Test: `internal/network/worldtime_test.go`
- Modify: `internal/server/publication.go`
- Test: `internal/server/publication_test.go`
- Test: `internal/server/multiplayer_memory_integration_test.go`
- Test: `internal/server/multiplayer_tcp_gameplay_test.go`
- Modify: `internal/client/collision.go`
- Modify: `internal/client/predictor.go`
- Modify: `internal/client/predictor_advance.go`
- Modify: `internal/client/predictor_reconcile.go`
- Test: `internal/client/submersion_test.go`
- Create: `internal/client/predictor_burning_test.go`
- Test: `internal/client/predictor_reconcile_test.go`
- Modify: `cmd/mornlea/app_messages.go`
- Modify: `cmd/mornlea/app_frame.go`
- Modify: `cmd/mornlea/app_lifecycle.go`
- Test: `cmd/mornlea/app_audio_splash_test.go`
- Test: `cmd/mornlea/app_pause_test.go`
- Test: `cmd/mornlea/app_connection_test.go`
- Test: `cmd/mornlea/presentation_conversion_test.go`
- Modify: `internal/render/underwater.go`
- Modify: `internal/render/hud/layout.go`
- Modify: `internal/render/hud/renderer.go`
- Modify: `internal/render/hud/encode.go`
- Test: `internal/render/underwater_test.go`
- Test: `internal/render/hud/layout_test.go`
- Test: `internal/render/hud/renderer_test.go`
- Test: `cmd/mornlea/capture_hud_test.go`
- Test: `cmd/mornlea/health_hud_test.go`

**Interfaces:**

```go
type FluidSource interface {
	FluidKindAt(core.BlockPos) core.FluidKind
}

type SubmersionState struct {
	BodyInWater bool
	BodyInLava  bool
	EyeInWater  bool
}

func SubmersionAt(position mgl32.Vec3, source FluidSource) SubmersionState
```

`network.PlayerState` 追加严格 bool `Burning`；协议升为 `P0+1`。不修改 player schema、world metadata、companions schema、Rust physics ABI 或 client ABI。

- [ ] **Step 1：先写完整浸没分类与 spawn 失败测试**

覆盖空气、纯水、纯岩浆、眼在水/身体在岩浆、身体跨水和岩浆。身体 AABB 扫描必须走完整固定小范围，不能首命中提前返回。Rust physics 输入只取 `BodyInWater || BodyInLava`；氧气、水下视觉、水花和出水恢复只取 water/`EyeInWater`。同一步先锁定 spawn：lava 候选始终拒绝，water 保持既有“完全浸没”兜底，只有 lava 时回退到既有安全锚点。

- [ ] **Step 2：迁移服务端与客户端到同一几何 helper**

把 `IsFluidAt bool` adapter 改成 `FluidKindAt`；服务端权威与客户端 mirror 都调用 `SubmersionAt`。保留本地 fluid movement prediction，但不把 `BodyInLava` 转成 burning 或 damage。spawn 候选将 lava 视为危险且无“完全浸没兜底”，water 的既有规则保持。

- [ ] **Step 3：先写 100/20 tick、回血、死亡和生命周期失败测试**

逐 tick 锁定：首次接触只设 100/20，不立即伤害；持续接触期间 remaining 每 tick 刷新并保持 100，第 20 tick 扣 1 并重置 damage counter；只有不接触 lava 时 remaining 才递减，离开后最多 100 tick；同 tick 水+lava 清零且不伤害；重新接触刷新 remaining；流体在玩家阶段后流入身体格时下一 tick 才点燃。燃烧伤害与自然回血同 tick 到期时 health 净减 1、回复计时被 `applyDamage` 重置、红色 damage feedback 仍触发；燃烧可致死。死亡、断线、退菜单、新 session 清零；Memory/Disk restart 不恢复计数器；停服墙钟不补算。

- [ ] **Step 4：实现两个非持久运行态计数器**

在现有 active player state 保存 `burnTicksRemaining` 与 `burnDamageTicks`，不放入 storage DTO、snapshot/hash 或网络私有输入。玩家阶段使用互斥分支：若 `BodyInWater`，清零两个计数器并跳过本 tick 的 lava refresh/damage；否则才执行 lava refresh → burn countdown/`applyDamage(1)` → 仅在未接触 lava 时递减 remaining。之后统一进入 natural regeneration → death settlement。难度不影响伤害。

- [ ] **Step 5：先写协议、权威镜像和 transport parity 失败测试**

从 Task 1 冻结的 `PlayerState` 私有字段尾部追加一个 canonical bool 的测试预期，绝对 offset 由 `internal/network/worldtime_test.go` 的尾部链式计算锁定。覆盖 false/true round trip、byte 0/1、非法 2..255、截断、尾随、fuzz、旧 `P0` 在 Play 前拒绝。Memory transport 也必须走相同 Validate/codec，不可直接传 Go struct 绕过。客户端测试覆盖 `Begin`、新鲜 `ApplyPlayerState`、陈旧/相同 tick 拒绝、`Ready=false`/断线/菜单/capture reset/新 session 清零；`app_lifecycle.go.resetSessionOwnedState` 必须重置或替换 predictor，不能只清外围 UI 字段。fresh `Reset=true` 仍以同一包的权威 `Burning` 为准，服务端死亡 reset 必须发布 `Burning=false`。本地 lava、health 和 block prediction 均不能切换 burning。Memory/TCP transcript 对 burning、health、death/reset 和反应后的 chunk change 一致。

- [ ] **Step 6：追加 `Burning` 并接通权威投影与客户端 reset**

在 `SaturationZero` 后、`WorldTimeTicks` 前追加 `Burning`，协议升为 `P0+1`；只修改 server-message codec。服务端发布 `Burning = burnTicksRemaining > 0`。客户端只在通过现有 tick/reconciliation 新鲜度检查后更新镜像；`resetSessionOwnedState` 重建 predictor，连同断线、菜单、capture reset 和新 session 一起清零。

- [ ] **Step 7：先写 HUD 四边布局失败测试**

`Burning=false` 与旧布局/编码逐字相同、零新增 quad；true 时在 HUD stream 最前追加四条，top/bottom 全宽、left/right 避开上下重叠，厚度 `6*scale`、RGBA 精确为 `(0.96,0.30,0.04,0.35)`。后续 hotbar/status/crosshair/text 在其上；红色 world damage overlay 仍先绘，两个反馈同帧可见。最坏 quad 数必须 `<=Q0`。

- [ ] **Step 8：最小接入 frame，不改 client ABI**

只把权威 bool 传给现有 Go HUD layout，继续编码现有 UI quad stream。不要增加 shader、texture、音频、独立 pass、frame TLV 或 Rust client ABI 字段。

- [ ] **Step 9：focused 验证并由 implementer 提交**

```bash
gofmt -w internal/physics internal/sim internal/network internal/server internal/client internal/render cmd/mornlea
make rust
go test ./internal/physics ./internal/sim ./internal/network/... ./internal/server ./internal/client ./internal/render ./internal/render/hud ./cmd/mornlea -race -count=1
git diff --check
```

```bash
git add internal/physics internal/sim internal/network internal/server internal/client internal/render cmd/mornlea
git commit -m "feat: publish authoritative player burning feedback"
```

控制会话在提交后记录 HEAD 和验证证据，完成独立 task review 与必要 fix/re-review，再进入 Task 7。

---

### Task 7：增加 capture/benchmark，完成终审、归档和合流

**Files:**

- Modify: `cmd/mornlea/capture.go`
- Modify: `cmd/mornlea/capture_scene.go`
- Create: `cmd/mornlea/capture_lava_pocket_test.go`
- Modify: `cmd/mornlea/capture_scene_order_test.go`
- Modify: `cmd/mornlea/capture_far_horizon_test.go`
- Modify: `cmd/mornlea/capture_water_underwater_test.go`
- Add through formal producer: `cmd/mornlea/testdata/golden/lava-pocket.png`
- Modify: `cmd/mornlea/benchmark.go`
- Modify: `cmd/mornlea/benchmark_scenario_test.go`
- Modify: `cmd/mornlea/benchmark_report_test.go`
- Modify: `cmd/mornlea/benchmark_v5_test.go`
- Modify: `cmd/perfcheck/main.go`
- Modify: `cmd/perfcheck/compare.go`
- Modify: `cmd/perfcheck/migration_test.go`
- Modify: `cmd/perfcheck/helpers_test.go`
- Test: `cmd/perfcheck/cli_test.go`
- Inspect only: `internal/archcheck/baseline_test.go`
- Modify after sync/archive: `openspec/specs/{common-block-materials,authoritative-fluid,rust-engine-worldgen,authoritative-mining,companion-world-actions,fluid-survival,authoritative-health,authoritative-spawn-support,authoritative-farming,local-audio-feedback,tunable-constants,static-block-light,fluid-presentation,survival-hud-presentation,rust-engine-mesh,rust-client-render-terrain,visual-verification,bounded-benchmark-workload,rust-engine-lod-shell}/spec.md`
- Modify: `AGENTS.md`
- Inspect only: `internal/AGENTS.md`
- Inspect only: `engine/AGENTS.md`
- Inspect only: `cmd/mornlea/AGENTS.md`
- Modify: `openspec/config.yaml`
- Modify: `docs/notes/progress.md`
- Modify: `docs/feature-backlog.md`
- Modify: `openspec/changes/authoritative-lava-stone/{tasks.md,ledger.md}` until archive

**Interfaces:**

- Produces: 一个无窗口 `lava-pocket` 正式场景；scenario `S0+1`；只允许 `S0→S0+1` 的 compare migration；已同步并归档的 OpenSpec；全绿 PR/CI。

- [ ] **Step 1：先写 capture 顺序和状态隔离失败测试**

场景表精确插入：`... lava-pocket, water-surface-slope, ... main-menu, settings-menu, far-horizon, water-underwater`。断言总场景/golden 数为 Task 1 基线 `+1`，`far-horizon` 倒数第二、`water-underwater` 唯一末项。`lava-pocket.Apply` 显式清理菜单、HUD、underwater、damage、remote entities 和旧 burning，再注入本场景权威 burning；切到下一场景后不得泄漏。

- [ ] **Step 2：实现最小确定性 lava fixture**

使用程序化方块构造暗处 pocket：至少一个 source、多个 flowing level、可观察 block-light falloff、同 kind slope、水+source 后 Obsidian、水+flowing 后 Cobblestone、尚未提交反应的临时异种边界、`Burning=true` 橙边和一次红色伤害反馈。fixture 不依赖自然 worldgen 随机落点，也不启动真实前台窗口。

- [ ] **Step 3：先写 scenario 与 dry benchmark 失败测试**

断言 producer 写 `S0+1`；benchmark generator 明确 `fluidEnabled=false`，因此自然 water/lava 都禁用；新增 registry、tick 分支和 HUD 固定容量属于新 workload 身份，但 benchmark 报告字段、完整性和硬错误语义不变。

- [ ] **Step 4：只保留最终相邻 scenario migration**

把 `cmd/perfcheck/compare.go` 现有唯一允许对改为 `baseline.ScenarioVersion == S0 && current.ScenarioVersion == S0+1`，同时要求相同 transport/producer 身份等既有条件；同步 `cmd/perfcheck/main.go` 的 `S0:S0+1` CLI 文案。测试：`S0→S0+1` 通过，反向、跨两版、旧历史对、跨 transport+跨 scenario 均拒绝；不要累积多条历史特判。

- [ ] **Step 5：focused 验证并生成视觉 golden**

```bash
gofmt -w cmd/mornlea cmd/perfcheck
go test ./cmd/mornlea ./cmd/perfcheck -race -count=1
make visual-update
make visual-check
```

逐图审核 `lava-pocket.png`；既有 golden 若变化，逐张记录可归因原因并取得批准，不能批量接受或调宽阈值。性能 producer 生成一份正式 dry 报告并用 perfcheck 比较；只记录数值，不因性能波动改退出状态，但 overflow、缺样本、身份、数据丢失和 I/O 错误仍硬失败。

- [ ] **Step 6：由 implementer 提交并完成 Task 7 review**

Task 7 implementer 先把 `AGENTS.md`、`openspec/config.yaml` 和 `docs/notes/progress.md` 更新为最终真实版本矩阵，但不宣称全量门禁已通过；backlog 仍保持已认领，OpenSpec 最终验证项仍未勾选。检查 `internal/AGENTS.md`、`engine/AGENTS.md`、`cmd/mornlea/AGENTS.md` 与 `internal/archcheck/baseline_test.go`：它们当前不拥有本 change 的绝对版本值，除非届时真实代码出现对应当前事实，否则不得修改。

```bash
git add cmd/mornlea cmd/perfcheck
git commit -m "test: cover lava visuals and benchmark identity"
git add AGENTS.md openspec/config.yaml docs/notes/progress.md
git commit -m "docs: prepare lava and stone version baselines"
```

控制会话记录 HEAD 和 focused/视觉证据，按普通 Task 流程生成 BASE..HEAD package，取得独立 SPEC PASS / QUALITY APPROVED 并完成最多 5 轮 fix/re-review。此时 OpenSpec `tasks.md` 只可勾选实现、capture、benchmark 子项；Task 7 的 post-implementation runbook 保持未完成。

- [ ] **Step 7：运行包含最终基线文件的整分支独立终审**

调用 `requesting-code-review`，用 merge-base..HEAD package 派发最强可用模型的 fresh whole-branch reviewer，并要求分别给出 SPEC 与 QUALITY verdict。若有 findings，只派发一个 final fix implementer 处理完整清单，再做一次 scoped re-review；残余项逐条 Ruling，不回到每 Task 的五轮循环。最终必须得到 SPEC PASS / QUALITY APPROVED，控制会话不能自评。

- [ ] **Step 8：在干净 closeout SHA 上运行完整门禁**

先调用 `verification-before-completion`，再在最终未改动 SHA 上运行：

```bash
make rust
go test ./internal/core ./internal/fluid ./internal/worldgen ./internal/lod ./internal/physics ./internal/sim ./internal/network/... ./internal/server ./internal/client ./internal/assets ./internal/mesh ./internal/render ./internal/render/hud ./cmd/mornlea -race -count=1
go test ./internal/archcheck -count=1
go vet ./...
make dev-check
make test-race-changed
make test-race
make rust-check
make visual-check
openspec validate --all --strict --no-interactive
git diff --check
```

同 SHA 下已有 ledger 证据可引用；若全量 race 出现仓库已知负载 flake，按 `docs/notes/test-quickstart.md` 分诊并记录，不为无关 flake 修改生产代码。任何真实失败必须由一个 fresh final-gate fixer 修复并提交，随后对 fix diff 做一次 scoped SPEC/QUALITY re-review，再重跑受影响命令和完整门禁；任何 code、spec、contract 或版本矩阵变化都会使旧 whole-branch verdict 失效，必须完成这次 re-review。

- [ ] **Step 9：记录门禁、sync/archive 并完成 backlog**

恢复 Task 7 implementer，把 Step 8 的完整命令、输出摘要和 SHA 写入 ledger，勾选 OpenSpec 最终验证项和 pre-archive runbook。使用 `openspec-sync-specs` 将 delta 合入上列精确主规格，确认 `authoritative-fluid` 已收窄混合确定性保证；使用 `openspec-archive-change` 归档。把 backlog B-28 改为“已完成”并保留认领履历；同级 `CLAUDE.md` 保持薄导入。

```bash
git add openspec docs
git commit -m "docs: archive authoritative lava and stone"
```

归档提交只移动/同步规格、ledger 并完成 backlog，不再改变代码或版本矩阵。随后运行 `go test ./internal/archcheck -count=1`、`openspec validate --all --strict --no-interactive` 和 `git diff --check`；若这三项失败，恢复 Task 7 implementer 修复文档闭环，把所有 tracked 修复先提交，再对 fix diff 做 scoped SPEC/QUALITY re-review并重跑受影响门禁。config、版本文档、golden、backlog 与 archived artifacts 的修复也服从这条规则，不只限稳定规格。stage 前逐项查看 `git status`/`git diff`，不纳入用户或其他 agent 的无关修改。

- [ ] **Step 10：PR、CI、合入和清理**

先从 SDD `progress.md` 收集全部 `Ruling:`，形成执行会话最终回复必须逐条交付的 “Rulings I made” 清单；final review clean 后删除本计划专属 `.superpowers/sdd/<plan-basename>/` workspace，不触碰 sibling workspace。再调用 `finishing-a-development-branch`。默认按 `AGENT_MODE=pr`：检查 status/diff/log/remote/base diff，推送分支，创建标题含 B-28 的 PR，body 附设计、归档 change、版本增量和验证摘要；`gh pr checks --watch` 直到全绿，失败读 `gh run view --log-failed` 并修复，最多 10 轮。若 CI 要求任何 tracked 修复，先重新调用 `sdd-workspace` 建同 plan identity 的新恢复目录，从 archived ledger 恢复既有 rulings，派发 fresh fixer；所有代码、spec、config、版本文档、golden、backlog 或 archived artifact 修复必须先提交，再做 scoped SPEC/QUALITY re-review并重跑受影响门禁。继续 CI 前再次收集新增 rulings、合并进最终清单并删除重建的 SDD workspace。全绿后以 merge commit 合入，本地 `main` 执行 `git pull --ff-only`，再同步 Discussion #71；确认远端事实后删除本 change 的 worktree/分支。绝不 force-push、跳 Hook 或清理无关 worktree。

## 完成定义

- 九个 BlockID append-only，所有新增方块名称、属性、程序化材质、registry 容量和无物品边界可穷举验证。
- 单队列按 kind 调度，水 5/lava 30，同批 reducer 顺序无关；候选 `6×budget`、heap `84×budget` 硬上界通过。
- 同 kind 跨预算/重扫平衡态一致；混合流体测试只承诺相同预算逐 tick 一致，并显式证明不同预算可有不同合法结果。
- seed 42 固定向量、fluid-enabled 整块 golden、dense/probe parity、既有 dry golden、LOD 不变、MGW1/ABI 混装拒绝和失败原子性通过。
- chunk `C0→C0+1` 恒等迁移保留 block/revision，旧区块不重生成；future schema 拒绝。
- 岩浆 emission 15、同材质角高、异种单面和 terrain/water pass 分流通过，client ABI 不变。
- 玩家 100/20 burning、水优先、回血顺序、生命周期 reset、严格协议 bool、Memory/TCP parity 和权威-only 四边 HUD 通过。
- `lava-pocket` 场景与尾序通过，golden 逐图批准；benchmark 仅允许 `S0→S0+1`，dry workload 和完整性硬门禁保持。
- 全分支 SPEC PASS、QUALITY APPROVED、完整验证全绿、OpenSpec 已同步归档、长期版本矩阵和 backlog 已更新、PR/CI 已合入。
