# Authoritative Hostile Nightwalker Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task with fresh implementers, independent SPEC/QUALITY reviews, and a ledger. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付一种原创、服务端权威、可近战追逐玩家、白昼燃烧、死亡掉落并跨重启持久化的敌对生物“夜行者”，形成第一夜可验证的威胁闭环。

**Architecture:** `sim` 拥有最多 64 个按 ID 排序的夜行者身体、生命与攻击事实，统一走 Rust physics；`server` 的 bounded manager 每 tick 最多裁两份不可变 A* 快照给 worker，复用 `internal/companion` 的整数寻路并以 generation/latest-wins 回收结果；spawn 每 tick 最多验证一个确定性候选。独立 `hostile_mobs.bin` schema v1 原子保存完整恢复事实。协议按玩家订阅发布 bounded spawn/state/despawn，客户端复用扩容后的 avatar pass 但不生成 nametag。

**Tech Stack:** Go 1.26、既有 companion A*、Rust physics、二进制 storage/network、Rust wgpu avatar pass、OpenSpec。

**Spec:** `docs/superpowers/specs/2026-08-23-first-night-survival-parallel-wave-design.md`

## Global Constraints

- 基于 batch 共享契约 SHA；战斗分支合流前只测试夜行者自身伤害入口，最终必须删除临时入口并复用统一 combat settlement。
- 批次执行时本计划 Task 1 先在 integration 分支创建并评审，功能 worktree 从共享契约提交继续并直接从 Task 2 开始；独立执行本计划时才在本分支创建 Task 1 产物。
- 固定上限：全服 64 只、每玩家 48 格内 8 只、每 tick 1 个 spawn candidate、每 tick 2 个 A* snapshot、单次 A* 4096 nodes、每消息 64 records、总 avatar 75 bodies/450 instances。
- 固定数值：20 health、3 damage、1.8 格攻击距离、20 tick 攻击冷却；白昼露天每 20 tick 受 1 点燃烧伤害；距所有 active 玩家超过 64 格累计 600 active ticks 后 despawn；死亡固定掉 1 个 rotten flesh。
- spawn 只在 display phase 13000..23000、水平距选中玩家 24..48、局部 block light ≤7、双格空气、下方 solid、非 fluid、完整 loaded cell 时成立。
- rotten flesh 恢复 hunger 4、saturation 0；复用现有进食状态机，不加中毒或通用 effect。
- 不做刷怪笼、难度、装备、门、群体战术、远程攻击、声音感知、ECS 或任意 mob registry。

---

## Task 1：建立 OpenSpec change

**Files:**

- Create: `openspec/changes/authoritative-hostile-nightwalker/.openspec.yaml`
- Create: `openspec/changes/authoritative-hostile-nightwalker/proposal.md`
- Create: `openspec/changes/authoritative-hostile-nightwalker/design.md`
- Create: `openspec/changes/authoritative-hostile-nightwalker/tasks.md`
- Create: `openspec/changes/authoritative-hostile-nightwalker/ledger.md`
- Create: `openspec/changes/authoritative-hostile-nightwalker/specs/authoritative-hostile-nightwalker/spec.md`
- Create: `openspec/changes/authoritative-hostile-nightwalker/specs/hostile-mob-persistence/spec.md`
- Create: `openspec/changes/authoritative-hostile-nightwalker/specs/hostile-mob-protocol/spec.md`
- Create: `openspec/changes/authoritative-hostile-nightwalker/specs/authoritative-health/spec.md`
- Create: `openspec/changes/authoritative-hostile-nightwalker/specs/authoritative-hunger/spec.md`
- Create: `openspec/changes/authoritative-hostile-nightwalker/specs/rust-client-render-entities/spec.md`
- Create: `openspec/changes/authoritative-hostile-nightwalker/specs/visual-verification/spec.md`

- [ ] **Step 1: 基线验证**

  若已存在共享提交，用 `shared_sha=$(git log -1 --format=%H --grep='^feat: reserve first-night survival contracts$')` 和 `git merge-base --is-ancestor "$shared_sha" HEAD` 验证；若尚不存在，本 Task 必须位于 `codex/first-night-survival-integration`。运行 `make rust`、`go test ./internal/companion ./internal/storage ./internal/sim ./internal/server ./internal/render ./cmd/mornlea -race -count=1`，记录 ledger。

