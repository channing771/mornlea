package codec

import (
	"encoding/hex"
	"github.com/channing771/mornlea/internal/network/protocol"
	"reflect"
	"testing"

	"github.com/channing771/mornlea/internal/core"
)

func remotePlayerStatesWireFixture(ids ...core.PlayerID) []byte {
	payload := make([]byte, 9)
	payload[8] = byte(len(ids))
	for _, id := range ids {
		payload = append(payload, id[:]...)
		payload = append(payload, make([]byte, 25)...)
	}
	return payload
}

func blockChangesWireFixture(baseRevision, newRevision uint64, changes []protocol.BlockChange) []byte {
	var encoder byteEncoder
	encoder.i32(int32(core.Overworld))
	encoder.i32(0)
	encoder.i32(0)
	encoder.u64(baseRevision)
	encoder.u64(newRevision)
	encoder.uvarint(uint32(len(changes)))
	for _, change := range changes {
		encoder.i32(change.Position.X)
		encoder.i32(change.Position.Y)
		encoder.i32(change.Position.Z)
		encoder.u16(uint16(change.Block))
	}
	return encoder.data
}

func blockChangesCountFixture(count uint32) []byte {
	var encoder byteEncoder
	encoder.i32(int32(core.Overworld))
	encoder.i32(0)
	encoder.i32(0)
	encoder.u64(1)
	encoder.u64(2)
	encoder.uvarint(count)
	return encoder.data
}

