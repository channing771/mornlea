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
- Task 3 独立评审，SPEC PASS：报告 `.superpowers/sdd/2026-08-28-tiered-swords-unified-combat/task-3-review.md:7-81` 对提交区间 `fe6b15fb..c82a9a5e` 完成 15 项契约核对；72/72 fail-closed、四类冷却、射线全序与冻结、hostile-first reservation、loser 零副作用、mutual、武器与耐久、六类拒绝、0.35 击退、生命周期与 CombatHits 均符合 Task 3 brief；Minor 1 为清单性偏差（brief 列 `mining.go` 未改）。
- Task 3 独立评审，QUALITY PASS：同一报告 `:84-109` 给出 Task quality `Approved`；范围收敛、固定数组、私有值、无接口/配置/协议/存档/Rust 扩张，Critical 0、Important 0、Minor 1 且不影响玩法/安全契约。
- Task 3 完成：`fe6b15fb..c82a9a5e` 已具备 implementer RED/GREEN、focused gates、self-review 与独立 SPEC/QUALITY PASS；`tasks.md` 3.1..3.11 全部完成，后续实现从 Task 4 开始。

## Task 4 Implementation

- 起始 HEAD：`2d6d88df1f968c90f8b669d2c2cd3ef81a51623c`；起始 `git status --short --branch` 退出 0，仅输出分支名，无 tracked/untracked 改动。
- RED（wire shape、值域和 registry）：`go test ./internal/network -run 'TestCombatHit' -count=1`，exit 1；按预期因 `network.CombatHit` 不存在而编译失败（`undefined: CombatHit`），证明新增 wire 测试能捕获缺失登记。
- GREEN（wire shape、值域和 registry）：实现固定 10-byte `CombatHit` 与 `Validate`（tick>0、damage 1..20、kind.Valid()），protocol 31→32，registry S→C ID 25 对称，codec 控制分支与确切 10-byte 校验；`gofmt -w internal/network/message_combat.go internal/network/message_combat_test.go internal/network/packet.go internal/network/packet_test.go internal/network/worldtime_test.go internal/network/registry.go internal/network/registry_test.go internal/network/codec_server.go internal/network/codec_golden_test.go internal/network/codec_fuzz_test.go internal/network/drop_test.go internal/network/place_block_succeeded_test.go internal/network/message_companion_test.go internal/network/message_hostile_test.go`，exit 0；`go test ./internal/network/... -run 'Test(CombatHit|ProtocolVersionPinned|ServerPacket|CodecGolden|Transport)' -count=1`，exit 0（network 2.438s、tcp 1.803s 无匹配但 PASS）；`go test ./internal/network -run '^$' -fuzz '^FuzzSmallPacketCodec$' -fuzztime=5s`，exit 0，300k execs，无 crash/acceptance drift，fuzz seeds 含合法 CombatHit 与当前/前一 hello。
- RED（publication recipient/order/backpressure）：`go test ./internal/server -run 'TestCombatHitPublication' -count=1`，exit 1；按预期攻击者仅收到 InventoryState+PlayerState（2 条），未投影 CombatHit，受击者/旁观者/trusted observer 逻辑与慢 session backpressure 均未满足，根因为 publication 尚未投影。
- GREEN（publication recipient/order/backpressure）：保持 `publishLocalResult` 既有 inventory/crafting/furnace/chest 顺序，仅在 `playerUpdate.Session==current.id` 且 `PlayerState` enqueue 成功后扫描 sorted `result.CombatHits` 并私发 `network.CombatHit{ServerTick:result.Tick}`，任一 enqueue 失败立即 `closePublicationSessionLocked` 并 return；`go test ./internal/server -run 'TestCombatHitPublication' -count=1`，exit 0（3.201s），inventory→PlayerState→CombatHit 顺序、tick 来自 result.Tick、私有性与慢 session 按既有 outbox 策略断开且健康 session 仍收 hit 均通过。
- 扩展 parity（player↔player 与 player→hostile）：`make rust`，exit 0，Rust 1.97.1 release 通过；`go test ./internal/server -run 'Test.*(MeleeParity|SwordCombatParity)' -race -count=1`，exit 0（Melee 1.44s、Sword 5.90s，合计 9.363s），durability 2 iron sword、目标 PlayerState health/velocity、攻击者 inventory mirror sword stack、player/hostile-kind hit、相邻 hit 10-tick 间隔、冷却期无确认、drain 持续到预期 hit 数、hostile health/velocity、剑耐久/损坏形态、Memory/TCP 去绝对 tick 后相等且各 transport 内 hit tick 严格递增均通过，未使用 `PlayerHash`；`go test ./internal/network/tcp -run 'Test.*TransportConsistency' -race -count=1`，exit 0（1.530s，无匹配但 PASS，Memory/TCP 同 codec 契约）。
- 协议基线与 focused gates：根 `AGENTS.md` 协议 v31→v32；`gofmt -w` 覆盖全部 Task 4 文件，exit 0；`go test ./internal/network/... ./internal/sim ./internal/server ./cmd/mornlea-server ./cmd/mornlea/app -race -count=1` 使用 `-p 1` 顺序执行，exit 0（network 4.462s、tcp 1.950s、sim 45.511s、server 230.780s、mornlea-server 21.450s、app 31.743s），避免并行 race 下 subprocess flake，原同命令并行时偶发 `TestMornleaServerProcessReleasesWorldLockAfterSIGTERM`，顺序执行后全绿；`go test ./internal/archcheck -count=1`，exit 0（6.302s）；`openspec validate --all --strict --no-interactive`，exit 0，77 passed、0 failed；`git diff --check`，exit 0；`gofmt -l .` 空。
- Checklist：4.1..4.7 已有实际 RED/GREEN 或直接通过证据；4.8 按控制器指令保持未勾选，等待 implementer 外部的独立 SPEC/QUALITY review。

