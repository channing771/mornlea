package codec

import (
	"reflect"
	"testing"

	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

// 水桶双命令与骨粉同形（u64 序号 + 两个 f32 朝向），编解码必须对称往返。
func TestBucketCodecRoundTrip(t *testing.T) {
	for _, pkt := range []protocol.ClientPacket{
		protocol.CollectWater{Sequence: 7, Yaw: 1, Pitch: 2},
		protocol.PlaceWater{Sequence: 8, Yaw: 3, Pitch: 4},
	} {
		packetID, enc, err := encodeClientPacketPayload(protocol.StatePlay, pkt)
		if err != nil {
			t.Fatal(err)
		}
		dec, err := decodeClientPacketPayload(protocol.StatePlay, packetID, enc)
		if err != nil {
			t.Fatal(err)
		}
		if reflect.DeepEqual(dec, pkt) == false {
			t.Fatalf("往返 = %+v，想要 %+v", dec, pkt)
		}
	}
}
