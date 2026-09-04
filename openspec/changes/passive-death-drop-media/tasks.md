## 1. 协议原因位与版本

- [ ] 1.1 `PassiveDespawn` 追加原因位并升协议 v34→v35：`PassiveDespawnRecord` 尾部 +1 字节原因位（0=消失，1=死亡），`PassiveDespawnWireBytes` 8→9，编解码、拒绝矩阵（原因位非 0/1 整包拒绝）、字节推导 golden 与 fuzz 种子同步，`ProtocolVersion` 34→35，`AGENTS.md` 与 `openspec/config.yaml` 矩阵同步（`packages/shared/network/protocol`、`packages/shared/network/codec`、`packages/audit`；验证 `go test ./packages/shared/network/... -race -count=1` 与 `go test ./packages/audit -count=1`）
- [ ] 1.2 服务端发布携带原因位：`settlePassiveDeaths` 记录当 tick 死亡 ID 集合并经 runtime 窄委派暴露，`publishPassives` 按集合投影原因位（命中为 1 否则 0），Memory/TCP 发布序列一致（`packages/server/sim/entity`、`packages/server/sim/runtime`、`packages/server/server`；验证 `go test ./packages/server/sim/entity ./packages/server/server -race -count=1`）

## 2. 客户端死亡过渡

- [ ] 2.1 死亡保留镜像：`ApplyDespawn` 按原因位分流（0 立删，1 进 dying 表冻结位姿），最大 `ServerTick` 推进保留进度，满 20 tick 移除，保留期 state 丢弃（`packages/client/client`；验证 `go test ./packages/client/client -race -count=1`）
- [ ] 2.2 红闪侧倒渲染：死亡相位函数（`dropAnimationPhase` 同形，T0+ID 派生，禁用墙钟）+ roll/红闪合成进既有 96B 实例通道（零值逐字节一致），确定性重放测试（`packages/client/render`；验证 `go test ./packages/client/render -race -count=1`）

## 3. 纹理掉落

- [ ] 3.1 缺口审计：枚举全部可掉落物品（`BlockDrop` 全表 + 牛死亡生牛肉 + 夜行者腐肉 + 收获多产物）与已有 atlas 层的映射，产出缺口清单（只读分析，不改生产代码；缺口清单写入任务报告；若某缺口需外部像素且无法原创程序化则停下报 BLOCKED）
- [ ] 3.2 缺层追加（如 3.1 有缺口；无缺口则跳过并记录）：枚举末位追加原创程序化层（植物 31..54、火把/床/短草/裂纹/牛层不动），`PROVENANCE.json`/`ATTRIBUTION.md` 溯源，像素唯一性测试（`packages/client/assets`；验证 `go test ./packages/client/assets -race -count=1`）
- [ ] 3.3 掉落采样切换：`buildItemDropParts` 由纯色改 atlas 层采样（可放置走注册表顶面层，食物走牛肉/小麦层），零值/未知物品仍不可见，容量与确定性测试保持（`packages/client/render`；验证 `go test ./packages/client/render -race -count=1`）
- [ ] 3.4 牧场 golden 按需更新：运行 `make visual-check`；除含掉落物的 `passive-herd.png` 外全部 PNG 必须直接通过，若 `passive-herd` 因批准的掉落外观变更超阈值则经显式更新重生成并逐图复核（旧 PNG 意图不动，本景为例外需控制会话评审确认；验证 `make visual-check`）

## 4. GIF 动态基线

- [ ] 4.1 GIF 剧本 runner：tick 步进抓帧（`physics.FixedDelta`，禁用墙钟）+ 标准库 `image/gif` 编码 + 帧预算校验（≤48）+ 逐帧双阈值比对入口，与 `captureScenes` 解耦（`packages/client/cmd/mornlea/capture`；验证 `go test ./packages/client/cmd/mornlea/capture -race -count=1`）
- [ ] 4.2 四剧本基线入库：吃草前后/持麦靠近/击杀/牛肉掉落 `.gif` 存 `testdata/visual-golden/passive-death/`，比对测试解码逐帧沿用双阈值；旧 PNG 逐字节不动（同包；验证 `go test ./packages/client/cmd/mornlea/capture -race -count=1` 与 `make visual-check`）

## 5. 收尾门禁

- [ ] 5.1 `gofmt` + 六模块 `go vet` + 受影响包全量 race + `go test ./packages/audit -count=1` + `openspec validate --all --strict --no-interactive` + `make visual-check`
- [ ] 5.2 全量门禁与串行收尾：`make test-race` 全绿后 `git fetch origin && git rebase origin/feat/passive-graze-lure`（有冲突立即停下报 BLOCKED，不强解）

## 6. 用户验收打磨波

- [ ] 6.1 GIF 自适应调色板：逐基线直方图取色 + 确定性并列决胜 + 抖动，stdlib 内实现；四基线重抓，草绿/牛肉红棕保真（`packages/client/cmd/mornlea/capture`；验证 `go test ./packages/client/cmd/mornlea/capture -race -count=1` 与 `make visual-check`，GIF 逐帧复核）
- [ ] 6.2 低头角重算 + 闲时点头/看人：吻部贴草常量重算并锁定；客户端 tick 驱动 pitch 点头；服务端闲时 6 格朝人规则（`packages/client/render`、`packages/server/sim/entity`；验证两包 `-race` 全绿）
- [ ] 6.3 死亡掉落关联滞后呈现：邻域+tick 窗关联，50% 前隐藏、后 scale-in + 白闪，拾取不受影响（`packages/client/client`、`packages/client/render`；验证两包 `-race` 全绿）
- [ ] 6.4 GIF 剧本语义升级：lure 跟随重写、graze 草→泥切换（含 capture-only 写块口）、kill 时机诚实；四基线重抓并逐帧人工确认（`packages/client/cmd/mornlea/capture`；验证 capture 测试 + `make visual-check`）
