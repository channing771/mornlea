## 1. 雪层方块族

- [x] 1.1 失败测试先行：四档方块 ID/谓词/属性（无碰撞、透明、不可放置、徒手采掘无掉落、容量满也成功清块）与客户端材质层/`BlockTopRaw` 1..4（呈现高度 2/16..5/16）/mesh 快照守护（`packages/shared/core`、`packages/shared/physics`、`packages/client/assets`、`packages/client/mesh`；`go test ./packages/shared/core ./packages/shared/physics ./packages/client/assets ./packages/client/mesh -race -count=1`）

## 2. 积雪与消融

- [x] 2.1 失败测试先行：随机 tick 积雪（雪形态降水+白名单露天顶格逐档升 4 上限、悬挑/水下/夏季低地不积）与消融（>融点逐档降、回差区间稳定）；runtime 季节温度快照接线进环境推进（`packages/server/sim/realm`、`sim/runtime`；`go test ./packages/server/sim/... -race -count=1`，含未加载 chunk 不写与预算有界断言）

## 3. 踩雪四项交互

- [x] 3.1 失败测试先行：脚印（落足位移 ≥0.6 格降档、1 档踩碎、静止不削减、玩家与被动牛同机制、每实体每 tick ≤1 格写）（`packages/server/sim/entity`、`sim/runtime`；`go test ./packages/server/sim/... -race -count=1`）
- [x] 3.2 失败测试先行：≥3 档减速 ×0.7 共享物理落足采样、≤2 档不减速、权威/预测同表（`packages/shared/physics`；`go test ./packages/shared/physics -race -count=1` 与 `./packages/client/client` 预测一致性测试）
- [x] 3.3 失败测试先行：踩雪音效限频与踢雪粒子共享 256 预算、非雪面不触发（晴天残雪照常）（`packages/client/audio`、`packages/client/cmd/mornlea/app`；`go test ./packages/client/... -race -count=1 -run 'Snow|Cue|Particle'`）

## 4. capture 场景与收尾门禁

- [ ] 4.1 失败测试先行：`snow-cover` 场景（冬季钉+雪形态+预铺 4 档雪层、清单 27→28 插 `camera-third-front` 后）与官方清单守护更新；`SCENES=snow-cover make visual-update` 首录新 golden 后 `make visual-check` 28 景全绿（旧 27 景零差异）（`packages/client/cmd/mornlea/capture`；`go test ./packages/client/cmd/mornlea/capture -race -count=1`）
- [ ] 4.2 全量验证与归档就绪（`gofmt -l` 无输出、六模块 `go vet` 与 `make dev-check`、`make test-race`、`go test ./packages/audit -count=1`、`make visual-check` 28 景、`openspec validate --all --strict --no-interactive`；记录 benchmark 数值只记录不改退出态）
