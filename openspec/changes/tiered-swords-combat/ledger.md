# Ledger

## Baseline

- 隔离 worktree：`/Users/chen/work/mornlea/.worktrees/A-03-tiered-swords-combat`，分支 `feat/A-03-tiered-swords-combat`。
- Task 1 起始 HEAD：`6a992a5c5081d707423b2361ea5dc985375f19c8`；起始 `git status --short --branch` 退出 0，仅输出分支名，无 tracked/untracked 改动。
- 冻结检查对应的代码基线：`67fcc604bd3f3b5ce9326b5cd7498381163296d6`。Task 起始 HEAD 与该 SHA 的差异只承接控制器已完成的 admission/claim 规划事实；本 Task 不重复 admission、认领或 frozen tests。
- 基线版本矩阵：协议 v31、player schema v8、chunk schema v9、world metadata v3、`companions.ai` schema v4、`hostile_mobs` schema v1、engine ABI v8、client ABI v10、benchmark scenario v19。
- 基线 append-only 与固定容量：`ItemBed=46`、`ItemIDMax=47`、`RecipeBed=16`、Play S→C registry 尾号 24、正式 capture 23 项、HUD 最大关闭/打开/容量 96/257/267。
- 已继承证据：`CARGO_TARGET_DIR="$PWD/engine/target" make rust`，exit 0，隔离 Rust release build 通过；tested SHA `67fcc604bd3f3b5ce9326b5cd7498381163296d6`。
- 已继承证据：`go test ./internal/archcheck -run 'TestBaselineVersionsMatchCode|TestClientCommandSubpackageDependencyDirections' -count=1`，exit 0，版本矩阵与 client command 分包依赖方向通过；tested SHA `67fcc604bd3f3b5ce9326b5cd7498381163296d6`。
- 已继承证据：`go test ./internal/core ./internal/network ./internal/render/hud ./cmd/mornlea/capture -run 'Test(ItemIDsAppendOnly|RegisteredRecipeCellsStayInsideShapeBounds|ProtocolVersionPinned|HostileMessageIDsAreFrozen|HotbarLayoutStaysWithinFixedCapacity|CaptureSceneOrderAndAICompanionDeterminism)$' -count=1`，exit 0，冻结 protocol v31、item 46/47、recipe 16、S→C tail 24、23 scenes 与 HUD 96/257/267；tested SHA `67fcc604bd3f3b5ce9326b5cd7498381163296d6`。

## Rulings

