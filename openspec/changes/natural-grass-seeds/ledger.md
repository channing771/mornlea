# Natural Grass Seeds Ledger

## Setup

- OpenSpec change: `natural-grass-seeds`.
- Planning baseline commit: `b7a1ec00237ee81d51c3fc25b677896ddc2c9eb9`.
- Isolated worktree: `/Users/chen/work/mornlea/.worktrees/sim-ownership-convergence`.
- Branch: `refactor/sim-ownership-convergence`.
- Implementation dispatch baseline: `PENDING`（控制会话创建 proposal/specs/design/tasks/ledger 的 planning-only commit；派发 Task 1.1 brief 时在外部钉住该 planning commit SHA，并在 Task 1.1 完成后的 tasks/ledger-only evidence commit 中回填；Task 1.1 的实际 baseline SHA 同样在该 evidence commit 中回填）。
- Version boundary: protocol v32、player schema v8、chunk schema v9、world metadata v3、`companions.ai` schema v4、`hostile_mobs` schema v1 与 client ABI v13 保持不变；本 change 只把 engine ABI v9 升到 v10、benchmark scenario v20 升到 v21。
- Scope owner: `codex-control`；B-04 的实现范围以 proposal、十一份 delta specs、design、tasks 与本 ledger 为准。
- Control-session rule: 控制会话只派发、协调和裁决，不直接修改实现；Task 1.1–8.1 各由 fresh implementer 完成，再由互相独立的 fresh SPEC/QUALITY reviewers 双裁决。
- Repair rule: R1–R3 续用原 implementer；R4–R5 换 fresh implementer；超过五轮必须停止并逐条记录 `Ruling: <决定> — <原因> — <错判成本>`。
- SHA lineage: 每项实现任务先记录 code/test commit `C`；完成全部 scoped repair 后冻结 final verified implementation SHA `I`，所有 focused verification 与最终双评审必须绑定 `I`；随后只允许用 tasks/ledger-only evidence commit `L` 勾选任务并固化证据。`L` 不冒充 `I`，下一任务基线必须包含上一任务 `L`。
- Evidence reuse: 同一 `I` 且实现树未变化时直接引用已记录证据，不重复跑同等全量命令；reviewer 只做 focused 抽查，full race 只在 Task 9.2 与 CI 执行。
- Completion rule: 未记录 baseline、implementer、RED、`C`、`I`、绑定 `I` 的 focused verification、SPEC/QUALITY、repair、`L` 与最终 `Ruling` 的实现任务不得勾选。

Ruling: 追认 B-04 在认领前由排队晋升为就绪 — A-05 已完成且用户明确要求 Implement the plan，控制会话具备晋升权 — 原认领提交遗漏了显式就绪中间态，现于实现派发前恢复记录

Ruling: B-04 不修改 F-04 独占的 `docs/notes/lan-server.md`，由 F-04 独立同步 — 避免并行 backlog 项跨越独占所有权 — 若该边界判断错误，由 F-04 或控制会话在其独占范围内补齐，不扩大 B-04 实现范围

Ruling: candidate `734d8ae8974d8645c1ea6003b8b95c61e2ee081c` 的独立 SPEC reviewer `/root/b04_t1_spec_review` 与 QUALITY reviewer `/root/b04_t1_quality_review` 因 static block light 缺口与主规格冲突均裁决 `FAIL`；用户确认光照规则为 sky 仅由完整不透明方块阻断，static 仅向 `AirID` 或植物格传播、任何其他非空气且非植物方块均阻断；planning commits `ed158474bc8e3c654e0cb5d368bd83c1947a606e`、`df283c41ceaf2061fb601a75545a44807747f47d` 与 `410e0bc6bbf15a3e42124d28c014cdfacf6a6847` 必须先于 R1，修复后重新执行 focused verification 与独立双评审 — 保留主规格冲突会使候选实现“假绿”，撤回植物透光规则则会违背本 change 的 plant 契约与 Minecraft 手感。

