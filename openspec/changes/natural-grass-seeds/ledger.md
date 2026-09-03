# Natural Grass Seeds Ledger

## Setup

- OpenSpec change: `natural-grass-seeds`.
- Planning baseline commit: `b7a1ec00237ee81d51c3fc25b677896ddc2c9eb9`.
- Isolated worktree: `/Users/chen/work/mornlea/.worktrees/sim-ownership-convergence`.
- Branch: `refactor/sim-ownership-convergence`.
- Implementation dispatch baseline: `114003bcca63def9c84fb86da2a449a2aad26288`（创建 proposal/specs/design/tasks/ledger 的 planning-only commit；Task 1.1 实际 baseline 同 SHA，本行在 Task 1.1 完成后的 tasks/ledger-only evidence commit 中回填）。
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

- Status: `DONE`（2026-09-02 控制会话勾选）。
- Task baseline SHA: `114003bcca63def9c84fb86da2a449a2aad26288`。
- Implementer: fresh implementer（初始 `C` 与 R1 两笔修复同一 implementer 续修，符合 R1–R3 续用规则）。
- RED evidence: 初始 `C` 的定点测试由 implementer 会话按先失败后通过完成；QUALITY reviewer 在最终 `I` 上独立复核 R1 新增队列容量测试为真 RED——旧 FIFO 实现在 torch 14 + light 15 混合源下需 110,593 个入队、超出固定 `48³` 容量 110,592,`crop_and_short_grass_plant_block_light_mixed_sources_fit_fixed_queue`（Rust）与 `TestMeshSectionPlantBlockLightMixedSourcesFitFixedQueue`（Go）在旧实现上以真实 `QueueOverflow` 失败;`TestMeshSectionPlantDirectSkyLightMatchesGoOracle`/`TestMeshSectionPlantBlockLightMatchesGoOracle` 以生产 `assets.NewRegistry` + 真实 neighborhood 编码 + `mesh.MeshSection` native FFI 取得 Rust packed light,并与 Go light oracle 对同一输入逐格/逐面对照（绝对值断言 14/13、15/15），未用测试侧复制谓词。
- Code/test commit C: `734d8ae8974d8645c1ea6003b8b95c61e2ee081c` `feat(blocks): add short grass plant semantics`（不含 `tasks.md`/`ledger.md`）。
- Files changed: `docs/texture-packs.md`;mornlea_client 三处 comment-only（`src/render/shaders.rs`、`shaders/terrain.wgsl`、`src/render/farmland_tests.rs` 床 layer 末位过期注释）;mornlea_engine `{greedy/plant_tests.rs, input.rs, light.rs, quad.rs}`;internal/assets `{blocks.go, procedural.go}` + 5 个测试;internal/core `{block.go, block_name.go, block_properties.go, farming.go}` + 6 个测试（含 `short_grass_test.go` 穷举）;internal/mesh `{quad.go, native_input.go(comment-only), fluid_light_test.go, light_oracle_test.go, plant_test.go, greedy_oracle_test.go(comment-only)}`;internal/physics `{types.go, collision_test.go}`;internal/server `{companion_snapshot.go, companion_manager_test.go}`。`input.rs` 除 comment/doc 修正外含 4 行只读 `contains(&self, id)` 访问器（Ruling 追认,见下）。
- Presentation/provenance evidence: `ShortGrassID=84`、`BlockIDMax=85`、`ItemIDMax=53` 不变、无 `ItemID`/放置入口/`BlockDrop`（穷举扫描）;`LayerShortGrass=68` 追加于 `LayerBedHeadEast=67` 后、`55..67` 不动、layerCount 69、registry `85<=96`;Go `mesh.PlantMaterial` 与 Rust `quad::plant_material` 均为 `[31..54] ∪ {68}`,真实 native mesher 每格恰好 `4` 条 `8` 字节实例;`shortGrassTexture` 原创程序化纹理 alpha 仅 `0/255` 且两类像素都存在,默认 pack 不含 `short_grass.png`（断言 `fs.ErrNotExist`）并回落程序化,用户 pack 可经 `textures/short_grass.png` 覆盖;无任何 PNG/二进制/Mojang 资源（numstat 扫描）。光照证据:植物在 sky 中无材料额外衰减（直射 `15` 竖直穿过植物仍 `15`,非直射每轴向步衰减 `1`）,static block light 只向 `AirID` 或植物传播且每格衰减 `1`;玻璃、水、普通方块、未知方块与缺失邻区全部阻断 static block light;sky 未知方块 fail closed 与主规格一致;`levels`/`queue` 保持精确 `48³`,scratch 复用,packed light、registry entry、native mesh/light ABI 与 client ABI v13 均不变。
- Final verified implementation SHA I: `b54abb9abad2bd84f0bbc9ae10de7d62970432a3`（`C` + R1 修复 `fe5d5c728f54baf07cdfd5f26e9ea02750dc0ee1`、`b54abb9abad2bd84f0bbc9ae10de7d62970432a3`）。
- Focused verification bound to I: 2026-09-02 控制会话在 clean candidate `b54abb9a`（工作树干净）执行,全绿:`make rust`（0.5s 增量,dylib 签名回拷成功）;`cargo test -p mornlea_engine --locked plant` = 14 passed/0 failed;`cargo test -p mornlea_engine --locked plant_block_light` = 3 passed/0 failed;`go test ./internal/core ./internal/assets ./internal/mesh ./internal/physics -race -count=1` = 全 ok（27.6s,含 mesh native FFI 光照走廊与 Go oracle 对照）;`go test ./internal/server -run '^TestCompanionManagerPathBlockTableMatchesCollisionOracle$' -race -count=1` = ok;`go test ./internal/archcheck -count=1` = ok;`make test-race-changed RACE_BASE=ec3eb35cf08566d6a222eacbc19c8097f97b3bd1` = 12 包全 ok（3:25,含 server 191s）;`git diff --check` = clean;`openspec validate natural-grass-seeds --strict --no-interactive` = valid。
- Initial failed candidate review: candidate `734d8ae8974d8645c1ea6003b8b95c61e2ee081c`;SPEC `/root/b04_t1_spec_review` = `FAIL`,QUALITY `/root/b04_t1_quality_review` = `FAIL`;两份独立评审均指出 static block light 未实现/未验证,且 sky/static 口径与当时主规格冲突,因此该 SHA 不是 final `I`。planning 修复（`ed158474`→`ec3eb35c`）取得独立 `PLAN SPEC PASS` 与 `PLAN QUALITY PASS` 后进入 R1。
- SPEC reviewer / verdict / findings at I: fresh zcode SPEC reviewer（控制会话派发,独立于 implementer 与 QUALITY）= `SPEC PASS`,0 Critical/0 Important/1 Minor:R1 在 `input.rs` 声明 comment-only 但追加了 4 行只读 `contains(&self, id)` 访问器——无 registry record layout/字段/常量/校验逻辑/ABI 变化,且为 `light.rs` 天空光未知方块 fail closed 所需;建议 ledger 如实记录或迁回 `light.rs`。A–H 八项逐条 SATISFIED（证据含 `internal/core/short_grass_test.go:13-49`、`internal/assets/short_grass_test.go:20-105`、`internal/mesh/plant_test.go:153-189`、`greedy/plant_tests.rs:120-140`、`light.rs:77-151/219-310`、`light_oracle_test.go:394-509` 真实 FFI 走廊等）。
- QUALITY reviewer / verdict / findings at I: fresh zcode QUALITY reviewer（独立于 implementer/SPEC）= `QUALITY PASS`,0 Critical/0 Important/3 Minor:（1）同 `input.rs` `contains` 偏差,建议控制会话追认而非回退;（2）Go oracle 以 `core.IsPlant`、Rust 以 material 集合 `[31..54]∪{68}` 表达植物判定,当前 registry 下 outcome-equal,但 design.md 要求“完全相同的条件”,未来若出现 plant-layer material 与 `IsPlant` 不一致的新方块可能静默分叉,建议 ledger 记录或加穷举守卫;（3）信息性:`build_block` 每存在发射级做一次全 volume 重扫（最坏 16 次,生产 torch=14/light=15 为 3 次）,有界、零分配;旧测试名 `TestGoLightOracleBuildSconsEachCellOnceForEmission` 的“每格一次”性质现仅对无源输入成立。
- Repair rounds and commits: R1 = `fe5d5c728f54baf07cdfd5f26e9ea02750dc0ee1`（`fix(light): let block light pass through plants`）+ `b54abb9abad2bd84f0bbc9ae10de7d62970432a3`（`fix(light): preserve plant sky and queue bounds`）:修复 static block light 向植物传播、sky 直射保持 15、未知方块 fail closed、`build_block` 改为严格降级单次入队桶调度（`tail <= 48³` 可证,fail-safe `QueueOverflow`）、修正 fe5d5c72 一处空洞测试、补齐 Rust client 三处 stale comments、`input.rs` doc 修正（含追认的 `contains`）、`fluid_light_test.go` 旧文案。R2–R5 未触发。
- Tasks/ledger-only evidence commit L: 本 evidence commit（只含 `tasks.md` 1.1 checkbox 与本 ledger 证据,不改实现树;不回写自身 SHA,不冒充 `I`）。
- Ruling: Task 1.1 final `I` = `b54abb9a`,focused verification 与独立双评审均 PASS,勾选成立 — 双评审零 Critical/Important — Minor 项按如下追认/记录。（1）追认 `input.rs` 中 4 行只读 `contains(&self, id: u16) -> bool` 超出 comment-only 字面范围 — 该访问器是 design 决策 5 天空光未知方块 fail closed 的结构必需（`opaque()` 对未注册 ID fail-open,`index()` 为私有）,纯只读二分查找,不改 registry record layout/字段/ABI,两份独立评审同判“追认优于回退” — 若未来需要更严格的字面合规,可在后续 change 迁回 `light.rs`,本任务不回退。（2）Go oracle（`core.IsPlant`）与 Rust（material 集合）的植物判定形式差异记录在案 — 当前生产 registry 下两者 outcome-equal 且由真实 FFI 走廊绝对值断言把守,`internal/core/short_grass_test.go` 与 `internal/assets/short_grass_test.go` 的穷举测试已覆盖现有编号的 material⟺`IsPlant` 一致性;新增方块若引入 plant-layer material 而非 `IsPlant` 分叉,由 9.1 whole-change review 复核,不在本任务追加守卫。（3）`build_block` 每级重扫为有界零分配常数成本,记录备查,无需修复。

