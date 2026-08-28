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
- **Task 7（协议/客户端/ABI v10）**：实现提交 `b28a3978`→R1 修复（`ffi.rs` `abi_version_is_nine` 仍断言 9 致 `cargo test -p mornlea_client` 红；改名 `abi_version_is_ten`/断言 10/design D6 kind 3→4 同步）→amend `d93aff50`→R1 复核 PASS。评审 SPEC PASS，四项自报裁决成立（`EntityHostile`=4 系 kind 域实占顺延；CLAUDE.md shim 等价符合 A-04-q1 实质意图；发布「spawn 当 tick 跳 state」与 spec「随后每 tick」措辞相容且与伙伴先例一致；transcript parity 剔除绝对 ServerTick 系 fluid 先例忠实扩展）。修复轮：R1（1 轮，原实现者）。
- **控制会话裁决（受击/追逐呈现，2026-08-28）**：现行渲染契约对夜行者只有固定调色 6-cuboid、无像素级受击/追逐视觉语言；`hostile-mob` 场景以「夹具状态经真实镜像注入（`ApplySpawn`/`ApplyStates`）+ 镜像/呈现层断言 + 布局使受击/追逐个体位置朝向可辨」满足 spec「MUST 可辨认受击与追逐呈现」。裁决：接受，不引入第二套呈现语义；如未来需要像素级受击反馈属新呈现契约，另立行。
- **Task 4（sim 身体/生成/暗度/生命周期）**：实现提交 `5adfd1df`；评审 SPEC PASS + QUALITY PASS，透明格光差异裁决成立（客户端 air-only 是网格呈现域简化、sim 按 D3 单表语义，公共定义域逐位一致经整窗对照锁定、差异域由专项测试钉死）；despawn 清零半径以 spec >64/≤64 为准（tasks 措辞已同步更正）；`TestHostileSpawnReplayIsDeterministic` 双引擎 240 tick 逐字段全等。修复轮：0。遗留交接：坠落移除阈值 `MinY-16` 沿用玩家先例，但 Y∈[MinY−16, MinY) 的个体位置会被存档校验拒绝——Task 6 接通 autosave 前须收紧为 `Y < core.MinY` 即移除（本 ledger 显式记录，Task 6 验收含此项）；完整 `Step()` 双引擎重放变体并入收尾终审。

## 最终验证输出摘要（收尾补）

- `make rust`：通过（exit 0；`cargo build --locked --release` 命中缓存 0.39s）。另实跑 `cargo test -p mornlea_client --release -run abi_version_is_ten`：ok（1 passed）。
- `go test ./... -race -count=1`（全量）：首轮与 `make visual-check`/`go vet`/archcheck 并行执行，cmd/mornlea 撞 10 分钟超时 panic、cmd/mornlea-server SIGTERM 用例与 internal/server 两个用例失败；三个包在无竞争复跑下全部通过（internal/server 221.990s、cmd/mornlea-server 19.725s、cmd/mornlea 370.234s，均 `ok`），失败归因于终审并行验证自身造成的资源竞争、非分支缺陷。随后单独串行复跑全量：**exit 0，26 个包全部 `ok`**（cmd/mornlea 583.936s、internal/server 269.871s、internal/sim 82.479s、internal/storage 33.303s、internal/mesh 54.798s、internal/worldgen 16.806s、internal/fluid 12.022s、其余包 1.6–8.2s）。
- `go test ./internal/archcheck -count=1`：ok 8.068s（含 `TestBaselineVersionsMatchCode`：AGENTS.md 契约段版本行与代码逐项一致）。
- `go vet ./...`：exit 0，无输出。`gofmt -l .`：无输出。`git diff --check`：exit 0，无输出。
- `openspec validate --all --strict --no-interactive`：72 passed, 0 failed。
- `make visual-check`：exit 0；22/22 个场景全部「最大通道差 0，差异像素 0/230400」，其中 `hostile-mob` 场景在 `ai-companion` 与 `water-surface-slope` 之间命中 golden。
- 说明：终审清单所述「五份 delta」实为七份（3 new + 4 modified capability，与 proposal Capabilities 节一致），终审按七份逐一核对。

## 整分支终审（8.3）

对象：`git diff 691a0b7e..6b4b5215`（merge-base `origin/main` 至分支头，98 files，+10669/−127）。逐项结论：

