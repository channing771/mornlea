package protocol_test

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/companion"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

func TestProtocolMessageShapesImplementSealedInterfaces(t *testing.T) {
	clientMessages := []protocol.ClientMessage{
		protocol.PlayerInput{
			Sequence: 1,
			MoveX:    -1,
			MoveZ:    1,
			Jump:     true,
			Yaw:      90,
			Pitch:    -15,
			Mining:   true,
		},
		protocol.PlaceBlock{
			Sequence: 3,
			Yaw:      90,
			Pitch:    -15,
			Slot:     4,
		},
		protocol.SelectHotbar{Sequence: 9, Slot: 8},
		protocol.MoveInventoryStack{Sequence: 10, From: 0, To: 35},
		protocol.MoveCraftingStack{Sequence: 11, From: 9, To: 0},
		protocol.RequestChunkResync{
			Sequence:     4,
			Dimension:    core.Overworld,
			Chunk:        core.ChunkPos{X: 2, Z: -3},
			HaveRevision: 7,
		},
		protocol.KeepAliveReply{Token: 1},
		protocol.ChatCommand{Text: "@A x"},
	}
	serverMessages := []protocol.ServerMessage{
		protocol.ChunkSnapshot{},
		protocol.BlockChanges{},
		protocol.ForgetChunks{},
		protocol.CommandRejected{
			Sequence: 4,
			Reason:   protocol.RejectInvalidRay,
		},
		protocol.PlayerState{
			ServerTick:          8,
			LastInputSequence:   7,
			Dimension:           core.Overworld,
			Position:            mgl32.Vec3{1, 2, 3},
			Velocity:            mgl32.Vec3{4, 5, 6},
			Yaw:                 90,
			Pitch:               -15,
			OnGround:            true,
			Ready:               true,
			Reset:               true,
			MiningActive:        true,
			MiningTarget:        core.BlockPos{X: 1, Y: 2, Z: 3},
			MiningProgressTicks: 6,
			MiningRequiredTicks: 15,
			MiningHarvestable:   true,
		},
		protocol.RemotePlayerSpawn{PlayerID: core.PlayerID{0, 1, 2, 3, 4, 5, 0x46, 7, 0x88, 9, 10, 11, 12, 13, 14, 15}, DisplayName: "Chen"},
		protocol.RemotePlayerDespawn{PlayerID: core.PlayerID{0, 1, 2, 3, 4, 5, 0x46, 7, 0x88, 9, 10, 11, 12, 13, 14, 15}},
		protocol.RemotePlayerStates{Players: []protocol.RemotePlayerState{{PlayerID: core.PlayerID{0, 1, 2, 3, 4, 5, 0x46, 7, 0x88, 9, 10, 11, 12, 13, 14, 15}}}},
		protocol.KeepAlive{Token: 1},
		protocol.Disconnect{Code: protocol.DisconnectTimeout},
		protocol.InventoryState{},
		protocol.ItemDropUpserts{},
		protocol.ItemDropRemoves{},
		protocol.ChatEvent{},
		protocol.CompanionSpawn{},
		protocol.CompanionStates{},
		protocol.CompanionDespawn{ID: companion.ID{}},
		protocol.PlaceBlockSucceeded{Sequence: 1},
	}
	if len(clientMessages) != 8 || len(serverMessages) != 18 {
		t.Fatal("消息集合不完整")
	}
}

func TestHotbarMessagesValidateFixedBounds(t *testing.T) {
	var hotbar core.Hotbar
	hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: core.MaxStackCount}
	valid := []interface{ Validate() error }{
		protocol.PlaceBlock{Slot: core.HotbarSlots - 1},
		protocol.SelectHotbar{Slot: 0},
		protocol.InventoryState{Inventory: core.Inventory{Hotbar: hotbar}},
	}
	for _, message := range valid {
		if err := message.Validate(); err != nil {
			t.Fatalf("%T 合法值被拒绝: %v", message, err)
		}
	}

	overflow := hotbar
	overflow.Slots[1] = core.ItemStack{Item: core.ItemDirt, Count: core.MaxStackCount + 1}
	unknown := hotbar
	unknown.Slots[2] = core.ItemStack{Item: core.ItemID(4242), Count: 1}
	ghost := hotbar
	ghost.Slots[3] = core.ItemStack{Item: core.ItemNone, Count: 1}
	selected := hotbar
	selected.Selected = core.HotbarSlots
	invalid := []interface{ Validate() error }{
		protocol.PlaceBlock{Slot: core.HotbarSlots},
		protocol.SelectHotbar{Slot: 255},
		protocol.InventoryState{Inventory: core.Inventory{Hotbar: overflow}},
		protocol.InventoryState{Inventory: core.Inventory{Hotbar: unknown}},
		protocol.InventoryState{Inventory: core.Inventory{Hotbar: ghost}},
		protocol.InventoryState{Inventory: core.Inventory{Hotbar: selected}},
	}
	for _, message := range invalid {
		if err := message.Validate(); err == nil {
			t.Fatalf("%T 非法值被接受: %+v", message, message)
		}
	}
}

func TestRejectReasonsAreStableProtocolValues(t *testing.T) {
	tests := []struct {
		got  protocol.RejectReason
		want string
	}{
		{protocol.RejectInvalidRay, "invalid_ray"},
		{protocol.RejectNoTarget, "no_target"},
		{protocol.RejectChunkNotReady, "chunk_not_ready"},
		{protocol.RejectProtectedBlock, "protected_block"},
		{protocol.RejectInvalidBlock, "invalid_block"},
		{protocol.RejectOccupied, "occupied"},
		{protocol.RejectInvalidInput, "invalid_input"},
		{protocol.RejectPlayerNotReady, "player_not_ready"},
		{protocol.RejectInvalidSlot, "invalid_slot"},
		{protocol.RejectHotbarFull, "hotbar_full"},
		{protocol.RejectDropCapacity, "drop_capacity"},
	}
	for _, tc := range tests {
		if string(tc.got) != tc.want {
			t.Fatalf("reject reason = %q，想要 %q", tc.got, tc.want)
		}
	}
}
