# seasonal-time-temperature ledger

- 基线 SHA：`b179fec1`（origin/main）
- Ruling: 季节偏移由 seed 派生而非持久化 metadata v5 — seed 已随 metadata v1 持久化，同 seed 重启偏移确定不变 — v5 迁移是零收益包袱。
- Ruling: 昼长走显示相位 warp 而非改昼夜实际 tick 数 — 绝对时间消费者（作物/流体/掉落寿命）契约钉死与显示相位正交 — 改绝对周期会污染全部寿命结算。
- Ruling: 降水形态维持客户端派生、仅换依据（共享温度公式局部求值）而非新增权威 WeatherKind::Snow — 温度已权威，形态再占状态即双真值 — 延续 authoritative-weather「形态不进权威状态」既有哲学。
- Ruling: capture 季节相位 override 默认钉分点（dayFraction=0.5，warp 恒等、tint=0）— 既有 27 景 golden 逐字节不变，本 change 视觉风险归零 — 冬季雪景场景留给积雪 change 一并补。
- Ruling: 每季 3 游戏日、昼弧 0.35..0.65、温度三钢锚点（夏至正午海平面 30℃/Y=88=0℃/冬至正午海平面 <0℃）与踩雪四项交互均经用户显式选择/批准（本会话 AskUserQuestion 与计划批准）。
- 版本：协议 v36→v37；metadata v4 / engine ABI v10 / client ABI v18 / 区块 schema v9 不变；Rust 零改动。

- Ruling: 温度日内项采用公式字面值（黄昏 +5 峰/黎明 −5 谷，热滞后形态）而非旁注「正午最高、午夜最低」 — 全部硬锚点（夏至正午海平面 30℃、Y=88 恰 0℃、冬至正午 <0）与旁注公式互斥而与字面公式唯一相容（旁注公式会给夏至正午 35℃、Y=88 为 5℃，直接违反两条 MUST 锚点；若改锚点保旁注则 Y=88 连续性破坏且冬正午无法同时 <0） — 实现者 aa46 发现矛盾上报，控制会话修 design §0/§3 与 spec 第三条 Requirement 措辞后放行。

## 任务进度

- 1.1+1.2 季节推进与昼长 warp：提交 `6026e6b3` + 加固提交 `3aad2e57`（基线 60280cef）。验证：`go test ./packages/shared/core -race -count=1` ok（1.4s/1.9s）、gofmt/vet 干净。评审 APPROVE（8 项全 PASS）：数值锚点独立复算吻合（冬至 8400/夏至 15600/分点 12000、SeasonProgress 值域恰 [0,255]）、uint32 中间量无溢出、「取偶效应」微抖经独立重写差分扫描证实为真实固有量化效应（全年 arc 变化 8128 次均 ±2、断言上界 5 恰紧非掩盖）、EffectiveMorningOffset 五昼弧×24000 M×5 worldTime 差分零失配、splitmix64 与 server 侧逐行同语义。加固：盐值 golden 锚（seed 0→158435、42→14818、MinInt64→186548）+ 连续性注释更正（实现者以实测反例纠正评审「前半段 ≤−1」说法：黑夜支路前半段亦可 −2，注释写入两处均至多 −2 的可证伪版本，断言 |Δ|≤5 未动）。Ruling: 注释按实证版本而非评审口述 — 注释必须可被代码证伪检验 — 评审的推导只在冬至邻域成立。
