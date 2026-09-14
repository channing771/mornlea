# 主手与持物验证记录

> 最新生产版本a75ff171：在已修正的共用手臂、短手腕与镐尖动作上，调整锄头实际自由刃端的凿击落点；28044686仅补强测试。真实新版窗口已显示铁锄与完整底部HUD，用户实机试用后明确确认锄刃落点及手腕衔接协调、可以收尾。完整原生鼠标动作录制仍未取得，不将此项自动化证据记为通过。

## 最新轮次：短手腕与锄刃落点

- Task7生产提交`3c457e6f`统一工具手臂挥动路径、缩短实体腕部；测试补强`c9aff089`。独立任务审查及整分支增量审查通过；六模块race、vet和OpenSpec严格校验通过。用户随后在实机中指出锄头凿击点不协调，故未将整体视觉验收标记通过。
- Task8生产提交`a75ff171`只改变锄头工作姿态，保留共用手臂、短手腕、其他工具中立和动作路径；测试补强`28044686`。独立规格与质量审查通过，无待处理重要发现。编码的实际自由刃端在1280×720接触相位投影为完整款中心`(768,373)`、外角`(760,379)`，损坏款中心`(767,358)`、外角`(772,358)`；均位于柄套内侧且朝工作方向领先。四种锄头在`build/held-items-approved/task-8-motion/`有各60张30fps真实渲染原帧和GIF，已目检起手、接触和回收；离屏图不含原生HUD。
- `go test ./packages/client/assets ./packages/client/render ./packages/client/cmd/mornlea/app ./packages/client/cmd/mornlea/capture -race -count=1`、最终`make test-race`六模块、六模块`go vet`、`openspec validate --all --strict --no-interactive`均通过；日志分别在`build/held-items-approved/task-8-test-race.log`、`task-8-vet.log`、`task-8-openspec.log`。Task8未改Rust，沿用此前通过的Rust门禁。`make dev-check`仍有在未修改主线复现的服务端短测试失败，未绕过或扩大修复范围。
- 隔离世界的原生窗口已使用Task8二进制，`build/held-items-approved/task-8-native-hoe-hud.png`为铁锄与真实完整HUD同屏截图。复制新二进制后原app包签名无效，已对任务专用构建包重新作本机临时签名，真实窗口正常运行；这不更改入库源码或产品签名。原生短录制虽取得16张PNG，但自动点击未可靠触发游戏轮询动作，帧间工具位置未变化，不能充当点击/按住证据。用户在当前窗口手动试用第3格铁锄的单击和按住后，回复“现在协调，可以收尾”，故锄刃与手腕的实机手感已获确认。

## 镐尖方向修正与最终代码验证

- 用户指出横着挖不正确后，只修改镐起手/工作姿态：先转入凿击平面，尖端向前下击，再连续收回；其他类别动作保持。实际编码尖端在接触时领先柄套约0.263，尖端轴与接近方向点积0.826。完整、损坏石镐/铁镐均验证。
- 镐转动后露出的镐头/柄套间隙已用真实3D连接修复；柄套下端、静态外轮廓、实例数量保持。补强回归覆盖XYZ连接与完整两翼/损坏单翼的数量。
- 生产提交991a8e7d，测试补强4d8f5407。独立任务审查及整分支增量终审均APPROVED，无阻断发现；早先的6728dbe8..d4754c73整分支集成审查仍有效。所有报告和裁决在`.superpowers/sdd/approved-plan/`。
- `go test ./packages/client/assets ./packages/client/render -race -count=1`通过；最终生产冻结后`make test-race`再次通过六模块，日志`build/held-items-approved/pick-tip-final-test-race.log`；六模块`go vet`通过，日志`pick-tip-final-vet.log`。测试补强另跑实体连接定点测试通过，生产未变。Rust未改动，沿用已通过的fmt/clippy/client234/engine262证据。
- `make dev-check`的主线既有服务端short失败仍见下方历史证据，没有修改或绕过该问题；以上通过命令不表示dev-check全绿。
- 最新真实渲染器30fps镐循环：`build/held-items-approved/task-6-motion/011/pick-30fps.gif`及75张原帧；四种镐分别在010/011/012/013目录。Task5图集内的镐动作是旧方向，以Task6目录为准，其余物品未变。
- 原生程序已替换为991a8e7d并启动隔离世界。`pick-tip-native-wood-sword-1280.png`为用户操作后的木剑与完整真实HUD；不能因文件前缀误当作镐截图。短原生记录成功得到8张PNG，但首尾哈希相同、画面是静止木剑，不能证明真实点击动作；长记录曾超时。最新镐尖方向的用户反馈问题仍待回复，最终视觉验收不标记通过。

