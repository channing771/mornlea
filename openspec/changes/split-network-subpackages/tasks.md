## Execution And Review Protocol

每个以下未勾选任务都是独立实现任务。控制会话 MUST 为每项任务派发此前未参与
该项的 fresh subagent implementer（任务 brief 为唯一需求来源），并分别取得
规格合规与代码质量双评审；控制会话不得直接实现。每项任务完成或修复后，必须
在 `ledger.md` 记录实现 SHA、验证输出、评审结论与裁决，才可勾选或移交下一项。

每个实现任务共用验收锚点：`go test ./internal/network/... -list '.*'` 并集
与 `baseline-test-list.txt` 完全一致（根包 164 = 151 Test + 7 Benchmark +
6 Fuzz，tcp 33 Test，共 197 项逐名）；消费方生产代码零改动（`git status`
不含 `internal/server`、`internal/client`、`internal/sim`、`cmd/` 生产文件
与 `internal/network/tcp` 生产文件）。

## 1. 基线与 change 建立

- [x] 1.1 在基线 `6fd03011` 记录 `go test ./internal/network/... -list '.*'`
  全量入口快照到本 change 目录 `baseline-test-list.txt`（条目按原始输出逐行
  保留、剔除空行与 ok 行；根包 164 + tcp 33 = 197 逐项核对），并把根包
  race 计时基线写入 `ledger.md`。
- [x] 1.2 建立 `proposal.md`、delta specs（`repository-code-organization`）、
  `design.md`、`tasks.md`、`ledger.md`，通过
  `openspec validate --all --strict --no-interactive`。

## 2. protocol 子包（协议消息层）

- [x] 2.1 新建 `internal/network/protocol`（package protocol），以 `git mv`
  迁移 `packet.go`、`message.go`、`message_chunk.go`、`message_command.go`、
  `message_companion.go`、`message_container.go`、`message_drop.go`、
  `message_hostile.go`、`message_inventory.go`、`message_player.go`、
  `registry.go`、`snapshot.go`；`validateDecodedClientWirePacket` 自
  `codec_client.go` 归位 protocol 并导出为 `ValidateDecodedClientWirePacket`
  （Handshake/Login 放行逻辑原样）；临时导出包 ID 访问器 6 个（
  `clientPacketID`/`clientPacketForID`/`serverPacketID`/`serverPacketForID`
  /`commandRejectReasonID`/`commandRejectReasonForID`，导出名按仓库命名习惯
  定并记 ledger）与 companion/hostile 的 wire 常量及 encode/decode 函数
  （临时导出，供仍留守根包的 codec 文件过渡调用）。
- [x] 2.2 新建根包 `types.go`：package doc（根包保留会话与传输编排 + 别名
  再导出保证）+ 全部迁移符号的别名再导出（类型/常量/错误/var 函数别名，
  逐个别名带中文注释说明归属与身份保证）；留守根包的 codec 文件改
  `protocol.` 限定调用；根包 `package network` 测试引用已迁移 unexported
  符号处就地 `protocol.` 限定或按被测主体提前随迁（测试函数名与 `t.Run`
  标签逐名保留，preflight 裁决记 ledger）。
- [x] 2.3 archcheck 登记 `"internal/network/protocol": {"internal/core",
  "internal/companion"}`，根包边移除 internal/companion、新增 protocol 边。
  验证：`go build ./...`；`go test ./internal/network/... -race -count=1`；
  `go test ./internal/archcheck -count=1`；`-list` 并集与基线逐名一致；
  消费方与 tcp 生产代码零改动；临时导出清单记入 ledger。

## 3. codec 子包（编解码层）

- [x] 3.1 新建 `internal/network/codec`（package codec），以 `git mv` 迁移
  `chunk_codec.go`、`codec.go`、`codec_client.go`、`codec_server.go`、
  `codec_values.go`、`codec_primitives.go`、`frame.go`；companion/hostile
  的 wire encode/decode 函数自 protocol 归位 codec（恢复 unexported，其
  调用点 `codec_server.go`/`codec_client.go` 回到包内直呼）；wire 常量留
  在 protocol 保持导出；包 ID 访问器与 `ValidateDecodedClientWirePacket`
  保持导出（codec 永久消费）；回收裁决逐项记 ledger。
- [x] 3.2 测试随迁（跟随被测主体）：codec 收 `chunk_codec_test.go`(+fuzz)、
  `codec_fuzz_test.go`、`codec_golden_test.go`、`codec_invalid_test.go`、
  `codec_inventory_test.go`、`codec_helpers_test.go`、
  `codec_primitives_test.go`(+fuzz)、`frame_test.go`(+fuzz)、`drop_test.go`
  、`furnace_test.go`、`container_test.go`、`hunger_test.go`、
  `worldtime_test.go`、`place_block_succeeded_test.go`、
  `message_companion_fuzz_test.go` 及全部 6 个 Fuzz；纯 Validate 主体测试
  → protocol；混合文件按主体拆分，测试函数名与 `t.Run` 标签逐名保留；
  根包留 `login_test.go`/`memory_test.go`/`seed_test.go`/`benchmark_test.go`
  /`benchmark_helpers_test.go`；`testdata/chunk-snapshot-v1.bin` 迁至
  `internal/network/codec/testdata`（`git mv`，逐字节不变）。
- [x] 3.3 根 `types.go` 增补 codec 侧别名（`Codec`/`NewCodec`/
  `MaxCompressedSnapshot`/`MaxDecodedSnapshot`/`MaxSmallPayload`/
  `MaxFrameBytes`/`WriteFrame`/`ReadFrame` 等）；archcheck 登记
  `"internal/network/codec": {"internal/network/protocol", "internal/core"}`。
  验证：任务 2 全部验收项 + `go test ./internal/network/... -bench .
  -benchtime 1x`（微基准盲区预演，数值只记录入 ledger）。

## 4. 文档与收尾门禁

- [x] 4.1 重写 `internal/network/AGENTS.md`（根包会话/传输范围 +
  protocol/codec/tcp 子包地图 + 依赖方向 + 既有信任边界/传输一致性/协议
  演进契约保留），新建 `protocol/AGENTS.md`、`codec/AGENTS.md`（按
  `docs/agents-md-style.md`，子包不放 CLAUDE.md；`tcp/AGENTS.md` 不动）。
- [x] 4.2 CI 与文档同步：`.github/workflows/ci.yml` 架构门禁步骤
  `./internal/network` → `./internal/network/...`（M3C 步骤与 Makefile
  `bench-multiplayer` 不动）；`docs/notes/test-quickstart.md` 定点命令行改
  `./internal/network/...`；`docs/architecture.md` 网络边界描述、
  `docs/notes/compatibility.md` 的 `ProtocolVersion` 指涉（别名说明）同步。
  验证：`go test ./internal/archcheck -count=1`（本任务不改 archcheck 表）。
- [x] 4.3 收尾门禁：`-list` 并集终对照（与 `baseline-test-list.txt` 逐名
  diff 为空）；`gofmt -l .` 无输出；`go vet ./...`；`make dev-check`；
  `make test-race`；`go test ./internal/network/... -bench . -benchtime 1x`
  （数值只记录）；`openspec validate --all --strict --no-interactive`；
  命令结果与评审裁决写入 `ledger.md`，全部通过后再勾选任务。
