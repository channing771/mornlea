# selective-visual-capture ledger

基线 SHA：`76e47f56`（main），分支 `feat/selective-visual-capture`。

## 2026-09-06 立项

- Ruling: 子集选择采用手动 `--capture-scenes`/`SCENES=`，不做 git diff 自动场景映射 — 自动映射需维护"代码→场景"映射表，映射错误会静默漏检；用户裁决本期先手动选择 — 无。
- Ruling: 子集更新基线仍执行 LOD on/off 近环 control — control 是防全局回归被固化进基线的守卫，用户裁决不弱化；子集只省未选场景的渲染时间 — 无。
- Ruling: 不更换实现语言 — 比对为毫秒级像素循环、渲染已在 Rust/wgpu，瓶颈（GPU 帧数、世界加载、GIF 浪费）与语言无关，重写零收益 — 无。
- Ruling: check 模式默认跳过 GIF 生成，新增 `--capture-gifs` 显式开启 — GIF 比对已退役、每次 check 无条件重生成约 156 帧纯属浪费；update 模式生成行为不变 — 无。
- Ruling: 场景名校验放 parse 层并在 capture 层防御性重复 — 对齐 `--motion-scene` 拒未知值的既有风格，capture 层兜底供直接调用方 — 无。

## 2026-09-06 Task 1（capture 层：场景子集与 GIF 门控）

- 实现：提交 `e783eaf7`（capture.go 新增 `RunOptions`/`gifsEnabled`/`SceneNames`/`ValidateSceneSelection`/内部 `selectScenes`，`RunCapture` 签名改造与 GIF 门控；新增 capture_scene_selection_test.go 8 测试；main.go 适配器与 run_test.go 三处 fake 签名同步）。
- 评审：全新评审者裁决 **ACCEPT**，无必须修复项（缺省 update 路径逐行 diff 等价、场景表两提交间逐行一致、Task 2 范围未提前实现）。
- Ruling: 评审观察"场景表实际 23 景"判定为误报 — `TestCaptureSceneOrderAndAICompanionDeterminism` 硬断言 `len(captureScenes)==24` 且通过，golden 目录恰 24 张 PNG，spec 亦为 24 — 评审者手工清点误差，无需改动。
- Ruling: `selectScenes` 缺省分支返回场景表本体切片而非副本 — 唯一调用方 `RunCapture` 只读遍历，无变异风险；未来出现可变调用方时再改拷贝 — 无。
- Ruling: 跳过 GIF 的提示文案提及 `--capture-gifs`（Task 2 才落地）— 同一 change 连续交付，Task 2 紧随其后，可接受 — 无。
- 验证证据（SHA `e783eaf7`，实现者与评审者各自真实执行）：
  - `go test ./packages/client/cmd/mornlea/capture -run 'SelectScenes|SceneNames|GifsEnabled|SceneSelection' -count=1` → 8/8 PASS
  - `go test ./packages/client/cmd/mornlea/capture -run 'TestCaptureSceneOrder' -count=1` → PASS（控制会话复核）
  - `go test ./packages/client/cmd/mornlea -run 'TestRun' -count=1` → 17/17 PASS；`-run 'TestTextureGoldenUpdate'` → 2/2 PASS
  - `go build ./packages/client/cmd/mornlea`、gofmt/vet（两包）无输出
  - 环境注记：本机 Rust dylib 陈旧曾致 8 个 GPU 测试报 ABI 不匹配，`make rust` 重建后 capture 包全量通过。
