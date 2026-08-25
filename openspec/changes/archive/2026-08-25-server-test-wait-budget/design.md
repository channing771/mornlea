## Context

动机见 [proposal.md](proposal.md)。当前 `internal/server` 已有两套互补的等待约束：真实异步 goroutine/TCP 接收使用 `context` 或墙钟 deadline；由测试调用 `StepForTest`/`parityStep` 主动推进的确定性模拟则适合用 tick 预算。后者已有 `farmingLoginBudget` 与 `hungerLoopLoginBudget` 两份 3000 tick 的内联实现，但十个同形登录循环仍没有聚合预算。

本变更只涉及 `package server` 的测试代码。受影响循环分布在 farming、hunger、till、material processing、eating、transport parity、fluid、block light、placement success 与 player melee 测试中；它们都在同一 goroutine 更新局部 Ready/背包/镜像状态，不引入新的并发所有权。

## Goals / Non-Goals

**Goals:**

- 让所有由测试主动推进权威 tick 的登录就绪循环共享同一个宽松预算和同一失败格式。
- 保留每个调用方对“就绪”的精确定义，并在失败时输出该场景的动态状态。
- 用 helper 自身的单元测试锁定边界，再由原集成用例证明迁移不改变脚本语义。

**Non-Goals:**

- 不把真实墙钟等待改成 tick 等待，也不把 tick 预算当作性能阈值。
- 不抽象登录消息解析或合并不同测试的业务断言；helper 只控制循环活性。
- 不修改 `server_test` 外部包、生产代码、网络/存档契约或全仓 deadline 常量。

## Decisions

### D1：按推进次数计预算，固定为 3000 个权威 tick

共享常量命名为 `integrationLoginTickBudget`，沿用 farming/hunger 在高负载环境验证过的 3000。helper 每次只在条件未满足时调用一次 `step`；条件在第 3000 次推进后成立仍算成功，第 3001 次推进不会发生。

选择 tick 而不是墙钟，因为这些循环的进展由测试显式调用 `StepForTest`/`parityStep` 驱动：tick 数能稳定描述“脚本空转了多久”，不随 CI 调度速度改变。否决给每个循环加 `time.Now` 的方案，因为它继续复制控制逻辑，且在慢 runner 上更容易假红。

### D2：helper 只拥有活性，不拥有就绪语义

在现有 `tcp_integration_helpers_test.go` 中新增共享 helper。调用方传入：

- 场景标签；
- `ready func() bool`；
- `diagnostics func() string`；
- `step func()`。

helper 在进入循环和每次推进后都通过下一轮条件检查允许立即成功；耗尽时用 `t.Fatalf` 输出标签、固定预算及动态诊断。消息分类、镜像应用、背包等值判断和双玩家 Ready 合并仍留在原测试，避免一个“大而全”的登录 fixture 隐藏测试差异。

否决抽取统一 `waitParityReady` 并在 helper 内解析消息的方案，因为 block-light、melee 与普通 parity 的 step 返回形状不同，而且部分用例还需要同时更新第二个镜像或远端状态；统一解析会把业务语义塞进基础设施。

### D3：通过最小测试接口验证失败路径

helper 接收只包含 `Helper`/`Fatalf` 的测试接口，生产调用仍传 `*testing.T`。主题测试文件用记录型 fake 验证三条性质：初始满足时零推进、预算内满足时精确推进、永不满足时恰好推进 3000 次并输出调用方诊断。这样无需启动会故意失败的子测试或依赖五分钟包级 timeout。

否决只依赖迁移后的集成测试为 helper 提供覆盖，因为正常集成路径不会走预算耗尽分支，最重要的诊断契约会没有可执行证据。

### D4：迁移边界按“主动 tick + 登录就绪”判定

迁移十个无总预算循环，并把 farming/hunger 的两份内联预算收敛到 helper，共十二个调用点。以下形态保留原样：

- `Recv(ctx)` 已共享一个聚合 context 的等待；
- 已检查 `time.Now`/`ctx.Err` 的墙钟循环；
- 固定次数的远端玩家、命令收敛或业务阶段循环；
- 缺席断言、故意超时断言和性能门禁。

这条边界防止本测试整理顺手改变其它等待的语义或容错尺度。

## Risks / Trade-offs

- [3000 tick 使真实挂起仍会消耗一些 CPU] → 这是非性能门禁的宽松上界；正常路径约数百 tick 即返回，挂起也远早于五分钟全局超时。
- [调用方诊断遗漏某个条件] → helper 强制接收诊断回调；评审逐调用点核对其包含同一 `ready` 条件使用的状态。
- [机械迁移改变一次推进的先后顺序] → helper 采用“先查条件、预算未耗尽再 step”的循环，并用原定点集成测试覆盖；实现 diff 不重写消息处理闭包。
- [后续新增同形循环绕开 helper] → helper 与设计成为首选入口；本变更不增加扫描源码的脆弱 AST 门禁，避免把合法的其它循环误判为登录等待。

## Migration Plan

1. 先为尚不存在的 helper 写编译失败测试，随后实现 helper，使主题测试转绿。
2. 逐调用点迁移十二个登录循环，保持原消息处理和业务断言不变；每批运行对应定点用例。
3. 运行完整 `internal/server` race 测试、archcheck、全仓 race、vet、gofmt 与 OpenSpec strict 校验。

本变更没有数据或运行时迁移。回退只需还原测试 helper 与调用点；不会影响存档、线上协议、发布产物或玩家世界。
