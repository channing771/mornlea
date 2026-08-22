# bedrock-survival-hud 执行 ledger

本文件记录 subagent-driven-development 的实现、双评审、修复循环与 controller 裁决。只有实际发生且有证据的结果才能回填；`待执行` 不代表通过。每个执行 Task 最多 5 轮修复/复审，超限后由 controller 对每条 finding 单独裁决。

| Task | Implementer | Implementer commit | Spec review | Quality review | 修复轮次 | 验证证据 | Ruling |
|---|---|---|---|---|---:|---|---|
| 1 OpenSpec change | `/root/hud_task1_openspec` | `b67f38e`（候选）/ `978c8f3`（round 1 fix）/ `5c9ce64`（round 2 fix） | `/root/hud_task1_review`：初审 SPEC FAIL；round 1 复审 SPEC PASS；round 2 复审 SPEC PASS | `/root/hud_task1_review`：初审 QUALITY FAIL；round 1 复审 QUALITY FAIL；round 2 复审 QUALITY PASS | 2 | 三次提交前均为 OpenSpec 56/0、diff check EXIT 0；最终 review package SHA-256 `52eb2bc47378ed10f297d85a73cc50c06e58cb0ef31372a1bdc0afe675d253b2`，strict 56/0、范围 diff check EXIT 0 | 通过 |
| 2 atlas icons | `/root/hud_task2_atlas` | `eb440ed`（候选）/ `1b94ee3`（round 1 fix）/ `8d3b4d7`（round 2 ledger-only fix；`BASE` 为 `6886679c44eeb548347bac77aa10bdbaed749ada`） | `/root/hud_task2_review`：首轮 SPEC FAIL（3 个 P2）；round 1 复审 SPEC FAIL（仅 package SHA-256 未闭合）；round 2 复审 SPEC PASS | `/root/hud_task2_review`：首轮 QUALITY FAIL（3 个 P2）；round 1 复审 QUALITY FAIL（仅 package SHA-256 未闭合）；round 2 复审 QUALITY PASS | 2 | `make rust` EXIT 0；红测按预期失败；HUD race、archcheck、gofmt、diff check 均 EXIT 0。round 1 覆盖全枚举 placement 与字面列顺序；round 2 ledger-only 后官方 package 覆盖完整 `BASE..HEAD`，原始 diff SHA-256 `4ad046ef0d15285b9f015dd13e462769e39158b0e63a3790f7979babc4ccb6c9`，复审重新计算一致 | 通过 |
| 3 hotbar/mining | `/root/hud_task3_hotbar` | `4aabc6a`（候选）/ `3e047b5`（打开态修复）/ `ad2c6bb`（round 1 补测）/ `60ddd5b`（round 1 验证）/ `29f4e58`（见证收紧） | `/root/hud_task3_review`：首轮 SPEC FAIL（1 个 P2）；round 1 SPEC PASS | `/root/hud_task3_review`：首轮 QUALITY FAIL（1 个 P2）；round 1 QUALITY PASS | 1 | `make rust` EXIT 0；快捷栏、采掘与 BASE archive renderer 红测均按预期失败；renderer focused、HUD race、archcheck、gofmt、diff check 均 EXIT 0；package raw diff SHA-256 `cfe0d5cd946d66bc8b343f5f50a5b0ba1fd059311af0577a9ccbdfd46f7595bc` | 通过 |
| 4 status/layout/capacity | `/root/hud_task4_status` | `92f0c64`（候选）/ `5e7f970`（round 1 docs fix）/ `a15ac66`（round 1 code fix）/ `04ecf8c`（round 2 comment fix）/ `ade4768`（round 2 verification）/ `02a7a8a`（round 3 downstream fix）/ `3991aed`（round 4 chat anchor fix）/ `fix: bound chat above survival status`（round 5 fix，hash 待回填） | `/root/hud_task4_review`：首轮 SPEC FAIL（P1/P2）；round 1/2/3 复审 SPEC PASS；round 4 复审 SPEC FAIL（新 P2）；round 5 待复审 | `/root/hud_task4_review`：首轮 QUALITY FAIL（P1/P2）；round 1 复审 QUALITY FAIL（新 P3 注释漂移）；round 2/3 复审 QUALITY PASS；round 4 复审 QUALITY FAIL（新 P2）；round 5 待复审 | 5 | 全部生命/氧气、响应式、容量与下游集成分段均真实 RED/GREEN；round 5 表驱动 RED 覆盖 7 尺寸×open/closed，实测 640×300/open `y=-24`、240×40 open/closed 越界；聊天独立有界 scale 后 HUD race、chat/capture focused、archcheck、benchmark v18、strict 56/0、gofmt/diff 均 PASS | `/root`：status 保持 survival scale，chat 以真实栈高/最宽文本取不大于 survival 的统一 scale；不删行/不隐藏状态/不改容量；3.10 仍不勾 |
| 4B chat float32 strict bounds | `/root/hud_task4b_bounds` | 待提交 | 待全新独立 reviewer | 待全新独立 reviewer | 0 | 待真实 RED/GREEN、focused/race/integration/archcheck/benchmark/OpenSpec 与 committed package | `/root`：Task 4 round 5 P2 有效；不放宽任一正尺寸 framebuffer 严格界内 MUST。该 finding 在真实栈高约束后才隔离为 float32 稳定性根因，拆为独立 Task 4B，单独最多 5 轮；3.10 在通过前保持未勾选 |
| 5 capture/golden | `/root/hud_task5_capture` | 本候选提交（SHA 见提交记录） | 待独立评审 | 待独立评审 | 0 | `make rust`、真实 RED、focused、`cmd/mornlea` race 187.189s、archcheck、gofmt、diff check、15 场景 visual-update/逐图验收、最终 visual-check 15×0 像素差均 PASS | 待 controller 裁决 |
| 6 closeout | 待派发 | 待提交 | 待独立评审 | 待独立评审 | 0 | 待执行 | 待 controller 裁决 |

