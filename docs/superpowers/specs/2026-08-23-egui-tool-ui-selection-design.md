# 游戏内工具型 UI 层技术选型：egui

日期：2026-08-23

状态：选型已裁决，实施 OpenSpec change 尚未创建。按 `AGENTS.md` 纪律，「项目定位」基线只陈述已交付能力，本选型在首个 egui change 交付归档前不写入 `AGENTS.md`/`CLAUDE.md`；归档时随基线同步。

## 背景

进入游戏后的全部 UI 今天都是程序化 quad 体系：HUD（快捷栏、生命/饥饿/氧气条）、容器 overlay（背包/合成/熔炉/箱子）、聊天与 debug 面板由 Go CPU 半部做布局与编码，Rust `mornlea_client`（winit 0.30 + wgpu 29，client ABI v7）绘制，固定上传容量（267 quad / 700 glyph / 46912 bytes）。这套体系适合像素风的常驻 HUD，但每新增一个窗口型界面都要手写布局、命中测试、焦点、滚动与文本输入等通用控件逻辑，开发效率成为瓶颈。

本选型为「工具型 UI」——进入游戏后的菜单、设置、调试面板等窗口型界面（不含常驻 HUD 与容器格子交互）——确定唯一技术栈。选型前提是仓库硬边界不可松动：

- GPU 渲染只经 Rust `mornlea_client`；任何 Go 包不得导入 WebGPU/GUI 绑定（`internal/archcheck` 全仓禁止）。
- 无窗口 capture 与 benchmark 路径必须继续可用；自动测试不启动前台窗口。
- 不加入 Mojang 版权材质或其他未经授权的二进制美术资源。
- 跨组件/ABI 变更必须走 OpenSpec change。

## 候选调研（2026-08 时点）

| 候选 | 模式 | 许可证 | 结论 |
|---|---|---|---|
| egui 0.36.1（2026-08-07 发布） | 即时模式 | MIT OR Apache-2.0 | 采用 |
| iced 0.14.0（2025-12-07 发布，wgpu 默认后端） | 保留模式（Elm） | MIT | 否决 |
| Slint 1.12+（1.12 起才有 wgpu 后端） | 声明式 `.slint` | GPLv3 / Royalty-Free / 商业三重 | 否决 |
| RAUI | 保留模式、面向游戏 | MIT | 否决 |
| bevy_ui / Feathers | 保留模式 | MIT | 不可行（绑定 Bevy 引擎） |
| Go 侧 GUI（Gio / Fyne / nucular） | — | — | 不可行（违反 GPU 只经 `mornlea_client` 的边界） |
| HTML/webview 叠层（wry 等） | Web | — | 否决（输入穿透、合成时序与无窗口路径冲突） |

## 方案选择

### 采用：egui 作为工具型 UI 层唯一技术栈

- 只引入 `egui` + `egui-wgpu` 两个 crate，**不引入 `egui-winit`**：winit 保持在 0.30，菜单输入由现有 winit 事件翻译手工构造 `egui::RawInput` 喂入，避免 egui-winit 对更新 winit 的版本耦合；`egui` + `egui-wgpu` 的 wgpu 依赖必须与 `mornlea_client` 的 wgpu 29 同线（见集成约束第 1 条）。
- egui 以附加 wgpu render pass 挂进 `mornlea_client` 既有管线，排在既有 HUD pass 之后；不新建窗口、surface 或事件循环，不接管游戏输入，指针/键盘事件按焦点规则在 egui 与游戏之间分发。
- UI 语义状态仍在 Go：Go 持有权威镜像数据（物品、设置项、聊天缓冲），经 client ABI（v7 → v8）喂给 Rust 呈现；菜单事件经同一 ABI 回流 Go。Rust 侧不产生游戏语义。
- 既有程序化 HUD 与容器 overlay **不迁移**：快捷栏、生命/饥饿/氧气条、容器格子、聊天呈现与 name tag 继续走 quad 管线；egui 只服务新工具型窗口。
- 字体政策：关闭 egui `default_fonts` feature（内嵌 Ubuntu 字体不进二进制），以开源许可（OFL 一类）字体自备或继续程序化生成，满足二进制资产红线。

