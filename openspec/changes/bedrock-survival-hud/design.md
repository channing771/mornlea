## Context

动机见 `proposal.md` 的 Why，行为契约见本 change 的两个 delta spec。当前分支已完成 Pixel Perfection 生存 HUD：`internal/render/hud` 在 Go CPU 半部生成固定上限的 quad/glyph 列表，以 48-byte instance 编码上传给 Rust 客户端的既有 HUD pass；生命与氧气使用十个 resolved slot，快捷栏、状态行、采掘和聊天已经共享严格 framebuffer 边界。分支 fork 之后，main 合入 `authoritative-hunger`，增加了权威饥饿、两列程序化鸡腿、20-quad 双层饥饿条、协议 v24、玩家 schema v7 与 benchmark scenario v19。

两边分别修改了 HUD atlas、布局、容量、app 接线、capture golden 和长期基线文档。main 对饥饿状态、网络、存档和版本契约具有权威性；本 change 对最终 Pixel Perfection HUD 组合具有权威性。后续实现必须做正常 merge commit 和逐处语义整合，不能把任一侧当成可整文件丢弃的版本。

## Goals / Non-Goals

**Goals:**

- 保留 main 的 authoritative hunger 行为、协议 v24、玩家 schema v7 与 scenario v19，同时把饥饿纳入本 change 的最终 HUD 构图。
- 让生命、耗损氧气与饥饿形成一个固定三栏状态组，并与快捷栏、采掘和聊天共享已经证明严格界内的响应式几何。
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

### 3. 一个共享状态组几何决定三栏锚点

三栏依次为 health / depleted oxygen / hunger。每栏沿用十个 16px 图标和九个 1px 间隔，设计宽度 `169px`；两处栏间距各 `12px`，整组设计宽度 `531px` 并水平居中。满氧时中心栏不发实例，但 `531px` 容器仍保留，左右栏不移动。

实现只引入满足三栏共享所需的最小几何：一个窄包内 helper，或直接复用一段共享算术，以实际更短者为准；不得为三栏各写一份 anchor。`hudScale` 的宽度取既有快捷栏/打开态面板宽度与 `531px` 状态组的较大者，高度继续使用已证明的打开/关闭约束。垂直方向不增加新行：关闭态状态组仍在快捷栏上一行，采掘在它上一行；打开态同一状态组仍在快捷栏下一行。

### 4. 三种状态保留各自最小呈现语义

`appendHungerBar` 接收 `open`，使用状态组右栏；生命使用左栏，耗损氧气使用中心栏。main 的饥饿继续使用十个常驻空槽加最多十个覆盖填充，因此奇数值最后填充露右半边且合法最大为 20 quad。本 change 的生命和氧气继续分别使用十个 resolved slot，生命无背景，氧气满值隐藏。

三者都只消费 app 从 predictor 权威镜像传入的确认值，HUD 不推导或推进状态。聊天继续锚定在关闭态状态组上方，并保留已经通过真实 CJK/Latin 临界尺寸验证的 framebuffer pixel `1/256` fit slack；整合不得恢复分散 anchor 或削弱严格界内契约。

### 5. 固定容量继承 scenario v19 并按互斥分支证明

main 的固定资源值保持 `267` quad、`700` glyph、glyph offset `13312` bytes、总容量 `46912` bytes；instance 保持 48 bytes，固定区间保持 256-byte 对齐。当前分支已证明的关闭/打开最坏值 `76/245` 分别加入饥饿最大 20 quad 后恰好为 `96/265`，两者都在 267 内。测试继续分别构造互斥分支，不把两者相加，也不改为动态增长。

稳定态只更新固定上传资源的实际实例前缀，不创建每帧动态 GPU 资源。scenario v19 已由 authoritative-hunger 建立，本 change 不再升 benchmark 版本。

### 6. capture 夹具在既有恢复边界中加入饥饿

当前分支的 `captureHUDFixture` 已保存原 predictor 指针与 mining overlay，并通过幂等 closure 恢复。只需让夹具构造的新 predictor 同时钉住 hunger：`hud-survival-feedback` 与 `inventory-crafting` 都使用 health `5`、oxygen `core.MaxOxygenTicks / 3`、hunger `9`，前者另保留磨损工具与 blocked mining `4/9`。恢复 predictor 指针自然同时恢复 hunger，不新增产品测试 API 或每帧 capture 分支。

`hud-hotbar-health` 使用十个满心、满饥饿与隐藏的满氧。完整场景顺序仍为既有 15 项，`water-surface-slope`、倒数第二的 `far-horizon` 与唯一末场景 `water-underwater` 不动。

### 7. golden 与长期文档只做语义整合

PNG 冲突不选择 ours/theirs。文本/source 冲突编译并通过聚焦测试后，在合并后的 Pixel Perfection + hunger 基线上重新生成全部 15 张正式 golden，并逐张人工复核；双阈值、LOD 近环 control 和场景尾序保持不变。

`AGENTS.md`、`CLAUDE.md` 与 `docs/notes/progress.md` 按能力语义合并，`AGENTS.md`/`CLAUDE.md` 最终逐字节相同。长期基线必须陈述协议 v24、玩家 schema v7、benchmark scenario v19；engine ABI v6、client ABI v7、区块 schema v9、世界 metadata v2 与 `companions.ai` schema v4 保持不变。

## Risks / Trade-offs

- [三栏宽度未进入共享缩放，窄 framebuffer 越界] → 以 `531px` 为统一宽度约束，覆盖正尺寸、零尺寸、open/closed 和满/耗损氧气，遍历全部 quad/glyph 严格界内。
- [满氧隐藏后左右栏跳动或三份 anchor 漂移] → 用同一共享几何返回三栏位置，并成对比较满氧/耗氧帧的 health/hunger 坐标。
- [atlas 合并错位使物品或鸡腿采样错误] → 锁定七列字面顺序、二值 alpha、确定性与全部可放置物品列逐像素等价。
- [容量仍沿用历史 v18 数字或把互斥分支相加] → 精确见证 `96/265` 与 `267/700/13312/46912`，并运行 tasks 6.11 写明的 scenario-v19 producer、HUD fixed-layout 与 `18:19` perfcheck 聚焦命令。
- [capture 临时 hunger 污染后续场景] → 通过既有 predictor 指针恢复覆盖成功、错误和重复 restore，顺序测试检查跨场景隔离。
- [二进制冲突掩盖主分支或当前分支视觉语义] → 禁止 ours/theirs，合并后统一重生成并人工验收 15 张图，不调整阈值。

## Migration Plan

这是对两个已实现分支的客户端呈现整合，不创建新的线上迁移。后续先提交本次 OpenSpec 修订，再从精确核对的 main 发起不提交的正常 merge 并列出全部冲突；按 atlas、三栏布局/容量、app/capture、长期文档、golden 的顺序以 TDD 语义整合。index 完全解冲突且提交前门禁通过后才创建正常 merge commit，再执行独立 SPEC/QUALITY 双裁决；finding 最多五轮追加修复与复审，最后做整分支终审并正常 push 到现有 PR。

最终版本继承 main：协议 v24、玩家 schema v7、benchmark scenario v19；engine ABI v6、client ABI v7、区块 schema v9、世界 metadata v2、`companions.ai` schema v4 与配置格式不变。回退整合提交即可恢复到 merge 前两条历史，不需要转换用户数据；不得在没有用户明确批准时归档 change。
