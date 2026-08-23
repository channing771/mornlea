# Tasks

## 1. OpenSpec 契约与 v25 wire 冻结

- 目标文件/包：`openspec/changes/authoritative-player-melee/`、`internal/network`、`cmd/mornlea`、`cmd/mornlea-server`、`AGENTS.md`、`CLAUDE.md`。
- 验证命令：`make rust`；`go test ./internal/network ./cmd/mornlea ./cmd/mornlea-server -count=1`；`go test ./internal/archcheck -count=1`；`cmp -s AGENTS.md CLAUDE.md`；`openspec validate authoritative-player-melee --strict --no-interactive`；`git diff --check`。
- [x] 写入近战与采掘分流的 delta specs、设计、提案和 ledger。
- [x] 先更新失败测试，再将协议版本升至 v25；保持 `PlayerInput` 与所有 play packet wire 不变。
- [x] 更新握手 golden、fuzz 种子、当前版本断言及双份基线文档。
- [x] 运行网络、入口、架构和 OpenSpec 验证。

## 2. 权威近战裁决

- 目标文件/包：`internal/sim`（及同包近战测试）、`internal/network` 的既有输入契约测试、`openspec/changes/authoritative-player-melee/`。
- 验证命令：`make rust`；`go test ./internal/sim -race -count=1`；`go test ./internal/network -count=1`；`go test ./... -race`；`go vet ./...`；`openspec validate authoritative-player-melee --strict --no-interactive`。
- [x] 在 `sim` 实现 active 同维玩家的 3 格、最近命中、`SessionID` 平局、方块遮挡和流体穿透。
- [x] 通过既有伤害入口结算 2 点伤害和 10 tick 目标冷却，并覆盖同 tick 意图快照与采掘抑制。
- [ ] 验证单机与 TCP 共用权威路径。
