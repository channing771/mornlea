## Context

动机见 `proposal.md` 的 Why，行为契约见本 change 的两个 delta spec。当前分支已完成 Pixel Perfection 生存 HUD：`internal/render/hud` 在 Go CPU 半部生成固定上限的 quad/glyph 列表，以 48-byte instance 编码上传给 Rust 客户端的既有 HUD pass；生命与氧气使用十个 resolved slot，快捷栏、状态行、采掘和聊天已经共享严格 framebuffer 边界。分支 fork 之后，main 合入 `authoritative-hunger`，增加了权威饥饿、两列程序化鸡腿、20-quad 双层饥饿条、协议 v24、玩家 schema v7 与 benchmark scenario v19。

两边分别修改了 HUD atlas、布局、容量、app 接线、capture golden 和长期基线文档。main 对饥饿状态、网络、存档和版本契约具有权威性；本 change 对最终 Pixel Perfection HUD 组合具有权威性。后续实现必须做正常 merge commit 和逐处语义整合，不能把任一侧当成可整文件丢弃的版本。

## Goals / Non-Goals

**Goals:**

- 保留 main 的 authoritative hunger 行为、协议 v24、玩家 schema v7 与 scenario v19，同时把饥饿纳入本 change 的最终 HUD 构图。
- 让生命/饥饿主状态行精确锚定快捷栏左右边缘，并让耗损氧气沿饥饿右边缘向快捷栏外侧堆叠；两行与采掘、聊天共享已经证明严格界内的响应式几何。
- 复用现有程序化 atlas、quad、glyph、上传缓冲和 HUD pass，保持 API/ABI、instance 字节布局和稳定态资源行为。
- 在合并后的 Pixel Perfection + hunger 基线上重新生成并逐张验收全部 15 个正式 capture golden。

**Non-Goals:**

- 不重做 main 的饥饿模拟、进食、网络或存档迁移，不改变服务端权威与客户端预测边界。
- 不重绘或重排打开态的 36 格背包、十行合成、箱子和熔炉，不改变 `InventorySlotAt` 命中语义。
- 不增加 registry/theme abstraction、PNG UI 源资产、shader、GPU pass、上传格式、配置项、产品测试 API 或第三方依赖。

## Decisions

### 1. 用正常语义 merge 分配所有权

后续对已核对的 `origin/main` 发起 `--no-ff --no-commit` 正常 merge，先保持 merge 未提交；main 保留 authoritative hunger 的状态机、协议、存档、app 权威值接线、scenario v19 与 benchmark 迁移，本 change 保留 Pixel Perfection 的快捷栏、resolved-slot 生命/氧气、响应式状态行、聊天边界和 capture 场景。清点全部冲突后，每处按语义组合两边结果；在未提交 merge 中完成计划内 TDD、聚焦验证与 golden 重生成，只有 index 已完全解冲突且必需的提交前门禁通过后才创建正常 merge commit。

否决 rebase + force-push：PR 已有可审查历史，改写历史没有产品收益。否决对冲突文件或 PNG 选择 whole-file ours/theirs：两边在同一文件中各自拥有承重行为，这会静默丢失其中一侧。

### 2. HUD 图集使用七个字面程序化列

物品列之前的固定列顺序为：空心、半心、满心、空气泡、满气泡、空鸡腿、满鸡腿；物品列紧随其后。复用 main 的 `paintHotbarDrumstick` 与当前分支的 heart/bubble cell，不把 HUD 图标移入 `internal/assets`，也不引入 registry/theme abstraction 或 PNG UI 源。

七列字面顺序使既有逐像素 atlas 测试可以直接覆盖偏移后的全部可放置物品列。通用 sprite 注册表在这里只有一个消费者且不能减少状态空间，因此不需要存在。

### 3. 共享快捷栏边界决定主行与向外氧气行

health 与 hunger 组成稳定主状态行。两条各沿用十个 16px 图标和九个 1px 间隔，设计宽度 `169px`；health 的 X 直接取 `hotbarRowBounds` 返回的左边缘，hunger 的 X 由同一快捷栏右边缘减去 `169px * scale` 得到。中间空间是快捷栏实际宽度扣除两条状态宽度后的自然余量，不增加栏间 gap 或主状态组 width 常量，也不再为状态行单独扩大宽度缩放边界。用户所称的“coordinated 350px”只作为首批 capture 视觉反馈的描述，不进入布局常量或自动化判据；机器契约始终是两条各 `169px` 且精确锚定快捷栏两端。

