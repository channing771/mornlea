## Why

第一人称视角下屏幕上没有任何身体呈现：手持什么、是否在挖掘/攻击全靠准星与裂纹猜，用户要求按《我的世界》交互逻辑补上双手——双手与第三人称手臂统一样式，右手持物并随挖掘/攻击挥动，左手本闭环占位并保留扩展。

## What Changes

- 新增第一人称 viewmodel 渲染：屏幕左右两侧呈现双手，几何/颜色/材质与第三人称手臂同源（`avatar.go` 臂尺寸 `0.1×0.7×0.25`、`avatarShade(base, 0.82)`、头层 `+12` 的臂层）。
- 右手持物三形态：空手（只有手）、手持方块（微缩立方）、手持工具/物品（扁长条程序化几何，不依赖 HUD sprite，不阻塞 D-11 也不代替 D-11）。
- 左手本闭环为空手姿态占位，编码保留副手字段，**不加**任何协议/存档字段（副手交互另行 change）。
- 挖掘动作：`miningOverlay` active 期间按裂纹进度驱动右手挥动；攻击动作：`CombatHit` marker 6 帧窗内右手一次挥动；两者都只消费既有呈现信号，不新增权威语义。
- 工具六档摆幅/节奏参数表：空手/方块/剑/镐/铲/斧；斧铲在 B-36 落地前取镐默认占位，落地后只改表不动管线。
- client ABI v16→v17：frame 新增 viewmodel TLV 段（tag 取下一个空闲值），header/Rust/Go 三端同步；无段帧与变更前逐字节一致。

## Non-Goals

- 左手持物与副手交互（格挡、双持使用）；第三人称自看；FOV/音效联动；新材质与任何第三方资源；服务端玩法规则变更。

## Capabilities

### New Capabilities

- `first-person-viewmodel`：第一人称双手呈现、右手持物三形态、挖掘/攻击挥动与工具六档参数的全部可观察行为。

### Modified Capabilities

- `rust-client-render-cutover`：frame 编解码新增 viewmodel TLV 段，client ABI v16→v17 三端同步与版本拒绝。

## Impact

- Go：`packages/client/render`（新增 viewmodel 编码）、`packages/client/client`（frame TLV + ABI 常量 + bridge 测试）、`packages/client/cmd/mornlea/app`（frame 装配派生 viewmodel 输入）。
- Rust：`packages/engine/crates/mornlea_client`（viewmodel pass、ffi 解码）、`packages/engine/include/mornlea_client.h`（ABI 常量）。
- 视觉：capture golden 更新并逐图人工复核；benchmark scenario 预计不变（工作负载未变，若场景表被迫变更则跟随既有升级纪律）。
- 协议/存档/engine ABI：全部不变。独占文件集见 design.md；刻意不碰 `packages/server`、`packages/shared/network` 与任何 schema。
