package protocol

import (
	"fmt"
	"testing"
)

func TestProtocolV1PacketIDsAreFrozen(t *testing.T) {
	client := []struct {
		state  State
		packet ClientPacket
		id     uint32
	}{
		{StateHandshake, ClientHello{}, 0}, {StateLogin, LoginStart{}, 0},
		{StatePlay, PlayerInput{}, 0},
		{StatePlay, PlaceBlock{}, 2}, {StatePlay, RequestChunkResync{}, 3},
		{StatePlay, KeepAliveReply{}, 4},
	}
	server := []struct {
		state  State
		packet ServerPacket
		id     uint32
	}{
		{StateHandshake, ServerHello{}, 0}, {StateHandshake, HandshakeReject{}, 1},
		{StateLogin, LoginSuccess{}, 0}, {StateLogin, LoginReject{}, 1},
		{StatePlay, ChunkSnapshot{}, 0}, {StatePlay, BlockChanges{}, 1},
		{StatePlay, ForgetChunks{}, 2}, {StatePlay, PlayerState{}, 3},
		{StatePlay, CommandRejected{}, 4}, {StatePlay, KeepAlive{}, 5},
		{StatePlay, Disconnect{}, 6},
	}
	assertClientRegistry(t, client)
	assertServerRegistry(t, server)
	if _, ok := ClientPacketForID(StatePlay, 1); ok {
		t.Fatal("Play client packet ID 1 必须保持未分配")
	}
	if id, ok := ClientPacketID(StatePlay, PlaceBlock{}); !ok || id != 2 {
		t.Fatalf("PlaceBlock ID = %d, ok=%v，想要 2, true", id, ok)
	}
	if id, ok := ClientPacketID(StatePlay, CloseContainer{}); !ok || id != 10 {
		t.Fatalf("CloseContainer ID = %d, ok=%v，想要 10, true", id, ok)
	}
}

// TestGridCraftingPacketIDsAreFrozen 钉死格子工作台三条消息的最终编号：
// C→S `MoveCraftingStack=7`（复用已删除 recipe-click 消息释放的旧槽位）、
// `TakeCraftingOutput=15`、S→C `CraftingState=21`。协议版本保持主基线 v27
// 不变，这些消息类型是既有扩展不升版。
// 上界断言写成「末项 +1」而不是裸字面量，下次追加 packet 时它会跟着末项走，
// 不会静默退化成「测一个已合法的 ID」。
func TestGridCraftingPacketIDsAreFrozen(t *testing.T) {
	assertClientRegistry(t, []struct {
		state  State
		packet ClientPacket
		id     uint32
	}{
		{StatePlay, MoveCraftingStack{}, 7},
		{StatePlay, TakeCraftingOutput{}, 15},
	})
	assertServerRegistry(t, []struct {
		state  State
		packet ServerPacket
		id     uint32
	}{
		{StatePlay, CraftingState{}, 21},
	})
	if _, ok := ClientPacketForID(StatePlay, 15+1); ok {
		t.Fatal("Play client packet ID 16 必须保持未分配")
	}
	if _, ok := ServerPacketForID(StatePlay, 25+1); ok {
		t.Fatal("Play server packet ID 26 必须保持未分配")
	}
	if ProtocolVersion != 32 {
		t.Fatalf("协议版本 = %d，想要 32——夜行者三类消息由 v30 承载、显示相位偏移由 v31 承载、私有战斗命中由 v32 承载", ProtocolVersion)
	}
}

