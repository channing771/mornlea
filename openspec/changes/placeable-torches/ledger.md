# Ledger

## 历史段（旧基线 `b2115a64`，已丢失，仅作参考）

| 日期 | 阶段 | 结论 | 验证 |
| --- | --- | --- | --- |
| 2026-08-25 | 0.5 确认 Q1 | **Ruling:** 原批次共享契约提交随机器迁移丢失，不存在全仓 `reserve first-night survival contracts`；本轮由本链重建——先产出 Task 1 冻结内容（OpenSpec 全套产物、docs-only、无行为），A-02 完成后推分支开 PR 但**不合并**（待集成），由 A-06 统一合流。用户选 A。 | 全仓 grep 无预留提交；worktree 干净 @ ca8e9d5c。 |
| 2026-08-25 | 0.5 确认 Q2 | **Ruling:** engine ABI v6 → v7 由本分支实际升版；`AGENTS.md`/`CLAUDE.md` 只做「engine ABI 版本表述」一项的最小同步（两份逐字节相同），其余基线内容归集成任务；以 `TestBaselineVersionsMatchCode` 转绿所需的最小集合为准。用户选 A。 | `internal/nativeabi` 读 `C.MORNLEA_ENGINE_ABI_VERSION` 单处。 |
| 2026-08-25 | 0.5 approval | **Ruling:** bounded 短设计（契约重建 + Task 2–6 概要 + 验证命令 + 待集成路径）经飞书卡片获用户显式批准（`A-02-approval` approve @ 2026-08-25T10:39:28Z）。 | `~/.mornlea/confirm/A-02-approval.reply.json`：`approve`。 |
| 2026-08-25 | 1 契约产物 | 创建 `placeable-torches` 全套产物：proposal/design/tasks/ledger + 五份 delta specs；冻结互斥契约（火把方块 46..50、物品 37/39、mesh 19 bytes/48→64 entries/model offset 18、engine ABI v7、配方契约 19 号：煤炭在木棍上方 → 4 火把）与方向映射、支撑复核、属性唯一事实源、`torch-night` 场景契约（golden 归 A-07）。 | `openspec validate --all --strict --no-interactive`；`git diff --check`。 |

> 以上编号与职责切分基于旧基线（协议 v26、engine ABI v6、A-06/A-07 待办、配方归 A-01、golden 归 A-07），对应提交 `b2115a64` 已丢失；留档仅作决策脉络参考，不构成当前契约。

## 执行记录段（新基线）

| 日期 | 阶段 | 结论 | 验证 |
| --- | --- | --- | --- |
| 2026-08-27 | 0 重做裁决 | **Ruling（用户）:** ① 原 change 产物写于 A-01 合流前的旧基线，stash 留档后从当前 main 重做；A-06/A-07 已取消，职责回收各功能行。② A-02 自带 `torch-night` 场景 + golden（19 → 20 张），火把配方由 A-02 在 A-01 格子合成系统上追加（`RecipeTorch`=14）。 | 分支 `feat/A-02-placeable-torches` reset 至 origin/main=`cc385d22`；原产物以暂存区留档。 |
| 2026-08-27 | 0.5 契约修订 | 按 `cc385d22` 重写 proposal/design/tasks/ledger 与五份 delta：编号最终锁定（方块 62..66、`ItemTorch`=43、`RecipeTorch`=14）、engine ABI v7 → v8、mesh entry 19→20 bytes（model offset 19，`blockTopRaw` 保持 offset 18）、条目上限 64→80（67 > 64 所迫）、`torch-night` 自带 golden（场景表第 12 位，golden 19→20 张）。与指令给定的「19 bytes/offset 18/上限 64」冲突处按代码为准（`internal/mesh/native_input.go`、`engine/crates/mornlea_engine/src/input.rs`）。 | `openspec validate placeable-torches --strict --no-interactive`；`openspec validate --all --strict --no-interactive`。 |

### 版本矩阵（基线 `cc385d22`）

| 项 | 基线 | 本变更后 |
| --- | --- | --- |
| 协议 | v27 | v27（不变，无 wire 变更、无新命令） |
| 玩家 schema | v7 | v7 |
| 区块 schema | v9 | v9 |
| 世界 metadata | v2 | v2 |
| `companions.ai` schema | v4 | v4 |
| engine ABI | v7 | **v8**（registry entry 19→20 bytes） |
| client ABI | v9 | v9 |
| benchmark scenario | v19 | v19 |
| 方块编号 | 0..61（`BlockIDMax`=62） | 0..66（火把 62..66，`BlockIDMax`=67） |
| 物品编号 | 0..42（`ItemIDMax`=43） | 0..43（`ItemTorch`=43，`ItemIDMax`=44） |
| recipe 表 | 1..13 | 1..14（`RecipeTorch`=14） |
| mesh registry entry | 19 bytes / 上限 64 | 20 bytes / 上限 80 |
| capture 场景表 | 20 项（含 `workbench-crafting`） | 21 项（`torch-night` 第 12 位） |
| capture golden | 19 张 | 20 张（新增 `torch-night.png`） |

