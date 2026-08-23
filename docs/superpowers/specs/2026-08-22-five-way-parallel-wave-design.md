# Mornlea 下一批五路并行设计

日期：2026-08-22
状态：已逐节确认，实施计划已完成
前置：先归档已完成的 `bedrock-survival-hud`，五个实现 change 再从归档后的同一 `main` HEAD 建立独立 worktree。

## 1. 背景与目标

当前 `main` 已合入 PR #64，生存 HUD、默认 Pixel Perfection 材质、权威饥饿、远环 LOD、农业和流体闭环均已存在。下一批不继续堆叠同一改动面，而采用三项玩家可见能力与两项工程稳定性修复的混合波次：

1. 对齐背包、合成、箱子和熔炉的容器 UI；
2. 交付服务端权威的玩家徒手近战；
3. 交付 Darwin 图形客户端的基础本地音效；
4. 消除 PR 重复 CI 并把长作业拆成可单独重跑的门禁；
5. 修复伙伴台词过时结果测试的调度偶发性。

成功标准不是“同时开五个分支”，而是五项从同一基线出发后可以独立实现、评审、验证、回退和合入。产品代码主要改动面必须分离；共享长期文档与主规格只在实现合入后串行归档。

## 2. 已选组织方式

采用五个独立 OpenSpec change、五个独立 worktree/PR。每项都遵循仓库的 subagent-driven-development 流程：每个实现任务由全新 implementer 完成，随后由独立 reviewer 同时裁决规格合规与代码质量；finding 进入有界修复循环，执行证据与 ruling 写入各自 ledger，最后做整分支终审。

被否决的两种组织方式：

- 先抽 UI token、客户端事件总线等共享基础层再并行：它会新增未经真实任务证明的抽象，还会制造第六个前置任务；
- 允许五项交叉修改、最后集中解冲突：它把实现速度换成一次高风险语义整合，重演 HUD 与 hunger 合流的成本。

## 3. 并行边界

| Change | 主要独占改动面 | 明确不碰 |
|---|---|---|
| `container-ui-visual-alignment` | `internal/render/hud/container*`、容器 capture/golden | 协议、权威模拟、音频、CI |
| `authoritative-player-melee` | `internal/sim/combat*`、生命/协议相关规格与测试 | HUD、伙伴、音频、CI |
| `darwin-local-audio-feedback` | 新音频包、config、独立 app 音效接线 | HUD 布局、协议、权威模拟 |
| `ci-retry-isolation` | `.github/workflows/ci.yml` | 产品代码、测试语义与性能阈值 |
| `stabilize-companion-dialogue-outcome-test` | `internal/server/companion_dialogue*_test.go` | 生产台词状态机与超时值 |

四个不升协议的实现 PR 不修改 `AGENTS.md`、`CLAUDE.md` 或 `docs/notes/progress.md`。近战 PR 因 `TestBaselineVersionsMatchCode` 的 fail-closed 门禁，只逐字节同步 `AGENTS.md`/`CLAUDE.md` 的当前协议号 v24→v25，不提前写完整能力段；五项合入后再按实际最终版本串行归档各 change，并在归档提交中同步主规格、完整能力描述与 `progress.md`。不得修改或放宽 archcheck 来消除这个例外。

## 4. Change 1：容器 UI 视觉统一

### 4.1 可观察行为

`container-ui-visual-alignment` 把现有背包、合成、箱子和熔炉统一为原创像素视觉：高对比外框、内凹格子、中文标题、来源格选中轮廓，以及熔炉的火焰和进度构图。它只参考 Minecraft 容器界面的构图层级与信息关系，不导入、复制、描摹或分发 Mojang UI 像素、字体、截图切片或其他版权资产。

背包仍为 27 格背包加 9 格快捷栏；熔炉仍使用统一栏位 `0..38`；箱子仍使用统一栏位 `0..62`；合成仍为当前十条固定配方。两次点击整堆移动、配方可用性、容器打开/关闭与全部服务端权威语义不变。

### 4.2 布局与资源

