package network

import (
	"encoding/hex"
	"github.com/channing771/mornlea/internal/network/protocol"
	"testing"

	"github.com/channing771/mornlea/internal/core"
)

func TestProtocolV11InventoryCarriesWornToolDurability(t *testing.T) {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: 73}
	inventory.Backpack[0] = core.ItemStack{Item: core.ItemIronPickaxe, Count: 1, Durability: 149}
	packet := InventoryState{Inventory: inventory}
	packetID, payload, err := encodeServerControlPayload(StatePlay, packet)
	if err != nil {
		t.Fatalf("编码磨损工具背包: %v", err)
	}
	if packetID != 10 || len(payload) != 1+core.InventorySlots*5 {
		t.Fatalf("InventoryState id=%d payload=%d，想要 id=10 payload=181", packetID, len(payload))
	}
	decoded, err := decodeServerControlPayload(StatePlay, packetID, payload)
	if err != nil || decoded != packet {
		t.Fatalf("磨损工具背包往返 = %+v, %v，想要 %+v", decoded, err, packet)
	}
}

func TestProtocolV15InventoryCarriesLightBlockItem(t *testing.T) {
	var inventory core.Inventory
	inventory.Hotbar.Selected = 3
	inventory.Hotbar.Slots[3] = core.ItemStack{Item: core.ItemLightBlock, Count: 17}
	packet := InventoryState{Inventory: inventory}

	packetID, payload, err := encodeServerControlPayload(StatePlay, packet)
	if err != nil {
		t.Fatalf("编码发光块物品背包: %v", err)
	}
	if packetID != 10 || len(payload) != 181 {
		t.Fatalf("InventoryState id=%d payload=%d，想要 id=10 payload=181", packetID, len(payload))
	}
	const lightBlockOffset = 16
	if got := hex.EncodeToString(payload[lightBlockOffset : lightBlockOffset+5]); got != "0f00110000" {
		t.Fatalf("发光块物品 wire=%s，想要 0f00110000", got)
	}

	decoded, err := decodeServerControlPayload(StatePlay, packetID, payload)
	if err != nil {
		t.Fatalf("解码发光块物品背包: %v", err)
	}
	got, ok := decoded.(InventoryState)
	if !ok {
		t.Fatalf("发光块物品背包解码类型=%T，想要 InventoryState", decoded)
	}
	if got.Inventory.Hotbar.Selected != packet.Inventory.Hotbar.Selected ||
		got.Inventory.Hotbar.Slots != packet.Inventory.Hotbar.Slots ||
		got.Inventory.Backpack != packet.Inventory.Backpack {
		t.Fatalf("发光块物品背包往返=%+v，想要 %+v", got, packet)
	}
}

// TestGridCraftingMoveValueDomain 覆盖 `MoveCraftingStack` 的静态值域（spec
// authoritative-crafting「合成遵循命令顺序并私有确认」）：统一格 0..44、
// From≠To、至少一端落在网格区 0..8——两端都在背包区时 MUST 拒绝，背包内部
// 移动仍只能走既有 `MoveInventoryStack`。个人网格扩展格（尺寸 2 时的格 4..8）
// 是尺寸相关拒绝，由 sim 的 `craftingMoveCommandReasons` 在权威侧执行，网络层
// 不知道网格尺寸、不在此处重复。
func TestGridCraftingMoveValueDomain(t *testing.T) {
	valid := []MoveCraftingStack{
		{Sequence: 1, From: 9, To: 0},
		{Sequence: 2, From: 0, To: 9},
		{Sequence: 3, From: 8, To: 44},
		{Sequence: 4, From: 44, To: 8},
		{Sequence: 5, From: 4, To: 9},
	}
	for _, command := range valid {
		if err := command.Validate(); err != nil {
			t.Fatalf("合法移动 %+v 被拒绝: %v", command, err)
		}
		id, payload, err := encodeClientPacketPayload(StatePlay, command)
		if err != nil || id != 7 {
			t.Fatalf("编码 %+v = id %d, err %v，想要 id=7", command, id, err)
		}
		round, err := decodeClientPacketPayload(StatePlay, id, payload)
		if err != nil || round != (ClientPacket)(command) {
			t.Fatalf("往返 %+v = %+v, %v", command, round, err)
		}
	}
	invalid := []MoveCraftingStack{
		{Sequence: 1, From: protocol.GridCraftingViewSlots, To: 0},
		{Sequence: 1, From: 0, To: protocol.GridCraftingViewSlots},
		{Sequence: 1, From: 9, To: 9},
		{Sequence: 1, From: 0, To: 0},
		// 两端都在背包区 9..44：必须走既有背包移动命令。
		{Sequence: 1, From: 9, To: 10},
		{Sequence: 1, From: 44, To: 43},
	}
	for _, command := range invalid {
		if err := command.Validate(); err == nil {
			t.Fatalf("非法移动 %+v 通过了 Validate", command)
		}
		if _, _, err := encodeClientPacketPayload(StatePlay, command); err == nil {
			t.Fatalf("非法移动 %+v 被编码接受", command)
		}
	}
	// wire 侧同判：手工构造 45 越界与双背包端载荷，解码必须拒绝。
	for _, wire := range [][]byte{
		{1, 0, 0, 0, 0, 0, 0, 0, protocol.GridCraftingViewSlots, 0},
		{1, 0, 0, 0, 0, 0, 0, 0, 9, 10},
	} {
		if packet, err := decodeClientPacketPayload(StatePlay, 7, wire); err == nil {
			t.Fatalf("非法移动 wire %x 解码为 %+v", wire, packet)
		}
	}
}

