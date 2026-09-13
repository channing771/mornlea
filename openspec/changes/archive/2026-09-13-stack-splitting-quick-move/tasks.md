# 任务：完整分堆与快捷搬运

> 每任务组先写失败测试再实现（red → green → refactor）；测试与被测代码同目录；验证命令在任务组内全部通过并记入 ledger（按基线 SHA 复用）后才勾选。

- [ ] 1. core `MoveStackAmount` 原语
  - 文件：`packages/shared/core/inventory.go`（`MoveStackAmount(from, to uint8, amount uint8) (Inventory, bool)`：copy-on-write；目标空放 `min(amount, source.Count)`；同类合并 `min(amount, space)` 余量留源；异类非空、from==to、越界、源空、amount==0 一律 false 不改动）、同包测试（半组边界 ceil、单件、合并受 stack limit、异类拒绝、原子性、与 `MoveStack` 的行为对照）。
  - 验证：`go test ./packages/shared/core -race -count=1`。

- [ ] 2. 协议 v44：`MoveStackPartial` 与 `QuickMoveStack`
  - 文件：`packages/shared/network/protocol/packet.go`（`ProtocolVersion = 44` + 版本史）、`registry.go`（C→S 19/20 + 反查表 + 「下一空闲」推进 21）、`message_inventory.go` 或新文件（两命令 DTO + `Validate`：View 值域 {0,1,2}、域内索引上界 35/44/62|38、View≠2 Container 零值、View==2 合法 ContainerRef、Partial 的 From!=To）、`codec/codec_client.go`（编码/解码 + 定长校验）、wire frozen/越界拒绝/往返测试。
  - 验证：`go test ./packages/shared/network -race -count=1`。

- [ ] 3. sim 半组/单件三视图域
  - 文件：`packages/server/sim/entity/`（inventory 域接线 `CommandMoveStackPartial` 内联结算；crafting 域 amount 变体 + repack 预演；container 域 amount 变体进 `containerMoves` 延迟结算 + 熔炉槽约束 + repack 预演；`contract.go` 命令 kind/字段扩展）、同包测试（三域半组/单件矩阵、异类拒绝、熔炉约束、repack 不变量、数量服务端推导、拒绝原因）。
  - 验证：`go test ./packages/server/sim/... -race -count=1`。

- [ ] 4. sim 快捷搬运四方向
  - 文件：`packages/server/sim/entity/`（`CommandQuickMoveStack` 结算：container 域容器↔背包双向 + 熔炉智能槽优先级；crafting 域网格↔背包双向；inventory 域 hotbar↔backpack 互移；全部固定目标序 + copy-on-write 原子）、同包测试（四方向矩阵、余量留源、无可容纳拒绝、确定性重放、拒绝原因）。
  - 验证：`go test ./packages/server/sim/... -race -count=1`。

- [ ] 5. server ingress 与 parity
  - 文件：`packages/server/server/session_ingress.go`（两命令翻译）、同包 parity 测试（Memory/TCP 半组/单件/快捷搬运逐字段一致）。
  - 验证：`go test ./packages/server/server -race -count=1`。

- [ ] 6. 前端事件、桥与语义分支
  - 文件：`packages/engine/crates/mornlea_client/frontend/src/ui/GamePanels.tsx`（onContextMenu + shiftKey）、`src/bridge/schema.json` + `src/bridge/game.ts`、`packages/client/client/ui_game_bridge.go`（三端钉值同批）、`packages/client/cmd/mornlea/app/app_game_ui.go`（handleGameAction 分支 + 页脚文案）、vitest 组件断言（右键/shift 事件发射矩阵、提示行）。
  - 验证：`make frontend-check`；`go test ./packages/client/... -race -count=1`。

- [ ] 7. fixtures、门禁与基线
  - 文件：`frontend/visual` panel-* fixtures 与 `testdata/visual-golden/ui/` 重生成（文案变更）、根 `AGENTS.md`/`openspec/config.yaml`/`docs/notes/progress.md` 基线同步、全仓 `grep -rn "ProtocolVersion" packages/**/*_test.go` 钉值扫描（B-23/B-24 教训）。
  - 命令：`make frontend-visual-check`；`make visual-check`（世界 31 景零差异）；`test -z "$(gofmt -l .)"`；六模块 `go vet`；`make test-race`；`make rust` + `make rust-check`；`openspec validate --all --strict --no-interactive`；`go test ./packages/audit -count=1`。
  - 全部通过后按 ledger 记录证据，进入整分支终审与归档。
