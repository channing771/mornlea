---
doc_id: visual-verification-guide
doc_revision: 2026-09-20.1
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

Godot 试点把身份完整的抓帧写入 `build/visual/godot-pilot/<run-id>/`。`godot-visual-evidence` 生成抓帧，`godot-visual-compare` 生成分类报告。当前 pilot 比较器不会生成差异图或自行调用身份校验，需要单独校验所选运行，并在有意义时补充差异图。两者都不得写 `testdata/visual-golden/`、创建 renderer-specific tracked 类别或放宽阈值。

未来 producer handoff 必须点名既有 UI/world/motion identity、旧/new producer、预期差异、受影响文件、审查证据与回退。只有显式批准并完成人工检查后，才可通过既有更新纪律修改 tracked evidence。

## 历史重归因说明

自然短草上线时曾对 24 景 world 基线做重归因，当时 engine ABI 为 v10。九个场景仅因确定性短草可见而变化，十五个场景逐字节不变，两次干净全量比对复现了已接受结果。这是历史 provenance，不是当前场景数量或 ABI 身份；当前值以代码和视觉索引为准。

## 更新纪律

只有视觉行为有意变化且逐项打开候选图确认后才运行更新。不得仅为清除红灯覆盖基线；必须先检查 actual/diff，再判断实现还是证据错误。未受影响证据保持逐字节一致；阈值只能通过独立的实测和批准契约变更调整。

当前 world/UI 像素基线是本地 GPU/人工审查流程，不进入普通 Go 测试或必需 CI；计划中的生产采集门禁须先验证各支持环境。


## Rust/Godot 迁移证据与当前限制

[目标架构](../architecture-target.zh.md) 将语义正确性与呈现分开：F1/F2 验证协议、存档、回放和权威结果，F3 验证客户端镜像、预测和 typed frame，Godot/Python 验证有界呈现及场景生命周期。PNG/GIF 不能证明服务端权威、存档正确性或音频播放。

当前映射表是 `testdata/godot-pilot/visual-semantics.json`。缺少映射会报告为未覆盖，命令成功不表示完整 parity。使用 `scripts/godot/visual-compare.sh --run-dir <run-directory>` 固定证据目录，避免隐式选择最新运行或触发采集。当前比较器只生成分类报告，不生成差异图，也不会自行调用身份校验；单独执行 `python3 scripts/godot/visual_evidence_contract.py --identity <run-directory>/identity.json --run-dir <run-directory>`，并在有意义时补充差异图。确认环境、输入夹具、相机、视口、资产、就绪条件和输出身份后，才能认定可比较。

当前采集脚本使用 macOS display driver 与 Metal。Godot 的 dummy `--headless` renderer 不能提供真实 GPU 像素；没有合格的非前台采集环境时，应明确记录像素证据不可用，并继续语义及生命周期检查。自动测试不得启动或聚焦前台游戏窗口。

[生产工具规划](../../openspec/changes/godot-production-tooling/proposal.md) 将引入必需场景的严格覆盖检查和经审查的生产者归属，这些门禁尚未由 pilot 命令实现。工具基础设施可先于 feature 交接完成；随后每个 feature 仅交接已审查的 `ui/`、`world/` 或 `motion/` 场景。交接前仍由旧生产者负责。相同生产者的像素回归和跨生产者的语义审查是两类证据，渲染差异需要审查，不能靠放宽阈值通过。

[visual-baseline 技能](../../.codex/skills/visual-baseline/SKILL.md) 及其交接参考文档规定操作流程、元数据和回滚记录。规划更新不改变当前 golden 或生产者归属。当前 world/UI 像素工作流是本地 GPU 与人工审查，不属于普通 Go 测试或必需 CI；未来生产采集门禁必须先验证各支持环境。

`make visual-update SCENES=...` 仍会生成已登记的 passive GIF，`VISUAL_OUT` 不会重定向受版本控制的基线写入。若授权只覆盖更小范围，应在隔离的准确源码快照中运行更新并保留全部采集保护，只发布已审查且被授权的文件，其他基线与任务开始前的哈希逐项核对。
