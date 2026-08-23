# Authoritative Bed Sleep Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task with fresh implementers, independent SPEC/QUALITY reviews, and a ledger. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付可合成、四向双格原子放置/破坏的原创白床，以及服务端权威多人睡眠、跳到黎明和持久床复活点，同时保持单调模拟时间不跳变。

**Architecture:** 床用 8 个稳定 BlockID 表达 foot/head×四方向，因同区块限制可在现有 pending changes 中原子写两格；Rust finite model emitter 只生成 9/16 高的原创床体。`sim` 为 active 玩家保存瞬态 sleeping 状态，所有 active 且已出生玩家同时 sleeping 时只更新 `DayTimeOffsetTicks`；`WorldTimeTicks` 仍加一。metadata v3 保存 offset，player schema v8 保存精确 bed foot；死亡后的既有 PendingSpawn 流程异步加载并验证床，再选安全邻格。

**Tech Stack:** Go 1.26、Rust 1.97.1 finite block model、二进制 metadata/player storage、Memory/TCP、OpenSpec。

**Spec:** `docs/superpowers/specs/2026-08-23-first-night-survival-parallel-wave-design.md`

## Global Constraints

- 基于 batch 共享契约 SHA；bed Rust emitter 可并行实现为独立模块，最终 dispatcher 接线在 integration 分支完成。
- 批次执行时本计划 Task 1 先在 integration 分支创建并评审，功能 worktree 从共享契约提交继续并直接从 Task 2 开始；独立执行本计划时才在本分支创建 Task 1 产物。
- 床 foot/head 必须位于同一 chunk；跨 chunk 明确拒绝，不建立通用跨区块事务。
- 两格都必须是 air、下方各为 solid、非 fluid；成功时两格写入和扣 1 bed 同 tick 原子完成。
- 床碰撞高度固定 9/16，非 opaque、非发光、非 fluid；破坏任一半只掉 1 个 bed。
- 睡眠窗口 display phase 13000..23000；附近 8 格内任一 living nightwalker 阻止入睡。
- active 集合只含已出生在线玩家；PendingSpawn 不计票。伤害实际扣血、床被破坏或断线都会结束相关 sleeping 状态。
- 不做彩床、占床锁、梦境、白天小憩、睡眠百分比配置、躺姿远端同步或通用多方块事务。

---

## Task 1：建立 OpenSpec change

**Files:**

- Create: `openspec/changes/authoritative-bed-sleep/.openspec.yaml`
- Create: `openspec/changes/authoritative-bed-sleep/proposal.md`
- Create: `openspec/changes/authoritative-bed-sleep/design.md`
- Create: `openspec/changes/authoritative-bed-sleep/tasks.md`
- Create: `openspec/changes/authoritative-bed-sleep/ledger.md`
- Create: `openspec/changes/authoritative-bed-sleep/specs/authoritative-bed-sleep/spec.md`
- Create: `openspec/changes/authoritative-bed-sleep/specs/authoritative-daylight/spec.md`
- Create: `openspec/changes/authoritative-bed-sleep/specs/local-data-migration/spec.md`
- Create: `openspec/changes/authoritative-bed-sleep/specs/authoritative-health/spec.md`
- Create: `openspec/changes/authoritative-bed-sleep/specs/authoritative-crafting/spec.md`
- Create: `openspec/changes/authoritative-bed-sleep/specs/rust-engine-mesh/spec.md`
- Create: `openspec/changes/authoritative-bed-sleep/specs/visual-verification/spec.md`

- [ ] **Step 1: 基线验证**

  若已存在共享提交，用 `shared_sha=$(git log -1 --format=%H --grep='^feat: reserve first-night survival contracts$')` 和 `git merge-base --is-ancestor "$shared_sha" HEAD` 验证；若尚不存在，本 Task 必须位于 `codex/first-night-survival-integration`。运行 `make rust`、`go test ./internal/core ./internal/storage ./internal/sim ./internal/server ./internal/render ./cmd/mornlea -race -count=1`，记录 ledger。

- [ ] **Step 2: 写可判定 specs**

  覆盖 8 个方块形态、同 chunk 原子放/拆、支撑、睡眠门禁/计票/唤醒、offset 跳夜、deadline/crop hash 不变、metadata v2→v3、player v7→v8、床复活验证/fallback、v27 wire 与 `bed-sleep`。

