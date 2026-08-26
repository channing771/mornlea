package server

import (
	"math"
	"testing"

	"github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/network"
)

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

func TestIdleDialogueAudienceHorizontalBoundaryAndEligibility(t *testing.T) {
	host, _, _, body, liveIssuer := idleDialogueDispatchRig(t)
	manager := host.world.companionManager
	cases := []struct {
		name     string
		issuer   companionTaskIssuer
		position [3]float32
		online   bool
		want     bool
	}{
		{
			name:     "正好十六格且忽略高度",
			issuer:   liveIssuer,
			position: [3]float32{body.Position[0] + 16, body.Position[1] + 4096, body.Position[2]},
			online:   true,
			want:     true,
		},
		{
			name:     "超过十六格",
			issuer:   liveIssuer,
			position: [3]float32{body.Position[0] + 16.01, body.Position[1], body.Position[2]},
			online:   true,
			want:     false,
		},
		{
			name:     "玩家离线",
			issuer:   liveIssuer,
			position: body.Position,
			online:   false,
			want:     false,
		},
		{
			name:     "恢复合成身份",
			issuer:   restoredIssuerIdentity,
			position: body.Position,
			online:   true,
			want:     false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host.world.stepMu.Lock()
			manager.onlinePlayers = func() []companion.PlanPlayer {
				if !tc.online {
					return nil
				}
				return []companion.PlanPlayer{{ID: tc.issuer.playerID, Position: tc.position}}
			}
			got := manager.idleDialogueAudience(tc.issuer, body)
			host.world.stepMu.Unlock()
			if got != tc.want {
				t.Fatalf("idleDialogueAudience()=%v，want %v", got, tc.want)
			}
		})
	}
}

func TestIdleDialogueDispatchFirstEligibleTickOnlyArms(t *testing.T) {
	host, dialogue, id, _, issuer := idleDialogueDispatchRig(t)
	manager := host.world.companionManager

	host.world.stepMu.Lock()
	slot := manager.slots[id]
	slot.currentIssuer = issuer
	now := manager.engine.TickCount()
	beforeBudget := slot.dialogueRequests
	manager.dispatchIdleDialogues()
	wantDeadline := now + idleDialogueInterval(id, now)
	if !slot.hasIdleDialogueAtTick || slot.idleDialogueAtTick != wantDeadline {
		host.world.stepMu.Unlock()
		t.Fatalf("首次期限=%d/%v，want %d/true", slot.idleDialogueAtTick,
			slot.hasIdleDialogueAtTick, wantDeadline)
	}
	if slot.dialogueInFlight || slot.dialogueRequests != beforeBudget {
		host.world.stepMu.Unlock()
		t.Fatalf("首次排期立即派发：inFlight=%v budget=%d，want false/%d",
			slot.dialogueInFlight, slot.dialogueRequests, beforeBudget)
	}
	host.world.stepMu.Unlock()
	if requests, _, _ := dialogue.snapshotCounts(); requests != 0 {
		t.Fatalf("首次排期模型请求数=%d，want 0", requests)
	}
}

func TestIdleDialogueDispatchDueEligibleAnchorsToOldDeadline(t *testing.T) {
	host, dialogue, id, _, issuer := idleDialogueDispatchRig(t)
	manager := host.world.companionManager

	host.world.stepMu.Lock()
	slot := manager.slots[id]
	slot.currentIssuer = issuer
	now := manager.engine.TickCount()
	if now == 0 {
		host.world.stepMu.Unlock()
		t.Fatal("测试世界 tick 未推进")
	}
	oldDeadline := now - 1
	wantDeadline := oldDeadline + idleDialogueInterval(id, oldDeadline)
	if fromObservation := now + idleDialogueInterval(id, now); wantDeadline == fromObservation {
		host.world.stepMu.Unlock()
		t.Fatal("测试种子未区分旧期限与观察 tick")
	}
	slot.idleDialogueAtTick = oldDeadline
	slot.hasIdleDialogueAtTick = true
	slot.dialogueRequests = 2
	beforeBudget := slot.dialogueRequests
	manager.dispatchIdleDialogues()
	if slot.idleDialogueAtTick != wantDeadline || !slot.hasIdleDialogueAtTick {
		host.world.stepMu.Unlock()
		t.Fatalf("晚到机会的下一期限=%d/%v，want %d/true",
			slot.idleDialogueAtTick, slot.hasIdleDialogueAtTick, wantDeadline)
	}
	if !slot.dialogueInFlight || slot.dialogueRequests != beforeBudget {
		host.world.stepMu.Unlock()
		t.Fatalf("eligible idle 派发状态 inFlight=%v budget=%d，want true/%d",
			slot.dialogueInFlight, slot.dialogueRequests, beforeBudget)
	}
	host.world.stepMu.Unlock()

	waitForDialogueRequests(t, dialogue, 1)
	records := dialogue.snapshotDialogueRequests()
	if len(records) != 1 || records[0].NodeKind != "idle" {
		t.Fatalf("idle 请求=%+v，want 单个 idle 节点", records)
	}
	releaseIdleDialogueRequest(t, host, dialogue, id)
}