- [ ] **Step 2: 写可判定 specs**

  覆盖确定性 spawn、暗度/空间/距离/上限、追逐/重验、攻击/燃烧/死亡/掉落/despawn、schema v1 错误矩阵、重启、每会话订阅消息、插值、无 nametag、client ABI v8 和 `hostile-mob` 场景。

- [ ] **Step 3: 固化持久记录**

  ```go
  type HostileMobID = core.HostileMobID

  type HostileBody struct {
      ID HostileMobID
      Dimension core.DimensionID
      State physics.State
      Yaw float32
      Health uint8
      AttackCooldownTicks uint8
      HurtCooldownTicks uint8
      BurnTicks uint8
      HasTarget bool
      Target core.PlayerID
      NextRepathTick uint64
      DistantTicks uint16
  }
  ```

  路径和 worker generation 不落盘；恢复后路径为空、`NextRepathTick=0` 立即重算。

- [ ] **Step 4: strict validate、双评审并提交**

  运行 `openspec validate --all --strict --no-interactive`、`git diff --check`，提交 `docs: propose authoritative nightwalkers`。

## Task 2：实现 `hostile_mobs.bin` schema v1

**Files:**

- Create: `internal/storage/hostile_types.go`
- Create: `internal/storage/hostile_codec.go`
- Create: `internal/storage/hostile_codec_test.go`
- Create: `internal/storage/hostile_codec_fuzz_test.go`
- Create: `internal/storage/hostile_store_test.go`
- Modify: `internal/storage/types.go`
- Modify: `internal/storage/memory.go`
- Modify: `internal/storage/disk.go`
- Modify: `internal/storage/world_files.go`
- Modify: `internal/storage/world_files_test.go`
- Modify: `internal/storage/backup.go`
- Modify: `internal/storage/backup_test.go`

- [ ] **Step 1: 写 codec 布局失败测试**

  文件固定为 32-byte envelope header + 最多 64 条 72-byte record，最大 4640 bytes；magic `MHST`、envelope v1、schema v1、revision u64、count u16、payload length u32、CRC32。记录按 ID 严格升序且非零。

- [ ] **Step 2: 写字段校验失败矩阵**

  拒绝：future schema/envelope、截断/尾随、bad CRC、count>64、重复/逆序/零 ID、未知 dimension、NaN/Inf body/yaw、health 0 或 >20、非法 bool、无目标却带 PlayerID、有目标却非 UUIDv4、cooldown/burn/despawn 越界、world Y 越界。解码 payload 必须读空。

- [ ] **Step 3: 实现固定 binary codec**

  使用现有 byte encoder/decoder/CRC helper；不使用 JSON、gob 或 reflection。`physics.State` 只编码 position、velocity、onGround；浸没标志每 tick 从世界重算，不落盘；保留字段必须为零。

- [ ] **Step 4: 实现 Memory/Disk store contract**

  `HostileMobStore` 提供 `LoadHostileMobs`/`SaveHostileMobs`；missing 返回独立 `ErrHostileMobsNotFound`。Disk 使用同目录 temp+fsync+rename、0600、revision conflict 与损坏正式文件保护，路径固定 `hostile_mobs.bin`；WorldStore 组合新接口，backup 精确复制正式文件并忽略 temp。

- [ ] **Step 5: fuzz/故障注入与提交**

  运行 `gofmt -w internal/storage`、`go test ./internal/storage -race -count=1`；双评审后提交 `feat: persist hostile mobs`。

## Task 3：实现 sim 夜行者身体、spawn、局部暗度与生命周期

**Files:**

- Create: `internal/sim/hostile.go`
- Create: `internal/sim/hostile_test.go`
- Create: `internal/sim/hostile_spawn.go`
- Create: `internal/sim/hostile_spawn_test.go`
- Create: `internal/sim/block_light_query.go`
- Create: `internal/sim/block_light_query_test.go`
- Modify: `internal/sim/engine.go`
- Modify: `internal/sim/engine_step.go`
- Modify: `internal/sim/command.go`
- Modify: `internal/sim/drop.go`
- Modify: `internal/sim/tunables.go`
- Modify: `internal/core/hunger.go`
- Modify: `internal/core/hunger_test.go`

