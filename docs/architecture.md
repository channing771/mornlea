# Mornlea 当前架构

## 1. 系统总览

Mornlea 由 Go 应用与两个 Rust `cdylib` 组成。Go 持有应用装配、世界与权威模拟、协议与传输、存储、客户端镜像，以及渲染的 CPU 布局和上传编排；Rust `mornlea_engine` 提供无窗口数值内核，Rust `mornlea_client` 提供 Darwin 窗口、事件和 GPU 后端。Go 与 Rust 只经既定 C ABI bridge 协作，不存在可切换的第二套生产实现。

## 2. 服务端权威与客户端镜像

服务端是世界、玩家、伙伴、库存和玩法状态的唯一权威。客户端提交输入意图，保存经过验证的权威镜像，并可为本地响应进行可纠正的预测；预测结果不能反向确认服务端结算。

权威状态变更在服务端 tick 内按固定阶段串行结算，再通过协议发布。客户端渲染和 UI 只消费镜像与本地呈现状态，不直接读取或修改权威模拟。

## 3. Memory/TCP 共用路径

普通本地游戏使用 Memory transport，远程游戏使用 TCP transport；两者复用同一 packet/codec 契约、登录状态机、会话装配和权威模拟。Memory 只改变传送介质，不提供绕过登录、输入校验或服务端裁决的同进程特权路径。

`internal/network` 负责有界消息传送和连接生命周期，不决定业务结果。Host 与模拟层消费相同的已验证命令，因此本地和远程模式共享行为语义。

## 4. Go 包职责与 archcheck 依赖边界

- `cmd/mornlea` 和 `cmd/mornlea-server` 负责应用入口与资源生命周期装配。
- `internal/world` 持有区块、section、容器和掉落物等世界数据模型。
- `internal/sim` 持有权威 tick、规则结算和世界变更编排。
- `internal/network` 持有 packet、codec、登录状态机与 Memory/TCP transport。
- `internal/storage` 持有世界、玩家和伙伴数据的编码、迁移、恢复与磁盘生命周期。
- `internal/server` 装配 Host、会话、权威模拟与持久化 worker。
- `internal/client` 持有客户端镜像、输入预测、消息接收、client ABI bridge 和渲染侧 CPU 编排。
- `internal/render`、`internal/mesh`、`internal/assets`、`internal/lod` 与 `internal/worldgen` 持有领域数据描述、CPU 编码和 Rust 调用编排，不拥有 GPU 后端或第二套数值生产实现。
- `internal/nativeabi` 是 engine ABI 的唯一 Go bridge。

内部包允许的直接依赖以 `internal/archcheck/dependency_test.go` 的 `allowed` 表为准。`internal/archcheck` 同时守住无 WebGPU Go 依赖、无图形专服闭包和长期版本基线；架构文档不复制会随包演进的依赖白名单。

## 5. `mornlea_engine` / engine ABI v7

`mornlea_engine` 是 mesh/light、collision、raycast、physics tick 积分与 worldgen 的唯一生产实现。该 crate 保持无窗口，不拥有权威世界状态，不执行文件或网络 I/O，也不承载伤害、库存、权限或 tick 编排等业务规则。

engine C ABI 当前为 v7。Go 侧只有 `internal/nativeabi` 可以接触该 ABI；领域包构造语义输入并解码结果。header、Rust FFI、Go bridge、ABI 版本和跨语言一致性检查必须成套演进，调用结束后任一侧都不得保留对方指针。

## 6. `mornlea_client` / client ABI v9

`mornlea_client` 持有 Darwin 窗口与事件采集、egui、GPU 资源、shader、render pass、窗口 surface 和离屏渲染。Go 不导入 WebGPU 绑定，只通过 `internal/client` 提供的 client ABI bridge 使用窗口和 renderer 领域接口。

client C ABI 当前为 v9，并与 engine ABI 独立演进。header、Rust FFI、`internal/client` bridge、版本和跨语言检查必须同步更新；失败或容量不足不能发布部分输出。

## 7. 图形客户端与无图形专服 release unit

