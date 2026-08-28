package protocol

import (
	"testing"

	"github.com/channing771/mornlea/internal/core"
)

func dropTestID(slot uint8, generation uint32) core.DropID {
	return core.DropID{
		Dimension:  core.Overworld,
		Chunk:      core.ChunkPos{X: 1, Z: -2},
		Slot:       slot,
		Generation: generation,
	}
}

func dropTestUpsert(slot uint8, generation uint32, count uint8) ItemDrop {
	return ItemDrop{
		ID: dropTestID(slot, generation), BlockIndex: 9,
		Item: core.ItemStone, Count: count,
	}
}

func TestProtocolV4DropPacketIDsAreFrozen(t *testing.T) {
	assertServerRegistry(t, []struct {
		state  State
		packet ServerPacket
		id     uint32
	}{
		{StatePlay, ItemDropUpserts{}, 11},
		{StatePlay, ItemDropRemoves{}, 12},
	})
}

func TestItemDropMessagesValidateBoundedBatches(t *testing.T) {
	full := make([]ItemDrop, 32)
	fullIDs := make([]core.DropID, 32)
	for index := range full {
		full[index] = dropTestUpsert(uint8(index), 1, 1)
		fullIDs[index] = dropTestID(uint8(index), 1)
	}
	valid := []interface{ Validate() error }{
		ItemDropUpserts{Drops: full},
		ItemDropRemoves{IDs: fullIDs},
	}
	for _, message := range valid {
		if err := message.Validate(); err != nil {
			t.Fatalf("%T 合法批次被拒绝: %v", message, err)
		}
	}

	unsorted := []ItemDrop{dropTestUpsert(2, 1, 1), dropTestUpsert(1, 1, 1)}
	duplicate := []ItemDrop{dropTestUpsert(1, 1, 1), dropTestUpsert(1, 1, 2)}
	stonePickaxeDurability, _ := core.ItemMaxDurability(core.ItemStonePickaxe)
	invalid := []interface{ Validate() error }{
		ItemDropUpserts{},
		ItemDropUpserts{Drops: append(append([]ItemDrop(nil), full...), dropTestUpsert(31, 2, 1))},
		ItemDropUpserts{Drops: unsorted},
		ItemDropUpserts{Drops: duplicate},
		ItemDropUpserts{Drops: []ItemDrop{{ID: dropTestID(0, 0), Item: core.ItemStone, Count: 1}}},
		ItemDropUpserts{Drops: []ItemDrop{{ID: dropTestID(0, 1), Item: core.ItemID(4242), Count: 1}}},
		ItemDropUpserts{Drops: []ItemDrop{{ID: dropTestID(0, 1), Item: core.ItemStone, Count: 0}}},
		ItemDropUpserts{Drops: []ItemDrop{{
			ID: dropTestID(0, 1), Item: core.ItemStone, Count: core.MaxStackCount + 1,
		}}},
		ItemDropUpserts{Drops: []ItemDrop{{
			ID: dropTestID(0, 1), Item: core.ItemStone, Count: 1, BlockIndex: MaxChunkBlockIndex,
		}}},
		ItemDropUpserts{Drops: []ItemDrop{{
			ID: dropTestID(0, 1), Item: core.ItemStonePickaxe, Count: 2,
			Durability: stonePickaxeDurability,
		}}},
		ItemDropUpserts{Drops: []ItemDrop{{
			ID: dropTestID(0, 1), Item: core.ItemStonePickaxe, Count: 1,
		}}},
		ItemDropUpserts{Drops: []ItemDrop{{
			ID: dropTestID(0, 1), Item: core.ItemStonePickaxe, Count: 1,
			Durability: stonePickaxeDurability + 1,
		}}},
		ItemDropUpserts{Drops: []ItemDrop{{
			ID: dropTestID(0, 1), Item: core.ItemStone, Count: 1, Durability: 1,
		}}},
		ItemDropRemoves{},
		ItemDropRemoves{IDs: append(append([]core.DropID(nil), fullIDs...), dropTestID(31, 2))},
		ItemDropRemoves{IDs: []core.DropID{dropTestID(2, 1), dropTestID(1, 1)}},
		ItemDropRemoves{IDs: []core.DropID{dropTestID(1, 1), dropTestID(1, 1)}},
		ItemDropRemoves{IDs: []core.DropID{dropTestID(0, 0)}},
	}
	for index, message := range invalid {
		if err := message.Validate(); err == nil {
			t.Fatalf("非法批次 %d 被接受: %T %+v", index, message, message)
		}
	}
}