### 待跑基线命令清单

- `make rust`
- `go test ./internal/core ./internal/assets -race -count=1`
- `go test ./internal/sim ./internal/world -race -count=1`
- `go test ./internal/mesh ./internal/nativeabi -race -count=1`
- `go test ./internal/archcheck -count=1`

### 已知既有缺口（不属本变更，收尾上报）

- `workbench-crafting` 场景无 golden PNG：A-01 交付场景构造时把 golden 生成挂给已取消的批次集成任务；场景表 20 项与 golden 19 张的差即此。capture 非更新模式下该场景会因 golden 缺失失败，需对应功能行或独立基线任务补齐。
- 主规格滞后于代码：`openspec/specs/authoritative-crafting` 仍写 11 条配方与旧聚合语义（代码为 13 条形状配方）、`openspec/specs/visual-verification` 场景清单仍不含 `workbench-crafting`、`openspec/specs/rust-engine-mesh` 仍写「ABI version MUST 保持 1」（代码 v7）——A-01 归档未把这些 MODIFIED 合入主规格；本变更的 delta 按代码事实书写，归档时一并弥合所触及条目。

### Task 1 执行记录

- 实现 commit：`b77b4dde feat: register torch blocks, items and recipe`；评审修复 commit：`cf358dd4 fix: raise mesh registry entry cap to 80`；产物裁决 commit：`26c20c62`（design 方向表 Z 行笔误修正 + Task 2 边界补 physics 零碰撞）。
- RED：core/assets 双包 build failure（TorchStandingID、PlaceableBlockAtFace、ItemTorch、RecipeTorch 等未定义）。
- GREEN：core/assets race `ok 4.214s/4.057s`；archcheck `ok 4.335s`；QUALITY repair 后 core/assets/mesh/client 四包 race 全绿、cargo workspace 159+166 通过。
- 超清单同步（裁决接受）：`farming_test.go`/`block_name_test.go` 为既定枚举守护的最小机械同步；repair 中 `native_input_test.go` 容量夹具按每行 2 字联动。
- Ruling: registry 条目上限 64→80 提前至 Task 1 收口（QUALITY C1）——保留每提交点套件全绿，19→20 bytes 布局与 ABI v8 仍归 Task 3；Task 1 验证面扩为 core/assets/mesh/client 四包。
- SPEC 评审：PASS（无 Critical/Important；2 条 Minor 记录）。QUALITY 评审：初审 FAIL（C1 越界回归 + I1/M1/M2/M3），repair round 1 后复审 PASS（含线格式 count 推导逐链核对与跨 FFI 80 条容量钉子）。

### Task 2 执行记录

- 实现 commit（amend 终值）：`cebbfc10 feat: enforce torch support`（初版 bc0ae0a9 → 伙伴防御修复 2214f6e6 → 产物一致性修复 cebbfc10）。
- RED：五向放置被旧 `ItemPlacement` 预检拒绝（reason 4）、支撑移除无法进行（火把放不进世界）、physics 满盒碰撞、companion 防御清单放行火把采掘目标。
- GREEN：sim/world/physics/companion 四包 race 全绿（sim -race -count=2 复核 94.230s）；archcheck ok；openspec strict 通过。
- Ruling: Task 2 边界扩为含 `internal/sim/mining.go`、`internal/companion/plan_types.go`——spec「伙伴不获得火把能力」的 mine 半边因 Task 1 `BlockDrop` 登记被通用判据放行，须显式拒绝（计划生成与模拟执行两处防守，与既有农业拒绝同构）。
- Ruling: 掉落容量不足时整体保留火把（与 `RejectDropCapacity` 同构：静默丢物品是硬失败、悬空火把是可自愈瞬态，下次权威变化重新触发）与邻居区块未加载跳过本轮——两项边界经 repair round 2 写入 spec delta（两个新 Scenario）与 design 取舍记录，并有临界容量钉子测试。
- SPEC 评审：PASS（无 Critical/Important；Minor：容量保留语义入产物——已在 repair 落实、checkbox 同步、热路径分配记录）。QUALITY 评审：初审 FAIL（I1 注释任务编号、I2 产物不一致），repair round 2 后复审 PASS。
- 基线债务上报（不属本变更）：`internal/companion/plan_types.go:467` 历史提交含「（A-01）」字样违反编号禁令，建议独立卫生任务清理；`sweepUnsupportedTorches` 与 `finishChanges` 各自枚举 pending 的可选合并优化。

