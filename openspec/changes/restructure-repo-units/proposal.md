## Why

仓库当前以"语言 + Go 惯例"组织：Go 全部挤在单一 module 的 `cmd/`+`internal/` 下，Rust、Python、前端又各占一套顶层目录（`engine/`、`services/companion-agent/`、`web/agent-board/`），并存在三类结构性杂乱：

1. **跨语言组件摆放不一致**：同为前端，菜单 WebView 在 `engine/crates/mornlea_client/frontend/`、看板在 `web/agent-board/`（npm vs pnpm 双工具链）；同为独立服务，Python 在 `services/` 下而 Rust workspace 在顶层 `engine/`。
2. **单元不可独立构建**：Go 服务端、客户端与共享域共享一个 `go.mod`，边界只靠 archcheck 白名单语义强制，`cd` 进任何一个功能域都无法独立 `go build ./...`。
3. **构建/CI 知识多点重复**：dylib 部署内联在 Makefile `rust` target 而 CI 平行实现；CI 里 artifact 校验 bash 块逐字重复 3 次；一次性工具（`scripts/composite_grass_side.go` 是 Go main 包、`cmd/gfxspike`）混进 `scripts/` 与 `cmd/`，迫使 CI 分片特判。

`internal/archcheck/dependency_test.go` 的 `allowed` 表已验证全仓内部依赖是 DAG，具备按单元切割且不改任何生产逻辑的条件。

## What Changes

- **顶层只留全局内容**：`go.work`、`Makefile`、`.github/`、`docs/`、`openspec/`、`scripts/`、`testdata/`、`README*`、`LICENSE`、`AGENTS.md`、`CLAUDE.md`、`.gitignore`。
- **全部独立单元收纳进 `packages/`**：
  - `packages/shared/`（Go 模块）：`core, nativeabi, logging, physics, pathfind, world, companion, network(+protocol/codec/tcp), worldgen, profile, config`，外加自 `internal/sim/tuning` 上提的 `tuning`（config→tuning 是唯一横跨服务端域的共享层引用，tuning 为纯调参值对象叶子包）。
  - `packages/server/`（Go 模块）：`sim/{contract,realm,entity,runtime}, fluid, storage(+六子包), server(+persistence)` 与 `cmd/mornlea-server`。
  - `packages/client/`（Go 模块）：`client, render(+hud), mesh, lod, audio, assets` 与 `cmd/mornlea`（app/capture/benchmark/devcapture 随迁）。
  - `packages/tools/`（Go 模块）：`perfcheck, agent-board`（Go 服务 + 看板前端并入 `agent-board/web`）、`gfxspike`、`composite_grass_side`。
  - `packages/audit/`（Go 模块，纯测试）：archcheck 升级为跨模块单元边界校验。
  - `packages/contracts/`（最小 Go 模块）：embed.go 必须与 JSON 同目录故自成模块。
  - `packages/engine/`：Rust workspace 保留原名整体搬迁（crates/、include/、rust-toolchain.toml 原样）。
  - `packages/agent/companion/`：Python 伙伴 Agent 服务（原 `services/companion-agent`），`agent/` 为服务族目录、内部按 agent 组织。
- **Go 多模块 workspace**：根 `go.mod` 解散，`go.work` 直辖上述 6 个 Go 模块；单元间 require 边（server/client → shared+contracts 等）由 go.mod 与 archcheck 双层强制。
- **构建/CI 收敛**：dylib 部署提取 `scripts/engine/deploy-dylib.sh`；CI artifact 校验收敛为 `scripts/ci/verify-native-artifact.sh`；frontend job 改调 `make frontend-check`；`make_demo_gif.swift` 修复失效 golden 路径；看板前端 npm→pnpm 与菜单前端统一 corepack 钉版姿势。
- **工作区卫生**：删除根目录遗留 `client.test`/`mesh.test` 与空 golden 目录；`.gitignore` 增 `/.claude/worktrees/`。
- **分 8 个阶段、每阶段 1 个可独立合并回退的 PR**，全程不改 wire bytes、协议、schema、engine/client ABI、golden 像素与任何生产行为（纯目录与 import path 搬迁 + 构建编排收敛）。

## Capabilities

### New Capabilities

无（复用既有能力，无新增）。

### Modified Capabilities

- `repository-code-organization`：新增"packages 单元化布局""Go 模块边界双层强制""单元迁移保持测试入口与产物"三个 Requirement；把钉死 `internal/`、`cmd/`、`services/` 路径的既有 Requirement 系统性改写为 `packages/` 路径（含 `sim/tuning` 上提 shared 的语义调整），语义不变。
- `project-identity`：Agent 服务 locked sync 场景的目录随 `services/companion-agent → packages/agent/companion` 更新，身份与命令不变。
- `test-timing-discipline`：go-race 分片 union 的包集基准从根模块 `go test ./...` 改为 go.work 全部模块的 `go list ./...` 之并（单元迁移期间分片组织方式允许过渡），无包丢失/重复的意图不变。

## Impact

- 受影响：全部 Go 源文件 import path、`Makefile`、`.github/workflows/ci.yml`、`.gitignore`、`scripts/agents/race-changed.sh`、cgo LDFLAGS 三处、`cmd/mornlea/capture` golden 目录常量、Python `pyproject.toml` force-include 相对路径、根/各级 `AGENTS.md` 与 `docs/architecture.md` 目录导览。
- 不适用：协议、存档 schema、engine ABI v10、client ABI v14、benchmark scenario 均不变；golden 像素逐字节不变。
- 兼容性：对外 `go install github.com/channing771/mornlea/...` 的旧路径失效（本仓库无外部消费方，README 安装说明同步更新）；`make rust`/`make test-race` 等入口名保持不变。
- 回退：每阶段一个 PR，反向 `git mv` + 路径引用还原即回退；S4–S7 期间 root 模块始终可编译。
