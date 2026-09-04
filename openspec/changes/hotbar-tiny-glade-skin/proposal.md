## Why

底部快捷栏沿用暖深棕整条底带加直角米棕凹槽的旧 HUD 质感，与目标 Tiny Glade 式粉彩手绘风格差距明显：参考图中工具栏是悬浮的独立粉彩方块、深可可描边、大圆角加陶土厚度，没有深色长条底带。需求方要求把 hub 栏改成图中样式且高度还原。

## What Changes

- 快捷栏九格贴条纯换肤：深色长条底带改为透明悬浮排布；每格改为独立粉彩方块（逐格不同低饱和底色、深可可描边、约 10px design 圆角、顶部浅高光加底部沉边加柔投影）。
- 选中态由 sage 内衬平铺改为暖橙赭石外框加方块抬起阴影；数量字与耐久条由暗底暖白翻为亮底深棕以保对比度。
- 空格增加淡印手绘线稿衬底（内联 SVG，零二进制资产、零网络）。
- 九格数量、物品加数量加耐久语义、桥协议、Go 加 Rust 侧、HUD 缩放、状态行加聊天加弹条全部不动。

## Capabilities

### New Capabilities

（无：换肤仍由 `survival-hud-presentation` 能力覆盖，不引入新能力。）

### Modified Capabilities

- `survival-hud-presentation`：快捷栏外观改为粉彩悬浮方块（底带透明、逐格粉彩底、墨描边、圆角）；选中格标识改为暖橙外框加抬起阴影（仍保留外扩几何，忽略颜色后可区分）；数量与耐久改为亮底深棕呈现。

## Impact

- 影响包：`packages/engine/crates/mornlea_client/frontend`（`src/tokens.css`、`src/hud/hud.css`、`src/hud/Hotbar.tsx`、新建 `src/hud/slotDoodles.tsx`、测试、visual fixture）与 `testdata/visual-golden/ui/hud-hotbar*.png`（重拍）。
- 不影响：格数（仍 9）、桥协议、Go 组装、Rust 内嵌、geometry 常量与 design 分母、状态行、聊天、弹条、字体、retroui 换肤段、协议加存档加 ABI。
- 兼容性：纯呈现变更；回退即 revert 本分支。
