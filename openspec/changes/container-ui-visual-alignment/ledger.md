# container-ui-visual-alignment 执行 ledger

共享基线：`cff1133f62782b04a36a0461549edb64be877de2`。本文件只记录已经发生且可复验的事实；`待派发`、`待评审` 和 `待执行` 不代表通过。每个执行 Task 的 finding 最多进入 5 轮追加修复/同一 reviewer 复审，超限后由 controller 逐项裁决。

| Task | Implementer | Candidate / fix commits | Reviewer | SPEC | QUALITY | 修复轮次 | 验证证据 | Controller ruling |
|---|---|---|---|---|---|---:|---|---|
| 1 OpenSpec change | `/root/hud_merge_impl`（本轮 fresh implementer） | `f36eb5bb3686403fa1e70cadb90688d4a27276bc`; `93a2d8d81a48777997d5ee11d3bb31e520d6fe3f`; `a87b5cc47dbf017fbe8fc4f61f40cfaf16a0c935` | `/root/container_task1_review`（`task-1-review.md`、`task-1-review-round-1.md`、`task-1-review-round-2.md`） | PASS | PASS | 2/5 已关闭 | change 创建前结构 RED；strict/diff；轮次 2 复审确认完整规划范围 | 轮次 2 无 finding，接受双 PASS |
| 2 程序化容器 atlas | `/root/container_task2_impl`（本轮 fresh implementer） | `506395c95595b7187377888d4c914fe2608fd2d9`; `07f28e079cb2c8bfc5ef6981afcbaf3dcca5c485`; `5196de7af3938fd1d1423d33ba8d2089ed019e7a` | `/root/container_task2_review`（`task-2-review.md`、`task-2-review-round-1.md`） | PASS | PASS | 1/5 已关闭 | RED、HUD focused/race、archcheck、vet、format、strict、diff check | 轮次 1 无 finding，接受双 PASS |
| 3 overlay/interaction redlines | `/root/container_task3_impl`（本轮 fresh implementer） | `79a98947b45bdb2818871ee01846a7564452ca85`; `c7598752835af837d923ecc4bfacab11759fe6e3`; `e63fa7c009d4fa0bf84a1f8560a6c4dce84bae07` | `/root/container_task3_review`（`task-3-review.md`、`task-3-review-round-1.md`） | PASS | PASS | 1/5 已关闭 | RED、HUD/cmd focused race、HUD race、archcheck、vet、format、strict、diff check 均通过 | 轮次 1 P2 已最小修复，接受双 PASS |
| 4 capture/golden | `/root/container_task4_impl`（本轮 fresh implementer） | `b98ba7145f65b82acd6ceb0e9535827e79384ded`; `efdd922e781ffd7ec5e1d8a126e880503ea570dd` | `/root/container_task4_review`（`task-4-review.md`） | PASS | PASS | 0/5 已关闭 | RED/聚焦 GREEN、Metal update/check、17 图人工审查、HUD/archcheck race、vet、strict、format、diff check 均通过 | 无 finding，接受双 PASS；完整 cmd race 由 Task 5 串行门禁补齐 |
| 5 closeout | `/root/container_task5_impl`（本轮 fresh implementer） | `2f5f28ff33c13e62a184a86359c340804d9d0be1`; 修复轮次 1 待提交 | `/root/container_task5_review`（`task-5-review.md`） | FAIL | FAIL | 1/5 复审待发生 | 修复轮次 1 的单独/combined/full race 与 scenario v19 producer/perfcheck 已通过；最终静态与 package 审计待本轮提交后完成 | 三项 P1 仅可由本轮事实回填、精确 package 与同一 reviewer 复审关闭；不得预写 PASS |

## Task 1 现状与决策记录

