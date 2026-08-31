# extract-companion-agent-service ledger

## 基线与规则

- Change：`extract-companion-agent-service`；认领基线 `8b8891a3`。
- 执行模型：每个 Task 使用 fresh implementer；每项完成后由独立 SPEC reviewer 与 QUALITY reviewer 双裁决；控制会话不直接实现生产代码。
- 修复循环：单任务最多 5 轮；未通过双评审不得勾选 `tasks.md`。
- 验证复用：只有相同基线 SHA、命令与范围的完整输出可复用；所有结果记录实际 exit code，不把未完成命令写成通过。
- 当前版本事实：protocol v32、player v8、chunk v9、metadata v3、companions v5、hostile v1、engine ABI v9、client ABI v13、scenario v20。
- 计划目标：仅 companions 升 v5；Agent HTTP application contract v1；MCP tool contract v1；MCP wire `2025-11-25`。
- Ruling: 以执行时 `main` 的 engine ABI v9/client ABI v13 为不变基线，本 change 不认领 native ABI 升版 — 代码与用户更新的项目指南高于旧规划中的 v8/v11/v12 文档事实 — 若裁决错误，成本是收尾版本文档与钉死测试需重做，不改变 Agent HTTP/MCP 合同。
- Ruling: Task 6 只移除 Go config 的 provider/direct-model 生产语义并新增强类型 Agent HTTP client；现有 Planner/Dialogue direct-model 实现保留为未接线的编译过渡，由 Task 9/10 在权威编排切换时删除 — 这避免 Task 6 提前改变运行路径，也不引入 fallback、隐式映射或 backend 开关 — 若裁决错误，成本是 Task 9/10 的删除面扩大，不影响 Task 6 合同。

## 规划裁决

- Python Planner graph 不配置持久 checkpointer；Dialogue 仅 transient graph state；SQLite 只保存 compact MemoryState/CAS、lease 与 tombstone 元数据。
- MCP 外层 raw envelope 在 Go SDK 前拒绝 batch/GET/ping/subscription/其他方法；显式 capabilities 只有 Tools 且 `listChanged=false`。
- 当前共同 MCP wire 下跨语言 request cancellation 不可靠；snapshot registry 用自己的 deadline/cancel/TTL 收口，并由真实跨语言测试覆盖。
- accepted Dialogue reservation 通过首次 tick 重验后不再由后续 generation 变化撤销；commit 只按 operation/epoch 关联。
- Go 是 task/world/lifecycle/epoch 权威，Python 是运行期 compact memory 权威；没有 direct-model fallback、remote MCP 或 Docker。
- Task 7 terrain ruling：以 `floor(companion.Position)` 为中心冻结固定 33×17×33（水平 ±16/垂直 ±8）dense projection；ready bitmap、1,089 个 height 与 18,513 个 `BlockID` 的 data plane 每份 ≤40 KiB、四槽 ≤160 KiB。tick 先以最多 18,513 次 world `blockAt` 完整填充 primary projection，再从 cache 派生 exposed 且零追加 world read；world-valid 投影外/未 ready 邻居为 unknown/non-air，world 垂直边界外为空气。`query_terrain` 与 mine 直接读 projection、不依赖 `ExposedBlocks<=256`，因此 Chest/Furnace 继续接受、农业/火把/无掉落/未交付多掉落继续拒绝；规划 ±8 与寻路 ±4 解耦。
- Task 7 domain-result ruling：`3a713a78` 独立评审发现 find/query stable code 没有 machine-readable wire。Task 7 在 Go MCP 前先以 TDD 修订 MCP v1 manifest/schema/golden 与 Python domain/adapter/Planner：每工具列 `domain_result_codes`，find/query 保持 success object 并新增 strict failure `oneOf`，精确 normal result 分别为 `{code:"unknown_block",hint}` 与 `{code:"out_of_bounds",hint}`、strict UTF-8 hint ≤256 bytes、无部分结果、`isError=false`；Python 把它作为普通 tool message，`isError=true`/transport/protocol/schema 仍 unavailable/`PlannerUnavailable`。这是 active change 内的 contract amendment，不改变 HTTP v1、MCP v1 标识、游戏 wire、存档或 ABI；Task 1/4 历史完成证据保留，但 Task 7 prerequisite 必须重跑相关 Python gates。
- Task 7 naming ruling：`internal/core` 作为 `BlockID`/`ItemID` 所有者新增 canonical English `snake_case` registry；名称分别在 BlockID 域和 ItemID 域内唯一，完整方块 item 与 block 跨域同名是预期。UI 中文 display name 不复用，Planner place 白名单只保留语义 ID 集并从 core 派生拼写；未知 ID/name fail closed，不生成数值/中文 fallback。
- Task 7 digest/cancellation ruling：terrain digest DTO 固定 BE/Base64 planes、bitmap 末 7 个 unused bits 为零、terrain <53 KiB、完整 digest input ≤96 KiB，且不重复编码 legacy `PlanSnapshot.Heights`。registry 删除阻止新 lookup 并 signal cancellation，`Close`/TTL 不等待 handler；已取得 immutable view 的一次有界读取可在尚未观察 cancellation 时完成，但入口/循环/编码前后/response commit 一旦观察就丢弃全部结果且不返回成功。
- Task 8 wire ruling：v5 保持 32-byte envelope；payload 是 namespace[16] 加按 `CompanionID` 排序的 record，record 固定 body[221]+flags(active/task/FIFO 为 bit0/1/2)+epoch u64，active 总带 revision/operation/summary-length+bytes 后接 task/FIFO，inactive只带 tombstone且 flags=0。合法可达上限固定 `MaxFileLength=393,904`；v1..v4 只读，encoder只写v5。若裁决错误，成本是 v5 golden/codec/磁盘上限与迁移重做，不影响游戏 wire 或 ABI。
- Task 8 migration ruling：legacy 是隐式 epoch0并统一落epoch1；v4 active 非空 summary 迁为 rev1+fresh operation，active 空 summary 使用 canonical-zero且不存在第二个 active migration operation，inactive使用 fresh tombstone。entropy 顺序固定为缺失 namespace 首先一个 UUID，随后按 `CompanionID` 升序只为每个需要的 nonzero mirror operation或tombstone各一个，canonical-zero active与unchanged v5零消费。新 ID 先以 metadata `SpawnDimension`、`SpawnAnchor*16+0.5`、`core.MaxY+1`、零朝向/空背包 provisional body 同步原子写v5，entropy/Save失败则 persistence/world/Agent/MCP均不构造；模拟 ready 后 Observe 覆盖位置。
- Task 8 staging ruling：Task 8 persistence只 deep-clone/carry-through namespace/lifecycle/mirror/tombstone并checked推进aggregate revision；body/task autosave绝不从旧direct Dialogue裸 Summary改写mirror，也不实现Agent memory mutation。Task 9 Planner不改memory；Task 10删除/替换裸写路径，只在Agent commit/reconcile成功结果回tick且通过epoch与该状态适用的replay identity关联后整体更新mirror并mark dirty。
- Task 8/10 active-zero reconcile ruling：higher `memory_epoch` 本身 fence 全部旧 epoch；active canonical-zero不新增HTTP/v5字段也不伪造operation，精确 replay key 是 `{namespace,companion,epoch,active,canonical-zero}`。active nonzero使用mirror operation，inactive使用tombstone operation；Agent离线跨 active N→inactive N+1→active zero N+2 后可只reconcile N+2并拒绝迟到N，Task 10必须先写该RED。
- Task 8 empty-config/atomic ruling：`WorldStore`必须提供Memory/Disk同语义的metadata-only existence probe；missing+empty只probe且0 Load/Save/create，existing+empty才Load并迁移/退休，already all-inactive v5不推进revision/epoch/tombstone且0 Save。只有pre-rename失败保证旧正式v4字节不变；parent-directory sync在rename后失败仍令启动失败但official path只能是完整可decode v5，不能要求旧v4保持；overflow返回`ErrCorrupt`语义且在entropy/mutation/Save之前失败。

