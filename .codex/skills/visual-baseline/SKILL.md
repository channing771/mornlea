---
name: visual-baseline
description: 视觉基线三类路由与更新纪律：窗口型归 ui、单帧稳定态归 world、跨 tick 过程归 GIF（演示/门禁分子类）。新增或更新基线前先用本 skill 确定落点与门禁。
---

视觉基线统一住 `testdata/visual-golden/`，分类依据见该目录 `README.md`“三类边界与选用规则”（权威，不在本文件复制清单与阈值）。

## 三类路由

- 窗口/WebView 层 → `ui/`：注册表以 `fixture-names.ts` 的 `fixtureNames` 为准，本机 Chrome 抓取比对，不进 CI。
- 无头世界单帧稳定态 → `world/`：注册表以 `capture/capture.go` 的 `captureScenes` 为准，无头离屏收敛后抓帧。
- 跨 tick 状态迁移 → GIF 全流程（触发前、结算、收敛全覆盖，不得只截片段）：
  - 供人眼审查 → `motion/` 演示，不进任何比对；
  - 需门禁钉住 → `passive-death/`（或同类门禁 GIF 目录），逐帧比对、全帧通过、帧预算有界。

世界帧不得携带窗口 chrome，UI 夹具不得复刻世界像素；同一行为禁 PNG + GIF 双存，例外必须在 README 注明理由（门禁采样点 vs 全流程审查物）。

## 入口

- 世界：`make visual-check` 比对，`make visual-update` 显式覆盖，路径常量以 `capture/capture_image.go` 为准。
- 部件：`make frontend-visual-check` / `make frontend-visual-update`（或在 `frontend/` 内 `corepack pnpm visual-check` / `visual-update`），目录推导以 `visual/visual.mjs` 为准。
- 演示 GIF：仓库根运行 `--motion-demo` 独立入口，不碰场景表与 PNG 基线。

## 更新纪律

先目检后覆盖：预期视觉变化已逐图人工确认后才更新；普通验证只比较不自动接受；漂移先看实拍图与差异图定位，再决定修代码还是更新基线；基线缺失不静默创建，必须显式请求更新。比对口径与阈值以源码比对函数为准，本文档与 skill 不复制数值。
