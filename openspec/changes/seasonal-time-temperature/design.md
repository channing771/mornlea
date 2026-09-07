# 设计：seasonal-time-temperature

## 0. 数值总表（全部钉在 `shared/core`，单测锚定）

| 量 | 值 | 说明 |
|---|---|---|
| `SeasonLengthTicks` | 72000 | 3 游戏日/季（用户裁决） |
| `YearTicks` | 288000 | 四季整年 |
| 昼弧比例 `dayFraction(yearPhase)` | `0.5 + 0.15·sin(2π·yearPhase)` | 冬至最短 0.35、夏至最长 0.65、分点 0.5 |
| `SeasonIndex` | `floor(yearPhase·4)`，0=春 1=夏 2=秋 3=冬 | 季节边界在 yearPhase 的 1/4 整分点 |
| 昼弧 tick `D` | `round(dayFraction·24000)` 取偶数 | 一天仍是 24000 tick |
| 季节基线（海平面） | `11 + 19·sin(2π·yearPhase)` ℃ | 夏至中点 +30、冬至中点 −8 |
| 日内温差 | `5·sin(2π·(effPhase−6000)/24000)` ℃ | 正午 +5、午夜 −5、晨昏 0 |
| 降水降温 | 雨/雷暴 −4℃ | 晴 0 |
| 海拔递减 | 海平面(64) 以上每格 −1.25℃，以下不升温 | 24 格（Y=64→88）恰 −30℃ |
| 钢锚点 | 夏至正午海平面 30℃；夏至正午 Y=88 = 0℃；冬至正午海平面 −8℃ | 后者保证冬季低地全境降雪 |
| 雪点 / 融点 | 0℃ / 2℃ | 回差；融点常量本轮落库供积雪 change 消费 |
| 温度上下界 | clamp 到 [-40, 45]℃ | wire int8 容量内 |

季节语义核对：夏 [0.25,0.5) 内 dayFraction 由 0.65 递减到 0.5（昼最长季）；冬 [0.75,1) 由 0.35 升到 0.5（昼最短季）；春递增、秋递减——「至点在季节边界、分点也在季节边界」，气象学式季节划分，观感自洽。

## 1. 季节推进（`shared/core` 新文件 `season.go`）

- `Season uint8` 枚举（Spring/Summer/Autumn/Winter，0..3）。
- `SeasonOffsetFromSeed(seed int64) uint64`：`splitmix64(seed ^ 固定盐) % YearTicks`，`NewEngine` 装配期求一次，字段无锁只读（与 weatherKind 同纪律）。**零持久化**：seed 已在 metadata v1 持久化，同 seed 重启偏移不变；seasonOffset 是派生量不是权威状态。
- `YearPhaseAt(worldTime, seasonOffset) float64` = `(worldTime+offset) % YearTicks / YearTicks`。
- `SeasonAt(...)`、`SeasonProgressAt(...)`（0..255 u8 量化）。
- 服务端访问器 `SeasonOffset()` 供契约层组装；capture 用 override 注入（见 §5）。

## 2. 昼长 warp（`day_phase.go` 扩展，唯一相位算式入口）

现有 `DisplayDayPhase(worldTime, offset)` 语义不动（未 warp 入口，绝对时间→线性相位）。新增：

- `DayFractionAt(yearPhase) float64`（§0 公式）与 `DayArcTicks(yearPhase) uint16`（D，取偶）。
- `EffectiveDayPhase(worldTime, offset uint16, dayArc uint16) uint16`：先把 `(worldTime%24000+offset)%24000` 得线性相位 p，再按 p∈[0,D) 映射 `e = p·12000/D`、p∈[D,24000) 映射 `e = 12000+(p−D)·12000/(24000−D)`。D=12000（分点）时恒等——**既有全部行为与视觉基线在分点逐字节不变**。
- `EffectiveMorningOffset(worldTime, dayArc) uint16`：反解「季节化相位 = 早晨常量」所需 offset，供入睡跳夜换算（读 `sim/entity/sleep.go` 现有早晨相位常量后对齐）。

实现约束：全 uint32 中间量防溢出、纯函数、中文注释写明「绝对时间消费者禁用本函数」。

### 消费点切换清单（一处不漏，切完 grep 无残端）

| 消费点 | 文件 | 动作 |
|---|---|---|
| 入睡判定/跳夜 | `sim/entity/sleep.go`（14/51/80 行附近） | 判夜换 `EffectiveDayPhase`；跳夜 offset 换 `EffectiveMorningOffset` |
| 夜行者生成窗口 | `sim/entity/hostile_spawn.go`（15/65 行附近） | 换季节化相位 |
| 白昼灼烧 | `sim/entity/hostile.go`（317/328 行附近） | `phaseIsDay` 内部换 |
| 昼间牛生成 | `sim/entity/passive_spawn.go`（37 行附近） | 同上 |
| runtime 派生 | `sim/runtime/engine.go`（62/162/164 行附近，weather/dayPhase 同区） | Step 尾部派生 season/seasonProgress/effPhase 供契约 |
| 客户端昼夜 | `client/render/daylight.go`（62/72/75 行附近） | `DayNightAt` 增 yearPhase/dayArc 输入，内部换 `EffectiveDayPhase` |

