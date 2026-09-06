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

## 2026-09-06 Task 2（CLI flag、接线与 Makefile 透传）

- 实现：提交 `3ca4ca3c`（options.go 两 flag 与 `parseCaptureScenes`、main.go 两调用点组装 `RunOptions`、run_test.go 接线 DeepEqual 断言、Makefile SCENES/GIFS 透传；5 文件 +195/-4）。
- 评审：全新评审者裁决 **ACCEPT**，无必须修复项（拒绝条款、`--capture-scenes ""` 等价全量与「空项拒绝」不冲突的裁定、Makefile 七种 `make -n` 组合核对均过）。
- Ruling: `--capture-scenes ""`（显式空值）按「未请求子集」处理、等价全量 — 与 `--capture ""` 既有惯例一致；spec 拒绝的是清单内部空项（`"a,,b"` 类），纯空白值仍被切分后以空项拒绝 — 无。
- Ruling: `$(if $(GIFS),...)` 沿用仓库 `$(if)` 惯用法（任何非空值即开启，`GIFS=0` 不算关闭）— 与 `RACE_BASE` 等既有变量同一语义，注释限定 `GIFS=1` — 无。
- 文档跟进（并入 Task 3）：`make help` 文案补 `SCENES=`/`GIFS=1`；`packages/client/cmd/mornlea/AGENTS.md` Entry Modes 表补新 flag 措辞。
- 验证证据（SHA `3ca4ca3c`，实现者与评审者各自真实执行）：
  - `go test ./packages/client/cmd/mornlea -run 'ParseCapture|CaptureScenes|CaptureGifs|RunCapture' -count=1` → ok（新增 8 测试/子用例全 PASS）
  - `go test ./packages/client/cmd/mornlea -run 'TestRun' -count=1` → ok
  - `make -n visual-check SCENES=mining-crack-early,mining-crack-heavy GIFS=1` / `make -n visual-update SCENES=main-menu` / 缺省四种组合 → 展开正确，缺省与改造前 shell 等价
  - `go build ./packages/client/cmd/mornlea`、gofmt 无输出

## 2026-09-06 Task 3（文档与路由说明）

- 实现：提交 `cefd9f38`（docs/notes/visual-verification.md 新增「子集运行与分层纪律」、SKILL.md 路由与 GIF 时机、golden README 陈旧 GIF 陈述同步、make help 文案、局部 AGENTS.md Entry Modes 表）；评审修正提交 `416647fd`（子集更新仍先执行 LOD 近环 control 的一句说明；SKILL.md「逐帧比对、全帧通过」旧路由行改为与"GIF 不进自动比对"现状一致）。
- 评审：全新评审者裁决 **ACCEPT**，两条观察以 `416647fd` 收口（见上）。
- 验证证据（SHA `cefd9f38`/`416647fd`，实现者与评审者各自真实执行）：
  - `make help` 正常渲染；`make -n visual-check GIFS=1 SCENES=terrain-noon` 等抽查展开正确
  - `grep` 核对四处文档 flag/变量拼写与 options.go/Makefile 一致
  - `go test ./packages/audit -count=1` → ok（文档守卫）

## 2026-09-06 Task 4（真机等价性与性能验证，Apple Silicon / Metal）

- 前置：`make rust` 重建并部署双 dylib（签名替换正常）。
- 等价性（同一工作区、同一 golden、同一双阈值，全部 EXIT=0 且差异像素 0/230400、最大通道差 0）：
  - 全量 `make visual-check`：24/24 景全绿，**60.3s**（user 285.6s / cpu 504%），GIF 跳过提示正常打印。
  - `SCENES=mining-crack-early,mining-crack-heavy`：**48.3s**。
  - `SCENES=main-menu`：**52.2s**（菜单相位含全景管线额外装配等待，符合预期）。
  - `SCENES=water-underwater`：**48.9s**（唯一末景子集通过，跨场景状态残留未影响等价性）。
  - `GIFS=1 SCENES=mining-crack-early`：**53.5s**，graze/lure/kill/beef-drop 四条 GIF 生成成功写入 build/visual（"不进比对"提示正常）。
- Ruling: 跨场景状态残留风险实测未成立 — 首景/中段/菜单相位/唯一末景的子集运行与全量运行对同一 golden 全部 0 差异像素，warmup+收敛判据+场景 reset 纪律足以保证子集等价 — 无需子集不安全名单。
- 性能结论（如实记录）：固定下限为世界加载+渲染器初始化 ≈ 47-48s（约 240-270s user CPU、~500% 并行），每景边际 ≈ 0.5s，GIF 四条 ≈ 5.7s。改动前全量 check ≈ 66s；改动后全量 60.3s（省 GIF），子集 48-53s（较改动前省约 20-28%）。世界加载下限由规格钉死（抓帧视距必须与真实客户端一致），攻击该下限属另一独立 change（如持久化世界快照复用），不在本 change 范围。
- 验证证据（SHA `43fd840a` 工作区，控制会话真实执行，日志 /tmp/visual-*.log）。