- Ruling: 继承控制器已完成的 Task 1 Steps 1–4，不重复 promotion、claim、Discussion、worktree、Rust build 或 frozen tests — 这些前置已给出可复用的 SHA/exit 证据，重复执行既无新增事实又违反任务边界 — 原问题是 brief 的完整流程仍列出 admission 与基线命令，但当前 implementer 只负责 Steps 5–7 的 artifacts。
- Ruling: 不编辑 `docs/feature-backlog.md` — admission 与认领规划事实已经由控制器单独完成，本提交只包含 active change 文件 — 原问题是原始 Task 1 文件清单包含 backlog，但控制器明确裁定该 prerequisite 已完成且不得重复。
- Ruling: 同时记录 Task 起始 HEAD `6a992a5c5081d707423b2361ea5dc985375f19c8` 与 frozen code SHA `67fcc604bd3f3b5ce9326b5cd7498381163296d6` — 前者标识本次 artifact 工作的真实起点，后者标识可复用代码/测试证据的真实内容基线 — 原问题是单写任一 SHA 都会混淆 docs-only claim 提交与代码冻结证据。
- Ruling: `openspec/config.yaml` 保持不变并忽略其中过时的 v27/v7/v2/ABI prose — 仓库真相优先级要求使用代码、测试、根 `AGENTS.md` 和批准设计，Task 6 负责最终同步配置 — 原问题是 CLI instructions 会注入 stale context，若照抄会把 active change 降回错误版本矩阵。
- Ruling: 不读取、修改或消费 dirty sibling worktrees — 本任务只在已指定 isolation worktree 内工作，兄弟工作区与本 change 无关 — 原问题是并行分支可能包含未合入状态，读取它们会污染批准基线与文件所有权。
- Ruling: 不执行 GitHub Discussion 远程 bookkeeping — Discussion 暂时不可达且 admission/remote 同步由控制器持有，本 Task 只记录该边界 — 原问题是原计划要求 GraphQL 评论与正文 refresh，但当前网络状态和职责分工均禁止 implementer 重试。
- Ruling: 不派发 subagent 或独立 reviewer，本 Task 只执行 implementer self-review — 控制器对本次 fresh implementer 明确下达 no-dispatch/no-reviewer 指令，高于计划中的一般双评审流程 — 原问题是 brief 的 Step 7 通常要求 fresh SPEC/QUALITY 评审，但本次任务要求由控制器在 implementer 之外处理任何后续审阅。
- Ruling: `.openspec.yaml` 使用 CLI 生成的 `spec-driven` schema，并创建 proposal、11 份 delta、design、仅含 Tasks 2–5 的 tasks 与 ledger — 这是 `applyRequires=tasks` 的完整 transitive closure，同时排除 Task 1 admission 和 Task 6 integration runbook — 原问题是只创建 `tasks.md` 会让 file-existence status 虚假完成，也会把 archive/PR 工作错误暴露为 active implementation checkbox。
- Ruling: 协议唯一升版为 v32，S→C 追加 `CombatHit=25`，其余 player/chunk/world/companions/hostile/engine/client/benchmark 保持 8/9/3/4/1/8/10/19 — 批准设计已完成兼容选择且没有待定值 — 原问题是 `openspec/config.yaml` 的旧版本 prose 与当前代码事实冲突。
- Ruling: 统一战斗固定 72 actor/72 intent、hostile-first reservation、player 10 tick、hostile 20 tick、0.35 击退和整阶段 overflow fail closed — 这些是批准的玩法与安全边界，不能在实现期配置化或静默改变 — 原问题是保留双循环、截断 overflow 或统一 cooldown 都会破坏跨类型确定性和 A-04 兼容。
- Ruling: HUD 以代码事实 96/257/267 为基线，marker 后为 100/261，不沿用主规格中过时的 266 — 容量测试已在 frozen SHA 证明真实 opening maximum 为 257，新增 4 quad 无需扩容 — 原问题是旧 `container-ui-presentation` 数字会诱导扩大固定上传布局并造成 client ABI/benchmark 漂移。
- Ruling: 正式 capture 从 23 增为 24，固定相邻顺序为 `ai-companion`、`sword-combat`、`hostile-mob`、`water-surface-slope` — 该顺序同时保持 `far-horizon` 倒数第二与 `water-underwater` 唯一末项 — 原问题是主规格残留 19/21/22 项历史口径且 hostile 原先直接紧随 companion。

## Task Reviews

- Task 1 implementer self-review round 1，SPEC PASS：proposal、11 份 delta、design 与 tasks 均逐项覆盖 brief/design 的 ItemID 47..53、recipe 17..19、2/4/5/6 damage、59/131/250 durability、72/72 failure boundary、mixed target 全序、10/20 tick、hostile-first、互击、0.35 击退、100 milli fatigue、private v32 `CombatHit=25`、PCM 参数/hash、4 quad/6 成功帧、100/261≤267 和 24-scene order；没有新增玩法值或开放问题。
- Task 1 implementer self-review round 1，QUALITY PASS：artifact 依赖按 `openspec status --json` 与 `openspec instructions ... --json` 的顺序创建；modified requirement 使用现有 requirement 精确标题；tasks 只有计划 Tasks 2–5，未包含 admission、sync/archive、PR、CI 或 cleanup；没有生产/测试代码或长期配置改动。
- Task 1 implementer self-review round 2，SPEC PASS：逐文件复核完整 16-file active change diff，并与 task brief、批准设计和实施计划 Tasks 2–5 对照；11 个 capability、固定值、failure/capacity boundary、兼容矩阵与 publication/client/capture 边界均完整，无遗漏、冲突或待定值。
- Task 1 implementer self-review round 2，QUALITY PASS：13 个 `MODIFIED` requirement 标题均与当前主规格精确匹配；所有 requirement 都有可判定 scenario；tasks 保持 2.1..2.9、3.1..3.11、4.1..4.8、5.1..5.13 且不包含 Task 1/6；全文未发现需要修复的 finding。
- Task 1 独立评审，SPEC PASS：评审报告 `.superpowers/sdd/2026-08-28-tiered-swords-unified-combat/task-1-review.md:1-11` 对提交区间 `6a992a5c..80c4affa` 的 16-file active change 完成核对；11 个 capability、冻结增量/不变矩阵、玩法数值、72/72 fail-closed、hostile-first、私有 `CombatHit`、HUD/capture 边界与 Tasks 2–5 镜像均符合批准设计，无法从 diff 验证的项目为 0。
- Task 1 独立评审，QUALITY PASS：同一报告 `:13-42` 给出 Task quality `Approved`，Critical 0、Important 0、Minor 0；没有 correctness、scope、maintainability 或 verification finding。
- Task 1 完成：起始提交 `6a992a5c` 到 proposal 提交 `80c4affa` 的 docs-only 区间已完成 implementer self-review、独立 SPEC/QUALITY 评审和既有 validation evidence，后续工作从 `tasks.md` 的 Task 2 开始。