func TestIdleDialogueDispatchTaskStateClearsAndRearms(t *testing.T) {
	for _, state := range []string{"current", "pending"} {
		t.Run(state, func(t *testing.T) {
			host, dialogue, id, _, issuer := idleDialogueDispatchRig(t)
			manager := host.world.companionManager

			host.world.stepMu.Lock()
			slot := manager.slots[id]
			slot.currentIssuer = issuer
			slot.idleDialogueAtTick = manager.engine.TickCount()
			slot.hasIdleDialogueAtTick = true
			if !slot.queue.Enqueue(companion.TaskCommand("测试任务")) {
				host.world.stepMu.Unlock()
				t.Fatal("构造 pending 任务失败")
			}
			if state == "current" && !slot.queue.BeginHead() {
				host.world.stepMu.Unlock()
				t.Fatal("构造 current 任务失败")
			}
			manager.dispatchIdleDialogues()
			if slot.hasIdleDialogueAtTick {
				host.world.stepMu.Unlock()
				t.Fatalf("%s 任务未清除 idle 期限", state)
			}
			if state == "pending" && !slot.queue.BeginHead() {
				host.world.stepMu.Unlock()
				t.Fatal("消费 pending 任务失败")
			}
			if !slot.queue.BeginPlanning() || len(slot.queue.FailPlanning(companion.TaskFailPlannerUnavailable)) != 1 {
				host.world.stepMu.Unlock()
				t.Fatal("清空测试任务失败")
			}
			rearmedAt := manager.engine.TickCount()
			manager.dispatchIdleDialogues()
			wantDeadline := rearmedAt + idleDialogueInterval(id, rearmedAt)
			if !slot.hasIdleDialogueAtTick || slot.idleDialogueAtTick != wantDeadline {
				host.world.stepMu.Unlock()
				t.Fatalf("%s 清空后期限=%d/%v，want %d/true", state,
					slot.idleDialogueAtTick, slot.hasIdleDialogueAtTick, wantDeadline)
			}
			if slot.dialogueInFlight {
				host.world.stepMu.Unlock()
				t.Fatalf("%s 清空后的首 tick 立即派发", state)
			}
			host.world.stepMu.Unlock()
			if requests, _, _ := dialogue.snapshotCounts(); requests != 0 {
				t.Fatalf("%s 清空后的模型请求数=%d，want 0", state, requests)
			}
		})
	}
}

func TestIdleDialogueDispatchNoRealIssuerClearsDeadline(t *testing.T) {
	cases := []struct {
		name   string
		issuer companionTaskIssuer
	}{
		{name: "无发令者", issuer: companionTaskIssuer{}},
		{name: "恢复合成身份", issuer: restoredIssuerIdentity},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host, dialogue, id, _, _ := idleDialogueDispatchRig(t)
			manager := host.world.companionManager

			host.world.stepMu.Lock()
			slot := manager.slots[id]
			slot.currentIssuer = tc.issuer
			slot.idleDialogueAtTick = manager.engine.TickCount()
			slot.hasIdleDialogueAtTick = true
			manager.dispatchIdleDialogues()
			if slot.hasIdleDialogueAtTick || slot.dialogueInFlight {
				host.world.stepMu.Unlock()
				t.Fatalf("无真实发令者后状态 deadline=%v inFlight=%v，want false/false",
					slot.hasIdleDialogueAtTick, slot.dialogueInFlight)
			}
			host.world.stepMu.Unlock()
			if requests, _, _ := dialogue.snapshotCounts(); requests != 0 {
				t.Fatalf("无真实发令者产生模型请求=%d", requests)
			}
		})
	}
}