- [ ] **Step 1: 写固定集合与 Rust physics 失败测试**

  插入/恢复按 ID 排序，重复/第65只拒绝；active interest 内按玩家→伙伴→夜行者、夜行者 ID 顺序送入现有 Rust physics batch；碰撞/水中状态与玩家使用同一 tunables，输出逐体写回。

- [ ] **Step 2: 实现最小 `hostileState` slice**

  `Engine` 持有 capacity 64 的排序 slice 和复用 scratch；添加 restore/snapshot/action 测试入口，不建 map/ECS。夜行者用与玩家相同 AABB，移动 input 只含水平与 jump。

- [ ] **Step 3: 写 display phase helper 失败测试**

  在 `core` 添加 overflow-safe `DisplayDayPhase(worldTime uint64, offset uint16) uint16`：先 `%24000` 再相加取模；本分支调用 offset 0，bed 分支接持久 offset。边界覆盖 MaxUint64 和 offset 23999。

- [ ] **Step 4: 写局部 block light 失败测试**

  半径 14、emission 14 torch/15 light block、每步固定减 1 加 `BlockLightAttenuation`、opaque 阻挡、fluid 额外减 1、unknown/unloaded 当阻挡；与 Rust light oracle 的小夹具结果一致。重复调用 allocations=0。

- [ ] **Step 5: 实现一次候选用的预分配 16-bucket BFS**

  `Engine` 保存 29³ light values/visited/bucket scratch；每次只扫描候选半径方块，调用 `core.BlockEmission`/`BlockLightAttenuation` 与现有 collision/opaque 查询。不保存跨 tick/world cache。

- [ ] **Step 6: 写 spawn 决定失败测试**

  active sessions 排序后用 `WorldTimeTicks % playerCount` 选 anchor；先以 `splitmix64(uint64(seed)^tick)` 产生半径/轴向并得到候选坐标，再依次把 `uint32(X)`、`uint32(Z)` 和 `uint32(Y)` 混入新的 splitmix64 值作为 spawn hash；候选水平距离必须落在24..48，且仅 hash 低8位<13时尝试。测试 night/day、距离、双格空间、solid support、fluid、loaded、light7/8、global64、nearby8、相同输入重放和每tick读取候选≤1。

- [ ] **Step 7: 实现 spawn 与稳定 ID**

  ID 取同一 hash 的非零结果；若与现存 ID 冲突，最多按 splitmix64 重散列 64 次，仍冲突则本 tick 不 spawn。spawn 直接加入排序 slice，初始 health20/cooldown0/path empty。

- [ ] **Step 8: 写燃烧/despawn/死亡掉落失败测试**

  露天白昼每20 tick扣1，遮顶/夜间重置 burn counter；距全部 active 玩家>64时 `DistantTicks` 加1，回到范围清零，第600 tick despawn；health0 同 tick移除并用既有 `PrepareDropBatch` 在死亡 chunk 放1 rotten flesh，槽满则按已排序 Ready chunk 环形尝试，全部满时确定性省略掉落但仍完成死亡。

- [ ] **Step 9: 扩展食物表**

  `FoodValue(ItemRottenFlesh)=(4,0,true)`，stack64；现有 `advanceEating` 原样消费，不新增状态效果。更新“只有面包”穷举测试为精确两种食物。

- [ ] **Step 10: focused 验证与提交**

  运行 `gofmt -w internal/core internal/sim`、`make rust`、`go test ./internal/core ./internal/sim -race -count=1`；双评审后提交 `feat: simulate hostile nightwalkers`。

## Task 4：复用 companion A* 实现有界追逐与近战意图

**Files:**

