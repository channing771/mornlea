# 设计：fix-passive-cow-behavior

三处修复互不依赖，共用一个 change 以便一次评审回退；实现层各占一个任务组，按「先失败测试后最小实现」推进。

## 修复 1：漫游分段稳定朝向（服务端）

- 位置：`packages/server/sim/entity/passive.go` 的 `passiveStepInput` 漫游分支（现 329-332 行：`splitmix64(seed ^ tick ^ id)` 每 tick 重抽）。
- 方案：新增常量 `passiveWanderSegmentTicks = 40`（2 秒）；段序号 `segment = tick / 40`；目标朝向 `yaw = normalizeYaw(splitmix64(seed ^ segment ^ id) & 0xFFFFFF …)`——段内因 segment 不变而稳定；`entry.yaw = turnYawToward(entry.yaw, want, passiveIdleLookMaxTurn)` 段间有界过渡。
- 既有守护不变：出生区块邻域回滚（`outsideHomeNeighborhood`）、确定性纯函数（不读全局随机数）。
- 测试陷阱：既有漫游测试（`passive_test.go`、`passive_idle_test.go`）循环里不推进 tick，冻结 tick 时新旧实现都给出恒定朝向，无法区分。新测试 MUST 逐 tick 递进 `engine.tick`，断言「段内目标朝向恒定」与「跨段单 tick 转角 ≤ 0.2 rad」。

## 修复 2：闲时看人只看不靠近（服务端）

- 位置：`passiveStepInput` 闲时看人分支（现 316-328 行）。
- 方案：删除「>1.5 格 MoveZ:1 靠近」位移分支，仅保留 `turnYawToward` 转向；`passiveIdleLookStopDistance` 常量随之失去唯一消费者，一并删除（防止留下死语义）。
- 不变量：引诱分支（持麦、2.5 格止步）、优先级链、`passiveIdleLookTarget` 扫描均不动。
- 测试：`passive_idle_test.go` 断言从「靠近到约 1.5 格」改为「水平位置不动 + 朝向收敛到面向玩家」；`sim/runtime/passive_step_test.go`（Step 接线，玩家摆在 10 格外避开 idle-look）复核不受影响。

## 修复 3：渲染朝向装配层对齐（客户端）

- 位置：`packages/client/cmd/mornlea/app/app_render.go` 的 `AppendPassiveRenderPresentationsInto`（141 行 `Yaw: presentation.Yaw` 直传）。
- 方案：牛模型静止面朝局部 +X（`render/avatar.go` 牛头在 +X，`passive_avatar_test.go` 已锁定），物理 `yaw=0` 面向 -Z；装配处对被动牛 yaw 施加 `yaw + π/2` 并归一化，其余实体不动。低头俯仰、闲时点头、死亡侧倒通道不受影响。
- 测试：`passive_avatar_test.go` 的「牛面朝 +X」类断言更新为「给定权威 yaw，模型头部指向物理前进方向 (-sin yaw, -cos yaw)」。

## 数据所有权与并发

- 无新增字段、无 schema/协议/ABI 变更；权威 yaw 仍是唯一朝向真值，装配偏移是纯呈现层映射。
- tick 串行读写边界不变；哈希计算量级不变（每 tick 每牛一次 splitmix64），权威 tick 预算无放大。

## 被否决的替代方案

- **服务端发布时换算 yaw**：会污染权威 yaw 语义（逃跑/引诱内部计算与 wire 值都被迫带呈现偏移），且 `PassiveState` 协议字段失去正交性——被否决，映射留在客户端装配层。
- **漫游增加站立停顿段**：观感更拟真但超出本修复范围，列为后续打磨候选。
- **完全移除闲时看人**：牛对玩家毫无反应显得死板，保留「只看」维持生命力且不贴脸。

## 验证方法

- `go test ./packages/server/sim/... -race -count=1`
- `go test ./packages/client/... -race -count=1`（聚焦 `-run 'Passive|Avatar'` 后全量）
- `go test ./packages/audit -count=1`
- 收尾：`gofmt -l`、六模块 `go vet`（经 `make dev-check`）、`make test-race`、`openspec validate --all --strict --no-interactive`
