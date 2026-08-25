# Ledger — hud-atlas-texel-stable-uv

## 认领与批准记录

- 2026-08-25 认领 backlog D-05（farming 遗留 18），分支 `fix/D-05-hud-atlas-texel-uv`，认领提交 `4f89fe1c`（docs-only，main）。
- 阶段 0.5 内容确认：分类 bounded；短设计（对称亚纹素收进 δ=1/256、签名兼容、golden 零触碰 + 差异升级路径）经用户在控制会话内显式 **Approve**（2026-08-25）。无澄清轮——范围由 farming design 遗留清单第 18 条钉死。

## 执行进度

- 2026-08-25 Task 1.1 + 1.2 + 2.1（implementer 子代理 ses_fc73519e4ffePPmQpkM7xDOWTz）：RED 由 `TestHotbarColumnUVKeepsMarginFromColumnBoundaries` 承载（精确边界下解码界距列边界为 0）；GREEN 后包内 `-race` 全绿、gofmt/vet 干净。偏离记录：①性质测试「探针集合跨宽度一致」与「相邻列不重叠」在 RED 阶段即通过（探针取纹素中点距边界恒 0.5 纹素，重归一化噪声差三个数量级无法翻转；相邻列共享边界的除法表达式逐位相同）——实现者未扭曲测试制造假失败，两条测试保留为 GREEN 态守护；②gofmt 补一处既有缺失空行（格式合规）；③Task 2.1 注释初版引用了 design.md D1 的错误数字，经 R1/R2 修复（见 Rulings）。
- 待办：Task 3.1（本地 capture 对比）、Task 4.1/4.2（收尾门禁）、双评审。

## Rulings

- Ruling: design.md D1 噪声模型算术修订（适用域 `W < 2^16` → `W ≤ 2^15` 纹素，删去错误的「≤1/2048、>8×」表述） — implementer 在 RED 复核时发现 `W·2^-24` 在 `W = 2^16` 处等于 δ 本身，初稿把保守上界与半 ulp 口径混用且域边界算错 — 错在控制会话起草 design.md 时的算术，不在实现；规格阈值（1/512 裕度）与方案不变，仅修正推导与适用域。修复轮 R1 同步代码注释，R2 把「>160×」修为保守口径的「>80×」（半 ulp 口径才是 ~167×）。