depleted oxygen 继续是 `169px`，右边缘与 hunger 完全相同。设 `statusRowStep = healthHeartSize + statusBarGap`。关闭态主行 Y 仍为 `hotbarY - statusRowStep*scale`，oxygen Y 为 `primaryY - statusRowStep*scale`；mining Y 改为 `oxygenY - (miningBarGap + miningBarHeight)*scale`，聊天也锚定完整两行栈顶。打开态主行 Y 仍为 `hotbarY + (hotbarSlotSize + statusBarGap)*scale`，oxygen Y 为 `primaryY + statusRowStep*scale`，两者始终从快捷栏向外堆叠。

实现只引入表达左右边缘和行位移所需的最小共享算术/helper，不为 health/hunger/oxygen 复制三份 scale 或 hotbar anchor。`hudScale` 的宽度继续使用既有快捷栏/打开态面板内容；`closedHUDHeight` 在单行公式上增加一个 `statusRowStep`，`openHUDHeight` 也增加恰好一个 `statusRowStep`。打开态 `hotbarRowBounds` 的底部预留同步从 `hotbarBottomMargin` 增为 `hotbarBottomMargin + statusRowStep`，使新增的向下 oxygen 行仍保留 `statusBarGap` 底部余量，并让背包、快捷栏和配方作为同一单元移动/缩放。

### 4. 三种状态保留各自最小呈现语义

`appendHungerBar` 接收 `open`，从快捷栏右边缘向左布局；生命从快捷栏左边缘向右布局；耗损氧气与饥饿共享右边缘并按容器状态选择向上或向下的次行。main 的饥饿继续使用十个常驻空槽加最多十个覆盖填充，因此奇数值最后填充露右半边且合法最大为 20 quad。本 change 的生命和氧气继续分别使用十个 resolved slot，生命无背景，氧气满值隐藏。

三者都只消费 app 从 predictor 权威镜像传入的确认值，HUD 不推导或推进状态。满氧或未确认氧气只省略实例，不收缩两行垂直几何；动态折叠被否决，因为它会让 health/hunger、mining 或聊天在氧气显隐时跳动。聊天继续锚定在关闭态完整两行状态栈上方，并保留已经通过真实 CJK/Latin 临界尺寸验证的 framebuffer pixel `1/256` fit slack；整合不得恢复分散 anchor 或削弱严格界内契约。

### 5. 固定容量继承 scenario v19 并按互斥分支证明

main 的固定资源值保持 `267` quad、`700` glyph、glyph offset `13312` bytes、总容量 `46912` bytes；instance 保持 48 bytes，固定区间保持 256-byte 对齐。当前分支已证明的关闭/打开最坏值 `76/245` 分别加入饥饿最大 20 quad 后恰好为 `96/265`，两者都在 267 内。测试继续分别构造互斥分支，不把两者相加，也不改为动态增长。

稳定态只更新固定上传资源的实际实例前缀，不创建每帧动态 GPU 资源。scenario v19 已由 authoritative-hunger 建立，本 change 不再升 benchmark 版本。

### 6. capture 夹具在既有恢复边界中加入饥饿

当前分支的 `captureHUDFixture` 已保存原 predictor 指针与 mining overlay，并通过幂等 closure 恢复。只需让夹具构造的新 predictor 同时钉住 hunger：`hud-survival-feedback` 与 `inventory-crafting` 都使用 health `5`、oxygen `core.MaxOxygenTicks / 3`、hunger `9`，前者另保留磨损工具与 blocked mining `4/9`。恢复 predictor 指针自然同时恢复 hunger，不新增产品测试 API 或每帧 capture 分支。

