# 原创物品图标来源

本目录的物品图标由 Mornlea 项目代码原创生成，不含 Mojang 或其他外部游戏资源。
透明轮廓图的形状与配色定义在 `item_icons.go`，构造时写入 atlas 末尾追加层；
生牛肉与熟牛肉复用本项目已有的合法材质层（程序化回退为原创，默认包文件的 CC0
来源见 `packs/pastelcraft/ATTRIBUTION.md`）。可放置完整方块不复制一套固定图片，
而是在 `Registry` 装配及材质包原子替换后，从当前顶面与侧面材质生成 16×16 等距
立方体，所以合法材质覆盖会同步反映到栏位图标。

`Registry.ItemIconRGBA` 返回注册表持有的只读 16×16 RGBA 缓存，调用方不得修改；
`ItemIconLayer` 返回透明轮廓在世界 atlas 中的 `uint32` 层号。应用启动时把每个注册
物品的 RGBA 编码为一次 PNG data URI，HUD、背包、容器与配方只复用该目录；世界
中的非方块掉落薄片直接采样相同 atlas 层。

设置 `MORNLEA_ITEM_ICON_SHEET=/absolute/path.png` 后运行
`go test ./packages/client/assets -run '^TestItemIconContactSheet$' -count=1 -v`
可生成带物品编号与中文名称的奶油底全量 contact sheet。该文件是人工目检产物，
不作为仓库 golden 提交。
