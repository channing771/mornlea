# Tasks

## 1. OpenSpec 契约与 v25 wire 冻结

- [x] 写入近战与采掘分流的 delta specs、设计、提案和 ledger。
- [x] 先更新失败测试，再将协议版本升至 v25；保持 `PlayerInput` 与所有 play packet wire 不变。
- [x] 更新握手 golden、fuzz 种子、当前版本断言及双份基线文档。
- [x] 运行网络、入口、架构和 OpenSpec 验证。

## 2. 权威近战裁决

- [ ] 在 `sim` 实现 active 同维玩家的 3 格、最近命中、`SessionID` 平局、方块遮挡和流体穿透。
- [ ] 通过既有伤害入口结算 2 点伤害和 10 tick 目标冷却，并覆盖同 tick 意图快照与采掘抑制。
- [ ] 验证单机与 TCP 共用权威路径。