- [ ] **Step 3: 固化方向与黎明公式**

  yaw 量化后 head 指向玩家视线的最近水平轴；点击位置是 foot。若本 tick 末世界时间为 `next=t+1`，全员睡眠时：

  ```go
  DayTimeOffsetTicks = uint16((24000 - next%24000) % 24000)
  ```

  因而该 tick 发布的 display phase 精确为 0，而 `WorldTimeTicks==next`。

- [ ] **Step 4: strict validate、双评审并提交**

  运行 `openspec validate --all --strict --no-interactive`、`git diff --check`，提交 `docs: propose authoritative bed sleep`。

## Task 2：实现床方块配对、放置、破坏和碰撞

**Files:**

- Modify: `internal/core/block.go`
- Modify: `internal/core/block_name.go`
- Modify: `internal/core/item.go`
- Modify: `internal/core/item_test.go`
- Create: `internal/core/bed.go`
- Create: `internal/core/bed_test.go`
- Modify: `internal/physics/types.go`
- Create: `internal/physics/bed_collision_test.go`
- Modify: `internal/sim/engine_placement.go`
- Modify: `internal/sim/mining.go`
- Modify: `internal/sim/engine_step.go`
- Create: `internal/sim/bed.go`
- Create: `internal/sim/bed_test.go`
- Modify: `internal/sim/placement_success_test.go`
- Modify: `internal/sim/mining_test.go`

- [ ] **Step 1: 写 8 形态双向映射失败测试**

  `BedPart(block)` 返回 foot/head、方向与 counterpart offset；对每个 block，移动 offset 得到配对形态且反向恰回原格。非 bed 返回 false。`ItemWhiteBed` stack64、不可由普通 `ItemPlacement` 单格放置，8种 block drop 归一到一个 bed。

- [ ] **Step 2: 实现固定 switch 和 9/16 collision**

  添加 `IsBed`/`BedPart`/`BedBlocks(direction)`；`physics.BlockCollisionBoxes` 对 8 种 bed 返回单盒 `{0,0,0}..{1,9/16,1}`。不建立 multiblock interface。

- [ ] **Step 3: 写原子放置失败测试**

  四方向逐一验证：foot/head 两格 air、两支撑 solid、同 chunk、loaded，并沿用现有玩家放置的身体碰撞判定；任一条件失败时零 world write、零 inventory decrement、零 `PlacementSuccess`。成功时两 change 同一 revision batch、恰减一 bed。

- [ ] **Step 4: 在既有 placement 中加入 bed 专用分支**

  只允许命中 `BlockFacePosY`；foot 取普通 adjacent target，方向从 yaw 量化，head=foot+offset；用两格局部副本预验后连续 `recordChange`，最后扣物品。跨 chunk 在读取/写入前拒绝 `RejectInvalidBed`。

- [ ] **Step 5: 写破坏/支撑失效失败测试**

  破坏完整 pair 任一半：同 tick 两格 Air、只创建 1 bed drop；若 counterpart 缺失/方向不互认，只移除命中的 bed half且仍只掉1，绝不删除无关块。任一支撑格被其他系统移除时，完整 pair 同样拆除一次。

- [ ] **Step 6: 复用 pending change 做最小支撑复核**

  对本 tick 改变的支撑位置只检查正上方一格；若是 bed，调用同一个 `removeBedPair`。同一 pair 用规范 foot pos 去重，避免两支撑同 tick 消失时双掉落。

- [ ] **Step 7: focused 验证与提交**

  运行 `gofmt -w internal/core internal/physics internal/sim`、`make rust`、`go test ./internal/core ./internal/physics ./internal/sim -race -count=1`；双评审后提交 `feat: place paired beds atomically`。

## Task 3：实现 bed finite model 与原创材质

**Files:**

- Modify: `internal/assets/blocks.go`
- Modify: `internal/assets/blocks_test.go`
- Create: `internal/mesh/bed_test.go`
- Create: `engine/crates/mornlea_engine/src/greedy/bed.rs`
- Create: `engine/crates/mornlea_engine/src/greedy/bed_tests.rs`
- Modify: `engine/crates/mornlea_engine/src/greedy/mod.rs`
- Modify: `engine/crates/mornlea_engine/src/greedy/test_support.rs`