## Task 2 Implementation

- 起始 HEAD：`259b702ff818f57b514c48cda935da27580569b5`；起始 `git status --short --branch` 退出 0，仅输出分支名，无 tracked/untracked 改动。
- RED（combat kind 与 item registry）：`go test ./internal/core -run 'Test(CombatTargetKind|Sword|ItemID)' -count=1`，exit 1；按预期因 `CombatTargetKind`、两个 kind、六个 sword ItemID 与 sword helpers 尚未定义而编译失败，证明新增测试能捕获缺失登记。
- GREEN（combat kind 与 item registry）：`gofmt -w internal/core/combat.go internal/core/combat_test.go internal/core/item.go internal/core/item_test.go`，exit 0；`go test ./internal/core -run 'Test(CombatTargetKind|Sword|ItemID|Durability|Broken)' -count=1`，exit 0，append-only kind/item、stack limit、耐久、损坏映射与伤害通过。
- RED（sword recipes）：`go test ./internal/core -run 'Test.*SwordRecipe' -count=1`，exit 1；按预期因 `RecipeWoodenSword`、`RecipeStoneSword`、`RecipeIronSword` 尚未定义而编译失败，证明三档固定 pattern 与 matcher 行为测试能捕获缺失登记。
- GREEN（sword recipes）：`gofmt -w internal/core/recipe.go internal/core/recipe_test.go internal/core/recipe_shape_internal_test.go`，exit 0；`go test ./internal/core -run 'Test.*SwordRecipe' -count=1`，exit 0，recipe 17..19、三列平移、满耐久产物与拒绝边界通过。
- 当前 schema 兼容特征测试：`go test ./internal/storage -run 'Test.*SwordItems' -count=1`，exit 0；按 brief 预期在 core 登记后直接通过，现有通用 player/chest/companion codec 无需生产改动。`TestChunkCodecRoundTripsSwordItemDrops` 的精确名称不落入 brief 给定的复数过滤器，将由最终完整 storage package gate 覆盖；四条测试都同时加入 `core.ItemIDMax` 对应 invalid case，未修改 schema 或 shared fixture。
- RED（intact sword mining durability）：`go test ./internal/sim -run 'TestMiningIntactSwordsDoNotConsumeDurability' -count=1`，exit 1；三档部分磨损剑均在成功移除泥土后错误减少 1 点耐久（29→28、65→64、125→124），与预期根因“通用 durability wrapper 磨损全部有耐久物品”一致。
- GREEN（durability seam）：`gofmt -w internal/sim/mining.go internal/sim/mining_test.go`，exit 0；`go test ./internal/sim -run 'TestMiningIntactSwordsDoNotConsumeDurability' -count=1`，exit 0；采掘 wrapper 排除完好剑，既有路径复用按 slot/item/count 重验的 `consumeToolDurabilityAt`。
- Focused gofmt：`gofmt -w internal/core/combat.go internal/core/combat_test.go internal/core/item.go internal/core/item_test.go internal/core/bed_test.go internal/core/recipe.go internal/core/recipe_test.go internal/core/recipe_shape_internal_test.go internal/storage/player_codec_test.go internal/storage/chunk_chest_test.go internal/storage/chunk_drop_test.go internal/storage/companion_codec_test.go internal/sim/mining.go internal/sim/mining_test.go`，exit 0，无输出。
- Focused race：`go test ./internal/core ./internal/storage ./internal/sim -race -count=1`，exit 0；core、storage、sim 分别通过（2.649s、22.544s、53.647s），包含精确名称未落入早期 storage 过滤器的 sword drop round-trip，并保持既有镐、锄、作物×锄头和伙伴采掘测试全绿。
- 架构门禁：`go test ./internal/archcheck -count=1`，exit 0（9.660s）。
- OpenSpec 门禁：`openspec validate --all --strict --no-interactive`，exit 0，77 passed、0 failed。
- Diff 门禁：`git diff --check`，exit 0，无输出。
- Checklist：2.1..2.8 已有实际 RED/GREEN 或 brief 指定的直接兼容通过证据；2.9 按控制器指令保持未勾选，等待 implementer 外部的独立 SPEC/QUALITY review。

