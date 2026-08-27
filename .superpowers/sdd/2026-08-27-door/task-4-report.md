# Task 4 Report — 权威模拟（放置/开合/破坏/碰撞）

**Status:** ✅ Done
**Branch:** feat/B-17-door

## 交付
- `internal/sim/door.go`（新建）：实现 `tryPlaceDoor`（下实心上空原子，下为 `isSolidSupport` 判 Opaque/Farmland，上为 Air 原子双写，回滚）、`handleInteractDoor`（命中 upper 则 y-1 定位 lower，Closed↔Open 同 dir 切换，原子更新 lower，经 `recordChange`）、`yawToDoorDir`（South0/West1/North2/East3）、`doorLowerID`/`isDoorOpen`；新增 `executeInteractDoor` 权威射线分发
- `internal/sim/engine_placement.go:110-128`：`executePlacement` 接入 `IsDoor` 分支，`yaw→dir` 经 `tryPlaceDoor` 校验，成功则消耗 1 并原子落盘
- `internal/sim/mining.go:47-62,563-618`：`miningRule` 新增 `IsDoor → 15,true`；`completeMining` 开头新增 `IsDoor` 双清分支（定位 lower/upper，PrepareDropBatch 单堆 ItemDoor 预演，DoDrop=false 仍双清零掉落，原子 SetBlock 空气 + 双 `recordChange` + Commit）
- `internal/sim/command.go:36-39`：新增 `CommandInteractDoor`（append-only）
- `internal/sim/engine_step.go:246-265,410-429`：新增 CommandInteractDoor 的校验、interactions 收敛与 `executeInteractDoor` 分发
- `internal/physics/types.go:131-212`：`BlockCollisionBoxes` 新增门分支：关闭厚 3/16 贴 dir 边，开启旋转 90° 到左铰链边（南→东、西→南、北→西、东→北），上半按空气无碰撞（下半已阻挡）
- `internal/core/raycast.go:204-218`：`InteractionTarget` 新增门分支：关闭实心（true）、开启穿透（false），上半保持实心以便命中交互
- `internal/fluid/rules.go:23-38`：`Replaceable` 新增门分支：开启可流入（true）、关闭/上半不可流入（false）
- `internal/sim/door_test.go`（新建）：`TestDoorPlaceAndToggle` 覆盖四向放置、上方占用拒、下方非实心拒、上下联动翻转、下/上半双清掉1、DoDrop=false 仍双清、开启穿透射线、碰撞 3/16 贴边与旋转差异、流体 Replaceable

## 验证
- TDD Step1：`go test ./internal/sim -run TestDoorPlaceAndToggle -count=1` 在 door.go 桩时 FAIL（`lower=0 want 62` / `rejected`），实现后 PASS
- `go test ./internal/sim -run TestDoor -count=1 -race` → PASS
- `go test ./internal/sim -count=1 -short -race` → PASS（38s）
- `go test ./internal/physics -count=1 -race` → PASS
- `go test ./internal/core -count=1 -race` → PASS
- `go test ./internal/fluid -count=1 -race` → PASS
- `go test ./internal/archcheck -count=1` → PASS

## 契约
- 放置：`lower==Air && upper==Air && below SolidSupport` 否则 RejectOccupied/RejectInvalidBlock，跨区块未就绪 RejectChunkNotReady，原子双格
- 交互：`IsDoorUpper → y-1` 定位 lower，Closed↔Open 同 dir，`recordChange` 原子
- 破坏：任一半 `completeMining` 双格置 Air，harvestable 时 PrepareDropBatch ItemDoor×1，DoDrop=false 仍双清
- 碰撞：关闭 3/16 贴边 AABB，开启旋转 90° 薄边
- 射线：关闭 InteractionTarget true，开启 false
- 流体：关闭 Replaceable false，开启 true

## 约束
- TryPlaceDoor 严格要求上空为空气（流体视为占用），下方实心复用 Opaque 语义（Glass/Leaves/Fluid/Crop/Door 非实心）
- Mining 双清单块掉落槽复用 lower 区块的 PrepareDropBatch，上半按同一 chunk 假设
- DoorUpper 碰撞按空气处理，流体上半按实心处理（下半已决定通行/阻挡）

## Fix Round 1 (2026-08-27) — 评审 3 Major + 2 Minor

- [Major-1] `internal/core/raycast.go:204-223` 上半恒实心改为 `isDoorSolidForRaycast=!Open` 回退（孤立 ID 按关闭处理），真实上半固体性下沉至 `internal/sim/mining.go:149-163` 的 `blockRaycastSampler`：`IsDoorUpper → 查 y-1 lower 是否 IsDoorOpen（不存在按 Closed）`，`Open→false 可穿透, Closed→true`；`internal/core/block.go:166-175` 新增 `IsDoorOpen` 供复用。
- [Major-2] `internal/sim/door_test.go` 矩阵扩展：新增 `TestDoorYawToDirBoundaries` 7 点边界、`TestDoorPlaceSupportEnumerations`（Glass/Leaves/WaterSource/WaterLevel1/WheatStage0/Door/DoorUpper/Air ≥3 例）、`TestDoorInteractViaRaycast`（makeTestWorld 起服后 `CommandInteractDoor` 射线命中下半翻转）、`TestDoorUpperInteractDirPreserved`（上半 y-1 定位且 dir 保持，4 向）。
- [Major-3] `TestDoorCollisionFourDirections` 扩展四向全验：关闭贴边（South z 0.8125..1, West x 0..0.1875, North z 0..0.1875, East x 0.8125..1）、开启铰链（SouthOpen x 0.8125..1, WestOpen z 0.8125..1, NorthOpen x 0..0.1875, EastOpen z 0..0.1875）、`DoorUpper Count==0 Loaded true` 与未加载 `Loaded false`。
- [Minor-4] `TestDoorPhysicsFluidDivergence` 锁定分歧：`DoorUpper` 物理 `Count 0 Loaded true` 但流体 `Replaceable false`（关实心）、关闭物理/流体双实心、开启物理 1 且流体可流入；并校验 `InteractionTarget` 回退及 sampler 的上半依下半态。
- [Minor-5] `TestDoorMiningDropCapacity` 满掉落槽故障注入（`fillMiningDrops`）验证 `RejectDropCapacity` 原子性；`TestDoorPlaceUpperRollback` 验证上方占用原子拒绝无残留（`pending 0`）。

验证：
- `go test ./internal/sim -run TestDoor -count=1 -race` → PASS
- `go test ./internal/sim -count=1 -short -race` → PASS
