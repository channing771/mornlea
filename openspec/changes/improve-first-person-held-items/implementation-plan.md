# First-person held items Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** 优化主手构图、隐藏空闲左手、让各方块和工具在手中可辨且协调。
**Architecture:** 资产注册表缓存图标像素棱柱，Go 编码共同握持根与分面方块，Rust 沿用现有实例布局并同步有界容量。
**Tech Stack:** Go、Rust/wgpu、既有 capture。
**Spec:** openspec/changes/improve-first-person-held-items/specs/first-person-viewmodel/spec.md; design.md

## Global Constraints
- 单帧 viewmodel 实例恒 ≤257（主手与最多 256 个有厚度像素部件）。
- 96 字节布局、TLV tag、ABI v18 保持不变；仅内部预算变化。预热后编码零分配。
- 空闲左手 MUST 隐藏；主手继续同身份第三人称材质与颜色。
- 不导入外部版权素材。不改服务端、存档、权威确认或挥动触发语义。
- 图标缓存与当前注册表材质覆盖一致；方块分面材质与对应世界面一致。
- 自动验证不得启动或聚焦前台游戏窗口；不得自动覆盖视觉基线。
- 中文注释，不含任务编号；保护用户与其他任务改动。只在此 worktree 修改，不推送或合并。

### Task 1: 主手与持物呈现
**Files:** packages/client/assets/item_icons.go 及相关注册表、packages/client/render/viewmodel*.go、packages/client/cmd/mornlea/app/app_viewmodel.go 与相关装配、packages/engine/crates/mornlea_client/src/render/viewmodel.rs 及相关容量测试。读取所有祖先与局部 AGENTS.md。
**Interfaces:** Consumes 当前 Registry.ItemIconRGBA/Material 与 ViewmodelInput；Produces 同一 ViewmodelEncoder 接口或最小注册表输入扩展，新的图标缓存查询接口由实现者命名并在报告写出，Task 2 消费生产链。
- [ ] 首先针对图标 alpha/颜色/材质更新、隐藏左手、分类与分面材质、握持变换、全工具可见主体与损坏差异、257 上限及零分配写出有意义的失败测试并运行记录。
- [ ] 在 registry 刷新时预计算有厚度图标棱柱（16×16，alpha ≥ 0.5，可合并同行同色像素）；帧内只读取不可变缓存。覆盖生产与默认测试路径，材质切换不可读旧缓存。
- [ ] 构建主手共同握持根，图标优先于 placement；空闲左手不输出。工具握点按图标实际手柄位置，剑/镐/锄分别配置；其他物品也有合理托握，保留全部注册物品轮廓与颜色。手臂沿用材质/颜色，可合理调整第一人称比例与位置。
- [ ] 方块六薄面部件采样世界对应面纹理，整体转向可见顶与侧，控制缝隙与重叠。
- [ ] 同步 Go/Rust 最大257预算与超量拒绝；所有实例保持96字节编码，无 ABI 改动，无公共 avatar/drop shader行为变化。
- [ ] 更新旧双手/四实例/长条特定测试，保留权威/可重放/投影保证。默认 FOV 在16:9和4:3下测完整挥动，主要识别部位在屏内，准星不被遮住。
- [ ] 验证 `go test ./packages/client/assets ./packages/client/render ./packages/client/cmd/mornlea/app -count=1`；`cd packages/engine && CARGO_TARGET_DIR=target/cargo cargo test -p mornlea_client --locked viewmodel`；`make rust` 如 Rust有改动；`git diff --check`。将红绿证据与生产接线说明写报告。完成本任务后仅提交相关代码和测试，不提交 planning 或别人的文件。

### Task 2: 无头视觉演示与修整
**Files:** packages/client/cmd/mornlea/capture/motion*、packages/client/cmd/mornlea/capture/AGENTS.md、packages/client/cmd/mornlea/options.go 与入口路由测试及需要的相关主手调校文件，演示说明；不修改既有PNG/GIF基线。
**Interfaces:** Consumes Task1生产主手编码与资产缓存；Produces 可重复运行的无头完整物品演示及实际输出路径。
- [ ] 读取Task1报告确认新增API与限制，给capture演示路由与确定性输入先写失败测试，再复用 motion-demo 入口添加手持目录演示（短名由实现者决定）。
- [ ] 覆盖空手、草/原木/工作台/石方块、全部已注册工具及损坏状态，以及火把、门、床、桶、食物的代表外观。完整动作中含中立、峰值、恢复，无前台窗口。
- [ ] 复用 motionMaxFrames 现有180帧上限，不提高全局演示预算。可用一个中立物品目录GIF加同目录逐物品完整动作GIF，逐项捕获/编码并释放中间帧，不把所有物品长动画驻留内存。输出到本 change worktree 的临时审查物目录，提供用于逐图检查的中立与峰值联系图或帧文件，同时提供完整GIF。允许修整Task1姿态但必须记录测试和原因。
- [ ] 运行 capture 定点测试与实际演示，目检识别度、握点、裁切、遮挡、材质面；问题修到协调。将命令与可读路径写入报告。提交相关代码，不提交临时媒体和 planning。