## Task 2 Self-Review

- SPEC PASS：逐项核对 ItemID 47..52、`ItemIDMax=53`、`ItemBed=46`、combat kind 1/2、damage 2/4/5/6、durability 59/131/250、broken mapping、recipe 17..19、三列平移/拒绝边界、四条 current-schema round-trip 与 sword mining exemption；没有遗漏或额外行为。
- QUALITY PASS：完整 diff 只含 Task 2 的 14 个 Go 文件和本 change 的 tasks/ledger；没有 schema、shared fixture、网络、server、render、Rust、长期文档或依赖变化，也没有 registry struct、配置、显示名表、公共 `DamageTool` 或 sword-specific wire 分支。
- Mutation check：kind 值/合法域、任一 sword ID/limit/durability/broken/damage switch、recipe ID/pattern/material/output/matcher tail、任一 codec 的 item/count/durability round-trip 或 `ItemIDMax` 拒绝、以及 mining sword guard 的删除/错分支均至少会使一条新增测试失败；既有全包测试覆盖 helper 委托后的镐耗损、最后一点损坏、锄头与作物豁免和伙伴路径。
- Findings：Critical 0、Important 0、Minor 0；无需修复轮次。
- Task 2 独立评审，SPEC PASS：报告 `.superpowers/sdd/2026-08-28-tiered-swords-unified-combat/task-2-review.md:1-18` 对提交区间 `259b702f..29eb6ee2` 完成核对；combat kind、ItemID/耐久/损坏映射/伤害、recipe 17..19、四条 current-schema round-trip、采掘 durability seam、禁止项与 14 个 Go 文件加 tasks/ledger 的范围均符合 Task 2 brief，没有缺失、额外或误解行为。
- Task 2 独立评审，Cannot-verify 全部解决：RED/GREEN 时序和 post-commit 命令结果由 `task-2-report.md:38-46,60-69` 的 implementer evidence 解决；player v8、chunk v9、companions v4 的未改数值由无生产 codec/schema/fixture diff、四条 current generic codec 测试与已记录 archcheck/OpenSpec 门禁解决；既有镐、锄、作物×锄头和伙伴行为由完整 `internal/sim` race package 结果及 `task-2-report.md:57,65` 解决；没有剩余无法验证项。
- Task 2 独立评审，QUALITY PASS：同一报告 `:20-45` 给出 Task quality `Approved`；实现保持 fixed switch、真实 package 行为测试和既有包边界，Critical 0、Important 0、Minor 0，没有 correctness、scope、maintainability、verification、删除或简化 finding。
- Task 2 完成：`259b702f..29eb6ee2` 已具备 implementer RED/GREEN、focused gates、self-review 与独立 SPEC/QUALITY PASS；`tasks.md` 2.1..2.9 全部完成，后续实现从 Task 3 开始。

## Task 3 Implementation