## Task 2.1 Rust Worldgen, MGW1 Layout, And Engine ABI

- Status: `DONE`（2026-09-03 控制会话勾选）。
- Task baseline SHA: `7115649a040e5907069eec5300fe4272443a90a2`（Task 1.1 `L`）。
- Implementer: fresh zcode implementer（独立于 Task 1.1）。
- RED evidence: Rust 基线 20 failed/214 passed,含 `exported_version_is_ten`（ABI=9）、`mgw1_layout_three_framing_is_frozen`（564≠566）、layout-3 解析 panic、`lod` 582 字节被 580 解析器拒绝、`worldgen_chunk_invalid_input_leaves_output_untouched`（legacy layout 2 被 v9 接受且改写输出——真 RED）;Go 侧 `TestABIValuesMatchEngineContract`（=9≠10）、layout-3 被拒/legacy 被收、短草缺席、566≠564 等。兼容性 pin（`TestShortGrassNormalizedOutputMatchesPreUpgradeDigests` 与两个 saved-chunk no-backfill 测试）在改动前按设计空过,其旧摘要在生成器改动前捕获。
- Code/test commit C: `1d21e8e49f100fd94bb63f04137a26b88da3f157` `feat(worldgen): generate deterministic short grass`（14 文件 +1221/−153,不含 `tasks.md`/`ledger.md`）。
- Files changed: `engine/crates/mornlea_engine/src/{worldgen.rs,lod.rs,ffi.rs}`、`engine/include/mornlea_engine.h`、`internal/worldgen/{generator.go,grass_test.go(新),fluid_test.go,testdata/golden_seed42.txt}`、`internal/nativeabi/{native.go,native_test.go}`、`internal/lod/lod_test.go`、`internal/storage/chunk/chunk_grass_backfill_test.go`(新)、`internal/server/short_grass_persistence_test.go`(新)、`AGENTS.md`（单 token `engine ABI v9`→`v10`,见 Ruling）。
- ABI/version evidence: engine ABI **v10**（C header 宏 + Rust `ABI_VERSION` + Go cgo 常量三端同步并由 `exported_version_is_ten`/`TestABIValuesMatchEngineContract` 钉住）、MGW1 layout **3**、15 材料（`short_grass` 追加于偏移 52）、header **566**、perm@**54**、chunk input **574**、probe input **570+16N**（count u32@[566,570)）、probe output **8N**、chunk output **196608**、LOD input **582**;protocol v32/client ABI v13/benchmark v20 等其他版本不动。
- Worldgen compatibility evidence: `SHORT_GRASS_GENERATION_SALT=0x5348_4f52_5447_5253` 与 design 逐字节一致;顺序 terrain/ores→trees→sea→恰好 256 列短草判定,`BaseBlockAt` 同序,`TerrainBlockAt`/`HeightAt`/LOD 忽略装饰,LOD golden `.bin` 未变;四份升级前摘要 `758c980a…/49a4124a…/52355bb9…/68532f6a…` 逐字保留,`TestShortGrassNormalizedOutputMatchesPreUpgradeDigests` 以新生成器+ShortGrassID→AirID 归一化后 SHA256 逐字节相等;固定 hit(-32,64,-32)/miss(-32,64,-31) 样本、跨区块/负坐标整块与单点 parity、树/水优先与生成顺序无关性均有测试;新 golden 中 chunk(1,0) 摘要不变（无合格命中列,真实）。
- Saved-chunk v9 evidence: `chunk_grass_backfill_test.go`（schema==9、逐格相等、无 ShortGrassID、codec+Region 往返）;`short_grass_persistence_test.go`（generator 0 次调用、加载逐格相等、重存不变）。
- Full-output compatibility evidence: chunk 精确 `196608`、probe `8N`、LOD 每 quad `20B` 两段式容量探测/`output_len` 语义保留;全部非法输入（含 legacy layout 2、572 字节 v9 wire frame、非豁免重复、short_grass 别名、错误 Y/长度/魔数）输出缓冲逐字节 untouched（canary 快照断言）。
- Final verified implementation SHA I: `1d21e8e49f100fd94bb63f04137a26b88da3f157`（无 repair 轮）。
- Focused verification bound to I: implementer 于 clean commit `1d21e8e4` 执行全绿:`make rust` ok;`cargo test -p mornlea_engine --locked` 234/0（~1.3s）;`go test ./internal/nativeabi ./internal/worldgen ./internal/lod -race -count=1` ok（~12s）;`go test ./internal/storage/chunk ./internal/server -race -count=1` 全命令无 -run 收窄 ok（~3:07）;`go test ./internal/archcheck -count=1` ok;`make test-race-changed RACE_BASE=7115649a` 40 包全 ok（~3:46）;`git diff --check` clean。控制会话另行核验 HEAD SHA、工作树干净与 commit 内容。
- SPEC reviewer / verdict / findings at I: fresh zcode SPEC reviewer = `SPEC PASS`,0 Critical/0 Important/1 Minor+1 Informational:（Minor）`ffi.rs:3033-3034` 注释把偏移 26 的 stone 槽写成 dirt,断言本身正确;（Info）短输出缓冲返回 `OUTPUT_OVERFLOW` 而非 `INPUT`——与 v9 基线逐字节相同,属既有主规格解读,非本候选新偏差。A–J 十项全部 SATISFIED/JUDGED CONFORMANT,附逐条 file:line 证据;三处 implementer 声明偏差均被判合规（AGENTS.md 单 token 有 `d7590eeb` 先例;fluid fixture 更新在 delta 明确允许范围内且断言 sea 优先;no-stacking 放宽是对过严 RED 的正确修正——邻列树冠叶层可合法位于 surface+2）。
- QUALITY reviewer / verdict / findings at I: fresh zcode QUALITY reviewer = `QUALITY PASS`,0 Critical/0 Important/2 Minor+2 Informational:（Minor1）`apply_short_grass` 与 `short_grass_block_at` 的世界顶部守卫不对称（latent only,冻结地形常数下不可达,`[−64,320)` parity 测试也无法触及 y=320,建议后续 change 对齐）;（Minor2）`audit_short_grass` else 分支 `assert_ne!` 恒真,死断言无危害;（Info1）AGENTS.md 单 token 由 `TestBaselineVersionsMatchCode` 机械强制,benchmark 保持 v20 正确;（Info2）固定样本 (-32,64,-32) 仅适用于 dry world（surface=63/target=64=海平面）,fluid-on 生产配置的 E2E 必须另选冻结样本——Task 5.1 派发时必须遵守。独立复核:RED 锋利度（legacy v9 wire frame 在旧引擎被接受且写输出）、常量双侧钉住、golden 纪律（旧摘要与 `git show 7115649a` 逐字一致）、`apply_short_grass` 256 次常数判定零分配零递归、`ore_hash` 负坐标 wrapping 安全、无共享可变状态/锁/RNG/IO、注释与提交卫生全部通过。
- Repair rounds and commits: 无（首轮双 PASS,R1–R5 未触发）。
- Tasks/ledger-only evidence commit L: 本 evidence commit（只含 `tasks.md` 2.1 checkbox 与本 ledger 证据;不回写自身 SHA,不冒充 `I`）。
- Ruling: Task 2.1 final `I` = `1d21e8e4`,focused verification 与独立双评审均 PASS,勾选成立 — 双评审零 Critical/Important — Minor/Info 处置:（1）追认 `AGENTS.md` 单 token `engine ABI v9→v10` 提前于 Task 8.1 — `TestBaselineVersionsMatchCode` 把该 token 与 header 宏绑定,是本任务 focused verification 的机械必需,有 `d7590eeb` 先例 — Task 8.1 仍独占其余现行文档段落,benchmark v20 等其余 token 未动。（2）`ffi.rs:3033` 注释措辞（stone 误写 dirt）与 `audit_short_grass` 恒真断言为无害瑕疵 — 不在本任务返工,记录在案;若 9.1 whole-change review 升级其严重度,回派本任务 implementer。（3）顶部世界守卫不对称为 latent-only（冻结地形常数下不可达）— 记录为 deferred,不在本 change 修复;若未来调整地形常数,须先对齐两路径守卫。（4）Task 5.1 派发约束:fluid-on 世界固定样本必须重新冻结,不得复用 (-32,64,-32)。