## 任务记录

| Task | Implementer | 起始 SHA | RED/GREEN 与提交 | SPEC 评审 | QUALITY 评审 | 裁决 |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | `task1_contracts_impl` | `af57e420` | RED：缺少 HTTP schema；复审 RED：UTF-8 byte/权威常量、非确定 `oneOf` 与未支持 keyword；GREEN：focused `-count=100`、companion race、diff-check；提交 `36eed9d9`、`7d019d1c`、`a1640211` | `task1_spec_review` round 3 PASS | `task1_quality_review` round 3 PASS | Accepted |
| 2 | `task2_python_scaffold_impl` | `5d368f8c` | RED：package 缺失；复审 RED：exact const、URL/secret/path、golden path 与 import boundary；GREEN：locked sync、ruff/format、mypy、focused 两轮各 199 passed、diff-check；提交 `91bca693`、`665d609a`、`40c57fce` | `task2_spec_review` round 3 PASS | `task2_quality_review` round 3 PASS | Accepted |
| 3 | `task3_memory_impl` | `ea82d028` | RED：缺少 memory module；复审 RED：operation 复用、lease/SQLite 取消窗口、损坏库修补、重复 cancel 与 receipt 篡改；GREEN：focused 61 passed、full 260 passed、locked sync/ruff/mypy/diff-check；提交 `dcb99bc6`、`225fdb09`、`f68a6eca`、`7a1af977` | `memory_design_audit` round 4 PASS | `openspec_artifacts` round 4 PASS | Accepted |
| 4 | `openspec_artifacts` | `69f6e6ec` | RED：缺少 Planner/adapters；复审 RED：schema 漂移、transport/envelope 上限、validator wrapper 与 JSON 类型混淆；GREEN：focused 77 passed、full 337 passed、import boundary 48 passed、locked sync/ruff/mypy/wheel/diff-check；提交 `06473b29`、`e4b5dc8f`、`0e773804` | `task4_graph_spec_audit` round 3 PASS | `task4_quality_review` round 3 PASS | Accepted |
| 5 | `task5_http_research`、`task5_startup_cancel_fix` | `f4009ec0` | RED：缺少 Dialogue/FastAPI；复审 RED：HTTP disconnect、重复取消、响应 fence、关闭与 startup 所有权；GREEN：focused `56 passed`、full Python `393 passed`、import boundary `48 passed`、locked/ruff/mypy/diff-check；提交 `fb569bdd`、`ff7b5be9`、`962e361e`、`2d9042bf` | `task4_fix_quality` round 4 PASS | `task5_quality_review` round 4 PASS | Accepted |
| 6 | `task6_go_agent_client`、`task6_strict_lifecycle_fix` | `c871b1ec` | RED：配置/client 缺失，Round 1/2 复审继续拒绝 codec、manifest、边界与生命周期缺口；GREEN：隔离 focused race 两遍、config/companion full race、vet、archcheck、gofmt/diff-check；提交 `0fd248d1`、`f56b42bf`、`5ee80a7c`、`6b1deb59`、`f7585788`、`0ea46c31` | `task6_spec_review` round 1/2 FAIL，round 3 PASS | `task6_quality_review` round 1/2 FAIL，round 3 PASS | Accepted |
| 7 | `task7_contract_prereq`（Task 7A）、`task7_mcp_main`（Task 7B） | `148b935c`、`22991a82` | Task 7A 提交 `5812be64`、repair `038c4b86`；Task 7B initial `b00481e0`、repair 1 `79b5965f`、repair 2 `92f80e4f` | Task 7A 初次/repair PASS；Task 7B formal round 1 FAIL、repair 1 scoped FAIL、repair 2 final PASS | Task 7A 初次 FAIL/repair PASS；Task 7B formal round 1 FAIL、repair 1 scoped PASS、repair 2 final PASS | Accepted |
| 8 | `task8_storage_v5` | `148b935c` | codec/merge/probe/bootstrap/carry RED→GREEN；提交 `11f897c7`、repair `660c02a1`、memory reuse repair `0dc592c8` | 初次独立 PASS；两轮 repair scoped PASS | 初次独立 FAIL；repair 1 scoped PASS；repair 2 scoped PASS | Accepted |
| 9 | `task9_planner_cutover` | `6c5c4011` | Agent Planner cutover RED→GREEN；提交 `84c03161`、repair `edfb1574`、`b2c75a6c` | initial FAIL、repair 1 FAIL、repair 2 PASS | initial FAIL、repair 1 PASS、repair 2 PASS | Accepted |
| 10 | `task10_dialogue_memory` | `012a4b86` | Dialogue/memory/shutdown RED→GREEN；提交 `1f6006f6`、repair `6ef874b1`、`7283b746`、`c2cebeb9`、`6bdf2b7e` | initial FAIL、repair 1–4 后 PASS | initial FAIL、repair 1 FAIL、repair 2–4 后 PASS | Accepted |
| 11 | `task11_cross_language` | `e1bf9e52` | 真实 MCP/HTTP 进程合同 RED→GREEN；提交 `efa85718`、`a6b4d059`、`3635b6a1`、`356fe396`、`41add71e`、证据 `2f6ab0ba` | PASS | PASS | Accepted |
| 12 | `task12_ci_docs_gates` | `9423f067` | archcheck/Make/CI/version RED→GREEN；提交 `9dfdbc69`、`49fbc7f9`、证据 `ee642a05`、全量门禁 repair `3394fc19`；最终全绿 | Review pending | Review pending | Pending |