## Task 4 Self-Review

- SPEC PASS：逐项核对固定 10-byte little-endian `08070605040302010602`、tick>0、damage 1..20、kind 1/2、截断/尾随/wrong state 拒绝、protocol 32、S→C ID 25、ID 26 unknown、golden `1f→20` `1f01026e6f→2001026e6f`、unknown boundary 25→26、fuzz 合法 CombatHit 与 hello seeds、publication 仅私发攻击者且严格排在 inventory/container mirror 与 PlayerState 之后、wire tick=`result.Tick`、victim/bystander/trusted 不收、慢 session 按既有 outbox 策略断开、player melee 与 sword combat 均用 durability 2 iron sword、在目标 PlayerState/hostile health/velocity 与攻击者 inventory mirror / wire hit 上逐 tick 验证 10-tick 间隔与跨传输相等且无 `PlayerHash`。
- QUALITY PASS：完整 diff 只含 Task 4 的 23 个受控文件（新增 4 个：`message_combat.go`、`message_combat_test.go`、`combat_hit_publication_test.go`、`sword_combat_parity_test.go`；修改 15 个 network/server/app/根文件、tasks/ledger），无 core schemas、sim combat 逻辑、render/app capture 超范围修改、Rust 长期文档或 sibling worktrees 变化；无重试/dedupe map/战斗专用队列或 trusted observer 特例；注释中文且无任务编号；wire 编码固定 `u64/u8/u8`，解码在分配前要求恰好 10 bytes 并依赖 done-check 拒绝尾随。
- Mutation check：任一 CombatHit 字段/值域/ID/长度校验、protocol 升版、registry 单侧缺口、golden 单字节漂移、fuzz 缺失合法种子、publication 私发过滤/顺序/backpressure 分支、parity 中耐久/伤害/间隔/跨传输去 origin 比较的删除或错值均至少使一条新增测试失败；既有 network 全包、sim 全包、server 全包、archcheck 与 OpenSpec 均绿。
- Findings：Critical 0、Important 0、Minor 0；无需修复轮次。
- Task 4 独立评审，SPEC PASS：报告 `.superpowers/sdd/2026-08-28-tiered-swords-unified-combat/task-4-review.md:11-66` 对提交区间 `2d6d88df..9f82b654` 完成 7 项契约核对；固定 10-byte `08070605040302010602`、tick>0 damage 1..20 kind 1/2、ID 25/26 boundary、golden `1f→20`、`publication` 私有顺序与 backpressure、10-tick parity 均符合 Task 4 brief；无遗漏或超规。
- Task 4 独立评审，QUALITY PASS：同一报告 `:68-101` 给出 Task quality `Approved`；范围收敛、固定 codec、私有发布无重试/dedupe、根 `AGENTS.md` v32、23+1 文件 scope clean，Critical 0、Important 0、Minor 0。
- Task 4 完成：`2d6d88df..9f82b654` 已具备 implementer RED/GREEN、focused gates、self-review 与独立 SPEC/QUALITY PASS；`tasks.md` 4.1..4.8 全部完成，后续实现从 Task 5 开始。