Ruling: planning candidate `ab8292569393edfb6748a82f6303c50b0c7654e9` 的独立 PLAN SPEC reviewer `/root/b04_light_plan_spec_review` 裁决 `FAIL`（delta 计数未完整收敛，且 Task 1.1 漏列 `engine/crates/mornlea_engine/src/input.rs` 的 comment-only scope）；独立 PLAN QUALITY reviewer `/root/b04_light_plan_quality_review` 也因同样的 delta 计数与 `input.rs` scope 两项裁决 `FAIL`。规划修复 commits 为 `4314d025ac0f6b4ea600105de6e89b8f8dcd725e` 与 `e8dc416fa4bcbef0eb6b69b43d155782263c89f4`，ledger ownership 修复 commit 为 `ec3eb35cf08566d6a222eacbc19c8097f97b3bd1`；在 clean candidate `ec3eb35cf08566d6a222eacbc19c8097f97b3bd1` 上，`/root/b04_light_plan_spec_review` = `PLAN SPEC PASS`，`/root/b04_light_plan_quality_review` = `PLAN QUALITY PASS`，`openspec validate natural-grass-seeds --strict --no-interactive` = `PASS`，`openspec validate --all --strict --no-interactive` = `80/80 PASS`，`git diff --check` = `PASS`。这些只是 planning re-review 通过证据，不得写成 Task 1.1 的 final `I`、实现 SPEC/QUALITY `PASS` 或 `L`。

## Task 1.1 Stable Block, Plant Presentation, And Passability

- Status: `PENDING`.
- Task baseline SHA: `PENDING`.
- Implementer: `PENDING`（fresh）。
- RED evidence: `PENDING`（命令、失败测试名、失败原因与发生在生产实现前的时序；覆盖稳定编号、无 Item/`BlockDrop`、植物谓词、原创程序化纹理、Go/Rust 真 mesher、碰撞/遮光/射线和伙伴通行；还必须包含真实天空光/静态方块光走廊，以生产 `assets.Registry`、真实 neighborhood 编码与 `mesh.MeshSection` native FFI 取得 Rust packed light，并与 Go light oracle 对同一输入逐格/逐面对照）。
- Code/test commit C: `PENDING`（完整 SHA + 单行英文 subject；不得包含 `tasks.md`/`ledger.md`）。
- Files changed: `PENDING`（本任务独占 block/assets/mesh/physics/passability 目标、`engine/crates/mornlea_engine/src/light.rs` 与同主题 light tests、`internal/mesh` Go light oracle/真实 native FFI 光照测试、`docs/texture-packs.md` 的短草材质入口说明，以及 Rust client 三处过期注释 `engine/crates/mornlea_client/src/render/shaders.rs`、`engine/crates/mornlea_client/shaders/terrain.wgsl`、`engine/crates/mornlea_client/src/render/farmland_tests.rs`；还包含 `engine/crates/mornlea_engine/src/input.rs` 中与 registry record 布局相关的过期注释（comment-only）和 `internal/mesh/fluid_light_test.go` 的旧光照文案。这五处只修正 comments/测试文案，不改 registry record layout、常量、atlas layer、packed light 或任何 ABI）。
- Presentation/provenance evidence: `PENDING`（`ShortGrassID=84`、`BlockIDMax=85`、`LayerShortGrass=68`、`[31..54] ∪ {68}`、`55..67` 不动、registry `85<=96`、`4×8` 字节实例、alpha 仅 `0/255`、无 PNG/外部或 Mojang 资源；光照证据要明确植物格在 sky 中无材料额外衰减、在 static block light 中可按每格 `1` 传播，同时证明玻璃、水、普通方块、未知方块与缺失邻区均阻断 static block light；`levels`、`queue` 与复用 scratch 均保持固定 `48³` 容量，packed light、registry、native mesh/light ABI 与 client ABI v13 不变）。
- Final verified implementation SHA I: `PENDING`（`C` 加本任务全部 repair 后的完整 SHA）。
- Focused verification bound to I: `PENDING`（`make rust`；`cd engine && cargo test -p mornlea_engine --locked plant`；`cd engine && cargo test -p mornlea_engine --locked plant_block_light`；`go test ./internal/core ./internal/assets ./internal/mesh ./internal/physics -race -count=1`（必须包含 Go light oracle 与真实 native FFI 天空光/静态方块光测试）；server path-table、archcheck、race-changed 与 `git diff --check` 的命令、耗时和摘要）。
- Initial failed candidate review: candidate `734d8ae8974d8645c1ea6003b8b95c61e2ee081c`；SPEC `/root/b04_t1_spec_review` = `FAIL`，QUALITY `/root/b04_t1_quality_review` = `FAIL`；两份独立评审均指出 static block light 未实现/未验证，且 sky/static 口径与当时主规格冲突，因此该 SHA 不是 final `I`。
- SPEC reviewer / verdict / findings at I: `PENDING`（fresh，独立于 implementer/QUALITY）。
- QUALITY reviewer / verdict / findings at I: `PENDING`（fresh，独立于 implementer/SPEC）。
- Repair rounds and commits: `R1 READY`（planning re-review 已在 clean candidate `ec3eb35cf08566d6a222eacbc19c8097f97b3bd1` 取得独立 `PLAN SPEC PASS` 与 `PLAN QUALITY PASS`，可回派原 implementer `/root/b04_t1_impl_retry`。R1 由该 implementer 修复 static block light，补齐真实天空光/静态方块光证据、Rust client 三处 stale comments、`engine/crates/mornlea_engine/src/input.rs` comment-only 修正与 `internal/mesh/fluid_light_test.go` 旧文案修正；不得改动 registry record layout、packed light、registry/native mesh/light 或 client ABI。随后产生 repair SHA、重跑 focused verification 并接受 scoped 独立双评审；Task 1.1 final `I`、实现 SPEC/QUALITY 与 `L` 仍为 `PENDING`）。
- Tasks/ledger-only evidence commit L: `PENDING`（只含本任务 checkbox 与 ledger 证据，不改变 `I` 的实现树）。
- Ruling: `PENDING`.

