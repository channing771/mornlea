# client-webui-pixel-style Delta Spec

## Purpose

把 WebView 四面板（主菜单、设置、暂停、调试面板）的呈现统一到
pixel-retroui 组件与 tokens.css 像素令牌之上，并引入覆盖 CJK 的开源像素
字体作为 UI 首选字体；所有既有行为契约（文案、桥事件、焦点与键盘语义、
令牌纪律）零改动。

## ADDED Requirements

### Requirement: 面板统一像素组件风格

四面板的可交互元素 SHALL 经统一像素呈现层呈现：按钮与表单控件 MUST 使用
pixel-retroui 组件或按 `tokens.css` 像素令牌重绘的自绘控件；颜色、几何与
阴影 MUST 经 `tokens.css` 令牌供给（琥珀为唯一强调色、危险红仅用于错误、
`prefers-reduced-motion` 下动效归零）；文案、上行事件、焦点顺序与键盘
语义 MUST 与改造前逐项一致；音量控件 MUST 保持滑块形态与 `[0,1]` 映射。

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
