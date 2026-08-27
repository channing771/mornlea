# Tasks: bone-meal

- [ ] 1. `internal/core/item.go` 追加 `ItemBoneMeal`（`ItemIDMax` 前，`ItemStackLimit` 64，`RegisteredItem` 覆盖）并更新 `item_test.go` 枚举守护
- [ ] 2. `internal/network` 追加 `BoneMeal` 命令：`message_command.go`（`Validate` 有限角）、`packet.go`（`ProtocolVersion 27` 注释）、`registry.go`（`clientPacketID 14` 与 `clientPacketForID`）、`codec_client.go`/`codec_server.go` 编解码与 `ValidateClientPacket` 分支、冻结测试 `registry_test.go`/`packet_test.go`
- [ ] 3. `internal/sim/command.go` 追加 `CommandBoneMeal`，新建 `internal/sim/bone_meal.go`（`executeBoneMeal`）与 `internal/server/session_ingress.go` 分发
- [ ] 4. `cmd/mornlea/app_input.go` 在放置分流前插入骨粉分支（手持 `BoneMeal` 且命中作物时发 `BoneMeal`）
- [ ] 5. 新建 `internal/sim/bone_meal_test.go`：成功 0→1/6→7、成熟拒绝不消耗、非作物拒绝、距离/区块未就绪拒绝、空手/非骨粉拒绝、同 tick 幂等与确定性重放、消耗精确 1
- [ ] 6. 运行 `make rust`（`rust` 侧零改动，仅校验）、`go test ./internal/sim -race -count=1`、`go test ./internal/network -race -count=1`、`go test ./internal/archcheck -count=1`、`gofmt -l`、`go vet ./...`、`openspec validate --all --strict --no-interactive`、`go test ./... -short`
- [ ] 7. 归档与主规格合入（`authoritative-farming` 新增骨粉催熟 Requirement）并回填 `docs/feature-backlog.md` B-03 → 已完成、`docs/notes/progress.md` 基线段与 `openspec/config.yaml` 版本矩阵