### Task 3 执行记录

- 实现 commit（amend 终值）：`dc623408 feat: mesh finite torch models`；产物补记 commit：`d00b004e`（控制会话与 repair 合并形态）；基线 `4241f1ea`。
- RED：Rust E0432/E0599（torch 常量与 RegistryView::model 未定义）+ ffi 版本钉子期望 8；Go `BlockProperties.Model` 未定义 + nativeabi ABI 钉子 7→8 必红。
- GREEN：make rust 通过；cargo workspace 333 passed/0 failed（engine 174 + client 159）；mesh -race -count=3 复核 60.9s ok；nativeabi ok；archcheck ok（TestBaselineVersionsMatchCode 转绿）；gofmt/govet 干净。
- 交付：entry 19→20 bytes（model@19）、model 封闭集合 0/1..5/6-拒绝、emit_torch（落地 4 quad 交叉斜面、墙面 3 quad 斜板+贴面帽）、dispatcher model≠0 跳过轴向面、ABI v7→v8 三处同步（ffi.rs/C header/AGENTS.md）、ModelReader 可选接口（assets 未实现回落 tag 0，Task 4 接入）。
- Ruling: 两项实现偏离经控制会话裁决接受并写入产物（implementer 汇报中「经用户裁决」的说法不实，本 ledger 以控制会话裁决为准）：①quad.go 打包断言双向→单向（落地火把复用 face 6/7 交叉斜面编组，与 Rust quad.rs 同口径，被否方案：新开 face 值耗尽 3 位字段、新开 bit 侵犯 bit 63）；②mornlea_client terrain.wgsl 角高度解码门控扩 torch_material(==58) + shaders.rs 常量（斜板斜顶边需要解码半边，D-07 blockTopRaw 先例）。
- SPEC 评审：PASS（Important：3 处旧「双向断言」注释残留——repair 后修正，含 grep 补发的 quad_test.go 共 4 处；Minor：贴面帽近乎不可见已记 design 交 Task 4 golden 把关、design 基线句如实化、逐位不变为间接锁定记录备查）。QUALITY 评审：PASS（Important：ffi.rs 版本注释误记 v8 含上限扩容——repair 后与 header/ledger 对齐；Minor：算术笔误修正、model/fluid/blockTop 互斥未强制（潜伏型，Go assets 造不出该组合，记录上报）、墙面整段上界由逐形态计数覆盖、TORCH_MATERIAL=58 前向引用由 Task 4 登记闭合）。
- 提交拓扑说明：repair amend 期间一度误折控制会话 docs 提交，已按语义重建（feat 与 docs 分立，最终树与错误版本仅差 design.md 去重 1+/2-，git diff 佐证）。

### Task 4 执行记录

- 实现 commit（amend 终值）：`13027d1b feat: paint torches and capture night scene`；基线 `38cdd3b8`。
- RED：LayerTorch/纹理/Model tag 三测失败（层长 0、Model=0）、场景表缺 torch-night、像素断言无场景可跑。
- GREEN：assets/client/cmd 三包 race 全绿（cmd/mornlea 287s 含 GPU 像素测试；SPEC reviewer 复跑 412.5s 亦绿）；archcheck ok；gofmt/govet 干净。
- 交付：`torchTexture`（确定性 hash 噪声、窄柄+暖芯、火芯刻意画第 2 行以下避开斜板裁切）、`LayerTorch`=58（枚举末位、闭合 Task 3 的 TORCH_MATERIAL 前向引用）、`Model` 算术映射 1..5、torch-night 封闭暗室场景（落地+±X 双墙形态、近亮远暗、无漏光像素断言）、golden 仅新增 torch-night.png（20 张口径、其余 19 张逐字节不变、workbench-crafting 缺口保持）。
- 超清单同步（裁决接受）：`internal/assets/pack_test.go` 追加 textures/torch.png 绑定哨兵一行（材质包覆盖槽机制的最小机械同步）；`internal/assets/procedural.go`/`procedural_test.go` 落入 Task 4 语义范围（纹理层实现与测试，修订版清单的合理包含）。
- 事实核实与上报（控制会话在本机 main 上复跑 --capture compare 确认）：main 既有基线债务——inventory-crafting/chest-container/furnace-container 在 main 上 compare 即超阈值（36%/49%/32%）、workbench-crafting golden 缺失、terrain-noon/avatar-nametag/oak-grove/ai-companion/far-horizon/water-underwater 亦有超阈值差异；A-02 用改前/改后逐字节对比证明对全部既有场景渲染零影响。该债务不属本变更，上报后续独立基线任务处理。
- SPEC 评审：PASS（无 Critical/Important；Minor：「三面墙」注释失实——repair 修正为两面墙；阈值机相关观察已记录）。QUALITY 评审：PASS（无 Critical/Important；Minor：ceiling 阈值余量最紧记录、crafting 重置断言为既有共性缺口、纹理前两行约束由场景级间接兜底）。

