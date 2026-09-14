package codec

import (
	"encoding/hex"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

// stackSplittingChestHex 是 `testChestRef` 的 18 字节容器引用固定编码。
const stackSplittingChestHex = "00000000" + "fdffffff" + "07000000" + "01" + "05" + "09000000"

// stackSplittingFurnaceHex 是 Overworld (4,-2)、槽 3、generation 5 的熔炉
// 引用固定编码，与 `protocol` 域测试的 `stackSplittingFurnaceRef` 同一取值。
const stackSplittingFurnaceHex = "00000000" + "04000000" + "feffffff" + "00" + "03" + "05000000"

func stackSplittingFurnaceRef() core.ContainerRef {
	return core.ContainerRef{
		Dimension:  core.Overworld,
		Chunk:      core.ChunkPos{X: 4, Z: -2},
		Kind:       core.ContainerKindFurnace,
		Slot:       3,
		Generation: 5,
	}
}

// stackSplitPartialWire 手工构造部分移动载荷，绕过编码器校验注入非法值域。
func stackSplitPartialWire(ref core.ContainerRef, view, from, to, single uint8) []byte {
	var encoder byteEncoder
	encoder.u64(1)
	encodeContainerRef(&encoder, ref)
	encoder.u8(view)
	encoder.u8(from)
	encoder.u8(to)
	encoder.u8(single)
	return encoder.data
}

// stackSplitQuickWire 手工构造快捷搬运载荷，绕过编码器校验注入非法值域。
func stackSplitQuickWire(ref core.ContainerRef, view, from uint8) []byte {
	var encoder byteEncoder
	encoder.u64(1)
	encodeContainerRef(&encoder, ref)
	encoder.u8(view)
	encoder.u8(from)
	return encoder.data
}

// TestStackSplitGoldenWireLayout 覆盖分堆双命令的冻结布局：部分移动是
// u64 序号 + 18 字节容器引用 + u8 视图 + u8 来源 + u8 目标 + u8 单件标志的
// 固定 30 字节；快捷搬运去掉目标与单件标志，是固定 28 字节。两组夹具分别
// 取箱子/熔炉引用并让视图、来源、目标、单件标志全部取非零中间值，任何
// 换位、漏写或宽度漂移都会改变期望字节。截断与尾随字节都必须整包拒绝。
func TestStackSplitGoldenWireLayout(t *testing.T) {
	partial := protocol.MoveStackPartial{
		Sequence: 19, Container: testChestRef(), View: protocol.StackViewContainer,
		From: 10, To: 34, Single: true,
	}
	partialID, partialPayload, err := encodeClientPacketPayload(protocol.StatePlay, partial)
	if err != nil || partialID != 19 || len(partialPayload) != 30 ||
		hex.EncodeToString(partialPayload) != "1300000000000000"+stackSplittingChestHex+"020a2201" {
		t.Fatalf("部分移动 id=%d payload=%x err=%v，想要 id=19 len=30 hex=%s",
			partialID, partialPayload, err, "1300000000000000"+stackSplittingChestHex+"020a2201")
	}
	if round, err := decodeClientPacketPayload(protocol.StatePlay, partialID, partialPayload); err != nil ||
		round != (protocol.ClientPacket)(partial) {
		t.Fatalf("部分移动往返 = %#v, %v", round, err)
	}
	for length := 0; length < len(partialPayload); length++ {
		if _, err := decodeClientPacketPayload(protocol.StatePlay, partialID, partialPayload[:length]); err == nil {
			t.Fatalf("部分移动截断到 %d 字节仍被接受", length)
		}
	}
	if _, err := decodeClientPacketPayload(
		protocol.StatePlay, partialID, append(append([]byte(nil), partialPayload...), 0),
	); err == nil {
		t.Fatal("部分移动尾随字节被接受")
	}

	quick := protocol.QuickMoveStack{
		Sequence: 20, Container: stackSplittingFurnaceRef(), View: protocol.StackViewContainer, From: 38,
	}
	quickID, quickPayload, err := encodeClientPacketPayload(protocol.StatePlay, quick)
	if err != nil || quickID != 20 || len(quickPayload) != 28 ||
		hex.EncodeToString(quickPayload) != "1400000000000000"+stackSplittingFurnaceHex+"0226" {
		t.Fatalf("快捷搬运 id=%d payload=%x err=%v，想要 id=20 len=28 hex=%s",
			quickID, quickPayload, err, "1400000000000000"+stackSplittingFurnaceHex+"0226")
	}
	if round, err := decodeClientPacketPayload(protocol.StatePlay, quickID, quickPayload); err != nil ||
		round != (protocol.ClientPacket)(quick) {
		t.Fatalf("快捷搬运往返 = %#v, %v", round, err)
	}
	for length := 0; length < len(quickPayload); length++ {
		if _, err := decodeClientPacketPayload(protocol.StatePlay, quickID, quickPayload[:length]); err == nil {
			t.Fatalf("快捷搬运截断到 %d 字节仍被接受", length)
		}
	}
	if _, err := decodeClientPacketPayload(
		protocol.StatePlay, quickID, append(append([]byte(nil), quickPayload...), 0),
	); err == nil {
		t.Fatal("快捷搬运尾随字节被接受")
	}
}

// TestStackSplitZeroContainerRoundTrip 锁死非容器视图的零值引用在线上是
// 18 个零字节：编码与解码都必须无损往返回零值 `core.ContainerRef`，防止
// 「校验放行零值、编解码却搬错字段」的漂移。
func TestStackSplitZeroContainerRoundTrip(t *testing.T) {
	packets := []protocol.ClientPacket{
		protocol.MoveStackPartial{Sequence: 1, View: protocol.StackViewInventory, From: 0, To: 35, Single: true},
		protocol.MoveStackPartial{Sequence: 2, View: protocol.StackViewCrafting, From: 44, To: 9},
		protocol.QuickMoveStack{Sequence: 3, View: protocol.StackViewInventory, From: 35},
		protocol.QuickMoveStack{Sequence: 4, View: protocol.StackViewCrafting, From: 0},
	}
	for _, packet := range packets {
		packetID, payload, err := encodeClientPacketPayload(protocol.StatePlay, packet)
		if err != nil {
			t.Fatalf("编码 %T 失败: %v", packet, err)
		}
		// 两条命令的容器引用都紧跟 u64 序号之后，占固定 18 字节。
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

// TestStackSplitWireRejectionMatrix 覆盖 wire 侧同判：手工构造的非法载荷
// （非法视图、容器视图零值引用、非容器视图非零引用、按视图分派的越界索引、
// 部分移动同格、非法单件标志字节）在解码路径必须整包拒绝，编码路径对同类
// 结构体值也必须前置拒绝。
func TestStackSplitWireRejectionMatrix(t *testing.T) {
	chest := testChestRef()
	furnace := stackSplittingFurnaceRef()
	invalidPartialWire := [][]byte{
		stackSplitPartialWire(chest, 3, 0, 1, 0),
		stackSplitPartialWire(core.ContainerRef{}, protocol.StackViewContainer, 0, 1, 0),
		stackSplitPartialWire(chest, protocol.StackViewInventory, 0, 1, 0),
		stackSplitPartialWire(core.ContainerRef{}, protocol.StackViewInventory, core.InventorySlots, 0, 0),
		stackSplitPartialWire(core.ContainerRef{}, protocol.StackViewInventory, 0, core.InventorySlots, 0),
		stackSplitPartialWire(core.ContainerRef{}, protocol.StackViewCrafting, protocol.GridCraftingViewSlots, 0, 0),
		stackSplitPartialWire(furnace, protocol.StackViewContainer, core.FurnaceViewSlots, 0, 0),
		stackSplitPartialWire(chest, protocol.StackViewContainer, 0, core.ChestViewSlots, 0),
		stackSplitPartialWire(core.ContainerRef{}, protocol.StackViewInventory, 7, 7, 0),
		// 单件标志字节只接受 0/1：2 是非法 bool 编码。
		stackSplitPartialWire(core.ContainerRef{}, protocol.StackViewInventory, 0, 1, 2),
	}
	for _, wire := range invalidPartialWire {
		if packet, err := decodeClientPacketPayload(protocol.StatePlay, 19, wire); err == nil {
			t.Fatalf("非法部分移动 wire %x 解码为 %#v", wire, packet)
		}
	}
	invalidQuickWire := [][]byte{
		stackSplitQuickWire(chest, 3, 0),
		stackSplitQuickWire(core.ContainerRef{}, protocol.StackViewContainer, 0),
		stackSplitQuickWire(chest, protocol.StackViewInventory, 0),
		stackSplitQuickWire(core.ContainerRef{}, protocol.StackViewInventory, core.InventorySlots),
		stackSplitQuickWire(core.ContainerRef{}, protocol.StackViewCrafting, protocol.GridCraftingViewSlots),
		stackSplitQuickWire(furnace, protocol.StackViewContainer, core.FurnaceViewSlots),
		stackSplitQuickWire(chest, protocol.StackViewContainer, core.ChestViewSlots),
	}
	for _, wire := range invalidQuickWire {
		if packet, err := decodeClientPacketPayload(protocol.StatePlay, 20, wire); err == nil {
			t.Fatalf("非法快捷搬运 wire %x 解码为 %#v", wire, packet)
		}
	}

	invalidPartial := []protocol.MoveStackPartial{
		{Sequence: 1, Container: chest, View: 3, From: 0, To: 1},
		{Sequence: 1, View: protocol.StackViewContainer, From: 0, To: 1},
		{Sequence: 1, Container: chest, View: protocol.StackViewInventory, From: 0, To: 1},
		{Sequence: 1, View: protocol.StackViewInventory, From: core.InventorySlots, To: 0},
		{Sequence: 1, View: protocol.StackViewCrafting, From: 0, To: protocol.GridCraftingViewSlots},
		{Sequence: 1, Container: furnace, View: protocol.StackViewContainer, From: 0, To: core.FurnaceViewSlots},
		{Sequence: 1, Container: chest, View: protocol.StackViewContainer, From: core.ChestViewSlots, To: 0},
		{Sequence: 1, View: protocol.StackViewInventory, From: 7, To: 7},
	}
	for _, command := range invalidPartial {
		if _, _, err := encodeClientPacketPayload(protocol.StatePlay, command); err == nil {
			t.Fatalf("非法部分移动 %+v 被编码接受", command)
		}
	}
	invalidQuick := []protocol.QuickMoveStack{
		{Sequence: 1, Container: chest, View: 3, From: 0},
		{Sequence: 1, View: protocol.StackViewContainer, From: 0},
		{Sequence: 1, Container: chest, View: protocol.StackViewCrafting, From: 0},
		{Sequence: 1, View: protocol.StackViewInventory, From: core.InventorySlots},
		{Sequence: 1, Container: furnace, View: protocol.StackViewContainer, From: core.FurnaceViewSlots},
		{Sequence: 1, Container: chest, View: protocol.StackViewContainer, From: core.ChestViewSlots},
	}
	for _, command := range invalidQuick {
		if _, _, err := encodeClientPacketPayload(protocol.StatePlay, command); err == nil {
			t.Fatalf("非法快捷搬运 %+v 被编码接受", command)
		}
	}
}