// TestTakeCraftingOutputRequiresNonZeroSequence 锁死取出命令的序号值域：
// Sequence MUST 非零——零序号无法参与「过期序列不重复效果」的确认协议。
func TestTakeCraftingOutputRequiresNonZeroSequence(t *testing.T) {
	command := TakeCraftingOutput{Sequence: 0x1122334455667788}
	if err := command.Validate(); err != nil {
		t.Fatalf("合法取出命令被拒绝: %v", err)
	}
	id, payload, err := encodeClientPacketPayload(StatePlay, command)
	if err != nil || id != 15 || len(payload) != 8 {
		t.Fatalf("取出命令 id=%d payload=%x err=%v，想要 id=15 且 8 字节", id, payload, err)
	}
	if round, err := decodeClientPacketPayload(StatePlay, id, payload); err != nil || round != (ClientPacket)(command) {
		t.Fatalf("取出命令往返 = %+v, %v", round, err)
	}
	if err := (TakeCraftingOutput{}).Validate(); err == nil {
		t.Fatal("零序号取出命令通过了 Validate")
	}
	if _, _, err := encodeClientPacketPayload(StatePlay, TakeCraftingOutput{}); err == nil {
		t.Fatal("零序号取出命令被编码接受")
	}
	if packet, err := decodeClientPacketPayload(StatePlay, 15, make([]byte, 8)); err == nil {
		t.Fatalf("零序号取出命令 wire 解码为 %+v", packet)
	}
}

// goldenCraftingStateHex 是 51 字节固定编码的期望值：u8 尺寸 + 9 格 5 字节栈
// （u16 物品、u8 数量、u16 耐久，全部 little-endian）+ 5 字节产物格。夹具取
// 尺寸 3、格 0 装 2 个石头（物品编号 1）、格 4 装 1 根木棍（编号 37）、产物
// 4 个石砖（编号 4）——三处非零样本让「字段漏编码」与「编错偏移」都现形。
func goldenCraftingState() CraftingState {
	var state CraftingState
	state.Size = 3
	state.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 2}
	state.Slots[4] = core.ItemStack{Item: core.ItemStick, Count: 1}
	state.Output = core.ItemStack{Item: core.ItemStoneBrick, Count: 4}
	return state
}

