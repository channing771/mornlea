# SDD ledger — plan: openspec/changes/extract-companion-agent-service/tasks.md

## 恢复状态

- Task 1: complete (commits `36eed9d9`..`a1640211`, review clean)
- Task 2: complete (commits `91bca693`..`40c57fce`, review clean)
- Task 3: complete (commits `dcb99bc6`..`7a1af977`, review clean)
- Task 4: complete (commits `06473b29`..`0e773804`, review clean)
- Task 5: complete (commits `fb569bdd`..`2d9042bf`, review clean)
- Task 6 preflight: Ruling: 本任务只删除 config 的 provider/direct-model 生产语义并新增 Agent HTTP client；旧 Planner/Dialogue client 由 Task 9/10 删除 — 若错误，后续删除面扩大，但不引入运行 fallback。
- Task 6: complete (implementers `task6_go_agent_client`/`task6_strict_lifecycle_fix`, base `c871b1ec`, commits `0fd248d1`..`0ea46c31`, round 3 SPEC/QUALITY review clean)。
- Task 7A prerequisite: complete (implementer `task7_contract_prereq`, base `148b935c`, commits `5812be64` + `038c4b86`; initial SPEC PASS/QUALITY FAIL, repair SPEC/QUALITY PASS；focused Python `118→119 passed`，full Python `401→402 passed`)。
- Task 7: complete (implementer `task7_mcp_main`, commits `b00481e0`..`6c5c4011`; initial SPEC/QUALITY FAIL，Repair 1 SPEC FAIL/QUALITY PASS，Repair 2 SPEC/QUALITY PASS；Task 7A 与 7B 均 Accepted)。
- Task 8: complete (implementer `task8_storage_v5`, commits `11f897c7`..`22991a82`; initial SPEC PASS/QUALITY FAIL，Repair 1 scoped SPEC/QUALITY PASS 后由完整 server race 暴露 Memory post-Close reuse 回归，Repair 2 撤回错误 Minor 并最终 SPEC/QUALITY PASS；Accepted)。
- Task 9: complete (implementer `task9_planner_cutover`, commits `84c03161`、`edfb1574`、`b2c75a6c`; initial SPEC/QUALITY FAIL，Repair 1 SPEC FAIL/QUALITY PASS，Repair 2 final SPEC/QUALITY PASS；Accepted)。
- Task 10: complete (implementer `task10_dialogue_memory`, commits `1f6006f6`、`6ef874b1`、`7283b746`、`c2cebeb9`、`6bdf2b7e`; initial SPEC/QUALITY FAIL，四轮 repair 后 final SPEC/QUALITY PASS；Accepted)。
- Task 11: complete (implementer `task11_cross_language`, commits `efa85718`、`a6b4d059`、`3635b6a1`、`356fe396`、`41add71e`、`2f6ab0ba`; final SPEC/QUALITY PASS；Accepted)。
- Task 12: implementation and full gates complete, review pending (implementer `task12_ci_docs_gates`, base `9423f067`, commits `9dfdbc69`、`49fbc7f9`、`ee642a05`、repair `3394fc19`; 独立 SPEC/QUALITY 待记录)。
- 下一任务：完成 Task 12 独立 SPEC/QUALITY 评审；评审前不勾选 `tasks.md`。

## 执行前接口冲突扫描

