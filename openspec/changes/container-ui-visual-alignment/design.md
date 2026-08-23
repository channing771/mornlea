## Context

当前 `internal/render/hud` 已在一个固定 HUD pass 中绘制三种互斥打开态：普通背包/十条合成配方、玩家背包/熔炉三格与两条进度、玩家背包/箱子 27 格。`InventorySlotAt`、`FurnaceSlotAt`、`ChestSlotAt` 分别共享绘制几何并返回 36/39/63 个统一索引；`cmd/mornlea` 在第二次栏位点击时才发送现有整堆移动命令，确认前不改客户端镜像。打开态合法最大为 265 quad，scenario v19 固定缓冲仍提供 267 quad、700 glyph、13312-byte glyph offset 和 46912 bytes 总容量。

当前容器主要由纯色 panel/slot/bar quad 组成，只有 `inventory-crafting` 有正式 capture。行为契约见本 change 的两个 delta spec；本设计只决定如何以最少新状态完成视觉换肤和两场景覆盖。

## Goals / Non-Goals

**Goals:**

- 用同一套原创程序化像素框、凹槽、标题和来源轮廓统一三类容器，熔炉进度补充火焰/箭头图示。
- 保持 36/39/63 统一栏位、十条 UI 配方、两次点击整堆移动和服务端权威语义逐项不变。
- 复用现有 atlas/layout/pass，在固定 267/700/13312/46912 资源内完成。
- 以 `furnace-container`、`chest-container` 把正式无窗口场景从 15 增为 17，并保留既有尾序。

**Non-Goals:**

- 不改变任何栏位或按钮坐标、命中函数、移动/合成命令、镜像生命周期、状态 HUD 或容器内容。
- 不新增 PNG UI 源、Mojang 像素、字体、shader、GPU pass、动态资源、配置、依赖或主题/registry 抽象。
- 不推进协议、存档、benchmark scenario 或 ABI，也不修改服务端、模拟或存储。

## Decisions

### 1. 直接扩展既有程序化 HUD atlas

在现有 hotbar atlas 中追加当前产品实际需要的栏位凹槽 cell 与三个程序化标题 cell（背包/合成、熔炉、箱子）。标题像素在 atlas 构建时生成，每个 overlay 只发一个带 UV 的 quad，不走 700 glyph 流；凹槽由现有 slot quad 采样同一个 cell。原创外框、来源轮廓与按钮强调继续用已有 panel/source/button quad 原位换颜色或 UV，不引入九宫格组件、sprite registry 或主题对象。

标题只有三个固定消费者，通用文本烘焙或可配置主题不会减少当前代码和状态，按 PONYTAIL 明确不做。二进制 PNG 被否决，因为本仓禁止 Mojang 版权资源，而这些低分辨率形状可由既有程序化 atlas 确定性生成并逐像素测试。

### 2. 三个 overlay 只增加一个标题 quad

`appendRecipeRows`、`appendFurnaceRow`、`appendChestGrid` 各在自己的 panel 内追加一个标题 quad。现有 panel、36 个玩家 slot、三格熔炉/27 格箱子、十行配方按钮和两组 furnace bar quad 均不增删、不移动，只原位替换 UV/颜色。熔炉火焰与箭头使用既有两组 bar 的底/填充 quad 表达空、部分与完成状态，不另建进度组件。

当前最大 overlay 是 91-quad recipe 组成，追加标题后 `maxOverlayQuads` 为 92，打开态合法最大从 265 变为 266；仍低于固定 267。标题零 glyph，所以 700 glyph、13312 offset、46912 bytes、48-byte instance 和 256-byte 对齐全部不变。稳定态仍只更新固定缓冲的实例前缀。

### 3. 绘制、命中和命令继续共用现有几何与权威链路

`inventorySlotOrigin`、`recipeSlotOrigin`、`chestSlotOrigin` 继续是绘制与 `InventorySlotAt`/`FurnaceSlotAt`/`ChestSlotAt` 的唯一几何来源。标题和装饰位于非交互 panel 空间，来源轮廓只读取既有 `inventorySource`，不得进入命中判断。用换肤前后边界表和全部 36/39/63 中心点锁住返回值，十个 `RecipeButtonAt` 区域同样不变。

`cmd/mornlea.clickInventorySlot` 继续在第一次有效点击只记录来源，第二次才按当前打开容器发送一个既有移动命令并清空来源；普通背包移动、熔炉 `MoveContainerStack`、箱子 `MoveContainerStack` 均等待服务端确认，不本地改物品。渲染层继续只消费已确认镜像，不依赖 network 类型。

### 4. 两个新场景紧随现有容器场景

在 `inventory-crafting` 后依次插入 `furnace-container` 和 `chest-container`。三者相邻便于逐图比较同一像素语言；每个新场景仍显式重置 remote/panel/mining/damage、互斥的 furnace/chest 镜像、inventory、相机和世界时间，不依赖前一个场景。熔炉夹具钉住三个非空格、部分 burn/smelt 值和来源；箱子夹具钉住跨三行的非空格和来源。

最终顺序恰好 17 项，后续 10 个既有场景只整体后移两个索引；`water-surface-slope`/`far-horizon`/`water-underwater` 仍是末三项，既有双阈值和两张不计数的 far-horizon diagnostic controls 不变。所有 17 张由最终实现一次重生成并逐图人工验收。

### 5. 版本和数据格式全部不动

这是纯客户端呈现与 capture 覆盖。协议 v24、玩家 schema v7、区块 schema v9、世界 metadata v2、`companions.ai` schema v4、benchmark scenario v19、engine ABI v6、client ABI v7 和配置格式全部保持原值；没有迁移。回退实现与 golden 提交即可恢复当前 15 场景皮肤，用户数据与线上消息不需转换。

## Risks / Trade-offs

- [atlas cell 偏移破坏既有 HUD/物品 UV] → 逐像素锁定新增 cell、既有七个 survival cell 与全部物品列偏移，并连续构建验证确定性。
- [标题或装饰覆盖命中区] → 保存换肤前边界样本，穷举 36/39/63 栏位与十个配方按钮，GREEN 后逐项比较相同。
- [三个 overlay 重复样式算术] → 只复用现有 atlas UV 与 append 路径；仅在两个以上调用点确有相同一行算术时才抽包内 helper。
- [标题使固定 quad 溢出] → 每个互斥 overlay 只加一个标题，精确见证最大 266/267 且 glyph/offset/总容量不变。
- [场景相互污染或尾序漂移] → 每个 Apply 显式装入完整状态，顺序测试锁死 17 个名称，`water-underwater` 保持唯一末项。
- [“像素风”滑向版权复制] → 只用原创程序化几何和现有授权材质；人工验收检查构图一致性而不导入、描摹或复制 Mojang 像素。

## Migration Plan

按 `tasks.md` 依次完成 atlas RED/GREEN、三类 overlay 与交互不变量 RED/GREEN、两个 capture 场景与 17 张 golden。每个执行任务独立提交并接受同一 fresh task reviewer 的 SPEC/QUALITY 双裁决；finding 只以追加 fix commit 修复，单任务最多五轮。最后运行全量门禁和整分支终审。回退对应实现/golden 提交即可，不涉及数据迁移；未经用户明确批准不得归档 change。
