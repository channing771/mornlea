# Mornlea 功能规划与认领表

本表是 Mornlea 后续开发任务的**单一规划来源**：每一行 = 一个可独立认领、独立评审、独立验收的功能或工程任务，通常对应一条 OpenSpec change。控制会话与 implementer 子代理在开始任何开发前先读本表；认领后立即在行内打标记并提交，防止多个 agent 抢同一任务。本表只描述「要做什么、谁在做、依赖什么」，**不排时间表**。

> 开发流程、工程约定与验证门禁以根 `AGENTS.md`、对应作用域的局部 `AGENTS.md` 与 `openspec/config.yaml` 为准；规则只更新对应作用域的 `AGENTS.md`，同级 `CLAUDE.md` 保持薄导入。本表不重复、不替代这些入口；它们与代码矛盾时，以代码、测试与 `openspec/specs/` 主规格为真相。
>
> 状态列表镜像（按状态分组、每轮刷新）见 GitHub Discussion [#71 Mornlea 功能缺口拆解与任务认领池（规划）](https://github.com/channing771/mornlea/discussions/71)；两处状态不一致时以本文件为准并回改讨论。

## 状态图例

| 状态 | 含义 | 谁可继续 |
|---|---|---|
| 就绪 | 需求、依赖、版本槽和文件冲突均已清空 | 任意 agent 可认领 |
| 已认领 | 正在内容确认或 OpenSpec 设计，尚未改功能代码 | 只有认领人 |
| 开发中 | 已进入实现或验证 | 只有认领人 |
| 待集成 | 实现与任务评审完成，等待本行自己的归档、PR 或合入 | 只有认领人 |
| 排队 | 范围已明确但前序未完成，不占版本号和文件 | 无人；由 planner 晋升 |
| 设计候选 | umbrella、推测性或远期能力，尚不能形成独立实现闭环 | 无人；先重新设计 |
| 已完成 | 已合入 `main` 并归档 | 无人 |
| 已取消 | 已被其它流程取代或不再需要，保留历史原因 | 无人 |

## 认领方式

1. 读取本表、`openspec/config.yaml` 与 `AGENTS.md`「开始工作前」清单。
2. 只选择一行**就绪**行；同一时间只认领一行。`排队` 与 `设计候选` 不得认领或预占文件，planner 只在前序完成后晋升队首为 `就绪`。
3. 编辑该行：`状态` → `已认领`，`认领人` → `<agent 标识> @ <分支名>`，并在备注写明独占文件集（如与其它已认领行冲突则换行）。提交（docs-only，不关联 OpenSpec change）。
4. 按 `AGENTS.md` 的 isolation worktree 习惯创建分支（`using-git-worktrees` 技能）。
5. 按下方「开发流程」执行；合入 `main` 并归档后，把该行 `状态` → `已完成`（认领人保留履历）。
6. 已认领行不得抢；转移必须经控制会话裁决并留档。

## 开发流程

完整流程（认领 → OpenSpec change → subagent-driven-development 执行 → SPEC/QUALITY 双评审 → 门禁 → 归档收尾 → 并行冲突规则）在**唯一**的说明文档 [`docs/development-process.md`](development-process.md)；本表与 GitHub Discussion #71 只引用它，不再内嵌重复。

日常执行者：每日固定时间扩展规划的**规划者**、从本表认领并开发收尾的**实现者**——角色卡、调度与运行入口见 [`docs/agents/README.md`](agents/README.md)（`make agent-planner` / `make agent-implementer`）。收尾前运行 `scripts/agents/gates.sh` 汇总门禁子集，并单独运行 `make rust-check`；两者都通过才是完整提交前门禁。

## 当前基础玩法发布列车

近期产品边界是「首夜生存 + 自给家园」。核心玩法按下表串行推进；同一时刻最多一行进入版本化实现，绝对版本号在各行基于届时 `main` 确定。
| 顺序 | 行 | 结果 | 解锁条件 |
|---|---|---|---|
| 1 | A-01 | 可真实使用的 2×2/3×3 格子合成与最小单件拆分 | 当前在途 |
| 2 | A-02 | 可放置、可支撑、会发光的火把 | A-01 已完成 |
| 3 | A-03 | 三级剑与统一近战结算 | A-02 已完成 |
| 4 | A-04 | 可生成、追击、攻击、掉落和持久化的夜行者 | A-03 已完成 |
| 5 | A-05 | 床、跳夜、敌怪阻睡和个人重生点 | A-04 已完成 |
| 6 | B-04 | 草丛自然掉种子并取消固定种子赠送 | A-05 已完成 |
| 7 | B-02 | 可搬运水源 | B-04 已完成 |
| 8 | B-33 | 树苗与橡树再生 | B-02 已完成 |
| 9 | B-27 | 单一被动生物、原肉与熟肉 | B-33 已完成 |
| 10 | B-34 | 移除十四组整栈初始材料包 | B-27 已完成 |

## 并行与冲突规则

- **核心玩法串行、每行自行收尾**：当前发布列车同一时刻最多一行进入版本化实现；每行自行完成实现、任务评审、版本与 golden、归档、PR 和合入，不再另设批次合流或基线任务。
- **版本号互斥**：协议 / 存档 schema / engine ABI / client ABI / benchmark scenario 的升版行互斥——同一时间只能被一个认领者持有；绝对版本号基于该行实现时的 `main` 确定。
- **保护无关改动**：认领时在备注声明独占文件集；与其它在途行冲突即换行或延迟，且不得清理、覆盖或 rebase 无关 worktree 改动。
- **范围冻结**：认领后不得扩大范围；实现发现规格不成立时，先改该 change 的 OpenSpec 产物再继续编码。

---

## A. 第一夜生存队列（串行推进）

> 原设计与计划见 `docs/superpowers/specs/2026-08-23-first-night-survival-parallel-wave-design.md` 与 `docs/superpowers/plans/2026-08-23-first-night-survival-parallel-wave.md`。原批次的五个功能分支仅存在于前一开发机的本地 worktree（`.worktrees/`）与本地 `codex/*` 分支（共享契约 SHA `785ea07b`），从未推送远端，已随机器迁移丢失；2026-08-25 曾经控制会话裁决重做。当前按 `docs/superpowers/specs/2026-08-26-basic-gameplay-backlog-replan-design.md` 改为逐行串行推进并自行收尾，各行基于届时 `main` 使用 next version，不预先承诺未来绝对版本号。

| ID | 功能 | 简述 | 状态 | 认领人 | 来源与备注 |
|---|---|---|---|---|---|
| A-01 | 权威格子工作台 | 背包 2×2 + 工作台 3×3 形状配方、权威产物取出、关闭/断线装回不变量、七条新配方 | 已完成 | zcode-implementer @ feat/A-01-authoritative-grid-crafting | 原分支头 `a657c1cb`（实现与整功能评审完成）未推送远端已丢失；重做后当前分支头 `8c2ea638`，四任务组全部双评审 PASS（SPEC+QUALITY）、功能线门禁全绿。2026-08-27 控制会话完成收尾：PR #100 已合并（CI 8/8 全绿 · merge `f0518776`），change 归档为 archive/2026-08-27-authoritative-grid-crafting 并 sync 进 `authoritative-grid-crafting` 主规格（新建能力）；编号终值按 A-06 取消后自行锁定——删除 `CraftRecipe` 死代码、`MoveCraftingStack=7`（顶替）、`TakeCraftingOutput=15`（14 已被 B-03 `BoneMeal` 占用）。顺延项（归档时记录）：mini-authority 语义对齐复查、`CraftRecipe` 措辞精化、`FinalOuput` 拼写、B2/B3/B4（背包 36 格穷举、产物格×背包格互斥断言、chest/furnace 显式 Reset）、`Command.Recipe` 死字段、`container_test.go:197` 颜色断言、`app_input.go` 裸标识符、两次点击脚手架；recipe 14..18 由 A-02/A-03/A-05 追加；golden 19 张口径（含 `main-menu.png`） |
| A-02 | 可放置火把 | 落地 + 四向墙面五形态、支撑约束与移除反应、发光等级 14、mesh model tag 与窄柱几何 | 已认领 | zcode-implementer @ feat/A-02-placeable-torches | 原认领履历 `ox-alpha-implementer @ feat/A-02-placeable-torches`；原分支头 `b2115a64`（实现与整功能评审完成）未推送远端已丢失；2026-08-27 用户裁决从当前 `main` 重做：旧 worktree 的 16 个未提交 core/assets/physics 残留 stash 留档后干净基线重建。2026-08-27 控制会话批准重做边界：复用原 change 产物并按 A-01 合流后的基线修订——方块/物品/配方编号按当前 registry 终值追加（A-01 已锁 `BlockIDMax 62`/`ItemIDMax 43`/recipe 编号）、火把配方（煤上棍下出 4）由本行在 A-01 格子合成系统上追加、engine ABI v7→v8（v7 已被 D-07 占用）、torch-night 场景与 golden 由本行自带（A-07 取消后视觉基线职责回收）、无 wire/协议/schema/scenario 变更。独占文件集：`internal/core` 方块/物品/配方/光源表登记、`internal/assets`、`internal/sim` 火把放置与支撑、`internal/world/chunk.go`、`internal/mesh`、`internal/nativeabi`、`engine/crates/mornlea_engine`、`openspec/changes/placeable-torches`、`cmd/mornlea/capture_scene*` 与对应 golden |
| A-03 | 三级剑与统一战斗 | 木/石/铁剑（4/5/6 伤害）、统一候选（≤72）与冷却/击退/耐久、CombatHit 私有确认 | 排队 | — | 原分支头 `42f3dead`（SPEC/QUALITY PASS）未推送远端已丢失；recipe 15..17 依赖 A-01 合流；依赖 A-02；版本和编号不再由 A-06 预分配 |
| A-04 | 权威近战夜行者 | 确定性夜间生成、全服 64/每玩家 8 上限、A* + Rust 物理、灼烧/消失/腐肉掉落、`hostile_mobs` 持久化 | 排队 | — | 原认领履历 `claude-implementer @ feat/A-04-hostile-nightwalker`；原分支头 `eb1923eb`（持久化修复两轮已提交）未推送远端已丢失；保留原批准设计 `openspec/changes/authoritative-hostile-nightwalker`。依赖 A-03；排队期不占文件 |
| A-05 | 床与睡眠 | 双格床八形态、同区块原子放置、全员睡眠跳夜（`DayTimeOffsetTicks`）、个人重生点 | 排队 | — | 原认领履历 `codex-implementer @ feat/A-05-authoritative-bed-sleep`；原共享契约 SHA `785ea07b` 未推送远端已丢失（本无实现损失）；原 OpenSpec change `authoritative-bed-sleep`。依赖 A-04；排队期不占文件 |
| A-06 | 五路合流集成 | 原批次合流职责已拆回各功能行 | 已取消 | — | 原来源：第一夜批次计划 Task 3；合流与接线职责回收到 A-01..A-05 |
| A-07 | 版本基线与视觉基线 | 原批次统一基线职责已拆回各功能行 | 已取消 | — | 原来源：第一夜批次版本与视觉基线任务；版本、golden 和基线职责回收到各功能行。历史上原批次曾预占 client ABI v10，使 D-02/D-03 在合流前被锁；本次取消 A-07 后该预占已作废，后续各功能行基于届时 `main` 确定 next client ABI |
| A-08 | 整分支终审、归档与推送 | 原批次统一收尾职责已拆回各功能行 | 已取消 | — | 原来源：第一夜批次计划 Task 5；终审、归档和 PR 职责回收到各功能行 |

## B. 生存与世界深化（后续功能候选）

> 每行一条独立 OpenSpec change。来源列括号内是归档 change 的 `design.md`「遗留与简化清单」或 `proposal.md`「非目标」的条目，或批次设计「非目标 / 已知简化与升级条件」。「版本与契约影响」只写方向性结论（如「协议升版」「编号追加」），绝对版本号按合入时序在「并行与冲突规则」下重排。

| ID | 功能 | 简述 | 版本与契约影响 | 状态 | 认领人 | 来源与备注 |
|---|---|---|---|---|---|---|
| B-01 | 更多作物 | 在既有小麦之外追加作物，按编号、纹理和生长参数分别形成闭环 | 方块/物品编号与配方表追加，无 wire 结构变更 | 已完成 | opencode-implementer @ feat/B-01-more-crops | farming 遗留 2；hunger 遗留 1 的肉类与熟食已收敛到 B-27，本行不再重复；原作物范围曾因与 A-02 文件及 mesh registry 容量冲突而延迟，须重新设计后再进入队列；2026-08-27 控制会话晋升设计候选→就绪→已认领并当日完成：追加 16 方块（`PotatoStage0..7`/`CarrotStage0..7`）+ 3 物品（`ItemPotato/Carrot/PoisonousPotato`，堆叠 64），`IsCrop` 三区间并集，`growCrop` 同规分派，成熟 `1..4`（马铃薯附 2% 毒土豆）经独立 `splitmix64` salt，食物 `1/600`、`3/3600`、`2/1200`，`cross` 复用、存档 round-trip、骨粉/踩踏/水冲沿 `IsCrop` 自动覆盖；独占文件集：`internal/core (BlockID/ItemID), internal/sim (crop/mining/bone_meal), internal/assets, internal/mesh, internal/storage`；2026-08-27 归档为 `openspec/changes/archive/2026-08-27-more-crops/` 并 sync 进 `authoritative-farming` 主规格（`openspec validate --all --strict` 67 passed），`go test ./... -short` 全绿，协议 v27、ABI 与 scenario 均不变 |
| B-02 | 水桶（可搬运流体） | 舀水/倒水物品，解除「农业只能在天然水体 4 格内」约束 | 物品编号追加；无限水源规则随本行一并裁决 | 排队 | — | farming 遗留 25（显式非目标解除）；fluid proposal 非目标「两个源相邻生成新源」约定随水桶交付；依赖 B-04 |
| B-03 | 骨粉 | 新物品 + 「立即推进 N 阶段」动作，走翻地同形命令路径 | 物品编号追加；命令段可能追加 | 已完成 | opencode-implementer @ feat/B-03-bone-meal | farming 遗留 3；2026-08-27 控制会话晋升排队→已完成，bounded：`ItemBoneMeal` + `BoneMeal` 命令（`Yaw/Pitch`，目标由射线决定，`ProtocolVersion 26→27`）立即推进 `WheatStage0..6→下一阶段` 1 阶，`Stage7` 不变，消耗 1，拒绝零消耗。独占文件集：`internal/core/item.go`（`ItemBoneMeal` append-only）、`internal/network`（`message_command.go`/`packet.go`/`registry.go`/`codec_client.go`）、`internal/sim/command.go`+`bone_meal.go`及测试、`internal/server/session_ingress.go`、`cmd/mornlea/app_input.go`；2026-08-27 归档为 `openspec/changes/archive/2026-08-27-bone-meal/` 并 sync 进 `authoritative-farming` 主规格（`openspec validate --all --strict` 67 passed），`go test ./... -short` 全绿 |
| B-04 | 草丛与除草掉种子 | 植物几何第二消费者；草丛掉落种子替代初始材料包供给 | 植物区间编号追加 | 排队 | — | farming 遗留 6；依赖 A-05；同一行取消固定 64 种子赠送 |
| B-05 | 踩踏破坏耕地 | 实体落地事件接进方块变更（物理侧已有落地判定） | 无 wire 变更 | 已完成 | zcode3-implementer @ feat/B-05-trample-farmland | farming 遗留 4；2026-08-25 认领。控制会话裁决：三处最小受控重叠——`internal/sim/player.go` 仅限摔落/落地结算区插入踩踏判定调用、`internal/sim/crop.go` 仅限 `advanceCrops` 首部一处结算调用（B-10 已归档无在途独占）、`internal/sim/engine.go` 仅限 `Engine` 结构体追加一行暂存字段声明（append-only，详见 change `farmland-trample` ledger Ruling）。独占文件集：`internal/sim` 新建踩踏结算文件与同包新增测试、OpenSpec change 目录；踩踏触发的作物掉落只调用既有导出掉落 API；刻意不触碰 `combat.go`/`hunger.go`（B-13）、`engine_step.go`/`drop.go`/`tunables.go`（A-04）、`mining.go`（C-01）与 `internal/core` 编号段（A-01/A-02/A-04）；2026-08-25 当日完成：PR #85 已合并（CI 8/8 全绿，run `32874791885`），`farmland-trample` 已归档并 sync 进 `authoritative-farming` 主规格，基线段已同步 |
| B-06 | 干耕地退回泥土 | 需第三个耕地编号或附加状态字节 | 方块编号追加或状态编码 | 已完成 | opencode-implementer @ feat/B-06-dry-farmland-revert | farming 遗留 5；2026-08-27 控制会话晋升排队→就绪并认领，bounded 实现：干+上方为空气时经随机tick 30%退回泥土，湿/有作物不退，零掉落原子写入，复用crop抽样有界性；`internal/sim/farmland_revert.go`+`farmland_revert_test.go`、`internal/sim/crop.go`复用抽样；2026-08-27 归档为 `openspec/changes/archive/2026-08-27-dry-farmland-revert/` 并 sync 进 `authoritative-farming` 主规格（`openspec validate --all --strict` 67 passed），`go test ./... -short`全绿 |
| B-07 | 水冲毁作物 | 流体可流入作物格并触发掉落；需同步流体的确定性论证与不动点论证 | 流体 `evalCell` 规则变更，论证同步更新 | 已完成 | ox-alpha-implementer @ feat/B-07-flood-destroys-crops | farming 遗留 1；2026-08-25 认领。独占文件集：`internal/fluid` 全部（`evalCell` 作物流入规则、确定性/不动点论证与测试）、`internal/sim` 新建作物冲毁结算文件（只调用既有导出掉落 API，不改 A-01/A-04/B-10 已认领的 `drop.go`/`mining.go`/`crop.go`/`engine_step.go`）、OpenSpec change 目录；无 wire 变更、无编号追加，不触碰 A 批次任何独占文件集 |
| B-08 | 地下农场 | 服务端可查询的极简方块光模型，或重新裁决「服务端不计算光照」禁令 | 视裁决（spec 禁令重裁或新可查模型） | 设计候选 | — | farming 遗留 7 |
| B-09 | 湿润判定即时化 | 水源移除后扇出的有界化（与流体队列同构），或耕地湿度缓存；成本计量口径改为方块读取次数 | 无 wire；成本契约计量口径调整 | 已完成 | opencode-implementer @ feat/B-09-instant-farmland-moisture | farming 遗留 9、21；2026-08-26 认领。范围冻结为显式有界耕地湿度更新队列、方块读取次数成本契约与重启/兴趣边界恢复。独占文件集：新建 `internal/sim/farmland_moisture*.go` 及相关测试，修改 `internal/sim/fluid.go`、`farming.go`、`crop.go`、`crop_perf_test.go`、`fluid_perf_test.go`、`companion_action_test.go`，并按关注点拆分 `crop_test.go`；控制会话批准与 A-04 的三处最小受控重叠：`engine.go` 仅追加队列/计数状态字段，`engine_step.go` 仅追加固定阶段，`tunables.go` 仅更正 `RandomTicksPerSection` 既有农业注释。实现阶段刻意不触碰 `internal/fluid`、A-04 的 hostile 状态与 tunable 字段，不改协议/schema/ABI/scenario、capture golden 与长期基线文档；归档阶段已同步 OpenSpec 主规格及长期基线文档。OpenSpec change `instant-farmland-moisture`；2026-08-26 完成：归档为 `openspec/changes/archive/2026-08-26-instant-farmland-moisture/`，PR #94 CI 8/8 全绿 |
| B-10 | 作物随机掉落数量 | `hash(worldSeed, tick, pos)` 定数量，与生长抽样共用哈希 | 无 wire 变更 | 已完成 | ox-alpha-implementer @ feat/B-10-crop-drop-hash | farming 遗留 10；2026-08-25 认领并当日完成：PR #81 已合并（CI 8/8 首跑全绿），`crop-random-drop-count` 已归档；成熟小麦 1–3 小麦 + 1–3 种子、重放确定，D9 固定掉落决策由其接替 |
| B-11 | 难度系统 | 困难难度饿死、和平回满、刷怪门控等难度分支 | 配置格式追加难度项 | 排队 | — | hunger 遗留 4；批次设计非目标；依赖 B-30 |
| B-12 | 饱和抖动提示 | `PlayerState` 追加 `SaturationZero` 一位 | 协议升版（`PlayerState` 追加字段） | 已认领 | opencode-implementer @ feat/B-12-saturation-jitter | hunger 遗留 3；2026-08-27 控制会话经 brainstorming（bounded，方案 A）晋升设计候选→已认领：`saturationZero = saturationMilli==0` 每 tick 定格随 `PlayerState` 下发，`ProtocolVersion 28→29` 尾部追加 1 bool，HUD 抖动仅呈现分支不改存档；独占文件集：`internal/sim`（player/hunger）、`internal/network`（packet/codec）、`internal/client` 透传、`internal/render/hud` 抖动分支、`openspec/changes/saturation-jitter` |
| B-13 | 冲刺与攻击疲劳 | 对应动作出现后疲劳表加行 | 无 wire（疲劳表加行） | 已完成 | ox-alpha-implementer @ feat/B-13-attack-exhaustion | hunger 遗留 6；攻击疲劳半边随 change `attack-exhaustion` 交付并归档：固定表第六行 `exhaustionMeleeMilli=100`，判定点在意图冻结分叉。PR #84 以 merge `04bd58ad` 合入，run `32878866254` 的 8 项全绿；SPEC/QUALITY 双评审与整分支终审通过（终审三项文档级发现经一轮修复波清偿）。冲刺半边依赖 B-30 协议升版，另行认领 |
| B-14 | 进食动画/音效/进度 HUD | 复用采掘进度条呈现形状 + 既有音频确认边界 | 无 wire（呈现层） | 已完成 | zcode2-implementer @ feat/B-14-eating-progress-hud | hunger 遗留 2；2026-08-26 认领。控制会话裁决：三点位最小受控重叠——`cmd/mornlea/app.go` 仅追加 eating overlay 与本地进度计数的字段声明行、`app_frame.go` 仅 `Prepare` 实参处构造并传入 eating overlay 的行、`app_lifecycle.go` 仅复位行（B-05 先例同形），其余 app 文件内容不触碰。独占文件集：`internal/render/hud/layout.go`、`renderer.go`（eating bar 呈现与 overlay 参数）及同包测试、OpenSpec change 目录；刻意不触碰 `hud/container.go`（A-01）、`capture_scene*.go`（E-12）、`internal/audio` 与音频装配（进食完成 cue 已交付，列为非目标）、`combat.go`/`hunger.go`（B-13）、`engine_step.go`/`drop.go`（A-04）；无 wire 变更（进度为客户端预测，不升协议）、不动 golden（无既有场景进食） |；2026-08-26 完成：PR #89 已合并（CI 8/8 全绿 · merge bb65a28e，含与 B-13/D-01/E-04/C-01 的 main 合流解冲突），change 归档为 archive/2026-08-26-eating-progress-hud 并 sync 进 survival-hud-presentation 主规格（饥饿已满门控随评审 NIT-4 升级交付）
| B-15 | 伙伴饥饿与自动进食 | 伙伴接三层状态 + 疲劳表 + 自动进食计划步骤 | `companions.ai` schema 可能升版 | 设计候选 | — | hunger 遗留 5；依赖伙伴能力扩展组 |
| B-16 | 横向原木与薄雪层 | 方向/高度/碰撞/选取/协议/存档状态编码（现为全方块方向固定） | 区块 schema 升版（方向状态编码） | 设计候选 | — | common-block-materials 延期项 |
| B-17 | 门 | 方块 + 交互开合，原创模型 | 方块编号追加 + 开合状态编码 | 已完成 | opencode-implementer @ feat/B-17-door | 批次设计非目标；B-17→B-30→B-11→B-24→B-23→B-35→B-36→B-37 串行链队首；2026-08-27 控制会话晋升排队→已认领→已完成：双格高 9 ID（`DoorLower 62..69`+`DoorUpper 70`，`BlockIDMax71`/`ItemDoor43/44`/`RecipeDoor14`），原子放置/开合联动/破坏双清，关闭 3/16 贴边/开启 90°旋转，`DoorUpper` 零碰撞可通过，`fluid`/`raycast` 按开合分流，`LayerDoor55`/`nativeMax71` 双侧同步；独占文件集：`internal/core`、`internal/assets`、`internal/sim`、`internal/physics`、`internal/mesh`/`engine`、`openspec/changes/authoritative-door`；2026-08-27 归档为 `openspec/changes/archive/2026-08-27-authoritative-door/` 并 sync 进 `door` 主规格（`openspec validate --all --strict` 68 passed），`go test ./... -short` 全绿，协议 v27/schema v9/ABI 不变；PR #102 已合并（`3ef68e5e`，含 `gofmt`+`I-1/I-2` 修复，CI 8/8 全绿） |
| B-18 | 完整生物群系 | 多生物群系地形/植被/材料分布 | worldgen 与材料表变更（大） | 设计候选 | — | 批次设计非目标（大） |
| B-19 | 红石 | 方块更新调度预留接口的直接挂载（水流/沙落/草蔓延/作物生长同机制） | 契约待设计（大） | 设计候选 | — | `2026-07-26-minecraft-go-design.md` §4.6（大，方向性） |
| B-20 | 多维度世界 | 世界/区块/存档/协议多维度化 | 全 schema/协议升版（大，待设计） | 设计候选 | — | 同上 §4.6 预留（方向性） |
| B-21 | 床跨区块放置 | 通用跨区块原子写入事务；出现第二个消费者时才设计 | 无 wire；存储事务 | 设计候选 | — | 批次设计「已知简化与升级条件」 |
| B-22 | 数据驱动方块模型 | 有限 model tag 无法清晰表达新形态时升级 | mesh registry/有限 model tag 扩展，engine ABI 可能升版 | 设计候选 | — | 同上 |
| B-23 | 远程敌怪与投射物 | 投射物协议与命中模型 | 协议新消息 → 升版 | 排队 | — | 同上；依赖 B-24 |
| B-24 | 护甲 | 伤害减免与防护槽位 | 玩家 schema（护甲槽）+ 协议 | 排队 | — | 同上；依赖 B-11 |
| B-25 | 有界状态效果系统 | 腐肉中毒等；出现第二个持续状态消费者时建设 | `PlayerState`/玩家 schema 扩展 | 设计候选 | — | 同上 |
| B-26 | 第二类敌怪 → AI/ECS 共享边界 | 出现行为显著不同的 mob 后评估共享抽象 | 待评估 | 设计候选 | — | 同上；B-27 被动生物落地后一并评估 |
| B-27 | 被动生物（家畜类） | 一种被动生物、原肉、熟肉和一条熔炉配方；出生、上限与持久化按夜行者先例 | 实体与掉落编号；持久化类比 `hostile_mobs`（可能新 schema） | 排队 | — | hunger 遗留 1（「肉类需生物」）；依赖 B-33；范围仅含一种被动生物、原肉、熟肉和一条熔炉配方 |
| B-28 | 岩浆与造石 | 岩浆流体（发光、接触伤害/灼烧）、水×岩浆→石头/黑曜石 | 流体编号区间扩展 + 方块光/伤害规则；无新协议消息 | 设计候选 | — | authoritative-fluid 与 fluid-presentation proposal 非目标（两处显式延期）；灼烧语义与 A-04 协调 |
| B-29 | 水流推力 | 流动水对实体施加方向力与水面流向呈现 | 物理侧 tunable 追加；无 wire 变更 | 设计候选 | — | 同上非目标（「水流对实体的推力与水流方向动画」）；衔接既有浸没物理 |
| B-30 | 冲刺（疾跑） | 移动输入位与速度提升 | `PlayerInput` 追加输入位 → 协议升版 | 已完成 | opencode-implementer @ feat/B-30-sprint | hunger 遗留 6（「动作不存在」）；B-13 冲刺疲劳依赖本行；依赖 B-17 已满足（B-17 已完成 2026-08-27，串行链队首晋升）；2026-08-27 控制会话经 brainstorming（bounded，方案 A）晋升排队→已认领→已完成：`PlayerInput.Sprinting`（`Eating` 后尾部追加）+`ProtocolVersion 27→28`，门控 `Sprinting&&MoveZ>0&&OnGround&&!BodyInFluid&&Hunger>=6` 双层校验（sim 饥饿门控+physics 前移/地面/浸没），1.3x（`SprintSpeedMultiplier=1.3`）水平加速，疲劳表新增 `exhaustionSprintMilli=80` 按阈值 4000 循环扣饱和度→饥饿，无 FOV/HUD/音效；独占文件集：`internal/network`、`internal/physics`+`engine/crates/mornlea_engine/src/step.rs`（`StepInput` v3 129/148..152）、`internal/sim`、`internal/client`+`cmd/mornlea`、`internal/config`、`openspec/changes/sprint`；2026-08-27 归档为 `openspec/changes/archive/2026-08-27-sprint/` 并 sync 进 `sprint` 主规格（`openspec validate --all --strict` 69 passed），`go test ./... -short` 全绿，CI 8/8 全绿；PR #103 已合并（`d3a441ee`，含 `gofmt`+版本冻结修复） |
| B-31 | 开箱中断进食 | 打开容器或视野未就绪时中断进食且不扣料 | 无契约变更（`advanceEating` 中断条件） | 已完成 | zcode2-implementer @ feat/B-31-eating-container-interrupt | hunger 遗留 10；需补「开箱中断不扣料」Scenario；2026-08-25 认领。控制会话裁决：批准与 A-01 在 `internal/sim/player.go` 的最小受控重叠——仅限 `advanceEating` 调用点为传入 `viewContainer`/`hasView` 中断所需的最小参数改动，该文件其余内容不触碰。独占文件集：`internal/sim/eating.go`、同包新增测试文件、OpenSpec change 目录；刻意不触碰 `combat.go`/`hunger.go`（B-13）、`engine_step.go`/`drop.go`（A-04）与 `internal/core` 编号段（A-01/A-02/A-04） |；2026-08-26 完成：PR #83 已合并（CI 8/8 首跑全绿 · merge e1c725e8），change 归档为 archive/2026-08-26-eating-container-interrupt 并 sync 进 authoritative-hunger 主规格，中断清单第五条与「中断优先于同 tick 结算」契约落地
| B-32 | 流体音效 cue | 涉水/流水音频，复用既有 cue 纪律（权威确认边界触发） | 无 wire 变更（客户端音频） | 已完成 | ox-alpha-implementer @ feat/B-32-fluid-audio-cue | fluid-presentation proposal 非目标（不做流体音效）。2026-08-26 本会话认领，最小范围：入水边沿 splash cue（`BodyInFluid` 上升沿在权威镜像位置上判定，复用 `physics.SubmersionFlags` 与既有 `cueSpec` 方波合成器）；水下环境音、出水音与 DSP 均不做。独占文件集：`internal/audio/cue.go`、`cmd/mornlea/app_audio.go`、`cmd/mornlea/app_messages.go`（单点接线）、同目录测试文件与 OpenSpec change 目录 `fluid-audio-cue`；刻意不触碰 `internal/audio/player_darwin.go`、`internal/physics/` 与协议/schema/golden PR #92 以 merge `aa1dd0ff` 合入，run `32938339681` 的 8 项全绿；SPEC/QUALITY 双评审与整分支终审通过（终审一项注释笔误经修复波清偿）。 |
| B-33 | 树苗与橡树再生 | 一种树苗掉落、种植与有界确定性生长，让木材可再生 | 方块/物品编号追加；worldgen/随机 tick 规则扩展 | 排队 | — | `2026-08-26-basic-gameplay-backlog-replan-design.md` §7；依赖 B-02；不建设通用植被系统 |
| B-34 | 生存初始背包 | 在关键资源已有自然取得路径后移除十四组整栈材料包，并锁定新世界可达性 | 玩家首次初始化语义；无 wire 结构变更 | 排队 | — | 同上 §7；依赖 B-27；必须证明工作台、照明、工具、食物均可从空背包取得 |
| B-35 | 完整分堆与快捷搬运 | 在 A-01 最小合成拆分之外补齐容器半组/单件与快捷搬运 | 协议命令与容器 UI 扩展 | 排队 | — | 同上 §8；依赖 B-23；不做拖拽铺放或自动整理 |
| B-36 | 斧与铲 | 木/石/铁斧铲的配方、采掘速度、收获等级与耐久 | 物品/配方与采掘规则追加 | 排队 | — | 同上 §8；依赖 B-35；不建设通用工具组件系统 |
| B-37 | 基础洞穴生成 | 单一确定性洞穴 carve 竖切，暴露地下矿物并保持旧区块不改写 | worldgen 与 engine ABI 可能升版 | 排队 | — | 同上 §8；依赖 B-36；不含结构、生物群系装饰或地下城 |

## C. 伙伴能力扩展（后续功能候选）

> 来源：`docs/superpowers/specs/2026-08-13-ai-native-companions-design.md` §16「非目标与后续方向」——这些能力只有在真实玩家体验证明需要后，才分别进入新的 OpenSpec change。另有 farming 遗留 11 的伙伴农业语义待裁决。

| ID | 功能 | 简述 | 版本与契约影响 | 状态 | 认领人 | 来源与备注 |
|---|---|---|---|---|---|---|
| C-01 | 伙伴采掘容器/多掉落方块 | `mine` 从「单一 `BlockDrop` 且非容器」扩展到容器与多掉落，需先定原子容量语义 | 视实现（`mine` 语义扩展，可能无 wire 变更） | 已完成 | zcode4-implementer @ feat/C-01-companion-mine-containers | §16 + 伙伴 mine 首版明确留给后续单独设计；2026-08-25 认领、2026-08-26 完成待合入。范围冻结：仅放开容器（箱子/熔炉）与多掉落方块作为伙伴 `mine` 目标；农业十编号的显式拒绝保持不变（C-11 另行裁决）；原子容量语义经 brainstorming 裁决为全或无（方案 A）。交付：`companion-mine-containers` 已归档（REMOVED+ADDED 重写 `companion-world-actions` 主规格条目），批量全或无结算 + sim/Runner 共享 `CompanionMineContainerStaging` + Memory/TCP parity；整分支终审 PASS、`gates.sh` 六项全绿（首轮聚合 race 失败取证为并行会话负载 flake）。独占文件集（实际触碰）：`internal/sim/mining.go` 与 `companion_mining_container_test.go`、`internal/companion/plan_types.go`/`planner.go` 及测试、`internal/server/companion_interact.go` 与 `companion_interact_container_test.go`/`companion_interact_parity_test.go` + OpenSpec change 目录 |
| C-02 | 伙伴自动拾取 | 世界掉落物入伙伴背包，须先验证 grid/背包装回不变量 | 视实现（背包不变量） | 设计候选 | — | §16 |
| C-03 | 伙伴背包整理/合成/熔炼/开容器 | 自动合成与熔炼需服务端权威语义 | 视实现（`companions.ai` 可能升版） | 设计候选 | — | §16 |
| C-04 | 伙伴自动挖障碍/搭桥/游泳/无限世界寻路 | 寻路与计划能力扩展 | 计划步骤扩展 → `companions.ai` schema 升版 | 设计候选 | — | §16 |
| C-05 | 伙伴自主目标/自动采集/自动建造 | 无指令自主行为 | 契约待设计（大） | 设计候选 | — | §16（大） |
| C-06 | 玩家创建伙伴与所有权 | 创建 UI、所有权、ACL（计费不在范围内） | 配置与 schema 待设计 | 设计候选 | — | §16 |
| C-07 | 完整聊天历史/RAG/长期人格演化 | 存储与检索（现仅 ≤2 KiB 近期摘要） | `companions.ai` schema 升版 | 设计候选 | — | §16 |
| C-08 | 伙伴空闲随机聊天 | 非任务触发台词 | 复用既有 `CompanionSpeech`，无 wire 变更 | 已完成 | zcode-implementer @ feat/C-08-companion-idle-dialogue | §16；2026-08-27 控制会话批准最小闭环：只有最近任务发令者仍在线且位于伙伴水平 16 格内时，完全空闲的伙伴才按每伙伴确定性的 60–120 秒间隔触发 Dialogue；复用既有模型并发槽、单在途纪律与 `CompanionSpeech` 广播，不改任务/FIFO/摘要/持久化、协议/schema/ABI/scenario 或 golden。2026-08-27 认领并当日完成：PR #97 已合并（CI 8/8 首跑全绿），change `companion-idle-dialogue` 已归档并 sync 进 `companion-dialogue` 主规格。交付零载荷 `DialogueNodeIdle` 节点（HTTP kind `"idle"`）、FNV-1a(`ID‖LE(seed)`) 确定性 1200..2400 tick 期限派发（`restored` 合成身份排除、复用四槽与单在途、不占任务八次 Dialogue 预算）、结果时空队列/同一真实 issuer/在线且水平 16 格重验与 `CompanionSpeech` 广播（摘要不变、模型失败不补发不重排期），Memory/TCP parity 投影一致。完整验证与 SPEC/QUALITY 双评审证据见归档 ledger。 |
| C-09 | 伙伴主动修改世界/动态任务/世界事件 | 世界事件系统 | 契约待设计（大） | 设计候选 | — | §16 |
| C-10 | 多 Agent 协商/模型代码与工具执行 | 伙伴间自主协商、模型代码执行 | 契约待设计（大） | 设计候选 | — | §16（大） |
| C-11 | 伙伴参与农业 | 种什么/何时收/成熟度判断语义未裁决；放开三处防御清单前必须先决定语义 | 视裁决（放开三处防御清单） | 设计候选 | — | farming 遗留 11（先裁决再开工） |

## D. 客户端 UI/体验（后续功能候选）

> egui 工具型 UI 选型见 `docs/superpowers/specs/2026-08-23-egui-tool-ui-selection-design.md`；「后续每个具体菜单（设置、暂停、调试面板等）各自独立 change，逐个归档进主规格」。

| ID | 功能 | 简述 | 版本与契约影响 | 状态 | 认领人 | 来源与备注 |
|---|---|---|---|---|---|---|
| D-01 | 设置菜单 | 音量（`audioVolume`）、材质包目录（`texturePackPath`）、窗口等设置项 | 配置项暴露，无 wire 变更 | 已完成 | codex-implementer @ codex/D-01-settings-menu | PR #86；change `settings-menu` 已归档并同步主规格。交付世界装配前的 committed/draft 设置页、三字段 raw JSON 原子 patch、材质候选预验证、音量/窗口保存后即时生效、client ABI v9 结构化有界事件批及 19 场景 capture；协议/schema、engine ABI、benchmark scenario 与配置版本均不变。完整验证与 baseline-equivalent 视觉门禁证据见归档 ledger。 |
| D-02 | 暂停菜单 | 单机暂停最小闭环：Esc 开合暂停覆盖层、权威 tick 冻结、退回主菜单 | 无 wire 变更（权威暂停门 + 呈现层） | 已完成 | zcode-implementer @ feat/D-02-pause-menu | egui 实施路径；2026-08-27 控制会话晋升排队→就绪并经用户批准 bounded 边界后认领：`internal/server` 加原子暂停门（`Pause()`/`Resume()`，`RunTicks` 调度前检查），TCP 远程会话不宣称暂停（服务端照常 tick，菜单注明）；菜单条目仅「返回游戏 / 退回主菜单」，设置入口列为顺延项。2026-08-27 当日完成（SDD 全链，三任务组 fresh implementer + SPEC/QUALITY 双评审 PASS，整分支终审曾 FAIL 于跨层 winit Esc 回声缺陷→删 Rust 侧合成修复→复审 PASS）：`internal/server` 原子暂停门 + `Host` 直通唯一暴露面、egui 暂停页 layout v4（动作 8/9 与 Go 双向钉值）、`menuPhasePaused` 相位机与 Esc 栈五档、退回主菜单 `resetSessionOwnedState` 拆链防二次泄漏、-connect 防御性拒绝；版本矩阵全不变、golden 零变化（`make visual-check` 失败经 merge-base 对照归因预存陈旧基线，移交清单见归档 ledger）；归档为 `openspec/changes/archive/2026-08-27-pause-menu/` 并 sync 进 `egui-tool-ui` 主规格（+3 Requirements，validate 67 passed）；以 PR #104 merge `9249ff12` 合入，CI run `33086007726` 8 项首跑全绿 |
| D-03 | 调试面板 egui 化 | 既有程序化 debug 面板迁入 egui 或并存；性能影响需评审 | 无 wire（呈现层） | 已完成 | opencode-implementer @ feat/D-03-debug-panel-egui | egui 实施路径；2026-08-26 曾由 `opencode-implementer @ feat/D-03-debug-panel-egui` 认领，因原 A-07 client ABI v10 预占冲突退回；A-07 取消后于 2026-08-27 经控制会话授权重新设计（完全迁移删旧程序化渲染路径、保留行选中+值编辑+F3 全捕获）并重新认领。2026-08-27 完成：PR #98 已合并（CI 8/8 全绿 · merge `8ea0aa0d`），change 归档为 archive/2026-08-27-egui-debug-panel 并 sync 进 egui-tool-ui + debug-panel 主规格；`debug-panel.png` golden 再生（用户人工确认 egui 14pt 半透明可滚动面板）。实现：Rust `UI_DEBUG_LAYOUT_VERSION=3` 解码+egui 面板控件（159 测试）、Go v3 段编码+事件回传、旧程序化路径删除 |
| D-04 | 合成面板分页/滚动 | 配方行数增长后按窗口高度自适应（当前 10 行，矮窗口整体缩小） | 无 wire；HUD 布局（capture 校验） | 已取消 | — | farming 遗留 20；原固定配方行分页/滚动需求由 A-01 的 2×2/3×3 权威格子合成界面取代；若该替代前提不成立，D-04 必须恢复为 `排队` |
| D-05 | HUD 物品图集 UV 对齐 | 按 texel 中心/整数像素计算，使图集扩列不影响既有图标 | 无 wire（UV 计算） | 已完成 | ox-alpha-implementer @ fix/D-05-hud-atlas-texel-uv | farming 遗留 18；PR #82 已合并（CI 8/8 全绿），change `hud-atlas-texel-stable-uv` 已归档并 sync 进 `survival-hud-presentation` 主规格；最终方案为对称亚纹素收进 1/256 纹素（半纹素中心对齐被否决），两景心形区 golden 经用户裁决外科手术式再生 |
| D-06 | 材质包 v2 | HUD 图集覆盖（鸡腿/爱心/气泡可替换）、构建期许可校验与成果打包 | 配置语义扩展 + 构建期校验 | 设计候选 | — | hunger 遗留 7；farming 遗留 12（贴图管线） |
| D-07 | 耕地 mesh 顶面下沉 | 按 material 固定下移顶面，或复用水面角高度位 | capture golden 更新 | 已完成 | ox-alpha-implementer @ feat/D-07-farmland-mesh-top-sink | farming 遗留 13；2026-08-26 认领并当日完成：PR #91 已合并（CI 8/8 首跑全绿），change `farmland-mesh-top-sink` 已归档；registry +1 字节 `block_top_raw`（engine ABI v7）、terrain.wgsl 客户端解码半边、showcase 扩两列并单景再生 golden；执行期发现并补齐客户端解码缺口（design D2a），与演进后 main 变基集成零版本碰撞 |
| D-08 | 农田+花草 capture 场景 | 植物种类增多后补视觉场景并记录差异来源 | capture 场景与 golden 追加 | 设计候选 | — | farming 遗留 24 |
| D-09 | 第三人称与角色姿态呈现 | 第三人称相机模式与角色姿态（游泳等） | client ABI 可能扩展（相机/姿态） | 设计候选 | — | fluid-presentation proposal 非目标（不做游泳姿态动画与第三人称呈现）；avatar pass 为基础 |

## E. 工程与基础设施（后续任务候选）

> 多数行是迁移/重构/稳定性任务，仍需各自独立 OpenSpec change；只有标「直接修改豁免」的小型修复可直接改。

| ID | 任务 | 简述 | 版本与契约影响 | 状态 | 认领人 | 来源与备注 |
|---|---|---|---|---|---|---|
| E-01 | Rust 阶段 3：状态与规则分离 | Rust 接管 world/player 可变状态与 deterministic step executor；Go 规则收敛为 `EventBatch → CommandBatch`；须先设计事件身份、命令校验与回放语义 | 事件/命令契约待设计（大） | 设计候选 | — | `2026-08-12-rust-engine-go-rules-design.md` §6（大，方向性） |
| E-02 | Rust 阶段 4：网络、存储与服务端运行时 | Rust 接管 transport、wire codec、存档与调度，保持协议/schema/fixture/故障注入/parity | transport 契约保持（大） | 设计候选 | — | 同上 §6（大） |
| E-03 | Rust 阶段 5：客户端、渲染与入口反转 | Rust 接管 client application、渲染与主循环，静态链接 Go 规则库，删除剩余 Go 引擎 | 入口反转（大） | 设计候选 | — | 同上 §6（大） |
| E-04 | 删除旧 Go oracle | 碰撞/射线/物理/世界生成等迁移后的 test-only oracle，以独立 change 删除 | 无（test-only 删除） | 已完成 | ox-alpha-implementer @ chore/E-04-drop-go-oracles | rust-engine-go-rules §15「oracle 删除条款」；2026-08-25 认领。范围冻结为三切片：physics 步进/碰撞差分（`step_native_test.go`/`collision_native_test.go` 及仅剩这两者消费的 helper 与旧实现体）、core 射线 DDA 差分与 fuzz 基准（`raycast_native_test.go`/`raycast_fuzz_test.go`/`raycast_helpers_test.go`）、worldgen 生成差分（`oracle_test.go`/`parity_test.go` 及 `generator.go` 内失去消费者的 test-only 旧噪声/地形实现）；`internal/mesh` 的 greedy/light oracle 切片因 A-02 独占 `internal/mesh` 且正改 Rust mesher 而延迟，待 A 批次合流后另行认领。2026-08-25 认领并当日完成：PR #87 已合并（CI 8/8 首跑全绿），`drop-go-test-oracles` 已归档；三切片净删约 1.2k 行，physics 位级 golden 向量与 raycast 几何不变量补位；mesh 切片延迟待 A 批次合流 |
| E-05 | 世界 goroutine 区域分片 | 吞吐瓶颈缓解（单核 world goroutine 是硬上限）；严格限界后按维度/区域分片 | 并发结构重构，无 wire | 设计候选 | — | `2026-07-26-minecraft-go-design.md` §8.1（方向性） |
| E-06 | 图形客户端平台扩展 | Windows/Linux 客户端构建与验收（当前 engine 只承诺 Apple Silicon/macOS 正式验收） | 平台构建矩阵 | 设计候选 | — | rust-engine-go-rules §16 平台链接差异 |
| E-07 | 存档 Flush 恒脏自旋修复 | `playerPersistence.Flush` 去重键去掉 revision 或「连续 N 次重派无进展即放弃」 | 无（持久化循环修复） | 已完成 | claude-implementer @ fix/E-07-flush-stall-guard | hunger 遗留 9；独占文件集：`internal/server/player_flush.go` 及 `internal/server/` 相关测试 + OpenSpec change `fix-player-flush-stall` |
| E-08 | `HighestOpaque` 语义改名/钉死 | 返回最高非空气方块，名不副实；跨包改名或 GoDoc 首句钉死语义 | 无（跨包改名或 GoDoc） | 已完成 | codex-implementer @ chore/E-08-highest-opaque-semantics | 完成证据：GoDoc 自 `7a7253e7` 已钉死“最高非空气”；既有 world/crop 测试覆盖；零代码变更 |
| E-09 | 作物×锄头耐久豁免 | 对齐 MC：手持锄头收获不扣耐久，`completeMining` 加豁免表并配测试 | 无（豁免表 + 测试） | 已完成 | codex2-implementer @ fix/E-09-hoe-harvest-durability | farming 遗留 16；`fix-hoe-harvest-durability` 已归档；SPEC/QUALITY 与整分支终审通过，`scripts/agents/gates.sh` 全绿 |
| E-10 | `findSpawnInColumn` 读落脚盒顶面 | 出生点与 support/safe 存档点三处口径同步（耕地 1/16 空隙） | 无（出生点口径） | 已完成 | codex-implementer @ fix/E-10-spawn-support-top | farming 遗留 14；`fix-spawn-support-top` 已归档；真实耕地出生/恢复/safe 测试、SPEC/QUALITY 双评审与整分支 `gates.sh` 均通过 |
| E-11 | server 测试等待预算化 | 既有登录等待循环多数无界（5 分钟超时而非可读断言），统一有预算等待助手 | 无（测试基础设施） | 已完成 | codex-implementer @ codex/fix-E-11-server-test-wait-budget | farming 遗留 19（测试基础设施）；归档 change：`2026-08-25-server-test-wait-budget`；Task 1 R1 后 SPEC/QUALITY 双评审 PASS，整分支终审 0 findings，fresh gates 全绿 |
| E-12 | M5E 再递延字面同源化 | `capture_scene.go` 与 `capture_ai_companion_test.go` 的 `[32]network.ChatEvent` 字面；ChatCommand 编解码 1024 字面与错误文案 | 无（字面同源化） | 已完成 | zcode-implementer @ chore/E-12-literal-single-source（2026-08-27 接手续做） | m5e 归档 proposal「延期与放弃」递延 4、5；独占文件集：`cmd/mornlea/capture_scene.go`、`cmd/mornlea/capture_ai_companion_test.go`、`internal/network/codec_client.go` 及相关定点测试 + OpenSpec change `m5e-literal-single-source`；2026-08-25 原认领（chore/E-12-companion-limits）无推送记录，随机器迁移重置重认领；2026-08-25 本行由 claude-implementer 认领，分支 chore/E-12-literal-single-source；worktree 有未提交实现，允许原认领者收尾；2026-08-27 控制会话裁决转移给 zcode-implementer 续做：原认领者进程已不存在且 worktree 自 2026-08-25 晚起长期零活动（机器迁移失联先例），沿既有分支与已批准设计（ledger 已录飞书 approve）执行，范围与独占文件集不变；当日完成：fresh implementer 收敛遗留实现并以 `1401c7a2` 落位 L1/L2，SPEC/QUALITY 双评审与整分支终审 PASS（两条 NIT 顺延入 proposal「延期与放弃」），合流 main 后串行 fresh gates.sh 六项与 `make rust-check` 全绿，skip_specs 零主规格变化，change 已归档，协议 v27 与其余版本矩阵及视觉 golden 均不变；以 PR #99 merge `6add2597` 合入，CI run `33045444253` 8 项首跑全绿 |
| E-13 | benchmark 新 scenario record-only 报告 | 为已升版 scenario 补 Memory/TCP 记录报告并追加到 `perf-baseline.md`（数值只记录） | 无（报告只记录） | 已完成 | codex-implementer @ chore/E-13-benchmark-record-report | farming 遗留 23；`E-13-approval2` 批准 bounded 短设计；冻结 `923fa0d7` 生成并自检 Memory/TCP v19 报告及跨 transport 比较，SPEC/QUALITY PASS，`gates.sh` 全绿；基线 JSON 未覆盖 |
| E-14 | rust-engine 主规格 oracle 措辞卫生 | E-04 删除 Go oracle 后 `rust-engine-{physics-step,collision-raycast,worldgen}` 三份主规格仍以「与 Go oracle 逐位一致」作验收措辞，与替代门禁（位级 golden 向量／几何不变量+确定性孪生／黑盒双出口对照）失真；MODIFIED delta 更正为现行门禁描述 | 无（docs-only 主规格措辞） | 已完成 | ox-alpha-implementer @ feat/E-14-spec-oracle-wording | drop-go-test-oracles proposal「延期与放弃」递延②；2026-08-26 认领并当日完成：PR #93 已合并（CI 8/8 首跑全绿），change 归档为 archive/2026-08-26-spec-oracle-wording-hygiene 并经 delta 应用 + design D2 五行直编落进三份主规格（openspec 1.7.0 对已存在主规格忽略 Purpose、MODIFIED 不支持 Scenario 改名，控制会话裁决归档阶段直编）；SPEC/QUALITY 双评审 PASS 零 blocking、R1 清偿三条 nit；`rust-engine-mesh` 两处措辞随实存 oracle 保留待 A 批次 drop-go-test-oracles proposal「延期与放弃」递延②；2026-08-26 认领。`rust-engine-mesh` 两处措辞在 mesh oracle 实存期间保持原样，待 A 批次合流后随 mesh 切片更正。独占文件集：OpenSpec change 目录 `spec-oracle-wording-hygiene` 与 `openspec/specs/rust-engine-physics-step/spec.md`、`rust-engine-collision-raycast/spec.md`、`rust-engine-worldgen/spec.md` 三份主规格文本；不触碰任何代码、测试、golden、其它规格与 capture 场景 |

## F. 小型修复队列（直接修改豁免，但同样认领登记）

> 不需要 OpenSpec change，但完成前仍须相称验证（受影响包测试）。

| ID | 修复 | 简述 | 版本与契约影响 | 状态 | 认领人 | 来源与备注 |
|---|---|---|---|---|---|---|
| F-01 | 拒绝原因白名单补登记 | `client/mirror.go` 补登记 `RejectContainerCapacity`，服务端发该原因时客户端不再报 unknown | 无 wire 变更 | 已完成 | claude-implementer @ fix/F-01-reject-container-capacity | farming 遗留 15；PR #73 已合并（CI 8/8 全绿） |
| F-02 | perfcheck 文案修正 | `cmd/perfcheck/migration_test.go` 的 v15 比较测试失败信息误写为 v16 | 无（一处字符串） | 已完成 | claude-implementer @ fix/F-02-perfcheck-v15-message | PR #72 已合并（CI 8/8 全绿） |
| F-03 | 「使用」键放置判定收敛 | 客户端按 `core.ItemPlacement` 决定是否发 `PlaceBlock`，不可放置物一律不发 | 无（客户端判定；钉住现状的 `TestUseKeyRisingEdgeSkipsPlaceWhileHoldingFood` 需同步调整） | 已完成 | claude-implementer @ fix/F-03-use-key-placement | PR #74 已合并（CI 首跑 1 flake 重跑后 8/8 全绿） |
| F-04 | LAN 专用服务端事实同步 | 校正 `docs/notes/lan-server.md` 中过时的启动方式、玩法能力、伙伴行为、版本矩阵与安全边界 | 无（docs-only） | 已认领 | opencode-implementer @ docs/F-04-lan-server-facts | 独占文件集：`docs/notes/lan-server.md`；依据代码、测试、`openspec/specs/`、`README.md` 与 `docs/notes/progress.md`，不改实现、协议、存档、ABI、benchmark 或 golden |
| F-05 | 视觉基线再生 | `make visual-check` 预存失败的 10 场景 golden 再生（含从未入库的 `workbench-crafting.png`） | 无代码变更（`cmd/mornlea/testdata/golden/*.png` 再生） | 已认领 | zcode-implementer @ fix/F-05-visual-golden-refresh | D-02 收尾移交（archive/2026-08-27-pause-menu ledger「4 收尾门禁」行）；2026-08-27 用户指令修复。诊断已定：失败集在 merge-base 与当前 main 逐项一致，纯预存陈旧；两次抓取实际图逐字节一致（确定性成立）；逐图确认——inventory/chest/furnace 为 A-01 后容器与合成 UI 预期演进、workbench 为补齐新基线（3×3 工作台界面正确）、terrain-noon/oak-grove/far-horizon/avatar-nametag/ai-companion/water-underwater 为 D-07/B-01 累积的世界渲染与文字边缘亚感知像素漂移。独占文件集：`cmd/mornlea/testdata/golden/*.png`；不碰 capture 场景表与任何代码（A-02 在途独占 `capture_scene*`，其 torch-night 新 golden 不受影响） |
