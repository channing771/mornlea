## Context

见 `proposal.md` 的 Why。当前 `cmd/mornlea` 在普通本地交互启动时先构造窗口、Rust renderer、材质 registry、HUD/字体和本地音频，再停留在主菜单；世界 store、Host 与登录延迟到「进入游戏」。Go 的 `menuState` 拥有菜单语义，client ABI v8 的 UI layout v1 把标题、按钮和错误行下发给 Rust，Rust 用 egui 呈现并只回传按钮 `u32` id。

这条边界无法直接承载设置表单：文本框和滑块必须把值回传 Go，裸按钮 id 既不能无损表达 UTF-8 路径，也无法证明同帧「编辑后保存」的顺序。现有配置已有 `audioVolume`、`texturePackPath` 和原子 `Config.Save`；窗口已有创建、`SetContentSize` 和尺寸快照；材质 registry 只在 application 构造时上传，音频播放器则可通过既有工厂安全替换。

## Goals / Non-Goals

**Goals:**

- 保持 Go 对 UI 业务语义和配置 I/O 的所有权，Rust 只持有 egui 焦点、光标等瞬态呈现状态。
- 用固定上限的 client ABI v9 同时支持主菜单 action、设置草稿变化和 UTF-8 文本，任何容量不足均显式失败且不丢事件。
- 保证保存动作的顺序是「校验候选 → 读取最新磁盘配置 → 原子落盘 → 更新 committed → 应用当前进程音量/窗口」，失败前缀没有部分运行时副作用。
- 保持材质加载不可变：设置阶段可以校验变化的候选目录，但不替换当前 renderer、mesher 或 HUD registry。
- 让 640×360 离屏场景覆盖完整设置页，同时保证 benchmark 零 UI 参与。

**Non-Goals:**

- 不把设置入口加入游戏内或远程 `-connect` 路径，不建设暂停语义。
- 不引入通用表单 DSL、Rust 配置读写、原生文件选择器、剪贴板、`egui-winit` 或新 crate。
- 不热重载材质，不在保存后重建 application；不扩展到任意窗口尺寸、全屏、VSync、显示器、FOV、视距或键位。
- 不改变线上协议、存档、engine ABI、benchmark scenario 或 HUD 固定上传布局。

## Decisions

### 1. 配置新增固定 `WindowSize` 枚举，路径上限在加载与 UI 共用

`internal/config` 新增字符串类型 `WindowSize` 和三个稳定值 `640x360`、`960x540`、`1280x720`，提供逻辑宽高映射；`Config.WindowSize` 为顶层字段，`Defaults` 取 `1280x720`。`Load` 对缺席字段保留默认，对显式 `null`、非字符串或未知值报错；`Save` 原样输出字符串。该字段不进入 `Fields()`，因为它不是连续数值调参，也不属于专服运行时。

设置页和配置加载共用 `MaxTexturePackPathBytes = 1024`，并在加载边界统一拒绝 CR/LF。上限按 UTF-8 字节计，既符合 ABI 与 UI 固定内存预算，也覆盖 Darwin 可用路径；超长旧值需用户缩短或清空。Darwin 虽允许目录名含换行，但该值无法无损进入单行控件，需把目录改为单行名称或清空配置；配置版本仍为 1。把限制只放在 UI 会让已加载值在设置页编码时 panic；静默截断或过滤又会在保存时改写路径，因此选择在配置加载边界显式拒绝，并在 application 构造时对直接注入的 options 再做防御验证。

被否决方案：任意宽高文本会引入比例、HUD 最小尺寸和巨大 framebuffer 问题；把窗口预设塞进 `Render` 会混淆纯渲染调参与平台窗口状态；UI 单独截断路径会导致数据丢失。

### 2. Go 使用 committed/draft 双状态并补充设置相位

`menuPhase` 增加 settings 相位；`settingsState` 保存 `committed`、`draft`、错误和提示，值只含三项暴露设置。`applicationOptions` 同时携带原始 `TexturePackPath` 与 `WindowSize`，而 registry 继续消费解析后的启动路径。进入设置页复制 committed；每个结构化 change 事件只改 draft；dirty 由两个值比较得到，不单独信任 Rust 位。

「取消更改」恢复 draft 并留在页面；「返回」/Escape 只在非 dirty 时切回主菜单，否则写提示。所有 action 仍由 Go 决定相位和副作用，Rust 不判断是否可以保存、返回或启动世界。

被否决方案：让 Rust 长期持有 draft、只在 Save 查询整份表单，会把业务状态移出 Go并需要第二条查询 ABI；用按钮 id 范围模拟加减和文本录入会造成脆弱、不可访问的伪表单。

