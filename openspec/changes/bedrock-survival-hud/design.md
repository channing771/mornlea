## Context

动机见 `proposal.md` 的 Why，行为契约见本 change 的两个 delta spec。当前 `internal/render/hud` 已在 Go CPU 半部生成固定上限的 quad/glyph 列表，以 48-byte `hotbarInstance` 编码上传给 Rust 客户端的既有 HUD pass；生命、氧气、物品和采掘 overlay 已由 app 从权威镜像转换，渲染器不拥有或推进游戏状态。现有生命固定在 framebuffer 左下，氧气是其上方纯色比例条，采掘条独立锚定快捷栏，三者没有共享的响应式边界。

这次改动跨 `internal/render/hud` 与 `cmd/mornlea` capture，但不跨越 Go/Rust 渲染 ABI。设计必须保留 `InventorySlotAt` 的 36 格命中语义、关闭/打开容器的互斥布局容量、现有完整 offscreen capture 链路和视觉双阈值。

## Goals / Non-Goals

**Goals:**

- 让 `internal/render/hud` 独占生存 HUD 的视觉 token、程序化 atlas、布局和固定容量计算。
- 让快捷栏、生命、氧气和采掘反馈共享一个几何基准与缩放比例，并在关闭/打开容器两种状态下具有可测试的边界。
- 用现有 quad、glyph、atlas、上传缓冲和 HUD pass 完成全部呈现，保持 instance 字节布局和稳定态资源行为。
- 用一个有界且可恢复的 capture-only 夹具固定新 HUD 场景，不向产品状态 API 添加测试入口。

**Non-Goals:**

- 不重绘或重排打开态的 36 格背包、合成、箱子和熔炉，只为生命与氧气使用快捷栏下方的既有底部留白。
- 不增加主题/token 对象、通用 sprite registry、PNG UI 源资产、动画状态、shader、GPU pass、配置项或第三方依赖。
- 不改变服务端权威、客户端预测或 transport 边界，不让 HUD 自行推导或推进生命、氧气、耐久与采掘状态。

## Decisions

### 1. 视觉资产和 token 留在现有 HUD 包

`internal/render/hud` 继续独占所有包内尺寸、颜色、间距、atlas 列和容量常量。现有热栏 atlas 在物品列前追加原创生成的空心、半心、满心、空气泡和满气泡五个 16×16 cell；alpha 只使用 0/255，构建结果必须确定。图标通过已有 `hotbarTextureUV` 和两个窄分派 helper 取 UV，物品 layer 仍逐像素复制到偏移后的列。

选择程序化像素是因为仓库已有同类 atlas 构建路径，既不需要二进制资产，也能直接测试确定性、alpha 和列边界。否决 PNG UI 资产以避免版权与发布资源面扩大；否决通用 sprite/theme abstraction，因为本 change 只有五个固定 cell 和一套固定视觉。

### 2. 一个快捷栏几何基准服务所有生存元素

在 `layout.go` 提供一个包内 `hotbarRowBounds(width, height)`，返回快捷栏左边沿、上边沿、总宽和缩放比例。`inventorySlotOrigin`、生命/氧气锚点与采掘轨道复用该结果，避免三份居中与缩放公式漂移。

关闭容器时，生命与快捷栏左边沿对齐、氧气与右边沿对齐，两者都位于快捷栏上方；采掘轨道在状态行上方。打开容器时，生命与氧气移到快捷栏下方既有底部留白，`InventorySlotAt` 和全部可交互格子的几何计算保持不变。`hudScale` 对关闭态使用快捷栏、状态行和采掘轨道的联合宽高边界，对打开态继续兼顾既有 `openHUDHeight`；宽或高为零时上层 append 函数不产生实例。

否决为窄窗口另建布局或增加 `uiScale` 配置：同一套比例缩放足以满足边界，分支重排会扩大命中与视觉状态空间。否决修改打开态格子位置：只移动非交互状态行即可避让，改格子会破坏 `InventorySlotAt` 契约。

### 3. 生命和氧气只改变呈现，不改变数据来源

`HotbarRenderer.Prepare` 继续接收 app 已转换的 `HealthOverlay` 与 `OxygenOverlay`，只把现有 `open` 状态传给两个布局函数。生命输入先钳制到 `core.MaxHealth`，十个槽位各自直接选择完整的空心、半心或满心 cell，不追加背景或覆盖层。氧气未确认或满值时立即返回；耗损时按纯整数公式 `(value*10 + MaxOxygenTicks - 1) / MaxOxygenTicks` 计算满气泡数，十个槽位各自直接选择空或满气泡 cell。

否决裁剪满心 UV 生成半心，因为完整半心 cell 能避免采样边缘并让 atlas 单元直接可验。否决本地预测或新的 predictor 分支：权威值的现有所有权已经满足行为需求，复制状态逻辑会让单机/TCP 呈现分叉。

### 4. 采掘形状复用纯色矩形

采掘轨道和填充继续使用 `hotbarInstance` 纯色矩形，比例钳制到 1。可采中段进度追加一个随填充末端移动的固定宽度亮 cap；不可采中段进度追加固定数量、固定位置的 warning notch，使去掉 RGB 后两者仍产生不同几何序列。

