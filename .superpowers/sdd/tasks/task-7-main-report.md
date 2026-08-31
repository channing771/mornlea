# Task 7B implementation report

## 结果与边界

Go frozen snapshot registry 与 MCP v1 已按 Task 7B brief 实现：core canonical
machine names、33×17×33 frozen terrain、canonical digest、四槽 registry、六个纯
工具/validator、官方 Go MCP SDK v1.7.0、严格 raw outer gate、loopback service
及 Host component lifecycle 已闭环。没有把 Planner/Dialogue 切到 Agent HTTP，
没有接 namespace lease、memory CAS、planner generation/FIFO 或 Task 9/10 run
lifecycle。

## 实现摘要

- `internal/core` 为全部 84 个合法 `BlockID`（含 air）和 52 个非空合法
  `ItemID` 提供唯一、可逆、fail-closed 的小写 ASCII snake_case registry；完整
  方块 item 从 `ItemPlacement` 与 block registry 派生，place 白名单只保存 ID。
- planning snapshot 以伙伴 floor 格为中心冻结 33×17×33 data plane：1,089-bit
  ready bitmap、1,089 个 BE int16 height 与 18,513 个 BE uint16 block，总计
  39,341 bytes。全 ready/world-valid 时恰好 18,513 次 primary block read，
  exposed/height 之后只从 cache 派生；Dialogue 仍保留原 ±4 路径。
- snapshot digest 使用独立 snake_case DTO、RFC 4648 padded Base64 与 SHA-256；
  terrain/full canonical bytes 分别受 `<53 KiB`/`<=96 KiB` 约束，legacy
  `PlanSnapshot.Heights` 不重复进入 digest。固定 snapshot 与 terrain canonical
  SHA-256 golden 分别为
  `b1bcb780f32b59233983c3bc06fb662ffd42260cba9d6c7cf964c146fd7a8a50` 与
  `2942dddd4743b1c783b153a3b4e2f5e7026584844bb741bff2a6135ca350f363`。
- registry 容量固定为 4，第五槽立即失败；随机 UUIDv4 snapshot ID 与独立
  256-bit Base64URL capability 只在 worker 路径生成。注册/lookup 双向深拷贝，
  fake clock/timer 覆盖 exact deadline+5s expiry，past、到期中途与不可表示 timer
  deadline 都 fail closed。Complete/Cancel/TTL/Close 删除 record 并 signal，
  handler 在入口、所有有界循环、编码前后和 response commit 前检查 cancellation。
- 六工具直接复用 planner 的 strict step decoder、`planMineableBlock`、place
  registry 与 checkpoint-aware inventory/step validation。find/query domain failure
  保持 `isError=false` 且无 partial；validator 只产生七个 stable code。成功值同时
  提供相同对象的 StructuredContent 与恰一 canonical JSON TextContent。
- production contract loader 直接嵌入 checked-in manifest/schema，拒绝 duplicate
  或 trailing JSON，递归 resolve local `$ref`，按 manifest 顺序广告 exact input/
  output schema，并把 place enum、domain codes 与 canonical byte limits 锁回 Go
  registry。
- `/mcp` 使用 `github.com/modelcontextprotocol/go-sdk v1.7.0` 的 raw
  `Server.AddTool` 与 stateless JSON Streamable HTTP。outer 在 SDK 前检查 path、
  method、actual Host/Origin、Bearer、body/content type/UTF-8、单 object、method、
  id 与 protocol version；响应在 160 KiB+1 recorder 中完整缓冲，拒绝 SSE、任意
  session header、重复 content type、cancellation 与 overflow 后才一次 commit。
  runtime/SDK error 正文化为固定 unavailable，不回显输入或内部错误。
- Host 仅在配置存在伙伴时创建 registry 与 `127.0.0.1:0` MCP service；endpoint
  使用 listener 实际 authority。Serve 意外失败不影响 world，Close 幂等并解除
  Serve。最终 shutdown 顺序仍留给 Task 10。
- archcheck 显式登记 `internal/server -> contracts/companion-agent/mcp-v1` 单向边，
  并把 `contracts` 纳入跨平台 identity type-check 根。

## TDD RED → GREEN

首轮 RED 均在对应生产 API 存在前执行：

- `go test ./internal/core -run CanonicalName -count=1` 因 canonical registry API
  不存在而编译失败；实现穷举 registry 后转绿。
- terrain/digest/registry focused tests 起初因 `TerrainProjection`、
  `CanonicalSnapshotDigest`、`SnapshotRegistry` 不存在而编译失败；server counting
  fake 同时证明 planning snapshot 尚无 33×17×33 primary/cache split。最小值类型、
  builder、digest 与 registry 实现后转绿。
- planning tool/validator tests 起初因六工具 API 与 stable validator result 不存在
  而编译失败；直接消费 machine fixtures 并复用既有 planner 规则后转绿。
- MCP tests 起初因 SDK dependency、contract loader、outer handler 与 service 不存在
  而编译失败；最小 SDK/outer/service/Host 装配后转绿。

收口加固也保留了实际失败证据：

- registry 在冻结期间越过 deadline、year 3000 timer duration 两例最初均错误返回
  success；最终注册前重验有效 deadline 与可表示 duration 后转绿。
