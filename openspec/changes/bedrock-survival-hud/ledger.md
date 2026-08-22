# bedrock-survival-hud 执行 ledger

本文件记录 subagent-driven-development 的实现、双评审、修复循环与 controller 裁决。只有实际发生且有证据的结果才能回填；`待执行` 不代表通过。每个执行 Task 最多 5 轮修复/复审，超限后由 controller 对每条 finding 单独裁决。

| Task | Implementer | Implementer commit | Spec review | Quality review | 修复轮次 | 验证证据 | Ruling |
|---|---|---|---|---|---:|---|---|
| 1 OpenSpec change | `/root/hud_task1_openspec` | `b67f38e`（候选）/ `978c8f3`（round 1 fix）/ `5c9ce64`（round 2 fix） | `/root/hud_task1_review`：初审 SPEC FAIL；round 1 复审 SPEC PASS；round 2 复审 SPEC PASS | `/root/hud_task1_review`：初审 QUALITY FAIL；round 1 复审 QUALITY FAIL；round 2 复审 QUALITY PASS | 2 | 三次提交前均为 OpenSpec 56/0、diff check EXIT 0；最终 review package SHA-256 `52eb2bc47378ed10f297d85a73cc50c06e58cb0ef31372a1bdc0afe675d253b2`，strict 56/0、范围 diff check EXIT 0 | 通过 |
| 2 atlas icons | `/root/hud_task2_atlas` | `eb440ed`（候选）/ `1b94ee3`（round 1 fix）/ `8d3b4d7`（round 2 ledger-only fix；`BASE` 为 `6886679c44eeb548347bac77aa10bdbaed749ada`） | `/root/hud_task2_review`：首轮 SPEC FAIL（3 个 P2）；round 1 复审 SPEC FAIL（仅 package SHA-256 未闭合）；round 2 复审 SPEC PASS | `/root/hud_task2_review`：首轮 QUALITY FAIL（3 个 P2）；round 1 复审 QUALITY FAIL（仅 package SHA-256 未闭合）；round 2 复审 QUALITY PASS | 2 | `make rust` EXIT 0；红测按预期失败；HUD race、archcheck、gofmt、diff check 均 EXIT 0。round 1 覆盖全枚举 placement 与字面列顺序；round 2 ledger-only 后官方 package 覆盖完整 `BASE..HEAD`，原始 diff SHA-256 `4ad046ef0d15285b9f015dd13e462769e39158b0e63a3790f7979babc4ccb6c9`，复审重新计算一致 | 通过 |
| 3 hotbar/mining | `/root/hud_task3_hotbar` | `4aabc6a`（候选）/ `3e047b5`（打开态修复）/ `ad2c6bb`（round 1 补测）/ `60ddd5b`（round 1 验证）/ `29f4e58`（见证收紧） | `/root/hud_task3_review`：首轮 SPEC FAIL（1 个 P2）；round 1 SPEC PASS | `/root/hud_task3_review`：首轮 QUALITY FAIL（1 个 P2）；round 1 QUALITY PASS | 1 | `make rust` EXIT 0；快捷栏、采掘与 BASE archive renderer 红测均按预期失败；renderer focused、HUD race、archcheck、gofmt、diff check 均 EXIT 0；package raw diff SHA-256 `cfe0d5cd946d66bc8b343f5f50a5b0ba1fd059311af0577a9ccbdfd46f7595bc` | 通过 |
| 4 status/layout/capacity | `/root/hud_task4_status` | `92f0c64`（候选）/ `5e7f970`（round 1 docs fix）/ `fix: preserve survival HUD capacity`（round 1 code，本提交 hash 待 controller 回填） | `/root/hud_task4_review`：首轮 SPEC FAIL（P1/P2）；round 1 复审待执行 | `/root/hud_task4_review`：首轮 QUALITY FAIL（P1/P2）；round 1 复审待执行 | 1 | 首轮 package raw SHA-256 `6e1fb162177cfdceb9524ea2c595aabc68b4602e3218cdf308dc118bbf3e3940`；round 1 resolved-slot/极窄窗口 RED 后 focused GREEN；`make rust`、HUD race、archcheck、benchmark scenario/layout focused、OpenSpec strict 56/0、gofmt、diff check、`encode.go` 不变均 EXIT 0 | `/root`：保持 scenario v18，以视觉等价 resolved-slot 恢复固定容量并修复 1px floor；待复审 |
| 5 capture/golden | 待派发 | 待提交 | 待独立评审 | 待独立评审 | 0 | 待执行 | 待 controller 裁决 |
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
| 4 | 1 | `/root/hud_task4_review` | P1：候选把固定 HUD 布局移到 265/13056/46656，却仍标记 benchmark scenario v18，违反主规格锁定的 247/12288/45888 | `5e7f970` 先把 active change 改为 resolved-slot 与 v18 固定契约；round 1 code 每个生命/氧气槽只发一个实例，合法最大 76/245，固定布局恢复 247/12288/45888 | 待 round 1 复审 | `/root`：不升 v19、不新增 benchmark delta；用视觉等价 resolved-slot 保持 v18 |
| 4 | 1 | `/root/hud_task4_review` | P2：打开态分隔线 `max(scale*2,1)` 在极窄正尺寸 framebuffer 越过下沿 | round 1 code 删除 1px floor；新增 17×800、800×17、16×16、1×1 的打开/关闭全部 quad/glyph finite 且界内回归 | 待 round 1 复审 | `/root`：最小修复并覆盖更小正尺寸 |

## 人工视觉验收

| 执行人 | 候选目录 | 接受的 golden | 拒绝项与根因 | 结论 |
|---|---|---|---|---|
| 待执行 | 待记录 | 待逐图列出 | 待记录 | 待记录 |

## 整分支终审

| Reviewer | 基线与 head | 规格结论 | 质量结论 | 验证证据 | 最终 ruling |
|---|---|---|---|---|---|
| 待派发 | 待记录 | 待记录 | 待记录 | 待记录 | 待 controller 裁决 |
