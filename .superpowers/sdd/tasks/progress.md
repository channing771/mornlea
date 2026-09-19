# SDD ledger — plan: openspec/changes/weather-camera-tree-diversity/tasks.md

- Worktree: /Users/chen/work/mornlea/.worktrees/weather-camera-tree-diversity, branch feat/weather-camera-tree-diversity, base main@76e47f56 (clean).
- Harness note: `task` tool has no model parameter; all subagents inherit the session model. Model-selection tiers from the skill are recorded as intent only.
- Spec authority: openspec/changes/weather-camera-tree-diversity/{proposal.md,specs/*/spec.md,design.md,tasks.md} (copied into worktree untracked; validation 98/98 strict).

## Pre-flight conflict scan

| Tasks sharing file/interface | One produces / other consumes | Finding |
|---|---|---|
| 1.1 ↔ 1.2 (`PlayerState` struct vs codec) | 1.1 adds weather state field; 1.2 adds v36 wire encoding | Same struct, different files (`protocol/message_player.go` vs `codec/*`); sequential dispatch, no conflict. |
| 1.2 → 1.1/1.3/2.1 (protocol v36) | 1.2 defines wire format + version bump | 1.1/1.3/2.1 consume it; order in plan (1.1 before 1.2) means 1.1 must not encode wire itself — struct field only. Carried in dispatches. |
| 1.2/1.3 → `packages/audit` baseline versions | Version matrix test pins protocol v35/metadata v3 | Plan does not name audit updates. Ruling A below. |
| 2.2 ↔ 3.3 (Rust mornlea_client + client ABI) | 2.2 may bump client ABI v17→v18 for weather frame inputs | 3.3 must reuse, not re-bump. Ruling B below. |
| 3.1 → 3.2/3.3 (`CameraMode` type + persistence) | 3.1 defines mode enum + storage | Later tasks consume; order respected. |
| 4.1 → 4.2/4.3 (Rust worldgen rules) | 4.1 implements; 4.2 parity+golden; 4.3 radius/upper-bound | 4.3 may touch 4.1 code (neighborhood radius); sequential with review, acceptable. |
| 4.x → worldgen golden files | 4.2 rewrites goldens | Only new-chunk output may differ; old-chunk tests must stay green. In brief. |
| 5.1 ← all (gates + version matrix docs) | Closeout runs full gates, updates AGENTS.md matrix + openspec/config.yaml matrix | Ruling C below. |

- Each task's tests-vs-code: 1.1/1.2/1.3/2.1/3.x/4.x each pair failing-test with implementation in the same task group — self-consistent.
- Ruling A: `packages/audit` baseline-version updates ride with the version-bumping task (1.2 protocol v35→v36 incl. `protocol` version constant; 1.3 metadata v3→v4); implementer must run `go test ./packages/audit -count=1`.
- Ruling B: any client ABI bump rides with 2.2 (header + Go bridge + versions + cross-language tests as one atomic set); 3.2/3.3 reuse the resulting ABI without bumping.
- Ruling C: root AGENTS.md version matrix + openspec/config.yaml matrix + `docs/notes/progress.md` updates ride with 5.1 closeout (archive step folds behavior contracts into main specs).
- Ruling D: snow needs no new authoritative state (approved design): precipitation form derives locally from height vs existing snow-line constant; reviewers must flag any new wire/metadata field for snow as a defect.
- Ruling: 接受 1.1 的骰子权重填补（到期晴:雨=3:1、雨→雷暴 1/8、雷暴时长复用雨段区间，均为命名常量+分布测试钉住）—— spec 只写“固定分布/小概率”未给数值，选 MC 量级且可调；若错，代价是改三处常量+重跑分布测试。
- Task 1.1: complete (commits 76e47f5..4901f19, review clean; 2 deferred minors: splitmix64 第三份拷贝、到期路径零分配未直接钉住）
- Note: 1.2 已同步更新 AGENTS.md 与 openspec/config.yaml 版本矩阵（audit 门禁所必需）；5.1 收尾只核验不再重改。`docs/notes/compatibility.md` 的 v34 滞后为存量问题，不归本 change（最小 diff 未动）。
- Task 1.2: complete (commits 4901f19..63d0d1c, review clean)
- Ruling: 接受“剩余时长默认值 = 解码零值经 RestoreWeather 转首段掷骰时长”语义（spec“剩余时长取默认值”未定常量还是掷骰；掷骰与新世界初始一致且已测试钉住）。41B 载荷（u8 kind + u32 remaining）按 design D1 接受。1.2 遗留的 3 处协议 pin 在 1.3 就地接受（测试必需、无行为变化）。
- Task 1.3: fix round 1/5 (1 addressed, 0 open; commits b13b7bc..f294c09)
- Task 1.3: complete (commits 63d0d1c..f294c09, review clean after 1 fix round; deferred minors: 原子读写说明、main_test 注释、Thunder/7776 脆弱性说明）
- Ruling: 2.1 stale-not-ready 路径安全——已亲自核验 `predictor_reconcile.go:20` tick 门在 `!Ready` 分支（:28）之前，陈旧 teardown 不可能清除已确认天气；reviewer 的 Important 项实为误报，无需 fix 轮。Predictor 落位正确（hunger 标量模式，brief 引用的 Mirror 表述不准确但实现符合架构）。Transport 一致性由 1.2 的 codec/tcp 测试覆盖，不重复验证。
- Task 2.1: complete (commits f294c09..d34131d, review clean; deferred minors: 范围校验复用、clearForNotReady 不对称存量、2.2 app 侧查询路径）
- Ruling: 2.2 的 3 个 Important 全部走 fix 轮修复（非打回）；2.1 遗留的 audit DeepEqual 注释问题路由到 2.2 fix 轮一并修复（测试注释-only，零行为变化），audit 已亲验全绿。2.2 reviewer 的⚠️(3) HUD 与⚠️(4) sky_data 零初值已亲验成立，无需改动。主规格 v17 钉转 5.1 同步。
- Task 2.2: fix round 1/5 (4 addressed, 0 open; commits 378a086..5f70fc9)
- Task 2.2: complete (commits d34131d..5f70fc9, review clean after 1 fix round)
- 天气 A 组（1.1/1.2/1.3/2.1/2.2）全部完成。
- Ruling: 3.1 存储/归属/时机三问裁决 —— (a) `shared/config` 新增顶层字段 + 独立 patch 函数（复用原子 rename 机制，不碰 `SettingsPatch` 设置页三字段契约；验证命令扩展跑 `shared/config` 包）；类型放 `client` 包（3.2/3.3 渲染消费，避免搬家），值与 F5 接线归 app；只在世界 teardown 路径持久化（退回主菜单/窗口 Close/会话关闭的公共收口），不做每次按键落盘。若错，代价是换存储函数或搬类型（小范围）。
- Ruling: 3.1 review 的 Critical 实为误报——app 全包 `//go:build darwin`（AGENTS 明文惯例“文件迁移时逐字保留 build tag”），调用点同包同门，linux 从不编译该包；亲验 `head` + AGENTS，无需 fix。
- Ruling: 3.1 Important#1（跨世界内存）闭环已亲验——同一 Application 实例跨菜单/世界复用（`startWorld` 菜单相位后装配同一实例），`resetSessionOwnedState` 不碰 mode，进程启动快照=上次 teardown 落盘值；无需 fix。
- Ruling: 3.1 Important#2（关闭链）闭环已亲验——`Close()` 首行即 `CloseClientSession`（`closeOnce`），暂停退回主菜单走独立路径各调一次（`clientCloseOnce` 幂等）；无需 fix。
- Ruling: 3.1 Important#3（panelVisible）亲验为纯调试面板（`a.panel != nil && visible`），背包不在其中，“背包不抑制”注释属实；spec 未要求背包抑制，接受现状。benchmark/capture 强制第一人称接受为自动化确定性豁免。
- Task 3.1: complete (commits 5f70fc9..967e9e9, review Approved-subject-to-controller-checks, all checks closed by controller; deferred minors: Next 越界回绕、魔法数、capture 豁免说明）
- Ruling: 3.2 开包/菜单隐藏自身接受 spec 字面“同隐同现”（纸娃娃属前端领地，不在本 change）；3.3 前眼位糊头为预期分工；正面帧级测试间接覆盖记为 deferred minor，终审 triage。
- Task 3.2: complete (commits 967e9e9..25d1da6, review clean)
- Ruling: 3.3 全塌缩保留后视朝向接受为产品意图（spec 只定位置）；正面帧级间接覆盖沿用 3.2 的 deferred 记录。
- Task 3.3: fix round 1/5 (3 addressed, 1 open) + fix round 2/5 (1 addressed, 0 open; commits f51f760..94bc6d8)
- Task 3.3: complete (commits 25d1da6..94bc6d8, review clean after 2 fix rounds)
- 视角 B 组（3.1/3.2/3.3）全部完成。
- Task 4.1: fix round 1/5 (3 addressed, 0 open; commits 61664ba..f08790b)
- Task 4.1: complete (commits 94bc6d8..f08790b, review clean after 1 fix round)
- Ruling: 4.2 reviewer 的两项 Important 转 5.1 收尾项——(a) `generator_test.go:68-73` 过强注释弱化为“生成器无跨块状态；旧字节重载不变由存储层测试锁定”（5.1 改，注释-only）；(b) 体素归因由 golden SHA + 冻结块 + 5.1 全量门禁承接，不再要求导出脚本入库。存储层亲验 `go test ./storage -count=1` 全绿。
- Task 4.2: complete (commits f08790b..ddcc41b, review clean)
- Ruling: 4.3 树干拒绝分支接受包络锁定（自然地形不可达，硬造需改生产可测性，违默认路径）；3 个 Minor（守卫计数器、口径文案、注释收敛）转 5.1 一并改（测试-only 一行级）。
- Task 4.3: complete (commits ddcc41b..18d2a19, review clean)
- 树木 C 组（4.1/4.2/4.3）全部完成。
- Task 5.1: complete (commits 18d2a19..b53a4659, review clean; flake TestCompanionManagerPlaceAtomicConsumeAndDepletion noted pre-existing in untouched code → backlog candidate)
- 全部分支任务完成，进入终审。
- Final whole-branch review: Merge GO (no Critical/Important; all deferred items ACCEPT with reasoning). Branch feat/weather-camera-tree-diversity ready for PR on user request (no push without explicit consent).
