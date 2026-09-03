package protocol

import "testing"

func TestCompanionMessageIDsAreAppendOnly(t *testing.T) {
	assertClientRegistry(t, []struct {
		state  State
		packet ClientPacket
		id     uint32
	}{
		{StatePlay, PlayerInput{}, 0},
		{StatePlay, PlaceBlock{}, 2},
		{StatePlay, RequestChunkResync{}, 3},
		{StatePlay, KeepAliveReply{}, 4},
		{StatePlay, SelectHotbar{}, 5},
		{StatePlay, MoveInventoryStack{}, 6},
		{StatePlay, MoveCraftingStack{}, 7},
		{StatePlay, OpenContainer{}, 8},
		{StatePlay, MoveContainerStack{}, 9},
		{StatePlay, CloseContainer{}, 10},
		{StatePlay, DropSelectedItem{}, 11},
		{StatePlay, ChatCommand{}, 12},
	})
	assertServerRegistry(t, []struct {
		state  State
		packet ServerPacket
		id     uint32
	}{
		{StatePlay, ChunkSnapshot{}, 0},
		{StatePlay, BlockChanges{}, 1},
		{StatePlay, ForgetChunks{}, 2},
		{StatePlay, PlayerState{}, 3},
		{StatePlay, CommandRejected{}, 4},
		{StatePlay, KeepAlive{}, 5},
		{StatePlay, Disconnect{}, 6},
		{StatePlay, RemotePlayerSpawn{}, 7},
		{StatePlay, RemotePlayerDespawn{}, 8},
		{StatePlay, RemotePlayerStates{}, 9},
		{StatePlay, InventoryState{}, 10},
		{StatePlay, ItemDropUpserts{}, 11},
		{StatePlay, ItemDropRemoves{}, 12},
		{StatePlay, FurnaceState{}, 13},
		{StatePlay, ContainerClosed{}, 14},
		{StatePlay, ChestState{}, 15},
		{StatePlay, ChatEvent{}, 16},
		{StatePlay, CompanionSpawn{}, 17},
		{StatePlay, CompanionStates{}, 18},
		{StatePlay, CompanionDespawn{}, 19},
		{StatePlay, PlaceBlockSucceeded{}, 20},
	})
	if _, ok := ClientPacketForID(StatePlay, 1); ok {
		t.Fatal("Play client packet ID 1 必须保持未分配")
	}
	// v22 把 13 分配给了 TillSoil，v27 把 14 分配给了 BoneMeal，格子工作台把
	// 15 分配给 `TakeCraftingOutput`（`MoveCraftingStack` 复用 7）；本表的
	// 「下一个仍未分配」上界随之推进到 16。
	if _, ok := ClientPacketForID(StatePlay, 16); ok {
		t.Fatal("未知 client packet ID 16 被接受")
	}
	// 格子工作台把 21 分配给了 `CraftingState`，夜行者把 22/23/24 分配给
	// `HostileSpawn`/`HostileState`/`HostileDespawn`，私有战斗命中把 25 分配给
	// `CombatHit`；下一个仍未分配的上界推进到 26。
	if _, ok := ServerPacketForID(StatePlay, 26); ok {
		t.Fatal("未知 server packet ID 26 被接受")
	}
}

func TestChatEventTaskEnumsAreFrozen(t *testing.T) {
	// v17 在既有 kind 1..2 之后追加任务生命周期 kind 3..7，v18 追加停止 kind 8，
	// v19 追加伙伴台词 kind 9；未知 kind 10 仍非法。
	kinds := []struct {
		name string
		got  ChatEventKind
		want uint8
	}{
		{"accepted", ChatEventAccepted, 1},
		{"rejected", ChatEventRejected, 2},
		{"task started", ChatEventTaskStarted, 3},
		{"task progress", ChatEventTaskProgress, 4},
		{"task completed", ChatEventTaskCompleted, 5},
		{"task failed", ChatEventTaskFailed, 6},
		{"task timed out", ChatEventTaskTimedOut, 7},
		{"task stopped", ChatEventTaskStopped, 8},
		{"companion speech", ChatEventCompanionSpeech, 9},
	}
	for _, kind := range kinds {
		if uint8(kind.got) != kind.want {
			t.Fatalf("%s kind = %d, want %d", kind.name, kind.got, kind.want)
		}
	}

	// QueueFull 取 4：值 3 预留，v18 追加 NotFollowing=5，拒绝原因保留 0..15 的编号空间。
	reasons := []struct {
		name string
		got  ChatRejectReason
		want uint8
	}{
		{"none", ChatRejectNone, 0},
		{"invalid format", ChatRejectInvalidFormat, 1},
		{"unknown companion", ChatRejectUnknownCompanion, 2},
		{"queue full", ChatRejectQueueFull, 4},
		{"not following", ChatRejectNotFollowing, 5},
	}
	for _, reason := range reasons {
		if uint8(reason.got) != reason.want {
			t.Fatalf("%s reject reason = %d, want %d", reason.name, reason.got, reason.want)
		}
	}

	// TaskFailReason 与拒绝原因共用 reason 槽位，从 16 起错开区间；v18 追加 InventoryFull=20。
	failReasons := []struct {
		name string
		got  TaskFailReason
		want uint8
	}{
		{"planner unavailable", TaskFailPlannerUnavailable, 16},
		{"invalid plan", TaskFailInvalidPlan, 17},
		{"path unreachable", TaskFailPathUnreachable, 18},
		{"world changed", TaskFailWorldChanged, 19},
		{"inventory full", TaskFailInventoryFull, 20},
	}
	for _, reason := range failReasons {
		if uint8(reason.got) != reason.want {
			t.Fatalf("%s task fail reason = %d, want %d", reason.name, reason.got, reason.want)
		}
	}
}
