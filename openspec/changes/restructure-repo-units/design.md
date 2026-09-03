# Design: 仓库单元化重构

## 1. 切割依据：allowed 依赖图的模块归属

以 `internal/archcheck/dependency_test.go` 的 `allowed` 表为唯一权威输入（全图已验证为 DAG）。逐包归属：

| 包 | 目标单元 | 依据 |
|---|---|---|
| core, nativeabi, logging, physics, pathfind, world, companion, profile | shared | 被 server 与 client 双侧消费或为共享叶子 |
| network(+protocol/codec/tcp) | shared | 登录状态机与 packet 契约双侧共用（权威边界要求 Memory/TCP 同路径） |
| worldgen | shared | 被 server（生成 worker）与 client 侧 assets 消费；自身仅依赖 core/world/nativeabi |
| config | shared | 被 cmd 双侧消费（source guard：仅 cmd 导入 config）；依赖 sim/tuning |
| **sim/tuning → shared/tuning** | shared | `config→sim/tuning` 是唯一横跨服务端域的共享层引用；tuning 是纯调参值对象叶子包（仅依赖 core，且受"tunable 只在 tunables.go 读默认值"守卫），上提 shared 是最小语义让步 |
| sim/{contract,realm,entity,runtime} | server | 权威模拟编排，仅服务端消费 |
| fluid | server | 仅 sim/realm 消费；独立可穷举测试的性质在单元内保持 |
| storage(+六子包) | server | 磁盘生命周期归服务端权威 |
| server(+persistence) | server | 权威装配 |
| client, render(+hud), mesh, lod, audio, assets | client | 呈现侧；assets→mesh 单元内、assets→worldgen 下沉 shared 均为合法向下边 |
| cmd/mornlea(+app/capture/benchmark/devcapture) | client | 客户端入口；cmd 子树方向契约原样保留 |
| cmd/mornlea-server | server | 服务端入口 |
| perfcheck, agent-board, gfxspike, composite_grass_side | tools | 开发工具；agent-board 吸收顶层 web/agent-board 前端为 agent-board/web |
| archcheck 全套测试 | audit | 跨模块枚举校验者，不得住进任何被审单元 |
| contracts(含 embed.go) | contracts | go:embed 只能引用包目录子树，embed.go 必须与 JSON 同目录；Python force-include 亦指向该处，自成最小模块保住双消费者 |

单元间合法 require 边（向下单向）：

```
server  → shared, contracts        （生产代码；另 MAY 仅因测试 require client，
                                     生产文件禁 import client 由源码守卫强制）
client  → shared
tools   → shared, server, client, contracts   （perfcheck 消费 server/network/render）
audit   → （不导入被审单元，纯 go list/AST 观察）
shared  → （标准库与既有三方依赖）
contracts → （无内部依赖）
```

> server→client 的测试专用豁免是 S5 落地的既成事实（28 个客户端镜像驱动的
> Memory/TCP 集成测试）经控制会话裁决正式化：模块层允许（Go 的 require 无法
> 按 test 限定），语义层由 archcheck 源码守卫兜底——与仓库既有的
> persistence_contract 豁免式守卫同风格。

## 2. 目录与路径映射

| 旧 | 新 |
|---|---|
| `internal/X` | `packages/{shared,server,client}/X`（按上表） |
| `internal/sim/tuning` | `packages/shared/tuning` |
| `internal/archcheck` | `packages/audit`（包名保持 archcheck） |
| `cmd/mornlea...` | `packages/client/cmd/mornlea...` |
| `cmd/mornlea-server` | `packages/server/cmd/mornlea-server` |
| `cmd/perfcheck` `cmd/mornlea-agent-board` `cmd/gfxspike` | `packages/tools/{perfcheck,agent-board,gfxspike}` |
| `scripts/composite_grass_side.go` | `packages/tools/composite_grass_side/` |
| `web/agent-board` | `packages/tools/agent-board/web`（顶层 web/ 取消） |
| `services/companion-agent` | `packages/agent/companion`（services/ 取消；agent/ 为服务族目录） |
| `engine/` | `packages/engine/`（保留原名，内部结构原样） |
| `contracts/` | `packages/contracts/` |

import path 前缀统一 `github.com/channing771/mornlea/internal/X → github.com/channing771/mornlea/packages/{unit}/X`，机械 sed + gofmt 完成，不改任何符号、签名与逻辑。

## 3. 分阶段（每阶段一个 PR，全程绿）

