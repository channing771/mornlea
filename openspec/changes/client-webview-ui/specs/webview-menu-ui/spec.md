# webview-menu-ui Specification

## Purpose

为窗口型界面（主菜单、设置、暂停、调试面板）建立进程内 WebView（Vite + TypeScript + React）技术基础与世界全景背景：交互客户端启动停留在世界全景之上的主菜单，世界装配延迟到用户点击之后；菜单语义与既有规格逐条平移，`egui` 即时模式栈完全退役。

## ADDED Requirements

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

点击「进入游戏」SHALL 一次性执行延迟的世界装配：打开世界存储、启动本地权威服务端、完成登录并把远环 LOD 播种器接线到登录种子；装配期间 MUST 忽略重复点击；装配成功后 MUST 隐藏主菜单并捕获光标，游戏输入自此生效。

#### Scenario: 装配成功进入游戏

- **GIVEN** 主菜单可见且世界未装配
- **WHEN** 点击「进入游戏」且装配成功
- **THEN** 主菜单不再显示，光标被捕获，WebView 进入隐藏零参与态
- **AND** WASD 等输入开始驱动玩家，菜单阶段累积的按键不产生副作用

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

`mornlea_client` SHALL 通过 `objc2-web-kit` 将一个透明 WKWebView 挂载到既有 winit NSWindow 的 contentView 之上，MUST NOT 引入 wry、tao 或第二套窗口栈；webview 资产 MUST 经 `WKURLSchemeHandler` 从二进制内嵌字节以 `mornlea://` 供给，MUST NOT 访问网络、CDN 或磁盘临时文件；`egui` 与 `egui-wgpu` 依赖 MUST 全部移除；`wgpu` 主版本 MUST NOT 单侧升级；任何 Go 包 MUST NOT 引入 GUI 绑定。

#### Scenario: 透明覆盖与资产离线供给

- **GIVEN** 菜单相位的客户端窗口
- **WHEN** WebView 呈现菜单 chrome
- **THEN** WebView 背景 MUST 为透明，wgpu 渲染的世界全景在其下可见
- **AND** 全部 HTML/JS/CSS/字体请求 MUST 由内嵌 scheme handler 供给，无任何网络请求

#### Scenario: 游戏相位零参与

- **GIVEN** 游戏进行中（无菜单相位）
- **WHEN** 系统渲染帧
- **THEN** WebView MUST 处于隐藏态且不参与响应链
- **AND** GPU 呈现路径与菜单迁移前一致

#### Scenario: 依赖版本线替换

- **GIVEN** `engine/crates/mornlea_client/Cargo.toml` 与依赖树
- **WHEN** 检查依赖
- **THEN** `egui`、`egui-wgpu` 及其传递依赖 MUST 不存在
- **AND** `objc2`、`objc2-web-kit` MUST 在锁文件中钉版本

### Requirement: 菜单状态与事件桥

菜单状态权威 SHALL 在 Go：Go 侧在状态变化时经 client ABI 以 JSON 字符串推送菜单/设置/调试状态下行，Rust 侧转发为 WebView 内 `window.mornlea.onState` 调用；上行 SHALL 由 WebView 脚本消息进入 Rust 队列并以版本化 JSON 事件批经既有排空出口交付 Go 依序消费。桥 schema SHALL 以单源 JSON Schema 文件为准，Go/Rust/TS 三端 MUST 各有钉值一致性测试；未知事件类型、schema 越界或非法 UTF-8 MUST 被拒绝且不触碰运行态。

#### Scenario: 状态下行事件驱动

- **GIVEN** 菜单相位下 Go 侧菜单/设置/调试状态发生一次变化
- **WHEN** 系统处理该变化
- **THEN** MUST 恰好推送一份包含变化后完整状态的 JSON，且 MUST NOT 存在每帧重复推送

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

### Requirement: client ABI v12 菜单桥扩展

`mornlea_client` ABI SHALL 提升到 v12：新增菜单状态推送出口（JSON 字符串下行）并保留版本化事件批排空出口（信封格式更新）；`upload_ui_font` 出口与帧 TLV tag 9 UI 段及其 layout 编解码 MUST 退役。ABI 版本不匹配 MUST 在所有出口被拒绝，v11 与 v12 二进制不可混装。协议、存档、engine ABI、benchmark scenario MUST NOT 变化。

#### Scenario: 版本号三处一致

- **GIVEN** Rust client、C header 与 Go 绑定
- **WHEN** 检查 ABI 版本常量和查询出口
- **THEN** 三处 client ABI MUST 均为 `12`
- **AND** ABI 查询出口 MUST 返回 `12`

#### Scenario: 退役出口被拒绝

- **GIVEN** v12 二进制
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
