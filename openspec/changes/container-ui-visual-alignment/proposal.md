## Why

背包/合成、熔炉和箱子已经共享同一套权威物品镜像、统一栏位索引与 HUD pass，但三种打开态仍主要由纯色色块组成，缺少能把面板、凹槽、标题、来源选择和熔炉流程组织成一个整体的原创像素视觉。现有 15 个无窗口场景又只覆盖 `inventory-crafting`，熔炉和箱子没有独立视觉基线，因此容器换肤无法在不触碰交互语义的前提下稳定验收。

## What Changes

- 只为背包/合成、熔炉和箱子换用同一套原创程序化像素框、栏位凹槽、标题与来源轮廓；熔炉另以原创火焰/箭头图示区分燃烧和熔炼进度。
- 保持玩家背包 `0..35` 共 36 格、熔炉统一 `0..38` 共 39 格、箱子统一 `0..62` 共 63 格的绘制与命中语义，固定 UI 配方仍恰好为 10 条。
- 保持栏位移动的两次点击整堆语义：第一次只选择来源，第二次才发送恰好一个权威移动请求；客户端不预测扣增物品，服务端继续在应用前验证全部状态。
- 在 `inventory-crafting` 之后依次新增 `furnace-container` 与 `chest-container` 两个确定性无窗口场景，使正式场景从 15 增为 17；既有其余顺序和 `water-surface-slope`、倒数第二 `far-horizon`、唯一末项 `water-underwater` 的尾序不变。

非目标：不改变栏位坐标、命中区域、配方目录、物品移动规则、权威/预测边界、容器生命周期、HUD 状态行、网络/存档/配置格式或版本；不增加二进制 UI 美术、Mojang 像素、shader、GPU pass、动态资源、第三方依赖或主题/registry 抽象。

## Capabilities

### New Capabilities

- `container-ui-presentation`: 规定三类容器的原创像素框、凹槽、标题、来源轮廓、熔炉图示，以及交互和固定资源不变量。

### Modified Capabilities

- `visual-verification`: 新增熔炉与箱子两个确定性容器场景，把完整正式场景清单固定为 17 项并保持既有尾序和双阈值。

## Impact

- 后续实现范围集中在 `internal/render/hud` 的程序化 atlas、容器 overlay/layout 与测试，以及 `cmd/mornlea` 的两个 capture 夹具、顺序测试和 17 张 golden；本规划提交不修改产品代码或 golden。
- UI 继续只消费客户端已确认的 inventory/furnace/chest 镜像，移动与合成继续只发送现有权威命令；不新增跨 goroutine 状态或阻塞工作。
- HUD 固定资源保持 scenario v19 的 267 quad、700 glyph、13312-byte glyph offset、46912-byte 总容量、48-byte instance 与 256-byte 对齐；标题不进入 glyph 流。
- 协议 v24、玩家 schema v7、区块 schema v9、世界 metadata v2、`companions.ai` schema v4、benchmark scenario v19、engine ABI v6、client ABI v7 和配置格式全部不变，无迁移。