func TestCraftingStateWireAndValueDomain(t *testing.T) {
	packet := goldenCraftingState()
	id, payload, err := encodeServerControlPayload(StatePlay, packet)
	if err != nil {
		t.Fatalf("编码网格状态: %v", err)
	}
	if id != 21 || len(payload) != 1+core.CraftingGridSlots*5+5 {
		t.Fatalf("CraftingState id=%d payload=%d，想要 id=21 且固定 51 字节", id, len(payload))
	}
	if got := hex.EncodeToString(payload); got != "03"+"0100020000"+"0000000000"+"0000000000"+"0000000000"+
		"2500010000"+"0000000000"+"0000000000"+"0000000000"+"0000000000"+"0400040000" {
		t.Fatalf("CraftingState wire=%s", got)
	}
	decoded, err := decodeServerControlPayload(StatePlay, id, payload)
	if err != nil {
		t.Fatalf("解码网格状态: %v", err)
	}
	if got, ok := decoded.(CraftingState); !ok || got != packet {
		t.Fatalf("网格状态往返 = %+v (%T)，想要 %+v", decoded, decoded, packet)
	}
	for length := 0; length < len(payload); length++ {
		if _, err := decodeServerControlPayload(StatePlay, id, payload[:length]); err == nil {
			t.Fatalf("CraftingState 截断到 %d bytes 仍被接受", length)
		}
	}
	if _, err := decodeServerControlPayload(StatePlay, id, append(append([]byte(nil), payload...), 0)); err == nil {
		t.Fatal("CraftingState 带尾随字节仍被接受")
	}

	// 尺寸 2 时格 4..8 必须为空：这是「网格状态私有同步且有界」的 wire 硬边界
	//（sim 侧构造上不可能产出该状态，这里是 codec 对伪造载荷的防御层）。
	personal := CraftingState{Size: 2}
	personal.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 1}
	personalID, personalPayload, err := encodeServerControlPayload(StatePlay, personal)
	if err != nil {
		t.Fatalf("编码尺寸 2 状态: %v", err)
	}
	if round, err := decodeServerControlPayload(StatePlay, personalID, personalPayload); err != nil ||
		round != (ServerPacket)(personal) {
		t.Fatalf("尺寸 2 状态往返 = %+v, %v", round, err)
	}
	residue := personal
	residue.Slots[4] = core.ItemStack{Item: core.ItemStone, Count: 1}
	if err := residue.Validate(); err == nil {
		t.Fatal("尺寸 2 且格 4 非空的状态通过了 Validate")
	}
	if _, _, err := encodeServerControlPayload(StatePlay, residue); err == nil {
		t.Fatal("尺寸 2 且格 4 非空的状态被编码接受")
	}
	// 尺寸 3 下同一内容合法：证明拒绝确实来自尺寸边界而不是内容本身。
	extended := personal
	extended.Size = 3
	if err := extended.Validate(); err != nil {
		t.Fatalf("尺寸 3 同一内容状态被拒绝: %v", err)
	}

	// 未知尺寸（0、1、4）与非法栈（未注册物品、超长数量、空物品带数量）在
	// Validate 与 wire 解码两处都必须被拒。
	for _, size := range []uint8{0, 1, 4, 255} {
		bad := packet
		bad.Size = size
		if err := bad.Validate(); err == nil {
			t.Fatalf("尺寸 %d 通过了 Validate", size)
		}
	}
	badStack := packet
	badStack.Slots[1] = core.ItemStack{Item: core.ItemID(9999), Count: 1}
	if err := badStack.Validate(); err == nil {
		t.Fatal("未注册物品通过了 Validate")
	}
	overlong := packet
	overlong.Output = core.ItemStack{Item: core.ItemStone, Count: core.MaxStackCount + 1}
	if err := overlong.Validate(); err == nil {
		t.Fatal("超长产物数量通过了 Validate")
	}
	ghost := packet
	ghost.Slots[2] = core.ItemStack{Item: core.ItemNone, Count: 2}
	if err := ghost.Validate(); err == nil {
		t.Fatal("空物品带数量通过了 Validate")
	}

	// wire 侧同判：对编码器产物做单字节腐化——尺寸字节改成 4、格 0 数量改成
	// 65，解码都必须拒绝。
	corruptSize := append([]byte(nil), personalPayload...)
	corruptSize[0] = 4
	if _, err := decodeServerControlPayload(StatePlay, 21, corruptSize); err == nil {
		t.Fatal("尺寸 4 的 wire 载荷被解码接受")
	}
	corruptCount := append([]byte(nil), personalPayload...)
	corruptCount[3] = core.MaxStackCount + 1
	if _, err := decodeServerControlPayload(StatePlay, 21, corruptCount); err == nil {
		t.Fatal("格 0 数量 65 的 wire 载荷被解码接受")
	}
}
