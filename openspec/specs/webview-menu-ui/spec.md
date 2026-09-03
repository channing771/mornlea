# webview-menu-ui Specification

## Purpose

为窗口型界面（主菜单、设置、暂停、调试面板）建立进程内 WebView（Vite + TypeScript + React）技术基础与世界全景背景：交互客户端启动停留在世界全景之上的主菜单，世界装配延迟到用户点击之后；菜单语义与既有规格逐条平移，`egui` 即时模式栈完全退役。
## Requirements
### Requirement: 交互客户端启动停留在主菜单

交互式图形客户端（非 benchmark/capture，且未指定 `-connect`）SHALL 在窗口与渲染器初始化完成后停留在主菜单界面，MUST NOT 在此之前打开世界存储、启动本地权威服务端或完成登录。主菜单与设置页相位 SHALL 以世界全景为背景：全景由固定种子 worldgen 直供区块经既有网格与天空光照渲染路径产出，由固定脚本相机驱动，MUST NOT 触发世界存储打开、本地权威服务端启动或登录。

#### Scenario: 交互启动显示菜单且世界未装配

- **GIVEN** 默认配置的交互式启动
- **WHEN** 客户端完成窗口与渲染器初始化
- **THEN** 主菜单可见且背景呈现世界全景
- **AND** 世界存储未打开、本地权威服务端未启动、登录未发生
- **AND** 菜单期间的按键（WASD、F3 等）不产生任何游戏输入上行，玩家状态不改变

#### Scenario: 命令行远程连接跳过菜单

- **GIVEN** 启动参数指定了 `-connect`
- **WHEN** 客户端启动
- **THEN** MUST NOT 显示主菜单，直接连接远程服务端并进入游戏

#### Scenario: benchmark 与 capture 路径不显示交互菜单

- **GIVEN** benchmark 或 capture 模式启动
- **WHEN** 客户端初始化完成
- **THEN** 交互式主菜单与设置页 MUST NOT 显示，WebView MUST 保持隐藏且零参与
- **AND** benchmark 路径 MUST NOT 运行任何菜单 UI 代码
- **AND** capture 路径仅在 `main-menu` 或 `settings-menu` 场景渲染全景底图（无头路径无 WebView 参与）

#### Scenario: 全景背景确定性

- **GIVEN** 相同种子、相同客户端构建与同一台机器
- **WHEN** 两次进入主菜单相位
- **THEN** 全景的世界内容与相机轨迹 MUST 逐帧一致
- **AND** 全景渲染 MUST 不打开世界存储、不启动本地权威服务端

### Requirement: 主菜单参考经典标题画面布局

主菜单 SHALL 在世界全景之上显示大标题「Mornlea」、中心纵排按钮列与底部版本行；按钮列 MUST 垂直居中排布、互不重叠；版本行 MUST 显示应用版本号，无法取得时显示 `dev`。菜单 chrome 由 React 组件呈现，其视觉与结构 MUST 由前端组件断言测试覆盖。

#### Scenario: 标题、按钮列与版本行可判定

- **GIVEN** 1280×720 的菜单帧
- **WHEN** 系统渲染主菜单
- **THEN** 标题「Mornlea」可见且位于按钮列上方
- **AND** 按钮列包含「进入游戏」「多人游戏」「设置」「退出游戏」四个按钮，垂直排列且互不重叠
- **AND** 底部出现版本行

#### Scenario: 按钮点击几何与命中一致

- **GIVEN** 任一菜单帧
- **WHEN** 指针位于某按钮的可见矩形内并点击
- **THEN** 该按钮（而非其他按钮）产生且仅产生一次点击事件

#### Scenario: 菜单 chrome 由组件断言覆盖

- **GIVEN** 前端构建链可用
- **WHEN** 运行前端组件测试
- **THEN** 四屏（主菜单/设置/暂停/调试面板）的布局关系、按钮集合与禁用态 MUST 由 vitest 断言覆盖
- **AND** 菜单 chrome 的像素不参与无头 capture 的 golden 比对（系统 WebView 渲染不可钉死）

### Requirement: 禁用按钮不产生事件

