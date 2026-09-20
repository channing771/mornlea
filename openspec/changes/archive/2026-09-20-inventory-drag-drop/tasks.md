# Tasks

## 1. 协议与契约：DropStack（协议 v45）

- 目标文件：`packages/shared/network/protocol/message_drop_stack.go`（新）与测试、`registry.go`、`packet.go`、`packages/server/sim/contract/contract.go`
- C→S ID 21 钉死、解码校验（视图/槽位域）、协议常量 v45；`packages/audit` 基线与根 `AGENTS.md`、`openspec/config.yaml` 版本矩阵同步
- 验证：`go test ./packages/shared/network/protocol -race -count=1`；`go test ./packages/audit -count=1`

## 2. 服务端权威结算

- 目标文件：`packages/server/server/session_ingress.go`、`packages/server/sim/entity/drop.go`、`packages/server/sim/entity/tick.go`、`packages/server/sim/entity/drop_stack_test.go`（新）
- 三视图结算（背包/合成内联、容器延迟通道）、空槽与非法拒绝、脚下整组投放与掉落同步
- 验证：`go test ./packages/server/sim/entity ./packages/server/server -race -count=1`

## 3. 桥与 Go 翻译

- 目标文件：`packages/client/client/ui_game_bridge.go`、`packages/client/cmd/mornlea/app/app_game_ui.go` 及测试
- `dragMove` 复用既有 slot 映射发 Move*；`drop` 发 `DropStack`；非法载荷拒绝
- 验证：`go test ./packages/client/client ./packages/client/cmd/mornlea/app -race -count=1`

## 4. 前端拖拽交互

- 目标文件：`frontend/src/ui/GamePanels.tsx`、`frontend/src/bridge/game.ts`、`frontend/src/bridge/schema.json` 及测试
- 拖起/跟随/落槽/落空白/落面板外五路径、Esc 取消、未确认不发起、拖拽中权威刷新并存
- 验证：`make frontend-check`

## 5. 收尾门禁

- `gofmt`；六模块 `go vet` 或 `make dev-check`；`openspec validate inventory-drag-drop --strict --no-interactive`