func TestIdleDialogueDispatchDueSkipsAdvanceExactDeadline(t *testing.T) {
	cases := []struct {
		name         string
		prepare      func(*companionManager, *companionTaskSlot, companion.ID, companion.Body, companionTaskIssuer)
		cleanup      func(*companionManager, *companionTaskSlot, companion.ID, companion.Body, companionTaskIssuer)
		wantInFlight bool
	}{
		{
			name: "inactive",
			prepare: func(manager *companionManager, _ *companionTaskSlot, id companion.ID, _ companion.Body, _ companionTaskIssuer) {
				delete(manager.bodies, id)
			},
		},
		{
			name: "offline",
			prepare: func(manager *companionManager, _ *companionTaskSlot, _ companion.ID, _ companion.Body, _ companionTaskIssuer) {
				manager.onlinePlayers = func() []companion.PlanPlayer { return nil }
			},
		},
		{
			name: "超出水平范围",
			prepare: func(manager *companionManager, _ *companionTaskSlot, _ companion.ID, body companion.Body, issuer companionTaskIssuer) {
				manager.onlinePlayers = func() []companion.PlanPlayer {
					return []companion.PlanPlayer{{
						ID: issuer.playerID,
						Position: [3]float32{
							body.Position[0] + 16.01,
							body.Position[1],
							body.Position[2],
						},
					}}
				}
			},
		},
		{
			name: "共享模型槽满",
			prepare: func(manager *companionManager, _ *companionTaskSlot, _ companion.ID, _ companion.Body, _ companionTaskIssuer) {
				for range cap(manager.semaphore) {
					manager.semaphore <- struct{}{}
				}
			},
			cleanup: func(manager *companionManager, _ *companionTaskSlot, _ companion.ID, _ companion.Body, _ companionTaskIssuer) {
				for len(manager.semaphore) != 0 {
					<-manager.semaphore
				}
			},
		},
		{
			name: "已有台词在途",
			prepare: func(_ *companionManager, slot *companionTaskSlot, _ companion.ID, _ companion.Body, _ companionTaskIssuer) {
				slot.dialogueInFlight = true
			},
			cleanup: func(_ *companionManager, slot *companionTaskSlot, _ companion.ID, _ companion.Body, _ companionTaskIssuer) {
				slot.dialogueInFlight = false
			},
			wantInFlight: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host, dialogue, id, body, issuer := idleDialogueDispatchRig(t)
			manager := host.world.companionManager

			host.world.stepMu.Lock()
			slot := manager.slots[id]
			slot.currentIssuer = issuer
			oldDeadline := manager.engine.TickCount()
			slot.idleDialogueAtTick = oldDeadline
			slot.hasIdleDialogueAtTick = true
			slot.dialogueRequests = 3
			if tc.prepare != nil {
				tc.prepare(manager, slot, id, body, issuer)
			}
			manager.dispatchIdleDialogues()
			wantDeadline := oldDeadline + idleDialogueInterval(id, oldDeadline)
			if !slot.hasIdleDialogueAtTick || slot.idleDialogueAtTick != wantDeadline {
				host.world.stepMu.Unlock()
				t.Fatalf("跳过后的期限=%d/%v，want %d/true",
					slot.idleDialogueAtTick, slot.hasIdleDialogueAtTick, wantDeadline)
			}
			if slot.dialogueInFlight != tc.wantInFlight || slot.dialogueRequests != 3 {
				host.world.stepMu.Unlock()
				t.Fatalf("跳过后的状态 inFlight=%v budget=%d，want %v/3",
					slot.dialogueInFlight, slot.dialogueRequests, tc.wantInFlight)
			}
			if tc.cleanup != nil {
				tc.cleanup(manager, slot, id, body, issuer)
			}
			host.world.stepMu.Unlock()
			if requests, _, _ := dialogue.snapshotCounts(); requests != 0 {
				t.Fatalf("跳过场景模型请求数=%d，want 0", requests)
			}
		})
	}
}

