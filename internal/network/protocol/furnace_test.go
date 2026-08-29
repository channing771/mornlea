package protocol

import "testing"

func TestProtocolV7FurnacePacketIDsAreFrozen(t *testing.T) {
	assertClientRegistry(t, []struct {
		state  State
		packet ClientPacket
		id     uint32
	}{
		{StatePlay, OpenContainer{}, 8},
		{StatePlay, MoveContainerStack{}, 9},
		{StatePlay, CloseContainer{}, 10},
	})
	assertServerRegistry(t, []struct {
		state  State
		packet ServerPacket
		id     uint32
	}{
		{StatePlay, FurnaceState{}, 13},
		{StatePlay, ContainerClosed{}, 14},
	})
	if _, ok := ClientPacketForID(StatePlay, 1); ok {
		t.Fatal("Play client packet ID 1 必须保持未分配")
	}
}
