# Mornlea 功能规划与认领表

本表是 Mornlea 后续开发任务的**单一规划来源**：每一行 = 一个可独立认领、独立评审、独立验收的功能或工程任务，通常对应一条 OpenSpec change。控制会话与 implementer 子代理在开始任何开发前先读本表；认领后立即在行内打标记并提交，防止多个 agent 抢同一任务。本表只描述「要做什么、谁在做、依赖什么」，**不排时间表**。

> 开发流程、工程约定与验证门禁以 `AGENTS.md`（与 `CLAUDE.md` 逐字节相同）与 `openspec/config.yaml` 为准。本表不重复、不替代二者；二者与代码矛盾时，以代码、测试与 `openspec/specs/` 主规格为真相。
>
> 状态列表镜像（按状态分组、每轮刷新）见 GitHub Discussion [#71 Mornlea 功能缺口拆解与任务认领池（规划）](https://github.com/channing771/mornlea/discussions/71)；两处状态不一致时以本文件为准并回改讨论。

## 状态图例

| 状态 | 含义 | 谁可继续 |
|---|---|---|
| 未认领 | 等待认领 | 任意 agent 可认领 |
| 已认领 | 已被某 agent 认领（可含「已开工」信息） | 只有认领人 |
| 开发中 | 已认领且正在派发实现 | 只有认领人 |
| 待集成 | 功能分支实现与双评审完成，等待批次集成 | 批次集成任务认领人 |
| 已完成 | 已合入 `main` 并归档 | 无人（履历保留在认领人列） |

## 认领方式

1. 读取本表、`openspec/config.yaml` 与 `AGENTS.md`「开始工作前」清单。
2. 选择一行**未认领**且其依赖行已满足的行；同一时间只认领一行。
3. 编辑该行：`状态` → `已认领`，`认领人` → `<agent 标识> @ <分支名>`，并在备注写明独占文件集（如与其它已认领行冲突则换行）。提交（docs-only，不关联 OpenSpec change）。
4. 按 `AGENTS.md` 的 isolation worktree 习惯创建分支（`using-git-worktrees` 技能）。
5. 按下方「开发流程」执行；合入 `main` 并归档后，把该行 `状态` → `已完成`（认领人保留履历）。
6. 已认领行不得抢；转移必须经控制会话裁决并留档。

## 开发流程

完整流程（认领 → OpenSpec change → subagent-driven-development 执行 → SPEC/QUALITY 双评审 → 门禁 → 归档收尾 → 并行冲突规则）在**唯一**的说明文档 [`docs/development-process.md`](development-process.md)；本表与 GitHub Discussion #71 只引用它，不再内嵌重复。

日常执行者：每日固定时间扩展规划的**规划者**、从本表认领并开发收尾的**实现者**——角色卡、调度与运行入口见 [`docs/agents/README.md`](agents/README.md)（`make agent-planner` / `make agent-implementer`）。收尾前可运行 `scripts/agents/gates.sh` 汇总标准门禁。

## 并行与冲突规则

- **同批并行必须共享契约**：参照第一夜批次先例——第一步在功能分支共同基线上冻结「追加编号 / 协议消息 / 有限模型 tag」的 append-only 共享契约提交，各功能分支从该 SHA 创建；`AGENTS.md`/`CLAUDE.md`、capture golden、benchmark scenario 与版本基线由**集成任务独占**，功能分支不得触碰（避免并行覆盖 PNG 与文档冲突）。
- **版本号互斥**：协议 / 存档 schema / engine ABI / client ABI / benchmark scenario 的升版行互斥——同一时间只能被一个认领者持有；冲突时按实际合入顺序重排（例：第一夜批次原设计写 client ABI v8，先被 egui 主菜单占用 v8、再被设置页占用 v9，须重排为 v10）。
- **文件所有权**：认领时在备注声明独占文件集；与本表其它已认领行重叠即换行或延迟。
- **范围冻结**：认领后不得扩大范围；实现发现规格不成立时，先改该 change 的 OpenSpec 产物再继续编码。

---

## A. 第一夜生存批次（在途，不得重复认领）

> 设计与计划见 `docs/superpowers/specs/2026-08-23-first-night-survival-parallel-wave-design.md` 与 `docs/superpowers/plans/2026-08-23-first-night-survival-parallel-wave.md`。原批次的五个功能分支仅存在于前一开发机的本地 worktree（`.worktrees/`）与本地 `codex/*` 分支（共享契约 SHA `785ea07b`），从未推送远端，已随机器迁移丢失；2026-08-25 经控制会话裁决将 A-01..A-05 全部重置为未认领、从当前 `main` 重做。按当前基线重排后的版本目标为：协议 v27、玩家 schema v8、世界 metadata v3、`hostile_mobs` schema v1、engine ABI v7、client ABI v10、benchmark scenario v20；区块 schema v9 与 `companions.ai` v4 不变。

| ID | 功能 | 简述 | 状态 | 认领人 | 来源与备注 |
|---|---|---|---|---|---|
| A-01 | 权威格子工作台 | 背包 2×2 + 工作台 3×3 形状配方、权威产物取出、关闭/断线装回不变量、七条新配方 | 已认领 | zcode-implementer @ feat/A-01-authoritative-grid-crafting | 原分支头 `a657c1cb`（实现与整功能评审完成）未推送远端已丢失；从当前 `main` 重做。独占文件集：`internal/core`（`recipe.go` 网格匹配与 recipe 1..18 形状、`inventory.go`、`item.go` 及测试）、`internal/assets/blocks.go`（工作台方块与纹理及测试）、`internal/sim`（新建 `crafting.go` 与 `player.go`/`command.go`/`engine_step.go`/`container.go`/`drop.go`/`death.go`/`persistence.go` 及测试）、`internal/network` crafting 消息（`message_inventory.go`/`codec_client.go`/`codec_server.go`/`packet.go` 及测试）、`internal/server`（`session_ingress.go`/`publication.go` 及 parity 测试）、`internal/client/inventory.go` 及镜像测试、`internal/render/hud/container.go` 及测试、`cmd/mornlea`（`app*.go` 合成 UI 与 `capture_scene*` 场景构造、不写 golden）及 OpenSpec change `authoritative-grid-crafting`；与 A-02 备注 `internal/core`/`internal/assets` 的边界按批次计划「crafting 独占 `recipe.go` 的 recipe 1..18 形状，torches 只追加方块/物品登记」裁决，`codec_client.go`/`capture_scene.go` 与 E-12、server parity 测试与 E-11 关注点不相交，合并序均按集成裁决；批次共享契约（编号/消息预留）归属待控制会话裁决 |
| A-02 | 可放置火把 | 落地 + 四向墙面五形态、支撑约束与移除反应、发光等级 14、mesh model tag 与窄柱几何 | 已认领 | ox-alpha-implementer @ feat/A-02-placeable-torches | 原分支头 `b2115a64`（实现与整功能评审完成）未推送远端已丢失；从当前 `main` 重做。独占文件集：`internal/core` 方块属性/物品/配方登记、`internal/assets`、`internal/sim` 火把放置与支撑、`internal/world/chunk.go`、`internal/mesh`、`internal/nativeabi`、`engine/crates/mornlea_engine`、`openspec/changes/placeable-torches`；`cmd/mornlea/capture_scene*` 仅追加 `torch-night` 场景构造不写 golden（与 E-12 已认领的字面同源化关注点不相交，合并序按集成裁决）；共享契约文件（`internal/network` 编号预留等）归属待确认裁决后在 change 中记录 |
| A-03 | 三级剑与统一战斗 | 木/石/铁剑（4/5/6 伤害）、统一候选（≤72）与冷却/击退/耐久、CombatHit 私有确认 | 未认领 | — | 原分支头 `42f3dead`（SPEC/QUALITY PASS）未推送远端已丢失；recipe 15..17 依赖 A-01 合流；从当前 `main` 重做 |
| A-04 | 权威近战夜行者 | 确定性夜间生成、全服 64/每玩家 8 上限、A* + Rust 物理、灼烧/消失/腐肉掉落、`hostile_mobs` v1 | 已认领 | claude-implementer @ feat/A-04-hostile-nightwalker | 原分支头 `eb1923eb`（持久化修复两轮已提交）未推送远端已丢失；从当前 `main` 重做。独占文件集：`openspec/changes/authoritative-hostile-nightwalker`（新建）；`internal/storage`（新建 `hostile_types.go`/`hostile_codec.go`/`hostile_codec_*_test.go`/`hostile_store_test.go`，修改 `types.go`/`memory.go`/`disk.go`/`world_files*`/`backup*`）；`internal/sim`（新建 `hostile*.go`/`block_light_query*.go`，修改 `engine.go`/`engine_step.go`/`command.go`/`drop.go`/`tunables.go`）；`internal/core`（`hunger.go` 及测试——腐肉食物值与 `DisplayDayPhase`；`ItemRottenFlesh` 条目与 A-02 火把条目同属 `item.go` append-only 不同段，编号终值按 A-06 固定合并序裁决）；`internal/server`（新建 `hostile_manager`/`hostile_snapshot`/`hostile_path_worker`/`hostile_publication`/`hostile_persistence` 及各自测试/`hostile_restore_test`/`host_shutdown_test`，修改 `server.go`/`host.go`/`shutdown.go`/`persistence_status.go`；与 E-11 既有 `*_test.go` 等待助手不相交）；`internal/network`（新建 `message_hostile*.go`，修改 `codec_server.go`/`packet.go`/`registry.go`；不含 E-12 的 `codec_client.go`）；`internal/client`（新建 `hostiles*.go`，修改 `window*.go`）；`internal/render/avatar*.go`；`cmd/mornlea`（`app.go`/`app_messages.go`/`app_render.go`/`presentation_conversion_test.go`，`capture_scene*` 仅追加 `hostile-mob` 场景构造不写 golden）；`engine/crates/mornlea_client`（`src/render/entity.rs`/`src/ffi.rs`/`src/lib.rs`，与 A-02 的 `mornlea_engine` 不相交）；不触碰协议/存档/engine ABI/client ABI/scenario 版本号（A-07 独占）、golden PNG 与 `AGENTS.md`/`CLAUDE.md`/`progress.md` 基线（A-07 独占）；攻击先经既有 `applyDamage` 通道并以专用 damage test seam 验证 3 伤害/20 冷却，统一 combat settlement 由 A-06 接通（批次计划 Task 4 Step 4） |
| A-05 | 床与睡眠 | 双格床八形态、同区块原子放置、全员睡眠跳夜（`DayTimeOffsetTicks`）、个人重生点 | 已认领 | codex-implementer @ feat/A-05-authoritative-bed-sleep | 原共享契约 SHA `785ea07b` 未推送远端已丢失（本无实现损失）；从当前 `main` 重做。独占文件集：OpenSpec change `authoritative-bed-sleep`；床配对/放置/破坏/碰撞、原创材质与独立 Rust bed emitter；metadata v3/player schema v8 的 offset 与床复活点；权威睡眠状态、`UseBed` 处理、客户端昼夜/睡眠 UI、`bed-sleep` 场景构造（不写 golden）；不触碰协议/ABI/scenario 最终版本号、golden PNG 与 `AGENTS.md`/`CLAUDE.md`/`progress.md`（A-07 独占），共享编号/消息/model tag 契约待批次控制会话裁决 |
| A-06 | 五路合流集成 | 固定顺序合并 crafting→torches→swords→nightwalker→bed；TDD 接通剑×夜行者统一战斗、夜行者阻睡、bed model dispatcher；删除 `network.CraftRecipe` 过渡类型 | 未认领 | — | 依赖 A-01..A-05；按批次计划 Task 3 |
| A-07 | 版本基线与视觉基线 | 协议 v27 / schema v8 / metadata v3 / `hostile_mobs` v1 / engine ABI v7 / client ABI v10 / benchmark v20 迁移 `19:20`；生成新 5 场景 golden；同步 `AGENTS.md`+`CLAUDE.md`+`progress.md` | 未认领 | — | 依赖 A-06。client ABI 按已占用的 v8 主菜单、v9 设置页重排为 v10；场景总数按当前 19 个加新增 5 个计算为 24，批次历史计划中的 22 未含 `main-menu`/`settings-menu` |
| A-08 | 整分支终审、归档与推送 | 独立整分支终审（规格、上限、并发/持久化错误路径、wire 安全、24 图、无版权资源）、五个 change 归档、合入 `main` | 未认领 | — | 依赖 A-07；按批次计划 Task 5 |

## B. 生存与世界深化（后续功能候选）

> 每行一条独立 OpenSpec change。来源列括号内是归档 change 的 `design.md`「遗留与简化清单」或 `proposal.md`「非目标」的条目，或批次设计「非目标 / 已知简化与升级条件」。「版本与契约影响」只写方向性结论（如「协议升版」「编号追加」），绝对版本号按合入时序在「并行与冲突规则」下重排。

| ID | 功能 | 简述 | 版本与契约影响 | 状态 | 认领人 | 来源与备注 |
|---|---|---|---|---|---|---|
| B-01 | 更多食物与作物 | 肉类生物掉落+熟食熔炉食谱、更多作物（编号+纹理+生长参数即可扩展） | 物品/方块编号与配方表追加，无 wire 结构变更 | 未认领 | — | hunger 遗留 1；farming 遗留 2；肉类依赖 B-27（被动生物）；2026-08-25 控制裁决：肉类与熟食熔炉食谱并入 B-27 随其落地交付；作物半边因与第一批次 A-02 独占文件集（`internal/assets`、`internal/mesh`、engine crate）重叠且 mesh registry 容量仅剩 3/48 条与批次编号契约耦合，按「换行或延迟」规则待第一批次合流后再认领 |
| B-02 | 水桶（可搬运流体） | 舀水/倒水物品，解除「农业只能在天然水体 4 格内」约束 | 物品编号追加；无限水源规则随本行一并裁决 | 未认领 | — | farming 遗留 25（显式非目标解除）；fluid proposal 非目标「两个源相邻生成新源」约定随水桶交付 |
| B-03 | 骨粉 | 新物品 + 「立即推进 N 阶段」动作，走翻地同形命令路径 | 物品编号追加；命令段可能追加 | 未认领 | — | farming 遗留 3 |
| B-04 | 草丛与除草掉种子 | 植物几何第二消费者；草丛掉落种子替代初始材料包供给 | 植物区间编号追加 | 未认领 | — | farming 遗留 6 |
| B-05 | 踩踏破坏耕地 | 实体落地事件接进方块变更（物理侧已有落地判定） | 无 wire 变更 | 已完成 | zcode3-implementer @ feat/B-05-trample-farmland | farming 遗留 4；2026-08-25 认领。控制会话裁决：三处最小受控重叠——`internal/sim/player.go` 仅限摔落/落地结算区插入踩踏判定调用、`internal/sim/crop.go` 仅限 `advanceCrops` 首部一处结算调用（B-10 已归档无在途独占）、`internal/sim/engine.go` 仅限 `Engine` 结构体追加一行暂存字段声明（append-only，详见 change `farmland-trample` ledger Ruling）。独占文件集：`internal/sim` 新建踩踏结算文件与同包新增测试、OpenSpec change 目录；踩踏触发的作物掉落只调用既有导出掉落 API；刻意不触碰 `combat.go`/`hunger.go`（B-13）、`engine_step.go`/`drop.go`/`tunables.go`（A-04）、`mining.go`（C-01）与 `internal/core` 编号段（A-01/A-02/A-04）；2026-08-25 当日完成：PR #85 已合并（CI 8/8 全绿，run `32874791885`），`farmland-trample` 已归档并 sync 进 `authoritative-farming` 主规格，基线段已同步 |
| B-06 | 干耕地退回泥土 | 需第三个耕地编号或附加状态字节 | 方块编号追加或状态编码 | 未认领 | — | farming 遗留 5 |
| B-07 | 水冲毁作物 | 流体可流入作物格并触发掉落；需同步流体的确定性论证与不动点论证 | 流体 `evalCell` 规则变更，论证同步更新 | 已认领 | ox-alpha-implementer @ feat/B-07-flood-destroys-crops | farming 遗留 1；2026-08-25 认领。独占文件集：`internal/fluid` 全部（`evalCell` 作物流入规则、确定性/不动点论证与测试）、`internal/sim` 新建作物冲毁结算文件（只调用既有导出掉落 API，不改 A-01/A-04/B-10 已认领的 `drop.go`/`mining.go`/`crop.go`/`engine_step.go`）、OpenSpec change 目录；无 wire 变更、无编号追加，不触碰 A 批次任何独占文件集 |
| B-08 | 地下农场 | 服务端可查询的极简方块光模型，或重新裁决「服务端不计算光照」禁令 | 视裁决（spec 禁令重裁或新可查模型） | 未认领 | — | farming 遗留 7 |
| B-09 | 湿润判定即时化 | 水源移除后扇出的有界化（与流体队列同构），或耕地湿度缓存；成本计量口径改为方块读取次数 | 无 wire；成本契约计量口径调整 | 未认领 | — | farming 遗留 9、21 |
| B-10 | 作物随机掉落数量 | `hash(worldSeed, tick, pos)` 定数量，与生长抽样共用哈希 | 无 wire 变更 | 已完成 | ox-alpha-implementer @ feat/B-10-crop-drop-hash | farming 遗留 10；2026-08-25 认领并当日完成：PR #81 已合并（CI 8/8 首跑全绿），`crop-random-drop-count` 已归档；成熟小麦 1–3 小麦 + 1–3 种子、重放确定，D9 固定掉落决策由其接替 |
| B-11 | 难度系统 | 困难难度饿死、和平回满、刷怪门控等难度分支 | 配置格式追加难度项 | 未认领 | — | hunger 遗留 4；批次设计非目标 |
| B-12 | 饱和抖动提示 | `PlayerState` 追加 `SaturationZero` 一位 | 协议升版（`PlayerState` 追加字段） | 未认领 | — | hunger 遗留 3 |
| B-13 | 冲刺与攻击疲劳 | 对应动作出现后疲劳表加行 | 无 wire（疲劳表加行） | 已完成 | ox-alpha-implementer @ feat/B-13-attack-exhaustion | hunger 遗留 6；攻击疲劳半边随 change `attack-exhaustion` 交付并归档：固定表第六行 `exhaustionMeleeMilli=100`，判定点在意图冻结分叉。PR #84 以 merge `04bd58ad` 合入，run `32878866254` 的 8 项全绿；SPEC/QUALITY 双评审与整分支终审通过（终审三项文档级发现经一轮修复波清偿）。冲刺半边依赖 B-30 协议升版，另行认领 |
| B-14 | 进食动画/音效/进度 HUD | 复用采掘进度条呈现形状 + 既有音频确认边界 | 无 wire（呈现层） | 已完成 | zcode2-implementer @ feat/B-14-eating-progress-hud | hunger 遗留 2；2026-08-26 认领。控制会话裁决：三点位最小受控重叠——`cmd/mornlea/app.go` 仅追加 eating overlay 与本地进度计数的字段声明行、`app_frame.go` 仅 `Prepare` 实参处构造并传入 eating overlay 的行、`app_lifecycle.go` 仅复位行（B-05 先例同形），其余 app 文件内容不触碰。独占文件集：`internal/render/hud/layout.go`、`renderer.go`（eating bar 呈现与 overlay 参数）及同包测试、OpenSpec change 目录；刻意不触碰 `hud/container.go`（A-01）、`capture_scene*.go`（E-12）、`internal/audio` 与音频装配（进食完成 cue 已交付，列为非目标）、`combat.go`/`hunger.go`（B-13）、`engine_step.go`/`drop.go`（A-04）；无 wire 变更（进度为客户端预测，不升协议）、不动 golden（无既有场景进食） |；2026-08-26 完成：PR #89 已合并（CI 8/8 全绿 · merge bb65a28e，含与 B-13/D-01/E-04/C-01 的 main 合流解冲突），change 归档为 archive/2026-08-26-eating-progress-hud 并 sync 进 survival-hud-presentation 主规格（饥饿已满门控随评审 NIT-4 升级交付）
| B-15 | 伙伴饥饿与自动进食 | 伙伴接三层状态 + 疲劳表 + 自动进食计划步骤 | `companions.ai` schema 可能升版 | 未认领 | — | hunger 遗留 5；依赖伙伴能力扩展组 |
| B-16 | 横向原木与薄雪层 | 方向/高度/碰撞/选取/协议/存档状态编码（现为全方块方向固定） | 区块 schema 升版（方向状态编码） | 未认领 | — | common-block-materials 延期项 |
| B-17 | 门 | 方块 + 交互开合，原创模型 | 方块编号追加 + 开合状态编码 | 未认领 | — | 批次设计非目标 |
| B-18 | 完整生物群系 | 多生物群系地形/植被/材料分布 | worldgen 与材料表变更（大） | 未认领 | — | 批次设计非目标（大） |
| B-19 | 红石 | 方块更新调度预留接口的直接挂载（水流/沙落/草蔓延/作物生长同机制） | 契约待设计（大） | 未认领 | — | `2026-07-26-minecraft-go-design.md` §4.6（大，方向性） |
| B-20 | 多维度世界 | 世界/区块/存档/协议多维度化 | 全 schema/协议升版（大，待设计） | 未认领 | — | 同上 §4.6 预留（方向性） |
| B-21 | 床跨区块放置 | 通用跨区块原子写入事务；出现第二个消费者时才设计 | 无 wire；存储事务 | 未认领 | — | 批次设计「已知简化与升级条件」 |
| B-22 | 数据驱动方块模型 | 有限 model tag 无法清晰表达新形态时升级 | mesh registry/有限 model tag 扩展，engine ABI 可能升版 | 未认领 | — | 同上 |
| B-23 | 远程敌怪与投射物 | 投射物协议与命中模型 | 协议新消息 → 升版 | 未认领 | — | 同上 |
| B-24 | 护甲 | 伤害减免与防护槽位 | 玩家 schema（护甲槽）+ 协议 | 未认领 | — | 同上 |
| B-25 | 有界状态效果系统 | 腐肉中毒等；出现第二个持续状态消费者时建设 | `PlayerState`/玩家 schema 扩展 | 未认领 | — | 同上 |
| B-26 | 第二类敌怪 → AI/ECS 共享边界 | 出现行为显著不同的 mob 后评估共享抽象 | 待评估 | 未认领 | — | 同上；B-27 被动生物落地后一并评估 |
| B-27 | 被动生物（家畜类） | 确定性生成、游走 AI、猎杀掉落肉与副产物；出生/上限/持久化按夜行者先例 | 实体与掉落编号；持久化类比 `hostile_mobs`（可能新 schema） | 未认领 | — | hunger 遗留 1（「肉类需生物」）；B-01 依赖本行；建议 A-04 合入后复用其实体框架；与 B-26 联动评估；繁殖语义待裁决 |
| B-28 | 岩浆与造石 | 岩浆流体（发光、接触伤害/灼烧）、水×岩浆→石头/黑曜石 | 流体编号区间扩展 + 方块光/伤害规则；无新协议消息 | 未认领 | — | authoritative-fluid 与 fluid-presentation proposal 非目标（两处显式延期）；灼烧语义与 A-04 协调 |
| B-29 | 水流推力 | 流动水对实体施加方向力与水面流向呈现 | 物理侧 tunable 追加；无 wire 变更 | 未认领 | — | 同上非目标（「水流对实体的推力与水流方向动画」）；衔接既有浸没物理 |
| B-30 | 冲刺（疾跑） | 移动输入位与速度提升 | `PlayerInput` 追加输入位 → 协议升版 | 未认领 | — | hunger 遗留 6（「动作不存在」）；B-13 冲刺疲劳依赖本行 |
| B-31 | 开箱中断进食 | 打开容器或视野未就绪时中断进食且不扣料 | 无契约变更（`advanceEating` 中断条件） | 已完成 | zcode2-implementer @ feat/B-31-eating-container-interrupt | hunger 遗留 10；需补「开箱中断不扣料」Scenario；2026-08-25 认领。控制会话裁决：批准与 A-01 在 `internal/sim/player.go` 的最小受控重叠——仅限 `advanceEating` 调用点为传入 `viewContainer`/`hasView` 中断所需的最小参数改动，该文件其余内容不触碰。独占文件集：`internal/sim/eating.go`、同包新增测试文件、OpenSpec change 目录；刻意不触碰 `combat.go`/`hunger.go`（B-13）、`engine_step.go`/`drop.go`（A-04）与 `internal/core` 编号段（A-01/A-02/A-04） |；2026-08-26 完成：PR #83 已合并（CI 8/8 首跑全绿 · merge e1c725e8），change 归档为 archive/2026-08-26-eating-container-interrupt 并 sync 进 authoritative-hunger 主规格，中断清单第五条与「中断优先于同 tick 结算」契约落地
| B-32 | 流体音效 cue | 涉水/流水音频，复用既有 cue 纪律（权威确认边界触发） | 无 wire 变更（客户端音频） | 已认领 | ox-alpha-implementer @ feat/B-32-fluid-audio-cue | fluid-presentation proposal 非目标（不做流体音效）。2026-08-26 本会话认领，最小范围：入水边沿 splash cue（`BodyInFluid` 上升沿在权威镜像位置上判定，复用 `physics.SubmersionFlags` 与既有 `cueSpec` 方波合成器）；水下环境音、出水音与 DSP 均不做。独占文件集：`internal/audio/cue.go`、`cmd/mornlea/app_audio.go`、`cmd/mornlea/app_messages.go`（单点接线）、同目录测试文件与 OpenSpec change 目录 `fluid-audio-cue`；刻意不触碰 `internal/audio/player_darwin.go`、`internal/physics/` 与协议/schema/golden |

## C. 伙伴能力扩展（后续功能候选）

> 来源：`docs/superpowers/specs/2026-08-13-ai-native-companions-design.md` §16「非目标与后续方向」——这些能力只有在真实玩家体验证明需要后，才分别进入新的 OpenSpec change。另有 farming 遗留 11 的伙伴农业语义待裁决。

| ID | 功能 | 简述 | 版本与契约影响 | 状态 | 认领人 | 来源与备注 |
|---|---|---|---|---|---|---|
| C-01 | 伙伴采掘容器/多掉落方块 | `mine` 从「单一 `BlockDrop` 且非容器」扩展到容器与多掉落，需先定原子容量语义 | 视实现（`mine` 语义扩展，可能无 wire 变更） | 已完成 | zcode4-implementer @ feat/C-01-companion-mine-containers | §16 + 伙伴 mine 首版明确留给后续单独设计；2026-08-25 认领、2026-08-26 完成待合入。范围冻结：仅放开容器（箱子/熔炉）与多掉落方块作为伙伴 `mine` 目标；农业十编号的显式拒绝保持不变（C-11 另行裁决）；原子容量语义经 brainstorming 裁决为全或无（方案 A）。交付：`companion-mine-containers` 已归档（REMOVED+ADDED 重写 `companion-world-actions` 主规格条目），批量全或无结算 + sim/Runner 共享 `CompanionMineContainerStaging` + Memory/TCP parity；整分支终审 PASS、`gates.sh` 六项全绿（首轮聚合 race 失败取证为并行会话负载 flake）。独占文件集（实际触碰）：`internal/sim/mining.go` 与 `companion_mining_container_test.go`、`internal/companion/plan_types.go`/`planner.go` 及测试、`internal/server/companion_interact.go` 与 `companion_interact_container_test.go`/`companion_interact_parity_test.go` + OpenSpec change 目录 |
| C-02 | 伙伴自动拾取 | 世界掉落物入伙伴背包，须先验证 grid/背包装回不变量 | 视实现（背包不变量） | 未认领 | — | §16 |
| C-03 | 伙伴背包整理/合成/熔炼/开容器 | 自动合成与熔炼需服务端权威语义 | 视实现（`companions.ai` 可能升版） | 未认领 | — | §16 |
| C-04 | 伙伴自动挖障碍/搭桥/游泳/无限世界寻路 | 寻路与计划能力扩展 | 计划步骤扩展 → `companions.ai` schema 升版 | 未认领 | — | §16 |
| C-05 | 伙伴自主目标/自动采集/自动建造 | 无指令自主行为 | 契约待设计（大） | 未认领 | — | §16（大） |
| C-06 | 玩家创建伙伴与所有权 | 创建 UI、所有权、ACL（计费不在范围内） | 配置与 schema 待设计 | 未认领 | — | §16 |
| C-07 | 完整聊天历史/RAG/长期人格演化 | 存储与检索（现仅 ≤2 KiB 近期摘要） | `companions.ai` schema 升版 | 未认领 | — | §16 |
| C-08 | 伙伴空闲随机聊天 | 非任务触发台词 | 复用既有 `CompanionSpeech`，无 wire 变更 | 已认领 | opencode-implementer @ feat/C-08-companion-idle-dialogue | §16；2026-08-26 认领。范围冻结为伙伴空闲 Dialogue 的确定性触发、既有模型并发纪律复用与广播接线。独占文件集：`internal/companion/dialogue_nodes.go`、`dialogue_types.go`、`dialogue_client.go` 及相关定点测试，`internal/server/companion_manager.go`、`companion_dialogue.go`、新建 `companion_idle_dialogue*.go` 及相关测试，OpenSpec change `companion-idle-dialogue`；刻意不触碰 Planner/任务/FIFO、`internal/network`、`internal/storage`、协议/schema/ABI/scenario 版本号、capture golden 与 `AGENTS.md`/`CLAUDE.md`/`progress.md` |
| C-09 | 伙伴主动修改世界/动态任务/世界事件 | 世界事件系统 | 契约待设计（大） | 未认领 | — | §16 |
| C-10 | 多 Agent 协商/模型代码与工具执行 | 伙伴间自主协商、模型代码执行 | 契约待设计（大） | 未认领 | — | §16（大） |
| C-11 | 伙伴参与农业 | 种什么/何时收/成熟度判断语义未裁决；放开三处防御清单前必须先决定语义 | 视裁决（放开三处防御清单） | 未认领 | — | farming 遗留 11（先裁决再开工） |

## D. 客户端 UI/体验（后续功能候选）

> egui 工具型 UI 选型见 `docs/superpowers/specs/2026-08-23-egui-tool-ui-selection-design.md`；「后续每个具体菜单（设置、暂停、调试面板等）各自独立 change，逐个归档进主规格」。

| ID | 功能 | 简述 | 版本与契约影响 | 状态 | 认领人 | 来源与备注 |
|---|---|---|---|---|---|---|
| D-01 | 设置菜单 | 音量（`audioVolume`）、材质包目录（`texturePackPath`）、窗口等设置项 | 配置项暴露，无 wire 变更 | 已完成 | codex-implementer @ codex/D-01-settings-menu | PR #86；change `settings-menu` 已归档并同步主规格。交付世界装配前的 committed/draft 设置页、三字段 raw JSON 原子 patch、材质候选预验证、音量/窗口保存后即时生效、client ABI v9 结构化有界事件批及 19 场景 capture；协议/schema、engine ABI、benchmark scenario 与配置版本均不变。完整验证与 baseline-equivalent 视觉门禁证据见归档 ledger。 |
| D-02 | 暂停菜单 | 暂停/继续/退出；先设计单人权威模拟的暂停语义 | 视暂停语义裁决 | 未认领 | — | egui 实施路径 |
| D-03 | 调试面板 egui 化 | 既有程序化 debug 面板迁入 egui 或并存；性能影响需评审 | 无 wire（呈现层） | 未认领 | — | egui 实施路径 |
| D-04 | 合成面板分页/滚动 | 配方行数增长后按窗口高度自适应（当前 10 行，矮窗口整体缩小） | 无 wire；HUD 布局（capture 校验） | 未认领 | — | farming 遗留 20 |
| D-05 | HUD 物品图集 UV 对齐 | 按 texel 中心/整数像素计算，使图集扩列不影响既有图标 | 无 wire（UV 计算） | 已完成 | ox-alpha-implementer @ fix/D-05-hud-atlas-texel-uv | farming 遗留 18；PR #82 已合并（CI 8/8 全绿），change `hud-atlas-texel-stable-uv` 已归档并 sync 进 `survival-hud-presentation` 主规格；最终方案为对称亚纹素收进 1/256 纹素（半纹素中心对齐被否决），两景心形区 golden 经用户裁决外科手术式再生 |
| D-06 | 材质包 v2 | HUD 图集覆盖（鸡腿/爱心/气泡可替换）、构建期许可校验与成果打包 | 配置语义扩展 + 构建期校验 | 未认领 | — | hunger 遗留 7；farming 遗留 12（贴图管线） |
| D-07 | 耕地 mesh 顶面下沉 | 按 material 固定下移顶面，或复用水面角高度位 | capture golden 更新 | 已认领 | ox-alpha-implementer @ feat/D-07-farmland-mesh-top-sink | farming 遗留 13；2026-08-26 认领。独占文件集：`engine/crates/mornlea_engine/`（mesher 顶面几何与测试）、受影响 capture golden 与 change 产物；不触碰 wire/schema/registry 名额与 `internal/physics`（碰撞侧已是 15/16） |
| D-08 | 农田+花草 capture 场景 | 植物种类增多后补视觉场景并记录差异来源 | capture 场景与 golden 追加 | 未认领 | — | farming 遗留 24 |
| D-09 | 第三人称与角色姿态呈现 | 第三人称相机模式与角色姿态（游泳等） | client ABI 可能扩展（相机/姿态） | 未认领 | — | fluid-presentation proposal 非目标（不做游泳姿态动画与第三人称呈现）；avatar pass 为基础 |

## E. 工程与基础设施（后续任务候选）

> 多数行是迁移/重构/稳定性任务，仍需各自独立 OpenSpec change；只有标「直接修改豁免」的小型修复可直接改。

| ID | 任务 | 简述 | 版本与契约影响 | 状态 | 认领人 | 来源与备注 |
|---|---|---|---|---|---|---|
| E-01 | Rust 阶段 3：状态与规则分离 | Rust 接管 world/player 可变状态与 deterministic step executor；Go 规则收敛为 `EventBatch → CommandBatch`；须先设计事件身份、命令校验与回放语义 | 事件/命令契约待设计（大） | 未认领 | — | `2026-08-12-rust-engine-go-rules-design.md` §6（大，方向性） |
| E-02 | Rust 阶段 4：网络、存储与服务端运行时 | Rust 接管 transport、wire codec、存档与调度，保持协议/schema/fixture/故障注入/parity | transport 契约保持（大） | 未认领 | — | 同上 §6（大） |
| E-03 | Rust 阶段 5：客户端、渲染与入口反转 | Rust 接管 client application、渲染与主循环，静态链接 Go 规则库，删除剩余 Go 引擎 | 入口反转（大） | 未认领 | — | 同上 §6（大） |
| E-04 | 删除旧 Go oracle | 碰撞/射线/物理/世界生成等迁移后的 test-only oracle，以独立 change 删除 | 无（test-only 删除） | 已完成 | ox-alpha-implementer @ chore/E-04-drop-go-oracles | rust-engine-go-rules §15「oracle 删除条款」；2026-08-25 认领。范围冻结为三切片：physics 步进/碰撞差分（`step_native_test.go`/`collision_native_test.go` 及仅剩这两者消费的 helper 与旧实现体）、core 射线 DDA 差分与 fuzz 基准（`raycast_native_test.go`/`raycast_fuzz_test.go`/`raycast_helpers_test.go`）、worldgen 生成差分（`oracle_test.go`/`parity_test.go` 及 `generator.go` 内失去消费者的 test-only 旧噪声/地形实现）；`internal/mesh` 的 greedy/light oracle 切片因 A-02 独占 `internal/mesh` 且正改 Rust mesher 而延迟，待 A 批次合流后另行认领。2026-08-25 认领并当日完成：PR #87 已合并（CI 8/8 首跑全绿），`drop-go-test-oracles` 已归档；三切片净删约 1.2k 行，physics 位级 golden 向量与 raycast 几何不变量补位；mesh 切片延迟待 A 批次合流 |
| E-05 | 世界 goroutine 区域分片 | 吞吐瓶颈缓解（单核 world goroutine 是硬上限）；严格限界后按维度/区域分片 | 并发结构重构，无 wire | 未认领 | — | `2026-07-26-minecraft-go-design.md` §8.1（方向性） |
| E-06 | 图形客户端平台扩展 | Windows/Linux 客户端构建与验收（当前 engine 只承诺 Apple Silicon/macOS 正式验收） | 平台构建矩阵 | 未认领 | — | rust-engine-go-rules §16 平台链接差异 |
| E-07 | 存档 Flush 恒脏自旋修复 | `playerPersistence.Flush` 去重键去掉 revision 或「连续 N 次重派无进展即放弃」 | 无（持久化循环修复） | 已完成 | claude-implementer @ fix/E-07-flush-stall-guard | hunger 遗留 9；独占文件集：`internal/server/player_flush.go` 及 `internal/server/` 相关测试 + OpenSpec change `fix-player-flush-stall` |
| E-08 | `HighestOpaque` 语义改名/钉死 | 返回最高非空气方块，名不副实；跨包改名或 GoDoc 首句钉死语义 | 无（跨包改名或 GoDoc） | 已完成 | codex-implementer @ chore/E-08-highest-opaque-semantics | 完成证据：GoDoc 自 `7a7253e7` 已钉死“最高非空气”；既有 world/crop 测试覆盖；零代码变更 |
| E-09 | 作物×锄头耐久豁免 | 对齐 MC：手持锄头收获不扣耐久，`completeMining` 加豁免表并配测试 | 无（豁免表 + 测试） | 已完成 | codex2-implementer @ fix/E-09-hoe-harvest-durability | farming 遗留 16；`fix-hoe-harvest-durability` 已归档；SPEC/QUALITY 与整分支终审通过，`scripts/agents/gates.sh` 全绿 |
| E-10 | `findSpawnInColumn` 读落脚盒顶面 | 出生点与 support/safe 存档点三处口径同步（耕地 1/16 空隙） | 无（出生点口径） | 已完成 | codex-implementer @ fix/E-10-spawn-support-top | farming 遗留 14；`fix-spawn-support-top` 已归档；真实耕地出生/恢复/safe 测试、SPEC/QUALITY 双评审与整分支 `gates.sh` 均通过 |
| E-11 | server 测试等待预算化 | 既有登录等待循环多数无界（5 分钟超时而非可读断言），统一有预算等待助手 | 无（测试基础设施） | 已完成 | codex-implementer @ codex/fix-E-11-server-test-wait-budget | farming 遗留 19（测试基础设施）；归档 change：`2026-08-25-server-test-wait-budget`；Task 1 R1 后 SPEC/QUALITY 双评审 PASS，整分支终审 0 findings，fresh gates 全绿 |
| E-12 | M5E 再递延字面同源化 | `capture_scene.go` 与 `capture_ai_companion_test.go` 的 `[32]network.ChatEvent` 字面；ChatCommand 编解码 1024 字面与错误文案 | 无（字面同源化） | 已认领 | claude-implementer @ chore/E-12-literal-single-source | m5e 归档 proposal「延期与放弃」递延 4、5；独占文件集：`cmd/mornlea/capture_scene.go`、`cmd/mornlea/capture_ai_companion_test.go`、`internal/network/codec_client.go` 及相关定点测试 + OpenSpec change `m5e-literal-single-source`；2026-08-25 原认领（chore/E-12-companion-limits）无推送记录，随机器迁移重置重认领；2026-08-25 本行由 claude-implementer 认领，分支 chore/E-12-literal-single-source |
| E-13 | benchmark 新 scenario record-only 报告 | 为已升版 scenario 补 Memory/TCP 记录报告并追加到 `perf-baseline.md`（数值只记录） | 无（报告只记录） | 已完成 | codex-implementer @ chore/E-13-benchmark-record-report | farming 遗留 23；`E-13-approval2` 批准 bounded 短设计；冻结 `923fa0d7` 生成并自检 Memory/TCP v19 报告及跨 transport 比较，SPEC/QUALITY PASS，`gates.sh` 全绿；基线 JSON 未覆盖 |

## F. 小型修复队列（直接修改豁免，但同样认领登记）

> 不需要 OpenSpec change，但完成前仍须相称验证（受影响包测试）。

| ID | 修复 | 简述 | 版本与契约影响 | 状态 | 认领人 | 来源与备注 |
|---|---|---|---|---|---|---|
| F-01 | 拒绝原因白名单补登记 | `client/mirror.go` 补登记 `RejectContainerCapacity`，服务端发该原因时客户端不再报 unknown | 无 wire 变更 | 已完成 | claude-implementer @ fix/F-01-reject-container-capacity | farming 遗留 15；PR #73 已合并（CI 8/8 全绿） |
| F-02 | perfcheck 文案修正 | `cmd/perfcheck/migration_test.go` 的 v15 比较测试失败信息误写为 v16 | 无（一处字符串） | 已完成 | claude-implementer @ fix/F-02-perfcheck-v15-message | PR #72 已合并（CI 8/8 全绿） |
| F-03 | 「使用」键放置判定收敛 | 客户端按 `core.ItemPlacement` 决定是否发 `PlaceBlock`，不可放置物一律不发 | 无（客户端判定；钉住现状的 `TestUseKeyRisingEdgeSkipsPlaceWhileHoldingFood` 需同步调整） | 已完成 | claude-implementer @ fix/F-03-use-key-placement | PR #74 已合并（CI 首跑 1 flake 重跑后 8/8 全绿） |