- [ ] **Step 1: 写原创材质失败测试**

  生成 foot top、head top（含原创枕形）、cloth side、wood frame 四个 deterministic 16×16 cutout/opaque cells；非空、互异、无外部 Mojang 图片。8 blocks 的 materials/model tag 精确登记。

- [ ] **Step 2: 写 bed emitter 失败测试**

  每半床范围 Y=0..9/16；完整 pair 隐藏相接内侧面，各发5个外表面 quad；孤半床发6面；四方向旋转后 top UV/枕端与 head 方向一致。quad8 bytes、bit63=0、光/AO沿用当前格邻域。

- [ ] **Step 3: 实现独立 `emit_bed`**

  直接用现有 quad pack helper；不改全局 dispatcher/engine ABI（由 torches 分支拥有）。在 `greedy/mod.rs` 仅登记模块供直接 unit tests；integration 在 ABI v7 dispatcher 中把8个 bed tags接到该函数。

- [ ] **Step 4: focused 验证与提交**

  运行 `make rust`、`gofmt -w internal/assets internal/mesh`、`go test ./internal/assets ./internal/mesh -race -count=1`；双评审后提交 `feat: mesh finite bed models`。

## Task 4：实现 metadata v3 的 display day offset

**Files:**

- Modify: `internal/storage/types.go`
- Modify: `internal/storage/metadata.go`
- Modify: `internal/storage/metadata_test.go`
- Modify: `internal/storage/metadata_worldtime_test.go`
- Modify: `internal/storage/world_files_test.go`
- Modify: `internal/sim/engine.go`
- Modify: `internal/sim/engine_step.go`
- Modify: `internal/sim/player.go`
- Modify: `internal/sim/worldtime_test.go`
- Modify: `internal/server/server.go`
- Modify: `internal/server/publication.go`
- Modify: `internal/server/persistence_metadata.go`
- Modify: `internal/server/metadata_persistence_test.go`
- Modify: `internal/server/metadata_restart_test.go`

- [ ] **Step 1: 写 metadata v3 codec 失败测试**

  payload 28→30 bytes，末尾追加 `DayTimeOffsetTicks u16`，仅0..23999；v2/v1 decode 后在内存规范为 v3、offset0、NeedsRewrite/首次保存写 v3；future、坏长度、bad CRC、offset24000拒绝。

- [ ] **Step 2: 实现 metadata migration**

  保留 v1/v2 frozen fixtures；只追加 v3 encoder/decoder 分支，`Metadata.FormatVersion` 当前值3。更新 Memory/Disk atomic save contract，不改 world seed/spawn/time 字节。

- [ ] **Step 3: 把 offset 纳入权威 tick 出口**

  `Engine` 构造接收 offset，`TickResult`/`PlayerUpdate` 返回同 tick 末值；server metadata save job 同时 latest-wins 合并 `WorldTimeTicks` 和 offset，不能保存跨 tick 拼接的一新一旧字段。

- [ ] **Step 4: 证明单调时间不受 offset 影响**

  测试 MaxUint64 附近 phase helper、连续 Step 世界时间恰+1、crop random hash/companion deadline 只读 `WorldTimeTicks`，display daylight/nightwalker只读 `DisplayDayPhase(time,offset)`。

- [ ] **Step 5: focused 验证与提交**

  运行 `gofmt -w internal/storage internal/sim internal/server`、`go test ./internal/storage ./internal/sim ./internal/server -race -count=1`；双评审后提交 `feat: persist display day offset`。

## Task 5：实现 player schema v8 床复活点

**Files:**

- Modify: `internal/storage/player_types.go`
- Modify: `internal/storage/player_codec.go`
- Modify: `internal/storage/player_migration.go`
- Modify: `internal/storage/player_codec_test.go`
- Modify: `internal/storage/player_codec_fuzz_test.go`
- Modify: `internal/storage/player_migration_test.go`
- Create: `internal/storage/testdata/player-v8.bin`
- Modify: `internal/sim/player.go`
- Modify: `internal/sim/persistence.go`
- Modify: `internal/sim/death.go`
- Modify: `internal/sim/spawn.go`
- Create: `internal/sim/bed_respawn_test.go`
- Modify: `internal/server/player_persistence_snapshot.go`
- Modify: `internal/server/player_persistence_helpers_test.go`