func TestIdleDialogueDispatchIgnoresTaskBudget(t *testing.T) {
	host, dialogue, id, _, issuer := idleDialogueDispatchRig(t)
	manager := host.world.companionManager

	host.world.stepMu.Lock()
	slot := manager.slots[id]
	slot.currentIssuer = issuer
	oldDeadline := manager.engine.TickCount()
	slot.idleDialogueAtTick = oldDeadline
	slot.hasIdleDialogueAtTick = true
	slot.dialogueRequests = companion.MaxDialogueRequestsPerTask
	manager.dispatchIdleDialogues()
	wantDeadline := oldDeadline + idleDialogueInterval(id, oldDeadline)
	if !slot.dialogueInFlight || !slot.hasIdleDialogueAtTick || slot.idleDialogueAtTick != wantDeadline {
		host.world.stepMu.Unlock()
		t.Fatalf("预算满时派发状态 inFlight=%v deadline=%d/%v，want true/%d/true",
			slot.dialogueInFlight, slot.idleDialogueAtTick, slot.hasIdleDialogueAtTick, wantDeadline)
	}
	if slot.dialogueRequests != companion.MaxDialogueRequestsPerTask {
		host.world.stepMu.Unlock()
		t.Fatalf("idle 改写任务预算=%d，want %d", slot.dialogueRequests,
			companion.MaxDialogueRequestsPerTask)
	}
	host.world.stepMu.Unlock()

	waitForDialogueRequests(t, dialogue, 1)
	records := dialogue.snapshotDialogueRequests()
	if len(records) != 1 || records[0].NodeKind != "idle" {
		t.Fatalf("预算满时 idle 请求=%+v", records)
	}
	releaseIdleDialogueRequest(t, host, dialogue, id)
}

func TestIdleDialogueDispatchPlanningPrecedesIdleAtAuthorityTick(t *testing.T) {
	plannerDefinition := companion.Definition{ID: chatTestCompanionID(1), Name: "规划伙伴"}
	idleDefinition := companion.Definition{ID: chatTestCompanionID(2), Name: "空闲伙伴"}
	host := newCompanionManagerHost(t,
		[]companion.Definition{plannerDefinition, idleDefinition}, nil, nil)
	client := openCompanionChatClient(t, host, "memory", integrationIdentity(0x71, "在线玩家"))
	waitForCompanionChatWorld(t, host, []network.ClientEndpoint{client}, 2)
	manager := host.world.companionManager

	host.world.stepMu.Lock()
	manager.refreshBodies()
	plannerBody, plannerActive := manager.body(plannerDefinition.ID)
	idleBody, idleActive := manager.body(idleDefinition.ID)
	host.world.stepMu.Unlock()
	if !plannerActive || !idleActive {
		t.Fatalf("测试伙伴未全部激活：planner=%v idle=%v", plannerActive, idleActive)
	}
	plannerCell := standingCellOf(plannerBody.Position)
	planner := newFakeCompanionModel(t, [3]int32{plannerCell.X, plannerCell.Y, plannerCell.Z})
	planner.holdRequests()
	t.Cleanup(planner.releaseRequests)
	manager.replacePlannerForTest(t, planner)
	dialogue := newFakeDialogueModel(t)
	dialogue.holdRequests()
	t.Cleanup(dialogue.releaseRequests)
	manager.replaceDialogueForTest(t, dialogue)
	plannerIssuer := stopTestIssuer(integrationIdentity(0x72, "规划发令者"))
	idleIssuer := stopTestIssuer(integrationIdentity(0x73, "空闲发令者"))

	host.world.stepMu.Lock()
	if !manager.enqueueCommand(plannerDefinition, companion.TaskCommand("规划任务"), plannerIssuer) {
		host.world.stepMu.Unlock()
		t.Fatal("构造 pending Planner 请求失败")
	}
	idleSlot := manager.slots[idleDefinition.ID]
	idleSlot.currentIssuer = idleIssuer
	oldDeadline := manager.engine.TickCount()
	idleSlot.idleDialogueAtTick = oldDeadline
	idleSlot.hasIdleDialogueAtTick = true
	manager.onlinePlayers = func() []companion.PlanPlayer {
		return []companion.PlanPlayer{{ID: idleIssuer.playerID, Position: idleBody.Position}}
	}
	releaseReserved := reserveIdleDialogueModelSlots(t, manager, cap(manager.semaphore)-1)
	host.world.stepMu.Unlock()

	result := host.world.StepForTest()
	receiveCompanionChatTick(t, client, result.Tick)
	host.world.stepMu.Lock()
	plannerInFlight := manager.slots[plannerDefinition.ID].planningInFlight
	idleInFlight := idleSlot.dialogueInFlight
	gotDeadline := idleSlot.idleDialogueAtTick
	hasDeadline := idleSlot.hasIdleDialogueAtTick
	host.world.stepMu.Unlock()
	if !plannerInFlight {
		t.Fatal("Planner 未在 authority tick 先取得最后一个共享模型槽")
	}
	if idleInFlight {
		t.Fatal("Planner 取得最后槽位后 idle 仍发起了请求")
	}
	wantDeadline := oldDeadline + idleDialogueInterval(idleDefinition.ID, oldDeadline)
	if !hasDeadline || gotDeadline != wantDeadline {
		t.Fatalf("槽满跳过后的 idle 期限=%d/%v，want %d/true",
			gotDeadline, hasDeadline, wantDeadline)
	}

	waitForModelRequests(t, planner, 1)
	if requests, _, inFlight, _ := planner.snapshotCounts(); requests != 1 || inFlight != 1 {
		t.Fatalf("Planner 请求状态 requests=%d inFlight=%d，want 1/1", requests, inFlight)
	}
	if requests, _, _ := dialogue.snapshotCounts(); requests != 0 {
		t.Fatalf("最后槽位被 Planner 占用后 idle 请求数=%d，want 0", requests)
	}
	planner.releaseRequests()
	waitIntegrationCondition(t, "Planner 结果进入 tick 队列", func() bool {
		return len(manager.plannerResults) == 1
	})
	releaseReserved()
}

