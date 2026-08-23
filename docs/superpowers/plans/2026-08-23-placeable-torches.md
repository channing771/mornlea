# Placeable Torches Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task with fresh implementers, independent SPEC/QUALITY reviews, and a ledger. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付可合成、可站立或四向挂墙、支撑失效会掉落并发出 14 级方块光的原创火把，同时为火把和床提供严格有限的 Rust 方块模型标签。

**Architecture:** `core` 是放置形态、支撑方向、碰撞/透明/发光属性的唯一事实源；`sim` 在现有原子放置与 pending block changes 上校验支撑，并只检查发生变化方块的六个邻居；Go mesh registry 给 Rust 传一个封闭 model byte，Rust greedy emitter 针对火把生成少量 cutout quads，继续走 terrain pass 和既有 8-byte quad。服务端夜行者后续用同一 `core.BlockEmission` 和有界局部 BFS 判暗，不维护第二套光源表。

**Tech Stack:** Go 1.26、Rust 1.97.1 `mornlea_engine`、既有 mesh/light/terrain pass、OpenSpec。

**Spec:** `docs/superpowers/specs/2026-08-23-first-night-survival-parallel-wave-design.md`

## Global Constraints

- 基于 batch 共享契约 SHA；只实现已冻结的 5 个 torch model tag，bed tag 留给 bed 分支。
- 批次执行时本计划 Task 1 先在 integration 分支创建并评审，功能 worktree 从共享契约提交继续并直接从 Task 2 开始；独立执行本计划时才在本分支创建 Task 1 产物。
- engine ABI v6→v7；quad 仍为 8 bytes，bit 63 仍空闲，不新增 GPU pass、instance pool 或动态资源。
- 火把零碰撞、非 opaque、非 fluid、天空光额外衰减 0、固定 emission 14；不可放水中、天花板或无实心支撑处。
- 不做彩色光、手持光源、熄灭火把、火焰粒子、任意模型描述语言或光照缓存。
- 纹理由现有程序化 atlas/原创资源生成，不复制 Mojang 火把图片。

---

## Task 1：建立 OpenSpec change

**Files:**

- Create: `openspec/changes/placeable-torches/.openspec.yaml`
- Create: `openspec/changes/placeable-torches/proposal.md`
- Create: `openspec/changes/placeable-torches/design.md`
- Create: `openspec/changes/placeable-torches/tasks.md`
- Create: `openspec/changes/placeable-torches/ledger.md`
- Create: `openspec/changes/placeable-torches/specs/placeable-torches/spec.md`
- Create: `openspec/changes/placeable-torches/specs/static-block-light/spec.md`
- Create: `openspec/changes/placeable-torches/specs/rust-engine-mesh/spec.md`
- Create: `openspec/changes/placeable-torches/specs/authoritative-crafting/spec.md`
- Create: `openspec/changes/placeable-torches/specs/visual-verification/spec.md`

- [ ] **Step 1: 证明共享基线并跑 focused tests**

  若已存在共享提交，用 `shared_sha=$(git log -1 --format=%H --grep='^feat: reserve first-night survival contracts$')` 和 `git merge-base --is-ancestor "$shared_sha" HEAD` 验证；若尚不存在，本 Task 必须位于 `codex/first-night-survival-integration`。运行 `make rust`、`go test ./internal/core ./internal/assets ./internal/mesh ./internal/sim -race -count=1`，把证据写入 ledger。

- [ ] **Step 2: 写 Requirement/Scenario**

  覆盖火把配方、五种放置、拒绝面、水/替换性、支撑移除掉落、属性、光传播、有限 model tag、engine ABI v7、固定容量以及 `torch-night` 场景。明确 chunk schema 与 client ABI 不变。

- [ ] **Step 3: 固化方向映射**

  `BlockFacePosY` 命中上表面→standing；`NegX/PosX/NegZ/PosZ` 命中侧面→同名 wall variant，variant 的支撑格位于 `face.Opposite()` 方向；`NegY` 拒绝。墙火把外观向远离支撑的方向倾斜，但碰撞仍为空。

- [ ] **Step 4: strict validate、双评审并提交**

  运行 `openspec validate --all --strict --no-interactive`、`git diff --check`，提交 `docs: propose placeable torches`。

## Task 2：把方块属性集中到 core

**Files:**

