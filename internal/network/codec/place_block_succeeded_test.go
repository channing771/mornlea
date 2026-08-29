package codec

import (
	"encoding/hex"
	"errors"
	"github.com/channing771/mornlea/internal/network/protocol"
	"reflect"
	"testing"
)

func TestProtocolV26PlaceBlockSucceededWire(t *testing.T) {
	packet := protocol.PlaceBlockSucceeded{Sequence: 0x1122334455667788}
	packetID, payload, err := encodeServerControlPayload(protocol.StatePlay, packet)
	if err != nil || packetID != 20 || hex.EncodeToString(payload) != "8877665544332211" {
		t.Fatalf("protocol.PlaceBlockSucceeded id=%d payload=%x err=%v", packetID, payload, err)
	}
	decoded, err := decodeServerControlPayload(protocol.StatePlay, packetID, payload)
	if err != nil || !reflect.DeepEqual(decoded, packet) {
		t.Fatalf("protocol.PlaceBlockSucceeded round=%#v err=%v", decoded, err)
	}
	for length := 0; length < len(payload); length++ {
		if _, err := decodeServerControlPayload(protocol.StatePlay, packetID, payload[:length]); err == nil {
			t.Fatalf("protocol.PlaceBlockSucceeded 截断到 %d bytes 仍被接受", length)
		}
	}
	if _, err := decodeServerControlPayload(protocol.StatePlay, packetID, append(payload, 0)); err == nil {
		t.Fatal("protocol.PlaceBlockSucceeded 带尾随字节仍被接受")
	}
	if _, _, err := encodeServerControlPayload(protocol.StateLogin, packet); err == nil {
		t.Fatal("protocol.PlaceBlockSucceeded 在 Login state 仍可编码")
	}
}

// TestProtocolV26PlaceBlockSucceededRegistryBoundary 钉死 v26 占用的 S→C ID 20
// 与紧随其后的边界。格子工作台把 21 分配给 `protocol.CraftingState`（in-branch 临时
// 编号，design.md D1），「下一个仍未分配」的上界随之推进到 22。
func TestProtocolV26PlaceBlockSucceededRegistryBoundary(t *testing.T) {
	packetID, ok := protocol.ServerPacketID(protocol.StatePlay, protocol.PlaceBlockSucceeded{})
	if !ok || packetID != 20 {
		t.Fatalf("protocol.PlaceBlockSucceeded ID=(%d,%v)，想要 (20,true)", packetID, ok)
	}
	registered, ok := protocol.ServerPacketForID(protocol.StatePlay, 20)
	if !ok {
		t.Fatal("Play S→C ID 20 未注册")
	}
	if _, ok := registered.(protocol.PlaceBlockSucceeded); !ok {
		t.Fatalf("Play S→C ID 20=%T，想要 protocol.PlaceBlockSucceeded", registered)
	}
	if _, ok := protocol.ServerPacketForID(protocol.StatePlay, 26); ok {
		t.Fatal("Play S→C ID 26 必须保持未分配")
	}
	if _, err := decodeServerControlPayload(protocol.StatePlay, 26, nil); !errors.Is(err, errUnknownPacketID) {
		t.Fatalf("Play S→C ID 26 解码错误=%v，想要 %v", err, errUnknownPacketID)
	}
}
