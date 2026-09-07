# seasonal-time-temperature ledger

- 基线 SHA：`b179fec1`（origin/main）
- Ruling: 季节偏移由 seed 派生而非持久化 metadata v5 — seed 已随 metadata v1 持久化，同 seed 重启偏移确定不变 — v5 迁移是零收益包袱。
- Ruling: 昼长走显示相位 warp 而非改昼夜实际 tick 数 — 绝对时间消费者（作物/流体/掉落寿命）契约钉死与显示相位正交 — 改绝对周期会污染全部寿命结算。
- Ruling: 降水形态维持客户端派生、仅换依据（共享温度公式局部求值）而非新增权威 WeatherKind::Snow — 温度已权威，形态再占状态即双真值 — 延续 authoritative-weather「形态不进权威状态」既有哲学。
- Ruling: capture 季节相位 override 默认钉分点（dayFraction=0.5，warp 恒等、tint=0）— 既有 27 景 golden 逐字节不变，本 change 视觉风险归零 — 冬季雪景场景留给积雪 change 一并补。
- Ruling: 每季 3 游戏日、昼弧 0.35..0.65、温度三钢锚点（夏至正午海平面 30℃/Y=88=0℃/冬至正午海平面 <0℃）与踩雪四项交互均经用户显式选择/批准（本会话 AskUserQuestion 与计划批准）。
- 版本：协议 v36→v37；metadata v4 / engine ABI v10 / client ABI v18 / 区块 schema v9 不变；Rust 零改动。

- Ruling: 温度日内项采用公式字面值（黄昏 +5 峰/黎明 −5 谷，热滞后形态）而非旁注「正午最高、午夜最低」 — 全部硬锚点（夏至正午海平面 30℃、Y=88 恰 0℃、冬至正午 <0）与旁注公式互斥而与字面公式唯一相容（旁注公式会给夏至正午 35℃、Y=88 为 5℃，直接违反两条 MUST 锚点；若改锚点保旁注则 Y=88 连续性破坏且冬正午无法同时 <0） — 实现者 aa46 发现矛盾上报，控制会话修 design §0/§3 与 spec 第三条 Requirement 措辞后放行。

- 2.1 共享温度公式：提交 `aae5a502`（基线 3aad2e57）+ 产物修正 `28bde596`。验证：`go test ./packages/shared/core -race -count=1` ok（7 个温度测试全 PASS）。评审 APPROVE：评审者以 -overlay 独立参考实现全网格比对（约 235 万点）最大偏差 1 ulp（float32 收窄）、三钢锚点位级成立（Y=88 返回 0x0 位级精确、`<=` 判雪边界安全）、常量镜像注释精确到 Rust `SEA_LEVEL_Y=64`、测试期望全部手算独立。实现者上报 design/spec 日内旁注与公式互斥（硬锚点唯一钉死字面公式：黄昏峰/黎明谷），控制会话修产物后放行。

- 3.1 协议 v37：提交 `d0b320e0`（基线 28bde596）。验证：`go test ./packages/shared/network/... -race -count=1` 4 包 ok、`go test ./packages/audit -count=1` ok、两侧握手/cmd 版本钉测试 ok。评审 APPROVE：v36 及更早字段序列逐字节不变（golden 4 条既有条目仅尾部 +3 零字节）、Season 越界三处拒绝与 v36 WeatherKind 同模式（解码经 `validateServerWirePacket` 覆盖）、golden v37 新条目 `01 02 80 f8` 正确、grep 无版本 36 残端、tcp transcript 夹具锁两传输逐字段一致。Ruling: `openspec/config.yaml` 版本矩阵行与 AGENTS.md 协议行的单行镜像同步属 3.1 范围 — audit 的 TestBaselineVersionsMatchCode 双读两文件、无同步必红且有 v36 先例（63d0d1c3） — 禁令保护对象是 change 产物（proposal/specs/design/tasks/ledger）而非版本矩阵行。