- 起始 HEAD：`fe6b15fb1c2ddfc9cf9ffcda756188ed1c301b04`；起始 `git status --short --branch` 退出 0，仅输出分支名，无 tracked/untracked 改动。
- RED（固定容量与来源冷却）：`go test ./internal/sim -run 'TestCombat(Snapshot|Intent|PlayerCooldown|HostileCooldown|Cooldowns)' -count=1`，exit 1；按预期因 `CombatHit`/`TickResult.CombatHits`、拆分后的 player attack/hurt cooldown、`advanceCombat` 与 `advanceCombatWithLimits` 尚不存在而编译失败，证明测试能捕获当前 8-intent player loop、共享 victim cooldown 与平行 hostile loop 尚未统一的根因。
- GREEN（固定容量与来源冷却）：`gofmt -w internal/sim/combat.go internal/sim/player.go internal/sim/hostile_melee.go internal/sim/command.go internal/sim/combat_capacity_test.go internal/sim/combat_exhaustion_test.go internal/sim/hostile_combat_test.go internal/sim/mining_test.go`，exit 0；`go test ./internal/sim -run 'TestCombat(Snapshot|Intent|PlayerCooldown|HostileCooldown|Cooldowns)' -count=1`，exit 0（0.667s），72/72、两个降低 limit 的下一次 append fail-closed、四类 cooldown 原子递减及 player 1/11、hostile 20-tick 边界通过。
- RED（mixed target 与冻结语义）：`go test ./internal/sim -run 'TestPlayerCombat(Target|Protected|Frozen|Ray)' -count=1`，exit 1；距离更近 hostile、hostile 同 kind 最小 ID、受保护最近 hostile 不穿透和冻结 hostile 目标四处按预期失败，根因是当前 player producer 仍只枚举 player snapshot；非零 pitch、固体严格在前、表面等距与流体透明断言已直接通过。
- GREEN（mixed target 与冻结语义）：`gofmt -w internal/sim/combat.go internal/sim/hostile.go`，exit 0；`go test ./internal/sim -run 'TestPlayerCombat(Target|Protected|Frozen|Ray)' -count=1`，exit 0（0.573s），player/hostile mixed target 按距离、kind、stable ID 全序选择，保护目标不穿透，射线边界与冻结 hostile 目标通过。
- RED（全局 victim reservation）：`go test ./internal/sim -run 'TestCombat(Reservation|Mutual|Loser)' -count=1`，exit 1；同 victim 的 hostile/player 与两个 hostile 都被重复结算，目标 health 分别错误降至 15/14，player reservation loser 还错误写入 attack cooldown 10；不同 victim 的 player↔player 与 player↔hostile mutual 用例已通过，失败与缺少全局 reservation 的预期根因一致。
- GREEN（全局 victim reservation）：`gofmt -w internal/sim/combat.go internal/sim/combat_resolution_test.go`，exit 0；`go test ./internal/sim -run 'TestCombat(Reservation|Mutual|Loser)' -count=1`，exit 0（1.321s），固定数组 hostile-first reservation、同 kind 最小 stable ID、loser 零副作用与两类 mutual 用例通过。
- RED（武器、副作用与击退）：`go test ./internal/sim -run 'Test(PlayerCombatWeapon|CombatReservationLoser|Combat.*Mining|Combat.*Exhaustion|CombatKnockback)' -count=1`，exit 1；按预期因原提交面尚无 `settleCombatIntent` 而编译失败，证明 frozen slot fail-closed、player/hostile 武器伤害、耐久、fatigue、采掘抑制、领域 hit 与两类 knockback 尚未进入统一原子 settlement。
- GREEN（武器、副作用与击退）：`gofmt -w internal/sim/combat.go internal/sim/combat_weapon_test.go internal/sim/combat_knockback_test.go internal/sim/combat_exhaustion_test.go internal/sim/mining_test.go internal/sim/hunger.go`，exit 0；`go test ./internal/sim -run 'Test(PlayerCombatWeapon|CombatReservationLoser|Combat.*Mining|Combat.*Exhaustion|CombatKnockback)' -count=1`，exit 0（1.012s），空手/普通/broken 2、wood 4、stone 5、iron 6、最后一点转损坏、player→hostile、六类拒绝路径零副作用、XZ 0.35 additive 与 yaw fallback 通过。
- 生命周期重排：`engine_step.go` 按 spawn/action/movement → unified combat → burn → hostile deaths/drop → distant despawn → player deaths 固定；`hostile_manager.go` 删除 `AttackCooldown==0` 前置过滤，范围内每 tick enqueue，sim 递减后唯一准入；`AttackCooldown=1` 同 tick 命中；注释同步为 `advanceCombat`。新增 `health=1` burn 到期与 `DistantTicks=599` 同 tick 证明先掉腐肉再移除。
- 领域出口：`go test ./internal/sim -run 'TestCombatHits' -count=1`，exit 0（0.637s），`CombatHits` 按 `SessionID` 升序、每 session 每 tick 至多一条且 hostile 攻击不产生 hit；`go test ./internal/server -run 'TestHostileChase' -count=1`，exit 0（3.532s with -race），hostile manager 在 sim 20 tick 节奏下同 tick 生效。
- Focused gofmt：`gofmt -w internal/sim/combat.go internal/sim/player.go internal/sim/hostile.go internal/sim/hostile_action.go internal/sim/command.go internal/sim/engine_step.go internal/sim/hunger.go internal/sim/combat_test.go internal/sim/combat_capacity_test.go internal/sim/combat_resolution_test.go internal/sim/combat_weapon_test.go internal/sim/combat_knockback_test.go internal/sim/combat_exhaustion_test.go internal/sim/hostile_combat_test.go internal/sim/hostile_lifecycle_test.go internal/sim/mining_test.go internal/server/hostile_manager.go internal/server/hostile_manager_test.go`，exit 0。
- Focused sim race：`go test ./internal/sim -race -count=1`，exit 0（64.484s）；server 相关 `TestHostileChase`/`TestCombatHits` with race 通过，完整 server 套件含两处已在基线即 flaky 的 parity 超时（fluid crop 与 companion 交互对齐预算 256），与本 Task 变更无关且在基线重试即复现。
- 架构门禁：`go test ./internal/archcheck -count=1`，exit 0（8.144s）。
- OpenSpec 门禁：`openspec validate --all --strict --no-interactive`，exit 0，77 passed、0 failed。
- 旧符号清零：`rg -n 'advancePlayerMelee|advanceHostileMelee|damageHostileTarget' internal/sim --glob '*.go' --glob '!*_test.go'`，exit 1，无命中，生产代码已无平行结算函数。
- Diff 门禁：`git diff --check`，exit 0。
- Checklist：3.1..3.10 已有实际 RED/GREEN 或直接通过证据；3.11 按控制器指令保持未勾选，等待 implementer 外部的独立 SPEC/QUALITY review。

