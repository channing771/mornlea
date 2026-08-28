# Ledger：authoritative-hostile-nightwalker（A-04）

> 记录：控制会话裁决（ruling）、每 Task 评审结论与修复循环、最终验证输出摘要。

## 内容确认记录（brainstorming 硬门禁，2026-08-25）

- **分类**：architectural（新实体子系统 + 新存档 schema + 新协议消息 + 客户端 ABI 升版 + Rust 呈现改动）。
- **探索**：批次设计 `docs/superpowers/specs/2026-08-23-first-night-survival-parallel-wave-design.md` 任务四、计划 `docs/superpowers/plans/2026-08-23-authoritative-hostile-nightwalker.md`；全仓现状核对（Explore 子代理报告）：无物理批处理 API（per-actor `physics.Step`）、发光/衰减表在 `assets`/`mesh` 且 `sim` 不可导入、`splitmix64` 在 `internal/sim/crop.go`、avatar `maxAvatars=11`（66 实例，每帧 GPU 上限）、client ABI v8、协议 v26、`WorldTimeTicks` 无偏移、后端存储原语/CRC/原子写先例齐全。
- **Ruling（控制会话既有裁决，经 A-02-q1/q2 卡片取得并采用）**：批次各分支自建 PR 不合并（待集成），A-06 按固定顺序合流；分支可对两份基线文档做最小同步（只改本人负责 ABI 的版本行、两份逐字节一致），其余基线归集成。
- **Ruling: A-04-q1（2026-08-25T10:45:00Z，approve）** — client ABI 本分支直接升 v9（Rust/Go 常量与容量拒绝同步），并按 A-02-q2 先例对两份基线仅同步 client ABI 版本行 — 为什么：批次设计要求「旧动态库不得在运行到敌怪帧后才迟发拒绝」，主基线已 v8（egui 占用）故实际新值 v9；唯一例外的合理载体是实际改动的分支。与 A-07 的版本基线独占不冲突（A-07 只补其余行）。
- **Ruling: A-04-q2（2026-08-25T10:52:33Z，answer A）** — 本分支在 `internal/core` 引入 `BlockEmission`/`BlockLightAttenuation` 单一表（按现有 assets/mesh 值迁移，二者改为委托 core；若 A-02 契约先行则消费其表不重复创建） — 为什么：`sim` 只能依赖 `core`/`companion`/`fluid`/`physics`/`world`，暗度判定规则必须落 core；批次设计任务二把 `core.BlockEmission` 单一表归 A-02，故本分支仅在 A-02 未落地时创建并保持值一致。
- **批准轮 A-04-approval（2026-08-25T10:54:16Z，approve「批准」）** — 按节呈现的设计（§1 范围 / §2 数据所有权 / §3 关键裁决 / §4 固定上限 / §5 验证 / §6 不做）经用户显式批准；结论已誊入本 change 的 proposal/design 与 tasks。

## 重定基线裁决（2026-08-28，控制会话 brainstorming）

- **Ruling: A-04-rebase-1（approve）** — 批次合流模式正式弃用（原 A-06/A-07 集成职责已拆回各功能行并标记取消）：本行改为自包含直接合并，协议 v29→v30、client ABI v9→v10、`hostile_mobs` v1、golden（21→22 张口径）与两份基线文档版本行由本行自带同步；benchmark scenario 不动。为什么：批次模式的「PR 不合并、版本不动、golden 延后」前提随 A-06/A-07 取消失效，现行约定以 A-02（协议 v29 内自带 engine ABI v8、torch-night 场景与 golden）为先例。
- **Ruling: A-04-rebase-2（approve）** — 消息编号改取 S→C 22/23/24（21 已被 A-01 `CraftingState` 实占；原设计预留 21/22/23 已撞号）；实现期以注册表实占空闲位为准，与并行行撞号由后合并者重订（A-02 撞号重订先例）。
- **Ruling: A-04-rebase-3（approve）** — `core.BlockEmission`/`core.BlockLightAttenuation` 已由现行 `internal/core/block_properties.go` 提供（A-02 落地）：按原 D2 预设判据走「直接消费、不重复创建」路径，本行只新增 `core.BlockOpaque` 单一表并把 assets/mesh 不透明谓词改为委托。
- **Ruling: A-04-rebase-4（approve）** — 与并行行 A-05（床与睡眠）解耦互动：睡眠不查询夜行者，跳夜后白昼灼烧规则自然生效；两行唯一共享契约为 `core.DisplayDayPhase(ticks, offset)`（本行交付并消费、offset 恒 0，A-05 后续提供 offset 生产端）。战斗 seam 保留，待 A-03 统一战斗落地后收编删除。
- **分支操作**：分支 `feat/A-04-hostile-nightwalker` 已 rebase 到 `origin/main`（`fe3890ed`），原两提交（proposal 23df0525 + rulings c96a6851）重放为 69e5c1f0 + 717cd3e7；重定基线文档修订以新提交追加。