## Task 5 Implementation

- 起始 HEAD：`ee44f6a74245ab7dfd8da549aa12526450a41265`；起始 `git status --short --branch` 退出 0，仅输出分支名，无 tracked/untracked 改动。
- RED（combat feedback 状态机）：`go test ./cmd/mornlea/app -run 'TestCombatFeedback' -count=1`，exit 1；按预期因 `combatFeedback`、`combatMarkerFrameCount` 不存在而编译失败（`undefined: combatFeedback`），证明状态机测试能捕获缺失实现。
- GREEN（combat feedback 状态机）：实现 Darwin-tagged `combatFeedback{lastServerTick uint64, remainingFrames uint8}` 与固定 `6`、`Observe` 严格递增、`ArmMarker`、`MarkerVisible`、`AfterRender(rendered bool)` 仅 rendered 时递减、`Reset` 清零，值直接归属 `Application`；`gofmt -w cmd/mornlea/app/combat_feedback.go cmd/mornlea/app/combat_feedback_test.go cmd/mornlea/app/app.go`，exit 0；`go test ./cmd/mornlea/app -run 'TestCombatFeedback' -count=1`，exit 0（1.06s），tick 1 接受 6、重复/陈旧忽略、tick 2 重置、`AfterRender(false)` 不减、六次 true 后不可见、`Reset` 清零通过。
- RED（app message/lifecycle）：`go test ./cmd/mornlea/app -run 'TestApplicationCombat|Test.*Session.*Reset' -count=1`，exit 1；首次因 `audio.CueCombatHit` 不存在编译失败（`undefined: audio.CueCombatHit`），追加 cue 后仍因 `DrainServerMessages` 未消费 `CombatHit` 而导致 `unsupported server message network.CombatHit` 并关闭会话，`recorder` 为空，证明消息与生命周期测试能捕获缺失发布与重置逻辑；`go test ./internal/audio -run '^TestCueCombatHitPCM$' -count=1` 增加后首次仍因 cue 缺失而编译失败，符合音视频 RED 预期。
- GREEN（audio 固定 PCM cue）：在 `CueWaterSplash` 后追加 `{samples:1323, startHz:520, endHz:180, amplitude:10500}`，复用 `[cueCount][]int16` 与现有队列；`gofmt -w internal/audio/cue.go internal/audio/cue_test.go`，exit 0；`go test ./internal/audio -run '^TestCueCombatHitPCM$' -count=1`，exit 0（0.7s），append-only 值、`CueCombatHit==CueWaterSplash+1`、`cueCount==CueCombatHit+1`、参数与 little-endian PCM SHA-256 `17752cdda0232ebb88b0e6db1e39fa4a4889e5469bac0c28a07044b677710dae` 精确匹配。
- GREEN（app message/lifecycle）：在 `DrainServerMessages` 的 `PlayerState` 分支后增加 `network.CombatHit` 分支，仅 `Observe` 成功时 `playLocalCue(audio.CueCombatHit)` 并 `continue`；`PlayerState.Reset` 同时 `audioFeedback.Reset()` 与 `combatFeedback.Reset()`；`resetSessionOwnedState` 同时 `combatFeedback.Reset()` 并在非 nil 时 `hostiles.Reset()`，不新增重复生命周期调用；`gofmt -w cmd/mornlea/app/app_messages.go cmd/mornlea/app/app_lifecycle.go cmd/mornlea/app/combat_feedback_application_test.go`，exit 0；`go test ./cmd/mornlea/app -run 'TestApplicationCombat|Test.*Session.*Reset' -count=1`，exit 0（1.36s），严格新 hit 播放一次 cue 并 arm、同 tick Reset 后仍接受、disconnect/菜单/新 session/authoritative reset 全清、共享 session 清 hostile 均通过。
- RED（HUD marker geometry/capacity）：`go test ./internal/render/hud -run 'Test.*(CombatMarker|FixedCapacity|ReusesLayout|Responsive)' -count=1` 在未实现前因 `HotbarRenderer.Prepare` 缺少 `combatMarker bool` 参数而编译失败（`not enough arguments`），证明 HUD 测试能捕获缺失输入；实现后 `HotbarRenderer.Prepare` 在 chat 与尺寸参数间追加 `combatMarker bool`，既有调用传 false，新增 `appendCombatMarker` 在 health/oxygen/hunger/chat 后仅在 true 时追加 4 个白色不透明 untextured quad，公式严格 `(4+8/2)*scale`，不改 `maxHotbarQuads`、`maxHotbarGlyphs`、`hotbarGlyphOffset`、`hotbarUploadBytes`、atlas、shader 或 pipeline。
- GREEN（HUD marker）：`gofmt -w internal/render/hud/combat_marker.go internal/render/hud/renderer.go internal/render/hud/renderer_test.go internal/render/hud/chat_test.go internal/render/hud/hunger_test.go internal/render/hud/eating_test.go`，exit 0；`go test ./internal/render/hud -run 'TestHotbar' -count=1`，exit 0（0.4s）；marker-on 最大关闭/打开 100/261 仍 `<=267`，warmed `Prepare` 零分配且实例严格位于 framebuffer 内，几何中心 `(width/2,height/2)`、`2×8` 垂直、`8×2` 水平、内缘 `4*hudScale` 均通过，现有 96/257 不变。
- GREEN（frame marker 消费）：`app_frame.go` 将 `combatMarker` 纳入 `hudVisible` 并传 `Prepare`，仅在 `client.RenderFrame` 返回后调用 `a.combatFeedback.AfterRender(rendered)`，零 framebuffer、entity overflow、name-tag/HUD prepare error 均在 native render 前 return 而不扣帧；`go test ./cmd/mornlea/app -run 'TestCombatMarkerOnlyConsumedAfterSuccessfulNativeRender' -count=1`，exit 0（0.13s），四个失败边界均保持 6 帧，六次成功后不可见通过；`go test ./cmd/mornlea/app -run 'TestApplicationCombat.*' -count=1` 全绿。
- GREEN（sword colors 与 atlas）：`render.ItemColor` 固定 switch 增加六个原创 alpha=1 互异颜色（木 184,134,72、石 148,148,148、铁 200,205,210、损坏木 92,67,33、损坏石 82,82,80、损坏铁 115,120,125），两两互异且 intact/broken 不同，掉落经 `itemDropColor` 可见；`gofmt -w internal/render/drop.go internal/render/drop_test.go`，exit 0；`go test ./internal/render -run 'TestSword' -count=1`，exit 0；HUD atlas 宽度由 `ItemIDMax` 自动 960→1056，旧 800 注释改为动态 `hotbarTextureWidth` 与 `W <= 2^15`，UV 稳定性探针改为 `hotbarTextureWidth`、`+16`、`+32`、2048、4096，`go test ./internal/render/hud -run 'Test.*Atlas' -count=1`，exit 0。
- RED（sword-combat capture）：`go test ./cmd/mornlea/capture -run 'Test(SwordCombatCaptureState|CaptureSceneOrderAndAICompanionDeterminism)' -count=1`，exit 1；按预期因 `sword-combat` 场景与 `SceneApplication` 三方法缺失而编译失败（`undefined: captureSwordCombatPlayerID` / `scene "sword-combat" 不存在`），证明场景与接口测试能捕获缺失实现。
- GREEN（sword-combat capture）：仅向 `Application` 与 `SceneApplication` 增加真实消费方法 `ArmCombatMarker`/`ResetCombatFeedback`/`CombatMarkerVisible`（`combat_feedback.go` 与 `scene_application.go`），`resetCapturePresentation` 调 `ResetCombatFeedback`，`sword-combat` 复用 `prepareAICompanion` 的开阔草地/air helper，`PinVolatile` 重 arm 6 帧，相机 `(5.5,3.2,9.5)`、时间 6000、选中铁剑 `Durability:125`、合法 UUIDv4 远端玩家经 spawn/state 沿 0.35 击退、marker 可见；同步 `app/AGENTS.md` 与 `capture/AGENTS.md` 的文件地图、最小接口与 reset 责任，不复制漂移清单；`gofmt -w cmd/mornlea/capture/capture.go cmd/mornlea/capture/capture_scene.go cmd/mornlea/capture/scene_application.go cmd/mornlea/capture/capture_sword_combat.go cmd/mornlea/capture/capture_sword_combat_test.go cmd/mornlea/capture/capture_hostile_mob_test.go cmd/mornlea/capture/capture_scene_order_test.go`，exit 0；`go test ./cmd/mornlea/capture -run 'Test(SwordCombatCaptureState|CaptureSceneOrderAndAICompanionDeterminism)' -count=1`，exit 0（0.9s），顺序严格 `ai-companion,sword-combat,hostile-mob,water-surface-slope`、总数 24、far-horizon 倒数第二、water-underwater 末项、hostile 原“紧随 ai”断言已更新。
- 视觉基线（RED→GREEN）：`make rust`，exit 0（Rust 1.97.1 release）；`make visual-check VISUAL_OUT=build/visual-sword-combat`，exit 1，唯一失败为缺少 `sword-combat.png` 并在输出目录生成候选 `sword-combat.png`（196K，24 张），其他 23 张差异 0；人工复核候选确认非满耐久铁剑、远端玩家、hit marker 与 0.35 击退均可见；`make visual-update VISUAL_OUT=build/visual-sword-combat-update`，exit 0，写入基线 LOD control 通过并生成 24 张 golden；`git status --short cmd/mornlea/capture/testdata/golden` 仅新增 `?? sword-combat.png`，其他 PNG 无 diff；`make visual-check VISUAL_OUT=build/visual-sword-combat-check`，exit 0，24/24 差异 0，阈值未放宽。
- Focused gates（预提交）：`gofmt -w` 覆盖全部 Task 5 文件，exit 0；`go test ./internal/audio ./internal/render ./internal/render/hud ./cmd/mornlea/app ./cmd/mornlea/capture -race -count=1`，exit 0（audio 1.7s、render 5.0s、hud 2.7s、app 29.9s、capture 5.8s）；`go test ./internal/archcheck -count=1`，exit 0（5.0s）；`make visual-check`，exit 0（24/24）；`openspec validate --all --strict --no-interactive`，exit 0，77 passed、0 failed；`git diff --check`，exit 0。
- Checklist：5.1..5.12 已有实际 RED/GREEN 或直接通过证据；5.13 按控制器指令保持未勾选，等待 implementer 外部的独立 SPEC/QUALITY review。

