# Cream Game Experience Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Follow TDD; controller supplies an independently reviewed task brief.

**Goal:** 将游戏面板全部迁到前端奶油 UI，支持鼠标操作并统一物品、人物与掉落视觉。
**Architecture:** React 绘制和命中；Go 镜像组装和权威请求；Rust 接管窗口响应链与 GPU 世界。资产与世界变换保持有界。
**Tech Stack:** Go 1.26、Rust 1.97.1/wgpu、React/TypeScript/Vite/pnpm。
**Spec:** `openspec/changes/cream-game-experience/design.md` 与同 change 的 delta specs。

## Global Constraints

- 所有面板类 MUST 由前端呈现，禁止生产 GPU 面板 fallback。
- 服务端是唯一权威；前端不得预测库存、计算合成结果或直接修改镜像；Memory/TCP 复用既有命令。
- 网络协议 v35、存档 schema、engine ABI v10、client ABI v15 的 C 布局与现有固定 GPU 容量不变；进程内 JSON 桥可以同步扩展。
- 不加入未经授权的素材、不增加依赖；前端零网络/零持久化，样式取 tokens.css。
- GPU 稳定态零动态资源，图像生成/编码只在装配阶段缓存；未知值与真实 overflow 必须明确拒绝。
- 不启动或聚焦前台游戏窗口；验证只用无头工具。保留根工作区用户已有 grass-closeup-scene。
- 所有代码注释中文，Go 标识符用反引号，无任务编号；提交为单行英文 conventional commit，不推送、不合并。

### Task 1: 前端面板、严格语义桥与鼠标完整闭环

**Files/ownership:** `packages/engine/crates/mornlea_client/frontend/src/bridge/`, `src/hud/`, `src/ui/`, `src/tokens.css`, frontend visual fixtures/dist；Rust `src/overlay.rs`, `src/webview.rs`, `src/window.rs` 及相关桥测试；`packages/client/client/ui_bridge.go`, `ui_hud_state.go`, `ui_hud_push*.go` 与测试；`packages/client/cmd/mornlea/app/app_ui_state.go`, `interactive.go`, `app_input.go`, `app_render.go` 与输入/状态测试；生产 GPU 面板调用相关 capture/tests。按职责拆小文件，不把新增全部塞入 interactive.go。适用 AGENTS 与本计划 Global Constraints 均约束本任务。

**Interfaces:** 下行扩展 UIHudState（可新增独立 UI 游戏状态文件），带明确视图 kind、会话/视图 token、来源、玩家 36 格、合成 2/3 网格与产物、箱子 27 格、熔炉 3 格和权威进度、十条既有配方。slot 添加可选 name/icon 图像信息出口，供后续物品素材任务接线。上行新增严格 game action 类型，携带操作与合法索引/视图 token；不要用字符串拼接任意 action id，更不能把语义索引反算成坐标调用旧命中。所有传入值逐层 schema/TS/Rust/Go 验证，过期面板事件不得改变新面板来源。可使用现有 typed JSON ABI，不新增 C 出口。

**Deliverable:** 真实生产可操作的前端个人背包/合成、工作台、箱子、熔炉、只读人物信息与 tooltip。奶油卡面、可可轮廓和文字，槽位与 HUD 同族圆角阴影；个人页可有背包/人物切换，人物页为原创纸面体素肖像及已有生命/饥饿读数，不造装备系统。配方保持原有选择语义；合成产物只发送 TakeCraftingOutput。数量/耐久/进度等待权威确认，第一次点击来源只本地记录高亮，第二次发对应命令。槽位用 button/aria，tooltip 不吞指针，窄视口可访问全部网格和关闭按钮。菜单/设置/暂停/调试沿同一 tokens 奶油外观。

**Input:** 先核实原有释放入口；若无合适入口，用 Tab 显示自由光标（前端显示简短按键提示），再次 Tab/点击世界背景恢复捕获。自由光标可点击 HUD，捕获时 HUD 完全穿透。面板 E/Esc/关闭按钮与数字键正确返回 Go；原生键鼠状态在 WebView 接管/归还时清理，关闭当帧不挖掘、不跳视角。-connect 交互窗口支持同一前端；capture/benchmark 无 WebView。面板本地变化及时下行，权威值仍按 tick 合并。复用现有请求方法并删除失去生产消费的 GPU 命中/渲染接线；暂时保留固定 ABI 兼容编解码不是 fallback。

**HUD motion:** 基础槽位命中不动，内部视觉层选中上移 5 design px/scale 1.08，左右邻格上移 2 design px/scale 1.03，180ms ease-out，全部经 tokens；减少动态效果时零过渡，边缘不裁切。不乐观改变选中状态。真实图标由 Task 2 接入，本任务保留明确的 slot icon 出口。

