# Tasks: more-crops

- [x] 1. OpenSpec change 与 backlog 晋升 — 创建 `openspec/changes/more-crops/{proposal.md,design.md,tasks.md,specs/authoritative-farming/spec.md}` 并修改 `docs/feature-backlog.md:83` 为 `已认领 | opencode-implementer @ feat/B-01-more-crops` 且备注独占文件集 `internal/core (BlockID/ItemID), internal/sim (crop/mining/bone_meal), internal/assets, internal/mesh, internal/storage`；验证 `openspec validate --all --strict --no-interactive` PASS

- [x] 2. core 枚举 — BlockID 与 farming 谓词 — 修改 `internal/core/block.go:69-97` 在 `WheatStage7ID` 后追加 `PotatoStage0..7`、`CarrotStage0..7` 并保持 `BlockIDMax` 恒末，修改 `internal/core/farming.go:3-52` 扩展 `IsCrop` 为三区间并集并新增 `IsPotato/IsCarrot/CropStage` helper；新增 `internal/core/farming_test.go` 覆盖 `IsCrop/IsPotato/IsCarrot` 穷举与 `BlockIDMax == CarrotStage7ID+1` 守护；验证 `go test ./internal/core -race -count=1`、`go vet ./...`、`gofmt -l`

- [x] 3. core 枚举 — ItemID、FoodValue、ItemPlacement — 修改 `internal/core/item.go:41-68,235-253,298-352` 追加 `ItemPotato/Carrot/PoisonousPotato` 并更新 `ItemStackLimit 64` 与 `ItemPlacement` 两行（毒土豆不可放置），修改 `internal/core/hunger.go:41-51` 追加 `FoodValue` 三行 `Potato 1/600、Carrot 3/3600、Poisonous 2/1200`；扩展 `internal/core/item_test.go` 与 `hunger_test.go` 守护 `ItemIDMax==Poisonous+1` 与千分位值；验证 `go test ./internal/core -race -count=1`

- [x] 4. sim 生长管线 — `growCrop` 与独立 yield — 修改 `internal/sim/crop.go:96-211,299-357` 实现 `growCrop` 三作物分派（`wet&&sky→+1`、封顶 `Stage7`）并新增 `cropYieldRollsPotato/Carrot` 与 `poisonRoll`（独立 `splitmix64` salt，`1..4` 与 `2%`）、更新 `advanceCropCell` 区块标签；扩展 `internal/sim/crop_test.go` 与 `bone_meal_test.go` 覆盖 `wet&&sky` 推进、干/遮挡/成熟不推进及 salt 独立；验证 `go test ./internal/sim -race -count=1 -run TestGrowCrop`

- [x] 5. sim 收获 — mining 掉落 `1..4` 与毒土豆 — 修改 `internal/sim/mining.go` 成熟分支：`CarrotStage7` 产 `1..4`（单 salt）、`PotatoStage7` 产 `1..4` + 2% 毒土豆（独立 salt），未成熟各产 `1` 自身，复用原子容量语义；扩展 `internal/sim/mining_test.go` 覆盖未成熟 `1`、成熟区间 `1..4`、毒土豆 2%、重放确定、跨维度独立与容量回滚；验证 `go test ./internal/sim -race -count=1 -run TestHarvest`

- [x] 6. 骨粉 / 踩踏 / 水冲 — `IsCrop` 覆盖 — `internal/sim/bone_meal.go` 与 `trample.go`、`fluid/evalCell` 沿 `IsCrop` 零改，仅扩展 `internal/sim/bone_meal_test.go`、`trample_test.go`、`internal/fluid/*_test.go` 用例：马铃薯/胡萝卜 `Stage0→1`/`Stage7拒绝`、距离/就绪/空手拒绝、踩踏/水冲新区间；验证 `go test ./internal/sim -race -count=1 -run BoneMeal`、`go test ./internal/fluid -count=1`

- [x] 7. 资产 / 网格 / 存档 — 修改 `internal/assets/blocks.go` 按 Wheat 同形注册两套 `cross` 植物（`plant` 区间追加），修改 `internal/mesh/registry.go` 复用 `cross` 仅更新 `block_top_raw` 校验（不新增 model tag），扩展 `internal/storage/world_block_test.go` 与 `world_test.go` 覆盖新 `BlockID` round-trip；新增占位 `assets/textures/potato_*.png` 与 `carrot_*.png`（纯色合规）；验证 `go test ./internal/storage -race -count=1`、`go test ./internal/mesh -count=1`、`go test ./internal/assets -count=1`

- [x] 8. 规格合入、门禁与归档 — 同步 `openspec/specs/authoritative-farming/spec.md` 增 `Requirement: 马铃薯与胡萝卜闭环` 的 7 个 Scenario（经 `openspec archive more-crops --yes` 或手改），更新 `docs/feature-backlog.md:83` 为 `已完成` 与 `docs/notes/progress.md` 基线段；验证 `make rust`、`go test ./... -race -count=1`、`go test ./internal/archcheck -count=1`、`go vet ./...`、`gofmt -l .`、`openspec validate --all --strict --no-interactive` 全绿后执行 `openspec archive more-crops --yes`