### 3. 保存按全成全败前缀执行，运行时应用晚于落盘

保存函数依次执行：

1. 防御性验证 draft 的音量、窗口枚举、单行 UTF-8 路径和字节上限。
2. 仅当材质原文相对 committed 发生变化且非空时，按配置文件所在目录解析路径，并通过 `applicationDependencies.newRegistry` 完整构造候选 registry 后立即丢弃；空值不访问用户目录，未变化值不重复读取。该读取仍位于世界启动前菜单阶段，当前材质不会被替换。
3. 重新 `config.Load(configPath)`，只覆盖三项字段，再调用既有 `Config.Save` 原子替换。这样保存时并发发生的其他配置字段修改不会被 application 启动快照覆盖；若文件此时损坏，保存失败而不是重建默认文件。
4. 落盘成功后才更新 committed。若音量变化，通过既有 `newAudioPlayer` 创建新播放/关闭闭包，先完成替换再关闭旧播放器；零音量沿用不请求设备的无声路径。若窗口变化，调用 `SetContentSize`、`Poll` 和既有物理 framebuffer 上限收缩。材质只记录“下次启动生效”提示。

错误完整写入日志；UI 错误在 Go 侧按 UTF-8 边界截到 256 字节，避免任意路径错误触发 UI 编码 panic。音频设备不可用继续按既有无声降级，不把设备状态变成配置保存失败。

被否决方案：先应用音量/窗口再写盘会在 I/O 失败时分叉；保存整个启动时 `Config` 会覆盖用户在菜单期间对其他字段的外部编辑；材质热重载需要替换 atlas、mesher、HUD registry 和潜在在途任务，超出 D-01。

### 4. client ABI v9 下行使用两个 layout，上行使用结构化事件批

帧 TLV tag 9 保持不变。下行：

- layout v1 主菜单逐字段布局保持原样，仅「设置」的 enabled 从 0 改为 1。
- layout v2 设置页固定为：`layout u32`、`flags u32`、`audio f32`、`window u32`、`path [u32 len + bytes]`、`dirty u32`、`status [len+bytes]`、`error [len+bytes]`。音量必须有限且在 `[0,1]`，窗口枚举只接受三值，布尔只接受 0/1，路径/提示/错误均有字节上限，解码器必须消费全部输入并拒绝尾随垃圾。

上行 batch 固定为 `batch_layout u32 = 1`、`event_count u32`，随后重复 `[kind u32, payload_len u32, payload]`：

- action payload：一个 `u32 action_id`，覆盖主菜单与设置页按钮/Escape。
- settings-changed payload：`audio f32`、`window u32`、`path [u32 len + bytes]`，是一帧控件变化后的完整最终草稿。

Rust 每个 egui 帧最多发一个 settings-changed，再按控件绘制顺序追加 action，因此同帧编辑后点击保存时 change 必然在 save 前。全局输出队列固定 64 条；超过容量时当前 UI 帧返回 `CAPACITY`，不得丢最旧或最新事件。单条最大 payload 由 1024 字节路径决定，Go renderer 持有按最大 64 条推导的一块复用 scratch，不在每帧动态增长。

排空 C 出口把最后一个指针解释为 `out_written` 字节数。只有整个 batch 能装入调用方缓冲时才编码并 pop；不足返回 `CAPACITY`，out、`out_written` 和队列均不变。Go 解码器再次验证版本、计数、长度、UTF-8、数值域和尾随字节，返回 `[]client.UIEvent`。这是函数语义与事件线格式的破坏性变化，故必须 v8 → v9，不能只改 layout 常量却继续标 v8。

被否决方案：沿用 v8 的写满截断会丢最后一次文本或 Save；Rust 只回传 Save 后由 Go调用查询会增加状态所有权和另一条 ABI；无版本事件流无法在未来扩展时可靠拒绝混装。

### 5. Rust 设置页只拥有瞬态控件状态，最小尺寸使用可滚动表单

`UiFrame` 改为 layout 枚举，`UiState` 根据 layout 选择主菜单或设置页。设置页将下行值克隆到局部 widget 变量；egui 修改后立即生成完整 settings-changed 事件，下一帧由 Go 回显。Rust 只保留 egui context 内的焦点、光标、IME 等瞬态状态。

