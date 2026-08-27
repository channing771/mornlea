# Tasks: authoritative-door

## 1. Core 注册（Block/Item/Recipe）

- [ ] 1.1 修改 `internal/core/block.go:28-120` 在 `CarrotStage7ID=61` 后追加 9 BlockID（`DoorLowerSouthClosed 62..DoorUpper 70, BlockIDMax 71`）并实现 `IsDoor/IsDoorLower/IsDoorUpper/DoorDir`
- [ ] 1.2 修改 `internal/core/block_name.go:3-30` 同步 9 名称 `door_lower_* / door_upper`
- [ ] 1.3 修改 `internal/core/item.go:66-82` 追加 `ItemDoor=43, ItemIDMax 44, MaxStack 64`，`ItemPlacement[ItemDoor]=DoorLowerSouthClosed`，`BlockDrop[Door*]=ItemDoor×1`
- [ ] 1.4 修改 `internal/core/recipe.go:12-45` 追加 `3×3 木板6（两列满）→ Door×3` 配方（接 `Workbench` 后）
- [ ] 1.5 修改 `internal/core/farming.go:9` 追加 `IsDoor` 区间（参考 `IsCrop`），扩展 `internal/core/*_test.go` 守护 `62..69 < 70 < 71` 有序与 `IsDoor` 区间
- [ ] 1.6 验证 `go test ./internal/core -run TestDoor -count=1`、`go test ./internal/core -count=1 -short`、`go vet ./...`、`gofmt -l .`

## 2. 资产与网格

- [ ] 2.1 修改 `internal/assets/blocks.go:46-284` 新增 `LayerDoor=55`（`Plant 31..54` 后），`Material Door` 非透明、`TopRaw` 不下沉，`FaceVisible` 关按 `3/16` 贴边剔除、开旋转 `90°`
- [ ] 2.2 修改 `internal/mesh/quad.go:47-69` 新增 `DoorMaterial` 单值
- [ ] 2.3 修改 `internal/mesh/native_input.go:42` `nativeMaxRegistryEntries 64→71` 并同步 `engine/crates/mornlea_engine/src/native_input.rs:MAX_REGISTRY_ENTRIES=71`
- [ ] 2.4 扩展 `internal/assets/blocks_test.go` 与 `internal/mesh/quad_test.go` 覆盖 `LayerDoor` 不与 `Plant` 重叠、门几何快照（关贴边/开旋转）
- [ ] 2.5 验证 `go test ./internal/assets -run TestDoor -count=1`、`go test ./internal/mesh -count=1`、`make rust-check`

## 3. 权威模拟（放置/开合/破坏/碰撞）

- [ ] 3.1 新建 `internal/sim/door.go` 实现 `tryPlaceDoor(lowerPos, dir)`（校验 `lower/upper 可替换 && 下方实心 && upper section 就绪`，原子双格）与 `handleInteractDoor(pos)`（定位 `lowerPos`、`Closed↔Open` 联动）
- [ ] 3.2 修改 `internal/sim/mining.go:622-744` 在 `completeMining` 接入 `IsDoor` 双清掉落（命中 `Upper` 定位 `lower`，原子 `Air` + `Drop ItemDoor 1`，`DoDrop=false` 仍双清）
- [ ] 3.3 修改 `internal/sim/command.go` 为 `InteractBlock` 分发至 `handleInteractDoor`
- [ ] 3.4 修改 `internal/world/chunk.go`（或 `sim` 内）实现 `BlockCollisionBoxes`（关厚 `3/16` 贴边、开旋转薄边）、`IsSolidForRaycast`（关实心开可穿透）、`fluid.evalCell`（关不可流入开可流入）
- [ ] 3.5 新增 `internal/sim/door_test.go` 覆盖四向放置、上方占用拒、无支撑拒、开合四向翻转、破下/破上均双清+掉1、开启射线可穿透
- [ ] 3.6 验证 `go test ./internal/sim -run TestDoor -count=1 -race`、`go test ./internal/sim -count=1 -short`

## 4. 存储与集成

- [ ] 4.1 验证 `internal/storage/chunk_codec*.go` 无需迁移（`BlockID uint16` 已泛化，`71` 内可编码），仅测试覆盖
- [ ] 4.2 扩展 `internal/storage/world_test.go` 新增 `TestDoorRoundTrip`（写入 `DoorLower+Upper` 编解码后相等，`chunk schema v9` 不升版）
- [ ] 4.3 扩展 `internal/mesh/quad_test.go` 门关/开几何快照薄板位置
- [ ] 4.4 验证 `go test ./internal/storage -run TestDoorRoundTrip -count=1`、`go test ./internal/mesh -run TestDoor -count=1`

## 5. 收尾（门禁、校验、归档准备）

- [ ] 5.1 同步 delta 至主规格（`openspec/specs/door/spec.md` 已含 3 场景，最终对齐）
- [ ] 5.2 门禁：`make rust`、`go test ./... -count=1 -short -race`、`go test ./internal/archcheck -count=1`、`go vet ./...`、`gofmt -l .`
- [ ] 5.3 校验：`openspec validate --all --strict --no-interactive` 全绿（含新 change）
- [ ] 5.4 归档 ready：`openspec archive authoritative-door --yes` 前置检查，`docs/feature-backlog.md` 排队→已完成待合入后更新
