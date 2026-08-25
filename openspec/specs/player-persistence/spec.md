# player-persistence Specification

## Purpose
TBD - created by archiving change fix-player-flush-stall. Update Purpose after archive.
## Requirements
### Requirement: Player flush terminates boundedly

`playerPersistence.Flush` SHALL 在单次调用内对每个玩家至多派发一次冻结重试（retry 类）与一次最新快照（fresh 类）保存，且 retry 派发只可能发生在 fresh 派发之前——fresh 派发后该玩家在本次 Flush 内不再有任何派发；即使玩家快照在 Flush 期间持续变化（恒脏态），Flush MUST 在全部已派发保存完成后返回，不得无界重派。继承自 Flush 开始前的 in-flight 保存 MUST 预占该玩家的 retry 类名额，其等待屏障仍按 `(playerID, revision)` 精确身份匹配。

#### Scenario: 恒脏玩家不触发无界重派

- **GIVEN** 一个脏玩家，其快照在每次保存完成之前都被再次更新（保存结果永不等于当前快照）
- **WHEN** 调用 `Flush`
- **THEN** 该玩家在本次 Flush 内恰好被派发一次 fresh 保存，Flush 在该保存完成后返回，不再产生第二次派发

#### Scenario: 冻结重试后仍追派最新快照

- **GIVEN** 一个玩家带有此前失败冻结的 retry job，且其快照已更新为更新内容
- **WHEN** 调用 `Flush` 且冻结重试成功完成
- **THEN** 同次 Flush 内继续派发一次携带最新快照的 fresh 保存，两次派发后 Flush 返回

#### Scenario: 继承失败不在同次 Flush 重派

- **GIVEN** Flush 开始时继承了一个 in-flight 保存，该保存随后失败并冻结为 retry
- **WHEN** 继承等待屏障完成
- **THEN** 本次 Flush 不重派该玩家的冻结 retry，失败按既有错误文本上报，retry 保留给下一次 Flush

### Requirement: Flush stall is reported instead of masked

Flush 在「无 in-flight 保存且本轮未产生新派发」的退出路径上返回时，若仍有已完成加载的玩家处于脏状态且本次 Flush 未记录该玩家的任何保存失败，Flush SHALL 为该玩家计入一次 `errPlayerFlushStalled` 失败并带错误返回；MUST NOT 静默成功返回残余脏状态（ctx 或 scheduler 取消的提前返回本就携带非 nil 终止错误，不属于静默掩盖）。该玩家的脏状态、快照与冻结 retry MUST 完整保留，供后续 Flush 重试。已记录保存失败的玩家 MUST 只上报原错误，既有错误文本逐字不变。

#### Scenario: 恒脏失速带错误返回

- **GIVEN** 一个恒脏玩家已用尽本次 Flush 的派发名额且没有保存失败记录
- **WHEN** Flush 退出
- **THEN** 返回的错误满足 `errors.Is(err, errPlayerFlushStalled)` 且指明该玩家身份

#### Scenario: 失速不丢数据

- **GIVEN** 上一次 Flush 因恒脏以 `errPlayerFlushStalled` 返回
- **WHEN** 快照不再变化并再次调用 `Flush`
- **THEN** 最新快照被派发并成功落盘，Flush 返回 nil

#### Scenario: 已有失败只报原错误

- **GIVEN** 一个玩家在本次 Flush 内已记录一次保存失败（冻结 retry 待下次重试）
- **WHEN** Flush 退出
- **THEN** 该玩家只出现在原保存失败的错误文本中，不额外追加 `errPlayerFlushStalled`