### Task 1 评审修复记录

- Round 1：两路评审拒绝 code-point 代替 UTF-8 byte、未约束 Dialogue 终态矩阵、任意 MCP callback URL、未机器化工具排序/位置对应，以及未交叉校验权威容量常量的初版契约。
- Round 2：原规格缺口关闭；两路评审共同拒绝依赖 Go map 顺序选择 `oneOf` 子错误，QUALITY 另要求未知标准 JSON Schema validation keyword 硬失败。
- Round 3：SPEC 与 QUALITY 均 PASS；`oneOf` 使用稳定父级错误，关键叶级规则由 direct fixtures 校验，schema keyword allowlist/audit、私有扩展硬失败与 100 次 focused 重复测试通过。
- 控制会话复验：`go test ./internal/companion -run 'ContractFixture' -count=100` exit 0；工作树在 ledger 更新前 clean。

### Task 2 评审修复记录

- Round 1：SPEC 与 QUALITY 拒绝 Pydantic `Literal` 的 bool/number 交叉强转；QUALITY 另拒绝任意 invalid error、非 ASCII header secret、不完整 provider authority/IDNA、SQLite symlink 裸异常及可绕过的 import boundary。
- Round 2：exact const、golden path、secret 与 SQLite value 修复通过；两路评审继续拒绝配置文件路径裸异常、手写 IDNA 与 httpx 双向漂移，以及未按 future layer 收口的动态 import/external dependency 门禁。
- Round 3：SPEC 与 QUALITY 均 PASS；配置 path 全收口，直接 `idna` 依赖与 httpx 兼容，CLI/app 是唯一 dynamic seam，harness/adapters/storage 采用集中 fail-closed allowlist 且 Task 3/4 合法依赖 probes 通过。
- 控制会话复验：`uv sync --locked`、ruff format/check、mypy exit 0；focused pytest 连续两轮均 199 passed；`git diff --check` exit 0。

### Task 3 评审修复记录

- Round 1：SPEC 与 QUALITY 拒绝仅保留最近 commit receipt、DB fence 已提交后取消旧 run 的窗口、aiosqlite 迟到 `BEGIN IMMEDIATE`、已有库 `CREATE IF NOT EXISTS` 修补损坏以及 close 失败后不可重试。修复引入不含历史摘要正文的 immutable SHA-256 operation receipts、cancellation-safe drain 和 fail-closed canonical schema v1。
- Round 2：SPEC PASS；QUALITY 拒绝重复 `task.cancel()` 打断 runner 异步清理，以及 readiness 未核对当前 active/tombstone receipt。修复后 cancel 仅在 false→true 边沿触发，当前 receipt 缺失或状态篡改硬失败。
- Round 3：SPEC PASS；QUALITY 拒绝 current commit `payload_fingerprint` 无法从持久元数据重算。修复持久 canonical `commit_lease_id`，按 lease/epoch/base/current summary 重算 payload，并用 nullable FK 关联永久 lease history，不保存历史 summary。
- Round 4：SPEC 与 QUALITY 均 PASS；合法 lease rotation/reopen、旧 operation exact replay、active-mirror/tombstone/zero control、transaction double-cancel、run cleanup、schema/FK 与 close retry 对抗探针通过。
- 控制会话复验：原 lease cancellation 窗口探针已关闭；Task 3 focused `61 passed`；工作树在 ledger 更新前 clean。

### Task 4 评审修复记录

