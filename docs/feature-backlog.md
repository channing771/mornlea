# Mornlea 功能规划与认领表

本表是 Mornlea 后续开发任务的**单一规划来源**：每一行 = 一个可独立认领、独立评审、独立验收的功能或工程任务，通常对应一条 OpenSpec change。控制会话与 implementer 子代理在开始任何开发前先读本表；认领后立即在行内打标记并提交，防止多个 agent 抢同一任务。本表只描述「要做什么、谁在做、依赖什么」，**不排时间表**。

> 开发流程、工程约定与验证门禁以 `AGENTS.md`（与 `CLAUDE.md` 逐字节相同）与 `openspec/config.yaml` 为准。本表不重复、不替代二者；二者与代码矛盾时，以代码、测试与 `openspec/specs/` 主规格为真相。

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

## 开发流程（superpowers：subagent-driven-development）

每个任务认领后**必须**以子智能体驱动方式执行，控制会话不直接实现：

1. **先建 OpenSpec change**（复杂功能 / 新模块 / 跨包重构 / 存档 / 协议 / 性能契约）：创建 `proposal.md`、delta specs、`design.md`、`tasks.md`、`ledger.md`，通过 `openspec validate --all --strict --no-interactive`。小型修复（拼写、格式、一次性实验）可直接修改，但仍须相称验证。
2. **每 Task 派发全新 implementer 子代理**：任务 brief 是唯一需求来源（当前 Task + 共享契约 SHA + 对应计划 + change 产物 + 全局约束 + 精确验证命令）；implementer 不得自行派生子代理或评审者。
3. **TDD（red → green → refactor）**；测试按关注点单文件、复用包内唯一 helper 中心。
4. **每 Task 后独立双评审**：SPEC 合规评审 + QUALITY 代码评审（全新 reviewer）；修复循环单任务最多 5 轮（R≤3 续用原 implementer，R≥4 换新 implementer），超限逐条裁决。
5. **一切进度、评审结论与裁决写入 `ledger.md`**（格式：Ruling — 决定什么 — 为什么 — 错在哪）。
6. 全部 Task 完成后**整分支终审**，再执行全量门禁：

   ```bash
   make rust
   go test ./... -race
   go vet ./...
   test -z "$(gofmt -l .)"
   openspec validate --all --strict --no-interactive
   # 渲染/tick/存储/协议热路径变化另加：benchmark（数值只记录）、fuzz/golden、perfcheck、
   # make visual（capture 场景变化时必须逐图验收，禁止放宽阈值）
   ```

   迭代期可用 `make dev-check` / `make test-race-short` 快检；CI 与最终门禁不带 `-short`。
7. **归档与合入**：`openspec sync` → archive → 同步 `AGENTS.md` 与 `CLAUDE.md`（**逐字节相同**，archcheck 兜底）与 `docs/notes/progress.md` → 合并 `main` → 本表该行 `已完成`。

## 并行与冲突规则

- **同批并行必须共享契约**：参照第一夜批次先例——第一步在功能分支共同基线上冻结「追加编号 / 协议消息 / 有限模型 tag」的 append-only 共享契约提交，各功能分支从该 SHA 创建；`AGENTS.md`/`CLAUDE.md`、capture golden、benchmark scenario 与版本基线由**集成任务独占**，功能分支不得触碰（避免并行覆盖 PNG 与文档冲突）。
- **版本号互斥**：协议 / 存档 schema / engine ABI / client ABI / benchmark scenario 的升版行互斥——同一时间只能被一个认领者持有；冲突时按实际合入顺序重排（例：第一夜批次设计写的 client ABI v8 已被 egui 主菜单占用，须重排为 v9）。
- **文件所有权**：认领时在备注声明独占文件集；与本表其它已认领行重叠即换行或延迟。
- **范围冻结**：认领后不得扩大范围；实现发现规格不成立时，先改该 change 的 OpenSpec 产物再继续编码。

---

## A. 第一夜生存批次（在途，不得重复认领）

