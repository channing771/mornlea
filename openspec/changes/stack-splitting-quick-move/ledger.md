# Ledger：完整分堆与快捷搬运（B-35）

> 格式：`Ruling: <决定什么> — <为什么> — <错在哪>`；验证证据按基线 SHA 记录。

## 裁决

- Ruling: sim 侧视图域常量定义在 `sim/contract`（`StackViewInventory/Crafting/Container`，与 `protocol.StackView*` 逐值相同并由 contract 测试钉住）而不是让 entity 生产代码导入 protocol — audit 的 entity 依赖允许表不含 `shared/network`，生产依赖方向不得为省三个常量破例 — 若 Task 5 ingress 翻译时发现两套常量漂移，以 contract 测试 `TestStackSplitViewConstantsMirrorProtocol` 的失败为拦截点。
- Ruling: 部分移动的数量推导点收敛在 `entity.stackSplitAmount`（结算时点权威来源栈：单件 1、半组 ceil），三个视图域共用；crafting 整堆入口以 `amount=source.Count` 复用同一泛化函数，既有整堆路径按位不变 — 数量纪律要求唯一推导点，客户端无从表达任意数量 — 若未来在 wire/命令上加数量字段即违反本裁决。
- Ruling: `mergeStacks` 保持整堆交换语义不动，部分移动走伴生 `mergeStacksAmount`（异类非空目标拒绝而非交换）— 整堆交换是既有箱子/熔炉行为契约，泛化单函数会让空源边界行为漂移（`mergeStacks` 对空源返回 true）— 若合并两者必须先证明既有整堆调用点逐位不变。

## 任务组验证证据

- Task 3（提交 `c9fc5da3`，基线 `d5d2650b`）：`go test ./packages/server/sim/... -race -count=1` 全绿（contract 1.686s、entity 7.339s、realm 8.573s、runtime 21.681s）；`go vet ./packages/server/...`、`gofmt -l` 干净。audit 基线红（协议 v44 与 AGENTS.md/config.yaml v43 滞后）为 Task 7 计划内状态，未新增失败项。