## Task 3.1 Fluid Replacement And Bounded Support Cleanup

- Status: `DONE`（2026-09-03 控制会话勾选）。
- Task baseline SHA: `fb01a2c9bf49bef482643a9349ffe6680890cf2f`（Task 2.1 `L`）。
- Implementer: fresh zcode implementer（独立于 1.1/2.1）。
- RED evidence: `TestReplaceable_ShortGrassReplaceableAtAllLevels`（7 子测试 false≠true）;golden「短草下方垂直优先/短草水平可替换」旧 dylib 把短草当实心（`ff 00 00…`≠`02 1c 00…`）;Rust `cargo test fluid` 13 个编译错误（`SHORT_GRASS`/`is_plant` 不存在）;`TestWildGrassSweepRunsBeforeTorchSweep/BedSweep`（短草未清）;door/bed 支撑拒绝枚举失败;realm 包构建失败（`SweepUnsupportedWildPlants` 未定义,4 处）。绿起点 pin:entity 火把放置已由 Task 1.1 physics `IsPlant` 零碰撞拒绝,补测试 pin 而无生产改动;differential/fuzz/rescan 夹具更新为双侧一致性网。
- Code/test commit C: `de7886012949d7a7ef243e492f87bffce12bba9f` `feat(sim): clear environmental short grass`（20 文件 +836/−52,不含 `tasks.md`/`ledger.md`）。
- Files changed: `internal/fluid/rules.go`（`Replaceable` 植物分支 `IsCrop`→`IsPlant`）+ 4 个测试;`engine/.../fluid_eval.rs`（冻结 `SHORT_GRASS=84`+`is_plant`,`replaceable` 消费）+ `fluid_rescan.rs`（测试钉住 rescan 复用 eval 谓词的 fixed point）;`internal/sim/realm/environment.go`（`SweepUnsupportedWildPlants`+`invalidateWildGrassAbove`;torch/支撑谓词 `IsCrop`→`IsPlant`;`settleFloodedCrop` 保持精确 `IsCrop(old) && IsFluid(new)`）+ 3 个测试;`internal/sim/runtime/engine_step.go`（wild grass→torch→bed 顺序）+ 3 个测试（ownership guard 注册,design affected-files 在案）;`internal/sim/entity/{door.go,torch.go}`（支撑拒绝;torch comment-only）+ 3 个测试。
- Boundedness/atomicity evidence: 稳定快照（清草循环前物化 `ChangedBlocks()` 副本,`TestSweepUnsupportedWildPlantsDoesNotRescanNewChanges` 证明上方第二株堆叠草存活=无递归）;每变化格只读正上一格,最终值 last-write-wins 重读世界态（`KeepsShortGrassOverGrassSupport`）;runtime 固定 wild grass→torch→bed,两个顺序敏感测试证明 torch/bed sweep 同 tick 观察到清草;工作量只随 changed set 线性增长,无 goroutine/IO/全世界扫描;短草容量满仍零掉落替换（`TestFluidWildGrassFloodWithFullDropSlotsStillReplaces` 全槽位逐位不变）,sweep 满槽零副作用,双源合并恰好一次结算零掉落,`assertNoSeedDrops` 钉住“环境不是种子来源”边界;作物容量语义不变。
- Final verified implementation SHA I: `de7886012949d7a7ef243e492f87bffce12bba9f`（无 repair 轮）。
- Focused verification bound to I: implementer 于 clean commit 全绿:`make rust` ok;`cargo test fluid` 49/0;`go test ./internal/fluid ./internal/sim/realm ./internal/sim/runtime ./internal/sim/entity -race -count=1` ok（10.1/7.7/21.0/7.6s）;`go test ./internal/server -run 'Fluid' -race -count=1` ok;`go test ./internal/archcheck -count=1` ok;`make test-race-changed RACE_BASE=fb01a2c9` 12 包 ok（3:33）;`git diff --check` clean。
- SPEC reviewer / verdict / findings at I: fresh zcode SPEC reviewer = `SPEC PASS`,0 Critical/0 Important/3 Info（rescan_differential_test.go 文件名不在 `*fluid*` glob 但属任务正文“更新差分”与 design D5 要求;ownership guard 测试不在 target-path 但 design affected-files 点名且为机械必需;torch comment-only+pin 为正确最小动作）。A–H 全部 SATISFIED,独立复跑定点抽查全绿。
- QUALITY reviewer / verdict / findings at I: fresh zcode QUALITY reviewer = `QUALITY PASS`,0 Critical/0 Important/2 Minor+2 Observation:（Minor1）design Verification 措辞的“采掘/翻地/流体后同 revision 清草”三源 E2E 未逐字实例化——采掘有 runtime E2E,机制测试 source-agnostic（直接 `mutation.Record`）,翻地/流体覆盖支撑无专属同 tick E2E,风险低因所有权威写者共用同一 record 路径;（Minor2）同 tick 双写一格（非草再回草）无专属测试——结构上不可能回归（`ChangedBlocks()` 只回位置,sweep 重读活世界态）;（Obs）支撑拒绝为代表性样本非穷举（穷举在 Task 1.1 core 测试）;`environment.go:1166` `!supportReady` 分支防御性不可达。下游核验:区块生成绕过 tick mutation（`ApplyGenerated`),自然短草不会被 sweep 清除（5.1 夹具不变量成立）;4.1 采草记录草格本身,sweep 只查上方空气,种子掉落与疲劳不受影响。
- Repair rounds and commits: 无（首轮双 PASS）。
- Tasks/ledger-only evidence commit L: 本 evidence commit（不回写自身 SHA,不冒充 `I`）。
- Ruling: Task 3.1 final `I` = `de7886012949d7a7ef243e492f87bffce12bba9f`,双评审零 Critical/Important,勾选成立 — 三处偏差（torch comment-only、rescan test-only 复用、ownership guard 注册）均在 design/spec 允许内,追认 — Minor 两项（三源 E2E 逐字实例化、同 tick 双写专属测试）记录在案不返工,机制由 source-agnostic 测试与结构不变量覆盖,若 9.1 升级严重度再回派。

