# 伙伴台词结果测试稳定化实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复 `TestCompanionDialogueStaleOutcomeDiscarded` 及两个同源测试在模型放行后依赖 goroutine 调度的偶发失败，让测试等待真实的结果入队事实，再用一个权威 tick 验证应用或丢弃。

**Architecture:** 生产代码不变。测试复用现有有界轮询 `waitIntegrationCondition`，观察 `companionManager.dialogueResults` 的 channel 长度；手动 tick 测试没有并发消费者，因此“队列已有结果”是推进下一 tick 的稳定前置条件。只替换三个 `releaseRequests` 后紧跟固定 tick 循环的测试段，其余清理型释放保持原样。

**Tech Stack:** Go 1.26、`testing`、现有 `httptest` 假模型与 server 集成测试 helper。

**Spec:** `docs/superpowers/specs/2026-08-22-five-way-parallel-wave-design.md` §8、§3、§9。

## Global Constraints

- [ ] 从共享 main 创建 `codex/stabilize-companion-dialogue-outcome-test` 独立 worktree/change；不夹带其他四项改动。
- [ ] change 使用 `skip_specs: true`，但保留 proposal/design/tasks/ledger；这是测试同步修复，不改变可观察产品行为。
- [ ] 不增加 sleep、固定 tick 次数或全局 timeout，不添加生产重试/生产测试钩子，不修改 `internal/server/companion_dialogue.go`。
- [ ] 若结果已确认入队后单 tick 仍出现错误副作用、未清在途或世代判断错误，立即停止测试侧修复，记录证据并把问题升级为生产缺陷重新设计。
- [ ] 每个 task 全新 implementer、独立 SPEC/QUALITY reviewer、最多 5 轮追加修复，证据写 ledger。

---

## Task 1: 固化根因证据与 test-only OpenSpec 边界

**Files:**
- Create: `openspec/changes/stabilize-companion-dialogue-outcome-test/.openspec.yaml`
- Create: `openspec/changes/stabilize-companion-dialogue-outcome-test/proposal.md`
- Create: `openspec/changes/stabilize-companion-dialogue-outcome-test/design.md`
- Create: `openspec/changes/stabilize-companion-dialogue-outcome-test/tasks.md`
- Create: `openspec/changes/stabilize-companion-dialogue-outcome-test/ledger.md`

- [ ] 在 ledger 记录已知 RED：CI run `32619746660` 的 main 失败点是 `TestCompanionDialogueStaleOutcomeDiscarded`，下一次相同代码通过；保存失败断言与运行环境，不把偶发失败伪装成稳定复现。
- [ ] 运行修改前压力测试并原样记录结果；失败即补充频次，未复现也保留“未复现”事实：

```bash
make rust
GOMAXPROCS=1 go test ./internal/server \
  -run '^(TestCompanionDialogueOneInFlightPerCompanion|TestCompanionDialogueStaleOutcomeDiscarded|TestCompanionDialogueGenerationBumpDiscardsOutcome)$' \
  -race -count=100
```

- [ ] 在 design 画出已存在的顺序：`releaseRequests` 只关闭 HTTP handler 的阻塞 channel；之后仍需 handler 写响应、HTTP client 读取/解析、`dialogueWorker` 发送 `dialogueResults`。证明 close 与最终发送之间没有 happens-before，因此立即跑 10/50 个极快 `StepForTest` 不是合法同步。
- [ ] 审计 `internal/server/companion_dialogue_test.go` 全部 `releaseRequests()`：只有 OneInFlight、StaleOutcome、GenerationBump 三处是“放行后立即以固定 tick 猜异步完成”；其余是 planner 推进或 cleanup，不纳入修改。
- [ ] `.openspec.yaml` 使用：

```yaml
schema: spec-driven
created: 2026-08-22
skip_specs: true
```

- [ ] proposal 明确无生产行为、协议、schema、ABI、benchmark/capture 变化；tasks 逐项映射本计划；验证并提交：

```bash
openspec validate stabilize-companion-dialogue-outcome-test --strict --no-interactive
git diff --check
git add openspec/changes/stabilize-companion-dialogue-outcome-test
git commit -m "docs(openspec): plan dialogue outcome test stabilization"
```

