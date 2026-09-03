package codec

import (
	"github.com/channing771/mornlea/packages/shared/network/protocol"
	"math"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

func testFurnaceRef() core.FurnaceRef {
	return core.FurnaceRef{
		Dimension:  core.Overworld,
		Chunk:      core.ChunkPos{X: -3, Z: 7},
		Slot:       5,
		Generation: 9,
	}
}

func TestProtocolV12ContainerPayloadsAreFixedLength(t *testing.T) {
	ref := testFurnaceRef()
	clients := []struct {
		name   string
		packet protocol.ClientPacket
		bytes  int
	}{
		{"open", protocol.OpenContainer{Sequence: 3, Yaw: 1.5, Pitch: -0.5}, 16},
		{"move", protocol.MoveContainerStack{
			Sequence: 4, Container: ref, From: 0, To: core.FurnaceInputSlot,
		}, 28},
		{"close", protocol.CloseContainer{Sequence: 5}, 8},
	}
	for _, tc := range clients {
		t.Run(tc.name, func(t *testing.T) {
			_, payload, err := encodeClientPacketPayload(protocol.StatePlay, tc.packet)
			if err != nil || len(payload) != tc.bytes {
				t.Fatalf("%T payload=%d err=%v，想要 %d 字节", tc.packet, len(payload), err, tc.bytes)
			}
			for length := range len(payload) {
				id, _ := protocol.ClientPacketID(protocol.StatePlay, tc.packet)
				if _, err := decodeClientPacketPayload(protocol.StatePlay, id, payload[:length]); err == nil {
					t.Fatalf("截断到 %d 字节仍被接受", length)
				}
			}
			id, _ := protocol.ClientPacketID(protocol.StatePlay, tc.packet)
			trailing := append(append([]byte(nil), payload...), 0)
			if _, err := decodeClientPacketPayload(protocol.StatePlay, id, trailing); err == nil {
				t.Fatal("尾随字节被接受")
			}
		})
	}

	servers := []struct {
		name   string
		packet protocol.ServerPacket
		bytes  int
	}{
		{"state", protocol.FurnaceState{
			Furnace:       ref,
			Input:         core.ItemStack{Item: core.ItemRawIron, Count: 7},
			Fuel:          core.ItemStack{Item: core.ItemCoal, Count: 2},
			Output:        core.ItemStack{Item: core.ItemIronIngot, Count: 5},
			ProgressTicks: 137, BurnTicks: 1463,
		}, 36},
		{"closed", protocol.ContainerClosed{Container: ref}, 18},
	}
	for _, tc := range servers {
		t.Run(tc.name, func(t *testing.T) {
			id, payload, err := encodeServerControlPayload(protocol.StatePlay, tc.packet)
			if err != nil || len(payload) != tc.bytes {
				t.Fatalf("%T payload=%d err=%v，想要 %d 字节", tc.packet, len(payload), err, tc.bytes)
			}
			round, err := decodeServerControlPayload(protocol.StatePlay, id, payload)
			if err != nil || round != tc.packet {
				t.Fatalf("round=%#v err=%v", round, err)
			}
			for length := range len(payload) {
				if _, err := decodeServerControlPayload(protocol.StatePlay, id, payload[:length]); err == nil {
					t.Fatalf("截断到 %d 字节仍被接受", length)
				}
			}
		})
	}
}

func TestFurnaceMessagesRoundTripMaterials(t *testing.T) {
	ref := testFurnaceRef()
	states := []protocol.FurnaceState{
		{Furnace: ref, Input: core.ItemStack{Item: core.ItemRawIron, Count: 3}, Output: core.ItemStack{Item: core.ItemIronIngot, Count: 2}},
		{Furnace: ref, Input: core.ItemStack{Item: core.ItemSand, Count: 4}, Fuel: core.ItemStack{Item: core.ItemCoal, Count: 1}, Output: core.ItemStack{Item: core.ItemGlass, Count: 2}},
		{Furnace: ref, Input: core.ItemStack{Item: core.ItemClay, Count: 5}, Output: core.ItemStack{Item: core.ItemBrick, Count: 2}},
		// 输入耗尽或切换后，已注册产物仍可留在输出格。
		{Furnace: ref, Output: core.ItemStack{Item: core.ItemBrick, Count: 1}},
		{Furnace: ref, Input: core.ItemStack{Item: core.ItemSand, Count: 1}, Output: core.ItemStack{Item: core.ItemIronIngot, Count: 1}},
	}
	for _, state := range states {
		id, payload, err := encodeServerControlPayload(protocol.StatePlay, state)
		if err != nil {
			t.Fatalf("编码 protocol.FurnaceState %+v: %v", state, err)
		}
		round, err := decodeServerControlPayload(protocol.StatePlay, id, payload)
		if err != nil || round != state {
			t.Fatalf("protocol.FurnaceState round-trip = %#v, %v，想要 %#v", round, err, state)
		}
	}
}

func TestFurnaceMessagesRejectInvalidValues(t *testing.T) {
	ref := testFurnaceRef()
	clients := []protocol.ClientPacket{
		protocol.OpenContainer{Yaw: float32(math.NaN())},
		protocol.OpenContainer{Pitch: float32(math.Inf(1))},
		protocol.MoveContainerStack{Container: ref, From: core.FurnaceViewSlots, To: 0},
		protocol.MoveContainerStack{Container: ref, From: 0, To: core.FurnaceViewSlots},
		protocol.MoveContainerStack{Container: ref, From: 3, To: 3},
		// 输出格只能作为来源。
		protocol.MoveContainerStack{Container: ref, From: 0, To: core.FurnaceOutputSlot},
		// generation 为 0 的引用永远不可能有效。
		protocol.MoveContainerStack{Container: core.FurnaceRef{Dimension: core.Overworld}, From: 0, To: 36},
		// 非 Overworld 维度。
		protocol.MoveContainerStack{
			Container: core.FurnaceRef{Dimension: core.DimensionID(1), Generation: 1},
			From:      0, To: 36,
		},
		// 槽位越界。
		protocol.MoveContainerStack{
			Container: core.FurnaceRef{
				Dimension: core.Overworld, Slot: core.FurnacesPerChunk, Generation: 1,
			},
			From: 0, To: 36,
		},
		// 未知容器种类。
		protocol.MoveContainerStack{
			Container: core.ContainerRef{
				Dimension: core.Overworld, Kind: core.ContainerKind(2), Generation: 1,
			},
			From: 0, To: 36,
		},
	}
	for _, packet := range clients {
		if _, _, err := encodeClientPacketPayload(protocol.StatePlay, packet); err == nil {
			t.Fatalf("非法客户端熔炉消息被编码: %#v", packet)
		}
	}

	servers := []protocol.ServerPacket{
		protocol.FurnaceState{Furnace: core.FurnaceRef{Dimension: core.Overworld}},
		protocol.FurnaceState{Furnace: ref, Input: core.ItemStack{Item: core.ItemCoal, Count: 1}},
		protocol.FurnaceState{Furnace: ref, Fuel: core.ItemStack{Item: core.ItemRawIron, Count: 1}},
		protocol.FurnaceState{Furnace: ref, Output: core.ItemStack{Item: core.ItemSand, Count: 1}},
		protocol.FurnaceState{Furnace: ref, Input: core.ItemStack{Item: core.ItemRawIron, Count: 1, Durability: 1}},
		protocol.FurnaceState{Furnace: ref, Output: core.ItemStack{Item: core.ItemGlass, Count: 1, Durability: 1}},
		protocol.FurnaceState{Furnace: ref, Fuel: core.ItemStack{Durability: 1}},
		protocol.FurnaceState{Furnace: ref, Input: core.ItemStack{Item: core.ItemID(math.MaxUint16), Count: 1}},
		protocol.FurnaceState{Furnace: ref, Output: core.ItemStack{Item: core.ItemID(math.MaxUint16), Count: 1}},
		protocol.FurnaceState{Furnace: ref, Input: core.ItemStack{Item: core.ItemSand}},
		protocol.FurnaceState{Furnace: ref, Output: core.ItemStack{Item: core.ItemBrick, Count: core.MaxStackCount + 1}},
		protocol.FurnaceState{Furnace: ref, Input: core.ItemStack{Count: 1}},
		protocol.FurnaceState{Furnace: ref, Output: core.ItemStack{Durability: 1}},
		protocol.FurnaceState{Furnace: ref, ProgressTicks: core.FurnaceSmeltTicks},
		protocol.FurnaceState{Furnace: ref, BurnTicks: core.FurnaceBurnTicks + 1},
		// 熔炉专属消息必须拒绝箱子种类的引用。
		protocol.FurnaceState{Furnace: core.ContainerRef{
			Dimension: core.Overworld, Kind: core.ContainerKindChest, Slot: 1, Generation: 1,
		}},
		protocol.ContainerClosed{Container: core.FurnaceRef{Dimension: core.Overworld}},
		protocol.ContainerClosed{Container: core.ContainerRef{
			Dimension: core.Overworld, Kind: core.ContainerKind(2), Generation: 1,
		}},
	}
	for _, packet := range servers {
		if _, _, err := encodeServerControlPayload(protocol.StatePlay, packet); err == nil {
			t.Fatalf("非法服务端熔炉消息被编码: %#v", packet)
		}
	}
}

func TestFurnaceDecodeRejectsUnknownWireValues(t *testing.T) {
	ref := testFurnaceRef()
	moveID, _ := protocol.ClientPacketID(protocol.StatePlay, protocol.MoveContainerStack{})
	_, payload, err := encodeClientPacketPayload(protocol.StatePlay, protocol.MoveContainerStack{
		Sequence: 1, Container: ref, From: 0, To: core.FurnaceInputSlot,
	})
	if err != nil {
		t.Fatal(err)
	}
	// 最后两个字节是 from/to；改成越界值必须被拒绝。
	corrupted := append([]byte(nil), payload...)
	corrupted[len(corrupted)-1] = core.FurnaceViewSlots
	if _, err := decodeClientPacketPayload(protocol.StatePlay, moveID, corrupted); err == nil {
		t.Fatal("越界目标索引被接受")
	}

	stateID, _ := protocol.ServerPacketID(protocol.StatePlay, protocol.FurnaceState{})
	_, statePayload, err := encodeServerControlPayload(protocol.StatePlay, protocol.FurnaceState{
		Furnace: ref, ProgressTicks: 10, BurnTicks: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	badProgress := append([]byte(nil), statePayload...)
	badProgress[len(badProgress)-3] = core.FurnaceSmeltTicks
	if _, err := decodeServerControlPayload(protocol.StatePlay, stateID, badProgress); err == nil {
		t.Fatal("越界进度被接受")
	}
}