- [ ] **Step 1: 写 v8 追加布局失败测试**

  v7 payload 末尾追加固定17 bytes：`HasBed u8 + Dimension i32 + X/Y/Z i32`；无 bed 时16个坐标字节必须为0。v7→v8迁移为 nil，v1..v7 fixtures继续读，future/invalid dimension/Y/flag/unused bytes拒绝。

- [ ] **Step 2: 扩持久值链**

  `StoredPlayer`/`PlayerSave`/DTO/`PlayerRestore`/`PlayerSnapshot` 加 `BedSpawn *BedSpawnPoint`，clone深拷贝；进入有效睡眠时记录规范 foot pos并置 player dirty。当前 `Safe` 字段保持原语义，不能复用成 bed point。

- [ ] **Step 3: 写死亡异步验证失败测试**

  bed chunk未加载时死亡进入 PendingSpawn并请求该唯一 chunk；加载后完整 reciprocal pair才有效。围绕两半的六个外侧水平格按 `(X,Z)` 字典序选择第一个 feet/head clear且下方solid的点；无安全点、床缺失/错配时回落既有世界 spawn。

- [ ] **Step 4: 接入既有 restore candidate 流程**

  `beginReset` 把 bed candidate 放在 world spawn 前；候选验证读取同 chunk 两半与安全邻格，不同步加载、不传送进未就绪块。成功重生不清 BedSpawn；床之后被破坏时清除在线拥有者的对应点并置脏。

- [ ] **Step 5: golden/fuzz 与提交**

  生成独立 v8 fixture，验证 v7 fixture bytes不变；运行 `gofmt -w internal/storage internal/sim internal/server`、`go test ./internal/storage ./internal/sim ./internal/server -race -count=1`；双评审后提交 `feat: persist bed respawn points`。

## Task 6：实现权威睡眠状态机与跳夜

**Files:**

- Create: `internal/sim/sleep.go`
- Create: `internal/sim/sleep_test.go`
- Modify: `internal/sim/command.go`
- Modify: `internal/sim/engine_step.go`
- Modify: `internal/sim/player.go`
- Modify: `internal/sim/combat.go`
- Modify: `internal/sim/eating.go`
- Modify: `internal/sim/container.go`
- Modify: `internal/sim/mining.go`
- Modify: `internal/sim/death.go`
- Modify: `internal/server/session_ingress.go`
- Create: `internal/server/bed_sleep_parity_test.go`

- [ ] **Step 1: 写 UseBed 门禁失败测试**

  权威 raycast必须命中完整床任一半并规范到foot；phase 13000/23000含端点，12999/23001拒绝；living hostile距任一半≤8拒绝，dead/更远不拒绝；本分支通过 `tryUseBed(..., hostileNearby bool)` 直接测试真假，integration 用真实 hostile snapshot计算该 bool。

- [ ] **Step 2: 添加精确 reject reasons**

  追加 ID 13..16：`RejectNotNight`、`RejectHostileNearby`、`RejectInvalidBed`、`RejectSleeping`；更新 sim/network枚举映射、string和codec测试。`UseBed` 非有限朝向/零sequence在network边界拒绝。

- [ ] **Step 3: 写睡眠输入隔离失败测试**

  sleeping 玩家后续移动输入被归零，不能jump/mining/attack/eat/open/move container/drop/place/till；命令返回 `RejectSleeping` 且不改状态。实际扣血通过 `applyDamage` 立即wake；其床pair被移除wake；session unregister不留下计票。

- [ ] **Step 4: 实现最小状态机**

  `playerState` 只加 `sleeping bool` 与 `sleepBedFoot core.BlockPos`；`tryUseBed` 成功时清输入/交互状态、设sleeping并保存BedSpawn。不要建睡眠manager。

- [ ] **Step 5: 写多人计票/跳夜失败测试**

  1人入睡同 tick末phase0并wake；3人中2 sleeping不跳，第三人 sleeping后一次跳；PendingSpawn排除、active断线后剩余全睡可跳；全员跳夜只更新offset，`WorldTimeTicks`仍+1、companion deadline/crop抽样读数不变。

- [ ] **Step 6: 在 Step 末时间推进前结算票数**

  用已排序 active sessions固定扫描；若非空且全 sleeping，先算 `nextWorldTime=WorldTimeTicks+1` 对应 offset，再让现有 `advanceWorldTime` 恰执行一次，最后wake全部。不得直接 Store/Add 23999 到 worldTime。

