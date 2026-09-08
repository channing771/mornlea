# 主手与持物验证记录

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
