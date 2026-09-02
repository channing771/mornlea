## A. 前置 Spike 与钉值（任一判据不达成即暂停并回 propose）

- [x] A.1 Spike S1（hitTest 分级）：WKWebView 子类 `hitTest:` 返回 nil 的穿透可行性验证，产出最小可运行验证（Rust 侧临时实现 + 手动输入序列清单），结论与判据结果写入本 change design.md D7。判据：菜单/游戏两态切换正确、GameOverlay 态输入行为与无 WebView 基线逐项一致。
- [x] A.2 Spike S2（合成开销）：GameOverlay 常驻合成的帧开销/功耗实测（含空载与持续动画两组），数据写入 D7。判据：交互帧率不低于无 WebView 基线 −5%。
- [x] A.3 容器保留面资源重算：解析退役常显层后的容器面板/tooltip 最坏组合 quad/glyph 数（关闭态与打开态、含 marker），数值钉入本 change `specs/survival-hud-presentation/spec.md` 的「容器保留面 GPU 资源契约重钉」requirement、design.md D4 与对应测试描述。验证：`openspec validate --all --strict --no-interactive` 通过。

## B. 桥与参与模式

- [x] B.1 桥 schema 游戏相位状态族：`frontend/src/bridge/schema.json` 单源演进（hud 对象：hotbar 镜像+选中、health/hunger/oxygen、mining/eating、popup、chat 缓冲、marker、crosshair、containerOpen），TS 类型与守卫同步，`schema.test.ts`/`client.test.ts` 钉值扩展（合法/非法/未知类型用例），Go 侧组装结构体与钉值测试。验证：`corepack pnpm test`、`go test ./internal/client -race -count=1` 全绿。
- [x] B.2 Rust 两态参与模式：WKWebView 子类化 hitTest 分级 + 相位驱动切换（Menu/GameOverlay）+ GameOverlay 态下行渲染接线；rust 侧钉值/行为测试（两态命中行为、无头路径零参与回归）。验证：`cd engine && cargo test -p mornlea_client --locked` 全绿。
- [x] B.3 Go 推送纪律：每权威 tick 合并脏标记、无变化零推送、marker 计时状态机保持（成功呈现帧计数、失败不消耗、生命周期 reset），推送事件单测覆盖。验证：`go test ./internal/client ./cmd/mornlea/app -race -count=1` 全绿。

## C. 前端 HUD 组件族

- [x] C.1 组件实现：`frontend/src/hud/` 组件树（Hotbar/StatusRow/ProgressTrack/ItemPopup/Crosshair/ChatLog），按 `prototype.html` 布局与 `tokens.css` 令牌；`--hud-scale` 单一比例缩放与构图语义（生命左缘/饥饿右缘/氧气外堆叠/容器打开态避让）；颜色无关可辨性（选中双层轮廓、采掘形状差异）；零网络、零本地存储纪律延续。验证：`corepack pnpm typecheck && corepack pnpm test` 全绿。
- [x] C.2 组件断言与视觉基线：vitest 断言覆盖权威驱动/未确认隐藏/构图关系/形状差异/缩放协调；`frontend/visual` fixtures 扩容 HUD 部件并生成基线（update 入口 + 控制会话多模态自检 + 用户终审）。验证：visual-check 退出码 0。

## D. Go 侧常显层退役

- [x] D.1 常显层绘制退役与状态组装：`internal/render/hud` 移除常显层绘制路径（保留容器面板/tooltip/atlas），HUD 状态按 B.1 schema 组装下行；聊天输入与行缓冲状态机保留。验证：`go test ./internal/render/hud ./internal/client -race -count=1` 全绿。
- [x] D.2 保留面钉值落地：按 A.3 数值更新固定资源断言（`layout_test.go` 等的关闭/打开最坏组合钉值）；移除常显层 quads 的容量消费面。验证：`go test ./internal/render/hud -race -count=1` 全绿。
- [x] D.3 输入与命中回归：游戏输入序列（采掘/放置/快捷栏/聊天/Esc/容器开关）行为回归测试，GameOverlay 不产生上行事件断言。验证：`go test ./cmd/mornlea/app -race -count=1`、`go test ./internal/archcheck -count=1` 全绿。

## E. 验收链重构

- [x] E.1 capture 场景清单修订：退役 hud-hotbar-health/hud-survival-feedback/hud-item-name-popup 三景（或改造为容器保留面场景，按 D4 实际保留面定），更新 `captureScenes` 表与 `visual-verification` 场景断言；世界类 golden 显式更新（HUD 条带消失属合法波及）；far-horizon 倒数第二与 water-underwater 末场景约束保持。验证：compare 全部场景通过、`go test ./cmd/mornlea/capture -count=1` 全绿。
- [x] E.2 benchmark scenario 演进：常显层退役后的 scenario 数值按版本纪律递增钉值（v20 → 新版本），benchmark 观察路径零 WebView 参与断言。验证：`make test-multiplayer`（如涉及）与 `go test ./cmd/mornlea/benchmark -race -count=1` 全绿。

## F. 收尾门禁与文档

- [x] F.1 全量门禁：`gofmt`、`go vet ./...`、`make rust`、`make rust-check`、`make frontend-check`、`make dev-check`、`openspec validate --all --strict --no-interactive`。验证：全部退出码 0。
- [x] F.2 文档：`docs/notes/progress.md` 条目、`engine/crates/mornlea_client/AGENTS.md`（GameOverlay 模式与所有权 + `hud-*` 类名前缀白名单）、`cmd/mornlea/AGENTS.md`（场景清单 25→22）、根 AGENTS.md 版本矩阵（scenario v21，E.2 已改）更新；`docs/notes/visual-verification.md`（25 景旧清单与三个退役场景名，E.1 评审指出的矛盾文档）；README.md/README.en.md 与 `docs/notes/perf-baseline.md`/`compatibility.md`/`perf-baseline-m5.md` 的 benchmark v20→v21 表述（E.2 移交）；change ledger 记录 spike 数据、评审裁决与用户终审结论。验证：`openspec validate --all --strict --no-interactive` 通过。
