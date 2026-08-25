# Ledger — hud-atlas-texel-stable-uv

## 认领与批准记录

- 2026-08-25 认领 backlog D-05（farming 遗留 18），分支 `fix/D-05-hud-atlas-texel-uv`，认领提交 `4f89fe1c`（docs-only，main）。
- 阶段 0.5 内容确认：分类 bounded；短设计（对称亚纹素收进 δ=1/256、签名兼容、golden 零触碰 + 差异升级路径）经用户在控制会话内显式 **Approve**（2026-08-25）。无澄清轮——范围由 farming design 遗留清单第 18 条钉死。

## 执行进度

- 2026-08-25 Task 1.1 + 1.2 + 2.1（implementer 子代理 ses_fc73519e4ffePPmQpkM7xDOWTz）：RED 由 `TestHotbarColumnUVKeepsMarginFromColumnBoundaries` 承载（精确边界下解码界距列边界为 0）；GREEN 后包内 `-race` 全绿、gofmt/vet 干净。偏离记录：①性质测试「探针集合跨宽度一致」与「相邻列不重叠」在 RED 阶段即通过（探针取纹素中点距边界恒 0.5 纹素，重归一化噪声差三个数量级无法翻转；相邻列共享边界的除法表达式逐位相同）——实现者未扭曲测试制造假失败，两条测试保留为 GREEN 态守护；②gofmt 补一处既有缺失空行（格式合规）；③Task 2.1 注释初版引用了 design.md D1 的错误数字，经 R1/R2 修复（见 Rulings）。
- 2026-08-25 Task 3.1（视觉基线对比，golden 裁决）：本机（Apple Silicon，当前开发机）先在 main 基线 `4f89fe1c` 跑 `make visual-check`——**main 本身即有 13 景超阈值**（固有机器 vs golden 生成机偏差，如 10px@(423,123)、oak-grove 56px），证明该门禁在本机对 main 本就不绿。本分支与 main 的**净差异**（同机实拍逐像素比对）恰好只有两景：`hud-hotbar-health`/`hud-survival-feedback` 各 +81px，全部位于心形图标区（x[113,174], y[300,323]）——收进使距内部纹素边界 <1/256 纹素的采样翻到相邻纹素，即设计预测的「贴边界样本」；这批样本正是下次扩列时本来就会翻转的同一批（farming 0.115% 的同类）。其余 16 景净差异为 0。按 design D3 升级裁决，用户显式批准「外科手术式再生两张 golden」：仅从本分支实拍拷贝两张 PNG 进 `testdata/golden/`，不触碰其余 16 景。再生后两景在本机 `make visual-check` 差异归零；每张 golden 相对旧基线的 91px 变更 = 81px 收进翻转 + 10px 本机既有偏差（后者随本机实拍固化，与其余 16 景的偏差同类）。CI 不运行视觉门禁，无跨机器门禁风险。
- 2026-08-25 双评审（SPEC ses_fc72251b1ffeG2v8NdIp22nsb8 FAIL→清偿；QUALITY ses_fc7221ddeffeW1lWUEc737G3dl PASS 附 Important→清偿；QUALITY 复核 ses_fc70742bcffe1eTXRom5O8rs67 抓出 archcheck 门禁误报→R4 清偿）。修复轮 R1–R4 全部由原 implementer 承载（未超 5 轮上限）。评审实证矩阵：删收进/缩收进由 margin 测试杀死；单侧收进由 margin 测试杀死；对称外扩由 overlap 测试独立杀死；收进随宽度缩放由新增上界断言（<1/64）杀死；错列/重排/列外解码由探针集合测试杀死。
- 待办：Task 4.1/4.2（收尾门禁）、归档。

## Rulings

- Ruling: design.md D1 噪声模型算术修订（适用域 `W < 2^16` → `W ≤ 2^15` 纹素，删去错误的「≤1/2048、>8×」表述） — implementer 在 RED 复核时发现 `W·2^-24` 在 `W = 2^16` 处等于 δ 本身，初稿把保守上界与半 ulp 口径混用且域边界算错 — 错在控制会话起草 design.md 时的算术，不在实现；规格阈值（1/512 裕度）与方案不变，仅修正推导与适用域。修复轮 R1 同步代码注释，R2 把「>160×」修为保守口径的「>80×」（半 ulp 口径约 164×）。
- Ruling: 再生 `hud-hotbar-health.png` 与 `hud-survival-feedback.png` 两张 golden（用户显式批准，2026-08-25） — 收进使距内部纹素边界 <1/256 纹素的采样确定性翻转（两景各 81px，全部位于心形图标区），数学上「零像素变化 + 永久免疫」不可兼得，而这批样本正是下次扩列时本来就会翻转的同一批；按 design D3 停下呈报后获批，外科手术式只再生受影响的两张、其余 16 景零触碰 — 错不在实现（行为即设计目标本身），需裁决的是一次性 golden 成本的承担方式；否决的替代：δ 缩为 1/1024（仍剩 ~20px 不归零且适用域收缩）、放弃收进（无稳定性收益）。
- Ruling: 测试注释不引用闭包标识符 `probeSet`（R4） — R3 按反引号纪律给闭包名加反引号后，`internal/archcheck` 注释标识符门禁误报（该门禁只索引顶层声明，函数体内 `:=` 短声明不进索引） — 错在控制会话的 R3 指令未预见门禁索引范围；裁决采纳评审推荐：改写注释不提标识符，不改动门禁本身（扩展 `collectDeclaredNames` 收录 `token.DEFINE` 属门禁变更，须独立 change）。