- Modify: `internal/core/block.go`
- Modify: `internal/core/block_name.go`
- Create: `internal/core/block_properties.go`
- Create: `internal/core/block_properties_test.go`
- Modify: `internal/core/item.go`
- Modify: `internal/core/item_test.go`
- Modify: `internal/assets/blocks.go`
- Modify: `internal/assets/blocks_test.go`

- [ ] **Step 1: 写属性唯一事实源失败测试**

  穷举 0..<`BlockIDMax`，断言 `BlockEmission` 返回 0..15、只有 `LightBlockID=15` 与五种 torch=14；`BlockLightAttenuation` 对八个 water=1、其余=0；五种 torch 非 opaque、无 collision、可被 raycast 瞄准。

- [ ] **Step 2: 实现两个固定 switch**

  添加 `BlockEmission(BlockID) uint8` 与 `BlockLightAttenuation(BlockID) uint8`；`assets.Registry.Emission/LightAttenuation` 直接转调，删除 assets 内重复 switch。未知 ID 返回 0，注册完整性由穷举测试兜底。

- [ ] **Step 3: 写放置/掉落映射失败测试**

  `ItemTorch` 栈上限 64；`BlockDrop` 的五种 torch 都返回一支 torch；`PlaceableBlock` 不能无视 face 返回单一 block，新增 `PlaceableBlockAtFace(ItemID, BlockFace)` 精确映射，既有立方体物品对任意合法 face 仍返回原 block。

- [ ] **Step 4: 最小实现并复用所有调用点**

  修改玩家放置使用新 helper；伙伴防御清单继续拒绝 torch，不为伙伴扩 scope。不得在 sim 再写 item→torch switch。

- [ ] **Step 5: focused 验证与提交**

  运行 `gofmt -w internal/core internal/assets`、`go test ./internal/core ./internal/assets -race -count=1`；双评审后提交 `feat: register torch block properties`。

## Task 3：实现支撑放置和六邻居失效

**Files:**

- Modify: `internal/sim/engine_placement.go`
- Modify: `internal/sim/block.go`
- Modify: `internal/sim/engine_step.go`
- Create: `internal/sim/torch.go`
- Create: `internal/sim/torch_test.go`
- Modify: `internal/sim/drop_test.go`
- Modify: `internal/sim/placement_success_test.go`
- Modify: `internal/world/chunk.go`

- [ ] **Step 1: 写五向放置失败测试**

  对 standing 和四面 wall 分别断言：目标格 loaded/replaceable、支撑格 solid、非 fluid、玩家不碰撞、成功时世界写入与扣一物品同 tick 原子完成；无支撑、天花板、水格、未知 chunk、玩家占位全部拒绝且不扣物品。

- [ ] **Step 2: 实现 `torchSupport` 与 placement 分支**

  `torchSupport(block, pos) (supportPos, bool)` 是唯一形态→支撑映射；`executePlacement` 在现有 world write 前调用，不建立 block behavior interface。

- [ ] **Step 3: 写支撑移除失败测试**

  覆盖玩家采掘、伙伴采掘、流体/作物阶段的 block replacement 和直接测试 pending change；改变支撑后只移除确实依赖该格的相邻 torch，生成一支持久掉落，并与原变化共享 revision/broadcast/save。改变非支撑邻居不影响 torch。

- [ ] **Step 4: 在 pending changes 上追加有界复核**

  对本 tick 已改变位置排序去重，每个位置检查精确六邻居；若邻居是 torch 且 `torchSupport` 指回改变格，则通过既有 `recordChange` 写 Air 并调用既有 drop append。新移除的 torch 不会支撑另一火把，因此不需要通用递归队列。

- [ ] **Step 5: focused 验证与提交**

  运行 `gofmt -w internal/sim internal/world`、`go test ./internal/sim ./internal/world -race -count=1`；双评审后提交 `feat: enforce torch support`。

## Task 4：实现 Rust 有限模型与 engine ABI v7

**Files:**

- Modify: `internal/mesh/registry.go`
- Modify: `internal/mesh/native_input.go`
- Modify: `internal/mesh/native_abi.go`
- Modify: `internal/mesh/native_abi_test.go`
- Modify: `internal/mesh/native_capacity_test.go`
- Modify: `internal/mesh/native_input_test.go`
- Create: `internal/mesh/torch_test.go`
- Modify: `internal/nativeabi/native.go`
- Modify: `internal/nativeabi/native_test.go`
- Modify: `engine/crates/mornlea_engine/src/input.rs`
- Modify: `engine/crates/mornlea_engine/src/greedy/mod.rs`
- Create: `engine/crates/mornlea_engine/src/greedy/model.rs`
- Create: `engine/crates/mornlea_engine/src/greedy/torch_tests.rs`
- Modify: `engine/crates/mornlea_engine/src/greedy/test_support.rs`
- Modify: `engine/crates/mornlea_engine/src/ffi.rs`

