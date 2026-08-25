# settings-menu 执行 ledger

## 领取与基线

- 任务：`D-01 设置菜单`
- 分支：`codex/D-01-settings-menu`
- 基线：`origin/main` `345f6077`
- 领取提交：`bfb91297`（已推送）
- 领取讨论：https://github.com/channing771/mornlea/discussions/71#discussioncomment-18147961
- 工作树：`/Users/chen/work/mornlea/.worktrees/D-01-settings-menu`
- 规划调查：只读子代理 `d01_design_investigation`；未修改实现。

## 开工前验证

| 命令 | 结果 |
| --- | --- |
| `make rust` | PASS |
| `go test ./internal/config ./internal/client ./cmd/mornlea -race -count=1` | PASS（`internal/config` 2.588s；`internal/client` 4.318s；`cmd/mornlea` 270.709s） |

## 子任务裁决

每个实现任务由全新的 implementer 完成，并由互相独立的 SPEC 与 QUALITY reviewer 双裁决。任何修复轮次、候选提交、验证结果和控制会话 ruling 均在本表追加；未完成双裁决前不得勾选 `tasks.md`。

| 任务 | Implementer | 候选/修复提交 | SPEC reviewer | QUALITY reviewer | 修复轮次 | 验证 | 控制会话 ruling |
| --- | --- | --- | --- | --- | ---: | --- | --- |
| 1.1 配置契约 | `d01_t1_config_impl` | `e1bfaa89` | `d01_t1_spec_review`：PASS | `d01_t1_quality_review`：PASS | 0 | RED 编译失败；`make rust`、`go test ./internal/config -race -count=1`、`go vet ./internal/config`、archcheck、gofmt、diff-check PASS | ACCEPTED |
| 2.1 client ABI v9 | `d01_t2_abi_impl` | `79623956`；修复 `e0e08099`、`c0b5e5d0` | `d01_t2_spec_review`：FAIL→PASS→PASS | `d01_t2_quality_review`：FAIL→FAIL→PASS | 2 | RED 编译失败；Rust 105 tests、clippy/fmt、`make rust`、client race、cmd compile/action routing、diff-check PASS；Rust 测试名集合不变 | ACCEPTED |
| 3.1 Rust egui 设置页 | `d01_t3_rust_ui_impl` | `0c656133`；修复 `c79150f6`、`3643b0bf` | `d01_t3_spec_review`：PASS→PASS→PASS | `d01_t3_quality_review`：FAIL→PASS→PASS | 2 | RED 44 个编译错误 + 容量预检失败 + action 超 viewport 17pt；`make rust-check`（client 119、engine 160）、release Rust、client race、fmt/clippy/diff-check PASS | ACCEPTED |
| 4.1 Go 事务与接线 | `d01_t4_go_settings_impl` | `844c9f9c`；规格协调 `ddaa419e`；修复 `9819d823` | `d01_t4_spec_review`：PASS→PASS | `d01_t4_quality_review`：FAIL→PASS | 1 | RED 缺少状态类型 + CR/LF/资源前置测试失败；cmd full race 279.221s、short race、config/client race、Rust 118、clippy/fmt、vet/diff-check PASS | ACCEPTED |
| 5.1 视觉基线 | `d01_t5_visual_impl` | `845250f2`、`ce98240d`；修复 `7c72527f`（另触发 Task 3 `3643b0bf`） | `d01_t5_spec_review`：PASS→PASS | `d01_t5_quality_review`：FAIL→PASS | 1 | RED 缺少 Settings fixture；19场景、focused/short race、两张 UI 0差、17张旧 PNG hash不变、人工检查 PASS；全局 visual-check 的13个旧场景继承失败见 R-011 | ACCEPTED（变更特定视觉门禁通过） |
| 5.2 长期文档 | `d01_t5_docs_impl` | `e929388a`；修复 `f096cb33`、`7496406d`、`1b2c1f0c`、`e41e328d`、`2ced2d37`；5.3 后终审 | `d01_t5_docs_spec_review`：PASS→FAIL→FAIL→PASS→FAIL→FAIL→PASS | `d01_t5_docs_quality_review`：FAIL→PASS→PASS→FAIL→FAIL→PASS→PASS | 5（达到上限） | archcheck、OpenSpec strict、两语 JSON/链接、AGENTS/CLAUDE cmp、版本/场景扫描、diff-check PASS | ACCEPTED（剩余项由5.3清偿） |
| 5.3 README tunable 勘误 | `d01_t5_tunable_docs_impl` | `42c94f41`；修复 `45118bb0` | `d01_t5_tunable_spec_review`：FAIL→PASS | `d01_t5_tunable_quality_review`：PASS→PASS | 1 | 文档 RED/GREEN 断言、JSON/链接、archcheck、OpenSpec strict、cmp、diff-check PASS | ACCEPTED |
| 6.1 整分支收尾 | `d01_finalizer` | 主线融合至 `c2b799ce`；修复 `13318afc`、`ad347b84`、`8f639e7a`；最终 HEAD `b41bc2e9` | `d01_branch_final_review` 综合终审：FAIL→FAIL→PASS→PASS | 同左（整分支规格+质量双域） | 1 | Rust 127+160、全仓 race 581s、vet、gofmt、archcheck、OpenSpec 65/65、cmp、diff/golden/status PASS；visual 见 R-011 | ACCEPTED |