## 变更产物

- [x] `openspec new change authoritative-hostile-nightwalker`；proposal/7 delta specs/design/tasks/ledger 已建。
- Ruling: 本分支产物提交到功能分支（本 worktree）而非 main — 为什么：控制会话裁决（A-02-q1 路径 A）批次分支自包含；claims 类 docs-only 提交才上 main。

## 评审记录（Task 1 起，逐 Task 追加）

- **Task 1（基线验证）**：完成，提交 `de1e4e1e`。`make rust` 与 11 包 `-race` 全绿（数值见下方验证小节）；事实核对 a–f 与重定基线裁决一致（S→C 22/23/24 空闲、`BlockEmission`/`BlockLightAttenuation` 在 core、`BlockOpaque`/`DisplayDayPhase` 缺位、`ItemTorch`=44、`BlockIDMax`=76、client ABI 9、协议 29）。控制会话抽查通过。
- **Task 2（core 单一表/显示相位/腐肉）**：实现提交 `74e999b2`；评审 SPEC PASS + QUALITY PASS。`DisplayDayPhase(worldTime uint64, offset uint16) uint16` 签名与「先 `%24000` 再相加取模」语义经独立复算锁定（MaxUint64+23999 分水岭用例）；`BlockOpaque` 与迁移前 `Registry.Opaque` 逐值恒等（判据含门/火把排除，design D2 括注已补齐，提交 `8668a312`）；腐肉=45、食物表精确五食物；assets 转调 core、mesh 经接口天然单一源。非阻塞建议（D2 括注）已落实，其余两条留归档期参考。修复轮：0。
- **Task 3（hostile_mobs.bin 存储契约）**：实现提交 `ef85bf96`；评审 SPEC PASS + QUALITY PASS。spec 16 类错误矩阵逐例拒绝归因复核为真（`repairHostileCRC` 保证拒绝来自字段校验）；Memory/Disk 同构契约测试、原子写四处故障注入、backup 过滤临时文件；fuzz 10s 97.8 万 execs 0 失败；「起点拒绝且不覆盖旧文件」落在 `DiskStore.LoadHostileMobs`/`SaveHostileMobs`，可被 Task 6 直接复用。加强项（revision=0 拒绝、冷却 ≤20、Distant ≤600、UUIDv4、dimension 白名单）与 spec 相容。非阻塞建议三条留归档期参考（`putF32NoRepair` 文档注释措辞、防御断言标注、目录 fsync 失败语义的 spec 措辞区分）。修复轮：0。

- **Task 6（持久化装配/启动恢复/错误路径）**：实现提交 `586452a1`；评审 SPEC PASS + QUALITY PASS，三项自报裁决成立（`newWorld` 改 error 签名影响面最小且 panic 口径有先例；夜行者持久化仅 `NewHost` 装配系类型约束且 nil 路径全守卫；Sync/rename 注入在 storage 侧覆盖五阶段+目录 fsync 语义、server 侧目录只读注入闭环），MinY 交接修复验收完成（`Y < core.MinY` 与存储校验闭合、red→green）。非阻塞建议一条升级处理：`NewHost` 在持久化 worker 启动后出错返回的 goroutine 泄漏（`hostiles.Close()` 缺失）——并入 Task 7 修复清单。其余三条留归档期参考。修复轮：0。
- **Task 5（server 追逐 worker/路径执行/damage seam）**：实现提交 `c7200884`；评审 SPEC PASS + QUALITY PASS，四项自报裁决全部核实成立（`TargetSession` 系攻击寻址必要补充且 sim 侧同维/存活/保护期/距离全重验；20 tick 重规划周期与满载 ~32 tick 轮转推算正确；编排次序刷新→应用→派发→执行系「进入攻击距离同 tick 冻结意图」的充分保障且结构性无阻塞；`companion_snapshot.go` 提取为必要共享、companion 路径行为零变化）。设计外最小新增：`EnqueueHostileAction`（inbox cap 64）、`PlanHostileChase` 权威通道、`HostileAction.TargetSession` 字段。非阻塞建议三条留归档期参考（`buildChaseGrid` 注释措辞、64 只字面满载用例并入终审、跨区块角移动保守重排注释）。修复轮：0。
- **Task 4（sim 身体/生成/暗度/生命周期）**：实现提交 `5adfd1df`；评审 SPEC PASS + QUALITY PASS，透明格光差异裁决成立（客户端 air-only 是网格呈现域简化、sim 按 D3 单表语义，公共定义域逐位一致经整窗对照锁定、差异域由专项测试钉死）；despawn 清零半径以 spec >64/≤64 为准（tasks 措辞已同步更正）；`TestHostileSpawnReplayIsDeterministic` 双引擎 240 tick 逐字段全等。修复轮：0。遗留交接：坠落移除阈值 `MinY-16` 沿用玩家先例，但 Y∈[MinY−16, MinY) 的个体位置会被存档校验拒绝——Task 6 接通 autosave 前须收紧为 `Y < core.MinY` 即移除（本 ledger 显式记录，Task 6 验收含此项）；完整 `Step()` 双引擎重放变体并入收尾终审。

