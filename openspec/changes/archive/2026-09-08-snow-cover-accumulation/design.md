# 设计：snow-cover-accumulation

前置依赖：`seasonal-time-temperature`（温度/雪点/融点/季节派生与协议 v37）必须先合入——本 change 在其基线上开发。

## 0. 数值与机制总表（钉在共享常量，测试锚定）

| 量 | 值 | 说明 |
|---|---|---|
| 雪层档位 | 4 个方块 `snow_layer_1..4`，呈现高度 (raw+1)/16 = 2/16..5/16（`block_top_raw` = 1..4，第 k 档 raw=k） | 档间差 1/16 清晰可辨；raw=0 为满格哨兵不可用 |
| 雪层碰撞 | 全档无碰撞 | 贴地装饰层；实体由下方承载方块支撑，脚部占据雪层格 |
| 积雪白名单地表 | 草、土、石、沙、砾石、整块雪（`SnowBlockID`）顶面 | 悬挑下/水下/非白名单不积 |
| 积雪条件 | 权威天气 ∈ {雨, 雷暴} 且该格局部温度 ≤ 雪点 0℃ | 局部温度 = `core.TemperatureAt(yearPhase, effPhase, weather, 格Y)` |
| 融雪条件 | 局部温度 > 融点 2℃ | 回差区间 [0,2]℃ 稳定不变 |
| 推进机制 | realm 随机 tick（`RandomTicksPerSection` 既有预算）；命中白名单顶格时按确定性抽选升/降档 | 预算与作物/耕地共用框架，不新增无界遍历 |
| 脚印触发 | 落足（OnGround）实体水平位移累计 ≥0.6 格经过雪层格 → 该格降 1 档（1 档→空气） | 玩家与被动牛同机制；静止不削减 |
| 减速 | 落足格雪层 ≥3 档 → 水平移动目标速度 ×0.7 | 共享物理落足采样，权威/预测同路径 |
| 音效 | 雪上移动 crunch 提示音，限频（每 ≥0.8 格位移一次） | `client/audio/cue.go` 程序化先例 |
| 粒子 | 疾跑/落地踢雪尘 ≤8 粒/帧 | 复用 precip 实例流（96B/实例同布局）与 256 总上限 |

## 1. 方块族（shared/core + assets + mesh）

- `shared/core/block.go`：`SnowLayer1BlockID..SnowLayer4BlockID` 追加在 `BlockIDMax` 前；谓词 `SnowLayerTier(id) (tier uint8, ok bool)`（0 表非雪层）与 `IsSnowLayer`。
- `block_properties.go`：emission 0、attenuation 0、opaque false、`PlaceableBlockAtFace` 拒绝（玩家不可放置——雪层只由积雪机制产生；采掘移除可用）。
- `shared/physics/types.go` `BlockCollisionBoxes`：雪层返回 0 盒。
- `packages/client/assets/blocks.go`：新材质层 `LayerSnowLayerTop/Side`（程序化雪白像素，与 `LayerSnowTop` 同族可复用噪声但独立层号追加在枚举末位）；`Material`/`Opaque`（false，两面可见）/`FaceVisible`（邻接剔除按透明处理）/`BlockTopRaw`（1..4，第 k 档 raw=k）/`Model`（普通）。
- `packages/client/mesh/registry.go`：快照自动跟随；守护测试更新（方块数、层号连续性）。
- `sim/entity/mining.go`：四档采掘掉落映射为无掉落（照短草/无掉落先例），徒手即可。

## 2. 积雪与消融（sim/realm + runtime 接线）

- 接线：`sim/runtime` 在环境推进阶段把当 tick `WeatherKind` 与季节温度快照（yearPhase/effPhase——4.1 的 `seasonSnapshot` 已有）传入 realm 环境推进（沿 Tunables/参数束或显式参数，选最小侵入形态，实现时定）。
- realm 随机 tick 命中地表格时的判定序（全部确定性，无全局随机）：
  1. 该格为白名单地表顶面（heightmap 顶，天空直射）且上方格为空气 → 局部温度 ≤ 雪点且天气有降水 → 上方格置 1 档或升 1 档（≤4）；
  2. 上方格已是雪层 → 温度 > 融点降 1 档（1 档→空气）；温度在回差区间不动；温度 ≤ 雪点且降水升档（同 1）。