布局在 `640×360` 使用居中有界面板和纵向 `ScrollArea`：音量滑块、材质路径单行输入、三个窗口预设、错误/提示与保存/取消/返回按钮均可通过滚动访问且互不重叠。Escape 生成返回 action；文本框激活时普通字符和 IME 仍走现有手工 RawInput，路径过滤换行并按 UTF-8 字节边界阻止超限。

被否决方案：为固定 640×360 强行压缩所有控件会损害可读性；新建原生窗口或引入 `egui-winit` 违反既有窗口和依赖边界。

### 6. 窗口创建和运行期调整复用同一预设与双重钳制

`runWithDependencies` 把生效的原始设置传入 application；交互窗口创建前用 `WindowSize.Dimensions` 取逻辑宽高。Rust `ClientWindow::set_content_size` 与创建路径共用当前 monitor 工作区钳制，Go 随后复用 `fitFramebuffer` 确保 Retina 等高缩放下物理 framebuffer 不超过 `2560×1440`，只缩不放大。benchmark/capture 不消费 `WindowSize`，继续走固定离屏尺寸。

运行期 API 只承诺发出受约束的尺寸请求；窗口管理器可按平台规则调整最终尺寸。设置保存发生在同一锁定 OS 主线程，没有新增 goroutine 或跨线程窗口访问。

### 7. 视觉基线只允许两个 UI 文件变化

capture 场景在现有 `main-menu` 后插入 `settings-menu`，再接 `far-horizon`、`water-underwater`。设置快照使用非默认音量、短相对路径和 `960x540`，不加载本机配置、不校验目录、不创建设备。`main-menu.png` 因设置按钮启用允许更新，新增 `settings-menu.png`；提交前对其余既有 PNG 做字节比较，任何额外变化都视为回归。benchmark 不上传字体、不产生 UI 段，scenario 保持 v19。

### 8. 流程、文件与文档边界

实现按以下关注点分组：`internal/config` 负责配置类型/解析；`internal/client` 与 Rust/header 负责 ABI；`cmd/mornlea/app_settings.go` 负责状态和保存；窗口/音频只经既有 application 依赖注入；capture 单独收尾。Rust `ui.rs` 已较大，新增设置测试优先拆到主题子模块并继续保留一个 UI helper 中心。

client ABI v9 和用户能力需同步 `AGENTS.md` 与 `CLAUDE.md`（逐字节相同）、README 中英文配置说明及 `docs/notes/progress.md`。不把本 change 的非目标写入长期基线。

## Risks / Trade-offs

- [材质校验到下一次启动之间存在 TOCTOU] → 保存时和下一次启动都执行完整校验；保存校验只改善即时反馈，不替代启动门禁。
- [超长或含 CR/LF 的旧材质路径从“可解析”变为配置错误] → 上限与 Darwin 路径/ABI 预算同源，单行约束保证 UI 编码安全，错误包含字段名；迁移只需缩短/清空或改名单行目录，配置版本不变。
- [结构化事件批比裸 id 更复杂] → 只定义 action 与 full-snapshot change 两种 kind，Go/Rust 交叉 golden、非法输入矩阵、顺序和容量不消费测试锁定。
- [配置落盘后音频设备可能不可用] → 沿用本地音频的无声降级；配置保存不依赖设备成功，simulation/network/render 不受影响。
- [窗口管理器可能不完全接受请求尺寸] → 契约限定为受双重钳制的请求；下一次快照据实更新 renderer aspect，不伪造结果。
- [主菜单 golden 合法变化可能掩盖其他漂移] → 只允许 `main-menu.png` 和新增 `settings-menu.png`，其余 PNG 逐字节守卫；两个 UI 图人工复核。
- [保存同步 I/O 会短暂停顿菜单帧] → 世界和权威 tick 尚未启动，操作由用户显式触发且全部输入有界；不引入后台写入与生命周期竞态。

## Migration Plan

1. 先以配置 v1 兼容测试落地 `windowSize` 默认与三预设，以及 1024 字节、无 CR/LF 的单行路径门禁；旧普通配置无需改写，含换行目录名需改名或清空路径。
2. client ABI v9 的 Rust 常量、C header、Go 绑定、UI layout 和事件出口必须在同一任务/提交内演进，发布时 `mornlea_client` 与 Go binary 继续作为不可混装 release unit。
3. 接入 Go 设置状态机与保存后，再生成两张 UI golden；确认其余 golden 字节不变、benchmark scenario 与线上/存档版本未变。
4. 回滚可整体撤销本 change。新配置中的 `windowSize` 对旧程序是未知字段并按既有纪律告警忽略；若旧程序重新保存可能丢弃该字段，但音量、材质路径和世界数据不受影响。
