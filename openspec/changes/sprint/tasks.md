# Tasks: sprint

## 1. 网络层
- [ ] `internal/network/message_command.go` 追加 `Sprinting bool`（`Eating` 后）
- [ ] `packet.go` `ProtocolVersion 28` + 注释
- [ ] `codec_client.go` 编解码尾部 bool（`Eating` 后）
- [ ] `registry_test.go`/`codec_golden` 按需更新（无 ID 变化）

## 2. 物理层
- [ ] `internal/physics/tunables.go` 追加 `SprintSpeedMultiplier`
- [ ] `types.go` `Input.Sprinting`
- [ ] `step.go` `stepHeaderBytes` 保持 160、`stepLayoutVersion 3`、encode `bytes[129]`/`[148:152]`、`stepSweepBounds` 中 effectiveWalkSpeed
- [ ] `engine/crates/mornlea_engine/src/step.rs` 同步 decode/validate/integrate

## 3. 模拟层
- [ ] `internal/sim/hunger.go` 新增 `exhaustionSprintMilli`（固定表第六行，`80`）
- [ ] `internal/sim/player.go` 饥饿门控 + 疲劳触发（实际加速时）
- [ ] `cmd/mornlea/app_input.go` 等输入装配（如有疾跑键位）

## 4. 测试与门禁
- [ ] 门控矩阵（地面/水下/饥饿5 vs 6/静止 vs 前移/浸没）
- [ ] 疲劳跨阈值（饱和度→饥饿）
- [ ] 双传输 parity（Sprint 位往返）
- [ ] `go test ./internal/... -short -race`、`cargo test`、`openspec validate`