## Task 5 Self-Review

- SPEC PASS：逐项核对 Darwin-tagged `combatFeedback` 6 帧、严格递增、`AfterRender` 仅 rendered true 递减、`Reset` 清零、`Application` 直接持有、仅新 `CombatHit` 播放一次 `CueCombatHit` 且同 tick Reset 后仍接受、disconnect/菜单/新 session/authoritative reset 全清且共享 reset 清 hostile、`CueCombatHit` 1323/520→180/10500 与 SHA-256、HUD 4 quad 白色不透明 untextured、中心与 `2×8`/`8×2`、内缘 `4*scale` 与 `(4+8/2)*scale`、最大 100/261≤267、warmed 零分配、仅成功 native render 后扣帧且四失败边界不扣、六 sword 颜色 alpha=1 互异且 intact/broken 不同、atlas 1056 与 UV 探针、`sword-combat` 铁剑 125、UUIDv4 远端玩家 0.35 击退、固定相机/时间、marker 可见且 `PinVolatile` 重 arm、顺序 24 与 far-horizon/water-underwater 尾序。
- QUALITY PASS：完整 diff 只含 Task 5 的 27 个受控文件（新增 6：`combat_feedback.go`、`combat_feedback_test.go`、`combat_feedback_application_test.go`、`combat_marker.go`、`capture_sword_combat.go`、`capture_sword_combat_test.go`、`sword-combat.png`；修改 20：audio、render、hud、app、capture、tasks/ledger、AGENTS），无 core 协议外 schema、sim/server 存档、Rust、依赖或长期文档超范围修改；无 map/sort 分配、新接口/配置或 speculative helper；注释中文且无任务编号；HUD 保持 267/700/13312/46912 与 256 对齐，不改 shader/pipeline/atlas 生产计算。
- Mutation check：任一 combat tick 去重、剩余帧、`AfterRender` 条件、`Reset` 范围、`CombatHit` 分支位置、`PlayerState.Reset`/`resetSessionOwnedState`/`resetCapturePresentation` 清理、`CueCombatHit` 参数/hash、`Prepare` 签名与 `appendCombatMarker` 条件/公式/颜色/容量、`ItemColor` 任一剑颜色、`hotbarTextureWidth` 探针、`sword-combat` 任一夹具/相机/耐久/UUID/距离/marker/PinVolatile/顺序 的删除或错值均至少使一条新增测试失败；既有 audio 全包、render 全包、hud 全包、app 全包、capture 全包、archcheck 与 OpenSpec 均绿。
- Findings：Critical 0、Important 0、Minor 0；无需修复轮次。
- Task 5 独立评审，SPEC PASS：报告 `.superpowers/sdd/2026-08-28-tiered-swords-unified-combat/task-5-review.md:12-62` 对提交区间 `ee44f6a7..1be4a9f7` 完成 8 项契约核对；Darwin 6 帧严格递增、`AfterRender` 仅 rendered、`CueCombatHit` 1323/520→180/10500 SHA、`4 quad` 几何与 `100/261≤267`、成功 render 后扣帧、6 剑颜色与 1056 atlas、`sword-combat` 125/UUIDv4/0.35/24 顺序均符合 Task 5 brief；无遗漏或超规。
- Task 5 独立评审，QUALITY PASS：同一报告 `:63-89` 给出 Task quality `Approved`；范围收敛、无 animation manager/shader/pipeline、中文注释无任务编号，Critical 0、Important 0、Minor 1（未提交 100/261 定量锁，行为正确且不影响发布契约）。
- Task 5 完成：`ee44f6a7..1be4a9f7` 已具备 implementer RED/GREEN、focused gates、self-review 与独立 SPEC/QUALITY PASS；`tasks.md` 5.1..5.13 全部完成，后续实现进入 Task 6。