## Finding 与修复记录

| Task | 轮次 | Reviewer | Finding | 修复 commit / 证据 | 复审结论 | Controller ruling |
|---|---:|---|---|---|---|---|
| 1 | 1 | `/root/hud_task1_review` | P1：MODIFIED delta 与新生命契约丢失既有“生命行无背景”语义 | round 1 在生命 Requirement/Scenario、完整 MODIFIED requirement、`hud-hotbar-health` Scenario、design 与 Task 4 红测中恢复；fix `978c8f3` | round 1 复审：ADDRESSED | `/root`：要求最小修复 |
| 1 | 1 | `/root/hud_task1_review` | P1：五个任务组把独立评审排在候选提交之前，无法生成 committed review package | round 1 改为 implementer 自证→候选提交→review package→评审→追加 fix commit/复审；fix `978c8f3` | round 1 复审：ADDRESSED | `/root`：要求 Task 6 同步修复 |
| 1 | 1 | `/root/hud_task1_review` | P2：Task 2、3、5 的 Go 提交前验证遗漏 archcheck | round 1 在 `tasks.md` 1.4、2.5、4.4 增加 `go test ./internal/archcheck -count=1`；fix `978c8f3` | round 1 复审：ADDRESSED | `/root`：要求补齐三处 |
| 1 | 1 | `/root/hud_task1_review` | P2：Task 4 响应式/命中与容量见证测试排在实现之后 | round 1 把两类失败测试分别前移到 3.5/3.6，最小实现后移到 3.7；fix `978c8f3` | round 1 复审：ADDRESSED | `/root`：要求恢复 red-green 顺序 |
| 1 | 2 | `/root/hud_task1_review` | P1：五组任务把单一 task reviewer 扩成分别的 spec reviewer 与 quality reviewer，重复占用评审席位 | round 2 把五处派发统一为一名新的独立 task reviewer 同时给出 SPEC/QUALITY 双裁决，复审由同一 scoped reviewer 双裁决；fix `5c9ce64` | round 2 复审：ADDRESSED | `/root`：只做该项最小修复 |
| 1 | 2 | `/root/hud_task1_review` | 第 2 轮复审无新 P0、P1、P2 或 P3 finding | 复审完整范围 `08932d9..5c9ce64`；review package SHA-256 `52eb2bc47378ed10f297d85a73cc50c06e58cb0ef31372a1bdc0afe675d253b2`；strict 56/0；diff check EXIT 0 | SPEC PASS / QUALITY PASS | `/root`：通过 |
| 2 | 1 | `/root/hud_task2_review` | P2：顶面逐像素测试手写 placement 子集，遗漏 `ItemLightBlock`、`ItemWheatSeeds` 与未来注册项 | `1b94ee3` 改为 `0 <= item < ItemIDMax`，仅跳过 `ItemPlacement` 为 false 的项 | round 1/2 复审：ADDRESSED | `/root`：最小覆盖修复 |
| 2 | 1 | `/root/hud_task2_review` | P2：未锁定五个 UI cell 与物品 offset 的字面列顺序 | `1b94ee3` 断言 `[6]int{0, 1, 2, 3, 4, 5}` | round 1/2 复审：ADDRESSED | `/root`：最小覆盖修复 |
| 2 | 1 | `/root/hud_task2_review` | P2：review package 未包含 committed diff、commit list 与 stat | round 1 由 `review-package` 脚本重生成完整 scratch package | round 1：部分 ADDRESSED | `/root`：要求使用官方脚本 |
| 2 | 1 | `/root/hud_task2_review` | round 1 复审：前两个 P2 ADDRESSED；package 已有 diff、commit list 与 stat，但缺少范围 SHA-256 | `8d3b4d7` 后以脚本生成新 `BASE..HEAD` package，并在包头写入原始 diff SHA-256 | round 2：ADDRESSED | `/root`：最小流程修复 |
| 2 | 2 | `/root/hud_task2_review` | round 2 复审无新 P0、P1、P2 或 P3 finding | 完整范围 `6886679..8d3b4d7` 的 package、SHA-256 与 diff check 均核对一致 | SPEC PASS / QUALITY PASS | `/root`：通过 |
| 3 | 1 | `/root/hud_task3_review` | P2：`renderer_test.go` 未进入候选范围，导致 2.1 的勾选和 renderer 层 TDD 证据不真实 | `ad2c6bb` 新增 renderer 级关闭态/采掘实例前缀见证；在 BASE archive 叠加该测试后真实失败；focused、HUD race、archcheck、gofmt、diff check 均 EXIT 0 | round 1 复审：ADDRESSED，SPEC PASS / QUALITY PASS | `/root`：通过 |
| 3 | 1 | `/root/hud_task3_review` | round 1 复审无新 P0、P1、P2 或 P3 finding | 完整范围 `218bc9c..29f4e58` package 与 raw diff SHA-256 `cfe0d5cd946d66bc8b343f5f50a5b0ba1fd059311af0577a9ccbdfd46f7595bc` 一致，diff check EXIT 0 | SPEC PASS / QUALITY PASS | `/root`：通过 |
| 4 | 1 | `/root/hud_task4_review` | P1：候选把固定 HUD 布局移到 265/13056/46656，却仍标记 benchmark scenario v18，违反主规格锁定的 247/12288/45888 | `5e7f970` 先把 active change 改为 resolved-slot 与 v18 固定契约；`a15ac66` 每个生命/氧气槽只发一个实例，合法最大 76/245，固定布局恢复 247/12288/45888 | round 1 复审 ADDRESSED；round 2 复审保持 ADDRESSED，SPEC/QUALITY PASS | `/root`：不升 v19、不新增 benchmark delta；用视觉等价 resolved single-slot 保持 v18 |
| 4 | 1 | `/root/hud_task4_review` | P2：打开态分隔线 `max(scale*2,1)` 在极窄正尺寸 framebuffer 越过下沿 | `a15ac66` 删除 1px floor；新增 17×800、800×17、16×16、1×1 的打开/关闭全部 quad/glyph finite 且界内回归 | round 1 复审 ADDRESSED；round 2 复审保持 ADDRESSED，SPEC/QUALITY PASS | `/root`：最小修复并覆盖更小正尺寸 |
| 4 | 2 | `/root/hud_task4_review` | P3：容量测试注释仍声称打开态恰好打满上限且容量可随有界实例更新，与合法最大 245、scenario v18 固定 247 矛盾 | `04ecf8c` 只把两处注释改为保留 2 quad 余量且只有显式 benchmark scenario 迁移才能改变固定布局；测试逻辑与生产代码不变 | round 2 复审 ADDRESSED，SPEC/QUALITY PASS | `/root`：最小注释修复，通过 |
| 4 | 2 | `/root/hud_task4_review` | round 2 复审无新 P0、P1、P2 或 P3 finding | 完整范围 `04e62a9..ade4768` package raw SHA-256 `7ac6b350e86f315da2ed33bd11a0eedf7e51e1ed7e7071c80c725aacbaadb6a7`；独立验证全部 PASS | SPEC PASS / QUALITY PASS | `/root`：Task 4 通过 |
| 4 | 3 | `/root/hud_task4_review` | Task 5 integration 暴露 late downstream finding：`cmd/mornlea/chat_test.go` 与 `health_hud_test.go` 仍锁定 Task 4 前 health 16 quad/oxygen 2 quad，与已评审 resolved-slot 各10 quad 契约矛盾 | `02a7a8a` 精确更新所有同类断言与说明，不改生产代码；复审在纯 `git archive HEAD` 中运行 focused 与完整 `cmd/mornlea` race，隔离 Task 5 未提交文件 | round 3 复审 ADDRESSED，SPEC/QUALITY PASS | `/root`：修正测试根因，不放宽断言，通过 |
| 4 | 3 | `/root/hud_task4_review` | round 3 复审无新 P0、P1、P2 或 P3 finding | 完整范围 `04e62a9..02a7a8a` package raw SHA-256 `6412be44786380e5cc03c27589246c7b81ee52e53e3f9915764f201e515207ae`；纯 archive 独立验证全部 PASS，Task 5 三个文件保持原状 | SPEC PASS / QUALITY PASS | `/root`：Task 4 round 3 通过 |
| 4 | 4 | `/root/hud_task5_review` | P2：`ai-companion` 的 accepted chat history `y=244..284` 与关闭态 survival status `y=268..284` 重叠，破坏聊天语义可审查性 | `3991aed` 以同帧 accepted line、open input、confirmed health/depleted oxygen 真实 RED，改为复用 `hotbarRowBounds(false)` 的 status top 向上锚定 | round 4 复审：640×360 重叠 ADDRESSED，但新 P2 使 SPEC/QUALITY FAIL | `/root`：保留 shared anchor，继续修复最大聊天栈越界 |
| 4 | 5 | `/root/hud_task4_review` | 新 P2：640×300 下 open input+六行 history 被 shared anchor 推到 `y=-24`，聊天 scale 未受真实栈高约束 | 表驱动 RED 覆盖 640×360、640×300、240×40、17×800、800×17、16×16、1×1 与 open/closed；`chat.go` 按真实行数/输入栈高和最宽可见文本计算统一有界 chat scale，float32 bound 向零保留一 ULP | round 5 待复审 | `/root`：本 Task 第 5/最后修复轮，不删行/不动 Task 5 |
| 4 | 5 | `/root/hud_task4_review` | P2：理论 fit scale 只向零留一个 float32 ULP，panel 尺寸/坐标与最多 32 glyph 的后续乘加仍可累计越界；实测 `44x40` open CJK 右沿和 `109x40` open 窄 Latin 上沿失败 | Task 4B 待以真实反例和相邻临界整数扫描 RED，再在共享聊天 scale 一处增加可证明覆盖最终乘加的固定亚像素余量 | Task 4 round 5 SPEC FAIL / QUALITY FAIL；转 Task 4B 全新 reviewer | `/root`：finding 有效且不得豁免；这是后隔离出的数值根因，不计为 Task 4 第六轮。Task 4B 独立最多 5 轮，Task 5 capture/golden 保持不变 |