**Steps:**
- [ ] 先写并执行失败测试：合法/越界/额外字段/过期 token 事件；各面板 DOM 点击产生正确语义事件；Go 两次点击只一次命令且镜像不变；输出格拒绝非法目标；未确认、暂停和关闭后不发送；面板/自由光标参与转换与关闭按键；HUD 边缘槽点击和 reduce-motion；窄窗口 fixture。
- [ ] 先 schema 后 Go/TS/Rust 同步，实现上述闭环。全新组件保持关注点分离，生产只走前端面板，GPU capture 的面板断言转为无 GPU 面板实例，前端 fixture 承接外观。
- [ ] 运行 `corepack pnpm install --frozen-lockfile`、`corepack pnpm typecheck`、`corepack pnpm test`、`corepack pnpm build`（frontend cwd）；`cd packages/engine && CARGO_TARGET_DIR=target/cargo cargo test -p mornlea_client --locked`；`make rust`；`go test ./packages/client/client ./packages/client/cmd/mornlea/app -race -count=1`；`go test ./packages/audit -count=1`。失败定位后修复，记录 red/green 输出。
- [ ] 对已完成能力更新适用 AGENTS 中冲突的“两态全穿透/前端仅菜单/生产 GPU 面板”描述及 change delta，遵守版本不变。构建 dist 后一起提交。自审并写任务报告，返回新增接口与尚待 Task 2 接线点。

### Task 2: 全类别同源物品图像

**Files/ownership:** `packages/client/assets/` 新增 item_icons.go/tests，atlas 层注册与 cutout；`packages/client/render/drop.go` 材质映射；client/app 的图标目录缓存与桥组装；frontend ItemIcon/slot/recipe 呈现与 schema/守卫/test/dist。资产测试夹具与来源说明。

**Interfaces:** 在 assets 提供 `func (r *Registry) ItemIconRGBA(item core.ItemID) ([]byte, bool)`，返回 16×16 RGBA 的只读缓存；`func ItemIconLayer(item core.ItemID) (uint32, bool)` 返回非方块原创物品图的 atlas layer。所有已注册 ItemID 可取得 icon，未知返回 false。前端消费 Task 1 slot icon 出口；实际 UI 目录只在 registry 装配时缓存编码，消息有界且不重复编码。若单槽 data URI 使现有消息上界不足，改用一次缓存目录/引用而不放宽容量。

**Deliverable:** 可放置方块依据当前材质生成可辨小方块图；非方块使用原创透明底轮廓，具体包含镐/剑/锄（木石铁材质与断裂状态）、煤/粗铁/铁锭、木棍/骨粉、牛肉/熟肉/腐肉、种子/小麦/面包/胡萝卜/马铃薯，其他注册项同样穷举。奶油色调保留材料差异，边缘深浅分层与透明空隙，小图可读。HUD/背包/容器/配方同源；世界非方块薄片使用同图层，不再工具贴成铁块整面。未知图标明确缺失，不使用 generic 方块代替。禁止以同一白方块假装覆盖所有物品。

**Steps:**
- [ ] 先写失败测试穷举 registered IDs、未知 ID、像素非空、alpha 輪廓、相邻材料/完好与破损不相同；UI icon 与 registry 像素同源；最坏面板 JSON 不越桥容量；图像缓存不在每次 HUD tick 编码。
- [ ] 实现原创像素生成与图层、缓存、前端引用，新增原创建图说明。复用现有合法纹理，不下载外部美术。
- [ ] `go test ./packages/client/assets ./packages/client/render ./packages/client/client ./packages/client/cmd/mornlea/app -race -count=1`，frontend typecheck/test/build，若改 Rust 资源清单则 cargo test -p mornlea_client --locked 与 make rust，audit 定点。输出全物品 contact sheet 供控制会话目检，不能只报测试通过。
- [ ] 更新能力变更文档，自审并提交，报告 API 与图像产物路径。

### Task 3: 原创人物细节与水平步态

**Files/ownership:** `packages/client/assets/` 人物图层与测试，`packages/client/render/avatar.go`, `frame_streams.go`, avatar_swing/tests；Rust avatar.wgsl 与 shader 测试。

**Interfaces:** 保持每具 6 件、75 具上限、96-byte 实例与450总容量，不增加身体部件。通过材质与分面 UV 体现细节；为人物专属 material 范围提供明确分面规则，其他已有牛/敌怪/掉落材质分支不得受影响。