1. **规格合规：PASS。** 七份 delta 的 Requirement/Scenario 逐条对应实现与测试（闭环 sim 侧 `hostile_*_test.go` 六文件、server 侧四文件、storage 三文件、network/client/render/cmd 各一至两文件）；proposal What Changes 十七条与「延期与放弃」六条逐项核对无悄悄突破：benchmark scenario 与 perfcheck 零改动，玩家 schema v7/区块 schema v9/metadata v2/`companions.ai` v4 常量未动（`internal/storage` 仅 `WorldStore` 接口追加 `HostileMobStore` 组合段），damage seam 以授权形态存在（`damageHostileTarget` 注释明示待 A-03 收编），无 ECS/mob registry/中毒。
2. **上限：PASS。** 全服 64（`sim/hostile.go` `maxHostiles`、集合容量、`spawn` 前置拒绝；`storage.MaxHostileMobs`、`network.maxHostileRecords`、`client.MaxHostiles` 同值）、每玩家 8/半径 48（`sim/hostile_spawn.go` `maxHostilesNearPlayer`/`maxHostilesNearRadius`，`TestHostileSpawnRejectsNinthNearAnchorPlayer` 含远处不计入的反向断言）、每 tick 2 快照（`server/hostile_manager.go` `hostileMaxSnapshotsPerTick`，`TestHostileChaseBuildsAtMostTwoSnapshotsSmallestIDsFirst`）、1 候选/tick（`advanceHostileSpawn` 单候选构造 + `findSpawningTick` delta≤1 断言）、4096 nodes（复用 `companion.MaxPathNodes`）、镜像 64（`client.MaxHostiles` + `TestHostileMirrorCapacityIsStableAtSixtyFour`）、75 bodies/450 instances（`render.maxAvatars`=75，`TestAvatarCapacityAcceptsSeventyFiveBodies` 锁 450×80=36000 bytes；Rust `AVATAR_MAX_INSTANCES`=450 同源）、72-byte 记录（`storage.hostileRecordLength`，布局测试逐字段锁位）、4640 文件（`maxHostileFileLength`，解码分配前拒绝，`TestHostileCodecAcceptsMaximumRecordsAndEnforcesFileLimit`）。
3. **worker 不阻 tick：PASS（结构性核查）。** `hostileManager`：快照派发的 semaphore 获取是 select/default 非阻塞（满槽顺延），results channel 容量 2 恰覆盖「在途 ≤2、每 worker 恰发一次、tick 边界先排空再派发」的峰值，worker 发送永不滞留；`advance()` 全程只非阻塞排空。`hostilePersistence`：`Poll` 对 completions 只做非阻塞排空，dispatch 走 select/default，慢 Save 由 worker goroutine 承载（`TestHostilePersistenceDoesNotHoldMutexDuringStoreSave`）；`Flush`/`waitForInflight` 仅在关服屏障路径阻塞且带 ctx 逃逸。专项用例 `TestHostileChaseFullSlotsDeferWithoutBlocking` 以 5s 超时断言满槽 `advance` 立即返回。
4. **spawn 重放：PASS。** `TestHostileSpawnReplayIsDeterministic` 双独立引擎（seed 42）240 个夜间 tick 生成序列逐字段全等（`HostileMob` 值比较覆盖 ID/位置/身体/冷却/目标/累计）；门槛哈希与 ID 同源性由 `TestHostileSpawnGateMatchesHashLowByte` 可观察钉死。ledger Task 4 交接的「完整 `Step()` 双引擎重放变体」裁决为**接受现形态、不再补测**：直驱变体已锁死确定性内核（整数哈希链、锚点选择、候选列），完整 `Step()` 路径的相位编排在 `TestStepSpawnsHostileAtNightWithoutMovingIt` 与 `TestStepRunsHostilePhasesBetweenPhysicsAndFluid` 钉住，跨相位确定性另由 server 端到端与全量模拟测试承载，直驱与全 Step 的生成输入（seed/tick/锚点/区块就绪）完全同源，残余风险可忽略。
5. **暗度 oracle：PASS。** `TestLocalBlockLightMatchesClientOracleOnSharedRuleFixtures` 在只含空气/不透明/发射方块的公共定义域夹具上 29³ 整窗逐格全等；`TestLocalBlockLightDocumentsTransparentCellDivergence` 把透明非空气差异域（服务端按 core 单一表透射、客户端 air-only 网格简化）与取值显式钉死，与 design D3「差异必须记录并裁决」一致；零分配（`AllocsPerRun`=0）与全光源窗口有界性均有专项测试。
6. **schema 错误矩阵：PASS。** storage 侧：`TestHostileCodecRejectsInvalidSaves` 17 例（编码端）+ `TestHostileCodecRejectsCorruptFiles` 29 例（解码端，记录字段补丁全部修复 CRC 使拒绝确实来自字段校验）+ 零 revision/重复 ID/超长/截断/尾随等独立断言，合计逾 50 例；启动错误路径 `hostile_restore_test.go` 覆盖 missing→空集合、valid→首 tick 前恢复、corrupt/future/read error→`NewHost` 失败不启动 tick 与 worker、重复/超 64 不截断恢复；`TestDiskHostileSaveDoesNotOverwriteCorruptOrFutureFile` 与 `TestDiskHostileOversizedFileIsCorruptAndSaveDoesNotOverwriteIt` 锁死「起点拒绝且不覆盖旧文件」。
7. **重启：PASS。** `TestHostileRestartRestoresFullAuthorityAcrossLifetimes` 以 Memory/Disk 双变体跑两段完整宿主生命周期：首 tick 前逐字段恢复（含冷却/目标/DistantTicks）、管理器槽位为空（派生物不跨重启）、目标登录后路径按当前世界重算并应用、关服屏障落盘与引擎终态逐字段一致、第二段恢复与第一段落盘全等（重启不清怪）。
8. **wire 订阅：PASS。** `registry.go` 的 `serverPacketID`/`serverPacketForID` 两处 22/23/24 对称追加、`ValidateServerPacket` 三 case、`codec_server.go` 编解码与分配前 wire 上限四分支齐全；`ProtocolVersion` 29→30 为纯追加（无 C→S 新增、既有 packet 形状与长度上限不变、无新 `RejectReason`）；订阅门控 `hostileCandidateVisible`（脚底 chunk 已订阅且快照已送达）与 `TestHostilePublicationUnsubscribedFootChunkNeverSends`、despawn/spawn/state 时序、`TestMemoryTCPHostilePublicationTranscriptParity` 端到端逐字段 parity（含 spawn=1、state≥ticks−2 的夹具守卫）全部成立。
9. **75-body ABI：PASS。** Go `maxAvatars` 11→75/`avatarInstanceSize` 450 侧与 Rust `AVATAR_MAX_INSTANCES` 66→450 同源（两侧注释互引，Go 测试锁 36000 bytes）；第 76 具在帧边界被 `validateEntityPresentationCounts`（`maxFrameAvatars`=75）稳定拒绝、无部分渲染；client ABI v10 三端一致：Rust `CLIENT_ABI_VERSION=10`（`abi_version_is_ten` 实跑绿）、`mornlea_client.h` `MORNLEA_CLIENT_ABI_VERSION 10u`、Go 侧 `TestClientABIVersionMatchesHeader` 同时断言导出符号与编译期 header 常量为 10（旧库在首个 FFI 入口被 `STATUS_ABI_VERSION` 拒绝）。
10. **无 nametag：PASS。** 专项断言 `TestCaptureHostileMobFixtureIsDeterministicAndTagFree`：8 只夜行者 + 玩家/伙伴经与 `renderFrame` 相同的装配函数后 `tags` 必须为空且全部身体 `Kind==EntityHostile`；`appendHostileRenderPresentationsInto` 结构上只写实体通道；名标容量常量（`maxNameTags`=12）未动。capture 场景 `hostile-mob` 位置、8 只夹具、受击/追逐值、场景清理均有断言。
11. **无版权资源：PASS。** 全分支新增二进制仅两个：`cmd/mornlea/testdata/golden/hostile-mob.png`（程序化 capture 产物，torch-night 先例允许）与 `internal/storage/testdata/hostile-mobs-v1.bin`（codec golden 夹具）；调色为原创数值（`hostileBaseColor`/`hostileHeadColor`），`TestHostileAvatarUsesOriginalPaletteAndProportions` 断言与玩家/伙伴共用调色板全部槽位不重合且注释明示原创；无任何 Mojang 材质或第三方美术入库。
12. **受击/追逐呈现裁决复核：相容（维持原裁决）。** 控制会话裁决（镜像注入 + 布局可辨，不引入像素语言）与 visual-verification delta「夹具确定性且无名标」场景的「MUST 可辨认受击与追逐呈现」不冲突：现行渲染契约对夜行者只有固定调色 6-cuboid（proposal 客户端呈现段即如此圈定，无受击闪烁/追踢动画语义），「可辨认」在该契约下的唯一可满足形态就是夹具经真实权威消息入口（`ApplySpawn`/`ApplyStates`）注入受击（ID 103 生命 13、位姿变化）与追逐（ID 107 逼近相机、yaw=π）状态、以投影布局使其一眼可辨，并由字节级 golden 永久钉死画面；像素级受击反馈属新呈现契约，按裁决另立行。判定：spec 措辞在实现契约内可达且已达成，不构成规格突破。
13. **注释纪律：PASS。** 全 diff 新增行中 `[A-F]-[0-9]{2}` 编号在 `.go`/`.rs`/`.h` 代码文件为 **0 例**（grep 实证）；命中全部落在 OpenSpec change 产物（proposal/design/ledger/tasks，规则允许的规划层）。新增注释全中文、以意图/边界/取舍行文、Go/Rust 标识符反引号包裹（抽查全部 hostile 相关文件与 Rust diff 均合规）。
14. **并行行边界：PASS。** 与 A-05（床与睡眠）的文件交集如proposal所限：`internal/core` 全部为追加段（`BlockOpaque` 新函数、`FoodValue` 新 case、`ItemRottenFlesh` 插于 `ItemIDMax` 哨兵前、`day_phase.go` 新文件），`registry.go` 两侧均为 case 列表末尾追加；`dayPhaseOffset` 为零值字段、无 setter，消费点仅 `advanceHostileSpawn` 与 `advanceHostileBurn` 两处，与「本行消费恒 0、A-05 提供生产端」的共享契约一致。

**终审总结论：PASS（可进 PR）。** 上述第 4、5 项中两个历史遗留交接（完整 Step 重放变体、64 只满载字面用例）按本节第 1、3 项所引结构性证据裁决为现形态可接受，不构成分支合并前置条件；Task 3/5/6 评审记录中「留归档期参考」的非阻塞建议维持原分类。验证输出摘要见上方小节。

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
