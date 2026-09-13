# 实现计划：完整分堆与快捷搬运（B-35）

> OpenSpec change：`openspec/changes/stack-splitting-quick-move/`（唯一契约来源）。上游设计：`docs/superpowers/specs/2026-09-13-b35-stack-splitting-design.md`（已批准）。
> 工作分支：`feat/B-35-stack-splitting`（worktree `.worktrees/B-35-stack-splitting`）。

## Global Constraints（每个任务组都必须遵守）

- 版本纪律：本 change 只升协议 v43→v44（两条 C→S pure-append 命令 19/20）；engine ABI v11、client ABI v19、全部存档 schema、benchmark scenario v23 一律不动；桥 schema 三端钉值扩展不改 client ABI。
- 数量纪律：移动数量只由服务端按源栈推导（半组 `ceil(n/2)`、单件 1）；客户端不得传任意数量。
- 原子纪律：全部移动 copy-on-write 试算后原子写回；失败零改动；拒绝原因零新增（复用 `RejectPlayerNotReady`/`RejectInvalidSlot`/`RejectInvalidInput`）。
- 确定性纪律：快捷搬运目标序为固定契约（AddStack 4 相位 / 统一索引升序扫描 / 熔炉输入优先于燃料）；无 map 遍历。
- 既有约束复用：crafting repack 预演、熔炉槽位物品约束、箱子视图规则、结算相位（inventory/crafting 内联、container 延迟至区块写相位）逐条复用不放宽。
- 注释纪律：新代码注释一律中文；禁止任务编号（`[A-F]-[0-9]{2}`）出现在生产或测试代码注释。
- 测试纪律：red → green → refactor；测试与被测代码同目录；一个测试文件一个主题；不跑全量 race（收尾门禁）。
- Git 提交信息：单行英文 `<type>(<scope>): <subject>`；无正文无页脚无 Co-Authored-By。
- 独占文件集：`packages/shared/core`、`packages/shared/network`、`packages/server/sim`、`packages/server/server`、`packages/client`（client/ui_game_bridge/app_game_ui 及测试）、`packages/engine/crates/mornlea_client/frontend`（GamePanels/bridge/visual）、`packages/audit`（基线钉值）、OpenSpec change、基线文档；不触碰 `docs/notes/lan-server.md` 与其它在途 worktree。

## Task 1: core `MoveStackAmount` 原语

文件：`packages/shared/core/inventory.go`（`MoveStackAmount(from, to, amount uint8) (Inventory, bool)`——目标空放 `min(amount, source.Count)`；同类合并 `min(amount, space)` 余量留源；异类非空/from==to/越界/源空/amount==0 一律 false 不改动；沿 `MoveStack` 形状与注释口径）、同包 `inventory_test.go` 或新主题文件（半组 ceil 边界 1..9、单件、合并受 stack limit 截断、异类拒绝、原子性全量对照、与 `MoveStack` 行为差异表）。
验证：`go test ./packages/shared/core -race -count=1`。

## Task 2: 协议 v44 两命令

文件：`packages/shared/network/protocol/packet.go`（`ProtocolVersion=44`+版本史一行）、`registry.go`（C→S 19 `MoveStackPartial`/20 `QuickMoveStack`+反查+下一空闲 21）、新 message 文件（DTO+`Validate`：View {0,1,2}；索引上界按 View（0→35、1→44、2→Container.Kind 分派 62/38）；View==2 合法 ContainerRef、View≠2 零值；Partial From!=To；定长常量与注释）、`codec/codec_client.go`（编解码+精确字节长）、测试（wire frozen 定长、往返、越界/非法 View/零值违规/From==To 拒绝矩阵）。
验证：`go test ./packages/shared/network -race -count=1`。

## Task 3: sim 半组/单件三视图域

文件：`packages/server/sim/contract/contract.go`（`CommandMoveStackPartial` kind+字段：View/From/To/Single 复用 Command 既有字段位）、`packages/server/sim/entity/`（inventory 域内联 `MoveStackAmount`；crafting 域 amount 变体（`applyMoveCraftingStack` 泛化或伴生函数）+`canRepackCrafting` 预演+异类拒绝；container 域 amount 变体进 `containerMoves`+`mergeStacks` amount 版+熔炉约束+repack 预演）、`tick.go` 接线、同包测试（三域矩阵：半组/单件 × 空/同类/异类目标 × 熔炉三槽 × repack 边界；数量推导；拒绝原因；Memory 语义与相位）。
验证：`go test ./packages/server/sim/... -race -count=1`。

## Task 4: sim 快捷搬运四方向

文件：`packages/server/sim/contract/contract.go`（`CommandQuickMoveStack`）、`packages/server/sim/entity/`（结算：container 域容器区→背包 AddStack/背包→箱子 36..62 首个可容纳/背包→熔炉输入优先燃料次之/均无拒绝；crafting 域网格→背包 AddStack、背包→网格 0..8 首个可容纳+repack 预演；inventory 域 hotbar↔backpack 对侧 AddStack 序；余量留源）、`tick.go` 接线（container 方向延迟相位）、同包测试（四方向矩阵、熔炉优先级（生铁/煤/两类皆是）、无可容纳拒绝、余量留源、确定性重放逐格一致）。
验证：`go test ./packages/server/sim/... -race -count=1`。

## Task 5: server ingress 与 parity

文件：`packages/server/server/session_ingress.go`（两命令翻译）、同包 parity 测试（Memory/TCP：半组/单件/快捷搬运三组命令的镜像与拒绝投影逐字段一致）。
验证：`go test ./packages/server/server -race -count=1`。

## Task 6: 前端事件、桥与语义分支

文件：`frontend/src/ui/GamePanels.tsx`（`slotButton` onClick 携带 shiftKey、新增 onContextMenu preventDefault）、`frontend/src/bridge/schema.json`+`game.ts`（slot 变体 +button +shift required）、`packages/client/client/ui_game_bridge.go`（requireExactKeys 同步）、`packages/client/cmd/mornlea/app/app_game_ui.go`（handleGameAction：shift+左键→QuickMoveStack 直发并清源；右键无源→选中；右键有源→MoveStackPartial{Single:shift}；左键无 shift→现状不变；页脚文案）、vitest（事件发射矩阵：左/右 × shift × 有无源的六态、提示行断言、onContextMenu 不冒泡菜单）。
验证：`make frontend-check`；`go test ./packages/client/... -race -count=1`。

## Task 7: fixtures、门禁与基线

文件：`frontend/visual` panel-* fixture 更新与 `testdata/visual-golden/ui/` 重生成（页脚文案变更所致）、根 `AGENTS.md`/`openspec/config.yaml`/`docs/notes/progress.md` 基线同步（协议 v44）、全仓测试钉值扫描 `grep -rn "ProtocolVersion" packages --include=*_test.go`（B-23/B-24 教训：升版必须全仓扫钉值，含 `cmd/mornlea-server/main_test.go`）。
命令：`make frontend-visual-check`；`make visual-check`（世界 31 景零差异）；`test -z "$(gofmt -l .)"`；六模块 vet；`make test-race`；`make rust`+`make rust-check`；`openspec validate --all --strict --no-interactive`；`go test ./packages/audit -count=1`。

## 验收

1. 右键两击=半组、Shift+右键两击=单件、Shift+左键=快捷搬运、左键两击整堆不变。
2. 数量全部服务端推导；异类非空目标部分移动拒绝且零改动。
3. 协议 v44 唯一升版；其余矩阵不动；世界 golden 零差异；panel-* 随文案重生成。
4. Memory/TCP parity 逐字段一致。
