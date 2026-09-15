package codec

import (
	"encoding/hex"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

// dropStackWire 手工构造整组丢弃载荷，绕过编码器校验注入非法值域。
func dropStackWire(ref core.ContainerRef, view, slot uint8) []byte {
	var encoder byteEncoder
	encoder.u64(1)
	encodeContainerRef(&encoder, ref)
	encoder.u8(view)
	encoder.u8(slot)
	return encoder.data
}

// TestDropStackGoldenWireLayout 覆盖整组丢弃命令的冻结布局：u64 序号 +
// 18 字节容器引用 + u8 视图 + u8 统一索引的固定 28 字节。夹具取箱子引用并让
// 视图与索引取非零中间值，任何换位、漏写或宽度漂移都会改变期望字节；
// 截断与尾随字节都必须整包拒绝。
func TestDropStackGoldenWireLayout(t *testing.T) {
	drop := protocol.DropStack{
		Sequence: 21, Container: testChestRef(), View: protocol.StackViewContainer, Slot: 62,
	}
	dropID, dropPayload, err := encodeClientPacketPayload(protocol.StatePlay, drop)
	if err != nil || dropID != 21 || len(dropPayload) != 28 ||
		hex.EncodeToString(dropPayload) != "1500000000000000"+stackSplittingChestHex+"023e" {
		t.Fatalf("整组丢弃 id=%d payload=%x err=%v，想要 id=21 len=28 hex=%s",
			dropID, dropPayload, err, "1500000000000000"+stackSplittingChestHex+"023e")
	}
	if round, err := decodeClientPacketPayload(protocol.StatePlay, dropID, dropPayload); err != nil ||
		round != (protocol.ClientPacket)(drop) {
		t.Fatalf("整组丢弃往返 = %#v, %v", round, err)
	}
	for length := 0; length < len(dropPayload); length++ {
		if _, err := decodeClientPacketPayload(protocol.StatePlay, dropID, dropPayload[:length]); err == nil {
			t.Fatalf("整组丢弃截断到 %d 字节仍被接受", length)
		}
	}
	if _, err := decodeClientPacketPayload(
		protocol.StatePlay, dropID, append(append([]byte(nil), dropPayload...), 0),
	); err == nil {
		t.Fatal("整组丢弃尾随字节被接受")
	}
}

// TestDropStackZeroContainerRoundTrip 锁死非容器视图的零值引用在线上是
// 18 个零字节：编码与解码都必须无损往返回零值 `core.ContainerRef`。
func TestDropStackZeroContainerRoundTrip(t *testing.T) {
	packets := []protocol.ClientPacket{
		protocol.DropStack{Sequence: 1, View: protocol.StackViewInventory, Slot: 35},
		protocol.DropStack{Sequence: 2, View: protocol.StackViewCrafting, Slot: 44},
	}
	for _, packet := range packets {
		packetID, payload, err := encodeClientPacketPayload(protocol.StatePlay, packet)
		if err != nil {
			t.Fatalf("编码 %T 失败: %v", packet, err)
		}
		refBytes := payload[8 : 8+18]
		for _, b := range refBytes {
			if b != 0 {
				t.Fatalf("%T 零值引用编码出非零字节 %x", packet, refBytes)
			}
		}
		round, err := decodeClientPacketPayload(protocol.StatePlay, packetID, payload)
		if err != nil || round != packet {
			t.Fatalf("%T 往返 = %#v, %v", packet, round, err)
		}
	}
}

// TestDropStackWireRejectionMatrix 覆盖 wire 侧同判：手工构造的非法载荷
// （非法视图、容器视图零值引用、非容器视图非零引用、按视图分派的越界索引）
// 在解码路径必须整包拒绝，编码路径对同类结构体值也必须前置拒绝。
func TestDropStackWireRejectionMatrix(t *testing.T) {
	chest := testChestRef()
	furnace := stackSplittingFurnaceRef()
	invalidWire := [][]byte{
		dropStackWire(chest, 3, 0),
		dropStackWire(core.ContainerRef{}, protocol.StackViewContainer, 0),
		dropStackWire(chest, protocol.StackViewInventory, 0),
		dropStackWire(core.ContainerRef{}, protocol.StackViewInventory, core.InventorySlots),
		dropStackWire(core.ContainerRef{}, protocol.StackViewCrafting, protocol.GridCraftingViewSlots),
		dropStackWire(furnace, protocol.StackViewContainer, core.FurnaceViewSlots),
		dropStackWire(chest, protocol.StackViewContainer, core.ChestViewSlots),
	}
	for _, wire := range invalidWire {
		if packet, err := decodeClientPacketPayload(protocol.StatePlay, 21, wire); err == nil {
			t.Fatalf("非法整组丢弃 wire %x 解码为 %#v", wire, packet)
		}
	}
	invalidCommands := []protocol.DropStack{
		{Sequence: 1, Container: chest, View: 3, Slot: 0},
		{Sequence: 1, View: protocol.StackViewContainer, Slot: 0},
		{Sequence: 1, Container: chest, View: protocol.StackViewCrafting, Slot: 0},
		{Sequence: 1, View: protocol.StackViewInventory, Slot: core.InventorySlots},
		{Sequence: 1, Container: furnace, View: protocol.StackViewContainer, Slot: core.FurnaceViewSlots},
		{Sequence: 1, Container: chest, View: protocol.StackViewContainer, Slot: core.ChestViewSlots},
	}
	for _, command := range invalidCommands {
		if _, _, err := encodeClientPacketPayload(protocol.StatePlay, command); err == nil {
			t.Fatalf("非法整组丢弃 %+v 被编码接受", command)
		}
	}
}