「多人游戏」按钮 SHALL 呈现为禁用状态，点击后 MUST NOT 产生任何桥事件；「设置」按钮 SHALL 呈现为启用状态并在点击时产生恰好一个导航事件。

#### Scenario: 点击多人游戏无事件

- **GIVEN** 主菜单可见
- **WHEN** 点击「多人游戏」
- **THEN** 排空桥事件队列 MUST 得到空结果
- **AND** 菜单内容与相位 MUST 不变

#### Scenario: 点击设置产生一次导航事件

- **GIVEN** 主菜单可见
- **WHEN** 点击「设置」
- **THEN** 排空桥事件队列 MUST 得到恰好一个设置导航事件
- **AND** Go 菜单状态 MUST 切换到设置页

### Requirement: 进入游戏执行延迟的世界装配

点击「进入游戏」SHALL 一次性执行延迟的世界装配：打开世界存储、启动本地权威服务端、完成登录并把远环 LOD 播种器接线到登录种子；装配期间 MUST 忽略重复点击；装配成功后 MUST 隐藏主菜单并进入世界加载相位（`world-loading-screen` capability：不透明加载屏覆盖渐进加载中的世界），光标捕获与游戏输入生效 MUST 推迟到加载收敛之后。装配失败时 MUST 保持主菜单可见并显示装配错误文本。

#### Scenario: 装配成功进入游戏

- **GIVEN** 主菜单可见且世界未装配
- **WHEN** 点击「进入游戏」且装配成功
- **THEN** 主菜单不再显示，客户端进入加载相位并呈现世界加载屏
- **AND** 光标尚未捕获，WASD 等游戏输入不生效，菜单阶段累积的按键不产生副作用

#### Scenario: 装配期间重复点击只装配一次

- **GIVEN** 世界装配进行中
- **WHEN** 再次点击「进入游戏」
- **THEN** 装配只执行一次；等待完成单次装配结果

#### Scenario: 装配失败展示错误且可退出

- **GIVEN** 主菜单可见且世界装配失败（如存档不可打开）
- **WHEN** 点击「进入游戏」
- **THEN** 主菜单保持可见并显示装配错误文本
- **AND** 「退出游戏」仍可用且客户端进程不崩溃

### Requirement: 退出游戏关闭客户端

点击「退出游戏」SHALL 请求关闭图形客户端；世界已装配时 MUST 走既有会话与本地服务端的正常关闭路径。

#### Scenario: 菜单阶段退出

- **GIVEN** 主菜单可见（世界未装配）
- **WHEN** 点击「退出游戏」
- **THEN** 客户端正常退出，无残留服务端进程或打开句柄错误

### Requirement: 菜单期间游戏输入不生效且由 WebView 消费

菜单可见时 SHALL 不捕获光标；菜单相位下键盘与指针输入 MUST 由 WebView 原生消费并仅经桥事件上行，winit 路径的移动、采掘/放置、快捷栏、调试面板与聊天键 MUST NOT 生效；主菜单不可见后 WebView MUST 进入隐藏零参与态，游戏输入恢复由 winit 独占采集。

#### Scenario: 菜单不捕获光标

- **GIVEN** 主菜单可见
- **WHEN** 无用户操作
- **THEN** 光标可见且未被捕获

#### Scenario: 游戏阶段无残留菜单事件

- **GIVEN** 主菜单已关闭且游戏进行中
- **WHEN** 用户正常游戏点击
- **THEN** 游戏行为与无菜单路径一致，桥事件队列为空
- **AND** WebView MUST 不产生任何 evaluateJavaScript 调用或消息处理

### Requirement: WebView 集成技术边界

`mornlea_client` SHALL 通过 `objc2-web-kit` 将一个透明 WKWebView 挂载到既有 winit NSWindow 的 contentView 之上，MUST NOT 引入 wry、tao 或第二套窗口栈；webview 资产 MUST 经 `WKURLSchemeHandler` 从二进制内嵌字节以 `mornlea://` 供给，MUST NOT 访问网络、CDN 或磁盘临时文件；`egui` 与 `egui-wgpu` 依赖 MUST 全部移除；`wgpu` 主版本 MUST NOT 单侧升级；任何 Go 包 MUST NOT 引入 GUI 绑定。WebView 参与模式 SHALL 恰有两态——菜单相位 `Menu`（全参与）与游戏相位 `GameOverlay`（可见合成、不参与响应链，见 `game-overlay-webview` capability）：两态 MUST 由同一 WKWebView 实例经命中测试分级（子类化 hitTest）实现，MUST NOT 依赖窗口叠层顺序或第二实例切换；GameOverlay 态的建立 MUST NOT 引入新的 C ABI 出口。

