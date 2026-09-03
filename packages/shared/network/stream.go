package network

import (
	"context"

	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

type ClientPacketStream interface {
	Send(context.Context, protocol.State, protocol.ClientPacket) error
	Recv(context.Context, protocol.State) (protocol.ServerPacket, error)
	Close() error
}

type ServerPacketStream interface {
	Send(context.Context, protocol.State, protocol.ServerPacket) error
	Recv(context.Context, protocol.State) (protocol.ClientPacket, error)
	Peer() string
	Close() error
}

type Listener interface {
	Accept(context.Context) (ServerPacketStream, error)
	Addr() string
	Close() error
}