## Task 4.1 Authoritative Mining, Stable Seed Drops, And Durability

- Status: `DONE`（2026-09-03 控制会话勾选）。
- Task baseline SHA: `4ad91fde834a4f9c34599f5c326b89c009e380f7`（Task 3.1 `L`）。
- Implementer: fresh zcode implementer（独立于 1.1–3.1）。
- RED evidence: `TestShortGrassMiningRuleOneTickAnyHeld`（`(0,false)`≠`(1,true)`）;hit/miss/容量满/零磨损/重放稳定等行为测试全部因采掘不完成而失败（`采除后方块=84，想要空气`）;`TestCompleteMiningShortGrassNoOpLeavesStateUntouched`（RejectProtectedBlock≠RejectNoTarget）;疲劳测试（0≠5）;双侧 AST guard（`companionMineableBlock`/`planMineableBlock` 未显式点名 `IsWildGrass`）;runtime parity（无区块变更）;roll API 编译 RED（`undefined: shortGrassSeedDropSalt/Roll`）。`TestCompanionMiningNeverSettlesShortGrass` 改动前偶然通过（缺失 `BlockDrop` 巧合拒绝）,作为行为回归保留,AST guard 是"显式拒绝"的承重 RED。
- Code/test commit C: `012676171de572d71f09271703c2bb5777af5956` `feat(gameplay): drop seeds from short grass`（8 文件 +927/−8,不含 `tasks.md`/`ledger.md`,Go-only 无 Rust/ABI）。
- Files changed: `internal/sim/entity/mining.go`（`miningRule` 短草分支任意手持 `(1,true)`+harvestable=资格;`completeMining` 专用分支位于 door/bed/furnace/chest 后、作物多产物与 `BlockDrop` 前,三条常数路径;`wildGrassDurabilityExempt` 第三类耐久豁免;`companionMineableBlock` 显式拒绝）、`internal/sim/entity/yield.go`（`shortGrassSeedDropSalt=0x4752_4153_5353_4544`+`shortGrassSeedDropRoll` 复用既有 splitmix64 链）、entity 三个测试文件、`internal/companion/plan_types.go`+新测试、`internal/sim/runtime/short_grass_drop_test.go`(新,真实 input→FinishWorld→commit 管线 parity)。
- Drop/atomicity evidence: salt 精确 `0x4752_4153_5353_4544` 且与 5 个 entity salt 及 Rust worldgen salt 互异（测试钉住）;hash 链只折 `uint64(worldSeed)^salt`→`uint32(dimension)`→`uint32(x/y/z)`（有符号经位模式）,`hash&7==0`,签名不含 tick/player/held/retry——重放稳定由 3 子测试+冻结判定表（含负坐标 `(−9,1,5)` hit/`(−1,1,5)` miss、dimension=1 翻转样本）钉住,4096 样本比率 0.1208≈1/8;命中+容量满:连续 3 次 `RejectDropCapacity` 全不变（chunk/drops hash、revision、tool、三层疲劳、inventoryDirty）,释放一格后同坐标结算一致（重试不可重掷）;未命中+容量满:仍清块、revision+1、drops hash 不变;零磨损:11 种手持（含耐久 1 工具/损坏形态）逐字段不变、耐久 1 不转损坏形态、无额外 dirty;同锄先草后泥土正常磨损 1;疲劳 hit/miss 各累积既有 5、拒绝路径零。
- Final verified implementation SHA I: `012676171de572d71f09271703c2bb5777af5956`（无 repair 轮）。
- Focused verification bound to I: implementer 于 clean commit 全绿:`go test ./internal/sim/entity ./internal/companion ./internal/sim/runtime -race -count=1` ok（19.8s）;`go test ./internal/server -run '(Mining|Drop|TransportParity)' -race -count=1` ok（8.8s）;`go test ./internal/archcheck -count=1` ok;`make test-race-changed RACE_BASE=4ad91fde` 21 包 ok（3:26）;`git diff --check` clean;gofmt/vet clean;无 Rust 改动故无需 `make rust`。
- SPEC reviewer / verdict / findings at I: fresh zcode SPEC reviewer = `SPEC PASS`,0 Critical/0 Important/2 非阻塞（`wildGrassDurabilityExempt` 单行包装属风格选择且与既有豁免命名平行;runtime parity 用平铺夹具属 4.1 正确范围,自然生成 E2E 归 5.1）。A–G 全部 SATISFIED,附 file:line 证据;独立抽查（ShortGrass/WildGrass/Durability|Exhaustion|Harvest|Wheat|CompanionMine）全绿。
- QUALITY reviewer / verdict / findings at I: fresh zcode QUALITY reviewer = `QUALITY PASS`,0 Critical/0 Important/1 Minor+2 Info:（Minor,下游）`shortGrassSeedDropRoll` 为包私有且无导出包装,5.1 夹具的"发送输入前断言命中"在 `internal/server` 无法不复制算法地实现 — 建议 5.1 导出访问器（有 `CompanionMineContainerStaging` 先例）或修订 design,记录在案;（Info）4096 样本块的 `hits==0||hits==4096` 为死条件、纯函数 replay 循环意义有限（装饰性）;（Info）AST guard 为真 `go/parser`+`ast.Inspect` 标识符遍历,失败即 `Fatalf`,行为断言闭环当前行为。reviewer 独立以独立程序重实现 design 链逐位复核:3 hit/3 miss 冻结位置、runtime 夹具 (0,1,1)hit/(0,1,4)miss、dim=1 翻转、比率 0.1208 全部一致;`PrepareDrop` 确认为纯预演（by-value 副本,文档钉住）。
- Repair rounds and commits: 无（首轮双 PASS）。
- Tasks/ledger-only evidence commit L: 本 evidence commit（不回写自身 SHA,不冒充 `I`）。
- Ruling: Task 4.1 final `I` = `01267617`,双评审零 Critical/Important,勾选成立 — Task 5.1 派发约束:（1）必须导出 `shortGrassSeedDropRoll` 访问器（entity 侧导出或经共享 helper,循 `CompanionMineContainerStaging` 先例）供 server E2E 夹具预断言命中,禁止复制算法;（2）fluid-on 生产配置世界必须重新冻结命中样本,不得复用 Task 2.1 的 dry-world 样本 `(-32,64,-32)`（其 surface=63/target=64=海平面,fluid-on 下被淹）。Minor 两项（死条件、包装风格）记录不返工。