- 4.1 服务端派生与发布：提交 `6ba61540`（基线 d0b320e0）。验证：`go test ./packages/server/sim/... ./packages/server/server -race -count=1` 全 ok（server 245s -race）、`go test ./packages/audit -count=1` ok。评审 APPROVE：跳夜 tick 上「Publish 返回值 Store 后重读」时序论证成立（settleSleepThroughNight 在 Publish 前写入、快照在 Store 后）、TemperatureAt 每玩家一次无分配放大、双路径共用 seasonSnapshotAt 无分叉、audit 穷举清单追加 seasonOffset 最小必要、parity transcript 归一化只消登录时序噪声且搬运由 publication 测试钉住。Ruling: TickResult 不出 yearPhase/effPhase 契约字段 — 当前无契约层消费方，最小化不可变契约面 — 客户端 yearPhase 由镜像 Season+SeasonProgress 重建（量化步进 ≤0.8% 亮度台阶/约 14 秒一次，可接受，记入 design §5）。Ruling: 派生位置在 Publish 之后而非 advanceWeatherClock 同位 — effPhase 必须消费跳夜结算后的显示偏移。

- 4.2 判相位消费点切换：提交 `800d08f7`（基线 6ba61540）。验证：grep 非 test 生产代码 `DisplayDayPhase(` 直呼清零、`go test ./packages/server/sim/... -race -count=1` 全 ok、`go test ./packages/audit -count=1` ok。评审 APPROVE：五个消费点全部经新组合入口 `core.EffectiveDayPhaseAt`（薄组合层非自建 warp，core 三锚点测试锁定）、entity/runtime 双源 seasonOffset 同 seed 同纯函数必同值且装配链唯一、跳夜 M=0 闭环全年 288000 tick 零违例且分点下与旧算式逐位一致、夜锚 13000→18000 的跨季节论断独立验证（eff 恒落 15428..19384 夜窗）、四个新行为测试经数值模拟回退验证必区分 warp/非 warp、作物/流体/掉落路径零相位引用。Ruling: entity `State` 自带 seasonOffset（NewState 从 seed 派生）而非 runtime 访问器注入 — State 本就持有 seed、零新增跨包接线 — 同一 core 纯函数保证与 runtime 同值。

- 5.1 客户端镜像：提交 `33071cff`（基线 800d08f7）。验证：`go test ./packages/client/client -race -count=1` ok（3.4s）、`./packages/client/cmd/mornlea/app -race -count=1` ok（69.5s）。评审 APPROVE：与 weather 镜像六位置逐处同构（Begin 拒绝/写入、和解接受、未就绪重置、纵深拒绝、访问器双返回），去重门后写入不回退、Advance/墙钟路径零引用结构性满足不外插；评审者以四个变异（删写入/去重门<=改</拒绝移写入后/app 移出冻结守卫）验证 7 测试全部抓住对应回归。Ruling: `app_startup.go` 会话复位点补三字段属 5.1 范围 — weather 同点复位先例、缺位则二次装配残留旧值。

- Ruling: `rain-noon` 场景改钉夏至正午 + `DayPhaseOffset` 相位补偿（天空/日照逐字节不变），该景 golden 按预期重录 — 分点正午雨的雪形边界 y=69.6 使其降水柱 80% 变雪，「27 景零重录」与温度形态数学互斥（实现者 5.2 上报，design §5 原「分点+正午 15℃ 仍为雨」为算术错误，海平面实际 7℃、y=70 已 −0.5℃）；夏至正午温度边界 y=84.8，降水主体为雨、柱顶雪尘为温度梯度真实表现 — 26/27 景零差异证明分点恒等锚对其余场景生效，新增 `weather-camera-showcase` delta 同步场景 Requirement 语义（「雪线下无雪」依据已随形态判定温度化失效）。

## 任务进度

- 1.1+1.2 季节推进与昼长 warp：提交 `6026e6b3` + 加固提交 `3aad2e57`（基线 60280cef）。验证：`go test ./packages/shared/core -race -count=1` ok（1.4s/1.9s）、gofmt/vet 干净。评审 APPROVE（8 项全 PASS）：数值锚点独立复算吻合（冬至 8400/夏至 15600/分点 12000、SeasonProgress 值域恰 [0,255]）、uint32 中间量无溢出、「取偶效应」微抖经独立重写差分扫描证实为真实固有量化效应（全年 arc 变化 8128 次均 ±2、断言上界 5 恰紧非掩盖）、EffectiveMorningOffset 五昼弧×24000 M×5 worldTime 差分零失配、splitmix64 与 server 侧逐行同语义。加固：盐值 golden 锚（seed 0→158435、42→14818、MinInt64→186548）+ 连续性注释更正（实现者以实测反例纠正评审「前半段 ≤−1」说法：黑夜支路前半段亦可 −2，注释写入两处均至多 −2 的可证伪版本，断言 |Δ|≤5 未动）。Ruling: 注释按实证版本而非评审口述 — 注释必须可被代码证伪检验 — 评审的推导只在冬至邻域成立。
