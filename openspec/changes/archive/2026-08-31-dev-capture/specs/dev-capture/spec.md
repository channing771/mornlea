# dev-capture Delta Spec

## Purpose

给运行中的交互式客户端提供默认关闭的本地捕获服务：外部调用方（agent 或用户）
经仅绑定回环地址的 HTTP 端点按需获取窗口完整合成截图（世界 + wgpu HUD +
WKWebView 菜单层）与有界短录屏帧序列，且帧循环在空闲时零额外开销、捕获失败
不产生静默假图。

## ADDED Requirements

### Requirement: 按需窗口合成截图

启用 `--dev-capture` 的交互式客户端 SHALL 提供本地捕获服务：`GET /screenshot`
MUST 返回当前窗口合成画面的 PNG（`image/png`），内容 MUST 包含窗口全部可见
图层；服务 MUST 仅绑定回环地址；未启用 `--dev-capture` 时客户端 MUST NOT
监听任何捕获端口，MUST NOT 写入端口发现文件。捕获桥报告不可用（如屏幕录制
授权缺失、平台捕获失败）时，服务 MUST 返回 503 与稳定中文错误 JSON，MUST NOT
返回无关内容冒充画面。

#### Scenario: 截图返回完整合成画面

- GIVEN 客户端以 `--dev-capture` 启动并处于菜单或游戏阶段
- WHEN 向 `GET /screenshot` 发起请求
- THEN 响应状态 200 且 Content-Type 为 `image/png`，解码后尺寸与窗口合成图
  一致，且画面为最近呈现帧的完整合成内容

#### Scenario: 未启用时无监听与无端口文件

- GIVEN 客户端未加 `--dev-capture` 启动
- WHEN 检查本机端口与 `~/.mornlea/dev-capture.json`
- THEN 不存在捕获服务监听，也不存在端口发现文件

#### Scenario: 捕获不可用映射为 503

- GIVEN 捕获桥返回捕获不可用状态
- WHEN 向 `GET /screenshot` 发起请求
- THEN 响应状态 503，错误 JSON 含稳定中文文案，客户端进程不崩溃、帧循环继续运行

### Requirement: 有界短录屏帧序列

捕获服务 SHALL 提供 `GET /record`：按 `fps` 采样窗口合成帧，返回 zip
（`application/zip`），内含 PNG 帧序列与 `manifest.json`（每帧时间戳与尺寸、
请求参数、实际帧数、丢帧数、GIF 有无）；`format=gif` 时 zip 额外包含
`preview.gif`。参数 MUST 满足 `0 < seconds ≤ 20`、`0 < fps ≤ 12`、
`seconds×fps ≤ 240`，越界 MUST 返回 400 与中文错误；采样 SHALL 以单 outstanding
请求推进（上一帧交付后才请求下一帧），帧间隔等待 MUST 发生在服务 goroutine 侧。

#### Scenario: 默认参数录制产出可解析帧序列

- GIVEN 客户端以 `--dev-capture` 启动
- WHEN 请求 `GET /record?seconds=2&fps=8`
- THEN 响应 200 且为合法 zip，manifest 声明的帧数与 zip 内 PNG 帧数一致，
  每帧尺寸与窗口合成图一致，帧时间戳单调不减

#### Scenario: 越界参数拒绝

- WHEN 分别请求 `GET /record?seconds=100`、`GET /record?fps=0` 与
  `GET /record?seconds=21&fps=12`（总帧 252 超上限）
- THEN 三次响应均为 400 与稳定中文错误 JSON，不产生任何帧捕获

#### Scenario: GIF 附加产物

- WHEN 请求 `GET /record?seconds=1&fps=4&format=gif`
- THEN zip 内同时包含 PNG 帧序列与可解码的 `preview.gif`，manifest 标记
  GIF 存在

### Requirement: 空闲零开销与生命周期清理

捕获泵 SHALL 按需执行：无待执行请求时帧循环 MUST NOT 调用捕获桥、MUST NOT
产生额外分配；有待执行请求时每帧 MUST 至多执行一次捕获，原始像素 MUST 立即
移交（非阻塞、发送后不可变），PNG 编码与 zip 打包 MUST 发生在帧循环之外。
服务启动时 MUST 写入 `~/.mornlea/dev-capture.json`（pid、实际端口、启动
时间），进程优雅退出时 MUST 清除该文件。帧循环已停止时，挂起的请求 MUST 在
有限时间内以 503 失败，MUST NOT 永久挂起。

#### Scenario: 空闲帧零捕获调用

- GIVEN 服务已启用但无任何在途请求
- WHEN 帧循环连续运行多帧
- THEN 捕获桥调用次数为零、无额外分配，帧循环仅新增一次非阻塞的待办检查
  与每帧常数次非阻塞原子状态发布（`/status` 观察快照，发布不触碰请求通道
  与捕获路径）

#### Scenario: 端口文件写入与清理

- WHEN 服务启动后进程优雅退出
- THEN `~/.mornlea/dev-capture.json` 在运行期间存在且内容含 pid 与实际端口，
  退出后不存在

#### Scenario: 循环停止后请求限时失败

- GIVEN 帧循环已退出而服务仍在
- WHEN 向 `GET /screenshot` 发起请求
- THEN 在服务超时上限内收到 503，请求不永久挂起

### Requirement: 窗口合成捕获桥 ABI 契约

窗口合成捕获 MUST 经 client ABI 唯一桥接：`mornlea_client` 新增导出 SHALL
校验版本、句柄、指针与容量，失败时不触碰输出缓冲、panic 不得跨越 FFI 边界；
输出容量不足时 SHALL 以溢出状态返回所需字节数（两段式协议），输出为 BGRA8
原始像素。只有 `internal/client` MAY 接触该导出；头文件宏、Rust 版本常量、
Go 侧钉位与根 `AGENTS.md` 基线 MUST 同批升至 client ABI v13。

#### Scenario: 容量不足走两段式溢出

- GIVEN 输出缓冲容量小于窗口合成图所需字节数
- WHEN 调用捕获导出
- THEN 返回溢出状态，所需字节数写入出参，输出缓冲保持调用前内容；以足量缓冲
  重试后返回完整 BGRA8 像素与实际宽高

#### Scenario: 版本失配拒绝

- GIVEN 调用方传入的 ABI 版本与动态库不一致
- WHEN 调用任意捕获导出
- THEN 返回版本失配状态码，输出缓冲不被触碰

#### Scenario: 基线同批钉位

- GIVEN 头文件宏已升 v13
- WHEN 运行基线与钉位门禁
- THEN Rust 版本常量、Go `ClientABIVersion` 钉位与根 `AGENTS.md` 基线行均为
  13，任一不同步即门禁红
