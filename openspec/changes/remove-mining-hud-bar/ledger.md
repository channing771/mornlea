# remove-mining-hud-bar Ledger

基线 SHA：`dca9197e`（main）。分支 `remove-mining-hud-bar`。

## 阶段 1：内容确认

- Ruling: 需求方在对话中直接裁决「破坏进度条可以去掉，使用裂纹替换」—
  与已交付的裂纹反馈构成明确取舍（屏幕条与裂纹并存重复）— 分类 bounded
  （既有呈现通道的删除，无新子系统）。设计决策（桥分节彻底删除、
  Harvestable 死字段一并删、进食条独立）见 design.md。

## 任务执行

（implementer 派发、评审结论、验证证据按 SHA 记录于此）
