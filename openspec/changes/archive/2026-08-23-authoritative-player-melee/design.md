# 设计

## 数据所有权和时序

服务端 `sim` 是唯一的近战裁决者。每个 tick 先冻结在线玩家的 primary-action 意图，再按稳定玩家顺序处理；命中候选只来自 active、同维度玩家。射线最长 3 格，固体方块阻挡、流体不阻挡；取最近命中，距离相同按 `SessionID` 选择。一次有效命中经既有伤害入口扣 2 点，并以目标的最近成功命中 tick 实现 10 tick 冷却。

同 tick 冻结避免处理先后影响「谁正在持续 primary action」。命中玩家只抑制发起者该 tick 的采掘分支；无合法玩家命中时立即回到原采掘路径，下一 tick 仍完全取决于持续输入。

## 协议与兼容性

`PlayerInput.Mining` 在 v25 起表示持续 primary action；其字段顺序、packet ID 和编码不变。握手版本升级到 25，拒绝 v24 和更早版本，不进行协商或降级。没有存档格式变化，也没有新的跨 goroutine 消息。

## 风险与验证

候选遍历必须使用稳定顺序，不能依赖 map。验证覆盖阻挡、流体、距离、平局、冷却、同 tick 快照和采掘分流；网络 golden 固定 hello 的 v25 uvarint 与所有 play packet 不变字节。
