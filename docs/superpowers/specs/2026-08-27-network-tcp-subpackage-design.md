# `network` TCP 子包拆分设计

## 背景

`internal/network` 当前同时承载协议模型、编解码、登录状态机、Memory
transport 和 TCP transport。当前生产文件数量为 23 个，其中 TCP 实现主要
集中在 `tcp.go` 与 `stream.go`，可以形成一个不依赖业务编排的清晰边界。

Go 的目录就是 package。把 TCP 文件直接放入子目录会改变 import path，因此
本次设计将显式建立 `internal/network/tcp` 子包，而不是做无效的文件归档。

## 目标

- 将 TCP listener、dial、stream、deadline 和 socket 生命周期移入
  `internal/network/tcp`。
- 保留 `internal/network` 的协议、编解码、登录和 Memory 所有权。
- 保留 packet stream、endpoint 和 listener 的公共接口类型，避免跨层复制类型。
- 保持 wire 布局、登录流程、Memory/TCP 语义、错误边界和并发约束不变。
- 让依赖方向保持单向：`network/tcp -> network`，根包不依赖 TCP 子包。

## 非目标

- 不在本次变更中抽取 `network/protocol`。
- 不同时拆出 Memory 子包；为了对称而引入循环依赖不值得。
- 不修改 packet、codec、schema、协议版本、登录超时或 transport 行为。
- 不整理 `server`、`sim`、`storage` 等其他包；它们另行评估和拆分。

## 目标结构

```text
internal/network/
  packet.go、message_*.go、snapshot.go
  codec*.go、frame.go、registry.go
  login.go
  transport.go       # ErrClosed、endpoint 接口与 Play endpoint
  stream.go          # ClientPacketStream、ServerPacketStream、Listener
  memory.go          # Memory 实现

internal/network/tcp/
  tcp.go             # ListenTCP、DialTCP、TCP listener
  stream.go          # TCP stream、deadline、并发读写、关闭
```

`internal/network/tcp` 导入根包以使用 packet 类型、`Codec`、frame 函数、
stream 接口和 `ErrClosed`。根包不导入 `internal/network/tcp`，因此不会形成
循环依赖。TCP 子包只实现传输，不决定登录或玩法结果。

调用方使用：

```go
import networktcp "github.com/channing771/mornlea/internal/network/tcp"

listener, err := networktcp.ListenTCP(address)
stream, err := networktcp.DialTCP(ctx, address)
```

返回值仍为根包定义的 `network.Listener` 和
`network.ClientPacketStream`，登录调用仍使用
`network.BeginServerLogin`、`network.LoginClient` 或
`network.LoginClientWithSeed`。

## 数据所有权与并发

- TCP 子包拥有 `net.TCPListener`、`net.TCPConn`、codec 实例和读写 owner gate
  的生命周期。
- 根包拥有 packet 类型、状态校验和登录阶段；TCP 子包不绕过这些入口。
- 一个 TCP stream 继续保持单一读 owner、单一写 owner、幂等关闭和 context
  deadline 行为。
- 发送成功后的 packet 和 payload 继续按不可变值处理；不因子包边界改变切片
  所有权规则。
- Memory transport 继续使用根包的有界 channel 和共享关闭状态。

## 调用方迁移

替换所有 TCP 构造函数引用：

- `cmd/mornlea` 的 TCP listener 与 dial 依赖。
- `cmd/mornlea-server` 的 listener 默认实现。
- `internal/client`、`internal/server` 和网络 benchmark 中直接创建 TCP
  listener 或 stream 的测试与辅助代码。
- 根包的跨 transport 一致性测试迁入 `internal/network/tcp`，并在那里通过
  `networktcp` 打开 TCP stream。

`network.NewMemoryPair` 和 `network.NewMemoryStreamPair` 不迁移，因此 Memory
调用方不需要改动。由于这些是仓库内部 API，不增加根包兼容包装；兼容包装会
要求根包反向导入子包，破坏本设计的依赖方向。

## 测试组织

- `internal/network/tcp_test.go` 整体迁移到
  `internal/network/tcp/tcp_test.go`，包名改为 `tcp`，以继续覆盖 TCP 私有
  实现。
- 测试函数名、子测试标签和断言保持不变。
- `internal/network/transport_consistency_test.go` 整体迁入
  `internal/network/tcp` 并改为 `package tcp`，因为其中的非法 wire transcript
  需要访问 TCP 私有 stream；其中正常 transcript 仍通过公开接口使用真实的
  `network.NewMemoryStreamPair` 与 TCP opener，不得用 raw Memory 替代。非法 wire
  用例仅在测试中使用受限 raw injector，并另以真实 Memory 测试锁住其发送侧校验。
- Memory 测试留在根包；codec、login 和 packet 测试不迁移。
- 新包加入 `internal/archcheck` 依赖白名单，仅允许其依赖
  `internal/network`。

## 兼容性、风险与回退

本次不改变线上协议、存档、ABI 或版本基线。唯一源码兼容性变化是 TCP 构造
函数从 `network.*` 移至 `network/tcp`；所有仓库内调用方在同一变更中更新。

主要风险是遗漏调用方、测试私有符号失去可见性或误加反向依赖。通过全仓搜索
构造函数、将 TCP 私有测试随实现迁移、运行 archcheck 和全量 Go 测试控制风险。

若验证失败，回退本次单一提交即可恢复原目录与调用路径；不需要数据迁移或协议
回滚。

## 被否决的方案

### 直接建立 `internal/network/transport`

如果 transport 子包导入根包，而根包继续提供 TCP 构造函数，就会形成循环依赖。
先把所有 packet 类型迁入 `network/protocol` 可以解决这个问题，但会扩大为
协议模型、codec、登录和全部调用方的迁移，超过本次最小闭环。

### 只重命名根包文件

这能改善文件名，却不会减少平铺目录，也不能形成可检查的依赖边界，不满足本次
结构整理目标。

## 验证

实现阶段至少执行：

```bash
make rust
gofmt -w internal/network internal/network/tcp cmd/mornlea cmd/mornlea-server
go vet ./internal/network/... ./cmd/mornlea ./cmd/mornlea-server
go test ./internal/network/... -race -count=1
go test ./internal/archcheck -count=1
go test ./... -count=1
openspec validate --all --strict --no-interactive
```

实现只应包含本设计涉及的生产文件、调用方和 TCP 测试迁移；不得顺手修改
`internal/sim/door_test.go` 或其他工作区已有改动。