## Validation Evidence

### Post-Merge Reconciliation

- 归档前按当前 main 重新执行视觉验证：`go clean -cache` 清除旧的 cgo package cache（capture 进程曾观察到 Go bridge `ABIVersion=7` 而当前 engine 为 8）；`make rust`、`go test ./cmd/mornlea/capture -race -count=1`、`go test ./cmd/mornlea/app -run 'TestApplicationItemPopup|TestCombatFeedback' -race -count=1` 与 `go test ./internal/nativeabi -race -count=1` 均通过。
- 初次 `make visual-check` 仅 `sword-combat` 失败（最大通道差 213、差异像素 13.3342%）；图像与调用链复核确认 `prepareAICompanion` 遗留的选中基线使装入第 2 格铁剑时错误触发“铁剑”弹条，并使该场景 HUD 与现有 golden 偏移。`applySwordCombatCaptureState` 增加既有 `ResetItemPopupBaseline`，不放宽阈值。
- `make visual-update VISUAL_OUT=build/visual-update` 首次因 120 秒门限中止且未写入 golden；随后以 600 秒门限完成，逐图复核并确认仅 `cmd/mornlea/capture/testdata/golden/sword-combat.png` 改变。当前正式 capture 与 golden 均为 25 项。
- 修复后 `make visual-check VISUAL_OUT=build/visual-final` 通过，25/25 场景最大通道差与差异像素均为 0；`go test ./cmd/mornlea/capture ./cmd/mornlea/app -race -count=1`、`go test ./internal/render/hud -race -count=1`、`openspec validate --all --strict --no-interactive`（77 passed、0 failed）和 `git diff --check` 均通过。
- `go test ./internal/archcheck -count=1` 当前被工作区已有配置变化阻断：`.codex/hooks.json` 被删除且 `.claude/settings.json` 被修改，`TestMornleaCurrentIdentity` 因缺失 `.codex/hooks.json` 失败；本次不恢复、不覆盖这些用户配置。

- 基线验证证据见 `Baseline`，均对应 frozen code SHA `67fcc604bd3f3b5ce9326b5cd7498381163296d6`，本 Task 未重复运行。
- `openspec status --change tiered-swords-combat`，exit 0，`4/4 artifacts complete`；验证时 HEAD `6a992a5c5081d707423b2361ea5dc985375f19c8`，工作区仅含本 change 的 16 个 intent-to-add 文件。
- `openspec validate --all --strict --no-interactive`，exit 0，77 passed、0 failed；验证时 HEAD 与工作区状态同上。
- `git diff --check`，exit 0，无输出；验证时 HEAD 与工作区状态同上。写入本证据后，提交前 MUST 对最终 active change bytes 重新运行同一组三项命令。

## Deferred And Abandoned

- 无未决玩法、协议、容量、兼容或视觉数值。
- OpenSpec sync/archive、长期版本文档、`openspec/config.yaml`、backlog/Discussion、PR/CI 与 worktree cleanup 明确属于计划 Task 6，不是本 active change checklist 的延期项。
- dirty sibling worktrees 与 `docs/notes/lan-server.md` 永久排除在本 change 文件范围之外。