func TestIdleDialogueDispatchUsesOrderedIDs(t *testing.T) {
	firstID := chatTestCompanionID(1)
	laterID := chatTestCompanionID(2)
	definitions := []companion.Definition{
		{ID: laterID, Name: "后序伙伴", ResolvedPersona: "canonical-later"},
		{ID: firstID, Name: "先序伙伴", ResolvedPersona: "canonical-first"},
	}
	host := newCompanionManagerHost(t, definitions, nil, nil)
	client := openCompanionChatClient(t, host, "memory", integrationIdentity(0x71, "在线玩家"))
	waitForCompanionChatWorld(t, host, []network.ClientEndpoint{client}, 2)
	manager := host.world.companionManager
	dialogue := newFakeDialogueModel(t)
	dialogue.holdRequests()
	t.Cleanup(dialogue.releaseRequests)
	manager.replaceDialogueForTest(t, dialogue)
	firstIssuer := stopTestIssuer(integrationIdentity(0x74, "先序发令者"))
	laterIssuer := stopTestIssuer(integrationIdentity(0x75, "后序发令者"))

	host.world.stepMu.Lock()
	manager.refreshBodies()
	firstBody, firstActive := manager.body(firstID)
	laterBody, laterActive := manager.body(laterID)
	if !firstActive || !laterActive {
		host.world.stepMu.Unlock()
		t.Fatalf("测试伙伴未全部激活：first=%v later=%v", firstActive, laterActive)
	}
	if len(manager.orderedIDs) != 2 || manager.orderedIDs[0] != firstID || manager.orderedIDs[1] != laterID {
		host.world.stepMu.Unlock()
		t.Fatalf("测试定义未形成 canonical 顺序：%v", manager.orderedIDs)
	}
	firstSlot := manager.slots[firstID]
	laterSlot := manager.slots[laterID]
	firstSlot.currentIssuer = firstIssuer
	laterSlot.currentIssuer = laterIssuer
	oldDeadline := manager.engine.TickCount()
	firstSlot.idleDialogueAtTick = oldDeadline
	firstSlot.hasIdleDialogueAtTick = true
	laterSlot.idleDialogueAtTick = oldDeadline
	laterSlot.hasIdleDialogueAtTick = true
	manager.onlinePlayers = func() []companion.PlanPlayer {
		return []companion.PlanPlayer{
			{ID: firstIssuer.playerID, Position: firstBody.Position},
			{ID: laterIssuer.playerID, Position: laterBody.Position},
		}
	}
	releaseReserved := reserveIdleDialogueModelSlots(t, manager, cap(manager.semaphore)-1)
	host.world.stepMu.Unlock()

	result := host.world.StepForTest()
	receiveCompanionChatTick(t, client, result.Tick)
	host.world.stepMu.Lock()
	firstInFlight := firstSlot.dialogueInFlight
	laterInFlight := laterSlot.dialogueInFlight
	firstDeadline := firstSlot.idleDialogueAtTick
	laterDeadline := laterSlot.idleDialogueAtTick
	host.world.stepMu.Unlock()
	if !firstInFlight || laterInFlight {
		t.Fatalf("canonical idle 获槽状态 first=%v later=%v，want true/false",
			firstInFlight, laterInFlight)
	}
	wantFirstDeadline := oldDeadline + idleDialogueInterval(firstID, oldDeadline)
	wantLaterDeadline := oldDeadline + idleDialogueInterval(laterID, oldDeadline)
	if firstDeadline != wantFirstDeadline || laterDeadline != wantLaterDeadline {
		t.Fatalf("ordered idle 下一期限 first=%d later=%d，want %d/%d",
			firstDeadline, laterDeadline, wantFirstDeadline, wantLaterDeadline)
	}

	waitForDialogueRequests(t, dialogue, 1)
	records := dialogue.snapshotDialogueRequests()
	if len(records) != 1 || records[0].Persona != "canonical-first" || records[0].NodeKind != "idle" {
		t.Fatalf("唯一 idle 请求=%+v，want canonical-first idle", records)
	}
	releaseIdleDialogueRequest(t, host, dialogue, firstID)
	releaseReserved()
}