视觉矩形与 `InventorySlotAt`、`FurnaceSlotAt`、`ChestSlotAt`、`RecipeButtonAt` 必须继续由同一份现有布局边界推导，禁止另写一套只供命中或只供绘制的坐标。打开容器后的生命、氧气和饥饿状态栈继续位于交互区域之外。

重绘以替换既有矩形为主，不扩大固定 HUD 资源：保持 267 quad、700 glyph、13312-byte glyph offset、46912-byte 总容量、48-byte instance 和 256-byte 对齐。协议 v24、玩家 schema v7、benchmark scenario v19、engine ABI v6 与 client ABI v7 均不因本 change 改变。

### 4.3 视觉验证

在既有 `inventory-crafting` 之外新增 `chest-container` 与 `furnace-container` 两个正式无窗口场景，使正式清单从 15 增至 17。两个新场景紧随 `inventory-crafting`，既有其余相对顺序保持不变，`far-horizon` 仍为倒数第二，`water-underwater` 仍为唯一末场景。

全部 17 张 golden 必须由正常完整渲染链路生成并逐张人工复核；双阈值、显式更新纪律和不启动前台窗口的要求保持不变。

### 4.4 失败处理与验证

零尺寸 framebuffer 继续产生空输出；任何实例容量溢出必须返回错误而不是截断。focused 验证至少覆盖全部栏位命中、十条配方、来源选择、极窄正尺寸 framebuffer、固定容量与 17 场景顺序；收尾运行视觉更新/比对、`go test ./... -race`、`go vet ./...`、`gofmt -l .`、archcheck 与 OpenSpec strict。

## 5. Change 2：服务端权威玩家近战

### 5.1 输入与权威边界

`authoritative-player-melee` 复用现有 `PlayerInput.Mining` 作为“主操作键正在按住”的 wire 信号，不增加客户端声明的目标 ID，也不新增攻击消息。客户端只上行按键与朝向；服务端从权威玩家位置、朝向、生命周期和世界方块重新选择目标。

虽然 wire 字节形状不变，`Mining` 的语义从“只尝试采掘”扩展为“先尝试玩家近战，否则采掘方块”，因此协议身份从 v24 升到 v25，禁止新旧语义客户端静默混用。玩家、区块、世界与伙伴存档 schema 均不变。

### 5.2 目标选择与结算

每个权威 tick 在玩家完成物理推进与订阅收敛后、`settleDeaths` 与方块采掘之前执行近战判定，不增加新的 `stepPhase`：

1. 攻击者与目标都须为生命值大于零的 `PlayerActive`，目标还须同维度且非自身；
2. 用攻击者权威眼睛位置与 look 方向，对目标的 `physics.PlayerBounds` 做射线相交；
3. 最大触及距离固定为 3 格；先取最近交点，距离相同按 `SessionID` 决定；
4. 用现有 Rust 方块 DDA 路径取得更近的阻挡方块，方块在目标前时不得命中；流体继续沿现有交互谓词不阻挡射线；
5. 冷却为零时命中造成固定 2 点伤害并把冷却设为 10 tick；冷却未结束时不造成伤害；
6. 有效玩家命中取消攻击者本 tick 的方块采掘进度，否则既有采掘行为逐位保持。

候选枚举和攻击意图先从 tick 内同一份权威玩家快照写入最多八项的固定数组，再按攻击者 `SessionID` 应用伤害。这样同 tick 已形成的合法攻击不会因攻击者先被另一意图打到零血而消失，双方可以在同一 tick 互相击杀；同一目标收到多次伤害仍按固定顺序结算。全部意图应用后才执行既有 `settleDeaths`，外部仍观察不到生命值为零的中间状态。

近战伤害必须调用现有 `applyDamage`，从而复用回血计时清零、进食取消、死亡掉落、出生锚点重生与客户端确认红边反馈。冷却是瞬态玩家状态，不持久化；死亡与 reset 清零。伙伴、待出生玩家与攻击者自身永远不是目标。本 change 不增加武器伤害、护甲、击退、伙伴受伤或攻击者命中特效。

### 5.3 确定性与验证

