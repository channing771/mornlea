## MODIFIED Requirements

### Requirement: 当前项目身份统一为 Mornlea

系统 MUST 以 `Mornlea` 作为当前产品名，以 `github.com/channing771/mornlea` 作为 Go module，以 `mornlea`、`mornlea-server` 与 `mornlea-companion-agent` 作为客户端、专用服务端与伙伴 Agent 服务命令。Python distribution、包元数据、日志 module 与帮助文本 MUST 使用 Mornlea 身份，不得恢复旧 `mcgo`/`mcgod` 身份。

#### Scenario: clean checkout 构建当前原生入口

- **WHEN** 在 Apple Silicon/macOS 执行 canonical Go/Rust build
- **THEN** MUST 生成 `bin/mornlea`、`bin/mornlea-server` 与同目录 `libmornlea_engine.dylib`

#### Scenario: clean checkout 安装 Agent 服务入口

- **GIVEN** Python 3.12 与 uv 可用
- **WHEN** 在 `services/companion-agent` 执行 locked sync
- **THEN** `mornlea-companion-agent serve --config` MUST 可寻址并以 Mornlea 包身份报告版本/帮助

#### Scenario: Linux 专服发布为同目录 bundle

- **WHEN** 在 Linux amd64 原生执行 canonical server build
- **THEN** MUST 生成同目录 `mornlea-server` 与 `libmornlea_engine.so`
- **AND** 两者 MUST 作为一个不可混装的发布单元升级；Python Agent 服务 SHALL 独立安装和启动，不得混入 native bundle

#### Scenario: 旧入口不再发布

- **WHEN** 枚举当前 module、命令、native ABI、构建与 Agent 服务身份
- **THEN** MUST 不存在 `mcgo`/`mcgod` wrapper、旧 `mcgo` C symbol、`libmornlea_mesh.dylib`、`libmornlea_mesh.so` 或旧环境变量 fallback
- **AND** additive ABI v1 的 `mornlea_mesh_section` MUST 继续保留

## ADDED Requirements

### Requirement: Agent 抽离只升级 companions.ai 版本

本 change 完成时，游戏协议 MUST 保持 v32、玩家 schema v8、区块 schema v9、世界 metadata v3、`hostile_mobs` schema v1、engine ABI v9、client ABI v14 与 benchmark scenario v21；本 change 自身 SHALL 只把 `companions.ai` 从 schema v4 升到 v5，client ABI v13→v14 与 benchmark scenario v20→v21 的升版来自 main 同步而非本 change 的交付行为。Agent HTTP application contract SHALL 为 v1，Go MCP application tool contract SHALL 为 v1；二者不是游戏 wire 或 native ABI 版本。

#### Scenario: 基线版本矩阵只变化一个存档域

- **GIVEN** 实现前版本矩阵与本 change 完成后的构建
- **WHEN** 运行版本一致性和协议/存档/ABI 钉死测试
- **THEN** 除 `companions.ai` 为 v5 外，协议 v32、玩家 v8、区块 v9、metadata v3、hostile v1、engine ABI v9、client ABI v14 与 scenario v21 MUST 逐项不变
- **AND** client ABI 与 benchmark scenario 的升版（v13→v14、v20→v21）MUST 只来自 main 同步，本 change 自身的交付行为 MUST 不触发这两项升版

#### Scenario: Agent contract 不触发游戏协议升版

- **GIVEN** Agent HTTP v1 与 MCP tool v1 已启用
- **WHEN** Memory 与 TCP 客户端登录并交换现有伙伴消息
- **THEN** 两种 transport MUST 继续使用协议 v32 与相同 wire bytes，客户端 MUST 不感知 Agent 内部合同