- `2026-08-23`：worktree `codex/container-ui-visual-alignment` 在共享基线 `cff1133f62782b04a36a0461549edb64be877de2` 上 clean；唯一 implementer 为 `/root/hud_merge_impl`，未派生子代理或 reviewer。
- 结构 RED：change 创建前，`openspec status --change container-ui-visual-alignment --json` 与 `openspec instructions apply --change container-ui-visual-alignment --json` 均 EXIT 1 并报告 change not found；`openspec validate container-ui-visual-alignment --strict --no-interactive` EXIT 1 并报告 unknown item。
- 当前行为红线由代码/测试核实：`InventorySlotAt` 为 36 格 `0..35`，`FurnaceSlotAt` 为 39 格 `0..38`，`ChestSlotAt` 为 63 格 `0..62`；UI 固定配方为 10 条；第一次点击只选来源，第二次才发送一次整堆移动且确认前不改镜像。
- 当前固定资源为 267 quad、700 glyph、13312-byte glyph offset、46912-byte 总容量、48-byte instance、256-byte 对齐；三种打开态互斥，当前合法最大为 265 quad。每 overlay 只新增一个零 glyph 标题后最大为 266，无需版本或容量迁移。
- 当前正式 capture 恰好 15 项。本 change 把新场景命名为 `chest-container`、`furnace-container` 并依次插在 `inventory-crafting` 后，最终恰好 17 项；其余 15 项相对顺序不变，末三项继续是 `water-surface-slope`、`far-horizon`、`water-underwater`，两张 diagnostic controls 不计入正式场景。
- `2026-08-23`：`openspec validate container-ui-visual-alignment --strict --no-interactive` 通过，`git diff --check` 零输出；apply instructions 确认规划产物 complete、后续实现进度 `0/23`，本 Task 不提前勾选。
- `2026-08-23`：首轮独立 review 在 `task-1-review.md` 记录 SPEC FAIL / QUALITY FAIL；报告未署 reviewer 身份，ledger 不猜测。Controller ruling 只修正场景顺序、火焰/箭头 atlas cell 与裁剪接线、20px header 几何边界和 candidate SHA，未修改产品代码或 golden。
- `2026-08-23`：修复轮次 1 的定向 strict validate 通过，全量 strict validate 为 58 passed / 0 failed，`git diff --check` 零输出；此证据不代表独立复审结论。
- `2026-08-23`：独立 reviewer `/root/container_task1_review` 在 `task-1-review-round-1.md` 确认修复轮次 1 为 Spec PASS / Quality FAIL，唯一 finding 是 ledger 遗漏 fix commit `93a2d8d81a48777997d5ee11d3bb31e520d6fe3f` 与 reviewer 身份；修复轮次 2 只补这两项已发生事实，不声称尚未发生的复审 PASS。
- `2026-08-23`：同一 reviewer 在 `task-1-review-round-2.md` 复审 `cff1133f62782b04a36a0461549edb64be877de2..a87b5cc47dbf017fbe8fc4f61f40cfaf16a0c935`，给出 SPEC PASS / QUALITY PASS、无 finding；验证包括 change 与全仓 strict OpenSpec、精确范围 diff check、规划文件范围与两轮 ledger 事实。

## Task 2 已发生事实