## 严格校对轮次：桌面与真实工作方向

- Task4静态修正：袖口右边缘入画点从约88%H降低至95.26%H，底边交点从93%W调整至84.78%W；拳掌缩短加厚、闭合握柄。剑尖89.17%W/20.78%H，剑身48.20%H；镐头加大并加入真实渐尖末端，缩短外露柄。Task4独立复审通过933d4107。
- Task5动作修正：实际相位驱动多轴挥拳/挥砍/下劈，连续起止与较慢回收；实体手腕与袖口、掌心在3D中相接。真实101相位×全部66项×三桌面尺寸×三FOV检查HUD/near-plane/头部/臂根；264实例、96字节、ABI19和权威输入边界保持。Task5独立代码审查通过，不能代替实机手感验收。
- 生产deddd3ae的实际1280桌面五类75帧30fps循环：`build/held-items-approved/task-5-motion/index.html`；66项198张原图：`task-5-catalogue/index.html`。这些是离屏生产渲染器输出，没有伪造HUD。
- 实机新版剑与完整HUD：`build/held-items-approved/strict-native-final-sword-1280.png`。此前名为strict-native-sword-1280的截图实际处于第三人称，已改名strict-native-third-person.png，不作为手部证据。12秒原生PNG记录请求55秒超时，未得到动作文件，不记为录制成功。
- 用户已实际测试并明确指出：镐需要以尖侧挖掘，当前横向姿态不正确。Task6只修正镐工作平面/尖端领先关系；整体视觉验收保持未完成。
- `make test-race` 在deddd3ae生产冻结后通过全部六模块，日志`strict-final-test-race.log`；期间d4754c73仅更新中立测试注释并移除单元素循环，其定点测试另行通过。六模块`go vet`通过，日志`strict-final-vet.log`。旧主线已复现的dev-check服务端short失败依然单独披露，无绕过。
- 整分支6728dbe8..d4754c73独立审查：代码集成APPROVED，无阻断正确性发现；视觉验收NOTAPPROVED，遵循用户镐方向反馈继续修正。现有Rust检查证据仍有效，Task4/5没有Rust更改。

## 历史：已审核效果图版本：2026-09-14

最终生产提交 `30c0396ae09615be5af5a400b81d0801cab91a58`，集成主线基线 `6728dbe8ff2e44670c7d9e839ae93aa1c512e5a3`。隔离目录 `.worktrees/first-person-held-items`、分支 `codex/first-person-held-items`；主工作区用户已有改动保留，未推送或合并。

### 实现与对照

用户最新明确暂时不需要适配小屏幕：停止进一步窄屏调整，当前验收集中于1280×720等常用桌面窗口与审核图的一致性。已完成的窄屏检查仅作附加历史证据，不作为本轮新增验收条件。

隐藏空闲左手；右手为短斜前臂、闭合拳掌、简单袖口。剑、镐、锄采用独立立体分件和当前图标调色板，保留木/石/铁及损坏区别；握柄为深棕分段，金属亮前面和暗侧面清晰。工具相对拳掌向右上倾斜，剑尖与渐缩剑身连续；方块沿用世界各面材质，其他物品保留完整像素轮廓。264 实例预算、96 字节实例、ABI19 与权威行为保持。小窗口收拢、抬高持物以避开实际 HUD。

