# 设计：奶油默认包与破坏粒子

## Context

见 proposal（动机与范围）。现状约束：`packages/client/assets` 以 `textureBindings`（逻辑名→layer）+ `applyPack`（16x16 PNG 逐槽覆盖）+ `procedural.go`（全量回退）三层结构成材质；内嵌默认包是 `packs/pixel_perfection`（CC BY-SA 4.0，ATTRIBUTION + PROVENANCE +逐文件 sha256 pin）。掉落物呈现走 `packages/client/render` 的 avatar pass（`buildItemDropParts`，颜色走 `ItemColor`，相位由 tick + ID 确定性推导）。层号冻结纪律：植物 31..54、火把 59、床 60..67、短草 68、裂纹 69..78 一律不动——本次不增删任何 layer，只换像素内容。

## Goals / Non-Goals

- Goals：默认外观整体换为 Pastelcraft 奶油风（含泥土系）；破坏后有确定性同色碎块 burst；来源 pin 纪律与 Pixel Perfection 同级。
- Non-Goals：pack format 变更、协议/存档/ABI 版本变更、Rust 改动、服务端逻辑改动、frontend 改动。

## Decisions

1. **Vendor 整包目录而非混合覆盖**：新建 `packs/pastelcraft/`（`pack.json` + `textures/` + ATTRIBUTION + PROVENANCE + LICENSE），`default_pack.go` 只改 embed 路径一行，删除 `packs/pixel_perfection/`。理由：单包即真相，回退 = revert 单提交；混合包会让两种画风的不一致永久留在默认外观里。被否决：混合包（见 proposal 非目标）。
2. **来源 pin 用"版本文件 + sha256"**：Pastelcraft 经 Modrinth 分发、无 git 可 pin，沿用牛肉图标先例（`source_url` + 版本 + sha256/文件），写入 PROVENANCE.json；ATTRIBUTION 写作者与 MIT 链接。门禁：下载后先验 ZIP 内许可声明与页面一致，不一致则停下汇报（tasks 首项即此门禁）。
3. **槽位映射按名直译**：`block/dirt.png`→`dirt`、`grass_block_top`→`grass_top`、`grass_block_side`→`grass_side`、`farmland`→`farmland_dry`（干湿两态如只有一张则湿态沿用程序化或同图，映射表在实现时对照 ZIP 落定）；无对应物槽位留空→程序化回退自动生效，零代码。
4. **粒子纯客户端派生、复用 avatar pass**：新 `ItemDrop` ID 首次出现即 burst；8 粒初速由 ID 散列取上半球 8 方向（沿用 crack.go 的固定盐整数散列习惯）；年龄= tick − 首次 tick，20 tick 消失；尺寸 0.09→0；颜色=`ItemColor`。被否决：服务端权威事件（要升协议版本）、新 Rust pass（跨语言成本）。
5. **跟踪表定界 16、实例上限 64**：环形淘汰最老 burst；编码在现有 frame 预算内，满足热路径无无界工作纪律。
6. **掉落物下落纯客户端派生**（速率部分被 §7 取代）：app 层读镜像向下定界 16 格找首个不透明方块顶面为支撑高度（无数据/超深按生成高度），支撑计算与地板钳制语义沿用；下落积分见 §7。服务端权威位置、拾取判定、协议一律不动。粒子 Y 以同一支撑高度为地板钳制（防穿地），burst 原点仍是破坏方块位置（与下落解耦）。
7. **粒子原点铺展与掉落重力**：8 粒初始位置按 ID 散列铺展到原 1 格方块体积内（破裂首帧即在原方块处可见，不藏进 0.25 掉落体内）；掉落改与角色同形的重力积分（`v += g·dt`、`dt=50ms` 半隐式欧拉、`g`/终端速度取客户端生效 tunables 并显式传参进纯函数，着陆钳制）， tunables 说明见 `tunable-constants`（本地生效值，服务端各持一份，呈现侧只取形状一致）。

## Risks / Trade-offs

- [Pastelcraft 实际许可与页面不符] → tasks 首项门禁，不入库并汇报。
- [ZIP 内缺泥土系某槽位] → 该槽位留空走程序化回退，不阻塞；映射表如实记录。
- [27 张 golden 全变] → `make visual-update` 后必须逐图人工确认（capture 纪律），误冻基线比不冻更糟。
- [粒子与掉落物本体视觉重叠] → 粒子初速上半球 + 本体在 0.5 高度浮动，首帧即分离；capture 抓帧是确定性的，重叠与否可直接看图。
- [无掉落物的移除无粒子] → 已接受（选型时确认），不覆盖。

## Migration Plan

- 落地：单分支单提交（assets + render + goldens + 文档），经正常 PR 门禁合入；用户 override 包不受影响（槽位名不变）。
- 回退：整体 revert 该提交；atlas 层数/尺寸/语义全程不变，无数据迁移。

## 演示产物（motion，不进门禁）

- `testdata/visual-golden/motion/break-burst.gif` 是给人眼验证的演示产物：capture 包内独立入口（不加入 `captureScenes`、不进 `visual-check` 比对），完整采掘生命周期 45 帧、标准库 `image/gif` 编码。
- 时间线（帧号 = 合成 tick 偏移）：0–4 方块静置无采掘；5–24 采掘爬坡（`ProgressTicks` 0→`RequiredTicks`，裂纹阶段 0→9 扫完）；25 破坏同帧三件事（镜像目标置空 + 采掘 overlay inactive + 注入泥土掉落，裂纹不得残留）；25–44 粒子 20 tick + 掉落留存。机位与裂纹场景同姿势（目标砖 `(0,3,-3)`），泥土目标就用奶油 dirt。
- 不做逐帧 GIF 比对门禁：GIF 256 色量化 + 时间维度做门禁又脆又抖，确定性已由 §3 单元测试钉住；README motion 索引节明示"生成入口 + 人工确认，不参与比对"，`world/` 27 张 PNG 纪律原样不动。

## Open Questions

无（Pastelcraft ZIP 内文件清单在实现任务中对照落定，不改变 specs 与分工）。