- Round 1：SPEC 拒绝模型工具与 MCP discovery 未精确使用 checked-in schema、`validate_plan` 绕过 wrapper 上限；QUALITY 拒绝 MCP/provider 在 SDK 缓冲前无正文硬上限，以及完整 `PlanResponse` 可超过 HTTP 64 KiB。修复增加 wheel 内置 contract、type-safe schema pin、MCP 160 KiB/provider 1 MiB bounded transport、validator wrapper 与完整 response envelope 检查。
- Round 2：原 transport、wrapper、envelope 与 schema 漂移缺口关闭；SPEC 继续拒绝 Python dict equality 把 JSON `true`/`1` 和 `1.0`/`1` 视为相等。修复改用 canonical JSON bytes 做 schema 类型精确比较并 fail closed。
- Round 3：SPEC 与 QUALITY 均 PASS；bool/int、int/float、序列化失败、Content-Length/chunked/content-encoding、取消与 close、wheel 脱离源码导入、单 session/no retry、fresh graph/no checkpoint 对抗测试通过。
- 控制会话复验：Task 4 focused `77 passed`；完整 Python `337 passed`；工作树在 ledger 更新前 clean。

### Task 5 评审修复记录

- Round 1：SPEC 拒绝 body 读完后不再监听 ASGI `http.disconnect`；QUALITY 另拒绝重复 cancellation 泄漏 `RunGate`、model close 失败跳过 SQLite、response start 后双发、h11 header 边界及 lifespan credential 强引用。修复增加 run 期 disconnect watcher、response-start fence、cancellation-safe 槽位/关闭 pipeline、h11 余量与 one-shot secret bootstrap。
- Round 2：SPEC 未发现新违例；QUALITY 拒绝 component factory 成功后、移交 runtime 前取消会泄漏 model/SQLite。修复建立 factory 结果的临时所有权，成功 `runtime.start` 后才原子移交。
- Round 3：SPEC 与 QUALITY 共同拒绝用 `_drain_cleanup` shield 整个 bootstrap；阻塞 factory 在外部取消后不收到 `CancelledError`，使 startup/关服可无界等待。修复改为主动取消 owned bootstrap task，再 cancellation-safe drain 其资源清理。
- Round 4：SPEC 与 QUALITY 均 PASS；阻塞 factory 单次/重复取消、慢清理、close 异常、完成/取消同 tick 竞态、disconnect、lease/correlation、no checkpoint 与 terminal no-precommit 对抗通过；32 次重复取消与 200 次所有权竞态无死锁、泄漏或双关。
- 控制会话复验：Task 5 focused `56 passed`；完整 Python `393 passed`；asyncio debug + warnings-as-errors HTTP `36 passed`；工作树在 ledger 更新前 clean。

### Task 6 评审修复记录

- Round 1：SPEC 与 QUALITY 均 FAIL；评审拒绝未由 checked-in contract 驱动的 codec、`json.RawMessage`/非强类型 variant、request/response strictness、route/status/error/correlation 漏项、context 生命周期泄漏、transport/secret 所有权与非确定 config parsing。修复逐步落在 `5ee80a7c` 与 `6b1deb59`，但尚未达到接受条件。
- Round 2：SPEC 与 QUALITY 均 FAIL；`f7585788` 虽关闭 11-route/14-variant/59-error/79-correlation manifest matrix，独立复审仍拒绝 non-nullable explicit `null`、未按 Python scope 统计的真实 outbound header、重复 response Content-Type、body-read cancellation 与 `Close` admission 竞态、解引用 client 的 secret 表示、显式非法 port，以及缺少 production request-cap 证据。
- Round 3：`0ea46c31` 以 closed DTO null allowlist、真实 16 KiB wire-header gate、唯一 Content-Type、同步 closed/admission 与 active cancel registry、安全 formatter/log value、端口范围和 public typed request preflight 闭环上述问题；独立 SPEC 与 QUALITY 均 PASS，裁决 Accepted。
- 隔离验证：只将 Task 6 文件的 staged binary diff 应用到 detached `148b935c` 临时 worktree；首次 Go 链接只因 clean worktree 缺少 `libmornlea_engine` 失败，随后按仓库规则运行 `make rust` PASS，并从头执行 `go test ./internal/config ./internal/companion -run 'Agent|AIConfig|Contract' -race -count=1 -timeout=120s` 连续两遍 PASS、`go test ./internal/config -race -count=1 -timeout=120s` PASS、`go test ./internal/companion -race -count=1 -timeout=120s` PASS、`go vet ./internal/config ./internal/companion` PASS、`go test ./internal/archcheck -count=1 -timeout=120s` PASS；Task 6 文件 `gofmt -l` 无输出，`git diff --check` PASS，且删除临时 worktree 前无残留 `go test` 进程。
- 裁决边界：共享 worktree 当时由 Task 7A machine-contract fixtures 与 Task 8 storage v5 的并发预期 RED 阻断；这些文件未被修改或暂存，也未纳入 Task 6 的通过证据或裁决。

### Task 7A machine-contract 前置实施与评审记录

