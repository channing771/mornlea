package protocol

import (
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

func TestProtocolV12ChestStatePacketIDIsFrozen(t *testing.T) {
	assertServerRegistry(t, []struct {
		state  State
		packet ServerPacket
		id     uint32
	}{
		{StatePlay, ChestState{}, 15},
	})
}

// TestMoveContainerStackChestUnifiedSlotRange 覆盖箱子统一栏位 0..62：
// 63 及以上必须被拒绝，62 是最后一个合法索引。
func TestMoveContainerStackChestUnifiedSlotRange(t *testing.T) {
	ref := testChestRef()
	if err := (MoveContainerStack{Container: ref, From: 0, To: core.ChestViewSlots - 1}).Validate(); err != nil {
		t.Fatalf("箱子统一栏位上限 %d 被拒绝: %v", core.ChestViewSlots-1, err)
	}
	if err := (MoveContainerStack{Container: ref, From: 0, To: core.ChestViewSlots}).Validate(); err == nil {
		t.Fatalf("箱子越界统一栏位 %d 被接受", core.ChestViewSlots)
	}
	// 箱子没有输出格限制：熔炉的输出格索引在箱子这里是普通格。
	if err := (MoveContainerStack{
		Container: ref, From: 0, To: core.FurnaceOutputSlot,
	}).Validate(); err != nil {
		t.Fatalf("箱子格 %d 被错误地当成熔炉输出格拒绝: %v", core.FurnaceOutputSlot, err)
	}
}

// TestContainerNeutralMessagesRejectUnknownKind 覆盖容器中性消息（打开命令除外，
// 它本身不携带引用）对未知种类枚举值的一致拒绝。
func TestContainerNeutralMessagesRejectUnknownKind(t *testing.T) {
	unknown := core.ContainerRef{Dimension: core.Overworld, Kind: core.ContainerKind(9), Generation: 1}
	if err := (MoveContainerStack{Container: unknown, From: 0, To: 1}).Validate(); err == nil {
		t.Fatal("MoveContainerStack 接受了未知容器种类")
	}
	if err := (ContainerClosed{Container: unknown}).Validate(); err == nil {
		t.Fatal("ContainerClosed 接受了未知容器种类")
	}
}
