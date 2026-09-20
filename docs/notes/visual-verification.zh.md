---
doc_id: visual-verification-guide
doc_revision: 2026-09-15.1
language: zh-CN
counterpart: visual-verification.md
---

# 视觉验证

Mornlea 使用按可观察语义分类的视觉证据，使渲染错误能够被稳定审查。权威行为契约位于 `openspec/specs/visual-verification/spec.md`，完整的当前注册表见[视觉证据索引](../../testdata/visual-golden/README.zh.md)。

## 证据类别与当前 producer

- `world/` 包含 31 张无头世界稳定帧 PNG；当前场景注册表及顺序由 `packages/client/cmd/mornlea/capture/capture.go` 的 `captureScenes` 管理。
- `ui/` 包含 31 张窗口/UI 部件 PNG；当前 fixture 注册表由 `packages/engine/crates/mornlea_client/frontend/visual/fixture-names.ts` 的 `fixtureNames` 管理。
- `motion/` 包含 11 个有界跨 tick GIF，完整呈现触发前、结算和收敛过程，只供人工审查，不参与自动像素比对。

视觉证据按可观察语义而不是 renderer 身份路由。世界帧不包含窗口或 WebView chrome，UI fixture 不复刻世界像素，motion 证据也不拆成孤立静态帧替代全过程。当前 Rust/WebView producer 在具体 feature handoff 获批前保持权威。

## 世界抓帧与比对

`--capture <directory>` 使用无头 offscreen 路径，不创建或聚焦前台游戏窗口。每个世界场景使用确定性状态、固定相机、内嵌默认材质和显式 settled 点。capture 与 benchmark/远程 connect 模式互斥。

世界比对使用 `packages/client/cmd/mornlea/capture/visual_compare.go` 实现的两项指标：单像素最大通道差与差异像素占比，两项都必须落在代码拥有的限制内。本文不复制阈值数值。基线缺失时必须失败，除非调用方明确请求更新。

```bash
make visual-check
VISUAL_OUT=/tmp/shots make visual-check
make visual-update
```

比对失败时，输出目录生成 `<scene>-actual.png` 与 `<scene>-diff.png`，差异图突出变化像素供人工定位。

## 定点运行与阶段边界

`--capture-scenes`（Make 变量 `SCENES=`）选择非空子集，并保持权威完整清单顺序。未知、重复或空名称在抓帧前失败。定点检查用于加速编辑循环，不能替代阶段边界的完整 `make visual-check`。

```bash
make visual-check SCENES=mining-crack-early,mining-crack-heavy
make visual-update SCENES=mining-crack-early,mining-crack-heavy
```

子集更新在写任何 world 基线前仍执行现有 LOD on/off 近环 control。`GIFS=1` 可在比对运行中请求 motion 生成；显式视觉更新会生成已注册 motion GIF。GIF 仍只供人工审查，不进入自动比较。

## Godot 试点证据

Godot 试点把身份完整的抓帧写入 `build/visual/godot-pilot/<run-id>/`。`godot-visual-evidence` 生成抓帧，`godot-visual-compare` 生成比对与差异报告。两者都不得写 `testdata/visual-golden/`、创建 renderer-specific tracked 类别或放宽阈值。

未来 producer handoff 必须点名既有 UI/world/motion identity、旧/new producer、预期差异、受影响文件、审查证据与回退。只有显式批准并完成人工检查后，才可通过既有更新纪律修改 tracked evidence。

## 历史重归因说明

自然短草上线时曾对 24 景 world 基线做重归因，当时 engine ABI 为 v10。九个场景仅因确定性短草可见而变化，十五个场景逐字节不变，两次干净全量比对复现了已接受结果。这是历史 provenance，不是当前场景数量或 ABI 身份；当前值以代码和视觉索引为准。

## 更新纪律

只有视觉行为有意变化且逐项打开候选图确认后才运行更新。不得仅为清除红灯覆盖基线；必须先检查 actual/diff，再判断实现还是证据错误。未受影响证据保持逐字节一致；阈值只能通过独立的实测和批准契约变更调整。

视觉抓帧是本地 GPU/人工审查流程，不进入普通 Go 测试或 CI。
