## Context

动机见 `proposal.md`，行为契约见
`specs/repository-code-organization/spec.md`。当前 `internal/network/stream.go`
同时包含根包的三个共享 stream 接口和 TCP stream 的具体实现；`tcp.go` 包含
TCP listener、dial、socket 配置和错误归类。`transport.go` 还包含根包的
`ErrClosed`、Play endpoint 接口和登录后的 endpoint 包装，因此不能把整个根包
反向依赖一个包含 endpoint 的 TCP 子包。

## Goals / Non-Goals

**Goals:**

- 把 TCP 专属实现放入 `internal/network/tcp`，让其作为根包的下游实现包。
- 让根包继续拥有共享接口、协议类型、编解码、登录和 Memory transport。
- 让所有仓库内调用方在同一提交序列中迁移到新的 TCP import path。
- 让 TCP 私有测试保留私有可见性，并让跨 transport 的协议测试继续覆盖根包。

**Non-Goals:**

- 不抽取 `internal/network/protocol` 或 `internal/network/memory`。
- 不提供 `network.ListenTCP`、`network.DialTCP` 的兼容转发函数。
- 不改变 codec、frame、packet、登录、错误、并发、容量或 wire 行为。
- 不同时重构其他内部包。

## Decisions

### 1. 采用 `network/tcp`，而不是通用 `network/transport`

`network/tcp` 直接表达实现边界，并只依赖父包。根包不导入子包，所以父子包
不会形成循环依赖。通用 `network/transport` 会暗示 Memory 和 TCP 同时迁移；
若保留根包构造函数则会产生循环，若先抽取 protocol 则会扩大为协议模型迁移，
因此不采用。

### 2. 根包保留接口，子包实现接口

`internal/network/stream.go` 只保留 `ClientPacketStream`、
`ServerPacketStream` 和 `Listener` 接口。`ErrClosed` 与 Play endpoint 包装继续
留在 `internal/network/transport.go`。`internal/network/tcp` 的具体类型实现
这些接口，并使用根包的 `Codec`、frame 函数、packet 类型和 `ErrClosed`。

这种安排使 `network.LoginClient`、`network.BeginServerLogin` 和应用装配继续
依赖稳定的抽象；具体 TCP 包只负责 socket 生命周期和 packet 编解码调用，不
承载登录或玩法规则。

### 3. TCP 构造函数只在子包公开

`ListenTCP` 和 `DialTCP` 移到 `internal/network/tcp`，调用方用包别名
`networktcp` 导入。它们的参数和返回接口保持不变：listener 仍满足
`network.Listener`，dial 结果仍满足 `network.ClientPacketStream`。

根包不增加兼容转发函数。根包若调用子包会形成反向依赖，而这是本次要建立的
架构边界；仓库内调用方可以在同一变更中一次性更新。

### 4. 私有 TCP 测试随实现迁移

`internal/network/tcp_test.go` 移至
`internal/network/tcp/tcp_test.go`，包名改为 `tcp`，因此对
`tcpStream`、`tcpListener` 和测试替身的白盒覆盖不丢失。根包的
`transport_consistency_test.go` 和 TCP benchmark 留在根包，因为它们测试登录
协议与两种 transport 的组合；只把 TCP opener 改为调用 `networktcp`。

### 5. 依赖守卫和架构文档同步

`internal/archcheck/dependency_test.go` 增加 `internal/network/tcp` 的白名单
项，只允许依赖 `internal/network`。`docs/architecture.md` 和
`internal/network/AGENTS.md` 同步描述根包与 TCP 子包的职责，避免长期文档继续
声称 TCP 实现全部位于根包。

## Risks / Trade-offs

- [内部 import path 改变] → 用全仓 `git grep` 找出所有 `network.ListenTCP` 与
  `network.DialTCP` 引用，并在同一变更中更新；不保留会制造循环依赖的兼容层。
- [根包与子包测试入口分散] → 保留测试函数名和子测试标签，分别运行根包与
  `network/tcp`，并保存迁移前后的 `go test -list` 快照。
- [TCP 私有类型失去可见性] → 整体迁移 `tcp_test.go` 为同包测试，不改为外部
  测试包。
- [新增依赖边未登记] → 在实现子包的同一任务中更新 archcheck，并运行
  `go test ./internal/archcheck -count=1`。
- [并发或 wire 行为意外变化] → 使用现有 TCP、Memory、login、codec 和跨
  transport 测试，不修改实现逻辑；最终运行 race 和全量测试。

## Migration Plan

1. 在变更开始时保存 `internal/network` 的测试入口快照，并确认工作区已有的
   `internal/sim/door_test.go` 不在本变更文件集中。
2. 创建 `internal/network/tcp`，将 `tcp.go` 和 `stream.go` 中的 TCP 专属代码
   搬入；根 `stream.go` 只保留共享接口。
3. 将 TCP 白盒测试搬入子包，更新根包的跨 transport 测试和 benchmark。
4. 更新 `cmd/mornlea`、`cmd/mornlea-server`、`internal/client`、
   `internal/server` 及其他直接调用 TCP 构造的测试辅助代码。
5. 更新 archcheck、架构文档和 `internal/network/AGENTS.md`，然后执行 scoped
   race、vet、全量 Go 测试和 OpenSpec 校验。

回退方式是回退该结构变更的单一实现提交，恢复原始文件位置和构造函数调用；
无需存档迁移、协议回滚或运行时数据修复。
