# selective-visual-capture ledger

基线 SHA：`76e47f56`（main），分支 `feat/selective-visual-capture`。

## 2026-09-06 立项

- Ruling: 子集选择采用手动 `--capture-scenes`/`SCENES=`，不做 git diff 自动场景映射 — 自动映射需维护"代码→场景"映射表，映射错误会静默漏检；用户裁决本期先手动选择 — 无。
- Ruling: 子集更新基线仍执行 LOD on/off 近环 control — control 是防全局回归被固化进基线的守卫，用户裁决不弱化；子集只省未选场景的渲染时间 — 无。
- Ruling: 不更换实现语言 — 比对为毫秒级像素循环、渲染已在 Rust/wgpu，瓶颈（GPU 帧数、世界加载、GIF 浪费）与语言无关，重写零收益 — 无。
- Ruling: check 模式默认跳过 GIF 生成，新增 `--capture-gifs` 显式开启 — GIF 比对已退役、每次 check 无条件重生成约 156 帧纯属浪费；update 模式生成行为不变 — 无。
- Ruling: 场景名校验放 parse 层并在 capture 层防御性重复 — 对齐 `--motion-scene` 拒未知值的既有风格，capture 层兜底供直接调用方 — 无。
- 验证证据：暂无（实现未开始）。
