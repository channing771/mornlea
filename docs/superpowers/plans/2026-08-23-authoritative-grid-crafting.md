# Authoritative Grid Crafting Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task with fresh implementers, independent SPEC/QUALITY reviews, and a ledger. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 用服务端权威的个人 2×2 与工作台 3×3 实物网格替换 recipe-click 自动合成，并把现有与本批次配方改成明确、可镜像的形状配方。

**Architecture:** `core` 只拥有固定 shape、裁边规范化与镜像匹配；`sim` 为每个玩家保存 9 格瞬态 crafting grid，验证所有移动和 output take，始终维持“背包可完整回收网格”不变量；`server` 只做协议映射与完整状态发布；客户端/HUD 只显示权威 `CraftingState`。工作台是普通完整方块，打开后只把同一玩家网格的有效尺寸从 2 变为 3，不存区块容器记录。

**Tech Stack:** Go 1.26、既有 `core.Inventory`、`sim.Engine`、Memory/TCP 共用协议、`internal/render/hud`、OpenSpec。

**Spec:** `docs/superpowers/specs/2026-08-23-first-night-survival-parallel-wave-design.md`

## Global Constraints

- 基于 batch 共享契约 SHA；不得重排 Item/Block/Recipe ID 或改动其他功能的保留消息。
- 批次执行时本计划 Task 1 先在 integration 分支创建并评审，功能 worktree 从共享契约提交继续并直接从 Task 2 开始；独立执行本计划时才在本分支创建 Task 1 产物。
- `RecipeID` 仍只用于注册表与 UI 身份，不上线、不落盘；v27 可修改 recipe 1..11 的形状，因为 v26 客户端已拒绝。
- crafting grid 不落盘；任意 tick 结束时，36 格 inventory 必须能容纳 grid 全部物品；take output 还必须预演“完整 output + 消费后剩余 grid”都可容纳，避免关闭、断线或死亡丢物。
- 不实现 shift-click、拖拽分堆、配方书、任意尺寸 recipe 或持久工作台 inventory。
- 只使用已有整栈移动语义；本任务不发明鼠标手持栈状态。

---

## Task 1：建立 OpenSpec change 与资源契约

**Files:**

- Create: `openspec/changes/authoritative-grid-crafting/.openspec.yaml`
- Create: `openspec/changes/authoritative-grid-crafting/proposal.md`
- Create: `openspec/changes/authoritative-grid-crafting/design.md`
- Create: `openspec/changes/authoritative-grid-crafting/tasks.md`
- Create: `openspec/changes/authoritative-grid-crafting/ledger.md`
- Create: `openspec/changes/authoritative-grid-crafting/specs/authoritative-grid-crafting/spec.md`
- Create: `openspec/changes/authoritative-grid-crafting/specs/authoritative-crafting/spec.md`
- Create: `openspec/changes/authoritative-grid-crafting/specs/authoritative-inventory/spec.md`
- Create: `openspec/changes/authoritative-grid-crafting/specs/bounded-benchmark-workload/spec.md`
- Create: `openspec/changes/authoritative-grid-crafting/specs/visual-verification/spec.md`

- [ ] **Step 1: 证明分支基线**

  运行 `git status --short`。若已存在共享提交，则用 `shared_sha=$(git log -1 --format=%H --grep='^feat: reserve first-night survival contracts$')` 和 `git merge-base --is-ancestor "$shared_sha" HEAD` 验证；若尚不存在，则本 Task 必须位于 `codex/first-night-survival-integration`。随后运行 `make rust`、`go test ./internal/core ./internal/sim ./internal/network ./internal/render/hud ./cmd/mornlea -race -count=1`，把命令和结果写入 ledger。

- [ ] **Step 2: 写可判定 delta specs**

  覆盖：2×2/3×3 权威网格、裁掉外围空行列、仅水平镜像、一次 output take 只合成一次、工作台距离/关闭、grid 回收不变量、拾取容量拒绝、死亡/断线回收、v27 拒绝 v26，以及 `inventory-crafting`/`workbench-crafting` 场景。`bounded-benchmark-workload` delta 同时锁定批次最终 scenario v20、唯一迁移 `19:20`、9格grid、72战斗候选、64 mobs、75 avatars/450 instances和64项mesh registry；integration closeout只核对这些已定值，不重新裁决。

