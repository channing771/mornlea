# Ledger：权威护甲

## 基线

- 分支 `feat/B-24-armor`（worktree `.worktrees/B-24-armor`），基于 main `f10fbb57`。
- 版本槽位持有：协议 v41→v42、玩家 schema v8→v9、client ABI v18→v19。

## 内容确认（阶段 1）

- 2026-09-12 控制会话以 brainstorming 分类为 **bounded**（沿门/床/夜行者行先例的既有流程改动），短设计经用户**显式批准**（AskUserQuestion 四项裁决全取推荐项）：仅铁质一套、4 槽、点数 + 4%/点减免、耐久随本行交付。
- 已核实的代码事实：`playerState.applyDamage` 为全部伤害来源唯一入口（`combat.go:384`、`player.go:646`、`oxygen.go:36`、`hunger.go:158`）；近战结算单点 `settleCombatIntent`；`PlayerState` 尾部追加先例与协议 v41；HUD 状态行已迁 WebView 组件；`ItemIDMax = 58`、C→S 命令 ID 下一空闲 18（ID 1 已废止不复用）。

## Rulings

- `Ruling: 护甲四件的快捷栏/HUD 物品 sprite 由顺延改为任务组 6 交付 — 实现者实测客户端存在「所有注册物品必须有图标」的守护测试（`packages/client/cmd/mornlea/app` 21 例因物品 58 缺图标变红），顺延将使分支长期带红测试 — 2026-09-12 proposal 延期节原决策被推翻，proposal 与 tasks.md 已同步修订。`
- `Ruling: 损坏形态护甲以「数量 1、耐久 0」原地表达，不新增损坏护甲物品编号 — 规格只允许 58..61 四个编号；`ArmorPoints` 完好判定 = Count≥1 且耐久 1..上限，越界 fail closed 计 0 点 — 与 design D6 一致，测试已钉住。`
- `Ruling: `ReducedDamage` 对非正伤害返回 0 — 沿 core 值域函数不 panic 惯例；中间量 int64 收敛 int32 防极端回绕 — 实现者裁量，测试已钉住。`
- `Ruling: 玩家 schema v9 装备区由「每槽 3 字节、共 12 字节」更正为「每槽沿用背包格同一 5 字节栈编码（item u16 小端 + count 1 字节 + durability u16 小端）、共 20 字节」 — 任务组 3 实现发现原前提与 codec 现实不符（3 字节只是 v2/v3 legacy 无耐久布局），而耐久逐位保真与重启保值 MUST 只有 5 字节编码可满足；按「先改 OpenSpec 产物再编码」规程处理，四处产物已同步并附修订记录 — 控制会话追认。`
- 待办登记：`packages/shared/core/recipe_shape_internal_test.go` 头注释「recipe 1..19」陈旧（现 1..24，断言仍成立），后续任务组顺手修正。

## 验证证据（按 SHA）

- SHA `0fb77e74`（任务组 1）：`go test ./packages/shared/core -race -count=1` → ok 1.461s，verbose 235 PASS / 0 FAIL；`gofmt -l packages/shared/core` 无输出；`go vet ./packages/shared/core/...` 干净；worktree 首跑前已执行 `make rust`（成功），六模块 `go build ./...` 通过。已知下游影响：`packages/client/cmd/mornlea/app` 21 例因护甲缺图标红（任务组 6 交付 sprite 后清偿）。评审（SHA `0fb77e74`）：SPEC PASS / QUALITY PASS，零 blocking；待办仅 `recipe_shape_internal_test.go` 头注释陈旧。
- SHA `4e5b91df`（任务组 2，含修复轮 `4e5b91df`）：首轮 `a654843b` 全部 `packages/shared/network/... -race` 4 包 ok、fuzz 冒烟 10s 0 失败、六模块 build 通过；评审 SPEC PASS / QUALITY FAIL（1 blocking：子树外 3 处协议钉值测试仍断言 41，重蹈 v41 升版失误模式）；修复轮 `4e5b91df` 后定点复核全绿——`go test ./packages/server/cmd/mornlea-server -count=1` ok 1.259s、`go test ./packages/client/cmd/mornlea/app -count=1 -run Protocol` ok、`go test ./packages/shared/network -race -count=1` ok 1.862s（gofmt/vet 三处包树干净），按审查者预授权标准达标关闭。已知豁免维持：app 包 21 例图标红待任务组 6。
- SHA `dbd2e908`（任务组 3）：`go test ./packages/server/storage/... -race -count=1` 全 7 包 ok；`go test ./packages/server/server/persistence -race -count=1` ok；gofmt/vet 干净；六模块 build 通过；全仓钉值扫查完成（功能性钉值仅 player 包 2 处已推进，根包 `player_store_test.go` 用 `CurrentSchema+1` 自动跟随）。已知红（登记豁免，任务组 7 清偿）：`go test ./packages/audit -run TestBaselineVersionsMatchCode`——根 `AGENTS.md` 与 `openspec/config.yaml` 版本矩阵滞后（协议 v42 自任务组 2 起、玩家 schema v9 自本组起）。
- SHA `aa49b345`（任务组 3 关闭，含修复轮 `aa49b345`）：首轮 `dbd2e908` 评审 SPEC PASS / QUALITY FAIL（2 major：两处新写测试注释残留旧 12/6 字节布局数字，断言与行为全部合格）；修复轮纯注释更正后 `go test ./packages/server/storage/player -count=1` ok 全绿、gofmt 干净，按审查者预授权标准关闭。评审备忘移交任务组 5：`packages/server/server/persistence/players_snapshot.go` 的 `cachedPlayerFromStored`/`restore`/`save`/`matchesSave`/`playerSnapshotsEqual` 五处必须同步装备区，否则仅装备变化的 tick 判无变化永不落盘。
- SHA `9b5f30b9`（任务组 4）：`go test ./packages/server/sim/... -race -count=1` contract/entity/realm/runtime 全 ok；gofmt/vet 干净；六模块 build 通过。偏离（控制会话追认）：`contract` 兄弟包最小越界（恢复/快照/命令枚举的代码现实所在）；`PlayerHash` 装备区含耐久（每槽 5 字节局部编码——耐久直接决定减免与损坏形态，省略会使耐久漂移对 parity 不可见）；`PlayerUpdate.ArmorPoints` 投影提前到本组（spec 场景与任务组 5 parity 均需）。spec 场景「占用槽位互换」GIVEN 措辞不自洽已由控制会话更正（胸部槽位+胸甲）。已知红（stash 干净 HEAD 复核确认为分支既有，非本组引入）：server 包 `TestDualDimensionReloadAfterRestart`、`TestWarpParityMemoryVsTCP`（`unsupported passive dimension 1`，与积压表 F-11 在案 flake 同族）；audit 包版本矩阵红（任务组 7）+ `not_armor` 标识符红（任务组 2 引入，audit 钉值清单随任务组 7 推进）。
- SHA `9b5f30b9`（任务组 4 关闭）：评审 SPEC PASS / QUALITY PASS，零 blocking/major，3 条 nit 留档（占用互换测试实例化用头部槽而非场景字面胸部槽——机制同构另有跨槽覆盖；空槽拒绝无独立用例——与石剑同路径；`consumeArmorDurability` 注释「同口径」措辞略强）——不构成修复轮，后续任务组可顺手。focused 复核 `go test ./packages/server/sim/... -race -count=1` 四包全 ok。任务组 5 备忘再次确认：`players_snapshot.go` 五处同步装备区（含脏检测，否则仅耐久变化的 tick 永不落盘）。