**Deliverable:** 玩家/伙伴原创肤色、额发与后脑、双眼/眉鼻提示、奶油内衫与鼠尾草/陶土外衣、衣领/袖口、裤装与深色靴；头部正背可辨，面部仅朝前。保持身体高度与碰撞体。可小幅修正细瘦手臂比例但不得改变权威。步频基于水平距离，建议周期距离 `4*legLength*sin(maxAngle)` 与实际摆幅同源；不是 IK，不宣称物理零滑步。不同种类肢体维持各自合理幅度。每次呈现累计 XZ 差（同位置零增加），同 tick 插值不丢路程；速度更新/静止归零按权威 tick，回退/Reset/超过合理单帧传送阈值归零，死亡/特殊位姿优先。

**Steps:**
- [ ] 失败测试覆盖正背部材质/UV、6件容量、所有 avatar kind、慢快同距同周期、同 tick 分段距离、Y-only 不踏步、停步/回退/传送/重置。
- [ ] 最小实现资产与 shader 分面、动画修正，固定容量、实例编码及 alpha 语义保持。必要 shader 动画验证不启动窗口。
- [ ] `go test ./packages/client/assets ./packages/client/render -race -count=1`；`cd packages/engine && CARGO_TARGET_DIR=target/cargo cargo test -p mornlea_client --locked`；`make rust`；`go test ./packages/audit -count=1`。生成无头角色正背图/运动演示用于人工目检；若沿用 capture 工具先读 visual-baseline skill。
- [ ] 自审，更新对应 delta/design 的最终材质与步幅数值，提交并报告。

### Task 4: 同格稳定散落与有界高密度布局

**Files/ownership:** `packages/client/render/drop_fall.go`, `drop.go`, 新增 focused scatter helper/tests，app/drop 仅必要接线。

**Interfaces:** 消费现有 ItemDrop ID/Block/Item/SupportY/DeathTick 与 Task 2 物品图层；每权威堆仅一实例，DropID/网络/存档不变，固定800容量不变。

**Deliverable:** 同 BlockPos 分组，按完整稳定 ID 排序分配确定槽位，ID 哈希抖动留有边界；单堆也不固定中心。同组1/4/16/32堆在 XZ 方块内呈现，按旋转包围半径保证稀疏不重叠，高密度缩放并分层且漂浮幅度计入层间净空。原支撑下落与死亡渐显保留，中心计算考虑缩放后的底面避免穿地。重排输入同身份同变换，当前集合变更允许重新分配；不得按时钟随机，每帧不得无界排序/搜索或破坏原零分配约束。

**Steps:**
- [ ] 失败测试对1/4/16/32混合块/薄片、多个位置正负坐标、相同ID重排、旋转全部关键相位、浮动与死亡scale-in测保守包围体不重叠、XZ边界和支撑；invalid 与超过800仍真实失败。
- [ ] 实现有界分组与布局，复用 scratch 固定容量；保留首见年龄表和生命周期，不生成额外实例。
- [ ] `go test ./packages/client/render -run 'Drop' -race -count=1`；`go test ./packages/client/cmd/mornlea/app -run 'Drop' -race -count=1`；提交前 `go test ./packages/client/render -race -count=1`。生成无头稀疏/密集混合散落图供目检。
- [ ] 自审并记录最终几何/容量/密度取舍到 design，提交与报告。

### Task 5: 视觉验收、文档同步与全量门禁

**Files/ownership:** 前端 visual fixtures、world/motion capture 适当测试与基线、局部 AGENTS 与 docs 当前说明、必要集成修复，change tasks/ledger 由控制会话勾选。

**Deliverable:** 按 visual-baseline skill 路由所有 UI 面板到 UI，人物/掉落稳定画面到 world，走路与散落过程到 GIF。覆盖空/满背包、人物页、工作台、箱子、熔炉、HUD选中两端、全物品、小窗口。先生成审查产物并告知控制会话路径；控制会话目检通过后才能更新黄金基线。无头无需 WebView 的世界图不重新加入旧 GPU 面板。按用户要求核实所有面板前端一致，旧生产入口不残留。当前文档与 AGENTS 写事实并解释 -connect 现在支持前端面板。

**Steps:**
- [ ] 读 visual-baseline skill 与相关 capture/docs AGENTS。补相称 fixtures，先跑比对，输出差异/预览供控制会话目检，再经显式 update 入口更新认可基线；不得更新无关世界基线或放宽阈值。
- [ ] 执行 gofmt（改动Go文件）、`make frontend-check`（dist先构建并提交，保证字节门禁）、`make rust-check`、`make test-race`、`make dev-check`（六模块vet）、`openspec validate --all --strict --no-interactive`。每个相同基线只跑一次，失败做具体诊断与必要修复后覆盖复测。
- [ ] 将命令、退出码、输出文件与环境限制逐项写报告；未经执行不能声称视觉全绿。自审，提交集成/文档更改，等待控制会话整分支独立审查，不推送、不合并。
