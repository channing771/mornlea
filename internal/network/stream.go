package network

import "context"

type ClientPacketStream interface {
	Send(context.Context, State, ClientPacket) error
	Recv(context.Context, State) (ServerPacket, error)
	Close() error
}

type ServerPacketStream interface {
	Send(context.Context, State, ServerPacket) error
	Recv(context.Context, State) (ClientPacket, error)
	Peer() string
	Close() error
}

type Listener interface {
	Accept(context.Context) (ServerPacketStream, error)
	Addr() string
	Close() error
}
