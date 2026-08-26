# 权威模拟

## 所有权

- 服务端是世界、玩家、伙伴、库存和玩法状态的唯一权威；客户端输入只表达意图。
- `Engine.Step` 串行组合固定阶段。新增阶段或写者时先核对 `engine_step.go` 的顺序约束、订阅收敛点和最终发布边界，不要在旁路 tick 改权威状态。

## 结算规则

- 状态只在成功路径提交。库存、耐久、产物、容器和方块相互依赖时，先在副本上完整预演，再在同一权威 tick 原子落地；拒绝和容量不足不得留下部分结果。
- 方块写入经 `recordChange` 汇入当前 tick 的 `pendingChunkChanges`，由 `finishChanges` 统一推进 revision 和发布批次。不要另发平行的方块变更通道。
- 每 tick 工作必须有配置或固定上界，并保持确定性顺序。磁盘、网络和模型调用只能通过有界队列或快照离开热路径。

## 依赖方向

本包不得依赖 `internal/client`、`internal/render` 或具体 network transport。协议翻译和会话装配留在上层，模拟只消费领域命令并产出权威结果。

## 定点验证与入口

- 测试：`go test ./internal/sim -race -count=1`。
- 当前文档入口：`docs/notes/go-rust-division.md`。