func idleDialogueDispatchRig(
	t *testing.T,
) (*Host, *fakeDialogueModel, companion.ID, companion.Body, companionTaskIssuer) {
	t.Helper()
	id := chatTestCompanionID(1)
	host, _, _ := companionManagerHostReady(t,
		[]companion.Definition{{ID: id, Name: "阿木"}}, nil)
	dialogue := newFakeDialogueModel(t)
	dialogue.holdRequests()
	t.Cleanup(dialogue.releaseRequests)
	manager := host.world.companionManager
	manager.replaceDialogueForTest(t, dialogue)
	issuer := stopTestIssuer(integrationIdentity(0x71, "发令者"))
	if issuer.restored {
		t.Fatal("实时发令者被错误标记为恢复身份")
	}

	host.world.stepMu.Lock()
	defer host.world.stepMu.Unlock()
	manager.refreshBodies()
	body, active := manager.body(id)
	if !active {
		t.Fatal("空闲台词测试伙伴未激活")
	}
	manager.onlinePlayers = func() []companion.PlanPlayer {
		return []companion.PlanPlayer{{ID: issuer.playerID, Position: body.Position}}
	}
	return host, dialogue, id, body, issuer
}

func releaseIdleDialogueRequest(
	t *testing.T,
	host *Host,
	dialogue *fakeDialogueModel,
	id companion.ID,
) {
	t.Helper()
	dialogue.releaseRequests()
	waitForDialogueOutcomeQueued(t, host)
	host.world.stepMu.Lock()
	host.world.companionManager.applyDialogueOutcomes()
	inFlight := host.world.companionManager.slots[id].dialogueInFlight
	host.world.stepMu.Unlock()
	if inFlight {
		t.Fatal("idle 结果未清除台词在途标记")
	}
}

func reserveIdleDialogueModelSlots(
	t *testing.T,
	manager *companionManager,
	count int,
) func() {
	t.Helper()
	for range count {
		manager.semaphore <- struct{}{}
	}
	released := false
	release := func() {
		if released {
			return
		}
		for range count {
			<-manager.semaphore
		}
		released = true
	}
	t.Cleanup(release)
	return release
}