### 整分支终审与门禁执行记录

- 整分支终审（merge base `cc385d22` → `562a6ed8`，reviewer 独立）：PASS，无 Critical/Important；57 个变更文件逐个判定全部计划内/裁决内；协议 v27、区块 schema v9、世界 metadata v2、玩家 schema v7、companions.ai v4、benchmark scenario v19、client ABI v9 零触碰；engine ABI v7→v8 为唯一版本变更且三处同步；三条 Minor（ledger 措辞微偏差、proposal「延期与放弃」占位句、tasks 措辞历史性偏差）均记录不改代码。
- 门禁修复三连（终审后）：①`cargo fmt` 两处格式 + clippy `too_many_arguments`（emit_wall/emit_standing 坐标合并为 `[i32;3]`，无 allow）→ `d445b2c4`；②`internal/server` 伙伴寻路可通过表漏纳火把（physics 零碰撞的一致性消费者，oracle 测试守护）→ `67fa2c2c`，Task 2 验证面回补 server race；③一次 TestScenarioV12 GPU panic 经复跑 3 次确认为控制会话并发跑两条重型验证管线的资源冲突（非代码问题），此后全部串行执行。
- 教训入 ledger：重型验证管线（全量 race、gates.sh、GPU capture 测试）严禁并发执行；验证输出必须完整审查 FAIL 行（一次全量 race 的 FAIL 被 head 截断漏看）。
- 最终门禁（串行全绿）：`make rust-check`（fmt+clippy+333 tests）；8 包 race（core/assets/mesh/nativeabi/sim/world/client/cmd/mornlea，cmd 429.451s）；`go test ./... -race` exit 0；`go vet ./...`；`gofmt -l .` 空；`git diff --check` 干净；`scripts/agents/gates.sh` 全部门禁通过；`openspec validate --all --strict` 68 passed。
- 版本矩阵终态：协议 v27、玩家 schema v7、区块 schema v9、世界 metadata v2、companions.ai v4、engine ABI v7→**v8**（唯一变更）、client ABI v9、benchmark scenario v19、capture golden 19→**20** 张（torch-night.png 唯一新增）。
- 上报不属本变更的基线债务：main 上 capture compare 既有超阈值（inventory/chest/furnace 36%/49%/32% 等）与 workbench-crafting golden 缺失——A-02 经改前/改后逐字节对比证明对全部既有场景渲染零影响；`internal/companion/plan_types.go:467` 历史任务编号字样。

## 集成重订段（merge main，2026-08-27）

PR #107 因 main 前进 CONFLICTING，worktree `A-02-torches` 执行 merge origin/main（16 个 UU 冲突逐一解决）。控制会话在合并前核定编号重订终值，集成 implementer 按终值执行。

### 撞号与重订终值

main 期间合入：木门批次（方块 62..70、`ItemDoor`=43、`RecipeDoor`=14、`LayerDoor`=55 与工作台三层 56..58——工作台层自 A-01 的 55..57 后移）、B-12（协议 v28→v29：`PlayerState` 尾部追加 `SaturationZero` 提示位）、D-02 暂停菜单、B-30 疾跑、F-05 golden 全面再生（含补齐 `workbench-crafting.png`）。A-02 原始编号与门批次撞号，重订终值（方向不变、仍为 append-only）：

