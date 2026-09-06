## Context

第一人称 viewmodel 当前不存在：`packages/client/client/render.go` 的 frame TLV 只到 tag 10（crack），`RenderFrame` 无持物通道；手臂样式锚点是 `packages/client/render/avatar.go:178-181`（臂 `0.1×0.7×0.25`、`avatarShade(base, 0.82)`、材质头层 `+12`）；手持真相源是 `app_frame.go:61` 的已确认 `Hotbar()`，挖掘信号是 `miningOverlay` + `deriveBlockCrack`，攻击信号是 `combatFeedback` 6 帧 marker；GPU 唯一实现是 Rust `mornlea_client`，Go 不得碰 WebGPU。client ABI v16→v17 是本 change 唯一的版本互斥持有（当前无其他在途 client ABI 行）。

## Goals / Non-Goals

目标是双手同时呈现、右手三形态持物、挖掘/攻击挥动、工具六档参数，全部纯呈现、确定可重放。非目标见 proposal.md；D-11（非放置物品 sprite）不阻塞本 change——工具走程序化几何；副手交互、B-36 斧铲规则均不在本 change。

## Decisions

1. 新增 viewmodel pass 而非复用 avatar pass：avatar 实例是世界空间 96 字节/实例，viewmodel 是相机空间叠加层；混用会污染 `AVATAR_MAX_INSTANCES=450` 预算与排序语义。新 TLV tag 取下一个空闲值（11），段空时帧字节与之前一致，既有 golden 根基不动。
2. Go 侧纯函数编码 + Rust 侧只绘制：相位函数与 `AvatarSwingAngle` 同形（`(tick, 档, 触发沿)` 纯函数，无墙钟），`InstanceEncoder` 同式复用缓冲零分配；Rust 不做任何摆动推测，单帧实例恒 ≤4。超限处理裁决（任务评审 F1，见 ledger）：Rust 侧超限走整帧 `Invalid` 拒绝——与 avatar/drop/轮廓/裂纹共用的 `validate_frame` 纪律同形（`frame_streams.go` 既有注释“超限帧会被 Rust 侧整体拒绝”为该纪律的成文先例）；Go 编码侧恒 ≤4 使该分支生产不可达，拒绝是响亮失败而非静默截断。原“整段丢弃”措辞作废。
3. 持物形态只认已确认镜像：`Hotbar()` 确认值是唯一真相源，本地选择请求绝不推进形态（与 `updateItemPopup` 的确认纪律同形）；未注册物品按无持物，不 panic。
4. 工具六档先占位后填实：斧铲取镐默认值的裁决写进参数表注释，B-36 落地后只改表值、不动编码与 ABI；参数表是呈现侧常量，不进任何线上契约。
5. 左手只占位：编码保留副手字段，协议/存档零字段；副手数据结构与交互的 change 另立。
6. 跨语言常量同一 Task 改齐：ABI 17 的 header/Rust/Go 三处与一致性测试在同一任务组内完成，杜绝半截 ABI。
7. 否决“HUD 前端 DOM 画手”：双手是 3D 世界空间叠加（透视、遮挡、光照与世界一致），DOM 画不出透视持物；也否决“复用掉落物薄片画工具”：持物需要体块感，走程序化立方/长条几何。

## Risks / Trade-offs

- client ABI 升版是破坏性面：旧动态库 + 新 bridge 在 bind 阶段硬失败，符合既有“无兼容层”纪律；回退即撤销本 change 提交，ABI 回 16。
- capture golden 必变（画面多了双手）：逐图人工复核，benchmark workload 不变故 scenario 预计不变；若场景表被迫变更则跟随升级纪律另行裁决。
- 手持方块微缩立方的材质采样复用世界 atlas 层；工具长条走纯色 + 臂层材质分支（`avatarMaterialSolid` 哨兵先例），避免引入新贴图与版权风险。
- 帧预算：viewmodel 编码是常数小量工作（≤4 实例），不进 mesher/scheduler 热路径；`validateEntityPresentationCounts` 另加 viewmodel 计数门。

## Migration Plan

tasks.md 五任务组 TDD 落地：Go 编码 → 跨语言 ABI 同步 → Rust 绘制 → app 装配 → golden 与全量门禁。每组一轮开发一轮审查，ledger 记证据与 Ruling；不推送、不合并，回退为撤销本 change 提交。

## 独占文件集

`packages/client/render/viewmodel*.go`、`packages/client/client/render.go` + ABI 常量与测试、`packages/engine/crates/mornlea_client/src/{render/viewmodel.rs,ffi.rs}` + `packages/engine/include/mornlea_client.h`、`packages/client/cmd/mornlea/app/app_viewmodel*.go` + frame 接线行、capture 场景/golden、OpenSpec change 目录。刻意不碰 `packages/server`、`packages/shared/network`、任何 schema 与 `docs/feature-backlog.md`（新行注册留给 planner）。
