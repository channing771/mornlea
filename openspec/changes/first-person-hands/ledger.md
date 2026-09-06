# first-person-hands Ledger

基线 SHA：`da148639`（分支 `feat/first-person-hands`，干净起步；worktree `.worktrees/first-person-hands`）。

## 内容确认（brainstorming architectural 路径）

- 分类：architectural（新渲染子系统 + client ABI 升版），已按重路径走。
- 用户澄清结论：双手同时呈现（左手空手占位 + 编码保留副手字段，不加协议/存档字段）；动作按工具微调（六档参数表，斧铲取镐默认占位，B-36 落地后只改表）。
- 用户已显式批准短设计（2026-09-06），批准结论已折入 proposal/design/brief。
- Ruling: 不在 `docs/feature-backlog.md` 自建新行 — 新行注册留给 planner（backlog 是 planner 的调度面，控制会话不抢）；独占文件集已在 design.md 声明。

## Change 产物

- `proposal.md` / `specs/first-person-viewmodel/spec.md`（ADDED×6）/ `specs/rust-client-render-cutover/spec.md`（ADDED×1）/ `design.md` / `tasks.md`（5 组）已建，待 `openspec validate --all --strict --no-interactive`。

## Task 1：Go viewmodel 编码（子代理开发轮 + 审查轮）

- Red→Green：`4f4ca8dc`，`go test ./packages/client/render -race -count=1` 全绿。
- 任务评审 Spec ❌：I-1（tick 回退不清 `lastAttackTick` 致新会话首挥丢失）/ I-2（确认切换无显式测试）/ I-3（缺 `EncodeRenderFrame` 字节一致回归）+ M-1..M-4。
- 修复轮 1/5：`10901e37` 全闭合，scoped re-review 全 ADDRESSED，无新 breakage。
- Task 1: complete (commits 4a5adffd..10901e37, review clean)

## Task 2：跨语言 client ABI v17 同步（子代理开发轮 + 审查轮）

- Green：`6c88a315`，header/Rust/Go 三端 17 + TLV tag 11 + 解码 + 三端一致/v16 拒绝测试；client race、`cargo test -p mornlea_client`（viewmodel 6/6，全 crate 197/197）、render 全绿。
- 任务评审 Spec ✅；Quality 有条件 ✅：Important-1（Task 1 遗留两处注释反引号致 audit 红）+ Minor-1（ffi 白名单注释过期）。
- Ruling: audit 全绿是硬门禁，纯注释修复零语义变化，不违反文件所有权——允许 fix loop 修 Task 1 注释行。
- 修复轮 1/5：`d60ffa6d`，re-review 全 ADDRESSED；audit/render/client/cargo 全绿。
- Task 2: complete (commits 10901e37..d60ffa6d, review clean)

## Task 3：Rust viewmodel 绘制 pass（子代理开发轮 + 审查轮）

- Green：`f6aeb238`，相机空间叠加（裂纹后/名牌前）、空段不变、无推测、`#[cfg(test)]` 两组；cargo 204/204，fmt+clippy 绿。
- 任务评审 Spec ❌ 仅 F1：超限走整帧 `Invalid`，与 brief“整段丢弃”字面不一致，但与 avatar/drop/轮廓/裂纹门禁纪律一致。
- Ruling: 接受整帧拒绝——成文先例（超限帧整体拒绝）+ Go 恒 ≤4 生产不可达 + 响亮失败优于静默截断；“整段丢弃”措辞作废，已同步回 design.md §2 与 tasks.md（`b9a21e40`）。错了的代价：不可达分支的失败粒度差异，可回退改段丢弃。
- 修复轮 1/5：`de244d57`，re-review 全 ADDRESSED（F1-docs/Q2/Q3），cargo 205/205。
- Task 3: complete (commits d60ffa6d..b9a21e40, review clean)

## Task 4：app 装配（子代理开发轮 + 审查轮）

- Green：`81d56918`，确认纪律 + 相位门控 + 计数门 + frame 接线；app race（11 新测试）与 audit 全绿。
- 任务评审 Spec ❌：Critical C1（`Player` 零值致 S1 在生产落空，真身份一读可达——必须本 change 修，不接受“后续 change”）；C2 cleared（无可达路径）；C3 转任务 5（capture 跨场景边沿）；M1（96 常量重复）。
- 修复轮 1/5：`b14d6866`，re-review 全 ADDRESSED（C1/M1/C3）。
- Task 4: complete (commits b9a21e40..b14d6866, review clean)

