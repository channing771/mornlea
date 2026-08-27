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

`mornlea_client` ABI SHALL 提升到 v9：保留字体上传与帧 TLV tag 9，UI 下行保留 layout v1 主菜单并新增 layout v2 设置页；UI 上行 SHALL 由裸按钮 ID 序列升级为有版本、有类型、有长度且顺序稳定的结构化事件批。ABI 版本不匹配 MUST 在所有出口被拒绝，v8 与 v9 二进制不可混装。

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

### Requirement: 调试面板 layout v3 段

UI 下行 SHALL 除 layout v1 主菜单与 v2 设置页外接受 layout v3 调试面板状态段，编码为定宽行记录：段头（layout=3 + 可见 flags + 模式名 + 行计数）后接每行定宽记录（标签 ≤24 字节、值 ≤24 字节、每行 flags 编码只读/选中/可编辑/编辑态）。未知或非法 layout MUST 被拒绝。调试面板行数上限 `64`、读数 `7`、每行标签/值 `24` 的既有约束 MUST 保留；段长度 MUST 受既有 `MAX_UI_SEGMENT_BYTES` 上界约束。

#### Scenario: 合法 layout v3 段被接受

- **GIVEN** 布局合法、flags 合法、行数与各类长度合法的 layout v3 段
- **WHEN** 渲染该帧
- **THEN** MUST 成功，面板状态按段呈现

#### Scenario: 非法 layout v3 段被拒绝

- **GIVEN** 未携带 UI 或 UI 段携带未知 layout、空模式名、行数超上界、标签/值超 `24` 字节或截断字节
- **WHEN** 渲染该帧
- **THEN** MUST 返回 `INVALID_ARGUMENT` 且不触碰渲染器 UI 状态

### Requirement: 调试面板事件上行

UI 上行结构化事件批 SHALL新增调试面板动作类型：选中行移动、进入编辑、编辑值输入、确认写回、取消编辑、关闭面板。每帧事件批 SHALL 按 egui 产生顺序依次传递，Go 侧 MUST 依序消费。

#### Scenario: 编辑序列事件顺序稳定

- **GIVEN** 同一 egui 帧用户先移动选中行、再进入编辑、再输入值、再确认
- **WHEN** Go 排空结构化事件批
- **THEN** 移行、编辑、值输入、确认事件 MUST 按顺序出现

#### Scenario: 未知调试动作被拒

- **GIVEN** 上行事件批含未知调试面板动作类型
- **WHEN** 渲染该帧
- **THEN** 事件批 MUST 被拒绝

### Requirement: 游戏内 Esc 打开暂停覆盖层

游戏中（世界已装配相位）按 Esc SHALL 打开暂停覆盖层并释放光标；覆盖层只含「返回游戏」「退回主菜单」两个按钮。再次按 Esc 或点击「返回游戏」SHALL 关闭覆盖层、重新捕获光标并回到游戏，重复触发 MUST 只生效一次。

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

