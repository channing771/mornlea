## Task 1: 卫生、单元边界检查器与 companion 迁入 agent 族

- [x] 1.1 在 `internal/archcheck` 新增 `unit_boundary_test.go`：定义单元允许 require 边表（server→{shared,contracts}、client→{shared}、tools→{shared,server,client,contracts}、audit/contracts→∅）与检查器函数，先写合成 drift 测试证明检查器对每类违规（client→server、server→client、shared→server、audit 导入被审单元）都报错；真实树断言按枚举 `packages/` 下现存 `go.mod` 动态激活（无模块时空转绿）。以 `go test ./internal/archcheck -count=1` 验证。
- [x] 1.2 工作区卫生：删除仓库根遗留 `client.test`、`mesh.test` 与两个空 golden 目录（`cmd/mornlea/capture/testdata/golden/`、`engine/crates/mornlea_client/frontend/visual/golden/`，本地残留不入库）；`.gitignore` 增 `/.claude/worktrees/`；修复 `scripts/make_demo_gif.swift` 失效的旧 golden 路径改为 `testdata/visual-golden/world`。以 `git status --short` 仅显示预期改动验证。
- [x] 1.3 `git mv services/companion-agent packages/agent/companion`（`packages/` 与 `packages/agent/` 立项）；`pyproject.toml` force-include 路径 `../../contracts/...` → `../../../contracts/...`；同步 Makefile `COMPANION_AGENT_DIR`、`.github/workflows/ci.yml`、根 `AGENTS.md` 与 `docs/` 中 `services/companion-agent` 路径引用。以 `make companion-agent-check` 与 `make companion-agent-integration` 验证。
- [x] 1.4 CI 去重：新增 `scripts/ci/verify-native-artifact.sh`（manifest 行数、sha256、双 dylib 校验），`ci.yml` 的 quality/go-race/integration 三个 job 改调该脚本；frontend job 改调 `make frontend-check`（补 corepack 就绪步骤）。以 `bash -n`、`make frontend-check` 与 CI YAML 语法自检验证。

## Task 2: engine 整体迁入 packages/engine 并收敛 dylib 部署

- [x] 2.1 `git mv engine packages/engine`；Makefile `RUST_DIR`、`CARGO_TARGET_DIR`、`FRONTEND_DIR` 与 dylib 常量改路径；`internal/nativeabi`、`internal/mesh`、`internal/client` 的 `#cgo LDFLAGS` 相对路径同步；`.gitignore` 的 `/engine/target/` 与 frontend node_modules/visual-dist 条目改 `packages/engine/...`；`ci.yml`、根/局部 `AGENTS.md`、`docs/` 中 `engine/` 路径全量更新。以 `go clean -cache && make rust && go vet ./...` 验证。
- [x] 2.2 把 Makefile `rust` target 的 dylib 拷贝、`install_name_tool` 与 codesign 逻辑提取为 `scripts/engine/deploy-dylib.sh`（参数：cargo 共享 target dir 与规范 release dir），Makefile 与 CI 共用；`make rust-check` 后接 `make rust` 与定点 `go test ./internal/nativeabi ./internal/mesh ./internal/client -short -count=1` 验证。

## Task 3: 建立 go.work 与 contracts 模块化

- [x] 3.1 新建根 `go.work`（初始 `use . ./packages/contracts`）；`git mv contracts packages/contracts` 并为 `packages/contracts` 建 `go.mod`（module `github.com/channing771/mornlea/packages/contracts`）；根 `go.mod` 增加 require+replace 指向本地 contracts；`internal/server` 的 contracts import path 改写；pyproject force-include 再改为 `../../../packages/contracts/...`。以 `make companion-agent-check`、`go test ./internal/server -run MCP -short -count=1`、`go vet ./...` 验证。

## Task 4: shared 模块切割

- [ ] 4.1 `git mv internal/{core,nativeabi,logging,physics,pathfind,world,companion,network,worldgen,profile,config} packages/shared/`，`git mv internal/sim/tuning packages/shared/tuning`；建 `packages/shared/go.mod`（require+replace contracts）；全仓 import path 改写 `internal/(core|nativeabi|logging|physics|pathfind|world|companion|network|worldgen|profile|config|sim/tuning)` → `packages/shared/...`；go.work 增 `use ./packages/shared`；archcheck `allowed` 表与 sim 局部表路径同步、`TestSimAllowedEdgesMatchesGlobalAllowed` 保持绿；`scripts/agents/race-changed.sh` 与 CI go-race 分片路径同步。以 `go clean -cache && make rust && make test-race && go test ./internal/archcheck -count=1` 验证。