## Task 5（第一轮）：基线门禁暴露接缝 bug，转计划缺陷修复

- 门禁（buggy SHA）：rust ✅ / gofmt ✅ / dev-check ✅ / test-race 六模块 ✅ / validate 96/0 ✅；visual-check 22/24——materials-showcase 24px 与 grass-closeup 19px 逐像素证实为世界原点固定点残片，sword-combat 配剑零差异为反向证据；visual-update 未执行（纪律正确）。
- Ruling: 接缝 bug——Go 相机空间 bake × Rust 世界 VP，design 未指定投影机制是计划缺陷。修复：Go 按本帧相机位姿烘焙世界变换，Rust 零改动；否决 P×I 独立通道；贴墙裁剪为已知限制。错了的代价：若世界烘焙在近裁剪/遮挡上不可接受，需另起 P×I change（ABI v18），回退成本为本 change 内重做。
- 已同步：delta spec（根变换 + 重放 GIVEN 含相机）/ design.md §1 / tasks.md（§5 重做 + §6.1/6.2）。C3 与 scenario v22 结论暂定，修复后重裁。
- Task 5: 重做（任务 6 之后）

## Task 6：viewmodel 世界烘焙修复（子代理开发轮 + 审查轮）

- Green：`908a82bc`，根变换由本帧相机位姿派生（`T·Ry·Rx` 经评审对 `Forward` 验算通过），落点测试红→绿；render/app/audit 绿，Rust 零改动。
- 任务评审 Spec ✅；Quality 有条件 ✅：Important-1（capture 重置调用点无直接测试）+ Minor-2（补带 yaw 落点）。
- 修复轮 1/5：`3730ab31`（test-only 双红证），re-review 全 ADDRESSED。
- 附带：bed-night 探针红为双手在屏遮挡（预期 collateral，归任务 5 重做：探针搬迁 + golden 重拍）。
- Task 6: complete (commits bfbee576..3730ab31, review clean)

## Task 5 重做：基线与收尾门禁（子代理开发轮 + 审查轮）

- Green：`331b291f`（bed-night 探针搬迁避开双手）+ `01c71a8f`（world golden 重拍）；visual-check 24/24 + 4/4 零差异；gofmt ✅；dev-check ✅；test-race 六模块 exit 0（45 ok）；validate 96/0；C3 closed（跨场景重置 + 首帧锁定）；scenario 保持 v22（workload 未变；若 benchmark 观察者持物则重裁）。
- 任务评审 Spec ✅ Quality ✅ PASS。Minor-1：terrain-noon/debug-panel 新旧 blob 一致系 intentional（两景无确认背包→无段，设计行为）。Minor-2：门禁 SHA=`01c71a8f52d245da755897ffc48ac8ee3807f0d9`（补记）。
- Task 5: complete (commits 3730ab31..01c71a8f, review clean)

## Ruling 汇总（终审复核清单）

1. backlog 不自建新行，新行注册留给 planner。
2. audit 全绿硬门禁下，纯注释修复可跨任务文件（零语义变化）。
3. 超限走 Rust 整帧 `Invalid`（avatar 纪律同形），“整段丢弃”措辞作废，已同步设计/任务。
4. 接缝 bug 走 Go 世界烘焙（Rust 零改动）；否决 P×I 独立通道；贴墙裁剪为已知限制留后续 change。
5. scenario v22 保持附条件：benchmark 观察者持物/挥动则重裁。

## 整分支终审

- 终审（ses_f8b0b73cdffeK7zHTCMjkm20PC 的后继终审会话）：CLEAN。递延项 triage：D1 接受（包边界所迫）/ D2 接受（无确认背包则无段，intentional）/ D3 接受（归因 + 更新后零差异闭环，无反证）/ D4 接受（响亮失败即正确告警行为）。
- Ruling: 保留 `.superpowers/sdd/tasks` 工作区不删——ledger 引用的全部报告证据住在该 git-ignored 目录内，删除即销毁证据链；与 SDD 默认“终审后删除”冲突处，以证据存续为准。
- 本 change 代码工作完成：6 任务组（T1/T2/T3/T4/T6/T5重做）全部一轮开发一轮审查关闭，breaker 从未触发，无 parked 项。待办（需用户授权）：推送分支 → PR（含 change 链接与验证摘要）→ CI 全绿 → 合并 → sync/archive → planner 注册 backlog 行。