## 3. 温度（`shared/core` 新文件 `temperature.go`）

- `TemperatureAt(yearPhase, effPhase uint16, weather core.WeatherKind, y float32) float32`：§0 公式合成后 clamp。纯函数。
- 常量 `TemperatureSnowPoint = 0`、`TemperatureMeltPoint = 2`、`TemperatureLapsePerBlock = 1.25`、`TemperatureSeaLevel = 64`（与 Rust `SEA_LEVEL` 对齐的 Go 侧镜像常量，注释钉住双端语义）。
- 降水形态判定 `PrecipitationIsSnow(yearPhase, effPhase, weather, y) bool` = `TemperatureAt(...) <= TemperatureSnowPoint`。

## 4. 协议 v37 与服务端发布

- `packages/shared/network/protocol/packet.go`：`ProtocolVersion 36 → 37`。
- `message_player.go` `PlayerState` 尾部（`WeatherKind` 之后）追加：`Season uint8`（0..3）、`SeasonProgress uint8`（0..255）、`Temperature int8`。Season 越界三处拒绝（Validate/编码/解码，沿用 v36 WeatherKind 模式）；Temperature int8 域内天然有界。
- codec 编解码 + golden 新版本条目；`packages/audit` 版本基线测试同步 v37。
- 服务端：`sim/contract` `PlayerUpdate`/`TickResult` 追加派生字段 → `server/publication.go` 按人复制（温度按各玩家 Y 求值，`WeatherKind` 同批取值）。
- 客户端 predictor：镜像三字段、起始校验、ServerTick 门控、越界拒绝、`Weather()` 旁新增 `Season()/SeasonProgress()/Temperature()` 访问器。

## 5. 客户端表现

- **降水形态**：`client/render/weather.go` `BuildWeatherParts` 判定从 `particleY >= WeatherSnowLineY` 换为 `PrecipitationIsSnow(yearPhase, effPhase, weather, particleY)`；`WeatherSnowLineY` 常量退役为校准锚（保留并注释其新角色：Y=88 是夏至正午的等效雪线锚点）。输入（yearPhase/effPhase）由 app 从镜像状态算出传入。
- **昼夜弧**：`daylight.go` 太阳/亮度/星空全部消费 `EffectiveDayPhase`；冬季天空冷色 tint（`ClearColor` 向冷蓝偏移 ≤0.03，随 yearPhase 正弦权重，分点为 0——保基线）。
- **Rust 零改动**：sky uniform 输入仍是 Go 计算的 daylight/sky_color（帧 TLV 与 client ABI v18 不动）。
- **capture 基线零重录裁决**：`capture/scene_application.go` 增加 capture-only 季节相位 override（模式照抄 `SetCaptureWeather`），默认把 yearPhase 钉在 **分点 0（春始，dayFraction=0.5）** → warp 恒等、冷 tint=0 → 既有 27 景 golden 逐字节不变。`rain-noon` 场景（雪线下机位锁雨形）在分点+正午 15℃ 下天然仍为雨，语义保持。

## 6. 并发与预算

seasonOffset 装配期一次写、tick 串行读（同 weatherKind 纪律）；season/temperature 每_tick_派生为 O(1) 纯函数（每玩家一次温度求值，玩家数上限既有约束）；无新 goroutine、无 I/O、无分配进热路径。audit `sim_authority` 派生字段不入权威字段集（无锁语义不变），版本基线测试升 v37。

## 7. 被否决的替代方案

- **持久化 seasonOffset 到 metadata v5**：seed 派生已保证重启稳定，v5 迁移纯增包袱 — 被否决，零迁移。
- **改一天的实际 tick 数实现昼长**：污染全部绝对时间消费者（作物/流体/掉落）契约 — 被否决，warp 留在显示层。
- **新增权威 WeatherKind::Snow=3**：温度已是权威派生，形态再占权威状态造成双真值 — 被否决，维持「形态客户端派生」哲学，仅换派生依据。
- **季节边界跳变昼长**：换季瞬间太阳瞬跳，观感差 — 被否决，年相位连续正弦。
- **wire 温度作为粒子形态唯一输入（不本地派生）**：PlayerState 只有玩家单点温度，无法表达每粒子海拔差异 — 被否决，双端共享公式本地求值（与旧雪线本地判定同哲学）。

## 8. 验证方法

- `go test ./packages/shared/core -race -count=1`（锚点：分点恒等、夏至 15600/8400、三温度锚、换季连续、反解）
- `go test ./packages/shared/network/... -race -count=1`（v37 golden、三处拒绝）
- `go test ./packages/server/... -race -count=1`（派生接线、publication、sleep/h spawn 判夜切换）
- `go test ./packages/client/... -race -count=1`（镜像、形态温度化、daylight warp、capture 恒等）
- `make visual-check`（预期 27 景零差异——分点锚定裁决）
- 收尾：`gofmt -l`、六模块 vet、`make dev-check`、`make test-race`、`openspec validate --all --strict --no-interactive`