## Task 2.1 Rust Worldgen, MGW1 Layout, And Engine ABI

- Status: `PENDING`.
- Task baseline SHA: `PENDING`.
- Implementer: `PENDING`（fresh；不得复用 Task 1.1 implementer）。
- RED evidence: `PENDING`（layout/长度/材料唯一性/失败缓冲、整块/单点、负坐标/跨区块、树水优先、密度、旧输出归一化、旧存档不调用 generator）。
- Code/test commit C: `PENDING`（完整 SHA + 单行英文 subject；不得包含 `tasks.md`/`ledger.md`）。
- Files changed: `PENDING`（仅 worldgen/nativeabi/LOD/engine ABI、`internal/storage/chunk` 与 `internal/server` 的旧区块加载/重存同主题测试及对应 golden）。
- ABI/version evidence: `PENDING`（engine ABI v10、MGW1 layout 3、15 材料、header `566`、perm offset `54`、chunk input `574`、probe input `570+16N`、LOD input `582` 三端一致；其他版本尤其 client ABI v13 不变）。
- Worldgen compatibility evidence: `PENDING`（升级前四份摘要、新完整摘要、把 `ShortGrassID` 归一为空气后的逐字节等价、固定样本有草也有空隙、负坐标/边界 chunk/probe parity、tree/sea priority）。
- Saved-chunk v9 evidence: `PENDING`（加载升级前已保存的 chunk schema v9 时 generator-not-called；再次保存后全部既有 blocks 逐格不变且 `noShortGrass`）。
- Full-output compatibility evidence: `PENDING`（chunk 完整输出固定 `196608` 字节、probe 完整输出 `8N` 字节、LOD 每 quad `20B` 且保留 two-pass capacity probe/`output_len` 语义；所有输入错误与 failure 都保持完整输出缓冲 untouched）。
- Final verified implementation SHA I: `PENDING`（`C` 加本任务全部 repair 后的完整 SHA）。
- Focused verification bound to I: `PENDING`（clean baseline `make rust`、engine crate、nativeabi/worldgen/lod race、`go test ./internal/storage/chunk ./internal/server -race -count=1`、archcheck、race-changed 与 `git diff --check` 的命令、耗时和摘要；实现者可用 `-run` 收窄当轮定点执行，但最终证据必须包含该可执行命令）。
- SPEC reviewer / verdict / findings at I: `PENDING`.
- QUALITY reviewer / verdict / findings at I: `PENDING`.
- Repair rounds and commits: `PENDING`.
- Tasks/ledger-only evidence commit L: `PENDING`（只含本任务 checkbox 与 ledger 证据）。
- Ruling: `PENDING`.