> 设计与计划见 `docs/superpowers/specs/2026-08-23-first-night-survival-parallel-wave-design.md` 与 `docs/superpowers/plans/2026-08-23-first-night-survival-parallel-wave.md`。五个功能分支与集成分支均已存在于本地 worktree（`.worktrees/`）与 `codex/*` 分支；共享契约 SHA 为 `785ea07b`（`fix: address shared contract review`）。最终版本：协议 v27、玩家 schema v8、世界 metadata v3、`hostile_mobs` schema v1、engine ABI v7、benchmark scenario v20；区块 schema v9 与 `companions.ai` v4 不变。

| ID | 功能 | 简述 | 状态 | 认领人 | 来源与备注 |
|---|---|---|---|---|---|
| A-01 | 权威格子工作台 | 背包 2×2 + 工作台 3×3 形状配方、权威产物取出、关闭/断线装回不变量、七条新配方 | 待集成 | 前批实现线 `codex/authoritative-grid-crafting` | 分支头 `a657c1cb`，实现与整功能评审完成 |
| A-02 | 可放置火把 | 落地 + 四向墙面五形态、支撑约束与移除反应、发光等级 14、mesh model tag 与窄柱几何 | 待集成 | 前批实现线 `codex/placeable-torches` | 分支头 `b2115a64`，实现与整功能评审完成 |
| A-03 | 三级剑与统一战斗 | 木/石/铁剑（4/5/6 伤害）、统一候选（≤72）与冷却/击退/耐久、CombatHit 私有确认 | 待集成 | 前批实现线 `codex/tiered-swords-combat` | 分支头 `42f3dead`；SPEC/QUALITY PASS，recipe 15..17 依赖 A-01 合流 |
| A-04 | 权威近战夜行者 | 确定性夜间生成、全服 64/每玩家 8 上限、A* + Rust 物理、灼烧/消失/腐肉掉落、`hostile_mobs` v1 | 开发中 | 前批实现线 `codex/authoritative-hostile-nightwalker` | 分支头 `7c3d5e60`；worktree 尚有未提交的持久化修复（`hostile_persistence*.go`） |
| A-05 | 床与睡眠 | 双格床八形态、同区块原子放置、全员睡眠跳夜（`DayTimeOffsetTicks`）、个人重生点 | 已认领 | 前批实现线 `codex/authoritative-bed-sleep` | 分支头 `785ea07b` = 共享契约，**尚无实现**；从共享 SHA 继续 |
| A-06 | 五路合流集成 | 固定顺序合并 crafting→torches→swords→nightwalker→bed；TDD 接通剑×夜行者统一战斗、夜行者阻睡、bed model dispatcher；删除 `network.CraftRecipe` 过渡类型 | 未认领 | — | 依赖 A-01..A-05；按批次计划 Task 3 |
| A-07 | 版本基线与视觉基线 | 协议 v27 / schema v8 / metadata v3 / `hostile_mobs` v1 / engine ABI v7 / benchmark v20 迁移 `19:20`；生成新 5 场景 golden；同步 `AGENTS.md`+`CLAUDE.md`+`progress.md` | 未认领 | — | 依赖 A-06。**认领时注意**：a) client ABI 设计值 v8 已被 egui 占用，按实际重排；b) 场景总数按合入时的实际清单（既有 18 + 新增 5 = 23，批次计划写 22 系编写时未含 main-menu） |
| A-08 | 整分支终审、归档与推送 | 独立整分支终审（规格、上限、并发/持久化错误路径、wire 安全、22/23 图、无版权资源）、五个 change 归档、合入 `main` | 未认领 | — | 依赖 A-07；按批次计划 Task 5 |

## B. 生存与世界深化（后续功能候选）

> 每行一条独立 OpenSpec change。来源列括号内是归档 change 的 `design.md`「遗留与简化清单」条目号，或批次设计「非目标 / 已知简化与升级条件」。

