## 1. 原创心形与气泡 atlas（对应执行 Task 2）

- [x] 1.1 重新读取 `proposal.md`、两个 delta spec、`design.md` 和本组任务；在 clean checkout 先运行 `make rust`，并把 implementer 身份与起始 commit 记入 `ledger.md`。
- [x] 1.2 在 `internal/render/hud/atlas_test.go`、`health_test.go` 与 `oxygen_test.go` 先写失败测试：固定空/半/满心、空/满气泡五列顺序，证明每列非空且互异、alpha 仅 0/255、atlas 连续构建逐字节相等、偏移后的物品列仍逐像素等于 registry 顶面、全部列 UV 不越 16×16 cell；运行 `go test ./internal/render/hud -run 'TestHotbar(TextureAtlas|ColumnUV)' -count=1` 并记录预期失败。
- [x] 1.3 在 `internal/render/hud/atlas.go` 与 `health.go` 最小实现原创程序化半心/气泡、固定列偏移及 `hotbarHeartUV`/`hotbarBubbleUV` 窄 helper；复用现有 atlas 与 `hotbarTextureUV`，不增加 PNG、sprite registry、主题对象或新依赖。
- [x] 1.4 运行 `gofmt -w internal/render/hud/atlas.go internal/render/hud/atlas_test.go internal/render/hud/health.go internal/render/hud/health_test.go internal/render/hud/oxygen_test.go`、`go test ./internal/render/hud -race -count=1`、`go test ./internal/archcheck -count=1` 与 `git diff --check`，把命令、退出状态和关键计数写入 `ledger.md`。
- [x] 1.5 implementer 自证通过后提交候选 `git commit -m "feat: add survival HUD atlas icons"`；记录本组 `BASE`（候选提交的 parent）与 `HEAD`，生成包含两者、`git diff "$BASE..$HEAD"` 及其 SHA-256 的 scratch review package。
- [x] 1.6 把 committed review package 交给一名新的独立 task reviewer，由同一 reviewer 同时给出 SPEC 与 QUALITY 两项裁决，并记录其身份、finding 和裁决；reviewer MUST NOT 评审未提交工作区 diff。
- [x] 1.7 任一 finding 只以追加 fix commit 修复，不得 amend 候选提交；每轮重跑 1.4、更新 `HEAD`/review package，并由同一 scoped task reviewer 同时复审 SPEC 与 QUALITY，最多 5 轮。两项均 PASS/CLEAN 后用后续 bookkeeping commit 回填 `ledger.md`，不得把评审前结论写入候选提交。

## 2. 快捷栏、选中格、耐久与采掘轨道（对应执行 Task 3）

- [x] 2.1 在 `internal/render/hud/layout_test.go` 与 `renderer_test.go` 先写失败测试：固定关闭态居中九格、面板外阴影/内表面、每格表面、选中格外扩高对比外框与强调内框、数量双层最多两位及耐久显示条件；运行 `go test ./internal/render/hud -run 'Test(ClosedHotbar|HotbarCount|Durability)' -count=1` 并记录预期失败。
- [x] 2.2 在 `internal/render/hud/layout.go` 增加当前产品所需的包内常量和 `hotbarRowBounds`，让 `inventorySlotOrigin`、状态锚点与采掘轨道复用同一几何；只改关闭态快捷栏绘制，继续复用现有物品、数量和耐久 append 路径，不改变打开态容器视觉或 `InventorySlotAt`。
- [x] 2.3 在 `layout_test.go` 先覆盖采掘 inactive、零 required ticks、0%、中段、超 100%、可采与不可采，证明可采有移动末端 cap、不可采有固定 warning notch，忽略 RGB 后几何仍不同；运行 `go test ./internal/render/hud -run 'TestMining' -count=1` 并记录预期失败。
- [x] 2.4 在 `layout.go` 以现有纯色 `hotbarInstance` 最小扩展 `appendMiningBar`：比例钳制到 1，可采追加末端 cap，不可采追加固定 notch；按本组新增实例更新关闭分支容量组成，不增加 shader、贴图 cell 或动画。
- [x] 2.5 运行 `gofmt -w internal/render/hud/layout.go internal/render/hud/layout_test.go internal/render/hud/renderer_test.go`、`go test ./internal/render/hud -race -count=1`、`go test ./internal/archcheck -count=1` 与 `git diff --check`，记录验证证据。
- [x] 2.6 implementer 自证通过后提交候选 `git commit -m "feat: restyle survival hotbar feedback"`；记录候选 parent 为 `BASE`、候选为 `HEAD`，生成包含 committed diff 与 SHA-256 的 scratch review package，再交给一名新的独立 task reviewer，由同一 reviewer 同时给出 SPEC 与 QUALITY 两项裁决。
- [ ] 2.7 finding 只以追加 fix commit 修复；每轮重跑 2.5、更新 review package，并由同一 scoped task reviewer 同时复审 SPEC 与 QUALITY，最多 5 轮。重点检查 `InventorySlotAt`、打开态容器视觉、权威物品语义和非颜色形状；两项 PASS/CLEAN 后用后续 bookkeeping commit 回填 ledger。

