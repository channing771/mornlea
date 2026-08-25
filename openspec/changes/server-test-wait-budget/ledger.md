# E-11 `server-test-wait-budget` 执行账本

- 认领基线：`0989103a`（`codex/fix-E-11-server-test-wait-budget`）。
- 内容确认：`bounded`；用户于 2026-08-25 明确批准“共享 3000 tick 登录等待预算、迁移主动 tick 登录循环、保留已有 context/墙钟等待”的短设计。
- 基线验证：`make rust` 通过；十二条相关登录/Memory-TCP 定点测试通过（`go test ./internal/server -race -count=1 -run ...`，8.974s）。
- Ruling: change 设置 `skip_specs: true` — 本行只改变测试基础设施和失败诊断，不改变玩家可观察行为或主规格 Requirement — 为通过校验虚构产品 capability 会让规格失真。
- Ruling: 迁移边界是十二个“测试主动推进 tick + 登录就绪”的循环 — 其中十个无总预算、farming/hunger 两个已有内联预算；`Recv(ctx)`、墙钟 deadline、有限次数及业务阶段等待保留 — 扩到全包所有循环会改变无关断言语义。

## Task 1

- Implementer：`/root/e11_task1_implementer`，冻结契约 `b8120ef4`。
- RED：`GOCACHE=/private/tmp/mornlea-e11-go-cache /Users/chen/.gvm/gos/go1.26.0/bin/go test ./internal/server -run TestIntegrationLoginTickBudget -count=1` 按预期编译失败；`login_wait_budget_test.go` 的三处 `waitIntegrationLoginReady` 与三处 `integrationLoginTickBudget` 引用均报 `undefined`，包以 build failed 退出。
- GREEN：在 `tcp_integration_helpers_test.go` 加入固定 `integrationLoginTickBudget = 3000`、最小 `Helper`/`Fatalf` 接口与只负责活性的 `waitIntegrationLoginReady`；同一命令通过（`ok github.com/channing771/mornlea/internal/server 1.014s`）。R0 主题测试只证明初始满足零推进、预算内恰好 7 次推进，以及永不满足时恰好 3000 次推进、动态诊断只求值一次且 `Fatalf` 只调用一次；它尚未证明第 3000 次推进后成立仍成功，见下方初审与 R1 修复。
- 迁移清单（十二个调用点）：
  1. `farming_loop_e2e_test.go`：Ready、非空背包发布、九区块镜像；删除 `farmingLoginBudget` 及其注释。
  2. `hunger_loop_e2e_test.go`：Ready、非空背包发布、九区块镜像；保留 `wireInventory` 更新，删除 `hungerLoopLoginBudget` 及其注释。
  3. `farming_integration_test.go`：Ready、初始背包等值、九区块镜像。
  4. `till_soil_integration_test.go`：Ready、初始背包等值、九区块镜像。
  5. `material_processing_integration_test.go`：Ready、初始背包等值、九区块镜像；复用原 `step(nil)`。
  6. `eating_parity_test.go`：合法 Ready 状态、初始背包确认、九区块镜像。
  7. `transport_parity_integration_test.go` 采掘登录：合法 Ready 状态、初始背包确认、九区块镜像；保留掉落镜像与无远端玩家断言。
  8. `transport_parity_integration_test.go` 业务 transcript 登录：Ready、九区块镜像；保留 readiness 消息录制。
  9. `fluid_transport_parity_test.go`：Ready、九区块镜像。
  10. `block_light_integration_test.go`：Ready、初始背包确认、独立光照镜像九区块加载；保留双镜像应用。
  11. `placement_success_integration_test.go`：Ready、初始背包确认、九区块镜像。
  12. `player_melee_parity_test.go`：攻击者与目标双方 Ready；后续固定十次远端位置等待保持原样。
- 定点与全量门禁：
  - `make rust`：通过。
  - helper GREEN 命令：通过（1.014s）。
  - 十二条受影响用例的指定 `go test ./internal/server -race -count=1 -run ...`：通过（9.265s）。
  - `go test ./internal/server -race -count=1`：通过（174.919s）。
  - `go test ./internal/archcheck -count=1`：通过（15.134s）。
  - `go vet ./...`：通过，无输出。
  - 首次 `go test ./... -race`：`internal/server` 再次通过（181.676s），唯一失败为既有 `cmd/mornlea` 包共享 10 分钟 timeout，栈停在 `TestScenarioV12GPUCompletionBatchIsRecordedInReport` 的 C 渲染调用，无 data race；该失败用例独立重跑通过（202.226s）。随后原命令重跑通过，`cmd/mornlea` 261.652s，其余包通过或复用同一源状态下首次运行的成功缓存。
  - `gofmt -l .`：通过，无输出。
  - `openspec validate --all --strict --no-interactive`：65 passed，0 failed。
- SPEC review：FAIL（Important）— R0 缺少第 `integrationLoginTickBudget` 次 `step` 后 `ready` 成立仍成功的精确边界用例；删除 helper 最后一次 `ready()` 检查时原测试仍绿。
- QUALITY review：FAIL（Important）— 与 SPEC review 同项；此外耗尽测试只独立搜索字符串 `"3000"`，该数字可能来自调用方 diagnostics，未把预算绑定到 `Fatalf` 前缀。
- R1/5 修复：在 `login_wait_budget_test.go` 保留普通 7 次预算内用例，并新增第 3000 次推进后才成立的边界用例，精确断言 `steps == integrationLoginTickBudget`、`Fatalf` 零次、diagnostics 零次；删除 helper 循环后的末次 `ready()` 检查会让 fake 记录一次 `Fatalf`，该变异不再存活。耗尽用例改为断言完整前缀 `等待 never-ready 登录就绪耗尽 3000 tick:`，并另行断言调用方动态诊断完整片段。
- R1/5 验证：helper 非 race（1.464s）与 race（1.628s）定点测试通过；十二条指定 race 用例通过（9.381s）；完整 `internal/server -race -count=1` 通过（168.481s）；`gofmt -l .` 无输出，`git diff --check` 通过，OpenSpec strict 65 passed / 0 failed。全仓 race 按修复 brief 留给整分支 fresh 终审。
- R1/5 scoped re-review：原 SPEC reviewer 判定 `ADDRESSED`、无新 finding、最终 SPEC PASS；原 QUALITY reviewer 判定 `ADDRESSED`、无新 finding、最终 QUALITY PASS。两者均允许进入整分支终审。
- Task 1 完成：实现提交 `0834fb8b`，R1 修复提交 `44c7506d`；任务级 SPEC + QUALITY 双评审 clean。
- 修复轮次：1/5。

## 整分支终审与收尾

- 整分支终审：待执行。
- 最终门禁：待执行。
- 归档、规划回填与 PR/CI：待执行。