// TestProtocolV22TillSoilPacketIDIsFrozen 钉死 v22 唯一的 wire 变化：翻地命令
// 占 Play/C→S 的 ID 13。上界断言写成「翻地之后的相邻已分配编号」而不是裸
// 字面量，下次追加客户端 packet 时它会跟着末项走，不会静默退化成「测一个
// 已合法的 ID」。
func TestProtocolV22TillSoilPacketIDIsFrozen(t *testing.T) {
	id, ok := ClientPacketID(StatePlay, TillSoil{})
	if !ok || id != 13 {
		t.Fatalf("TillSoil ID = %d, ok=%v，想要 13, true", id, ok)
	}
	packet, ok := ClientPacketForID(StatePlay, 13)
	if !ok {
		t.Fatal("Play client packet ID 13 未注册")
	}
	if _, isTill := packet.(TillSoil); !isTill {
		t.Fatalf("Play client packet ID 13 = %T，想要 TillSoil", packet)
	}
	// 14 已由骨粉催熟占用；15 由格子工作台的 `TakeCraftingOutput` 占用，
	// 保持「相邻编号不被静默占用」的门禁语义。
	if packet, ok := ClientPacketForID(StatePlay, 14); !ok {
		t.Fatal("Play client packet ID 14 必须已分配给 BoneMeal")
	} else if _, isMeal := packet.(BoneMeal); !isMeal {
		t.Fatalf("Play client packet ID 14 = %T，想要 BoneMeal", packet)
	}
	if packet, ok := ClientPacketForID(StatePlay, 15); !ok {
		t.Fatal("Play client packet ID 15 必须保持分配给 TakeCraftingOutput")
	} else if _, isTake := packet.(TakeCraftingOutput); !isTake {
		t.Fatalf("Play client packet ID 15 = %T，想要 TakeCraftingOutput", packet)
	}
	if ProtocolVersion != 32 {
		t.Fatalf("协议版本 = %d，想要 32", ProtocolVersion)
	}
}

func TestProtocolV27BoneMealPacketIDIsFrozen(t *testing.T) {
	id, ok := ClientPacketID(StatePlay, BoneMeal{})
	if !ok || id != 14 {
		t.Fatalf("BoneMeal ID = %d, ok=%v，想要 14, true", id, ok)
	}
	packet, ok := ClientPacketForID(StatePlay, 14)
	if !ok {
		t.Fatal("Play client packet ID 14 未注册")
	}
	if _, isBone := packet.(BoneMeal); !isBone {
		t.Fatalf("Play client packet ID 14 = %T，想要 BoneMeal", packet)
	}
	if _, ok := ClientPacketForID(StatePlay, 14+2); ok {
		t.Fatal("Play client packet ID 16 必须保持未分配")
	}
}

func TestProtocolV2RemotePlayerPacketIDsAreFrozen(t *testing.T) {
	for _, test := range []struct {
		packet ServerPacket
		id     uint32
	}{
		{RemotePlayerSpawn{}, 7},
		{RemotePlayerDespawn{}, 8},
		{RemotePlayerStates{}, 9},
	} {
		gotID, ok := ServerPacketID(StatePlay, test.packet)
		if !ok || gotID != test.id {
			t.Fatalf("server packet %T id=%d ok=%v, want %d true", test.packet, gotID, ok, test.id)
		}
		decoded, ok := ServerPacketForID(StatePlay, test.id)
		if !ok || fmt.Sprintf("%T", decoded) != fmt.Sprintf("%T", test.packet) {
			t.Fatalf("server packet ID %d decoded=%T ok=%v, want %T true", test.id, decoded, ok, test.packet)
		}
	}
}

func TestProtocolV3HotbarPacketIDsAreFrozen(t *testing.T) {
	assertClientRegistry(t, []struct {
		state  State
		packet ClientPacket
		id     uint32
	}{{StatePlay, SelectHotbar{}, 5}})
	assertServerRegistry(t, []struct {
		state  State
		packet ServerPacket
		id     uint32
	}{{StatePlay, InventoryState{}, 10}})
	if _, ok := ClientPacketForID(StatePlay, 1); ok {
		t.Fatal("Play client packet ID 1 必须保持未分配")
	}
}