- context tool 初始只有 3 次 checkpoint（缺 chunk loop），find names、plan decode、
  digest 后、online/inventory/output loops 同样缺检查；checkpoint 计数 RED 后统一
  补齐并通过 cancel/expiry race。
- embedded JSON duplicate 与 malformed trailing 起初均被接受；outer 起初会回显
  `LEAK-ME-NOT` SDK error、允许 notification SSE/空 session header，并在 request
  context 已取消时仍调用 SDK（`calls/status=1/200`）。严格 decoder、受控错误与
  cancellation/header commit gate 后全部转绿。
- snapshot/terrain exact canonical digest 先用 `TODO` golden 真实失败并记录实际
  SHA，再锁定上述常量。
- 新 dense scan 改变 test goroutine 的 wall-clock 调度后，interaction parity 的
  60-tick async wait 曾耗尽但 worker 尚未获调度。没有放宽业务 tick 窗口；测试只
  在等待异步 event 的未命中 tick 固定退避 1ms。定点 no-race `-count=5` 与 race
  `-count=3` 均稳定通过。

## Fixture 与协议覆盖

- checked-in MCP golden 总数沿用 Task 7A：20 valid、44 invalid、8 mine，共 72。
  Go 六工具执行测试直接运行 6 个 tool input valid、9 个适用 invalid input 与全部
  8 个 mine classification case；输出再交给 checked-in resolved schema validator。
- validator 明确覆盖全部 7 个 stable code、dense mine 不在 exposed cap 的成功、
  Chest/Furnace/Stone 与 farming/torch/no-drop/multi-drop 语义。
- raw pre-SDK matrix 有 29 个拒绝 case，另覆盖 Origin absent/exact 两个成功分支；
  SDK round-trip 覆盖 initialize、initialized、tools/list、成功 tool、两种 normal
  domain result、schema `isError=true`、unknown-tool 去敏与 Accept compatibility。
- response recorder 覆盖 oversize、SSE、非空/空 session、duplicate content type、
  error detail、registry/request cancellation；256 KiB request 与 160 KiB response
  exact 边界成功，`+1` 均拒绝且没有 partial write。
- 新增核心 Task 7 测试函数 31 个；registry 另含 exact TTL/capacity/deep-copy 与
  concurrent cancel/expiry，service 另含 actual loopback authority、Close unblock、
  Serve failure isolation 与 conditional Host assembly。

## 最终 GREEN

- `go test ./internal/core ./internal/companion ./internal/server -run 'CanonicalName|MCP|Snapshot|Terrain|PlanningTool' -race -count=1`：PASS
  （core 2.264s；companion 4.020s；server 6.003s）。
- `go test ./internal/core ./internal/companion -race -count=1`：PASS
  （core 1.532s；companion 9.734s）。
- `go test ./internal/server -race -count=1`：PASS（214.301s）。
- `go test ./internal/server -run '^TestCompanionInteractionMemoryTCPParity$' -count=5`：
  PASS（14.288s）。
- `go test ./internal/server -run '^TestCompanionInteractionMemoryTCPParity$' -race -count=3`：
  PASS（14.913s）。
- `go test ./internal/archcheck -count=1`：PASS（6.456s）。
- `go vet ./internal/core ./internal/companion ./internal/server`：PASS。
- `openspec validate --all --strict --no-interactive`：PASS（80/80）。
- `gofmt` 与 `git diff --check`：PASS。

完整 server race 的首次运行（207.168s）同时暴露了本任务已修复的 parity 调度
RED，以及三个独立的 Task 8 `file already closed` regression。后者归属 storage
repair，Task 7 未修改 storage；repair commit `0dc592c8` 进入当前 HEAD 后已按上述
命令从头复跑并全绿。

## 文件与依赖

- 依赖：`go.mod`、`go.sum`（direct `github.com/modelcontextprotocol/go-sdk v1.7.0`）。
- embedded contract：`contracts/companion-agent/mcp-v1/embed.go`。
- core：`internal/core/canonical_name.go` 及测试。
- companion：`terrain_projection.go`、`snapshot_digest.go`、
  `snapshot_registry.go`、`planning_tools.go` 及测试；`plan_types.go`、`planner.go`、
  `planner_test.go` 的最小复用/迁移修改。
- server：`companion_snapshot.go`、`companion_planning_terrain_test.go`、
  `companion_mcp_contract.go`、`companion_mcp_outer.go`、`companion_mcp.go`、
  `companion_mcp_test.go`、`host.go`、`host_shutdown.go`，以及 parity test 的有界
  async wait 修复。
- architecture：`internal/archcheck/dependency_test.go`、`identity_test.go`。

## 风险与提交状态

- Task 9 必须把一次 run 的 register/capability/HTTP/complete-cancel 与权威重验
  接入现有 planner worker；当前 registry 尚未被生产 Planner 使用，这是刻意边界。
- Task 10 仍负责最终 shutdown 顺序；当前 component Close 是幂等且可组合的。
- 未查看、比较或合并 main，未修改版本文档或 OpenSpec tasks/ledger。
- 所有门禁与最终 status/staging 核对完成后仍不自行提交；commit 等待控制会话
  明确授权。