## Task 2: 用真实入队条件替换固定 tick 猜测

**Files:**
- Modify: `internal/server/companion_dialogue_test.go`

- [ ] 先保留旧代码跑目标压力命令作为失败测试证据；如果本机没有复现，使用 Task 1 的 CI RED 与 happens-before 分析作为红灯，不加入随机调度或人为 sleep 来制造失败。
- [ ] 紧邻 `waitForDialogueRequests` 添加唯一 helper；直接复用现有轮询，不新建接口、channel 或 timeout：

```go
func waitForDialogueOutcomeQueued(t *testing.T, host *Host) {
	t.Helper()
	manager := host.world.companionManager
	waitIntegrationCondition(t, "台词结果进入 tick 队列", func() bool {
		return len(manager.dialogueResults) > 0
	})
}
```

- [ ] 在 helper 注释说明 `len(channel)` 只用于测试同步：这些用例由当前 goroutine 手动推进 tick，等待期间没有并发结果消费者；观察到非零后，下一个 `StepForTest` 必定能排空该结果。不要把 helper 移进生产代码。
- [ ] 修改 `TestCompanionDialogueOneInFlightPerCompanion`：`dialogue.releaseRequests()` 后等待入队，只执行一次 `StepForTest`/`receiveCompanionChatTick`，随后原样断言 `effects == 1 && !inFlight`；删除 50 tick 与 `applied` 循环。
- [ ] 修改 `TestCompanionDialogueStaleOutcomeDiscarded`：放行后等待入队，只执行一个 tick，再原样断言 effects/inFlight/requests；删除 10 tick 循环。
- [ ] 修改 `TestCompanionDialogueGenerationBumpDiscardsOutcome`：同样等待入队并只执行一个 tick，再原样断言世代不匹配无副作用且清除在途；保留 planner cleanup。
- [ ] 再次 `rg -n 'releaseRequests|for range (10|50)' internal/server/companion_dialogue_test.go`，逐处确认没有遗漏同源猜测，也没有误改“挂起 20 tick 不阻塞”的性能断言。
- [ ] 格式化并运行目标 GREEN：

```bash
gofmt -w internal/server/companion_dialogue_test.go
GOMAXPROCS=1 go test ./internal/server \
  -run '^(TestCompanionDialogueOneInFlightPerCompanion|TestCompanionDialogueStaleOutcomeDiscarded|TestCompanionDialogueGenerationBumpDiscardsOutcome)$' \
  -race -count=100
```

- [ ] 若任何失败发生在“结果已入队”之后，停止并按 Global Constraints 升级；不得靠扩大等待掩盖。
- [ ] 提交 `git add internal/server/companion_dialogue_test.go && git commit -m "test: synchronize companion dialogue outcomes"`，完成双裁决。

## Task 3: 同包回归、共享门禁与整分支终审

**Files:**
- Modify: `openspec/changes/stabilize-companion-dialogue-outcome-test/tasks.md`
- Modify: `openspec/changes/stabilize-companion-dialogue-outcome-test/ledger.md`

- [ ] 运行目标三个测试的普通调度压力，确认 race instrumentation 不是稳定性的前提：

```bash
go test ./internal/server \
  -run '^(TestCompanionDialogueOneInFlightPerCompanion|TestCompanionDialogueStaleOutcomeDiscarded|TestCompanionDialogueGenerationBumpDiscardsOutcome)$' \
  -count=100
```

- [ ] 运行整个 server 包 race 与全仓共享门禁：

```bash
make rust
go test ./internal/server -race -count=1
go test ./internal/archcheck -count=1
go test ./... -race
go vet ./...
gofmt -l .
openspec validate --all --strict --no-interactive
git diff --check
```

- [ ] ledger 记录修改前压力结果、修改后两组 `-count=100`、同包 race 与共享门禁的命令/退出码；不宣称“绝不 flaky”，只证明同步边界已从调度猜测变为入队事实。
- [ ] 生成 `BASE..HEAD` committed review package/SHA-256，独立终审确认：产品 diff 为空、仅三个同源等待段变化、无 sleep/timeout/循环次数增加、OpenSpec `skip_specs` 合理。
- [ ] 修复最多 5 轮，更新 tasks/ledger 后提交；PR 不自行归档。
