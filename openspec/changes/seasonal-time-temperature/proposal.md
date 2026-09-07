## Why

天气系统已落地晴/雨/雷暴权威轮转，但世界仍是「永春」：昼夜长短恒定（昼弧固定 12000 tick）、降水形态只由静态雪线 Y=88 派生（冬天低海拔不下雪、夏天高山照下雪但与任何冷暖无关）。用户要求引入游戏原创的时间与四季体系：每季白天对应时间不同、内置温度体系、温度达到阈值触发下雪天，为后续积雪覆盖（后续 change）打地基。

## What Changes

- **四季时间**：新增权威季节（春/夏/秋/冬），每季 3 游戏日（72000 tick，全年 288000 tick）；季节起点偏移由世界 seed 确定性派生，随 seed 持久化、无新增 metadata 字段。
- **昼长季节化**：昼弧比例按年相位连续正弦曲线 0.35（冬至）..0.65（夏至），换季不跳变；绝对世界时间每 tick +1 与「一天 24000 tick」不变，仅在显示相位层新增季节 warp 入口 `EffectiveDayPhase`，全部判夜/天空消费点统一切换。
- **温度体系**：共享纯函数 `TemperatureAt`（季节基线 + 日内温差 + 降水降温 − 海拔递减），校准锚点：夏至正午海平面 30℃、Y=88 恰 0℃（高山夏雪与旧雪线行为连续）、冬至正午海平面 <0℃（低地冬季降雪）。雪点 0℃、融点 2℃（回差，融点供后续积雪 change 消费）。
- **雪形态联动**：降水形态从「粒子高度 vs 静态雪线」改为「共享温度公式的局部温度 ≤ 雪点」，冬季低地、夏季高山均按温度呈现雪/雨。
- **同步**：`PlayerState` 尾部追加 `Season u8 + SeasonProgress u8 + Temperature int8`（3 字节），协议 v36→v37，越界三处拒绝。
- 非目标：不做积雪/消融与踩雪交互（后续 change）；不做温度对玩家的生存惩罚；不做按季节的地形/植被/作物变化；天气轮转分布不随季节改变。

## Capabilities

### New Capabilities

- `seasonal-time-temperature`: 季节推进、昼长季节 warp、温度公式、协议同步与客户端消费。

### Modified Capabilities

- `authoritative-daylight`: 显示相位在既有偏移之上叠加季节 warp；判夜消费点切换到季节化入口。
- `authoritative-weather`: 降水形态派生依据从静态雪线高度改为共享温度公式的局部温度。

## Impact

- 共享：`packages/shared/core` 新增 season/temperature 纯函数与 `EffectiveDayPhase`；`packages/shared/network` 协议 v37（`PlayerState` 追加 3 字节、golden、越界拒绝）。
- 服务端：`packages/server/sim/runtime` 从 seed 派生季节偏移并每 tick 派生季节/温度随 `PlayerState` 发布；`sim/entity` 判夜消费点（入睡、夜行者窗口、白昼灼烧、昼间牛生成）切换季节化相位。
- 客户端：`packages/client/client` 镜像季节/温度；`packages/client/render` 降水形态温度化、昼夜弧季节 warp、冬季冷色天空 tint；capture 注入季节相位锚定分点（dayFraction=0.5，warp 恒等）以保既有 27 景 golden 逐字节不变。
- 版本：协议 v36→v37；世界 metadata v4、engine ABI v10、client ABI v18、区块 schema v9 均不变；Rust 侧零改动（天空输入仍由 Go 计算经既有帧字段下发）。
- 性能：季节/温度为每 tick 常数次纯函数求值（无遍历、无分配、无 I/O），权威 tick 预算不变。
