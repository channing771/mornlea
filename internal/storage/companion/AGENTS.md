# companion 包：companion 存档域 codec

`internal/storage/companion` 承载 companion 存档域：companions.ai 聚合
文件（MCAI 信封）的编解码、任务区/FIFO/摘要载荷校验与伙伴存档值类型。
本包是纯 codec 域：依赖 `internal/companion` 领域模型与 `internal/core`
值类型，哨兵经 `internal/storage/storagedef` 取用；不感知根包编排
（companions.ai 文件的原子替换与路径编排在根包
DiskStore/MemoryStore），`CompanionStore` 接口属根包存储契约家族，定义
留在根包 `types.go`。依赖方向由 `internal/archcheck` 的
`TestInternalDependenciesAreOneWay` 强制。行为规格见
`openspec/specs/local-data-migration/spec.md`；全树共享的迁移与数据安全
纪律见上级 `../AGENTS.md`，本文件不重复。

## 信封编解码 (`companion/companion_codec.go`, `companion/codec_primitives.go`)

- `Encode`/`Decode` 是本域唯一编解码入口（原根包内部入口改名导出）：
  解码 schema 白名单是字面枚举（只接受已存在的 schema 成员，不做范围
  比较），`TestCompanionDecodeSchemaWhitelistListsLiteralV4` 钉死白名单
  语义，未来版本文件按损坏拒绝。
- `MaxFileLength` 是物理文件字节上界，编码与解码两侧都在解析前按它
  拒绝越界；`CurrentSchema` 导出供根包与测试构造故障注入。损坏矩阵
  （CRC/截断/未来版本/超限记录）由
  `TestCompanionCodecRejectsCRCTruncationFutureVersionAndOversizedRecords`
  钉死。
- 记录身份约束：记录按 ID 严格升序、不允许重复，
  `TestCompanionCodecRejectsDuplicateOrUnsortedIDs` 钉死；边界容量位形
  由 `TestCompanionCodecAcceptsMaximumStoredRecords` 钉死。
- 隐私边界：存档只持久化身体快照与任务域载荷，不持久化名字、任务文本
  与 persona，`TestCompanionCodecDoesNotPersistNameTaskOrPersona` 钉死。
- `codec_primitives.go` 是本域私有字节原语副本：与 chunk/player/hostile
  包的同名助手同源，域内 codec 是唯一消费方，域间不共享原语包。

## 持久化上界同源 (`companion/companion_types.go`)

- 任务区与 FIFO 的持久化上界常量与 `internal/companion` 的运行期上界
  同源（直接引用同一常量，如任务指令上界绑定
  `companion.MaxPlanCommandBytes`、FIFO 条数绑定
  `companion.MaxTaskQueueDepth`、摘要上界绑定
  `companion.MaxDialogueSummaryBytes`）：同一常量保证「能被写进请求的
  摘要/指令」与「能被存档保留」两条边界不可能漂移。取值以这些常量为准，
  不在指南复制数字。
- `StoredCompanions`/`CompanionSave`/`StoredCompanionTask`/
  `StoredCompanionQueue` 是域值类型，根包以类型别名再导出保持
  `storage.X` 引用与类型身份不变；`ErrCompanionsNotFound` 随域定义
  （「聚合存档缺失」是本域存档契约的一部分），根包绑定同一错误值再导出。

## 跨重启恢复与迁移 (`companion/companion_restore_test.go`, `companion/companion_summary_test.go`)

- 任务区与 FIFO 跨重启精确恢复、只含 FIFO 的队列往返、损坏任务载荷
  拒绝、计划步骤与 FIFO 条数上界，由 `TestCompanionRestoreTasksAndFIFOAcrossRestart`、
  `TestCompanionRestoreFIFOOnlyQueueRoundTrip`、
  `TestCompanionRestoreRejectsCorruptTaskPayloads`、
  `TestCompanionRestorePlanAndFIFOBounds`、
  `TestCompanionRestoreV3MaxVariableStepBudget` 钉死。
- 旧 schema 只读迁移、首次保存写当前 schema、摘要区的字节上界与空摘要
  语义、inactive 去激活丢弃摘要，由 `TestCompanionRestoreV1ReadOnlyMigrationAndFirstSaveWritesV4`、
  `TestCompanionRestoreV2ReadOnlyMigrationAndV4Rewrite`、
  `TestCompanionCodecV4SummaryBoundariesAndEmptySemantics`、
  `TestCompanionCodecV4FileBoundIncludesSummary`、
  `TestCompanionRestoreV4DeactivationDropsSummary` 钉死。
- 域内最小装配 `companionFileFixture`（定义在
  `companion_restore_test.go`）：以本包 `Encode`/`Decode` 直接读写
  companions.ai 文件，替代根包 DiskStore 夹具——域包测试禁止反向导入
  根包。只承载装配不改断言；原子替换、revision 冲突与关闭语义等 store
  编排行为仍由留根的根包 `companion_store_test.go` 以 DiskStore 覆盖。
- `-update-storage-fixtures` flag 语义与 chunk/player 包一致（声明在
  `companion_summary_test.go`）：置位时重写本包 committed fixture，
  普通运行只读比较、漂移一律失败。

## helper 中心与回归测试 (`companion/companion_restore_test.go`)

- 共享夹具（`companionFileFixture`、记录/队列构造器）住
  `companion_restore_test.go`；每包最多一个 helper 中心，规则见
  `docs/test-organization.md`。
- golden 冻结回归由 `TestCompanionCodecV1RoundTripAndGolden` 等
  `TestCompanionCodecV*RoundTripAndGolden` 族钉住；模糊入口
  `FuzzDecodeCompanions` 以 fixture 与合成载荷为种子。

## Focused Verification

- 定点测试：`go test ./internal/storage/companion -race -count=1`（纯
  codec 域，秒级，不编译执行其他域的测试）。
- 根包编排：`go test ./internal/storage -run 'CompanionStore|DiskCompanion' -race -count=1`。
- 依赖方向与文档守卫：`go test ./internal/archcheck -count=1`。
