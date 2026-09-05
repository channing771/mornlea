## Why

默认材质包 Pixel Perfection 偏灰暗的写实像素风，与项目近期 UI 换肤（Tiny Glade pastel 快捷栏）不协调；用户要求换成奶油风（pastel/cream）默认外观，并让方块破坏有可见的破碎反馈（当前破坏后只有旋转掉落物本体，无碎裂粒子）。

## What Changes

- 内嵌默认材质包由 `packs/pixel_perfection`（CC BY-SA 4.0）整体替换为 Pastelcraft 来源（MIT，16x 手绘 pastel 风）的 vendor 子集；`default_pack.go` 切换 embed 路径并删除旧目录。
- 无 vanilla 对应物的槽位（`light_block`、`roof_tile`、火把/床/门/工作台/裂纹/牛/短草等程序化原创层）保留现有程序化像素做回退，材质语义（layer 编号、方块面映射、透明分类）一律不动。
- 新增方块破坏破碎粒子：客户端在镜像中发现新权威掉落物时，在原方块位置爆出 8 粒小方块，轨迹与寿命由 drop ID + tick 确定性推导，复用 avatar pass，无协议与 Rust 改动。
- 掉落物呈现增加支撑下落：无支撑时按与角色同形的重力加速下落至下方首个不透明方块顶面，着陆保留浮动旋转；纯客户端派生，服务端权威与拾取判定不动。
- 27 个世界场景 golden 基线逐图重拍并人工确认后入库（像素全变是预期内变化）。
- 另附 `testdata/visual-golden/motion/break-burst.gif` 演示产物（独立生成入口，仅人工验证，不进比对门禁）。

非目标：不改动材质包加载格式（pack format v1 不变）、不新增协议消息与版本、不改 Rust renderer/C ABI、不碰服务端权威逻辑与存档、不改 frontend UI 基线。

用户可观察结果：世界方块（重点是泥土/草方块/耕地/沙子等常用方块）呈现奶油 pastel 外观；采掘或碎裂破坏方块后，原位置有短暂的同色碎块迸发随后消失，掉落物本体行为不变。

## Capabilities

### New Capabilities

- `break-burst-particles`：方块破坏后客户端派生破碎粒子的触发、确定性轨迹、寿命与容量边界。

### Modified Capabilities

- `voxel-visual-presentation`：内嵌默认包由 Pixel Perfection 子集改为 Pastelcraft（MIT）子集；程序化回退与"无 Mojang/未授权资源"门禁不变。

## Impact

- 受影响包：`packages/client/assets`（新 pack 目录、embed 切换、ATTRIBUTION/PROVENANCE）、`packages/client/render`（粒子编码）、`testdata/visual-golden/world`（27 张重拍）。
- 兼容性：协议、存档、玩家 schema、engine/client ABI 均不动；用户目录材质 override 按槽位名继续生效（槽位名不变）。
- 并发/性能：粒子编码在帧实例预算内定界（64 实例环），热路径无无界工作；纹理 atlas 层数与尺寸不变。