func TestProtocolV1RegistryRejectsUnknownIDsAndStates(t *testing.T) {
	if _, ok := ClientPacketForID(StateHandshake, 1); ok {
		t.Fatal("unknown handshake client packet ID accepted")
	}
	if _, ok := ServerPacketForID(StatePlay, 26); ok {
		t.Fatal("unknown play server packet ID accepted")
	}
	if _, ok := ClientPacketID(StateLogin, ClientHello{}); ok {
		t.Fatal("wrong-state client packet accepted")
	}
	if _, ok := ServerPacketID(StateLogin, KeepAlive{}); ok {
		t.Fatal("wrong-state server packet accepted")
	}
}

func TestCommandRejectReasonIDsAreFrozen(t *testing.T) {
	reasons := []struct {
		reason RejectReason
		id     uint8
	}{
		{RejectInvalidRay, 1}, {RejectNoTarget, 2}, {RejectChunkNotReady, 3},
		{RejectProtectedBlock, 4}, {RejectInvalidBlock, 5}, {RejectOccupied, 6},
		{RejectInvalidInput, 7}, {RejectPlayerNotReady, 8},
		{RejectInvalidSlot, 9}, {RejectHotbarFull, 10}, {RejectDropCapacity, 11},
		{RejectContainerCapacity, 12},
	}
	for _, tc := range reasons {
		got, ok := CommandRejectReasonID(tc.reason)
		if !ok || got != tc.id {
			t.Fatalf("reason %q ID = %d, ok=%v; want %d, true", tc.reason, got, ok, tc.id)
		}
		decoded, ok := CommandRejectReasonForID(tc.id)
		if !ok || decoded != tc.reason {
			t.Fatalf("ID %d decoded to %q, ok=%v; want %q, true", tc.id, decoded, ok, tc.reason)
		}
	}
	if _, ok := CommandRejectReasonID(RejectReason("unknown")); ok {
		t.Fatal("unknown rejection reason encoded")
	}
	if _, ok := CommandRejectReasonForID(0); ok {
		t.Fatal("zero rejection reason ID decoded")
	}
	if _, ok := CommandRejectReasonForID(13); ok {
		t.Fatal("unknown rejection reason ID decoded")
	}
}

func assertClientRegistry(t *testing.T, packets []struct {
	state  State
	packet ClientPacket
	id     uint32
}) {
	t.Helper()
	for _, tc := range packets {
		got, ok := ClientPacketID(tc.state, tc.packet)
		if !ok || got != tc.id {
			t.Fatalf("client packet %T in state %d ID = %d, ok=%v; want %d, true", tc.packet, tc.state, got, ok, tc.id)
		}
		decoded, ok := ClientPacketForID(tc.state, tc.id)
		if !ok || !sameClientPacketType(decoded, tc.packet) {
			t.Fatalf("client packet ID %d in state %d decodes to %T, ok=%v; want %T, true", tc.id, tc.state, decoded, ok, tc.packet)
		}
	}
}

func assertServerRegistry(t *testing.T, packets []struct {
	state  State
	packet ServerPacket
	id     uint32
}) {
	t.Helper()
	for _, tc := range packets {
		got, ok := ServerPacketID(tc.state, tc.packet)
		if !ok || got != tc.id {
			t.Fatalf("server packet %T in state %d ID = %d, ok=%v; want %d, true", tc.packet, tc.state, got, ok, tc.id)
		}
		decoded, ok := ServerPacketForID(tc.state, tc.id)
		if !ok || !sameServerPacketType(decoded, tc.packet) {
			t.Fatalf("server packet ID %d in state %d decodes to %T, ok=%v; want %T, true", tc.id, tc.state, decoded, ok, tc.packet)
		}
	}
}