本地主键动作按实际经过时长驱动，空挥立即启动、按住循环、释放后收回；菜单、聊天、行囊与鼠标重新捕获期间阻断。自动测试覆盖这些状态转换，尚未取得真实鼠标交互的完整实机动作验收。

审核图为 `build/held-items-approved/approved-concept.png`。实机和离屏证据均为生产渲染器输出，不将概念图当作实机结果，不宣称逐像素一致。正常窗口拳掌锚点约84%W/79%H；最终剑尖最高角约91.79%W/22.78%H，与概念估计89%W仍有小幅水平差异。640宽的自适应位置有意不同于正常窗口。

### 实际视觉证据

所有路径相对该隔离目录：
- `build/held-items-approved/reference-native-sword-1280.png`：最终剑尖修复后真实窗口与快捷栏、护甲、生命、饥饿条同时显示。
- `build/held-items-approved/reference-native-empty-1280.png`：空手闭合拳掌和短前臂，空闲左手隐藏。
- `build/held-items-approved/reference-native-hoe-1280.png`：锄头薄斜刃、深棕握柄、完整 HUD。
- `build/held-items-approved/reference-native-pick-640.png`：窄窗口手持镐与实际 HUD，完整头部可见，手臂根留在屏幕外，未覆盖底部栏。
- `build/held-items-approved/task-3-point-geometry.gif` 及同名 `-items/`：空手加全部65注册物品，66 GIF、330 PNG、5张总览。最终剑尖修复仅改变15张完整剑变体图片，其余315张与上一批逐字节相同。已检查中立/攻击总览和代表原图，包含草方块三面、原木、工具及头盔握持。

离屏 GIF 验证模型运动，未合成原生 WebView HUD，不能当作实机点击录制。之前实机 PNG 记录未捕获到可靠挥动；自动化键鼠未稳定触发游戏轮询输入。用户实际点击、按住、释放与界面切换的手感确认仍待完成；保持对应任务未勾选。

### 验证与审查

- 最终 assets/render 完整 race、app/capture viewmodel 定点 race 通过；覆盖全部物品、完整挥动、640×360/1280×720/800×600/480×360、FOV30/70/110、实际投影角点/凸包与 HUD 安全区、握点连接、缓存刷新、最坏256像素覆盖和零分配。
- `make rust` 已完成264容量版本构建。`make rust-check` 通过 Rust fmt、clippy `-D warnings`、client234项和engine262项测试；此后无 Rust 改动，沿用该证据。
- `go build -o build/held-items-approved/mornlea-reference ./packages/client/cmd/mornlea` 在最终生产提交成功，已用于实机程序。
- `openspec validate --all --strict --no-interactive` 在最终文档回填后再次通过113项，日志 `final-docs-openspec.log`。
- 独立任务审查与相对当前主线的整分支审查通过；唯一P3剑尖帽头已于 `30c0396a` 修复并复审关闭。报告保存在 `.superpowers/sdd/approved-plan/`。
- `make dev-check` 当前失败于服务端 `TestWarpParityMemoryVsTCP`：`unsupported passive dimension 1`。已在未修改主线 `6728dbe8` 以同一定点 `-short -count=1` 命令复现；没有修改服务端/共享域/engine 数值代码。六模块 vet 已通过，停止后未执行的客户端短测试与 Rust 门禁分别补测；绝不记为全绿。日志 `final-stable-dev-check.log`、`warp-parity-main-repro.log`。
- 最终稳定提交 `30c0396a` 的 `make test-race` 六模块全量通过，日志 `final-stable-test-race.log`；`go test ./packages/client/... ./packages/tools/... ./packages/audit/... -short` 通过，日志 `final-stable-client-short.log`。先前与实现者失败测试交叠的运行出现资产/剑尖断言失败，属于开发中间态，记录保留，不作为最终稳定提交结论。

---


