# seasonal-time-temperature ledger

- 基线 SHA：`b179fec1`（origin/main）
- Ruling: 季节偏移由 seed 派生而非持久化 metadata v5 — seed 已随 metadata v1 持久化，同 seed 重启偏移确定不变 — v5 迁移是零收益包袱。
- Ruling: 昼长走显示相位 warp 而非改昼夜实际 tick 数 — 绝对时间消费者（作物/流体/掉落寿命）契约钉死与显示相位正交 — 改绝对周期会污染全部寿命结算。
- Ruling: 降水形态维持客户端派生、仅换依据（共享温度公式局部求值）而非新增权威 WeatherKind::Snow — 温度已权威，形态再占状态即双真值 — 延续 authoritative-weather「形态不进权威状态」既有哲学。
- Ruling: capture 季节相位 override 默认钉分点（dayFraction=0.5，warp 恒等、tint=0）— 既有 27 景 golden 逐字节不变，本 change 视觉风险归零 — 冬季雪景场景留给积雪 change 一并补。
- Ruling: 每季 3 游戏日、昼弧 0.35..0.65、温度三钢锚点（夏至正午海平面 30℃/Y=88=0℃/冬至正午海平面 <0℃）与踩雪四项交互均经用户显式选择/批准（本会话 AskUserQuestion 与计划批准）。
- 版本：协议 v36→v37；metadata v4 / engine ABI v10 / client ABI v18 / 区块 schema v9 不变；Rust 零改动。

## 任务进度

（每任务完成后由控制会话回填验证证据与评审结论）