否决只换颜色，因为不能满足颜色无关辨识。否决贴图、shader 分支和动画，因为少量现有矩形已能表达所需形状。两类形状必须各司其职：固定 warning notch 只表示不可采，不能替代可采状态的移动末端 cap。

### 5. 固定容量按互斥布局分别证明

`maxHotbarQuads` 的 benchmark scenario v18 契约保持精确 247，不随本 change 移动；`maxHotbarGlyphs`、glyph offset 与固定上传总容量同样保持 700、12288 bytes 与 45888 bytes。关闭分支和打开分支仍分别求合法最坏值，生命和耗氧上限各为 10 quad，故含聊天的关闭/打开合法最坏分别为 76/245，均不把互斥分支相加且较大值仍留在 v18 的 247 上限内。测试分别构造两种合法最大组合，并保留 `hotbarInstanceBytes == 48`、256-byte offset 对齐、区间不重叠和只导出实际实例前缀的既有门禁。

否决动态增长或按帧分配，因为实例数有清晰小上限，现有固定缓冲更简单且保持稳定态零动态 GPU 资源。否决为了省算术而把互斥分支相加，这会无根据扩大长期容量契约；也否决把双层空槽与覆盖实例计入容量，因为 resolved-slot 使用自带轮廓的完整 cell 可保持相同最终像素并避免静默改变 v18 workload。

### 6. capture 用临时且幂等恢复的 HUD 夹具

`cmd/mornlea` 在 `captureScene` 中增加可选的 capture-only HUD 数据。应用夹具时保存原 predictor 指针与 mining overlay，从原 predictor 读取有限物理状态，再创建一个仅钉住固定健康、氧气、相机和采掘值的新 predictor；返回的幂等 closure 在成功和所有后续错误路径恢复原状态。夹具在场景 Apply 后、收敛帧前生效，使收敛与最终帧一致，又不污染共享 application 的下一场景。

`hud-survival-feedback` 紧随 `hud-hotbar-health`，固定低血、约三分之一氧气、磨损工具和不可采目标中段进度。全部 15 个场景继续走同一 offscreen 完整渲染链路和既有双阈值；`far-horizon` 保持倒数第二，`water-underwater` 保持唯一末场景。

否决给 `Predictor` 增加“只改 HUD”产品 API，也否决在每帧热路径增加 capture 条件：测试夹具只属于 capture 生命周期，恢复闭包是更小且更易证明隔离的边界。

### 7. 所有权、依赖和并发边界保持不变

服务端仍是生命、氧气、物品和采掘状态的唯一权威；客户端 predictor/镜像提供只读确认值，HUD 只把输入转为实例。`internal/render/hud` 不依赖 `network` 或 `sim`，`cmd/mornlea` 只在 capture 装配层构造既有客户端状态。没有新增 goroutine、共享可变切片、I/O 或热路径阻塞工作；发送后的 network 消息不可变规则不受影响。

## Risks / Trade-offs

- [窄 framebuffer 中联合高度计算遗漏某一状态行，导致矩形越界] → 用零尺寸、多个窄尺寸、关闭/打开态和最大 overlay 的表驱动几何测试遍历全部 rectangle。
- [新增 atlas 列改变物品缩略图采样] → 测试每个 UI cell 的二值 alpha/确定性，并逐像素证明偏移后的物品列仍等于 registry 顶面且 UV 不越列。
- [固定容量算小造成 overflow，算大则静默改变 benchmark v18 workload] → 分别见证关闭 76、打开 245 和 glyph 700 的合法最大组合，并精确锁定 247 quad、12288 glyph offset、45888 总容量、编码与实际前缀门禁。
- [capture 临时 predictor 污染后续 LOD/水下场景] → restore closure 幂等并覆盖所有返回路径；顺序测试同时固定完整 15 场景、倒数第二远环和唯一末尾水下。
- [HUD 改动使多个旧 golden 合法变化] → 逐张检查全部候选，只接受能由 HUD 区域解释的差异，不调整既有双阈值；世界、实体、光照、水与 LOD 区域变化先查根因。

## Migration Plan

这是客户端呈现与视觉基线的原地替换，不需要数据或线上滚动迁移。实现按 atlas、快捷栏/采掘、生命/氧气/容量、capture/golden 的顺序提交；每组先写失败测试并完成 implementer 自证与候选提交，再以该提交生成 review package 进行独立规格与质量评审，finding 只通过追加 fix commit 修复并重新评审。最后同步长期文档、提交候选、执行 Rust、全仓 race/vet、架构、OpenSpec 和 visual 门禁，再按同样顺序完成整分支终审。

协议保持 v23，engine ABI 保持 v6，client ABI 保持 v7，benchmark scenario 保持 v18；玩家 schema v6、区块 schema v9、世界 metadata v2、`companions.ai` schema v4 和配置格式均不变。回退时按相反顺序撤销本 change 的提交及新增 golden 即可，不需要恢复或转换用户数据；归档仅在实现、视觉验收和整分支终审通过后进行。