## 3. 生命、氧气、响应式布局与固定容量（对应执行 Task 4）

- [ ] 3.1 在 `internal/render/hud/health_test.go` 先写失败测试，覆盖未确认、0/1/2/19/20/越界、零尺寸与打开/关闭态，断言十个空心及半/满覆盖数、没有额外生命背景实例、关闭态快捷栏左上锚点和打开态快捷栏左下锚点；运行 `go test ./internal/render/hud -run 'TestHealth' -count=1` 并记录预期失败。
- [ ] 3.2 在 `internal/render/hud/health.go` 最小重写生命布局：删除未使用 atlas 参数，钳制到 `core.MaxHealth`，只使用完整空/半/满 cell 且不追加背景，并仅依赖 `hotbarRowBounds`、`open` 和 framebuffer 尺寸。
- [ ] 3.3 在 `internal/render/hud/oxygen_test.go` 先写失败测试，覆盖未确认、满值、0、1 tick、全部分段边界、`core.MaxOxygenTicks-1`、越界、零尺寸与打开/关闭态；断言满氧零实例、耗氧十空槽加整数向上取整覆盖，以及快捷栏右侧上下锚点；运行 `go test ./internal/render/hud -run 'TestOxygen' -count=1` 并记录预期失败。
- [ ] 3.4 在 `internal/render/hud/oxygen.go` 最小改为 atlas 气泡：钳制权威值，保留未确认/满氧隐藏，用纯整数公式计算十段覆盖，不再绘制纯色比例横条。
- [ ] 3.5 在 `layout_test.go` 与 `renderer_test.go` 先写响应式/命中失败测试：覆盖 1280×720、640×360、窄窗口和零尺寸的打开/关闭态，遍历全部 rectangle 验证 framebuffer 内，证明打开态生命/氧气不与 36 个 `InventorySlotAt` 可命中格相交，且边界命中样本语义不变；运行 `go test ./internal/render/hud -run 'Test(Responsive|Open.*Status|InventorySlotAt)' -count=1` 并记录预期失败。
- [ ] 3.6 在 `renderer_test.go` 先写容量见证失败测试：分别构造关闭与打开互斥分支合法最大组合，要求 `healthQuads == 20`、`oxygenQuads == 20` 且较大分支见证 `maxHotbarQuads`；同时锁定 48-byte 编码、256-byte offsets、区间不重叠和实际实例前缀。运行对应 `go test ./internal/render/hud -run 'TestHotbar(Fixed|Capacity|Maximum)' -count=1` 并记录因容量尚未对账而失败。
- [ ] 3.7 在 `internal/render/hud/renderer.go` 只把已有 `open` 传给生命/氧气布局；在 `layout.go` 扩展关闭态联合缩放边界并实现打开态状态行避让、零尺寸无实例；分别计算关闭/打开分支上限后取较大值，保持 `internal/render/hud/encode.go` 不变，以最小实现使 3.5/3.6 红测转绿。
- [ ] 3.8 运行 `gofmt -w internal/render/hud/health.go internal/render/hud/health_test.go internal/render/hud/oxygen.go internal/render/hud/oxygen_test.go internal/render/hud/layout.go internal/render/hud/layout_test.go internal/render/hud/renderer.go internal/render/hud/renderer_test.go`、`go test ./internal/render/hud -race -count=1`、`go test ./internal/archcheck -count=1` 与 `git diff --check`，记录验证证据。
- [ ] 3.9 implementer 自证通过后提交候选 `git commit -m "feat: align survival status around hotbar"`；以候选 parent/候选提交生成 committed review package 和 SHA-256，再交给一名新的独立 task reviewer，由同一 reviewer 同时给出 SPEC 与 QUALITY 两项裁决。
- [ ] 3.10 finding 只以追加 fix commit 修复；每轮重跑 3.8、更新 review package，并由同一 scoped task reviewer 同时复审 SPEC 与 QUALITY，最多 5 轮。重点检查权威值来源、生命无背景、满氧隐藏、打开态可见性/避让、窄窗口、固定容量和编码不变；两项 PASS/CLEAN 后用后续 bookkeeping commit 回填 ledger。