#### Scenario: 透明覆盖与资产离线供给

- **GIVEN** 菜单相位的客户端窗口
- **WHEN** WebView 呈现菜单 chrome
- **THEN** WebView 背景 MUST 为透明，wgpu 渲染的世界全景在其下可见
- **AND** 全部 HTML/JS/CSS/字体请求 MUST 由内嵌 scheme handler 供给，无任何网络请求

#### Scenario: 游戏相位零参与

- **GIVEN** 游戏进行中（GameOverlay 模式）
- **WHEN** 系统渲染帧
- **THEN** GameOverlay WebView MUST 保持可见合成且 MUST NOT 进入响应链（指针、键盘、滚轮事件全部由 winit 采集），wgpu 呈现路径与迁移前一致
- **AND** 桥事件排空 MUST 为空，游戏输入行为与无 WebView 路径一致

#### Scenario: 依赖版本线替换

- **GIVEN** `engine/crates/mornlea_client/Cargo.toml` 与依赖树
- **WHEN** 检查依赖
- **THEN** `egui`、`egui-wgpu` 及其传递依赖 MUST 不存在
- **AND** `objc2`、`objc2-web-kit` MUST 在锁文件中钉版本

### Requirement: 菜单状态与事件桥

菜单状态权威 SHALL 在 Go：Go 侧在状态变化时经 client ABI 以 JSON 字符串推送菜单/设置/调试状态下行，Rust 侧转发为 WebView 内 `window.mornlea.onState` 调用；上行 SHALL 由 WebView 脚本消息进入 Rust 队列并以版本化 JSON 事件批经既有排空出口交付 Go 依序消费。桥 schema SHALL 以单源 JSON Schema 文件为准，Go/Rust/TS 三端 MUST 各有钉值一致性测试；未知事件类型、schema 越界或非法 UTF-8 MUST 被拒绝且不触碰运行态。游戏相位 SHALL 经同一 JSON 下行出口推送常显 HUD 状态族（`game-overlay-webview` capability）：状态族按权威 tick 合并推送、禁止每帧重复推送；schema 的游戏相位状态族与既有菜单状态族共用单源文件与三端钉值纪律，client ABI 版本 MUST 保持不变。

#### Scenario: 状态下行事件驱动

- **GIVEN** 菜单相位下 Go 侧菜单/设置/调试状态发生一次变化
- **WHEN** 系统处理该变化
- **THEN** MUST 恰好推送一份包含变化后完整状态的 JSON，且 MUST NOT 存在每帧重复推送

#### Scenario: 游戏相位 HUD 状态 tick 合并下行

- **GIVEN** 游戏相位下同一权威 tick 内多类 HUD 状态变化
- **WHEN** 桥下行运行
- **THEN** MUST 恰好推送一份合并终态的 JSON；无变化的 tick MUST 零推送

#### Scenario: 上行事件保序

- **GIVEN** 同一交互序列先后产生设置值编辑与按钮点击
- **WHEN** Go 排空事件批
- **THEN** 事件 MUST 按 WebView 产生顺序出现，值编辑事件先于点击事件

#### Scenario: 非法桥载荷被拒绝

- **GIVEN** 桥载荷含未知事件类型、schema 越界字段或非法 UTF-8
- **WHEN** Rust 或 Go 处理该载荷
- **THEN** MUST 返回错误且不触碰菜单状态与渲染器状态

### Requirement: 前端资产与构建链

`mornlea_client` 的菜单前端 SHALL 位于 `frontend/`（Vite + TypeScript + React），依赖由提交的 pnpm 锁文件钉死（corepack `packageManager` 钉版）；构建产物 dist SHALL 提交入库并被 Rust 二进制内嵌；构建链 MUST 保证 `pnpm install --frozen-lockfile + 类型检查 + 组件测试 + 构建` 后的 dist 与入库版本逐字节一致，任何漂移 MUST 使门禁失败。

