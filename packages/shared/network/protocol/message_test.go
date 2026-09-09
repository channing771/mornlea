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

func TestPlayerChunkMessagesAcceptDepths(t *testing.T) {
	// 玩家与区块类消息的 `Dimension` 接受主世界与 `Depths`（0 与 1），
	// `Dimension >= 2` 一律拒绝；伙伴/敌怪/被动生物类消息不在此列，
	// 它们继续只接受主世界（由既有拒绝矩阵覆盖）。
	validID := core.PlayerID{0, 1, 2, 3, 4, 5, 0x46, 7, 0x88, 9, 10, 11, 12, 13, 14, 15}
	depthsSections := make([]protocol.SectionData, core.SectionsPerChunk)
	for index := range depthsSections {
		depthsSections[index] = protocol.SectionData{Y: int32(index), Storage: protocol.SectionSingle, Single: core.AirID}
	}
	valid := []struct {
		name    string
		message interface{ Validate() error }
	}{
		{"resync", protocol.RequestChunkResync{Dimension: core.Depths}},
		{"snapshot", protocol.ChunkSnapshot{Dimension: core.Depths, Revision: 1, Sections: depthsSections}},
		{"block changes", protocol.BlockChanges{Dimension: core.Depths, BaseRevision: 1, NewRevision: 2}},
		{"forget chunks", protocol.ForgetChunks{Dimension: core.Depths, Chunks: []core.ChunkPos{{}}}},
		{"player state", protocol.PlayerState{Dimension: core.Depths}},
		{"remote spawn", protocol.RemotePlayerSpawn{PlayerID: validID, DisplayName: "Chen", Dimension: core.Depths}},
		{"remote states", protocol.RemotePlayerStates{Players: []protocol.RemotePlayerState{{PlayerID: validID, Dimension: core.Depths}}}},
	}
	for _, tc := range valid {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.message.Validate(); err != nil {
				t.Fatalf("depths %s 被拒绝: %v", tc.name, err)
			}
		})
	}

	rejected := []struct {
		name    string
		message interface{ Validate() error }
	}{
		{"resync", protocol.RequestChunkResync{Dimension: core.DimensionID(2)}},
		{"snapshot", protocol.ChunkSnapshot{Dimension: core.DimensionID(2), Revision: 1, Sections: depthsSections}},
		{"block changes", protocol.BlockChanges{Dimension: core.DimensionID(2), BaseRevision: 1, NewRevision: 2}},
		{"forget chunks", protocol.ForgetChunks{Dimension: core.DimensionID(2), Chunks: []core.ChunkPos{{}}}},
		{"player state", protocol.PlayerState{Dimension: core.DimensionID(2)}},
		{"remote spawn", protocol.RemotePlayerSpawn{PlayerID: validID, DisplayName: "Chen", Dimension: core.DimensionID(2)}},
		{"remote states", protocol.RemotePlayerStates{Players: []protocol.RemotePlayerState{{PlayerID: validID, Dimension: core.DimensionID(2)}}}},
	}
	for _, tc := range rejected {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.message.Validate(); err == nil {
				t.Fatalf("dimension 2 %s 被接受", tc.name)
			}
		})
	}
}

func TestBucketCommandIDsAppendOnly(t *testing.T) {
	if id, ok := protocol.ClientPacketID(protocol.StatePlay, protocol.CollectWater{}); !ok || id != 16 {
		t.Fatalf("CollectWater ID = (%d,%v)，想要 (16,true)", id, ok)
	}
	if id, ok := protocol.ClientPacketID(protocol.StatePlay, protocol.PlaceWater{}); !ok || id != 17 {
		t.Fatalf("PlaceWater ID = (%d,%v)，想要 (17,true)", id, ok)
	}
	if protocol.ProtocolVersion != 39 {
		t.Fatalf("ProtocolVersion = %d，想要 39", protocol.ProtocolVersion)
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
		{protocol.RejectContainerCapacity, "container_capacity"},
		{protocol.RejectNotFluidSource, "not_fluid_source"},
		{protocol.RejectBucketMismatch, "bucket_mismatch"},
	}
	for _, tc := range tests {
		if string(tc.got) != tc.want {
			t.Fatalf("reject reason = %q，想要 %q", tc.got, tc.want)
		}
	}
}
