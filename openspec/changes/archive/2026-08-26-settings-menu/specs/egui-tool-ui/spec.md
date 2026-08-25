## MODIFIED Requirements

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

## RENAMED Requirements

- FROM: `### Requirement: client ABI v8 扩展`
- TO: `### Requirement: client ABI v9 结构化设置事件扩展`
