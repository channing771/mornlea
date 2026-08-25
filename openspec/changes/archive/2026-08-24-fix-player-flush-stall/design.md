# fix-player-flush-stall 设计（方案 A′，已获批准）

## 确认结论（brainstorming 轮次）

- 第 1 轮（2026-08-25T01:52Z）：呈现方案 B（保留精确键 + 每玩家派发上限 4）与方案 A（去重键去掉 revision）。用户回复 `edit: A`。
- 第 2 轮（2026-08-25T02:16Z）：落地核对发现纯 `playerID` 单名额会破坏三条既有钉住测试（冻结 retry 后必须追派 fresh、继承完成后必须追派最新快照、继承失败不得同 Flush 重派），提出修订版 A′——仍去掉 revision，但名额分 retry / fresh 两类。用户回复 `approve`（2026-08-25T02:49Z）。本设计即 A′。

## 根因

`Flush` 主循环用 `attempted map[playerSaveKey]struct{}`（`(playerID, revision)` 精确键）去重。成功完成后 `applyCompletionWithDispatchLocked` 置 `dirty = !player.matchesSave(save)`；恒脏态下 `playerFlushJob` 每轮返回 `save(persisted + 1)`——revision 单调铸新，精确键永不命中，循环无界。

## 方案 A′

### 记账结构

`attempted` 改为 `map[core.PlayerID]playerFlushSlots`，其中：

```go
// playerFlushSlots 记录单个玩家在本次 Flush 内已占用的派发类。
type playerFlushSlots struct {
    retry bool // 冻结重试类：player.retry 非空的重派，或继承 in-flight 预占
    fresh bool // 最新快照类：save(persisted + 1) 的新 revision 派发
}
```

规则：

1. **继承预占**：Flush 开始快照 in-flight 玩家时，除把 `(playerID, revision)` 精确键放进 `inherited` 等待屏障外，同时置该玩家 `slots.retry = true`。继承完成若失败会冻结 retry，本次 Flush 不得重派它（`TestPlayerFlushInheritedFailureDoesNotDispatchForcedFollowup` 钉住）。
2. **retry 派发**（`player.retry != nil`）：要求 `!slots.retry`；派发后置 `slots.retry = true`，不占 fresh——冻结重试成功后仍允许追派最新快照（`TestPlayerFlushFailureRetainsFrozenJobForLaterRetry` 钉住）。
3. **fresh 派发**（`player.retry == nil`）：要求 `!slots.fresh`；派发后 `slots.retry = slots.fresh = true`。占 retry 是因为 fresh 失败会冻结同 revision 的 retry，同 Flush 重派它会违反「每 revision 只试一次」（`TestPlayerFlushAttemptsEachRevisionOnceAndSortsErrors` 钉住）。
4. 主循环挑选候选时按类判占用：被占则 `continue` 看下一个玩家；其余结构（sorted 枚举、单玩家 dispatch 后 `break`、completion 处理、`dispatchFollowup=false`）不动。

由 1–3，每玩家每次 Flush 至多 2 次派发（retry → fresh），循环有界终止。

### 失速上报

`Flush` 的 `!inFlight && !dispatched` 退出路径（唯一可能带残余脏返回的路径）在返回前，对活动缓存中仍 `dirty && !loading` 的每个玩家检查 `failures` 是否已含该 `playerID` 的记录：

- 无记录 → `failures[playerSaveKey{playerID: id, revision: player.persisted + 1}] = errPlayerFlushStalled`（revision 取「被挡下的下一个 fresh revision」，错误文本经既有 `joinPlayerFlushErrors` 格式化，确定性排序不变）。
- 有记录 → 不追加，照旧只报原错误；既有全部错误文本逐字不变。

`errPlayerFlushStalled` 为包内哨兵：`errors.New("server: player flush stalled: still dirty after bounded dispatches")`。脏状态、冻结 retry 与快照全部保留，下次 Flush 正常重试——不掩盖残余脏，方向与数据丢失门禁一致。

### 附带收紧（有意为之）

旧代码在 `TrySubmit` 失败（scheduler 满/已关）时会带着脏玩家静默成功返回；新退出检查使这类情况同样上报 `errPlayerFlushStalled`。关服路径对 Flush 失败本就保留 runtime 并重试（`TestHostShutdownRetriesPlayerFlushBeforeWorldClose`），行为兼容。

## 不变量与既有测试

- 继承等待屏障 `waitInheritedFlushCompletions` 的 `inherited` 精确键、外键 completion 丢弃语义不变（`TestPlayerFlushInheritedBarrierRejectsForeignRevision`）。
- `failures` 仍键 `playerSaveKey`、仍经 `joinPlayerFlushErrors` 排序拼接；错误文本格式 `save player %s revision %d: %w` 不变。
- 10 条既有 Flush 测试（`player_flush_test.go` 2 条、`player_flush_barrier_test.go` 8 条）已逐一沙盘核对，零改动全过。

## 新测试

新文件 `internal/server/player_flush_stall_test.go`（单一主题：恒脏自旋有界终止）：controllable store 收到 save 后、完成前由测试线程 `Observe` 新快照制造恒脏；断言 Flush 返回错误且 `errors.Is(err, errPlayerFlushStalled)`、store 恰收到 1 次 save（无第二次派发）、随后一次 Flush 把最新快照落盘并返回 nil（脏数据未丢失）。

## 被否决的替代

- **方案 B（精确键 + 派发上限 4）**：第 1 轮被用户否决（选 A）。
- **纯 `playerID` 单名额**：破坏三条既有钉住测试，且对应真实关机数据丢失场景，故修订为双类名额。
- **静默成功返回**：掩盖残余脏状态，违背数据丢失门禁方向。

## 受影响文件

- `internal/server/player_flush.go`（记账结构与退出检查）
- `internal/server/player_flush_stall_test.go`（新增）