- [ ] **Step 1: 先写 ABI/输入失败测试**

  锁定 registry entry 19 bytes、64 entries、model offset 18、未知 tag 失败、engine ABI 7；Go/Rust 同一 64 项容量夹具必须跨 FFI 成功，第 65 项在 Go 侧失败。

- [ ] **Step 2: 写火把 quad 失败测试**

  每种 variant 在全空气邻域发固定数量的双面 cutout quad；standing 竖直居中，wall variant 贴近对应支撑面且向外倾；坐标全部在本格范围，material/light/AO 来自 registry/邻域，面不参与 greedy merge。解包后 quad 仍 8 bytes、bit63=0。

- [ ] **Step 3: 实现最小 model dispatcher**

  cube 继续走既有 greedy，plant 继续走既有交叉片；五种 torch 调 `emit_torch(model, ...)`。bed tag 在本分支 registry 中不会出现，dispatcher 对它返回现有 invalid/unsupported input error，待 bed 分支实现；不创建 trait/object hierarchy。

- [ ] **Step 4: 更新 ABI 和所有 Rust fixtures**

  `mornlea_engine_abi_version()` 返回 7；更新 C header/Go constant、输入文档、status 映射和所有硬编码 `ENTRY_BYTES`。运行 `make rust` 和 Rust unit tests。

- [ ] **Step 5: Go/Rust parity 与提交**

  运行 `go test ./internal/mesh ./internal/nativeabi -race -count=1`，核对 Go 解包对 model quad 的材料/光值，双评审后提交 `feat: mesh finite torch models`。

## Task 5：火把纹理、配方、客户端与视觉构造

**Files:**

- Modify: `internal/assets/blocks.go`
- Modify: `internal/assets/blocks_test.go`
- Modify: `internal/core/recipe.go`
- Modify: `internal/core/recipe_test.go`
- Modify: `internal/client/mesher_test.go`
- Modify: `cmd/mornlea/capture_scene.go`
- Modify: `cmd/mornlea/capture_scene_test.go`

- [ ] **Step 1: 写原创 atlas 层失败测试**

  五种 torch 共用一个 16×16 alpha-cutout material，像素确定、alpha 仅 0/255、非空且与现有层不同；禁止加入外部 Mojang PNG。

- [ ] **Step 2: 生成最小原创像素并登记 model**

  在现有程序化 material builder 画窄木柄、暖色火芯；`assets.Registry.Model` 对五种 variant 返回冻结 tag，其他块保持 cube/plant 规则。

- [ ] **Step 3: 完成 torch recipe 测试**

  在 crafting 已有 shape registry 上确认 coal 位于 stick 上方时输出 4，水平/倒置/多余材料失败；不新增 torch 专用合成路径。

- [ ] **Step 4: 加 `torch-night` 场景构造**

  固定夜晚房间中同时出现 standing 与至少两种 wall torch，像素测试证明火把附近亮度高于远处且透明边缘不是实心矩形。本分支不写 golden。

- [ ] **Step 5: focused 验证与提交**

  运行 `gofmt -w internal/assets internal/core internal/client cmd/mornlea`、`make rust`、`go test ./internal/assets ./internal/core ./internal/client ./cmd/mornlea -race -count=1`；双评审后提交 `feat: present placeable torches`。

## Task 6：功能线终审

**Files:**

- Modify: `openspec/changes/placeable-torches/tasks.md`
- Modify: `openspec/changes/placeable-torches/ledger.md`

- [ ] **Step 1: 运行功能线验证**

  ```bash
  make rust
  go test ./internal/core ./internal/assets ./internal/mesh ./internal/nativeabi ./internal/sim ./internal/client ./cmd/mornlea -race -count=1
  go test ./internal/archcheck -count=1
  openspec validate --all --strict --no-interactive
  git diff --check
  ```

- [ ] **Step 2: 独立终审并移交**

  reviewer 核对支撑原子性、六邻居上限、光属性唯一来源、19-byte/64-entry 双端一致、ABI v7、无新增 pass/资源和原创纹理；写 ledger 后把 SHA 交给 integration controller，不更新 golden、不合 main。
