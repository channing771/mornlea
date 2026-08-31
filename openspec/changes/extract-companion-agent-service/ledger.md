# extract-companion-agent-service ledger

## 基线与规则

- Change：`extract-companion-agent-service`；认领基线 `8b8891a3`。
- 执行模型：每个 Task 使用 fresh implementer；每项完成后由独立 SPEC reviewer 与 QUALITY reviewer 双裁决；控制会话不直接实现生产代码。
- 修复循环：单任务最多 5 轮；未通过双评审不得勾选 `tasks.md`。
- 验证复用：只有相同基线 SHA、命令与范围的完整输出可复用；所有结果记录实际 exit code，不把未完成命令写成通过。
- 当前版本事实：protocol v32、player v8、chunk v9、metadata v3、companions v4、hostile v1、engine ABI v9、client ABI v13、scenario v20。
- 计划目标：仅 companions 升 v5；Agent HTTP application contract v1；MCP tool contract v1；MCP wire `2025-11-25`。
- Ruling: 以执行时 `main` 的 engine ABI v9/client ABI v13 为不变基线，本 change 不认领 native ABI 升版 — 代码与用户更新的项目指南高于旧规划中的 v8/v11/v12 文档事实 — 若裁决错误，成本是收尾版本文档与钉死测试需重做，不改变 Agent HTTP/MCP 合同。
- Ruling: Task 6 只移除 Go config 的 provider/direct-model 生产语义并新增强类型 Agent HTTP client；现有 Planner/Dialogue direct-model 实现保留为未接线的编译过渡，由 Task 9/10 在权威编排切换时删除 — 这避免 Task 6 提前改变运行路径，也不引入 fallback、隐式映射或 backend 开关 — 若裁决错误，成本是 Task 9/10 的删除面扩大，不影响 Task 6 合同。

## 规划裁决

- Python Planner graph 不配置持久 checkpointer；Dialogue 仅 transient graph state；SQLite 只保存 compact MemoryState/CAS、lease 与 tombstone 元数据。
- MCP 外层 raw envelope 在 Go SDK 前拒绝 batch/GET/ping/subscription/其他方法；显式 capabilities 只有 Tools 且 `listChanged=false`。
- 当前共同 MCP wire 下跨语言 request cancellation 不可靠；snapshot registry 用自己的 deadline/cancel/TTL 收口，并由真实跨语言测试覆盖。
- accepted Dialogue reservation 通过首次 tick 重验后不再由后续 generation 变化撤销；commit 只按 operation/epoch 关联。
- Go 是 task/world/lifecycle/epoch 权威，Python 是运行期 compact memory 权威；没有 direct-model fallback、remote MCP 或 Docker。
- Task 7 terrain ruling：以 `floor(companion.Position)` 为中心冻结固定 33×17×33（水平 ±16/垂直 ±8）dense projection；ready bitmap、1,089 个 height 与 18,513 个 `BlockID` 的 data plane 每份 ≤40 KiB、四槽 ≤160 KiB。`query_terrain` 只读请求体素并全有或全无，未 ready/越界为 `out_of_bounds`；mine 对精确 frozen block 复用既有 validator，不依赖 `ExposedBlocks<=256`，因此 Chest/Furnace 继续接受、农业/火把/无掉落/未交付多掉落继续拒绝。规划 ±8 与寻路 ±4 明确解耦；若裁决错误，成本是 Task 7 snapshot/validator 重做，不影响 Task 1 machine-readable contract、游戏 wire、存档或 ABI。
- Task 7 naming ruling：`internal/core` 作为 `BlockID`/`ItemID` 所有者新增唯一 canonical English `snake_case` registry；UI 中文 display name 不复用，Planner place 白名单只保留语义 ID 集并从 core 派生拼写。未知 ID/name fail closed，不生成数值/中文 fallback；Task 1 schema/golden 保持不变并由 consistency tests 交叉锁定。
- Task 8 wire ruling：v5 保持 32-byte envelope；payload 是 namespace[16] 加按 `CompanionID` 排序的 record，record 固定 body[221]+flags(active/task/FIFO 为 bit0/1/2)+epoch u64，active 总带 revision/operation/summary-length+bytes 后接 task/FIFO，inactive只带 tombstone且 flags=0。合法可达上限固定 `MaxFileLength=393,904`；v1..v4 只读，encoder只写v5。若裁决错误，成本是 v5 golden/codec/磁盘上限与迁移重做，不影响游戏 wire 或 ABI。
- Task 8 migration ruling：legacy 是隐式 epoch0并统一落epoch1；v4 active 非空 summary 迁为 rev1+fresh operation，active 空 summary 使用 canonical-zero且不存在第二个 active migration operation，inactive使用 fresh tombstone。新 ID 先以 metadata `SpawnDimension`、`SpawnAnchor*16+0.5`、`core.MaxY+1`、零朝向/空背包 provisional body 同步原子写v5，entropy/Save失败则 persistence/world/Agent/MCP均不构造；模拟 ready 后 Observe 覆盖位置。
- Task 8 staging ruling：Task 8 persistence只 deep-clone/carry-through namespace/lifecycle/mirror/tombstone并checked推进aggregate revision；body/task autosave绝不从旧direct Dialogue裸 Summary改写mirror，也不实现Agent memory mutation。Task 9 Planner不改memory；Task 10删除/替换裸写路径，只在Agent commit/reconcile成功结果回tick且通过epoch/operation关联后整体更新mirror并mark dirty。
- Task 8 empty-config ruling：`WorldStore`必须提供Memory/Disk同语义的metadata-only existence probe；missing+empty只probe且0 Load/Save/create，existing+empty才Load并迁移/退休，already all-inactive v5不推进revision/epoch/tombstone且0 Save。pre-rename失败保持旧正式字节；parent-directory sync在rename后失败仍令启动失败但正式文件是完整v5；overflow返回`ErrCorrupt`语义且在entropy/mutation/Save之前失败。

