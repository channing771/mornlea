# split-network-subpackages

## Why

`internal/network` 在 PR #109 拆出 `tcp/`（6 文件）后，根包仍平铺 53 个 Go
文件（实测：23 个生产文件约 5.3k 行 + 30 个测试文件约 7.5k 行），协议消息层、
编解码层与传输会话三类异质关注点共处一个命名空间，包内边界只靠文件名前缀
约定：

1. 协议消息层（约 2.6k 行）：`packet.go`（`ProtocolVersion` v31、`State`、
   密封 `ClientPacket`/`ServerPacket`、握手/登录包、`ValidateClientPacket`/
   `ValidateServerPacket`）+ `message.go` 与 8 个 `message_*.go`（域 DTO 与
   `Validate`）+ `registry.go`（冻结包 ID 表）+ `snapshot.go`（区块 wire DTO）。
2. 编解码层（约 1.7k 行）：`chunk_codec.go`（`Codec` 门面 + zstd +
   `ChunkSnapshot` 专路）、`codec.go`、`codec_client.go`/`codec_server.go`
   （双向分发）、`codec_values.go`、`codec_primitives.go`、`frame.go`
   （帧封装）。
3. 传输与会话（约 0.9k 行）：`stream.go`（共享 stream 接口）、`transport.go`
   （`ErrClosed`、`ClientEndpoint`/`ServerEndpoint` 与适配器）、`memory.go`
   （Memory transport）、`login.go`（共享登录状态机）。

后果：

1. 测试无法定点：164 个测试入口（151 Test + 7 Benchmark + 6 Fuzz）同包编译，
   tcp 另有 33 个 Test。`go test -run` 任一域的测试都要编译执行整个根包测试
   二进制（本 worktree 实测整包 `-race -count=1` 2.797s），迭代协议消息域或
   编解码域无法避开其他域的测试。
2. 职责边界不可检查：`internal/archcheck` 只能约束整包
   `internal/network` 的依赖边（当前仅 `{"internal/companion",
   "internal/core"}`），包内跨域耦合不可见——`codec_server.go` 调度住在
   message 文件里的 companion/hostile wire 函数、`memory.go` 调用住在
   `codec_client.go` 的协议级校验 `validateDecodedClientWirePacket`，这些
   跨层泄漏在整包边视角下全部不可见。

拆出子包后，`go test ./internal/network/protocol` 等定点命令不再编译执行
其他域的测试，与 T0–T3 分层及 `race-changed` 改动闭包天然配合；跨域泄漏以
显式导出面进入 archcheck 可检查范围。

## What Changes

- **BREAKING（仓库内部源码结构）** 将 `internal/network` 拆为编排根包加两个
  子包（`tcp/` 完全不动）：
  - 根包保留会话与传输编排：`stream.go`/`transport.go`/`memory.go`/
    `login.go`（`Identity`、`RemoteError`、`PendingLogin`、
    `BeginServerLogin`/`Accept`/`Reject`、`LoginClient`/`LoginClientWithSeed`、
    `NewMemoryPair`/`NewMemoryStreamPair`、`ClientEndpoint`/`ServerEndpoint`、
    三个共享 stream 接口、`ErrClosed`），新建 `types.go` 以别名再导出迁出
    符号；测试留 `login_test.go`/`memory_test.go`/`seed_test.go`/
    `benchmark_test.go`（跨域基准留守，CI M3C 步骤与 Makefile
    `bench-multiplayer` 零改动）；
  - `internal/network/protocol`：`packet.go`、`message.go` 与 8 个
    `message_*.go`、`registry.go`、`snapshot.go` 及协议级测试。密封接口
    `ClientPacket`/`ServerPacket` 以 unexported marker 密封且全部 message
    DTO 与 snapshot 实现之、`registry.go` 与 `message_player.go` 的
    `CommandRejected.Validate` 双向引用——四者必须同包；
  - `internal/network/codec`：`chunk_codec.go`、`codec.go`、
    `codec_client.go`/`codec_server.go`、`codec_values.go`、
    `codec_primitives.go`、`frame.go` 及编解码测试（含基准夹具
    `benchmark_helpers_test.go`）与 `testdata/chunk-snapshot-v1.bin`。帧封装
    复用编码原语的 canonical uvarint，随 codec 包且原语保持 unexported。
- 依赖方向单向并由 `internal/archcheck` 按任务增量登记：network（根）→
  {protocol, codec, internal/core}（`login.go` 的 `NormalizeDisplayName`
  保留既有 core 边）；protocol → {internal/core, internal/companion}（
  companion 边随 `message_companion.go` 自根包移交）；codec → {protocol,
  internal/core}；tcp → network（既有边不动）。子包不得依赖根包，protocol
  与 codec 之间仅允许 codec → protocol 单向边。