| 产出 Task | 消费 Task | 共享文件/接口 | 裁决 |
| --- | --- | --- | --- |
| 1 | 2、4、5、6、7、11 | HTTP/MCP schema、manifest、golden | 后续只消费 checked-in contract，不发明平行 schema。 |
| 2 | 3、4、5、11 | Python config/domain/import boundary | 后续 adapter 经 domain port 组合，app 不反向渗入 domain。 |
| 3 | 5、8、10 | lease、RunGate、MemoryState/CAS | Python 为运行期摘要权威，Go v5 只作恢复镜像。 |
| 4 | 5、7、9、11 | Planner graph、model/MCP adapter | Go MCP 必须满足已钉住 wire；Task 9 才删除 direct-model Planner。 |
| 5 | 6、8、10、11 | Agent HTTP v1、Dialogue、memory routes | Go client 逐 route 强类型；terminal proposal 在 commit 前不持久。 |
| 6 | 9、10、11 | Go Agent config/client | Task 6 不提前接线或删除 Task 9/10 尚在使用的旧 client。 |
| 7 | 9、10、11 | frozen snapshot registry、`/mcp` | registry 自有 deadline/cancel/TTL，不依赖 raw request cancellation。 |
| 8 | 9、10、12 | `companions.ai` v5 identity/memory | identity 必须在 acquire/MCP/Agent 前原子落盘。 |
| 9 | 10、11 | Planner cutover、shared slots、snapshot | Task Runner 仍是唯一动作入口；Dialogue 复用同一并发槽。 |
| 10 | 11、12 | Dialogue/memory/shutdown | accepted reservation 不被后续 generation 撤销；关服顺序可观察。 |
| 11 | 12 | real Go/Python integration target | CI/Make 只调用固定无外网入口，不启动游戏窗口。 |
| 12 | 整分支 | docs、CI、versions、full gates | 只升 companions v5；保持执行时 main 的 engine v9/client v13。 |

## 任务内部一致性

| Task | 测试与实现文件 | 结论 |
| --- | --- | --- |
| 1 | fixtures 与 Go consistency tests | 一致，已完成。 |
| 2 | Python scaffold/config/domain 与 focused gates | 一致，已完成。 |
| 3 | lease/memory RED 与 storage/harness | 一致，已完成。 |
| 4 | fake model/MCP RED 与 adapters/graph | 一致，已完成。 |
| 5 | Dialogue/HTTP RED 与 lifespan | 一致，已完成。 |
| 6 | config/client RED 与 `internal/config`,`internal/companion` | 一致；旧 client 的生产删除归 Task 9/10。 |
| 7 | Task 7A contract/Python 前置；Task 7B registry/MCP RED 与 companion/server | 一致，已完成；官方 SDK、strict outer gate、冻结投影/digest、registry cancellation 与 Host lifecycle 均经两轮 repair 和独立双评审关闭。 |
| 8 | v1..v4→v5 RED 与 codec/bootstrap | 一致，已完成；metadata-only probe、retirement、identity-first save barrier、mirror/tombstone carry-through 与 Memory reuse 回归均已关闭。 |
| 9 | Planner orchestration RED 与 cutover | 一致，已完成；Agent HTTP/MCP cutover、共享 gate、严格计划分类、attempt/correlation 与当前世界重验均已关闭，Task Runner 仍是唯一动作入口。 |
| 10 | Dialogue/memory/shutdown RED 与 lifecycle | 一致，已完成；accepted reservation、memory fencing/reconcile 与可重试 shutdown 已经四轮 repair 和独立双评审关闭。 |
| 11 | 真进程 integration RED 与 Make target | 一致，已完成；真实 Python MCP SDK/Go MCP、真实 FastAPI/Uvicorn/Go AgentClient、registry cancellation 与 shared fixtures 已由独立双评审关闭，全程无外网/provider/游戏窗口。 |
| 12 | docs/CI/version/full gates | Ruling: 将旧 v8/v11/v12 规划漂移更正为主线 v9/v13，本 change 不升 native ABI — 若错误，需重做版本文档和钉死测试，Agent 合同不变。 |

## Task 7 完成恢复点