- Create: `internal/server/hostile_manager.go`
- Create: `internal/server/hostile_manager_test.go`
- Create: `internal/server/hostile_snapshot.go`
- Create: `internal/server/hostile_path_worker.go`
- Create: `internal/server/hostile_path_worker_test.go`
- Modify: `internal/server/server.go`
- Modify: `internal/server/host.go`
- Modify: `internal/server/shutdown.go`
- Modify: `internal/sim/hostile.go`
- Create: `internal/sim/hostile_combat_test.go`

- [ ] **Step 1: 写目标与预算失败测试**

  每只夜行者选择最近 active 同维 living player，等距按 PlayerID 字节序；每 tick 只为 ID 最小且到期的两只构造 path snapshot，其余延后；snapshot 覆盖 33×9×33 和 revisions，第三份不得读取世界。

- [ ] **Step 2: 实现两槽 worker**

  复用 `companion.NewPathGrid`/`FindPath`，channel capacity 2；请求携带 mob ID+generation，结果在 tick 边界按 ID 排序应用，旧 generation/target/revision 变化丢弃。权威 tick 只做快照，不等待 A*。

- [ ] **Step 3: 写路径执行失败测试**

  目标超出窗口时 goal 钳到朝玩家方向的窗口边缘可站立格；每个 waypoint 前重验 revisions 和当前 cell；失效清 path 并把 `NextRepathTick` 设为下一 tick。到 1.8 格内停止移动并冻结一次 attack intent；无路径时不穿墙直线移动。

- [ ] **Step 4: 实现最小 action 接口**

  manager 每 tick 提交按 ID 排序的 `HostileAction{MoveX,MoveZ,Jump,AttackTarget}`；sim 在物理前消费，攻击只形成 tagged intent。本分支用夜行者专用 damage test seam 验证3 damage/20 cooldown；最终 integration 删除 seam 并接统一 combat settlement。

- [ ] **Step 5: focused 验证与提交**

  运行 `gofmt -w internal/server internal/sim`、`go test ./internal/companion ./internal/server ./internal/sim -race -count=1`；双评审后提交 `feat: drive bounded nightwalker AI`。

## Task 5：接通异步持久化、启动恢复与错误路径

**Files:**

- Create: `internal/server/hostile_persistence.go`
- Create: `internal/server/hostile_persistence_test.go`
- Create: `internal/server/hostile_restore_test.go`
- Modify: `internal/server/host.go`
- Modify: `internal/server/server.go`
- Modify: `internal/server/shutdown.go`
- Modify: `internal/server/persistence_status.go`
- Modify: `internal/server/host_shutdown_test.go`

- [ ] **Step 1: 写启动错误矩阵**

  missing→空集合；valid→在第一 tick 前恢复；corrupt/future/read error→Host 构造失败且不启动 tick/path worker；重复/超64不能被截断恢复。

- [ ] **Step 2: 复用 companion persistence 状态机形状**

  写一个专用、短小的 hostile persistence worker：容量1 jobs/completions、revision、dirty/inFlight/retry、autosave tick、Flush/Close；保存输入是 sim 已排序值快照。不要抽取 companion/hostile 通用 generic persistence。

- [ ] **Step 3: 写非阻塞与故障注入测试**

  慢 Save 不持 mutex/不阻 tick；失败按现有 config retry；新快照在 in-flight 时合并为 latest；shutdown flush 最新身体；context cancel/Sync/rename error 保留旧正式文件并返回错误。

- [ ] **Step 4: 重启端到端**

  Memory 与 Disk 都验证位置、速度、health、cooldowns、target、despawn tick 恢复；path 不恢复且首 tick排入重算。进程重启不得把 mobs 当 missing 清空后覆盖旧文件。

- [ ] **Step 5: focused 验证与提交**

  运行 `gofmt -w internal/server`、`go test ./internal/storage ./internal/server -race -count=1`；双评审后提交 `feat: restore hostile nightwalkers`。

## Task 6：协议、客户端插值和 75-body avatar pass

**Files:**