func mustDecodeHex(t *testing.T, value string) []byte {
	t.Helper()
	decoded, err := hex.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func mustCodecPlayerID(t *testing.T) core.PlayerID {
	t.Helper()
	id, err := core.ParsePlayerID("00112233-4455-4677-8899-aabbccddeeff")
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func goldenInventoryState() protocol.InventoryState {
	var inventory core.Inventory
	inventory.Hotbar.Selected = 2
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 5}
	inventory.Hotbar.Slots[4] = core.ItemStack{Item: core.ItemGrass, Count: core.MaxStackCount}
	inventory.Backpack[0] = core.ItemStack{Item: core.ItemDirt, Count: 1}
	inventory.Backpack[core.BackpackSlots-1] = core.ItemStack{Item: core.ItemStone, Count: 9}
	return protocol.InventoryState{Inventory: inventory}
}

// goldenInventoryStateHex 是 1 字节选中栏位加 36 格固定编码，共 181 字节。
func goldenInventoryStateHex() string {
	empty := "0000000000"
	hex := "02" + "0100050000" + empty + empty + empty + "0300400000"
	for range 4 {
		hex += empty
	}
	hex += "0200010000"
	for range core.BackpackSlots - 2 {
		hex += empty
	}
	return hex + "0100090000"
}

// inventoryStateWire 手工构造固定负载，用于绕过编码器校验注入非法状态。
func inventoryStateWire(inventory core.Inventory) []byte {
	wire := make([]byte, 0, 1+core.InventorySlots*5)
	wire = append(wire, inventory.Hotbar.Selected)
	appendStack := func(stack core.ItemStack) {
		wire = append(wire,
			byte(stack.Item), byte(stack.Item>>8), stack.Count,
			byte(stack.Durability), byte(stack.Durability>>8),
		)
	}
	for _, stack := range inventory.Hotbar.Slots {
		appendStack(stack)
	}
	for _, stack := range inventory.Backpack {
		appendStack(stack)
	}
	return wire
}

func sameClientPacket(got, want protocol.ClientPacket) bool {
	switch got := got.(type) {
	case protocol.ClientHello:
		other, ok := want.(protocol.ClientHello)
		return ok && got == other
	case protocol.LoginStart:
		other, ok := want.(protocol.LoginStart)
		return ok && got == other
	case protocol.PlayerInput:
		other, ok := want.(protocol.PlayerInput)
		return ok && got == other
	case protocol.PlaceBlock:
		other, ok := want.(protocol.PlaceBlock)
		return ok && got == other
	case protocol.RequestChunkResync:
		other, ok := want.(protocol.RequestChunkResync)
		return ok && got == other
	case protocol.KeepAliveReply:
		other, ok := want.(protocol.KeepAliveReply)
		return ok && got == other
	case protocol.SelectHotbar:
		other, ok := want.(protocol.SelectHotbar)
		return ok && got == other
	case protocol.MoveInventoryStack:
		other, ok := want.(protocol.MoveInventoryStack)
		return ok && got == other
	case protocol.DropSelectedItem:
		other, ok := want.(protocol.DropSelectedItem)
		return ok && got == other
	case protocol.OpenContainer:
		other, ok := want.(protocol.OpenContainer)
		return ok && got == other
	case protocol.MoveContainerStack:
		other, ok := want.(protocol.MoveContainerStack)
		return ok && got == other
	case protocol.CloseContainer:
		other, ok := want.(protocol.CloseContainer)
		return ok && got == other
	case protocol.ChatCommand:
		other, ok := want.(protocol.ChatCommand)
		return ok && got == other
	case protocol.TillSoil:
		other, ok := want.(protocol.TillSoil)
		return ok && got == other
	case protocol.MoveCraftingStack:
		other, ok := want.(protocol.MoveCraftingStack)
		return ok && got == other
	case protocol.TakeCraftingOutput:
		other, ok := want.(protocol.TakeCraftingOutput)
		return ok && got == other
	case protocol.BoneMeal:
		other, ok := want.(protocol.BoneMeal)
		return ok && got == other
	default:
		return false
	}
}

func sameServerPacket(got, want protocol.ServerPacket) bool {
	switch got := got.(type) {
	case protocol.ServerHello:
		other, ok := want.(protocol.ServerHello)
		return ok && got == other
	case protocol.HandshakeReject:
		other, ok := want.(protocol.HandshakeReject)
		return ok && got == other
	case protocol.LoginSuccess:
		other, ok := want.(protocol.LoginSuccess)
		return ok && got == other
	case protocol.LoginReject:
		other, ok := want.(protocol.LoginReject)
		return ok && got == other
	case protocol.BlockChanges:
		other, ok := want.(protocol.BlockChanges)
		if !ok || got.Dimension != other.Dimension || got.Chunk != other.Chunk || got.BaseRevision != other.BaseRevision || got.NewRevision != other.NewRevision || len(got.Changes) != len(other.Changes) {
			return false
		}
		for index := range got.Changes {
			if got.Changes[index] != other.Changes[index] {
				return false
			}
		}
		return true
	case protocol.ForgetChunks:
		other, ok := want.(protocol.ForgetChunks)
		if !ok || got.Dimension != other.Dimension || len(got.Chunks) != len(other.Chunks) {
			return false
		}
		for index := range got.Chunks {
			if got.Chunks[index] != other.Chunks[index] {
				return false
			}
		}
		return true
	case protocol.PlayerState:
		other, ok := want.(protocol.PlayerState)
		return ok && got == other
	case protocol.CommandRejected:
		other, ok := want.(protocol.CommandRejected)
		return ok && got == other
	case protocol.PlaceBlockSucceeded:
		other, ok := want.(protocol.PlaceBlockSucceeded)
		return ok && got == other
	case protocol.CraftingState:
		other, ok := want.(protocol.CraftingState)
		return ok && got == other
	case protocol.KeepAlive:
		other, ok := want.(protocol.KeepAlive)
		return ok && got == other
	case protocol.Disconnect:
		other, ok := want.(protocol.Disconnect)
		return ok && got == other
	case protocol.InventoryState:
		other, ok := want.(protocol.InventoryState)
		return ok && got == other
	case protocol.ChatEvent:
		other, ok := want.(protocol.ChatEvent)
		return ok && got == other
	case protocol.CompanionSpawn:
		other, ok := want.(protocol.CompanionSpawn)
		return ok && got == other
	case protocol.CompanionStates:
		other, ok := want.(protocol.CompanionStates)
		return ok && reflect.DeepEqual(got, other)
	case protocol.CompanionDespawn:
		other, ok := want.(protocol.CompanionDespawn)
		return ok && got == other
	default:
		return false
	}
}

// tooManyValidBlockChanges 构造超出单批上限的合法方块变更批次（4097 条），
// 供 wire 层预分配拒绝路径使用；与 protocol 域 packet 校验测试同名夹具保持
// 同一取值，跨包复制系测试跟随被测主体的机械结果。
func tooManyValidBlockChanges() protocol.BlockChanges {
	changes := make([]protocol.BlockChange, 4097)
	for index := range changes {
		changes[index] = protocol.BlockChange{
			Position: core.BlockPos{
				X: int32(index % core.SectionSize),
				Y: core.MinY + int32(index/(core.SectionSize*core.SectionSize)),
				Z: int32((index / core.SectionSize) % core.SectionSize),
			},
			Block: core.StoneID,
		}
	}
	return protocol.BlockChanges{BaseRevision: 1, NewRevision: 2, Changes: changes}
}