- 写块走既有 realm Mutation 事务（吃草先例：每事件 ≤1 格、未加载 chunk 不写、不同步加载）。
- 持久化：新方块 ID 经既有 paletted container 自动兼容旧存档；dirty 标记与既有一致。

## 3. 四项踩雪交互

- **脚印（服务端权威）**：`engine_step` 玩家/被动牛物理推进后，对每个 OnGround 实体累计水平位移，跨过 0.6 格阈值时采样落足格：为雪层则降 1 档（Mutation，≤1 格/tick/实体）；被动牛共用同一 helper。位移累计为每实体瞬态（不落盘）。
- **减速（shared/physics）**：`physics.Step` 在积分前经 `CollisionSource` 采样落足格方块 ID，`SnowLayerTier ≥3` 时把水平目标速度乘 0.7（中文注释钉住权威/预测同路径保证一致；客户端 `MirrorCollisionSource` 同表）。不新增 `physics.Input` 字段、不动 engine ABI（Rust 积分输入不变，减速在 Go 侧目标速度层生效）。
- **音效（客户端）**：`client/audio/cue.go` 新增 `CueSnowStep` 程序化短促噪声（照既有方波/噪声合成先例）；app 在本地预测移动累计 ≥0.8 格且落足格为雪层时触发，限频。
- **粒子（客户端）**：app 帧装配在疾跑或落地事件时向 precip 实例流追加 ≤8 粒雪尘（同 96B 布局、位置为脚部附近确定性抖动、与天气粒子共享 256 上限，超限舍弃踢雪尘保天气粒子）。

## 4. capture 场景与基线

- 新场景 `snow-cover`：冬季钉（`SetCaptureSeason(Winter, 中点)`）+ 雪/雨夹具（冬季低地温度 ≤ 雪点 → 形态为雪）+ 场景夹具在镜头前地表预铺 4 档雪层（含不同档位对比区）+ 固定机位；清单插在 `camera-third-front` 后、`main-menu` 前（27→28），官方场景清单守护测试更新。
- 既有 27 景零重录（分点/夏至钉不受本 change 影响；材质层只追加不扰动）。

## 5. 数据所有权与并发

- 雪层方块状态即持久化真值（chunk）；脚印/积雪/消融全部经 realm Mutation 串行写；随机 tick 预算有界；无新权威字段、无协议变更。
- 位移累计（脚印/音效触发）为每实体瞬态，重启不恢复（脚印已在方块里，累计器只是触发器）。

## 6. 被否决的替代方案

- **单档雪层**：用户裁决 4 档渐厚（脚印=降档、消融=逐档更自然）。
- **雪层占权威天气状态（WeatherKind::Snow）**：延续 B 裁决，形态客户端派生、积雪由温度驱动。
- **8 档 1/16 步进**：`block_top_raw` 0 为满格哨兵、1/16 档不可表达且 8 档视觉差异过细，4 档（2/16..5/16）档间可辨性最好。
- **Rust 侧减速**：Rust step 只见碰撞盒不见方块 ID，减速语义属于方块判定，放 Go 共享层两侧自动一致。
- **雪层可被玩家放置**：会产生建造语义与放置面校验面，超出装饰层范围；本轮只由机制产生。

## 7. 验证方法

- `go test ./packages/shared/core -race -count=1`（方块族谓词/属性守护）
- `go test ./packages/shared/physics -race -count=1`（减速采样/预测一致）
- `go test ./packages/server/sim/... -race -count=1`（随机 tick 积雪/消融/脚印、预算）
- `go test ./packages/client/... -race -count=1`（材质/mesh 快照、音效粒子装配、capture 场景）
- `make visual-check`（28 景：新 snow-cover golden 首录 + 旧 27 景零差异）
- 收尾：`gofmt -l`、六模块 vet、`make dev-check`、`make test-race`、`openspec validate --all --strict --no-interactive`