## 用户评审：斜持姿态与视觉基线（同一 change 内精化，分支未合）

- 用户结论：柱状双手太抽象，要 MC 式左下/右下斜持；补静态四景 + 动作 GIF 两剧本基线。
- 已同步回 proposal/design（Pose + 基线节）/delta spec（斜向入画 Scenario）/tasks（§7.1–7.4，§5 再次重做）；validate 96/96。

## Task 7 评审：攻击基线空心（Critical A1）

- 评审 Spec ❌ Quality ❌：capture 只调 `ArmCombatMarker` 从不设 `AttackTick` 边沿，hand-attack 实为空心；生产路径不受影响。姿态数值自洽但截图轮次未入库致观感不可验证；passive-death GIF 属合法耦合转任务 5 重录；bed-night 3-skip 可接受。
- Ruling: 批准 fix round 加 capture 缝 `ObserveCombatHit`（调既有 `Observe`，`ArmCombatMarker` 先例，零生产语义变化）；V1 由控制会话亲自目检最终 PNG 后关闭。

## Ruling：任务 7 fix 轮并入任务 8 合并评审

- 用户二次评审（静态禁手/HUD 一体/passive-death 并 motion）到达时 Task 7 fix（`1ce0d4f2` 攻击缝）尚未 re-review；其 diff 完整落入任务 8 合并评审区间，任务 7 findings 转输入清单，无覆盖损失。
- 产物同步：spec（HUD 一体 + 静态无手臂）/ proposal / design / tasks §8；validate 96/96（`2a095ffa`）。

## Task 7 修 + Task 8：合并评审关闭

- Task 7 fix（`1ce0d4f2`）：capture 战斗缝 + GIF 真实挥动；Task 8（`c1e042e8` + `943413b0`）：HUD 门、静态抑制、四景回退、passive-death 并 motion、world golden 恢复本 change 前基线。
- 合并评审 Spec ✅ Quality ✅：A1（缝为 capture-only，生产零触碰）/ V1（28° 右主导 屏角外 准星净空）/ P1（零悬空引用，world 与 `da148639` 逐字节一致）/ B1（skip 保留有据）/ H1（门信号正确，HUD 未动）/ S1（scope clean）；Minor M-a（moot）、M-b（接受）。
- Task 7: complete; Task 8: complete (commits f28ff7e..943413b0, review clean)

## Ruling：目检不通过，姿态第二轮迭代

- 控制会话目检最终 PNG/GIF：全长板条（超半屏）、工具被臂遮挡、f0/f6 挥动不可见——V1 数值验收≠视觉验收，打回。
- 追加验收（可测）：可见臂段 ≤1/3 屏高、刃像素非遮挡可断言、GIF 挥动帧手区差显著；臂尺寸不动，只调落位与持物前置；摆幅可在六档表内调并记录。

## Task 7 R2+R3：目检关闭

- R2 落位 + R3 镐头前置着色（新 implementer，1 视觉轮；根因为天空伪装非遮挡）；目检：静息镐/攻击剑均清晰，臂短斜，准星净空。
- Scoped re-review 15 项全过，无新 breakage。Task 7: complete。

## Task 5 终跑（姿态修复后最终 SHA `a75742c7`）：全绿收官

- rust ✅ / visual-check 24/24 零差异 ✅ / gofmt ✅ / dev-check ✅ / test-race 六模块 45 包 ✅ / validate 96/0 ✅ / scenario v22 保持（静态零差异 + 姿态只进 motion GIF）。
- Ruling: 零改动纯门禁轮由控制会话直接验证据（SHA 一致 + exit code 表完整），不再派子代理评审——无 diff 可审，前例（5rrr）同形。
- 本 change 代码工作全部完成：T1/T2/T3/T4/T6/T7(R3)/T8 + 三次门禁轮，breaker 从未触发，无 parked 项。待办（需用户授权）：推送 → PR → CI 全绿 → 合并 → sync/archive → planner 注册 backlog 行。
