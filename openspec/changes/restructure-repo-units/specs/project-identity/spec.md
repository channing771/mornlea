## MODIFIED Requirements

### Requirement: 当前项目身份统一为 Mornlea

系统 MUST 以 `Mornlea` 作为当前产品名，以 `github.com/channing771/mornlea` 作为 Go module，以 `mornlea`、`mornlea-server` 与 `mornlea-companion-agent` 作为客户端、专用服务端与伙伴 Agent 服务命令。Python distribution、包元数据、日志 module 与帮助文本 MUST 使用 Mornlea 身份，不得恢复旧 `mcgo`/`mcgod` 身份。

#### Scenario: clean checkout 构建当前入口

- **WHEN** 在 Apple Silicon/macOS 执行 canonical Go/Rust build
- **THEN** MUST 生成 `bin/mornlea`、`bin/mornlea-server` 与同目录 `libmornlea_engine.dylib`

#### Scenario: clean checkout 安装 Agent 服务入口

- **GIVEN** Python 3.12 与 uv 可用
- **WHEN** 在 `packages/agent/companion` 执行 locked sync
- **THEN** `mornlea-companion-agent serve --config` MUST 可寻址并以 Mornlea 包身份报告版本/帮助

#### Scenario: Linux 专服发布为同目录 bundle

- **WHEN** 在 Linux amd64 原生执行 canonical server build
- **THEN** MUST 生成同目录 `mornlea-server` 与 `libmornlea_engine.so`
- **AND** 两者 MUST 作为一个不可混装的发布单元升级；Python Agent 服务 SHALL 独立安装和启动，不得混入 native bundle

#### Scenario: 旧入口不再发布

- **WHEN** 枚举当前 module、命令、native ABI、构建与 Agent 服务身份
- **THEN** MUST 不存在 `mcgo`/`mcgod` wrapper、旧 `mcgo` C symbol、`libmornlea_mesh.dylib`、`libmornlea_mesh.so` 或旧环境变量 fallback
- **AND** additive ABI v1 的 `mornlea_mesh_section` MUST 继续保留