- **S1** 卫生 + `services/companion-agent → packages/agent/companion` + CI 去重（verify-native-artifact.sh、frontend job 调 make）+ `make_demo_gif.swift` 修路径 + `.gitignore` 增 `/.claude/worktrees/`。此阶段 `packages/` 立项。
- **S2** `engine/ → packages/engine/`：Makefile `RUST_DIR`/`CARGO_TARGET_DIR`、cgo LDFLAGS 三处（nativeabi/mesh/client 的 `#cgo LDFLAGS`）、`.gitignore` engine 条目、CI、docs 路径；dylib 拷贝+install_name_tool+codesign 提取 `scripts/engine/deploy-dylib.sh`。迁移后 `go clean -cache`（GOCACHE 陈旧 cgo 前科）。
- **S3** 建 `go.work`（初始仅 root module + `packages/contracts`）；contracts embed 模块化；pyproject force-include 改 `../../../packages/contracts/...`。
- **S4** shared 切割：12 包 + tuning 上提；root 模块 require shared；archcheck `allowed` 表路径重写；race-changed 脚本与 CI 分片同步。
- **S5** server 切割（sim 余下四包/fluid/storage/server/cmd）。
- **S6** client 切割；`captureGoldenDir` 相对深度修复，`make visual-check` 全量 golden 验证。
- **S7** tools 切割 + 根 `go.mod` 解散 + `packages/audit` 立顶 + 单元边界 archcheck 激活全量断言。
- **S8** agent-board 前端 npm→pnpm；Makefile 重构为 per-unit 目标 + 聚合门禁（`make test`/`make test-race` 显式按模块循环，不依赖 workspace 隐式 `./...` 语义）；docs/AGENTS.md 链全面改写；openspec 归档。

## 4. 关键设计决策与被否决方案

- **否决"单 module 目录重排"**：不满足"单元独立构建"目标；go.mod require 边让单元边界获得编译器级强制，archcheck 继续负责语义级方向（两者互补而非重复：go.mod 管"能否 import"，archcheck 管"该不该 import"）。
- **否决 engine/ 更名**（用户裁决）：保留 `engine` 原名，仅搬迁；"engine 目录含 client crate"的命名瑕疵由 docs 说明承担。
- **否决 sim 整体下沉 shared**：权威模拟是服务端独有职责，仅 tuning 值对象上提。
- **否决菜单前端迁出 crate**：dist 经 `include_bytes!` 内嵌进二进制，源码与 dist 同处 crate 有内聚性；仅统一包管理器姿势。
- **单元边界测试先行**：第一颗实现 PR（Task 1.1）落地边界检查器 + 合成 drift 测试（纯内存断言检查器本身有效）；真实树断言按"枚举 `packages/` 下现存 go.mod"动态激活——阶段前期无模块时空转绿，S4–S7 每落一个模块即自动收紧，全程无中间红。
- **测试组织不变**：测试仍与被测代码同目录（docs/test-organization.md 的分层纪律整体随包迁移，不建集中 tests/）。

## 5. 风险与对策

| 风险 | 对策 |
|---|---|
| cgo `#cgo LDFLAGS` 相对路径（${SRCDIR} 深度）S2/S4 各变一次 | 每阶段 `make rust` + 定点 `go test ./...nativeabi ./...mesh ./...client` + `go clean -cache` |
| `captureGoldenDir` 相对深度 | S6 修复后 `make visual-check` 全量 43 张 golden 验证，禁止重生成 |
| CI go-race 分片路径过期 | 每切割阶段同步分片；S7 后分片=模块（shared/server/client/tools） |
| workspace 下 `go test ./...` 语义 | Makefile/CI 显式按模块循环 |
| 共享 CARGO_TARGET_DIR 跨 worktree 串扰 | 重型门禁前后 `make rust`，禁止并发跑重型管线 |
| archcheck 双真相表（global + sim 局部）漂移 | 迁移时同步改写两表并保持 `TestSimAllowedEdgesMatchesGlobalAllowed` 钩子 |
| CLAUDE.md shim 逐字节一致性测试 | 目录更名不改导入模式，`TestClaudeMdImportsAgentsMd` 类基线测试保持绿 |

## 6. 验证方法（每阶段边界）

T1：`gofmt -l`、`go vet`、`go test <受影响包> -short`、`go test ./internal/archcheck -count=1`（S7 后 `./packages/audit/...`）。
T2/T3（阶段边界）：`make rust` → `make test-race` → `npx --yes @fission-ai/openspec@1.7.0 validate --all --strict`；S6 加 `make visual-check`；S8 加 `make frontend-check`、`make companion-agent-check`、`make companion-agent-integration`、`make build-linux-server`。
