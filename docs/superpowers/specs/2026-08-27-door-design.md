# B-17 门 — 设计文档

- 日期：2026-08-27
- 分支：`feat/B-17-door`
- 状态：已获三节确认（架构/数据流/错误处理与测试），待 `writing-plans` 拆解

## 1. 目标与非目标

**目标（最小闭环）：** 单材质木门，双格高原子放置，开/关可交互切换，关闭实心碰撞、开启可通行，破坏双格原子掉落 1 门物品，配方走 A-01 3×3 格子合成。

**非目标：** 红石驱动（`B-19`）、双门联动、铁门差异、铰链左右扩展（首版固定左铰链）、多材质门（后续追加 ID）。

## 2. 架构与注册

- `internal/core/block.go:28`：`CarrotStage7ID=61` 后追加 9 ID
  - `DoorLowerSouthClosed=62, SouthOpen=63, WestClosed=64, WestOpen=65, NorthClosed=66, NorthOpen=67, EastClosed=68, EastOpen=69`
  - `DoorUpper=70`（上半无方向，关联下半方向由下半决定）
  - `BlockIDMax 62→71`，`IsDoor() = id∈[62,70]`，`IsDoorLower=[62,69]`，`IsDoorUpper=70`，`DoorDir(id) → 0..3`
  - `block_name.go:12` 同步 9 名称（`door_lower_*`, `door_upper`）
- `internal/core/item.go:66`：`ItemPoisonousPotato=42` 后 `ItemDoor=43`，`ItemIDMax 44`，`MaxStack 64`，`ItemPlacement[ItemDoor]=DoorLowerSouthClosed`（默认南，放置时按 yaw 重定向），`BlockDrop[Door*]=ItemDoor×1`
- `internal/assets/blocks.go:46`：新增 `LayerDoor=55`（`Plant 31..54` 后首个非植物层），`Material Door` 非透明，`TopRaw` 不下沉；`FaceVisible` 关闭时按门面厚 3/16 剔除，开启时按旋转 90° 薄边
- `internal/mesh/quad.go:47`：`DoorMaterial` 单值，`nativeMaxRegistryEntries 64→71`，同步 `engine/crates/mornlea_engine/src/native_input.rs:MAX_REGISTRY_ENTRIES`
- `internal/core/recipe.go`：`3×3` 追加 `木板 6（两列满）→ Door×3`，编号接 `Workbench` 后
- 版本：`protocol v27`、`chunk schema v9`、`engine ABI v7`、`client ABI v9` 均不变（每态一 ID，避开 `B-16` 状态字节重构）

## 3. 数据流与权威模拟

### 3.1 放置
- 客户端：`ItemDoor + Yaw` 经 `UseKey` 发 `PlaceBlock{pos, dir}`（`dir` 由 `yaw→South/West/North/East`）
- 服务端 `sim.HandlePlace`：`tryPlaceDoor(lowerPos, dir)` 校验 `lower可替换 && upper可替换 && 下方实心（Opaque/Farmland）`，否则 `Reject`；成功则原子 `SetBlock(lower=DoorLowerDirClosed, upper=DoorUpper)` 经 `PrepareDropBatch`，消耗 1，`upper` 隐含关联 `lower.dir`
- 跨区块：若 `upper` 所在 section 未就绪则拒绝（视作 `upper` 被占）

### 3.2 开合
- 客户端右键 → `InteractBlock{pos}`（新增或复用交互位）
- 服务端 `sim.HandleInteract`：若 `IsDoor`，定位 `lowerPos`（若命中 `Upper` 则 `y-1`），`Closed↔Open` 同 `dir` 翻转，上下均原子更新（`lower` 切 `Closed/Open` 同向，`upper` 不变但逻辑关联），无消耗

### 3.3 破坏
- `sim/mining.go completeMining`：`IsDoor` 时原子双格 `Air` 并 `DropItemDoor 1`（命中 `Upper` 时定位 `lowerPos`），`DoDrop=false` 则零掉落仍双清

### 3.4 碰撞/射线/流体
- `BlockCollisionBoxes`：关闭返回厚 3/16 贴边 AABB（按 `dir` 南贴南等），开启返回旋转 90° 薄边（门缝侧可通行）
- `IsSolidForRaycast`：关闭实心、开启非实心（可穿透）
- `fluid.evalCell`：`DoorClosed` 视作实心不可流入，`DoorOpen` 视作空气可流入

## 4. 错误处理与边界

- 拒绝：`upper` 被占用、下方非实心、跨区块未就绪、創造模式外无 `ItemDoor` 均拒绝；交互命中非门无事
- 原子性：放置/开合/破坏均双格原子，任意半失败则全回滚，不残留半门
- 确定性：无随机 tick，纯输入驱动
- 成本：单格读写 ≤2 次，不计入 `cropBlockReads`

## 5. 测试

- `core/farming_test.go`：有序区间 `62..69 < 70 < 71` 守护
- `core/item_test.go`：`ItemIDMax 44`、放置/掉落映射、`block_name` 长度 71
- `core/blocks_test.go` / `mesh/quad_test.go`：`LayerDoor` 不与 `Plant 31..54` 重叠、几何快照（关贴边、开旋转）
- `sim/door_test.go`：四向放置、上方占用拒、无支撑拒、开合四向翻转、破下/破上均双清+掉1、开启射线可穿透
- `storage`：`DoorLower+Upper` 编解码 round-trip（`chunk-v9.bin` 不升版）
- `openspec delta`：`放置双格原子`、`右键开合联动`、`破坏双清掉落` 三场景

## 6. 文件清单

- `internal/core/block.go`, `block_name.go`, `item.go`, `recipe.go`, `farming.go`（`IsDoor`）
- `internal/assets/blocks.go`, `procedural.go`（`door` 纹理）、`internal/mesh/quad.go`, `native_input.go`, `engine/crates/mornlea_engine/src/*`
- `internal/sim/door.go`（新建）、`mining.go`, `command.go`（`Interact`）、`internal/world/chunk.go`（碰撞）、`internal/storage/*`（编解码）
- `openspec/changes/authoritative-door/*`

## 7. 方案权衡

- 选 A（每态一 ID）而非 B（单 ID+状态字节）：避 `schema v10` 迁移、存档双读与大改，首版 9 ID 仍在 `nativeMax` 内，与 `B-01+A-01` 追加纪律同构
- 不选 C（实体化）：门是建筑方块，实体化需持久化与网络实体同步，偏离方块即状态架构
