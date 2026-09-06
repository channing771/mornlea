## Context

第一人称 viewmodel 当前不存在：`packages/client/client/render.go` 的 frame TLV 只到 tag 10（crack），`RenderFrame` 无持物通道；手臂样式锚点是 `packages/client/render/avatar.go:178-181`（臂 `0.1×0.7×0.25`、`avatarShade(base, 0.82)`、材质头层 `+12`）；手持真相源是 `app_frame.go:61` 的已确认 `Hotbar()`，挖掘信号是 `miningOverlay` + `deriveBlockCrack`，攻击信号是 `combatFeedback` 6 帧 marker；GPU 唯一实现是 Rust `mornlea_client`，Go 不得碰 WebGPU。client ABI v16→v17 是本 change 唯一的版本互斥持有（当前无其他在途 client ABI 行）。

## Goals / Non-Goals

目标是双手同时呈现、右手三形态持物、挖掘/攻击挥动、工具六档参数，全部纯呈现、确定可重放。非目标见 proposal.md；D-11（非放置物品 sprite）不阻塞本 change——工具走程序化几何；副手交互、B-36 斧铲规则均不在本 change。

## Decisions

1. 新增 viewmodel pass 而非复用 avatar pass：avatar 实例是世界空间 96 字节/实例，viewmodel 是相机空间叠加层；混用会污染 `AVATAR_MAX_INSTANCES=450` 预算与排序语义。新 TLV tag 取下一个空闲值（11），段空时帧字节与之前一致，既有 golden 根基不动。投影机制裁决（任务 5 暴露接缝 bug，见 ledger）：Go 按本帧相机位姿把相机空间偏移烘焙为世界变换，Rust 复用既有世界 VP 绘制——零 Rust/ABI 增量，单点计算（Go 同时拥有相机与编码器）。否决“Rust 补 P×I 独立投影通道”：需新增帧字段/制服语义并另起深度处理，本闭环不付该成本；已知限制是相机贴墙或穿几何时手可能被世界裁剪，独立投影通道留给后续 change。
2. Go 侧纯函数编码 + Rust 侧只绘制：相位函数与 `AvatarSwingAngle` 同形（`(tick, 档, 触发沿)` 纯函数，无墙钟），`InstanceEncoder` 同式复用缓冲零分配；Rust 不做任何摆动推测，单帧实例恒 ≤4。超限处理裁决（任务评审 F1，见 ledger）：Rust 侧超限走整帧 `Invalid` 拒绝——与 avatar/drop/轮廓/裂纹共用的 `validate_frame` 纪律同形（`frame_streams.go` 既有注释“超限帧会被 Rust 侧整体拒绝”为该纪律的成文先例）；Go 编码侧恒 ≤4 使该分支生产不可达，拒绝是响亮失败而非静默截断。原“整段丢弃”措辞作废。
3. 持物形态只认已确认镜像：`Hotbar()` 确认值是唯一真相源，本地选择请求绝不推进形态（与 `updateItemPopup` 的确认纪律同形）；未注册物品按无持物，不 panic。
4. 工具六档先占位后填实：斧铲取镐默认值的裁决写进参数表注释，B-36 落地后只改表值、不动编码与 ABI；参数表是呈现侧常量，不进任何线上契约。
5. 左手只占位：编码保留副手字段，协议/存档零字段；副手数据结构与交互的 change 另立。
6. 跨语言常量同一 Task 改齐：ABI 17 的 header/Rust/Go 三处与一致性测试在同一任务组内完成，杜绝半截 ABI。
7. 否决“HUD 前端 DOM 画手”：双手是 3D 世界空间叠加（透视、遮挡、光照与世界一致），DOM 画不出透视持物；也否决“复用掉落物薄片画工具”：持物需要体块感，走程序化立方/长条几何。

## Pose（斜持精化，用户评审后追加）

竖直柱状双手被用户否决（“两根柱子”）。新姿态：双手自左下/右下屏角斜向入画，手臂长轴向画面中心倾斜约 20°–35°（roll 向中心 + 轻微 pitch，右手为主手更靠中心、持物位更高），臂根落在屏角之外只留前臂与持物入画。实现仍是世界烘焙（根变换多乘斜持旋转，无新通道）；角度/偏移以上线数值为初值，落点以 capture 实拍 PNG 目检为准迭代（3–5 轮内锁定，逐轮记录数值与截图结论，锁定后写死为常量 + 落点测试钉死）。

## 视觉基线（追加，用户评审后二次修订）

用户二次裁决：静态 golden 一律禁手——手臂只出现在“用手击碎方块”系列动作 GIF 中；游戏中手臂与 HUD 常显层一体，背包/菜单打开时与血条饥饿快捷栏同隐同现。

- 静态表全局禁手：静态 capture runner 在装配后置位 viewmodel 抑制（单点，注释载明用户裁决），GIF/motion runner 不置位。`hand-tool`/`hand-block`/`hand-mining`/`hand-attack` 四景撤销（任务 7 已加的需回退：场景表、README 索引、顺序测试）；world golden 恢复为本 change 前基线（抑制后逐字节一致即证明成立）。
- 生产门：`deriveViewmodelInput` 在背包/容器打开或非游戏相位时返回空（HUD 一体规则）；HUD 本体是否隐藏不在本 change 范围内，不动前端。
- passive-death 基线并入 motion：`testdata/visual-golden/passive-death` 迁入 `motion/`，像素比对测试退役（只保留生成能力），README 索引同步。理由：死亡动画是过程基线，与 motion GIF 同性质（人工审查，不进比对阈值）。
- 动作 GIF 基线保留 `hand-mining`/`hand-attack`（击碎序列，真实挥动：裂纹驱动 + capture 战斗缝注入），GIF 不进比对阈值，作为人工审查基线入库。姿态变更会改写既有含手场景的 golden，任务 5 再次重跑 `visual-update` + `visual-check`。

## Risks / Trade-offs

- client ABI 升版是破坏性面：旧动态库 + 新 bridge 在 bind 阶段硬失败，符合既有“无兼容层”纪律；回退即撤销本 change 提交，ABI 回 16。
- capture golden 必变（画面多了双手）：逐图人工复核，benchmark workload 不变故 scenario 预计不变；若场景表被迫变更则跟随升级纪律另行裁决。
- 手持方块微缩立方的材质采样复用世界 atlas 层；工具长条走纯色 + 臂层材质分支（`avatarMaterialSolid` 哨兵先例），避免引入新贴图与版权风险。
- 帧预算：viewmodel 编码是常数小量工作（≤4 实例），不进 mesher/scheduler 热路径；`validateEntityPresentationCounts` 另加 viewmodel 计数门。

## Migration Plan

tasks.md 五任务组 TDD 落地：Go 编码 → 跨语言 ABI 同步 → Rust 绘制 → app 装配 → golden 与全量门禁。每组一轮开发一轮审查，ledger 记证据与 Ruling；不推送、不合并，回退为撤销本 change 提交。

## 独占文件集

`packages/client/render/viewmodel*.go`、`packages/client/client/render.go` + ABI 常量与测试、`packages/engine/crates/mornlea_client/src/{render/viewmodel.rs,ffi.rs}` + `packages/engine/include/mornlea_client.h`、`packages/client/cmd/mornlea/app/app_viewmodel*.go` + frame 接线行、capture 场景/golden、OpenSpec change 目录。刻意不碰 `packages/server`、`packages/shared/network`、任何 schema 与 `docs/feature-backlog.md`（新行注册留给 planner）。
