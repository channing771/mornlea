## Why

Mornlea 当前无天气、只有第一人称、橡树形态固定（树高仅 4-6 三档、固定四层树冠），与《我的世界》对标的晴/雨/雷暴、F5 三态视角、同种多形态树存在明显体验差距。本 change 一次补齐三块用户可观察能力。

## What Changes

- 天气：服务端新增权威天气状态机（晴/Clear、雨/Rain、雷暴/Thunder），按权威 tick 推进并按 MC 时长分布自动轮转；经玩家状态同步给客户端，世界 metadata 持久化；客户端表现雨/雪粒子（降水形态按高度相对雪线确定：雪线以上为雪、以下为雨）、天空变灰、昼夜亮度压暗、雷暴闪光。
- 视角：新增 F5 循环三态（第一人称、第三人称背面、第三人称正面），本地持久化；第三人称显示自身模型并隐藏第一人称双手，相机后拉并做不透明方块防穿墙收缩；瞄准与挖掘射线仍以眼睛为准。
- 树多样性：同种橡树拉开高度与冠形两档，并以小概率生成珍异大树（含横向分杈与球状大冠）；全部由世界种子与世界坐标哈希确定，单点与整块一致，老区块不重写。
- 非目标：天气不做群系系统、不联动钓鱼/作物/怪物生成/睡眠跳过、不做闪电伤害与点火；视角不做电影运镜平滑、不做窒息强制回第一人称；树不引入新树种与新方块。

## Capabilities

### New Capabilities

- \`authoritative-weather\`: 服务端权威天气状态、同步、持久化与客户端天气表现。
- \`camera-perspectives\`: F5 三态视角切换、自身模型显隐、相机后拉与防穿墙。

### Modified Capabilities

- \`deterministic-tree-generation\`: 在既有确定性橡树语义上扩展高度/冠形多样性与珍异大树变种。

## Impact

- 服务端：\`packages/server/sim\` 新增天气时钟（权威 tick 常数工作量），世界 metadata v3→v4（老档迁移默认晴天），协议 v35→v36（\`PlayerState\` 尾部追加 1 字节天气，越界拒绝）。
- 客户端：\`packages/client/client\` 相机模式与镜像天气消费，\`packages/client/render\` 天气粒子与 viewmodel 显隐，Rust \`mornlea_client\` 天空/相机/粒子呈现（client ABI 可能 bump，同步头文件与 bridge）。
- 世界生成：Rust \`mornlea_engine/worldgen.rs\` 橡树规则扩展，Go \`packages/shared/worldgen\` 无生产实现（仅 ABI 编解码），Go/Rust parity 测试与区块 golden 更新；只影响新生成区块，已保存区块不扫描不重写。
- 性能：天气推进常数工作量、无堆分配与 I/O；稳定天气帧不增加 draw 数量级、不触发重网格；树生成仍有界（单 cell 常数哈希，整块遍历不变）。
- 说明：本 change 按用户要求把三块独立能力合一，评审与回退粒度变粗；tasks 内按天气/视角/树分三组，每组可独立验证。
