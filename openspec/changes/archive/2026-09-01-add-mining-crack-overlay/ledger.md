# add-mining-crack-overlay Ledger

基线 SHA：`d3e9fcbb`（main）。工作分支：`feat/mining-crack-overlay`
（worktree `.worktrees/mining-crack-overlay`）。

## 阶段 1：内容确认

- Ruling: 需求确认以需求方任务书为准 — 需求方在控制会话对话中给出了完整的
  目标、架构约束（真实 breakProgress 驱动 + 10 Stage + 单一可复用 Crack
  Overlay + Atlas/UV 切换 + 透明像素风）、禁止修改清单与 5 组验收 case，并
  明确指示"直接基于现有 Mornlea 项目代码完成修改"，构成显式设计批准；分类为
  `bounded`（仓库既有流程与渲染先例内的呈现层改动，无新子系统），设计结论
  全部落入本 change 的 proposal/design。— 依据：development-process 阶段 1
  "批准来源 = 用户或控制会话的显式确认"。— 无偏差。

## 任务执行

（每任务：implementer 派发、评审结论、验证证据按 SHA 记录于此）

### Task 1：程序化裂纹材质层

- 实现：commit `2c923b24`（fresh implementer）。出生阶段图实现增量生长；
  层号 68..77、cutout 分类、`crack_0..9` 绑定槽位；适配 `bed_test.go`
  （枚举末位哨兵改邻接断言，守卫语义等价）与 `pack_test.go`（固定打开顺序
  期望表追加）。
- 验证证据 @ `2c923b24`：`go test ./internal/assets -race -count=1` ok
  （66 tests）；`go test ./internal/mesh -run 'TestNativeOracleParity|
  TestPlantMaterial' -count=1` ok；`gofmt -l` 无输出；`go vet` 干净；
  `go test ./internal/archcheck -count=1` ok。
- SPEC 评审（fresh reviewer）：**pass**。层号/纹理七项/cutout/槽位/范围纪律
  全部 PASS；唯一缺口 = tasks 1.1 的"AtlasPixels 输出长度随 layerCount 走"
  无直接钉守（既有测试为双注册表相对比较）。
- QUALITY 评审（fresh reviewer）：**pass**。birth map 设计 5/5；视觉合理性
  4/5（stage 覆盖率 2%→35% 模拟确认）；两处 minor：`crack.go:46` 注释
  "约半数"与实测 25% 有效新增不符、`crack_test.go:40` 恒真断言循环。
- Ruling: R1 修复三处（AtlasPixels 位置性测试 + cutout 路径镜像断言、
  注释改门控概率描述、恒真循环改显式冻结表）— 均为评审明确的非阻塞缺口，
  修复成本低且加固 tasks 1.1 验收项 — 不构成范围漂移。
- R1 落地：commit `da5a219c`。`go test ./internal/assets -race -count=1` ok
  （68 tests，新增 `TestAtlasPixelsScalesWithLayerCount`、
  `TestCrackLayersTakeTheCutoutMipPath` 含双分支产物必须不同的夹具承重守卫
  与石头层平均路径对照）；gofmt/vet 干净。brief 中"layers×mips×1024"的字面
  公式与 mip 逐级减半的真实布局不符，implementer 按意图改为逐 mip 求和推导
  （78×1364），裁决接受。Task 1 关闭。

### Task 2：Go 侧镜像补全与实例编码

- 实现：commits `b44094ca` + `f7755fdf`（fresh implementer）。
  `hud.MiningOverlay` 补 `Target/HasTarget`；`BlockCrackStage` +
  `EncodeBlockCrackInstances`（80B = mat4 + f32 层号 + 12B 零填充，外扩
  0.001）；`frameTagCrack = 10` 条件段（空流帧逐位一致有根基测试）；
  app 侧 `deriveBlockCrack`（`app_render.go`，复用 `outline.Visible`
  继承 UI/Predictor/命中条件源）+ 相位门控 `MenuPhaseGame`。
- 偏差裁决（控制会话）：delta spec Scenario 的阶段期望数值 2/4 系控制会话
  笔误（公式 min(9,floor(p×10)) 下 9/30→3、15/30→5），已于 `371dabee`
  修正 spec——先改产物后评审，代码按公式实现正确。
- Ruling: 门控合取 `outline.Visible` 接受 — SPEC 评审裁决：spec 自身要求
  UI/菜单相位"下一次呈现消失"的本地即时门控，`outline.Visible` 是既有且
  已被测试钉死的同一本地谓词；"本地射线瞬时未命中但服务端仍在采掘"窗口被
  服务端逐 tick 重射线清空采掘限定为一 tick + RTT 自愈，无残留路径；该合取
  不制造第二进度来源（阶段/位置仍唯权威）。— 权衡替代方案（独立 UI 门控）
  需复制谓词且与选框可能出现同隐不同步。— 无错。
- 验证证据 @ `f7755fdf`：`go test ./internal/render/... ./internal/client
  ./cmd/mornlea/app -race -count=1` 全 ok；gofmt/vet 干净；
  `go test ./internal/archcheck -count=1` ok。评审者 focused 复跑全绿。
- SPEC 评审：**pass**（7/7 PASS，重点裁决见上）；QUALITY 评审：**pass**
  （全部 ≥4.5，三条非阻塞 nit：`walkFrameTLVs` 建议补 `Visible` 前置守卫、
  `ratio<0` 分支对 uint16 不可达系公式字面忠实、Go tag 10 先于 Rust 白名单
  落地为既定任务切分顺序）。nit 不触发修复循环，记录在案随 Task 4 收尾
  抽查。Task 2 关闭。