- [ ] **Step 3: 在 design 固化数据形状**

  ```go
  const CraftingGridSlots = 9

  type RecipePattern struct {
      Width, Height uint8
      Cells         [CraftingGridSlots]ItemID
      Output        ItemStack
      Mirror        bool
  }

  type CraftingGrid struct {
      Size  uint8 // 2 or 3
      Slots [CraftingGridSlots]ItemStack
  }
  ```

  `CraftingGrid` 只存在于 `playerState` 与网络镜像，不加入 `StoredPlayer`。统一 view slot 是 grid 0..8、inventory 9..44；个人网格只允许 0..3，工作台允许 0..8；两端都在 inventory 区时拒绝并要求走既有 `MoveInventoryStack`。

- [ ] **Step 4: 严格校验并提交规划产物**

  运行 `openspec validate --all --strict --no-interactive` 与 `git diff --check`，完成 SPEC/QUALITY review 后提交 `docs: propose authoritative grid crafting`。

## Task 2：用 shape registry 替换单输入配方

**Files:**

- Modify: `internal/core/recipe.go`
- Modify: `internal/core/recipe_test.go`
- Modify: `internal/core/inventory.go`
- Modify: `internal/core/inventory_test.go`
- Modify: `internal/core/item.go`
- Modify: `internal/core/item_test.go`
- Modify: `internal/assets/blocks.go`
- Modify: `internal/assets/blocks_test.go`
- Test: `internal/physics/collision_test.go`

- [ ] **Step 1: 写裁边与镜像失败测试**

  表驱动覆盖：同一形状位于网格四角、外围空行列被忽略、内部空洞保留、水平镜像成功、垂直翻转失败、额外物品失败、空网格失败、2×2 不匹配 3×3 配方。

  运行 `go test ./internal/core -run 'Test(RecipePattern|MatchCraftingGrid)' -count=1`，预期编译失败或断言失败。

- [ ] **Step 2: 实现最小规范化匹配器**

  只添加 `MatchCraftingGrid(grid CraftingGrid) (RecipeID, ItemStack, bool)` 与私有 `trimPattern`/`matchesPattern`；固定循环最多 9 格，不分配 map/slice，不添加通用矩阵包。

- [ ] **Step 3: 写 18 条配方表失败测试**

  锁定：石砖 2×2 stone；熔炉 3×3 cobblestone ring；铁块 3×3 iron ingot；石/铁镐为顶排三原料加中列两 stick；箱子为 3×3 plank ring；原木→4 plank；发光块 2×2 glass；石/铁锄为两原料加两 stick；面包为横向三 wheat；stick 为纵向两 plank→4；工作台 2×2 plank；torch 为 coal 在 stick 上→4；三把剑为纵向两材料加 stick；床为横向三 white wool 位于横向三 plank 上。

- [ ] **Step 4: 实现固定 recipe registry**

  保持 ID 1..18；`Recipe(id)` 返回 `RecipePattern`，不再返回聚合 `Input`。删除 `Inventory.Craft`，因为真实扣料必须来自 grid；更新所有调用测试，禁止保留双重合成路径。

- [ ] **Step 5: 写并实现网格原子消费**

  测试证明 `ConsumeRecipe` 对每个非空 pattern cell 恰减 1、耐久物品不参与材料、失败返回原 grid、输出不直接进入 inventory。实现为 `[9]ItemStack` 副本上的固定循环。

- [ ] **Step 6: 登记可放置工作台方块**

  先写测试锁定 `ItemWorkbench`→`WorkbenchID`、采掘掉回1个workbench、完整立方体碰撞、opaque、emission0、普通cube model；在现有程序化atlas生成原创木质顶面/侧面/底面，不加入外部版权PNG。实现只扩固定item/block/assets switch。

