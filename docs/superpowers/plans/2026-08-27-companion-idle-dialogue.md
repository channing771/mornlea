# C-08 Companion Idle Dialogue Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让完全空闲的伙伴按确定性 60–120 秒机会向仍在线且位于水平 16 格内的最近真实任务发令者说一句 Dialogue 台词。

**Architecture:** 在既有 `companionManager` 单写者 tick 内维护每伙伴下一空闲期限，使用 FNV-1a 64-bit 从伙伴 ID 与旧期限导出下一间隔；期限到达后复用现有 Dialogue worker、四模型槽、单在途和 `CompanionSpeech` 广播。新增 `DialogueNodeIdle`，空闲结果在 tick 边界重验 queue generation、空队列、真实发令者、在线性和距离，不新增协议、存档或调度器。

**Tech Stack:** Go 1.26、标准库 `encoding/binary`/`hash/fnv`、现有 OpenAI-compatible fake endpoint、OpenSpec 1.7、Memory/TCP transport tests。

**Spec:** `docs/superpowers/specs/2026-08-27-companion-idle-dialogue-design.md`

## Global Constraints

- 服务端仍是伙伴任务、期限和台词派发的唯一权威；worker 只处理发送后不可变的请求值。
- “完全空闲”只指 queue 无 current 且 pending 长度为 0；身体、真实发令者、在线性和水平 16 格距离是期限到达时的独立发言资格。
- 首个及后续机会间隔均为闭区间 `1200..2400` tick；机会序列确定，但 HTTP 完成时序与模型输出不承诺确定性。
- Dialogue 与 Planner 继续共享全服 4 个模型槽，每伙伴最多一个 Dialogue 在途；无抢占、排队、补发或重试。
- idle 不读取或递增每任务 8 次 Dialogue 预算，不更新 summary，不改变任务、FIFO、持久化或世界事实。
- `restoredIssuerIdentity` 必须被 server-only 标记为合成身份，永远不能成为空闲台词受众。
- wire、协议 v26、玩家 schema v7、区块 schema v9、metadata v2、`companions.ai` schema v4、engine ABI v7、client ABI v9、benchmark scenario v19 均不变。
- 不触碰 `internal/network`、`internal/storage`、任务/FIFO 实现、Planner、capture/golden 或长期基线文档。
- 注释和 GoDoc 使用中文；标识符与技术术语保留英文；每项行为按 red-green-refactor 完成。
- 每个 Task 由 fresh implementer 执行，随后由独立 SPEC reviewer 和 QUALITY reviewer 双裁决；结论写入 `openspec/changes/companion-idle-dialogue/ledger.md`。

---

## File Structure

**OpenSpec pre-code gate:**

- Create: `openspec/changes/companion-idle-dialogue/proposal.md`
- Create: `openspec/changes/companion-idle-dialogue/specs/companion-dialogue/spec.md`
- Create: `openspec/changes/companion-idle-dialogue/design.md`
- Create: `openspec/changes/companion-idle-dialogue/tasks.md`
- Create: `openspec/changes/companion-idle-dialogue/ledger.md`

**Task 1, Dialogue value contract:**

- Modify: `internal/companion/dialogue_nodes.go`
- Modify: `internal/companion/dialogue_nodes_test.go`
- Modify: `internal/companion/dialogue_client.go`
- Modify: `internal/companion/dialogue_client_test.go`

**Task 2, deterministic opportunity dispatch:**

- Create: `internal/server/companion_idle_dialogue.go`
- Create: `internal/server/companion_idle_dialogue_test.go`
- Modify: `internal/server/companion_manager.go`
- Modify: `internal/server/companion_dialogue.go`

**Task 3, outcome gates and transport parity:**

- Modify: `internal/server/companion_idle_dialogue_test.go`
- Modify: `internal/server/companion_dialogue.go`
- Modify: `internal/server/companion_dialogue_wiring_test.go`

No existing test file is split: `companion_idle_dialogue_test.go` is the single new theme file for scheduler and idle result behavior; existing `companion_dialogue_wiring_test.go` keeps cross-transport Dialogue wiring parity.

## OpenSpec Pre-Code Gate

Before dispatching Task 1, create change `companion-idle-dialogue` with these exact boundaries:

- `proposal.md`: goal is one deterministic, bounded idle speech loop; non-goals are persistence, summaries, task autonomy, movement, protocol/schema/ABI/scenario and UI changes.
- delta `companion-dialogue/spec.md`: add requirements for deterministic 1200..2400 tick opportunities, queue-idle versus dispatch eligibility, real recent issuer/online/16-block gates, restored-issuer exclusion, schedule reset on task arrival, skip-and-reschedule failure semantics, idle nonterminal response, and result-time stale checks.
- `design.md`: reference the approved design and record FNV-1a input order, modular due comparison, tick phase, data ownership, no-preemption tradeoff and rollback.
- `tasks.md`: mirror Tasks 1–3 below, then focused/full verification and archive readiness；spec sync、archive 与 backlog/Discussion completion 由文末集成门禁在全部 checkbox 完成后执行。
- `ledger.md`: record baseline commits `87928315` and `3f91fc31`, baseline commands (`make rust`, companion race, server race), user approvals, and an initially empty Ruling/review table.

Run and require success:

```bash
openspec validate companion-idle-dialogue --strict --no-interactive
openspec validate --all --strict --no-interactive
```

Commit only those five change artifacts:

```bash
git add openspec/changes/companion-idle-dialogue
git commit -m "docs(openspec): propose companion idle dialogue"
```

---

### Task 1: Add The Idle Dialogue Node Contract

**Files:**
- Modify: `internal/companion/dialogue_nodes.go:31-123`
- Modify: `internal/companion/dialogue_nodes_test.go:95-132`
- Modify: `internal/companion/dialogue_client.go:84-99`
- Modify: `internal/companion/dialogue_client_test.go`

**Interfaces:**
- Consumes: existing `DialogueNode`, `DialogueNode.Validate`, `NewDialogueRequest`, `buildDialogueUserPayload`.
- Produces: `companion.DialogueNodeIdle`; `dialogueNodeKindText(DialogueNodeIdle) == "idle"`; idle remains nonterminal because only `DialogueNodeTerminal` sets `terminal=true` in server worker.

- [ ] **Step 1: Extend the validation matrix with failing idle-node cases**

Add these exact rows to `TestDialogueNodeValidateMatrix`:

```go
"空闲节点零载荷":    {DialogueNode{Kind: DialogueNodeIdle}, true},
"空闲节点携带步骤类型": {DialogueNode{Kind: DialogueNodeIdle, StepKind: PlanStepGoTo}, false},
"空闲节点携带终态":   {DialogueNode{Kind: DialogueNodeIdle, State: TaskCompleted}, false},
"空闲节点携带原因":   {DialogueNode{Kind: DialogueNodeIdle, Reason: TaskFailWorldChanged}, false},
```

- [ ] **Step 2: Add a failing payload test**

Append this focused test to `dialogue_client_test.go`:

```go
func TestDialogueClientIdleNodePayload(t *testing.T) {
	req, err := NewDialogueRequest("", "", DialogueNode{Kind: DialogueNodeIdle}, DialogueEnvDigest{})
	if err != nil {
		t.Fatalf("构造空闲台词请求: %v", err)
	}
	payload := buildDialogueUserPayload(req)
	if payload.Node != (dialogueWireNode{Kind: "idle"}) {
		t.Fatalf("空闲节点 payload=%+v，want 仅 kind=idle", payload.Node)
	}
}
```

- [ ] **Step 3: Run the red tests**

Run:

```bash
go test ./internal/companion -run 'TestDialogueNodeValidateMatrix|TestDialogueClientIdleNodePayload' -count=1
```

Expected: build failure because `DialogueNodeIdle` is undefined.

- [ ] **Step 4: Implement the minimum node and mapping**

Append the enum after all existing kinds so existing internal numeric values do not move:

```go
// DialogueNodeIdle 是完全空闲伙伴的一次非任务台词机会，不携带任务载荷。
DialogueNodeIdle
```

Add the zero-payload validation branch:

```go
case DialogueNodeIdle:
	if n.StepKind != 0 || n.State != 0 || n.Reason != TaskFailNone {
		return fmt.Errorf("companion: 空闲台词节点不得携带载荷")
	}
	return nil
```

Add the stable wire text mapping:

```go
case DialogueNodeIdle:
	return "idle"
```