#### Scenario: dist 一致性门禁

- **GIVEN** 干净检出的仓库
- **WHEN** 运行前端检查门禁
- **THEN** `pnpm install --frozen-lockfile`、TypeScript 检查、组件测试与 `vite build` MUST 全部通过
- **AND** 重新构建的 dist 与入库 dist MUST 逐字节一致，否则 MUST 失败

#### Scenario: 离线运行

- **GIVEN** 断网环境
- **WHEN** 客户端启动并显示主菜单
- **THEN** 菜单功能完整可用，无任何网络依赖

### Requirement: client ABI v12 引入菜单桥并由 v14 保留

进程内 WKWebView 菜单桥 SHALL 在 client ABI v12 引入：新增菜单状态推送出口（JSON 字符串下行）并保留版本化事件批排空出口（信封格式更新）；`upload_ui_font` 出口与帧 TLV tag 9 UI 段及其 layout 编解码 MUST 退役。client ABI v13 在该 surface 上增加 window composite capture；当前 client ABI v14 MUST 保留 v12 菜单 surface、v13 capture surface 与退役状态，并增加独立的 MRW1 入口。版本不匹配 MUST 在全部接受 ABI version 的出口优先拒绝，v13 与 v14 二进制不可混装。菜单桥引入本身 MUST NOT 改变协议、存档、engine ABI 或 benchmark scenario；当前 engine ABI v9 的独立演进不改变菜单桥行为。

#### Scenario: 当前版本号三处一致且保留 v12 菜单 surface

- **GIVEN** 当前 Rust client、C header 与 Go 绑定
- **WHEN** 检查 ABI 版本常量、查询出口和菜单桥 exports
- **THEN** 三处 client ABI MUST 均为 `14`
- **AND** ABI 查询出口 MUST 返回 `14`
- **AND** v12 引入的 `ui_push_state` 与版本化 JSON event drain MUST 保持可用
- **AND** v13 引入的 window composite capture MUST 保持可用

#### Scenario: v12 引入事实保持可追溯

- **GIVEN** client ABI v11 的菜单前代 surface
- **WHEN** 进程内 WKWebView 菜单桥首次交付
- **THEN** 该次演进 MUST 记为 v11→v12
- **AND** `upload_ui_font`、frame TLV tag 9 与 UI layout v1–v4 MUST 自 v12 起保持退役

#### Scenario: 退役出口被拒绝

- **GIVEN** 当前 v14 二进制
- **WHEN** 调用已退役的字体上传出口或下发旧 tag 9 UI 段
- **THEN** MUST 返回版本/参数错误且不触碰渲染器状态

### Requirement: 调试面板以 WebView 呈现

调试面板 SHALL 迁移为 WebView React 组件：`debug-panel` capability 的全部行为语义（F3 边沿切换、`-dev` 门控、只读读数、参数分组、行选中/编辑/写回、联机 physics/sim 只读、行数上限、面板可见时捕获输入）MUST 保持；行数据经桥状态下行，编辑事件经桥事件批上行，Go 侧组装维持既有字节上限。

#### Scenario: 调试行为语义平移

- **GIVEN** `-dev` 启用的交互客户端
- **WHEN** 按 F3 边沿切换、选中行、进入编辑并确认
- **THEN** 全部行为与 `debug-panel` 既有规格一致
- **AND** 联机远端 physics/sim 分组的选中与编辑 MUST 仍被禁止

### Requirement: 游戏内 Esc 打开暂停覆盖层

游戏中（世界已装配相位）按 Esc SHALL 打开暂停覆盖层并释放光标；覆盖层只含「返回游戏」「退回主菜单」两个按钮。再次按 Esc 或点击「返回游戏」SHALL 关闭覆盖层、重新捕获光标并回到游戏，重复触发 MUST 只生效一次。Esc 优先级栈（聊天取消 → 关背包/容器 → 调试面板消费 → 暂停层返回游戏 → 打开暂停层）MUST 保持。

#### Scenario: Esc 打开暂停层

