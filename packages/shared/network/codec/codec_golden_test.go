package codec

import (
	"encoding/hex"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
	"reflect"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

func TestProtocolV1SmallPacketGolden(t *testing.T) {
	id := mustCodecPlayerID(t)
	clients := []struct {
		name    string
		state   protocol.State
		packet  protocol.ClientPacket
		wantID  uint32
		wantHex string
	}{
		{"hello", protocol.StateHandshake, protocol.ClientHello{ProtocolVersion: 35}, 0, "23"},
		{"login start", protocol.StateLogin, protocol.LoginStart{PlayerID: id, DisplayName: "Chen"}, 0, "00112233445546778899aabbccddeeff044368656e"},
		{"input", protocol.StatePlay, protocol.PlayerInput{Sequence: 1, MoveX: -1, MoveZ: 1, Jump: true, Yaw: 1.5, Pitch: -0.5, Mining: true}, 0, "0100000000000000ff01010000c03f000000bf" + "01" + "00" + "00"},
		// v24 新增：进食位是载荷最末一字节。夹具刻意取 Mining=false、
		// Eating=true——同真同假的样本无法分辨「两个布尔字节写反」的实现。
		// v28 追加 Sprinting 位（最末字节），此处取 Sprinting=false 以锁死尾部追加语义。
		{"input eating", protocol.StatePlay, protocol.PlayerInput{Sequence: 2, Eating: true}, 0, "0200000000000000" + "00" + "00" + "00" + "00000000" + "00000000" + "00" + "01" + "00"},
		{"place", protocol.StatePlay, protocol.PlaceBlock{Sequence: 3, Yaw: 2, Pitch: -1, Slot: 4}, 2, "030000000000000000000040000080bf04"},
		{"resync", protocol.StatePlay, protocol.RequestChunkResync{Sequence: 4, Dimension: core.Overworld, Chunk: core.ChunkPos{X: -2, Z: 3}, HaveRevision: 5}, 3, "040000000000000000000000feffffff030000000500000000000000"},
		{"keep alive reply", protocol.StatePlay, protocol.KeepAliveReply{Token: 6}, 4, "0600000000000000"},
		{"select hotbar", protocol.StatePlay, protocol.SelectHotbar{Sequence: 9, Slot: 8}, 5, "090000000000000008"},
		{"move inventory stack", protocol.StatePlay, protocol.MoveInventoryStack{Sequence: 10, From: 3, To: 35}, 6, "0a00000000000000" + "03" + "23"},
		{"move crafting stack", protocol.StatePlay, protocol.MoveCraftingStack{Sequence: 11, From: 9, To: 0}, 7, "0b00000000000000" + "09" + "00"},
		{"drop selected item", protocol.StatePlay, protocol.DropSelectedItem{
			Sequence: 0x1122334455667788,
		}, 11, "8877665544332211"},
		// v22 新增：与 place 同样的 u64 序号 + 两个 f32 朝向，但没有栏位字节。
		{"till soil", protocol.StatePlay, protocol.TillSoil{Sequence: 12, Yaw: 2, Pitch: -1}, 13,
			"0c0000000000000000000040000080bf"},
	}
	for _, tc := range clients {
		t.Run(tc.name, func(t *testing.T) {
			gotID, got, err := encodeClientPacketPayload(tc.state, tc.packet)
			if err != nil || gotID != tc.wantID || hex.EncodeToString(got) != tc.wantHex {
				t.Fatalf("%T id=%d payload=%x err=%v", tc.packet, gotID, got, err)
			}
			round, err := decodeClientPacketPayload(tc.state, gotID, got)
			if err != nil || !sameClientPacket(round, tc.packet) {
				t.Fatalf("round=%#v err=%v", round, err)
			}
			for length := 0; length < len(got); length++ {
				if _, err := decodeClientPacketPayload(tc.state, gotID, got[:length]); err == nil {
					t.Fatalf("truncated %T at %d accepted", tc.packet, length)
				}
			}
		})
	}

	servers := []struct {
		name    string
		state   protocol.State
		packet  protocol.ServerPacket
		wantID  uint32
		wantHex string
	}{
		{"server hello", protocol.StateHandshake, protocol.ServerHello{ProtocolVersion: 35}, 0, "23"},
		{"handshake reject", protocol.StateHandshake, protocol.HandshakeReject{ServerProtocolVersion: 35, Code: protocol.HandshakeVersionMismatch, Message: "no"}, 1, "2301026e6f"},
		{"login success", protocol.StateLogin, protocol.LoginSuccess{PlayerID: id, WorldSeed: 0x1122334455667788}, 0, "00112233445546778899aabbccddeeff8877665544332211"},
		{"login reject", protocol.StateLogin, protocol.LoginReject{Code: protocol.LoginInvalidIdentity, Message: "no"}, 1, "02026e6f"},
		{"block changes", protocol.StatePlay, protocol.BlockChanges{Dimension: core.Overworld, Chunk: core.ChunkPos{X: 1, Z: -1}, BaseRevision: 1, NewRevision: 2, Changes: []protocol.BlockChange{{Position: core.BlockPos{X: 16, Y: -64, Z: -1}, Block: core.StoneID}}}, 1, "0000000001000000ffffffff010000000000000002000000000000000110000000c0ffffffffffffff0200"},
		{"forget chunks", protocol.StatePlay, protocol.ForgetChunks{Dimension: core.Overworld, Chunks: []core.ChunkPos{{X: 1, Z: -1}, {X: 2, Z: 3}}}, 2, "000000000201000000ffffffff0200000003000000"},
		{"inactive player state", protocol.StatePlay, protocol.PlayerState{}, 3, "00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000" + "00" + "0000" + "00" + "00" + "0000" + "0000000000000000"},
		{"active player state", protocol.StatePlay, protocol.PlayerState{Dimension: core.Overworld, MiningActive: true, MiningTarget: core.BlockPos{X: 1, Y: 2, Z: 3}, MiningProgressTicks: 6, MiningRequiredTicks: 15, MiningHarvestable: true, Health: 15, Oxygen: core.MaxOxygenTicks, WorldTimeTicks: 24000}, 3, "000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000101000000020000000300000006000f0001" + "0f" + "2c01" + "00" + "00" + "0000" + "c05d000000000000"},
		// 氧气取一个既非 0 也非满值的中间值，锁死它确实按 u16 小端落在 Health 之后：
		// 取 0 会与相邻字节的零值混淆，取满值又会与"未初始化即满"的实现巧合重合。
		// v31 起这份夹具同时携带 0x0101 的中间值相位偏移，锁死它按 u16 小端
		// 落在 `SaturationZero` 之后、`WorldTimeTicks` 之前。
		{"partially drowned player state", protocol.StatePlay, protocol.PlayerState{Dimension: core.Overworld, Health: 15, Oxygen: 0x0101, DayPhaseOffset: 0x0101, WorldTimeTicks: 24000}, 3, "00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000" + "0f" + "0101" + "00" + "00" + "0101" + "c05d000000000000"},
		// v24 新增：饥饿值取 12（0x0c）这个既非 0 也非满值的中间值，锁死它确实
		// 按 u8 落在 Oxygen 之后、WorldTimeTicks 之前。取 0 与「编码器根本没写
		// 这个字段」不可分辨，取满值又与「未初始化即吃饱」的实现巧合重合。
		// v29 新增：`SaturationZero` 尾部 1 bool 在 `Hunger` 之后，false 时 0x00。
		// v31 新增：`DayPhaseOffset` 尾部 2 字节 u16 在 `SaturationZero` 之后，
		// 零值时是两个 0x00。
		{"hungry player state", protocol.StatePlay, protocol.PlayerState{Dimension: core.Overworld, Health: 15, Oxygen: core.MaxOxygenTicks, Hunger: 12, WorldTimeTicks: 24000}, 3, "00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000" + "0f" + "2c01" + "0c" + "00" + "0000" + "c05d000000000000"},
		{"command rejected", protocol.StatePlay, protocol.CommandRejected{Sequence: 7, Reason: protocol.RejectOccupied}, 4, "070000000000000006"},
		{"place block succeeded", protocol.StatePlay, protocol.PlaceBlockSucceeded{Sequence: 0x1122334455667788}, 20, "8877665544332211"},
		{"keep alive", protocol.StatePlay, protocol.KeepAlive{Token: 8}, 5, "0800000000000000"},
		{"disconnect", protocol.StatePlay, protocol.Disconnect{Code: protocol.DisconnectTimeout, Message: "bye"}, 6, "0203627965"},
		{"inventory state", protocol.StatePlay, goldenInventoryState(), 10, goldenInventoryStateHex()},
	}
	for _, tc := range servers {
		t.Run(tc.name, func(t *testing.T) {
			gotID, got, err := encodeServerControlPayload(tc.state, tc.packet)
			if err != nil || gotID != tc.wantID || hex.EncodeToString(got) != tc.wantHex {
				t.Fatalf("%T id=%d payload=%x err=%v", tc.packet, gotID, got, err)
			}
			round, err := decodeServerControlPayload(tc.state, gotID, got)
			if err != nil || !sameServerPacket(round, tc.packet) {
				t.Fatalf("round=%#v err=%v", round, err)
			}
			for length := 0; length < len(got); length++ {
				if _, err := decodeServerControlPayload(tc.state, gotID, got[:length]); err == nil {
					t.Fatalf("truncated %T at %d accepted", tc.packet, length)
				}
			}
		})
	}
}

func TestProtocolV2RemotePlayerGolden(t *testing.T) {
	id := mustCodecPlayerID(t)
	tests := []struct {
		packet  protocol.ServerPacket
		wantID  uint32
		wantHex string
	}{
		{protocol.RemotePlayerSpawn{PlayerID: id, DisplayName: "陈", ServerTick: 1, Dimension: core.Overworld, Position: mgl32.Vec3{1, 2, 3}, Yaw: 4, Pitch: -5}, 7, "00112233445546778899aabbccddeeff03e999880100000000000000000000000000803f0000004000004040000080400000a0c0"},
		{protocol.RemotePlayerDespawn{PlayerID: id}, 8, "00112233445546778899aabbccddeeff"},
		{protocol.RemotePlayerStates{ServerTick: 2, Players: []protocol.RemotePlayerState{{PlayerID: id, Dimension: core.Overworld, Position: mgl32.Vec3{1, 2, 3}, Yaw: 4, Pitch: -5, Reset: true}}}, 9, "02000000000000000100112233445546778899aabbccddeeff000000000000803f0000004000004040000080400000a0c001"},
	}
	for _, test := range tests {
		packetID, payload, err := encodeServerControlPayload(protocol.StatePlay, test.packet)
		if err != nil || packetID != test.wantID || hex.EncodeToString(payload) != test.wantHex {
			t.Fatalf("%T id=%d payload=%x err=%v", test.packet, packetID, payload, err)
		}
		decoded, err := decodeServerControlPayload(protocol.StatePlay, packetID, payload)
		if err != nil || !reflect.DeepEqual(decoded, test.packet) {
			t.Fatalf("round=%#v err=%v", decoded, err)
		}
	}
}

func TestSmallPacketErrorCodeWireValues(t *testing.T) {
	for _, tc := range []struct {
		packet protocol.ServerPacket
		want   string
	}{
		{protocol.HandshakeReject{ServerProtocolVersion: 8, Code: protocol.HandshakeVersionMismatch}, "080100"},
		{protocol.LoginReject{Code: protocol.LoginServerFull}, "0100"},
		{protocol.LoginReject{Code: protocol.LoginInvalidIdentity}, "0200"},
		{protocol.LoginReject{Code: protocol.LoginPlayerDataCorrupt}, "0300"},
		{protocol.LoginReject{Code: protocol.LoginStoreUnavailable}, "0400"},
		{protocol.LoginReject{Code: protocol.LoginProtocolViolation}, "0500"},
		{protocol.LoginReject{Code: protocol.LoginInternalError}, "0600"},
		{protocol.LoginReject{Code: protocol.LoginAlreadyOnline}, "0700"},
		{protocol.Disconnect{Code: protocol.DisconnectProtocolViolation}, "0100"},
		{protocol.Disconnect{Code: protocol.DisconnectTimeout}, "0200"},
		{protocol.Disconnect{Code: protocol.DisconnectServerShutdown}, "0300"},
		{protocol.Disconnect{Code: protocol.DisconnectSlowClient}, "0400"},
		{protocol.Disconnect{Code: protocol.DisconnectInternalError}, "0500"},
	} {
		state := protocol.StateLogin
		switch tc.packet.(type) {
		case protocol.HandshakeReject:
			state = protocol.StateHandshake
		case protocol.Disconnect:
			state = protocol.StatePlay
		}
		_, got, err := encodeServerControlPayload(state, tc.packet)
		if err != nil || hex.EncodeToString(got) != tc.want {
			t.Fatalf("%T payload=%x err=%v; want %s", tc.packet, got, err, tc.want)
		}
	}
}

func TestSmallPacketCommandRejectedReasonWireValues(t *testing.T) {
	tests := []struct {
		name    string
		reason  protocol.RejectReason
		wantHex string
	}{
		{"invalid ray", protocol.RejectInvalidRay, "010000000000000001"},
		{"no target", protocol.RejectNoTarget, "010000000000000002"},
		{"chunk not ready", protocol.RejectChunkNotReady, "010000000000000003"},
		{"protected block", protocol.RejectProtectedBlock, "010000000000000004"},
		{"invalid block", protocol.RejectInvalidBlock, "010000000000000005"},
		{"occupied", protocol.RejectOccupied, "010000000000000006"},
		{"invalid input", protocol.RejectInvalidInput, "010000000000000007"},
		{"player not ready", protocol.RejectPlayerNotReady, "010000000000000008"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			packet := protocol.CommandRejected{Sequence: 1, Reason: tc.reason}
			packetID, payload, err := encodeServerControlPayload(protocol.StatePlay, packet)
			if err != nil || packetID != 4 || hex.EncodeToString(payload) != tc.wantHex {
				t.Fatalf("encode payload=%x id=%d err=%v; want id=4 payload=%s", payload, packetID, err, tc.wantHex)
			}

			fixture, err := hex.DecodeString(tc.wantHex)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := decodeServerControlPayload(protocol.StatePlay, 4, fixture)
			if err != nil {
				t.Fatal(err)
			}
			got, ok := decoded.(protocol.CommandRejected)
			if !ok || got.Sequence != 1 || got.Reason != tc.reason {
				t.Fatalf("decode=%#v; want %#v", decoded, packet)
			}
		})
	}
}
