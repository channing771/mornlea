# Execution Ledger

## Baseline

- 基线提交：`123c51f1`，从 `main` 创建隔离 worktree。
- 基线环境：首次测试因新 worktree 缺少 `engine/target/release/libmornlea_engine` 链接失败；运行 `make rust` 后，`go test ./internal/network/... -race -count=1` 通过。
- 控制会话工作区中的 `internal/sim/door_test.go` 及其他 B-17 修改不属于本 change，不得进入实现 diff。

## Rulings

- Ruling: 首个结构整理范围限定为 `internal/network/tcp` — `network` 根包与 TCP 实现之间已有清晰下游边界，先做低风险叶子拆分，不同时重构其他包。
- Ruling: 根包保留 stream 接口、endpoint 包装、协议、codec、登录和 Memory — TCP 子包依赖根包而根包不反向依赖，避免循环依赖并保持登录装配稳定。
- Ruling: 不增加 `network.ListenTCP` / `network.DialTCP` 兼容包装 — 仓库内 API 可在同一变更中迁移，兼容包装会破坏单向依赖。
- Ruling: 使用 `repository-code-organization` 增量规格 — 本次是代码组织与架构边界变化，不引入新的用户功能。
- Ruling: 将 archcheck 从 TCP 子包初始迁移任务移到架构守卫任务 — 新包在白名单登记前运行守卫必然失败；先验证编译，再由同一变更登记并验证依赖边。
- Ruling: `transport_consistency_test.go` 整体留在 `internal/network/tcp` — 非法 wire 用例必须访问 TCP 私有 stream，拆分会重复大量测试装配；正常 transcript 仍使用真实 `network.NewMemoryStreamPair`，raw Memory 只限非法 wire 注入，并用独立测试覆盖真实 Memory 的发送侧校验；若该边界判断错误，代价是后续复审需要再拆分测试文件。

## Review Log

- Task 1：`ffd3e5fa` 完成 TCP 子包提取，`a27ef8e7` 让 raw transport close 确定化，`5b16ca49` 在 fix round 补真实 `network.NewMemoryStreamPair` 的旧握手发送侧拒绝覆盖；`go test ./internal/network/... -race -count=1`、真实 Memory/跨 transport 定点测试和 `go vet ./internal/network/...` 通过，最终 SPEC PASS / QUALITY APPROVED。
- Task 2：`5e25830f` 完成仓库调用方迁移；排除 `internal/archcheck/source_guards_test.go` 后旧构造调用搜索无输出，命令包编译与受影响包 race 测试通过。剩余命中只是 source guard 字面量，按范围留给 Task 3 更新；最终 SPEC PASS / QUALITY APPROVED。

## Task 3: Architecture Guard and Documentation

- 实现范围：在 `internal/archcheck` 登记唯一的 `internal/network/tcp` → `internal/network` 依赖边，将 `cmd/mornlea` benchmark TCP 构造守卫更新为 `networktcp.ListenTCP(`，并同步根包、TCP 子包的当前职责文档。
- 前置实现与评审：Task 1 和 Task 2 已通过评审，基线实现提交为 `5e25830f`；本任务未修改协议、运行时行为、packet bytes、登录语义、ABI 或存储文档。
- 失败验证：`go test ./internal/archcheck -count=1` 在修改前失败，原因是新包未登记且 source guard 仍要求 `network.ListenTCP(`；修改后以同一命令重跑通过。
- 验证通过：`gofmt -w internal/archcheck/dependency_test.go internal/archcheck/source_guards_test.go`；`go test ./internal/archcheck -count=1`；`git diff --check`。
- 调用方检查：`git grep -nE 'network\.(ListenTCP|DialTCP)' -- '*.go' ':!internal/archcheck/source_guards_test.go'` 无输出；source guard 包含 `networktcp.ListenTCP(` 且不再包含 `network.ListenTCP(`。
- 评审结论：改动限定于本任务的 archcheck、架构文档、网络包指南和 ledger；保留既有共享登录状态机要求及 no-WebGPU、ABI 边界。
- 实现提交：`553b6b72`（`docs(network): record TCP package boundary`）；未发现需要升级的风险或未解决问题。

## Task 4: Final Verification and Handoff