## 任务记录

| Task | Implementer | 起始 SHA | RED/GREEN 与提交 | SPEC 评审 | QUALITY 评审 | 裁决 |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | `task1_contracts_impl` | `af57e420` | RED：缺少 HTTP schema；复审 RED：UTF-8 byte/权威常量、非确定 `oneOf` 与未支持 keyword；GREEN：focused `-count=100`、companion race、diff-check；提交 `36eed9d9`、`7d019d1c`、`a1640211` | `task1_spec_review` round 3 PASS | `task1_quality_review` round 3 PASS | Accepted |
| 2 | `task2_python_scaffold_impl` | `5d368f8c` | RED：package 缺失；复审 RED：exact const、URL/secret/path、golden path 与 import boundary；GREEN：locked sync、ruff/format、mypy、focused 两轮各 199 passed、diff-check；提交 `91bca693`、`665d609a`、`40c57fce` | `task2_spec_review` round 3 PASS | `task2_quality_review` round 3 PASS | Accepted |
| 3 | `task3_memory_impl` | `ea82d028` | RED：缺少 memory module；复审 RED：operation 复用、lease/SQLite 取消窗口、损坏库修补、重复 cancel 与 receipt 篡改；GREEN：focused 61 passed、full 260 passed、locked sync/ruff/mypy/diff-check；提交 `dcb99bc6`、`225fdb09`、`f68a6eca`、`7a1af977` | `memory_design_audit` round 4 PASS | `openspec_artifacts` round 4 PASS | Accepted |
| 4 | `openspec_artifacts` | `69f6e6ec` | RED：缺少 Planner/adapters；复审 RED：schema 漂移、transport/envelope 上限、validator wrapper 与 JSON 类型混淆；GREEN：focused 77 passed、full 337 passed、import boundary 48 passed、locked sync/ruff/mypy/wheel/diff-check；提交 `06473b29`、`e4b5dc8f`、`0e773804` | `task4_graph_spec_audit` round 3 PASS | `task4_quality_review` round 3 PASS | Accepted |
| 5 | `task5_http_research`、`task5_startup_cancel_fix` | `f4009ec0` | RED：缺少 Dialogue/FastAPI；复审 RED：HTTP disconnect、重复取消、响应 fence、关闭与 startup 所有权；GREEN：focused `56 passed`、full Python `393 passed`、import boundary `48 passed`、locked/ruff/mypy/diff-check；提交 `fb569bdd`、`ff7b5be9`、`962e361e`、`2d9042bf` | `task4_fix_quality` round 4 PASS | `task5_quality_review` round 4 PASS | Accepted |
| 6 | 待派发 | 待记录 | 待记录 | 待记录 | 待记录 | Pending |
| 7 | 待派发 | 待记录 | 待记录 | 待记录 | 待记录 | Pending |
| 8 | 待派发 | 待记录 | 待记录 | 待记录 | 待记录 | Pending |
| 9 | 待派发 | 待记录 | 待记录 | 待记录 | 待记录 | Pending |
| 10 | 待派发 | 待记录 | 待记录 | 待记录 | 待记录 | Pending |
| 11 | 待派发 | 待记录 | 待记录 | 待记录 | 待记录 | Pending |
| 12 | 待派发 | 待记录 | 待记录 | 待记录 | 待记录 | Pending |

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