## Task 3.1 Fluid Replacement And Bounded Support Cleanup

- Status: `PENDING`.
- Task baseline SHA: `PENDING`.
- Implementer: `PENDING`（fresh）。
- RED evidence: `PENDING`（Go oracle、Rust eval/rescan、capacity-full zero-drop、crop regression、support mutation 与 runtime phase order）。
- Code/test commit C: `PENDING`（完整 SHA + 单行英文 subject；不得包含 `tasks.md`/`ledger.md`）。
- Files changed: `PENDING`（仅 fluid kernel/oracle、realm/runtime support cleanup、door/torch support 与同主题测试）。
- Boundedness/atomicity evidence: `PENDING`（稳定 `ChangedBlocks()` 快照；每个 change 只查正上方一格；wild grass → torch → bed；无递归、全世界扫描、新 goroutine 或 I/O；短草容量满仍零掉落替换，作物容量语义不变）。
- Final verified implementation SHA I: `PENDING`（`C` 加本任务全部 repair 后的完整 SHA）。
- Focused verification bound to I: `PENDING`（`make rust`、Rust fluid tests、Go oracle/differential/golden/fuzz、realm/runtime/entity/server fluid race、archcheck、race-changed 与 `git diff --check`）。
- SPEC reviewer / verdict / findings at I: `PENDING`.
- QUALITY reviewer / verdict / findings at I: `PENDING`.
- Repair rounds and commits: `PENDING`.
- Tasks/ledger-only evidence commit L: `PENDING`（只含本任务 checkbox 与 ledger 证据）。
- Ruling: `PENDING`.

## Task 4.1 Authoritative Mining, Stable Seed Drops, And Durability

- Status: `PENDING`.
- Task baseline SHA: `PENDING`.
- Implementer: `PENDING`（fresh）。
- RED evidence: `PENDING`（固定 hit/miss、跨 tick/player/tool 重放、容量满两分支、疲劳/耐久、最后一点耐久与伙伴 planner/executor 双拒绝）。
- Code/test commit C: `PENDING`（完整 SHA + 单行英文 subject；不得包含 `tasks.md`/`ledger.md`）。
- Files changed: `PENDING`（仅 entity mining/yield/durability、companion 双侧拒绝及必要 parity 测试）。
- Drop/atomicity evidence: `PENDING`（salt `0x4752_4153_5353_4544`；roll 只含 seed/dimension/坐标位模式；命中容量满时 block/drop/revision/tool/fatigue 全不变且重试结果稳定；miss 容量满仍清块；成功采草累积既有疲劳但所有手持状态零耐久）。
- Final verified implementation SHA I: `PENDING`（`C` 加本任务全部 repair 后的完整 SHA）。
- Focused verification bound to I: `PENDING`（entity/companion/runtime/server race、archcheck、race-changed 与 `git diff --check`）。
- SPEC reviewer / verdict / findings at I: `PENDING`.
- QUALITY reviewer / verdict / findings at I: `PENDING`.
- Repair rounds and commits: `PENDING`.
- Tasks/ledger-only evidence commit L: `PENDING`（只含本任务 checkbox 与 ledger 证据）。
- Ruling: `PENDING`.