## 4. 确定性 survival feedback capture 与 golden（对应执行 Task 5）

- [ ] 4.1 在新建 `cmd/mornlea/capture_hud_test.go` 和既有 capture tests 先写失败测试：固定完整 15 场景顺序、`hud-survival-feedback` 紧随 `hud-hotbar-health`、`far-horizon` 倒数第二、`water-underwater` 唯一末尾，以及 HUD 夹具值和一次/重复 restore 后 predictor 指针与 mining overlay 恢复；运行 `go test ./cmd/mornlea -run 'Test(HUDCapture|CaptureSceneOrder|WaterUnderwater|FarHorizon)' -count=1` 并记录预期失败。
- [ ] 4.2 在 `cmd/mornlea/capture.go` 增加最小 `captureHUDFixture` 和 `captureScene.HUD`：保存原 predictor/mining，复用原有限物理状态构造固定 Ready/Overworld predictor，钉住 health/oxygen/yaw/pitch/mining，并返回幂等 restore；不修改产品 `Predictor` API 或 `renderFrame` 热路径。
- [ ] 4.3 在 scene Apply 后、收敛帧前应用夹具并对所有后续返回路径 `defer restore()`；注册固定低血、`core.MaxOxygenTicks/3`、磨损工具、不可采 `4/9` 进度的 `hud-survival-feedback`，删除旧的部分心形 capture 限制注释，并更新既有顺序测试。
- [ ] 4.4 运行 `gofmt -w cmd/mornlea/capture.go cmd/mornlea/capture_ai_companion_test.go cmd/mornlea/capture_hud_test.go`、`go test ./cmd/mornlea -run 'Test(HUDCapture|Capture.*Scene|WaterUnderwater|FarHorizon)' -count=1`、`go test ./cmd/mornlea -race -count=1`、`go test ./internal/archcheck -count=1` 与 `git diff --check`。
- [ ] 4.5 在可用 Metal 环境运行 `make visual-update VISUAL_OUT=build/visual-bedrock-survival-hud-update`；确认场景共 15 张、新增 `hud-survival-feedback.png`，且只提交实际受影响的 `cmd/mornlea/testdata/golden` 文件，不移动场景尾序、不放宽双阈值。
- [ ] 4.6 人工逐图复核全部 15 张候选并把文件清单与结论写入 `ledger.md`；重点确认 `hud-hotbar-health` 的九格/双层选中/数量/耐久/无背景满血/满氧隐藏，`hud-survival-feedback` 的低血/气泡/磨损工具/不可采中段形状，以及 `inventory-crafting` 的状态行避让；任何世界、实体、光照、水或 LOD 区域异常先修根因。
- [ ] 4.7 运行 `make visual-check VISUAL_OUT=build/visual-bedrock-survival-hud-final`、`go test ./cmd/mornlea -run 'Test(HUDCapture|Capture.*Scene|WaterUnderwater|FarHorizon)' -count=1` 与 `git diff --check`。
- [ ] 4.8 implementer 自证和人工验收通过后提交候选 `git commit -m "test: lock survival HUD presentation"`；以候选 parent/候选提交生成 committed review package 和 SHA-256，再交给一名新的独立 task reviewer，由同一 reviewer 同时给出 SPEC 与 QUALITY 两项裁决。
- [ ] 4.9 finding 只以追加 fix commit 修复；每轮重跑 4.4/4.7、必要时重新逐图验收、更新 review package，并由同一 scoped task reviewer 同时复审 SPEC 与 QUALITY，最多 5 轮。重点检查 fixture 仅限 capture、错误路径恢复、场景隔离、完整顺序与阈值；两项 PASS/CLEAN 后用后续 bookkeeping commit 回填 ledger。

