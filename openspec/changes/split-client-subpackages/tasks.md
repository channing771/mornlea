## 1. 基线与 change 建立

- [ ] 1.1 在基线 SHA 记录 `go test ./cmd/mornlea -list '.*'` 全量入口快照（Test/Benchmark/Fuzz 计数与清单）到 `ledger.md` 引用的持久化文件；用 `grep` 生成 `captureGoldenDir`、`testdata/golden` 的全仓引用清单。
- [ ] 1.2 建立 proposal、delta specs、design、tasks、ledger 并通过 `openspec validate --all --strict --no-interactive`。

## 2. app 包提取

- [x] 2.1 创建 `cmd/mornlea/app`（`package app`），按 design 文件簇映射迁入 app 域生产与测试文件；`application` 导出为 `Application`，导出 main 包装配所需的最小构造器与方法；`main.go`/`options.go` 留在 main 包并改为导入 `app`；`app_dependencies.go` 的全部消费方位于 app 域，随 `Application` 一并迁入（见 design「文件簇映射」实施修订）。注释中的标识符引用同步改名。测试文件归属按 design 判定规则用符号引用核实。验证：`go build ./...`；`go test ./cmd/mornlea ./cmd/mornlea/app -list '.*'` 并集与基线快照一致；`gofmt -l cmd/mornlea` 无输出。
- [x] 2.2 适配 `internal/archcheck` 中对 `cmd/mornlea` 生产源码的字符串守卫扫描范围（登录路径守卫、benchmark TCP 路径守卫覆盖子树，保持断言模式不变）。验证：`go test ./internal/archcheck -count=1`；`go test ./cmd/mornlea/app -race -count=1`。

## 3. capture 包提取

- [x] 3.1 创建 `cmd/mornlea/capture`（`package capture`），迁入 capture 生产文件、`visual_compare.go`、对应测试与 `testdata/golden`（git mv）；`captureGoldenDir` 常量同步；capture 经包内消费端接口访问 app 状态；建立 capture helper 中心（每包一个 `*_helpers_test.go`）。验证：`go test ./cmd/mornlea/capture -race -count=1`；`go test ./cmd/mornlea/... -list '.*'` 并集仍与基线一致。
- [x] 3.2 同步 golden 路径引用（`.github/workflows/ci.yml`、`scripts/`、`docs/notes/perf-baseline.*` 如有）；运行 `make visual-check` 确认全场景通过且无 `*-actual.png`/`*-diff.png` 产生。

## 4. benchmark 包提取

- [x] 4.1 创建 `cmd/mornlea/benchmark`（`package benchmark`），迁入 benchmark 生产文件、对应测试、`benchmark_helpers_test.go` 与 `raceEnabled` 常量对；benchmark 经包内消费端接口访问 app 状态；共享常量下沉 app；更新 `Makefile` `test-multiplayer` 包路径。验证：`go test ./cmd/mornlea/benchmark -race -count=1`；`make test-multiplayer`。

## 5. 依赖方向守卫与文档

- [x] 5.1 在 `internal/archcheck` 新增断言：接受 main → app/capture/benchmark、capture → app、benchmark → app；拒绝 app → capture、app → benchmark、capture ↔ benchmark。验证：`go test ./internal/archcheck -count=1`（含正向与故意注入反向边的失败路径核对）。
- [x] 5.2 重写 `cmd/mornlea/AGENTS.md`（子包注释树、依赖方向、入口模式差异、文档同步政策、定点验证命令），新增 `app/`、`capture/`、`benchmark/` 三份 AGENTS.md 与三份薄 CLAUDE.md；更新根 `AGENTS.md` 局部指南清单与 `docs/notes/test-quickstart.md` 定点命令表、`docs/test-organization.md` 范例。验证：`go test ./internal/archcheck -count=1`（`TestClaudeImportsAgentGuidance` 等）。

## 6. 收尾验证

- [ ] 6.1 对比迁移前后 `./cmd/mornlea/...` 全部包的测试入口并集与基线快照（Test/Benchmark/Fuzz 名称及 `t.Run` 标签逐一不变）；确认与 change 无关的工作区改动未进入 diff。
- [ ] 6.2 执行收尾门禁：`gofmt -l .`、`go vet ./...`、`make dev-check`、`go test ./... -race`、`make rust-check`、`openspec validate --all --strict --no-interactive`；将命令结果、评审结论与 Ruling 写入 `ledger.md`，全部通过后再勾选任务。