理由：egui 是 Rust 游戏叠加/工具 UI 的事实标准，`egui-wgpu` 活跃维护；即时模式免自研布局、命中、焦点、滚动与文本输入；可作为库嵌入既有 wgpu 渲染器而不交出窗口所有权；MIT OR Apache-2.0 无 attribution 义务。

### 否决：iced

Elm 架构假设库拥有应用主循环与状态树，嵌入「渲染器是 cdylib、游戏状态在 Go」的结构成本远高于 egui；像素风还要自写 theme/widget，即用性优势归零。

### 否决：Slint

wgpu 后端 1.12 才落地；Royalty-Free 许可含 attribution 义务、与 GPLv3/商业许可三轨并存，合规判断成本高；事件循环所有权倾向自有，嵌入成本高。

### 否决：RAUI

面向游戏 UI 的定位契合，但社区规模与文档远小于 egui，长期维护风险与收益不匹配。

### 否决：bevy_ui / Feathers

绑定 Bevy 引擎整体，本仓不使用 Bevy，引入等于更换引擎。

### 否决：Go 侧 GUI 库（Gio / Fyne / nucular）

都需要自建 GPU/窗口上下文，直接违反「GPU 只经 Rust `mornlea_client`、Go 包不得导入 GPU 绑定」的 archcheck 门禁。

### 否决：HTML/webview 叠层

透明合成与输入穿透时序复杂、与无窗口 capture/benchmark 路径冲突、离线包体与像素风割裂。

## 集成约束（后续实施 change 必须满足）

1. **wgpu 版本线对齐**：`mornlea_client` 的 wgpu = 29 必须与 Go 绑定内嵌的 wgpu-native v29 同主版本线（`engine/crates/mornlea_client/Cargo.toml` 既有注释约束）。引入前先验证所选 egui/egui-wgpu 版本声明的 wgpu 依赖与 29 兼容；不兼容时以版本选择或 patch 解决，绝不单侧升级 wgpu。
2. **client ABI v7 → v8**：菜单状态下行与菜单事件上行都走版本化 ABI 扩展；跨 goroutine 发送后的消息及切片不可变等既有约定沿用；无图形专服不受影响。
3. **capture/benchmark**：含菜单的 capture 新场景遵守场景表既有顺序约束（`water-underwater` 仍居末位）；benchmark scenario 版本随上传布局或场景变化 bump；性能数值只记录，报告完整性、真实 overflow 与数据丢失门禁不变。
4. **风格边界**：egui 是矢量抗锯齿渲染，与像素 HUD 并存允许风格差异；像素风调参限于 egui Style 与字体配置，不 hack 渲染器。
5. **测试纪律**：菜单交互验证走无窗口 capture 与 ABI 层测试，自动测试不启动前台窗口。

## 实施路径

1. 按本设计创建首个 OpenSpec change（建议名 `add-egui-tool-ui-foundation`）：依赖引入与 wgpu 29 对齐验证、`egui::RawInput` 桥、附加 render pass、ABI v8 扩展、一个最小菜单竖切、capture 场景与 scenario bump。以 subagent-driven-development 执行。
2. 后续每个具体菜单（设置、暂停、调试面板等）各自独立 change，逐个归档进主规格。

## 参考

- egui：<https://github.com/emilk/egui>、<https://crates.io/crates/egui-wgpu>（0.36.1，2026-08-07）
- iced 0.14.0 changelog：<https://github.com/iced-rs/iced/blob/master/CHANGELOG.md>
- Slint wgpu 支持：<https://slint.dev/>、<https://www.reddit.com/r/rust/comments/1lctp0s/gui_toolkit_slint_112_released_with_wgpu_support/>
- 社区对比：egui vs iced 游戏集成 <https://users.rust-lang.org/t/egui-vs-iced-in-regards-to-game-engine-integration/74569>