- `2026-08-23`：fresh implementer `/root/container_task2_impl` 在 Task 1 的 `a87b5cc47dbf017fbe8fc4f61f40cfaf16a0c935` 基线上开始；先运行 `make rust`，完成后不派生子代理或 reviewer。
- RED：`make rust && go test ./internal/render/hud -run 'TestHotbarAtlas.*(Container|Item)|TestContainerPixelCells' -count=1` 以 six undefined atlas cell constants 编译失败，符合新增固定列尚未实现的预期。
- GREEN：追加六个固定 cell（列 7..12）后，`hotbarBlockColumnOffset=13`；七个 survival cell 的逐像素 SHA-256、所有可放置 `core.ItemID` 的注册表顶面逐像素比较、六个 cell 的非空/互异/二值 alpha/重复构建与全部 UV 界内均由 HUD 测试覆盖。
- GREEN：背包、合成、熔炉与箱子的栏位 surface 都采同一凹槽 UV；背包栏位的真实生产入口在 `layout.go`，故该最小 root-cause 修改与 `container.go` 同时提交，未改变坐标、hit-test、item tile、数量或耐久。
- 验证：`go test ./internal/render/hud -run 'TestHotbarAtlas|TestContainer' -race -count=1`、`go test ./internal/render/hud -race -count=1`、`go test ./internal/archcheck -count=1`、`go vet ./internal/render/hud`、`gofmt -l`（目标文件）与 `git diff --check` 均通过。
- 候选提交：`506395c95595b7187377888d4c914fe2608fd2d9`（`feat: add original container pixel primitives`）。Task reviewer 尚未派发；不得将本段视为 SPEC/QUALITY PASS。
- `2026-08-23`：独立 reviewer `/root/container_task2_review` 在 `task-2-review.md` 对 `a87b5cc..fc4ea55` 给出 SPEC PASS / QUALITY FAIL；唯一 P2 是 `atlas.go` 的列说明遗漏六个 container cell。修复轮次 1 只改该注释，复审结论尚未发生。
- 修复提交：`07f28e079cb2c8bfc5ef6981afcbaf3dcca5c485`（`docs: correct container atlas column comment`）；`gofmt`、brief HUD focused、strict OpenSpec 与 `git diff --check` 均通过。
- `2026-08-23`：同一 reviewer 在 `task-2-review-round-1.md` 复审 `a87b5cc47dbf017fbe8fc4f61f40cfaf16a0c935..5196de7af3938fd1d1423d33ba8d2089ed019e7a`，给出 SPEC PASS / QUALITY PASS、无 finding；验证包括 `make rust`、HUD focused/race、archcheck、vet、format、strict 与 diff check，review package SHA-256 为 `4ad965d1c1da79f470f21c67ba95f7fe2ae889dc1c14378905740fc406ec72e2`。

## Task 3 已发生事实

- `2026-08-23`：fresh implementer `/root/container_task3_impl` 在 Task 2 修复提交基线上开始，先运行 `make rust`；未派生子代理或 reviewer。
- RED：`go test ./internal/render/hud -run 'Test(ContainerTitles|ContainerSlotGeometry|Furnace.*Composition|RecipeButton)' -count=1` 因三个共享 header 常量尚不存在而编译失败。
- GREEN：三种互斥 overlay 各追加一个 atlas 标题 quad；panel 只向上扩 20px，36/39/63 格及十条配方的 origin/hit-test 保持原路径；熔炉两项填充复用既有 bar quad，并分别按火焰自下向上、箭头自左向右同步裁剪实例与 UV。
- 验证：同一 focused 命令、`go test ./internal/render/hud -race -count=1`、`go test ./internal/archcheck -count=1`、`go vet ./internal/render/hud`、目标 `gofmt -l` 与 `git diff --check` 均通过。三次 `cmd/mornlea` race 被工具 30 秒回合限制留下重复运行，已精确停止；它们没有通过结论。
- 候选提交：`79a98947b45bdb2818871ee01846a7564452ca85`（`feat: align container UI presentation`）。未进行 reviewer 双裁决，不得视为 SPEC/QUALITY PASS。
- `2026-08-23`：独立审查 `task-3-review.md` 对候选范围给出 SPEC PASS / QUALITY FAIL；唯一 P2 指出三尺寸集缺少 panel 四边与 `InventorySlotAt`/`FurnaceSlotAt`/`ChestSlotAt` 半开命中证明。
- 修复轮次 1：只扩展 `layout_test.go` 的既有表驱动测试。三种 framebuffer 直接读取三类 overlay panel，锁定旧 X/Width/底边不变及仅向上扩 `containerHeaderHeight*scale`；同一尺寸集穷举 36/39/63 格的左上闭、中心与右/下开命中。未改生产代码，复审结论尚未发生。
- 修复轮次 1 验证：focused HUD、HUD race、archcheck、目标 `gofmt -l`、`git diff --check` 与 strict OpenSpec 均通过。
- 修复提交：`c7598752835af837d923ecc4bfacab11759fe6e3`（`test: lock container header boundaries`）。
- `2026-08-23`：独立 reviewer `/root/container_task3_review` 在 `task-3-review-round-1.md` 复审 `5196de7af3938fd1d1423d33ba8d2089ed019e7a..e63fa7c009d4fa0bf84a1f8560a6c4dce84bae07`，确认轮次 1 SPEC PASS / QUALITY PASS、无 finding；验证包含 `make rust`、HUD focused/race、三类两次点击 cmd race、archcheck、vet、format、strict 与 diff check。

