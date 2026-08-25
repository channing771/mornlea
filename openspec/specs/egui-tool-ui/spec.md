# egui-tool-ui Specification

## Purpose
为工具型 UI（进入游戏后的菜单等窗口型界面；常驻 HUD 与容器格子不在其列）建立 egui 技术基础，并交付「进入游戏」主菜单竖切：交互客户端启动停留在主菜单，世界装配延迟到用户点击之后，参考经典标题画面的菜单可观察行为由此可自动验收。
## Requirements
### Requirement: 交互客户端启动停留在主菜单

交互式图形客户端（非 benchmark/capture，且未指定 `-connect`）SHALL 在窗口与渲染器初始化完成后停留在主菜单界面，MUST NOT 在此之前打开世界存储、启动本地权威服务端或完成登录。

#### Scenario: 交互启动显示菜单且世界未装配

- **GIVEN** 默认配置的交互式启动
- **WHEN** 客户端完成窗口与渲染器初始化
- **THEN** 主菜单可见
- **AND** 世界存储未打开、本地权威服务端未启动、登录未发生
- **AND** 菜单期间的按键（WASD、F3 等）不产生任何游戏输入上行，玩家状态不改变

#### Scenario: 命令行远程连接跳过菜单

- **GIVEN** 启动参数指定了 `-connect`
- **WHEN** 客户端启动
- **THEN** MUST NOT 显示主菜单，直接连接远程服务端并进入游戏

#### Scenario: benchmark 与 capture 路径不显示交互菜单

- **GIVEN** benchmark 或 capture 模式启动
- **WHEN** 客户端初始化完成
- **THEN** 交互式主菜单与设置页 MUST NOT 显示
- **AND** benchmark 路径 MUST NOT 上传菜单字体或运行 egui
- **AND** capture 路径仅在 `main-menu` 或 `settings-menu` 场景需要时上传字体并渲染对应 UI 快照

### Requirement: 主菜单参考经典标题画面布局

主菜单 SHALL 显示大标题「Mornlea」、中心纵排按钮列与底部版本行；按钮列 MUST 垂直居中排布、互不重叠；版本行 MUST 显示应用版本号，无法取得时显示 `dev`。

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

### Requirement: 禁用按钮不产生事件

「多人游戏」按钮 SHALL 呈现为禁用状态，点击后 MUST NOT 产生任何 UI 事件；「设置」按钮 SHALL 呈现为启用状态并在点击时产生恰好一个导航事件。

#### Scenario: 点击多人游戏无事件

- **GIVEN** 主菜单可见
- **WHEN** 点击「多人游戏」
- **THEN** 排空 UI 事件队列 MUST 得到空结果
- **AND** 菜单内容与相位 MUST 不变

#### Scenario: 点击设置产生一次导航事件

- **GIVEN** 主菜单可见
- **WHEN** 点击「设置」
- **THEN** 排空 UI 事件队列 MUST 得到恰好一个设置导航事件
- **AND** Go 菜单状态 MUST 切换到设置页

### Requirement: 进入游戏执行延迟的世界装配

点击「进入游戏」SHALL 一次性执行延迟的世界装配：打开世界存储、启动本地权威服务端、完成登录并把远环 LOD 播种器接线到登录种子；装配期间 MUST 忽略重复点击；装配成功后 MUST 隐藏主菜单并捕获光标，游戏输入自此生效。

#### Scenario: 装配成功进入游戏

- **GIVEN** 主菜单可见且世界未装配
- **WHEN** 点击「进入游戏」且装配成功
- **THEN** 主菜单不再显示，光标被捕获
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

### Requirement: 菜单期间游戏输入不生效

菜单可见时 SHALL 不捕获光标；菜单可见期间的指针与键盘输入 MUST 只服务于菜单交互，游戏移动、采掘/放置、快捷栏、调试面板与聊天键 MUST NOT 生效；主菜单不可见后 egui MUST NOT 消费输入。

#### Scenario: 菜单不捕获光标

- **GIVEN** 主菜单可见
- **WHEN** 无用户操作
- **THEN** 光标可见且未被捕获

#### Scenario: 游戏阶段无残留菜单事件

- **GIVEN** 主菜单已关闭且游戏进行中
- **WHEN** 用户正常游戏点击
- **THEN** 游戏行为与无菜单路径一致，菜单事件队列为空

### Requirement: egui 集成的技术边界