- 起始工作基线为 `148b935c`；Task 6 独立提交整合后，Task 7A 以 `5812be64` 交付 MCP v1 `domain_result_codes`、find/query strict success/failure `oneOf`、合法/非法 golden、Go fixture consistency、Python closed union 与 normal domain-result Planner 路径。该提交不实现 Go canonical registry、frozen projection、snapshot registry 或 MCP runtime。
- 初次独立裁决为 SPEC PASS、QUALITY FAIL。QUALITY 发现 discovery 的 `output_schema` 漂移仍访问旧的顶层 `outputSchema.properties`；实际 callback `KeyError` 被 adapter 统一映射成预期 `PlannerUnavailable`，而参数矩阵又没有 mutation-completion sentinel，导致 mock crash 可伪装为 schema rejection。QUALITY 同时拒绝 `domain.common` 的宽松公开 `ValidatorHint` 与 `domain.mcp_v1` 的严格同名定义并存。
- repair `038c4b86` 先用 delivered-response sentinel 复现 `output_schema` 单例失败（`1 failed, 7 passed`），再改为 `outputSchema.oneOf[0].properties.terrain.maxItems` 并断言 SDK 发出恰好一次 `tools/list`、收到的 payload 确实携带 mutation；同文件其余 transport fault injection 也补充 intended-response/invocation sentinel，避免内部异常被 `PlannerUnavailable` 掩盖。另以 runtime identity RED 证明两份 `ValidatorHint` 分叉，再把唯一严格定义收敛到 `domain.common`（strict string、UTF-8 1..256 bytes、无 NUL/control/edge whitespace），由 MCP v1 直接复用。
- repair 后独立 SPEC 与 QUALITY 均 PASS，Task 7A 裁决 Accepted；Task 7 的 checkbox 仍保持未勾选，下一阶段 Task 7B 才实现 Go canonical registry、frozen projection/snapshot registry 与 MCP v1 runtime。
- 初次验证：`go test ./internal/companion -run 'ContractFixture' -count=1` PASS；`cd services/companion-agent && uv run pytest tests/test_contracts.py tests/test_mcp_adapter.py tests/test_planner.py -q` 为 `118 passed`；`uv run ruff format --check . && uv run ruff check . && uv run mypy src` PASS；完整 `uv run pytest -q` 为 `401 passed`；`git diff --check` PASS。
- repair 验证：相同 Go fixture command PASS；相同 focused Python command 为 `119 passed`；ruff format/check 与 mypy PASS；完整 Python 从 `401` 增至 `402 passed`；`git diff --check` PASS。Task 8 的并发 `internal/server`/`internal/storage` 工作树改动未被修改、暂存或纳入本裁决。
- ledger 收尾验证：`openspec validate --all --strict --no-interactive` 为 `80 passed, 0 failed`；`git diff --check` PASS。

### Task 7 实施前设计裁决

- 预检基线 `f56b42bf` 证实现有 1,089 条 height + `ExposedBlocks<=256` 不能回答任意 `query_terrain` 体素，也会让未进入 exposed cap 的 mine 目标跳过方块语义；禁止由 MCP handler 回读 live world 填洞。
- 原规划 commit `3a713a78` 的独立 review 判 FAIL（2 Important + 3 Minor）：缺少 non-validator domain failure 的 strict machine wire；exposed 派生可能在 18,513 次主采样外回读 world；terrain exact wire/digest 边界、名称跨域唯一性和可实现的 cancellation 语义不完整。该 review 触发本轮 contract amendment，不能继续声称 Task 1 fixture 无需修改。
- 本轮 repair 基于 feature HEAD `9d7b5685`，保留其 Task 8 裁决且未查看/吸收 `main`；本提交只修订 OpenSpec planning artifacts，不提前修改 machine contract 或生产/测试代码，实际 contract/Python amendment 是 Task 7 的首个 RED/GREEN prerequisite。
- proposal/design、`companion-agent-mcp-tools` 与 `companion-planner` delta spec、Task 7 文案已同步：manifest 每工具 `domain_result_codes`、find/query strict success/failure `oneOf` 与 Python normal tool-message 路径；fixed dense projection、完整垂直 ±8、ready/missing、全有或全无 terrain、最多 18,513 world read 后纯 cache exposed、精确 mine validator；BE/Base64/unused bits/exact deterministic digest/53 KiB 与 96 KiB RED；分别在 BlockID/ItemID 域内唯一的 core canonical name；以及 registry-owned cancellation 的 bounded in-flight 语义。Task 7 保持未勾选，Task 1/4 历史完成记录不回写，但 Task 7 首先修订 machine contract/Python tests/types并重验相关 focused gates。
- 版本矩阵保持 protocol v32、player v8、chunk v9、metadata v3、companions v4→v5、hostile v1、engine ABI v9、client ABI v13、scenario v20；MCP application contract 仍为 v1，该裁决不触发游戏 wire、存档或 ABI 升版。
- 规划产物验证：`openspec validate --all --strict --no-interactive` exit 0，80 passed/0 failed；`git diff --check` exit 0。

### Task 8 实施前设计裁决

- 预检基线 `3a713a78` 证实现有 v4 隐式 active/summary 值模型、`WorldStore` 无 existence probe、启动晚于模拟出生才能取得新 body，以及 persistence 直接 `revision+1`/裸 Summary 写回，无法独立满足 v5 identity-first、canonical-zero 与无损 autosave。
- 原规划 commit `9d7b5685` 的独立 review 判 FAIL（1 Critical、2 Important、1 Minor）：active canonical-zero reconcile要求不存在的transition operation；I/O failure scenario把post-rename parent sync也错误要求旧v4不变；Task 8 server regex漏掉两个受v5/staging影响的既有测试；entropy顺序不足以写精确原子RED。
- proposal/design、`companion-persistence`、`companion-agent-memory`、`companion-identity-configuration` delta spec 与 Task 8 文案已同步 32-byte envelope、393,904-byte 精确上限、legacy epoch/operation、provisional body、mandatory metadata-only probe、persistence carry-through 及 Task 8→10 staging；Task 8 保持未勾选，未修改 Task 1 contracts。
- 本轮 repair 基于 feature HEAD `1cdfba94`，proposal 的目标/范围无变化故保持原文；design、`companion-agent-memory`、`companion-persistence`、Task 8/10 与本 ledger 同步 higher-epoch active-zero fence/replay、pre/post-rename原子语义、完整server gate和精确entropy顺序。Task 8保持未勾选，不修改v5或HTTP layout。
- 版本矩阵保持 protocol v32、player v8、chunk v9、metadata v3、companions v4→v5、hostile v1、engine ABI v9、client ABI v13、scenario v20；本裁决不查看或吸收后续 `main` 前进，不触发游戏 wire 或 ABI 升版。
- 规划产物验证：`openspec validate --all --strict --no-interactive` exit 0，80 passed/0 failed；`git diff --check -- openspec/changes/extract-companion-agent-service` exit 0。

