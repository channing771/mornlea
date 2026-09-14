package codec

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

// TestDepthsDimensionWireRoundtrip 钉死 v39 双维值域的 wire 事实：玩家与区块
// 类消息以 `Depths` 编码后必须能解码往返且维度保持为 `Depths`；越界维度仍由
// 既有拒绝矩阵覆盖，本测试只锁放行方向。
func TestDepthsDimensionWireRoundtrip(t *testing.T) {
	id := mustCodecPlayerID(t)
	t.Run("server control", func(t *testing.T) {
		packets := []protocol.ServerPacket{
			protocol.PlayerState{Dimension: core.Depths},
			protocol.BlockChanges{Dimension: core.Depths, BaseRevision: 1, NewRevision: 2},
			protocol.ForgetChunks{Dimension: core.Depths, Chunks: []core.ChunkPos{{X: 1, Z: -1}}},
			protocol.RemotePlayerSpawn{PlayerID: id, DisplayName: "Chen", Dimension: core.Depths},
			protocol.RemotePlayerStates{Players: []protocol.RemotePlayerState{{PlayerID: id, Dimension: core.Depths}}},
		}
		for _, packet := range packets {
			packetID, ok := protocol.ServerPacketID(protocol.StatePlay, packet)
			if !ok {
				t.Fatalf("%T 未注册 Play S→C 包 ID", packet)
			}
			_, payload, err := encodeServerControlPayload(protocol.StatePlay, packet)
			if err != nil {
				t.Fatalf("%T depths 编码被拒绝: %v", packet, err)
			}
			round, err := decodeServerControlPayload(protocol.StatePlay, packetID, payload)
			if err != nil {
				t.Fatalf("%T depths 解码被拒绝: %v", packet, err)
			}
			var dimension core.DimensionID
			switch message := round.(type) {
			case protocol.PlayerState:
				dimension = message.Dimension
			case protocol.BlockChanges:
				dimension = message.Dimension
			case protocol.ForgetChunks:
				dimension = message.Dimension
			case protocol.RemotePlayerSpawn:
				dimension = message.Dimension
			case protocol.RemotePlayerStates:
				dimension = message.Players[0].Dimension
			default:
				t.Fatalf("depths 往返得到意外类型 %T", round)
			}
			if dimension != core.Depths {
				t.Fatalf("%T 往返维度 = %d，想要 %d", packet, dimension, core.Depths)
			}
		}
	})
	t.Run("client resync", func(t *testing.T) {
		packet := protocol.RequestChunkResync{Sequence: 4, Dimension: core.Depths, Chunk: core.ChunkPos{X: -2, Z: 3}, HaveRevision: 5}
		packetID, ok := protocol.ClientPacketID(protocol.StatePlay, packet)
		if !ok {
			t.Fatal("RequestChunkResync 未注册 Play C→S 包 ID")
		}
		_, payload, err := encodeClientPacketPayload(protocol.StatePlay, packet)
		if err != nil {
			t.Fatalf("depths resync 编码被拒绝: %v", err)
		}
		round, err := decodeClientPacketPayload(protocol.StatePlay, packetID, payload)
		if err != nil {
			t.Fatalf("depths resync 解码被拒绝: %v", err)
		}
		if resync, ok := round.(protocol.RequestChunkResync); !ok || resync.Dimension != core.Depths {
			t.Fatalf("depths resync 往返 = %#v", round)
		}
	})
	t.Run("snapshot", func(t *testing.T) {
		codec := mustNewCodec(t)
		defer codec.Close()
		snapshot := fixtureSnapshot(core.ChunkPos{X: -3, Z: 7}, 19)
		snapshot.Dimension = core.Depths
		_, payload, err := codec.EncodeServer(protocol.StatePlay, snapshot)
		if err != nil {
			t.Fatalf("depths snapshot 编码被拒绝: %v", err)
		}
		round, err := codec.DecodeServer(protocol.StatePlay, 0, payload)
		if err != nil {
			t.Fatalf("depths snapshot 解码被拒绝: %v", err)
		}
		got, ok := round.(protocol.ChunkSnapshot)
		if !ok || got.Dimension != core.Depths {
			t.Fatalf("depths snapshot 往返 = %#v", round)
		}
	})
}