`hud-hotbar-health` 使用十个满心、满饥饿与隐藏的满氧，并验证两条主状态从快捷栏左右边缘相向排列、没有空的水平氧气栏。`hud-survival-feedback` 验证 oxygen 沿 hunger 右边缘向上堆叠；`inventory-crafting` 验证 oxygen 沿同一右边缘向下堆叠且避开全部命中格/配方。完整场景顺序仍为既有 15 项，`water-surface-slope`、倒数第二的 `far-horizon` 与唯一末场景 `water-underwater` 不动。

### 7. golden 与长期文档只做语义整合

PNG 冲突不选择 ours/theirs。文本/source 冲突编译并通过聚焦测试后，在合并后的 Pixel Perfection + hunger 基线上重新生成全部 15 张正式 golden，并逐张人工复核；双阈值、LOD 近环 control 和场景尾序保持不变。

Minecraft 官方页面 `https://www.minecraft.net/en-us/article/health-minecraft` 只用于核对“生命/饥饿分居快捷栏两端、气泡堆叠在饥饿外侧”的构图关系。七个 HUD cell 继续使用本项目已有的原创程序化 painter；不得下载、导入、描摹或复制 Mojang 像素。

`AGENTS.md`、`CLAUDE.md` 与 `docs/notes/progress.md` 按能力语义合并，`AGENTS.md`/`CLAUDE.md` 最终逐字节相同。长期基线必须陈述协议 v24、玩家 schema v7、benchmark scenario v19；engine ABI v6、client ABI v7、区块 schema v9、世界 metadata v2 与 `companions.ai` schema v4 保持不变。

## Risks / Trade-offs

- [第二状态行未进入共享高度或打开态底部预留，窄/矮 framebuffer 越界] → open/closed 高度都增加一个 `statusRowStep`，打开态 bottom reservation 同步增加同值；覆盖正尺寸、零尺寸、两种容器状态并遍历全部 quad/glyph 严格界内。
- [满氧隐藏后主行、mining/chat 或三份 anchor 跳动] → health/hunger 始终只从同一 `hotbarRowBounds` 取左右边缘，完整两行几何始终保留；成对比较满氧/耗氧帧的主行逐实例坐标、scale 与周边锚点。
- [打开态向下 oxygen 与 36 个命中格或 10 行配方相交] → 将 bottom reservation、`openHUDHeight` 和 `InventorySlotAt` 共用同一 scale/快捷栏位移，穷举全部格与配方矩形验证不相交。
- [atlas 合并错位使物品或鸡腿采样错误] → 锁定七列字面顺序、二值 alpha、确定性与全部可放置物品列逐像素等价。
- [容量仍沿用历史 v18 数字或把互斥分支相加] → 精确见证 `96/265` 与 `267/700/13312/46912`，并运行 tasks 6.11 写明的 scenario-v19 producer、HUD fixed-layout 与 `18:19` perfcheck 聚焦命令。
- [capture 临时 hunger 污染后续场景] → 通过既有 predictor 指针恢复覆盖成功、错误和重复 restore，顺序测试检查跨场景隔离。
- [二进制冲突掩盖主分支或当前分支视觉语义] → 禁止 ours/theirs，合并后统一重生成并人工验收 15 张图，不调整阈值。

## Migration Plan

这是对两个已实现分支的客户端呈现整合，不创建新的线上迁移。未提交 merge 的第一批 capture 暴露了三栏/531px 构图缺陷，用户批准改为快捷栏边缘主行与向外 oxygen 次行；先在 planning-only 阶段同步五份 OpenSpec、实现计划和 ledger，经 controller 检查后才恢复 `$openspec-apply-change`。恢复后按新的布局 RED、最小几何 GREEN、focused 验证与全部 15 张 golden 重生成继续同一个未提交 merge；index 完全解冲突且提交前门禁通过后才创建正常 merge commit，再执行独立 SPEC/QUALITY 双裁决；finding 最多五轮追加修复与复审，最后做整分支终审并正常 push 到现有 PR。

最终版本继承 main：协议 v24、玩家 schema v7、benchmark scenario v19；engine ABI v6、client ABI v7、区块 schema v9、世界 metadata v2、`companions.ai` schema v4 与配置格式不变。回退整合提交即可恢复到 merge 前两条历史，不需要转换用户数据；不得在没有用户明确批准时归档 change。