## 最终验证输出摘要（收尾补）

- （待整分支终审后补：make rust、focused -race、archcheck、vet、gofmt、openspec strict 的数值摘要；benchmark 数值只记录）

## Task 1 基线验证（2026-08-28）

### 验证命令结果（数值只记录，不改基线）

- `git status --short`：空输出，worktree 干净（位于本 change 的功能分支，未切换 checkout）。
- `make rust`：通过（exit 0）。`cargo build --locked --release`（rustup 1.97.1）完成于 29.19s，`mornlea_engine` 与 `mornlea_client` 均编译成功。注：命令外壳先输出十余条 `_encode`/`_decode: command not found`，来自用户 zsh profile 初始化，与 make 及 cargo 无关。
- `go test ./internal/core ./internal/companion ./internal/physics ./internal/sim ./internal/server ./internal/storage ./internal/network ./internal/client ./internal/render ./internal/nativeabi ./cmd/mornlea -race -count=1`：通过（exit 0），11 个包全部 `ok`。各包耗时（`-race`）：core 2.235s、companion 4.585s、physics 2.566s、sim 55.579s、server 254.491s、storage 23.170s、network 7.608s、client 5.649s、render 4.462s、nativeabi 7.870s、cmd/mornlea 373.466s（合计约 741.7s）。
- `openspec validate --all --strict --no-interactive`：通过（exit 0），72 items passed, 0 failed。
- `git diff --check`：通过（exit 0，无输出）。

### 事实核对

- a. `internal/network/registry.go`：StatePlay S→C 在 `serverPacketID`/`serverPacketForID` 均实占 0..21，21 为 `CraftingState`；22/23/24 空闲，可承接本 change 的三类敌怪消息。
- b. `internal/core/block_properties.go` 已有 `BlockEmission`（发光方块 15、五种火把形态 14）与 `BlockLightAttenuation`（八个流体编号 1、其余 0）；`internal/core` 全包无 `BlockOpaque`——符合 design「直接消费两表 + 只新增 `BlockOpaque`」的预设。
- c. `internal/core`（乃至整个 `internal/`）尚无 `DisplayDayPhase`，待 Task 2.3 新增。
- d. `internal/core/item.go` 物品枚举末项为 `ItemTorch` = 44（`ItemNone` = iota = 0 起第 45 项，其后是哨兵 `ItemIDMax` = 45）；`internal/core/block.go` 的 `BlockIDMax` = 76（`AirID` = 0 起 iota 第 77 项，独占上界，末个合法方块为 `TorchWallNegZID` = 75）；`internal/core/recipe.go` 配方段末为 `RecipeTorch` = 15（`RecipeStoneBricks` = iota+1 = 1 起第 15 项）。
- e. `internal/render/avatar.go` 的 `maxAvatars` = 11；Rust 侧 `engine/crates/mornlea_client/src/render/entity.rs` 的 `AVATAR_MAX_INSTANCES: usize = 66`；`engine/include/mornlea_client.h` 的 `MORNLEA_CLIENT_ABI_VERSION 9u`，与 client ABI v9 预期一致。
- f. `internal/network/packet.go` 的 `ProtocolVersion uint32 = 29`，与「协议 v29→v30 由本行升级」的起点一致。

### 产物一致性核对

- ledger「内容确认记录」与「重定基线裁决」两节已全文誊录既有批准结论（q1/q2、批准轮与 2026-08-28 重定基线各裁决），与 proposal/design/tasks 现文逐项一致：批次合流取消、本行自带协议 v30/client ABI v10/`hostile_mobs` v1/golden 同步、S→C 编号取 22/23/24（实现期以注册表实占空闲位为准）、发光/衰减表走「直接消费 + 只新增 `BlockOpaque`」路径、与并行床行仅共享 `DisplayDayPhase(ticks, offset)`（本行 offset 恒 0）。
- 上述事实核对 a–f 与 design「Context」记载的基线逐项吻合，未发现偏差。
- 结论：Task 1 通过，无需修复，可进入 Task 2。