| ID | 功能 | 简述 | 状态 | 认领人 | 来源与备注 |
|---|---|---|---|---|---|
| B-01 | 更多食物与作物 | 肉类生物掉落+熟食熔炉食谱、更多作物（编号+纹理+生长参数即可扩展） | 未认领 | — | hunger 遗留 1；farming 遗留 2 |
| B-02 | 水桶（可搬运流体） | 舀水/倒水物品，解除「农业只能在天然水体 4 格内」约束 | 未认领 | — | farming 遗留 25（显式非目标解除） |
| B-03 | 骨粉 | 新物品 + 「立即推进 N 阶段」动作，走翻地同形命令路径 | 未认领 | — | farming 遗留 3 |
| B-04 | 草丛与除草掉种子 | 植物几何第二消费者；草丛掉落种子替代初始材料包供给 | 未认领 | — | farming 遗留 6 |
| B-05 | 踩踏破坏耕地 | 实体落地事件接进方块变更（物理侧已有落地判定） | 未认领 | — | farming 遗留 4 |
| B-06 | 干耕地退回泥土 | 需第三个耕地编号或附加状态字节 | 未认领 | — | farming 遗留 5 |
| B-07 | 水冲毁作物 | 流体可流入作物格并触发掉落；需同步流体的确定性论证与不动点论证 | 未认领 | — | farming 遗留 1 |
| B-08 | 地下农场 | 服务端可查询的极简方块光模型，或重新裁决「服务端不计算光照」禁令 | 未认领 | — | farming 遗留 7 |
| B-09 | 湿润判定即时化 | 水源移除后扇出的有界化（与流体队列同构），或耕地湿度缓存；成本计量口径改为方块读取次数 | 未认领 | — | farming 遗留 9、21 |
| B-10 | 作物随机掉落数量 | `hash(worldSeed, tick, pos)` 定数量，与生长抽样共用哈希 | 未认领 | — | farming 遗留 10 |
| B-11 | 难度系统 | 困难难度饿死、和平回满、刷怪门控等难度分支 | 未认领 | — | hunger 遗留 4；批次设计非目标 |
| B-12 | 饱和抖动提示 | `PlayerState` 追加 `SaturationZero` 一位 | 未认领 | — | hunger 遗留 3 |
| B-13 | 冲刺与攻击疲劳 | 对应动作出现后疲劳表加行 | 未认领 | — | hunger 遗留 6 |
| B-14 | 进食动画/音效/进度 HUD | 复用采掘进度条呈现形状 + 既有音频确认边界 | 未认领 | — | hunger 遗留 2 |
| B-15 | 伙伴饥饿与自动进食 | 伙伴接三层状态 + 疲劳表 + 自动进食计划步骤 | 未认领 | — | hunger 遗留 5；依赖伙伴能力扩展组 |
| B-16 | 横向原木与薄雪层 | 方向/高度/碰撞/选取/协议/存档状态编码（现为全方块方向固定） | 未认领 | — | common-block-materials 延期项 |
| B-17 | 门 | 方块 + 交互开合，原创模型 | 未认领 | — | 批次设计非目标 |
| B-18 | 完整生物群系 | 多生物群系地形/植被/材料分布 | 未认领 | — | 批次设计非目标（大） |
| B-19 | 红石 | 方块更新调度预留接口的直接挂载（水流/沙落/草蔓延/作物生长同机制） | 未认领 | — | `2026-07-26-minecraft-go-design.md` §4.6（大，方向性） |
| B-20 | 多维度世界 | 世界/区块/存档/协议多维度化 | 未认领 | — | 同上 §4.6 预留（方向性） |
| B-21 | 床跨区块放置 | 通用跨区块原子写入事务；出现第二个消费者时才设计 | 未认领 | — | 批次设计「已知简化与升级条件」 |
| B-22 | 数据驱动方块模型 | 有限 model tag 无法清晰表达新形态时升级 | 未认领 | — | 同上 |
| B-23 | 远程敌怪与投射物 | 投射物协议与命中模型 | 未认领 | — | 同上 |
| B-24 | 护甲 | 伤害减免与防护槽位 | 未认领 | — | 同上 |
| B-25 | 有界状态效果系统 | 腐肉中毒等；出现第二个持续状态消费者时建设 | 未认领 | — | 同上 |
| B-26 | 第二类敌怪 → AI/ECS 共享边界 | 出现行为显著不同的 mob 后评估共享抽象 | 未认领 | — | 同上 |

