package codec

import (
	"encoding/hex"
	"github.com/channing771/mornlea/internal/network/protocol"
	"reflect"
	"testing"

	"github.com/channing771/mornlea/internal/core"
)

func testChestRef() core.ContainerRef {
	return core.ContainerRef{
		Dimension:  core.Overworld,
		Chunk:      core.ChunkPos{X: -3, Z: 7},
		Kind:       core.ContainerKindChest,
		Slot:       5,
		Generation: 9,
	}
}

func testChestItems() [core.ChestSlots]core.ItemStack {
	var items [core.ChestSlots]core.ItemStack
	items[0] = core.ItemStack{Item: core.ItemStone, Count: 5}
	return items
}

// chestStateGoldenHex 手工拼出与 encodeContainerRef/encodeItemStack 完全对应的固定字节，
// 用于捕获字段顺序、宽度或字节序上的静默漂移。
func chestStateGoldenHex() string {
	ref := "00000000" + "fdffffff" + "07000000" + "01" + "05" + "09000000"
	hex := ref + "0100050000"
	for range core.ChestSlots - 1 {
		hex += "0000000000"
	}
	return hex
}

// TestProtocolV12ChestStateGolden 覆盖 protocol.ChestState 的固定 153 字节布局：
// 18 字节容器引用加 27 × 5 字节格子，并验证截断与尾随字节都被拒绝。
func TestProtocolV12ChestStateGolden(t *testing.T) {
	packet := protocol.ChestState{Chest: testChestRef(), Items: testChestItems()}
	wantHex := chestStateGoldenHex()

	packetID, payload, err := encodeServerControlPayload(protocol.StatePlay, packet)
	if err != nil || packetID != 15 || len(payload) != 153 || hex.EncodeToString(payload) != wantHex {
		t.Fatalf("protocol.ChestState id=%d len=%d payload=%x err=%v，想要 id=15 len=153 hex=%s",
			packetID, len(payload), payload, err, wantHex)
	}
	decoded, err := decodeServerControlPayload(protocol.StatePlay, packetID, payload)
	if err != nil || !reflect.DeepEqual(decoded, packet) {
		t.Fatalf("round=%#v err=%v", decoded, err)
	}
	for length := 0; length < len(payload); length++ {
		if _, err := decodeServerControlPayload(protocol.StatePlay, packetID, payload[:length]); err == nil {
			t.Fatalf("截断到 %d 字节仍被接受", length)
		}
	}
	if _, err := decodeServerControlPayload(
		protocol.StatePlay, packetID, append(append([]byte(nil), payload...), 0),
	); err == nil {
		t.Fatal("尾随字节被接受")
	}
}

// TestChestStateRejectsInvalidValues 覆盖箱子专属拒绝路径：非箱子种类的引用、
// 越界统一索引之外的非法格与非箱子种类的整堆移动统一索引上限。
func TestChestStateRejectsInvalidValues(t *testing.T) {
	ref := testChestRef()
	invalidStack := core.ItemStack{Item: core.ItemID(0xffff), Count: 1}

	states := []protocol.ChestState{
		// Kind 是熔炉（零值）而不是箱子。
		{Chest: core.ContainerRef{Dimension: core.Overworld, Generation: 1}},
		// Kind 是既不是熔炉也不是箱子的未知值。
		{Chest: core.ContainerRef{Dimension: core.Overworld, Kind: core.ContainerKind(7), Generation: 1}},
		// 槽位越界（ChestsPerChunk = 16）。
		{Chest: core.ContainerRef{
			Dimension: core.Overworld, Kind: core.ContainerKindChest,
			Slot: core.ChestsPerChunk, Generation: 1,
		}},
		// generation 为 0 的引用永远不可能有效。
		{Chest: core.ContainerRef{Dimension: core.Overworld, Kind: core.ContainerKindChest}},
		// 非 Overworld 维度。
		{Chest: core.ContainerRef{Dimension: core.DimensionID(1), Kind: core.ContainerKindChest, Generation: 1}},
		// 非法格子内容。
		{Chest: ref, Items: func() [core.ChestSlots]core.ItemStack {
			var items [core.ChestSlots]core.ItemStack
			items[3] = invalidStack
			return items
		}()},
	}
	for _, state := range states {
		if _, _, err := encodeServerControlPayload(protocol.StatePlay, state); err == nil {
			t.Fatalf("非法箱子状态被编码: %#v", state)
		}
	}
}
