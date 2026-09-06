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