## Task 3 Self-Review

- SPEC PASS：逐项核对 72/72 固定容量与整阶段 overflow fail-closed、四类 cooldown 原子递减、player 10/hostile 20、(distance,kind,ID) 全序、固体严格在前、流体穿透、受保护目标不穿透、冻结后 live 改变不改写、hostile-first reservation、最小 stable ID、loser 零副作用、两类 mutual 同 tick 结算、武器 2/4/5/6 与最后一点先 damage 后损坏、六类拒绝零副作用、XZ 0.35 additive 与 yaw fallback、burn/distant 死亡与掉落环形、CombatHits 有序且 hostile 不产生 hit。
- QUALITY PASS：完整 diff 只含 Task 3 的 20 个受控文件（4 个新建 combat tests、14 个修改 Go 文件、删除 1 个 hostile_melee.go、tasks/ledger），无协议、存储、render、Rust、依赖或长期文档变化；无 map/sort 分配、新接口/配置/registry、额外 goroutine 或 speculative helper；注释中文且无任务编号。
- Mutation check：任一固定容量、cooldown 值、ray 选择、阻挡、kind 优先级、reservation 顺序、伤害/击退/疲劳/耐久/hit 分支的删除或错值均至少使一条新增测试失败；既有 sim 全包保留镐、锄、作物与伙伴行为，hostile lifecycle 的 600/20 与掉落路径全绿。
- Findings：Critical 0、Important 0、Minor 0；无需修复轮次。

## Validation Evidence

- 基线验证证据见 `Baseline`，均对应 frozen code SHA `67fcc604bd3f3b5ce9326b5cd7498381163296d6`，本 Task 未重复运行。
- `openspec status --change tiered-swords-combat`，exit 0，`4/4 artifacts complete`；验证时 HEAD `6a992a5c5081d707423b2361ea5dc985375f19c8`，工作区仅含本 change 的 16 个 intent-to-add 文件。
- `openspec validate --all --strict --no-interactive`，exit 0，77 passed、0 failed；验证时 HEAD 与工作区状态同上。
- `git diff --check`，exit 0，无输出；验证时 HEAD 与工作区状态同上。写入本证据后，提交前 MUST 对最终 active change bytes 重新运行同一组三项命令。

## Deferred And Abandoned

- 无未决玩法、协议、容量、兼容或视觉数值。
- OpenSpec sync/archive、长期版本文档、`openspec/config.yaml`、backlog/Discussion、PR/CI 与 worktree cleanup 明确属于计划 Task 6，不是本 active change checklist 的延期项。
- dirty sibling worktrees 与 `docs/notes/lan-server.md` 永久排除在本 change 文件范围之外。
