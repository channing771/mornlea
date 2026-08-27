package network_test

import (
	"context"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
	networktcp "github.com/channing771/mornlea/internal/network/tcp"
)

var (
	benchmarkPacket    any
	benchmarkPayload   []byte
	benchmarkByteCount int
)

func BenchmarkSmallPacketCodec(b *testing.B) {
	id := core.PlayerID{0xb3, 0x13, 0x61, 0xa7, 0xc8, 0x62, 0x45, 0x97, 0x92, 0x13, 0xc3, 0xac, 0xd4, 0x68, 0xd1, 0x18}
	clientPackets := []struct {
		name  string
		state network.State
		value network.ClientPacket
	}{
		{"ClientHello", network.StateHandshake, network.ClientHello{ProtocolVersion: network.ProtocolVersion}},
		{"LoginStart", network.StateLogin, network.LoginStart{PlayerID: id, DisplayName: "Benchmark"}},
		{"PlayerInput", network.StatePlay, network.PlayerInput{Sequence: 1, MoveX: 1, MoveZ: -1, Jump: true, Yaw: 0.5, Pitch: -0.25, Mining: true}},
		{"PlaceBlock", network.StatePlay, network.PlaceBlock{Sequence: 3, Yaw: 0.5, Pitch: -0.25, Slot: 2}},
		{"RequestChunkResync", network.StatePlay, network.RequestChunkResync{Sequence: 4, Dimension: core.Overworld, Chunk: core.ChunkPos{X: -2, Z: 3}, HaveRevision: 7}},
		{"KeepAliveReply", network.StatePlay, network.KeepAliveReply{Token: 9}},
		{"DropSelectedItem", network.StatePlay, network.DropSelectedItem{Sequence: 12}},
	}
	serverPackets := []struct {
		name  string
		state network.State
		value network.ServerPacket
	}{
		{"ServerHello", network.StateHandshake, network.ServerHello{ProtocolVersion: network.ProtocolVersion}},
		{"HandshakeReject", network.StateHandshake, network.HandshakeReject{ServerProtocolVersion: network.ProtocolVersion, Code: network.HandshakeVersionMismatch, Message: "mismatch"}},
		{"LoginSuccess", network.StateLogin, network.LoginSuccess{PlayerID: id}},
		{"LoginReject", network.StateLogin, network.LoginReject{Code: network.LoginServerFull, Message: "full"}},
		{"BlockChanges", network.StatePlay, network.BlockChanges{Dimension: core.Overworld, Chunk: core.ChunkPos{}, BaseRevision: 1, NewRevision: 2, Changes: []network.BlockChange{{Position: core.BlockPos{X: 1, Y: 2, Z: 3}, Block: core.DirtID}}}},
		{"ForgetChunks", network.StatePlay, network.ForgetChunks{Dimension: core.Overworld, Chunks: []core.ChunkPos{{X: -1, Z: 2}}}},
		{"PlayerState", network.StatePlay, network.PlayerState{ServerTick: 5, LastInputSequence: 4, Dimension: core.Overworld, Position: mgl32.Vec3{1, 2, 3}, Velocity: mgl32.Vec3{0.1, 0.2, 0.3}, Ready: true, MiningActive: true, MiningTarget: core.BlockPos{X: 1, Y: 2, Z: 3}, MiningProgressTicks: 6, MiningRequiredTicks: 15, MiningHarvestable: true}},
		{"CommandRejected", network.StatePlay, network.CommandRejected{Sequence: 6, Reason: network.RejectNoTarget}},
		{"KeepAlive", network.StatePlay, network.KeepAlive{Token: 7}},
		{"Disconnect", network.StatePlay, network.Disconnect{Code: network.DisconnectServerShutdown, Message: "shutdown"}},
		{"PlaceBlockSucceeded", network.StatePlay, network.PlaceBlockSucceeded{Sequence: 13}},
	}
	for _, fixture := range clientPackets {
		b.Run("EncodeClient/"+fixture.name, func(b *testing.B) {
			codec := benchmarkCodec(b)
			defer codec.Close()
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				_, payload, err := codec.EncodeClient(fixture.state, fixture.value)
				if err != nil {
					b.Fatal(err)
				}
				benchmarkPayload = payload
			}
		})
		b.Run("DecodeClient/"+fixture.name, func(b *testing.B) {
			codec := benchmarkCodec(b)
			defer codec.Close()
			packetID, payload, err := codec.EncodeClient(fixture.state, fixture.value)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				packet, err := codec.DecodeClient(fixture.state, packetID, payload)
				if err != nil {
					b.Fatal(err)
				}
				benchmarkPacket = packet
			}
		})
	}
	for _, fixture := range serverPackets {
		b.Run("EncodeServer/"+fixture.name, func(b *testing.B) {
			codec := benchmarkCodec(b)
			defer codec.Close()
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				_, payload, err := codec.EncodeServer(fixture.state, fixture.value)
				if err != nil {
					b.Fatal(err)
				}
				benchmarkPayload = payload
			}
		})
		b.Run("DecodeServer/"+fixture.name, func(b *testing.B) {
			codec := benchmarkCodec(b)
			defer codec.Close()
			packetID, payload, err := codec.EncodeServer(fixture.state, fixture.value)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				packet, err := codec.DecodeServer(fixture.state, packetID, payload)
				if err != nil {
					b.Fatal(err)
				}
				benchmarkPacket = packet
			}
		})
	}
}

