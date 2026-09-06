# selective-visual-capture

## Why

`make visual-check` 是视觉回归的本地门禁，但每次运行都无差别重新生成全部产物：一次完整世界加载（视距 32，约 4489 个区块列）+ 24 个场景各 40+ GPU 帧的预热与收敛 + 4 条 GIF 剧本（约 156 帧逐帧渲染回读），而 GIF 比对已退役、仅供人工审查，每次纯比对运行都重新生成纯属浪费。编辑环里的改动往往只波及少数场景（例如采掘裂纹 overlay 只影响 `mining-crack-early/heavy` 两景），全量重生成既拖慢迭代又消耗机器性能。

瓶颈核实结论：比对只是一次 640×360（230,400 像素）的 Go 单循环加一次 PNG 解码，毫秒级；真正耗时在 GPU 帧数、世界加载与 GIF 生成，全部与实现语言无关（渲染已在 Rust/wgpu）。因此本 change 不换语言、不重写管线，只引入选择性执行。

## What Changes

- 抓帧 CLI 新增 `--capture-scenes <逗号分隔场景名>`：显式选择场景子集，按场景表固有顺序**保序过滤**执行；未知、重复或空场景名直接拒绝启动；缺省不传时跑全部 24 景，既有行为逐项不变。
- 纯比对（check）模式默认不再生成 GIF 剧本，新增 `--capture-gifs` 显式请求生成到输出目录供人工审查；更新（`--update-golden`）模式的 GIF 生成行为不变。
- `Makefile` 的 `visual-check`/`visual-update` 透传 `SCENES=`（以及 check 侧的 `GIFS=1`）变量。
- update 路径的 LOD on/off 近环 control 门禁原样保留（用户裁决）：子集更新仍先过 control，再用 fresh application 按子集保序执行正式抓帧。
- 同步消除规格漂移：主规格 GIF 条款仍描述"逐帧比对裁决"，与代码现状（只生成不进比对）不符，按现状钉死为"仅供人工审查"。
- 非目标：不改双阈值、不改场景清单与顺序、不引入内容哈希缓存、不做 git diff 自动场景映射、不动 `frontend-visual-*`（Chrome UI 部件基线）、不更换实现语言。

## Capabilities

### Modified Capabilities

- `visual-verification`：抓帧模式的显式场景子集、GIF 生成时机门控、子集路径下的 update 近环 control 语义。

## Impact

- 代码：`packages/client/cmd/mornlea/capture`（`RunOptions`、场景名校验与保序过滤、GIF 门控、`RunCapture` 签名）、`packages/client/cmd/mornlea`（flag 解析与接线）、`Makefile`（变量透传）。
- 文档：`openspec/specs/visual-verification/spec.md`（delta）、`docs/notes/visual-verification.md`（子集使用纪律）、`.claude/skills/visual-baseline/SKILL.md`（路由说明）。
- 兼容性：缺省命令行为与产物逐字节不变；无存档、协议、engine/client ABI、benchmark scenario 影响；golden 基线文件不动。
- 风险：场景共享同一 application 且存在跨场景呈现状态残留（先例：GIF runner 对 `water-underwater` 留下的预测器浸没标志做无条件干眼重钉），子集运行的前置状态与全量不同，理论上可能引入像素漂移。以真机等价性验证兜底（全量与子集对同一 golden、同一双阈值都必须通过）；若发现某场景子集不安全，在过滤层显式处理并记入 ledger。