> 下列为第一轮实现的历史验证，对应当时基线及 257 实例预算；不能作为当前主线集成和已审核效果图版本的验证结论。最新一轮完成后另列精确提交与实机证据。

## 结果
用户选择隐藏空闲左手。生产路径现仅呈现主手，方块对应世界六面材质，工具与其他图标物品使用当前资产注册表的有厚度像素轮廓；主手与持物共用握持根。

## 自动验证
- 基线：`make rust` 与 assets/render/app 定点测试通过。
- 实现：`go test ./packages/client/assets ./packages/client/render ./packages/client/cmd/mornlea/app -count=1` 通过；图标 alpha/颜色/缓存更新、全物品预算、握点、挥动、16:9/4:3 默认 FOV 投影与零分配有覆盖。
- Rust：`CARGO_TARGET_DIR=target/cargo cargo test -p mornlea_client --locked viewmodel` 通过 14 项，含真实离屏 GPU 最大实例/超量拒绝。
- 收尾：`make dev-check` 通过，含六模块 vet/short tests、Rust fmt/clippy，以及 client 221 项、engine 250 项测试；这覆盖 `make rust-check` 的同一组 Rust 门禁，不重复执行。
- 收尾：`make test-race` 六模块全量通过。
- OpenSpec：`openspec validate --all --strict --no-interactive` 通过 102 项。

## 实际视觉证据
命令：`go run ./packages/client/cmd/mornlea --motion-scene held-items --motion-demo build/held-items-preview/catalogue.gif`。

成功生成空手与全部 56 种注册物品的目录及逐物品完整 GIF。审查物保留在本 worktree 的 `build/held-items-preview/`，不覆盖入库基线。控制会话逐页目检全部中立、挖掘正峰、挖掘负峰、攻击峰值，并查看草方块和橡木原木的原分辨率图：顶侧面可辨、工具主体完整、握点连接、臂根持续在画面外，无可见接缝问题。57 组恢复 PNG 与中立 PNG 的 RGB 逐像素比较全部相同。

首次实拍发现正向挥动时前臂末端悬空，已在实现阶段修正，并补充跨完整角度范围的臂根裁切回归；复拍确认修复。几何投影与包围盒测试是近似检查，实际视觉目检作为补充，两者不互相替代。

预算采样遍历全部默认物品：工具 27–42 实例，默认最大物品生牛肉 98 实例/9408 字节，低于 257 的固定容量。此为载荷计数，并非帧率 benchmark。

## 已知仓库构建问题
额外运行 `make build`，客户端与服务端 Go 编译均成功，但打包步骤复制 `packages/client/assets/packs/pixel_perfection/ATTRIBUTION.md` 失败。基线 `e61e8516` 的 Makefile 已使用该旧路径，而实际入库目录为 `packs/pastelcraft`；与本次手部改动无关，未扩大修改范围或伪造署名文件。因此不能将 `make build` 记录为成功。

## 审查
Task 1：提交 `82c4fb53`，独立规格审查 PASS、代码质量 PASS，无待处理发现。Task 2：提交 `8939b627`，独立规格审查 PASS、代码质量 PASS，无待处理发现。整分支终审 PASS，无生产缺陷或待处理发现；审查提示的 delta spec 文件尾额外空行已在回填时修正，并通过最终 diff 检查。完整审查记录保留在本 worktree 的 `build/held-items-preview/review-records/`。

原工作区仍在 `e61e8516`，用户原有进度与 bucket 计划改动保留。此 change 工作在 `codex/first-person-held-items`，未推送或合并。

## 用户反馈后的第二轮验收（进行中）
第一轮离屏图未合成 HUD，不能证明无 HUD 遮挡；用户要求工具结构进一步立体、实际 HUD 共存及即时空挥。真实1280×720窗口已复现前臂压住右侧饥饿图标，证据为 `build/held-items-refinement/before-hud.png`。第二轮门禁和图像待实现及审查后补充，以上第一轮结果不作为第二轮通过声明。
