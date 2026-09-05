## 1. 来源获取与许可门禁

- [ ] 1.1 从 Modrinth 下载 Pastelcraft 版本 ZIP，核对 ZIP 内许可声明与页面 MIT 一致，记录版本与下载 URL；不一致则停下汇报（对应 design 决策 2 门禁）
- [ ] 1.2 对照 ZIP 内 `assets/minecraft/textures/block/*.png` 落定到 `textureBindings` 槽位的映射表（含泥土系：dirt/grass_top/grass_side/farmland_dry/farmland_wet/sand/gravel/clay），缺槽位如实记为程序化回退
- [ ] 1.3 写失败测试：`default_pack_test.go` 断言默认包名为 Pastelcraft 且泥土系槽位像素不再等于旧程序化/旧包字节（红）

## 2. 默认包替换实现

- [ ] 2.1 新建 `packages/client/assets/packs/pastelcraft/`（pack.json/textures/ATTRIBUTION.md/PROVENANCE.json/LICENSE），按 1.2 映射表改名入库全部 16x16 PNG（绿）
- [ ] 2.2 `default_pack.go` 切换 embed 路径到新包，删除 `packs/pixel_perfection/`，跑 `go test ./...`（client 模块）与 `go vet`（绿）
- [ ] 2.3 更新 `docs/notes/compatibility.md` 默认包记录与 `openspec/specs` 无需变更项的确认说明（`texture-pack-loading` 行为不变）

## 3. 破碎粒子实现

- [ ] 3.1 写失败测试：新掉落物触发 8 粒同色 burst、同输入逐帧一致、20 tick 消失、16/64 容量淘汰（红）
- [ ] 3.2 `packages/client/render` 实现 burst 跟踪表与 avatar pass 实例编码（复用 `ItemColor` 与 ID 散列习惯，不引入 rand/时间）（绿）
- [ ] 3.3 跑 `go test ./... -race`（client 模块）与 `packages/audit`，修到全绿

## 4. 基线与收尾门禁

- [ ] 4.1 `make visual-update` 重拍 27 张世界基线，逐图人工确认后入库，再跑 `make visual-check`
- [ ] 4.2 `gofmt`、六模块 `go vet`（或 `make dev-check` 入口）、`openspec validate --all --strict --no-interactive`
- [ ] 4.3 提交 PR，正文按模板写来源/映射/验证，CI 全绿后合入

## 5. 破碎粒子 motion 演示产物

- [ ] 5.1 写失败测试：demo 生成器在固定 tick 序列下逐帧确定性、输出 GIF 可解码且帧数符合约定（红）
- [ ] 5.2 `packages/client/cmd/mornlea/capture` 新增独立 motion 演示入口（不得加入 `captureScenes`，不得进 `visual-check` 比对）：完整采掘生命周期 45 帧（静置 5 → 采掘爬坡 20，裂纹 0→9 扫完 → 第 25 帧破坏：镜像置空 + 采掘 inactive + 注泥土掉落同帧 → 粒子 20 帧 + 掉落留存），标准库 `image/gif` 编码到 `testdata/visual-golden/motion/break-burst.gif`，并给 README 加 motion 索引节（绿）
- [ ] 5.3 跑 `go test ./... -race`（client 模块）与 `packages/audit`，controller 亲眼确认 GIF 后合入本分支

## 6. 裂纹与粒子观感优化

- [ ] 6.1 写失败测试：`crack_0..9` 退回程序化（包内无文件、层像素等于程序化基线）、粒子初速下半球向外（红）
- [ ] 6.2 删除包内 `crack_0..9.png`（PROVENANCE/ATTRIBUTION/fallback 表同步），粒子初速改下半球向外（重力与寿命不变），重拍 mining-crack 两张 golden + motion GIF 并 `visual-check` 全绿（绿）
- [ ] 6.3 跑 `go test ./... -race`（client 模块）与 `packages/audit`，controller 亲眼确认后合入本分支

## 7. 掉落物支撑下落

- [ ] 7.1 写失败测试：无支撑恒速下落、有支撑保持、着陆不停浮动旋转、超深/无数据保持生成高度、同输入逐帧一致（红）
- [ ] 7.2 app 层算支撑高度（镜像向下定界 16 格）并传入 render；render 纯函数算下落偏移（速率恒定，着陆钳制）；粒子 Y 以同一支撑高度为地板钳制；burst 原点保持破坏方块位置不动（绿）
- [ ] 7.3 跑 `go test ./... -race`（client 模块）与 `packages/audit`，重拍受影响 golden（含 motion GIF，着陆可见）并 `visual-check` 全绿，controller 亲眼确认

## 8. 粒子原点铺展与掉落重力加速

- [ ] 8.1 写失败测试：8 粒初始位置分布原方块体积内（破裂首帧可见、非同一点）、掉落逐 tick 加速（增量递增）、终端速度钳制、着陆停留（红）
- [ ] 8.2 粒子初始位置按 ID 散列铺展到原 1 格方块体积内（速度/重力/寿命不动）；掉落改重力积分（`v += g·dt`、`dt=50ms`、`g`/终端取客户端生效 tunables、纯函数显式传参），着陆钳制与浮动保留（绿）
- [ ] 8.3 跑 `go test ./... -race`（client 模块）与 `packages/audit`，重拍 motion GIF（时间线不动，着陆提前属预期）并 `visual-check` 全绿，controller 亲眼确认