## Task 5.1 Missing-player Inventory And Natural-seed Farming E2E

- Status: `PENDING`.
- Task baseline SHA: `PENDING`.
- Implementer: `PENDING`（fresh）。
- RED evidence: `PENDING`（missing/confirm/disconnect/restore、旧玩家逐槽保留与 Memory/TCP 零种子自然闭环）。
- Code/test commit C: `PENDING`（完整 SHA + 单行英文 subject；不得包含 `tasks.md`/`ledger.md`）。
- Files changed: `PENDING`（仅 persistence starter state、server farming/hunger/transport E2E 与共享 helper）。
- Compatibility evidence: `PENDING`（前 14 个背包格各 64、其余背包和九格快捷栏为空、已有玩家逐槽保留、player schema v8 与 wire/storage 字节布局不变）。
- E2E evidence: `PENDING`（生产 `worldgen.New`/真实 Rust 自然生成；禁止 `flatGenerator`、手工 `SetBlock` 或测试侧复制分布算法；冻结 seed/dimension/hit position；Memory/真实 TCP；权威 drop；前 9/第 10 active tick；最终翻地和种植 parity）。
- Final verified implementation SHA I: `PENDING`（`C` 加本任务全部 repair 后的完整 SHA）。
- Focused verification bound to I: `PENDING`（`make rust`、persistence race、server farming/hunger/transport race、archcheck、race-changed 与 `git diff --check`）。
- SPEC reviewer / verdict / findings at I: `PENDING`.
- QUALITY reviewer / verdict / findings at I: `PENDING`.
- Repair rounds and commits: `PENDING`.
- Tasks/ledger-only evidence commit L: `PENDING`（只含本任务 checkbox 与 ledger 证据）。
- Ruling: `PENDING`.

## Task 6.1 Benchmark V21 And Record-only Workload

- Status: `PENDING`.
- Task baseline SHA: `PENDING`.
- Implementer: `PENDING`（fresh）。
- RED evidence: `PENDING`（producer scenario v21、比较器唯一 `20:21` 迁移、`19:20` 退役、同版本性能退化仍 record-only、结构/身份/overflow/data-loss/I/O 硬失败）。
- Code/test commit C: `PENDING`（完整 SHA + 单行英文 subject；不得包含 `tasks.md`/`ledger.md`，也不得包含 capture/golden 或当前文档同步）。
- Files changed: `PENDING`（仅 `cmd/mornlea/benchmark` 与 `cmd/perfcheck` 的 producer/comparator 代码和测试）。
- Record-only benchmark evidence: `PENDING`（fresh `mktemp -d` 路径、producer SHA=`I`、JSON SHA/身份/transport/样本完整性、v21 self-compare 原始输出与性能数值摘要；不得提升或覆盖 accepted `docs/notes/perf-baseline*.json`）。
- Hard-failure evidence: `PENDING`（性能数值不改变退出状态；报告缺失/损坏、身份或 transport/commit 不匹配、真实 overflow、数据丢失与 I/O 继续硬失败）。
- Final verified implementation SHA I: `PENDING`（`C` 加本任务全部 repair 后的完整 SHA）。
- Focused verification bound to I: `PENDING`（benchmark/perfcheck race、fresh v21 producer/self-compare、archcheck、race-changed 与 `git diff --check`）。
- SPEC reviewer / verdict / findings at I: `PENDING`.
- QUALITY reviewer / verdict / findings at I: `PENDING`.
- Repair rounds and commits: `PENDING`.
- Tasks/ledger-only evidence commit L: `PENDING`（只含本任务 checkbox 与 ledger 证据）。
- Ruling: `PENDING`.

## Task 7.1 Capture And 25-scene Visual Provenance

