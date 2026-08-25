# 攻击疲劳（B-13 攻击半边）任务

## 1. 测试基建与 RED

- [ ] 1.1 把 `internal/sim` 的共享 melee 测试 helper（`readyMeleePlayers`、`setMeleePlayer`）从 `combat_test.go` 迁入新建中心 `internal/sim/helpers_test.go`：零行为变化重组，测试函数名与子测试名一律不动。验证：`go test -list '.*' ./internal/sim` 前后集合一致（按集合语义），`go test ./internal/sim -count=1` 全绿。
- [ ] 1.2 新建 `internal/sim/combat_exhaustion_test.go`，先写失败测试证三条性质：(a) 近战命中使攻击者疲劳值增加 100 千分位且目标疲劳值不变；(b) 累积跨过 `ExhaustionThresholdMilli` 时经饱和度→饥饿值逐级结算的完整链路；(c) 无目标/方块遮挡/受击冷却免疫三种未命中路径疲劳值不变。验证：`go test ./internal/sim -run Exhaustion -count=1` 必须 RED。

## 2. 实现 GREEN

- [ ] 2.1 `internal/sim/hunger.go` 固定表追加 `exhaustionMeleeMilli = 100` 常量行（中文注释说明判定点与参考实现 ×1000 换算口径）；`internal/sim/combat.go` 的 `advancePlayerMelee` 意图冻结处对攻击者调用 `attacker.applyExhaustion(exhaustionMeleeMilli, engine.tunables.ExhaustionThresholdMilli)`。验证：`go test ./internal/sim -race -count=1` 全绿（含既有近战/饥饿测试零回归）。

## 3. 收尾门禁

- [ ] 3.1 包级定点验证：`go test ./internal/sim ./internal/archcheck -race -count=1`；`go vet ./...`；`gofmt -l .` 无输出；`openspec validate --all --strict --no-interactive` 通过。
- [ ] 3.2 全量验证：`go test ./... -race -count=1` 全绿并记录结果；确认无协议/存档/golden 影响（本 change 不触碰 capture/benchmark 路径）。