## Task 5.1 Missing-player Inventory And Natural-seed Farming E2E

- Status: `DONE`（2026-09-03 控制会话勾选）。
- Task baseline SHA: `7f9c4a3fbaf841ed48d75461eeb0afc106d69848`（Task 4.1 `L`）。
- Implementer: fresh zcode implementer（独立于 1.1–4.1）;R1 修复由接手 agent 完成（原 implementer 会话因平台配额不可恢复,如实记录）。
- RED evidence: 编译 RED `undefined: runtime.ShortGrassSeedDropRoll`;persistence 5 个行为 RED（材料包/starter 持久化/confirm 一次性/no-starter-seeds 扫描/legacy fixture 校准）;server 行为 RED（`登录时统一索引 23 已持有小麦种子 {34,64}`=旧第 15 格赠送;hunger 登录 64 颗）;开发期夹具校准 RED（拾取在第 9 步出现→定位 release-input `send` 多耗一个未计数 tick,修复为排队不步进,已在代码内注释）。
- Code/test commit C: `4c290ed40da4fc223ed602244d80df4cc62f73a1` `feat(server): start farming from natural grass`（7 文件 +651/−377,不含 `tasks.md`/`ledger.md`,无 Rust 改动）。
- Files changed: `internal/server/persistence/players_snapshot.go`（仅删除 `starterSeedSlot` 常量与 64 种子写入,唯一生产改动）、`internal/sim/entity/yield.go`（导出 `ShortGrassSeedDropRoll` 纯转发,循 4.1 Ruling）、`internal/sim/runtime/entity_delegate.go`（archcheck 依赖方向要求的 runtime 委托,紧邻 `CompanionMineContainerStaging` 先例）、`internal/server/farming_loop_e2e_test.go`（全量重写:冻结常量+共享 runner+完整循环）、`internal/server/transport_parity_integration_test.go`（新增 `TestNaturalSeedFarmingMemoryTCPParity`）、`internal/server/hunger_loop_e2e_test.go`（登录契约改零种子,平铺夹具保留）、`internal/server/persistence/player_persistence_lifecycle_test.go`（零种子 36 格扫描+legacy 保留 pin）。
- Compatibility evidence: 前 14 格材料逐项不变（`Backpack[13]`=64 苔石圆石钉住）;第 15 格及之后与九格快捷栏为空;player schema v8/wire/storage 未动（无 codec/storage 文件入 diff,`cachedPlayerFromStored`/restore/save 不变）;legacy 玩家（第 15 格 64 种子+快捷栏 5 种子）逐槽保留含 Confirm 保存;确认前断开不持久化（force/autosave/flush/abort 子测试）;确认后重登精确恢复首次保存。
- E2E evidence: 生产 `worldgen.New(42, true)`（fluid-on 生产默认）经真实 Rust MGW1 生成;冻结常量 seed 42/Overworld/target `(32,65,224)`/farmland `(32,64,224)`,离线冻结两阶段转写（列扫描+整块/单点双出口核实+私有生产 roll 白盒探针,scratch 程序已删,无算法复制、无运行时搜索）;输入前三重断言（`BaseBlockAt(target)==ShortGrassID`、已加载 realm 同格、`ShortGrassSeedDropRoll` 命中）;零种子登录 36 格扫描→真实持续 primary 输入 1 tick（恰好一条 BlockChange+一条 ItemDropUpserts）→前 9 步种子 0、第 10 步 1（release-input 不步进防 off-by-one,与引擎 `advanceDrops`/`completeMining` 相位序核对无窗口）→自然海水润湿翻地+种植 `WheatStage0ID`、种子归零;Memory 与真实 TCP listener（`ListenTCP 127.0.0.1:0`）同 runner `reflect.DeepEqual` 全字段收敛（含拒绝、锄头耐久、最终背包）;完整循环经 225 tick 生长、[1,3] 收获、再种植;显式种子集成测试保留未动。
- Final verified implementation SHA I: `1351b6045759088145db783546ce600167eb32b3`（`C` + R1 `docs(server): refresh starter seed comment`）。
- Focused verification bound to I: implementer 于 `4c290ed4` 全绿:`make rust` ok;`go test ./internal/server/persistence -race -count=1` ok（7.2s）;`go test ./internal/server -run '(Farming|Hunger|TransportParity|PlayerPersistence)' -race -count=1` ok（4.1s）;`go test ./internal/archcheck -count=1` ok;`make test-race-changed RACE_BASE=7f9c4a3f` 10 包 ok（3:19）;`git diff --check` clean;gofmt/vet clean。R1 后 scoped:`server -run 'Eating' -race` ok、archcheck ok、diff-check clean;R1 仅注释改动,实现树其余与 `4c290ed4` 逐字节一致。
- SPEC reviewer / verdict / findings at I: fresh zcode SPEC reviewer = `SPEC PASS`,0 Critical/0 Important/2 Minor 观察+1 Info:（观察 1）锄头经 `SetPlayerInventoryForTest` 给予而非脚本内合成——design D6 不约束锄头来源,合成链由 `TestPlantSeedsMemoryTCPParity`/`TestMemoryTCPCraftingGridConvergence` 双传输覆盖,测试头已注明取舍;（观察 2）hunger 回归沿用夹具小麦而非字面自然种子——D6 只要求 Memory farming 循环自然化,hunger 从未跑种植循环且来源注释已链到自然种子循环;（Info）RED 为结构性。A–G 全部 SATISFIED,附 file:line 证据与抽查（3.7s/3.0s）。
- QUALITY reviewer / verdict / findings at I: fresh zcode QUALITY reviewer = `QUALITY PASS`,0 Critical/0 Important/1 Minor+2 Info:（Minor）`internal/server/eating_parity_test.go:34-35` 过期注释声称材料包给种子——已由 R1 修复;（Info）`PickupDelaySteps` 跨传输 DeepEqual 该字段恒真（语义由 runner 内逐 tick 循环承载,非缺陷）;（Info）legacy 保留 pin 为 green-on-old-and-new 设计,RED 承重由 36 格扫描/hunger 登录契约/两个 E2E 承担。独立核验:拾取计时与引擎相位序核对无 off-by-one 窗口;冻结样本预断言对 Rust worldgen/roll 漂移真正绑定;导出链纯转发无第二 salt;`farming_integration_test.go` 字节不变。
- Repair rounds and commits: R1 = `1351b6045759088145db783546ce600167eb32b3` `docs(server): refresh starter seed comment`:修复 `eating_parity_test.go` 过期注释为“材料包不含种子/小麦/面包;第一颗种子来自自然短草,种子→小麦链路由自然种子 E2E 覆盖”,comment-only +4/−2。原 implementer 会话因平台配额不可恢复,由接手 agent 按同一修复说明完成——控制会话如实记录此交接。R1 scoped re-review（fresh）:SPEC PASS + QUALITY PASS,注释事实经 `players_snapshot.go`/`farming_loop_e2e_test.go` 独立核实,卫生合规,`server -run 'Eating' -race` ok。R2–R5 未触发。
- Tasks/ledger-only evidence commit L: 本 evidence commit（不回写自身 SHA,不冒充 `I`）。
- Ruling: Task 5.1 final `I` = `1351b604`,初始双评审与 R1 scoped 双复评均 PASS,勾选成立 — R1 交接因原 implementer 会话配额中止,由接手 agent 承接同一 scoped 修复并独立复评,协议实质（fresh 修复+独立验证）未破,记录在案 — 两条 Minor 观察（锄头夹具给予、hunger 夹具小麦）均在 design D6 字面范围内且有独立双传输覆盖,不返工。