## 人工视觉验收

| 执行人 | 候选目录 | 接受的 golden | 拒绝项与根因 | 结论 |
|---|---|---|---|---|
| `/root/hud_task5_capture` | `build/visual-bedrock-survival-hud-update` | `terrain-noon.png`、`hud-hotbar-health.png`、`hud-survival-feedback.png`、`avatar-nametag.png`、`inventory-crafting.png`、`debug-panel.png`、`skylight-tunnel.png`、`block-light-room.png`、`materials-showcase.png`、`target-block-feedback.png`、`oak-grove.png`、`ai-companion.png`、`water-surface-slope.png`、`far-horizon.png`、`water-underwater.png` | 无；14 张旧图的变化 bbox 均只落在 HUD 区域，新图是唯一新增 golden | 15 张逐图接受；`hud-hotbar-health` 的九格/双层选中/数量/耐久/满血/满氧隐藏、`hud-survival-feedback` 的低血/气泡/磨损工具/不可采中段缺口、`inventory-crafting` 的下方状态行均符合规格，世界/实体/光照/LOD/水域无异常 |

## 整分支终审

| Reviewer | 基线与 head | 规格结论 | 质量结论 | 验证证据 | 最终 ruling |
|---|---|---|---|---|---|
| 待派发 | 待记录 | 待记录 | 待记录 | 待记录 | 待 controller 裁决 |
