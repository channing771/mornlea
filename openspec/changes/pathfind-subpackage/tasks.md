## Execution And Review Protocol

每个以下未勾选任务都是独立实现任务。控制会话 MUST 为每项任务派发此前未参与该项的 fresh subagent implementer，并分别取得独立于 implementer 且彼此独立的规格评审与质量评审；控制会话不得直接实现。每项任务完成或修复后，必须在 `ledger.md` 记录任务编号、implementer、两项评审结论、发现、修复轮次和最终裁决，才可勾选或移交下一项。

## 1. Extract And Migrate Pathfind

- [x] 1.1 在 `internal/pathfind/contract_test.go` 先建立外部包 API 契约，并以 `go test ./internal/pathfind -run '^TestPublicPathfindFindsTrivialPath$' -count=1` 证实新包出现前的失败。
- [x] 1.2 将 `internal/companion/pathfind.go`、`pathfind_policy.go` 和 `pathfind_test.go` 迁至 `internal/pathfind/`，新增中文 `internal/pathfind/AGENTS.md` 并以 `gofmt -w internal/pathfind`、`go test ./internal/pathfind -race -count=1` 验证纯计算、不可变快照和 core-only 边界。
- [x] 1.3 在 `internal/companion/plan_types.go`、`planner_test.go`、五个 `internal/server` 调用文件和 `companion_manager_test.go` 一次性改用 `pathfind`，删除三个旧 `companion/pathfind*` 文件且不保留别名或转发；以计划中的 `git grep` 和 `go test ./internal/companion ./internal/server ./internal/pathfind -race -count=1` 验证。
- [x] 1.4 在 `internal/archcheck/dependency_test.go` 登记 `pathfind -> core` 及 `companion`、`server` 消费边；以 `go test ./internal/archcheck -count=1`、`go vet ./internal/pathfind ./internal/companion ./internal/server` 验证。

## 2. Document And Preserve The Boundary

- [x] 2.1 在 `docs/architecture.md` 记录 `internal/pathfind` 的不可变快照有界寻路所有权、core-only 依赖和 `companion`/`server` 消费边界；以 `git diff --check` 验证。
- [x] 2.2 以 `go test ./internal/companion -list '.*' > /tmp/pathfind-companion-after.txt`、`go test ./internal/pathfind -list '.*' > /tmp/pathfind-pathfind-after.txt` 和 `go test ./internal/server -list '.*' > /tmp/pathfind-server-after.txt` 生成 after inventory，并从 before/after 输出提取 `^(Test|Benchmark|Fuzz)` 名称；以 `cmp -s` 确认 server 入口不变、非寻路 companion 入口不变，且迁出的 `TestPathfind`、`BenchmarkPathfind`、`FuzzPathfind` 入口在 pathfind 中逐项保持不变。还 MUST 在迁移前以 `rg -n -F 't.Run(' internal/companion/pathfind_test.go > /tmp/pathfind-companion-t-run-before.txt || test $? -eq 1` 捕获精确调用行，在迁移后以 `rg -n -F 't.Run(' internal/pathfind/pathfind_test.go > /tmp/pathfind-pathfind-t-run-after.txt || test $? -eq 1` 捕获精确调用行，并运行 `cmp -s /tmp/pathfind-companion-t-run-before.txt /tmp/pathfind-pathfind-t-run-after.txt`；将两份清单、两份精确 `t.Run` 输出和比较结果记录到 `ledger.md`。
- [x] 2.3 向 A-04 提供最终 `pathfind` import 映射；A-04 合并前 MUST rebase 到本 change 的最终 import path，并运行 `go test ./internal/pathfind ./internal/server -race -count=1` 与 `go test ./internal/archcheck -count=1`，不得向本 change 加入别名或玩法改动。

## 3. Final Gates And Review

- [x] 3.1 运行 `make rust`、`gofmt -l .`、`go vet ./...`、`go test ./... -race -p=1 -count=1` 和 `openspec validate --all --strict --no-interactive`；所有命令 MUST 成功且 `gofmt -l .` 无输出。
- [x] 3.2 完成独立规格与质量评审，处理发现后更新本文件和 `ledger.md`，仅在所有 gate、保留清单与单向依赖边界均通过时勾选任务。
