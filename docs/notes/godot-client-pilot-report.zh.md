---
doc_id: godot-client-pilot-report
doc_revision: 2026-09-19.1
language: zh-CN
counterpart: godot-client-pilot-report.md
---

# Godot 客户端试点双端报告

本文记录 `pilot-godot-client-migration` 的 P7 双端评审：把现有 Rust/WebView 客户端与 Godot Python 主路径试点并排对照。Godot 路径不是默认入口。

Decision: GO

## 运行身份

| 字段 | 既有 Rust 客户端 | Godot 试点 |
|---|---|---|
| 机读报告 | `testdata/godot-pilot/legacy-rust-memory-v23.json` | `testdata/godot-pilot/godot-pilot-v23.json` |
| Git commit | `84f3e0e75dff6987e47cf2aa50f50d888e85eafb`（P0 基线） | `376f435a262bfc8fa611866dedfee8c56225444f` |
| Worktree | dirty（P0） | dirty |
| 协议 | protocol v44 | protocol v44 |
| Engine ABI | engine ABI v11 | engine ABI v11 |
| Client ABI | client ABI v19 | client ABI v19（试点不使用） |
| Client-core ABI | 不适用 | v1 |
| Benchmark | benchmark scenario v23 | 复用 v23 窗口 |
| Godot | 不适用 | 4.7.2-stable |
| Py4Godot | 不适用 | 4.7-alpha21，源修订 `d8e17428deeb0428587349b663f6da26cd71ef3a` |
| CPython | 不适用 | 嵌入式 3.14.4 |
| 目录 | 不适用 | `apps/mornlea-godot/config/feature_catalog.tres` |
| 平台 | darwin/arm64，macOS 26.6.2，Apple M2，Metal 4 | darwin/arm64，macOS 26.6.2，Apple M2 (Apple8)，API 4.0 |
| 分辨率 | 2560×1440 | 请求 2560×1440 |
| 视距 | 32 | 2（登录下限；连接族目前只带地址） |
| 种子 | 20260726 | 20260726 |
| 采样窗口 | warmup 10s、still 60s、flying 120s、cooldown 30s、至少 128 个 GPU 样本 | 请求相同窗口；CPU 样本 25,062 |
| 视觉证据 | 已跟踪 `testdata/visual-golden/{ui,world,motion}` | 未跟踪 `build/visual/godot-pilot/<run-id>/` |

## 视觉证据

Godot 试点从确定性地形转录以固定相机抓取一帧 640×360 世界图（`world/terrain-settled.png`），身份完整。Godot 4.7 的 headless 显示驱动只有 dummy 渲染器，因此抓帧使用无焦点、最小化的 macOS Metal 窗口，而不是前台窗口。

`make godot-visual-compare` 按既有双阈值（通道差上限 2、差异像素比 0.0001）分类全部已跟踪基线，且未写入 `testdata/visual-golden/`，也未创建 `testdata/visual-golden/godot/`：

- 31 个 `ui/` 部件：not-covered（UI 生产者仍是 WebView）
- 31 个 `world/` 场景：not-covered（世界生产者仍是离屏 Rust 抓帧；试点帧是转录地形，不是 `captureScenes` 世界）
- 11 个 `motion/` GIF：human-review-only（不参与自动像素比对）

## 性能

下列数值只记录。overflow 与失败上传仍是硬失败，本次均为 0。

