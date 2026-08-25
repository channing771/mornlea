# fix-player-flush-stall ledger

规划表行：E-07 存档 Flush 恒脏自旋修复（`docs/feature-backlog.md`）。
分支：`fix/E-07-flush-stall-guard`（基于 main@07617de8）。

## 内容确认（brainstorming）

| 轮次 | 时间（UTC） | 呈现 | 用户回复 |
|---|---|---|---|
| 1 | 2026-08-25T01:52 | 方案 B（精确键 + 上限 4，推荐）vs 方案 A（去掉 revision） | `edit: A` |
| 2 | 2026-08-25T02:16 | 修订版 A′（去掉 revision + retry/fresh 双类名额，附三条钉住测试冲突核对） | `approve`（02:49） |

结论已写入 `proposal.md` 与 `design.md`；批准来源：用户飞书显式 approve。

## Task 执行与评审

- Task 1（1.1–1.3，单一 TDD 闭环合并派发）：implementer 子代理（sonnet）交付 commit 817e98e3；RED 为真实运行时无界重派阻塞（goroutine dump 证据），GREEN 后新旧测试 `-race` 全绿。DONE_WITH_CONCERNS：RED 阶段先加一行哨兵声明使测试可编译——评审裁决可接受（同一允许文件、单 commit、允许改动集合未突破）。
- Task 1 评审（opus，SPEC + QUALITY 双裁决）：Spec ✅（6 Scenario 与全部绑定约束逐条核对，含继承屏障精确键未动、10 条既有测试零改动）；Quality Approved，5 Minor。
- Ruling：Minor 1–3（`playerFlushSlots` GoDoc 计数漂移、`errPlayerFlushStalled` GoDoc 触发条件失真、失速收集确定性理由错置）与 Minor 5（补双玩家「失败 + 失速」混合用例，直钉 spec Scenario「已有失败只报原错误」）进修复轮 1；Minor 4（候选 job 在名额判定前求值的无谓构造）跳过——16 玩家上界下无收益。
- Ruling：spec.md 失速 Requirement 措辞由控制会话收紧到「无 in-flight 且本轮未派发的退出路径」，与 design.md 限定一致（评审 ⚠️ 项）。
- 修复轮 1（commit 30d81bf5）：3 处 GoDoc 精确化（零逻辑变化，diff 逐 hunk 核实只动注释行）+ 新增 `TestPlayerFlushStallOnlyReportsStalledPlayerAlongsideExistingFailure` 双玩家混合用例。范围化复审（sonnet）：4/4 ADDRESSED、无新破坏；注释反引号标识符逐一 grep 存在；两条 stall 测试与全部既有 Flush 测试 `-race` 独立复跑通过。Task 1 完成。

## 整分支终审

- 终审（opus，07617de8..e3d48aca 全量 diff）：**可合入**，无 Critical/Important。独立复跑 gofmt/vet/全包 server race（150.9s ok）/archcheck/OpenSpec strict（62/62）/`cmp AGENTS.md CLAUDE.md` 全绿。
- 关键复核：`Flush` 唯一生产调用点 `host_shutdown.go` 的错误语义为既有钉住行为，本变更只扩大触发集且逐项论证生产不可达或严格更优（恒脏由「挂到超时」变「立即报错」；Flush 内 `TrySubmit` 拒绝只剩 scheduler 已关闭一种情形且 `Shutdown` 在 Flush 前不 `CloseWorker`；Flush 期间无并发 `Observe` 来源）。
- Ruling：终审 Minor 1/2（spec 措辞：dirty 未排除 loading、上界表述未排他）由控制会话直接收紧 spec.md（change 产物）。
- Ruling：终审 Minor 3 parked——失速记录固定用 `persisted + 1` 作 revision，在「冻结 retry + TrySubmit 拒绝」组合下与实际待落盘 job 的 revision 不一致；该组合经终审论证生产不可达，纯错误文本观感，不改。
- Ruling：Minor 4（候选 job 在名额判定前求值）维持跳过——终审 triage 认同：纯函数值拷贝上界 16×32，改动收益低于引入槽位判定顺序错误的风险。

## 最终验证

- gates.sh 首跑：gofmt/vet/archcheck/OpenSpec strict/make rust 通过，全量 race 步骤因 gates 子 shell 无 gvm PATH 报 `go: command not found`（环境问题非测试失败）。
- 带 `~/.gvm/gos/go1.26.0/bin` PATH 重跑：`gofmt -l .` 无输出、`go vet ./...` 通过、`go test ./internal/archcheck -count=1`（45 passed）、`go test ./... -race` 全量 27 包全 ok（`cmd/mornlea` 270.9s、`internal/server` 159.4s，数值只记录）。
- `openspec validate --all --strict --no-interactive`：62/62 通过（gates 首跑）。终审 Minor 1/2 的 spec 措辞收紧后再次 `openspec validate fix-player-flush-stall --strict` 通过。
- 结论：全部门禁通过，按 AGENT_MODE=pr 走 PR + CI 全绿 + merge。