func sameClientPacketType(left, right ClientPacket) bool {
	switch left.(type) {
	case ClientHello:
		_, ok := right.(ClientHello)
		return ok
	case LoginStart:
		_, ok := right.(LoginStart)
		return ok
	case PlayerInput:
		_, ok := right.(PlayerInput)
		return ok
	case PlaceBlock:
		_, ok := right.(PlaceBlock)
		return ok
	case RequestChunkResync:
		_, ok := right.(RequestChunkResync)
		return ok
	case KeepAliveReply:
		_, ok := right.(KeepAliveReply)
		return ok
	case SelectHotbar:
		_, ok := right.(SelectHotbar)
		return ok
	case MoveInventoryStack:
		_, ok := right.(MoveInventoryStack)
		return ok
	case OpenContainer:
		_, ok := right.(OpenContainer)
		return ok
	case MoveContainerStack:
		_, ok := right.(MoveContainerStack)
		return ok
	case CloseContainer:
		_, ok := right.(CloseContainer)
		return ok
	case DropSelectedItem:
		_, ok := right.(DropSelectedItem)
		return ok
	case ChatCommand:
		_, ok := right.(ChatCommand)
		return ok
	case MoveCraftingStack:
		_, ok := right.(MoveCraftingStack)
		return ok
	case TakeCraftingOutput:
		_, ok := right.(TakeCraftingOutput)
		return ok
	case TillSoil:
		_, ok := right.(TillSoil)
		return ok
	case BoneMeal:
		_, ok := right.(BoneMeal)
		return ok
	}
	return false
}

func sameServerPacketType(left, right ServerPacket) bool {
	switch left.(type) {
	case ServerHello:
		_, ok := right.(ServerHello)
		return ok
	case HandshakeReject:
		_, ok := right.(HandshakeReject)
		return ok
	case LoginSuccess:
		_, ok := right.(LoginSuccess)
		return ok
	case LoginReject:
		_, ok := right.(LoginReject)
		return ok
	case ChunkSnapshot:
		_, ok := right.(ChunkSnapshot)
		return ok
	case BlockChanges:
		_, ok := right.(BlockChanges)
		return ok
	case ForgetChunks:
		_, ok := right.(ForgetChunks)
		return ok
	case PlayerState:
		_, ok := right.(PlayerState)
		return ok
	case CommandRejected:
		_, ok := right.(CommandRejected)
		return ok
	case PlaceBlockSucceeded:
		_, ok := right.(PlaceBlockSucceeded)
		return ok
	case KeepAlive:
		_, ok := right.(KeepAlive)
		return ok
	case Disconnect:
		_, ok := right.(Disconnect)
		return ok
	case RemotePlayerSpawn:
		_, ok := right.(RemotePlayerSpawn)
		return ok
	case RemotePlayerDespawn:
		_, ok := right.(RemotePlayerDespawn)
		return ok
	case RemotePlayerStates:
		_, ok := right.(RemotePlayerStates)
		return ok
	case InventoryState:
		_, ok := right.(InventoryState)
		return ok
	case ItemDropUpserts:
		_, ok := right.(ItemDropUpserts)
		return ok
	case ItemDropRemoves:
		_, ok := right.(ItemDropRemoves)
		return ok
	case FurnaceState:
		_, ok := right.(FurnaceState)
		return ok
	case ChestState:
		_, ok := right.(ChestState)
		return ok
	case ContainerClosed:
		_, ok := right.(ContainerClosed)
		return ok
	case ChatEvent:
		_, ok := right.(ChatEvent)
		return ok
	case CompanionSpawn:
		_, ok := right.(CompanionSpawn)
		return ok
	case CompanionStates:
		_, ok := right.(CompanionStates)
		return ok
	case CompanionDespawn:
		_, ok := right.(CompanionDespawn)
		return ok
	case CraftingState:
		_, ok := right.(CraftingState)
		return ok
	case HostileSpawn:
		_, ok := right.(HostileSpawn)
		return ok
	case HostileState:
		_, ok := right.(HostileState)
		return ok
	case HostileDespawn:
		_, ok := right.(HostileDespawn)
		return ok
	case CombatHit:
		_, ok := right.(CombatHit)
		return ok
	}
	return false
}