### Task 8 实施、修复与评审记录

- `task8_storage_v5` 从 `148b935c` 开始，以 `11f897c7` 交付 schema v5 strict
  codec/merge、v1..v4 decode-only、committed v5 golden/fuzz、Memory/Disk
  metadata-only probe、原子替换、persistence metadata carry-through，以及
  identity-first synchronous bootstrap；未实现 Task 9/10 的 lease、MCP、Planner/
  Dialogue cutover 或 Agent memory mutation。
- 初次实现保留旧 v1..v4 fixture 字节与 digest，新
  `testdata/companions-v5.bin` 为 1,252 bytes、SHA-256
  `9f267e9d1fbcb7f4a83d38c699595d1d5ab4c02d13cab54cc0db8d7b16c89391`。
  完整 focused GREEN 包含 storage/companion race、root storage Companion race、
  persistence Companion race、完整 server race、两个受 staging 影响的指定 server
  tests、archcheck、full affected vet 与 diff-check；完整 server gate 确实执行
  `TestM5StageAcceptancePersonaDialogueEndToEnd` 和
  `TestCompanionDialogueSummaryLifecycle`。
- 对 `11f897c7` 的初次独立 SPEC 评审为 PASS；初次独立 QUALITY 评审为 FAIL，
  发现三项：`companion_restore_test.go` 用 current v5 `Encode` 却沿用 legacy v3
  offsets，reserved flags 未跨 namespace 且 kind/follow/deadline 未跨 lifecycle/
  mirror，形成假绿；最近 `internal/storage/companion/AGENTS.md` 仍把 v4 写成
  current 且缺少 v5 identity/lifecycle/mirror/tombstone 与现存 strict/golden
  事实；Memory `LoadCompanions`/`SaveCompanions` 在 `Close` 后未与 probe/Disk
  一致返回 `os.ErrClosed`，root contract 又跳过 Memory。
- repair `660c02a1` 以真实 RED 证明错误基线为 schema 5、旧 flags offset 读到
  `0x0` 而非 `0x7`、Memory close 返回 not-found 而非 closed；随后让 legacy v3
  直接读取 committed v3 fixture，current v5 offset 完整跨过 namespace、flags、
  epoch、revision、operation 与 summary prefix/bytes，并在每次 patch 前断言原
  字段。repair 同步最近 AGENTS，且只让 companion probe/Load/Save 统一 Memory
  Close 语义，不扩改其他既有 Memory API。
- repair staged binary diff 单独应用到 detached `11f897c7` 临时 worktree；首次
  clean checkout 链接仅因缺少未跟踪 Rust artifact 失败并清理，随后按指南先跑
  `make rust` PASS，再从头通过 `go test ./internal/storage/companion -race -count=1`
  （package 2.297s）、`go test ./internal/storage -run 'Companion' -race -count=1`
  （package 1.881s）、`go test ./internal/archcheck -count=1`（package 5.613s）、
  full affected `go vet` 与 `git diff --check`；临时 worktree 已清理。
- `11f897c7` + `660c02a1` 的最终独立 scoped SPEC 与 QUALITY 评审均 PASS，Task 8
  裁决 Accepted。Task 7 checkbox 保持未勾选，下一实施阶段仍是 Task 7B；不得
  因 Task 8 完成而提前进入 Task 9。
- Task 8 收尾后 change 进度为 7/12；
  `openspec validate --all --strict --no-interactive` 为 80 passed/0 failed，
  `git diff --check -- openspec/changes/extract-companion-agent-service/{tasks.md,ledger.md}`
  PASS。
- 收尾后的完整 server race 在 `136efc0c` 基线上暴露 repair 1 回归：
  `TestNewHostRetiresExistingCompanionsWhenConfigEmpty`、
  `TestNewHostDoesNotRepeatInactiveRetirement` 与
  `TestCompanionDialogueSummaryLifecycle` 均因 `file already closed` 失败。clean
  detached 复现证明三条用例都刻意在 `Host.Shutdown` 后检查同一个
  MemoryStore，其中 dialogue lifecycle 还会用同一 store 启动第二个 Host，
  以验证真实落库与跨重启恢复；不能通过改写测试/helper 绕过该断言。
- 契约裁决：repair 1 要求 Memory companion Load/Save 在 Close 后与 Disk 一致
  返回 `os.ErrClosed` 的原 QUALITY Minor 与既有 Memory post-Close 可观测、可复用
  语义冲突，因此有意识地撤回该 Minor。Disk Load/Save 的 closed 拒绝与 Memory/
  Disk `CompanionsExist` probe 的 `os.ErrClosed` 语义保持不变；repair 1 对 committed
  v3 fixture、完整 v5 offsets/patch 前断言及最近 AGENTS 的 Important 修复全部保留。
- repair 2 `0dc592c8` 只移除 Memory companion Load/Save 的 closed 拒绝，并在根
  store contract 正向证明 Memory 连续 Close 后仍可 Save+Load、Disk 继续拒绝；
  最近 companion AGENTS 与 Task 8 report 同步该局部事实，三条 server 测试未修改。
  detached `136efc0c` 隔离门禁通过 targeted 三回归、storage/companion race、root
  storage Companion race、persistence Companion race、完整 server race
  （package 188.862s）、两条指定 staging tests、archcheck、full affected vet 与
  diff-check；fixture 未修改。
