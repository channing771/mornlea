# E-11 `server-test-wait-budget` 执行账本

- 认领基线：`0989103a`（`codex/fix-E-11-server-test-wait-budget`）。
- 内容确认：`bounded`；用户于 2026-08-25 明确批准“共享 3000 tick 登录等待预算、迁移主动 tick 登录循环、保留已有 context/墙钟等待”的短设计。
- 基线验证：`make rust` 通过；十二条相关登录/Memory-TCP 定点测试通过（`go test ./internal/server -race -count=1 -run ...`，8.974s）。
- Ruling: change 设置 `skip_specs: true` — 本行只改变测试基础设施和失败诊断，不改变玩家可观察行为或主规格 Requirement — 为通过校验虚构产品 capability 会让规格失真。
- Ruling: 迁移边界是十二个“测试主动推进 tick + 登录就绪”的循环 — 其中十个无总预算、farming/hunger 两个已有内联预算；`Recv(ctx)`、墙钟 deadline、有限次数及业务阶段等待保留 — 扩到全包所有循环会改变无关断言语义。

## Task 1

- Implementer：待派发。
- RED/GREEN：待记录。
- 迁移清单：待记录。
- 定点与全量门禁：待记录。
- SPEC review：待裁决。
- QUALITY review：待裁决。
- 修复轮次：0/5。

## 整分支终审与收尾

- 整分支终审：待执行。
- 最终门禁：待执行。
- 归档、规划回填与 PR/CI：待执行。