## Task 4 已发生事实

- `2026-08-23`：fresh implementer `/root/container_task4_impl` 在现有 Task 3 修复基线上开始；未派生子代理或 reviewer，只修改本 worktree 的 `cmd/mornlea` capture/测试/golden。
- RED：`go test ./cmd/mornlea -run 'Test(CaptureSceneOrder|ContainerCapture|WaterUnderwater|FarHorizon)' -count=1` 以原有 15 项场景表与缺少 `chest-container`/`furnace-container` 失败；失败准确对应新增场景尚未注册。
- GREEN：场景表在 `inventory-crafting` 后严格插入 `chest-container`、`furnace-container`，完整正式清单为 terrain-noon、hud-hotbar-health、hud-survival-feedback、avatar-nametag、inventory-crafting、chest-container、furnace-container、debug-panel、skylight-tunnel、block-light-room、materials-showcase、target-block-feedback、oak-grove、ai-companion、water-surface-slope、far-horizon、water-underwater；最后三项不变。
- GREEN：两场景均复用 `resetCapturePresentation`，显式重置远端实体/面板/互斥容器并钉住 6000 tick、固定相机和背包；箱子装入 27 格跨三行内容且来源 36，熔炉装入 raw iron/coal/ingot、`ProgressTicks=73`、`BurnTicks=911` 且来源 37。测试锁定确认镜像、36 格背包、状态值及互斥 reset，未新增产品测试 API 或修改视觉阈值。
- Metal：`make visual-update VISUAL_OUT=build/visual-container-ui-update` 生成 17 张正式 golden（另有两张 far-horizon control，只在输出目录）；`make visual-check VISUAL_OUT=build/visual-container-ui-check` 成功，输出恰为 17 张正式场景图。
- 人工逐图审查：terrain-noon、hud-hotbar-health、hud-survival-feedback、avatar-nametag、inventory-crafting、chest-container、furnace-container、debug-panel、skylight-tunnel、block-light-room、materials-showcase、target-block-feedback、oak-grove、ai-companion、water-surface-slope、far-horizon、water-underwater 均通过；容器三图可见原创框/凹槽/来源、36/63/39 格、10 条配方及熔炉火焰/箭头，末三保持水面斜坡/远环/水下且无无关漂移；未使用 Mojang 像素，未放宽阈值。
- 验证：聚焦 GREEN、`go test ./internal/render/hud -race -count=1`、`go test ./internal/archcheck -count=1`、`go vet ./cmd/mornlea`、strict OpenSpec、`gofmt -l cmd/mornlea` 与 `git diff --check` 全通过。完整 `go test ./cmd/mornlea -race -count=1` 在与已有遗留同命令并行时超过常规时长，已只终止本 implementer 启动的精确 PID，未将其记为通过；这是待 reviewer/后续重跑的风险。
- 候选提交：`b98ba7145f65b82acd6ceb0e9535827e79384ded`（`test: lock container UI visuals`）；尚未进行独立 SPEC/QUALITY 双裁决，不得视为 PASS。
- `2026-08-23`：独立 reviewer `/root/container_task4_review` 在 `task-4-review.md` 对 `e63fa7c009d4fa0bf84a1f8560a6c4dce84bae07..efdd922e781ffd7ec5e1d8a126e880503ea570dd` 给出 SPEC PASS / QUALITY PASS、无 finding；复核 capture order/fixture、17 张正式 golden 与人工图审、阈值未动、聚焦 capture、HUD/archcheck race、vet、strict、format 与 diff check。该评审受当时已有同命令争用影响，未把另一次完整 cmd race 尝试记为通过。

## Task 5 已发生事实