## Task 6.1 Benchmark V21 And Record-only Workload

- Status: `DONE`（2026-09-03 控制会话勾选）。
- Task baseline SHA: `986e4fc231eef5664f898986a44b96838f423e40`（Task 5.1 `L`）。
- Implementer: fresh zcode implementer（独立于 1.1–5.1）。
- RED evidence: producer 3 处（`scenarioVersion=20, want 21` 等）;comparator 8 个顶层失败（`场景迁移授权 "20:21" 无效：只允许 v19 到 v20 使用 19:20`,含 17 子测试 incomplete-v21 与跨传输场景门）;archcheck `TestBaselineVersionsMatchCode` 机制性绑定根 `AGENTS.md` benchmark token（单 token 同步为机械必需）。实现者中途自查纠正一次迁移记录断言反转（`len(records) != 0`）,测试即捕获并恢复原语义（迁移仍产绝对门记录、只跳过相对回归）。
- Code/test commit C: `081248930c3ef89a1373b270f136682a39ecb80f` `chore(benchmark): advance natural grass workload`（10 文件 +110/−95,不含 `tasks.md`/`ledger.md`/capture/golden/docs-notes）。
- Files changed: `cmd/mornlea/benchmark/benchmark.go`（仅 `scenarioVersion` 20→21+理由注释:registry 84→85、`[31..54]∪{68}`、MGW1 layout 3/ABI v10、确定性短草、每格 4 交叉 quad;分辨率/阶段/运动/样本/指标/阈值全不动）、`cmd/perfcheck/compare.go`（唯一显式迁移 `20:21`,`19:20` 退役）、`cmd/perfcheck/main.go`（flag 用法）、perfcheck 5 个测试（v21 helper/migration 矩阵含 `{21,21,"20:21"}` 未用授权拒绝、incomplete-v21、v6..v20 历史可读）、benchmark 2 个测试（V20→V21、更名 `...IncludesNaturalGrassWorkload`）、根 `AGENTS.md` 单 token（见 Ruling）。
- Record-only benchmark evidence: fresh `mktemp -d` producer 绑定 `I`:JSON SHA256 `3979d358f04550c32c6e1b2effcae94a747629fbf8ca1493bdfa9b35ab951fd1`,身份 scenario 21/memory/`git_commit=08124893`/Apple M2 16GiB/macOS 26.6.2/go1.26.0/2560x1440;样本 dropped=0 全组（still 10045/flying 43522/ticks 200/persistence 4938/player 256/interest 1600/remote_gpu 128）;producer 与 self-compare 均 `性能记录: flying p99 20.762 ms >= 12 ms` + exit 0（record-only 活体证明）;`19:20` exit 2、不存在路径 exit 2;accepted `docs/notes/perf-baseline*.json` 未动。SPEC reviewer 另行独立复现（fresh run SHA `0b91763a…`,flying p99 20.198 ms breach 仍 exit 0）。
- Hard-failure evidence: 报告完整性/身份/transport/commit 一致性（`compare.go:78-98`、`validate.go`）、真实 overflow、数据丢失、I/O 继续 exit 2;性能数值与高水位 record-only（`validate.go:74-82`）。
- Final verified implementation SHA I: `081248930c3ef89a1373b270f136682a39ecb80f`（无 repair 轮）。
- Focused verification bound to I: implementer 于 clean commit 全绿:`make rust` ok;`go test ./cmd/mornlea/benchmark ./cmd/perfcheck -race -count=1` ok（8.1s,70 测试）;producer+self-compare 6:08 如上;`go test ./internal/archcheck -count=1` ok;`make test-race-changed RACE_BASE=986e4fc2` 闭包 4 包 ok（41.7s）;`git diff --check` clean。
- SPEC reviewer / verdict / findings at I: fresh zcode SPEC reviewer = `SPEC PASS`,0 Critical/0 Important。A–E 全部 SATISFIED;独立复现 fresh producer/self-compare（live record-only 证明）;AGENTS.md 恰好单 token;旧测试名仅存于历史归档材料,无门禁引用。信息性:fresh-run 性能数值（still fps 167.6/p99 9.501ms;flying fps 369.9/p99 20.198ms;outbox hwm 1）为 record-only 观测,非门禁失败。
- QUALITY reviewer / verdict / findings at I: fresh zcode QUALITY reviewer = `QUALITY PASS`,0 Critical/0 Important/1 Minor+2 Info:（Minor）CLI 级 exit-2 路径（缺报告 I/O、非法授权、硬件不匹配）无提交的自动化测试——检测逻辑经真实 validator 单测覆盖,`fail()→os.Exit(2)` 布线 4 行未动,implementer 已用真实 CLI 演练 exit-2,可接受;（Info）incomplete-report 为封闭变异表,新可选字段需扩展 validator——v21 不加字段,本任务无缺口;（Info）根 `AGENTS.md` 单 token 与 2.1/8dcb3b5b 先例一致,8.1 派发时不得重复标记。迁移三重匹配（baseline==20∧current==21∧allow=="20:21"）在 CLI 与程序两路均拒绝其他组合,`{21,21,"20:21"}` 拒绝为真实;`completeV21ComparableReport` 继承 v20 链含 v12 batch 语义;v6..v20 历史循环非空洞。
- Repair rounds and commits: 无（首轮双 PASS）。
- Tasks/ledger-only evidence commit L: 本 evidence commit（不回写自身 SHA,不冒充 `I`）。
- Ruling: Task 6.1 final `I` = `08124893`,双评审零 Critical/Important,勾选成立 — 追认根 `AGENTS.md` benchmark scenario 单 token v20→v21 与 2.1 Ruling 同机制（archcheck 机械强制,先例 `1d21e8e4`/`8dcb3b5b`）,Task 8.1 仍独占其余文档段落（局部 `cmd/mornlea/benchmark/AGENTS.md` 保持 v20 待 8.1 更新,勿重复标记）— 性能数值为 record-only 观测记录在案。