## 5. 长期基线、全量验证与整分支终审（对应执行 Task 6）

- [ ] 5.1 仅在实现与 visual 验收完成后逐字节同步 `AGENTS.md`/`CLAUDE.md` 的当前能力描述，并在 `docs/notes/progress.md` 记录里程碑；不把 change 非目标写入长期基线，不推进协议 v23、engine ABI v6、client ABI v7、benchmark scenario v18 或任何存档 schema。
- [ ] 5.2 对账 `tasks.md` 与 `ledger.md`：只勾选已有实现、focused 验证、spec review 和 quality review 证据的条目，记录所有 implementer/reviewer、commit、finding、修复轮次、人工 visual 结论与 controller ruling。
- [ ] 5.3 运行 `gofmt -l .`、`go test ./internal/render/hud ./cmd/mornlea -race -count=1`、`go test ./internal/archcheck -count=1`、`cmp -s AGENTS.md CLAUDE.md` 与 `git diff --check`；`gofmt -l .` 必须无输出，其余命令必须退出 0。
- [ ] 5.4 在整分支终审前仅运行一次 `make rust`、`make rust-check`、`go test ./... -race`、`go vet ./...`、`make visual-check VISUAL_OUT=build/visual-bedrock-survival-hud-final-review` 与 `openspec validate --all --strict --no-interactive`；不得放宽 correctness、overflow、完整性、I/O、数据丢失或视觉阈值门禁。
- [ ] 5.5 用 `git diff --name-only "$(git merge-base HEAD main)"..HEAD`、`git diff --stat "$(git merge-base HEAD main)"..HEAD` 与 `git status --short` 复核候选范围；确认没有产品改动落入 `engine/`、`internal/network`、`internal/sim`、`internal/storage` 或 `cmd/mornlea-server`，没有 PNG UI 源资产、第三方依赖、配置键、协议字段或 ABI 变化。
- [ ] 5.6 implementer 自证通过后先提交长期文档与 ledger 候选 `git commit -m "docs: record survival HUD milestone"`；记录 `BASE=$(git merge-base HEAD main)` 与 committed `HEAD`，生成覆盖完整 `BASE..HEAD` diff 及 SHA-256 的整分支 review package。
- [ ] 5.7 把 committed 整分支 review package 交给一名新的独立 task reviewer，由同一 reviewer 同时给出 SPEC 与 QUALITY 两项裁决，并逐项核对 proposal/spec/design/tasks、权威数据来源、生命无背景、容器避让、固定容量、capture 恢复、完整场景顺序、原创资产和 golden 范围。
- [ ] 5.8 finding 只以追加 fix commit 修复，不得 amend 历史候选；每轮按影响范围重跑 5.3/5.4、更新 `HEAD`/review package，并由同一 scoped task reviewer 同时复审 SPEC 与 QUALITY，最多 5 轮。两项 PASS/CLEAN 后以纯 bookkeeping commit 回填最终结论，再重跑 5.5 把该提交纳入最终范围复核并确认工作区只剩已知无关改动；不得自行归档，取得用户明确批准后才运行 `openspec-archive-change`。