- **GIVEN** 单机游戏已进入游戏相位且无聊天输入、无背包界面
- **WHEN** 按 Esc
- **THEN** 暂停覆盖层出现，「返回游戏」「退回主菜单」两按钮可判定
- **AND** 光标被释放

#### Scenario: 再次 Esc 返回游戏

- **GIVEN** 暂停覆盖层已打开
- **WHEN** 再按一次 Esc
- **THEN** 覆盖层关闭，光标重新捕获，回到游戏相位

#### Scenario: 远程会话呈现但不宣称暂停

- **GIVEN** 客户端经 TCP 连接远程服务端
- **WHEN** 在游戏相位按 Esc
- **THEN** 同一覆盖层出现且带注明行说明远程世界不会停止
- **AND** 服务端 tick 照常推进，客户端未调用任何本地服务端暂停接口

### Requirement: 本地权威暂停门冻结模拟并可恢复

单机嵌入服 SHALL 提供幂等的暂停/恢复门：暂停期间权威 tick 不被调度执行——世界时间计数、作物生长、流体推进与实体行为全部保持不变；恢复后模拟从原状态确定性续跑。

#### Scenario: 暂停期间世界时间冻结

- **GIVEN** 单机权威服务端已暂停
- **WHEN** 连续等待多个 tick 调度周期
- **THEN** 世界时间 ticks 计数不变
- **AND** 重复调用暂停门不产生额外状态变化

#### Scenario: 恢复后确定性续跑

- **GIVEN** 曾处于暂停状态的服务端已恢复
- **WHEN** 继续推进与暂停前相同数量的 tick 并以相同种子重放对照
- **THEN** 暂停段不改变任何后续结算结果

### Requirement: 退回主菜单安全拆解会话

在暂停覆盖层点击「退回主菜单」SHALL 复用既有会话拆链路径安全关闭当前会话（含持久化收尾），回到主菜单；此后「进入游戏」MUST 能重新装配世界。远程连接形态下不存在可装配的本地世界，此后点击「进入游戏」SHALL 以主菜单错误行优雅拒绝而非异常退出。

#### Scenario: 退回主菜单后可再进入

- **GIVEN** 暂停覆盖层已打开
- **WHEN** 点击「退回主菜单」
- **THEN** 会话安全关闭并回到主菜单
- **AND** 再点击「进入游戏」能成功装配并进入游戏

#### Scenario: 远程会话退回后再进入被优雅拒绝

- **GIVEN** 客户端经 TCP 连接远程服务端且暂停覆盖层已打开
- **WHEN** 点击「退回主菜单」后再次点击「进入游戏」
- **THEN** 界面停留在主菜单并显示错误行说明远程连接形态不支持本地世界装配
- **AND** 进程不异常退出、不触发任何本地世界装配

### Requirement: 面板统一像素组件风格

四面板的可交互元素 SHALL 经统一像素呈现层呈现：按钮与表单控件 MUST 使用
pixel-retroui 组件或按 `tokens.css` 像素令牌重绘的自绘控件；颜色、几何与
阴影 MUST 经 `tokens.css` 令牌供给（强调为鼠尾草绿与麦金组成的 Mornlea
双强调体系——选中与焦点态走鼠尾草绿，进度、来源与重要信息走麦金；危险红
仅用于错误；`prefers-reduced-motion` 下动效归零）；文案、上行事件、焦点
顺序与键盘语义 MUST 与改造前逐项一致；音量控件 MUST 保持滑块形态与
`[0,1]` 映射。

#### Scenario: 主菜单按钮像素化且行为不变

- GIVEN 主菜单可见
- WHEN 检查按钮列并点击任一按钮
- THEN 按钮以统一像素组件呈现（硬描边与偏移阴影风格）
- AND 该按钮产生与改造前相同的上行 `action` 事件，标题、按钮文案、
  纵排几何与版本行不变

#### Scenario: 设置表单像素化且语义不变

- GIVEN 设置页可见
- WHEN 检查三个控件并编辑后保存
- THEN 材质路径为单行文本输入、窗口大小为三预设选择、音量仍为滑块并
  显示百分比
- AND 编辑草稿、保存、取消与脏草稿返回语义与改造前逐项一致

#### Scenario: 令牌纪律与动效降级保持