| 指标 | 既有 Rust v23 | Godot 试点 | 判定 |
|---|---|---|---|
| CPU 帧 P50/P95/P99（ms） | still 5.998 / 6.431 / 7.257；flying 1.676 / 8.007 / 20.211 | 6.896 / 6.944 / 16.667（25,062 样本） | 只记录；既有 flying P99 同样不改退出码 |
| GPU 帧 P50/P95/P99 | 128 个 GPU 完成样本 | 不可比：最小化 Metal 窗口未暴露 GPU 时间戳 | 不得写成 0 |
| Python apply P50/P95/P99（ms） | 不适用 | 0.506 / 0.769 / 1.206 | 主线程 apply 有界 |
| Python process P50/P95/P99（ms） | 不适用 | 1.074 / 1.712 / 2.746 | 宿主 `_process` 有界 |
| 分配压力 | 不适用 | P50/P95/P99 = 53/56/58（每帧 allocated-block 增量，记在分位字段） | 记录解释器压力 |
| 冷启动 | load_seconds 43.807 | 到 Play 419.7 ms | 加载判据不同；不得用更少区块伪装更快 |
| 紧凑/展开字节 | 紧凑上传 | 760,056 紧凑，19,001,400 展开（约 25 倍） | 符合 CPU 展开 |
| 预备/上传（ms，累计） | 不适用 | 预备 108.0，上传 1003.5 | 只记录 |
| RID / mesh | 不适用 | 184 个 live RID，92 个区段，峰值 184 | 禁止每方块一个 Node |
| RSS 峰值（字节） | still 1,614,807,040；flying 1,735,655,424 | 581,484,544 | 记录；含eager 加载的 `libmornlea_client.dylib` 框架 |
| 输入到呈现 P50/P95/P99（ms） | 既有显式探针 | 6.896 / 6.944 / 7.407（17,265 次 look 样本） | 不把服务器 tick 算作引擎差 |
| Overflow / 失败上传 | 硬失败 | 0 / 0 | 非零即硬失败 |

与 Memory v23 观察者无法对齐的字段（权威 tick P99、协议编解码、玩家持久化、八会话探针、GPU 完成时间、视距 32）在 Godot JSON 中标为不可比或省略，而不是写成 0。

## 功能覆盖

试点已覆盖：远程 TCP 登录、有界 step、近环地形、桌面键鼠语义输入、可纠正相机、目标描边、远端玩家池、已确认 HUD、环境颜色映射、断开/复位。

未覆盖：本地 Memory 装配、完整菜单/容器、持物、粒子、远环、高级天气、音频设备、生产抓帧/benchmark 生产者、默认客户端切换。

## 差异分类

1. 必须等价：协议、镜像、预测、溢出失败即停。由 runtime/转录测试与零 overflow 守住。
2. 允许在批准范围内不同：抗锯齿、色调映射、字体光栅化。未做像素比对，也没有生产者移交。
3. 试点不覆盖：见功能覆盖。全部已跟踪 UI/world 基线仍由既有生产者持有。

## 终审

| 硬条件 | 结果 | 证据 |
|---|---|---|
| 精确钉死的 Py4Godot/CPython 产物 | GO | 钉死修订、补丁序列、离线资格 |
| 不依赖系统 Python 或运行时安装 | GO | 隔离 `PyConfig`、污染环境测试 |
| 无权威/协议/预测分叉 | GO | 共用 Go runtime 与 protocol v44 |
| 无静默丢失或部分批次 | GO | overflow_count 0，uploads_failed 0 |
| 地形与最小实体稳定 | GO | 地形检查、300s playable smoke、v23 窗口 92 个 live 区段 |
| GDExtension/Go core 重复关闭 | GO | 100 次隔离 smoke |
| Python 主线程成本有界 | GO | apply P99 1.21 ms，process P99 2.75 ms |
| 报告身份完整 | GO | `testdata/godot-pilot/godot-pilot-v23.json` 通过校验 |
| GPU 时间戳对比 | Evidence Missing | 字段标为不可比，未写 0 |
| 视距 32 对齐 | Evidence Missing | 连接族只带地址；登录视距为 2 |
| 生命周期崩溃 | GO | smoke、抓帧与 v23 窗口运行无崩溃 |

未观察到未资格运行时、系统 Python、无界 Python 热路径、数据丢失、权威分叉、缺失身份或生命周期崩溃。GPU 与视距缺口记录为后续工作，而不是被零值掩盖。

## 后续

P8–P14 已拆成候选 OpenSpec 变更。默认 `mornlea` 入口未切换。
