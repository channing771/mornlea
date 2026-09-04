package codec

import (
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

func dropTestID(slot uint8, generation uint32) core.DropID {
	return core.DropID{
		Dimension:  core.Overworld,
		Chunk:      core.ChunkPos{X: 1, Z: -2},
		Slot:       slot,
		Generation: generation,
	}
}

func dropTestUpsert(slot uint8, generation uint32, count uint8) protocol.ItemDrop {
	return protocol.ItemDrop{
		ID: dropTestID(slot, generation), BlockIndex: 9,
		Item: core.ItemStone, Count: count,
	}
}

func TestProtocolV4DropGolden(t *testing.T) {
	tests := []struct {
		packet  protocol.ServerPacket
		wantID  uint32
		wantHex string
	}{
		{
			protocol.ItemDropUpserts{ServerTick: 5, Drops: []protocol.ItemDrop{dropTestUpsert(3, 7, 4)}},
			11,
			"05000000000000000100000000010000" + "00feffffff0307000000090000000100" + "040000",
		},
		{
			protocol.ItemDropRemoves{ServerTick: 6, IDs: []core.DropID{dropTestID(3, 7)}},
			12,
			"06000000000000000100000000010000" + "00feffffff0307000000",
		},
	}
	for _, test := range tests {
		packetID, payload, err := encodeServerControlPayload(protocol.StatePlay, test.packet)
		if err != nil || packetID != test.wantID || hex.EncodeToString(payload) != test.wantHex {
			t.Fatalf("%T id=%d payload=%x err=%v", test.packet, packetID, payload, err)
		}
		decoded, err := decodeServerControlPayload(protocol.StatePlay, packetID, payload)
		if err != nil || !reflect.DeepEqual(decoded, test.packet) {
			t.Fatalf("round=%#v err=%v", decoded, err)
		}
		for length := 0; length < len(payload); length++ {
			if _, err := decodeServerControlPayload(protocol.StatePlay, packetID, payload[:length]); err == nil {
				t.Fatalf("truncated %T at %d accepted", test.packet, length)
			}
		}
		if _, err := decodeServerControlPayload(
			protocol.StatePlay, packetID, append(append([]byte(nil), payload...), 0),
		); err == nil {
			t.Fatalf("%T trailing byte accepted", test.packet)
		}
	}
}

func TestProtocolV16ToolDropUsesFiveByteStackWire(t *testing.T) {
	if protocol.ProtocolVersion != 33 {
		t.Fatalf("协议版本 = %d，想要 33", protocol.ProtocolVersion)
	}
	full, _ := core.ItemMaxDurability(core.ItemStonePickaxe)
	packet := protocol.ItemDropUpserts{ServerTick: 5, Drops: []protocol.ItemDrop{{
		ID: dropTestID(3, 7), BlockIndex: 9,
		Item: core.ItemStonePickaxe, Count: 1, Durability: full - 7,
	}}}
	packetID, payload, err := encodeServerControlPayload(protocol.StatePlay, packet)
	if err != nil {
		t.Fatalf("编码磨损工具掉落物: %v", err)
	}
	if packetID != 11 || len(payload) != 8+1+itemDropWireBytes {
		t.Fatalf("v11 工具掉落物 id=%d wire=%d，想要 id=11 wire=%d", packetID, len(payload), 8+1+itemDropWireBytes)
	}
	decoded, err := decodeServerControlPayload(protocol.StatePlay, packetID, payload)
	if err != nil || !reflect.DeepEqual(decoded, packet) {
		t.Fatalf("v11 工具掉落物往返 = %#v，error=%v，想要 %#v", decoded, err, packet)
	}
}

func TestItemDropDecodeRejectsOversizedCountBeforeAllocation(t *testing.T) {
	// 计数 33 超过固定上限，且剩余字节不足以支撑该计数。
	oversized := append([]byte{6, 0, 0, 0, 0, 0, 0, 0}, 33)
	for _, packetID := range []uint32{11, 12} {
		if _, err := decodeServerControlPayload(protocol.StatePlay, packetID, oversized); err == nil {
			t.Fatalf("packet %d 接受了超限计数", packetID)
		}
	}
}

func TestBlockChangesAllowZeroChangeRevisionBarrier(t *testing.T) {
	barrier := protocol.BlockChanges{
		Dimension: core.Overworld, Chunk: core.ChunkPos{X: 1, Z: -2},
		BaseRevision: 4, NewRevision: 5,
	}
	if err := barrier.Validate(); err != nil {
		t.Fatalf("零方块 revision barrier 被拒绝: %v", err)
	}
	packetID, payload, err := encodeServerControlPayload(protocol.StatePlay, barrier)
	if err != nil {
		t.Fatalf("编码 barrier: %v", err)
	}
	decoded, err := decodeServerControlPayload(protocol.StatePlay, packetID, payload)
	if err != nil {
		t.Fatalf("解码 barrier: %v", err)
	}
	got, ok := decoded.(protocol.BlockChanges)
	if !ok || got.BaseRevision != 4 || got.NewRevision != 5 || len(got.Changes) != 0 {
		t.Fatalf("barrier 往返 = %#v", decoded)
	}
}

func TestItemDropUpsertsAcceptEveryRegisteredItem(t *testing.T) {
	for _, item := range []core.ItemID{
		core.ItemCoal,
		core.ItemRawIron,
		core.ItemIronIngot,
		core.ItemStonePickaxe,
		core.ItemIronPickaxe,
	} {
		durability, _ := core.ItemMaxDurability(item)
		message := protocol.ItemDropUpserts{Drops: []protocol.ItemDrop{{
			ID: dropTestID(0, 1), BlockIndex: 9, Item: item, Count: 1, Durability: durability,
		}}}
		if err := message.Validate(); err != nil {
			t.Fatalf("已注册物品 %d 被拒绝: %v", item, err)
		}
		packetID, payload, err := encodeServerControlPayload(protocol.StatePlay, message)
		if err != nil {
			t.Fatalf("编码已注册物品 %d: %v", item, err)
		}
		if _, err := decodeServerControlPayload(protocol.StatePlay, packetID, payload); err != nil {
			t.Fatalf("解码已注册物品 %d: %v", item, err)
		}
	}
}