- Modify: `internal/network/message_hostile.go`
- Create: `internal/network/message_hostile_test.go`
- Create: `internal/network/message_hostile_fuzz_test.go`
- Modify: `internal/network/codec_server.go`
- Modify: `internal/network/packet.go`
- Modify: `internal/network/registry.go`
- Create: `internal/server/hostile_publication.go`
- Create: `internal/server/hostile_publication_test.go`
- Create: `internal/client/hostiles.go`
- Create: `internal/client/hostiles_test.go`
- Modify: `internal/render/avatar.go`
- Modify: `internal/render/avatar_test.go`
- Modify: `cmd/mornlea/app.go`
- Modify: `cmd/mornlea/app_messages.go`
- Modify: `cmd/mornlea/app_render.go`
- Modify: `cmd/mornlea/presentation_conversion_test.go`
- Modify: `internal/client/window.go`
- Modify: `internal/client/window_test.go`
- Modify: `engine/crates/mornlea_client/src/render/entity.rs`
- Modify: `engine/crates/mornlea_client/src/ffi.rs`
- Modify: `engine/crates/mornlea_client/src/lib.rs`

- [ ] **Step 1: 写 bounded wire 失败测试**

  三类消息各含 `ServerTick u64 + count u8 + ≤64 records`，严格 ID 升序；spawn 是 ID/dimension/position/yaw/health，state 是 ID/position/velocity/yaw/health，despawn 是 ID。拒绝重复/逆序/零ID、NaN/Inf、非法 health/dimension、count65、截断/尾随。

- [ ] **Step 2: 实现 per-session publication**

  每会话只镜像其已订阅 Ready chunks 内 mobs；进入视野发 spawn，持续发本 tick state，离开/死亡发 despawn。每类每 tick至多一包64条；IDs排序。Memory/TCP transcript 必须一致。

- [ ] **Step 3: 实现 latest-wins 客户端镜像**

  固定64 records，spawn 建立、state 只接受更新 tick、despawn 删除；未知 state 拒绝并请求下一 spawn，不隐式造实体。插值复用 remote player/companion 的时间边界。

- [ ] **Step 4: 扩 avatar 容量并升 client ABI v8**

  `maxAvatars/maxFrameAvatars` 11→75，instances 66→450；Go/Rust upload size、indirect offset、容量错误和 ABI constant 同步。新增 `EntityHostile`，ID u64 little-endian 写入 16-byte key；build parts 对 hostile 使用固定原创暗青/灰紫 palette 与不同头身比例，仍恰好6 cuboids。nametag 集合保持最多12且永不加入 hostile。

- [ ] **Step 5: Rust/Go 容量和 ABI 验证**

  恰75 bodies/450 instances 成功，第76失败；错误 ABI 所有入口拒绝；frame buffer/FFI length 精确一致，预热后零动态资源。

- [ ] **Step 6: focused 验证与提交**

  运行 `gofmt -w internal/network internal/server internal/client internal/render cmd/mornlea`、`make rust`、`go test ./internal/network ./internal/server ./internal/client ./internal/render ./internal/nativeabi ./cmd/mornlea -race -count=1`；双评审后提交 `feat: present hostile nightwalkers`。

## Task 7：视觉构造与功能线终审

**Files:**

- Modify: `cmd/mornlea/capture_scene.go`
- Modify: `cmd/mornlea/capture_scene_test.go`
- Modify: `openspec/changes/authoritative-hostile-nightwalker/tasks.md`
- Modify: `openspec/changes/authoritative-hostile-nightwalker/ledger.md`

- [ ] **Step 1: 加 `hostile-mob` 场景构造**

  固定夜间火把边缘展示 8 只夜行者，其中一只受击、一只追逐；测试断言无 nametag、实体数、health/state 和 scene 位置。本分支不写 golden。

- [ ] **Step 2: 运行功能线验证**

  ```bash
  make rust
  go test ./internal/core ./internal/companion ./internal/network ./internal/storage ./internal/sim ./internal/server ./internal/client ./internal/render ./internal/nativeabi ./cmd/mornlea -race -count=1
  go test ./internal/archcheck -count=1
  openspec validate --all --strict --no-interactive
  git diff --check
  ```

- [ ] **Step 3: 独立终审并移交**

  reviewer 核对所有硬上限、worker不阻tick、spawn重放、暗度算法、schema错误矩阵、重启、wire订阅、75-body ABI和无nametag；写 ledger 后把 SHA 交给 integration controller，不自行合 main、不更新 golden。
