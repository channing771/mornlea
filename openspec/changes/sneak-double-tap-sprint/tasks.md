# Tasks: sneak-double-tap-sprint

> 任务分解见 `docs/superpowers/plans/2026-09-11-sneak-double-tap-sprint.md` Task 1–7；本文件逐项引用，目标文件与验证命令以计划为准。

## 1. 协议 v41 与 codec 尾部追加（计划 Task 1）
- [ ] `packages/shared/network/protocol/message_command.go` 追加 `Sneaking bool`（`Sprinting` 后）
- [ ] `packages/shared/network/protocol/packet.go` `ProtocolVersion 40→41` + 版本史注释
- [ ] `packages/shared/network/codec/codec_client.go` 编解码尾部追加 1 bool
- [ ] `packages/shared/network/codec/hunger_test.go` 载荷长度与偏移常量 `v41`
- [ ] `packages/shared/network/codec/codec_golden_test.go` golden 夹具追加 `Sneaking` 真/假
- [ ] Run: `go test ./packages/shared/network/... -race -count=1`

## 2. 合同搬运链（计划 Task 2）
- [ ] `packages/server/sim/contract/contract.go` `Command` 追加 `Sneaking`
- [ ] `packages/server/server/session_ingress.go` `PlayerInput→Command` 搬运
- [ ] `packages/server/sim/entity/tick.go` `CommandPlayerInput→physics.Input` 搬运 + `sneakingHeld` 锁存
- [ ] `packages/server/sim/entity/player.go` `playerState` 加 `sneakingHeld bool`
- [ ] Run: `go test ./packages/server/sim/contract ./packages/server/server ./packages/server/sim/entity -race -count=1`

## 3. 物理 header v4 与 Rust 双侧积分（计划 Task 3）
- [ ] `packages/shared/physics/types.go` `Input` 加 `Sneaking` + 默认潜行倍率常量 `0.3`
- [ ] `packages/shared/physics/tunables.go` 加 `SneakSpeedMultiplier` + `DefaultTunables`
- [ ] `packages/shared/physics/step.go` 布局 `v3→v4`、编码 `130`/`152:156`、潜行优先门控
- [ ] `packages/engine/crates/mornlea_engine/src/step.rs` 解码 + 积分潜行分支 + 保留区校验
- [ ] Run: `make rust && go test ./packages/shared/physics -race -count=1`

## 4. 边缘保护纯函数与双侧接入（计划 Task 4）
- [ ] 新建 `packages/shared/physics/sneak_edge.go` + `sneak_edge_test.go`
- [ ] `packages/server/sim/entity/player.go` 物理步之前钳制
- [ ] `packages/client/client/predictor_advance.go` 预测同序钳制
- [ ] Run: `go test ./packages/shared/physics ./packages/server/sim/entity ./packages/client/client -race -count=1`

## 5. sim 门控、疲劳压制与交互分流（计划 Task 5）
- [ ] `packages/server/sim/entity/player.go` 饥饿门控后潜行压制 + 疲劳条件
- [ ] `packages/server/sim/entity/container.go` `openContainer` 潜行拒绝
- [ ] `packages/server/sim/entity/door.go`、`sleep.go` 门/床交互潜行分流预留
- [ ] Run: `go test ./packages/server/sim/... -race -count=1`

## 6. 客户端双击状态机、上行与放置分支（计划 Task 6）
- [ ] `packages/client/client/input.go` `InputState` 双击状态机（假时钟可注入，窗口 `300ms`）
- [ ] `packages/client/cmd/mornlea/app/interactive.go` 采样 + 接线，`Ctrl` 退役
- [ ] `packages/client/client/predictor.go` `Control` 加 `Sneaking`
- [ ] `packages/client/client/predictor_advance.go` 上行 + 镜像门控
- [ ] `packages/client/cmd/mornlea/app/app_input.go` `placeBlock` 潜行分支
- [ ] Run: `go test ./packages/client/... -race -count=1`

## 7. 全链集成、门禁与归档收尾（计划 Task 7）
- [ ] 跨端 parity 补测（Memory transport 潜行前进 + 悬崖边钳制场景，TCP/Memory 双路往返）
- [ ] Run: `make rust`
- [ ] Run: `go test ./packages/audit -count=1`
- [ ] Run: `make dev-check`（六模块 vet 与相关守卫）
- [ ] Run: `make visual-check`（30 景零漂）
- [ ] Run: `make test-race`（六模块全量 race）
- [ ] Run: `openspec validate --all --strict --no-interactive`
- [ ] Run: `gofmt -l packages/shared packages/server packages/client | grep . && exit 1 || true`
- [ ] 同步主规格（`sprint` MODIFIED + 新建 `sneak` 能力）并归档 change 目录