`mornlea_client` SHALL 引入 `egui` 与 `egui-wgpu` 0.35.x（与仓库 wgpu 29 主版本线一致），MUST NOT 引入 `egui-winit`；egui SHALL 以一条附加 wgpu render pass 挂在既有渲染管线中并排在既有 HUD pass 之后；任何 Go 包 MUST NOT 引入 GUI/GPU 绑定；`wgpu` 主版本 MUST NOT 单侧升级。

#### Scenario: 依赖版本线锁定

- **GIVEN** `engine/Cargo.toml`
- **WHEN** 检查依赖
- **THEN** `egui` 与 `egui-wgpu` 均为 0.35.x、`wgpu` 仍为 29.x
- **AND** 依赖树中等于 `egui-winit` 的 crate 不存在

#### Scenario: 菜单输入由手工 RawInput 驱动

- **GIVEN** 菜单可见时的 winit 事件
- **WHEN** 事件被翻译为 `egui::RawInput`
- **THEN** 指针位置、按键与文本事件全部进入 egui 事件流
- **AND** 翻译路径中不出现 `egui_winit` 类型

### Requirement: client ABI v9 结构化设置事件扩展

`mornlea_client` ABI SHALL 提升到 v9：保留字体上传与帧 TLV tag 9，UI 下行保留 layout v1 主菜单并新增 layout v2 设置页；UI 上行 SHALL 由裸按钮 ID 序列升级为有版本、有类型、有长度且顺序稳定的结构化事件批。ABI 版本不匹配 MUST 在所有入口被拒绝，v8 与 v9 二进制不可混装。

#### Scenario: 版本号三处一致

- **GIVEN** Rust client、C header 与 Go 绑定
- **WHEN** 检查 ABI 版本常量和查询出口
- **THEN** 三处 client ABI MUST 均为 `9`
- **AND** ABI 查询出口 MUST 返回 `9`

#### Scenario: 设置 UI 非法下行被拒绝

- **GIVEN** UI 段携带未知 layout、非法 UTF-8、NaN/Inf 音量、越界路径、未知窗口预设、非法布尔值或尾随字节
- **WHEN** 渲染该帧
- **THEN** MUST 返回 `INVALID_ARGUMENT` 且不触碰渲染器 UI 状态

#### Scenario: 结构化事件保持同帧顺序

- **GIVEN** 同一 egui 帧先完成文本编辑再点击保存
- **WHEN** Go 排空结构化事件批
- **THEN** 设置变化事件 MUST 排在保存 action 之前
- **AND** Go MUST 能用该批中的最终草稿执行保存

#### Scenario: 缓冲不足不排空事件

- **GIVEN** 待排空结构化事件的完整编码长度大于调用方缓冲
- **WHEN** 调用事件排空出口
- **THEN** MUST 返回 `CAPACITY`
- **AND** 调用方缓冲、输出计数与待排空队列 MUST 保持不变

#### Scenario: 字体缺失时 UI 帧被拒绝

- **GIVEN** 渲染器尚未收到菜单字体
- **WHEN** 渲染携带 UI 段的帧
- **THEN** UI 帧被拒绝（编程错误路径），非 UI 帧不受影响

### Requirement: 无 UI 帧时 egui 零参与

帧不携带 UI 段时，渲染器 SHALL NOT 运行 egui 上下文、上传 egui 纹理或提交 egui pass，且 MUST 丢弃积压的菜单输入事件。

#### Scenario: 游戏帧无 egui 工作

- **GIVEN** 不带 UI 段的渲染帧与积压的菜单事件
- **WHEN** 渲染该帧
- **THEN** egui 不产生任何 GPU 提交，且菜单事件队列被清空

### Requirement: 菜单字体只经 ABI 上传一次

菜单渲染 SHALL 使用仓库内已授权（OFL-1.1、含 provenance）的 Noto Sans CJK SC 字体字节；`egui` 的 `default_fonts` feature MUST 保持关闭；字体 SHALL 经 client ABI 从 Go 侧一次性上传，Rust 侧不内嵌字体副本。

#### Scenario: 字体上传单一来源

- **GIVEN** 交互式客户端启动
- **WHEN** 渲染器创建完成
- **THEN** 菜单字体恰好上传一次，字节与 `internal/render/assets/NotoSansCJKsc-Regular.otf` 完全一致
- **AND** Rust 侧无内嵌字体二进制
