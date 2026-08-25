## Why

`playerPersistence.Flush` 的重派去重使用 `(playerID, revision)` 精确键：每次成功完成后 `dirty = !matchesSave(save)`，恒脏态（快照与存档内容永不相等）下每轮都会用 `save(persisted + 1)` 铸出新 revision，精确键永不命中，Flush 进入无界重派自旋，只能靠 ctx 取消终止；关服路径的 Flush（`host_shutdown.go`）因此可能挂起。这是 `authoritative-hunger` design「遗留与简化清单」第 9 条（规划表 E-07）。

## What Changes

- 重派去重键从 `(playerID, revision)` 精确键改为按 `core.PlayerID` 记账，每玩家区分两个派发类：retry 类（冻结重试；继承 in-flight 在 Flush 开始即预占该类）与 fresh 类（`persisted + 1` 最新快照；fresh 派发同时占用两类）。每玩家每次 Flush 每类至多派发一次，自旋源被切断，Flush 有界终止（每玩家 ≤ 2 次派发 + 继承等待）。
- Flush 退出时仍有脏且该玩家没有已记录失败 → 以 `errPlayerFlushStalled` 计入 failures 一次并带错误返回，不静默掩盖残余脏状态；脏状态保留给下一次 Flush 重试。已有失败记录的玩家照旧只报原错误，全部既有错误文本逐字不变。
- 继承 in-flight 的等待屏障仍用 `(playerID, revision)` 精确身份（`TestPlayerFlushInheritedBarrierRejectsForeignRevision` 钉住的语义），只有重派记账换键。
- 新增恒脏自旋回归测试：controllable store 在每次完成前 `Observe` 新快照模拟恒脏，断言 Flush 恰在有界派发后返回 `errPlayerFlushStalled`、store 尝试次数有界、脏数据保留并可被下次 Flush 落盘。

## Non-Goals

- 不改 `Poll`/`Observe`/autosave 的派发路径与 retry backoff 语义。
- 不改伙伴侧 Flush：`companion_persistence.go` 已是有界结构（至多首派 + 一次补派），无同构缺陷。
- 不改玩家 schema、协议或任何 wire 形状；无迁移。

## Impact

受影响代码：`internal/server/player_flush.go` 与 `internal/server/` 新增恒脏测试文件；10 条既有 Flush 测试零改动全过（已逐一沙盘核对）。行为变化仅两点：恒脏/失速时 Flush 由「无界自旋或静默成功」变为「有界终止并带 `errPlayerFlushStalled` 返回」；`TrySubmit` 失败导致脏玩家未派发时同样上报失速而非静默成功——两者都朝数据丢失门禁的安全方向收紧。回退提交即可恢复旧行为。