测试必须覆盖：距离边界、方块遮挡、最近目标、等距 `SessionID` 裁决、按住攻击冷却、攻击与采掘互斥、死亡复用、双方同 tick 互击、八玩家枚举上界、自身/伙伴/待出生排除，以及 Memory/TCP parity。协议 v25 需更新 codec golden、fuzz 边界、登录版本身份与长期基线门禁；benchmark 固定工作负载若没有玩家主操作输入则只记录实测影响，不因无可观察 workload 变化擅自迁移 scenario。

## 6. Change 3：Darwin 基础本地音效

### 6.1 播放边界

`darwin-local-audio-feedback` 只服务 Darwin 图形客户端。实现使用 macOS 原生 AudioToolbox 和一份随 application 生命周期复用的播放器，不新增第三方依赖、网络消息、服务端状态或二进制音频资产。

四类原创短音效在启动时由固定参数生成 PCM：有效 UI 点击、确认采掘完成、确认进食完成、确认受伤。生成参数与样本数固定，使测试和不同运行间字节可复现；播放器使用有界预分配缓冲，热路径不得为每个 cue 创建新设备或启动外部进程。

### 6.2 触发语义

- UI 点击：鼠标命中当前有效栏位或可用配方，并实际推进本地选择/发送动作时立即播放；点击空白或禁用配方不播放。
- 采掘完成：权威方块变更确认当前采掘目标被移除时播放；松键、切换目标或拒绝不播放。
- 进食完成：确认饥饿上升且对应选中食物数量减少时播放；两份确认消息无论先后都由同一个有界本地状态机配对，reset、选中格变化或会话关闭立即清除半份匹配；在途、取消或服务端拒绝不播放。
- 受伤：确认生命值下降时播放；首次确认、回血、重生或未确认状态不播放。

后三类只从已有确认镜像的状态迁移推导，客户端不得预测结果。capture、benchmark、无图形专服和 Linux bundle 不初始化音频。

### 6.3 配置、降级与验证

新增唯一可选配置 `audioVolume`，合法范围 `0..1`，缺省为 `0.7`，`0` 即静音，不再增加第二个 mute 字段。配置 schema 保持 v1；非法值在配置边界拒绝。

设备初始化或播放失败只记录一次不含设备私密信息的告警，并把当前播放器降级为静默；音频失败不得阻止客户端启动、权威 tick 或渲染。自动测试通过 cue 回调和确定性 PCM 校验覆盖触发/不触发、音量边界、静音、重复播放与关闭幂等，不访问真实音频设备；最终另做一次人工听感验收。

## 7. Change 4：CI 去重与失败分片

### 7.1 触发与取消

`ci-retry-isolation` 把 workflow 的 `push` 限定为 `main`，保留全部 `pull_request` 触发，并按 PR 或 ref 设置 concurrency；同一 PR 新 SHA 到来时取消旧 SHA 的未完成运行。这样 feature 分支有 PR 时每个 SHA 只产生一条 workflow，合并后 `main` 再独立验证一次。

### 7.2 作业图

macOS 单体 `test` 拆为：

- `native-macos`：Rust 工具链身份、`make rust-check`、`make rust`，以包含 `${{ github.sha }}` 的 artifact 名称上传本次 SHA 对应的 engine/client dylib；
- `quality`：下载 dylib，运行 OpenSpec、Agent Hooks、架构/存储/协议门禁、vet 与 gofmt；
- `go-race` matrix：下载同一 dylib，分为 `cmd`、`internal/server`、其余 `internal` 三片，三片并集必须等于原 `go test ./... -race -p=1` 的包集合，继续排除并单独运行现有 50ms 探针；
- `integration`：下载同一 dylib，运行 50ms 探针、TCP 重启与 Memory/TCP parity 重复门禁、性能报告测试、微基准及多人微基准；
- 最终轻量汇总 job 继续命名为 `test`，只有全部 macOS 前置 job 成功才成功，保持既有 required check 名称。

`linux-server` 保持独立且保留现有构建、架构、ELF、相邻 so 加载与符号门禁。下游只下载精确包含当前 `${{ github.sha }}` 的 artifact 名称并恢复到 `engine/target/release`；Artifact 缺失、SHA 不匹配或任一分片失败必须 fail closed。

