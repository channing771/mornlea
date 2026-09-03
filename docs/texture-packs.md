# Mornlea 材质包

Mornlea 图形客户端支持启动时从本地目录逐层覆盖方块材质。当前唯一支持的格式是 v1 目录：

```text
my-pack/
├── pack.json
└── textures/
    ├── stone.png
    └── wheat_7.png
```

`pack.json` 必须包含整数 `format: 1` 和非空名称：

```json
{
  "format": 1,
  "name": "My Pack"
}
```

名称会去除首尾空白，结果必须非空且不超过 128 个 UTF-8 字节。manifest 最大 4 KiB；未知字段会被忽略并记录警告。

## 材质文件

`textures/` 中的文件均可选。每个已提供的文件必须是恰好 16×16 像素、最大 64 KiB 的 PNG；客户端加载时会把 PNG 支持的颜色模型统一转换为 8-bit RGBA。文件名区分大小写。

v1 的完整稳定逻辑名如下，文件路径为 `textures/<逻辑名>.png`：

```text
stone
dirt
grass_top
grass_side
bedrock
stone_brick
coal_ore
iron_ore
furnace
iron_block
chest
light_block
leaves
glass
cobblestone
smooth_stone
sand
gravel
oak_log_side
oak_log_top
oak_planks
brick
white_wool
roof_tile
clay
snow_top
snow_side
mossy_cobblestone
water
farmland_dry
farmland_wet
wheat_0
wheat_1
wheat_2
wheat_3
wheat_4
wheat_5
wheat_6
wheat_7
short_grass
```

缺失的已知文件先回退到客户端内嵌默认材质；内嵌包也未提供时，再回退到程序化材质。未知文件不会创建新材质或改变既有映射。

`textures/short_grass.png` 是短草层的可选覆盖路径。内嵌默认包可以不提供该文件；缺失时客户端使用原创程序化短草纹理，用户包提供有效的 16×16 PNG 时则按同一层覆盖它。

材质文件的 alpha 通道有硬性要求，按它在渲染路径里的层分类（`packages/client/assets/blocks.go` 的 `isCutoutLayer`）区分：

- **非 cutout 层**（默认，即不进入 `isCutoutLayer` 集合的所有层）：每一格的 alpha 必须全图等于 255，即完全不透明。这些层走不透明 terrain pass，片段着色器会丢弃 alpha<0.5 的片段；一旦混入透明像素，整个方块面就被丢弃，出现看穿/破洞，mip 降采样后远处整块面更会整块消失。
- **cutout 层**（`leaves`、`glass`、`wheat_0`..`wheat_7`、`short_grass`）：alpha 必须为 0 或 255（二值），不允许中间 alpha。这些层走 alpha cutout，由 mip 链的 `downsampleCutout` 保住覆盖率，中间 alpha 会被 `c.a < 0.5` 的判定丢弃。

因此像 Minetest/Luanti 里那种 overlay 型纹理（比如 `grass_side` 只有顶部几行草缘不透明、其余为透明，靠与 `dirt` 用 `^` 合成后显示）**不能直接入库**。建议直接提供完整的「泥土+草缘」合成图——Mornlea 内嵌默认的 `grass_side.png` 就是这样：把草缘以 straight-alpha source-over 合成到 `dirt` 之上得到完全不透明的 16×16 图。需要 overlay 时请在打包前自行合成，客户端不会做任何像素合成。

启动加载时，显式配置的目录、manifest 或已知材质文件不可读、无效、超限或尺寸错误会直接导致客户端启动失败，不会静默忽略整个用户包。清空 `texturePackPath` 可恢复内嵌默认与程序化回退。

## 配置与限制

在客户端 JSON 配置的顶层设置 `texturePackPath`：

```json
{
  "version": 1,
  "texturePackPath": "packs/my-pack"
}
```

相对路径以配置文件所在目录为基准；绝对路径直接使用。材质包只支持目录；生效材质只在客户端启动时装载并应用，运行中修改文件不会热重载。专用服务端不读取或分发材质包。

普通本地客户端的主菜单设置页显示并保存 `texturePackPath` 原文，不会把相对路径改写成绝对路径。世界启动前保存设置时，只有原文发生变化且新值非空，客户端才会读取按配置文件所在目录解析的候选路径，并对目录、manifest 和全部已知材质执行一次全成全败的完整校验；这次读取只做校验，不应用任何候选像素。任一项失败都会保留草稿、磁盘配置和当前运行态。空值或与已保存值相同的路径不会在设置保存阶段访问用户目录。候选通过并保存后，当前 atlas 仍不热替换，新的材质包从下次启动起装载并生效。

`texturePackPath` 必须是单行字符串，不得包含 CR/LF，且最多 1024 个 UTF-8 字节。该边界同时用于配置加载和设置页；配置格式版本仍为 v1。旧 v1 配置若包含超长路径或换行文件名，需要手工缩短/清空该字段，或把目录改成单行名称后更新路径。

此格式不兼容 Minetest/Luanti 或 Minecraft Java/Bedrock 的 manifest 和目录。v1 不支持 ZIP、热重载、动画、PBR，也不允许材质或方块面的重映射。

## Pixel Perfection 许可与来源

内嵌默认材质使用经许可的 Pixel Perfection 子集。源码中的许可、署名与逐文件来源记录位于：

- `packages/client/assets/packs/pixel_perfection/LICENSE.txt`
- `packages/client/assets/packs/pixel_perfection/ATTRIBUTION.md`
- `packages/client/assets/packs/pixel_perfection/PROVENANCE.json`

macOS 客户端执行 `make build` 后，同一组文件会随发布输出复制到 `bin/third-party/pixel-perfection/`。专用 Linux 服务端发布单元不包含这些客户端 notices。