- Task 7A 已关闭 machine-contract/Python 前置门禁：Go `ContractFixture` PASS；focused contract/adapter/Planner 从 `118` 增至 `119 passed`；ruff format/check、mypy PASS；完整 Python 从 `401` 增至 `402 passed`；OpenSpec strict `80 passed, 0 failed`；diff check PASS。初版 `5812be64` 的 SPEC PASS/QUALITY FAIL 已由 `038c4b86` 修复为双 PASS。
- Task 7B `b00481e0` 交付 canonical registry、33×17×33 frozen terrain/digest、四槽 snapshot registry、六个纯工具、loopback MCP v1 outer/SDK service 与 Host lifecycle。initial SPEC/QUALITY 均 FAIL。
- Repair 1 `79b5965f` 关闭 non-copying authorization/一次 materialization、24 KiB affordance 最长完整前缀、strict bounded-name、可重试 shutdown、constructor reverse cleanup 与 digest/terrain cancellation checkpoint；独立 QUALITY PASS，但 SPEC 仍因 mixed-order name validation 可被前项 unknown 掩盖后项 schema-invalid 而 FAIL。
- Repair 2 `92f80e4f` 改为先完整验证所有 bounded names、再 canonical lookup；direct/真实 SDK RED 转绿，最终 SPEC/QUALITY 均 PASS。`6c5c4011` 勾选 Task 7 并记录 Accepted；focused race、archcheck、vet/tidy、OpenSpec 80/80、gofmt/diff-check 均通过，完整 server race 复用同 lineage 已记录的 214.301s PASS。

## Task 8 完成恢复点

- `11f897c7` 交付 schema v5 strict codec/merge、v1..v4 decode-only、committed v5 golden/fuzz、Memory/Disk metadata-only probe、原子替换、persistence metadata carry-through 与 identity-first synchronous bootstrap；初次 SPEC PASS、QUALITY FAIL。
- Repair 1 `660c02a1` 以 committed v3 fixture 和完整 v5 offset/原字段断言关闭 corruption 假绿，并更新最近 AGENTS；当时 scoped SPEC/QUALITY 均 PASS，`136efc0c` 完成首次收尾。但其把 Memory companion Load/Save 的 post-Close 语义强行对齐 Disk，随后完整 server race 的 retirement/dialogue restart 用例真实失败。
- Repair 2 `0dc592c8` 撤回该错误 Minor，保留 probe 与 Disk 的 closed 拒绝，同时正向钉住 Memory Close 后仍可 Save/Load 的既有可复用语义；三条 server 回归、storage/companion、root storage、persistence、完整 server race 188.862s、指定 staging tests、archcheck、vet 与 diff-check 全绿。最终 SPEC/QUALITY 均 PASS，`22991a82` 记录 Accepted；Task 8 未提前实现 Agent lease/Planner/Dialogue/memory mutation。

## Task 9 完成恢复点

- `task9_planner_cutover` 以 `84c03161` 把生产规划从 Go direct-model 切到 Agent HTTP v1 + frozen snapshot MCP，接入 persisted namespace lease、global 4/per-companion 1 shared gate、bounded Planner worker、当前权威 tick revalidation 与既有 Task Runner；删除生产 direct Planner，未越界接线 Task 10 Dialogue/memory/release。initial SPEC/QUALITY 均 FAIL。
- Repair 1 `edfb1574` 关闭四组问题：plan step strict `oneOf` 的 presence/null/zero/wrong-type 并在完成 correlation 后映射 `InvalidModelOutput`；acquire/heartbeat 独立 deadline 与迟到 response fencing；tick-owned monotonic attempt、generation/`Planning`、canonical RunID/SnapshotID 与 frozen digest 全匹配前不释放 gate；place/follow/目标 chunk revision 当前世界重验使用真实 `TouchChunkForTest`，dense Chest/Furnace 保持成功且无关 projection 变化不扩大失效。独立 SPEC 仍 FAIL、QUALITY PASS。
- Repair 2 `b2c75a6c` 关闭 malformed 2xx Plan response 分类：已确认 `/v1/plan` 成功 status 后，response overflow、顶层 unknown field 与 trailing JSON 归 `ErrAgentInvalidModelOutput`→`ErrPlannerInvalidPlan`→`TaskFailInvalidPlan`；transport/header/content-type/status/body I/O 及解码后 identity mismatch 仍为 unavailable。最终独立 SPEC/QUALITY 均 PASS，Task 9 Accepted。
- 最终 canonical gates：`go test ./internal/companion ./internal/server -run 'Planner|CompanionTask|AgentUnavailable|Snapshot' -race -count=1` PASS（2.518s/11.704s）；`go test ./internal/companion -run 'Agent' -race -count=1 -timeout=90s` PASS（4.139s）；config race PASS（1.578s）；archcheck PASS（5.404s）；三个 cmd package PASS（1.077s/22.818s/3.965s）；affected vet、`go mod tidy -diff`、gofmt/diff-check PASS；`openspec validate --all --strict --no-interactive` PASS（80/80）。
- Task 9 closeout 后 change 进度应为 9/12；下一任务为 Task 10。