### 7.3 不变量与验收

不得删除测试、降低 `-count`、改变 race 覆盖、放宽性能/正确性阈值或把失败改成 allow-failure。验收检查同一 PR SHA 只有一条 workflow、任一分片失败会让汇总 `test` 失败，并验证 GitHub 的“rerun failed jobs”只需重跑失败分片及其汇总。总墙钟耗时与 runner provenance 只记录，不设受共享 runner 波动影响的时长门禁。

## 8. Change 5：伙伴台词过时结果测试稳定化

### 8.1 已知失败与根因假设

2026-08-23 的 `main` CI run `32619746660` 在 `TestCompanionDialogueStaleOutcomeDiscarded` 报告“过时结果未清除在途标记”，同代码下一次运行通过。现有测试在 `dialogue.releaseRequests()` 后立刻紧密执行十次 `StepForTest`；它没有建立“HTTP 假模型已经返回、worker 已把 outcome 放入 `dialogueResults`”的 happens-before。慢 runner 上 tick goroutine 可以在 worker 得到调度前跑完十次，形成假失败。

### 8.2 修复方式

`stabilize-companion-dialogue-outcome-test` 先用 `-race -count=100` 和受控调度验证假设，再在测试夹具中复用既有有界活性等待，显式等待 outcome 已进入结果 channel，之后推进一个权威 tick 并断言：在途标记清除、过时结果无副作用、请求数不增加。

同一任务必须审计全部 `releaseRequests → 紧密 StepForTest` 的台词测试路径，统一使用这一个完成屏障。禁止增加 sleep、扩大循环次数、抬高生产超时、重试失败模型请求或修改生产 `dialogueInFlight` 状态机。若受控复现证明生产顺序而非夹具同步存在缺陷，必须停止实现、升级 change 设计并重新取得批准，不能用测试等待掩盖产品竞态。

### 8.3 验证

focused 门禁至少包含目标用例和全部同类路径 `-race -count=100`、`go test ./internal/server -race -count=1`，最后执行全仓 race、vet、gofmt、archcheck 与 OpenSpec strict。该 change 无可观察产品行为变化，OpenSpec 使用 `skip_specs: true`，但仍保留 proposal、design、tasks 与 ledger。

## 9. 集成顺序与回退

五路实现可以同时开始。`ci-retry-isolation` 与 `stabilize-companion-dialogue-outcome-test` 评审通过后优先合入，使后三个产品 PR 使用新的稳定门禁；三个产品 PR 按准备完成顺序合入，不设代码依赖。

每个 PR 在终审前从最新 `main` 做正常语义同步；禁止 force-push、整文件 ours/theirs 或放弃一侧语义。任一 change 都必须能用单独 revert 恢复到合入前行为：容器 UI 没有数据迁移，音频失败可静默关闭，CI 可回退旧 workflow，台词修复只动测试；近战的协议 v25 回退必须与对应客户端/服务端 release unit 一起回退，不能跨版本混装。

五项均合入后，按 change 实际完成状态逐个执行 OpenSpec 归档；最终一次核对 `AGENTS.md` 与 `CLAUDE.md` 逐字节一致、`docs/notes/progress.md` 能力描述、协议/ABI/schema/scenario 身份、17 个 capture 场景顺序与全部主规格。

## 10. 批次完成判据

- 五个 change 都有独立可判定的任务、评审证据、ledger、全量验证与可回退提交；
- 五个产品/工程边界没有通过共享事件总线、主题框架或其他预留抽象重新耦合；
- 容器 UI 的四种界面可视觉审查且交互语义不变；
- 玩家近战由服务端唯一权威结算并复用现有伤害/死亡链；
- 四类音效只由合法本地操作或确认状态触发，设备失败不影响游戏；
- 同一 PR SHA 不再重复触发 push 与 pull_request，失败门禁可按分片重跑；
- 伙伴台词目标偶发用例在 race 压测下稳定，且未放宽任何生产超时或正确性门禁。
