# Task 8 implementation report

## 结果

Task 8 的 `companions.ai` schema v5 codec、历史迁移、metadata-only probe、
persistence 元数据携带与 identity-first bootstrap 已完成。实现保持 32-byte
MCAI envelope，encoder 只写 v5，v1..v4 仅只读；未实现 Task 9/10 的 Agent
lease、MCP、HTTP cutover 或 memory mutation。

## 实现摘要

- v5 payload 写入 UUIDv4 namespace，record 固定使用 221-byte body、
  active/task/FIFO flags、memory epoch、active mirror 或 inactive tombstone；
  `MaxFileLength` 精确为 393,904 bytes。
- `MergeV5` 对 missing、v1..v4 与 v5 做纯函数合并，按 namespace-first、
  CompanionID 升序消费可注入 entropy，checked epoch/revision overflow、容量与
  entropy 失败均在保存前原子返回。
- committed v5 golden 覆盖 active nonzero mirror + task/FIFO、active
  canonical-zero mirror 与 inactive tombstone；fuzz 加入 v5 fixture seed。
- `WorldStore.CompanionsExist` 成为 mandatory 接口；Disk 仅 `stat` 固定路径，
  Memory 仅检查 encoded presence，均覆盖 missing/saved/closed/canceled。
- persistence coordinator 深拷贝并携带 namespace、lifecycle、mirror、
  tombstone；inactive 不保存 queue，旧 direct Dialogue 裸 Summary 不修改 v5
  mirror，aggregate revision 溢出不 dispatch。
- `NewHost` 在 hostile persistence、world 与 companion worker 构造前完成静态
  配置校验、load/merge 和同步 v5 save。空配置 missing 只 probe；existing
  retirement 只保存一次；新伙伴 provisional body 使用世界出生 metadata。
- bootstrap 通过 `internal/storage` facade 使用 identity/merge，未引入 server
  到 codec 子包的直接依赖；根 `AGENTS.md` 只同步 companions.ai v5 基线。

## TDD RED

- 首轮 v5 codec/merge focused 测试因 `Identity`、lifecycle value、v5 flags、
  `MergeV5` 与 schema 5 尚不存在而编译失败；最小实现后 codec focused 包转绿。
- 根 storage probe 测试先因 mandatory `CompanionsExist` 与 v5 aliases 尚不存在
  编译失败；Memory/Disk metadata-only 实现后转绿。
- persistence 新增 metadata carry、canonical-zero、inactive queue 与 revision
  overflow 四组用例后先失败：旧 job 只携带 body/queue、裸 Summary 仍进入 queue，
  revision 会回绕；最小 carry/checked 实现后转绿。
- server bootstrap 测试先因 injectable identity seam 与 probe-first/同步保存路径
  尚不存在失败；重排 bootstrap 后 empty/missing、retirement/no-repeat、
  provisional body、capacity/entropy/save failure ordering 全部转绿。
- `go test ./internal/storage/companion -run '^TestCompanionCodecV5GoldenRoundTrip$' -count=1`
  先以 `open testdata/companions-v5.bin: no such file or directory` 失败；随后仅用
  显式 `-update-storage-fixtures` 生成 v5 fixture，普通测试逐字节复验通过。
- `go test ./internal/archcheck -count=1` 首轮真实失败为根 baseline 仍是 v4，且
  `internal/server` 直接依赖 `internal/storage/companion`；改由 storage facade
  转发并只更新 v5 baseline 后通过。facade 首版还暴露了两次有用 RED：错误引用
  codec 包不存在的 `Body`，随后 storage 根包直接依赖领域包；最终以 codec/root
  两层 type alias 收口，未放宽依赖白名单。

## Fixture 完整性

旧 v1..v4 文件未修改；最终 SHA-256：

- `companions-v1.bin`（474 bytes）：
  `0153f16082e8b1ac5e47ac7d4a22d8cfb117fe2fc222bd46bec9b116d394120c`
- `companions-v2.bin`（916 bytes）：
  `b919eaa4bfac80d676645980b759414a05370361958efe83f5c3c47e68f65db3`
- `companions-v3.bin`（883 bytes）：
  `3c004af9be5fa57bd25ae6dca6b63fc6beeceb60d269b9b568ff4c5fae05e05d`
- `companions-v4.bin`（1,000 bytes）：
  `6f8a81ee7c096a63e935e27a408797b88925b077145ad9bd704d844a61ffbea9`
- 新 `companions-v5.bin`（1,252 bytes）：
  `9f267e9d1fbcb7f4a83d38c699595d1d5ab4c02d13cab54cc0db8d7b16c89391`

`git diff --name-only` 对 v1..v4 fixture 无输出。

## 最终 GREEN

- `go test ./internal/storage/companion -race -count=1`：PASS
  （package 2.006s；wall 2.950s）。
- `go test ./internal/storage -run 'Companion' -race -count=1`：PASS
  （package 2.285s；wall 3.565s）。
- `go test ./internal/server/persistence -run 'Companion' -race -count=1`：PASS
  （package 2.567s；wall 4.146s）。
- `go test ./internal/server -race -count=1`：PASS
  （package 192.523s）。这是完整 server 包门禁，实际包含
  `TestM5StageAcceptancePersonaDialogueEndToEnd` 与
  `TestCompanionDialogueSummaryLifecycle`。
- `go test ./internal/server -run '^(TestM5StageAcceptancePersonaDialogueEndToEnd|TestCompanionDialogueSummaryLifecycle)$' -race -count=1`：
  PASS（package 3.988s），再次直接证明两条 staging contract 已运行。
- `go test ./internal/archcheck -count=1`：PASS（package 5.693s）。
- `go vet ./internal/storage/companion ./internal/storage ./internal/server/persistence ./internal/server`：PASS。
- `git diff --check`：PASS。

## 文件

- baseline：`AGENTS.md`。
- codec/value/fixture：`internal/storage/companion/companion_types.go`、
  `companion_codec.go`、`companion_v5.go`、对应 codec/restore/summary/fuzz/v5
  tests 与 `testdata/companions-v5.bin`。
- storage facade/probe/atomic tests：`internal/storage/types.go`、`disk.go`、
  `memory.go`、`companion_store_test.go`、`backup_test.go`。
- persistence：`internal/server/persistence/companions.go` 与
  `companion_persistence_test.go`。
- bootstrap/staging：`internal/server/config.go`、`host.go`、
  `companion_bootstrap_test.go`、`companion_dialogue_wiring_test.go`、
  `companion_stage_acceptance_test.go`，以及为合法 v5 seed/Memory close fixture
  做机械适配的 server companion/restart tests。

## 风险与后续边界

- 旧 direct Dialogue Summary 在本阶段仍可作为 transient 运行时输入，但不会
  写入 v5 mirror；真正的 Agent memory commit/reconcile/delete 留给 Task 10。
- Task 9/10 的 Agent/MCP construction 当前不存在，因此 failure-order 测试以
  persistence/world/hostile 构造边界证明同步身份保存先行；后续接线必须保持该
  顺序。
- 本任务未查看、比较或合并 main；未修改 OpenSpec tasks/ledger，也未暂存并发
  Task 6/7 文件。

## 提交状态

实现与报告将按 Task 8 文件清单显式暂存并核对；commit 等待控制会话明确授权。