## Task 10 完成恢复点

- `task10_dialogue_memory` 以 `1f6006f6` 完成 Agent Dialogue cutover、shared gate/tick correlation、terminal accepted reservation、memory commit/reconcile/epoch/tombstone、v5 mirror dirty 与单次广播、direct Dialogue/裸 summary 删除，以及 save/flush→Release→close 的可重试 shutdown；initial SPEC/QUALITY 均 FAIL。
- Repair 1 `6ef874b1` 关闭 persistence in-flight replacement dirty 重判与 checked revision reserve、reconcile per-companion 隔离/stale fence/unknown same-operation retry、shutdown quiescence、Release 失败重试和 dead `CompanionSummary` surface；独立 SPEC PASS、QUALITY FAIL。
- Repair 2 `7283b746` 让 acquire/reacquire 从完整 lifecycle 集合 reconcile inactive tombstone，并把 run cancellation 与有界 memory-finalization context 分离；caller timeout 后保留 lease/resources，下一次 Shutdown 可继续收敛。
- Repair 3 `c2cebeb9` 让每次新 Shutdown attempt 重新派发已有 unresolved reservation+pending，并修复共享 deadline 下旧 finalization context 继续派生 worker 的 timer 竞态。
- Repair 4 `6bdf2b7e` 处理上一轮 reconcile worker 到 deadline 才退出、outcome 尚未 drain 的边界：新 attempt 消费唯一旧结果后只 re-arm 一次，新派发失败不在同轮自旋；最终独立 SPEC/QUALITY 均 PASS，Task 10 Accepted。
- 最终稳定性与 canonical gates：Repair 3/4 shutdown 三测试 `-race -count=20` PASS（4.612s）；`MemoryReconcile|UnknownCommit|Shutdown|Release` race PASS（5.078s）；broader `Agent|Lease|Planner|Dialogue|Memory|Shutdown|CompanionSpeech` race PASS（companion 4.558s、server 95.791s）；persistence race PASS（2.813s）；archcheck PASS（5.319s）；三个 cmd package、affected vet、`go mod tidy -diff`、gofmt/diff-check 与 OpenSpec strict 80/80 全绿。
- Task 10 closeout 后 change 进度为 10/12；下一任务为 Task 11。

## Task 11 完成恢复点