- 消费面零改动：`internal/server`、`internal/client`、`internal/sim`、
  `cmd/mornlea`、`cmd/mornlea/app`、`cmd/mornlea/benchmark`、
  `cmd/mornlea/capture`、`cmd/mornlea-server` 8 个消费包与 `tcp/` 生产代码
  不得因拆分而修改；既有 `network.X` 符号引用（消费面计数为执行计划在同
  基线的先行实测：internal/server 127 符号 2033 处、cmd/mornlea/app 616、
  internal/client 411、capture 120、benchmark 39、cmd/mornlea 36、
  mornlea-server 34、sim 测试 5、tcp 400+ 处）
  继续以同一名称与类型/错误/常量身份解析。消费面最高频符号（
  `ClientEndpoint` 147、`Identity` 103、`NewMemoryPair` 96）全部留守根包
  原生，不经别名。
- 临时导出与回收：Task 2 建 protocol 时临时导出包 ID 访问器 6 个、
  `ValidateDecodedClientWirePacket`（自 `codec_client.go` 归位 protocol）
  与 companion/hostile 的 wire 常量及 encode/decode 函数（供仍留守根包的
  codec 文件过渡调用）；Task 3 codec 落位后 encode/decode 函数移入 codec
  恢复 unexported，包 ID 访问器、`ValidateDecodedClientWirePacket` 与 wire
  常量保持导出转正（codec 永久消费，属协议冻结契约面）。全部记入 ledger。
- 测试入口并集不变：拆分前 `go test ./internal/network/... -list '.*'` 的
  根包 164（151 Test + 7 Benchmark + 6 Fuzz）+ tcp 33 Test = 197 项逐名
  冻结进基线，拆分后各包 `-list` 并集与基线完全一致；测试函数名与 `t.Run`
  标签逐一不变；混合测试文件（如 `message_hostile_test.go` 同时承载
  Validate、Codec 往返与 `FuzzHostileMessageCodec`）按「跟随被测主体」拆分。
- fixture：`testdata/chunk-snapshot-v1.bin` 随 `chunk_codec_test` 迁至
  `internal/network/codec/testdata`（无外部引用），内容逐字节不变。
- 文档同步：`internal/network/AGENTS.md` 重写为子包地图与依赖方向总纲，新建
  `protocol/AGENTS.md` 与 `codec/AGENTS.md`（子包不放 CLAUDE.md）；
  `.github/workflows/ci.yml` 架构门禁步骤改用 `./internal/network/...`；
  `docs/notes/test-quickstart.md` 定点命令、`docs/architecture.md` 与
  `docs/notes/compatibility.md` 边界描述同步。
- 非目标：不改任何 wire 字节、包 ID、协议语义、校验规则、错误语义、
  Memory/TCP 行为与协议版本（`ProtocolVersion` v31）；不合并域、不在子包外
  新建兼容转发层、不迁移任何消费方调用点；Memory 与 TCP 继续受同一
  packet/codec 契约约束（既有「Transport 包整理保持传输行为」要求原样
  保留，本变更不放松）。

## Capabilities

### New Capabilities

无。该变更只建立网络层的代码组织边界，不引入新的用户可观察能力。

### Modified Capabilities

- `repository-code-organization`：为网络层建立根包 + protocol/codec 子包
  布局（protocol 收协议消息层，codec 收编解码与帧封装），要求依赖方向单向
  并按任务增量登记、别名再导出保持消费面与协议语义、测试入口集合保持不变。

## Impact

- 受影响生产包：`internal/network`（缩为会话与传输编排根包）与新增
  `internal/network/protocol`、`internal/network/codec`。
- 消费方生产代码零改动：上列 8 个消费包与 `internal/network/tcp` 对
  `network.X` 的既有引用保持不变（根包 `types.go` 别名再导出承接）。
- 受影响架构守卫：`internal/archcheck/dependency_test.go` 的 allowed 表按
  任务增量登记（Task 2 登记 protocol 边并移除根包 internal/companion 边；
  Task 3 登记 codec 边；Task 4 仅文档与 CI 门禁）。
- 受影响资产：`internal/network/testdata/chunk-snapshot-v1.bin` 随
  `chunk_codec_test` 迁至 codec 包（`git mv` 保留历史，逐字节不变）。
- 受影响文档：`internal/network/AGENTS.md` 及两份子包 AGENTS.md、
  `.github/workflows/ci.yml`、`docs/notes/test-quickstart.md`、
  `docs/architecture.md`、`docs/notes/compatibility.md`。
- 这是仓库内部源码 import path 的变更；无线上 wire、存档、协议或 ABI 兼容
  性影响。race 与微基准计时只记录，不改变任何退出状态。
