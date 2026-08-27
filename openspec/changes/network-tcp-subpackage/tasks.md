## 1. 基线与 TCP 子包

- [x] 1.1 在 `internal/network` 保存迁移前的 `go test -list` 入口快照，并用 `git grep` 列出所有 `network.ListenTCP` 与 `network.DialTCP` 引用；将基线 SHA、快照位置和既有工作区排除项记录到 `ledger.md`。
- [x] 1.2 创建 `internal/network/tcp`，将 `internal/network/tcp.go` 和 `internal/network/stream.go` 中的 TCP 专属实现迁入子包；根 `stream.go` 只保留 `ClientPacketStream`、`ServerPacketStream`、`Listener` 接口，确保 `network/tcp` 只依赖 `network`。验证：`go test ./internal/network/... -run '^$'`；依赖白名单在 4.1 更新后再运行 archcheck。

## 2. TCP 测试与根包组合测试

- [x] 2.1 将 `internal/network/tcp_test.go` 原样迁移到 `internal/network/tcp/tcp_test.go`，改为 `package tcp`，保留所有 Test、Benchmark、Fuzz 函数名和 `t.Run` 标签；验证：`go test ./internal/network/tcp -race -count=1`。
- [x] 2.2 将 `transport_consistency_test.go` 整体迁入 `internal/network/tcp` 并改为 `package tcp`，让跨 transport 测试和 TCP benchmark 通过 `network/tcp` 打开 TCP stream；正常 transcript 必须使用真实 Memory，非法 wire 只允许测试专用 raw injector，并补真实 Memory 发送侧校验测试。验证：`go test ./internal/network/... -race -count=1`。

## 3. 仓库内调用方迁移

- [x] 3.1 更新 `cmd/mornlea`、`cmd/mornlea-server`、`internal/client`、`internal/server` 及其他测试辅助代码中的 TCP 构造调用，使用 `networktcp.ListenTCP` 与 `networktcp.DialTCP`，不改变函数签名、登录路径或 Memory 构造调用；验证：排除架构守卫源码字面量后的 `git grep -nE 'network\.(ListenTCP|DialTCP)' -- '*.go' ':!internal/archcheck/source_guards_test.go'` 无输出，且 `go test ./cmd/mornlea ./cmd/mornlea-server ./internal/client ./internal/server -race -count=1` 通过。

## 4. 架构文档与守卫

- [x] 4.1 在 `internal/archcheck/dependency_test.go` 登记 `internal/network/tcp -> internal/network` 且不登记反向边；验证：`go test ./internal/archcheck -count=1`。
- [x] 4.2 更新 `docs/architecture.md` 与 `internal/network/AGENTS.md`，准确描述根包和 TCP 子包的职责；验证：`git diff --check`，并确认文档未修改协议、存档或 ABI 基线。
- [x] 4.3 更新 `internal/archcheck/source_guards_test.go` 中对 `cmd/mornlea` TCP benchmark 路径的构造函数字面量要求为 `networktcp.ListenTCP(`，保持其余登录路径守卫不变；验证：`go test ./internal/archcheck -count=1`。

## 5. 收尾验证

- [x] 5.1 对比迁移前后的根包与 TCP 子包测试入口集合，确认现有 Test、Benchmark、Fuzz 名称及 `t.Run` 标签未重命名；确认 `internal/sim/door_test.go` 等外部工作区改动未进入 diff。
- [x] 5.2 执行收尾门禁：`gofmt -l .`、`go vet ./...`、`go test ./... -race -p=1 -count=1`、`make rust-check`、`openspec validate --all --strict --no-interactive`；将命令结果、评审结论和任何 Ruling 写入 `ledger.md`，全部通过后再勾选任务。