## Task 7.1 Capture And 25-scene Visual Provenance

- Status: `DONE`（2026-09-03 控制会话勾选）。
- Task baseline SHA: `286f1c8f28ea244a614f3bc38e7793b9083f47ea`（Task 6.1 `L`）。
- Implementer: fresh zcode implementer（初始归因轮）;R1/R2 由接手 agent 完成（首个 R1 agent 被用户暂停且不可恢复,如实记录）。
- RED evidence: 新增 7 个 guard 测试（fixture 294 株自然短草全在 `GrassID` 上;oak-grove 差分可辨识像素测试;25 场景/顺序 guard;双阈值 `{2,0.0001}` pin;无窗口链路 guard;差分 fixture 奇偶性 guard）;真正 RED 是 golden 门禁本身——旧 golden 在 14 个场景失败（max delta 166-207,3.4%-11.9%,旧基线生成于 pre-change `17144636`）。
- Code/test commit C: `eb3e812c9fa30ff6b9e7cc3f32a9de19a2161865` `test(capture): attribute natural grass visuals`（16 文件:1 新测试+1 提炼共享场景表+14 张归因 golden）。
- 25-scene visual provenance: 更新前 25 张 tracked golden SHA 全记录（/tmp/pregolden_sha.txt）;RED 轮 25 张 current+14 对 actual/diff（/tmp/visual-red.aJigJD）;14 张变更逐图归因（terrain-noon/hud×3/avatar/inventory/workbench/chest/furnace/debug-panel/oak-grove/main-menu/settings-menu/far-horizon,全部归因自然短草/seed-42 worldgen,树/水优先、天空、HUD、面板干净）;11 张未变（全部无生产 worldgen 的合成夹具场景）逐字节不变;oak-grove 差分可辨识证明:rect (220,170)-(235,183),83 diff px,top-band 0/bottom-band 38（cutout 交叉斜面非实心块）。
- Visual verification evidence: 归因后 `make visual-update`（LOD on/off 近带控制通过）;初始轮 `make visual-check` 0/8 稳定失败——R1 根因双层:（a）渲染器既有非确定性（等深度重叠 cutout 的 atomic 追加顺序）,（b）权威 `PlayerState` 在 settle 帧覆盖场景钉住的世界时间;修复后 4 次连续全绿。
- Final verified implementation SHA I: `1d7485bbf8ba28246583f6b65b6eb09c3bacbc4d`（`C` + R1 `852d8702f3331bcf1e29b0a3cae5a2b242afa32a` + R2 `067b57089264c379e72205c8f2c16a511796b066` + engine fmt 回派 `1d7485bbf8ba28246583f6b65b6eb09c3bacbc4d`）。
- Repair rounds and commits: R1 = `852d8702` `fix(client): make distant plant culling deterministic`——cull.wgsl 三段确定性 compaction（cs_count→cs_scan→cs_place,无 atomicAdd,槽位=输入纯函数）+ 修复前 agent 自引入的两处缺失 `workgroupBarrier`（scan 入口,8 个屏障位点经复核完整）+ cull_tests.rs 4 个真测试（含 upload-order 竞态复现器,评审以还原屏障负向验证 3/3 失败）+ 世界时间冻结（`SetWorldTimeFrozen` 默认关,唯一生产调用点 `RunCapture` 带 defer 复位,只挡 `worldTimeTicks`/`dayPhaseOffset` 两个呈现量,ServerTick 与其余权威状态不受影响）+ 14 张 golden 重归因（每图 6-26 px,限于此前翻转 sliver 位置;11 张字节不变）;4 次连续 `make visual-check` 全绿（/tmp/r1-pass1..pass4,与提交 golden 逐字节一致）;两次预更新运行逐位相同（确定性证明）。R2 = `067b5708` `fix(client): satisfy rust fmt and clippy gates`（R1 引入的多余空行+未使用 `gz`/`gx`,两处一行修复）。engine fmt 回派 = `1d7485bb` `fix(engine): satisfy rustfmt gates`（Task 2.1/3.1 遗留的 `worldgen.rs`/`fluid_eval.rs` rustfmt 漂移,机械 reflow,`make rust-check` 由红转绿,语义零变化,engine 236/0）。
- Focused verification bound to I: R1 后 4×visual-check 全绿+cargo mornlea_client 101/0+capture race 9.0s+archcheck+race-changed（RACE_BASE=eb3e812c）5 包 75s+diff-check;R2/engine-fmt 后 `make rust-check` 全绿（fmt+clippy -D warnings+workspace 337 测试）、mornlea_engine 236/0、cull 7/7;SPEC reviewer 独立第 5 次复现 visual-check 25/25 场景 0.0000% exit 0。
- SPEC reviewer / verdict / findings at I: fresh zcode SPEC reviewer = `SPEC PASS`。A–F 全部 SATISFIED;偏差 E（app/capture 生产代码）裁决 **RATIFY**——冻结机制经失败日志（/tmp/r1-attrib3.log:无冻结时 torch-night 34.19%/bed-night 33.60% 等）证明承重,替代方案（放宽阈值/把夜晚 golden 重烤到墙钟漂移时间）均被 delta 禁止;capture-gated、生产安全、测试钉住。2 LOW（R1 的 fmt 空行与未使用变量,已由 R2 修复）+2 INFO（pass 轮 stdout 未留存,由 write-on-failure 语义+第 5 次独立复现补强;debug-panel 与 terrain-noon 逐字节相同的既有奇偶性——`DebugSegment` 恒空、面板已迁 WebView,建议 8.1 文档带一句）。
- QUALITY reviewer / verdict / findings at I: fresh zcode QUALITY reviewer 首轮 = `QUALITY FAIL`（窄判:仅 R1 引入的两处机械 CI 门禁破坏,实质工程维度全部通过——屏障纪律 8 位点复核无第三缺口、确定性纯函数、512KiB 一次性 buffer、3 pass 有界成本、4 测试真实（屏障还原负向验证）、冻结 defer-unwind、golden 重归因外科手术式、engine/ABI/benchmark 零改动）。R2+engine-fmt 修复后 scoped re-review（fresh）= `SPEC PASS` + `QUALITY PASS`,全部 Rust 门禁绿、worktree 干净。
- Tasks/ledger-only evidence commit L: 本 evidence commit（不回写自身 SHA,不冒充 `I`）。
- Ruling: Task 7.1 final `I` = `1d7485bb`,初始 SPEC PASS（含 RATIFY）+ QUALITY FAIL 两处机械修复后 scoped 双 PASS,勾选成立 — 追认 R1 超出原 target-path 的三块扩展:（1）`mornlea_client` cull 确定性修复——视觉像素门禁暴露的既有渲染器非确定性,不修则 `make visual-check` 不可复现,与本任务“完整无窗口链路”要求同源;（2）`cmd/mornlea/app`+`capture` 世界时间冻结——失败日志证明承重,唯一合规最小机制,默认关闭且 defer 复位;（3）engine fmt 回派修复 Task 2.1/3.1 遗留漂移 — 归因诚实（14 张两轮归因、11 张字节不变、每步 SHA 链可验）、阈值/场景/顺序零放宽 — debug-panel 奇偶性转 Task 8.1 文档注记。R3–R5 未触发。

