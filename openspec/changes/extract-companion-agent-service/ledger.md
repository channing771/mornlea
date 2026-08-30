# extract-companion-agent-service ledger

## 基线与规则

- Change：`extract-companion-agent-service`；认领基线 `8b8891a3`。
- 执行模型：每个 Task 使用 fresh implementer；每项完成后由独立 SPEC reviewer 与 QUALITY reviewer 双裁决；控制会话不直接实现生产代码。
- 修复循环：单任务最多 5 轮；未通过双评审不得勾选 `tasks.md`。
- 验证复用：只有相同基线 SHA、命令与范围的完整输出可复用；所有结果记录实际 exit code，不把未完成命令写成通过。
- 当前版本事实：protocol v32、player v8、chunk v9、metadata v3、companions v4、hostile v1、engine ABI v8、client ABI v12、scenario v20。
- 计划目标：仅 companions 升 v5；Agent HTTP application contract v1；MCP tool contract v1；MCP wire `2025-11-25`。

## 规划裁决

- Python Planner graph 不配置持久 checkpointer；Dialogue 仅 transient graph state；SQLite 只保存 compact MemoryState/CAS、lease 与 tombstone 元数据。
- MCP 外层 raw envelope 在 Go SDK 前拒绝 batch/GET/ping/subscription/其他方法；显式 capabilities 只有 Tools 且 `listChanged=false`。
- 当前共同 MCP wire 下跨语言 request cancellation 不可靠；snapshot registry 用自己的 deadline/cancel/TTL 收口，并由真实跨语言测试覆盖。
- accepted Dialogue reservation 通过首次 tick 重验后不再由后续 generation 变化撤销；commit 只按 operation/epoch 关联。
- Go 是 task/world/lifecycle/epoch 权威，Python 是运行期 compact memory 权威；没有 direct-model fallback、remote MCP 或 Docker。

## 任务记录

| Task | Implementer | 起始 SHA | RED/GREEN 与提交 | SPEC 评审 | QUALITY 评审 | 裁决 |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | `task1_contracts_impl` | `af57e420` | RED：缺少 HTTP schema；复审 RED：UTF-8 byte/权威常量、非确定 `oneOf` 与未支持 keyword；GREEN：focused `-count=100`、companion race、diff-check；提交 `36eed9d9`、`7d019d1c`、`a1640211` | `task1_spec_review` round 3 PASS | `task1_quality_review` round 3 PASS | Accepted |
| 2 | `task2_python_scaffold_impl` | `5d368f8c` | RED：package 缺失；复审 RED：exact const、URL/secret/path、golden path 与 import boundary；GREEN：locked sync、ruff/format、mypy、focused 两轮各 199 passed、diff-check；提交 `91bca693`、`665d609a`、`40c57fce` | `task2_spec_review` round 3 PASS | `task2_quality_review` round 3 PASS | Accepted |
| 3 | `task3_memory_impl` | `ea82d028` | RED：缺少 memory module；复审 RED：operation 复用、lease/SQLite 取消窗口、损坏库修补、重复 cancel 与 receipt 篡改；GREEN：focused 61 passed、full 260 passed、locked sync/ruff/mypy/diff-check；提交 `dcb99bc6`、`225fdb09`、`f68a6eca`、`7a1af977` | `memory_design_audit` round 4 PASS | `openspec_artifacts` round 4 PASS | Accepted |
| 4 | `openspec_artifacts` | `69f6e6ec` | RED：缺少 Planner/adapters；复审 RED：schema 漂移、transport/envelope 上限、validator wrapper 与 JSON 类型混淆；GREEN：focused 77 passed、full 337 passed、import boundary 48 passed、locked sync/ruff/mypy/wheel/diff-check；提交 `06473b29`、`e4b5dc8f`、`0e773804` | `task4_graph_spec_audit` round 3 PASS | `task4_quality_review` round 3 PASS | Accepted |
| 5 | 待派发 | 待记录 | 待记录 | 待记录 | 待记录 | Pending |
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