- 入口对比：重新运行根包与子包的 `go test -list '.*'` 均成功；`/tmp/network-tcp-before.txt` 中 185 个 Test、Benchmark、Fuzz 入口全部保留在当前根包/子包并集中，唯一非入口基线行是旧包结果。新增入口仅为 `TestMemoryTransportRejectsOutdatedHandshakeBeforeDelivery` 与 `TestTCPConstructorsExposeNetworkInterfaces`；原有 `t.Run` 标签未重命名。全仓搜索 Go 文件中的 `network.ListenTCP` 与 `network.DialTCP` 无匹配。
- diff 边界：基线仍为 `123c51f1`，计划内网络包、调用方、archcheck、文档与 OpenSpec 文件构成 change diff；`internal/sim/door_test.go` 和 `.superpowers` 报告不在 change diff。未修改协议、存档、ABI、生产行为或无关测试。
- 并发负载分诊：原始 `go test ./... -race -count=1` 只在跨包聚合负载下出现以下失败：`cmd/mornlea` 包级 GPU 测试达到 10 分钟超时；`TestApplicationAdvancesCompanionsExactlyOnceInFrameAndInteractiveLoops/interactive` 命中 `interactive companion x = %f, want one elapsed advance`；`internal/server/TestCompanionInteractionMemoryTCPParity/memory` 命中 `非法计划未以 TaskFailed 终结`，随后顶层断言 `Memory/TCP 多人指挥 transcript 不一致`；`TestCraftingSurvivesV2DiskRestartAndReconnectOrder` 命中 `player %s snapshot unavailable`。每个失败测试或包单独运行均通过，符合 `docs/notes/test-quickstart.md` 的负载 flake 分诊规则；未为这些失败修改生产代码或无关测试。
- Ruling: 本 change 的本地完整 race 门禁采用 `go test ./... -race -p=1 -count=1`。该命令执行相同的完整 package/test 并集，并与 CI race 分片的 `-p=1` 包调度一致；命令退出 0，所有包通过。关键耗时：`cmd/mornlea` 318.738s、`internal/server` 216.840s、`internal/network` 2.286s、`internal/network/tcp` 2.335s。
- 其余收尾门禁：`make rust` 退出 0（release build 0.41s）；`gofmt -l .` 退出 0 且无输出；`go vet ./...` 退出 0 且无输出；`git diff --check` 退出 0 且无输出；`make rust-check` 退出 0，Rust fmt/clippy、159 个 client 测试、166 个 engine 测试及 doc tests 全部通过；`openspec validate --all --strict --no-interactive` 为 68 passed、0 failed。
- 子包指南复审：未跟踪的 `internal/network/tcp/AGENTS.md` 在一轮标识符格式修正后获得 scoped SPEC PASS / QUALITY APPROVED；`git diff --no-index --check -- /dev/null internal/network/tcp/AGENTS.md` 无 whitespace 错误。该指南不作为本计划的新任务，本轮未编辑它。
- Whole-branch 终审初轮：规格评审与质量评审均为 FAIL；接受的问题包括历史 design、plan 和 ledger 的陈旧或重复描述，`openTCPStreamPair` 的多余 dial goroutine，根网络指南遗漏子包测试，以及 benchmark 重复计算 logical size。
- 终审修复轮：更正历史 design 的一致性测试位置，清理 plan 的重复 move 与已删除 gofmt 路径，补 Task 1/2 评审记录，复用 `dialAndAccept`，将根网络命令改为 `./internal/network/...`，并从已编码 snapshot envelope 读取 logical size、删除重复 helper；未修改生产代码或生产行为。
- 定点验证通过：`gofmt -w internal/network/benchmark_test.go internal/network/tcp/transport_consistency_test.go`；`go test ./internal/network/... -race -count=1`；`go vet ./internal/network/...`；`go test ./internal/archcheck -count=1`；`git diff --check`；`openspec validate --all --strict --no-interactive`（68 passed、0 failed）。
- Whole-branch 最终复审：SPEC PASS / VERDICT PASS，无剩余重要需求问题，先前 design、plan 与 ledger 问题均已关闭；QUALITY APPROVED / VERDICT PASS，无剩余 blocker、high 或 medium 问题，先前五项质量问题均已关闭。
- 最终复审未重跑耗时的全仓门禁，既有 gate 证据与 accepted race ruling 保持不变。
- 最终树补充门禁：review fix round 后 controller 重新运行 accepted serialized full race gate `go test ./... -race -p=1 -count=1`，退出 0 且全部包通过；关键耗时为 `cmd/mornlea` 312.091s、`internal/archcheck` 40.337s、`internal/network` 2.269s、`internal/network/tcp` 2.024s、`internal/server` 212.077s、`internal/sim` 46.809s、`internal/storage` 23.453s。
- 当前状态：handoff 已完成，OpenSpec tasks 5.1/5.2 保持完成；HEAD 仍为 `6bd33df52b2adec984805427f18fb650961d1681`，final artifacts 与 fix-round changes 均未 stage、未 commit。