- Status: `PENDING`.
- Task baseline SHA: `PENDING`.
- Implementer: `PENDING`（fresh）。
- RED evidence: `PENDING`（正式清单仍恰好 25 项且顺序不变、`oak-grove` 自然短草可辨识、完整无窗口链路和既有双阈值）。
- Code/test commit C: `PENDING`（完整 SHA + 单行英文 subject；不得包含 `tasks.md`/`ledger.md`、benchmark 代码或当前文档同步）。
- Files changed: `PENDING`（仅 `cmd/mornlea/capture` 的场景代码/测试与经逐图归因批准的既有 golden；不得新增第 26 场景）。
- 25-scene visual provenance: `PENDING`（更新前全部 25 张 tracked golden SHA；同一 `I` 生成的全部 25 张 current frame；逐图实际像素 diff 且不预设变化数量；每个差异项的 actual/diff 证据、人工归因与原创短草/树水优先检查；实际更新集合及更新后 SHA；所有未变化或不可归因项逐字节不变；`oak-grove` 可辨识；阈值与顺序不变）。
- Visual verification evidence: `PENDING`（fresh 输出目录上的无窗口 capture、只在归因完成后执行的 `make visual-update`、最终 `make visual-check` 与无前台窗口确认）。
- Final verified implementation SHA I: `PENDING`（`C` 加本任务全部 repair 后的完整 SHA）。
- Focused verification bound to I: `PENDING`（capture race、25 场景 provenance/compare、visual check、archcheck、race-changed 与 `git diff --check`）。
- SPEC reviewer / verdict / findings at I: `PENDING`.
- QUALITY reviewer / verdict / findings at I: `PENDING`.
- Repair rounds and commits: `PENDING`.
- Tasks/ledger-only evidence commit L: `PENDING`（只含本任务 checkbox 与 ledger 证据）。
- Ruling: `PENDING`.

## Task 8.1 Current Documentation And Version Matrix

- Status: `PENDING`.
- Task baseline SHA: `PENDING`.
- Implementer: `PENDING`（fresh）。
- RED evidence: `PENDING`（实现前记录 current-doc/version guard 的失败项与过期现行表述；不得把历史记录的旧版本当作 RED）。
- Code/test commit C: `PENDING`（完整 SHA + 单行英文 subject；本任务的实现是 current docs/guides 与相应 guard 更新，不得包含 `tasks.md`/`ledger.md`）。
- Exclusive documentation ownership: `PENDING`（本任务独占根 `AGENTS.md`、`openspec/config.yaml`、`README*.md`、benchmark/capture 局部 `AGENTS.md`、`docs/architecture.md` 与 `docs/notes/{progress.md,gameplay.md,limitations.md,compatibility.md,perf-baseline.md,perf-baseline-m5.md,visual-verification.md,go-rust-division.md}` 的经核实现行段落；`docs/texture-packs.md` 仍由 Task 1.1 独占；Setup 中的 F-04 边界独立生效）。
- Current-version evidence: `PENDING`（现行矩阵统一为 engine ABI v10、benchmark scenario v21、client ABI v13，并保持 protocol v32、player v8、chunk v9、metadata v3、companions v4、hostile v1；自然探索种子入口与 Task 7.1 的实际视觉结果同步，历史记录不批量改写）。
- Scope audit: `PENDING`（逐文件列出更新的现行段落、明确保留的历史段落，以及与 Tasks 1.1–7.1 无路径重叠的证据）。
- Final verified implementation SHA I: `PENDING`（`C` 加本任务全部 repair 后的完整 SHA）。
- Focused verification bound to I: `PENDING`（current-doc/version guards、相关文档检查、`go test ./internal/archcheck -count=1`、named/all strict OpenSpec validation、race-changed 如适用与 `git diff --check`）。
- SPEC reviewer / verdict / findings at I: `PENDING`.
- QUALITY reviewer / verdict / findings at I: `PENDING`.
- Repair rounds and commits: `PENDING`.
- Tasks/ledger-only evidence commit L: `PENDING`（只含本任务 checkbox 与 ledger 证据）。
- Ruling: `PENDING`.

## Section 9. Whole-change Review And T3

### Task 9.1 Whole-change Review