- `2026-08-23`：fresh implementer `/root/container_task5_impl` 在 `efdd922e781ffd7ec5e1d8a126e880503ea570dd` 上开始；只在本 worktree 更新长期基线、tasks 与 ledger，未改产品行为或 golden，未派生子代理或 reviewer。
- 长期基线：`AGENTS.md` 与 `CLAUDE.md` 逐字节同步容器像素界面、36/39/63 栏位、20px header、266/267 固定 HUD 容量和 17 项 capture 清单；`docs/notes/progress.md` 追加相同已交付能力，协议 v24、各 schema、benchmark scenario v19、engine/client ABI 与配置格式均保持原值。
- 已通过：`make rust`（0.20s）、`make rust-check`（29.3s）、`go test ./internal/render/hud -race -count=1`（2.784s）、`go test ./internal/archcheck -count=1`（3.603s）、`go vet ./...`（2.354s）、`gofmt -l .`、`cmp -s AGENTS.md CLAUDE.md`、`openspec validate --all --strict --no-interactive`（58 passed / 0 failed）与 `git diff --check`。
- 视觉：`make visual-check VISUAL_OUT=build/visual-container-ui-final`（37.532s）逐张比对 17 项正式场景，全部最大通道差 0、差异像素 0/230400；两张 far-horizon diagnostic controls 未计入正式清单。
- `2026-08-23`：独立 reviewer `/root/container_task5_review` 在 `task-5-review.md` 对 `efdd922e781ffd7ec5e1d8a126e880503ea570dd..2f5f28ff33c13e62a184a86359c340804d9d0be1` 给出 SPEC FAIL / QUALITY FAIL：此前 SIGTERM 不是测试结果、review package payload 不等于声明范围且未记录 raw SHA-256、ledger 漏记 Task 3/4 双 PASS。该结论是修复轮次 1 的起点，不得预写本轮 PASS。
- 修复轮次 1：确认无其他 `cmd/mornlea`、visual 或 race 进程后串行执行 `go test ./cmd/mornlea -race -count=1`（exit 0，195.002s）、`go test ./internal/render/hud ./cmd/mornlea -race -count=1`（exit 0，196.448s）与 `go test ./... -race`（exit 0，211.864s）。未改生产代码、golden、超时或阈值。
- 修复轮次 1：`make build`（exit 0）后运行 scenario v19 Memory producer，报告 `/tmp/container-ui-v19.CsnTAL/memory-v19.json`（exit 0，462.50s，SHA-256 `1cc6a61843d7c81a9db2104640434e0781c771c02c46a0ef466885c3bc0ca352`），身份为 scenario 19 / memory / Apple M5 / 24GiB / macOS 26.5.1 / go1.26.0 darwin/arm64；`go run ./cmd/perfcheck` 对该报告自比较 exit 0，输出 `同场景性能记录完成`。producer 记录 flying p99 18.877ms 的既有 record-only 行，未修改阈值或退出语义。
- 范围审计（`cff1133f62782b04a36a0461549edb64be877de2..efdd922e781ffd7ec5e1d8a126e880503ea570dd`）：产品改动仅为 `internal/render/hud` 与 `cmd/mornlea` capture，另有 OpenSpec、task report 和 17 张正式 golden；无 network/sim/storage/server、协议/存档/ABI/配置/依赖或二进制 UI 源改动。完整 committed package 与 SHA-256 只能在本轮 docs 提交后作为未通过门禁的审查证据提供，不能替代失败门禁。

## 五路归档收尾

- 已验证远端事实：PR #67 合入 `c1f205f1f7d7a8cd4aa71c5bf501aa5910cd07d5`（`Merge pull request #67 from channing771/codex/container-ui-visual-alignment`）。据此回填 Task 5 的完整 diff 审计与独立整分支终审为完成；不在本 ledger 杜撰未提供的 reviewer 身份或 run 编号。
- 17 张正式容器/世界 golden 的人工图审仍以既有 Task 4/5 证据为准；没有残余人工产品验收。change 保持 active，等待控制会话归档。
