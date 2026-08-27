# Proposal: authoritative-door

## Why

当前缺少建筑开合门闭环：需在权威服务端交付单材质木门（双格高原子放置、关闭实心碰撞/开启可通行、右键开合联动、破坏双清掉落），配方走 A-01 3×3 格子合成。参见 `docs/superpowers/specs/2026-08-27-door-design.md` §1 目标与非目标。

## What Changes

- 在 `BlockIDMax 62` 之后追加 9 个方块 ID：`DoorLowerSouthClosed=62, SouthOpen=63, WestClosed=64, WestOpen=65, NorthClosed=66, NorthOpen=67, EastClosed=68, EastOpen=69, DoorUpper=70`，`BlockIDMax 62→71`；新增辅助判定 `IsDoor/IsDoorLower/IsDoorUpper/DoorDir`，`block_name.go` 同步 9 名称。
- 在 `ItemIDMax 43` 之后追加 `ItemDoor=43`，`ItemIDMax 43→44`，`MaxStack 64`，`ItemPlacement[ItemDoor]=DoorLowerSouthClosed`（默认南，放置时按 yaw 重定向），`BlockDrop[Door*]=ItemDoor×1`。
- 在 `3×3` 追加配方 `木板 6（两列满）→ Door×3`，编号接 `Workbench` 后。
- 服务端权威实现双格原子：`sim.HandlePlace` 走 `tryPlaceDoor` 校验 `lower/upper 可替换 && 下方实心 && upper section 就绪`，`HandleInteract` 走 `Closed↔Open` 联动，`mining.completeMining` 走双清掉落。
- 碰撞/射线/流体沿 `§3.4` 落地：关厚 `3/16` 贴边、开旋转 `90°` 薄边；关实心开可穿透；`DoorClosed` 不可流入、`DoorOpen` 可流入。
- 资产与网格：`LayerDoor=55`、Material Door、TopRaw 不下沉、`FaceVisible` 按方向剔除；`DoorMaterial` 单值，`nativeMax 64→71` 同步 Rust `MAX_REGISTRY_ENTRIES=71`。
- 版本矩阵不变：`protocol v27`、`chunk schema v9`、`engine ABI v7`、`client ABI v9`、`benchmark scenario v19` 均不升版，无 wire/schema 变更。

## Capabilities

### New Capabilities

- `door`: 双格高木门的权威放置（双格原子）、右键开合联动与破坏双清掉落，含碰撞/射线/流体与配方闭环（对应 `specs/door/spec.md`）。

### Modified Capabilities

- `common-block-materials`: 方块与物品编号在哨兵前 append-only 追加（`BlockIDMax 71`、`ItemIDMax 44`），`RegisteredBlock/RegisteredItem` 仍为 `id < Max`。

## Impact

- **代码**：`internal/core/block.go, block_name.go, item.go, recipe.go, farming.go`；`internal/assets/blocks.go`；`internal/mesh/quad.go, native_input.go`；`engine/crates/mornlea_engine/src/native_input.rs`；`internal/sim/door.go（新建）、mining.go、command.go`；`internal/world/chunk.go`；`internal/storage` 编解码验证。
- **兼容性**：枚举 append-only，不重排；旧存档不含新 ID 视为空气，新方块在回滚代码下同理无迁移；协议与全部 schema/ABI/scenario 保持不变。
- **性能与并发**：单格读写 ≤2 次，不计入 `cropBlockReads`；无随机 tick，纯输入驱动；原子双格写入，跨区块未就绪拒绝。
- **回退**：删除哨兵前枚举与对应分支即可 revert，存档保持可读。

## Non-Goals

- 不做红石驱动（`B-19`）、双门联动、铁门差异、铰链左右扩展（首版固定左铰链）、多材质门。