- `task11_cross_language` 从 Task 10 closeout `e1bf9e52` 开始，以 `efa85718` 提交真实 Python MCP 子进程 RED；`a6b4d059` 关闭官方 Python SDK 的 `notifications/initialized` 可省略空 `params` 的跨语言兼容差异；`3635b6a1` 提交真实 Python Agent HTTP process RED；`356fe396` 完成真进程 harness、固定 Make target、materialized-view cancellation 与 shared `list_affordances` 坐标序合同修复；`41add71e` 补齐 HTTP 错误 request identity correlation；`2f6ab0ba` 记录完整实施证据。
- MCP 真进程由 Go test 持有真实 loopback listener/registry 与 Python 官方 SDK 子进程：精确验证 `2025-11-25` initialize→initialized→tools/list→tools/call、仅 Tools/`listChanged=false`、六工具 manifest/schema、StructuredContent 与 canonical TextContent、Origin 缺失/精确匹配，以及 GET/batch/ping/subscription/version/content-type/Host/Origin/Bearer/body outer gate 拒绝。materialized view 后的确定性 barrier 证明 Python timeout、registry 立即拒绝新 lookup、service Close 不等待 handler且迟到结果最终收敛。
- HTTP 真进程由 Go 预绑定 OS loopback listener 并传 inherited fd，Python 运行真实 `create_app`、Strict gate、lease manager、Planner/Dialogue harness 与临时 SQLite，仅 model port 使用 deterministic fake；生产 Go `AgentClient` 覆盖 live/ready、15 秒 lease acquire/heartbeat/release、namespace conflict/stale lease、真实 Plan→Python MCP client→Go MCP、nonterminal/terminal Dialogue、proposal 未预提交、commit/幂等重放/reconcile、active canonical-zero、inactive delete、显式 cancel 与 caller disconnect 后同伙伴立即新 run。HTTP/MCP fixtures 直接消费 Task 1 checked-in manifest/golden，错误面保持有界且不泄漏敏感正文。
- 最终独立 SPEC reviewer 与 QUALITY reviewer 均为 PASS，Task 11 Accepted。canonical gates：`make companion-agent-integration` PASS；Python ruff/mypy PASS、focused contract/adapter `81 passed`、brief Python 集合 `117 passed`；Go 真进程 focused race PASS（server 8.822s）、archcheck PASS（5.645s）、affected vet 与 `go mod tidy -diff` PASS；OpenSpec strict `80 passed, 0 failed`、diff/scope scan PASS。未运行或启动游戏窗口，未访问 provider/DNS/外网。
- Task 11 closeout 后 change 进度为 11/12；下一任务为 Task 12 CI、文档、版本矩阵与整分支全量门禁。

## Task 12 实施恢复点（Review pending）

- `task12_ci_docs_gates` 从 Task 11 closeout `9423f067` 开始；没有查看、比较或合并 main，版本裁决保持 protocol v32、player v8、chunk v9、metadata v3、hostile v1、engine ABI v9、client ABI v13、scenario v20，只把 `companions.ai` v4 升为 v5。
- RED archcheck 同时复现三项缺口：`openspec/config.yaml` 的 companions 仍为 v4；Make 缺少 locked Python check；CI 缺少 Python 3.12、固定 uv/cache 与 check/integration 调用。`9dfdbc69` 完成 Make/CI/archcheck/版本矩阵与 service 局部指南，focused archcheck PASS（5.532s），`make companion-agent-check` PASS（403 passed）。
- `49fbc7f9` 更新双语 README、architecture、configuration、LAN、test quickstart 与长期 progress，明确 Python 独立进程、Go 世界权威、loopback-only、严格 v1 配置、失败/关闭语义、v5 migration/backup/rollback，以及 `gates.sh` 不隐含 `make rust`、Python check 或真实 integration。文档后 archcheck 与 OpenSpec strict 80/80 PASS。
- 第一轮 `go test ./... -race` 在 392.59s 后唯一由 `internal/server` FAIL；focused 复现证明 Task 9 删除 direct-model endpoint 后两个旧测试 Host helper 留下空 model block，fake planner 没有通过 typed seam 注入。`3394fc19` 复用已有 `replacePlannerForTest`，生产路径不变，原失败 11 tests 全绿（package 12.281s，real 16.29s）。
- clean `3394fc19` 从头重跑 brief 全量清单并全部 PASS：Python locked/Ruff/mypy/403 tests、gofmt、full vet、full Go race（server 247.013s，real 248.75s）、Rust locked release、Make check、真实 integration（server 9.332s）、diff-check、OpenSpec strict 80/80。未访问 provider/外网，未启动游戏窗口。
- Task 12 仍是 Review pending，`tasks.md` 未改；下一步只做独立 SPEC/QUALITY 评审与相应 repair/closeout。