Darwin 图形客户端 `mornlea` 同时依赖同一次构建的 `mornlea_engine` 与 `mornlea_client`，不能跨构建混装 ABI 组件。无图形专服 `mornlea-server` 不依赖 `mornlea_client`、窗口或 GPU 栈，但权威物理和空间查询仍依赖同次构建的 `mornlea_engine`。

Linux 专服发布单元由 `mornlea-server` 与相邻的 `libmornlea_engine.so` 组成，并通过 `$ORIGIN` 加载。二者必须作为一个不可跨版本混装的 release unit 分发。

## 8. 并发、数据和热路径约束

- 跨 goroutine 发送成功后的消息及其 slice 视为不可变；后续修改必须复制。
- 权威 tick、渲染和网络热路径只执行有界工作，不阻塞磁盘、网络、模型调用或其他重 CPU 工作。
- 重工作通过有界队列、不可变快照或 worker 离开热路径，并在所有权清晰的边界汇合结果。
- 协议、存档和 FFI 入口先校验类型、长度、计数、容量与版本，再分配、遍历或写输出。
- overflow、数据丢失、报告身份不完整和 I/O 错误必须显式失败，不能静默截断或吞错。

## 9. 行为规格、实现进度和历史设计入口

当前可观察行为以代码、测试和 [`openspec/specs/`](../openspec/specs/) 为准。正在实施的变更位于 [`openspec/changes/`](../openspec/changes/)，流程见 [`docs/openspec.md`](openspec.md)。

实现编年史见 [`docs/notes/progress.md`](notes/progress.md)。`docs/superpowers/` 与 [`openspec/changes/archive/`](../openspec/changes/archive/) 保存历史设计和 change 证据，不覆盖当前架构与行为主规格。完整入口见 [`docs/README.md`](README.md)。

## 10. 仓库目录导览

```text
.
├── cmd/
│   ├── mornlea/             游戏客户端与内置服务端装配
│   ├── mornlea-server/      无图形 TCP 专用服务端
│   ├── mornlea-agent-board/ AI 工作者执行状态 Web 看板
│   ├── gfxspike/            Rust renderer 地形渲染验证程序
│   └── perfcheck/           性能报告比较工具
├── engine/
│   └── crates/
│       ├── mornlea_engine/  固定 Rust 1.97.1 cdylib：mesh/light/collision/raycast/physics/worldgen
│       └── mornlea_client/  Darwin 窗口、事件循环与全部 GPU 渲染（含 egui 菜单/面板页）
├── internal/                包职责见 §4，依赖白名单以 archcheck 为准
│   ├── core/                公共领域类型与 native raycast batch 驱动
│   ├── companion/           独立伙伴身份、静态定义与身体类型
│   ├── profile/             本机稳定玩家身份与档案
│   ├── config/              共享 JSON 配置加载与校验
│   ├── logging/             模块化日志
│   ├── audio/               Darwin 本地程序化提示音
│   ├── fluid/               有界权威流体更新队列
│   ├── lod/                 远环 tile 调度与 CPU 编码
│   ├── world/               区块和世界数据模型
│   ├── worldgen/            worldgen seed→perm 播种、Rust 调用与区块回写
│   ├── physics/             玩家运动与碰撞
│   ├── sim/                 权威世界模拟
│   ├── server/              服务端 Host、会话、发布与玩家持久化
│   ├── network/             二进制协议、登录状态机与 Memory/TCP 传输
│   ├── storage/             世界、区域文件与玩家状态持久化
│   ├── client/              输入、相机、预测、窗口/client ABI 与客户端镜像
│   ├── mesh/                区块网格生产 API（实现位于 Rust cdylib）
│   ├── nativeabi/           engine C ABI 的唯一 Go bridge
│   ├── render/              渲染 CPU 半部：布局、编码与上传调度
│   ├── assets/              方块定义与程序化材质
│   └── archcheck/           内部包依赖方向门禁测试
├── scripts/agent-hooks/     Claude Code 与 Codex 共用的自动 Hook 守卫
└── docs/                    设计、实施计划、性能记录与实现进度
```
