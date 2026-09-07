# seasonal-time-temperature Specification

## ADDED Requirements

### Requirement: 季节由绝对世界时间确定性推进

系统 SHALL 以绝对世界时间派生季节：每季 3 游戏日（72000 权威 tick），全年四季共 288000 tick 循环；季节起点偏移（0..287999）MUST 由世界 seed 确定性派生一次（`NewEngine` 装配期），重启 MUST NOT 改变同一 seed 的季节相位，MUST NOT 新增 metadata 持久化字段。季节枚举 SHALL 为春/夏/秋/冬四值；年内进度 SHALL 随时间在季内单调递增并于换季回绕。

#### Scenario: 季节随时间推进并回绕

- **GIVEN** 固定 seed 与季节偏移使世界处于春季起点
- **WHEN** 绝对世界时间推进 72000、144000、288000 tick
- **THEN** 季节 MUST 依次为夏、秋、春（288000 tick 恰整年回绕）

#### Scenario: 同 seed 重启季节相位不变

- **GIVEN** 一个已运行至秋季的世界
- **WHEN** 服务端以同一 seed 重启并恢复世界时间
- **THEN** 派生出的季节与年内进度 MUST 与关服前一致

### Requirement: 昼长按年相位连续伸缩且换季不跳变

显示相位 SHALL 在既有「绝对时间 + 显示偏移」之上叠加季节 warp：昼弧比例 MUST 按年相位连续正弦曲线取值，冬至最短昼 0.35、夏至最长昼 0.65、春秋分点恒等 0.5；一个昼夜的绝对 tick 数 MUST 保持 24000 不变，绝对时间消费者（作物、流体、掉落寿命）MUST NOT 受 warp 影响。季节化相位入口 SHALL 是 `shared/core` 的唯一新算式（`EffectiveDayPhase` 一类），MUST NOT 在任何消费方自建 warp。分点（昼弧比例 0.5）时 warp MUST 严格恒等于未 warp 的显示相位。

#### Scenario: 夏至昼长夜短

- **GIVEN** 年相位处于夏至（昼弧比例 0.65）
- **WHEN** 比较同一显示周期内白昼弧与黑夜弧的有效相位跨度
- **THEN** 白昼弧跨度 MUST 约为黑夜弧的 1.86 倍（15600 比 8400 tick）

#### Scenario: 分点 warp 恒等

- **GIVEN** 年相位处于春/秋分点（昼弧比例 0.5）
- **WHEN** 对任意绝对时间与显示偏移求季节化相位
- **THEN** 结果 MUST 逐值等于既有未 warp 显示相位

#### Scenario: 换季相位连续不跳变

- **GIVEN** 年相位跨过任意季节边界前后相邻的两个 tick
- **WHEN** 分别求季节化相位
- **THEN** 两值 MUST 连续（差值在单 tick 相位步进量级内），MUST NOT 出现瞬跳

### Requirement: 温度由季节、日内相位、天气与海拔共享公式派生

系统 SHALL 提供跨端共享的温度纯函数：海平面温度 = 季节基线（正弦年曲线）+ 日内温差（正午最高、午夜最低）+ 降水降温（雨/雷暴固定值）；任意高度温度 MUST 再按海拔递减（海平面以上每格固定降温，海平面以下不升温）。校准锚点 MUST 满足：夏至正午海平面 30℃、夏至正午 Y=88 为 0℃（高山夏雪与旧雪线行为连续）、冬至正午海平面低于 0℃（低地冬季降雪）。雪点 SHALL 为 0℃；融点 SHALL 为 2℃（回差防抖，供积雪能力消费）。温度函数 MUST 无副作用、不读墙钟与随机数。

#### Scenario: 夏季高山按温度下雪

- **GIVEN** 夏至正午、权威天气为雨、降水粒子位于 Y=90
- **WHEN** 求该处局部温度并判定降水形态
- **THEN** 局部温度 MUST ≤ 雪点，形态 MUST 为雪

#### Scenario: 冬季低地按温度下雪

- **GIVEN** 冬至正午、权威天气为雨、降水粒子位于海平面高度
- **WHEN** 求该处局部温度并判定降水形态
- **THEN** 局部温度 MUST ≤ 雪点，形态 MUST 为雪

#### Scenario: 降水与夜间降温有界

- **GIVEN** 任意季节任意高度
- **WHEN** 求局部温度
- **THEN** 结果 MUST 落在函数声明的固定上下界内，MUST NOT 发散

### Requirement: 季节与温度随玩家状态同步

`PlayerState` SHALL 在天气字段之后追加季节（u8，0..3）、年内进度（u8，0..255）与玩家位置温度（int8，摄氏度）三字节（协议 v37）；季节越界值 MUST 在 Validate、编码与解码三处被拒绝；温度 MUST 为该玩家所在高度按共享公式求得的权威观察值。客户端 MUST 以 ServerTick 门控镜像接受，MUST NOT 本地外插回退旧值。

#### Scenario: 越界季节被三处拒绝

- **GIVEN** 一条 `PlayerState`，其季节字段为 4
- **WHEN** 分别执行 Validate、编码与解码
- **THEN** 三处 MUST 均返回错误

#### Scenario: 玩家状态携带温度

- **GIVEN** 冬季午夜一名站在海平面的玩家
- **WHEN** 服务端发布该玩家的 `PlayerState`
- **THEN** 温度字段 MUST 等于共享公式在该时刻、该高度的值，且为负摄氏度
