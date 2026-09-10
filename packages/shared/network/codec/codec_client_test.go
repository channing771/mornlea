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

// TestLoginStartCodecRoundTripAcrossViewDistanceDomain 钉死 v40 视距字节的
// wire 行为：合法域内的每个值（含两端边界 2 与 64）都必须无损往返，且编码
// 在 `DisplayName` 之后恰好追加 1 字节；域外值的编码被 `ValidateClientPacket`
// 前置拒绝（wire 上不存在形状完整的域外编码产物），解码侧的域外拒绝语义由
// 根包登录驱动以 `LoginReject` 冻结路径承担，不在本层截获。
func TestLoginStartCodecRoundTripAcrossViewDistanceDomain(t *testing.T) {
	id := mustCodecPlayerID(t)
	for _, viewDistance := range []uint8{protocol.LoginViewDistanceMin, 8, 32, protocol.LoginViewDistanceMax} {
		packet := protocol.LoginStart{PlayerID: id, DisplayName: "Chen", ViewDistance: viewDistance}
		packetID, payload, err := encodeClientPacketPayload(protocol.StateLogin, packet)
		if err != nil {
			t.Fatalf("视距 %d 编码失败: %v", viewDistance, err)
		}
		if packetID != 0 || len(payload) != 16+1+len("Chen")+1 {
			t.Fatalf("视距 %d 载荷 = id %d %d 字节，想要尾部恰好追加 1 字节", viewDistance, packetID, len(payload))
		}
		if payload[len(payload)-1] != viewDistance {
			t.Fatalf("视距 %d 尾字节 = %d，想要原值落在 DisplayName 之后", viewDistance, payload[len(payload)-1])
		}
		decoded, err := decodeClientPacketPayload(protocol.StateLogin, packetID, payload)
		if err != nil {
			t.Fatalf("视距 %d 解码失败: %v", viewDistance, err)
		}
		if got := decoded.(protocol.LoginStart); got.ViewDistance != viewDistance {
			t.Fatalf("往返视距 = %d，想要 %d", got.ViewDistance, viewDistance)
		}
	}
	for _, viewDistance := range []uint8{0, 1, protocol.LoginViewDistanceMax + 1, 255} {
		packet := protocol.LoginStart{PlayerID: id, DisplayName: "Chen", ViewDistance: viewDistance}
		if _, _, err := encodeClientPacketPayload(protocol.StateLogin, packet); err == nil {
			t.Fatalf("域外视距 %d 被编码", viewDistance)
		}
	}
}