## C. 伙伴能力扩展（后续功能候选）

> 来源：`docs/superpowers/specs/2026-08-13-ai-native-companions-design.md` §16「非目标与后续方向」——这些能力只有在真实玩家体验证明需要后，才分别进入新的 OpenSpec change。另有 farming 遗留 11 的伙伴农业语义待裁决。

| ID | 功能 | 简述 | 状态 | 认领人 | 来源与备注 |
|---|---|---|---|---|---|
| C-01 | 伙伴采掘容器/多掉落方块 | `mine` 从「单一 `BlockDrop` 且非容器」扩展到容器与多掉落，需先定原子容量语义 | 未认领 | — | §16 + 伙伴 mine 首版明确留给后续单独设计 |
| C-02 | 伙伴自动拾取 | 世界掉落物入伙伴背包，须先验证 grid/背包装回不变量 | 未认领 | — | §16 |
| C-03 | 伙伴背包整理/合成/熔炼/开容器 | 自动合成与熔炼需服务端权威语义 | 未认领 | — | §16 |
| C-04 | 伙伴自动挖障碍/搭桥/游泳/无限世界寻路 | 寻路与计划能力扩展 | 未认领 | — | §16 |
| C-05 | 伙伴自主目标/自动采集/自动建造 | 无指令自主行为 | 未认领 | — | §16（大） |
| C-06 | 玩家创建伙伴与所有权 | 创建 UI、所有权、ACL（计费不在范围内） | 未认领 | — | §16 |
| C-07 | 完整聊天历史/RAG/长期人格演化 | 存储与检索（现仅 ≤2 KiB 近期摘要） | 未认领 | — | §16 |
| C-08 | 伙伴空闲随机聊天 | 非任务触发台词 | 未认领 | — | §16 |
| C-09 | 伙伴主动修改世界/动态任务/世界事件 | 世界事件系统 | 未认领 | — | §16 |
| C-10 | 多 Agent 协商/模型代码与工具执行 | 伙伴间自主协商、模型代码执行 | 未认领 | — | §16（大） |
| C-11 | 伙伴参与农业 | 种什么/何时收/成熟度判断语义未裁决；放开三处防御清单前必须先决定语义 | 未认领 | — | farming 遗留 11（先裁决再开工） |

## D. 客户端 UI/体验（后续功能候选）

> egui 工具型 UI 选型见 `docs/superpowers/specs/2026-08-23-egui-tool-ui-selection-design.md`；「后续每个具体菜单（设置、暂停、调试面板等）各自独立 change，逐个归档进主规格」。

| ID | 功能 | 简述 | 状态 | 认领人 | 来源与备注 |
|---|---|---|---|---|---|
| D-01 | 设置菜单 | 音量（`audioVolume`）、材质包目录（`texturePackPath`）、窗口等设置项 | 未认领 | — | egui 集成约束与实施路径 |
| D-02 | 暂停菜单 | 暂停/继续/退出；先设计单人权威模拟的暂停语义 | 未认领 | — | egui 实施路径 |
| D-03 | 调试面板 egui 化 | 既有程序化 debug 面板迁入 egui 或并存；性能影响需评审 | 未认领 | — | egui 实施路径 |
| D-04 | 合成面板分页/滚动 | 配方行数增长后按窗口高度自适应（当前 10 行，矮窗口整体缩小） | 未认领 | — | farming 遗留 20 |
| D-05 | HUD 物品图集 UV 对齐 | 按 texel 中心/整数像素计算，使图集扩列不影响既有图标 | 未认领 | — | farming 遗留 18 |
| D-06 | 材质包 v2 | HUD 图集覆盖（鸡腿/爱心/气泡可替换）、构建期许可校验与成果打包 | 未认领 | — | hunger 遗留 7；farming 遗留 12（贴图管线） |
| D-07 | 耕地 mesh 顶面下沉 | 按 material 固定下移顶面，或复用水面角高度位 | 未认领 | — | farming 遗留 13 |
| D-08 | 农田+花草 capture 场景 | 植物种类增多后补视觉场景并记录差异来源 | 未认领 | — | farming 遗留 24 |