### Task 7 实施前设计裁决

- 预检基线 `f56b42bf` 证实现有 1,089 条 height + `ExposedBlocks<=256` 不能回答任意 `query_terrain` 体素，也会让未进入 exposed cap 的 mine 目标跳过方块语义；禁止由 MCP handler 回读 live world 填洞。
- proposal/design、`companion-agent-mcp-tools` 与 `companion-planner` delta spec、Task 7 文案已同步 fixed dense projection、完整垂直 ±8、ready/missing、全有或全无 terrain、精确 mine validator 与 core canonical name 裁决；Task 1 manifest/schema/golden 未改，Task 7 保持未勾选。
- 版本矩阵保持 protocol v32、player v8、chunk v9、metadata v3、companions v4→v5、hostile v1、engine ABI v9、client ABI v13、scenario v20；该裁决不触发 wire、存档或 ABI 升版。
- 规划产物验证：`openspec validate --all --strict --no-interactive` exit 0，80 passed/0 failed；`git diff --check` exit 0。

### Task 8 实施前设计裁决

- 预检基线 `3a713a78` 证实现有 v4 隐式 active/summary 值模型、`WorldStore` 无 existence probe、启动晚于模拟出生才能取得新 body，以及 persistence 直接 `revision+1`/裸 Summary 写回，无法独立满足 v5 identity-first、canonical-zero 与无损 autosave。
- proposal/design、`companion-persistence`、`companion-agent-memory`、`companion-identity-configuration` delta spec 与 Task 8 文案已同步 32-byte envelope、393,904-byte 精确上限、legacy epoch/operation、provisional body、mandatory metadata-only probe、persistence carry-through 及 Task 8→10 staging；Task 8 保持未勾选，未修改 Task 1 contracts。
- 版本矩阵保持 protocol v32、player v8、chunk v9、metadata v3、companions v4→v5、hostile v1、engine ABI v9、client ABI v13、scenario v20；本裁决不查看或吸收后续 `main` 前进，不触发游戏 wire 或 ABI 升版。
- 规划产物验证：`openspec validate --all --strict --no-interactive` exit 0，80 passed/0 failed；`git diff --check -- openspec/changes/extract-companion-agent-service` exit 0。

## 整分支终审与门禁

- 整分支 SPEC review：待记录。
- 整分支 QUALITY review：待记录。
- Python locked/lint/type/test：待记录。
- Go focused/race/archcheck/vet/gofmt：待记录。
- Rust baseline/build：规划前 `make rust` 已由控制会话记录为通过；实现后须在新 SHA 重跑。
- 真实跨语言合同测试：待记录。
- OpenSpec strict：规划产物完成后记录；实现后须重跑。
- 规划产物门禁：`openspec validate --all --strict --no-interactive` exit 0，78 passed/0 failed；`git diff --check` exit 0。
- 回滚/备份人工文档检查：待记录。
