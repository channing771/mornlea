## 1. 漫游分段稳定朝向（服务端）

- [x] 1.1 失败测试先行：`packages/server/sim/entity` 新增「段内朝向稳定 + 跨段有界转向」测试（逐 tick 递进 `engine.tick`，锁定段长 40 与单 tick 转角上限 0.2 rad）；随后最小实现 `passive.go` 漫游分支分段派生（`passiveWanderSegmentTicks` 常量、段序号哈希、`turnYawToward` 过渡）（`go test ./packages/server/sim/entity -race -count=1 -run 'Passive'`）
- [x] 1.2 既有漫游/边界测试改为推进 tick 语义并复核全绿（`packages/server/sim/entity/passive_test.go`、`passive_idle_test.go` 等；`go test ./packages/server/sim/... -race -count=1`）

## 2. 闲时看人只看不靠近（服务端）

- [x] 2.1 失败测试先行：闲时玩家 4 格内牛水平位置不动且朝向收敛面向玩家（`packages/server/sim/entity/passive_idle_test.go` 断言重写）；随后最小实现删除靠近位移分支与 `passiveIdleLookStopDistance` 死常量；复核 `sim/runtime/passive_step_test.go` 不受影响（`go test ./packages/server/sim/... -race -count=1`）

## 3. 渲染朝向装配层对齐（客户端）

- [x] 3.1 失败测试先行：给定权威 yaw，装配后模型头部指向物理前进方向（`packages/client/render/passive_avatar_test.go` 断言更新）；随后最小实现 `packages/client/cmd/mornlea/app/app_render.go` 装配处 +π/2 归一化偏移（`go test ./packages/client/... -race -count=1`）

## 4. 收尾门禁

- [x] 4.1 全量验证与归档就绪（`gofmt -l` 相关包无输出、六模块 `go vet` 与 `make dev-check`、`make test-race` 六模块全量 race、`go test ./packages/audit -count=1`、`openspec validate --all --strict --no-interactive`；记录被动相关 capture 基线不受影响或按需重录）
