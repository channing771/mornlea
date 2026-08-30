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
| 2 | 待派发 | 待记录 | 待记录 | 待记录 | 待记录 | Pending |
| 3 | 待派发 | 待记录 | 待记录 | 待记录 | 待记录 | Pending |
| 4 | 待派发 | 待记录 | 待记录 | 待记录 | 待记录 | Pending |
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