- [ ] **Step 7: focused 验证与提交**

  运行 `gofmt -w internal/core internal/assets`、`go test ./internal/core ./internal/assets ./internal/physics -race -count=1`、`git diff --check`；双评审通过后提交 `feat: add shaped crafting recipes`。

## Task 3：实现玩家瞬态网格和容量不变量

**Files:**

- Modify: `internal/sim/player.go`
- Modify: `internal/sim/command.go`
- Modify: `internal/sim/engine_step.go`
- Modify: `internal/sim/container.go`
- Modify: `internal/sim/drop.go`
- Modify: `internal/sim/death.go`
- Modify: `internal/sim/persistence.go`
- Modify: `internal/sim/inventory_test.go`
- Create: `internal/sim/crafting.go`
- Create: `internal/sim/crafting_test.go`
- Modify: `internal/sim/death_test.go`
- Modify: `internal/sim/drop_test.go`

- [ ] **Step 1: 写移动和 output take 失败测试**

  覆盖 grid↔inventory、grid↔grid、禁止 inventory↔inventory、个人格 4..8 拒绝、不同物品不合并、同物品按栈上限合并、空源/同格拒绝、输出只由当前匹配派生、一次 take 恰消费一次并把完整 output 放入 inventory。

- [ ] **Step 2: 实现 `moveCraftingStack` 与 `takeCraftingOutput`**

  复用 `ItemStack.Valid`、`ItemStackLimit`、`Inventory.Slot`/`SetSlot`；用局部副本同时试算 grid 和 inventory，全部成功后再写回并置 `inventoryDirty`/`craftingDirty`。不建立通用 slot container interface。

- [ ] **Step 3: 写回收可打包失败测试**

  构造 36 格接近满载、grid 同类可并栈、grid 需要空格、output 恰好可放和少一格不可放；断言 `canRepackCrafting` 对全部 grid 物品和待产出执行与 `Inventory.AddStack` 相同顺序。

- [ ] **Step 4: 在所有 inventory 增量入口守住不变量**

  output take、掉落物拾取、采掘掉落、作物多掉落和 starter pack 使用同一个私有 `tryAddPreservingCrafting`；若拾取会破坏不变量，掉落保留在世界。玩家主动移动导致无法回收时拒绝该移动。

- [ ] **Step 5: 回收生命周期**

  关闭工作台只把 Size 3 降为 2，并先把 4..8 回收到 inventory；无法回收时关闭请求拒绝。断线持久化前和死亡清空 inventory 前必须先无损回收全部 9 格；不变量保证此处不会失败，测试把失败视为内部错误而不是静默丢物。

- [ ] **Step 6: 工作台打开验证**

  扩展 `openContainer` 识别 `WorkbenchID`：沿用权威 raycast/距离/loaded chunk 校验，设置玩家网格 Size=3 和命中位置；玩家离开触及距离、工作台被挖或主动关闭时回到 Size=2。工作台不占 `world.ContainerRef` 或 chunk slot。

- [ ] **Step 7: focused 验证与提交**

  运行 `gofmt -w internal/sim`、`go test ./internal/sim -race -count=1`；双评审后提交 `feat: make crafting grid authoritative`。

## Task 4：接通 v27 Memory/TCP 和客户端镜像

**Files:**

- Modify: `internal/network/message_inventory.go`
- Modify: `internal/network/codec_client.go`
- Modify: `internal/network/codec_server.go`
- Modify: `internal/network/packet.go`
- Modify: `internal/network/codec_inventory_test.go`
- Modify: `internal/network/registry_test.go`
- Modify: `internal/network/codec_fuzz_test.go`
- Modify: `internal/server/session_ingress.go`
- Modify: `internal/server/publication.go`
- Modify: `internal/server/transport_parity_integration_test.go`
- Modify: `internal/client/inventory.go`
- Modify: `internal/client/inventory_mirror_test.go`

- [ ] **Step 1: 完成消息值域失败测试**

  `MoveCraftingStack` 只接受 0..44、From≠To 且至少一端小于 9；`TakeCraftingOutput` 只携带非零 Sequence；`CraftingState.Size` 仅 2/3，Size=2 时 Slots[4..8] 必须空，全部栈与 Output 必须合法。codec 必须拒绝截断、尾随、未知 size 和超长 count。