## E. 工程与基础设施（后续任务候选）

> 多数行是迁移/重构/稳定性任务，仍需各自独立 OpenSpec change；只有标「直接修改豁免」的小型修复可直接改。

| ID | 任务 | 简述 | 状态 | 认领人 | 来源与备注 |
|---|---|---|---|---|---|
| E-01 | Rust 阶段 3：状态与规则分离 | Rust 接管 world/player 可变状态与 deterministic step executor；Go 规则收敛为 `EventBatch → CommandBatch`；须先设计事件身份、命令校验与回放语义 | 未认领 | — | `2026-08-12-rust-engine-go-rules-design.md` §6（大，方向性） |
| E-02 | Rust 阶段 4：网络、存储与服务端运行时 | Rust 接管 transport、wire codec、存档与调度，保持协议/schema/fixture/故障注入/parity | 未认领 | — | 同上 §6（大） |
| E-03 | Rust 阶段 5：客户端、渲染与入口反转 | Rust 接管 client application、渲染与主循环，静态链接 Go 规则库，删除剩余 Go 引擎 | 未认领 | — | 同上 §6（大） |
| E-04 | 删除旧 Go oracle | 碰撞/射线/物理/世界生成等迁移后的 test-only oracle，以独立 change 删除 | 未认领 | — | §15「oracle 删除条款」+ progress.md |
| E-05 | 世界 goroutine 区域分片 | 吞吐瓶颈缓解（单核 world goroutine 是硬上限）；严格限界后按维度/区域分片 | 未认领 | — | `2026-07-26-minecraft-go-design.md` §8.1（方向性） |
| E-06 | 图形客户端平台扩展 | Windows/Linux 客户端构建与验收（当前 engine 只承诺 Apple Silicon/macOS 正式验收） | 未认领 | — | rust-engine-go-rules §16 平台链接差异 |
| E-07 | 存档 Flush 恒脏自旋修复 | `playerPersistence.Flush` 去重键去掉 revision 或「连续 N 次重派无进展即放弃」 | 未认领 | — | hunger 遗留 9 |
| E-08 | `HighestOpaque` 语义改名/钉死 | 返回最高非空气方块，名不副实；跨包改名或 GoDoc 首句钉死语义 | 未认领 | — | farming 遗留 17 |
| E-09 | 作物×锄头耐久豁免 | 对齐 MC：手持锄头收获不扣耐久，`completeMining` 加豁免表并配测试 | 未认领 | — | farming 遗留 16 |
| E-10 | `findSpawnInColumn` 读落脚盒顶面 | 出生点与 support/safe 存档点三处口径同步（耕地 1/16 空隙） | 未认领 | — | farming 遗留 14 |
| E-11 | server 测试等待预算化 | 既有登录等待循环多数无界（5 分钟超时而非可读断言），统一有预算等待助手 | 未认领 | — | farming 遗留 19（测试基础设施） |
| E-12 | M5E 再递延字面同源化 | `capture_scene.go` 与 `capture_ai_companion_test.go` 的 `[32]network.ChatEvent` 字面；ChatCommand 编解码 1024 字面与错误文案 | 未认领 | — | m5e 归档 proposal「延期与放弃」递延 4、5 |
| E-13 | benchmark 新 scenario record-only 报告 | 为已升版 scenario 补 Memory/TCP 记录报告并追加到 `perf-baseline.md`（数值只记录） | 未认领 | — | farming 遗留 23；执行前读 `docs/notes/perf-baseline.md` |

## F. 小型修复队列（直接修改豁免，但同样认领登记）

> 不需要 OpenSpec change，但完成前仍须相称验证（受影响包测试）。

| ID | 修复 | 状态 | 认领人 | 来源与备注 |
|---|---|---|---|---|
| F-01 | `client/mirror.go` 拒绝原因白名单补登记 `RejectContainerCapacity` | 未认领 | — | farming 遗留 15 |
| F-02 | `cmd/perfcheck/migration_test.go` 的 v15 文案修正（写的 v16 实比 v15） | 未认领 | — | farming 遗留 22 |