- [ ] **Step 7: Memory/TCP parity 与提交**

  同一两玩家脚本逐 tick比较 offset、sleeping、position、world time、reject reason、metadata save；运行 `gofmt -w internal/sim internal/server`、`go test ./internal/sim ./internal/server -race -count=1`，双评审后提交 `feat: skip night when players sleep`。

## Task 7：完成 v27 wire、客户端昼夜与睡眠 UI

**Files:**

- Modify: `internal/network/message_player.go`
- Modify: `internal/network/message_bed.go`
- Create: `internal/network/message_bed_test.go`
- Modify: `internal/network/codec_client.go`
- Modify: `internal/network/codec_server.go`
- Modify: `internal/network/packet.go`
- Modify: `internal/network/registry.go`
- Modify: `internal/network/worldtime_test.go`
- Modify: `internal/network/codec_fuzz_test.go`
- Modify: `internal/server/publication.go`
- Modify: `cmd/mornlea/app.go`
- Modify: `cmd/mornlea/app_input.go`
- Modify: `cmd/mornlea/app_messages.go`
- Modify: `cmd/mornlea/app_celestial_test.go`
- Modify: `internal/render/daylight.go`
- Modify: `internal/render/daylight_test.go`
- Modify: `internal/render/hud/layout.go`
- Modify: `internal/render/hud/layout_test.go`

- [ ] **Step 1: 写 PlayerState 尾部布局失败测试**

  在 `WorldTimeTicks` 后追加 offset u16、sleeping u8；offset 0..23999、sleeping布尔。旧 v26握手拒绝；截断/尾随/offset24000/flag2拒绝。

- [ ] **Step 2: 接通 UseBed ID15**

  codec/registry/ingress使用 `Sequence,Yaw,Pitch` 固定16 bytes；客户端在准星命中任一bed half且按既有交互键时发送，不本地预测 sleeping/offset/respawn。

- [ ] **Step 3: 客户端只用 display phase**

  `render.DayNightAt` 接收 phase或`(time,offset)`并使用 overflow-safe helper；app只接受最新 `PlayerState` 同时更新time+offset，sky/cloud/fog共享该相位。远环/worldgen seed与simulation tick不受影响。

- [ ] **Step 4: 复用 HUD glyph pass 显示等待**

  `Sleeping=true` 时在屏幕中心显示原创中文“等待其他玩家入睡…”和半透明暗层；false时零额外实例。单人立即跳夜通常看不到持久 overlay，capture使用两玩家仅一人睡的固定状态。

- [ ] **Step 5: focused 验证与提交**

  运行 `gofmt -w internal/network internal/server internal/render cmd/mornlea`、`go test ./internal/network ./internal/server ./internal/render/... ./cmd/mornlea -race -count=1`；双评审后提交 `feat: synchronize bed sleep`。

## Task 8：配方、视觉构造与功能线终审

**Files:**

- Modify: `internal/core/recipe.go`
- Modify: `internal/core/recipe_test.go`
- Modify: `cmd/mornlea/capture_scene.go`
- Modify: `cmd/mornlea/capture_scene_test.go`
- Modify: `openspec/changes/authoritative-bed-sleep/tasks.md`
- Modify: `openspec/changes/authoritative-bed-sleep/ledger.md`

- [ ] **Step 1: 完成 white bed shape recipe**

  三个white wool横排、其下三个plank横排输出1 bed；网格平移允许，竖放/倒置/缺料/多余材料失败。

- [ ] **Step 2: 加 `bed-sleep` 场景构造**

  固定黄昏室内一张完整四向可辨的床、sleep overlay、display phase与床复活点状态；测试锁定pair/模型/布局/场景位置，本分支不写golden。

- [ ] **Step 3: 运行功能线验证**

  ```bash
  make rust
  go test ./internal/core ./internal/physics ./internal/assets ./internal/mesh ./internal/network ./internal/storage ./internal/sim ./internal/server ./internal/render/... ./cmd/mornlea -race -count=1
  go test ./internal/archcheck -count=1
  openspec validate --all --strict --no-interactive
  git diff --check
  ```

- [ ] **Step 4: 独立终审并移交**

  reviewer 核对双格原子性、同chunk限制、pair损坏防御、offset公式/metadata一致快照、v8迁移、异步床复活、全员计票与输入隔离；写ledger后把SHA交给integration controller，不自行接nightwalker、model dispatcher、golden或main。