| 项 | A-02 原值（`cc385d22` 基线） | 集成重订终值 |
| --- | --- | --- |
| 火把五形态方块 | 62..66 | **71..75**（门 62..70 之后追加） |
| `BlockIDMax` | 62→67 | **71→76** |
| `ItemTorch` | 43 | **44**（`ItemDoor`=43 之后） |
| `ItemIDMax` | 43→44 | **44→45** |
| `RecipeTorch` | 14 | **15**（`RecipeDoor`=14 之后），`MatchCraftingGrid` 循环上界同步；规划暂缺段 15..18 → 16..18 |
| `LayerTorch` | 58 | **59**（`LayerWorkbenchBottom`=58 之后）；terrain.wgsl `torch_material` 门控、shaders.rs `TORCH_MATERIAL`、Go 层号钉子三处同步 |
| mesh registry entry | 19→20 bytes、model@19、上限 80 | **不变**（main 侧仍 19 bytes/上限 71，方向不变；注释中「当前 67/71 条」更新为 76 条） |
| engine ABI | v7→v8 | **不变**（main 侧仍 v7） |
| 协议表述 | v27 | **v29**（main 现状；AGENTS.md 冲突保留 main 全部新内容，仅 engine ABI v7→v8） |

### 冲突解决要点（按文件）

- `internal/core/block.go`、`item.go`、`recipe.go`、`block_name.go`（及各自测试、`farming_test.go`）：门与火把并存——门枚举/`ItemDoor`/`RecipeDoor`/显示名在前，火把紧随其后；哨兵末项守护统一为 TorchWallNegZID（`BlockIDMax`=76）、`ItemTorch`（`ItemIDMax`=45）、`RecipeTorch`（15 条无空洞）；`farming_test.go` 的 `TestDoorIntervalOrdered` 门区间 62..70 保持、哨兵断言 71→76。
- `internal/assets/blocks.go`、`pack_test.go`：LayerDoor/工作台三层与 `LayerTorch`=59 并存；`Opaque` 同时排除 door 与 torch；材质绑定顺序 door→workbench×3→torch；`pack_test` 打开顺序清单同步。
- `internal/mesh/native_input.go`、`native_input_test.go`：上限保持 80（main 侧曾升至 71），容量夹具沿用「条目数 × 每行字数」推导；注释 67→76 条。
- `engine/crates/mornlea_engine/src/input.rs`：`MAX_REGISTRY_ENTRIES`=80、`REGISTRY_ENTRY_BYTES`=20 保持；注释 67→76 条、火把 model 序号 72..75。
- `internal/physics/types.go`：门碰撞（下半 3/16 薄板、上半零碰撞）与火把零碰撞并存（仅注释合并，函数体自动合并成功）。
- `internal/server/companion_snapshot.go`：passable 表同时含 `DoorUpper`（零碰撞对齐）与五火把形态。
- `AGENTS.md`：保留 main 全部新内容（协议 v29 等），仅 engine ABI v7→v8。
- 附带机械同步（非 UU、编号钉子/注释）：`block_properties_test.go`（71..75/76）、`item_test.go`（44/45、门位次）、`recipe_test.go`（15、暂缺段 16..18）、`blocks_test.go` 层号钉子 59 与 model tag 注释、`torch_test.go` 夹具序号注释、Rust `torch_tests.rs` 夹具 ID 71..75、`terrain.wgsl`/`shaders.rs` 门控 59。

### B-12 兼容性核对

B-12（`5b4b23cb`）的「bounded A zero flag 28→29」是协议侧 `PlayerState` 追加 `SaturationZero` 提示位并把 `ProtocolVersion` v28→v29，与 quad 位布局、AO 字段（bit 39..46）毫无交集；A-02 的 quad.go/quad.rs 注释（bit 13..19 保留位、W/H 借用、`plantReservedMask`、角高度 12..19/55..62、bit 63 留空）经逐条核对仍准确，torch.rs 的 `ao: 0xff` 语义不变。无实质冲突，无需改几何代码。

### F-05/golden 处理

golden 以 merge 自动合入的 main F-05 版本为准（20 张，含 `workbench-crafting.png`）+ A-02 的 `torch-night.png` = 21 张。`LayerTorch` 58→59 只是枚举值位移、纹理内容与画面不变；集成后在无头 compare 模式跑全部 21 个场景，每张「最大通道差 0、差异像素 0/230400」，未再生任何 golden。产物口径同步为 20→21 张（proposal/design/tasks/visual-verification delta）。

### 集成验证

- `make rust`；`cd engine && cargo test --workspace --locked`（全部通过，见下方记录）。
- `go test ./internal/core ./internal/assets ./internal/mesh ./internal/nativeabi ./internal/sim ./internal/world ./internal/physics ./internal/companion ./internal/server ./internal/client ./cmd/mornlea -race -count=1` 全绿。
- `go test ./internal/archcheck -count=1` 绿（版本矩阵 v29/v8 与代码一致）。
- `openspec validate placeable-torches --strict --no-interactive` 通过。
- 无头 capture compare 21 场景全部零差异（见上）。