func BenchmarkRemotePlayerStateCodec(b *testing.B) {
	players := make([]network.RemotePlayerState, 7)
	for index := range players {
		players[index] = network.RemotePlayerState{
			PlayerID:  core.PlayerID{0, 0, 0, byte(index + 1), 0, 0, 0x40, byte(index + 1), 0x80, 0, 0, 0, 0, 0, 0, byte(index + 1)},
			Dimension: core.Overworld,
			Position:  mgl32.Vec3{float32(index), 80, float32(-index)},
			Yaw:       float32(index) * 0.1,
			Pitch:     float32(index) * -0.01,
		}
	}
	packet := network.RemotePlayerStates{ServerTick: 42, Players: players}
	codec := benchmarkCodec(b)
	defer codec.Close()
	packetID, payload, err := codec.EncodeServer(network.StatePlay, packet)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("Encode", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, encoded, err := codec.EncodeServer(network.StatePlay, packet)
			if err != nil {
				b.Fatal(err)
			}
			benchmarkPayload = encoded
		}
	})
	b.Run("Decode", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			decoded, err := codec.DecodeServer(network.StatePlay, packetID, payload)
			if err != nil {
				b.Fatal(err)
			}
			benchmarkPacket = decoded
		}
	})
}

func BenchmarkChatCommandCodec(b *testing.B) {
	packet := network.ChatCommand{Text: strings.Repeat("x", 1024)}
	codec := benchmarkCodec(b)
	defer codec.Close()
	packetID, payload, err := codec.EncodeClient(network.StatePlay, packet)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("Encode", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, encoded, encodeErr := codec.EncodeClient(network.StatePlay, packet)
			if encodeErr != nil {
				b.Fatal(encodeErr)
			}
			benchmarkPayload = encoded
		}
	})
	b.Run("Decode", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			decoded, decodeErr := codec.DecodeClient(network.StatePlay, packetID, payload)
			if decodeErr != nil {
				b.Fatal(decodeErr)
			}
			benchmarkPacket = decoded
		}
	})
}

func BenchmarkCompanionMessageCodec(b *testing.B) {
	states := make([]network.CompanionState, 4)
	for index := range states {
		states[index] = network.CompanionState{ID: testCompanionID(byte(index + 1)), Dimension: core.Overworld}
	}
	packets := []struct {
		name   string
		packet network.ServerPacket
	}{
		{"ChatEvent", validAcceptedChatEvent()},
		{"Spawn", network.CompanionSpawn{ID: testCompanionID(1), Name: "A", Dimension: core.Overworld}},
		{"States", network.CompanionStates{States: states}},
		{"Despawn", network.CompanionDespawn{ID: testCompanionID(1)}},
	}
	for _, test := range packets {
		b.Run(test.name, func(b *testing.B) {
			codec := benchmarkCodec(b)
			defer codec.Close()
			packetID, payload, err := codec.EncodeServer(network.StatePlay, test.packet)
			if err != nil {
				b.Fatal(err)
			}
			b.Run("Encode", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					_, encoded, encodeErr := codec.EncodeServer(network.StatePlay, test.packet)
					if encodeErr != nil {
						b.Fatal(encodeErr)
					}
					benchmarkPayload = encoded
				}
			})
			b.Run("Decode", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					decoded, decodeErr := codec.DecodeServer(network.StatePlay, packetID, payload)
					if decodeErr != nil {
						b.Fatal(decodeErr)
					}
					benchmarkPacket = decoded
				}
			})
		})
	}
}

func BenchmarkWorstLegalChunkSnapshot(b *testing.B) {
	snapshot := worstLegalBenchmarkSnapshot()
	logical := make([]byte, logicalSnapshotSize(snapshot))
	codec := benchmarkCodec(b)
	defer codec.Close()
	packetID, payload, err := codec.EncodeServer(network.StatePlay, snapshot)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("Encode", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		b.ReportMetric(float64(len(logical))/float64(len(payload)), "compression-ratio")
		b.ReportMetric(float64(len(logical)), "logical-bytes")
		b.ReportMetric(float64(len(payload)), "wire-bytes")
		for range b.N {
			_, encoded, err := codec.EncodeServer(network.StatePlay, snapshot)
			if err != nil {
				b.Fatal(err)
			}
			benchmarkPayload = encoded
		}
	})
	b.Run("Decode", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		b.ReportMetric(float64(len(logical))/float64(len(payload)), "compression-ratio")
		b.ReportMetric(float64(len(logical)), "logical-bytes")
		b.ReportMetric(float64(len(payload)), "wire-bytes")
		for range b.N {
			decoded, err := codec.DecodeServer(network.StatePlay, packetID, payload)
			if err != nil {
				b.Fatal(err)
			}
			benchmarkPacket = decoded
		}
	})
}

