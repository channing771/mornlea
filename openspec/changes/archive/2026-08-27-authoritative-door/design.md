## Context

动机与范围见 `proposal.md` 与 `docs/superpowers/specs/2026-08-27-door-design.md` 全量。当前无门：首版需交付单材质木门 9 ID（8 下按方向×开/关 + 1 上无方向）双格高原子闭环，避开 `B-16` 状态字节重构与 schema 迁移，按每态一 ID 范式在 `BlockIDMax 62→71` 追加，二期多材质再追加 ID。权威 tick/渲染/网络热路径不得无界阻塞。

## Goals / Non-Goals

**Goals:**
- `BlockIDMax 71`（`CarrotStage7 61 → DoorLower 62..69 → DoorUpper 70`）、`ItemIDMax 44`（`ItemDoor 43`）哨兵前 append-only，`RegisteredBlock/RegisteredItem = id < Max`。
- 双格原子：放置/开合/破坏均同时改 `lower+upper`，跨区块 `upper` 未就绪拒绝，不残留半门。
- 碰撞关厚 `3/16` 贴边、开旋转 `90°` 薄边；射线关实心开可穿透；流体关不可流入开可流入。
- 配方 `3×3 木板6（两列满）→ Door×3` 接 `Workbench` 后；`LayerDoor=55`、`DoorMaterial` 单值、`nativeMax 64→71` 同步 Rust。
- 无协议/存档/ABI 升版（`protocol v27, chunk v9, engine ABI v7, client ABI v9, benchmark v19`）。

**Non-Goals:** 红石驱动、双门联动、铁门/多材质、铰链左右扩展、实体化门。

## Decisions

### D1：每态一 ID（Decision A）而非状态字节或实体化

`block.go:28` 在 `CarrotStage7ID=61` 后追加 9 ID（62..70），`D1` 选 A 而非 B（单 ID+状态字节）与 C（实体化）。A 避 `schema v10` 迁移与存档双读，大改与未知分支，与 `B-01+A-01` 追加纪律同构，首版 9 ID 仍在 `nativeMax 71` 内。

```go
const (
    DoorLowerSouthClosed BlockID = 62
    DoorLowerSouthOpen BlockID = 63
    DoorLowerWestClosed BlockID = 64
    DoorLowerWestOpen BlockID = 65
    DoorLowerNorthClosed BlockID = 66
    DoorLowerNorthOpen BlockID = 67
    DoorLowerEastClosed BlockID = 68
    DoorLowerEastOpen BlockID = 69
    DoorUpper BlockID = 70
    BlockIDMax BlockID = 71
)
func IsDoor(id BlockID) bool { return id >= DoorLowerSouthClosed && id <= DoorUpper }
func IsDoorLower(id BlockID) bool { return id >= DoorLowerSouthClosed && id <= DoorLowerEastOpen }
func IsDoorUpper(id BlockID) bool { return id == DoorUpper }
func DoorDir(id BlockID) int { /* South0 West1 North2 East3 */ }
```

`block_name.go` 同步 9 名，`farming.go` 追加 `IsDoor` 区间（参考 `IsCrop`），`item.go` 追加 `ItemDoor`/`ItemPlacement`/`BlockDrop`，`recipe.go` 追加门配方。

### D2：数据流与权威模拟

- **放置**：客户端 `ItemDoor + Yaw` 经 `UseKey` 发 `PlaceBlock{pos,dir}`（`yaw→South/West/North/East`）；`sim.HandlePlace` 走 `tryPlaceDoor(lowerPos,dir)` 校验 `lower可替换 && upper可替换 && 下方实心（Opaque/Farmland）`，跨区块 `upper` section 未就绪拒绝，否则原子 `SetBlock(lower=DoorLowerDirClosed, upper=DoorUpper)` 经 `PrepareDropBatch`、消耗 1，`upper` 隐含关联 `lower.dir`。
- **开合**：客户端右键 `InteractBlock{pos}`；`sim.HandleInteract` 若 `IsDoor` 则定位 `lowerPos`（`Upper→y-1`），`Closed↔Open` 同 `dir` 翻转，上下原子更新（`lower` 切态、`upper` 不变逻辑关联），无消耗。
- **破坏**：`sim/mining.go completeMining` 若 `IsDoor` 则原子双格 `Air` 并 `Drop ItemDoor 1`（命中 `Upper` 定位 `lowerPos`），`DoDrop=false` 零掉落仍双清。
- **碰撞/射线/流体**：`BlockCollisionBoxes` 关 `3/16` 贴边、开 `90°` 薄边；`IsSolidForRaycast` 关实心开非实心；`fluid.evalCell` `DoorClosed` 实心不可流入、`DoorOpen` 视空气可流入。

### D3：资产与网格

`internal/assets/blocks.go:46` 新增 `LayerDoor=55`（`Plant 31..54` 后首个非植物层），`Material Door` 非透明、`TopRaw` 不下沉；`FaceVisible` 关按门面厚剔除、开按旋转薄边。`internal/mesh/quad.go:47` 新增 `DoorMaterial` 单值，`nativeMaxRegistryEntries 64→71`，同步 `engine/crates/mornlea_engine/src/native_input.rs:MAX_REGISTRY_ENTRIES=71`。`internal/core/recipe.go` 配方 `木板6→Door×3` 接 `Workbench` 后。

### D4：文件结构与校验规则

- 改：`internal/core/block.go, block_name.go, item.go, recipe.go, farming.go`；`internal/assets/blocks.go, procedural.go`；`internal/mesh/quad.go, native_input.go`；`engine/.../native_input.rs`；`internal/sim/door.go（新建）、mining.go、command.go`；`internal/world/chunk.go`；`internal/storage/*` 仅验证。
- 校验：`IsDoor=[62,70]` 有序区间 `62..69 < 70 < 71`；`LayerDoor=55` 与 `Plant 31..54` 不重叠；`nativeMax 71` 双侧一致；`chunk-v9.bin` 不升版 round-trip。

## Alternatives Considered

- **每态一 ID vs 单 ID+状态字节 vs 实体化**：B 需 `schema v10` 与存档双读，大改量与回滚面大；C 需实体持久化与网络同步，偏离方块即状态架构；均否决，选 A 每态一 ID。
- **上半有方向 vs 无方向关联下半**：上半有方向省推导但翻倍 ID（16），首版固定左铰链无需，选单 `DoorUpper` 由 `lower` 推导方向，ID 经济。
- **跨区块懒就绪 vs 拒绝**：懒创建未就绪 section 会引入空洞与并发写入，已选拒绝路径最简且可判定。

## Risks / Trade-offs

- 每态一 ID 使 `BlockIDMax` 触 `nativeMax 71` 上限，后续多材质门需评估 `>71` 扩容与 Rust 同步。
- `LayerDoor 55` 紧接植物区，需 `blocks_test` 钉死不重叠，避免材质排序回归。
- 双格原子跨区块拒绝会使贴区块边界放置失败率为零倒是可预期的确定拒绝，非静默半门。
- `3/16` 薄板碰撞与射线穿透需 `sim/door_test` 四向快照锁定，避免后续网格调参漂移。

## Migration Plan

无存档/协议/ABI/scenario 迁移。枚举 append-only，旧存档无新 ID 即零，新方块在回滚代码下按空气处理。协议保持 `27`，`openspec/config.yaml` 版本矩阵不变。

## Open Questions

无。