### Task 3：Rust crack pass 与帧解析

- 实现：commits `a71d28d8` + `57c29a20`（fresh implementer）。ffi tag 10
  白名单 + `FrameInput.crack_instances` + `empty_passes` 纳入；新模块
  `render/crack.rs`（带 UV 立方体、`OutlineTranslucent` 同族管线、
  `Option<BindGroup>` 惰性绑定早退、容量 1、关联函数 `instances_valid`）；
  `shaders/crack.wgsl`（`mat4x4f + vec4f` 承载层号规避 vec3f 对齐陷阱、
  `alpha < 0.5` discard 与 terrain 同阈值、daylight 经 VsOut 插值入
  fragment）；mod.rs 集成（outline→crack→名牌）；water 门禁 5→6。
- 偏差裁决（控制会话）：bind 传递采用 LodPass 先例的第三方案（pass 自持
  `Option<BindGroup>`，record 早退）— 与库内既有跳过语义同构且省一次
  attachment load — 接受。
- 验证证据 @ `57c29a20`：`make rust` 成功；`cargo test -p mornlea_client
  --locked` 141 passed（新增 7）；`cargo test -p mornlea_engine --locked`
  218 passed；`go build ./...` ok；`go test ./cmd/mornlea/app -race
  -count=1` ok（新 dylib 下 Task 2 测试仍绿）；clippy/fmt 干净。
- SPEC 评审：**pass**（8/8 PASS；无新增 extern "C"，client ABI 仍 v14；
  80B 跨语言布局逐字段核对一致；GPU 集成测试为确定性三对照设计）。
  QUALITY 评审：**pass**（5 项满分、测试 4/5；两条非阻塞：门禁测试函数名
  过时、storage visibility 过度声明）。
- Ruling: R1 修复两处（门禁函数改名
  `translucent_render_passes_stay_within_water_and_crack`、storage
  visibility 收窄 VERTEX-only）— 函数名按名索骥会得到错误契约，必修；
  visibility 收窄与先例对齐 — 均非行为变更。
- R1 落地：commit `1bd12f2b`。`cargo test -p mornlea_client --locked`
  141 passed；clippy 干净；`make rust` 重建回拷。Task 3 关闭。

### Task 4：视觉场景与收尾门禁

- 实现：commits `8402e9d1`（场景+测试+golden）+ `f6eacceb`（walkFrameTLVs
  Visible 守卫，Task 2 备案 nit 闭环）+ `7e262973`（capture 文档同步）+
  `17e0c031`（visual-verification delta）。新场景 `mining-crack-early`
  （6/30→阶段 2）与 `mining-crack-heavy`（29/30→阶段 9）复用
  target-block-feedback 固定世界，砖块 target=(0,3,-3)，HasTarget 开/关
  像素差分对照（early 600/1600px、heavy 1520/1600px，maxΔ=71）。
- 偏差裁决（控制会话）：主规格 `visual-verification` 钉死"恰好 25 场景/
  25 golden"，新增场景必须同步 MODIFIED delta——控制会话已逐字比对 delta
  与主规格 diff，确认改动恰为 25→27 计数、新场景清单/相邻不变量、两处
  过时 sword-combat 叙事中性化与一段历史 Scenario 清单对齐，无超范围
  语义漂移，接受。
- 验证证据 @ `17e0c031`：`go test ./... -race -count=1` 44 包全 ok；
  `make rust-check` exit 0；`openspec validate --all --strict` 80/80；
  `make visual-check` 27/27 场景差异 0；既有 golden 逐位未动（更新前
  25 场景 0 差异 + `git status` 仅 2 个新增 PNG + 复跑全绿）。
- SPEC 评审：**pass**（8/8 PASS；delta 保真度独立逐字复核通过；golden
  纪律证据链完整；备注"非 active 支路由单元矩阵钉住、像素层用 HasTarget
  支路"为合理覆盖拆分）。QUALITY 评审：**pass**（4/5 与 5/5 居多；四条
  非阻塞 minor：场景注释 5.5/4.5 格口径、delta 缺 Purpose、零漂移断言
  Fatalf 补区域矩形、场景表头注释例外清单补漏）。
- Ruling: R1 修复四处注释/文档级 minor — 均不触行为与 golden — 记录于
  R1 落地条目。
- R1 落地：commit `dff3ee5a`（场景注释 4.5..5.5 格口径、delta 补 Purpose、
  Fatalf 补区域矩形、场景表头注释例外清单补漏）。`go test
  ./cmd/mornlea/capture -race -count=1` ok；`openspec validate --all
  --strict` 80/80；gofmt 干净。Task 4 关闭。

## 阶段 3 末：整分支终审（fresh reviewer @ `26f53165`）

- **pass**，无 block 项。跨语言 80B/tag10 契约逐字段一致；规格↔实现映射表
  全部 Scenario 有着落（Scenario 8 六面 / 9 遮挡不穿透为结构性覆盖：单实例
  整包围盒 + 只读深度 LessEqual 管线配置，无直接像素断言，备案接受——
  capture 场景无遮挡物，遮挡语义与 outline 共享同一机制先例）；提交序列
  卫生、遗留扫描、门禁证据完备性（终审者在 HEAD 复跑 gofmt/vet 与两侧
  focused 测试全绿）均通过。
- mainly-cosmetic 备案：ledger R1 落地条目与 tasks.md 勾选两条簿记已由
  控制会话补齐；`dff3ee5a` 纯注释改动提交类型 `docs(capture)` 偏宽（历史
  提交不重写）。
