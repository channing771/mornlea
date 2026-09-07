## 1. shared/core 季节与昼长 warp

- [x] 1.1 失败测试先行：季节推进/回绕/同 seed 偏移稳定、`SeasonProgress` 量化边界（`packages/shared/core/season.go` + 测试；`go test ./packages/shared/core -race -count=1 -run 'Season'`）
- [x] 1.2 失败测试先行：`DayFractionAt`/`DayArcTicks`/`EffectiveDayPhase`（分点恒等、夏至昼弧 15600/夜弧 8400、单调连续、换季不跳变、uint 溢出边界）与 `EffectiveMorningOffset` 反解闭环；随后最小实现并扩展 `day_phase.go` 文档注释（`go test ./packages/shared/core -race -count=1 -run 'Day|Phase'`）

## 2. shared/core 温度公式

- [ ] 2.1 失败测试先行：三钢锚点（夏至正午海平面 30℃、Y=88 恰 0℃、冬至正午海平面 <0℃）、降水 −4℃、午夜谷值、clamp 上下界、`PrecipitationIsSnow` 判定；随后最小实现 `temperature.go`（`go test ./packages/shared/core -race -count=1 -run 'Temperature|Precip'`）

## 3. 协议 v37

- [ ] 3.1 失败测试先行：`PlayerState` 追加 `Season/SeasonProgress/Temperature` 的编码-解码往返、Season 越界三处拒绝、golden v37 新条目；随后最小实现 `ProtocolVersion=37` 与 codec（`go test ./packages/shared/network/... -race -count=1`）

## 4. 服务端派生与发布

- [ ] 4.1 失败测试先行：`NewEngine` 从 seed 派生 `seasonOffset` 且同 seed 稳定；`Step` 尾部派生 season/seasonProgress/effPhase 进 `TickResult`/`PlayerUpdate`；`publication` 按玩家 Y 求温度复制进 `PlayerState`（`packages/server/sim/runtime`、`sim/contract`、`server`；`go test ./packages/server/sim/... ./packages/server/server -race -count=1`）
- [ ] 4.2 判相位消费点切换：sleep（含跳夜反解 `EffectiveMorningOffset`）、hostile_spawn、hostile 灼烧、passive_spawn 全部换季节化相位并 grep 无 `DisplayDayPhase` 残端（runtime 接线除外）（`go test ./packages/server/sim/... -race -count=1`；`packages/audit` 版本基线测试同步 v37：`go test ./packages/audit -count=1`）

## 5. 客户端镜像与表现

- [ ] 5.1 失败测试先行：predictor 镜像三字段（起始校验/接受/回退拒绝/Season 越界拒绝）与访问器（`packages/client/client`；`go test ./packages/client/client -race -count=1`）
- [ ] 5.2 失败测试先行：降水形态温度化（冬低地雪/夏高山雪/分点低地雨、`WeatherSnowLineY` 注释退役为锚点）；`DayNightAt` 季节 warp（冬至昼弧跨度、分点恒等）；冬季冷色 tint 有界且分点为 0；capture 季节相位 override 钉分点（`packages/client/render`、`capture`、`cmd/mornlea/app`；`go test ./packages/client/... -race -count=1`，随后 `make visual-check` 确认 27 景零差异）

## 6. 收尾门禁

- [ ] 6.1 全量验证与归档就绪（`gofmt -l` 无输出、六模块 `go vet` 与 `make dev-check`、`make test-race`、`go test ./packages/audit -count=1`、`make visual-check` 27 景零差异、`openspec validate --all --strict --no-interactive`；记录 benchmark 数值只记录不改退出态）