- Status: `PENDING`.
- Review baseline/range: `PENDING`（implementation dispatch baseline...Task 8.1 final `I`；并读取各任务 `L` 中的证据，不把 `L` 当实现 SHA）。
- Whole-change verified implementation SHA before review: `PENDING`.
- Whole-change SPEC reviewer: `PENDING`（fresh）。
- SPEC verdict and complete findings: `PENDING`（逐条对照 proposal、十一份 delta specs、design、tasks、实现和测试）。
- Whole-change QUALITY reviewer: `PENDING`（fresh，独立于 SPEC）。
- QUALITY verdict and complete findings: `PENDING`（所有权、ABI/版本、TDD 锋利度、原子失败、有界热路径、跨语言 parity、原创资源 provenance、benchmark record-only、25 场景归因与范围漂移）。
- Repair ownership: `PENDING`（每项 Critical/Important finding 回派最初拥有该文件的 Task/implementer；控制会话不得代修）。
- Repair rounds and commits: `PENDING`（修复后的 scoped SPEC/QUALITY re-review 必须全部 PASS）。
- Final verified implementation SHA after whole review: `PENDING`（供 Task 9.2 使用）。
- OpenSpec/review verification bound to final SHA: `PENDING`（两份独立 PASS、`git diff --check`、named/all strict validation）。
- Tasks/ledger-only whole-review evidence commit: `PENDING`.
- Ruling: `PENDING`.

### Task 9.2 T3 Gates And Evidence Closure

- Status: `PENDING`.
- Verified implementation SHA for T3: `PENDING`（完整 SHA；先冻结该实现 SHA，再运行和记录全部 T3；不得用之后的 evidence commit 替代）。
- Expected implementation status/diff at verified SHA: `PENDING`.
- `make rust`: `PENDING`（命令、耗时、输出摘要，绑定 verified implementation SHA）。
- `make rust-check`: `PENDING`.
- `test -z "$(gofmt -l .)"`: `PENDING`.
- `go vet ./...`: `PENDING`.
- `go test ./... -race -count=1`: `PENDING`.
- `go test ./internal/archcheck -count=1`: `PENDING`.
- `make test-multiplayer`: `PENDING`.
- `make visual-check`: `PENDING`（不得启动或聚焦前台游戏窗口）。
- Scenario v21 record-only report/perfcheck evidence at the same verified SHA: `PENDING`.
- `openspec validate natural-grass-seeds --strict --no-interactive`: `PENDING`.
- `openspec validate --all --strict --no-interactive`: `PENDING`.
- `git diff --check` and expected status: `PENDING`.
- T3 final release ruling: `PENDING`.
- Tasks/ledger-only T3 evidence commit: `PENDING`（T3 完成后才提交 checkbox 与证据；该提交不回写或声称包含自己的 SHA）。
- Post-evidence closure validation: `PENDING`（只复核 documentation-only scope、named/all strict OpenSpec、`go test ./internal/archcheck -count=1` 与 `git diff --check`；不以 ledger 自引用 SHA 声称 T3 在 evidence commit 上运行，也不因纯证据提交重跑 full T3）。
- Closure ruling: `PENDING`.

## Post-closure Control Sync And Archive

- Closure evidence state: `PENDING`（Task 9.2 evidence commit 后工作树与预期 documentation-only diff）。
- Delta-spec synchronization baseline: `PENDING`.
- Delta-spec synchronization: `PENDING`（控制会话按 `openspec-sync-specs` 审阅并沉淀十一份 delta 到主规格；记录提交、reviewer 与裁决）。
- Post-sync strict validation: `PENDING`（named + all strict、archcheck 与 diff-check）。
- Archive baseline: `PENDING`.
- Archive: `PENDING`（控制会话按 `openspec-archive-change` 执行；记录归档提交、归档前后 all-strict 与 change/archive 唯一性）。
- Backlog / current progress closure: `PENDING`.
- PR / CI / merge: `PENDING`（遵循 `.github/PULL_REQUEST_TEMPLATE.md`、单行英文标题与既有 CI 流程）。
- Final archive ruling: `PENDING`.