- `0dc592c8` 最终独立 scoped SPEC 与 QUALITY 评审均 PASS，Task 8 继续裁决
  Accepted；change 进度仍为 7/12，Task 7 整体仍 Pending，下一阶段仍是 Task 7B，
  不提前进入 Task 9。

### Task 7B Go frozen snapshot/MCP 实施、修复与评审记录

- Task 7A 已由 `5812be64` 与 repair `038c4b86` 完成 machine-contract/Python
  前置并裁决 Accepted；其初次 SPEC PASS/QUALITY FAIL 与 repair 后双 PASS 历史
  保持见上文，不因 Task 7B 收尾改写。
- `task7_mcp_main` 从 `22991a82` 开始，以 initial `b00481e0` 交付 Go canonical
  registry、33×17×33 frozen terrain/digest、四槽 snapshot registry、六个纯工具、
  loopback MCP v1 outer/SDK service 与 Host component lifecycle；未接入 Task 9 的
  Planner run wiring，也未实现 Task 10 的 Dialogue/memory/release 关服顺序。
- initial formal SPEC 评审为 FAIL：raw gate 在 Content-Type/body/UTF-8/envelope/
  version/pre-cancel 前已完整物化 snapshot，`Host.Shutdown` 在 world persistence
  失败时不可重试地关闭 MCP/registry，且 `bounded_name` 没有完整执行 strict
  schema。initial formal QUALITY 评审同为 FAIL，并额外拒绝普通 256-entry
  `list_affordances` 超过 24 KiB、`NewHost` MCP/world 构造失败回滚泄漏，以及
  validate-plan digest/terrain 循环缺少 cancellation checkpoint 等质量缺口。
- repair 1 `79b5965f` 拆分 non-copying capability authorization 与一次 snapshot
  materialization，定义 24 KiB 内坐标有序的最长完整 affordance 前缀，前置 strict
  bounded-name 校验，使 persistence 失败后的 shutdown 保留 MCP 可重试，补齐
  constructor reverse cleanup、digest/terrain inner-loop checkpoint 与陈旧注释清理。
- repair 1 后独立 scoped QUALITY 评审 PASS；scoped SPEC 仍 FAIL：
  `find_visible_blocks.block_names` 中合法但未知名称位于前项时，会抢先返回 normal
  `unknown_block`，掩盖后项 Unicode blank/control/NUL/超 64 UTF-8 bytes 的
  schema-invalid。repair 2 `92f80e4f` 改为第一遍完整 bounded-name/去重校验、
  第二遍 canonical lookup；direct 与真实 SDK 四组混合顺序 RED 均转绿。
- repair 2 后最终独立 SPEC 与 QUALITY 评审均 PASS，Task 7 裁决 Accepted。
- 同 feature lineage 的完整 `go test ./internal/server -race -count=1` 已在 Task 8
  memory reuse repair `0dc592c8` 进入后执行并 PASS（214.301s）；repair 1/2 复用
  该证据，没有谎报为重新执行的 full server gate。
- repair 1 focused gates：三包
  `CanonicalName|MCP|Snapshot|Terrain|PlanningTool|Shutdown|NewHost` race PASS
  （core 1.198s、companion 4.988s、server 8.031s）；完整 companion race PASS
  （13.718s）；server `MCP|Shutdown|NewHost` race PASS（5.142s）；archcheck PASS
  （5.519s）；affected vet、`go mod tidy -diff`、OpenSpec strict 80/80、gofmt 与
  diff-check 均 PASS。
- repair 2 focused gates：direct mixed-order race PASS（1.862s）、真实 SDK
  mixed-order race PASS（1.804s）；companion `PlanningTool|PlanningFindBlocks`
  race PASS（3.911s）；server MCP race PASS（2.367s）；archcheck PASS（5.156s）；
  OpenSpec strict 80/80、gofmt 与 diff-check 均 PASS。
- Task 7 closeout 后 change 进度为 8/12；下一任务为 Task 9。Task 8 的完整实施、
  repair 与 Accepted 记录保持原样，未查看、比较或吸收 main。

### Task 9 Planner cutover 实施与评审记录

- `task9_planner_cutover` 从 `6c5c4011` 开始，以 `84c03161` 把生产规划切到 Agent HTTP v1
  与 frozen snapshot MCP；持久 namespace lease、global 4/per-companion 1 gate、bounded worker、
  当前 tick 重验和既有 Task Runner 保持 Go 权威，旧 direct-model Planner 从生产路径删除。
- initial SPEC/QUALITY 均 FAIL。repair `edfb1574` 关闭 strict step presence/type、独立
  acquire/heartbeat deadline 与 fencing、tick-owned attempt/correlation/gate 释放、target/chunk
  当前世界重验；其后 SPEC 仍 FAIL、QUALITY PASS。repair `b2c75a6c` 将成功 status 下的
  overflow、unknown field 与 trailing JSON 精确归为 invalid plan，同时保持 transport 与 identity
  mismatch 的 unavailable 分类；最终独立 SPEC/QUALITY 均 PASS，Task 9 Accepted。
- canonical gates：Planner/Task/AgentUnavailable/Snapshot race PASS（companion 2.518s、server
  11.704s）；Agent race 4.139s；config race 1.578s；archcheck 5.404s；三个 cmd package、
  affected vet、tidy、gofmt/diff-check 与 OpenSpec strict 80/80 均 PASS。

### Task 10 Dialogue、memory 与 shutdown 实施与评审记录

- `task10_dialogue_memory` 从 `012a4b86` 开始，以 `1f6006f6` 完成 Dialogue cutover、shared
  gate/tick correlation、terminal accepted reservation、memory commit/reconcile/epoch/tombstone、
  v5 mirror dirty 与单次广播，并实现 save/flush→Release→close 的可重试 shutdown。
