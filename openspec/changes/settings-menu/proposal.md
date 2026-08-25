## Why

当前图形客户端已经具备 egui 主菜单基础，但「设置」仍是禁用占位；玩家只能退出程序并手工编辑 JSON，无法在进入世界前安全调整音量、材质包目录或窗口大小。D-01 将这条占位入口收敛成一个可保存、可取消、失败不产生部分副作用的首版设置闭环。

## What Changes

- 启用普通本地主菜单的「设置」按钮，新增独立设置页，首批暴露 `audioVolume`、`texturePackPath` 与固定 16:9 `windowSize` 预设（`640x360`、`960x540`、`1280x720`）。
- 设置页采用 Go 拥有的 committed/draft 状态：编辑只改草稿；保存成功后才原子落盘并应用音量和窗口大小；材质包保持启动加载，候选目录先完整校验、保存后明确提示下次启动生效。
- 增加取消与返回保护：取消恢复已保存值；有未保存修改时返回或 Escape 不静默丢弃；加载、校验或保存失败时保留草稿并显示有界错误，磁盘和当前运行态均不改变。
- 配置版本保持 `1`，新增可选顶层 `windowSize`；旧配置缺席时取 `1280x720`，非法显式值报带字段上下文的错误，专用服务端解析但不消费该客户端字段。
- **BREAKING（仅畸形本机配置）**：`texturePackPath` 统一限制为最多 1024 个 UTF-8 字节；此前可被 JSON 解析但无法形成 Darwin 可用文件系统路径的超长值将改为带字段上下文的加载错误，用户迁移方式是缩短或清空该字段。
- **BREAKING（仅进程内 client ABI）**：`mornlea_client` client ABI v8 升到 v9。UI 下行保留 layout v1 主菜单并新增 layout v2 设置页；UI 上行由裸按钮 `u32` 序列改为有界结构化事件批，容量不足不得先排空或静默截断。线上游戏协议、engine ABI 与存档 schema 均不变。
- 新增 `settings-menu` 无窗口视觉场景，并因「设置」按钮启用更新 `main-menu` golden；其余既有 golden 必须逐字节不变，`far-horizon` 继续倒数第二、`water-underwater` 继续唯一末项。

非目标：不做全屏、VSync、显示器选择、任意窗口尺寸、UI 缩放、键位、FOV/视距、暂停菜单、原生目录选择器或剪贴板；不热重载材质、不重建当前 application；不改变 `-connect`/benchmark/capture 的启动隔离语义，不触碰线上 wire、世界/玩家/伙伴存档、engine ABI 或 benchmark workload。

## Capabilities

### New Capabilities

- `settings-menu`: 定义设置页导航、草稿、校验、原子保存、即时/重启生效、失败原子性与有界输入行为。

### Modified Capabilities

- `egui-tool-ui`: 主菜单「设置」由禁用变为启用，并以 client ABI v9 的 layout v2 与结构化事件批承载设置表单。
- `texture-pack-loading`: 允许世界尚未启动的设置保存动作读取并全成全败校验已变化的候选目录，但当前进程材质仍不可变且只在下一次启动装载。
- `rust-client-window`: 增加由固定配置预设决定的初始与运行期窗口内容尺寸，并继续受显示器工作区与物理帧缓冲上限约束。
- `visual-verification`: 新增 `settings-menu` 正式场景、更新 `main-menu` 启用态，并锁定其余场景与尾序。

## Impact

- 配置与入口：`internal/config`、`cmd/mornlea` 的选项解析、应用构造、菜单循环、音频生命周期与窗口尺寸接线。
- Go/Rust UI 边界：`internal/client`、`engine/crates/mornlea_client`、`engine/include/mornlea_client.h`；client ABI 升为 v9，旧/新动态库不可混装。
- 视觉与文档：capture 场景、`main-menu.png`、新增 `settings-menu.png`、README、`AGENTS.md`/`CLAUDE.md` 与 `docs/notes/progress.md`。
- 兼容性：配置 JSON 仍为 v1，普通旧文件无需迁移；唯一收紧是上述超长 `texturePackPath`，可通过缩短或清空字段恢复。协议保持 v26，engine ABI v6、区块 schema v9、玩家 schema v7、世界 metadata v2、`companions.ai` schema v4 与 benchmark scenario v19 均不变。
- 并发与性能：设置 I/O/材质校验只发生在世界未启动的菜单动作，不进入权威 tick、网络或游戏渲染热路径；benchmark 继续不上传字体、不生成 UI 段、不运行 egui。
