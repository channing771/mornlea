# Ledger: eating-progress-hud

## 2026-08-26 认领与内容确认

- **Ruling: 认领 B-14 并批准三点位最小受控重叠 — 在途 11 行独占集覆盖全部功能落点，B-14 呈现主体全在无主文件（`internal/render/hud` 除 `container.go` 外、`internal/client` 新建），仅 `cmd/mornlea` 三点位装配需与 A-01/A-04 重叠（`app.go` 字段行、`app_frame.go` 构造与实参行、`app_lifecycle.go` 复位行，B-05 先例同形）— 排查期同轮评估 B-32（音频装配重叠 + cue 触发口径需先裁决）与等待批次合流，用户裁决选 B-14。**
- 认领提交 `cbf60ef3`（main，docs-only）；Discussion #71【状态变更】评论 `DC_kwDOToJS8M4BFPeZ` + `refresh-discussion.py --update`（76 行）；worktree `.worktrees/B-14-eating-progress-hud`，分支 `feat/B-14-eating-progress-hud`。
- **Ruling: 阶段 1 短设计获批（bounded）— 用户显式批准 — 客户端预测（不上协议，版本互斥让路 A 批次）、同锚点互斥复用采掘条形状（容量 267 quad/46912 bytes 与 scenario v19 零变化）、中断镜像覆盖输入归零/切格/换物、受伤/死亡不镜像记为已知简化；音频/动画/新 capture 场景为非目标。** 结论已写入 proposal.md 与 design.md（D1–D5）。

## 2026-08-26 Task 1：进食进度呈现实现（SDD）

- implementer 提交 `8bb70a31`：新建 `internal/client/eating_progress.go`（`EatingProgressTracker`，整毫秒累积、复位三源外加数量变化、钳制、时钟倒退零增量）与测试、`internal/render/hud/eating.go`（`EatingOverlay{Active, Progress}`、`appendEatingBar` 复用采掘条锚点/几何、互斥在函数内）与测试、`cmd/mornlea/eating_overlay_test.go`（FrameStreams 范式端到端）；`renderer.go`/`layout.go` 签名追加参数；三个裁决点位 `app.go`+3 行（另 gofmt 对齐重排）、`app_frame.go` Prepare 调用处构造块、`app_lifecycle.go`+2 行；5 个既有 hud 测试文件机械补 `EatingOverlay{}` 实参（容量常量 267/700/13312/46912 与断言零改动）。先红后绿（桩下 client 6 红、hud 4 红、cmd 2 红）。
- **Ruling: change 产物的「20 ms/tick」系撰写错误，实现以 `physics.FixedDelta`（50 ms、20 TPS）为准并回改产物 — 权威 tick 周期是 50 ms，20 ms 会使满程变 0.64 秒与「约 1.6 秒」的用户可观察结果矛盾；delta spec 的「以权威 tick 周期累积」措辞本就正确 — 控制会话撰写 design 时把「20 TPS」误写成「20 ms」。** proposal/design/tasks 已同步修正。
- **Ruling: 输入位在 `renderFrame` 作用域不可得，按 brief 的 fallback 条款以同源状态派生（`CursorCaptured && SecondaryButtonDown && FoodValue`）并记录 justCaptured 帧边界 — 与 `interactive.go` 的 `allowActions && actions.Use && holdingFood` 同源；无头路径恒假，capture/benchmark 输出逐字节不变 — predictor 不存 Control，是既有分层事实。**

- **SPEC 评审 PASS（0 MUST-FIX，S1–S8 全过）**；**QUALITY 评审 PASS（0 MUST-FIX，4 NIT）**。
- **Ruling: NIT-4 升级采纳——饥饿已满门控显示 — 权威侧 `sim/eating.go` 在 `hunger >= MaxHunger` 时不推进，客户端派生不查饥饿会使进度条假满，与「随后随权威结算消失」的可观察结果在该边界不符；`app_frame.go` 既有 `a.predictor.Hunger()` 权威镜像就在构造块作用域内，一个门控条件即可对齐 — 记入延期清单不如修掉，delta spec 激活条件已同步补「且权威确认的饥饿值未满」并新增 Scenario「饥饿已满不呈现进度条」。** NIT-1/2/3（时钟倒退显式测试、裸数字 4 换命名常量、不可达分支措辞）一并采纳进 R1。
- **R1 提交 `a216d9b6`**：饥饿门控落 `app_frame.go` 派生输入位（`hungerReady && hunger < core.MaxHunger`，仍在裁决点位内）+ 端到端用例（满时不出现 quad、降到 19 后正常起算；既有两用例适配未满饥饿；填充定位改按进食填充色扫描）；时钟倒退显式用例（半程→倒退帧仍 0.5→续帧精确 0.625）；裸 4 换 `closedHotbarPanelQuads + closedHotbarSelectionQuads + core.HotbarSlots`；不可达分支注释点明防零值误用。复跑 client/hud/cmd-short/archcheck 全绿、`gofmt`/`go vet` 干净。

## 2026-08-26 Task 2：变异验证与全量门禁

- **三变异各杀专属用例（均未提交、恢复后 `git status` 干净）**：互斥短路注释 → `TestAppendEatingBarMutuallyExclusiveWithMining` + cmd 端到端互斥帧断言红；slot 复位删除 → `TestEatingProgressResetsOnSlotItemAndCountChange/切换栏位清零` 红；饥饿门控注释 → `TestApplicationEatingOverlayGatedOnAuthoritativeHunger` 红。
- **全量门禁**：`gofmt -l .` 无输出；`go vet ./...` 干净；`go test ./... -race -count=1` 首跑仅 `internal/server` 的 `TestCompanionDialogueTerminalCoversFourTerminalStates/Stopped` 红（假台词模型请求数等待 60s 超时——满负载时序 flake：单测复跑 5/5 绿、独占复跑 `internal/server` 全包 ok 214s，该族有 ci-retry-isolation 既往史且分支 diff 与 `internal/server` 零交集）；其余 25 包首跑全 ok；`openspec validate --all --strict` 65/65；`internal/archcheck` ok。
- **分支 diff 核对**（merge-base `369d1c94` → HEAD，22 文件 +1073/−68）：冻结集全部在位（含 5 个既有 hud 测试机械插入）。**Ruling: `docs/feature-backlog.md` 的 1 行分支 diff 源于认领提交 `cbf60ef3` 属分支历史且内容与 main 逐字一致 — 认领提交先落 main 再分叉是本流程既定做法（B-31 同形），main 侧其后由他行推进产生 merge-base 偏移 — 无需处理，PR 合并时该行为空差异。**