- [ ] **Step 2: 用固定数组编码 `CraftingState`**

  复用既有 ItemStack codec；始终编码 9 个 slot，避免变长分支和长度歧义。完成 registry、ValidateClient/ServerPacket、Memory/TCP round trip 与 fuzz seed。

- [ ] **Step 3: ingress/publication 接线**

  ingress 映射到 `CommandMoveCraftingStack`/`CommandTakeCraftingOutput`；publication 只在 `craftingDirty`、打开/关闭或 output 改变时向所属 session 发完整 `CraftingState`，不广播其他玩家。

- [ ] **Step 4: 客户端 latest-wins 镜像**

  在 `internal/client` 保存最后一个合法完整状态；断线清空。测试 Memory/TCP 对同一命令序列给出逐字段相同的 grid、output、inventory 和拒绝结果。

- [ ] **Step 5: focused 验证与提交**

  运行 `gofmt -w internal/network internal/server internal/client`、`go test ./internal/network ./internal/server ./internal/client -race -count=1`；双评审后提交 `feat: synchronize crafting grids`。

## Task 5：实现 2×2/3×3 UI 与工作台视觉场景

**Files:**

- Modify: `internal/render/hud/container.go`
- Modify: `internal/render/hud/container_test.go`
- Modify: `cmd/mornlea/app.go`
- Modify: `cmd/mornlea/app_input.go`
- Modify: `cmd/mornlea/app_messages.go`
- Modify: `cmd/mornlea/app_inventory_crafting_test.go`
- Modify: `cmd/mornlea/capture_scene.go`
- Modify: `cmd/mornlea/capture_scene_test.go`

- [ ] **Step 1: 写布局/命中失败测试**

  个人面板精确 2×2、工作台精确 3×3、output 独立且不是普通 move target、inventory 36 格保持原位置；所有 slot 在窄窗口内可见且命中矩形与绘制矩形一致。

- [ ] **Step 2: 最小扩展既有 container layout**

  继续使用 `appendItemTile`、现有 atlas 和 quad/glyph pass；只加 grid size 参数和 output slot，不建立新 UI framework。删除 recipe-click 列表与 `CraftRecipe` 发送路径。

- [ ] **Step 3: 输入只发权威命令**

  grid/inventory 点击形成 `MoveCraftingStack`，output 点击形成 `TakeCraftingOutput`；客户端不预测扣料、产出或 grid 移动，直到完整 `CraftingState`/`InventoryState` 到达。

- [ ] **Step 4: 加两个固定 capture 构造**

  `inventory-crafting` 更新为 2×2 实物配方；新增 `workbench-crafting` 3×3 场景，展示至少一条镜像不对称配方和合法 output。只加场景与像素不变量测试，本分支不写 golden PNG。

- [ ] **Step 5: focused 验证与提交**

  运行 `gofmt -w internal/render/hud cmd/mornlea`、`go test ./internal/render/hud ./cmd/mornlea -race -count=1`；双评审后提交 `feat: present crafting grids`。

## Task 6：功能线终审

**Files:**

- Modify: `openspec/changes/authoritative-grid-crafting/tasks.md`
- Modify: `openspec/changes/authoritative-grid-crafting/ledger.md`

- [ ] **Step 1: 运行功能线验证**

  ```bash
  make rust
  go test ./internal/core ./internal/network ./internal/storage ./internal/sim ./internal/server ./internal/client ./internal/render/hud ./cmd/mornlea -race -count=1
  go test ./internal/archcheck -count=1
  openspec validate --all --strict --no-interactive
  git diff --check
  ```

- [ ] **Step 2: 独立终审并移交集成**

  reviewer 核对 18 条 shape、全部容量/回收路径、协议边界、Memory/TCP parity、无旧自动合成路径和无工作台持久容器；写 ledger、勾 tasks，并把 commit SHA 交给 integration controller，不自行合入 `main` 或更新 golden。