Update nearby GoDoc lists so idle is documented as non-task and zero-payload; do not change the system prompt or response schema.

- [ ] **Step 5: Run focused and package tests**

Run:

```bash
go test ./internal/companion -run 'TestDialogueNodeValidateMatrix|TestDialogueClientIdleNodePayload' -count=1
go test ./internal/companion -race -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit Task 1**

```bash
git add internal/companion/dialogue_nodes.go internal/companion/dialogue_nodes_test.go internal/companion/dialogue_client.go internal/companion/dialogue_client_test.go
git commit -m "feat(companion): add idle dialogue node"
```

---

### Task 2: Dispatch Deterministic Idle Opportunities

**Files:**
- Create: `internal/server/companion_idle_dialogue.go`
- Create: `internal/server/companion_idle_dialogue_test.go`
- Modify: `internal/server/companion_manager.go:46-99,362-379,818-864,1042-1055`
- Modify: `internal/server/companion_dialogue.go:42-107`

**Interfaces:**
- Consumes: Task 1 `companion.DialogueNodeIdle`; existing `orderedIDs`, `currentIssuer`, `followTarget`, `body`, `requestDialogue`, `companion.TicksPerMinute`, `companion.PathWindowHorizontalRadius`.
- Produces:

```go
const idleDialogueMinTicks uint64 = companion.TicksPerMinute
const idleDialogueMaxTicks uint64 = 2 * companion.TicksPerMinute
func idleDialogueInterval(id companion.ID, seed uint64) uint64
func idleDialogueDue(now, deadline uint64) bool
func withinIdleDialogueDistance(from, to [3]float32) bool
func (m *companionManager) idleDialogueAudience(issuer companionTaskIssuer, body companion.Body) bool
func (m *companionManager) dispatchIdleDialogues()
```

`companionTaskIssuer` gains `restored bool`; `companionTaskSlot` gains `idleDialogueAtTick uint64` and `hasIdleDialogueAtTick bool`.

- [ ] **Step 1: Write failing deterministic interval tests**

Create `companion_idle_dialogue_test.go` in package `server` with this exact golden and wrap contract:

```go
func TestIdleDialogueIntervalGoldenAndBounds(t *testing.T) {
	id := companion.ID{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	const seed = uint64(0x0102030405060708)
	if got := idleDialogueInterval(id, seed); got != 1369 {
		t.Fatalf("interval=%d，want FNV-1a golden 1369", got)
	}
	for current := uint64(0); current < 4096; current++ {
		got := idleDialogueInterval(id, current)
		if got < idleDialogueMinTicks || got > idleDialogueMaxTicks {
			t.Fatalf("seed=%d interval=%d 越界", current, got)
		}
	}
}

func TestIdleDialogueDueAcrossTickWrap(t *testing.T) {
	id := chatTestCompanionID(1)
	seed := uint64(math.MaxUint64 - 100)
	interval := idleDialogueInterval(id, seed)
	deadline := seed + interval
	if idleDialogueDue(seed, deadline) || idleDialogueDue(seed+interval-1, deadline) {
		t.Fatal("回绕前提前到期")
	}
	if !idleDialogueDue(seed+interval, deadline) {
		t.Fatal("经过完整间隔后仍未到期")
	}
}
```

- [ ] **Step 2: Write failing audience tests**

Use a manager from `companionManagerHostReady`, override `onlinePlayers` with fixed `PlanPlayer` slices, and table-test these inputs:

```go
cases := []struct {
	name     string
	issuer   companionTaskIssuer
	position [3]float32
	online   bool
	want     bool
}{
	{"正好十六格", liveIssuer, [3]float32{body.Position[0] + 16, body.Position[1], body.Position[2]}, true, true},
	{"超过十六格", liveIssuer, [3]float32{body.Position[0] + 16.01, body.Position[1], body.Position[2]}, true, false},
	{"玩家离线", liveIssuer, body.Position, false, false},
	{"恢复合成身份", restoredIssuerIdentity, body.Position, true, false},
}
```

For online cases, return one `companion.PlanPlayer{ID: tc.issuer.playerID, Position: tc.position}`. Assert `manager.idleDialogueAudience(tc.issuer, body) == tc.want`.

- [ ] **Step 3: Write failing dispatch-state tests**

Use existing `companionManagerHostReady`, `newFakeDialogueModel`, `stopTestIssuer`, `waitForDialogueRequests` and direct slot access under `stepMu`. Cover these exact properties in separate tests:

```go
slot.currentIssuer = stopTestIssuer(integrationIdentity(0x71, "发令者"))
slot.idleDialogueAtTick = manager.engine.TickCount()
slot.hasIdleDialogueAtTick = true
beforeBudget := slot.dialogueRequests
```

- First idle tick with a real issuer and no deadline: no request starts immediately; exact deadline becomes `now + idleDialogueInterval(id, now)`.
- Due and eligible: one request whose recorded `NodeKind` is `idle`; exact next deadline is `oldDeadline + idleDialogueInterval(id, oldDeadline)`; `dialogueRequests == beforeBudget`. Evaluate one case after `now` has advanced past `oldDeadline` to prove recurrence stays anchored to the old deadline rather than the late observation tick.
- Current or pending task: `hasIdleDialogueAtTick` becomes false and no request starts. After the queue becomes empty again, the next tick arms exactly `now + idleDialogueInterval(id, now)` without immediate speech.
- No real issuer: no deadline is armed.
- Inactive body, full semaphore, or `dialogueInFlight=true`: no request starts, but due deadline advances once.
- `dialogueRequests == companion.MaxDialogueRequestsPerTask`: idle still dispatches and budget remains unchanged.

Hold successful fake requests until assertions complete, then release them so cleanup cannot leak goroutines.

- [ ] **Step 4: Run the red server tests**

Run:

```bash
go test ./internal/server -run 'TestIdleDialogueInterval|TestIdleDialogueDue|TestIdleDialogueAudience|TestIdleDialogueDispatch' -count=1
```

Expected: build failure because the idle scheduler interfaces and fields do not exist.

- [ ] **Step 5: Implement deterministic helpers**

Create `companion_idle_dialogue.go` with standard-library FNV-1a input order and wrap-safe comparison:

```go
const (
	idleDialogueMinTicks uint64 = companion.TicksPerMinute
	idleDialogueMaxTicks uint64 = 2 * companion.TicksPerMinute
)

func idleDialogueInterval(id companion.ID, seed uint64) uint64 {
	hash := fnv.New64a()
	_, _ = hash.Write(id[:])
	var encoded [8]byte
	binary.LittleEndian.PutUint64(encoded[:], seed)
	_, _ = hash.Write(encoded[:])
	return idleDialogueMinTicks + hash.Sum64()%(idleDialogueMaxTicks-idleDialogueMinTicks+1)
}

func idleDialogueDue(now, deadline uint64) bool {
	return int64(now-deadline) >= 0
}

func withinIdleDialogueDistance(from, to [3]float32) bool {
	dx := from[0] - to[0]
	dz := from[2] - to[2]
	const radius = companion.PathWindowHorizontalRadius
	return dx*dx+dz*dz <= radius*radius
}
```

- [ ] **Step 6: Mark restored issuers without changing wire or storage**

Add `restored bool` to `companionTaskIssuer`; set it only on the existing synthetic value:

```go
var restoredIssuerIdentity = companionTaskIssuer{
	playerID: core.PlayerID{0, 0, 0, 0, 0, 0, 0x40, 0, 0x80, 0, 0, 0, 0, 0, 0, 0},
	name:     "未知发令者",
	position: [3]float32{0, 1, 0},
	restored: true,
}
```

Do not alter `captureIssuer`; its zero value `restored=false` identifies live ingress. Update restore GoDoc to state that the marker prevents idle audience reuse.

- [ ] **Step 7: Implement audience and schedule dispatch**

`idleDialogueAudience` rejects invalid/restored identities, resolves the player through existing `followTarget`, and applies horizontal distance. `dispatchIdleDialogues` must use this exact state order:

```go
func (m *companionManager) dispatchIdleDialogues() {
	now := m.engine.TickCount()
	for _, id := range m.orderedIDs {
		slot := m.slots[id]
		_, hasCurrent := slot.queue.Current()
		if hasCurrent || slot.queue.Len() != 0 {
			slot.hasIdleDialogueAtTick = false
			continue
		}
		if !slot.currentIssuer.playerID.Valid() || slot.currentIssuer.restored {
			slot.hasIdleDialogueAtTick = false
			continue
		}
		if !slot.hasIdleDialogueAtTick {
			slot.idleDialogueAtTick = now + idleDialogueInterval(id, now)
			slot.hasIdleDialogueAtTick = true
			continue
		}
		if !idleDialogueDue(now, slot.idleDialogueAtTick) {
			continue
		}
		seed := slot.idleDialogueAtTick
		slot.idleDialogueAtTick = seed + idleDialogueInterval(id, seed)
		body, active := m.body(id)
		if !active || !m.idleDialogueAudience(slot.currentIssuer, body) {
			continue
		}
		m.requestDialogue(id, companion.DialogueNode{Kind: companion.DialogueNodeIdle})
	}
}
```

Call `manager.dispatchIdleDialogues()` immediately after `manager.dispatchPlanning()` in `advanceCompanionTasks`.

- [ ] **Step 8: Exclude idle from the task request budget**

In `requestDialogue`, derive one local flag and guard/increment only task nodes:

```go
taskNode := node.Kind != companion.DialogueNodeIdle
if taskNode && slot.dialogueRequests >= companion.MaxDialogueRequestsPerTask {
	return
}
```

After request construction succeeds:

```go
slot.dialogueInFlight = true
if taskNode {
	slot.dialogueRequests++
}
```

Do not change semaphore acquisition, inactive behavior, worker construction or terminal derivation.

- [ ] **Step 9: Run focused and package tests**

Run:

```bash
gofmt -w internal/server/companion_idle_dialogue.go internal/server/companion_idle_dialogue_test.go internal/server/companion_manager.go internal/server/companion_dialogue.go
go test ./internal/server -run 'TestIdleDialogueInterval|TestIdleDialogueDue|TestIdleDialogueAudience|TestIdleDialogueDispatch' -count=1
go test ./internal/server -race -count=1
go test ./internal/archcheck -count=1
```

Expected: PASS. Idle outcomes may still be dropped by the old switch; Task 3 owns result application.

- [ ] **Step 10: Commit Task 2**

```bash
git add internal/server/companion_idle_dialogue.go internal/server/companion_idle_dialogue_test.go internal/server/companion_manager.go internal/server/companion_dialogue.go
git commit -m "feat(server): schedule idle companion dialogue"
```

---

### Task 3: Apply Valid Idle Results And Prove Transport Parity

**Files:**
- Modify: `internal/server/companion_idle_dialogue_test.go`
- Modify: `internal/server/companion_dialogue.go:141-230`
- Modify: `internal/server/companion_dialogue_wiring_test.go:708-780`

**Interfaces:**
- Consumes: Task 2 `idleDialogueAudience`, `companionTaskIssuer.restored`, schedule dispatch, existing `dialogueOutcome`, `applyDialogueEffect`, `taskEventDeliveries`, fake model helpers and `dialogueParityProjection`.
- Produces: `applyDialogueOutcome` accepts `DialogueNodeIdle` only while the original idle audience and empty queue remain valid; valid idle speech reuses `applyDialogueEffect` without summary mutation.

- [ ] **Step 1: Write failing valid-result and stale-result tests**

In `companion_idle_dialogue_test.go`, add a helper that creates an idle outcome from current slot state:

```go
func idleDialogueOutcomeFor(slot *companionTaskSlot, id companion.ID, line string) dialogueOutcome {
	return dialogueOutcome{
		id:         id,
		generation: slot.queue.Generation(),
		node:       companion.DialogueNode{Kind: companion.DialogueNodeIdle},
		issuer:     slot.currentIssuer,
		line:       line,
	}
}
```

For the valid case, arrange a real online issuer, active cached body, empty queue, nonempty existing summary and `dialogueInFlight=true`; call `applyDialogueOutcome`. Assert:

```go
if slot.dialogueInFlight {
	t.Fatal("有效结果后在途标记未清除")
}
if slot.summary != oldSummary {
	t.Fatalf("idle 改写摘要=%q，want %q", slot.summary, oldSummary)
}
facts := manager.takeEventFacts()
if len(facts) != 1 || facts[0].speech != "今天天气适合走走" || facts[0].issuer != slot.currentIssuer {
	t.Fatalf("idle speech facts=%+v", facts)
}
```

Use fresh hosts for stale subtests. Starting from the same valid arrangement, mutate exactly one property before applying the outcome:

- enqueue one pending command;
- construct the outcome with a generation different from the still-empty queue, without starting or enqueueing a task;
- replace `currentIssuer` with another real identity;
- set `currentIssuer = restoredIssuerIdentity`;
- make `onlinePlayers` return nil;
- return the issuer at horizontal distance `16.01`;
- remove the cached body.

Every stale case must clear `dialogueInFlight`, leave effects/events/summary unchanged, and produce no speech.

Add one eligible outcome with `err=companion.ErrDialogueUnavailable`; record the already-scheduled next idle deadline before applying it, then assert the same no-effect result, exact deadline preservation and `dialogueInFlight` clearing. This proves idle model failure does not fall through to broadcast, summary mutation or an extra reschedule.

- [ ] **Step 2: Write a failing no-preemption transition test**

Hold an eligible idle request in the fake Dialogue model. Enqueue a real task, let Planner return a valid one-step plan while idle remains held, and advance until `TaskStarted`. Assert:

```go
requests, inFlight, cancels := dialogue.snapshotCounts()
if requests != 1 || inFlight != 1 || cancels != 0 {
	t.Fatalf("idle 在途后任务启动：requests=%d inFlight=%d cancels=%d", requests, inFlight, cancels)
}
```

Release the idle request, wait for its outcome, advance one tick, and assert no `CompanionSpeech` or summary update. This locks “task start does not cancel idle, does not start a second Dialogue, and stale idle result is discarded.”

- [ ] **Step 3: Write the failing Memory/TCP parity test**

First add `TestCompanionIdleDialogueBroadcastsToAllPlayers`: attach two online clients, establish the first as recent issuer, arm one due idle opportunity, and collect the resulting `CompanionSpeech` from both clients. Assert both copies have the same speech, companion identity, issuer `PlayerID`/`PlayerName` and `Reason=None`; assert the non-issuer receives it too.

Then add `TestCompanionIdleDialogueMemoryTCPParity` beside the existing Dialogue parity test. For each transport:

1. Start one companion and one player with identity `integrationIdentity(0x95, "发令者")`.
2. Complete one deterministic task to establish a real recent issuer; drain its events and wait until no Dialogue is in flight.
3. Replace Dialogue with a fresh fake whose response line is stable.
4. Under `stepMu`, set `idleDialogueAtTick = engine.TickCount()` and `hasIdleDialogueAtTick = true`.
5. Advance until exactly one new `CompanionSpeech` arrives.
6. Assert EventIDs are strictly increasing within that transport.
7. Return only `projectDialogueParityEvents` for events from the armed idle opportunity.

Compare Memory and TCP projections exactly; do not compare absolute ticks or cross-transport EventIDs.

- [ ] **Step 4: Run the red tests**

Run:

```bash
go test ./internal/server -run 'TestIdleDialogueOutcome|TestIdleDialogueTaskStartDoesNotPreempt|TestCompanionIdleDialogueBroadcastsToAllPlayers|TestCompanionIdleDialogueMemoryTCPParity' -count=1
```

Expected: valid idle outcome is dropped because `applyDialogueOutcome` does not yet accept `DialogueNodeIdle`; parity receives no idle speech.

- [ ] **Step 5: Implement idle result revalidation**

Extend the existing node switch in `applyDialogueOutcome` without weakening task-node checks:

```go
case companion.DialogueNodeIdle:
	if _, hasCurrent := slot.queue.Current(); hasCurrent || slot.queue.Len() != 0 {
		return
	}
	if slot.currentIssuer.restored ||
		slot.currentIssuer.playerID != outcome.issuer.playerID ||
		slot.currentIssuer.name != outcome.issuer.name {
		return
	}
	body, active := m.body(outcome.id)
	if !active || !m.idleDialogueAudience(slot.currentIssuer, body) {
		return
	}
```

Keep generation comparison before this switch and error handling after it. Continue calling the existing:

```go
m.applyDialogueEffect(outcome.id, outcome.node, outcome.issuer, outcome.line, outcome.summary)
```

No `applyDialogueEffect` logic change is required: only `DialogueNodeTerminal` writes summary, so idle automatically broadcasts speech without summary mutation.

- [ ] **Step 6: Run focused, package and parity tests**

Run:

```bash
gofmt -w internal/server/companion_idle_dialogue_test.go internal/server/companion_dialogue.go internal/server/companion_dialogue_wiring_test.go
go test ./internal/server -run 'TestIdleDialogueOutcome|TestIdleDialogueTaskStartDoesNotPreempt|TestCompanionIdleDialogueBroadcastsToAllPlayers|TestCompanionIdleDialogueMemoryTCPParity' -count=1
go test ./internal/companion ./internal/server -race -count=1
go test ./internal/archcheck -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit Task 3**

```bash
git add internal/server/companion_idle_dialogue_test.go internal/server/companion_dialogue.go internal/server/companion_dialogue_wiring_test.go
git commit -m "feat(server): publish valid idle companion speech"
```

---

## Per-Task Review Loop

After each Task commit, the control session must:

1. Dispatch a fresh SPEC reviewer with only the approved design, OpenSpec artifacts, task brief, base SHA and commit SHA.
2. Dispatch a separate fresh QUALITY reviewer for correctness, concurrency, bounded work, naming, comments and test sharpness.
3. Route findings back to the original implementer for rounds 1–3; use a fresh implementer for rounds 4–5.
4. Re-run that Task’s exact focused commands after each repair.
5. Append implementation SHA, RED/GREEN evidence, SPEC verdict, QUALITY verdict and any `Ruling: <决定> — <原因> — <错误点>` to `openspec/changes/companion-idle-dialogue/ledger.md`.
6. Check the corresponding `tasks.md` boxes only after both reviews pass and verification output is fresh.

## Whole-Branch Completion Gate

After Task 3 passes dual review, dispatch an independent whole-branch reviewer against the merge base. Resolve all blocking findings through the same bounded repair loop, then run:

```bash
gofmt -w internal/companion/dialogue_nodes.go internal/companion/dialogue_nodes_test.go internal/companion/dialogue_client.go internal/companion/dialogue_client_test.go internal/server/companion_idle_dialogue.go internal/server/companion_idle_dialogue_test.go internal/server/companion_manager.go internal/server/companion_dialogue.go internal/server/companion_dialogue_wiring_test.go
test -z "$(gofmt -l internal/companion internal/server)"
git diff --check c60e8f69...HEAD
git diff --check
git diff --cached --check
test -z "$(git status --porcelain)"
git diff --name-only c60e8f69...HEAD
go test ./internal/companion ./internal/server -race -count=1
go test ./internal/archcheck -count=1
go vet ./...
go test ./... -race
make rust-check
openspec validate --all --strict --no-interactive
scripts/agents/gates.sh
```

The clean-worktree assertion is a precondition for the range audit；it prevents staged、unstaged or untracked paths from bypassing `git diff --name-only c60e8f69...HEAD`。Audit every range path against the approved plan：only the listed Go files、`openspec/changes/companion-idle-dialogue/`、this plan and the C-08 row in `docs/feature-backlog.md` are allowed before archive。Any version、schema、ABI、scenario、capture or golden path is a hard failure；do not rely on OpenSpec validation to prove file immutability。

No benchmark is required: the tick path scans at most `companion.MaxActive == 4` slots and performs the existing bounded environment scan only at a due, eligible opportunity. Record command outputs and any measured durations in the ledger; do not weaken timeouts or thresholds.

## Archive And Integration Gate

Only after the whole-branch reviewer and all gates pass:

1. Sync the `companion-dialogue` delta into the main spec.
2. Archive change `companion-idle-dialogue` and rerun strict validation.
3. Update C-08 from `已认领` to `已完成`, preserving claimant/history and adding archive, verification and PR evidence.
4. Refresh GitHub Discussion #71 from the canonical backlog and run its tests.
5. Push `feat/C-08-companion-idle-dialogue`, open a PR whose title contains `C-08`, and include design/change paths plus verification evidence.
6. Watch `gh pr checks --watch`; diagnose and repair failures without bypassing hooks, up to the repository’s 10-round limit.
7. Merge with `gh pr merge --merge` only when all checks are green, then fast-forward local `main` from `origin/main` without resetting or rebasing protected worktrees.