## 整分支终审与发布

- 独立终审：`d01_branch_final_review` 最终 PASS（两次阻断审查、两次 PASS 快照确认）
- 最终门禁：除 R-011 的 baseline-equivalent `make visual-check` 非零外全部 PASS；全仓 race 最终 PASS 581s
- OpenSpec 同步/归档：主规格同步完成（5 份 delta 全部合入并通过 strict validation）；归档 PENDING
- PR / CI / 合并：PENDING
- Discussion 完成回报：PENDING

## Rulings

- R-001：设置页只纳入 `audioVolume`、`texturePackPath` 和固定 `windowSize` 三项；不扩展在线协议、世界/玩家存档、benchmark scenario 或引擎 ABI。
- R-002：材质包保存时允许在世界装配前做候选校验，但活动材质注册表不热替换；新路径下次启动生效。
- R-003：持久化成功是运行时音频/窗口应用的前置条件；保存失败时磁盘、committed 状态与运行时均保持不变。
- R-004：client ABI v9 使用整批结构化事件与原子容量门禁；不得以旧 raw button ID 通道夹带设置值。
- R-005：接受非法直接构造的 `WindowSize.Dimensions()` 返回 `(0, 0)`；所有配置与 UI 输入边界负责先拒绝非法枚举，合法三预设映射已由测试锁定。
- R-006：client ABI 升级任务必须让现存 `cmd/mornlea` 保持可编译；在设置业务接入前，typed action 继续路由，settings-changed 只进入显式 deferred 分支且不得误触按钮。
- R-007：因本任务大量触碰既有混装 Rust UI 测试模块，按项目硬规范在同一任务拆成关注点文件并保留测试名集合；漏写主题头注释按阻断项修复，不以“仅测试”降级。
- R-008：egui UI 帧在消费 RawInput 前按布局保守预留最坏事件数（主菜单 8、设置页 4）；剩余容量不足时允许提前显式 `CAPACITY`，以换取队列、焦点、光标与滚动状态可无损重放。空队列始终可运行正常帧。
- R-009：`texturePackPath` 的单行不变量必须在 `Config.Load` 与 application 直接 options 构造两层防御；Darwin 上真实存在但名称含 CR/LF 的目录也拒绝，以保证任何 committed/draft 均可安全编码 layout v2。该兼容性收紧已同步 proposal/spec/design，配置版本保持 v1。
- R-010：Task 5 正式 640×360 图像发现三动作底部被裁 17pt，判为 Task 3 呈现回归而非 capture 可接受差异；由原 Task 3 implementer 修复并复审后再生成 golden，capture implementer 不越界改 Rust UI。
- R-011：冻结主线 `origin/main@c2b799ce` 与 D-01 在同机完整运行 `make visual-check` 均 exit 2；18 个同名场景的 `(maxdiff, count, first)` 逐项完全一致，双方同为 15 个旧场景非零、11 个超阈值。D-01 `main-menu` 对其新 golden 为 0 差，并新增 0 差的 `settings-menu`；其余 17 张主线 golden blob 不变。判为继承的机器视觉基线失败，不刷新无关 golden、不放宽阈值；整分支门禁如实记为“全局 FAIL（baseline-equivalent）”。
- R-012：任务 5.2 已达到 5 轮修复上限。最终 QUALITY PASS，但 SPEC 指出的唯一剩余项有效：掉落 10/40 tick、1.25 格与 6000 tick 均为可配置 `sim` tunable 的编译默认值，README 不得写成固定常量。该阻断不豁免、不继续第 6 轮，拆为全新任务 5.3，由新 implementer 和独立双评审清偿；5.3 通过后再终结 5.2。
- R-013：任务 5.3 已把四项掉落 tunable 全部限定为编译默认值，并把中文 `sim` 组从“常量”修为“参数”；独立双评审与原 5.2 双评审均最终 PASS，R-012 阻断正式关闭。
- R-014：整分支终审时 branch 相对最新 `origin/main` 已分叉 `31/33` 并在基线文档/进度产生冲突；任务 6.1 必须先集成最新主线、人工融合当前能力，再做任何最终门禁，不能以旧基线通过代替可合并性。
- R-015：设置保存契约中的“所有未暴露字段保持磁盘值”包含未知顶层/嵌套 JSON 与会被加载器钳制的已知字段原始值；typed `Config.Load` 后整份 `Save` 不合格，必须用经过完整验证的 raw JSON 三顶层字段原子 patch。
- R-016：`rust-client-window` 明确要求当前显示器工作区；屏幕全尺寸 90% 近似不合格。Darwin 创建与运行期 resize 必须读取实际 `NSScreen.visibleFrame` 并正确处理逻辑 point/物理像素，或在实现前显式修改规格（当前 ruling 选择实现真实工作区）。
- R-017：client 输出队列容量预检必须发生在 renderer 获取/清空窗口输入之前；只保护 `UiState` 内部瞬态不足以满足 R-008 的无损重放。
- R-018：归档 delta 必须完整修改稳定视觉规格中所有精确数量/全序/尾序 requirement，不能让 19 场景与旧“恰好17/全部17”断言并存。
- R-019：最终全仓 race 首跑 354s 唯一失败 `TestRemoteMessagesRouteOnlyToRoster`；该生产/测试路径与主线相同，单测 race `count=50`、完整 `cmd/mornlea` race、后续两次全仓 race（含冻结主线后的 581s 最终跑）均 PASS。判为共享 helper 固定 1ms 等待在满载 race 下的继承时序脆弱，不在 D-01 越界修改，但首次失败与全部复现证据必须保留。
- R-020：任务 6.1 修复 top-level `null` raw patch、rename commit-point 的 pre/postcommit 结果、真实 `NSScreen.visibleFrame` outer-frame/位置约束、renderer 外层输入重放及完整视觉 delta；冻结主线合并后独立整分支终审最终 PASS。
- R-021：OpenSpec 主规格同步覆盖 `egui-tool-ui`、`rust-client-window`、新建 `settings-menu`、`texture-pack-loading` 与 `visual-verification` 五个规格；保留 ABI 字体缺失这一未被 delta 推翻的既有场景，其余 delta requirement 与稳定规格逐项一致，strict validation 66/66 PASS。