func BenchmarkTCPLoopbackPlayerInput(b *testing.B) {
	client, server, closePair := benchmarkTCPPair(b)
	defer closePair()
	ctx := context.Background()
	packet := network.PlayerInput{Sequence: 1, MoveX: 1, MoveZ: -1, Yaw: 0.5, Pitch: -0.25, Mining: true}
	received := make(chan error, 1)
	go func() {
		for range b.N {
			value, err := server.Recv(ctx, network.StatePlay)
			if err != nil {
				received <- err
				return
			}
			benchmarkPacket = value
		}
		received <- nil
	}()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if err := client.Send(ctx, network.StatePlay, packet); err != nil {
			b.Fatal(err)
		}
		packet.Sequence++
	}
	if err := <-received; err != nil {
		b.Fatal(err)
	}
}

func BenchmarkTCPLoopbackChunkSnapshot(b *testing.B) {
	client, server, closePair := benchmarkTCPPair(b)
	defer closePair()
	ctx := context.Background()
	snapshot := worstLegalBenchmarkSnapshot()
	codec := benchmarkCodec(b)
	_, payload, err := codec.EncodeServer(network.StatePlay, snapshot)
	_ = codec.Close()
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(payload)))
	received := make(chan error, 1)
	go func() {
		for range b.N {
			value, err := client.Recv(ctx, network.StatePlay)
			if err != nil {
				received <- err
				return
			}
			benchmarkPacket = value
		}
		received <- nil
	}()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if err := server.Send(ctx, network.StatePlay, snapshot); err != nil {
			b.Fatal(err)
		}
	}
	if err := <-received; err != nil {
		b.Fatal(err)
	}
}

func benchmarkCodec(b *testing.B) *network.Codec {
	b.Helper()
	codec, err := network.NewCodec()
	if err != nil {
		b.Fatal(err)
	}
	return codec
}

func worstLegalBenchmarkSnapshot() network.ChunkSnapshot {
	sections := make([]network.SectionData, core.SectionsPerChunk)
	valuesPerWord := 64 / 15
	words := (core.BlocksPerSection + valuesPerWord - 1) / valuesPerWord
	for section := range sections {
		packed := make([]uint64, words)
		for index := 0; index < core.BlocksPerSection; index++ {
			value := uint64((section + index) % int(core.MossyCobblestoneID+1))
			packed[index/valuesPerWord] |= value << uint((index%valuesPerWord)*15)
		}
		sections[section] = network.SectionData{Y: int32(section), Storage: network.SectionDirect, Bits: 15, Packed: packed}
	}
	return network.ChunkSnapshot{Dimension: core.Overworld, Chunk: core.ChunkPos{X: -3, Z: 7}, Revision: 19, Sections: sections}
}

func benchmarkTCPPair(b *testing.B) (network.ClientPacketStream, network.ServerPacketStream, func()) {
	b.Helper()
	listener, err := networktcp.ListenTCP("127.0.0.1:0")
	if err != nil {
		b.Fatal(err)
	}
	type acceptResult struct {
		stream network.ServerPacketStream
		err    error
	}
	accepted := make(chan acceptResult, 1)
	go func() {
		stream, err := listener.Accept(context.Background())
		accepted <- acceptResult{stream: stream, err: err}
	}()
	client, err := networktcp.DialTCP(context.Background(), listener.Addr())
	if err != nil {
		_ = listener.Close()
		b.Fatal(err)
	}
	server := <-accepted
	if server.err != nil {
		_ = client.Close()
		_ = listener.Close()
		b.Fatal(server.err)
	}
	return client, server.stream, func() {
		_ = client.Close()
		_ = server.stream.Close()
		_ = listener.Close()
	}
}

func validAcceptedChatEvent() network.ChatEvent {
	return network.ChatEvent{
		EventID: 1, PlayerID: core.PlayerID(testCompanionID(9)), PlayerName: "Chen",
		CompanionID: testCompanionID(1), CompanionName: "A",
		Kind: network.ChatEventAccepted, RejectReason: network.ChatRejectNone, Command: "x",
	}
}

func testCompanionID(last byte) companion.ID {
	return companion.ID{0: 0x10, 6: 0x40, 8: 0x80, 15: last}
}

func logicalSnapshotSize(snapshot network.ChunkSnapshot) int {
	size := 20 + canonicalUvarintLength(uint32(len(snapshot.Sections)))
	for _, section := range snapshot.Sections {
		size += 2 + section.PayloadBytes()
		switch section.Storage {
		case network.SectionIndexed:
			size += 1 + canonicalUvarintLength(uint32(len(section.Palette))) +
				canonicalUvarintLength(uint32(len(section.Packed)))
		case network.SectionDirect:
			size += 1 + canonicalUvarintLength(uint32(len(section.Packed)))
		}
	}
	return size
}

func canonicalUvarintLength(value uint32) int {
	var encoded [binary.MaxVarintLen64]byte
	return binary.PutUvarint(encoded[:], uint64(value))
}