## Task 5: server 模块切割

- [ ] 5.1 `git mv internal/{sim,fluid,storage,server} packages/server/`，`git mv cmd/mornlea-server packages/server/cmd/mornlea-server`；建 `packages/server/go.mod`（require+replace shared/contracts）；import path 改写 `internal/(sim|fluid|storage|server)` → `packages/server/...` 与 `cmd/mornlea-server` → `packages/server/cmd/mornlea-server`；go.work 增 `use ./packages/server`；Makefile `SERVER` 常量、archcheck、race-changed、CI 分片同步。以 `go clean -cache && make rust && make test-race && go test ./internal/archcheck -count=1 && make build-linux-server` 验证。

## Task 6: client 模块切割与视觉基线验证

- [ ] 6.1 `git mv internal/{client,render,mesh,lod,audio,assets} packages/client/`，`git mv cmd/mornlea packages/client/cmd/mornlea`；建 `packages/client/go.mod`（require+replace shared）；import path 改写 `internal/(client|render|mesh|lod|audio|assets)` → `packages/client/...` 与 `cmd/mornlea` → `packages/client/cmd/mornlea`；go.work 增 `use ./packages/client`；修复 `packages/client/cmd/mornlea/capture` 的 `captureGoldenDir` 相对深度指向仓库根 `testdata/visual-golden`；Makefile `APP` 常量、archcheck cmd 子树表、CI 分片同步。以 `go clean -cache && make rust && make test-race && make visual-check && go test ./internal/archcheck -count=1` 验证（golden 全过、零 actual/diff 产物）。

## Task 7: tools 模块、根 go.mod 解散与 audit 立顶

- [ ] 7.1 `git mv cmd/perfcheck packages/tools/perfcheck`、`git mv cmd/mornlea-agent-board packages/tools/agent-board`、`git mv web/agent-board packages/tools/agent-board/web`（顶层 `web/` 取消）、`git mv cmd/gfxspike packages/tools/gfxspike`、`git mv scripts/composite_grass_side.go packages/tools/composite_grass_side/main.go`；建 `packages/tools/go.mod`（require+replace shared/server/client）；import path 改写；`.gitignore` 的 `/gfxspike` 条目调整；CI go-race 分片去掉 scripts Go 包特判。以 `go vet ./... && go test ./packages/tools/... -short -count=1` 验证。
- [ ] 7.2 `git mv internal/archcheck packages/audit`，包名与 `make archcheck` 语义保持；删除根 `go.mod`/`go.sum` 对应内容——根 `go.mod` 解散（`go.work` use 列表移除根并直辖六个模块）；archcheck `moduleRoot`/枚举逻辑改为跨 `go.work` 各模块；单元边界检查（Task 1.1）转为对全部现存模块生效。以 `go clean -cache && make rust && make test-race && make archcheck` 验证。

## Task 8: 收尾——前端工具链统一、Makefile per-unit 与文档基线

- [ ] 8.1 `packages/tools/agent-board/web` 从 npm 迁 pnpm（`packageManager` 钉版 + pnpm-lock，删 package-lock）；Makefile `agent-dashboard`/`agent-ui-dev` 改 corepack pnpm。以 `make agent-dashboard` 冒烟（构建可启动即停）验证。
- [ ] 8.2 Makefile 重构：`test`/`test-race`/`test-race-short`/`dev-check` 显式按模块循环（shared/server/client/tools/audit），`vet` 同步；help 文案更新。以 `make dev-check` 验证。
- [ ] 8.3 文档基线：`docs/architecture.md` 目录树与包路径、根 `AGENTS.md` 目录导览与门禁路径、各单元 `AGENTS.md`（原 internal 各级指南随包迁移并更新路径）、`docs/notes/test-quickstart.md` 定点命令、`README.md`/`README.en.md` 目录说明；`CLAUDE.md` 保持逐字节 shim 导入。以 `go test ./packages/audit -count=1`（含 CLAUDE.md 一致性基线）与 `openspec validate --all --strict` 验证。
- [ ] 8.4 全量门禁与归档：`gofmt -l` 空、`go vet ./...`、`make rust`、`make test-race`、`make visual-check`、`make frontend-check`、`make companion-agent-check`、`make companion-agent-integration`、`make build-linux-server`、`openspec validate --all --strict`，实际结果与每 Task 的 SPEC+QUALITY 裁决写入 `ledger.md` 后归档。