- initial SPEC/QUALITY 均 FAIL。四轮 repair `6ef874b1`、`7283b746`、`c2cebeb9`、`6bdf2b7e`
  依次关闭 in-flight persistence revision、unknown/stale reconcile、inactive lifecycle、run 与
  finalization context 所有权、重试 re-arm、旧 outcome drain 后 exactly-once re-arm/no-spin；最终
  独立 SPEC/QUALITY 均 PASS，Task 10 Accepted。
- canonical gates：shutdown 三测试 race count 20 PASS（4.612s）；MemoryReconcile/UnknownCommit/
  Shutdown/Release race PASS（5.078s）；broader companion/server race PASS（4.558s/95.791s）；
  persistence race 2.813s；archcheck 5.319s；三个 cmd package、affected vet、tidy、gofmt/
  diff-check 与 OpenSpec strict 80/80 均 PASS。

### Task 11 真实跨语言合同实施与评审记录

- `task11_cross_language` 从 `e1bf9e52` 开始，以 `efa85718`、`a6b4d059`、`3635b6a1`、
  `356fe396`、`41add71e` 实现真实 Python MCP SDK↔Go MCP 与真实 FastAPI/Uvicorn↔Go
  `AgentClient` 进程合同；`2f6ab0ba` 记录实施证据。测试只使用 loopback、临时 SQLite 与
  deterministic model port fake，不访问 provider/DNS/外网，也不启动游戏窗口。
- MCP 覆盖 `2025-11-25` initialize→initialized、Tools-only discovery/call、outer gate、materialized
  view cancellation；HTTP 覆盖 lease/plan/dialogue/memory/cancel、proposal precommit 与 CAS/replay。
  shared fixture 真实暴露并修复 `list_affordances` 坐标排序漂移。最终独立 SPEC/QUALITY 均 PASS，
  Task 11 Accepted。
- canonical gates：`make companion-agent-integration` PASS；Python ruff/mypy、focused 81 passed、
  brief 集合 117 passed；Go 真进程 race server 8.822s；archcheck 5.645s；affected vet/tidy、
  diff-check 与 OpenSpec strict 80/80 均 PASS。

### Task 12 CI、文档、版本与全量门禁实施记录（Review pending）

- `task12_ci_docs_gates` 从 Task 11 closeout `9423f067` 开始。RED archcheck 证明
  `openspec/config.yaml` 仍为 companions v4，Make 缺少 locked Python check，CI 缺少 Python
  3.12、固定 uv/cache 与 check/integration 调用；engine ABI v9/client ABI v13 无歧义且保持不变。
- `9dfdbc69` 增加 `make companion-agent-check`，在既有 macOS integration job 复用同 SHA native
  artifacts 后运行 locked Python check 与真实 integration，并新增 service/shared-contract/CI/Make/
  no-shell-FFI-embed archcheck；只把 `companions.ai` v4 升为 v5。focused archcheck PASS（5.532s），
  `make companion-agent-check` PASS（403 passed）。
- `49fbc7f9` 更新双语 README、architecture、configuration、LAN、test quickstart 与 progress：明确
  Go 世界权威、Python 独立 Agent 服务、loopback-only、启动/关闭/失败语义、严格 v1 配置、v5
  migration/backup/rollback 以及完整版本矩阵；文档后 archcheck 与 OpenSpec strict 80/80 PASS。
- 第一轮完整 race 真实运行 392.59s 后唯一在 `internal/server` FAIL：九个旧 mine/place tests
  直接 unavailable、两条 restore tests 各 60s timeout。focused 复现排除并发 flaky 后确认 Task 9
  删除 direct-model endpoint 时两个测试 Host helper 留下空 model block；`3394fc19` 改用已有 typed
  planner test seam，生产路径不变，原失败 11 tests 全绿（package 12.281s，real 16.29s）。
- clean `3394fc19` 从头重跑 mandatory gates：Python locked/Ruff/mypy/403 tests、gofmt、full vet、
  full Go race（server 247.013s，real 248.75s）、Rust locked release、Make check、真实 integration
  （server 9.332s）、diff-check 与 OpenSpec strict 80/80 全部 PASS；无 provider/外网/游戏窗口。
- Task 12 实施阶段未修改 `tasks.md`。独立 SPEC/QUALITY 评审尚未记录；本行保持 Review pending，
  只有双评审通过后才允许由控制会话完成 Task 12 closeout。

## 整分支终审与门禁

- 整分支 SPEC review：待记录。
- 整分支 QUALITY review：待记录。
- Python locked/lint/type/test：clean `3394fc19` PASS；locked sync、Ruff format/check、mypy 23 files、pytest 403 passed；`make companion-agent-check` 同样 PASS。
- Go focused/race/archcheck/vet/gofmt：首轮 full race 真实暴露测试 seam 缺口并由 `3394fc19` 修复；原失败 11 tests PASS；最终 `go test ./... -race`、full vet、archcheck 与 gofmt 全部 PASS。
- Rust baseline/build：clean `3394fc19` preflight 与 mandatory `make rust` 均 PASS；Rust 1.97.1 locked release，engine ABI v9/client ABI v13 未改。
- 真实跨语言合同测试：`make companion-agent-integration` PASS；companion 2.745s、server 9.332s，real 10.73s。
- OpenSpec strict：clean `3394fc19` PASS，80 passed/0 failed，real 1.57s。
- 规划产物门禁：`git diff --check` PASS；版本/CI/Make/service archcheck PASS。
- 回滚/备份人工文档检查：PASS；LAN 文档同时说明 world+SQLite 联合备份、v1..v4→v5 迁移和旧程序不得写 v5 的回滚边界。
