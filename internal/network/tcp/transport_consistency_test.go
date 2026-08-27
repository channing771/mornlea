package tcp

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
)

var transportOpeners = []struct {
	name string
	open func(*testing.T) (network.ClientPacketStream, network.ServerPacketStream)
}{
	{"memory", func(t *testing.T) (network.ClientPacketStream, network.ServerPacketStream) {
		return network.NewMemoryStreamPair(8)
	}},
	{"tcp", openTCPStreamPair},
}

func TestProtocolTranscriptSuccessMatchesMemoryAndTCP(t *testing.T) {
	for _, open := range transportOpeners {
		t.Run(open.name, func(t *testing.T) {
			clientStream, serverStream := open.open(t)
			t.Cleanup(func() { _ = clientStream.Close(); _ = serverStream.Close() })
			serverDone := make(chan error, 1)
			go func() {
				pending, err := network.BeginServerLogin(context.Background(), serverStream, 0)
				if err != nil {
					serverDone <- err
					return
				}
				var endpoint network.ServerEndpoint
				err = pending.Accept(context.Background(), func(attached network.ServerEndpoint) error {
					endpoint = attached
					return nil
				})
				if err == nil {
					// 饥饿值取非零非满的 12：这条 transcript 用整结构相等比较
					// （下面的 `packet != (...)`），带一个中间值才能真正锁住
					// v24 新增字段在两种传输上逐字段一致——满值或 0 与
					// "字段根本没搬运" 不可分辨。
					err = endpoint.Send(context.Background(), network.PlayerState{Ready: true, Hunger: 12})
				}
				serverDone <- err
			}()

			endpoint, err := network.LoginClient(context.Background(), clientStream, testIdentity(11))
			if err != nil {
				t.Fatal(err)
			}
			packet, err := endpoint.Recv(context.Background())
			if err != nil || packet != (network.PlayerState{Ready: true, Hunger: 12}) {
				t.Fatalf("play transcript = (%+v, %v)", packet, err)
			}
			if err := <-serverDone; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestProtocolTranscriptRejectMatchesMemoryAndTCP(t *testing.T) {
	for _, open := range transportOpeners {
		t.Run(open.name, func(t *testing.T) {
			clientStream, serverStream := open.open(t)
			t.Cleanup(func() { _ = clientStream.Close(); _ = serverStream.Close() })
			serverDone := make(chan error, 1)
			go func() {
				pending, err := network.BeginServerLogin(context.Background(), serverStream, 0)
				if err != nil {
					serverDone <- err
					return
				}
				serverDone <- pending.Reject(context.Background(), network.LoginServerFull, "server full")
			}()

			_, err := network.LoginClient(context.Background(), clientStream, testIdentity(12))
			var remote *network.RemoteError
			if !errors.As(err, &remote) || remote.State != network.StateLogin || remote.Code != uint8(network.LoginServerFull) || remote.Message != "server full" {
				t.Fatalf("reject transcript = %#v", err)
			}
			if err := <-serverDone; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestProtocolTranscriptRejectsEarlyPlayAcrossMemoryAndTCP(t *testing.T) {
	for _, open := range transportOpeners {
		t.Run(open.name, func(t *testing.T) {
			client, server := open.open(t)
			t.Cleanup(func() { _ = client.Close(); _ = server.Close() })
			serverDone := make(chan error, 1)
			go func() {
				if _, err := server.Recv(context.Background(), network.StateHandshake); err != nil {
					serverDone <- err
					return
				}
				serverDone <- server.Send(context.Background(), network.StatePlay, network.PlayerState{})
			}()

			_, err := network.LoginClient(context.Background(), client, testIdentity(13))
			if err == nil || !strings.Contains(err.Error(), "protocol violation") {
				t.Fatalf("early Play transcript error = %v", err)
			}
			if err := <-serverDone; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPlaySemanticValidationMatchesMemoryAndTCP(t *testing.T) {
	packets := []struct {
		name   string
		packet network.ClientPacket
	}{
		{"place block slot out of range", network.PlaceBlock{Slot: core.HotbarSlots}},
		{"resync outside overworld", network.RequestChunkResync{Dimension: core.DimensionID(1)}},
	}
	for _, packet := range packets {
		t.Run(packet.name, func(t *testing.T) {
			for _, transport := range transportOpeners {
				t.Run(transport.name, func(t *testing.T) {
					client, server := transport.open(t)
					t.Cleanup(func() { _ = client.Close(); _ = server.Close() })
					if err := client.Send(context.Background(), network.StatePlay, packet.packet); err == nil {
						t.Fatalf("%s accepted invalid %T", transport.name, packet.packet)
					}
				})
			}
		})
	}
}

func TestCommonBlockMaterialPlayTranscriptMatchesMemoryAndTCP(t *testing.T) {
	snapshot := repeatedSnapshot(network.SectionData{Storage: network.SectionSingle, Single: core.AirID})
	snapshot.Sections[0].Single = core.MossyCobblestoneID
	var inventory core.Inventory
	inventory.Backpack[0] = core.ItemStack{
		Item: core.ItemMossyCobblestone, Count: core.MaxStackCount,
	}
	want := []network.ServerMessage{snapshot, network.InventoryState{Inventory: inventory}}

	for _, open := range transportOpeners {
		t.Run(open.name, func(t *testing.T) {
			clientStream, serverStream := open.open(t)
			t.Cleanup(func() { _ = clientStream.Close(); _ = serverStream.Close() })
			serverDone := make(chan error, 1)
			go func() {
				pending, err := network.BeginServerLogin(context.Background(), serverStream, 0)
				if err != nil {
					serverDone <- err
					return
				}
				var endpoint network.ServerEndpoint
				err = pending.Accept(context.Background(), func(attached network.ServerEndpoint) error {
					endpoint = attached
					return nil
				})
				for _, message := range want {
					if err == nil {
						err = endpoint.Send(context.Background(), message)
					}
				}
				serverDone <- err
			}()

			endpoint, err := network.LoginClient(context.Background(), clientStream, testIdentity(14))
			if err != nil {
				t.Fatal(err)
			}
			for index, message := range want {
				got, err := endpoint.Recv(context.Background())
				if err != nil || !reflect.DeepEqual(got, message) {
					t.Fatalf("Play 消息 %d = (%#v, %v)，想要 %#v", index, got, err, message)
				}
			}
			if err := <-serverDone; err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestGridCraftingTranscriptMatchesMemoryAndTCP 锁死格子工作台三条 Play 消息
// 在 Memory 与 TCP 两种传输上的逐字段一致：客户端发出 `MoveCraftingStack` 与
// `TakeCraftingOutput`，服务端原样收到；服务端发出完整 `CraftingState`，
// 客户端原样收到。TCP 路径额外覆盖新 packet 的 frame/codec 往返。
func TestGridCraftingTranscriptMatchesMemoryAndTCP(t *testing.T) {
	commands := []network.ClientMessage{
		network.MoveCraftingStack{Sequence: 0x0102030405060708, From: 9, To: 0},
		network.MoveCraftingStack{Sequence: 2, From: 0, To: 1},
		network.TakeCraftingOutput{Sequence: 3},
	}
	var grid network.CraftingState
	grid.Size = 3
	grid.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 2}
	grid.Output = core.ItemStack{Item: core.ItemStoneBrick, Count: 4}

	for _, open := range transportOpeners {
		t.Run(open.name, func(t *testing.T) {
			clientStream, serverStream := open.open(t)
			t.Cleanup(func() { _ = clientStream.Close(); _ = serverStream.Close() })
			serverDone := make(chan error, 1)
			go func() {
				pending, err := network.BeginServerLogin(context.Background(), serverStream, 0)
				if err != nil {
					serverDone <- err
					return
				}
				var endpoint network.ServerEndpoint
				err = pending.Accept(context.Background(), func(attached network.ServerEndpoint) error {
					endpoint = attached
					return nil
				})
				for _, command := range commands {
					if err == nil {
						var got network.ClientMessage
						got, err = endpoint.Recv(context.Background())
						if err == nil && got != command {
							err = fmt.Errorf("服务端收到 %T=%+v，想要 %+v", got, got, command)
						}
					}
				}
				if err == nil {
					err = endpoint.Send(context.Background(), grid)
				}
				serverDone <- err
			}()

			endpoint, err := network.LoginClient(context.Background(), clientStream, testIdentity(15))
			if err != nil {
				t.Fatal(err)
			}
			for _, command := range commands {
				if err := endpoint.Send(context.Background(), command); err != nil {
					t.Fatalf("发送 %T: %v", command, err)
				}
			}
			got, err := endpoint.Recv(context.Background())
			if err != nil || got != (network.ServerMessage)(grid) {
				t.Fatalf("网格状态 = (%+v, %v)，想要 %+v", got, err, grid)
			}
			if err := <-serverDone; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestProtocolOutdatedHandshakeRejectMatchesMemoryAndTCP(t *testing.T) {
	// v15、v16 与 v17 都是过时版本：Memory 与 TCP 必须产生相同的版本不匹配
	// 拒绝(变基重编:WorldSeed 段由 v18 重编为 v23,v22 为 authoritative-farming
	// 已交付的翻地命令段,对本客户端同样是被拒的过时版本)。
	//
	// `ProtocolVersion - 1` 是**刚退役的那一版**，写成表达式而不是字面量：
	// 只列远古版本的话，升版当次退役的版本在两种传输上的拒绝行为无人覆盖
	// （v26 退役 v25 时正是这个缺口）。
	for _, version := range []uint32{15, 16, 17, network.ProtocolVersion - 1} {
		for _, open := range transportOpeners {
			t.Run(fmt.Sprintf("v%d/%s", version, open.name), func(t *testing.T) {
				client, server := openProtocolStreamPair(t, open)
				t.Cleanup(func() { _ = client.Close(); _ = server.Close() })
				serverDone := make(chan error, 1)
				go func() {
					_, err := network.BeginServerLogin(context.Background(), server, 0)
					serverDone <- err
				}()

				sendProtocolVersionHello(t, client, version)
				packet, err := client.Recv(context.Background(), network.StateHandshake)
				reject, ok := packet.(network.HandshakeReject)
				if err != nil || !ok || reject.Code != network.HandshakeVersionMismatch || reject.ServerProtocolVersion != network.ProtocolVersion {
					t.Fatalf("v%d 拒绝 = (%#v, %v)，想要服务端 v%d HandshakeVersionMismatch", version, packet, err, network.ProtocolVersion)
				}
				if err := <-serverDone; err == nil {
					t.Fatalf("v%d 握手意外进入登录", version)
				}
			})
		}
	}
}

func TestV26ClientRejectsPriorServerAcrossTransports(t *testing.T) {
	legacyVersions := []uint32{17, network.ProtocolVersion - 1}
	for _, legacy := range legacyVersions {
		for _, open := range transportOpeners {
			t.Run(fmt.Sprintf("v%d/%s", legacy, open.name), func(t *testing.T) {
				client, server := openLegacyServerPair(t, open)
				t.Cleanup(func() { _ = client.Close(); _ = server.Close() })
				serverDone := make(chan error, 1)
				go func() {
					if _, err := server.Recv(context.Background(), network.StateHandshake); err != nil {
						serverDone <- err
						return
					}
					serverDone <- sendRawServerHello(server, legacy)
				}()

				_, err := network.LoginClient(context.Background(), client, testIdentity(23))
				wantVersion := fmt.Sprintf("server protocol version %d", legacy)
				if err == nil || !strings.Contains(err.Error(), "protocol violation") ||
					!(strings.Contains(err.Error(), wantVersion) ||
						strings.Contains(err.Error(), "server handshake version mismatch")) {
					t.Fatalf("v%d 服务端登录结果 = %v，想要握手版本不匹配拒绝", legacy, err)
				}
				if err := <-serverDone; err != nil {
					t.Fatal(err)
				}
				if err := client.Send(context.Background(), network.StatePlay, network.PlayerInput{}); err == nil {
					t.Fatal("握手版本不匹配后客户端仍能发送 Play 包")
				}
			})
		}
	}
}

func openLegacyServerPair(t *testing.T, opener struct {
	name string
	open func(*testing.T) (network.ClientPacketStream, network.ServerPacketStream)
}) (network.ClientPacketStream, network.ServerPacketStream) {
	t.Helper()
	if opener.name == "memory" {
		return newRawMemoryStreamPair()
	}
	return opener.open(t)
}

func sendRawServerHello(server network.ServerPacketStream, version uint32) error {
	switch server := server.(type) {
	case *rawMemoryServerStream:
		return server.Send(context.Background(), network.StateHandshake, network.ServerHello{ProtocolVersion: version})
	case *tcpServerStream:
		return network.WriteFrame(server.stream.conn, 0, []byte{byte(version)})
	default:
		return fmt.Errorf("未知 server stream %T", server)
	}
}

func sendProtocolVersionHello(t *testing.T, client network.ClientPacketStream, version uint32) {
	t.Helper()
	switch client := client.(type) {
	case *rawMemoryClientStream:
		if err := client.Send(t.Context(), network.StateHandshake, network.ClientHello{ProtocolVersion: version}); err != nil {
			t.Fatal(err)
		}
	case *tcpClientStream:
		if err := network.WriteFrame(client.stream.conn, 0, []byte{byte(version)}); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("未知 client stream %T", client)
	}
}

func openProtocolStreamPair(t *testing.T, opener struct {
	name string
	open func(*testing.T) (network.ClientPacketStream, network.ServerPacketStream)
}) (network.ClientPacketStream, network.ServerPacketStream) {
	t.Helper()
	if opener.name == "memory" {
		return newRawMemoryStreamPair()
	}
	return opener.open(t)
}

func openTCPStreamPair(t *testing.T) (network.ClientPacketStream, network.ServerPacketStream) {
	t.Helper()
	listener, err := ListenTCP("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	clientDone := make(chan struct {
		stream network.ClientPacketStream
		err    error
	}, 1)
	go func() {
		stream, err := DialTCP(context.Background(), listener.Addr())
		clientDone <- struct {
			stream network.ClientPacketStream
			err    error
		}{stream, err}
	}()
	server, err := listener.Accept(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	client := <-clientDone
	if client.err != nil {
		t.Fatal(client.err)
	}
	return client.stream, server
}

func testIdentity(last byte) network.Identity {
	return network.Identity{
		PlayerID:    core.PlayerID{0, 1, 2, 3, 4, 5, 0x46, 7, 0x88, 9, 10, 11, 12, 13, 14, last},
		DisplayName: "Chen",
	}
}

func repeatedSnapshot(section network.SectionData) network.ChunkSnapshot {
	sections := make([]network.SectionData, core.SectionsPerChunk)
	for index := range sections {
		section.Y = int32(index)
		sections[index] = section
	}
	return network.ChunkSnapshot{Dimension: core.Overworld, Revision: 1, Sections: sections}
}

func TestCompanionMessagesMatchMemoryAndTCP(t *testing.T) {
	clientMessage := network.ChatCommand{Text: "@A x"}
	serverMessages := []network.ServerPacket{
		validAcceptedChatEvent(),
		network.CompanionSpawn{ID: testCompanionID(1), Name: "A", Tick: 1, Dimension: core.Overworld},
		network.CompanionStates{Tick: 2, States: []network.CompanionState{{ID: testCompanionID(1), Dimension: core.Overworld}}},
		network.CompanionDespawn{ID: testCompanionID(1)},
	}
	for _, open := range transportOpeners {
		t.Run(open.name, func(t *testing.T) {
			client, server := open.open(t)
			t.Cleanup(func() { _ = client.Close(); _ = server.Close() })
			if err := client.Send(context.Background(), network.StatePlay, clientMessage); err != nil {
				t.Fatal(err)
			}
			gotClient, err := server.Recv(context.Background(), network.StatePlay)
			if err != nil || !reflect.DeepEqual(gotClient, clientMessage) {
				t.Fatalf("client message = (%#v,%v)", gotClient, err)
			}
			for _, message := range serverMessages {
				if err := server.Send(context.Background(), network.StatePlay, message); err != nil {
					t.Fatal(err)
				}
				got, err := client.Recv(context.Background(), network.StatePlay)
				if err != nil || !reflect.DeepEqual(got, message) {
					t.Fatalf("server message = (%#v,%v), 想要 %#v", got, err, message)
				}
			}
		})
	}
}

func TestCompanionStatesFiveRejectedByMemoryAndTCP(t *testing.T) {
	five := make([]network.CompanionState, 5)
	for index := range five {
		five[index] = network.CompanionState{ID: testCompanionID(byte(index + 1)), Dimension: core.Overworld}
	}
	for _, open := range transportOpeners {
		t.Run(open.name, func(t *testing.T) {
			client, server := open.open(t)
			t.Cleanup(func() { _ = client.Close(); _ = server.Close() })
			if err := server.Send(context.Background(), network.StatePlay, network.CompanionStates{States: five}); err == nil {
				t.Fatal("五项 CompanionStates 被 transport 接受")
			}
		})
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

type rawMemoryPair struct {
	clientToServer chan network.ClientPacket
	serverToClient chan network.ServerPacket
	done           chan struct{}
	closeOnce      sync.Once
}

type rawMemoryClientStream struct{ pair *rawMemoryPair }

type rawMemoryServerStream struct{ pair *rawMemoryPair }

func newRawMemoryStreamPair() (network.ClientPacketStream, network.ServerPacketStream) {
	pair := &rawMemoryPair{
		clientToServer: make(chan network.ClientPacket, 1),
		serverToClient: make(chan network.ServerPacket, 1),
		done:           make(chan struct{}),
	}
	return &rawMemoryClientStream{pair: pair}, &rawMemoryServerStream{pair: pair}
}

func (stream *rawMemoryClientStream) Send(ctx context.Context, _ network.State, packet network.ClientPacket) error {
	select {
	case <-stream.pair.done:
		return network.ErrClosed
	default:
	}
	select {
	case stream.pair.clientToServer <- packet:
		return nil
	case <-stream.pair.done:
		return network.ErrClosed
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (stream *rawMemoryClientStream) Recv(ctx context.Context, _ network.State) (network.ServerPacket, error) {
	select {
	case packet := <-stream.pair.serverToClient:
		return packet, nil
	case <-stream.pair.done:
		return nil, network.ErrClosed
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (stream *rawMemoryClientStream) Close() error {
	stream.pair.close()
	return nil
}

func (stream *rawMemoryServerStream) Send(ctx context.Context, _ network.State, packet network.ServerPacket) error {
	select {
	case <-stream.pair.done:
		return network.ErrClosed
	default:
	}
	select {
	case stream.pair.serverToClient <- packet:
		return nil
	case <-stream.pair.done:
		return network.ErrClosed
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (stream *rawMemoryServerStream) Recv(ctx context.Context, _ network.State) (network.ClientPacket, error) {
	select {
	case packet := <-stream.pair.clientToServer:
		return packet, nil
	case <-stream.pair.done:
		return nil, network.ErrClosed
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (*rawMemoryServerStream) Peer() string { return "memory" }
func (stream *rawMemoryServerStream) Close() error {
	stream.pair.close()
	return nil
}

func (pair *rawMemoryPair) close() {
	pair.closeOnce.Do(func() { close(pair.done) })
}