- GIVEN 任一面板可见
- WHEN 系统开启 prefers-reduced-motion 并检查样式来源
- THEN 动效时长为零且无组件级 transition 绕开令牌
- AND 面板样式值不出现在组件内的裸色值，全部经 tokens.css 令牌供给
- AND 强调呈现只使用鼠尾草绿与麦金两个色相，错误行只使用危险红

### Requirement: 菜单尺寸随窗口比例协调

四面板的尺寸几何 SHALL 随窗口尺寸保持协调：主菜单按钮列、设置面板与调试面板的宽度 MUST 以视口宽度为约束上限（比例或 `min()`/`clamp()` 口径的令牌供给），不得在小窗口下溢出视口或在任意大的窗口下无限放大；主菜单的标题、按钮列与版本行 MUST 在合法窗口尺寸范围内保持视觉居中且互不重叠；尺寸令牌 MUST 经 `tokens.css` 供给，面板侧不得绕开令牌硬编码视口相关尺寸。

#### Scenario: 小窗口不溢出

- **GIVEN** 一个小于面板设计宽度的合法窗口尺寸
- **WHEN** 系统渲染任一面板
- **THEN** 面板与按钮列 MUST 完整位于视口内，MUST NOT 产生视口溢出或裁切
- **AND** 面板内部内容 MUST 保持可滚动或完整可见，交互元素不因尺寸收缩而重叠

#### Scenario: 大窗口不无限放大

- **GIVEN** 一个远大于面板设计宽度的窗口尺寸
- **WHEN** 系统渲染设置面板与主菜单按钮列
- **THEN** 面板宽度 MUST 停在其令牌上限而不得随视口线性放大
- **AND** 主菜单标题、按钮列与版本行 MUST 保持居中构图且按钮列不因窗口变大而失去间距层级

### Requirement: UI 字体资产

WebView UI SHALL 以缝合像素字体（Fusion Pixel，OFL-1.1）为首选字体并以
系统 CJK 栈兜底；字体文件与 OFL 许可文本 MUST 随仓库入库，字体作为唯一
白名单二进制 Web 资产经 `mornlea://` scheme 内嵌供给（Rust 侧资产表登记），
MUST NOT 产生任何网络请求或引入许可不明的字体文件；`dist` 字节复现门禁
MUST 覆盖字体资产。

#### Scenario: 字体内嵌供给零网络

- GIVEN 客户端离线运行
- WHEN 任一面板渲染
- THEN 文本以像素字体呈现（CJK 覆盖），资产全部来自内嵌 scheme，无任何
  网络请求

#### Scenario: 字体加载失败回退系统栈

- GIVEN 像素字体不可用（资产缺失或格式不受支持）
- WHEN 面板渲染文本
- THEN 字体栈按声明回退到系统 CJK 栈，UI 功能与布局不因字体缺失失败

#### Scenario: 许可文本随库入库

- WHEN 检查字体文件所在目录
- THEN OFL-1.1 许可文本与字体文件一同入库，来源与版本记录在案

### Requirement: UI 部件视觉基线

每一个 UI 部件（四整屏面板与各独立控件态）SHALL 拥有入库的视觉基线 PNG：
基线由仓库内脚本经无头浏览器对前端构建产物截取生成，部件清单 MUST 覆盖
四整屏面板与 pixel 组件的各呈现态（默认/禁用/选中强调、文本输入、滑块、
调试面板行态、错误行）；比对 SHALL 采用与世界 golden 管线同口径的双阈值
（通道差与差异像素占比）判定漂移；基线更新 MUST 经显式的 update 入口人工
确认；该管线 MUST 为本机开发工具（不进 CI 门禁、不触网、不改 dist 契约）。

#### Scenario: 基线生成与漂移检出

- GIVEN 前端构建产物与部件清单
- WHEN 运行视觉基线 update 入口
- THEN 每个部件产出一张基线 PNG 入库；对任一部件人为引入像素级改动后
  运行 check 入口，该部件以双阈值判定报告漂移并以非零退出失败，其余
  部件不受影响

#### Scenario: 基线工具自包含

- WHEN 在本机运行 check 入口
- THEN 管线只依赖仓库内产物与本机既有无头浏览器，零网络请求，不修改
  `dist/` 与其字节一致性门禁