## Task 8.1 Current Documentation And Version Matrix

- Status: `DONE`（2026-09-03 控制会话勾选）。
- Task baseline SHA: `ca6968e7410f9638352986795330f8092d2e57a6`（Task 7.1 `L`）。
- Implementer: fresh zcode implementer（独立于 1.1–7.1）。
- RED evidence: 无根 `AGENTS.md` 之外的自动 guard（archcheck `TestBaselineVersionsMatchCode` 只绑根 `AGENTS.md`,已由 2.1/6.1 单 token 同步保持绿）;RED 为编辑前逐文件清点的过期现行表述清单:config.yaml（v9/v12/v20）、README×2（v8/v12/v20、无种子入口）、benchmark 局部 `AGENTS.md`（"当前 v20"）、architecture §5/§6（v9/v12）、gameplay（无短草/种子唯一途径/火把光 Air-only/耐久唯一豁免/伙伴拒绝缺短草）、limitations（日期/方块光 Air-only/registry/起步种子/伙伴）、compatibility（v12/无 84 号方块/无降级说明/v20 与 19:20）、perf-baseline×2（v20/19:20）、progress/visual-verification/go-rust-division（缺当前条目）——共 13 处文件级清单 + 根 `AGENTS.md` 核实为已准确。
- Code/test commit C: `c64df1308175be943a7ee16d37400e548b2473df` `docs: document natural grass baselines`（14 文件 +51/−27,docs-only）。
- Current-version evidence: 版本矩阵全部从代码常量核实后写入:engine ABI **v10**（ffi.rs:44+header:47）、client ABI **v13**（mornlea_client ffi.rs:45+header:24;v13=窗口合成 capture,来自分支内更早 change,非本 change）、protocol v32、player v8、chunk v9、metadata v3、companions.ai **v4**、hostile v1、scenario **v21**、唯一迁移 `20:21`、MGW1 layout 3/15 材料/header 566。自然探索种子入口（1/8 确定性掉落、材料包无种子、第 10 活动tick拾取）、三类耐久豁免、植物光照口径、采掘表短草行、伙伴双拒绝、兼容性（旧档不回填/新块可含 84/降级需备份/禁混用）、7.1 视觉结果（14/11 拆分、确定性 culling、时间冻结、debug-panel 奇偶性注记）、go-rust-division MGW1 契约、capture/benchmark 局部指南同步。
- Scope audit: 恰好 14 文件全部在 8.1 独占清单内（根 `AGENTS.md` 经核实已准确零编辑,如实记录）;`docs/texture-packs.md`（1.1 独占）未动;无代码/测试/golden/accepted JSON;历史段落全部保留（progress 里程碑纯追加、perf-baseline v20 理由原文降级为"上一代"、compatibility v26-v31/v12 条目保留为历史）;`docs/notes/lan-server.md` 的陈旧版本按 Setup Ruling 留给 F-04,未动。
- Final verified implementation SHA I: `c64df1308175be943a7ee16d37400e548b2473df`（无 repair 轮）。
- Focused verification bound to I: `go test ./internal/archcheck -count=1` ok;`openspec validate natural-grass-seeds --strict --no-interactive` valid;`openspec validate --all --strict --no-interactive` **80 passed/0 failed**;`git diff --check` clean。双评审均独立复跑全绿。
- SPEC reviewer / verdict / findings at I: fresh zcode SPEC reviewer = `SPEC PASS`,0/0/0 需修复。A–F 全部 SATISFIED:版本矩阵 11 项 token 逐一对照代码常量独立复核全匹配;独占清单精确;历史纪律（progress 纯追加/perf-baseline 原文保留/compatibility 旧条目）;新段落内容逐句对码核验（salt/1/8、三类豁免、光照两口径、伙伴双侧、84/85、14 张名单逐一比对 `286f1c8f..1d7485bb` 实际 golden 变更集、冻结 defer、`&3==0` 密度）;根 `AGENTS.md` 不编辑的决策诚实正确。
- QUALITY reviewer / verdict / findings at I: fresh zcode QUALITY reviewer = `QUALITY PASS`,0/0/4 Info:（1）README.en 种子句较中文略简（CN 历史上更密,非矛盾）;（2）limitations 重验戳 2026-09-02 vs 提交日 09-03（无跨文件矛盾）;（3）`lan-server.md` 陈旧版本为 F-04 独占,正确未动,提请 F-04/控制会话后续处理;（4）任务编号 regex 仅命中 SHA-256 算法名,无任务 ID。内部一致性 grep 交叉核验零分歧;三处 implementer 偏差（根 `AGENTS.md` verified-not-edited、perf-baseline 不新增 v21 测量节、m5 文件内现行迁移句必要翻转）均判合理。
- Repair rounds and commits: 无（首轮双 PASS）。
- Tasks/ledger-only evidence commit L: 本 evidence commit（不回写自身 SHA,不冒充 `I`）。
- Ruling: Task 8.1 final `I` = `c64df130`,双评审零需修复项,勾选成立 — client ABI **v13** 与 companions **v4** 为本分支代码实况（分支早于 main 后续 v14/v5 bump）,文档如实记录,PR 并 main 时需按 main 最终基线重新核对版本矩阵 — `lan-server.md` 陈旧性转告 F-04 — 4 条 Info 记录在案不返工。

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
